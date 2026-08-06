package usecase

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/geo"
	"github.com/loadify/backend/pkg/logger"
)

// DriverUsecase owns driver profiles, documents, vehicles and availability.
type DriverUsecase struct {
	drivers   domain.DriverRepository
	docs      domain.DriverDocumentRepository
	vehicles  domain.VehicleRepository
	prefs     domain.DriverPreferenceRepository
	users     domain.UserRepository
	geo       domain.GeoRepository
	locations domain.LocationRepository
	files     *FileUsecase
	tx        domain.TxManager
	audit     domain.AuditLogger
	cache     cache.Cache
	log       logger.Logger
}

// NewDriverUsecase wires the driver usecase.
func NewDriverUsecase(
	drivers domain.DriverRepository,
	docs domain.DriverDocumentRepository,
	vehicles domain.VehicleRepository,
	prefs domain.DriverPreferenceRepository,
	users domain.UserRepository,
	geoRepo domain.GeoRepository,
	locations domain.LocationRepository,
	files *FileUsecase,
	tx domain.TxManager,
	audit domain.AuditLogger,
	c cache.Cache,
	log logger.Logger,
) *DriverUsecase {
	return &DriverUsecase{
		drivers: drivers, docs: docs, vehicles: vehicles, prefs: prefs,
		users: users, geo: geoRepo, locations: locations, files: files,
		tx: tx, audit: audit, cache: c, log: log,
	}
}

// SaveProfile creates or updates the caller's driver profile.
func (u *DriverUsecase) SaveProfile(ctx context.Context, userID uuid.UUID, req dto.CreateDriverProfileRequest) (*dto.DriverResponse, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	isNew := false
	if err != nil {
		if !apperror.Is(err, apperror.ErrNotFound.Code) {
			return nil, err
		}
		isNew = true
		driver = &domain.Driver{ID: uuid.New(), UserID: userID}
	}

	driver.LicenseNumber = nilIfBlank(req.LicenseNumber)
	driver.LicenseIssuedAt = req.LicenseIssuedAt
	driver.LicenseExpiresAt = req.LicenseExpiresAt
	driver.ExperienceYears = int16(req.ExperienceYears)
	driver.HomeCityID = req.HomeCityID
	driver.Bio = nilIfBlank(req.Bio)
	driver.LicenseCategories = domain.StringArray(req.LicenseCategories)
	if driver.LicenseCategories == nil {
		driver.LicenseCategories = domain.StringArray{}
	}
	driver.Languages = domain.StringArray(req.Languages)
	if len(driver.Languages) == 0 {
		driver.Languages = domain.StringArray{"uz"}
	}

	if req.HomeCityID != nil {
		if _, err := u.geo.FindCityByID(ctx, *req.HomeCityID); err != nil {
			return nil, apperror.ErrValidation.WithMessage("Unknown city").
				WithDetails(apperror.FieldError{Field: "home_city_id", Message: "City not found"})
		}
	}

	if isNew {
		if err := u.drivers.Create(ctx, driver); err != nil {
			return nil, err
		}
	} else {
		columns := map[string]any{
			"license_number":     driver.LicenseNumber,
			"license_issued_at":  driver.LicenseIssuedAt,
			"license_expires_at": driver.LicenseExpiresAt,
			"license_categories": driver.LicenseCategories,
			"experience_years":   driver.ExperienceYears,
			"home_city_id":       driver.HomeCityID,
			"languages":          driver.Languages,
			"bio":                driver.Bio,
		}
		if err := u.drivers.UpdateColumns(ctx, driver.ID, columns); err != nil {
			return nil, err
		}
	}

	action := "driver.profile_updated"
	if isNew {
		action = "driver.profile_created"
		// The driver id becomes part of the access token, so the cached claims
		// must be dropped for the change to take effect on the next refresh.
		u.invalidateUser(ctx, userID)
	}
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       action,
		ResourceType: "driver",
		ResourceID:   &driver.ID,
	})

	return u.GetProfile(ctx, driver.ID, true)
}

// GetProfile returns a driver profile. Contact details are included only for
// privileged viewers — a public search result must not leak phone numbers.
func (u *DriverUsecase) GetProfile(ctx context.Context, driverID uuid.UUID, includePrivate bool) (*dto.DriverResponse, error) {
	driver, err := u.drivers.FindWithRelations(ctx, driverID)
	if err != nil {
		return nil, err
	}
	return u.toResponse(ctx, driver, includePrivate, nil), nil
}

