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

type bookingRepository struct{ Base }

// NewBookingRepository builds the booking repository.
func NewBookingRepository(db *gorm.DB) domain.BookingRepository {
	return &bookingRepository{Base: NewBase(db)}
}

func (r *bookingRepository) Create(ctx context.Context, booking *domain.Booking) error {
	err := r.DB(ctx).Create(booking).Error
	if err != nil && isUniqueViolation(err) {
		// The partial unique index on (load_id, driver_id) for live statuses is
		// what actually prevents a double reservation under concurrency.
		return apperror.ErrBookingExists.Wrap(err)
	}
	return translate(err, "booking")
}

func (r *bookingRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Booking, error) {
	var booking domain.Booking
	if err := r.DB(ctx).First(&booking, "id = ?", id).Error; err != nil {
		return nil, translate(err, "booking")
	}
	return &booking, nil
}

func (r *bookingRepository) FindWithRelations(ctx context.Context, id uuid.UUID) (*domain.Booking, error) {
	var booking domain.Booking
	err := r.DB(ctx).
		Preload("Load").
		Preload("Load.PickupAddress").
		Preload("Load.DeliveryAddress").
		Preload("Load.CargoType").
		Preload("Driver").
		Preload("Driver.User").
		Preload("Vehicle").
		Preload("Company").
		Preload("Company.Logo").
		Preload("Trip").
		First(&booking, "id = ?", id).Error
	if err != nil {
		return nil, translate(err, "booking")
	}
	return &booking, nil
}

func (r *bookingRepository) List(ctx context.Context, f domain.BookingFilter, offset, limit int, order string) ([]domain.Booking, int64, error) {
	q := r.DB(ctx).Model(&domain.Booking{})

	if f.LoadID != nil {
		q = q.Where("load_id = ?", *f.LoadID)
	}
	if f.DriverID != nil {
		q = q.Where("driver_id = ?", *f.DriverID)
	}
	if f.CompanyID != nil {
		q = q.Where("company_id = ?", *f.CompanyID)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("status IN ?", f.Statuses)
	}
	if f.Initiator != nil {
		q = q.Where("initiated_by = ?", *f.Initiator)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "booking")
	}

	if order == "" {
		order = "created_at DESC"
	}
	var bookings []domain.Booking
	err := applyPage(q.
		Preload("Load").
		Preload("Load.PickupAddress").
		Preload("Load.DeliveryAddress").
		Preload("Driver").Preload("Driver.User").
		Preload("Vehicle").
		Preload("Company").
		Preload("Trip").
		Order(order), offset, limit).Find(&bookings).Error
	if err != nil {
		return nil, 0, translate(err, "booking")
	}
	return bookings, total, nil
}

// TransitionStatus applies a guarded status change to a booking.
func (r *bookingRepository) TransitionStatus(ctx context.Context, id uuid.UUID, from []domain.BookingStatus, to domain.BookingStatus, columns map[string]any) error {
	updates := make(map[string]any, len(columns)+1)
	maps.Copy(updates, columns)
	updates["status"] = to

	q := r.DB(ctx).Model(&domain.Booking{}).Where("id = ?", id)
	if len(from) > 0 {
		q = q.Where("status IN ?", from)
	}

	res := q.Updates(updates)
	if res.Error != nil {
		return translate(res.Error, "booking")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrBookingNotPending.WithMessage(
			"This booking has already been answered")
	}
	return nil
}

func (r *bookingRepository) FindLiveForPair(ctx context.Context, loadID, driverID uuid.UUID) (*domain.Booking, error) {
	var booking domain.Booking
	err := r.DB(ctx).
		Where("load_id = ? AND driver_id = ? AND status IN ?",
			loadID, driverID, domain.LiveBookingStatuses).
		First(&booking).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "booking")
	}
	return &booking, nil
}

func (r *bookingRepository) CountByDriver(ctx context.Context, driverID uuid.UUID, statuses []domain.BookingStatus) (int64, error) {
	q := r.DB(ctx).Model(&domain.Booking{}).Where("driver_id = ?", driverID)
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, translate(err, "booking")
	}
	return count, nil
}

// RejectOthers closes the remaining open bookings on a load once one is accepted.
// Without this, drivers would keep waiting for an answer that will never come.
func (r *bookingRepository) RejectOthers(ctx context.Context, loadID, keepBookingID uuid.UUID, reason string) (int64, error) {
	res := r.DB(ctx).Model(&domain.Booking{}).
		Where("load_id = ? AND id <> ? AND status IN ?",
			loadID, keepBookingID, []domain.BookingStatus{domain.BookingPending, domain.BookingOffered}).
		Updates(map[string]any{
			"status":           domain.BookingRejected,
			"response_message": reason,
			"responded_at":     time.Now().UTC(),
		})
	if res.Error != nil {
		return 0, translate(res.Error, "booking")
	}
	return res.RowsAffected, nil
}

// ExpireOverdue closes bookings nobody answered and returns them so the caller
// can notify both sides.
func (r *bookingRepository) ExpireOverdue(ctx context.Context, now time.Time) ([]domain.Booking, error) {
	var expired []domain.Booking
	err := r.DB(ctx).Raw(`
		UPDATE bookings SET status = 'expired', updated_at = now()
		WHERE status IN ('pending', 'offered')
		  AND expires_at IS NOT NULL
		  AND expires_at < ?
		  AND deleted_at IS NULL
		RETURNING *`, now).Scan(&expired).Error
	if err != nil {
		return nil, translate(err, "booking")
	}
	return expired, nil
}

// AverageResponseSeconds measures how fast a driver answers offers. It feeds the
// driver score, so companies can prefer responsive drivers.
func (r *bookingRepository) AverageResponseSeconds(ctx context.Context, driverID uuid.UUID) (int, error) {
	var avg *float64
	err := r.DB(ctx).Raw(`
		SELECT AVG(EXTRACT(EPOCH FROM (responded_at - created_at)))
		FROM bookings
		WHERE driver_id = ?
		  AND initiated_by = 'company'
		  AND responded_at IS NOT NULL
		  AND created_at > now() - interval '90 days'`, driverID).Scan(&avg).Error
	if err != nil {
		return 0, translate(err, "booking")
	}
	if avg == nil {
		return 0, nil
	}
	return int(*avg), nil
}
