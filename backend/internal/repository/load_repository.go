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

type loadRepository struct{ Base }

// NewLoadRepository builds the load repository.
func NewLoadRepository(db *gorm.DB) domain.LoadRepository {
	return &loadRepository{Base: NewBase(db)}
}

func (r *loadRepository) Create(ctx context.Context, load *domain.Load) error {
	return translate(r.DB(ctx).Create(load).Error, "load")
}

func (r *loadRepository) Update(ctx context.Context, load *domain.Load) error {
	err := r.DB(ctx).Model(&domain.Load{ID: load.ID}).Updates(load).Error
	return translate(err, "load")
}

func (r *loadRepository) UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	err := r.DB(ctx).Model(&domain.Load{}).Where("id = ?", id).Updates(columns).Error
	return translate(err, "load")
}

func (r *loadRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Load, error) {
	var load domain.Load
	if err := r.DB(ctx).First(&load, "id = ?", id).Error; err != nil {
		return nil, translate(err, "load")
	}
	return &load, nil
}

func (r *loadRepository) FindWithRelations(ctx context.Context, id uuid.UUID) (*domain.Load, error) {
	var load domain.Load
	err := r.DB(ctx).
		Preload("Company").
		Preload("Company.Logo").
		Preload("CargoType").
		Preload("PickupAddress").Preload("PickupAddress.Region").Preload("PickupAddress.City").
		Preload("DeliveryAddress").Preload("DeliveryAddress.Region").Preload("DeliveryAddress.City").
		Preload("Photos", func(db *gorm.DB) *gorm.DB { return db.Order("sort_order").Preload("File") }).
		Preload("Documents", func(db *gorm.DB) *gorm.DB { return db.Preload("File") }).
		First(&load, "id = ?", id).Error
	if err != nil {
		return nil, translate(err, "load")
	}
	return &load, nil
}

func (r *loadRepository) FindByReference(ctx context.Context, reference string) (*domain.Load, error) {
	var load domain.Load
	if err := r.DB(ctx).First(&load, "reference_no = ?", reference).Error; err != nil {
		return nil, translate(err, "load")
	}
	return &load, nil
}

