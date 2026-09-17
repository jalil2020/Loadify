package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/internal/infrastructure/crypto"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
	"github.com/loadify/backend/pkg/secret"
)

// ─── fakes ──────────────────────────────────────────────────────────

// fakeAuth plays Telegram. It hands out recognisable sessions so a test can
// check that what was sealed is what Telegram returned.
type fakeAuth struct {
	needPassword bool
	signInErr    error
	sendCodeErr  error
	dialogs      []domain.TelegramDialog
	slowmode     int
	codeCalls    int
	gotCode      string
	gotPassword  string
	resumed      []string
	loggedOut    bool
}

func (f *fakeAuth) SendCode(context.Context, string) (domain.TelegramLogin, error) {
	f.codeCalls++
	if f.sendCodeErr != nil {
		return domain.TelegramLogin{}, f.sendCodeErr
	}
	return domain.TelegramLogin{CodeHash: "hash-1", Session: []byte("session-after-code-request")}, nil
}

func (f *fakeAuth) SignIn(_ context.Context, login domain.TelegramLogin, code string) (domain.TelegramLoginResult, error) {
	f.gotCode = code
	f.resumed = append(f.resumed, string(login.Session))
	if f.signInErr != nil {
		return domain.TelegramLoginResult{}, f.signInErr
	}
	if f.needPassword {
		return domain.TelegramLoginResult{PasswordRequired: true, Session: []byte("session-awaiting-password")}, nil
	}
	return domain.TelegramLoginResult{Session: []byte("authorised-session"), TgUserID: 5550001}, nil
}

func (f *fakeAuth) CheckPassword(_ context.Context, login domain.TelegramLogin, password string) (domain.TelegramLoginResult, error) {
	f.gotPassword = password
	f.resumed = append(f.resumed, string(login.Session))
	if password != "correct horse" {
		return domain.TelegramLoginResult{}, domain.ErrTelegramPasswordInvalid
	}
	return domain.TelegramLoginResult{Session: []byte("authorised-session"), TgUserID: 5550001}, nil
}

func (f *fakeAuth) Dialogs(context.Context, []byte) ([]domain.TelegramDialog, error) {
	return f.dialogs, nil
}

func (f *fakeAuth) SlowmodeSeconds(context.Context, []byte, int64, *int64) (int, error) {
	return f.slowmode, nil
}

func (f *fakeAuth) LogOut(context.Context, []byte) error {
	f.loggedOut = true
	return nil
}

// accountBook keeps accounts the way the table would, including ownership.
type accountBook struct {
	rows         map[uuid.UUID]*domain.TelegramAccount
	disconnected []uuid.UUID
}

func (a *accountBook) Create(_ context.Context, account *domain.TelegramAccount) error {
	a.rows[account.ID] = account
	return nil
}

func (a *accountBook) FindByID(_ context.Context, _, id uuid.UUID) (*domain.TelegramAccount, error) {
	if row, ok := a.rows[id]; ok {
		return row, nil
	}
	return nil, apperror.ErrNotFound
}

func (a *accountBook) FindForUser(_ context.Context, _, userID, id uuid.UUID) (*domain.TelegramAccount, error) {
	row, ok := a.rows[id]
	if !ok || row.UserID != userID {
		return nil, apperror.ErrNotFound
	}
	return row, nil
}

func (a *accountBook) ListByCompany(context.Context, uuid.UUID) ([]domain.TelegramAccount, error) {
	var out []domain.TelegramAccount
	for _, row := range a.rows {
		out = append(out, *row)
	}
	return out, nil
}

func (a *accountBook) ListByUser(_ context.Context, _, userID uuid.UUID) ([]domain.TelegramAccount, error) {
	var out []domain.TelegramAccount
	for _, row := range a.rows {
		if row.UserID == userID {
			out = append(out, *row)
		}
	}
	return out, nil
}

func (a *accountBook) StoreSession(_ context.Context, id uuid.UUID, ciphertext []byte, version int16, tgUserID int64) error {
	row := a.rows[id]
	row.SessionCiphertext = ciphertext
	row.SessionKeyVersion = &version
	row.TgUserID = &tgUserID
	row.Status = domain.TelegramAccountActive
	return nil
}

func (a *accountBook) SetStatus(context.Context, uuid.UUID, domain.TelegramAccountStatus, *string, *time.Time) error {
	return nil
}
func (a *accountBook) TouchUsed(context.Context, uuid.UUID, time.Time) error { return nil }

