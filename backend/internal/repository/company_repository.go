package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
)

type companyRepository struct{ Base }

// NewCompanyRepository builds the company repository.
func NewCompanyRepository(db *gorm.DB) domain.CompanyRepository {
	return &companyRepository{Base: NewBase(db)}
}

func (r *companyRepository) Create(ctx context.Context, company *domain.Company) error {
	return translate(r.DB(ctx).Create(company).Error, "company")
}

func (r *companyRepository) Update(ctx context.Context, company *domain.Company) error {
	err := r.DB(ctx).Model(&domain.Company{ID: company.ID}).Updates(company).Error
	return translate(err, "company")
}

func (r *companyRepository) UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	err := r.DB(ctx).Model(&domain.Company{}).Where("id = ?", id).Updates(columns).Error
	return translate(err, "company")
}

func (r *companyRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Company, error) {
	var company domain.Company
	if err := r.DB(ctx).First(&company, "id = ?", id).Error; err != nil {
		return nil, translate(err, "company")
	}
	return &company, nil
}

func (r *companyRepository) FindWithRelations(ctx context.Context, id uuid.UUID) (*domain.Company, error) {
	var company domain.Company
	err := r.DB(ctx).
		Preload("Owner").
		Preload("Address").
		Preload("Address.Region").
		Preload("Address.City").
		Preload("Logo").
		Preload("Docs", func(db *gorm.DB) *gorm.DB { return db.Preload("File") }).
		Preload("Members", func(db *gorm.DB) *gorm.DB {
			return db.Where("is_active").Preload("User")
		}).
		First(&company, "id = ?", id).Error
	if err != nil {
		return nil, translate(err, "company")
	}
	return &company, nil
}

func (r *companyRepository) FindByOwner(ctx context.Context, ownerUserID uuid.UUID) (*domain.Company, error) {
	var company domain.Company
	err := r.DB(ctx).First(&company, "owner_user_id = ?", ownerUserID).Error
	if err != nil {
		return nil, translate(err, "company")
	}
	return &company, nil
}

func (r *companyRepository) FindByTIN(ctx context.Context, tin string) (*domain.Company, error) {
	var company domain.Company
	if err := r.DB(ctx).First(&company, "tin = ?", tin).Error; err != nil {
		return nil, translate(err, "company")
	}
	return &company, nil
}

func (r *companyRepository) List(ctx context.Context, filter domain.CompanyFilter, offset, limit int, order string) ([]domain.Company, int64, error) {
	q := r.DB(ctx).Model(&domain.Company{})

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		q = q.Where("brand_name ILIKE ? OR legal_name ILIKE ? OR tin ILIKE ? OR phone ILIKE ?",
			like, like, like, like)
	}
	if len(filter.Statuses) > 0 {
		q = q.Where("status IN ?", filter.Statuses)
	} else {
		q = q.Where("status = ?", domain.CompanyStatusVerified)
	}
	if filter.MinRating > 0 {
		q = q.Where("rating >= ?", filter.MinRating)
	}
	if filter.RegionID != nil {
		q = q.Where(`EXISTS (
			SELECT 1 FROM addresses a
			WHERE a.id = companies.address_id AND a.region_id = ?)`, *filter.RegionID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "company")
	}

	if order == "" {
		order = "rating DESC, completed_trips DESC"
	}
	var companies []domain.Company
	err := applyPage(q.Preload("Logo").Order(order), offset, limit).Find(&companies).Error
	if err != nil {
		return nil, 0, translate(err, "company")
	}
	return companies, total, nil
}

func (r *companyRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return translate(r.DB(ctx).Where("id = ?", id).Delete(&domain.Company{}).Error, "company")
}

func (r *companyRepository) RecalculateRating(ctx context.Context, companyID uuid.UUID) error {
	err := r.DB(ctx).Exec(`
		UPDATE companies SET
			rating = COALESCE(agg.avg_score, 0),
			rating_count = COALESCE(agg.total, 0)
		FROM (
			SELECT AVG(score)::numeric(3,2) AS avg_score, COUNT(*) AS total
			FROM ratings
			WHERE company_id = ? AND ratee_type = 'company' AND NOT is_hidden
		) agg
		WHERE companies.id = ?`, companyID, companyID).Error
	return translate(err, "company")
}

// ─── membership ─────────────────────────────────────────────────────

func (r *companyRepository) AddMember(ctx context.Context, member *domain.CompanyMember) error {
	return translate(r.DB(ctx).Create(member).Error, "company member")
}

func (r *companyRepository) UpdateMember(ctx context.Context, member *domain.CompanyMember) error {
	err := r.DB(ctx).Model(&domain.CompanyMember{ID: member.ID}).Updates(member).Error
	return translate(err, "company member")
}

func (r *companyRepository) FindMember(ctx context.Context, companyID, userID uuid.UUID) (*domain.CompanyMember, error) {
	var member domain.CompanyMember
	err := r.DB(ctx).Preload("User").
		First(&member, "company_id = ? AND user_id = ?", companyID, userID).Error
	if err != nil {
		return nil, translate(err, "company member")
	}
	return &member, nil
}

