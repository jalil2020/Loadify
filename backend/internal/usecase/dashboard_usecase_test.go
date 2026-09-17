package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
)

type statsBook struct {
	scope domain.DistributionScope
	stats domain.DistributionStats
}

func (s *statsBook) Stats(_ context.Context, scope domain.DistributionScope) (*domain.DistributionStats, error) {
	s.scope = scope
	stats := s.stats
	return &stats, nil
}

func newDashboardFixture(stats domain.DistributionStats, quota domain.QuotaGuard) (*DashboardUsecase, *statsBook) {
	book := &statsBook{stats: stats}
	u := NewDashboardUsecase(book, quota, true, time.FixedZone("+05", 5*60*60))
	// 20:30 UTC on 17 September is 01:30 on the 18th in Tashkent.
	u.now = func() time.Time { return time.Date(2026, 9, 17, 20, 30, 0, 0, time.UTC) }
	return u, book
}

func TestTheDashboardDayIsTheCompanysDay(t *testing.T) {
	u, book := newDashboardFixture(domain.DistributionStats{}, newQuotaStub(nil))

	res, err := u.Dashboard(context.Background(), uuid.New(), uuid.New(), true)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if res.Day != "2026-09-18" {
		t.Errorf("day = %s, want the 18th in Tashkent", res.Day)
	}
	if got := book.scope.DayStart.Format(time.RFC3339); got != "2026-09-18T00:00:00+05:00" {
		t.Errorf("day starts %s", got)
	}
	if !book.scope.DayEnd.Equal(book.scope.DayStart.Add(24 * time.Hour)) {
		t.Errorf("day ends %s", book.scope.DayEnd)
	}
	if got := book.scope.MonthStart.Format(time.DateOnly); got != "2026-09-01" {
		t.Errorf("month starts %s", got)
	}
}

func TestAPersonsDashboardIsNarrowedToThem(t *testing.T) {
	u, book := newDashboardFixture(domain.DistributionStats{}, newQuotaStub(nil))
	userID := uuid.New()

	res, err := u.Dashboard(context.Background(), userID, uuid.New(), false)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if res.Scope != dto.DashboardScopeMine || book.scope.UserID == nil || *book.scope.UserID != userID {
		t.Errorf("scope = %s, user = %v; want the caller only", res.Scope, book.scope.UserID)
	}

	res, err = u.Dashboard(context.Background(), userID, uuid.New(), true)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if res.Scope != dto.DashboardScopeCompany || book.scope.UserID != nil {
		t.Errorf("scope = %s, user = %v; want the whole company", res.Scope, book.scope.UserID)
	}
}

func TestTheDashboardAddsUpPostsAndCosts(t *testing.T) {
	skip := string(domain.SkipCooldown)
	stats := domain.DistributionStats{
		LoadsToday: 2,
		PostsToday: map[domain.PostingJobStatus]int{
			domain.PostingSent: 9, domain.PostingFailed: 1, domain.PostingSkipped: 4,
			domain.PostingPending: 2, domain.PostingRateLimited: 1,
		},
		Accounts:          map[domain.TelegramAccountStatus]int{domain.TelegramAccountActive: 2, domain.TelegramAccountPaused: 1},
		AICostMonthMicros: 1_250_000,
		RecentProblems: []domain.DistributionProblem{{
			JobID: uuid.New(), GroupTitle: "Yuk markazi", Status: domain.PostingSkipped, SkipReason: &skip,
		}},
	}
	quota := newQuotaStub(map[domain.QuotaMetric]int{domain.QuotaPostsPerMonth: 100})
	quota.used[domain.QuotaPostsPerMonth] = 30
	u, _ := newDashboardFixture(stats, quota)

	res, err := u.Dashboard(context.Background(), uuid.New(), uuid.New(), true)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}

	today := res.Posts.Today
	if today.Total != 17 || today.Sent != 9 || today.Queued != 3 || today.Skipped != 4 || today.Failed != 1 {
		t.Errorf("posts today = %+v", today)
	}
	// Skips are the guard doing its job and do not count against success.
	if res.Posts.SuccessRate == nil || *res.Posts.SuccessRate != 0.9 {
		t.Errorf("success rate = %v, want 0.9", res.Posts.SuccessRate)
	}
	if res.Accounts.Total != 3 || res.Accounts.ByStatus["paused"] != 1 {
		t.Errorf("accounts = %+v", res.Accounts)
	}
	if res.AI.CostMonthUSD != 1.25 || !res.AI.Enabled {
		t.Errorf("ai = %+v", res.AI)
	}
	if len(res.RecentProblems) != 1 || res.RecentProblems[0].Reason != "cooldown" {
		t.Errorf("problems = %+v", res.RecentProblems)
	}

	for _, item := range res.Quota {
		switch item.Metric {
		case string(domain.QuotaPostsPerMonth):
			if item.Limit == nil || *item.Limit != 100 || item.Remaining == nil || *item.Remaining != 70 {
				t.Errorf("posts quota = %+v", item)
			}
		default:
			if item.Limit != nil || item.Remaining != nil {
				t.Errorf("%s has no limit and should say null, got %+v", item.Metric, item)
			}
		}
	}
}

func TestNoFinishedPostsMeansNoSuccessRate(t *testing.T) {
	u, _ := newDashboardFixture(domain.DistributionStats{
		PostsToday: map[domain.PostingJobStatus]int{domain.PostingPending: 5},
	}, newQuotaStub(nil))

	res, err := u.Dashboard(context.Background(), uuid.New(), uuid.New(), true)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if res.Posts.SuccessRate != nil {
		t.Errorf("success rate = %v before anything finished, want null", *res.Posts.SuccessRate)
	}
}
