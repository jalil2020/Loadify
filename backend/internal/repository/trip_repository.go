package repository

import (
	"context"
	"maps"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
)

type tripRepository struct{ Base }

// NewTripRepository builds the trip repository.
func NewTripRepository(db *gorm.DB) domain.TripRepository {
	return &tripRepository{Base: NewBase(db)}
}

func (r *tripRepository) Create(ctx context.Context, trip *domain.Trip) error {
	return translate(r.DB(ctx).Create(trip).Error, "trip")
}

func (r *tripRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Trip, error) {
	var trip domain.Trip
	if err := r.DB(ctx).First(&trip, "id = ?", id).Error; err != nil {
		return nil, translate(err, "trip")
	}
	return &trip, nil
}

func (r *tripRepository) FindWithRelations(ctx context.Context, id uuid.UUID) (*domain.Trip, error) {
	var trip domain.Trip
	err := r.DB(ctx).
		Preload("Load").
		Preload("Load.CargoType").
		Preload("Load.PickupAddress").Preload("Load.PickupAddress.Region").
		Preload("Load.DeliveryAddress").Preload("Load.DeliveryAddress.Region").
		Preload("Driver").Preload("Driver.User").
		Preload("Vehicle").
		Preload("Company").Preload("Company.Logo").
		Preload("Events", func(db *gorm.DB) *gorm.DB { return db.Order("created_at DESC") }).
		Preload("Documents", func(db *gorm.DB) *gorm.DB { return db.Preload("File") }).
		First(&trip, "id = ?", id).Error
	if err != nil {
		return nil, translate(err, "trip")
	}
	return &trip, nil
}

func (r *tripRepository) FindByBooking(ctx context.Context, bookingID uuid.UUID) (*domain.Trip, error) {
	var trip domain.Trip
	if err := r.DB(ctx).First(&trip, "booking_id = ?", bookingID).Error; err != nil {
		return nil, translate(err, "trip")
	}
	return &trip, nil
}

func (r *tripRepository) List(ctx context.Context, f domain.TripFilter, offset, limit int, order string) ([]domain.Trip, int64, error) {
	q := r.DB(ctx).Model(&domain.Trip{})

	if f.DriverID != nil {
		q = q.Where("driver_id = ?", *f.DriverID)
	}
	if f.CompanyID != nil {
		q = q.Where("company_id = ?", *f.CompanyID)
	}
	if f.LoadID != nil {
		q = q.Where("load_id = ?", *f.LoadID)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("status IN ?", f.Statuses)
	}
	if f.ActiveOnly {
		q = q.Where("status IN ?", []domain.TripStatus{
			domain.TripCreated, domain.TripEnRoute, domain.TripAtPickup,
			domain.TripLoaded, domain.TripInTransit, domain.TripAtDelivery,
		})
	}
	if f.Range.From != nil {
		q = q.Where("created_at >= ?", *f.Range.From)
	}
	if f.Range.To != nil {
		q = q.Where("created_at <= ?", *f.Range.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "trip")
	}

	if order == "" {
		order = "created_at DESC"
	}
	var trips []domain.Trip
	err := applyPage(q.
		Preload("Load").
		Preload("Load.PickupAddress").
		Preload("Load.DeliveryAddress").
		Preload("Driver").Preload("Driver.User").
		Preload("Vehicle").
		Preload("Company").
		Order(order), offset, limit).Find(&trips).Error
	if err != nil {
		return nil, 0, translate(err, "trip")
	}
	return trips, total, nil
}

// TransitionStatus applies a guarded trip status change and stamps the matching
// timestamp column in the same statement.
func (r *tripRepository) TransitionStatus(ctx context.Context, id uuid.UUID, from []domain.TripStatus, to domain.TripStatus, columns map[string]any) error {
	updates := make(map[string]any, len(columns)+2)
	maps.Copy(updates, columns)
	updates["status"] = to
	if col := domain.TimestampColumnFor(to); col != "" {
		if _, given := updates[col]; !given {
			updates[col] = time.Now().UTC()
		}
	}

	q := r.DB(ctx).Model(&domain.Trip{}).Where("id = ?", id)
	if len(from) > 0 {
		q = q.Where("status IN ?", from)
	}

	res := q.Updates(updates)
	if res.Error != nil {
		return translate(res.Error, "trip")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrInvalidStatusChange.WithMessage(
			"This trip has already moved on, refresh and try again")
	}
	return nil
}

func (r *tripRepository) UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	err := r.DB(ctx).Model(&domain.Trip{}).Where("id = ?", id).Updates(columns).Error
	return translate(err, "trip")
}

// HasOverlapping reports whether the driver already has a trip whose pickup
// window overlaps the given one.
//
// A truck cannot be in two places at once, so this is the guard that stops a
// driver from reserving two loads for the same day. An open-ended window (no
// delivery deadline) is treated as blocking everything from its start.
func (r *tripRepository) HasOverlapping(ctx context.Context, driverID uuid.UUID, from time.Time, to *time.Time, excludeTripID *uuid.UUID) (bool, error) {
	q := r.DB(ctx).Model(&domain.Trip{}).
		Joins("JOIN loads l ON l.id = trips.load_id").
		Where("trips.driver_id = ?", driverID).
		Where("trips.status IN ?", []domain.TripStatus{
			domain.TripCreated, domain.TripEnRoute, domain.TripAtPickup,
			domain.TripLoaded, domain.TripInTransit, domain.TripAtDelivery,
		})
	if excludeTripID != nil {
		q = q.Where("trips.id <> ?", *excludeTripID)
	}

	// Overlap test: existing.start <= new.end AND existing.end >= new.start,
	// with COALESCE covering loads that state no delivery deadline.
	if to != nil {
		q = q.Where("l.pickup_from <= ?", *to)
	}
	q = q.Where("COALESCE(l.delivery_to, l.delivery_from, l.pickup_to, l.pickup_from) >= ?", from)

	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, translate(err, "trip")
	}
	return count > 0, nil
}

