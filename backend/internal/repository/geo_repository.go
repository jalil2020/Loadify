package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/geo"
)

type geoRepository struct{ Base }

// NewGeoRepository builds the geography repository.
func NewGeoRepository(db *gorm.DB) domain.GeoRepository {
	return &geoRepository{Base: NewBase(db)}
}

func (r *geoRepository) ListCountries(ctx context.Context) ([]domain.Country, error) {
	var countries []domain.Country
	err := r.DB(ctx).Where("is_active").Order("sort_order, name_en").Find(&countries).Error
	if err != nil {
		return nil, translate(err, "country")
	}
	return countries, nil
}

func (r *geoRepository) ListRegions(ctx context.Context, countryCode string) ([]domain.Region, error) {
	q := r.DB(ctx).Model(&domain.Region{}).Where("regions.is_active")
	if countryCode != "" {
		q = q.Joins("JOIN countries c ON c.id = regions.country_id").
			Where("c.code = ?", countryCode)
	}
	var regions []domain.Region
	if err := q.Order("regions.sort_order, regions.name_en").Find(&regions).Error; err != nil {
		return nil, translate(err, "region")
	}
	return regions, nil
}

func (r *geoRepository) ListCities(ctx context.Context, regionID *uuid.UUID, search string, limit int) ([]domain.City, error) {
	q := r.DB(ctx).Model(&domain.City{}).Where("is_active")
	if regionID != nil {
		q = q.Where("region_id = ?", *regionID)
	}
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("name_uz ILIKE ? OR name_ru ILIKE ? OR name_en ILIKE ?", like, like, like)
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}

	var cities []domain.City
	err := q.Order("is_capital DESC, name_en").Limit(limit).Find(&cities).Error
	if err != nil {
		return nil, translate(err, "city")
	}
	return cities, nil
}

func (r *geoRepository) FindRegionByID(ctx context.Context, id uuid.UUID) (*domain.Region, error) {
	var region domain.Region
	if err := r.DB(ctx).First(&region, "id = ?", id).Error; err != nil {
		return nil, translate(err, "region")
	}
	return &region, nil
}

func (r *geoRepository) FindCityByID(ctx context.Context, id uuid.UUID) (*domain.City, error) {
	var city domain.City
	if err := r.DB(ctx).Preload("Region").First(&city, "id = ?", id).Error; err != nil {
		return nil, translate(err, "city")
	}
	return &city, nil
}

// NearestRegion resolves a map pin to a region by proximity to region centres.
// Region polygons would be more accurate, but centre distance is correct for
// every Uzbek region pair at the scale that matters and costs one index scan.
func (r *geoRepository) NearestRegion(ctx context.Context, point geo.Point) (*domain.Region, error) {
	if !point.Valid() {
		return nil, nil
	}
	var region domain.Region
	err := r.DB(ctx).Model(&domain.Region{}).
		Where("is_active AND center IS NOT NULL").
		Order(gorm.Expr("center <-> ?::geography", point)).
		First(&region).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "region")
	}
	return &region, nil
}

// CreateAddress stores a resolved address, filling in the region from the
// coordinates when the client did not supply one.
func (r *geoRepository) CreateAddress(ctx context.Context, address *domain.Address) error {
	if address.RegionID == nil {
		if region, err := r.NearestRegion(ctx, address.Location); err == nil && region != nil {
			address.RegionID = &region.ID
			if address.CountryID == nil {
				address.CountryID = &region.CountryID
			}
		}
	}
	if address.CreatedAt.IsZero() {
		address.CreatedAt = time.Now().UTC()
	}
	return translate(r.DB(ctx).Create(address).Error, "address")
}

func (r *geoRepository) FindAddressByID(ctx context.Context, id uuid.UUID) (*domain.Address, error) {
	var address domain.Address
	err := r.DB(ctx).Preload("Region").Preload("City").First(&address, "id = ?", id).Error
	if err != nil {
		return nil, translate(err, "address")
	}
	return &address, nil
}

// ─── files ──────────────────────────────────────────────────────────

type fileRepository struct{ Base }

// NewFileRepository builds the file-metadata repository.
func NewFileRepository(db *gorm.DB) domain.FileRepository {
	return &fileRepository{Base: NewBase(db)}
}

func (r *fileRepository) Create(ctx context.Context, file *domain.File) error {
	return translate(r.DB(ctx).Create(file).Error, "file")
}

func (r *fileRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.File, error) {
	var file domain.File
	if err := r.DB(ctx).First(&file, "id = ?", id).Error; err != nil {
		return nil, translate(err, "file")
	}
	return &file, nil
}

func (r *fileRepository) FindByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.File, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var files []domain.File
	if err := r.DB(ctx).Where("id IN ?", ids).Find(&files).Error; err != nil {
		return nil, translate(err, "file")
	}
	return files, nil
}

func (r *fileRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return translate(r.DB(ctx).Where("id = ?", id).Delete(&domain.File{}).Error, "file")
}

