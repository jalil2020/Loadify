package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/geo"
	"github.com/loadify/backend/pkg/logger"
)

// defaultLoadExpiryHours is how long a published load stays live when the
// company does not say otherwise.
const defaultLoadExpiryHours = 72

// LoadUsecase owns the shipment lifecycle and the marketplace search.
type LoadUsecase struct {
	loads      domain.LoadRepository
	cargoTypes domain.CargoTypeRepository
	saved      domain.SavedRepository
	bookings   domain.BookingRepository
	companies  domain.CompanyRepository
	drivers    domain.DriverRepository
	vehicles   domain.VehicleRepository
	geo        domain.GeoRepository
	files      *FileUsecase
	tx         domain.TxManager
	audit      domain.AuditLogger
	events     domain.EventPublisher
	cache      cache.Cache
	log        logger.Logger
}

// NewLoadUsecase wires the load usecase.
func NewLoadUsecase(
	loads domain.LoadRepository,
	cargoTypes domain.CargoTypeRepository,
	saved domain.SavedRepository,
	bookings domain.BookingRepository,
	companies domain.CompanyRepository,
	drivers domain.DriverRepository,
	vehicles domain.VehicleRepository,
	geoRepo domain.GeoRepository,
	files *FileUsecase,
	tx domain.TxManager,
	audit domain.AuditLogger,
	events domain.EventPublisher,
	c cache.Cache,
	log logger.Logger,
) *LoadUsecase {
	return &LoadUsecase{
		loads: loads, cargoTypes: cargoTypes, saved: saved, bookings: bookings,
		companies: companies, drivers: drivers, vehicles: vehicles, geo: geoRepo,
		files: files, tx: tx, audit: audit, events: events, cache: c, log: log,
	}
}

// Viewer describes who is asking, which decides what a load response reveals.
type Viewer struct {
	UserID    uuid.UUID
	CompanyID *uuid.UUID
	DriverID  *uuid.UUID
	IsAdmin   bool
	// HasContactAccess is resolved per load: a live booking, company membership
	// or an admin role earns the shipper's phone number.
	Authenticated bool
}

// Create saves a shipment, publishing it immediately when asked.
func (u *LoadUsecase) Create(ctx context.Context, actorID, companyID uuid.UUID, req dto.SaveLoadRequest) (*dto.LoadResponse, error) {
	company, err := u.companies.FindByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if err := validateLoadRequest(req); err != nil {
		return nil, err
	}
	if req.Publish && !company.IsVerified() {
		return nil, apperror.ErrCompanyNotVerified.WithMessage(
			"Your company must be verified before you can publish loads")
	}
	if req.CargoTypeID != nil {
		if _, err := u.cargoTypes.FindByID(ctx, *req.CargoTypeID); err != nil {
			return nil, apperror.ErrValidation.
				WithDetails(apperror.FieldError{Field: "cargo_type_id", Message: "Unknown cargo type"})
		}
	}
	for _, fileID := range slices.Concat(req.PhotoFileIDs, req.DocumentFileIDs) {
		if err := u.files.AssertOwnership(ctx, actorID, fileID); err != nil {
			return nil, err
		}
	}

	load := &domain.Load{
		ID:              uuid.New(),
		CompanyID:       companyID,
		CreatedByUserID: actorID,
		Status:          domain.LoadDraft,
	}
	applyLoadRequest(load, req)

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		pickup, err := u.createAddress(txCtx, req.PickupAddress)
		if err != nil {
			return err
		}
		delivery, err := u.createAddress(txCtx, req.DeliveryAddress)
		if err != nil {
			return err
		}

		load.PickupAddressID = pickup.ID
		load.DeliveryAddressID = delivery.ID
		load.PickupLocation = pickup.Location
		load.DeliveryLocation = delivery.Location
		load.PickupRegionID = pickup.RegionID
		load.DeliveryRegionID = delivery.RegionID

		// Straight-line distance is a usable default; the client can override it
		// with a routed distance from the Maps API.
		if load.DistanceKm == nil {
			km := geo.DistanceKm(pickup.Location, delivery.Location)
			load.DistanceKm = &km
		}
		load.DuplicateHash = loadFingerprint(load)

		if req.Publish {
			if err := u.assertCanPublish(txCtx, companyID, load); err != nil {
				return err
			}
			now := time.Now().UTC()
			expiry := now.Add(defaultLoadExpiryHours * time.Hour)
			load.Status = domain.LoadPublished
			load.PublishedAt = &now
			load.ExpiresAt = &expiry
		}

		if err := u.loads.Create(txCtx, load); err != nil {
			return err
		}
		return u.attachFiles(txCtx, actorID, load.ID, req.PhotoFileIDs, req.DocumentFileIDs)
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "load.created",
		ResourceType: "load",
		ResourceID:   &load.ID,
		NewValues: map[string]any{
			"reference_no": load.ReferenceNo,
			"status":       load.Status,
			"price_amount": load.PriceAmount,
		},
	})
	if load.Status == domain.LoadPublished {
		u.announcePublished(ctx, load)
	}

	return u.Get(ctx, load.ID, Viewer{
		UserID: actorID, CompanyID: &companyID, Authenticated: true,
	}, false)
}

