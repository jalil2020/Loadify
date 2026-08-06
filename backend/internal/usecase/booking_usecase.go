package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
)

const (
	// defaultReservationTTL is how long a company has to answer a driver.
	defaultReservationTTL = 24 * time.Hour
	// defaultOfferTTL is how long a driver has to answer a company offer. It is
	// much shorter because the company is holding a load off the market while
	// it waits.
	defaultOfferTTL = 30 * time.Minute
)

// BookingUsecase owns the negotiation between a driver and a shipper.
type BookingUsecase struct {
	bookings  domain.BookingRepository
	loads     domain.LoadRepository
	trips     domain.TripRepository
	drivers   domain.DriverRepository
	vehicles  domain.VehicleRepository
	companies domain.CompanyRepository
	users     domain.UserRepository
	files     *FileUsecase
	tx        domain.TxManager
	audit     domain.AuditLogger
	events    domain.EventPublisher
	notifier  domain.Notifier
	log       logger.Logger
}

// NewBookingUsecase wires the booking usecase.
func NewBookingUsecase(
	bookings domain.BookingRepository,
	loads domain.LoadRepository,
	trips domain.TripRepository,
	drivers domain.DriverRepository,
	vehicles domain.VehicleRepository,
	companies domain.CompanyRepository,
	users domain.UserRepository,
	files *FileUsecase,
	tx domain.TxManager,
	audit domain.AuditLogger,
	events domain.EventPublisher,
	notifier domain.Notifier,
	log logger.Logger,
) *BookingUsecase {
	return &BookingUsecase{
		bookings: bookings, loads: loads, trips: trips, drivers: drivers,
		vehicles: vehicles, companies: companies, users: users, files: files,
		tx: tx, audit: audit, events: events, notifier: notifier, log: log,
	}
}

// Reserve is a driver claiming a published load.
//
// Every guard here is also enforced by a database constraint or a guarded
// UPDATE, because two taps on a slow connection arrive as two requests.
func (u *BookingUsecase) Reserve(ctx context.Context, userID, loadID uuid.UUID, req dto.ReserveLoadRequest) (*dto.BookingResponse, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !driver.IsVerified() {
		return nil, apperror.ErrDriverNotVerified
	}

	load, err := u.loads.FindByID(ctx, loadID)
	if err != nil {
		return nil, err
	}
	if load.Status != domain.LoadPublished && load.Status != domain.LoadReserved {
		return nil, apperror.ErrLoadNotPublished
	}
	if load.IsExpired(time.Now().UTC()) {
		return nil, apperror.ErrLoadNotPublished.WithMessage("This load has expired")
	}

	vehicle, err := u.resolveVehicle(ctx, driver.ID, req.VehicleID, load)
	if err != nil {
		return nil, err
	}

	if existing, err := u.bookings.FindLiveForPair(ctx, loadID, driver.ID); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, apperror.ErrBookingExists
	}

	// A truck cannot be committed to two overlapping pickup windows.
	overlapping, err := u.trips.HasOverlapping(ctx, driver.ID, load.PickupFrom, load.DeliveryTo, nil)
	if err != nil {
		return nil, err
	}
	if overlapping {
		return nil, apperror.ErrTripConflict
	}

	expiresAt := time.Now().UTC().Add(defaultReservationTTL)
	// Never hold a reservation open past the pickup window itself.
	if load.PickupTo != nil && load.PickupTo.Before(expiresAt) {
		expiresAt = *load.PickupTo
	}

	booking := &domain.Booking{
		ID:                 uuid.New(),
		LoadID:             loadID,
		DriverID:           driver.ID,
		VehicleID:          &vehicle.ID,
		CompanyID:          load.CompanyID,
		InitiatedBy:        domain.InitiatedByDriver,
		Status:             domain.BookingPending,
		OfferedPriceAmount: req.OfferedPrice,
		Currency:           load.PriceCurrency,
		Message:            nilIfBlank(req.Message),
		ExpiresAt:          &expiresAt,
	}
	if booking.OfferedPriceAmount == nil {
		// Silence means the driver accepts the advertised rate.
		price := load.PriceAmount
		booking.OfferedPriceAmount = &price
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.bookings.Create(txCtx, booking); err != nil {
			return err
		}
		// Move the load to reserved only from published: a second reservation on
		// an already-reserved load is legitimate and must not fail here.
		if load.Status == domain.LoadPublished {
			if err := u.loads.TransitionStatus(txCtx, loadID,
				[]domain.LoadStatus{domain.LoadPublished}, domain.LoadReserved, nil); err != nil {
				return err
			}
		}
		return u.loads.UpdateColumns(txCtx, loadID, map[string]any{
			"reservations_count": load.ReservationsCount + 1,
		})
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "booking.reserved",
		ResourceType: "booking",
		ResourceID:   &booking.ID,
		NewValues:    map[string]any{"load_id": loadID, "driver_id": driver.ID},
	})
	u.announce(ctx, booking, domain.EventBookingCreated, load, driver)

	return u.Get(ctx, booking.ID, userID, false)
}

