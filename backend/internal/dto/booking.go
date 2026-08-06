package dto

import (
	"time"

	"github.com/google/uuid"
)

// ─── bookings ───────────────────────────────────────────────────────

// ReserveLoadRequest is a driver claiming a published load.
type ReserveLoadRequest struct {
	VehicleID    *uuid.UUID `json:"vehicle_id" validate:"omitempty"`
	OfferedPrice *int64     `json:"offered_price" validate:"omitempty,gt=0"`
	Message      string     `json:"message" validate:"omitempty,max=1000"`
}

// OfferLoadRequest is a company offering a load to a specific driver.
type OfferLoadRequest struct {
	LoadID           uuid.UUID  `json:"load_id" validate:"required"`
	DriverID         uuid.UUID  `json:"driver_id" validate:"required"`
	VehicleID        *uuid.UUID `json:"vehicle_id" validate:"omitempty"`
	OfferedPrice     *int64     `json:"offered_price" validate:"omitempty,gt=0"`
	Message          string     `json:"message" validate:"omitempty,max=1000"`
	ExpiresInMinutes *int       `json:"expires_in_minutes" validate:"omitempty,gt=0,lte=1440"`
}

// RespondBookingRequest accepts or rejects a booking.
type RespondBookingRequest struct {
	Accept      bool   `json:"accept"`
	AgreedPrice *int64 `json:"agreed_price" validate:"omitempty,gt=0"`
	Message     string `json:"message" validate:"omitempty,max=1000"`
}

// CancelBookingRequest carries the mandatory reason.
type CancelBookingRequest struct {
	Reason string `json:"reason" validate:"required,min=3,max=500"`
}

// BookingSearchQuery filters a booking listing.
type BookingSearchQuery struct {
	Statuses  []string `query:"status" validate:"omitempty,dive,oneof=pending offered accepted rejected withdrawn expired cancelled"`
	LoadID    string   `query:"load_id" validate:"omitempty,uuid"`
	Initiator string   `query:"initiated_by" validate:"omitempty,oneof=driver company"`
	OpenOnly  bool     `query:"open_only"`
}

