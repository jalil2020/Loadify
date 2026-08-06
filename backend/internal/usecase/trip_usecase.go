package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/geo"
	"github.com/loadify/backend/pkg/logger"
)

// autoConfirmAfter is how long a delivered trip waits for the shipper to
// confirm before the platform closes it. Without this a driver could be left
// unpaid because nobody pressed a button.
const autoConfirmAfter = 72 * time.Hour

// TripUsecase owns the physical execution of a shipment.
type TripUsecase struct {
	trips     domain.TripRepository
	bookings  domain.BookingRepository
	loads     domain.LoadRepository
	drivers   domain.DriverRepository
	companies domain.CompanyRepository
	ratings   domain.RatingRepository
	locations domain.LocationRepository
	files     *FileUsecase
	settings  domain.SettingsReader
	tx        domain.TxManager
	audit     domain.AuditLogger
	events    domain.EventPublisher
	notifier  domain.Notifier
	log       logger.Logger
}

// NewTripUsecase wires the trip usecase.
func NewTripUsecase(
	trips domain.TripRepository,
	bookings domain.BookingRepository,
	loads domain.LoadRepository,
	drivers domain.DriverRepository,
	companies domain.CompanyRepository,
	ratings domain.RatingRepository,
	locations domain.LocationRepository,
	files *FileUsecase,
	settings domain.SettingsReader,
	tx domain.TxManager,
	audit domain.AuditLogger,
	events domain.EventPublisher,
	notifier domain.Notifier,
	log logger.Logger,
) *TripUsecase {
	return &TripUsecase{
		trips: trips, bookings: bookings, loads: loads, drivers: drivers,
		companies: companies, ratings: ratings, locations: locations,
		files: files, settings: settings, tx: tx, audit: audit,
		events: events, notifier: notifier, log: log,
	}
}

// Advance moves a trip forward. Only the driver may drive these transitions:
// they are statements about the physical world that only the driver observes.
func (u *TripUsecase) Advance(ctx context.Context, userID, tripID uuid.UUID, req dto.AdvanceTripRequest) (*dto.TripResponse, error) {
	trip, err := u.trips.FindByID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if trip.DriverID != driver.ID {
		return nil, apperror.ErrForbidden.WithMessage("This is not your trip")
	}

	target := domain.TripStatus(req.Status)
	if !target.IsDriverAdvanceable() {
		return nil, apperror.ErrForbidden.WithMessage(
			fmt.Sprintf("A driver cannot set the status '%s'", target))
	}
	if !trip.Status.CanTransitionTo(target) {
		return nil, apperror.ErrInvalidStatusChange.WithMessage(
			fmt.Sprintf("Cannot go from '%s' to '%s'", trip.Status, target))
	}
	for _, fileID := range req.DocumentFileIDs {
		if err := u.files.AssertOwnership(ctx, userID, fileID); err != nil {
			return nil, err
		}
	}

	var point *geo.Point
	if req.Location != nil {
		p := geo.NewPoint(req.Location.Lat, req.Location.Lng)
		if !p.Valid() {
			return nil, apperror.ErrValidation.WithMessage("Invalid coordinates")
		}
		point = &p
	}

	columns := map[string]any{}
	if req.OdometerKm != nil && target == domain.TripDelivered {
		columns["distance_actual_km"] = *req.OdometerKm
	}

	previous := trip.Status
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.trips.TransitionStatus(txCtx, tripID,
			[]domain.TripStatus{previous}, target, columns); err != nil {
			return err
		}
		if err := u.trips.AddEvent(txCtx, &domain.TripEvent{
			ID:          uuid.New(),
			TripID:      tripID,
			Type:        domain.EventStatusChange,
			FromStatus:  strPtr(string(previous)),
			ToStatus:    strPtr(string(target)),
			Location:    point,
			ActorUserID: &userID,
			Payload:     eventPayload(req.Note, req.OdometerKm),
		}); err != nil {
			return err
		}

		for _, fileID := range req.DocumentFileIDs {
			docType := req.DocumentType
			if docType == "" {
				docType = defaultDocumentTypeFor(target)
			}
			if err := u.trips.AddDocument(txCtx, &domain.TripDocument{
				ID: uuid.New(), TripID: tripID, FileID: fileID,
				Type: docType, UploadedBy: &userID,
			}); err != nil {
				return err
			}
		}

		// Starting a trip makes the driver busy; delivering frees them.
		switch target {
		case domain.TripEnRoute:
			if err := u.drivers.SetStatus(txCtx, driver.ID, domain.DriverStatusBusy); err != nil {
				return err
			}
		case domain.TripDelivered:
			if err := u.drivers.SetStatus(txCtx, driver.ID, domain.DriverStatusOnline); err != nil {
				return err
			}
		}

		// Keep the load status the shipper sees in step with the truck.
		if loadStatus := domain.LoadStatusForTrip(target); loadStatus != "" {
			current, err := u.loads.FindByID(txCtx, trip.LoadID)
			if err != nil {
				return err
			}
			if current.Status != loadStatus && current.Status.CanTransitionTo(loadStatus) {
				if err := u.loads.TransitionStatus(txCtx, trip.LoadID,
					[]domain.LoadStatus{current.Status}, loadStatus, nil); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "trip.status_changed",
		ResourceType: "trip",
		ResourceID:   &tripID,
		OldValues:    map[string]any{"status": previous},
		NewValues:    map[string]any{"status": target},
	})

	trip.Status = target
	u.announceStatus(ctx, trip, previous, target, point)

	if target == domain.TripDelivered {
		u.notifyCompanyMembers(ctx, trip.CompanyID, "Shipment delivered",
			"Confirm delivery to close the trip and release payment.",
			map[string]any{"trip_id": tripID, "load_id": trip.LoadID})
	}

	return u.Get(ctx, tripID, userID, false)
}

