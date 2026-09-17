package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
	"github.com/loadify/backend/pkg/secret"
)

// TelegramConsentVersion is the version of the text a person agrees to before
// connecting an account. Changing that text means changing this value, so a
// record of consent always says what was agreed to.
const TelegramConsentVersion = "2026-09"

const (
	// loginTTL is how long a started sign-in waits for its code. Telegram's own
	// codes expire on a similar scale.
	loginTTL = 10 * time.Minute
	// maxLoginAttempts bounds guesses at a code or a password. Telegram limits
	// them too; this keeps a mistyping person from reaching its limit.
	maxLoginAttempts = 5
)

// Login steps, as the client sees them.
const (
	loginStepCode      = "code"
	loginStepPassword  = "password"
	loginStepConnected = "connected"
)

// TelegramUsecase connects a person's own Telegram account and manages the
// groups it posts to.
//
// Nothing here posts anything. It is the part of the product a person touches
// before the first advert goes out, and the part they come back to when
// something needs changing.
type TelegramUsecase struct {
	accounts domain.TelegramAccountRepository
	groups   domain.TelegramGroupRepository
	posting  domain.PostingRepository
	auth     domain.TelegramAuthenticator
	vault    domain.SessionVault
	cache    cache.Cache
	limits   config.TelegramConfig
	audit    domain.AuditLogger
	log      logger.Logger
	now      func() time.Time
}

// NewTelegramUsecase wires the account and group flows. auth and vault are nil
// when Telegram is not configured, and every call that needs them says so.
func NewTelegramUsecase(
	accounts domain.TelegramAccountRepository,
	groups domain.TelegramGroupRepository,
	posting domain.PostingRepository,
	auth domain.TelegramAuthenticator,
	vault domain.SessionVault,
	c cache.Cache,
	limits config.TelegramConfig,
	audit domain.AuditLogger,
	log logger.Logger,
) *TelegramUsecase {
	return &TelegramUsecase{
		accounts: accounts, groups: groups, posting: posting, auth: auth, vault: vault,
		cache: c, limits: limits, audit: audit, log: log, now: time.Now,
	}
}

// pendingLogin is a sign-in between two requests. The in-progress session is
// sealed like a finished one: it already carries an authorisation key.
type pendingLogin struct {
	Phone      string `json:"phone"`
	CodeHash   string `json:"code_hash"`
	Ciphertext []byte `json:"ciphertext"`
	KeyVersion int    `json:"key_version"`
}

// StartLogin asks Telegram to send a code to the person's own app.
func (u *TelegramUsecase) StartLogin(
	ctx context.Context,
	userID, companyID uuid.UUID,
	req dto.StartTelegramLoginRequest,
) (*dto.TelegramLoginStep, error) {
	if err := u.configured(); err != nil {
		return nil, err
	}
	if req.ConsentVersion != TelegramConsentVersion {
		return nil, apperror.ErrValidation.
			WithMessage("The terms have changed since this page was opened. Reload it and read them again.")
	}

	existing, err := u.accounts.ListByUser(ctx, companyID, userID)
	if err != nil {
		return nil, err
	}
	for _, account := range existing {
		if account.Phone != req.Phone {
			continue
		}
		if account.Status != domain.TelegramAccountConnecting {
			return nil, apperror.ErrConflict.WithMessage("This number is already connected.")
		}
		// An abandoned attempt is retired rather than resumed: its consent
		// belongs to an earlier moment, and this one gets its own record.
		if err := u.accounts.Disconnect(ctx, companyID, account.ID); err != nil {
			return nil, err
		}
	}

	login, err := u.auth.SendCode(ctx, req.Phone)
	if err != nil {
		return nil, telegramFailure(err)
	}

	now := u.now()
	version := req.ConsentVersion
	account := &domain.TelegramAccount{
		ID:             uuid.New(),
		CompanyID:      companyID,
		UserID:         userID,
		Phone:          req.Phone,
		Label:          req.Label,
		Status:         domain.TelegramAccountConnecting,
		ConsentVersion: &version,
		ConsentAt:      &now,
	}
	if err := u.accounts.Create(ctx, account); err != nil {
		return nil, err
	}
	if err := u.storeLogin(ctx, account.ID, login); err != nil {
		return nil, err
	}

	u.record(ctx, userID, "telegram_account.connect_started", account.ID)
	return &dto.TelegramLoginStep{AccountID: account.ID, Step: loginStepCode}, nil
}

