package dto

import (
	"time"

	"github.com/google/uuid"
)

// SaveLoadRequest creates or updates a shipment.
type SaveLoadRequest struct {
	Title       string     `json:"title" validate:"required,min=3,max=255"`
	Description string     `json:"description" validate:"omitempty,max=4000"`
	CargoTypeID *uuid.UUID `json:"cargo_type_id" validate:"omitempty"`

	WeightKg  int      `json:"weight_kg" validate:"required,gt=0,lte=100000"`
	VolumeM3  *float64 `json:"volume_m3" validate:"omitempty,gt=0,lte=500"`
	LengthCm  *int     `json:"length_cm" validate:"omitempty,gt=0,lte=3000"`
	WidthCm   *int     `json:"width_cm" validate:"omitempty,gt=0,lte=500"`
	HeightCm  *int     `json:"height_cm" validate:"omitempty,gt=0,lte=500"`
	Packaging string   `json:"packaging" validate:"omitempty,oneof=pallet box bag bulk roll barrel container other"`
	Quantity  *int     `json:"quantity" validate:"omitempty,gt=0"`

	PickupAddress   AddressRequest `json:"pickup_address" validate:"required"`
	DeliveryAddress AddressRequest `json:"delivery_address" validate:"required"`
	PickupFrom      time.Time      `json:"pickup_from" validate:"required"`
	PickupTo        *time.Time     `json:"pickup_to" validate:"omitempty,gtefield=PickupFrom"`
	DeliveryFrom    *time.Time     `json:"delivery_from" validate:"omitempty"`
	DeliveryTo      *time.Time     `json:"delivery_to" validate:"omitempty"`
	DistanceKm      *float64       `json:"distance_km" validate:"omitempty,gt=0"`

	RequiredVehicleType   string `json:"required_vehicle_type" validate:"omitempty,oneof=tent refrigerator isothermal flatbed container tanker car_carrier grain dump van lowboy other"`
	RequiredCapacityKg    *int   `json:"required_capacity_kg" validate:"omitempty,gt=0"`
	RequiresADR           bool   `json:"requires_adr"`
	RequiresRefrigeration bool   `json:"requires_refrigeration"`
	TempMinC              *int   `json:"temp_min_c" validate:"omitempty,gte=-50,lte=50"`
	TempMaxC              *int   `json:"temp_max_c" validate:"omitempty,gte=-50,lte=50"`
	RequiresTailLift      bool   `json:"requires_tail_lift"`
	LoadingType           string `json:"loading_type" validate:"omitempty,oneof=rear side top full"`
	SpecialRequirements   string `json:"special_requirements" validate:"omitempty,max=2000"`

	PriceAmount       int64  `json:"price_amount" validate:"required,gt=0"`
	PriceCurrency     string `json:"price_currency" validate:"omitempty,len=3"`
	PriceType         string `json:"price_type" validate:"omitempty,oneof=fixed negotiable per_km"`
	PaymentTerms      string `json:"payment_terms" validate:"omitempty,oneof=on_loading on_delivery prepayment deferred"`
	PaymentMethod     string `json:"payment_method" validate:"omitempty,oneof=cash transfer card escrow"`
	PrepaymentPercent *int   `json:"prepayment_percent" validate:"omitempty,gte=0,lte=100"`
	DeferredDays      *int   `json:"deferred_days" validate:"omitempty,gte=0,lte=365"`

	PhotoFileIDs    []uuid.UUID `json:"photo_file_ids" validate:"omitempty,max=10"`
	DocumentFileIDs []uuid.UUID `json:"document_file_ids" validate:"omitempty,max=10"`
	IsUrgent        bool        `json:"is_urgent"`
	// Publish immediately instead of saving a draft.
	Publish bool `json:"publish"`
}

// PublishLoadRequest optionally overrides how long the load stays live.
type PublishLoadRequest struct {
	ExpiresInHours *int `json:"expires_in_hours" validate:"omitempty,gt=0,lte=720"`
}

