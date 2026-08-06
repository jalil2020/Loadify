package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/geo"
)

type driverRepository struct{ Base }

// NewDriverRepository builds the driver repository.
func NewDriverRepository(db *gorm.DB) domain.DriverRepository {
	return &driverRepository{Base: NewBase(db)}
}

func (r *driverRepository) Create(ctx context.Context, driver *domain.Driver) error {
	return translate(r.DB(ctx).Create(driver).Error, "driver")
}

func (r *driverRepository) Update(ctx context.Context, driver *domain.Driver) error {
	err := r.DB(ctx).Model(&domain.Driver{ID: driver.ID}).Updates(driver).Error
	return translate(err, "driver")
}

func (r *driverRepository) UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	err := r.DB(ctx).Model(&domain.Driver{}).Where("id = ?", id).Updates(columns).Error
	return translate(err, "driver")
}

func (r *driverRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Driver, error) {
	var driver domain.Driver
	if err := r.DB(ctx).First(&driver, "id = ?", id).Error; err != nil {
		return nil, translate(err, "driver")
	}
	return &driver, nil
}

func (r *driverRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*domain.Driver, error) {
	var driver domain.Driver
	if err := r.DB(ctx).First(&driver, "user_id = ?", userID).Error; err != nil {
		return nil, translate(err, "driver")
	}
	return &driver, nil
}

func (r *driverRepository) FindWithRelations(ctx context.Context, id uuid.UUID) (*domain.Driver, error) {
	var driver domain.Driver
	err := r.DB(ctx).
		Preload("User").
		Preload("HomeCity").
		Preload("Vehicles", func(db *gorm.DB) *gorm.DB {
			return db.Order("is_primary DESC, created_at").Preload("Photos").Preload("Trailer")
		}).
		Preload("Docs").
		First(&driver, "id = ?", id).Error
	if err != nil {
		return nil, translate(err, "driver")
	}
	return &driver, nil
}

// Search finds drivers, optionally ranked by distance from a point.
//
// Radius filtering uses ST_DWithin on the geography column so the GIST index is
// used; the distance is computed only for rows that survive the filter.
func (r *driverRepository) Search(ctx context.Context, filter domain.DriverSearchFilter, offset, limit int, order string) ([]domain.DriverSearchResult, int64, error) {
	q := r.DB(ctx).Model(&domain.Driver{}).
		Joins("JOIN users u ON u.id = drivers.user_id AND u.deleted_at IS NULL")

	if len(filter.Verification) > 0 {
		q = q.Where("drivers.verification_status IN ?", filter.Verification)
	} else {
		q = q.Where("drivers.verification_status = ?", domain.VerificationVerified)
	}
	if len(filter.Statuses) > 0 {
		q = q.Where("drivers.status IN ?", filter.Statuses)
	}
	if filter.CompanyID != nil {
		q = q.Where("drivers.company_id = ?", *filter.CompanyID)
	}
	if filter.MinRating > 0 {
		q = q.Where("drivers.rating >= ?", filter.MinRating)
	}
	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		q = q.Where("u.phone ILIKE ? OR (u.first_name || ' ' || u.last_name) ILIKE ?", like, like)
	}
	if len(filter.RegionIDs) > 0 {
		q = q.Where(`EXISTS (
			SELECT 1 FROM driver_availability da
			WHERE da.driver_id = drivers.id AND da.is_active
			  AND da.preferred_region_ids && ?::uuid[])`, uuidArrayLiteral(filter.RegionIDs))
	}
	if filter.AvailableFrom != nil {
		q = q.Where(`EXISTS (
			SELECT 1 FROM driver_availability da
			WHERE da.driver_id = drivers.id AND da.is_active
			  AND da.available_from <= ?
			  AND (da.available_to IS NULL OR da.available_to >= ?))`,
			*filter.AvailableFrom, *filter.AvailableFrom)
	}

	// Capability filters resolve against the driver's approved vehicles.
	if needsVehicleFilter(filter) {
		sub := r.DB(ctx).Table("vehicles v").
			Select("1").
			Where("v.driver_id = drivers.id AND v.status = ? AND v.deleted_at IS NULL", domain.VerificationApproved)
		if len(filter.VehicleTypes) > 0 {
			sub = sub.Where("v.type IN ?", filter.VehicleTypes)
		}
		if filter.MinCapacityKg > 0 {
			sub = sub.Where("v.capacity_kg >= ?", filter.MinCapacityKg)
		}
		if filter.MinVolumeM3 > 0 {
			sub = sub.Where("v.volume_m3 >= ?", filter.MinVolumeM3)
		}
		if filter.RequiresADR {
			sub = sub.Where("v.has_adr")
		}
		if filter.RequiresRefrig {
			sub = sub.Where("v.has_refrigeration")
		}
		q = q.Where("EXISTS (?)", sub)
	}

	if filter.Near != nil && filter.Near.Valid() && filter.RadiusKm > 0 {
		q = q.Where("drivers.current_location IS NOT NULL").
			Where("ST_DWithin(drivers.current_location, ?::geography, ?)",
				filter.Near, filter.RadiusKm*1000)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "driver")
	}

	type row struct {
		domain.Driver
		DistanceM *float64 `gorm:"column:distance_m"`
	}

	selection := "drivers.*"
	effectiveOrder := order
	if filter.Near != nil && filter.Near.Valid() {
		selection += ", ST_Distance(drivers.current_location, ?::geography) AS distance_m"
		q = q.Select(selection, filter.Near)
		if effectiveOrder == "" {
			effectiveOrder = "distance_m ASC"
		}
	} else {
		q = q.Select(selection)
		if effectiveOrder == "" {
			effectiveOrder = "drivers.rating DESC, drivers.completed_trips DESC"
		}
	}

	var rows []row
	if err := applyPage(q.Order(effectiveOrder), offset, limit).Scan(&rows).Error; err != nil {
		return nil, 0, translate(err, "driver")
	}

	results := make([]domain.DriverSearchResult, 0, len(rows))
	for _, item := range rows {
		res := domain.DriverSearchResult{Driver: item.Driver}
		if item.DistanceM != nil {
			km := *item.DistanceM / 1000
			res.DistanceKm = &km
		}
		results = append(results, res)
	}
	return results, total, nil
}