// Confirm is the shipper accepting the delivery, which closes the trip.
func (u *TripUsecase) Confirm(ctx context.Context, userID, companyID, tripID uuid.UUID) (*dto.TripResponse, error) {
	trip, err := u.trips.FindByID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if trip.CompanyID != companyID {
		return nil, apperror.ErrForbidden.WithMessage("This trip belongs to another company")
	}
	if trip.Status != domain.TripDelivered {
		return nil, apperror.ErrInvalidStatusChange.WithMessage(
			"Only a delivered trip can be confirmed")
	}
	if err := u.complete(ctx, trip, &userID); err != nil {
		return nil, err
	}
	return u.Get(ctx, tripID, userID, false)
}

// Cancel aborts a trip before the cargo is loaded.
func (u *TripUsecase) Cancel(ctx context.Context, userID, tripID uuid.UUID, req dto.CancelTripRequest, isAdmin bool) error {
	trip, err := u.trips.FindByID(ctx, tripID)
	if err != nil {
		return err
	}
	if !isAdmin {
		if err := u.assertParticipant(ctx, userID, trip); err != nil {
			return err
		}
	}
	if !trip.Status.CanTransitionTo(domain.TripCancelled) {
		return apperror.ErrInvalidStatusChange.WithMessage(
			"The cargo is already loaded; raise a dispute instead")
	}

	previous := trip.Status
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.trips.TransitionStatus(txCtx, tripID,
			[]domain.TripStatus{previous}, domain.TripCancelled,
			map[string]any{
				"cancelled_by_user_id": userID,
				"cancellation_reason":  req.Reason,
			}); err != nil {
			return err
		}
		if err := u.trips.AddEvent(txCtx, &domain.TripEvent{
			ID:          uuid.New(),
			TripID:      tripID,
			Type:        domain.EventStatusChange,
			FromStatus:  strPtr(string(previous)),
			ToStatus:    strPtr(string(domain.TripCancelled)),
			ActorUserID: &userID,
			Payload:     domain.JSONMap{"reason": req.Reason},
		}); err != nil {
			return err
		}
		if err := u.bookings.TransitionStatus(txCtx, trip.BookingID,
			[]domain.BookingStatus{domain.BookingAccepted}, domain.BookingCancelled,
			map[string]any{
				"cancelled_by_user_id": userID,
				"cancellation_reason":  req.Reason,
			}); err != nil {
			return err
		}
		if err := u.drivers.SetStatus(txCtx, trip.DriverID, domain.DriverStatusOnline); err != nil {
			return err
		}
		// Put the load back on the market rather than stranding it.
		current, err := u.loads.FindByID(txCtx, trip.LoadID)
		if err != nil {
			return err
		}
		if current.Status.CanTransitionTo(domain.LoadPublished) {
			return u.loads.TransitionStatus(txCtx, trip.LoadID,
				[]domain.LoadStatus{current.Status}, domain.LoadPublished,
				map[string]any{"published_at": time.Now().UTC()})
		}
		return nil
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "trip.cancelled",
		ResourceType: "trip",
		ResourceID:   &tripID,
		NewValues:    map[string]any{"reason": req.Reason},
	})
	trip.Status = domain.TripCancelled
	u.announceStatus(ctx, trip, previous, domain.TripCancelled, nil)
	return nil
}

