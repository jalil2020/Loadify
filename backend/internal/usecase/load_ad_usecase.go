package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
)

// LoadAdUsecase owns a freight advertisement from the moment someone types it
// until it is ready to be distributed.
//
// Reading and storing are deliberately separate operations. Parse writes
// nothing: a person sees what was understood, corrects it, and only then saves.
// That gap is what keeps a misread from reaching a hundred groups.
type LoadAdUsecase struct {
	ads     domain.LoadAdRepository
	parser  domain.LoadAdParser
	quota   domain.QuotaGuard
	posting domain.PostingRepository
	tx      domain.TxManager
	audit   domain.AuditLogger
	log     logger.Logger
}

// NewLoadAdUsecase wires the advertisement usecase.
func NewLoadAdUsecase(
	ads domain.LoadAdRepository,
	parser domain.LoadAdParser,
	quota domain.QuotaGuard,
	posting domain.PostingRepository,
	tx domain.TxManager,
	audit domain.AuditLogger,
	log logger.Logger,
) *LoadAdUsecase {
	return &LoadAdUsecase{
		ads: ads, parser: parser, quota: quota, posting: posting, tx: tx, audit: audit, log: log,
	}
}

// Parse reads an advertisement as it was written and returns a draft. It stores
// no advert; the only thing it may write is the record of a model call, which
// is charged to the person's company.
func (u *LoadAdUsecase) Parse(
	ctx context.Context,
	userID, companyID uuid.UUID,
	text string,
) (dto.ParseAdResponse, error) {
	ctx = domain.WithActor(ctx, domain.Actor{UserID: userID, CompanyID: companyID})
	draft, err := u.parser.Parse(ctx, text)
	if err != nil {
		return dto.ParseAdResponse{}, err
	}
	return parseResponse(draft), nil
}

// Create stores an advertisement a person has confirmed.
func (u *LoadAdUsecase) Create(
	ctx context.Context,
	userID, companyID uuid.UUID,
	req dto.SaveLoadAdRequest,
) (*dto.LoadAdResponse, error) {
	ad := adFromRequest(req)
	ad.CompanyID = companyID
	ad.CreatedByUserID = userID
	ad.Status = statusFor(&ad)

	err := reserve(ctx, u.quota, companyID, domain.QuotaLoadsPerMonth, 1, func(ctx context.Context) error {
		return u.ads.Create(ctx, &ad)
	})
	if err != nil {
		return nil, err
	}
	u.record(ctx, userID, "load_ad.create", ad.ID, map[string]any{"fingerprint": ad.Fingerprint})

	res := adResponse(&ad)
	return &res, nil
}

// Update edits an advertisement that has not been archived.
func (u *LoadAdUsecase) Update(
	ctx context.Context,
	userID, companyID, id uuid.UUID,
	req dto.SaveLoadAdRequest,
) (*dto.LoadAdResponse, error) {
	existing, err := u.ads.FindByID(ctx, companyID, id)
	if err != nil {
		return nil, err
	}
	if existing.Status == domain.LoadAdArchived {
		return nil, apperror.ErrConflict.WithMessage("This advert is archived. Copy it into a new one to post it again.")
	}

	updated := adFromRequest(req)
	updated.ID = existing.ID
	updated.CompanyID = existing.CompanyID
	updated.CreatedByUserID = existing.CreatedByUserID
	updated.CreatedAt = existing.CreatedAt
	updated.PublishedAt = existing.PublishedAt
	updated.MarketplaceLoadID = existing.MarketplaceLoadID
	updated.Status = statusFor(&updated)
	// An advert that has already been posted stays published: editing it changes
	// what the next run sends, not what the groups already received.
	if existing.Status == domain.LoadAdPublished {
		updated.Status = domain.LoadAdPublished
	}

	if err := u.ads.Update(ctx, &updated); err != nil {
		return nil, err
	}
	u.record(ctx, userID, "load_ad.update", updated.ID, map[string]any{"fingerprint": updated.Fingerprint})

	res := adResponse(&updated)
	return &res, nil
}

// Get returns one advertisement belonging to the caller's company.
func (u *LoadAdUsecase) Get(ctx context.Context, companyID, id uuid.UUID) (*dto.LoadAdResponse, error) {
	ad, err := u.ads.FindByID(ctx, companyID, id)
	if err != nil {
		return nil, err
	}
	res := adResponse(ad)
	return &res, nil
}

// List returns the company's advertisements, newest first.
func (u *LoadAdUsecase) List(
	ctx context.Context,
	userID, companyID uuid.UUID,
	query dto.LoadAdQuery,
	offset, limit int,
) ([]dto.LoadAdResponse, int64, error) {
	filter := domain.LoadAdFilter{Search: query.Search}
	if query.Status != "" {
		filter.Statuses = []domain.LoadAdStatus{domain.LoadAdStatus(query.Status)}
	}
	if query.Mine {
		filter.CreatedBy = &userID
	}

	ads, total, err := u.ads.List(ctx, companyID, filter, offset, limit)
	if err != nil {
		return nil, 0, err
	}

	out := make([]dto.LoadAdResponse, 0, len(ads))
	for i := range ads {
		out = append(out, adResponse(&ads[i]))
	}
	return out, total, nil
}

// Archive retires an advertisement. The row stays: the posts that went out
// still point at it, and a history entry whose advert vanished explains nothing.
//
// Archiving also stops whatever is still queued for the advert, in the same
// transaction: an archived advert that keeps arriving in groups is exactly the
// spam the person was trying to stop.
func (u *LoadAdUsecase) Archive(ctx context.Context, userID, companyID, id uuid.UUID) error {
	var stopped int64
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := u.ads.Archive(ctx, companyID, id); err != nil {
			return err
		}
		var err error
		stopped, err = u.posting.CancelForLoad(ctx, companyID, id, "The advert was archived.")
		return err
	})
	if err != nil {
		return err
	}
	u.record(ctx, userID, "load_ad.archive", id, map[string]any{"cancelled_posts": stopped})
	return nil
}

// statusFor decides whether a saved advert is ready to be distributed. It is
// derived rather than accepted from the client: readiness is a property of the
// content, and a client that claimed otherwise would only be wrong.
func statusFor(ad *domain.LoadAd) domain.LoadAdStatus {
	if len(ad.MissingForPublish()) == 0 {
		return domain.LoadAdReady
	}
	return domain.LoadAdDraft
}

func (u *LoadAdUsecase) record(ctx context.Context, userID uuid.UUID, action string, id uuid.UUID, values map[string]any) {
	entry := domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       action,
		ResourceType: "load_ad",
		ResourceID:   &id,
		NewValues:    values,
	}
	if err := u.audit.Record(ctx, entry); err != nil {
		u.log.Warn("audit write failed",
			logger.String("action", action), logger.Err(err))
	}
}
