package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

type inbox struct{ sent []domain.Notification }

func (i *inbox) Notify(ctx context.Context, n domain.Notification) error {
	return i.NotifyMany(ctx, []domain.Notification{n})
}

func (i *inbox) NotifyMany(_ context.Context, notifications []domain.Notification) error {
	i.sent = append(i.sent, notifications...)
	return nil
}

func (i *inbox) to(userID uuid.UUID) *domain.Notification {
	for k := range i.sent {
		if i.sent[k].UserID == userID {
			return &i.sent[k]
		}
	}
	return nil
}

// people is the user directory: who exists, in which language, with which
// platform role.
type people struct {
	domain.UserRepository
	users  map[uuid.UUID]*domain.User
	admins map[string][]uuid.UUID
}

func (p *people) FindByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	if u, ok := p.users[id]; ok {
		return u, nil
	}
	return nil, errors.New("no such user")
}

func (p *people) List(_ context.Context, filter domain.UserFilter, _, _ int, _ string) ([]domain.User, int64, error) {
	var out []domain.User
	for _, id := range p.admins[filter.RoleCode] {
		out = append(out, *p.users[id])
	}
	return out, int64(len(out)), nil
}

type roster struct {
	domain.CompanyRepository
	members []domain.CompanyMember
	company domain.Company
}

func (r *roster) ListMembers(context.Context, uuid.UUID, bool) ([]domain.CompanyMember, error) {
	return r.members, nil
}

func (r *roster) FindByID(context.Context, uuid.UUID) (*domain.Company, error) {
	return &r.company, nil
}

type brokenOnce struct{}

func (brokenOnce) SetNX(context.Context, string, any, time.Duration) (bool, error) {
	return false, errors.New("redis down")
}

type alertFixture struct {
	alerts  *DistributionAlertUsecase
	inbox   *inbox
	people  *people
	company uuid.UUID
	owner   uuid.UUID
	manager uuid.UUID
	clerk   uuid.UUID
	admin   uuid.UUID
}

func newAlertFixture(t *testing.T) *alertFixture {
	t.Helper()
	f := &alertFixture{
		inbox:   &inbox{},
		company: uuid.New(),
		owner:   uuid.New(),
		manager: uuid.New(),
		clerk:   uuid.New(),
		admin:   uuid.New(),
	}
	user := func(id uuid.UUID, locale domain.Locale) *domain.User {
		return &domain.User{ID: id, Locale: locale, Status: domain.UserStatusActive}
	}
	f.people = &people{
		users: map[uuid.UUID]*domain.User{
			f.owner:   user(f.owner, domain.LocaleRU),
			f.manager: user(f.manager, domain.LocaleEN),
			f.clerk:   user(f.clerk, domain.LocaleUZ),
			f.admin:   user(f.admin, domain.LocaleUZ),
		},
		admins: map[string][]uuid.UUID{domain.RoleAdmin: {f.admin}},
	}
	companies := &roster{
		company: domain.Company{ID: f.company, BrandName: "Yuk Express"},
		members: []domain.CompanyMember{
			{CompanyID: f.company, UserID: f.owner, MemberRole: domain.MemberRoleDispatcher},
			{CompanyID: f.company, UserID: f.manager, MemberRole: domain.MemberRoleManager},
			{CompanyID: f.company, UserID: f.clerk, MemberRole: domain.MemberRoleDispatcher},
		},
	}
	f.alerts = NewDistributionAlertUsecase(f.inbox, f.people, companies, newTestCache(t),
		time.FixedZone("+05", 5*60*60), logger.New("test", "error"))
	f.alerts.now = func() time.Time { return time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC) }
	return f
}

func (f *alertFixture) alert(kind domain.DistributionAlertKind) domain.DistributionAlert {
	return domain.DistributionAlert{
		Kind: kind, CompanyID: f.company, AccountID: uuid.New(), OwnerID: f.owner,
		AccountLabel: "+99890 *** ** 67", Cancelled: 4,
	}
}