// SubmitCode checks the code the person received.
func (u *TelegramUsecase) SubmitCode(
	ctx context.Context,
	userID, companyID, accountID uuid.UUID,
	code secret.Value,
) (*dto.TelegramLoginStep, error) {
	account, login, err := u.resume(ctx, userID, companyID, accountID)
	if err != nil {
		return nil, err
	}

	result, err := u.auth.SignIn(ctx, login, code.Reveal())
	if err != nil {
		return nil, telegramFailure(err)
	}
	if result.PasswordRequired {
		// The authorisation continues on the same session, which now knows a
		// code was accepted; only the password is still owed.
		login.Session = result.Session
		if err := u.storeLogin(ctx, account.ID, login); err != nil {
			return nil, err
		}
		return &dto.TelegramLoginStep{AccountID: account.ID, Step: loginStepPassword}, nil
	}
	return u.finish(ctx, userID, account, result)
}

// SubmitPassword completes a sign-in on an account with two-step
// verification.
func (u *TelegramUsecase) SubmitPassword(
	ctx context.Context,
	userID, companyID, accountID uuid.UUID,
	password secret.Value,
) (*dto.TelegramLoginStep, error) {
	account, login, err := u.resume(ctx, userID, companyID, accountID)
	if err != nil {
		return nil, err
	}

	result, err := u.auth.CheckPassword(ctx, login, password.Reveal())
	if err != nil {
		return nil, telegramFailure(err)
	}
	return u.finish(ctx, userID, account, result)
}

// resume loads a sign-in in progress, counting the attempt.
func (u *TelegramUsecase) resume(
	ctx context.Context,
	userID, companyID, accountID uuid.UUID,
) (*domain.TelegramAccount, domain.TelegramLogin, error) {
	if err := u.configured(); err != nil {
		return nil, domain.TelegramLogin{}, err
	}

	account, err := u.accounts.FindForUser(ctx, companyID, userID, accountID)
	if err != nil {
		return nil, domain.TelegramLogin{}, err
	}
	if account.Status != domain.TelegramAccountConnecting {
		return nil, domain.TelegramLogin{}, apperror.ErrConflict.WithMessage("This account is already connected.")
	}

	attempts, err := u.cache.Incr(ctx, loginAttemptsKey(accountID), loginTTL)
	if err != nil {
		return nil, domain.TelegramLogin{}, apperror.ErrUnavailable.Wrap(err)
	}
	if attempts > maxLoginAttempts {
		return nil, domain.TelegramLogin{}, apperror.ErrRateLimited.
			WithMessage("Too many attempts. Start connecting again to get a new code.")
	}

	var pending pendingLogin
	if err := u.cache.Get(ctx, loginKey(accountID), &pending); err != nil {
		return nil, domain.TelegramLogin{}, apperror.ErrConflict.
			WithMessage("This sign-in has expired. Start connecting again to get a new code.")
	}
	session, err := u.vault.Open(pending.Ciphertext, pending.KeyVersion, loginAAD(accountID))
	if err != nil {
		return nil, domain.TelegramLogin{}, fmt.Errorf("open pending login: %w", err)
	}
	return account, domain.TelegramLogin{Phone: pending.Phone, CodeHash: pending.CodeHash, Session: session}, nil
}