// Search runs the marketplace query.
//
// Every geo predicate is ST_DWithin so the GIST indexes are used; the distance
// itself is only computed for surviving rows and only when the caller supplied
// a point to measure from.
func (r *loadRepository) Search(ctx context.Context, f domain.LoadFilter, offset, limit int, order string) ([]domain.LoadSearchResult, int64, error) {
	q := r.DB(ctx).Model(&domain.Load{})

	if f.CompanyID != nil {
		q = q.Where("loads.company_id = ?", *f.CompanyID)
	}
	if f.CreatedByUserID != nil {
		q = q.Where("loads.created_by_user_id = ?", *f.CreatedByUserID)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("loads.status IN ?", f.Statuses)
	}
	if f.ExcludeExpired {
		q = q.Where("loads.expires_at IS NULL OR loads.expires_at > now()")
	}
	if len(f.CargoTypeIDs) > 0 {
		q = q.Where("loads.cargo_type_id IN ?", f.CargoTypeIDs)
	}
	if len(f.PickupRegionIDs) > 0 {
		q = q.Where("loads.pickup_region_id IN ?", f.PickupRegionIDs)
	}
	if len(f.DeliveryRegionIDs) > 0 {
		q = q.Where("loads.delivery_region_id IN ?", f.DeliveryRegionIDs)
	}
	if f.PickupNear != nil && f.PickupNear.Valid() && f.PickupRadiusKm > 0 {
		q = q.Where("ST_DWithin(loads.pickup_location, ?::geography, ?)",
			f.PickupNear, f.PickupRadiusKm*1000)
	}
	if f.DeliveryNear != nil && f.DeliveryNear.Valid() && f.DeliveryRadiusKm > 0 {
		q = q.Where("ST_DWithin(loads.delivery_location, ?::geography, ?)",
			f.DeliveryNear, f.DeliveryRadiusKm*1000)
	}
	if f.MinWeightKg > 0 {
		q = q.Where("loads.weight_kg >= ?", f.MinWeightKg)
	}
	if f.MaxWeightKg > 0 {
		q = q.Where("loads.weight_kg <= ?", f.MaxWeightKg)
	}
	if f.MinVolumeM3 > 0 {
		q = q.Where("loads.volume_m3 >= ?", f.MinVolumeM3)
	}
	if f.MaxVolumeM3 > 0 {
		q = q.Where("loads.volume_m3 <= ?", f.MaxVolumeM3)
	}
	if len(f.VehicleTypes) > 0 {
		// A load with no stated body type is open to any truck, so it must stay
		// in the results when a driver filters by their own type.
		q = q.Where("loads.required_vehicle_type IS NULL OR loads.required_vehicle_type IN ?", f.VehicleTypes)
	}
	if f.RequiresADR != nil {
		q = q.Where("loads.requires_adr = ?", *f.RequiresADR)
	}
	if f.RequiresRefrigeration != nil {
		q = q.Where("loads.requires_refrigeration = ?", *f.RequiresRefrigeration)
	}
	if f.RequiresTailLift != nil {
		q = q.Where("loads.requires_tail_lift = ?", *f.RequiresTailLift)
	}
	if f.MinPriceAmount > 0 {
		q = q.Where("loads.price_amount >= ?", f.MinPriceAmount)
	}
	if f.MaxPriceAmount > 0 {
		q = q.Where("loads.price_amount <= ?", f.MaxPriceAmount)
	}
	if f.Currency != "" {
		q = q.Where("loads.price_currency = ?", f.Currency)
	}
	if len(f.PriceTypes) > 0 {
		q = q.Where("loads.price_type IN ?", f.PriceTypes)
	}
	if len(f.PaymentTerms) > 0 {
		q = q.Where("loads.payment_terms IN ?", f.PaymentTerms)
	}
	if f.PickupFrom != nil {
		q = q.Where("loads.pickup_from >= ?", *f.PickupFrom)
	}
	if f.PickupTo != nil {
		q = q.Where("loads.pickup_from <= ?", *f.PickupTo)
	}
	if f.FeaturedOnly {
		q = q.Where("loads.is_featured")
	}
	if f.UrgentOnly {
		q = q.Where("loads.is_urgent")
	}
	if f.Search != "" {
		// websearch_to_tsquery tolerates whatever a person types; the ILIKE on
		// the reference number covers "find LD-2601-00042" exactly.
		q = q.Where("loads.search_vector @@ websearch_to_tsquery('simple', ?) OR loads.reference_no ILIKE ?",
			f.Search, "%"+f.Search+"%")
	}
	if f.FitsVehicleID != nil {
		q = q.Where(vehicleFitsPredicate, *f.FitsVehicleID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "load")
	}

	type row struct {
		domain.Load
		PickupDistanceM *float64 `gorm:"column:pickup_distance_m"`
	}

	effectiveOrder := order
	if f.PickupNear != nil && f.PickupNear.Valid() {
		q = q.Select("loads.*, ST_Distance(loads.pickup_location, ?::geography) AS pickup_distance_m", f.PickupNear)
		if effectiveOrder == "" {
			effectiveOrder = "pickup_distance_m ASC"
		}
	} else {
		q = q.Select("loads.*")
		if effectiveOrder == "" {
			// Featured first, then newest: paid placement must not be reorderable
			// away by the default sort.
			effectiveOrder = "loads.is_featured DESC, loads.published_at DESC NULLS LAST, loads.created_at DESC"
		}
	}

	var rows []row
	if err := applyPage(q.Order(effectiveOrder), offset, limit).Scan(&rows).Error; err != nil {
		return nil, 0, translate(err, "load")
	}

	results := make([]domain.LoadSearchResult, 0, len(rows))
	for _, item := range rows {
		res := domain.LoadSearchResult{Load: item.Load}
		if item.PickupDistanceM != nil {
			km := *item.PickupDistanceM / 1000
			res.PickupDistanceKm = &km
		}
		results = append(results, res)
	}

	// Scan reads plain columns and leaves associations nil, so a raw result has
	// no route and no shipper — the two things a load board exists to show.
	if err := r.hydrate(ctx, results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// hydrate attaches the addresses and companies a list result needs, in two
// batched queries rather than two per row.
func (r *loadRepository) hydrate(ctx context.Context, results []domain.LoadSearchResult) error {
	if len(results) == 0 {
		return nil
	}

	addressIDs := make([]uuid.UUID, 0, len(results)*2)
	companyIDs := make([]uuid.UUID, 0, len(results))
	for i := range results {
		addressIDs = append(addressIDs, results[i].Load.PickupAddressID, results[i].Load.DeliveryAddressID)
		companyIDs = append(companyIDs, results[i].Load.CompanyID)
	}

	var addresses []domain.Address
	if err := r.DB(ctx).Preload("Region").Preload("City").
		Where("id IN ?", addressIDs).Find(&addresses).Error; err != nil {
		return translate(err, "address")
	}
	addressByID := make(map[uuid.UUID]*domain.Address, len(addresses))
	for i := range addresses {
		addressByID[addresses[i].ID] = &addresses[i]
	}

	var companies []domain.Company
	if err := r.DB(ctx).Preload("Logo").
		Where("id IN ?", companyIDs).Find(&companies).Error; err != nil {
		return translate(err, "company")
	}
	companyByID := make(map[uuid.UUID]*domain.Company, len(companies))
	for i := range companies {
		companyByID[companies[i].ID] = &companies[i]
	}

	for i := range results {
		results[i].Load.PickupAddress = addressByID[results[i].Load.PickupAddressID]
		results[i].Load.DeliveryAddress = addressByID[results[i].Load.DeliveryAddressID]
		results[i].Load.Company = companyByID[results[i].Load.CompanyID]
	}
	return nil
}

// vehicleFitsPredicate mirrors domain.Vehicle.CanCarry in SQL.
//
// Duplicating the rule is deliberate: filtering in Go would break paging and
// totals, because the database cannot know how many rows survive. The Go method
// remains the authority — it is what the reservation guard calls — so this
// predicate may only ever be more permissive, never more restrictive. Any change
// to Vehicle.CanCarry must be mirrored here in the same commit.
const vehicleFitsPredicate = `EXISTS (
	SELECT 1 FROM vehicles v
	WHERE v.id = ?
	  AND v.status = 'approved'
	  AND v.deleted_at IS NULL
	  AND v.capacity_kg >= loads.weight_kg
	  AND (loads.volume_m3 IS NULL OR v.volume_m3 IS NULL OR v.volume_m3 >= loads.volume_m3)
	  AND (loads.required_vehicle_type IS NULL
	       OR loads.required_vehicle_type = 'other'
	       OR loads.required_vehicle_type = v.type)
	  AND (NOT loads.requires_adr OR v.has_adr)
	  AND (NOT loads.requires_refrigeration OR v.has_refrigeration)
	  AND (NOT loads.requires_tail_lift OR v.has_tail_lift)
	  AND (loads.temp_min_c IS NULL OR v.temp_min_c IS NULL OR v.temp_min_c <= loads.temp_min_c)
	  AND (loads.temp_max_c IS NULL OR v.temp_max_c IS NULL OR v.temp_max_c >= loads.temp_max_c)
	  AND (loads.length_cm IS NULL OR v.length_cm IS NULL OR v.length_cm >= loads.length_cm)
	  AND (loads.width_cm  IS NULL OR v.width_cm  IS NULL OR v.width_cm  >= loads.width_cm)
	  AND (loads.height_cm IS NULL OR v.height_cm IS NULL OR v.height_cm >= loads.height_cm)
)`

func (r *loadRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return translate(r.DB(ctx).Where("id = ?", id).Delete(&domain.Load{}).Error, "load")
}

// TransitionStatus applies a guarded status change.
//
// The `from` set is part of the WHERE clause, so if another request already moved
// the load the update affects zero rows and the caller gets a conflict rather
// than silently overwriting a newer state.
func (r *loadRepository) TransitionStatus(ctx context.Context, id uuid.UUID, from []domain.LoadStatus, to domain.LoadStatus, columns map[string]any) error {
	updates := make(map[string]any, len(columns)+1)
	maps.Copy(updates, columns)
	updates["status"] = to

	q := r.DB(ctx).Model(&domain.Load{}).Where("id = ?", id)
	if len(from) > 0 {
		q = q.Where("status IN ?", from)
	}

	res := q.Updates(updates)
	if res.Error != nil {
		return translate(res.Error, "load")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrInvalidStatusChange.WithMessage(
			"This load has already changed state, refresh and try again")
	}
	return nil
}

func (r *loadRepository) CountActiveByCompany(ctx context.Context, companyID uuid.UUID) (int64, error) {
	var count int64
	err := r.DB(ctx).Model(&domain.Load{}).
		Where("company_id = ? AND status IN ?", companyID, []domain.LoadStatus{
			domain.LoadPublished, domain.LoadReserved, domain.LoadAssigned, domain.LoadInTransit,
		}).Count(&count).Error
	if err != nil {
		return 0, translate(err, "load")
	}
	return count, nil
}

func (r *loadRepository) IncrementViews(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.Load{}).Where("id = ?", id).
		UpdateColumn("views_count", gorm.Expr("views_count + 1")).Error
	return translate(err, "load")
}

// RecordView logs a unique view. A repeat view by the same user is not an error.
func (r *loadRepository) RecordView(ctx context.Context, view *domain.LoadView) error {
	err := r.DB(ctx).Exec(`
		INSERT INTO load_views (load_id, user_id, ip_address, viewed_at)
		VALUES (?, ?, ?::inet, ?)
		ON CONFLICT (load_id, user_id) WHERE user_id IS NOT NULL DO NOTHING`,
		view.LoadID, view.UserID, view.IPAddress, time.Now().UTC()).Error
	return translate(err, "load view")
}

// ExpireOverdue closes published loads whose pickup window has passed.
func (r *loadRepository) ExpireOverdue(ctx context.Context, now time.Time) (int64, error) {
	res := r.DB(ctx).Model(&domain.Load{}).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at < ?", domain.LoadPublished, now).
		Updates(map[string]any{"status": domain.LoadExpired})
	if res.Error != nil {
		return 0, translate(res.Error, "load")
	}
	return res.RowsAffected, nil
}

// FindDuplicate looks for a live load with the same normalised fingerprint.
func (r *loadRepository) FindDuplicate(ctx context.Context, companyID uuid.UUID, hash string) (*domain.Load, error) {
	var load domain.Load
	err := r.DB(ctx).
		Where("company_id = ? AND duplicate_hash = ? AND status IN ?",
			companyID, hash, []domain.LoadStatus{domain.LoadPublished, domain.LoadReserved}).
		First(&load).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "load")
	}
	return &load, nil
}

