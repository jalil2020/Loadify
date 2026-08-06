package domain

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/pkg/geo"
)

// BookingStatus is where a booking sits between request and trip.
type BookingStatus string

const (
	BookingPending   BookingStatus = "pending"   // driver reserved, company must respond
	BookingOffered   BookingStatus = "offered"   // company offered, driver must respond
	BookingAccepted  BookingStatus = "accepted"  // agreed, a trip exists
	BookingRejected  BookingStatus = "rejected"  // the other side said no
	BookingWithdrawn BookingStatus = "withdrawn" // the initiator pulled out
	BookingExpired   BookingStatus = "expired"   // nobody responded in time
	BookingCancelled BookingStatus = "cancelled" // cancelled after acceptance
)

// LiveBookingStatuses are the states that occupy a load/driver pair. The partial
// unique index in the schema uses exactly this set.
var LiveBookingStatuses = []BookingStatus{BookingPending, BookingOffered, BookingAccepted}

// IsLive reports whether the booking still occupies the load/driver pair.
func (s BookingStatus) IsLive() bool { return slices.Contains(LiveBookingStatuses, s) }

// IsOpen reports whether the booking is waiting for a response.
func (s BookingStatus) IsOpen() bool { return s == BookingPending || s == BookingOffered }

// BookingInitiator is which side started the conversation. It decides who is
// allowed to accept: the other one.
type BookingInitiator string

const (
	InitiatedByDriver  BookingInitiator = "driver"
	InitiatedByCompany BookingInitiator = "company"
)