// BookingSummary is the compact form embedded in a load or trip.
type BookingSummary struct {
	ID          uuid.UUID  `json:"id"`
	Status      string     `json:"status"`
	InitiatedBy string     `json:"initiated_by"`
	Price       int64      `json:"price"`
	Currency    string     `json:"currency"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// BookingResponse is a booking with both sides resolved.
type BookingResponse struct {
	ID                 uuid.UUID            `json:"id"`
	LoadID             uuid.UUID            `json:"load_id"`
	Load               *LoadSummary         `json:"load"`
	DriverID           uuid.UUID            `json:"driver_id"`
	Driver             *BookingParty        `json:"driver"`
	CompanyID          uuid.UUID            `json:"company_id"`
	Company            *LoadCompanyResponse `json:"company"`
	VehicleID          *uuid.UUID           `json:"vehicle_id"`
	Vehicle            *VehicleSummary      `json:"vehicle"`
	InitiatedBy        string               `json:"initiated_by"`
	Status             string               `json:"status"`
	OfferedPrice       *int64               `json:"offered_price"`
	AgreedPrice        *int64               `json:"agreed_price"`
	Currency           string               `json:"currency"`
	Message            *string              `json:"message"`
	ResponseMessage    *string              `json:"response_message"`
	ExpiresAt          *time.Time           `json:"expires_at"`
	RespondedAt        *time.Time           `json:"responded_at"`
	CancellationReason *string              `json:"cancellation_reason"`
	TripID             *uuid.UUID           `json:"trip_id"`
	CreatedAt          time.Time            `json:"created_at"`
}

// BookingParty is the counterparty summary shown on a booking.
type BookingParty struct {
	UserID    uuid.UUID `json:"user_id"`
	FullName  string    `json:"full_name"`
	AvatarURL *string   `json:"avatar_url"`
	Rating    float64   `json:"rating"`
	Trips     int       `json:"trips"`
	// Phone appears once the booking gives both sides a reason to talk.
	Phone *string `json:"phone,omitempty"`
}

// VehicleSummary is the compact truck description on a booking or trip.
type VehicleSummary struct {
	ID          uuid.UUID `json:"id"`
	Type        string    `json:"type"`
	Make        string    `json:"make"`
	Model       string    `json:"model"`
	PlateNumber string    `json:"plate_number"`
	CapacityKg  int       `json:"capacity_kg"`
	VolumeM3    *float64  `json:"volume_m3"`
}

// LoadSummary is the compact load description embedded elsewhere.
type LoadSummary struct {
	ID                uuid.UUID `json:"id"`
	ReferenceNo       string    `json:"reference_no"`
	Title             string    `json:"title"`
	Status            string    `json:"status"`
	WeightKg          int       `json:"weight_kg"`
	PickupFormatted   string    `json:"pickup_formatted"`
	DeliveryFormatted string    `json:"delivery_formatted"`
	PickupFrom        time.Time `json:"pickup_from"`
	PriceAmount       int64     `json:"price_amount"`
	PriceCurrency     string    `json:"price_currency"`
	DistanceKm        *float64  `json:"distance_km"`
}

// ─── trips ──────────────────────────────────────────────────────────

// AdvanceTripRequest moves a trip to the next status.
type AdvanceTripRequest struct {
	Status          string        `json:"status" validate:"required,oneof=en_route_to_pickup at_pickup loaded in_transit at_delivery delivered"`
	Location        *PointRequest `json:"location" validate:"omitempty"`
	Note            string        `json:"note" validate:"omitempty,max=1000"`
	DocumentFileIDs []uuid.UUID   `json:"document_file_ids" validate:"omitempty,max=10"`
	DocumentType    string        `json:"document_type" validate:"omitempty,oneof=cmr invoice waybill pod photo signature other"`
	OdometerKm      *float64      `json:"odometer_km" validate:"omitempty,gte=0"`
}

// CancelTripRequest carries the mandatory reason.
type CancelTripRequest struct {
	Reason string `json:"reason" validate:"required,min=3,max=500"`
}

// AddTripDocumentRequest attaches a paper to a trip.
type AddTripDocumentRequest struct {
	FileID uuid.UUID `json:"file_id" validate:"required"`
	Type   string    `json:"type" validate:"required,oneof=cmr invoice waybill pod photo signature other"`
	Name   string    `json:"name" validate:"omitempty,max=255"`
}

// TripSearchQuery filters a trip listing.
type TripSearchQuery struct {
	Statuses   []string   `query:"status" validate:"omitempty,dive,oneof=created en_route_to_pickup at_pickup loaded in_transit at_delivery delivered completed cancelled disputed"`
	ActiveOnly bool       `query:"active_only"`
	From       *time.Time `query:"from"`
	To         *time.Time `query:"to"`
}

// TripResponse is a trip with its parties and timeline.
type TripResponse struct {
	ID                 uuid.UUID              `json:"id"`
	TripNumber         string                 `json:"trip_number"`
	BookingID          uuid.UUID              `json:"booking_id"`
	LoadID             uuid.UUID              `json:"load_id"`
	Load               *LoadSummary           `json:"load"`
	DriverID           uuid.UUID              `json:"driver_id"`
	Driver             *BookingParty          `json:"driver"`
	CompanyID          uuid.UUID              `json:"company_id"`
	Company            *LoadCompanyResponse   `json:"company"`
	Vehicle            *VehicleSummary        `json:"vehicle"`
	Status             string                 `json:"status"`
	AgreedPrice        int64                  `json:"agreed_price"`
	CommissionAmount   int64                  `json:"commission_amount"`
	DriverPayout       int64                  `json:"driver_payout"`
	Currency           string                 `json:"currency"`
	DistancePlannedKm  *float64               `json:"distance_planned_km"`
	DistanceActualKm   *float64               `json:"distance_actual_km"`
	ETAAt              *time.Time             `json:"eta_at"`
	StartedAt          *time.Time             `json:"started_at"`
	ArrivedPickupAt    *time.Time             `json:"arrived_pickup_at"`
	LoadedAt           *time.Time             `json:"loaded_at"`
	ArrivedDeliveryAt  *time.Time             `json:"arrived_delivery_at"`
	DeliveredAt        *time.Time             `json:"delivered_at"`
	CompletedAt        *time.Time             `json:"completed_at"`
	CancellationReason *string                `json:"cancellation_reason"`
	CurrentLocation    *PointResponse         `json:"current_location,omitempty"`
	Events             []TripEventResponse    `json:"events,omitempty"`
	Documents          []TripDocumentResponse `json:"documents,omitempty"`
	// CanRate tells the client whether the rating form should be shown.
	CanRate   bool            `json:"can_rate"`
	MyRating  *RatingResponse `json:"my_rating,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// TripEventResponse is one timeline entry.
type TripEventResponse struct {
	ID         uuid.UUID      `json:"id"`
	Type       string         `json:"type"`
	FromStatus *string        `json:"from_status"`
	ToStatus   *string        `json:"to_status"`
	Location   *PointResponse `json:"location,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// TripDocumentResponse is a paper attached to a trip.
type TripDocumentResponse struct {
	ID        uuid.UUID `json:"id"`
	FileID    uuid.UUID `json:"file_id"`
	URL       string    `json:"url"`
	Type      string    `json:"type"`
	Name      *string   `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// TripTrackResponse is the live position and route of an in-transit shipment.
type TripTrackResponse struct {
	TripID          uuid.UUID       `json:"trip_id"`
	Status          string          `json:"status"`
	CurrentLocation *PointResponse  `json:"current_location"`
	UpdatedAt       *time.Time      `json:"updated_at"`
	ETAAt           *time.Time      `json:"eta_at"`
	Route           []PointResponse `json:"route"`
	DistanceDoneKm  float64         `json:"distance_done_km"`
}

// ─── ratings ────────────────────────────────────────────────────────

// RateTripRequest scores the counterparty after a trip.
type RateTripRequest struct {
	Score    int            `json:"score" validate:"required,gte=1,lte=5"`
	Comment  string         `json:"comment" validate:"omitempty,max=1000"`
	Criteria map[string]any `json:"criteria" validate:"omitempty"`
}

// RatingResponse is one score with its author.
type RatingResponse struct {
	ID        uuid.UUID      `json:"id"`
	TripID    uuid.UUID      `json:"trip_id"`
	RaterName string         `json:"rater_name,omitempty"`
	RateeType string         `json:"ratee_type"`
	Score     int16          `json:"score"`
	Comment   *string        `json:"comment"`
	Criteria  map[string]any `json:"criteria,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}