func (a *accountBook) Disconnect(_ context.Context, _, id uuid.UUID) error {
	a.disconnected = append(a.disconnected, id)
	delete(a.rows, id)
	return nil
}

// groupBook keeps groups, rules and routes.
type groupBook struct {
	groups map[uuid.UUID]*domain.TelegramGroup
	rules  map[uuid.UUID]*domain.TelegramGroupRules
	routes map[uuid.UUID][]domain.TelegramGroupRoute
}

func newGroupBook() *groupBook {
	return &groupBook{
		groups: map[uuid.UUID]*domain.TelegramGroup{},
		rules:  map[uuid.UUID]*domain.TelegramGroupRules{},
		routes: map[uuid.UUID][]domain.TelegramGroupRoute{},
	}
}

func (g *groupBook) Upsert(_ context.Context, group *domain.TelegramGroup) error {
	for id, existing := range g.groups {
		if existing.AccountID == group.AccountID && existing.TgPeerID == group.TgPeerID {
			group.ID = id
		}
	}
	if group.ID == uuid.Nil {
		group.ID = uuid.New()
	}
	g.groups[group.ID] = group
	return nil
}

func (g *groupBook) FindByID(_ context.Context, _, id uuid.UUID) (*domain.TelegramGroup, error) {
	if group, ok := g.groups[id]; ok {
		return group, nil
	}
	return nil, apperror.ErrNotFound
}

func (g *groupBook) ListByAccount(_ context.Context, _, accountID uuid.UUID) ([]domain.TelegramGroup, error) {
	var out []domain.TelegramGroup
	for _, group := range g.groups {
		if group.AccountID == accountID {
			out = append(out, *group)
		}
	}
	return out, nil
}

func (g *groupBook) ListByCompany(context.Context, uuid.UUID) ([]domain.TelegramGroup, error) {
	return nil, nil
}

func (g *groupBook) ListWithRules(context.Context, uuid.UUID) ([]domain.GroupCandidate, error) {
	var out []domain.GroupCandidate
	for id, group := range g.groups {
		candidate := domain.GroupCandidate{Group: *group, Routes: g.routes[id]}
		if rules, ok := g.rules[id]; ok {
			candidate.Rules = *rules
		}
		out = append(out, candidate)
	}
	return out, nil
}

func (g *groupBook) Candidates(context.Context, uuid.UUID, domain.GroupCandidateQuery) ([]domain.GroupCandidate, error) {
	return nil, nil
}

func (g *groupBook) SaveRules(_ context.Context, rules *domain.TelegramGroupRules, routes []domain.TelegramGroupRoute) error {
	g.rules[rules.GroupID] = rules
	g.routes[rules.GroupID] = routes
	return nil
}

func (g *groupBook) Rules(_ context.Context, _, groupID uuid.UUID) (*domain.TelegramGroupRules, []domain.TelegramGroupRoute, error) {
	rules, ok := g.rules[groupID]
	if !ok {
		return nil, nil, apperror.ErrNotFound
	}
	return rules, g.routes[groupID], nil
}

func (g *groupBook) UpdateSettings(_ context.Context, _, id uuid.UUID, language *string, enabled *bool) error {
	if language != nil {
		g.groups[id].Language = *language
	}
	if enabled != nil {
		g.groups[id].PostingEnabled = *enabled
	}
	return nil
}

func (g *groupBook) Deactivate(context.Context, uuid.UUID, uuid.UUID, string) error     { return nil }
func (g *groupBook) DisablePosting(context.Context, uuid.UUID, uuid.UUID, string) error { return nil }
func (g *groupBook) MarkPosted(context.Context, uuid.UUID, time.Time) error             { return nil }
func (g *groupBook) Remove(_ context.Context, _, id uuid.UUID) error {
	delete(g.groups, id)
	return nil
}
func (g *groupBook) CountActive(context.Context, uuid.UUID) (int64, error) { return 0, nil }

// cancelBook records which accounts had their queue cleared.
type cancelBook struct {
	fakePosting
	cancelledAccounts []uuid.UUID
}

func (c *cancelBook) CancelForAccount(_ context.Context, accountID uuid.UUID, _ string) (int64, error) {
	c.cancelledAccounts = append(c.cancelledAccounts, accountID)
	return 0, nil
}

