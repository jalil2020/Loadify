package usecase

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

const (
	// alertOnceWindow is how long one account or group problem is reported
	// once. A burst of refusals from the same cause is one message, not twenty.
	alertOnceWindow = 24 * time.Hour
	// alertPauseThreshold is the shortest wait worth telling anyone about.
	// Telegram asks for short waits routinely and the queue absorbs them.
	alertPauseThreshold = 15 * time.Minute
	// alertAdminPause is the wait at which platform admins hear about it too:
	// Telegram asking for an hour or more means the limits were too loose.
	alertAdminPause = time.Hour
	// alertMaxAdmins bounds the platform-admin fan-out.
	alertMaxAdmins = 50
	// alertTimeout lets the message outlive a worker that is shutting down
	// right after the send that caused it.
	alertTimeout = 10 * time.Second
)

// AlertDeduper remembers that something was already reported.
type AlertDeduper interface {
	SetNX(ctx context.Context, key string, value any, ttl time.Duration) (bool, error)
}

// DistributionAlertUsecase tells people when sending stops for a reason they
// have to act on: Telegram restricting or ending an account, a long pause, a
// group that no longer accepts the account.
//
// The owner of the account hears about it in their own language. Company
// managers hear about everything but pauses, because they decide whose
// account posts where. Platform admins hear what only the platform can fix:
// restrictions and very long pauses, which say the limits were not enough, and
// errors nobody has classified yet.
type DistributionAlertUsecase struct {
	notifier  domain.Notifier
	users     domain.UserRepository
	companies domain.CompanyRepository
	once      AlertDeduper
	location  *time.Location
	log       logger.Logger
	now       func() time.Time
}

var _ domain.DistributionAlerter = (*DistributionAlertUsecase)(nil)

// NewDistributionAlertUsecase wires the alerts. location is the timezone times
// are written in.
func NewDistributionAlertUsecase(
	notifier domain.Notifier,
	users domain.UserRepository,
	companies domain.CompanyRepository,
	once AlertDeduper,
	location *time.Location,
	log logger.Logger,
) *DistributionAlertUsecase {
	if location == nil {
		location = time.UTC
	}
	return &DistributionAlertUsecase{
		notifier: notifier, users: users, companies: companies, once: once,
		location: location, log: log, now: time.Now,
	}
}

type alertRecipient struct {
	userID uuid.UUID
	admin  bool
}

// Alert reports one event. It never returns an error: what happened is already
// recorded on the job, the account and the group.
func (u *DistributionAlertUsecase) Alert(ctx context.Context, alert domain.DistributionAlert) {
	if alert.Kind == domain.AlertAccountPaused &&
		(alert.Until == nil || alert.Until.Sub(u.now()) < alertPauseThreshold) {
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), alertTimeout)
	defer cancel()

	if !u.first(ctx, alert) {
		return
	}

	recipients := u.recipients(ctx, alert)
	company := ""
	for _, r := range recipients {
		if r.admin {
			company = u.companyName(ctx, alert.CompanyID)
			break
		}
	}

	notifications := make([]domain.Notification, 0, len(recipients))
	for _, r := range recipients {
		locale := domain.LocaleUZ
		user, err := u.users.FindByID(ctx, r.userID)
		switch {
		case err != nil:
			u.log.Warn("alert recipient not loaded, writing in Uzbek",
				logger.String("user_id", r.userID.String()), logger.Err(err))
		case !user.IsActive():
			continue
		default:
			locale = user.Locale
		}

		text := alertCopy(alert, locale, u.when(alert.Until), r.admin, company)
		notifications = append(notifications, domain.Notification{
			UserID:    r.userID,
			Type:      string(alert.Kind),
			Title:     text.title,
			Body:      text.body,
			Data:      alertData(alert),
			Priority:  alertPriority(alert.Kind),
			ActionURL: alertAction(alert.Kind),
		})
	}

	if err := u.notifier.NotifyMany(ctx, notifications); err != nil {
		u.log.Warn("distribution alert not delivered",
			logger.String("kind", string(alert.Kind)), logger.Err(err))
	}
}

