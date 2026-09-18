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

// The group filter and the posting queue are raw SQL too. Every optional filter
// is exercised both empty and set, because a placeholder GORM fails to bind
// only shows up once the query reaches the server.
func TestDistributionQueriesRunAgainstTheSchema(t *testing.T) {
	db := liveDB(t)
	ctx := context.Background()
	groups := NewTelegramGroupRepository(db)
	company := uuid.New()

	vehicle, cargo, weight := "tent", "general", 20000
	country := "UZ"
	region := uuid.New()
	for name, query := range map[string]domain.GroupCandidateQuery{
		"no filters": {AccountID: uuid.New()},
		"every filter": {
			AccountID: uuid.New(), VehicleType: &vehicle, CargoTypeCode: &cargo, WeightKg: &weight,
			OriginCountry: &country, OriginRegionID: &region, DestCountry: &country, DestRegionID: &region,
		},
	} {
		found, err := groups.Candidates(ctx, company, query)
		if err != nil {
			t.Fatalf("Candidates (%s): %v", name, err)
		}
		if len(found) != 0 {
			t.Errorf("Candidates (%s) found %d groups for a company with none", name, len(found))
		}
	}
	if _, err := groups.ListWithRules(ctx, company); err != nil {
		t.Fatalf("ListWithRules: %v", err)
	}

	posting := NewPostingRepository(db)
	if _, err := posting.DueJobs(ctx, 10); err != nil {
		t.Fatalf("DueJobs: %v", err)
	}
	job, err := posting.Claim(ctx, uuid.New(), "live-test", time.Minute)
	if err != nil || job != nil {
		t.Fatalf("Claim of a missing job = %v, %v; want nothing and no error", job, err)
	}
	if queued, err := posting.QueuedGroups(ctx, company, uuid.New()); err != nil || len(queued) != 0 {
		t.Fatalf("QueuedGroups = %v, %v; want none", queued, err)
	}
	if _, _, err := posting.ListJobs(ctx, company, domain.PostingJobFilter{}, 0, 10); err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	backlog, err := NewPostingBacklogReader(db).Backlog(ctx)
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	for status := range backlog.Jobs {
		switch status {
		case domain.PostingPending, domain.PostingRateLimited, domain.PostingProcessing:
		default:
			t.Errorf("Backlog counted %q jobs, which are not waiting", status)
		}
	}
}