// finish seals the authorised session and switches the account on.
func (u *TelegramUsecase) finish(
	ctx context.Context,
	userID uuid.UUID,
	account *domain.TelegramAccount,
	result domain.TelegramLoginResult,
) (*dto.TelegramLoginStep, error) {
	if len(result.Session) == 0 {
		return nil, apperror.ErrInternal.WithMessage("Telegram accepted the sign-in but returned no session.")
	}

	ciphertext, version, err := u.vault.Seal(result.Session, account.SessionAAD())
	wipe(result.Session)
	if err != nil {
		return nil, fmt.Errorf("seal session: %w", err)
	}
	if err := u.accounts.StoreSession(ctx, account.ID, ciphertext, int16(version), result.TgUserID); err != nil {
		return nil, err
	}

	if err := u.cache.Delete(ctx, loginKey(account.ID), loginAttemptsKey(account.ID)); err != nil {
		u.log.Warn("pending login not cleared", logger.Err(err))
	}
	u.record(ctx, userID, "telegram_account.connected", account.ID)
	return &dto.TelegramLoginStep{AccountID: account.ID, Step: loginStepConnected}, nil
}

func (u *TelegramUsecase) storeLogin(ctx context.Context, accountID uuid.UUID, login domain.TelegramLogin) error {
	ciphertext, version, err := u.vault.Seal(login.Session, loginAAD(accountID))
	wipe(login.Session)
	if err != nil {
		return fmt.Errorf("seal pending login: %w", err)
	}
	pending := pendingLogin{Phone: login.Phone, CodeHash: login.CodeHash, Ciphertext: ciphertext, KeyVersion: version}
	if err := u.cache.Set(ctx, loginKey(accountID), pending, loginTTL); err != nil {
		return apperror.ErrUnavailable.Wrap(err)
	}
	return nil
}

// Accounts lists connected accounts. A person sees their own; a manager also
// sees their colleagues', to know who is connected — never anything that could
// be used to post as them.
func (u *TelegramUsecase) Accounts(
	ctx context.Context,
	userID, companyID uuid.UUID,
	wholeCompany bool,
) ([]dto.TelegramAccountResponse, error) {
	var (
		accounts []domain.TelegramAccount
		err      error
	)
	if wholeCompany {
		accounts, err = u.accounts.ListByCompany(ctx, companyID)
	} else {
		accounts, err = u.accounts.ListByUser(ctx, companyID, userID)
	}
	if err != nil {
		return nil, err
	}

	out := make([]dto.TelegramAccountResponse, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, dto.TelegramAccountResponse{
			ID:           account.ID,
			UserID:       account.UserID,
			Mine:         account.UserID == userID,
			Phone:        secret.MaskPhone(account.Phone),
			Label:        account.Label,
			Status:       string(account.Status),
			StatusReason: account.StatusReason,
			PausedUntil:  account.PausedUntil,
			ConsentAt:    account.ConsentAt,
			LastUsedAt:   account.LastUsedAt,
			CreatedAt:    account.CreatedAt,
		})
	}
	return out, nil
}

// Disconnect ends the session on Telegram's side and on ours, and stops
// everything that was waiting to be sent from it.
func (u *TelegramUsecase) Disconnect(ctx context.Context, userID, companyID, accountID uuid.UUID) error {
	account, err := u.accounts.FindForUser(ctx, companyID, userID, accountID)
	if err != nil {
		return err
	}

	// Logging out on Telegram's side removes the entry from the person's list
	// of devices. It is attempted, not required: an account whose session is
	// already dead must still be disconnectable here.
	if u.auth != nil && u.vault != nil && len(account.SessionCiphertext) > 0 && account.SessionKeyVersion != nil {
		if session, err := u.vault.Open(account.SessionCiphertext, int(*account.SessionKeyVersion), account.SessionAAD()); err == nil {
			if err := u.auth.LogOut(ctx, session); err != nil {
				u.log.Warn("telegram logout failed; disconnecting anyway", logger.Err(err))
			}
			wipe(session)
		}
	}

	if err := u.accounts.Disconnect(ctx, companyID, accountID); err != nil {
		return err
	}
	if _, err := u.posting.CancelForAccount(ctx, accountID, "The Telegram account was disconnected."); err != nil {
		u.log.Error("queued posts not cancelled after a disconnect", logger.Err(err))
	}
	if err := u.cache.Delete(ctx, loginKey(accountID), loginAttemptsKey(accountID)); err != nil {
		u.log.Warn("pending login not cleared", logger.Err(err))
	}
	u.record(ctx, userID, "telegram_account.disconnected", accountID)
	return nil
}