// Update edits a load. Published loads may still be corrected, but structural
// changes are rejected once a driver is committed.
func (u *LoadUsecase) Update(ctx context.Context, actorID, companyID, loadID uuid.UUID, req dto.SaveLoadRequest) (*dto.LoadResponse, error) {
	load, err := u.assertCompanyLoad(ctx, companyID, loadID)
	if err != nil {
		return nil, err
	}
	if err := validateLoadRequest(req); err != nil {
		return nil, err
	}
	// Once a truck is assigned the terms are a mutual agreement, not a form.
	if load.Status != domain.LoadDraft && load.Status != domain.LoadPublished && load.Status != domain.LoadExpired {
		return nil, apperror.ErrInvalidStatusChange.WithMessage(
			"This load can no longer be edited because a driver is already assigned")
	}
	for _, fileID := range slices.Concat(req.PhotoFileIDs, req.DocumentFileIDs) {
		if err := u.files.AssertOwnership(ctx, actorID, fileID); err != nil {
			return nil, err
		}
	}

	applyLoadRequest(load, req)

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		pickup, err := u.createAddress(txCtx, req.PickupAddress)
		if err != nil {
			return err
		}
		delivery, err := u.createAddress(txCtx, req.DeliveryAddress)
		if err != nil {
			return err
		}
		load.PickupAddressID = pickup.ID
		load.DeliveryAddressID = delivery.ID
		load.PickupLocation = pickup.Location
		load.DeliveryLocation = delivery.Location
		load.PickupRegionID = pickup.RegionID
		load.DeliveryRegionID = delivery.RegionID
		if req.DistanceKm == nil {
			km := geo.DistanceKm(pickup.Location, delivery.Location)
			load.DistanceKm = &km
		}
		load.DuplicateHash = loadFingerprint(load)

		if err := u.loads.Update(txCtx, load); err != nil {
			return err
		}
		if req.PhotoFileIDs != nil {
			if err := u.loads.ReplacePhotos(txCtx, load.ID, req.PhotoFileIDs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "load.updated",
		ResourceType: "load",
		ResourceID:   &load.ID,
	})

	return u.Get(ctx, load.ID, Viewer{
		UserID: actorID, CompanyID: &companyID, Authenticated: true,
	}, false)
}

// Publish moves a draft or expired load onto the marketplace.
func (u *LoadUsecase) Publish(ctx context.Context, actorID, companyID, loadID uuid.UUID, req dto.PublishLoadRequest) (*dto.LoadResponse, error) {
	load, err := u.assertCompanyLoad(ctx, companyID, loadID)
	if err != nil {
		return nil, err
	}
	if !load.Status.CanTransitionTo(domain.LoadPublished) {
		return nil, apperror.ErrInvalidStatusChange.WithMessage(
			fmt.Sprintf("A load in state '%s' cannot be published", load.Status))
	}
	if err := u.assertCanPublish(ctx, companyID, load); err != nil {
		return nil, err
	}

	hours := defaultLoadExpiryHours
	if req.ExpiresInHours != nil {
		hours = *req.ExpiresInHours
	}
	now := time.Now().UTC()
	expiry := now.Add(time.Duration(hours) * time.Hour)

	err = u.loads.TransitionStatus(ctx, loadID,
		[]domain.LoadStatus{domain.LoadDraft, domain.LoadExpired},
		domain.LoadPublished,
		map[string]any{
			"published_at":        now,
			"expires_at":          expiry,
			"cancelled_at":        nil,
			"cancellation_reason": nil,
		})
	if err != nil {
		return nil, err
	}

	load.Status = domain.LoadPublished
	load.PublishedAt = &now
	load.ExpiresAt = &expiry
	u.announcePublished(ctx, load)

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "load.published",
		ResourceType: "load",
		ResourceID:   &loadID,
	})

	return u.Get(ctx, loadID, Viewer{
		UserID: actorID, CompanyID: &companyID, Authenticated: true,
	}, false)
}