// CancelLoadRequest carries the mandatory reason.
type CancelLoadRequest struct {
	Reason string `json:"reason" validate:"required,min=3,max=500"`
}

// LoadSearchQuery is the marketplace search contract.
type LoadSearchQuery struct {
	PickupRegionIDs   []string `query:"pickup_region_id" validate:"omitempty,dive,uuid"`
	DeliveryRegionIDs []string `query:"delivery_region_id" validate:"omitempty,dive,uuid"`
	CargoTypeIDs      []string `query:"cargo_type_id" validate:"omitempty,dive,uuid"`

	PickupLat        *float64 `query:"pickup_lat" validate:"omitempty,latitude"`
	PickupLng        *float64 `query:"pickup_lng" validate:"omitempty,longitude"`
	PickupRadiusKm   float64  `query:"pickup_radius_km" validate:"omitempty,gt=0,lte=1000"`
	DeliveryLat      *float64 `query:"delivery_lat" validate:"omitempty,latitude"`
	DeliveryLng      *float64 `query:"delivery_lng" validate:"omitempty,longitude"`
	DeliveryRadiusKm float64  `query:"delivery_radius_km" validate:"omitempty,gt=0,lte=1000"`

	MinWeightKg int     `query:"min_weight_kg" validate:"omitempty,gte=0"`
	MaxWeightKg int     `query:"max_weight_kg" validate:"omitempty,gte=0"`
	MinVolumeM3 float64 `query:"min_volume_m3" validate:"omitempty,gte=0"`
	MaxVolumeM3 float64 `query:"max_volume_m3" validate:"omitempty,gte=0"`

	VehicleTypes          []string `query:"vehicle_type" validate:"omitempty,dive,max=24"`
	RequiresADR           *bool    `query:"requires_adr"`
	RequiresRefrigeration *bool    `query:"requires_refrigeration"`
	RequiresTailLift      *bool    `query:"requires_tail_lift"`

	MinPriceAmount int64    `query:"min_price" validate:"omitempty,gte=0"`
	MaxPriceAmount int64    `query:"max_price" validate:"omitempty,gte=0"`
	Currency       string   `query:"currency" validate:"omitempty,len=3"`
	PriceTypes     []string `query:"price_type" validate:"omitempty,dive,oneof=fixed negotiable per_km"`
	PaymentTerms   []string `query:"payment_terms" validate:"omitempty,dive,oneof=on_loading on_delivery prepayment deferred"`

	PickupFrom *time.Time `query:"pickup_from"`
	PickupTo   *time.Time `query:"pickup_to"`

	FeaturedOnly bool   `query:"featured_only"`
	UrgentOnly   bool   `query:"urgent_only"`
	Status       string `query:"status" validate:"omitempty,oneof=draft published reserved assigned in_transit delivered completed cancelled expired"`
	// FitsVehicleID restricts results to loads the given truck can legally carry.
	FitsVehicleID string `query:"fits_vehicle_id" validate:"omitempty,uuid"`
	// FitsMyVehicle applies the same filter using the driver's primary truck.
	FitsMyVehicle bool `query:"fits_my_vehicle"`
}