// Dialogs lists the groups the account could post to, marking the ones already
// connected.
func (u *TelegramUsecase) Dialogs(
	ctx context.Context,
	userID, companyID, accountID uuid.UUID,
) ([]dto.TelegramDialogResponse, error) {
	account, session, err := u.openAccount(ctx, userID, companyID, accountID)
	if err != nil {
		return nil, err
	}
	defer wipe(session)

	dialogs, err := u.auth.Dialogs(ctx, session)
	if err != nil {
		return nil, telegramFailure(err)
	}

	connected, err := u.groups.ListByAccount(ctx, companyID, account.ID)
	if err != nil {
		return nil, err
	}
	byPeer := make(map[int64]uuid.UUID, len(connected))
	for _, group := range connected {
		byPeer[group.TgPeerID] = group.ID
	}

	out := make([]dto.TelegramDialogResponse, 0, len(dialogs))
	for _, dialog := range dialogs {
		entry := dto.TelegramDialogResponse{
			PeerID:          strconv.FormatInt(dialog.PeerID, 10),
			Title:           dialog.Title,
			Username:        dialog.Username,
			Kind:            dialog.Kind,
			MemberCount:     dialog.MemberCount,
			SlowmodeEnabled: dialog.SlowmodeEnabled,
			CanSend:         dialog.CanSend,
		}
		if id, found := byPeer[dialog.PeerID]; found {
			groupID := id
			entry.GroupID = &groupID
		}
		out = append(out, entry)
	}
	return out, nil
}

// ConnectGroup adds a group the account is already a member of.
//
// The group has to be in the account's own dialog list. The platform never
// joins anything and never accepts a group it cannot see: posting into a group
// the account was not already in is exactly what gets accounts restricted.
func (u *TelegramUsecase) ConnectGroup(
	ctx context.Context,
	userID, companyID uuid.UUID,
	req dto.ConnectGroupRequest,
) (*dto.TelegramGroupResponse, error) {
	peerID, err := strconv.ParseInt(req.PeerID, 10, 64)
	if err != nil {
		return nil, apperror.ErrValidation.WithMessage("That group id is not valid.")
	}

	account, session, err := u.openAccount(ctx, userID, companyID, req.AccountID)
	if err != nil {
		return nil, err
	}
	defer wipe(session)

	dialogs, err := u.auth.Dialogs(ctx, session)
	if err != nil {
		return nil, telegramFailure(err)
	}
	index := slices.IndexFunc(dialogs, func(d domain.TelegramDialog) bool { return d.PeerID == peerID })
	if index < 0 {
		return nil, apperror.ErrNotFound.
			WithMessage("This account is not a member of that group. Join it in Telegram first.")
	}
	dialog := dialogs[index]
	if !dialog.CanSend {
		return nil, apperror.ErrForbidden.
			WithMessage("This account is not allowed to write in that group.")
	}

	slowmode := 0
	if dialog.SlowmodeEnabled {
		if seconds, err := u.auth.SlowmodeSeconds(ctx, session, dialog.PeerID, dialog.AccessHash); err == nil {
			slowmode = seconds
		} else {
			u.log.Warn("slow mode could not be read; the platform cooldown applies", logger.Err(err))
		}
	}

	language := req.Language
	if language == "" {
		language = "ru"
	}
	now := u.now()
	group := &domain.TelegramGroup{
		CompanyID:       companyID,
		AccountID:       account.ID,
		TgPeerID:        dialog.PeerID,
		AccessHash:      dialog.AccessHash,
		Title:           dialog.Title,
		Username:        dialog.Username,
		Kind:            dialog.Kind,
		Language:        language,
		MemberCount:     dialog.MemberCount,
		SlowmodeSeconds: slowmode,
		CanSend:         true,
		PostingEnabled:  true,
		Active:          true,
		LastSyncedAt:    &now,
	}
	if err := u.groups.Upsert(ctx, group); err != nil {
		return nil, err
	}

	// A group with no rules would never be offered, because the candidate
	// filter reads them. Defaults are written on first connect and left alone
	// on a reconnect, where the person has already said what they want.
	rules, routes, err := u.groups.Rules(ctx, companyID, group.ID)
	if apperror.Is(err, apperror.ErrNotFound.Code) {
		rules = u.defaultRules(companyID, group.ID)
		if err := u.groups.SaveRules(ctx, rules, nil); err != nil {
			return nil, err
		}
		routes = nil
	} else if err != nil {
		return nil, err
	}

	u.record(ctx, userID, "telegram_group.connected", group.ID)
	response := groupResponse(domain.GroupCandidate{Group: *group, Rules: *rules, Routes: routes})
	return &response, nil
}