// Offer is a company proposing a load to a specific driver.
func (u *BookingUsecase) Offer(ctx context.Context, actorID, companyID uuid.UUID, req dto.OfferLoadRequest) (*dto.BookingResponse, error) {
	load, err := u.loads.FindByID(ctx, req.LoadID)
	if err != nil {
		return nil, err
	}
	if load.CompanyID != companyID {
		return nil, apperror.ErrNotFound.WithMessage("load not found")
	}
	if load.Status != domain.LoadPublished && load.Status != domain.LoadReserved {
		return nil, apperror.ErrLoadNotPublished.WithMessage(
			"Publish the load before offering it to a driver")
	}

	driver, err := u.drivers.FindByID(ctx, req.DriverID)
	if err != nil {
		return nil, err
	}
	if !driver.IsVerified() {
		return nil, apperror.ErrValidation.WithMessage("This driver is not verified yet")
	}

	vehicle, err := u.resolveVehicle(ctx, driver.ID, req.VehicleID, load)
	if err != nil {
		return nil, err
	}

	if existing, err := u.bookings.FindLiveForPair(ctx, req.LoadID, driver.ID); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, apperror.ErrBookingExists.WithMessage(
			"There is already an open booking between this load and driver")
	}

	ttl := defaultOfferTTL
	if req.ExpiresInMinutes != nil {
		ttl = time.Duration(*req.ExpiresInMinutes) * time.Minute
	}
	expiresAt := time.Now().UTC().Add(ttl)

	booking := &domain.Booking{
		ID:                 uuid.New(),
		LoadID:             req.LoadID,
		DriverID:           driver.ID,
		VehicleID:          &vehicle.ID,
		CompanyID:          companyID,
		InitiatedBy:        domain.InitiatedByCompany,
		Status:             domain.BookingOffered,
		OfferedPriceAmount: req.OfferedPrice,
		Currency:           load.PriceCurrency,
		Message:            nilIfBlank(req.Message),
		ExpiresAt:          &expiresAt,
	}
	if booking.OfferedPriceAmount == nil {
		price := load.PriceAmount
		booking.OfferedPriceAmount = &price
	}

	if err := u.bookings.Create(ctx, booking); err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "booking.offered",
		ResourceType: "booking",
		ResourceID:   &booking.ID,
		NewValues:    map[string]any{"load_id": req.LoadID, "driver_id": driver.ID},
	})
	u.announce(ctx, booking, domain.EventBookingOffered, load, driver)

	return u.Get(ctx, booking.ID, actorID, false)
}