// GetMyProfile returns the caller's own driver profile.
func (u *DriverUsecase) GetMyProfile(ctx context.Context, userID uuid.UUID) (*dto.DriverResponse, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return u.GetProfile(ctx, driver.ID, true)
}

// SetStatus toggles online/offline/busy.
//
// Busy is owned by the trip lifecycle, not the driver: allowing a manual switch
// out of busy would let a driver appear available mid-trip.
func (u *DriverUsecase) SetStatus(ctx context.Context, userID uuid.UUID, req dto.UpdateDriverStatusRequest) error {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return err
	}
	target := domain.DriverStatus(req.Status)

	if target == domain.DriverStatusBusy {
		return apperror.ErrValidation.WithMessage("Busy is set automatically when a trip starts")
	}
	if driver.Status == domain.DriverStatusBusy {
		return apperror.ErrInvalidStatusChange.WithMessage("Finish your current trip before changing status")
	}
	if target == domain.DriverStatusOnline && !driver.IsVerified() {
		return apperror.ErrDriverNotVerified
	}
	return u.drivers.SetStatus(ctx, driver.ID, target)
}

// UpdateLocation records a position report submitted over REST.
func (u *DriverUsecase) UpdateLocation(ctx context.Context, userID uuid.UUID, req dto.UpdateDriverLocationRequest) error {
	point := geo.NewPoint(req.Lat, req.Lng)
	if !point.Valid() {
		return apperror.ErrValidation.WithMessage("Invalid coordinates")
	}

	recordedAt := time.Now().UTC()
	if req.RecordedAt != nil {
		recordedAt = req.RecordedAt.UTC()
	}

	var battery *int16
	if req.BatteryLevel != nil {
		b := int16(*req.BatteryLevel)
		battery = &b
	}

	return u.RecordLocation(ctx, userID, domain.DriverLocation{
		TripID:       req.TripID,
		Location:     point,
		SpeedKmh:     req.SpeedKmh,
		Heading:      req.Heading,
		AccuracyM:    req.AccuracyM,
		AltitudeM:    req.AltitudeM,
		BatteryLevel: battery,
		IsMoving:     req.SpeedKmh != nil && *req.SpeedKmh > 1,
		RecordedAt:   recordedAt,
	})
}

// RecordLocation stores one GPS sample and refreshes the driver's live marker.
//
// This is the shared entry point for both the REST endpoint and the WebSocket
// channel, so the same validation and caching apply to a driver reporting every
// 15 seconds over a socket and one posting a single fix over HTTP. The caller's
// driver id is resolved here rather than trusted from the payload.
func (u *DriverUsecase) RecordLocation(ctx context.Context, userID uuid.UUID, sample domain.DriverLocation) error {
	if !sample.Location.Valid() {
		return apperror.ErrValidation.WithMessage("Invalid coordinates")
	}

	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return err
	}
	sample.DriverID = driver.ID

	now := time.Now().UTC()
	if sample.RecordedAt.IsZero() {
		sample.RecordedAt = now
	}
	// Accept back-dated points from an offline queue, but never a future
	// timestamp — a device with a wrong clock would then block every subsequent
	// update, because UpdateLocation refuses to move the marker backwards.
	if sample.RecordedAt.After(now.Add(time.Minute)) {
		return apperror.ErrValidation.WithMessage("recorded_at is in the future")
	}

	if err := u.locations.InsertBatch(ctx, []domain.DriverLocation{sample}); err != nil {
		return err
	}
	if err := u.drivers.UpdateLocation(ctx, driver.ID, sample.Location, sample.RecordedAt); err != nil {
		return err
	}

	// Cache the latest fix so map queries never touch the partitioned table.
	if err := u.cache.Set(ctx, cache.DriverLocationKey(driver.ID),
		dto.PointResponse{Lat: sample.Location.Lat, Lng: sample.Location.Lng},
		10*time.Minute); err != nil {
		u.log.Warn("caching driver location failed", logger.Err(err))
	}
	return nil
}