func (u *TelegramUsecase) defaultRules(companyID, groupID uuid.UUID) *domain.TelegramGroupRules {
	daily := int16(3)
	if u.limits.GroupDaily > 0 && int16(u.limits.GroupDaily) < daily {
		daily = int16(u.limits.GroupDaily)
	}
	return &domain.TelegramGroupRules{
		GroupID:         groupID,
		CompanyID:       companyID,
		CargoTypeCodes:  domain.StringArray{},
		VehicleTypes:    domain.StringArray{},
		HourlyLimit:     1,
		DailyLimit:      daily,
		CooldownMinutes: int(u.limits.GroupCooldown.Minutes()),
	}
}

// Groups lists the company's connected groups with their rules.
func (u *TelegramUsecase) Groups(ctx context.Context, companyID uuid.UUID) ([]dto.TelegramGroupResponse, error) {
	groups, err := u.groups.ListWithRules(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.TelegramGroupResponse, 0, len(groups))
	for _, group := range groups {
		out = append(out, groupResponse(group))
	}
	return out, nil
}

// SaveRules replaces what a group accepts and how often it may be posted to.
func (u *TelegramUsecase) SaveRules(
	ctx context.Context,
	userID, companyID, groupID uuid.UUID,
	req dto.SaveGroupRulesRequest,
) (*dto.TelegramGroupResponse, error) {
	group, err := u.groups.FindByID(ctx, companyID, groupID)
	if err != nil {
		return nil, err
	}
	if err := validateRules(req); err != nil {
		return nil, err
	}

	rules := u.defaultRules(companyID, group.ID)
	rules.CargoTypeCodes = domain.StringArray(req.CargoTypeCodes)
	rules.VehicleTypes = domain.StringArray(req.VehicleTypes)
	rules.MinWeightKg = req.MinWeightKg
	rules.MaxWeightKg = req.MaxWeightKg
	rules.QuietFromHour = req.QuietFromHour
	rules.QuietToHour = req.QuietToHour
	rules.Notes = req.Notes
	if req.HourlyLimit > 0 {
		rules.HourlyLimit = req.HourlyLimit
	}
	if req.DailyLimit > 0 {
		rules.DailyLimit = min(req.DailyLimit, int16(max(u.limits.GroupDaily, 1)))
	}
	if req.CooldownMinutes != nil {
		// The platform floor still applies at send time; storing what the
		// person asked for keeps their intent visible.
		rules.CooldownMinutes = *req.CooldownMinutes
	}

	routes := make([]domain.TelegramGroupRoute, 0, len(req.Routes))
	for _, route := range req.Routes {
		routes = append(routes, domain.TelegramGroupRoute{
			GroupID:        group.ID,
			CompanyID:      companyID,
			OriginCountry:  route.OriginCountry,
			OriginRegionID: route.OriginRegionID,
			DestCountry:    route.DestCountry,
			DestRegionID:   route.DestRegionID,
		})
	}

	if err := u.groups.SaveRules(ctx, rules, routes); err != nil {
		return nil, err
	}
	if err := u.groups.UpdateSettings(ctx, companyID, group.ID, req.Language, req.PostingEnabled); err != nil {
		return nil, err
	}
	if req.Language != nil {
		group.Language = *req.Language
	}
	if req.PostingEnabled != nil {
		group.PostingEnabled = *req.PostingEnabled
	}

	u.record(ctx, userID, "telegram_group.rules_saved", group.ID)
	response := groupResponse(domain.GroupCandidate{Group: *group, Rules: *rules, Routes: routes})
	return &response, nil
}