// Unpublish pulls a load back to draft while nobody is committed to it.
func (u *LoadUsecase) Unpublish(ctx context.Context, actorID, companyID, loadID uuid.UUID) error {
	if _, err := u.assertCompanyLoad(ctx, companyID, loadID); err != nil {
		return err
	}
	err := u.loads.TransitionStatus(ctx, loadID,
		[]domain.LoadStatus{domain.LoadPublished}, domain.LoadDraft,
		map[string]any{"published_at": nil, "expires_at": nil})
	if err != nil {
		return err
	}
	// Drivers waiting on this load must be told, not left hanging.
	if _, err := u.bookings.RejectOthers(ctx, loadID, uuid.Nil, "The shipper withdrew this load"); err != nil {
		u.log.Warn("closing bookings after unpublish failed", logger.Err(err))
	}
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "load.unpublished",
		ResourceType: "load",
		ResourceID:   &loadID,
	})
	return nil
}

// Cancel closes a load and every open booking on it.
func (u *LoadUsecase) Cancel(ctx context.Context, actorID, companyID, loadID uuid.UUID, req dto.CancelLoadRequest) error {
	load, err := u.assertCompanyLoad(ctx, companyID, loadID)
	if err != nil {
		return err
	}
	if !load.Status.CanTransitionTo(domain.LoadCancelled) {
		return apperror.ErrInvalidStatusChange.WithMessage(
			fmt.Sprintf("A load in state '%s' cannot be cancelled", load.Status))
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.loads.TransitionStatus(txCtx, loadID,
			[]domain.LoadStatus{load.Status}, domain.LoadCancelled,
			map[string]any{
				"cancelled_at":        time.Now().UTC(),
				"cancellation_reason": req.Reason,
			}); err != nil {
			return err
		}
		_, err := u.bookings.RejectOthers(txCtx, loadID, uuid.Nil, req.Reason)
		return err
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "load.cancelled",
		ResourceType: "load",
		ResourceID:   &loadID,
		NewValues:    map[string]any{"reason": req.Reason},
	})
	return nil
}

// Delete removes a draft. Anything already published leaves a trail and is
// cancelled instead.
func (u *LoadUsecase) Delete(ctx context.Context, actorID, companyID, loadID uuid.UUID) error {
	load, err := u.assertCompanyLoad(ctx, companyID, loadID)
	if err != nil {
		return err
	}
	if load.Status != domain.LoadDraft {
		return apperror.ErrInvalidStatusChange.WithMessage(
			"Only drafts can be deleted; cancel the load instead")
	}
	if err := u.loads.SoftDelete(ctx, loadID); err != nil {
		return err
	}
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "load.deleted",
		ResourceType: "load",
		ResourceID:   &loadID,
	})
	return nil
}

// Get returns one load, revealing contact details only to entitled viewers.
func (u *LoadUsecase) Get(ctx context.Context, loadID uuid.UUID, viewer Viewer, countView bool) (*dto.LoadResponse, error) {
	load, err := u.loads.FindWithRelations(ctx, loadID)
	if err != nil {
		return nil, err
	}

	// A draft belongs to its company alone.
	if load.Status == domain.LoadDraft && !u.canManage(load, viewer) {
		return nil, apperror.ErrNotFound.WithMessage("load not found")
	}

	if countView && viewer.Authenticated && !u.canManage(load, viewer) {
		if err := u.loads.IncrementViews(ctx, loadID); err != nil {
			u.log.Warn("incrementing load views failed", logger.Err(err))
		}
		if err := u.loads.RecordView(ctx, &domain.LoadView{
			LoadID: loadID, UserID: &viewer.UserID,
		}); err != nil {
			u.log.Warn("recording load view failed", logger.Err(err))
		}
	}

	res := u.toResponse(ctx, load, nil)
	u.applyViewerFields(ctx, res, load, viewer)
	return res, nil
}