// ─── harness ────────────────────────────────────────────────────────

type connectFixture struct {
	usecase   *TelegramUsecase
	auth      *fakeAuth
	accounts  *accountBook
	groups    *groupBook
	posting   *cancelBook
	vault     domain.SessionVault
	companyID uuid.UUID
	userID    uuid.UUID
}

func newTestCache(t *testing.T) cache.Cache {
	t.Helper()
	server := miniredis.RunT(t)
	host, port, _ := strings.Cut(server.Addr(), ":")
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("miniredis port: %v", err)
	}
	c, err := cache.New(config.RedisConfig{Host: host, Port: portNumber, PoolSize: 4}, logger.New("test", "error"))
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func newConnectFixture(t *testing.T) *connectFixture {
	t.Helper()

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("key: %v", err)
	}
	vault, err := crypto.NewSessionVault(map[int][]byte{1: key}, 1)
	if err != nil {
		t.Fatalf("vault: %v", err)
	}

	f := &connectFixture{
		auth:      &fakeAuth{},
		accounts:  &accountBook{rows: map[uuid.UUID]*domain.TelegramAccount{}},
		groups:    newGroupBook(),
		posting:   &cancelBook{},
		vault:     vault,
		companyID: uuid.New(),
		userID:    uuid.New(),
	}
	f.usecase = NewTelegramUsecase(f.accounts, f.groups, f.posting, f.auth, vault, nil, newTestCache(t),
		config.TelegramConfig{GroupDaily: 3, GroupCooldown: time.Hour}, noopAudit{}, logger.New("test", "error"))
	return f
}

func (f *connectFixture) start(t *testing.T) uuid.UUID {
	t.Helper()
	step, err := f.usecase.StartLogin(context.Background(), f.userID, f.companyID, dto.StartTelegramLoginRequest{
		Phone: "+998901234567", ConsentVersion: TelegramConsentVersion,
	})
	if err != nil {
		t.Fatalf("StartLogin: %v", err)
	}
	if step.Step != loginStepCode {
		t.Fatalf("step = %q, want code", step.Step)
	}
	return step.AccountID
}