// ListOrphans finds uploads that were never attached to anything. Clients can
// abandon an upload halfway through a form; without this sweep those objects
// would sit in the bucket forever.
func (r *fileRepository) ListOrphans(ctx context.Context, olderThan time.Time, limit int) ([]domain.File, error) {
	var files []domain.File
	err := r.DB(ctx).Raw(`
		SELECT f.* FROM files f
		WHERE f.created_at < ?
		  AND f.deleted_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM users u WHERE u.avatar_file_id = f.id)
		  AND NOT EXISTS (SELECT 1 FROM companies c WHERE c.logo_file_id = f.id)
		  AND NOT EXISTS (SELECT 1 FROM company_documents cd WHERE cd.file_id = f.id)
		  AND NOT EXISTS (SELECT 1 FROM driver_documents dd WHERE dd.file_id = f.id)
		  AND NOT EXISTS (SELECT 1 FROM vehicle_photos vp WHERE vp.file_id = f.id)
		  AND NOT EXISTS (SELECT 1 FROM load_photos lp WHERE lp.file_id = f.id)
		  AND NOT EXISTS (SELECT 1 FROM load_documents ld WHERE ld.file_id = f.id)
		  AND NOT EXISTS (SELECT 1 FROM trip_documents td WHERE td.file_id = f.id)
		  AND NOT EXISTS (SELECT 1 FROM chat_messages cm WHERE cm.file_id = f.id)
		  AND NOT EXISTS (SELECT 1 FROM invoices i WHERE i.file_id = f.id)
		  AND NOT EXISTS (SELECT 1 FROM banners b WHERE b.image_file_id = f.id)
		ORDER BY f.created_at
		LIMIT ?`, olderThan, limit).Scan(&files).Error
	if err != nil {
		return nil, translate(err, "file")
	}
	return files, nil
}

// ─── driver preferences & availability ──────────────────────────────

type driverPreferenceRepository struct{ Base }

// NewDriverPreferenceRepository builds the preferences repository.
func NewDriverPreferenceRepository(db *gorm.DB) domain.DriverPreferenceRepository {
	return &driverPreferenceRepository{Base: NewBase(db)}
}

func (r *driverPreferenceRepository) GetPreferences(ctx context.Context, driverID uuid.UUID) (*domain.DriverPreferences, error) {
	var prefs domain.DriverPreferences
	err := r.DB(ctx).First(&prefs, "driver_id = ?", driverID).Error
	if err != nil {
		if isNotFound(err) {
			// Defaults are the contract: a driver who never opened the settings
			// screen still gets sensible matching behaviour.
			return &domain.DriverPreferences{
				DriverID:       driverID,
				MaxDistanceKm:  500,
				NotifyNewLoads: true,
				NotifyRadiusKm: 100,
			}, nil
		}
		return nil, translate(err, "driver preferences")
	}
	return &prefs, nil
}

func (r *driverPreferenceRepository) SavePreferences(ctx context.Context, prefs *domain.DriverPreferences) error {
	db := r.DB(ctx)
	var existing domain.DriverPreferences
	err := db.Where("driver_id = ?", prefs.DriverID).First(&existing).Error
	switch {
	case err == nil:
		prefs.ID = existing.ID
		return translate(db.Model(&domain.DriverPreferences{ID: existing.ID}).
			Select("max_distance_km", "preferred_cargo_types", "preferred_region_ids",
				"min_price_amount", "notify_new_loads", "notify_radius_km", "auto_accept_offers").
			Updates(prefs).Error, "driver preferences")
	case isNotFound(err):
		return translate(db.Create(prefs).Error, "driver preferences")
	default:
		return translate(err, "driver preferences")
	}
}

func (r *driverPreferenceRepository) CreateAvailability(ctx context.Context, availability *domain.DriverAvailability) error {
	return translate(r.DB(ctx).Create(availability).Error, "availability")
}

func (r *driverPreferenceRepository) ListAvailability(ctx context.Context, driverID uuid.UUID, activeOnly bool) ([]domain.DriverAvailability, error) {
	q := r.DB(ctx).Preload("Vehicle").Preload("FromAddress").Where("driver_id = ?", driverID)
	if activeOnly {
		q = q.Where("is_active AND (available_to IS NULL OR available_to >= now())")
	}
	var items []domain.DriverAvailability
	if err := q.Order("available_from DESC").Find(&items).Error; err != nil {
		return nil, translate(err, "availability")
	}
	return items, nil
}

func (r *driverPreferenceRepository) DeactivateAvailability(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.DriverAvailability{}).
		Where("id = ?", id).Update("is_active", false).Error
	return translate(err, "availability")
}

func (r *driverPreferenceRepository) FindAvailabilityByID(ctx context.Context, id uuid.UUID) (*domain.DriverAvailability, error) {
	var item domain.DriverAvailability
	if err := r.DB(ctx).First(&item, "id = ?", id).Error; err != nil {
		return nil, translate(err, "availability")
	}
	return &item, nil
}

// ─── location traces ────────────────────────────────────────────────

type locationRepository struct{ Base }

// NewLocationRepository builds the GPS-trace repository.
func NewLocationRepository(db *gorm.DB) domain.LocationRepository {
	return &locationRepository{Base: NewBase(db)}
}

// InsertBatch writes a whole buffer in one multi-row INSERT.
func (r *locationRepository) InsertBatch(ctx context.Context, points []domain.DriverLocation) error {
	if len(points) == 0 {
		return nil
	}
	err := r.DB(ctx).CreateInBatches(points, 500).Error
	return translate(err, "location")
}

func (r *locationRepository) ListByTrip(ctx context.Context, tripID uuid.UUID, limit int) ([]domain.DriverLocation, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	var points []domain.DriverLocation
	err := r.DB(ctx).
		Where("trip_id = ?", tripID).
		Order("recorded_at").
		Limit(limit).
		Find(&points).Error
	if err != nil {
		return nil, translate(err, "location")
	}
	return points, nil
}

func (r *locationRepository) LatestByDriver(ctx context.Context, driverID uuid.UUID) (*domain.DriverLocation, error) {
	var point domain.DriverLocation
	err := r.DB(ctx).
		Where("driver_id = ?", driverID).
		Order("recorded_at DESC").
		First(&point).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "location")
	}
	return &point, nil
}

func (r *locationRepository) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	res := r.DB(ctx).Where("recorded_at < ?", before).Delete(&domain.DriverLocation{})
	if res.Error != nil {
		return 0, translate(res.Error, "location")
	}
	return res.RowsAffected, nil
}