// Search runs the marketplace query for the given viewer.
func (u *LoadUsecase) Search(ctx context.Context, q dto.LoadSearchQuery, viewer Viewer, offset, limit int, order string) ([]dto.LoadResponse, int64, error) {
	filter, err := u.buildFilter(ctx, q, viewer)
	if err != nil {
		return nil, 0, err
	}

	results, total, err := u.loads.Search(ctx, filter, offset, limit, order)
	if err != nil {
		return nil, 0, err
	}

	loadIDs := make([]uuid.UUID, 0, len(results))
	for i := range results {
		loadIDs = append(loadIDs, results[i].Load.ID)
	}

	// One query for the whole page instead of one per row.
	savedMap := map[uuid.UUID]bool{}
	if viewer.Authenticated && len(loadIDs) > 0 {
		if m, err := u.saved.IsSaved(ctx, viewer.UserID, loadIDs); err == nil {
			savedMap = m
		}
	}

	out := make([]dto.LoadResponse, 0, len(results))
	for i := range results {
		load := results[i].Load
		item := u.toResponse(ctx, &load, results[i].PickupDistanceKm)
		item.IsSaved = savedMap[load.ID]
		// List results never carry a phone number; the detail endpoint decides.
		if item.Company != nil {
			item.Company.Phone = nil
		}
		out = append(out, *item)
	}
	return out, total, nil
}

// ListCompanyLoads returns the loads of the caller's company, drafts included.
func (u *LoadUsecase) ListCompanyLoads(ctx context.Context, companyID uuid.UUID, q dto.LoadSearchQuery, viewer Viewer, offset, limit int, order string) ([]dto.LoadResponse, int64, error) {
	filter, err := u.buildFilter(ctx, q, viewer)
	if err != nil {
		return nil, 0, err
	}
	filter.CompanyID = &companyID
	filter.ExcludeExpired = false
	if q.Status != "" {
		filter.Statuses = []domain.LoadStatus{domain.LoadStatus(q.Status)}
	} else {
		filter.Statuses = nil
	}

	results, total, err := u.loads.Search(ctx, filter, offset, limit, order)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.LoadResponse, 0, len(results))
	for i := range results {
		load := results[i].Load
		out = append(out, *u.toResponse(ctx, &load, results[i].PickupDistanceKm))
	}
	return out, total, nil
}

// ─── bookmarks & saved searches ─────────────────────────────────────

// SaveLoad bookmarks a load for the caller.
func (u *LoadUsecase) SaveLoad(ctx context.Context, userID, loadID uuid.UUID) error {
	if _, err := u.loads.FindByID(ctx, loadID); err != nil {
		return err
	}
	return u.saved.SaveLoad(ctx, userID, loadID)
}

// UnsaveLoad removes a bookmark.
func (u *LoadUsecase) UnsaveLoad(ctx context.Context, userID, loadID uuid.UUID) error {
	return u.saved.UnsaveLoad(ctx, userID, loadID)
}

// ListSavedLoads returns the caller's bookmarks.
func (u *LoadUsecase) ListSavedLoads(ctx context.Context, viewer Viewer, offset, limit int) ([]dto.LoadResponse, int64, error) {
	items, total, err := u.saved.ListSavedLoads(ctx, viewer.UserID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.LoadResponse, 0, len(items))
	for i := range items {
		if items[i].Load == nil {
			continue
		}
		item := u.toResponse(ctx, items[i].Load, nil)
		item.IsSaved = true
		if item.Company != nil {
			item.Company.Phone = nil
		}
		out = append(out, *item)
	}
	return out, total, nil
}

// CreateSavedSearch stores a reusable filter.
func (u *LoadUsecase) CreateSavedSearch(ctx context.Context, userID uuid.UUID, req dto.SaveSearchRequest) (*dto.SavedSearchResponse, error) {
	search := &domain.SavedSearch{
		ID:            uuid.New(),
		UserID:        userID,
		Name:          strings.TrimSpace(req.Name),
		Filters:       domain.JSONMap(req.Filters),
		NotifyEnabled: req.NotifyEnabled,
	}
	if search.Filters == nil {
		search.Filters = domain.JSONMap{}
	}
	if err := u.saved.CreateSearch(ctx, search); err != nil {
		return nil, err
	}
	return savedSearchResponse(search), nil
}

// ListSavedSearches returns the caller's stored filters.
func (u *LoadUsecase) ListSavedSearches(ctx context.Context, userID uuid.UUID) ([]dto.SavedSearchResponse, error) {
	items, err := u.saved.ListSearches(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SavedSearchResponse, 0, len(items))
	for i := range items {
		out = append(out, *savedSearchResponse(&items[i]))
	}
	return out, nil
}