func validateRules(req dto.SaveGroupRulesRequest) error {
	if req.MinWeightKg != nil && req.MaxWeightKg != nil && *req.MinWeightKg > *req.MaxWeightKg {
		return apperror.ErrValidation.WithMessage("The minimum weight is above the maximum.")
	}
	if (req.QuietFromHour == nil) != (req.QuietToHour == nil) {
		return apperror.ErrValidation.WithMessage("Quiet hours need both a start and an end.")
	}
	for _, code := range req.VehicleTypes {
		if !slices.Contains(domain.AllVehicleTypes, domain.VehicleType(code)) {
			return apperror.ErrValidation.WithMessage("Unknown body type: " + code)
		}
	}
	for _, route := range req.Routes {
		if route.OriginCountry == nil && route.OriginRegionID == nil {
			return apperror.ErrValidation.WithMessage("Every route needs a starting country or region.")
		}
		if route.DestCountry == nil && route.DestRegionID == nil {
			return apperror.ErrValidation.WithMessage("Every route needs a destination country or region.")
		}
	}
	return nil
}

// RemoveGroup stops posting to a group. Posts still queued for it are skipped
// by the worker, which finds the group gone.
func (u *TelegramUsecase) RemoveGroup(ctx context.Context, userID, companyID, groupID uuid.UUID) error {
	if err := u.groups.Remove(ctx, companyID, groupID); err != nil {
		return err
	}
	u.record(ctx, userID, "telegram_group.removed", groupID)
	return nil
}

// openAccount loads the caller's own account and opens its session.
func (u *TelegramUsecase) openAccount(
	ctx context.Context,
	userID, companyID, accountID uuid.UUID,
) (*domain.TelegramAccount, []byte, error) {
	if err := u.configured(); err != nil {
		return nil, nil, err
	}
	account, err := u.accounts.FindForUser(ctx, companyID, userID, accountID)
	if err != nil {
		return nil, nil, err
	}
	if len(account.SessionCiphertext) == 0 || account.SessionKeyVersion == nil {
		return nil, nil, apperror.ErrConflict.WithMessage("Finish connecting this account first.")
	}
	session, err := u.vault.Open(account.SessionCiphertext, int(*account.SessionKeyVersion), account.SessionAAD())
	if err != nil {
		return nil, nil, fmt.Errorf("open session: %w", err)
	}
	return account, session, nil
}

func (u *TelegramUsecase) configured() error {
	if u.auth == nil || u.vault == nil {
		return apperror.ErrUnavailable.WithMessage("Telegram posting is not set up on this server.")
	}
	return nil
}

func (u *TelegramUsecase) record(ctx context.Context, userID uuid.UUID, action string, id uuid.UUID) {
	if err := u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       action,
		ResourceType: "telegram",
		ResourceID:   &id,
	}); err != nil {
		u.log.Warn("audit write failed", logger.String("action", action), logger.Err(err))
	}
}