func needsVehicleFilter(f domain.DriverSearchFilter) bool {
	return len(f.VehicleTypes) > 0 || f.MinCapacityKg > 0 || f.MinVolumeM3 > 0 ||
		f.RequiresADR || f.RequiresRefrig
}

func (r *driverRepository) UpdateLocation(ctx context.Context, driverID uuid.UUID, point geo.Point, recordedAt time.Time) error {
	// Guard on the timestamp so an out-of-order packet from a reconnecting
	// device cannot move the marker backwards.
	err := r.DB(ctx).Model(&domain.Driver{}).
		Where("id = ?", driverID).
		Where("location_updated_at IS NULL OR location_updated_at <= ?", recordedAt).
		Updates(map[string]any{
			"current_location":    point,
			"location_updated_at": recordedAt,
		}).Error
	return translate(err, "driver")
}

func (r *driverRepository) SetStatus(ctx context.Context, driverID uuid.UUID, status domain.DriverStatus) error {
	columns := map[string]any{"status": status}
	if status == domain.DriverStatusOnline {
		columns["last_online_at"] = time.Now().UTC()
	}
	err := r.DB(ctx).Model(&domain.Driver{}).Where("id = ?", driverID).Updates(columns).Error
	return translate(err, "driver")
}

func (r *driverRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return translate(r.DB(ctx).Where("id = ?", id).Delete(&domain.Driver{}).Error, "driver")
}

// RecalculateRating recomputes the cached aggregate from the ratings table.
// Keeping it as one statement means concurrent trip completions cannot
// interleave a read-modify-write and lose a rating.
func (r *driverRepository) RecalculateRating(ctx context.Context, driverID uuid.UUID) error {
	err := r.DB(ctx).Exec(`
		UPDATE drivers SET
			rating = COALESCE(agg.avg_score, 0),
			rating_count = COALESCE(agg.total, 0)
		FROM (
			SELECT AVG(score)::numeric(3,2) AS avg_score, COUNT(*) AS total
			FROM ratings
			WHERE driver_id = ? AND ratee_type = 'driver' AND NOT is_hidden
		) agg
		WHERE drivers.id = ?`, driverID, driverID).Error
	return translate(err, "driver")
}

// uuidArrayLiteral renders a slice as a PostgreSQL uuid[] literal for the
// overlap operator, which has no parameterised slice form in GORM.
func uuidArrayLiteral(ids []uuid.UUID) string {
	if len(ids) == 0 {
		return "{}"
	}
	out := make([]byte, 0, len(ids)*38+2)
	out = append(out, '{')
	for i, id := range ids {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, id.String()...)
	}
	return string(append(out, '}'))
}