// Respond accepts or rejects a booking. The responder must be the side that did
// not start it: accepting your own request would be a self-deal.
func (u *BookingUsecase) Respond(ctx context.Context, userID, bookingID uuid.UUID, req dto.RespondBookingRequest) (*dto.BookingResponse, error) {
	booking, err := u.bookings.FindByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if !booking.Status.IsOpen() {
		return nil, apperror.ErrBookingNotPending
	}
	if booking.IsExpired(time.Now().UTC()) {
		return nil, apperror.ErrBookingNotPending.WithMessage("This booking has expired")
	}
	if err := u.assertResponder(ctx, userID, booking); err != nil {
		return nil, err
	}

	if !req.Accept {
		return u.reject(ctx, userID, booking, req.Message)
	}
	return u.accept(ctx, userID, booking, req)
}

// accept turns a booking into a trip inside one transaction.
func (u *BookingUsecase) accept(ctx context.Context, userID uuid.UUID, booking *domain.Booking, req dto.RespondBookingRequest) (*dto.BookingResponse, error) {
	load, err := u.loads.FindByID(ctx, booking.LoadID)
	if err != nil {
		return nil, err
	}
	if !load.Status.IsOpen() {
		return nil, apperror.ErrLoadAlreadyBooked
	}

	agreedPrice := booking.Price()
	if req.AgreedPrice != nil {
		agreedPrice = *req.AgreedPrice
	}
	if agreedPrice <= 0 {
		return nil, apperror.ErrValidation.WithMessage("The agreed price must be positive")
	}

	// Re-check the overlap at acceptance time: the driver may have taken another
	// load between reserving and being accepted.
	overlapping, err := u.trips.HasOverlapping(ctx, booking.DriverID, load.PickupFrom, load.DeliveryTo, nil)
	if err != nil {
		return nil, err
	}
	if overlapping {
		return nil, apperror.ErrTripConflict.WithMessage(
			"This driver already has an overlapping trip")
	}

	now := time.Now().UTC()
	trip := &domain.Trip{
		ID:                uuid.New(),
		BookingID:         booking.ID,
		LoadID:            booking.LoadID,
		DriverID:          booking.DriverID,
		VehicleID:         booking.VehicleID,
		CompanyID:         booking.CompanyID,
		Status:            domain.TripCreated,
		AgreedPriceAmount: agreedPrice,
		Currency:          booking.Currency,
		DistancePlannedKm: load.DistanceKm,
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.bookings.TransitionStatus(txCtx, booking.ID,
			[]domain.BookingStatus{domain.BookingPending, domain.BookingOffered},
			domain.BookingAccepted,
			map[string]any{
				"agreed_price_amount": agreedPrice,
				"response_message":    nilIfBlank(req.Message),
				"responded_at":        now,
				"responded_by":        userID,
			}); err != nil {
			return err
		}
		if err := u.loads.TransitionStatus(txCtx, booking.LoadID,
			[]domain.LoadStatus{domain.LoadPublished, domain.LoadReserved},
			domain.LoadAssigned, nil); err != nil {
			return err
		}
		if err := u.trips.Create(txCtx, trip); err != nil {
			return err
		}
		if err := u.trips.AddEvent(txCtx, &domain.TripEvent{
			ID:          uuid.New(),
			TripID:      trip.ID,
			Type:        domain.EventStatusChange,
			ToStatus:    strPtr(string(domain.TripCreated)),
			ActorUserID: &userID,
			Payload:     domain.JSONMap{"agreed_price": agreedPrice},
		}); err != nil {
			return err
		}
		// Everyone else waiting on this load must be released now.
		_, err := u.bookings.RejectOthers(txCtx, booking.LoadID, booking.ID,
			"The load was assigned to another driver")
		return err
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "booking.accepted",
		ResourceType: "booking",
		ResourceID:   &booking.ID,
		NewValues:    map[string]any{"trip_id": trip.ID, "agreed_price": agreedPrice},
	})

	booking.Status = domain.BookingAccepted
	booking.AgreedPriceAmount = &agreedPrice
	u.announceAccepted(ctx, booking, trip)
	u.refreshDriverStats(ctx, booking.DriverID)

	return u.Get(ctx, booking.ID, userID, false)
}