// connect runs the whole sign-in and returns the active account.
func (f *connectFixture) connect(t *testing.T) *domain.TelegramAccount {
	t.Helper()
	accountID := f.start(t)
	if _, err := f.usecase.SubmitCode(context.Background(), f.userID, f.companyID, accountID, secret.Value("12345")); err != nil {
		t.Fatalf("SubmitCode: %v", err)
	}
	return f.accounts.rows[accountID]
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if !apperror.Is(err, code) {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

// ─── connecting an account ──────────────────────────────────────────

func TestConnectingAnAccountSealsTheSessionTelegramReturned(t *testing.T) {
	f := newConnectFixture(t)
	accountID := f.start(t)

	account := f.accounts.rows[accountID]
	if account.Status != domain.TelegramAccountConnecting || account.ConsentAt == nil {
		t.Fatalf("a started account is %q with consent %v", account.Status, account.ConsentAt)
	}

	step, err := f.usecase.SubmitCode(context.Background(), f.userID, f.companyID, accountID, secret.Value("12345"))
	if err != nil {
		t.Fatalf("SubmitCode: %v", err)
	}
	if step.Step != loginStepConnected {
		t.Fatalf("step = %q, want connected", step.Step)
	}

	// The code step resumed the connection that asked for the code.
	if len(f.auth.resumed) != 1 || f.auth.resumed[0] != "session-after-code-request" {
		t.Errorf("sign-in resumed %v, want the session from the code request", f.auth.resumed)
	}
	if f.auth.gotCode != "12345" {
		t.Errorf("code reached Telegram as %q", f.auth.gotCode)
	}

	if account.Status != domain.TelegramAccountActive {
		t.Errorf("status = %q, want active", account.Status)
	}
	opened, err := f.vault.Open(account.SessionCiphertext, int(*account.SessionKeyVersion), account.SessionAAD())
	if err != nil {
		t.Fatalf("the stored session does not open with its own row's binding: %v", err)
	}
	if string(opened) != "authorised-session" {
		t.Errorf("stored session = %q", opened)
	}
	if strings.Contains(string(account.SessionCiphertext), "authorised-session") {
		t.Error("the session was stored in the clear")
	}
}

func TestTwoStepVerificationAsksForThePassword(t *testing.T) {
	f := newConnectFixture(t)
	f.auth.needPassword = true
	accountID := f.start(t)

	step, err := f.usecase.SubmitCode(context.Background(), f.userID, f.companyID, accountID, secret.Value("12345"))
	if err != nil {
		t.Fatalf("SubmitCode: %v", err)
	}
	if step.Step != loginStepPassword {
		t.Fatalf("step = %q, want password", step.Step)
	}

	if _, err := f.usecase.SubmitPassword(context.Background(), f.userID, f.companyID, accountID, secret.Value("wrong")); err == nil {
		t.Fatal("a wrong password was accepted")
	} else if !strings.Contains(err.Error(), "password is wrong") {
		t.Errorf("the error does not say what went wrong: %v", err)
	}

	step, err = f.usecase.SubmitPassword(context.Background(), f.userID, f.companyID, accountID, secret.Value("correct horse"))
	if err != nil {
		t.Fatalf("SubmitPassword: %v", err)
	}
	if step.Step != loginStepConnected {
		t.Errorf("step = %q, want connected", step.Step)
	}
	// The password step continued from where the code step left the session.
	if last := f.auth.resumed[len(f.auth.resumed)-1]; last != "session-awaiting-password" {
		t.Errorf("password step resumed %q", last)
	}
}

// Connecting lets the platform post as the person. Consent to an older text is
// not consent to this one.
func TestConsentMustMatchTheCurrentText(t *testing.T) {
	f := newConnectFixture(t)

	_, err := f.usecase.StartLogin(context.Background(), f.userID, f.companyID, dto.StartTelegramLoginRequest{
		Phone: "+998901234567", ConsentVersion: "2020-01",
	})
	wantCode(t, err, apperror.ErrValidation.Code)
	if f.auth.codeCalls != 0 {
		t.Error("Telegram was asked for a code before consent was valid")
	}
}

func TestGuessingTheCodeIsLimited(t *testing.T) {
	f := newConnectFixture(t)
	f.auth.signInErr = domain.ErrTelegramCodeInvalid
	accountID := f.start(t)

	for range maxLoginAttempts {
		_, err := f.usecase.SubmitCode(context.Background(), f.userID, f.companyID, accountID, secret.Value("00000"))
		wantCode(t, err, apperror.ErrValidation.Code)
	}
	_, err := f.usecase.SubmitCode(context.Background(), f.userID, f.companyID, accountID, secret.Value("00000"))
	wantCode(t, err, apperror.ErrRateLimited.Code)
}

func TestAnotherPersonCannotFinishYourSignIn(t *testing.T) {
	f := newConnectFixture(t)
	accountID := f.start(t)

	_, err := f.usecase.SubmitCode(context.Background(), uuid.New(), f.companyID, accountID, secret.Value("12345"))
	wantCode(t, err, apperror.ErrNotFound.Code)
}

func TestAConnectedNumberCannotBeConnectedAgain(t *testing.T) {
	f := newConnectFixture(t)
	f.connect(t)

	_, err := f.usecase.StartLogin(context.Background(), f.userID, f.companyID, dto.StartTelegramLoginRequest{
		Phone: "+998901234567", ConsentVersion: TelegramConsentVersion,
	})
	wantCode(t, err, apperror.ErrConflict.Code)
}

// An abandoned attempt is retired rather than resumed, so the new attempt gets
// its own record of consent.
func TestStartingAgainRetiresAnAbandonedAttempt(t *testing.T) {
	f := newConnectFixture(t)
	first := f.start(t)
	second := f.start(t)

	if first == second {
		t.Fatal("the second attempt reused the first attempt's record")
	}
	if len(f.accounts.disconnected) != 1 || f.accounts.disconnected[0] != first {
		t.Errorf("retired %v, want the first attempt", f.accounts.disconnected)
	}
}

func TestTelegramRefusalsBecomeMessagesAPersonCanActOn(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
		says string
	}{
		"wrong number":  {domain.ErrTelegramPhoneInvalid, apperror.ErrValidation.Code, "does not recognise"},
		"banned number": {domain.ErrTelegramPhoneBanned, apperror.ErrForbidden.Code, "banned"},
		"no account":    {domain.ErrTelegramPhoneUnknown, apperror.ErrValidation.Code, "no Telegram account"},
		"asked to wait": {&domain.TelegramError{Kind: domain.TelegramFloodWait, Wait: 90 * time.Second}, apperror.ErrRateLimited.Code, "1m30s"},
		"no connection": {&domain.TelegramError{Kind: domain.TelegramTransient, Err: errors.New("dial tcp")}, apperror.ErrUnavailable.Code, "could not be reached"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newConnectFixture(t)
			f.auth.sendCodeErr = tc.err

			_, err := f.usecase.StartLogin(context.Background(), f.userID, f.companyID, dto.StartTelegramLoginRequest{
				Phone: "+998901234567", ConsentVersion: TelegramConsentVersion,
			})
			wantCode(t, err, tc.code)
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("message %q does not say %q", err.Error(), tc.says)
			}
			if len(f.accounts.rows) != 0 {
				t.Error("an account row was created for a sign-in Telegram refused")
			}
		})
	}
}