func TestARestrictionReachesTheOwnerManagersAndAdmins(t *testing.T) {
	f := newAlertFixture(t)
	alert := f.alert(domain.AlertAccountRestricted)

	f.alerts.Alert(context.Background(), alert)

	if len(f.inbox.sent) != 3 {
		t.Fatalf("sent %d, want owner, manager and admin", len(f.inbox.sent))
	}
	if f.inbox.to(f.clerk) != nil {
		t.Error("a dispatcher who does not own the account was told")
	}

	owner := f.inbox.to(f.owner)
	if owner == nil || !strings.Contains(owner.Body, "Telegram ограничил аккаунт +99890 *** ** 67") ||
		!strings.Contains(owner.Body, "4") {
		t.Errorf("owner got %+v, want Russian with the account and the cancelled count", owner)
	}
	if owner.Priority != "high" || owner.Type != string(domain.AlertAccountRestricted) {
		t.Errorf("owner notification = %+v", owner)
	}
	if manager := f.inbox.to(f.manager); manager == nil || !strings.HasPrefix(manager.Title, "Telegram restricted") {
		t.Errorf("manager got %+v, want English", manager)
	}
	if admin := f.inbox.to(f.admin); admin == nil || !strings.Contains(admin.Body, "«Yuk Express»") {
		t.Errorf("admin got %+v, want the company named", admin)
	}

	f.alerts.Alert(context.Background(), alert)
	if len(f.inbox.sent) != 3 {
		t.Errorf("the same restriction was reported twice (%d messages)", len(f.inbox.sent))
	}
}

func TestOnlyALongPauseIsReportedAndOnlyToTheOwner(t *testing.T) {
	f := newAlertFixture(t)

	short := f.alert(domain.AlertAccountPaused)
	soon := f.alerts.now().Add(5 * time.Minute)
	short.Until = &soon
	f.alerts.Alert(context.Background(), short)
	if len(f.inbox.sent) != 0 {
		t.Fatalf("a five-minute wait was reported: %+v", f.inbox.sent)
	}

	long := f.alert(domain.AlertAccountPaused)
	later := f.alerts.now().Add(30 * time.Minute)
	long.Until = &later
	f.alerts.Alert(context.Background(), long)
	if len(f.inbox.sent) != 1 || f.inbox.sent[0].UserID != f.owner {
		t.Fatalf("sent %+v, want the owner only", f.inbox.sent)
	}
	// 09:30 UTC is 14:30 in Tashkent.
	if !strings.Contains(f.inbox.sent[0].Body, "17.09 14:30") {
		t.Errorf("body = %q, want the local time", f.inbox.sent[0].Body)
	}
	if f.inbox.sent[0].Data["until"] != "2026-09-17T09:30:00Z" {
		t.Errorf("data = %v", f.inbox.sent[0].Data)
	}
}

func TestAVeryLongPauseAlsoReachesPlatformAdmins(t *testing.T) {
	f := newAlertFixture(t)
	alert := f.alert(domain.AlertAccountPaused)
	until := f.alerts.now().Add(3 * time.Hour)
	alert.Until = &until

	f.alerts.Alert(context.Background(), alert)

	if len(f.inbox.sent) != 2 || f.inbox.to(f.manager) != nil {
		t.Fatalf("sent %+v, want the owner and the admin", f.inbox.sent)
	}
	if admin := f.inbox.to(f.admin); admin == nil || !strings.Contains(admin.Body, "«Yuk Express»") {
		t.Errorf("admin got %+v, want the company named", admin)
	}
}

