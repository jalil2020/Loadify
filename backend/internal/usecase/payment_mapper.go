package usecase

import (
	"encoding/json"
	"math"
	"time"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
)

// planResponse converts a plan, localising its name and decoding its feature list.
func planResponse(p *domain.SubscriptionPlan, locale domain.Locale) *dto.PlanResponse {
	res := &dto.PlanResponse{
		ID:          p.ID,
		Code:        p.Code,
		Name:        domain.LocalisedName(p.NameUZ, p.NameRU, p.NameEN, locale),
		Description: p.Description,
		TargetRole:  p.TargetRole,
		PriceAmount: p.PriceAmount,
		Currency:    p.Currency,
		Period:      p.Period,
		TrialDays:   p.TrialDays,
		Features:    []string{},
		Limits:      map[string]any(p.Limits),
		IsDefault:   p.IsDefault,
	}
	if res.Limits == nil {
		res.Limits = map[string]any{}
	}
	// Features are stored as a raw JSON array; a malformed value yields an empty
	// list rather than failing the whole pricing page.
	if len(p.Features) > 0 {
		var features []string
		if err := json.Unmarshal(p.Features, &features); err == nil {
			res.Features = features
		}
	}
	return res
}

// subscriptionResponse converts a subscription, including the days a client
// needs to render "expires in N days".
func subscriptionResponse(s *domain.Subscription, locale domain.Locale) *dto.SubscriptionResponse {
	res := &dto.SubscriptionResponse{
		ID:          s.ID,
		Status:      string(s.Status),
		StartedAt:   s.StartedAt,
		ExpiresAt:   s.ExpiresAt,
		CancelledAt: s.CancelledAt,
		AutoRenew:   s.AutoRenew,
		CreatedAt:   s.CreatedAt,
	}
	if s.Plan != nil {
		res.Plan = planResponse(s.Plan, locale)
		res.Plan.IsCurrent = true
	}
	if s.ExpiresAt != nil {
		days := int(math.Ceil(time.Until(*s.ExpiresAt).Hours() / 24))
		if days < 0 {
			days = 0
		}
		res.DaysLeft = &days
	}
	return res
}

func checkoutResponse(o *domain.PaymentOrder) *dto.CheckoutResponse {
	res := &dto.CheckoutResponse{
		OrderID:  o.ID,
		Provider: string(o.Provider),
		Amount:   o.Amount,
		Currency: o.Currency,
		Status:   string(o.Status),
		Paid:     o.Status == domain.OrderPaid,
	}
	if o.CheckoutURL != nil {
		res.CheckoutURL = *o.CheckoutURL
	}
	return res
}

func orderResponse(o *domain.PaymentOrder) dto.OrderResponse {
	return dto.OrderResponse{
		ID:           o.ID,
		Provider:     string(o.Provider),
		Purpose:      string(o.Purpose),
		Amount:       o.Amount,
		Currency:     o.Currency,
		Status:       string(o.Status),
		FailedReason: o.FailedReason,
		PaidAt:       o.PaidAt,
		CreatedAt:    o.CreatedAt,
	}
}

func refundResponse(r *domain.Refund) *dto.RefundResponse {
	return &dto.RefundResponse{
		ID:          r.ID,
		OrderID:     r.PaymentOrderID,
		Amount:      r.Amount,
		Currency:    r.Currency,
		Reason:      r.Reason,
		Status:      r.Status,
		ProcessedAt: r.ProcessedAt,
		CreatedAt:   r.CreatedAt,
	}
}

func invoiceResponse(i *domain.Invoice) dto.InvoiceResponse {
	return dto.InvoiceResponse{
		ID:        i.ID,
		Number:    i.Number,
		Amount:    i.Amount,
		TaxAmount: i.TaxAmount,
		Currency:  i.Currency,
		Status:    i.Status,
		IssuedAt:  i.IssuedAt,
		DueAt:     i.DueAt,
		PaidAt:    i.PaidAt,
	}
}