func TestWithoutTelegramConfiguredConnectingSaysSo(t *testing.T) {
	f := newConnectFixture(t)
	f.usecase.auth = nil

	_, err := f.usecase.StartLogin(context.Background(), f.userID, f.companyID, dto.StartTelegramLoginRequest{
		Phone: "+998901234567", ConsentVersion: TelegramConsentVersion,
	})
	wantCode(t, err, apperror.ErrUnavailable.Code)
}

func TestDisconnectingStopsEverything(t *testing.T) {
	f := newConnectFixture(t)
	account := f.connect(t)

	if err := f.usecase.Disconnect(context.Background(), f.userID, f.companyID, account.ID); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if !f.auth.loggedOut {
		t.Error("the session was not ended on Telegram's side")
	}
	if len(f.accounts.disconnected) != 1 {
		t.Error("the account was not disconnected here")
	}
	if len(f.posting.cancelledAccounts) != 1 || f.posting.cancelledAccounts[0] != account.ID {
		t.Error("posts queued from the account were left to go out")
	}
}

// ─── groups ─────────────────────────────────────────────────────────

func openGroup(peer int64, title string) domain.TelegramDialog {
	hash := int64(99)
	return domain.TelegramDialog{PeerID: peer, AccessHash: &hash, Title: title, Kind: "supergroup", CanSend: true}
}

// The platform never joins anything: a group has to be in the account's own
// list to be connected at all.
func TestAGroupMustAlreadyBeInTheAccountsList(t *testing.T) {
	f := newConnectFixture(t)
	account := f.connect(t)
	f.auth.dialogs = []domain.TelegramDialog{openGroup(-1001, "Yuk markazi")}

	_, err := f.usecase.ConnectGroup(context.Background(), f.userID, f.companyID, dto.ConnectGroupRequest{
		AccountID: account.ID, PeerID: "-1009",
	})
	wantCode(t, err, apperror.ErrNotFound.Code)
}

func TestAGroupTheAccountCannotWriteToIsRefused(t *testing.T) {
	f := newConnectFixture(t)
	account := f.connect(t)
	muted := openGroup(-1001, "Faqat o'qish")
	muted.CanSend = false
	f.auth.dialogs = []domain.TelegramDialog{muted}

	_, err := f.usecase.ConnectGroup(context.Background(), f.userID, f.companyID, dto.ConnectGroupRequest{
		AccountID: account.ID, PeerID: "-1001",
	})
	wantCode(t, err, apperror.ErrForbidden.Code)
}

// A group with no rules would never be offered, so connecting one writes
// cautious defaults — and records the group's own slow mode.
func TestConnectingAGroupWritesCautiousDefaults(t *testing.T) {
	f := newConnectFixture(t)
	account := f.connect(t)
	slow := openGroup(-1001, "Yuk markazi")
	slow.SlowmodeEnabled = true
	f.auth.dialogs = []domain.TelegramDialog{slow}
	f.auth.slowmode = 300

	group, err := f.usecase.ConnectGroup(context.Background(), f.userID, f.companyID, dto.ConnectGroupRequest{
		AccountID: account.ID, PeerID: "-1001", Language: "uz",
	})
	if err != nil {
		t.Fatalf("ConnectGroup: %v", err)
	}

	if group.SlowmodeSeconds != 300 {
		t.Errorf("slow mode = %d, want the group's own 300", group.SlowmodeSeconds)
	}
	if group.Language != "uz" || group.PeerID != "-1001" {
		t.Errorf("group = %+v", group)
	}
	if group.Rules == nil {
		t.Fatal("no rules were written, so the group would never be offered")
	}
	if group.Rules.HourlyLimit != 1 || group.Rules.DailyLimit > 3 || group.Rules.CooldownMinutes != 60 {
		t.Errorf("default rules = %+v, want one an hour, at most three a day, an hour apart", group.Rules)
	}
}