// Dispute freezes a trip for admin resolution.
func (u *TripUsecase) Dispute(ctx context.Context, userID, tripID uuid.UUID, reason string) error {
	trip, err := u.trips.FindByID(ctx, tripID)
	if err != nil {
		return err
	}
	if err := u.assertParticipant(ctx, userID, trip); err != nil {
		return err
	}
	if !trip.Status.CanTransitionTo(domain.TripDisputed) {
		return apperror.ErrInvalidStatusChange.WithMessage(
			fmt.Sprintf("A trip in state '%s' cannot be disputed", trip.Status))
	}

	previous := trip.Status
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.trips.TransitionStatus(txCtx, tripID,
			[]domain.TripStatus{previous}, domain.TripDisputed, nil); err != nil {
			return err
		}
		return u.trips.AddEvent(txCtx, &domain.TripEvent{
			ID:          uuid.New(),
			TripID:      tripID,
			Type:        domain.EventIssue,
			FromStatus:  strPtr(string(previous)),
			ToStatus:    strPtr(string(domain.TripDisputed)),
			ActorUserID: &userID,
			Payload:     domain.JSONMap{"reason": reason},
		})
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "trip.disputed",
		ResourceType: "trip",
		ResourceID:   &tripID,
		NewValues:    map[string]any{"reason": reason},
	})
	trip.Status = domain.TripDisputed
	u.announceStatus(ctx, trip, previous, domain.TripDisputed, nil)
	return nil
}

// Get returns one trip the caller is party to.
func (u *TripUsecase) Get(ctx context.Context, tripID, userID uuid.UUID, isAdmin bool) (*dto.TripResponse, error) {
	trip, err := u.trips.FindWithRelations(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		if err := u.assertParticipant(ctx, userID, trip); err != nil {
			return nil, err
		}
	}

	res := u.toResponse(ctx, trip)

	// A trip is only rateable once, and only after it closes.
	if trip.Status == domain.TripCompleted {
		existing, err := u.ratings.FindByTripAndRater(ctx, tripID, userID)
		if err != nil {
			u.log.Warn("looking up existing rating failed", logger.Err(err))
		} else if existing != nil {
			res.MyRating = ratingResponse(existing, "")
		} else {
			res.CanRate = true
		}
	}
	return res, nil
}

// ListForDriver returns the caller driver's trips.
func (u *TripUsecase) ListForDriver(ctx context.Context, userID uuid.UUID, q dto.TripSearchQuery, offset, limit int, order string) ([]dto.TripResponse, int64, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	filter := buildTripFilter(q)
	filter.DriverID = &driver.ID
	return u.list(ctx, filter, offset, limit, order)
}

// ListForCompany returns the caller company's trips.
func (u *TripUsecase) ListForCompany(ctx context.Context, companyID uuid.UUID, q dto.TripSearchQuery, offset, limit int, order string) ([]dto.TripResponse, int64, error) {
	filter := buildTripFilter(q)
	filter.CompanyID = &companyID
	return u.list(ctx, filter, offset, limit, order)
}