func TestAnUnknownTelegramErrorGoesToAdminsOncePerCode(t *testing.T) {
	f := newAlertFixture(t)
	alert := f.alert(domain.AlertTelegramUnknown)
	alert.ErrorCode = "SOMETHING_NEW"

	f.alerts.Alert(context.Background(), alert)
	if len(f.inbox.sent) != 1 || f.inbox.sent[0].UserID != f.admin {
		t.Fatalf("sent %+v, want the platform admin only", f.inbox.sent)
	}
	n := f.inbox.sent[0]
	if !strings.Contains(n.Body, "SOMETHING_NEW") || n.ActionURL != "" || n.Data["error_code"] != "SOMETHING_NEW" {
		t.Errorf("admin got %+v", n)
	}

	// The same code from another company's account is the same bug.
	alert.AccountID, alert.CompanyID = uuid.New(), uuid.New()
	f.alerts.Alert(context.Background(), alert)
	if len(f.inbox.sent) != 1 {
		t.Errorf("the same unknown code was reported again (%d messages)", len(f.inbox.sent))
	}
}

func TestAClosedGroupIsReportedOncePerGroup(t *testing.T) {
	f := newAlertFixture(t)
	alert := f.alert(domain.AlertGroupClosed)
	group := uuid.New()
	alert.GroupID, alert.GroupTitle = &group, "Fura bor"

	f.alerts.Alert(context.Background(), alert)
	if len(f.inbox.sent) != 2 || f.inbox.to(f.admin) != nil {
		t.Fatalf("sent %+v, want the owner and the manager", f.inbox.sent)
	}
	if n := f.inbox.to(f.owner); n.ActionURL != "/telegram/groups" || !strings.Contains(n.Body, "«Fura bor»") {
		t.Errorf("owner got %+v", n)
	}

	// Another account refused in the same group: still the same news.
	alert.AccountID = uuid.New()
	f.alerts.Alert(context.Background(), alert)
	if len(f.inbox.sent) != 2 {
		t.Errorf("the same group was reported again (%d messages)", len(f.inbox.sent))
	}

	other := uuid.New()
	alert.GroupID = &other
	f.alerts.Alert(context.Background(), alert)
	if len(f.inbox.sent) != 4 {
		t.Errorf("a different group was not reported (%d messages)", len(f.inbox.sent))
	}
}

func TestABlockedUserIsNotMessaged(t *testing.T) {
	f := newAlertFixture(t)
	f.people.users[f.manager].Status = domain.UserStatusBlocked

	f.alerts.Alert(context.Background(), f.alert(domain.AlertAccountRevoked))

	if f.inbox.to(f.manager) != nil {
		t.Error("a blocked manager was messaged")
	}
	if n := f.inbox.to(f.owner); n == nil || n.Title != "Сессия Telegram завершена" {
		t.Errorf("owner got %+v", n)
	}
}

func TestAnUnreachableDedupStoreStillReports(t *testing.T) {
	f := newAlertFixture(t)
	f.alerts.once = brokenOnce{}

	f.alerts.Alert(context.Background(), f.alert(domain.AlertAccountRevoked))

	if len(f.inbox.sent) == 0 {
		t.Fatal("a revoked session went unreported because Redis was down")
	}
}

func TestEveryAlertHasCopyInEveryLanguage(t *testing.T) {
	kinds := []domain.DistributionAlertKind{
		domain.AlertAccountRestricted, domain.AlertAccountRevoked, domain.AlertAccountPaused,
		domain.AlertGroupClosed, domain.AlertGroupLost, domain.AlertTelegramUnknown,
	}
	for _, kind := range kinds {
		for _, locale := range domain.AllLocales {
			for _, admin := range []bool{false, true} {
				text := alertCopy(domain.DistributionAlert{
					Kind: kind, AccountLabel: "acc", GroupTitle: "grp", ErrorCode: "CODE",
				},
					locale, "17.09 16:00", admin, "Co")
				if text.title == "Telegram" || text.body == string(kind) || strings.Contains(text.body, "%!") {
					t.Errorf("%s/%s/admin=%v has no proper copy: %+v", kind, locale, admin, text)
				}
			}
		}
	}
}