func (r *loadRepository) AddPhoto(ctx context.Context, photo *domain.LoadPhoto) error {
	return translate(r.DB(ctx).Create(photo).Error, "load photo")
}

func (r *loadRepository) DeletePhoto(ctx context.Context, photoID uuid.UUID) error {
	err := r.DB(ctx).Where("id = ?", photoID).Delete(&domain.LoadPhoto{}).Error
	return translate(err, "load photo")
}

func (r *loadRepository) AddDocument(ctx context.Context, doc *domain.LoadDocument) error {
	return translate(r.DB(ctx).Create(doc).Error, "load document")
}

func (r *loadRepository) DeleteDocument(ctx context.Context, docID uuid.UUID) error {
	err := r.DB(ctx).Where("id = ?", docID).Delete(&domain.LoadDocument{}).Error
	return translate(err, "load document")
}

// ReplacePhotos swaps the whole gallery, which is how an edit form behaves.
func (r *loadRepository) ReplacePhotos(ctx context.Context, loadID uuid.UUID, fileIDs []uuid.UUID) error {
	db := r.DB(ctx)
	if err := db.Where("load_id = ?", loadID).Delete(&domain.LoadPhoto{}).Error; err != nil {
		return translate(err, "load photo")
	}
	if len(fileIDs) == 0 {
		return nil
	}
	photos := make([]domain.LoadPhoto, 0, len(fileIDs))
	for i, fileID := range fileIDs {
		photos = append(photos, domain.LoadPhoto{
			ID:        uuid.New(),
			LoadID:    loadID,
			FileID:    fileID,
			SortOrder: int16(i),
		})
	}
	return translate(db.Create(&photos).Error, "load photo")
}