// reject closes a booking and returns the load to the market when nobody else
// is waiting on it.
func (u *BookingUsecase) reject(ctx context.Context, userID uuid.UUID, booking *domain.Booking, message string) (*dto.BookingResponse, error) {
	err := u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.bookings.TransitionStatus(txCtx, booking.ID,
			[]domain.BookingStatus{domain.BookingPending, domain.BookingOffered},
			domain.BookingRejected,
			map[string]any{
				"response_message": nilIfBlank(message),
				"responded_at":     time.Now().UTC(),
				"responded_by":     userID,
			}); err != nil {
			return err
		}
		return u.releaseLoadIfIdle(txCtx, booking.LoadID)
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "booking.rejected",
		ResourceType: "booking",
		ResourceID:   &booking.ID,
	})

	booking.Status = domain.BookingRejected
	u.notifyCounterparty(ctx, booking, domain.EventBookingRejected,
		"Booking declined", "Your booking request was declined")
	u.refreshDriverStats(ctx, booking.DriverID)

	return u.Get(ctx, booking.ID, userID, false)
}

// Withdraw lets the initiator pull out of their own open booking.
func (u *BookingUsecase) Withdraw(ctx context.Context, userID, bookingID uuid.UUID, req dto.CancelBookingRequest) error {
	booking, err := u.bookings.FindByID(ctx, bookingID)
	if err != nil {
		return err
	}
	if !booking.Status.IsOpen() {
		return apperror.ErrBookingNotPending
	}
	if err := u.assertInitiator(ctx, userID, booking); err != nil {
		return err
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.bookings.TransitionStatus(txCtx, bookingID,
			[]domain.BookingStatus{domain.BookingPending, domain.BookingOffered},
			domain.BookingWithdrawn,
			map[string]any{
				"cancelled_by_user_id": userID,
				"cancellation_reason":  req.Reason,
				"responded_at":         time.Now().UTC(),
			}); err != nil {
			return err
		}
		return u.releaseLoadIfIdle(txCtx, booking.LoadID)
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "booking.withdrawn",
		ResourceType: "booking",
		ResourceID:   &bookingID,
		NewValues:    map[string]any{"reason": req.Reason},
	})
	booking.Status = domain.BookingWithdrawn
	u.notifyCounterparty(ctx, booking, domain.EventBookingCancelled,
		"Booking withdrawn", req.Reason)
	return nil
}

// Cancel closes an accepted booking before the trip starts. Either side may do
// it; the trip is cancelled with it and the load goes back on the market.
func (u *BookingUsecase) Cancel(ctx context.Context, userID, bookingID uuid.UUID, req dto.CancelBookingRequest) error {
	booking, err := u.bookings.FindByID(ctx, bookingID)
	if err != nil {
		return err
	}
	if booking.Status != domain.BookingAccepted {
		return apperror.ErrInvalidStatusChange.WithMessage(
			"Only an accepted booking can be cancelled")
	}
	if err := u.assertParticipant(ctx, userID, booking); err != nil {
		return err
	}

	trip, err := u.trips.FindByBooking(ctx, bookingID)
	if err != nil && !apperror.Is(err, apperror.ErrNotFound.Code) {
		return err
	}
	// Once the cargo is on the truck, cancelling is not a booking decision any
	// more — it is a dispute.
	if trip != nil && !trip.Status.CanTransitionTo(domain.TripCancelled) {
		return apperror.ErrInvalidStatusChange.WithMessage(
			"The cargo is already loaded; raise a dispute instead of cancelling")
	}

	now := time.Now().UTC()
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.bookings.TransitionStatus(txCtx, bookingID,
			[]domain.BookingStatus{domain.BookingAccepted}, domain.BookingCancelled,
			map[string]any{
				"cancelled_by_user_id": userID,
				"cancellation_reason":  req.Reason,
			}); err != nil {
			return err
		}
		if trip != nil {
			if err := u.trips.TransitionStatus(txCtx, trip.ID,
				[]domain.TripStatus{trip.Status}, domain.TripCancelled,
				map[string]any{
					"cancelled_by_user_id": userID,
					"cancellation_reason":  req.Reason,
				}); err != nil {
				return err
			}
			if err := u.trips.AddEvent(txCtx, &domain.TripEvent{
				ID:          uuid.New(),
				TripID:      trip.ID,
				Type:        domain.EventStatusChange,
				FromStatus:  strPtr(string(trip.Status)),
				ToStatus:    strPtr(string(domain.TripCancelled)),
				ActorUserID: &userID,
				Payload:     domain.JSONMap{"reason": req.Reason},
			}); err != nil {
				return err
			}
			// Free the driver: they are no longer carrying anything.
			if err := u.drivers.SetStatus(txCtx, booking.DriverID, domain.DriverStatusOnline); err != nil {
				return err
			}
		}
		// The load returns to the marketplace with its remaining time.
		return u.loads.TransitionStatus(txCtx, booking.LoadID,
			[]domain.LoadStatus{domain.LoadAssigned, domain.LoadReserved},
			domain.LoadPublished,
			map[string]any{"published_at": now})
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "booking.cancelled",
		ResourceType: "booking",
		ResourceID:   &bookingID,
		NewValues:    map[string]any{"reason": req.Reason},
	})
	booking.Status = domain.BookingCancelled
	u.notifyCounterparty(ctx, booking, domain.EventBookingCancelled,
		"Booking cancelled", req.Reason)
	u.recordCancellation(ctx, booking, userID)
	return nil
}