// Search finds drivers matching the filter, ordered by distance when a point is given.
func (u *DriverUsecase) Search(ctx context.Context, q dto.DriverSearchQuery, offset, limit int, order string, includePrivate bool) ([]dto.DriverResponse, int64, error) {
	filter := domain.DriverSearchFilter{
		MinCapacityKg:  q.MinCapacityKg,
		MinVolumeM3:    q.MinVolumeM3,
		RequiresADR:    q.RequiresADR,
		RequiresRefrig: q.RequiresRefrig,
		MinRating:      q.MinRating,
		AvailableFrom:  q.AvailableFrom,
	}
	if q.Lat != nil && q.Lng != nil {
		point := geo.NewPoint(*q.Lat, *q.Lng)
		if !point.Valid() {
			return nil, 0, apperror.ErrValidation.WithMessage("Invalid search coordinates")
		}
		filter.Near = &point
		filter.RadiusKm = q.RadiusKm
		if filter.RadiusKm == 0 {
			filter.RadiusKm = 100
		}
	}
	for _, t := range q.VehicleTypes {
		vt := domain.VehicleType(t)
		if slices.Contains(domain.AllVehicleTypes, vt) {
			filter.VehicleTypes = append(filter.VehicleTypes, vt)
		}
	}
	for _, raw := range q.RegionIDs {
		if id, err := uuid.Parse(raw); err == nil {
			filter.RegionIDs = append(filter.RegionIDs, id)
		}
	}
	if q.OnlineOnly {
		filter.Statuses = []domain.DriverStatus{domain.DriverStatusOnline}
	} else {
		filter.Statuses = []domain.DriverStatus{domain.DriverStatusOnline, domain.DriverStatusBusy, domain.DriverStatusOffline}
	}

	results, total, err := u.drivers.Search(ctx, filter, offset, limit, order)
	if err != nil {
		return nil, 0, err
	}

	out := make([]dto.DriverResponse, 0, len(results))
	for i := range results {
		driver := results[i].Driver
		out = append(out, *u.toResponse(ctx, &driver, includePrivate, results[i].DistanceKm))
	}
	return out, total, nil
}

// ─── documents ──────────────────────────────────────────────────────

// UploadDocument attaches a file as a driver document and returns it to the
// review queue.
func (u *DriverUsecase) UploadDocument(ctx context.Context, userID uuid.UUID, req dto.UploadDocumentRequest) (*dto.DocumentResponse, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := u.files.AssertOwnership(ctx, userID, req.FileID); err != nil {
		return nil, err
	}

	side := req.Side
	if side == "" {
		side = "single"
	}
	doc := &domain.DriverDocument{
		ID:        uuid.New(),
		DriverID:  driver.ID,
		Type:      domain.DocumentType(req.Type),
		Side:      side,
		FileID:    req.FileID,
		Number:    nilIfBlank(req.Number),
		IssuedAt:  req.IssuedAt,
		ExpiresAt: req.ExpiresAt,
		Status:    domain.VerificationPending,
	}
	if err := u.docs.Upsert(ctx, doc); err != nil {
		return nil, err
	}

	// Replacing a document re-opens verification for the whole profile.
	if driver.VerificationStatus != domain.VerificationPending {
		if err := u.drivers.UpdateColumns(ctx, driver.ID, map[string]any{
			"verification_status": domain.VerificationPending,
			"rejection_reason":    nil,
		}); err != nil {
			u.log.Warn("resetting driver verification failed", logger.Err(err))
		}
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "driver.document_uploaded",
		ResourceType: "driver_document",
		ResourceID:   &doc.ID,
		NewValues:    map[string]any{"type": req.Type, "side": side},
	})

	stored, err := u.docs.FindByID(ctx, doc.ID)
	if err != nil {
		return nil, err
	}
	return u.documentResponse(ctx, stored), nil
}

// ListDocuments returns the caller's documents.
func (u *DriverUsecase) ListDocuments(ctx context.Context, userID uuid.UUID) ([]dto.DocumentResponse, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	docs, err := u.docs.ListByDriver(ctx, driver.ID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DocumentResponse, 0, len(docs))
	for i := range docs {
		out = append(out, *u.documentResponse(ctx, &docs[i]))
	}
	return out, nil
}

// ReviewDocument records an admin decision and promotes the driver to verified
// once every required document is approved.
func (u *DriverUsecase) ReviewDocument(ctx context.Context, adminID uuid.UUID, docID uuid.UUID, req dto.ReviewDocumentRequest) error {
	doc, err := u.docs.FindByID(ctx, docID)
	if err != nil {
		return err
	}
	status := domain.VerificationStatus(req.Status)
	reason := nilIfBlank(req.Reason)

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.docs.Review(txCtx, docID, status, reason, adminID); err != nil {
			return err
		}
		if status == domain.VerificationRejected {
			return u.drivers.UpdateColumns(txCtx, doc.DriverID, map[string]any{
				"verification_status": domain.VerificationRejected,
				"rejection_reason":    reason,
			})
		}
		return u.promoteIfComplete(txCtx, doc.DriverID, adminID)
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "driver.document_reviewed",
		ResourceType: "driver_document",
		ResourceID:   &docID,
		NewValues:    map[string]any{"status": req.Status, "reason": req.Reason},
	})
	return nil
}