// first reports whether this is the first time this problem is reported. When
// the store cannot say, the alert goes out: a repeated message is better than
// a missed restriction.
func (u *DistributionAlertUsecase) first(ctx context.Context, alert domain.DistributionAlert) bool {
	subject := alert.AccountID.String()
	switch {
	case alert.Kind == domain.AlertTelegramUnknown:
		// One message per new code, however many posts it hits.
		subject = alert.ErrorCode
	case alert.GroupID != nil:
		subject = alert.GroupID.String()
	}
	window := alertOnceWindow
	if alert.Kind == domain.AlertAccountPaused {
		// One message per pause: a new, later pause is news.
		subject += ":" + strconv.FormatInt(alert.Until.Unix()/60, 10)
		window = alert.Until.Sub(u.now())
	}

	fresh, err := u.once.SetNX(ctx, "alert:"+string(alert.Kind)+":"+subject, 1, window)
	if err != nil {
		u.log.Warn("alert dedup unavailable, sending anyway", logger.Err(err))
		return true
	}
	return fresh
}

func (u *DistributionAlertUsecase) recipients(ctx context.Context, alert domain.DistributionAlert) []alertRecipient {
	seen := map[uuid.UUID]bool{}
	var out []alertRecipient
	add := func(id uuid.UUID, admin bool) {
		if id == uuid.Nil || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, alertRecipient{userID: id, admin: admin})
	}

	if alert.Kind == domain.AlertTelegramUnknown {
		u.addAdmins(ctx, add)
		return out
	}

	add(alert.OwnerID, false)

	if alert.Kind != domain.AlertAccountPaused {
		members, err := u.companies.ListMembers(ctx, alert.CompanyID, true)
		if err != nil {
			u.log.Warn("company managers not loaded for an alert", logger.Err(err))
		}
		for _, m := range members {
			if m.MemberRole == domain.MemberRoleManager {
				add(m.UserID, false)
			}
		}
	}

	longPause := alert.Kind == domain.AlertAccountPaused && alert.Until.Sub(u.now()) >= alertAdminPause
	if alert.Kind == domain.AlertAccountRestricted || longPause {
		u.addAdmins(ctx, add)
	}
	return out
}

func (u *DistributionAlertUsecase) addAdmins(ctx context.Context, add func(uuid.UUID, bool)) {
	for _, role := range []string{domain.RoleAdmin, domain.RoleSuperAdmin} {
		admins, _, err := u.users.List(ctx, domain.UserFilter{
			RoleCode: role,
			Status:   []domain.UserStatus{domain.UserStatusActive},
		}, 0, alertMaxAdmins, "")
		if err != nil {
			u.log.Warn("platform admins not loaded for an alert", logger.Err(err))
			continue
		}
		for _, admin := range admins {
			add(admin.ID, true)
		}
	}
}

func (u *DistributionAlertUsecase) companyName(ctx context.Context, companyID uuid.UUID) string {
	company, err := u.companies.FindByID(ctx, companyID)
	if err != nil {
		return companyID.String()
	}
	if company.BrandName != "" {
		return company.BrandName
	}
	return company.LegalName
}

func (u *DistributionAlertUsecase) when(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.In(u.location).Format("02.01 15:04")
}

func alertData(alert domain.DistributionAlert) map[string]any {
	data := map[string]any{
		"kind":       string(alert.Kind),
		"company_id": alert.CompanyID.String(),
		"account_id": alert.AccountID.String(),
	}
	if alert.GroupID != nil {
		data["group_id"] = alert.GroupID.String()
	}
	if alert.Until != nil {
		data["until"] = alert.Until.UTC().Format(time.RFC3339)
	}
	if alert.Cancelled > 0 {
		data["cancelled"] = strconv.FormatInt(alert.Cancelled, 10)
	}
	if alert.ErrorCode != "" {
		data["error_code"] = alert.ErrorCode
	}
	return data
}