// Reconnecting a group keeps what the person already said about it.
func TestReconnectingAGroupKeepsItsRules(t *testing.T) {
	f := newConnectFixture(t)
	account := f.connect(t)
	f.auth.dialogs = []domain.TelegramDialog{openGroup(-1001, "Yuk markazi")}
	request := dto.ConnectGroupRequest{AccountID: account.ID, PeerID: "-1001"}

	group, err := f.usecase.ConnectGroup(context.Background(), f.userID, f.companyID, request)
	if err != nil {
		t.Fatalf("ConnectGroup: %v", err)
	}
	if _, err := f.usecase.SaveRules(context.Background(), f.userID, f.companyID, group.ID, dto.SaveGroupRulesRequest{
		VehicleTypes: []string{string(domain.VehicleTent)},
	}); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}

	again, err := f.usecase.ConnectGroup(context.Background(), f.userID, f.companyID, request)
	if err != nil {
		t.Fatalf("ConnectGroup again: %v", err)
	}
	if again.ID != group.ID {
		t.Error("reconnecting created a second group")
	}
	if len(again.Rules.VehicleTypes) != 1 {
		t.Errorf("rules after reconnecting = %+v, want the saved ones", again.Rules)
	}
}

func TestRulesThatContradictThemselvesAreRefused(t *testing.T) {
	cases := map[string]dto.SaveGroupRulesRequest{
		"minimum above maximum": {MinWeightKg: pointer(20000), MaxWeightKg: pointer(10000)},
		"half a quiet window":   {QuietFromHour: pointer(int16(22))},
		"unknown body type":     {VehicleTypes: []string{"spaceship"}},
		"route with no start":   {Routes: []dto.GroupRouteRequest{{DestCountry: pointer("RU")}}},
		"route with no end":     {Routes: []dto.GroupRouteRequest{{OriginCountry: pointer("UZ")}}},
	}

	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			f := newConnectFixture(t)
			account := f.connect(t)
			f.auth.dialogs = []domain.TelegramDialog{openGroup(-1001, "Yuk markazi")}
			group, err := f.usecase.ConnectGroup(context.Background(), f.userID, f.companyID,
				dto.ConnectGroupRequest{AccountID: account.ID, PeerID: "-1001"})
			if err != nil {
				t.Fatalf("ConnectGroup: %v", err)
			}

			_, err = f.usecase.SaveRules(context.Background(), f.userID, f.companyID, group.ID, req)
			wantCode(t, err, apperror.ErrValidation.Code)
		})
	}
}

// A group rule may make posting rarer, never more frequent than the platform
// allows.
func TestAGroupCannotAskForMoreThanThePlatformAllows(t *testing.T) {
	f := newConnectFixture(t)
	account := f.connect(t)
	f.auth.dialogs = []domain.TelegramDialog{openGroup(-1001, "Yuk markazi")}
	group, err := f.usecase.ConnectGroup(context.Background(), f.userID, f.companyID,
		dto.ConnectGroupRequest{AccountID: account.ID, PeerID: "-1001"})
	if err != nil {
		t.Fatalf("ConnectGroup: %v", err)
	}

	saved, err := f.usecase.SaveRules(context.Background(), f.userID, f.companyID, group.ID, dto.SaveGroupRulesRequest{
		DailyLimit: 20,
		Routes:     []dto.GroupRouteRequest{{OriginCountry: pointer("UZ"), DestCountry: pointer("RU")}},
	})
	if err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
	if saved.Rules.DailyLimit != 3 {
		t.Errorf("daily limit = %d, want the platform's 3", saved.Rules.DailyLimit)
	}
	if len(saved.Routes) != 1 {
		t.Errorf("routes = %+v", saved.Routes)
	}
}