// promoteIfComplete marks a driver verified once every required document is
// approved and at least one vehicle is approved. Called inside the review
// transaction so the check and the promotion cannot race.
func (u *DriverUsecase) promoteIfComplete(ctx context.Context, driverID, adminID uuid.UUID) error {
	approvedDocs, err := u.docs.CountApprovedByTypes(ctx, driverID, domain.RequiredDriverDocuments)
	if err != nil {
		return err
	}
	if int(approvedDocs) < len(domain.RequiredDriverDocuments) {
		return nil
	}
	approvedVehicles, err := u.vehicles.CountApproved(ctx, driverID)
	if err != nil {
		return err
	}
	if approvedVehicles == 0 {
		return nil
	}

	driver, err := u.drivers.FindByID(ctx, driverID)
	if err != nil {
		return err
	}
	if driver.IsVerified() {
		return nil
	}
	return u.drivers.UpdateColumns(ctx, driverID, map[string]any{
		"verification_status": domain.VerificationVerified,
		"verified_at":         time.Now().UTC(),
		"verified_by":         adminID,
		"rejection_reason":    nil,
	})
}

// ListPendingDocuments serves the admin verification queue.
func (u *DriverUsecase) ListPendingDocuments(ctx context.Context, offset, limit int) ([]dto.DocumentResponse, int64, error) {
	docs, total, err := u.docs.ListPending(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.DocumentResponse, 0, len(docs))
	for i := range docs {
		out = append(out, *u.documentResponse(ctx, &docs[i]))
	}
	return out, total, nil
}

// ─── vehicles ───────────────────────────────────────────────────────

// AddVehicle registers a truck for the caller.
func (u *DriverUsecase) AddVehicle(ctx context.Context, userID uuid.UUID, req dto.SaveVehicleRequest) (*dto.VehicleResponse, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := validateVehicleRequest(req); err != nil {
		return nil, err
	}

	existing, err := u.vehicles.ListByDriver(ctx, driver.ID)
	if err != nil {
		return nil, err
	}

	vehicle := &domain.Vehicle{ID: uuid.New(), DriverID: driver.ID, CompanyID: driver.CompanyID}
	applyVehicleRequest(vehicle, req)
	vehicle.Status = domain.VerificationPending
	vehicle.IsPrimary = len(existing) == 0

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.vehicles.Create(txCtx, vehicle); err != nil {
			return err
		}
		return u.saveVehicleChildren(txCtx, userID, vehicle.ID, req)
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "vehicle.created",
		ResourceType: "vehicle",
		ResourceID:   &vehicle.ID,
		NewValues:    map[string]any{"plate_number": vehicle.PlateNumber, "type": vehicle.Type},
	})

	stored, err := u.vehicles.FindByID(ctx, vehicle.ID)
	if err != nil {
		return nil, err
	}
	return u.vehicleResponse(ctx, stored), nil
}

// UpdateVehicle edits a truck the caller owns and sends it back for review.
func (u *DriverUsecase) UpdateVehicle(ctx context.Context, userID, vehicleID uuid.UUID, req dto.SaveVehicleRequest) (*dto.VehicleResponse, error) {
	vehicle, err := u.assertOwnsVehicle(ctx, userID, vehicleID)
	if err != nil {
		return nil, err
	}
	if err := validateVehicleRequest(req); err != nil {
		return nil, err
	}

	applyVehicleRequest(vehicle, req)
	// Any change to the specification invalidates the previous approval.
	vehicle.Status = domain.VerificationPending
	vehicle.RejectionReason = nil

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.vehicles.Update(txCtx, vehicle); err != nil {
			return err
		}
		if err := u.vehicles.UpdateColumns(txCtx, vehicle.ID, map[string]any{
			"status":           domain.VerificationPending,
			"rejection_reason": nil,
		}); err != nil {
			return err
		}
		return u.saveVehicleChildren(txCtx, userID, vehicle.ID, req)
	})
	if err != nil {
		return nil, err
	}

	stored, err := u.vehicles.FindByID(ctx, vehicle.ID)
	if err != nil {
		return nil, err
	}
	return u.vehicleResponse(ctx, stored), nil
}

