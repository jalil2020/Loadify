package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/loadify/backend/internal/domain"
)

type vehicleRepository struct{ Base }

// NewVehicleRepository builds the vehicle repository.
func NewVehicleRepository(db *gorm.DB) domain.VehicleRepository {
	return &vehicleRepository{Base: NewBase(db)}
}

func (r *vehicleRepository) Create(ctx context.Context, vehicle *domain.Vehicle) error {
	return translate(r.DB(ctx).Create(vehicle).Error, "vehicle")
}

func (r *vehicleRepository) Update(ctx context.Context, vehicle *domain.Vehicle) error {
	err := r.DB(ctx).Model(&domain.Vehicle{ID: vehicle.ID}).Updates(vehicle).Error
	return translate(err, "vehicle")
}

func (r *vehicleRepository) UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	err := r.DB(ctx).Model(&domain.Vehicle{}).Where("id = ?", id).Updates(columns).Error
	return translate(err, "vehicle")
}

func (r *vehicleRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Vehicle, error) {
	var vehicle domain.Vehicle
	err := r.DB(ctx).
		Preload("Photos", func(db *gorm.DB) *gorm.DB { return db.Order("sort_order") }).
		Preload("Trailer").
		First(&vehicle, "id = ?", id).Error
	if err != nil {
		return nil, translate(err, "vehicle")
	}
	return &vehicle, nil
}

func (r *vehicleRepository) ListByDriver(ctx context.Context, driverID uuid.UUID) ([]domain.Vehicle, error) {
	var vehicles []domain.Vehicle
	err := r.DB(ctx).
		Preload("Photos", func(db *gorm.DB) *gorm.DB { return db.Order("sort_order") }).
		Preload("Trailer").
		Where("driver_id = ?", driverID).
		Order("is_primary DESC, created_at").
		Find(&vehicles).Error
	if err != nil {
		return nil, translate(err, "vehicle")
	}
	return vehicles, nil
}

func (r *vehicleRepository) ListByCompany(ctx context.Context, companyID uuid.UUID, offset, limit int) ([]domain.Vehicle, int64, error) {
	q := r.DB(ctx).Model(&domain.Vehicle{}).Where("company_id = ?", companyID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "vehicle")
	}

	var vehicles []domain.Vehicle
	err := applyPage(q.Preload("Photos").Order("created_at DESC"), offset, limit).Find(&vehicles).Error
	if err != nil {
		return nil, 0, translate(err, "vehicle")
	}
	return vehicles, total, nil
}

func (r *vehicleRepository) ListPending(ctx context.Context, offset, limit int) ([]domain.Vehicle, int64, error) {
	q := r.DB(ctx).Model(&domain.Vehicle{}).Where("status = ?", domain.VerificationPending)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "vehicle")
	}

	var vehicles []domain.Vehicle
	err := applyPage(q.Preload("Photos").Order("created_at"), offset, limit).Find(&vehicles).Error
	if err != nil {
		return nil, 0, translate(err, "vehicle")
	}
	return vehicles, total, nil
}

// SetPrimary moves the primary flag atomically. A partial unique index enforces
// one primary per driver, so clearing must happen in the same transaction.
func (r *vehicleRepository) SetPrimary(ctx context.Context, driverID, vehicleID uuid.UUID) error {
	db := r.DB(ctx)
	err := db.Model(&domain.Vehicle{}).
		Where("driver_id = ? AND is_primary", driverID).
		Update("is_primary", false).Error
	if err != nil {
		return translate(err, "vehicle")
	}
	err = db.Model(&domain.Vehicle{}).
		Where("id = ? AND driver_id = ?", vehicleID, driverID).
		Update("is_primary", true).Error
	return translate(err, "vehicle")
}

func (r *vehicleRepository) Review(ctx context.Context, id uuid.UUID, status domain.VerificationStatus, reason *string) error {
	err := r.DB(ctx).Model(&domain.Vehicle{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status, "rejection_reason": reason}).Error
	return translate(err, "vehicle")
}

func (r *vehicleRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return translate(r.DB(ctx).Where("id = ?", id).Delete(&domain.Vehicle{}).Error, "vehicle")
}

func (r *vehicleRepository) AddPhoto(ctx context.Context, photo *domain.VehiclePhoto) error {
	return translate(r.DB(ctx).Create(photo).Error, "vehicle photo")
}

func (r *vehicleRepository) DeletePhoto(ctx context.Context, photoID uuid.UUID) error {
	err := r.DB(ctx).Where("id = ?", photoID).Delete(&domain.VehiclePhoto{}).Error
	return translate(err, "vehicle photo")
}