// DeleteSavedSearch removes one stored filter belonging to the caller.
func (u *LoadUsecase) DeleteSavedSearch(ctx context.Context, userID, searchID uuid.UUID) error {
	search, err := u.saved.FindSearchByID(ctx, searchID)
	if err != nil {
		return err
	}
	if search.UserID != userID {
		return apperror.ErrForbidden
	}
	return u.saved.DeleteSearch(ctx, searchID)
}

// CargoTypes returns the localised cargo vocabulary.
func (u *LoadUsecase) CargoTypes(ctx context.Context, locale domain.Locale) ([]dto.CargoTypeResponse, error) {
	key := "ref:cargo_types:" + string(locale)
	var cached []dto.CargoTypeResponse
	if err := u.cache.Get(ctx, key, &cached); err == nil {
		return cached, nil
	}

	types, err := u.cargoTypes.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.CargoTypeResponse, 0, len(types))
	for _, t := range types {
		out = append(out, dto.CargoTypeResponse{
			ID:                  t.ID,
			Code:                t.Code,
			Name:                domain.LocalisedName(t.NameUZ, t.NameRU, t.NameEN, locale),
			Icon:                t.Icon,
			RequiresADR:         t.RequiresADR,
			RequiresTemperature: t.RequiresTemperature,
		})
	}
	if err := u.cache.Set(ctx, key, out, referenceCacheTTL); err != nil {
		u.log.Warn("caching cargo types failed", logger.Err(err))
	}
	return out, nil
}

// ExpireOverdue is the scheduled sweep that closes stale published loads.
func (u *LoadUsecase) ExpireOverdue(ctx context.Context) (int64, error) {
	return u.loads.ExpireOverdue(ctx, time.Now().UTC())
}

// ─── internals ──────────────────────────────────────────────────────

// assertCompanyLoad loads a shipment and confirms it belongs to the company.
func (u *LoadUsecase) assertCompanyLoad(ctx context.Context, companyID, loadID uuid.UUID) (*domain.Load, error) {
	load, err := u.loads.FindByID(ctx, loadID)
	if err != nil {
		return nil, err
	}
	if load.CompanyID != companyID {
		// Not "forbidden": revealing that the load exists tells a competitor
		// something they have no right to know.
		return nil, apperror.ErrNotFound.WithMessage("load not found")
	}
	return load, nil
}

// assertCanPublish enforces the plan limit and the duplicate check.
func (u *LoadUsecase) assertCanPublish(ctx context.Context, companyID uuid.UUID, load *domain.Load) error {
	company, err := u.companies.FindByID(ctx, companyID)
	if err != nil {
		return err
	}
	if !company.IsVerified() {
		return apperror.ErrCompanyNotVerified
	}
	if load.PickupFrom.Before(time.Now().UTC().Add(-24 * time.Hour)) {
		return apperror.ErrValidation.WithDetails(apperror.FieldError{
			Field:   "pickup_from",
			Message: "The pickup date is in the past",
		})
	}
	if load.DuplicateHash != nil {
		existing, err := u.loads.FindDuplicate(ctx, companyID, *load.DuplicateHash)
		if err != nil {
			u.log.Warn("duplicate load check failed", logger.Err(err))
		} else if existing != nil && existing.ID != load.ID {
			return apperror.ErrConflict.WithMessage(
				"You already have an identical load live: " + existing.ReferenceNo)
		}
	}
	return nil
}

// canManage reports whether the viewer acts for the load's company.
func (u *LoadUsecase) canManage(load *domain.Load, viewer Viewer) bool {
	if viewer.IsAdmin {
		return true
	}
	return viewer.CompanyID != nil && *viewer.CompanyID == load.CompanyID
}

// applyViewerFields fills in the parts of the response that depend on who asked:
// the shipper's phone number, the bookmark flag and the viewer's own booking.
func (u *LoadUsecase) applyViewerFields(ctx context.Context, res *dto.LoadResponse, load *domain.Load, viewer Viewer) {
	if !viewer.Authenticated {
		if res.Company != nil {
			res.Company.Phone = nil
		}
		return
	}

	if m, err := u.saved.IsSaved(ctx, viewer.UserID, []uuid.UUID{load.ID}); err == nil {
		res.IsSaved = m[load.ID]
	}

	contactAllowed := u.canManage(load, viewer)

	if viewer.DriverID != nil {
		booking, err := u.bookings.FindLiveForPair(ctx, load.ID, *viewer.DriverID)
		if err != nil {
			u.log.Warn("looking up viewer booking failed", logger.Err(err))
		} else if booking != nil {
			res.MyBooking = &dto.BookingSummary{
				ID:          booking.ID,
				Status:      string(booking.Status),
				InitiatedBy: string(booking.InitiatedBy),
				Price:       booking.Price(),
				Currency:    booking.Currency,
				ExpiresAt:   booking.ExpiresAt,
				CreatedAt:   booking.CreatedAt,
			}
			// A live booking is exactly the point at which the two parties need
			// to be able to phone each other.
			contactAllowed = true
		}
	}

	if !contactAllowed && res.Company != nil {
		res.Company.Phone = nil
	}
}