// Track returns the live position and travelled route of a trip.
func (u *TripUsecase) Track(ctx context.Context, userID, tripID uuid.UUID, isAdmin bool) (*dto.TripTrackResponse, error) {
	trip, err := u.trips.FindByID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		if err := u.assertParticipant(ctx, userID, trip); err != nil {
			return nil, err
		}
	}

	res := &dto.TripTrackResponse{
		TripID: tripID,
		Status: string(trip.Status),
		ETAAt:  trip.ETAAt,
		Route:  []dto.PointResponse{},
	}

	points, err := u.locations.ListByTrip(ctx, tripID, 2000)
	if err != nil {
		return nil, err
	}
	var travelled float64
	for i := range points {
		res.Route = append(res.Route, dto.PointResponse{
			Lat: points[i].Location.Lat, Lng: points[i].Location.Lng,
		})
		if i > 0 {
			travelled += geo.DistanceKm(points[i-1].Location, points[i].Location)
		}
	}
	res.DistanceDoneKm = travelled

	if len(points) > 0 {
		last := points[len(points)-1]
		res.CurrentLocation = &dto.PointResponse{Lat: last.Location.Lat, Lng: last.Location.Lng}
		res.UpdatedAt = &last.RecordedAt
	} else if driver, err := u.drivers.FindByID(ctx, trip.DriverID); err == nil &&
		driver.CurrentLocation != nil && driver.CurrentLocation.Valid() {
		// No trace yet — fall back to the driver's last known position.
		res.CurrentLocation = &dto.PointResponse{
			Lat: driver.CurrentLocation.Lat, Lng: driver.CurrentLocation.Lng,
		}
		res.UpdatedAt = driver.LocationUpdatedAt
	}
	return res, nil
}

// CanWatchTrip authorises a client to subscribe to a trip's live position.
//
// Live location is the most sensitive data on the platform — it says where a
// person physically is — so the socket asks this before adding a watcher, and
// only the trip's two parties pass.
func (u *TripUsecase) CanWatchTrip(ctx context.Context, userID, tripID uuid.UUID) error {
	trip, err := u.trips.FindByID(ctx, tripID)
	if err != nil {
		return err
	}
	return u.assertParticipant(ctx, userID, trip)
}

// AddDocument attaches a paper to a trip.
func (u *TripUsecase) AddDocument(ctx context.Context, userID, tripID uuid.UUID, req dto.AddTripDocumentRequest) (*dto.TripDocumentResponse, error) {
	trip, err := u.trips.FindByID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if err := u.assertParticipant(ctx, userID, trip); err != nil {
		return nil, err
	}
	if err := u.files.AssertOwnership(ctx, userID, req.FileID); err != nil {
		return nil, err
	}

	doc := &domain.TripDocument{
		ID:         uuid.New(),
		TripID:     tripID,
		FileID:     req.FileID,
		Type:       req.Type,
		Name:       nilIfBlank(req.Name),
		UploadedBy: &userID,
	}
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.trips.AddDocument(txCtx, doc); err != nil {
			return err
		}
		return u.trips.AddEvent(txCtx, &domain.TripEvent{
			ID:          uuid.New(),
			TripID:      tripID,
			Type:        domain.EventDocument,
			ActorUserID: &userID,
			Payload:     domain.JSONMap{"document_type": req.Type},
		})
	})
	if err != nil {
		return nil, err
	}

	return &dto.TripDocumentResponse{
		ID:        doc.ID,
		FileID:    doc.FileID,
		Type:      doc.Type,
		Name:      doc.Name,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Rate scores the counterparty after a completed trip.
func (u *TripUsecase) Rate(ctx context.Context, userID, tripID uuid.UUID, req dto.RateTripRequest) (*dto.RatingResponse, error) {
	trip, err := u.trips.FindByID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if trip.Status != domain.TripCompleted {
		return nil, apperror.ErrInvalidStatusChange.WithMessage(
			"You can rate only after the trip is completed")
	}
	if err := u.assertParticipant(ctx, userID, trip); err != nil {
		return nil, err
	}
	if existing, err := u.ratings.FindByTripAndRater(ctx, tripID, userID); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, apperror.ErrConflict.WithMessage("You have already rated this trip")
	}

	driver, err := u.drivers.FindByID(ctx, trip.DriverID)
	if err != nil {
		return nil, err
	}
	company, err := u.companies.FindByID(ctx, trip.CompanyID)
	if err != nil {
		return nil, err
	}

	rating := &domain.Rating{
		ID:          uuid.New(),
		TripID:      tripID,
		RaterUserID: userID,
		Score:       int16(req.Score),
		Comment:     nilIfBlank(req.Comment),
		Criteria:    domain.JSONMap(req.Criteria),
	}
	// The rater's identity decides who is being rated: the driver rates the
	// company, the company rates the driver.
	if userID == driver.UserID {
		rating.RateeType = domain.RateeCompany
		rating.RateeUserID = company.OwnerUserID
		rating.CompanyID = &company.ID
	} else {
		rating.RateeType = domain.RateeDriver
		rating.RateeUserID = driver.UserID
		rating.DriverID = &driver.ID
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.ratings.Create(txCtx, rating); err != nil {
			return err
		}
		if rating.RateeType == domain.RateeDriver {
			return u.drivers.RecalculateRating(txCtx, driver.ID)
		}
		return u.companies.RecalculateRating(txCtx, company.ID)
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "trip.rated",
		ResourceType: "rating",
		ResourceID:   &rating.ID,
		NewValues:    map[string]any{"score": req.Score, "ratee_type": rating.RateeType},
	})
	return ratingResponse(rating, ""), nil
}