// Get returns one booking the caller is party to.
func (u *BookingUsecase) Get(ctx context.Context, bookingID, userID uuid.UUID, isAdmin bool) (*dto.BookingResponse, error) {
	booking, err := u.bookings.FindWithRelations(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		if err := u.assertParticipant(ctx, userID, booking); err != nil {
			return nil, err
		}
	}
	return u.toResponse(ctx, booking), nil
}

// ListForDriver returns the caller driver's bookings.
func (u *BookingUsecase) ListForDriver(ctx context.Context, userID uuid.UUID, q dto.BookingSearchQuery, offset, limit int, order string) ([]dto.BookingResponse, int64, error) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	filter := buildBookingFilter(q)
	filter.DriverID = &driver.ID
	return u.list(ctx, filter, offset, limit, order)
}

// ListForCompany returns the caller company's bookings.
func (u *BookingUsecase) ListForCompany(ctx context.Context, companyID uuid.UUID, q dto.BookingSearchQuery, offset, limit int, order string) ([]dto.BookingResponse, int64, error) {
	filter := buildBookingFilter(q)
	filter.CompanyID = &companyID
	return u.list(ctx, filter, offset, limit, order)
}

// ExpireOverdue is the scheduled sweep that closes unanswered bookings and
// returns their loads to the market.
func (u *BookingUsecase) ExpireOverdue(ctx context.Context) (int, error) {
	expired, err := u.bookings.ExpireOverdue(ctx, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	for i := range expired {
		booking := expired[i]
		if err := u.releaseLoadIfIdle(ctx, booking.LoadID); err != nil {
			u.log.Warn("releasing load after booking expiry failed",
				logger.String("load_id", booking.LoadID.String()), logger.Err(err))
		}
		u.notifyCounterparty(ctx, &booking, domain.EventBookingExpired,
			"Booking expired", "Nobody responded in time")
	}
	return len(expired), nil
}

// ─── internals ──────────────────────────────────────────────────────

func (u *BookingUsecase) list(ctx context.Context, filter domain.BookingFilter, offset, limit int, order string) ([]dto.BookingResponse, int64, error) {
	bookings, total, err := u.bookings.List(ctx, filter, offset, limit, order)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.BookingResponse, 0, len(bookings))
	for i := range bookings {
		out = append(out, *u.toResponse(ctx, &bookings[i]))
	}
	return out, total, nil
}