// ListVehicles returns the caller's trucks.
func (u *DriverUsecase) ListVehicles(ctx context.Context, userID uuid.UUID) ([]dto.VehicleResponse, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	vehicles, err := u.vehicles.ListByDriver(ctx, driver.ID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.VehicleResponse, 0, len(vehicles))
	for i := range vehicles {
		out = append(out, *u.vehicleResponse(ctx, &vehicles[i]))
	}
	return out, nil
}

// SetPrimaryVehicle marks which truck is used by default.
func (u *DriverUsecase) SetPrimaryVehicle(ctx context.Context, userID, vehicleID uuid.UUID) error {
	vehicle, err := u.assertOwnsVehicle(ctx, userID, vehicleID)
	if err != nil {
		return err
	}
	return u.vehicles.SetPrimary(ctx, vehicle.DriverID, vehicleID)
}

// DeleteVehicle removes a truck the caller owns.
func (u *DriverUsecase) DeleteVehicle(ctx context.Context, userID, vehicleID uuid.UUID) error {
	vehicle, err := u.assertOwnsVehicle(ctx, userID, vehicleID)
	if err != nil {
		return err
	}
	if err := u.vehicles.SoftDelete(ctx, vehicleID); err != nil {
		return err
	}
	// Deleting the primary truck must leave the driver with a primary, or the
	// booking flow has nothing to default to.
	if vehicle.IsPrimary {
		remaining, err := u.vehicles.ListByDriver(ctx, vehicle.DriverID)
		if err == nil && len(remaining) > 0 {
			if err := u.vehicles.SetPrimary(ctx, vehicle.DriverID, remaining[0].ID); err != nil {
				u.log.Warn("reassigning primary vehicle failed", logger.Err(err))
			}
		}
	}
	return nil
}

// ReviewVehicle records an admin decision on a truck.
func (u *DriverUsecase) ReviewVehicle(ctx context.Context, adminID, vehicleID uuid.UUID, req dto.ReviewDocumentRequest) error {
	vehicle, err := u.vehicles.FindByID(ctx, vehicleID)
	if err != nil {
		return err
	}
	status := domain.VerificationStatus(req.Status)
	reason := nilIfBlank(req.Reason)

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.vehicles.Review(txCtx, vehicleID, status, reason); err != nil {
			return err
		}
		if status == domain.VerificationApproved {
			return u.promoteIfComplete(txCtx, vehicle.DriverID, adminID)
		}
		return nil
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "vehicle.reviewed",
		ResourceType: "vehicle",
		ResourceID:   &vehicleID,
		NewValues:    map[string]any{"status": req.Status, "reason": req.Reason},
	})
	return nil
}

// ListPendingVehicles serves the admin vehicle queue.
func (u *DriverUsecase) ListPendingVehicles(ctx context.Context, offset, limit int) ([]dto.VehicleResponse, int64, error) {
	vehicles, total, err := u.vehicles.ListPending(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.VehicleResponse, 0, len(vehicles))
	for i := range vehicles {
		out = append(out, *u.vehicleResponse(ctx, &vehicles[i]))
	}
	return out, total, nil
}

// ─── availability & preferences ─────────────────────────────────────

// PublishAvailability advertises free capacity.
func (u *DriverUsecase) PublishAvailability(ctx context.Context, userID uuid.UUID, req dto.PublishAvailabilityRequest) (*dto.AvailabilityResponse, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !driver.IsVerified() {
		return nil, apperror.ErrDriverNotVerified
	}
	if req.VehicleID != nil {
		if _, err := u.assertOwnsVehicle(ctx, userID, *req.VehicleID); err != nil {
			return nil, err
		}
	}

	item := &domain.DriverAvailability{
		ID:                 uuid.New(),
		DriverID:           driver.ID,
		VehicleID:          req.VehicleID,
		AvailableFrom:      req.AvailableFrom,
		AvailableTo:        req.AvailableTo,
		PreferredRegionIDs: domain.UUIDArray(req.PreferredRegionIDs),
		MaxDistanceKm:      req.MaxDistanceKm,
		MinPriceAmount:     req.MinPriceAmount,
		Currency:           domain.CurrencyUZS,
		Notes:              nilIfBlank(req.Notes),
		IsActive:           true,
	}
	if item.PreferredRegionIDs == nil {
		item.PreferredRegionIDs = domain.UUIDArray{}
	}
	if req.Currency != "" {
		item.Currency = req.Currency
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if req.FromAddress != nil {
			address, err := u.createAddress(txCtx, *req.FromAddress)
			if err != nil {
				return err
			}
			item.FromAddressID = &address.ID
		}
		return u.prefs.CreateAvailability(txCtx, item)
	})
	if err != nil {
		return nil, err
	}

	stored, err := u.prefs.FindAvailabilityByID(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	return u.availabilityResponse(ctx, stored), nil
}