// ListDriverRatings returns the public reviews of a driver.
func (u *TripUsecase) ListDriverRatings(ctx context.Context, driverID uuid.UUID, offset, limit int) ([]dto.RatingResponse, int64, error) {
	ratings, total, err := u.ratings.ListForDriver(ctx, driverID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	return mapRatings(ratings), total, nil
}

// ListCompanyRatings returns the public reviews of a company.
func (u *TripUsecase) ListCompanyRatings(ctx context.Context, companyID uuid.UUID, offset, limit int) ([]dto.RatingResponse, int64, error) {
	ratings, total, err := u.ratings.ListForCompany(ctx, companyID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	return mapRatings(ratings), total, nil
}

// AutoConfirmDelivered is the scheduled job that closes delivered trips the
// shipper never confirmed.
func (u *TripUsecase) AutoConfirmDelivered(ctx context.Context) (int, error) {
	cutoff := time.Now().UTC().Add(-autoConfirmAfter)
	trips, err := u.trips.ListAwaitingConfirmation(ctx, cutoff, 200)
	if err != nil {
		return 0, err
	}
	closed := 0
	for i := range trips {
		if err := u.complete(ctx, &trips[i], nil); err != nil {
			u.log.Warn("auto-confirming trip failed",
				logger.String("trip_id", trips[i].ID.String()), logger.Err(err))
			continue
		}
		closed++
	}
	return closed, nil
}

// ─── internals ──────────────────────────────────────────────────────

// complete closes a trip, takes the commission and updates both sides' counters.
// actorID is nil when the platform auto-confirms.
func (u *TripUsecase) complete(ctx context.Context, trip *domain.Trip, actorID *uuid.UUID) error {
	commission := u.commissionFor(ctx, trip.AgreedPriceAmount)
	now := time.Now().UTC()

	err := u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.trips.TransitionStatus(txCtx, trip.ID,
			[]domain.TripStatus{domain.TripDelivered, domain.TripDisputed},
			domain.TripCompleted,
			map[string]any{"commission_amount": commission, "completed_at": now}); err != nil {
			return err
		}
		if err := u.trips.AddEvent(txCtx, &domain.TripEvent{
			ID:          uuid.New(),
			TripID:      trip.ID,
			Type:        domain.EventStatusChange,
			FromStatus:  strPtr(string(trip.Status)),
			ToStatus:    strPtr(string(domain.TripCompleted)),
			ActorUserID: actorID,
			Payload: domain.JSONMap{
				"commission_amount": commission,
				"auto_confirmed":    actorID == nil,
			},
		}); err != nil {
			return err
		}

		current, err := u.loads.FindByID(txCtx, trip.LoadID)
		if err != nil {
			return err
		}
		if current.Status.CanTransitionTo(domain.LoadCompleted) {
			if err := u.loads.TransitionStatus(txCtx, trip.LoadID,
				[]domain.LoadStatus{current.Status}, domain.LoadCompleted, nil); err != nil {
				return err
			}
		}

		driver, err := u.drivers.FindByID(txCtx, trip.DriverID)
		if err != nil {
			return err
		}
		distance := 0.0
		if trip.DistanceActualKm != nil {
			distance = *trip.DistanceActualKm
		} else if trip.DistancePlannedKm != nil {
			distance = *trip.DistancePlannedKm
		}
		if err := u.drivers.UpdateColumns(txCtx, driver.ID, map[string]any{
			"completed_trips":   driver.CompletedTrips + 1,
			"total_distance_km": driver.TotalDistanceKm + distance,
			"status":            domain.DriverStatusOnline,
		}); err != nil {
			return err
		}

		company, err := u.companies.FindByID(txCtx, trip.CompanyID)
		if err != nil {
			return err
		}
		return u.companies.UpdateColumns(txCtx, company.ID, map[string]any{
			"completed_trips": company.CompletedTrips + 1,
		})
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  actorID,
		Action:       "trip.completed",
		ResourceType: "trip",
		ResourceID:   &trip.ID,
		NewValues: map[string]any{
			"commission_amount": commission,
			"auto_confirmed":    actorID == nil,
		},
	})

	previous := trip.Status
	trip.Status = domain.TripCompleted
	trip.CommissionAmount = commission
	u.announceStatus(ctx, trip, previous, domain.TripCompleted, nil)

	if driver, err := u.drivers.FindByID(ctx, trip.DriverID); err == nil {
		u.notify(ctx, driver.UserID, "Trip completed",
			"The shipment is closed. You can now rate the shipper.",
			map[string]any{"trip_id": trip.ID})
	}
	u.notifyCompanyMembers(ctx, trip.CompanyID, "Trip completed",
		"The shipment is closed. You can now rate the driver.",
		map[string]any{"trip_id": trip.ID})
	return nil
}