// resolveVehicle picks the truck for a booking and proves it fits the load.
// This is the authoritative compatibility check — the SQL filter used by search
// is an optimisation, not the gate.
func (u *BookingUsecase) resolveVehicle(ctx context.Context, driverID uuid.UUID, vehicleID *uuid.UUID, load *domain.Load) (*domain.Vehicle, error) {
	if vehicleID != nil {
		vehicle, err := u.vehicles.FindByID(ctx, *vehicleID)
		if err != nil {
			return nil, err
		}
		if vehicle.DriverID != driverID {
			return nil, apperror.ErrForbidden.WithMessage("That vehicle belongs to another driver")
		}
		if !vehicle.IsApproved() {
			return nil, apperror.ErrNoApprovedVehicle.WithMessage("This vehicle is not approved yet")
		}
		if !vehicle.CanCarry(load.Requirements()) {
			return nil, apperror.ErrVehicleUnsuitable
		}
		return vehicle, nil
	}

	vehicles, err := u.vehicles.ListByDriver(ctx, driverID)
	if err != nil {
		return nil, err
	}
	requirements := load.Requirements()
	// ListByDriver returns the primary truck first, so the driver's default
	// choice wins when several would fit.
	for i := range vehicles {
		if vehicles[i].CanCarry(requirements) {
			return &vehicles[i], nil
		}
	}
	if len(vehicles) == 0 {
		return nil, apperror.ErrNoApprovedVehicle
	}
	return nil, apperror.ErrVehicleUnsuitable.WithMessage(
		"None of your vehicles meets this load's requirements")
}

// releaseLoadIfIdle puts a load back to published once no live booking remains.
func (u *BookingUsecase) releaseLoadIfIdle(ctx context.Context, loadID uuid.UUID) error {
	remaining, total, err := u.bookings.List(ctx, domain.BookingFilter{
		LoadID:   &loadID,
		Statuses: domain.LiveBookingStatuses,
	}, 0, 1, "")
	if err != nil {
		return err
	}
	if total > 0 || len(remaining) > 0 {
		return nil
	}
	// Only a reserved load rolls back; an assigned one has a trip behind it.
	err = u.loads.TransitionStatus(ctx, loadID,
		[]domain.LoadStatus{domain.LoadReserved}, domain.LoadPublished, nil)
	if err != nil && apperror.Is(err, apperror.ErrInvalidStatusChange.Code) {
		// The load was not in `reserved`; nothing to release.
		return nil
	}
	return err
}

// assertParticipant allows the driver or any member of the shipper company.
func (u *BookingUsecase) assertParticipant(ctx context.Context, userID uuid.UUID, booking *domain.Booking) error {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err == nil && driver.ID == booking.DriverID {
		return nil
	}
	if err != nil && !apperror.Is(err, apperror.ErrNotFound.Code) {
		return err
	}
	member, err := u.companies.FindMember(ctx, booking.CompanyID, userID)
	if err == nil && member.IsActive {
		return nil
	}
	if err != nil && !apperror.Is(err, apperror.ErrNotFound.Code) {
		return err
	}
	return apperror.ErrForbidden.WithMessage("You are not party to this booking")
}

// assertResponder allows only the side that did not initiate the booking.
func (u *BookingUsecase) assertResponder(ctx context.Context, userID uuid.UUID, booking *domain.Booking) error {
	if err := u.assertParticipant(ctx, userID, booking); err != nil {
		return err
	}
	isDriver := false
	if driver, err := u.drivers.FindByUserID(ctx, userID); err == nil {
		isDriver = driver.ID == booking.DriverID
	}

	switch booking.InitiatedBy {
	case domain.InitiatedByDriver:
		if isDriver {
			return apperror.ErrForbidden.WithMessage(
				"The shipper must answer your reservation")
		}
	case domain.InitiatedByCompany:
		if !isDriver {
			return apperror.ErrForbidden.WithMessage(
				"The driver must answer your offer")
		}
	}
	return nil
}