// ListAvailability returns the caller's published windows.
func (u *DriverUsecase) ListAvailability(ctx context.Context, userID uuid.UUID, activeOnly bool) ([]dto.AvailabilityResponse, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	items, err := u.prefs.ListAvailability(ctx, driver.ID, activeOnly)
	if err != nil {
		return nil, err
	}
	out := make([]dto.AvailabilityResponse, 0, len(items))
	for i := range items {
		out = append(out, *u.availabilityResponse(ctx, &items[i]))
	}
	return out, nil
}

// WithdrawAvailability deactivates one window.
func (u *DriverUsecase) WithdrawAvailability(ctx context.Context, userID, availabilityID uuid.UUID) error {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return err
	}
	item, err := u.prefs.FindAvailabilityByID(ctx, availabilityID)
	if err != nil {
		return err
	}
	if item.DriverID != driver.ID {
		return apperror.ErrForbidden
	}
	return u.prefs.DeactivateAvailability(ctx, availabilityID)
}

// GetPreferences returns the caller's matching preferences.
func (u *DriverUsecase) GetPreferences(ctx context.Context, userID uuid.UUID) (*dto.PreferencesResponse, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	prefs, err := u.prefs.GetPreferences(ctx, driver.ID)
	if err != nil {
		return nil, err
	}
	return &dto.PreferencesResponse{
		MaxDistanceKm:       prefs.MaxDistanceKm,
		PreferredCargoTypes: prefs.PreferredCargoTypes,
		PreferredRegionIDs:  prefs.PreferredRegionIDs,
		MinPriceAmount:      prefs.MinPriceAmount,
		NotifyNewLoads:      prefs.NotifyNewLoads,
		NotifyRadiusKm:      prefs.NotifyRadiusKm,
		AutoAcceptOffers:    prefs.AutoAcceptOffers,
	}, nil
}

// SavePreferences stores the caller's matching preferences.
func (u *DriverUsecase) SavePreferences(ctx context.Context, userID uuid.UUID, req dto.SavePreferencesRequest) (*dto.PreferencesResponse, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	prefs := &domain.DriverPreferences{
		DriverID:            driver.ID,
		MaxDistanceKm:       req.MaxDistanceKm,
		PreferredCargoTypes: domain.UUIDArray(req.PreferredCargoTypes),
		PreferredRegionIDs:  domain.UUIDArray(req.PreferredRegionIDs),
		MinPriceAmount:      req.MinPriceAmount,
		NotifyNewLoads:      req.NotifyNewLoads,
		NotifyRadiusKm:      req.NotifyRadiusKm,
		AutoAcceptOffers:    req.AutoAcceptOffers,
	}
	if prefs.PreferredCargoTypes == nil {
		prefs.PreferredCargoTypes = domain.UUIDArray{}
	}
	if prefs.PreferredRegionIDs == nil {
		prefs.PreferredRegionIDs = domain.UUIDArray{}
	}
	if err := u.prefs.SavePreferences(ctx, prefs); err != nil {
		return nil, err
	}
	return u.GetPreferences(ctx, userID)
}

// ─── internals ──────────────────────────────────────────────────────

func (u *DriverUsecase) assertOwnsVehicle(ctx context.Context, userID, vehicleID uuid.UUID) (*domain.Vehicle, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	vehicle, err := u.vehicles.FindByID(ctx, vehicleID)
	if err != nil {
		return nil, err
	}
	if vehicle.DriverID != driver.ID {
		return nil, apperror.ErrForbidden
	}
	return vehicle, nil
}

