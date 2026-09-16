package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
)

type loadAdRepository struct{ Base }

// NewLoadAdRepository builds the advertisement repository.
func NewLoadAdRepository(db *gorm.DB) domain.LoadAdRepository {
	return &loadAdRepository{Base: NewBase(db)}
}

// scoped is every query's starting point. The company comes from the caller
// rather than from the row being written, so a handler that forgot to check
// ownership still cannot read or change another tenant's advert.
func (r *loadAdRepository) scoped(ctx context.Context, companyID uuid.UUID) *gorm.DB {
	return r.DB(ctx).Model(&domain.LoadAd{}).Where("company_id = ?", companyID)
}

func (r *loadAdRepository) Create(ctx context.Context, ad *domain.LoadAd) error {
	if err := r.DB(ctx).Create(ad).Error; err != nil {
		return translate(err, "load advert")
	}
	return nil
}

func (r *loadAdRepository) Update(ctx context.Context, ad *domain.LoadAd) error {
	res := r.DB(ctx).Model(&domain.LoadAd{}).
		Where("id = ? AND company_id = ?", ad.ID, ad.CompanyID).
		Select("*").
		Omit("id", "company_id", "created_by_user_id", "created_at").
		Updates(ad)
	if res.Error != nil {
		return translate(res.Error, "load advert")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrNotFound.WithMessage("load advert not found")
	}
	return nil
}

func (r *loadAdRepository) UpdateColumns(ctx context.Context, companyID, id uuid.UUID, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	res := r.scoped(ctx, companyID).Where("id = ?", id).Updates(columns)
	if res.Error != nil {
		return translate(res.Error, "load advert")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrNotFound.WithMessage("load advert not found")
	}
	return nil
}

func (r *loadAdRepository) FindByID(ctx context.Context, companyID, id uuid.UUID) (*domain.LoadAd, error) {
	var ad domain.LoadAd
	err := r.DB(ctx).Where("id = ? AND company_id = ?", id, companyID).First(&ad).Error
	if err != nil {
		return nil, translate(err, "load advert")
	}
	return &ad, nil
}

func (r *loadAdRepository) List(
	ctx context.Context,
	companyID uuid.UUID,
	filter domain.LoadAdFilter,
	offset, limit int,
) ([]domain.LoadAd, int64, error) {
	q := r.scoped(ctx, companyID)

	if len(filter.Statuses) > 0 {
		q = q.Where("status IN ?", filter.Statuses)
	}
	if filter.CreatedBy != nil {
		q = q.Where("created_by_user_id = ?", *filter.CreatedBy)
	}
	if filter.From != nil {
		q = q.Where("created_at >= ?", *filter.From)
	}
	if filter.To != nil {
		q = q.Where("created_at < ?", *filter.To)
	}
	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		q = q.Where("raw_text ILIKE ? OR origin_text ILIKE ? OR dest_text ILIKE ?", like, like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "load advert")
	}

	var ads []domain.LoadAd
	err := applyPage(q.Order("created_at DESC"), offset, limit).Find(&ads).Error
	if err != nil {
		return nil, 0, translate(err, "load advert")
	}
	return ads, total, nil
}

// Archive retires an advertisement without deleting it: the posts that went out
// still point at it, and a history row whose advert vanished explains nothing.
func (r *loadAdRepository) Archive(ctx context.Context, companyID, id uuid.UUID) error {
	res := r.scoped(ctx, companyID).
		Where("id = ? AND status <> ?", id, domain.LoadAdArchived).
		Updates(map[string]any{"status": domain.LoadAdArchived})
	if res.Error != nil {
		return translate(res.Error, "load advert")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrNotFound.WithMessage("load advert not found")
	}
	return nil
}

// CountCreatedBetween backs the monthly plan limit on adverts.
func (r *loadAdRepository) CountCreatedBetween(ctx context.Context, companyID uuid.UUID, from, to time.Time) (int64, error) {
	var total int64
	err := r.scoped(ctx, companyID).
		Where("created_at >= ? AND created_at < ?", from, to).
		Count(&total).Error
	if err != nil {
		return 0, translate(err, "load advert")
	}
	return total, nil
}
