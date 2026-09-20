package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

// planStore answers the two calls listing plans makes. The embedded interface
// is deliberate: any other call panics in the test that introduced it rather
// than quietly returning a zero value.
type planStore struct {
	domain.SubscriptionRepository
	plans  []domain.SubscriptionPlan
	active *domain.Subscription
}

func (s planStore) ListPlans(context.Context, string) ([]domain.SubscriptionPlan, error) {
	return s.plans, nil
}

func (s planStore) ActiveForCompany(context.Context, uuid.UUID) (*domain.Subscription, error) {
	return s.active, nil
}

func companyPlans() []domain.SubscriptionPlan {
	return []domain.SubscriptionPlan{
		{ID: uuid.New(), Code: "company_start", NameUZ: "Start", TargetRole: "company", IsDefault: true},
		{ID: uuid.New(), Code: "company_business_m", NameUZ: "Business", TargetRole: "company", PriceAmount: 29900000},
	}
}

func plansFor(t *testing.T, store planStore, companyID uuid.UUID) map[string]bool {
	t.Helper()
	u := NewPaymentUsecase(nil, store, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger.New("test", "error"))
	listed, err := u.ListPlans(context.Background(), "company", domain.LocaleUZ, nil, &companyID)
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	current := map[string]bool{}
	for _, plan := range listed {
		current[plan.Code] = plan.IsCurrent
	}
	return current
}

// A company with no subscription is not on "no plan": it is on the free one,
// which is what the quota guard enforces. Saying otherwise would offer people
// the tier they are already on.
func TestTheFreePlanIsCurrentUntilSomethingIsBought(t *testing.T) {
	plans := companyPlans()
	companyID := uuid.New()

	current := plansFor(t, planStore{plans: plans}, companyID)
	if !current["company_start"] || current["company_business_m"] {
		t.Errorf("without a subscription, current = %v; want the free default", current)
	}

	paid := plans[1]
	bought := planStore{plans: plans, active: &domain.Subscription{
		ID: uuid.New(), PlanID: paid.ID, CompanyID: &companyID, Status: domain.SubscriptionActive,
	}}
	current = plansFor(t, bought, companyID)
	if current["company_start"] || !current["company_business_m"] {
		t.Errorf("with a paid subscription, current = %v; want only the bought plan", current)
	}
}