// buildFilter translates the query DTO into a repository filter.
func (u *LoadUsecase) buildFilter(ctx context.Context, q dto.LoadSearchQuery, viewer Viewer) (domain.LoadFilter, error) {
	filter := domain.LoadFilter{
		MinWeightKg:           q.MinWeightKg,
		MaxWeightKg:           q.MaxWeightKg,
		MinVolumeM3:           q.MinVolumeM3,
		MaxVolumeM3:           q.MaxVolumeM3,
		RequiresADR:           q.RequiresADR,
		RequiresRefrigeration: q.RequiresRefrigeration,
		RequiresTailLift:      q.RequiresTailLift,
		MinPriceAmount:        q.MinPriceAmount,
		MaxPriceAmount:        q.MaxPriceAmount,
		Currency:              q.Currency,
		PickupFrom:            q.PickupFrom,
		PickupTo:              q.PickupTo,
		FeaturedOnly:          q.FeaturedOnly,
		UrgentOnly:            q.UrgentOnly,
		Statuses:              []domain.LoadStatus{domain.LoadPublished},
		ExcludeExpired:        true,
	}

	filter.PickupRegionIDs = parseUUIDs(q.PickupRegionIDs)
	filter.DeliveryRegionIDs = parseUUIDs(q.DeliveryRegionIDs)
	filter.CargoTypeIDs = parseUUIDs(q.CargoTypeIDs)

	for _, t := range q.VehicleTypes {
		vt := domain.VehicleType(t)
		if slices.Contains(domain.AllVehicleTypes, vt) {
			filter.VehicleTypes = append(filter.VehicleTypes, vt)
		}
	}
	for _, t := range q.PriceTypes {
		filter.PriceTypes = append(filter.PriceTypes, domain.PriceType(t))
	}
	for _, t := range q.PaymentTerms {
		filter.PaymentTerms = append(filter.PaymentTerms, domain.PaymentTerms(t))
	}

	if q.PickupLat != nil && q.PickupLng != nil {
		point := geo.NewPoint(*q.PickupLat, *q.PickupLng)
		if !point.Valid() {
			return filter, apperror.ErrValidation.WithMessage("Invalid pickup coordinates")
		}
		filter.PickupNear = &point
		filter.PickupRadiusKm = q.PickupRadiusKm
		if filter.PickupRadiusKm == 0 {
			filter.PickupRadiusKm = 100
		}
	}
	if q.DeliveryLat != nil && q.DeliveryLng != nil {
		point := geo.NewPoint(*q.DeliveryLat, *q.DeliveryLng)
		if !point.Valid() {
			return filter, apperror.ErrValidation.WithMessage("Invalid delivery coordinates")
		}
		filter.DeliveryNear = &point
		filter.DeliveryRadiusKm = q.DeliveryRadiusKm
		if filter.DeliveryRadiusKm == 0 {
			filter.DeliveryRadiusKm = 100
		}
	}

	// Truck-fit filtering: an explicit vehicle id must belong to the caller.
	switch {
	case q.FitsVehicleID != "":
		id, err := uuid.Parse(q.FitsVehicleID)
		if err != nil {
			return filter, apperror.ErrValidation.WithMessage("Invalid vehicle id")
		}
		vehicle, err := u.vehicles.FindByID(ctx, id)
		if err != nil {
			return filter, err
		}
		if viewer.DriverID == nil || vehicle.DriverID != *viewer.DriverID {
			return filter, apperror.ErrForbidden.WithMessage("That vehicle is not yours")
		}
		filter.FitsVehicleID = &id
	case q.FitsMyVehicle && viewer.DriverID != nil:
		vehicles, err := u.vehicles.ListByDriver(ctx, *viewer.DriverID)
		if err != nil {
			return filter, err
		}
		for i := range vehicles {
			if vehicles[i].IsApproved() {
				// ListByDriver orders primary first, so this picks the default truck.
				filter.FitsVehicleID = &vehicles[i].ID
				break
			}
		}
		if filter.FitsVehicleID == nil {
			return filter, apperror.ErrNoApprovedVehicle
		}
	}

	return filter, nil
}