// Booking is a request to carry a specific load with a specific truck.
type Booking struct {
	ID                 uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	LoadID             uuid.UUID        `gorm:"type:uuid;not null;index"`
	DriverID           uuid.UUID        `gorm:"type:uuid;not null;index"`
	VehicleID          *uuid.UUID       `gorm:"type:uuid"`
	CompanyID          uuid.UUID        `gorm:"type:uuid;not null;index"`
	InitiatedBy        BookingInitiator `gorm:"size:8;not null"`
	Status             BookingStatus    `gorm:"size:16;not null;default:'pending'"`
	OfferedPriceAmount *int64
	AgreedPriceAmount  *int64
	Currency           string `gorm:"size:3;not null;default:'UZS'"`
	Message            *string
	ResponseMessage    *string
	ExpiresAt          *time.Time
	RespondedAt        *time.Time
	RespondedBy        *uuid.UUID `gorm:"type:uuid"`
	CancelledByUserID  *uuid.UUID `gorm:"type:uuid"`
	CancellationReason *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`

	Load    *Load    `gorm:"foreignKey:LoadID"`
	Driver  *Driver  `gorm:"foreignKey:DriverID"`
	Vehicle *Vehicle `gorm:"foreignKey:VehicleID"`
	Company *Company `gorm:"foreignKey:CompanyID"`
	Trip    *Trip    `gorm:"foreignKey:BookingID"`
}

// IsExpired reports whether an open booking has run out of time.
func (b *Booking) IsExpired(now time.Time) bool {
	return b.Status.IsOpen() && b.ExpiresAt != nil && now.After(*b.ExpiresAt)
}

// Price is the agreed amount if there is one, otherwise the offer.
func (b *Booking) Price() int64 {
	if b.AgreedPriceAmount != nil {
		return *b.AgreedPriceAmount
	}
	if b.OfferedPriceAmount != nil {
		return *b.OfferedPriceAmount
	}
	return 0
}

// TripStatus is the physical progress of a shipment.
type TripStatus string

const (
	TripCreated    TripStatus = "created"
	TripEnRoute    TripStatus = "en_route_to_pickup"
	TripAtPickup   TripStatus = "at_pickup"
	TripLoaded     TripStatus = "loaded"
	TripInTransit  TripStatus = "in_transit"
	TripAtDelivery TripStatus = "at_delivery"
	TripDelivered  TripStatus = "delivered"
	TripCompleted  TripStatus = "completed"
	TripCancelled  TripStatus = "cancelled"
	TripDisputed   TripStatus = "disputed"
)

// tripTransitions is the trip state machine. Cancellation is allowed up to the
// moment the cargo is loaded; after that the shipment must be delivered or
// disputed, because the goods are already on the truck.
var tripTransitions = map[TripStatus][]TripStatus{
	TripCreated:    {TripEnRoute, TripCancelled},
	TripEnRoute:    {TripAtPickup, TripCancelled},
	TripAtPickup:   {TripLoaded, TripCancelled},
	TripLoaded:     {TripInTransit},
	TripInTransit:  {TripAtDelivery, TripDelivered, TripDisputed},
	TripAtDelivery: {TripDelivered, TripDisputed},
	TripDelivered:  {TripCompleted, TripDisputed},
	TripDisputed:   {TripCompleted, TripCancelled},
	TripCompleted:  {},
	TripCancelled:  {},
}

// CanTransitionTo reports whether the trip may move to the target status.
func (s TripStatus) CanTransitionTo(target TripStatus) bool {
	return slices.Contains(tripTransitions[s], target)
}

// IsActive reports whether the driver is currently working this trip.
func (s TripStatus) IsActive() bool {
	switch s {
	case TripEnRoute, TripAtPickup, TripLoaded, TripInTransit, TripAtDelivery:
		return true
	default:
		return false
	}
}

// IsFinished reports whether the trip has reached a terminal state.
func (s TripStatus) IsFinished() bool {
	return s == TripCompleted || s == TripCancelled
}

// driverAdvanceable are the transitions a driver performs from the road. Every
// other transition belongs to the company or an admin.
var driverAdvanceable = map[TripStatus]bool{
	TripEnRoute:    true,
	TripAtPickup:   true,
	TripLoaded:     true,
	TripInTransit:  true,
	TripAtDelivery: true,
	TripDelivered:  true,
}

// IsDriverAdvanceable reports whether a driver may set this status.
func (s TripStatus) IsDriverAdvanceable() bool { return driverAdvanceable[s] }

// Trip is an accepted booking being carried out.
type Trip struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	BookingID uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex"`
	LoadID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	DriverID  uuid.UUID  `gorm:"type:uuid;not null;index"`
	VehicleID *uuid.UUID `gorm:"type:uuid"`
	CompanyID uuid.UUID  `gorm:"type:uuid;not null;index"`
	// See Load.ReferenceNo: the default tag is what lets the database generate
	// the number instead of GORM writing an empty string.
	TripNumber         string     `gorm:"size:24;not null;default:next_reference('TR', 'trip_number_seq')"`
	Status             TripStatus `gorm:"size:24;not null;default:'created'"`
	AgreedPriceAmount  int64      `gorm:"not null"`
	CommissionAmount   int64      `gorm:"not null;default:0"`
	Currency           string     `gorm:"size:3;not null;default:'UZS'"`
	DistancePlannedKm  *float64   `gorm:"type:numeric(10,2)"`
	DistanceActualKm   *float64   `gorm:"type:numeric(10,2)"`
	ETAAt              *time.Time `gorm:"column:eta_at"`
	StartedAt          *time.Time
	ArrivedPickupAt    *time.Time
	LoadedAt           *time.Time
	ArrivedDeliveryAt  *time.Time
	DeliveredAt        *time.Time
	CompletedAt        *time.Time
	CancelledAt        *time.Time
	CancelledByUserID  *uuid.UUID `gorm:"type:uuid"`
	CancellationReason *string
	Notes              *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`

	Booking   *Booking       `gorm:"foreignKey:BookingID"`
	Load      *Load          `gorm:"foreignKey:LoadID"`
	Driver    *Driver        `gorm:"foreignKey:DriverID"`
	Vehicle   *Vehicle       `gorm:"foreignKey:VehicleID"`
	Company   *Company       `gorm:"foreignKey:CompanyID"`
	Events    []TripEvent    `gorm:"foreignKey:TripID"`
	Documents []TripDocument `gorm:"foreignKey:TripID"`
}

// DriverPayout is what the driver receives after commission.
func (t *Trip) DriverPayout() int64 { return t.AgreedPriceAmount - t.CommissionAmount }

// TimestampColumnFor maps a status to the column that records when it happened,
// so a status change and its timestamp are always written together.
func TimestampColumnFor(status TripStatus) string {
	switch status {
	case TripEnRoute:
		return "started_at"
	case TripAtPickup:
		return "arrived_pickup_at"
	case TripLoaded:
		return "loaded_at"
	case TripAtDelivery:
		return "arrived_delivery_at"
	case TripDelivered:
		return "delivered_at"
	case TripCompleted:
		return "completed_at"
	case TripCancelled:
		return "cancelled_at"
	default:
		return ""
	}
}

// LoadStatusForTrip maps trip progress onto the load status the company sees.
// An empty result means the load status does not change.
func LoadStatusForTrip(status TripStatus) LoadStatus {
	switch status {
	case TripInTransit, TripLoaded:
		return LoadInTransit
	case TripDelivered:
		return LoadDelivered
	case TripCompleted:
		return LoadCompleted
	default:
		return ""
	}
}

// TripEventType classifies an entry in the trip timeline.
type TripEventType string

const (
	EventStatusChange TripEventType = "status_change"
	EventLocation     TripEventType = "location"
	EventDocument     TripEventType = "document"
	EventNote         TripEventType = "note"
	EventDelay        TripEventType = "delay"
	EventIssue        TripEventType = "issue"
)

// TripEvent is one immutable entry in the trip timeline.
type TripEvent struct {
	ID          uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TripID      uuid.UUID     `gorm:"type:uuid;not null;index"`
	Type        TripEventType `gorm:"size:24;not null"`
	FromStatus  *string       `gorm:"size:24"`
	ToStatus    *string       `gorm:"size:24"`
	Location    *geo.Point    `gorm:"type:geography(Point,4326)"`
	Payload     JSONMap       `gorm:"type:jsonb;not null;default:'{}'"`
	ActorUserID *uuid.UUID    `gorm:"type:uuid"`
	CreatedAt   time.Time
}

// TripDocument is a paper produced during the trip: waybill, CMR, proof of delivery.
type TripDocument struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TripID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	FileID     uuid.UUID  `gorm:"type:uuid;not null"`
	Type       string     `gorm:"size:24;not null"`
	Name       *string    `gorm:"size:255"`
	UploadedBy *uuid.UUID `gorm:"type:uuid"`
	CreatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`

	File *File `gorm:"foreignKey:FileID"`
}