// LoadResponse is a shipment as returned to clients.
type LoadResponse struct {
	ID          uuid.UUID  `json:"id"`
	ReferenceNo string     `json:"reference_no"`
	CompanyID   uuid.UUID  `json:"company_id"`
	Title       string     `json:"title"`
	Description *string    `json:"description"`
	CargoType   *string    `json:"cargo_type"`
	CargoTypeID *uuid.UUID `json:"cargo_type_id"`

	WeightKg  int      `json:"weight_kg"`
	VolumeM3  *float64 `json:"volume_m3"`
	LengthCm  *int     `json:"length_cm"`
	WidthCm   *int     `json:"width_cm"`
	HeightCm  *int     `json:"height_cm"`
	Packaging string   `json:"packaging"`
	Quantity  *int     `json:"quantity"`

	PickupAddress    *AddressResponse `json:"pickup_address"`
	DeliveryAddress  *AddressResponse `json:"delivery_address"`
	PickupFrom       time.Time        `json:"pickup_from"`
	PickupTo         *time.Time       `json:"pickup_to"`
	DeliveryFrom     *time.Time       `json:"delivery_from"`
	DeliveryTo       *time.Time       `json:"delivery_to"`
	DistanceKm       *float64         `json:"distance_km"`
	PickupDistanceKm *float64         `json:"pickup_distance_km,omitempty"`

	RequiredVehicleType   *string `json:"required_vehicle_type"`
	RequiredCapacityKg    *int    `json:"required_capacity_kg"`
	RequiresADR           bool    `json:"requires_adr"`
	RequiresRefrigeration bool    `json:"requires_refrigeration"`
	TempMinC              *int16  `json:"temp_min_c"`
	TempMaxC              *int16  `json:"temp_max_c"`
	RequiresTailLift      bool    `json:"requires_tail_lift"`
	LoadingType           string  `json:"loading_type"`
	SpecialRequirements   *string `json:"special_requirements"`

	PriceAmount       int64    `json:"price_amount"`
	PriceCurrency     string   `json:"price_currency"`
	PriceType         string   `json:"price_type"`
	PricePerKm        *float64 `json:"price_per_km,omitempty"`
	PaymentTerms      string   `json:"payment_terms"`
	PaymentMethod     string   `json:"payment_method"`
	PrepaymentPercent *int16   `json:"prepayment_percent"`
	DeferredDays      *int16   `json:"deferred_days"`

	Status             string     `json:"status"`
	PublishedAt        *time.Time `json:"published_at"`
	ExpiresAt          *time.Time `json:"expires_at"`
	CancellationReason *string    `json:"cancellation_reason,omitempty"`

	ViewsCount        int  `json:"views_count"`
	ReservationsCount int  `json:"reservations_count"`
	IsFeatured        bool `json:"is_featured"`
	IsUrgent          bool `json:"is_urgent"`
	IsSaved           bool `json:"is_saved"`

	// Company contact details appear only once the viewer has earned access:
	// a live booking, company membership or an admin role.
	Company   *LoadCompanyResponse `json:"company"`
	Photos    []FileResponse       `json:"photos,omitempty"`
	Documents []FileResponse       `json:"documents,omitempty"`

	// MyBooking is the viewer's own booking on this load, when one exists.
	MyBooking *BookingSummary `json:"my_booking,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LoadCompanyResponse is the shipper summary embedded in a load.
type LoadCompanyResponse struct {
	ID             uuid.UUID `json:"id"`
	BrandName      string    `json:"brand_name"`
	LogoURL        *string   `json:"logo_url"`
	Rating         float64   `json:"rating"`
	RatingCount    int       `json:"rating_count"`
	CompletedTrips int       `json:"completed_trips"`
	IsVerified     bool      `json:"is_verified"`
	// Phone is populated only for viewers allowed to make contact.
	Phone *string `json:"phone,omitempty"`
}

// SaveSearchRequest stores a reusable filter.
type SaveSearchRequest struct {
	Name          string         `json:"name" validate:"required,min=1,max=100"`
	Filters       map[string]any `json:"filters" validate:"required"`
	NotifyEnabled bool           `json:"notify_enabled"`
}

// SavedSearchResponse is a stored filter.
type SavedSearchResponse struct {
	ID             uuid.UUID      `json:"id"`
	Name           string         `json:"name"`
	Filters        map[string]any `json:"filters"`
	NotifyEnabled  bool           `json:"notify_enabled"`
	LastNotifiedAt *time.Time     `json:"last_notified_at"`
	CreatedAt      time.Time      `json:"created_at"`
}

// CargoTypeResponse is one entry of the cargo vocabulary.
type CargoTypeResponse struct {
	ID                  uuid.UUID `json:"id"`
	Code                string    `json:"code"`
	Name                string    `json:"name"`
	Icon                *string   `json:"icon"`
	RequiresADR         bool      `json:"requires_adr"`
	RequiresTemperature bool      `json:"requires_temperature"`
}