// telegramFailure turns what the client reported into something a person can
// act on.
func telegramFailure(err error) error {
	switch {
	case errors.Is(err, domain.ErrTelegramPhoneInvalid):
		return apperror.ErrValidation.WithMessage("Telegram does not recognise this phone number.")
	case errors.Is(err, domain.ErrTelegramPhoneBanned):
		return apperror.ErrForbidden.WithMessage("Telegram has banned this number.")
	case errors.Is(err, domain.ErrTelegramPhoneUnknown):
		return apperror.ErrValidation.WithMessage("There is no Telegram account on this number.")
	case errors.Is(err, domain.ErrTelegramCodeInvalid):
		return apperror.ErrValidation.WithMessage("The code is wrong. Check the message from Telegram and try again.")
	case errors.Is(err, domain.ErrTelegramCodeExpired):
		return apperror.ErrConflict.WithMessage("The code has expired. Start connecting again to get a new one.")
	case errors.Is(err, domain.ErrTelegramPasswordInvalid):
		return apperror.ErrValidation.WithMessage("The two-step verification password is wrong.")
	}

	var tgErr *domain.TelegramError
	if errors.As(err, &tgErr) {
		switch tgErr.Kind {
		case domain.TelegramFloodWait:
			return apperror.ErrRateLimited.WithMessage(
				fmt.Sprintf("Telegram asked this account to wait %s before trying again.", tgErr.Wait.Round(time.Second)))
		case domain.TelegramSessionRevoked:
			return apperror.ErrConflict.WithMessage("The Telegram session has ended. Connect the account again.")
		case domain.TelegramPeerFlood:
			return apperror.ErrForbidden.WithMessage("Telegram has restricted this account.")
		case domain.TelegramTransient:
			return apperror.ErrUnavailable.WithMessage("Telegram could not be reached. Try again in a moment.").Wrap(err)
		}
	}
	return apperror.ErrInternal.Wrap(err)
}

func loginKey(accountID uuid.UUID) string         { return "tg:login:" + accountID.String() }
func loginAttemptsKey(accountID uuid.UUID) string { return "tg:login:attempts:" + accountID.String() }

// loginAAD binds a pending sign-in to its account, so a sealed login cannot be
// swapped onto another one.
func loginAAD(accountID uuid.UUID) string { return "telegram_login:" + accountID.String() }

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func groupResponse(candidate domain.GroupCandidate) dto.TelegramGroupResponse {
	group, rules := candidate.Group, candidate.Rules
	response := dto.TelegramGroupResponse{
		ID:              group.ID,
		AccountID:       group.AccountID,
		PeerID:          strconv.FormatInt(group.TgPeerID, 10),
		Title:           group.Title,
		Username:        group.Username,
		Kind:            group.Kind,
		Language:        group.Language,
		MemberCount:     group.MemberCount,
		SlowmodeSeconds: group.SlowmodeSeconds,
		CanSend:         group.CanSend,
		PostingEnabled:  group.PostingEnabled,
		Active:          group.Active,
		InactiveReason:  group.InactiveReason,
		LastPostedAt:    group.LastPostedAt,
	}
	if rules.GroupID != uuid.Nil {
		response.Rules = &dto.GroupRulesResponse{
			CargoTypeCodes:  append([]string{}, rules.CargoTypeCodes...),
			VehicleTypes:    append([]string{}, rules.VehicleTypes...),
			MinWeightKg:     rules.MinWeightKg,
			MaxWeightKg:     rules.MaxWeightKg,
			HourlyLimit:     rules.HourlyLimit,
			DailyLimit:      rules.DailyLimit,
			CooldownMinutes: rules.CooldownMinutes,
			QuietFromHour:   rules.QuietFromHour,
			QuietToHour:     rules.QuietToHour,
			Notes:           rules.Notes,
		}
	}
	for _, route := range candidate.Routes {
		response.Routes = append(response.Routes, dto.GroupRouteRequest{
			OriginCountry:  route.OriginCountry,
			OriginRegionID: route.OriginRegionID,
			DestCountry:    route.DestCountry,
			DestRegionID:   route.DestRegionID,
		})
	}
	return response
}