// RateeType is which side of the trip a rating is about.
type RateeType string

const (
	RateeDriver  RateeType = "driver"
	RateeCompany RateeType = "company"
)

// Rating is one party's score for the other after a completed trip.
type Rating struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TripID      uuid.UUID  `gorm:"type:uuid;not null;index"`
	RaterUserID uuid.UUID  `gorm:"type:uuid;not null"`
	RateeUserID uuid.UUID  `gorm:"type:uuid;not null"`
	RateeType   RateeType  `gorm:"size:8;not null"`
	DriverID    *uuid.UUID `gorm:"type:uuid"`
	CompanyID   *uuid.UUID `gorm:"type:uuid"`
	Score       int16      `gorm:"not null"`
	Comment     *string
	Criteria    JSONMap `gorm:"type:jsonb;not null;default:'{}'"`
	IsHidden    bool    `gorm:"not null;default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ─── ports ──────────────────────────────────────────────────────────

// BookingFilter narrows a booking listing.
type BookingFilter struct {
	LoadID    *uuid.UUID
	DriverID  *uuid.UUID
	CompanyID *uuid.UUID
	Statuses  []BookingStatus
	Initiator *BookingInitiator
}

// BookingRepository persists bookings.
type BookingRepository interface {
	Create(ctx context.Context, booking *Booking) error
	FindByID(ctx context.Context, id uuid.UUID) (*Booking, error)
	FindWithRelations(ctx context.Context, id uuid.UUID) (*Booking, error)
	List(ctx context.Context, filter BookingFilter, offset, limit int, order string) ([]Booking, int64, error)
	// TransitionStatus guards on the current status inside the UPDATE so a
	// double-tapped accept cannot create two trips.
	TransitionStatus(ctx context.Context, id uuid.UUID, from []BookingStatus, to BookingStatus, columns map[string]any) error
	FindLiveForPair(ctx context.Context, loadID, driverID uuid.UUID) (*Booking, error)
	CountByDriver(ctx context.Context, driverID uuid.UUID, statuses []BookingStatus) (int64, error)
	// RejectOthers closes the remaining pending bookings once a load is assigned.
	RejectOthers(ctx context.Context, loadID, keepBookingID uuid.UUID, reason string) (int64, error)
	ExpireOverdue(ctx context.Context, now time.Time) ([]Booking, error)
	AverageResponseSeconds(ctx context.Context, driverID uuid.UUID) (int, error)
}

// TripFilter narrows a trip listing.
type TripFilter struct {
	DriverID   *uuid.UUID
	CompanyID  *uuid.UUID
	LoadID     *uuid.UUID
	Statuses   []TripStatus
	Range      TimeRange
	ActiveOnly bool
}

// TripRepository persists trips, their timeline and documents.
type TripRepository interface {
	Create(ctx context.Context, trip *Trip) error
	FindByID(ctx context.Context, id uuid.UUID) (*Trip, error)
	FindWithRelations(ctx context.Context, id uuid.UUID) (*Trip, error)
	FindByBooking(ctx context.Context, bookingID uuid.UUID) (*Trip, error)
	List(ctx context.Context, filter TripFilter, offset, limit int, order string) ([]Trip, int64, error)
	TransitionStatus(ctx context.Context, id uuid.UUID, from []TripStatus, to TripStatus, columns map[string]any) error
	UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error
	// HasOverlapping reports whether the driver is already committed during a
	// window, which is the guard against double-booking a truck.
	HasOverlapping(ctx context.Context, driverID uuid.UUID, from time.Time, to *time.Time, excludeTripID *uuid.UUID) (bool, error)
	ActiveByDriver(ctx context.Context, driverID uuid.UUID) (*Trip, error)
	ListAwaitingConfirmation(ctx context.Context, deliveredBefore time.Time, limit int) ([]Trip, error)

	AddEvent(ctx context.Context, event *TripEvent) error
	ListEvents(ctx context.Context, tripID uuid.UUID) ([]TripEvent, error)
	AddDocument(ctx context.Context, doc *TripDocument) error
	ListDocuments(ctx context.Context, tripID uuid.UUID) ([]TripDocument, error)
	DeleteDocument(ctx context.Context, docID uuid.UUID) error

	CountCompletedByDriver(ctx context.Context, driverID uuid.UUID) (int64, error)
}

// RatingRepository persists post-trip ratings.
type RatingRepository interface {
	Create(ctx context.Context, rating *Rating) error
	FindByTripAndRater(ctx context.Context, tripID, raterUserID uuid.UUID) (*Rating, error)
	ListForDriver(ctx context.Context, driverID uuid.UUID, offset, limit int) ([]Rating, int64, error)
	ListForCompany(ctx context.Context, companyID uuid.UUID, offset, limit int) ([]Rating, int64, error)
	Hide(ctx context.Context, id uuid.UUID, hidden bool) error
}