func (r *companyRepository) FindMemberByID(ctx context.Context, id uuid.UUID) (*domain.CompanyMember, error) {
	var member domain.CompanyMember
	if err := r.DB(ctx).Preload("User").First(&member, "id = ?", id).Error; err != nil {
		return nil, translate(err, "company member")
	}
	return &member, nil
}

func (r *companyRepository) ListMembers(ctx context.Context, companyID uuid.UUID, activeOnly bool) ([]domain.CompanyMember, error) {
	q := r.DB(ctx).Preload("User").Where("company_id = ?", companyID)
	if activeOnly {
		q = q.Where("is_active")
	}
	var members []domain.CompanyMember
	if err := q.Order("member_role, created_at").Find(&members).Error; err != nil {
		return nil, translate(err, "company member")
	}
	return members, nil
}

func (r *companyRepository) RemoveMember(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Where("id = ?", id).Delete(&domain.CompanyMember{}).Error
	return translate(err, "company member")
}

func (r *companyRepository) CountMembers(ctx context.Context, companyID uuid.UUID, role domain.MemberRole) (int64, error) {
	q := r.DB(ctx).Model(&domain.CompanyMember{}).
		Where("company_id = ? AND is_active", companyID)
	if role != "" {
		q = q.Where("member_role = ?", role)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, translate(err, "company member")
	}
	return count, nil
}

// ─── company documents ──────────────────────────────────────────────

type companyDocumentRepository struct{ Base }

// NewCompanyDocumentRepository builds the company-document repository.
func NewCompanyDocumentRepository(db *gorm.DB) domain.CompanyDocumentRepository {
	return &companyDocumentRepository{Base: NewBase(db)}
}

func (r *companyDocumentRepository) Upsert(ctx context.Context, doc *domain.CompanyDocument) error {
	db := r.DB(ctx)
	var existing domain.CompanyDocument
	err := db.Where("company_id = ? AND type = ?", doc.CompanyID, doc.Type).First(&existing).Error

	switch {
	case err == nil:
		doc.ID = existing.ID
		return translate(db.Model(&domain.CompanyDocument{}).
			Where("id = ?", existing.ID).
			Updates(map[string]any{
				"file_id":          doc.FileID,
				"number":           doc.Number,
				"issued_at":        doc.IssuedAt,
				"expires_at":       doc.ExpiresAt,
				"status":           domain.VerificationPending,
				"rejection_reason": nil,
				"reviewed_by":      nil,
				"reviewed_at":      nil,
			}).Error, "company document")
	case isNotFound(err):
		return translate(db.Create(doc).Error, "company document")
	default:
		return translate(err, "company document")
	}
}

func (r *companyDocumentRepository) ListByCompany(ctx context.Context, companyID uuid.UUID) ([]domain.CompanyDocument, error) {
	var docs []domain.CompanyDocument
	err := r.DB(ctx).Preload("File").
		Where("company_id = ?", companyID).Order("type").Find(&docs).Error
	if err != nil {
		return nil, translate(err, "company document")
	}
	return docs, nil
}

func (r *companyDocumentRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.CompanyDocument, error) {
	var doc domain.CompanyDocument
	if err := r.DB(ctx).Preload("File").First(&doc, "id = ?", id).Error; err != nil {
		return nil, translate(err, "company document")
	}
	return &doc, nil
}

func (r *companyDocumentRepository) Review(ctx context.Context, id uuid.UUID, status domain.VerificationStatus, reason *string, reviewedBy uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.CompanyDocument{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":           status,
			"rejection_reason": reason,
			"reviewed_by":      reviewedBy,
			"reviewed_at":      time.Now().UTC(),
		}).Error
	return translate(err, "company document")
}

func (r *companyDocumentRepository) ListPending(ctx context.Context, offset, limit int) ([]domain.CompanyDocument, int64, error) {
	q := r.DB(ctx).Model(&domain.CompanyDocument{}).Where("status = ?", domain.VerificationPending)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "company document")
	}

	var docs []domain.CompanyDocument
	err := applyPage(q.Preload("File").Order("created_at"), offset, limit).Find(&docs).Error
	if err != nil {
		return nil, 0, translate(err, "company document")
	}
	return docs, total, nil
}

func (r *companyDocumentRepository) CountApprovedByTypes(ctx context.Context, companyID uuid.UUID, types []domain.CompanyDocumentType) (int64, error) {
	if len(types) == 0 {
		return 0, nil
	}
	var count int64
	err := r.DB(ctx).Model(&domain.CompanyDocument{}).
		Distinct("type").
		Where("company_id = ? AND status = ? AND type IN ?",
			companyID, domain.VerificationApproved, types).
		Count(&count).Error
	if err != nil {
		return 0, translate(err, "company document")
	}
	return count, nil
}

func (r *companyDocumentRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Where("id = ?", id).Delete(&domain.CompanyDocument{}).Error
	return translate(err, "company document")
}