func (u *DriverUsecase) saveVehicleChildren(ctx context.Context, userID, vehicleID uuid.UUID, req dto.SaveVehicleRequest) error {
	for i, fileID := range req.PhotoFileIDs {
		if err := u.files.AssertOwnership(ctx, userID, fileID); err != nil {
			return err
		}
		photo := &domain.VehiclePhoto{
			ID:        uuid.New(),
			VehicleID: vehicleID,
			FileID:    fileID,
			SortOrder: int16(i),
		}
		if err := u.vehicles.AddPhoto(ctx, photo); err != nil {
			return err
		}
	}
	if req.Trailer != nil {
		trailer := &domain.Trailer{
			ID:          uuid.New(),
			VehicleID:   vehicleID,
			Type:        req.Trailer.Type,
			PlateNumber: req.Trailer.PlateNumber,
			Make:        nilIfBlank(req.Trailer.Make),
			Model:       nilIfBlank(req.Trailer.Model),
			CapacityKg:  req.Trailer.CapacityKg,
			VolumeM3:    req.Trailer.VolumeM3,
			LengthCm:    req.Trailer.LengthCm,
			WidthCm:     req.Trailer.WidthCm,
			HeightCm:    req.Trailer.HeightCm,
		}
		if req.Trailer.Year != nil {
			y := int16(*req.Trailer.Year)
			trailer.Year = &y
		}
		if req.Trailer.Axles != nil {
			a := int16(*req.Trailer.Axles)
			trailer.Axles = &a
		}
		if err := u.vehicles.UpsertTrailer(ctx, trailer); err != nil {
			return err
		}
	}
	return nil
}

func (u *DriverUsecase) createAddress(ctx context.Context, req dto.AddressRequest) (*domain.Address, error) {
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

func (u *DriverUsecase) invalidateUser(ctx context.Context, userID uuid.UUID) {
	if err := u.cache.Delete(ctx, cache.UserKey(userID), cache.UserPermissionsKey(userID)); err != nil {
		u.log.Warn("user cache invalidation failed", logger.Err(err))
	}
}

func validateVehicleRequest(req dto.SaveVehicleRequest) error {
	if req.TempMinC != nil && req.TempMaxC != nil && *req.TempMinC > *req.TempMaxC {
		return apperror.ErrValidation.WithDetails(apperror.FieldError{
			Field:   "temp_min_c",
			Message: "Minimum temperature cannot exceed the maximum",
		})
	}
	if req.HasRefrigeration && req.TempMinC == nil && req.TempMaxC == nil {
		return apperror.ErrValidation.WithDetails(apperror.FieldError{
			Field:   "temp_min_c",
			Message: "State the temperature range a refrigerated unit can hold",
		})
	}
	return nil
}

func applyVehicleRequest(v *domain.Vehicle, req dto.SaveVehicleRequest) {
	v.Type = domain.VehicleType(req.Type)
	v.Make = req.Make
	v.Model = req.Model
	v.PlateNumber = normalisePlate(req.PlateNumber)
	v.VIN = nilIfBlank(req.VIN)
	v.Color = nilIfBlank(req.Color)
	v.CapacityKg = req.CapacityKg
	v.VolumeM3 = req.VolumeM3
	v.LengthCm = req.LengthCm
	v.WidthCm = req.WidthCm
	v.HeightCm = req.HeightCm
	v.HasADR = req.HasADR
	v.HasRefrigeration = req.HasRefrigeration
	v.HasTailLift = req.HasTailLift
	v.HasGPS = req.HasGPS
	v.InsuranceExpiresAt = req.InsuranceExpiresAt
	v.TechInspectionExpiresAt = req.TechInspectionExpiresAt

	v.FuelType = "diesel"
	if req.FuelType != "" {
		v.FuelType = req.FuelType
	}
	if req.Year != nil {
		y := int16(*req.Year)
		v.Year = &y
	}
	if req.Axles != nil {
		a := int16(*req.Axles)
		v.Axles = &a
	}
	if req.TempMinC != nil {
		t := int16(*req.TempMinC)
		v.TempMinC = &t
	}
	if req.TempMaxC != nil {
		t := int16(*req.TempMaxC)
		v.TempMaxC = &t
	}
}

// normalisePlate uppercases and strips separators so "01 A 123 BC" and
// "01a123bc" collide on the unique index instead of registering twice.
func normalisePlate(plate string) string {
	var b []rune
	for _, r := range plate {
		switch {
		case r >= 'a' && r <= 'z':
			b = append(b, r-32)
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b = append(b, r)
		}
	}
	return string(b)
}