func alertPriority(kind domain.DistributionAlertKind) string {
	switch kind {
	case domain.AlertAccountRestricted, domain.AlertAccountRevoked, domain.AlertTelegramUnknown:
		return "high"
	default:
		return "normal"
	}
}

func alertAction(kind domain.DistributionAlertKind) string {
	switch kind {
	case domain.AlertGroupClosed, domain.AlertGroupLost:
		return "/telegram/groups"
	case domain.AlertTelegramUnknown:
		// Nothing in the product to open; the answer is in the logs.
		return ""
	default:
		return "/telegram/accounts"
	}
}

type alertText struct{ title, body string }

// alertCopy writes the message. Every text says what happened, what the
// platform already did about it, and what the person can do.
func alertCopy(
	alert domain.DistributionAlert,
	locale domain.Locale,
	when string,
	admin bool,
	company string,
) alertText {
	label, group, n, code := alert.AccountLabel, alert.GroupTitle, alert.Cancelled, alert.ErrorCode

	switch locale {
	case domain.LocaleRU:
		switch alert.Kind {
		case domain.AlertAccountRestricted:
			if admin {
				return alertText{"Telegram ограничил аккаунт клиента", fmt.Sprintf(
					"Аккаунт %s компании «%s» получил PEER_FLOOD. Публикация с него остановлена, "+
						"отменено постов в очереди: %d. Проверьте лимиты.", label, company, n)}
			}
			return alertText{"Telegram ограничил аккаунт", fmt.Sprintf(
				"Telegram ограничил аккаунт %s за слишком частые сообщения. Публикация с него остановлена, "+
					"отменено постов в очереди: %d. Проверьте статус через @SpamBot.", label, n)}
		case domain.AlertAccountRevoked:
			return alertText{"Сессия Telegram завершена", fmt.Sprintf(
				"Сессия аккаунта %s завершена: выход в Telegram или смена пароля. "+
					"Подключите его заново. Отменено постов в очереди: %d.", label, n)}
		case domain.AlertAccountPaused:
			if admin {
				return alertText{"Telegram надолго остановил аккаунт клиента", fmt.Sprintf(
					"Аккаунт %s компании «%s» не сможет публиковать до %s. Возможно, лимиты слишком мягкие.",
					label, company, when)}
			}
			return alertText{"Telegram попросил подождать", fmt.Sprintf(
				"Аккаунт %s не будет публиковать до %s — так попросил Telegram. "+
					"Посты в очереди продолжатся сами.", label, when)}
		case domain.AlertGroupClosed:
			return alertText{"Нельзя писать в группу", fmt.Sprintf(
				"Аккаунт %s больше не может писать в «%s»: группа ограничила сообщения или аккаунт заблокирован. "+
					"Публикация в эту группу отключена.", label, group)}
		case domain.AlertGroupLost:
			return alertText{"Аккаунт больше не в группе", fmt.Sprintf(
				"%s больше не состоит в «%s». Группа отключена; если вернётесь в неё, подключите заново.",
				label, group)}
		case domain.AlertTelegramUnknown:
			return alertText{"Telegram вернул неизвестную ошибку", fmt.Sprintf(
				"Telegram вернул ошибку %s, которую платформа не распознаёт (компания «%s»). "+
					"Пост не отправлялся повторно. Проверьте логи и добавьте классификацию.", code, company)}
		}

	case domain.LocaleEN:
		switch alert.Kind {
		case domain.AlertAccountRestricted:
			if admin {
				return alertText{"Telegram restricted a customer account", fmt.Sprintf(
					"%s at %q got PEER_FLOOD. Posting from it stopped and %d queued posts were cancelled. "+
						"Review the limits.", label, company, n)}
			}
			return alertText{"Telegram restricted an account", fmt.Sprintf(
				"Telegram restricted %s for sending too much. Posting from it has stopped and %d queued posts "+
					"were cancelled. Check its status with @SpamBot.", label, n)}
		case domain.AlertAccountRevoked:
			return alertText{"Telegram session ended", fmt.Sprintf(
				"The session for %s has ended: it was signed out in Telegram or the password changed. "+
					"Connect it again. %d queued posts were cancelled.", label, n)}
		case domain.AlertAccountPaused:
			if admin {
				return alertText{"Telegram paused a customer account for long", fmt.Sprintf(
					"%s at %q cannot post until %s. The limits may be too loose.", label, company, when)}
			}
			return alertText{"Telegram asked for a pause", fmt.Sprintf(
				"%s will not post until %s, as Telegram asked. Queued posts resume on their own.", label, when)}
		case domain.AlertGroupClosed:
			return alertText{"Can't write to a group", fmt.Sprintf(
				"%s can no longer write in %q: the group restricted messages or the account was blocked. "+
					"Posting to it is turned off.", label, group)}
		case domain.AlertGroupLost:
			return alertText{"Account left a group", fmt.Sprintf(
				"%s is no longer in %q. The group was turned off; if you rejoin it, connect it again.",
				label, group)}
		case domain.AlertTelegramUnknown:
			return alertText{"Telegram returned an unknown error", fmt.Sprintf(
				"Telegram returned %s, which the platform does not recognise (company %q). "+
					"The post was not retried. Check the logs and classify it.", code, company)}
		}
	}

	switch alert.Kind {
	case domain.AlertAccountRestricted:
		if admin {
			return alertText{"Mijoz akkaunti Telegram tomonidan cheklandi", fmt.Sprintf(
				"«%s» kompaniyasining %s akkaunti PEER_FLOOD oldi. Undan yuborish to‘xtatildi, "+
					"navbatdagi %d ta post bekor qilindi. Limitlarni tekshiring.", company, label, n)}
		}
		return alertText{"Telegram akkauntni chekladi", fmt.Sprintf(
			"Telegram %s akkauntini juda ko‘p xabar yuborgani uchun chekladi. Undan e’lon yuborish to‘xtatildi, "+
				"navbatdagi %d ta post bekor qilindi. Holatini @SpamBot orqali tekshiring.", label, n)}
	case domain.AlertAccountRevoked:
		return alertText{"Telegram sessiyasi tugadi", fmt.Sprintf(
			"%s akkauntining sessiyasi tugadi: Telegram’da chiqib ketilgan yoki parol o‘zgargan. "+
				"Akkauntni qayta ulang. Navbatdagi %d ta post bekor qilindi.", label, n)}
	case domain.AlertAccountPaused:
		if admin {
			return alertText{"Telegram mijoz akkauntini uzoq to‘xtatdi", fmt.Sprintf(
				"«%s» kompaniyasining %s akkaunti %s gacha post yubora olmaydi. "+
					"Limitlar yetarlicha qat’iy emasligi mumkin.", company, label, when)}
		}
		return alertText{"Telegram kutishni so‘radi", fmt.Sprintf(
			"%s akkaunti %s gacha post yubormaydi — buni Telegram so‘radi. "+
				"Navbatdagi postlar keyin o‘zi davom etadi.", label, when)}
	case domain.AlertGroupClosed:
		return alertText{"Guruhga yozib bo‘lmayapti", fmt.Sprintf(
			"%s akkaunti «%s» guruhiga endi yoza olmaydi: guruh yozishni cheklagan yoki akkaunt bloklangan. "+
				"Bu guruhga post yuborish o‘chirildi.", label, group)}
	case domain.AlertGroupLost:
		return alertText{"Akkaunt guruhdan chiqqan", fmt.Sprintf(
			"%s endi «%s» guruhida emas. Guruh o‘chirildi; qayta qo‘shilsangiz, uni yana ulang.",
			label, group)}
	case domain.AlertTelegramUnknown:
		return alertText{"Telegram noma’lum xato qaytardi", fmt.Sprintf(
			"Telegram %s xatosini qaytardi, platforma uni tanimaydi («%s» kompaniyasi). "+
				"Post qayta yuborilmadi. Loglarni tekshirib, xatoni tasniflang.", code, company)}
	}
	return alertText{"Telegram", string(alert.Kind)}
}