// commissionFor computes the platform fee from the configured percentage.
func (u *TripUsecase) commissionFor(ctx context.Context, amount int64) int64 {
	percent := 5.0
	if u.settings != nil {
		if v, err := u.settings.Float(ctx, "platform.commission_percent"); err == nil && v >= 0 {
			percent = v
		}
	}
	if percent <= 0 {
		return 0
	}
	return int64(float64(amount) * percent / 100)
}

// assertParticipant allows the driver or an active member of the shipper company.
func (u *TripUsecase) assertParticipant(ctx context.Context, userID uuid.UUID, trip *domain.Trip) error {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err == nil && driver.ID == trip.DriverID {
		return nil
	}
	if err != nil && !apperror.Is(err, apperror.ErrNotFound.Code) {
		return err
	}
	member, err := u.companies.FindMember(ctx, trip.CompanyID, userID)
	if err == nil && member.IsActive {
		return nil
	}
	if err != nil && !apperror.Is(err, apperror.ErrNotFound.Code) {
		return err
	}
	return apperror.ErrForbidden.WithMessage("You are not party to this trip")
}

func (u *TripUsecase) list(ctx context.Context, filter domain.TripFilter, offset, limit int, order string) ([]dto.TripResponse, int64, error) {
	trips, total, err := u.trips.List(ctx, filter, offset, limit, order)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.TripResponse, 0, len(trips))
	for i := range trips {
		out = append(out, *u.toResponse(ctx, &trips[i]))
	}
	return out, total, nil
}

