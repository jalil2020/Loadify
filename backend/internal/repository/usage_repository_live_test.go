package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
)

// The usage and dashboard queries are raw SQL with named arguments, several of
// them optional; this runs each one against the real schema so a typo or a
// binding GORM gets wrong is caught before a person opens the dashboard.
func TestUsageAndDashboardQueriesRunAgainstTheSchema(t *testing.T) {
	db := liveDB(t)
	ctx := context.Background()
	company := uuid.New()
	now := time.Now()
	from, to := domain.BillingMonth(now, time.UTC)

	usage, err := NewUsageRepository(db).Usage(ctx, company, from, to)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	for _, metric := range domain.QuotaMetrics {
		if n, ok := usage[metric]; !ok || n != 0 {
			t.Errorf("%s = %d (present %v), want 0 for a company with nothing", metric, n, ok)
		}
	}

	user := uuid.New()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for name, who := range map[string]*uuid.UUID{"company": nil, "person": &user} {
		stats, err := NewDistributionStatsRepository(db).Stats(ctx, domain.DistributionScope{
			CompanyID: company, UserID: who,
			DayStart: day, DayEnd: day.AddDate(0, 0, 1),
			MonthStart: from, MonthEnd: to,
		})
		if err != nil {
			t.Fatalf("Stats (%s): %v", name, err)
		}
		if stats.LoadsToday != 0 || len(stats.PostsMonth) != 0 || len(stats.RecentProblems) != 0 {
			t.Errorf("Stats (%s) = %+v, want empty", name, stats)
		}
	}

	if _, err := NewAITaskRepository(db).SpentSince(ctx, day); err != nil {
		t.Fatalf("SpentSince: %v", err)
	}
}