func TestDialogsMarkGroupsAlreadyConnected(t *testing.T) {
	f := newConnectFixture(t)
	account := f.connect(t)
	f.auth.dialogs = []domain.TelegramDialog{openGroup(-1001, "Ulangan"), openGroup(-1002, "Yangi")}
	if _, err := f.usecase.ConnectGroup(context.Background(), f.userID, f.companyID,
		dto.ConnectGroupRequest{AccountID: account.ID, PeerID: "-1001"}); err != nil {
		t.Fatalf("ConnectGroup: %v", err)
	}

	dialogs, err := f.usecase.Dialogs(context.Background(), f.userID, f.companyID, account.ID)
	if err != nil {
		t.Fatalf("Dialogs: %v", err)
	}
	for _, dialog := range dialogs {
		connected := dialog.GroupID != nil
		if connected != (dialog.PeerID == "-1001") {
			t.Errorf("%s connected = %v", dialog.Title, connected)
		}
	}
}

// ─── plan limits ────────────────────────────────────────────────────

func TestAFullPlanRefusesBeforeTelegramSendsACode(t *testing.T) {
	f := newConnectFixture(t)
	quota := newQuotaStub(map[domain.QuotaMetric]int{domain.QuotaTelegramAccounts: 1})
	quota.used[domain.QuotaTelegramAccounts] = 1
	f.usecase.quota = quota

	_, err := f.usecase.StartLogin(context.Background(), f.userID, f.companyID, dto.StartTelegramLoginRequest{
		Phone: "+998901234567", ConsentVersion: TelegramConsentVersion,
	})
	wantCode(t, err, apperror.ErrQuotaExceeded.Code)
	if f.auth.codeCalls != 0 {
		t.Error("a code was sent for a sign-in the plan cannot keep")
	}
	if len(f.accounts.rows) != 0 {
		t.Error("an account record was created anyway")
	}
}

func TestASignInThatLosesTheLastSeatIsUndoneOnTelegramToo(t *testing.T) {
	f := newConnectFixture(t)
	quota := newQuotaStub(map[domain.QuotaMetric]int{domain.QuotaTelegramAccounts: 1})
	f.usecase.quota = quota

	accountID := f.start(t)
	// Another sign-in, started alongside, finished first.
	quota.used[domain.QuotaTelegramAccounts] = 1

	_, err := f.usecase.SubmitCode(context.Background(), f.userID, f.companyID, accountID, secret.Value("12345"))
	wantCode(t, err, apperror.ErrQuotaExceeded.Code)
	if !f.auth.loggedOut {
		t.Error("the session Telegram issued was left alive in the person's device list")
	}
	if _, kept := f.accounts.rows[accountID]; kept {
		t.Error("the account that could not be kept was not retired")
	}
}

func TestTheFinishedSignInTakesTheSeat(t *testing.T) {
	f := newConnectFixture(t)
	quota := newQuotaStub(map[domain.QuotaMetric]int{domain.QuotaTelegramAccounts: 2})
	f.usecase.quota = quota

	f.connect(t)
	if got := quota.used[domain.QuotaTelegramAccounts]; got != 1 {
		t.Errorf("seats used = %d, want 1", got)
	}
}

func TestReconnectingAGroupNeedsNoFreeSeat(t *testing.T) {
	f := newConnectFixture(t)
	account := f.connect(t)
	quota := newQuotaStub(map[domain.QuotaMetric]int{domain.QuotaTelegramGroups: 1})
	f.usecase.quota = quota
	f.auth.dialogs = []domain.TelegramDialog{openGroup(-1001, "Yuk markazi"), openGroup(-1002, "Fura bor")}

	first := dto.ConnectGroupRequest{AccountID: account.ID, PeerID: "-1001"}
	if _, err := f.usecase.ConnectGroup(context.Background(), f.userID, f.companyID, first); err != nil {
		t.Fatalf("ConnectGroup: %v", err)
	}
	if _, err := f.usecase.ConnectGroup(context.Background(), f.userID, f.companyID, first); err != nil {
		t.Fatalf("reconnecting the same group was refused: %v", err)
	}

	_, err := f.usecase.ConnectGroup(context.Background(), f.userID, f.companyID,
		dto.ConnectGroupRequest{AccountID: account.ID, PeerID: "-1002"})
	wantCode(t, err, apperror.ErrQuotaExceeded.Code)
	if len(f.groups.groups) != 1 {
		t.Errorf("groups = %d, want the one the plan allows", len(f.groups.groups))
	}
}