// announceStatus pushes a trip transition to both sides in realtime.
func (u *TripUsecase) announceStatus(ctx context.Context, trip *domain.Trip, from, to domain.TripStatus, point *geo.Point) {
	if u.events == nil {
		return
	}
	payload := map[string]any{
		"trip_id":     trip.ID,
		"trip_number": trip.TripNumber,
		"load_id":     trip.LoadID,
		"from_status": from,
		"to_status":   to,
	}
	if point != nil {
		payload["lat"] = point.Lat
		payload["lng"] = point.Lng
	}
	if err := u.events.PublishToTrip(ctx, trip.ID, domain.EventTripStatusChange, payload); err != nil {
		u.log.Warn("publishing trip status failed", logger.Err(err))
	}
	if err := u.events.PublishToCompany(ctx, trip.CompanyID, domain.EventTripStatusChange, payload); err != nil {
		u.log.Warn("publishing trip status to company failed", logger.Err(err))
	}
	if driver, err := u.drivers.FindByID(ctx, trip.DriverID); err == nil {
		if err := u.events.PublishToUser(ctx, driver.UserID, domain.EventTripStatusChange, payload); err != nil {
			u.log.Warn("publishing trip status to driver failed", logger.Err(err))
		}
	}
}

func (u *TripUsecase) notify(ctx context.Context, userID uuid.UUID, title, body string, data map[string]any) {
	if u.notifier == nil {
		return
	}
	if err := u.notifier.Notify(ctx, domain.Notification{
		UserID: userID, Type: "trip", Title: title, Body: body, Data: data,
	}); err != nil {
		u.log.Warn("queueing trip notification failed", logger.Err(err))
	}
}

func (u *TripUsecase) notifyCompanyMembers(ctx context.Context, companyID uuid.UUID, title, body string, data map[string]any) {
	if u.notifier == nil {
		return
	}
	members, err := u.companies.ListMembers(ctx, companyID, true)
	if err != nil {
		return
	}
	notifications := make([]domain.Notification, 0, len(members))
	for _, m := range members {
		notifications = append(notifications, domain.Notification{
			UserID: m.UserID, Type: "trip", Title: title, Body: body, Data: data,
		})
	}
	if err := u.notifier.NotifyMany(ctx, notifications); err != nil {
		u.log.Warn("queueing company trip notifications failed", logger.Err(err))
	}
}

// eventPayload keeps timeline entries small but useful.
func eventPayload(note string, odometerKm *float64) domain.JSONMap {
	payload := domain.JSONMap{}
	if note != "" {
		payload["note"] = note
	}
	if odometerKm != nil {
		payload["odometer_km"] = *odometerKm
	}
	return payload
}

// defaultDocumentTypeFor picks the paper type a transition normally produces, so
// a driver uploading at "loaded" gets a waybill and at "delivered" a POD.
func defaultDocumentTypeFor(status domain.TripStatus) string {
	switch status {
	case domain.TripLoaded:
		return "waybill"
	case domain.TripDelivered:
		return "pod"
	default:
		return "photo"
	}
}

func buildTripFilter(q dto.TripSearchQuery) domain.TripFilter {
	filter := domain.TripFilter{ActiveOnly: q.ActiveOnly}
	for _, s := range q.Statuses {
		filter.Statuses = append(filter.Statuses, domain.TripStatus(s))
	}
	filter.Range = domain.TimeRange{From: q.From, To: q.To}
	return filter
}

func mapRatings(ratings []domain.Rating) []dto.RatingResponse {
	out := make([]dto.RatingResponse, 0, len(ratings))
	for i := range ratings {
		out = append(out, *ratingResponse(&ratings[i], ""))
	}
	return out
}

func ratingResponse(r *domain.Rating, raterName string) *dto.RatingResponse {
	criteria := map[string]any(r.Criteria)
	if criteria == nil {
		criteria = map[string]any{}
	}
	return &dto.RatingResponse{
		ID:        r.ID,
		TripID:    r.TripID,
		RaterName: raterName,
		RateeType: string(r.RateeType),
		Score:     r.Score,
		Comment:   r.Comment,
		Criteria:  criteria,
		CreatedAt: r.CreatedAt,
	}
}