// assertInitiator allows only the side that started the booking.
func (u *BookingUsecase) assertInitiator(ctx context.Context, userID uuid.UUID, booking *domain.Booking) error {
	if err := u.assertParticipant(ctx, userID, booking); err != nil {
		return err
	}
	isDriver := false
	if driver, err := u.drivers.FindByUserID(ctx, userID); err == nil {
		isDriver = driver.ID == booking.DriverID
	}
	if (booking.InitiatedBy == domain.InitiatedByDriver) != isDriver {
		return apperror.ErrForbidden.WithMessage("Only the initiator can withdraw this booking")
	}
	return nil
}

// refreshDriverStats recomputes the response-time and acceptance metrics that
// feed the driver score.
func (u *BookingUsecase) refreshDriverStats(ctx context.Context, driverID uuid.UUID) {
	avgSeconds, err := u.bookings.AverageResponseSeconds(ctx, driverID)
	if err != nil {
		u.log.Warn("computing driver response time failed", logger.Err(err))
		return
	}
	accepted, err := u.bookings.CountByDriver(ctx, driverID, []domain.BookingStatus{domain.BookingAccepted})
	if err != nil {
		return
	}
	answered, err := u.bookings.CountByDriver(ctx, driverID, []domain.BookingStatus{
		domain.BookingAccepted, domain.BookingRejected, domain.BookingExpired,
	})
	if err != nil {
		return
	}
	rate := 0.0
	if answered > 0 {
		rate = float64(accepted) / float64(answered) * 100
	}
	if err := u.drivers.UpdateColumns(ctx, driverID, map[string]any{
		"avg_response_time_sec": avgSeconds,
		"acceptance_rate":       rate,
	}); err != nil {
		u.log.Warn("updating driver stats failed", logger.Err(err))
	}
}

// recordCancellation increments the cancelling side's counter, which feeds the
// trust score both parties see.
func (u *BookingUsecase) recordCancellation(ctx context.Context, booking *domain.Booking, userID uuid.UUID) {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err == nil && driver.ID == booking.DriverID {
		if err := u.drivers.UpdateColumns(ctx, driver.ID, map[string]any{
			"cancelled_trips": driver.CancelledTrips + 1,
		}); err != nil {
			u.log.Warn("incrementing driver cancellations failed", logger.Err(err))
		}
		return
	}
	company, err := u.companies.FindByID(ctx, booking.CompanyID)
	if err != nil {
		return
	}
	if err := u.companies.UpdateColumns(ctx, company.ID, map[string]any{
		"cancelled_trips": company.CancelledTrips + 1,
	}); err != nil {
		u.log.Warn("incrementing company cancellations failed", logger.Err(err))
	}
}

// announce pushes a new booking to whichever side has to respond.
func (u *BookingUsecase) announce(ctx context.Context, booking *domain.Booking, event domain.EventType, load *domain.Load, driver *domain.Driver) {
	payload := map[string]any{
		"booking_id":   booking.ID,
		"load_id":      booking.LoadID,
		"reference_no": load.ReferenceNo,
		"driver_id":    booking.DriverID,
		"company_id":   booking.CompanyID,
		"status":       booking.Status,
		"price":        booking.Price(),
		"currency":     booking.Currency,
		"expires_at":   booking.ExpiresAt,
	}

	if booking.InitiatedBy == domain.InitiatedByDriver {
		u.publishToCompany(ctx, booking.CompanyID, event, payload)
		u.notifyCompany(ctx, booking.CompanyID, "New reservation",
			fmt.Sprintf("A driver reserved load %s", load.ReferenceNo), payload)
		return
	}

	if driver != nil {
		u.publishToUser(ctx, driver.UserID, event, payload)
		u.notify(ctx, driver.UserID, "New load offer",
			fmt.Sprintf("You were offered load %s", load.ReferenceNo), payload)
	}
}