// ActiveByDriver returns the trip a driver is currently working, if any.
func (r *tripRepository) ActiveByDriver(ctx context.Context, driverID uuid.UUID) (*domain.Trip, error) {
	var trip domain.Trip
	err := r.DB(ctx).
		Where("driver_id = ? AND status IN ?", driverID, []domain.TripStatus{
			domain.TripEnRoute, domain.TripAtPickup, domain.TripLoaded,
			domain.TripInTransit, domain.TripAtDelivery,
		}).
		Order("created_at DESC").
		First(&trip).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "trip")
	}
	return &trip, nil
}

// ListAwaitingConfirmation finds delivered trips the company never confirmed,
// which the auto-confirm job closes so a driver is not left unpaid indefinitely.
func (r *tripRepository) ListAwaitingConfirmation(ctx context.Context, deliveredBefore time.Time, limit int) ([]domain.Trip, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	var trips []domain.Trip
	err := r.DB(ctx).
		Where("status = ? AND delivered_at IS NOT NULL AND delivered_at < ?",
			domain.TripDelivered, deliveredBefore).
		Order("delivered_at").
		Limit(limit).
		Find(&trips).Error
	if err != nil {
		return nil, translate(err, "trip")
	}
	return trips, nil
}

func (r *tripRepository) AddEvent(ctx context.Context, event *domain.TripEvent) error {
	if event.Payload == nil {
		event.Payload = domain.JSONMap{}
	}
	return translate(r.DB(ctx).Create(event).Error, "trip event")
}

func (r *tripRepository) ListEvents(ctx context.Context, tripID uuid.UUID) ([]domain.TripEvent, error) {
	var events []domain.TripEvent
	err := r.DB(ctx).Where("trip_id = ?", tripID).
		Order("created_at DESC").Find(&events).Error
	if err != nil {
		return nil, translate(err, "trip event")
	}
	return events, nil
}

func (r *tripRepository) AddDocument(ctx context.Context, doc *domain.TripDocument) error {
	return translate(r.DB(ctx).Create(doc).Error, "trip document")
}

func (r *tripRepository) ListDocuments(ctx context.Context, tripID uuid.UUID) ([]domain.TripDocument, error) {
	var docs []domain.TripDocument
	err := r.DB(ctx).Preload("File").Where("trip_id = ?", tripID).
		Order("created_at").Find(&docs).Error
	if err != nil {
		return nil, translate(err, "trip document")
	}
	return docs, nil
}

func (r *tripRepository) DeleteDocument(ctx context.Context, docID uuid.UUID) error {
	err := r.DB(ctx).Where("id = ?", docID).Delete(&domain.TripDocument{}).Error
	return translate(err, "trip document")
}

func (r *tripRepository) CountCompletedByDriver(ctx context.Context, driverID uuid.UUID) (int64, error) {
	var count int64
	err := r.DB(ctx).Model(&domain.Trip{}).
		Where("driver_id = ? AND status = ?", driverID, domain.TripCompleted).
		Count(&count).Error
	if err != nil {
		return 0, translate(err, "trip")
	}
	return count, nil
}

// ─── ratings ────────────────────────────────────────────────────────

type ratingRepository struct{ Base }

// NewRatingRepository builds the rating repository.
func NewRatingRepository(db *gorm.DB) domain.RatingRepository {
	return &ratingRepository{Base: NewBase(db)}
}

func (r *ratingRepository) Create(ctx context.Context, rating *domain.Rating) error {
	if rating.Criteria == nil {
		rating.Criteria = domain.JSONMap{}
	}
	err := r.DB(ctx).Create(rating).Error
	if err != nil && isUniqueViolation(err) {
		return apperror.ErrConflict.WithMessage("You have already rated this trip").Wrap(err)
	}
	return translate(err, "rating")
}

func (r *ratingRepository) FindByTripAndRater(ctx context.Context, tripID, raterUserID uuid.UUID) (*domain.Rating, error) {
	var rating domain.Rating
	err := r.DB(ctx).First(&rating, "trip_id = ? AND rater_user_id = ?", tripID, raterUserID).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "rating")
	}
	return &rating, nil
}

func (r *ratingRepository) ListForDriver(ctx context.Context, driverID uuid.UUID, offset, limit int) ([]domain.Rating, int64, error) {
	return r.list(ctx, "driver_id = ? AND ratee_type = 'driver' AND NOT is_hidden", driverID, offset, limit)
}

func (r *ratingRepository) ListForCompany(ctx context.Context, companyID uuid.UUID, offset, limit int) ([]domain.Rating, int64, error) {
	return r.list(ctx, "company_id = ? AND ratee_type = 'company' AND NOT is_hidden", companyID, offset, limit)
}

func (r *ratingRepository) list(ctx context.Context, where string, id uuid.UUID, offset, limit int) ([]domain.Rating, int64, error) {
	q := r.DB(ctx).Model(&domain.Rating{}).Where(where, id)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "rating")
	}

	var ratings []domain.Rating
	err := applyPage(q.Order("created_at DESC"), offset, limit).Find(&ratings).Error
	if err != nil {
		return nil, 0, translate(err, "rating")
	}
	return ratings, total, nil
}

func (r *ratingRepository) Hide(ctx context.Context, id uuid.UUID, hidden bool) error {
	err := r.DB(ctx).Model(&domain.Rating{}).Where("id = ?", id).
		Update("is_hidden", hidden).Error
	return translate(err, "rating")
}