// ─── cargo types ────────────────────────────────────────────────────

type cargoTypeRepository struct{ Base }

// NewCargoTypeRepository builds the cargo-type repository.
func NewCargoTypeRepository(db *gorm.DB) domain.CargoTypeRepository {
	return &cargoTypeRepository{Base: NewBase(db)}
}

func (r *cargoTypeRepository) List(ctx context.Context) ([]domain.CargoType, error) {
	var types []domain.CargoType
	err := r.DB(ctx).Where("is_active").Order("sort_order, name_en").Find(&types).Error
	if err != nil {
		return nil, translate(err, "cargo type")
	}
	return types, nil
}

func (r *cargoTypeRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.CargoType, error) {
	var cargoType domain.CargoType
	if err := r.DB(ctx).First(&cargoType, "id = ?", id).Error; err != nil {
		return nil, translate(err, "cargo type")
	}
	return &cargoType, nil
}

// ─── bookmarks & saved searches ─────────────────────────────────────

type savedRepository struct{ Base }

// NewSavedRepository builds the bookmark and saved-search repository.
func NewSavedRepository(db *gorm.DB) domain.SavedRepository {
	return &savedRepository{Base: NewBase(db)}
}

// SaveLoad bookmarks a load. Saving twice is a no-op, not an error.
func (r *savedRepository) SaveLoad(ctx context.Context, userID, loadID uuid.UUID) error {
	err := r.DB(ctx).Exec(`
		INSERT INTO saved_loads (id, user_id, load_id, created_at)
		VALUES (?, ?, ?, now())
		ON CONFLICT (user_id, load_id) DO NOTHING`,
		uuid.New(), userID, loadID).Error
	return translate(err, "saved load")
}