// announceAccepted tells both sides a trip now exists.
func (u *BookingUsecase) announceAccepted(ctx context.Context, booking *domain.Booking, trip *domain.Trip) {
	payload := map[string]any{
		"booking_id": booking.ID,
		"trip_id":    trip.ID,
		"load_id":    booking.LoadID,
		"status":     domain.BookingAccepted,
		"price":      trip.AgreedPriceAmount,
		"currency":   trip.Currency,
	}
	u.publishToCompany(ctx, booking.CompanyID, domain.EventBookingAccepted, payload)
	if driver, err := u.drivers.FindByID(ctx, booking.DriverID); err == nil {
		u.publishToUser(ctx, driver.UserID, domain.EventBookingAccepted, payload)
		u.notify(ctx, driver.UserID, "Booking confirmed",
			"Your booking was accepted. The trip is ready to start.", payload)
	}
	u.notifyCompany(ctx, booking.CompanyID, "Booking confirmed",
		"A driver is assigned and the trip is ready to start.", payload)
}

// notifyCounterparty informs the side that did not perform the action.
func (u *BookingUsecase) notifyCounterparty(ctx context.Context, booking *domain.Booking, event domain.EventType, title, body string) {
	payload := map[string]any{
		"booking_id": booking.ID,
		"load_id":    booking.LoadID,
		"status":     booking.Status,
	}
	u.publishToCompany(ctx, booking.CompanyID, event, payload)
	if driver, err := u.drivers.FindByID(ctx, booking.DriverID); err == nil {
		u.publishToUser(ctx, driver.UserID, event, payload)
		u.notify(ctx, driver.UserID, title, body, payload)
	}
}

func (u *BookingUsecase) publishToUser(ctx context.Context, userID uuid.UUID, event domain.EventType, payload map[string]any) {
	if u.events == nil {
		return
	}
	if err := u.events.PublishToUser(ctx, userID, event, payload); err != nil {
		u.log.Warn("publishing booking event failed",
			logger.String("event", string(event)), logger.Err(err))
	}
}

func (u *BookingUsecase) publishToCompany(ctx context.Context, companyID uuid.UUID, event domain.EventType, payload map[string]any) {
	if u.events == nil {
		return
	}
	if err := u.events.PublishToCompany(ctx, companyID, event, payload); err != nil {
		u.log.Warn("publishing booking event to company failed",
			logger.String("event", string(event)), logger.Err(err))
	}
}

func (u *BookingUsecase) notify(ctx context.Context, userID uuid.UUID, title, body string, data map[string]any) {
	if u.notifier == nil {
		return
	}
	if err := u.notifier.Notify(ctx, domain.Notification{
		UserID: userID, Type: "booking", Title: title, Body: body, Data: data,
	}); err != nil {
		u.log.Warn("queueing booking notification failed", logger.Err(err))
	}
}

// notifyCompany reaches every active member, because any dispatcher on duty
// may be the one who can act.
func (u *BookingUsecase) notifyCompany(ctx context.Context, companyID uuid.UUID, title, body string, data map[string]any) {
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
			UserID: m.UserID, Type: "booking", Title: title, Body: body, Data: data,
		})
	}
	if err := u.notifier.NotifyMany(ctx, notifications); err != nil {
		u.log.Warn("queueing company notifications failed", logger.Err(err))
	}
}

func buildBookingFilter(q dto.BookingSearchQuery) domain.BookingFilter {
	filter := domain.BookingFilter{}
	for _, s := range q.Statuses {
		filter.Statuses = append(filter.Statuses, domain.BookingStatus(s))
	}
	if q.OpenOnly {
		filter.Statuses = []domain.BookingStatus{domain.BookingPending, domain.BookingOffered}
	}
	if q.LoadID != "" {
		if id, err := uuid.Parse(q.LoadID); err == nil {
			filter.LoadID = &id
		}
	}
	if q.Initiator != "" {
		initiator := domain.BookingInitiator(q.Initiator)
		filter.Initiator = &initiator
	}
	return filter
}

func strPtr(s string) *string { return &s }