func (u *LoadUsecase) createAddress(ctx context.Context, req dto.AddressRequest) (*domain.Address, error) {
	point := geo.NewPoint(req.Location.Lat, req.Location.Lng)
	if !point.Valid() {
		return nil, apperror.ErrValidation.WithMessage("Invalid address coordinates")
	}
	address := &domain.Address{
		ID:            uuid.New(),
		RegionID:      req.RegionID,
		CityID:        req.CityID,
		Line1:         req.Line1,
		Line2:         nilIfBlank(req.Line2),
		PostalCode:    nilIfBlank(req.PostalCode),
		Formatted:     req.Formatted,
		GooglePlaceID: nilIfBlank(req.GooglePlaceID),
		Location:      point,
		ContactName:   nilIfBlank(req.ContactName),
		ContactPhone:  nilIfBlank(req.ContactPhone),
		Notes:         nilIfBlank(req.Notes),
	}
	if address.Formatted == "" {
		address.Formatted = req.Line1
	}
	if err := u.geo.CreateAddress(ctx, address); err != nil {
		return nil, err
	}
	return address, nil
}

func (u *LoadUsecase) attachFiles(ctx context.Context, actorID, loadID uuid.UUID, photoIDs, docIDs []uuid.UUID) error {
	for i, fileID := range photoIDs {
		if err := u.loads.AddPhoto(ctx, &domain.LoadPhoto{
			ID: uuid.New(), LoadID: loadID, FileID: fileID, SortOrder: int16(i),
		}); err != nil {
			return err
		}
	}
	for _, fileID := range docIDs {
		if err := u.loads.AddDocument(ctx, &domain.LoadDocument{
			ID: uuid.New(), LoadID: loadID, FileID: fileID, UploadedBy: &actorID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// announcePublished tells the realtime layer a new load is live so subscribed
// drivers hear about it. A publish must not fail because the bus is down.
func (u *LoadUsecase) announcePublished(ctx context.Context, load *domain.Load) {
	if u.events == nil {
		return
	}
	payload := map[string]any{
		"load_id":      load.ID,
		"reference_no": load.ReferenceNo,
		"title":        load.Title,
		"company_id":   load.CompanyID,
		"weight_kg":    load.WeightKg,
		"price_amount": load.PriceAmount,
		"currency":     load.PriceCurrency,
		"pickup_lat":   load.PickupLocation.Lat,
		"pickup_lng":   load.PickupLocation.Lng,
		"vehicle_type": load.RequiredVehicleType,
		"pickup_from":  load.PickupFrom,
	}
	if err := u.events.PublishLoad(ctx, domain.EventLoadPublished, load.ID, payload); err != nil {
		u.log.Warn("announcing published load failed",
			logger.String("load_id", load.ID.String()), logger.Err(err))
	}
}

// validateLoadRequest covers the cross-field rules the struct tags cannot express.
func validateLoadRequest(req dto.SaveLoadRequest) error {
	var details []apperror.FieldError

	if req.TempMinC != nil && req.TempMaxC != nil && *req.TempMinC > *req.TempMaxC {
		details = append(details, apperror.FieldError{
			Field: "temp_min_c", Message: "Minimum temperature cannot exceed the maximum",
		})
	}
	if req.RequiresRefrigeration && req.TempMinC == nil && req.TempMaxC == nil {
		details = append(details, apperror.FieldError{
			Field: "temp_min_c", Message: "State the temperature range this cargo needs",
		})
	}
	if req.DeliveryFrom != nil && req.DeliveryFrom.Before(req.PickupFrom) {
		details = append(details, apperror.FieldError{
			Field: "delivery_from", Message: "Delivery cannot start before pickup",
		})
	}
	if req.DeliveryTo != nil && req.DeliveryFrom != nil && req.DeliveryTo.Before(*req.DeliveryFrom) {
		details = append(details, apperror.FieldError{
			Field: "delivery_to", Message: "The delivery window ends before it starts",
		})
	}
	if req.RequiredCapacityKg != nil && *req.RequiredCapacityKg < req.WeightKg {
		details = append(details, apperror.FieldError{
			Field: "required_capacity_kg", Message: "Required capacity is below the cargo weight",
		})
	}
	if req.PaymentTerms == string(domain.PayPrepayment) &&
		(req.PrepaymentPercent == nil || *req.PrepaymentPercent == 0) {
		details = append(details, apperror.FieldError{
			Field: "prepayment_percent", Message: "State the prepayment percentage",
		})
	}
	if req.PaymentTerms == string(domain.PayDeferred) &&
		(req.DeferredDays == nil || *req.DeferredDays == 0) {
		details = append(details, apperror.FieldError{
			Field: "deferred_days", Message: "State how many days payment is deferred",
		})
	}

	if len(details) > 0 {
		return apperror.ErrValidation.WithDetails(details...)
	}
	return nil
}

func applyLoadRequest(load *domain.Load, req dto.SaveLoadRequest) {
	load.Title = strings.TrimSpace(req.Title)
	load.Description = nilIfBlank(req.Description)
	load.CargoTypeID = req.CargoTypeID
	load.WeightKg = req.WeightKg
	load.VolumeM3 = req.VolumeM3
	load.LengthCm = req.LengthCm
	load.WidthCm = req.WidthCm
	load.HeightCm = req.HeightCm
	load.Quantity = req.Quantity
	load.PickupFrom = req.PickupFrom
	load.PickupTo = req.PickupTo
	load.DeliveryFrom = req.DeliveryFrom
	load.DeliveryTo = req.DeliveryTo
	load.DistanceKm = req.DistanceKm
	load.RequiredCapacityKg = req.RequiredCapacityKg
	load.RequiresADR = req.RequiresADR
	load.RequiresRefrigeration = req.RequiresRefrigeration
	load.RequiresTailLift = req.RequiresTailLift
	load.SpecialRequirements = nilIfBlank(req.SpecialRequirements)
	load.PriceAmount = req.PriceAmount
	load.IsUrgent = req.IsUrgent

	load.Packaging = firstNonEmpty(req.Packaging, "other")
	load.LoadingType = firstNonEmpty(req.LoadingType, "rear")
	load.PriceCurrency = firstNonEmpty(req.PriceCurrency, domain.CurrencyUZS)
	load.PriceType = domain.PriceType(firstNonEmpty(req.PriceType, string(domain.PriceFixed)))
	load.PaymentTerms = domain.PaymentTerms(firstNonEmpty(req.PaymentTerms, string(domain.PayOnDelivery)))
	load.PaymentMethod = firstNonEmpty(req.PaymentMethod, "cash")

	if req.RequiredVehicleType != "" {
		vt := domain.VehicleType(req.RequiredVehicleType)
		load.RequiredVehicleType = &vt
	} else {
		load.RequiredVehicleType = nil
	}
	load.TempMinC = int16Ptr(req.TempMinC)
	load.TempMaxC = int16Ptr(req.TempMaxC)
	load.PrepaymentPercent = int16Ptr(req.PrepaymentPercent)
	load.DeferredDays = int16Ptr(req.DeferredDays)
}

// loadFingerprint normalises the fields that identify a shipment so a
// re-posted duplicate is recognisable. Coordinates are rounded to ~1 km and the
// pickup date to the day, because a genuine repost varies in the noise.
func loadFingerprint(load *domain.Load) *string {
	raw := fmt.Sprintf("%s|%.2f,%.2f|%.2f,%.2f|%d|%s|%d",
		load.PickupFrom.UTC().Format("2006-01-02"),
		load.PickupLocation.Lat, load.PickupLocation.Lng,
		load.DeliveryLocation.Lat, load.DeliveryLocation.Lng,
		load.WeightKg,
		strings.ToLower(strings.TrimSpace(load.Title)),
		load.PriceAmount,
	)
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])
	return &hash
}

func savedSearchResponse(s *domain.SavedSearch) *dto.SavedSearchResponse {
	filters := map[string]any(s.Filters)
	if filters == nil {
		filters = map[string]any{}
	}
	return &dto.SavedSearchResponse{
		ID:             s.ID,
		Name:           s.Name,
		Filters:        filters,
		NotifyEnabled:  s.NotifyEnabled,
		LastNotifiedAt: s.LastNotifiedAt,
		CreatedAt:      s.CreatedAt,
	}
}

func parseUUIDs(raw []string) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		if id, err := uuid.Parse(s); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func int16Ptr(v *int) *int16 {
	if v == nil {
		return nil
	}
	out := int16(*v)
	return &out
}