func (r *savedRepository) UnsaveLoad(ctx context.Context, userID, loadID uuid.UUID) error {
	err := r.DB(ctx).Where("user_id = ? AND load_id = ?", userID, loadID).
		Delete(&domain.SavedLoad{}).Error
	return translate(err, "saved load")
}

func (r *savedRepository) ListSavedLoads(ctx context.Context, userID uuid.UUID, offset, limit int) ([]domain.SavedLoad, int64, error) {
	q := r.DB(ctx).Model(&domain.SavedLoad{}).Where("user_id = ?", userID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "saved load")
	}

	var items []domain.SavedLoad
	err := applyPage(q.
		Preload("Load").
		Preload("Load.Company").
		Preload("Load.PickupAddress").
		Preload("Load.DeliveryAddress").
		Order("created_at DESC"), offset, limit).Find(&items).Error
	if err != nil {
		return nil, 0, translate(err, "saved load")
	}
	return items, total, nil
}

// IsSaved answers the bookmark question for a whole page of loads in one query,
// so a list of 20 loads costs one round trip rather than 20.
func (r *savedRepository) IsSaved(ctx context.Context, userID uuid.UUID, loadIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(loadIDs))
	if len(loadIDs) == 0 {
		return out, nil
	}
	var saved []uuid.UUID
	err := r.DB(ctx).Model(&domain.SavedLoad{}).
		Where("user_id = ? AND load_id IN ?", userID, loadIDs).
		Pluck("load_id", &saved).Error
	if err != nil {
		return nil, translate(err, "saved load")
	}
	for _, id := range saved {
		out[id] = true
	}
	return out, nil
}

func (r *savedRepository) CreateSearch(ctx context.Context, search *domain.SavedSearch) error {
	return translate(r.DB(ctx).Create(search).Error, "saved search")
}

func (r *savedRepository) UpdateSearch(ctx context.Context, search *domain.SavedSearch) error {
	err := r.DB(ctx).Model(&domain.SavedSearch{ID: search.ID}).
		Select("name", "filters", "notify_enabled").Updates(search).Error
	return translate(err, "saved search")
}

func (r *savedRepository) ListSearches(ctx context.Context, userID uuid.UUID) ([]domain.SavedSearch, error) {
	var items []domain.SavedSearch
	err := r.DB(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&items).Error
	if err != nil {
		return nil, translate(err, "saved search")
	}
	return items, nil
}

func (r *savedRepository) FindSearchByID(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
	var search domain.SavedSearch
	if err := r.DB(ctx).First(&search, "id = ?", id).Error; err != nil {
		return nil, translate(err, "saved search")
	}
	return &search, nil
}

func (r *savedRepository) DeleteSearch(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Where("id = ?", id).Delete(&domain.SavedSearch{}).Error
	return translate(err, "saved search")
}

// ListNotifiableSearches returns searches eligible for a new-load push. The
// hour-long floor stops one busy corridor from spamming a driver.
func (r *savedRepository) ListNotifiableSearches(ctx context.Context, limit int) ([]domain.SavedSearch, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	var items []domain.SavedSearch
	err := r.DB(ctx).
		Where("notify_enabled").
		Where("last_notified_at IS NULL OR last_notified_at < now() - interval '1 hour'").
		Order("last_notified_at NULLS FIRST").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, translate(err, "saved search")
	}
	return items, nil
}

func (r *savedRepository) MarkNotified(ctx context.Context, id uuid.UUID, at time.Time) error {
	err := r.DB(ctx).Model(&domain.SavedSearch{}).
		Where("id = ?", id).Update("last_notified_at", at).Error
	return translate(err, "saved search")
}