// UpsertTrailer replaces the trailer of a vehicle; a truck pulls at most one.
func (r *vehicleRepository) UpsertTrailer(ctx context.Context, trailer *domain.Trailer) error {
	db := r.DB(ctx)
	var existing domain.Trailer
	err := db.Where("vehicle_id = ?", trailer.VehicleID).First(&existing).Error
	switch {
	case err == nil:
		trailer.ID = existing.ID
		return translate(db.Model(&domain.Trailer{ID: existing.ID}).Updates(trailer).Error, "trailer")
	case isNotFound(err):
		return translate(db.Clauses(clause.OnConflict{UpdateAll: true}).Create(trailer).Error, "trailer")
	default:
		return translate(err, "trailer")
	}
}

func (r *vehicleRepository) CountApproved(ctx context.Context, driverID uuid.UUID) (int64, error) {
	var count int64
	err := r.DB(ctx).Model(&domain.Vehicle{}).
		Where("driver_id = ? AND status = ?", driverID, domain.VerificationApproved).
		Count(&count).Error
	if err != nil {
		return 0, translate(err, "vehicle")
	}
	return count, nil
}

// ─── driver documents ───────────────────────────────────────────────

type driverDocumentRepository struct{ Base }

// NewDriverDocumentRepository builds the driver-document repository.
func NewDriverDocumentRepository(db *gorm.DB) domain.DriverDocumentRepository {
	return &driverDocumentRepository{Base: NewBase(db)}
}

// Upsert replaces a document of the same (driver, type, side). Re-uploading the
// back of a licence must overwrite the previous scan, not accumulate rows.
func (r *driverDocumentRepository) Upsert(ctx context.Context, doc *domain.DriverDocument) error {
	db := r.DB(ctx)
	var existing domain.DriverDocument
	err := db.Where("driver_id = ? AND type = ? AND side = ?",
		doc.DriverID, doc.Type, doc.Side).First(&existing).Error

	switch {
	case err == nil:
		doc.ID = existing.ID
		// A replaced document returns to the review queue.
		updates := map[string]any{
			"file_id":          doc.FileID,
			"number":           doc.Number,
			"issued_at":        doc.IssuedAt,
			"expires_at":       doc.ExpiresAt,
			"status":           domain.VerificationPending,
			"rejection_reason": nil,
			"reviewed_by":      nil,
			"reviewed_at":      nil,
		}
		return translate(db.Model(&domain.DriverDocument{}).
			Where("id = ?", existing.ID).Updates(updates).Error, "document")
	case isNotFound(err):
		return translate(db.Create(doc).Error, "document")
	default:
		return translate(err, "document")
	}
}

func (r *driverDocumentRepository) ListByDriver(ctx context.Context, driverID uuid.UUID) ([]domain.DriverDocument, error) {
	var docs []domain.DriverDocument
	err := r.DB(ctx).Preload("File").
		Where("driver_id = ?", driverID).
		Order("type, side").
		Find(&docs).Error
	if err != nil {
		return nil, translate(err, "document")
	}
	return docs, nil
}

func (r *driverDocumentRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.DriverDocument, error) {
	var doc domain.DriverDocument
	if err := r.DB(ctx).Preload("File").First(&doc, "id = ?", id).Error; err != nil {
		return nil, translate(err, "document")
	}
	return &doc, nil
}

func (r *driverDocumentRepository) Review(ctx context.Context, id uuid.UUID, status domain.VerificationStatus, reason *string, reviewedBy uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.DriverDocument{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":           status,
			"rejection_reason": reason,
			"reviewed_by":      reviewedBy,
			"reviewed_at":      time.Now().UTC(),
		}).Error
	return translate(err, "document")
}

func (r *driverDocumentRepository) ListPending(ctx context.Context, offset, limit int) ([]domain.DriverDocument, int64, error) {
	q := r.DB(ctx).Model(&domain.DriverDocument{}).Where("status = ?", domain.VerificationPending)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "document")
	}

	var docs []domain.DriverDocument
	// Oldest first: the review queue is a queue.
	err := applyPage(q.Preload("File").Order("created_at"), offset, limit).Find(&docs).Error
	if err != nil {
		return nil, 0, translate(err, "document")
	}
	return docs, total, nil
}

func (r *driverDocumentRepository) CountApprovedByTypes(ctx context.Context, driverID uuid.UUID, types []domain.DocumentType) (int64, error) {
	if len(types) == 0 {
		return 0, nil
	}
	var count int64
	err := r.DB(ctx).Model(&domain.DriverDocument{}).
		Distinct("type").
		Where("driver_id = ? AND status = ? AND type IN ?",
			driverID, domain.VerificationApproved, types).
		Count(&count).Error
	if err != nil {
		return 0, translate(err, "document")
	}
	return count, nil
}

func (r *driverDocumentRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Where("id = ?", id).Delete(&domain.DriverDocument{}).Error
	return translate(err, "document")
}
