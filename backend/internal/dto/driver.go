package dto

import (
	"time"

	"github.com/google/uuid"
)

// ─── shared ─────────────────────────────────────────────────────────

// PointRequest is a coordinate supplied by a client.
type PointRequest struct {
	Lat float64 `json:"lat" validate:"required,latitude"`
	Lng float64 `json:"lng" validate:"required,longitude"`
}

// PointResponse is a coordinate returned to a client.
type PointResponse struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// AddressRequest describes a pickup, delivery or company address.
type AddressRequest struct {
	RegionID      *uuid.UUID   `json:"region_id" validate:"omitempty"`
	CityID        *uuid.UUID   `json:"city_id" validate:"omitempty"`
	Line1         string       `json:"line1" validate:"required,max=255"`
	Line2         string       `json:"line2" validate:"omitempty,max=255"`
	PostalCode    string       `json:"postal_code" validate:"omitempty,max=20"`
	Formatted     string       `json:"formatted" validate:"omitempty,max=512"`
	GooglePlaceID string       `json:"google_place_id" validate:"omitempty,max=255"`
	Location      PointRequest `json:"location" validate:"required"`
	ContactName   string       `json:"contact_name" validate:"omitempty,max=120"`
	ContactPhone  string       `json:"contact_phone" validate:"omitempty,phone"`
	Notes         string       `json:"notes" validate:"omitempty,max=1000"`
}

// AddressResponse is a stored address.
type AddressResponse struct {
	ID           uuid.UUID     `json:"id"`
	RegionID     *uuid.UUID    `json:"region_id"`
	RegionName   string        `json:"region_name,omitempty"`
	CityID       *uuid.UUID    `json:"city_id"`
	CityName     string        `json:"city_name,omitempty"`
	Line1        string        `json:"line1"`
	Line2        *string       `json:"line2"`
	Formatted    string        `json:"formatted"`
	Location     PointResponse `json:"location"`
	ContactName  *string       `json:"contact_name"`
	ContactPhone *string       `json:"contact_phone"`
}

// FileResponse is an uploaded object with a URL the client can fetch.
type FileResponse struct {
	ID           uuid.UUID `json:"id"`
	URL          string    `json:"url"`
	OriginalName string    `json:"original_name"`
	MimeType     string    `json:"mime_type"`
	SizeBytes    int64     `json:"size_bytes"`
	CreatedAt    time.Time `json:"created_at"`
}

// ─── driver profile ─────────────────────────────────────────────────

// CreateDriverProfileRequest completes a driver's professional profile.
type CreateDriverProfileRequest struct {
	LicenseNumber     string     `json:"license_number" validate:"required,min=4,max=64"`
	LicenseIssuedAt   *time.Time `json:"license_issued_at" validate:"omitempty"`
	LicenseExpiresAt  *time.Time `json:"license_expires_at" validate:"omitempty"`
	LicenseCategories []string   `json:"license_categories" validate:"omitempty,max=10,dive,max=8"`
	ExperienceYears   int        `json:"experience_years" validate:"gte=0,lte=70"`
	HomeCityID        *uuid.UUID `json:"home_city_id" validate:"omitempty"`
	Languages         []string   `json:"languages" validate:"omitempty,max=10,dive,max=16"`
	Bio               string     `json:"bio" validate:"omitempty,max=1000"`
}

// UpdateDriverStatusRequest toggles availability.
type UpdateDriverStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=offline online busy"`
}

// UpdateDriverLocationRequest is a single position report over REST. The
// high-frequency path is the WebSocket channel; this exists for clients that
// cannot hold a socket open.
type UpdateDriverLocationRequest struct {
	Lat          float64    `json:"lat" validate:"required,latitude"`
	Lng          float64    `json:"lng" validate:"required,longitude"`
	SpeedKmh     *float64   `json:"speed_kmh" validate:"omitempty,gte=0,lte=300"`
	Heading      *float64   `json:"heading" validate:"omitempty,gte=0,lte=360"`
	AccuracyM    *float64   `json:"accuracy_m" validate:"omitempty,gte=0"`
	AltitudeM    *float64   `json:"altitude_m" validate:"omitempty"`
	BatteryLevel *int       `json:"battery_level" validate:"omitempty,gte=0,lte=100"`
	TripID       *uuid.UUID `json:"trip_id" validate:"omitempty"`
	RecordedAt   *time.Time `json:"recorded_at" validate:"omitempty"`
}

// DriverResponse is the public driver profile.
type DriverResponse struct {
	ID                 uuid.UUID          `json:"id"`
	UserID             uuid.UUID          `json:"user_id"`
	FirstName          string             `json:"first_name"`
	LastName           string             `json:"last_name"`
	FullName           string             `json:"full_name"`
	Phone              string             `json:"phone,omitempty"`
	AvatarURL          *string            `json:"avatar_url"`
	CompanyID          *uuid.UUID         `json:"company_id"`
	LicenseNumber      *string            `json:"license_number,omitempty"`
	LicenseExpiresAt   *time.Time         `json:"license_expires_at,omitempty"`
	LicenseCategories  []string           `json:"license_categories"`
	ExperienceYears    int                `json:"experience_years"`
	HomeCity           string             `json:"home_city,omitempty"`
	Languages          []string           `json:"languages"`
	Bio                *string            `json:"bio"`
	Status             string             `json:"status"`
	VerificationStatus string             `json:"verification_status"`
	RejectionReason    *string            `json:"rejection_reason,omitempty"`
	Rating             float64            `json:"rating"`
	RatingCount        int                `json:"rating_count"`
	CompletedTrips     int                `json:"completed_trips"`
	CancelledTrips     int                `json:"cancelled_trips"`
	AcceptanceRate     float64            `json:"acceptance_rate"`
	CurrentLocation    *PointResponse     `json:"current_location,omitempty"`
	LocationUpdatedAt  *time.Time         `json:"location_updated_at,omitempty"`
	DistanceKm         *float64           `json:"distance_km,omitempty"`
	Vehicles           []VehicleResponse  `json:"vehicles,omitempty"`
	Documents          []DocumentResponse `json:"documents,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
}

// DriverSearchQuery is the nearby-driver search contract.
type DriverSearchQuery struct {
	Lat            *float64   `query:"lat" validate:"omitempty,latitude"`
	Lng            *float64   `query:"lng" validate:"omitempty,longitude"`
	RadiusKm       float64    `query:"radius_km" validate:"omitempty,gt=0,lte=1000"`
	VehicleTypes   []string   `query:"vehicle_type" validate:"omitempty,dive,max=24"`
	MinCapacityKg  int        `query:"min_capacity_kg" validate:"omitempty,gte=0"`
	MinVolumeM3    float64    `query:"min_volume_m3" validate:"omitempty,gte=0"`
	RequiresADR    bool       `query:"requires_adr"`
	RequiresRefrig bool       `query:"requires_refrigeration"`
	MinRating      float64    `query:"min_rating" validate:"omitempty,gte=0,lte=5"`
	OnlineOnly     bool       `query:"online_only"`
	RegionIDs      []string   `query:"region_id" validate:"omitempty,dive,uuid"`
	AvailableFrom  *time.Time `query:"available_from"`
}

// ─── documents ──────────────────────────────────────────────────────

// UploadDocumentRequest attaches an already-uploaded file as a document.
type UploadDocumentRequest struct {
	Type      string     `json:"type" validate:"required,oneof=driving_license passport id_card insurance tech_passport medical other"`
	Side      string     `json:"side" validate:"omitempty,oneof=front back single"`
	FileID    uuid.UUID  `json:"file_id" validate:"required"`
	Number    string     `json:"number" validate:"omitempty,max=64"`
	IssuedAt  *time.Time `json:"issued_at" validate:"omitempty"`
	ExpiresAt *time.Time `json:"expires_at" validate:"omitempty"`
}

// ReviewDocumentRequest is the admin decision on a document.
type ReviewDocumentRequest struct {
	Status string `json:"status" validate:"required,oneof=approved rejected"`
	Reason string `json:"reason" validate:"required_if=Status rejected,omitempty,max=500"`
}

// DocumentResponse is one moderated document.
type DocumentResponse struct {
	ID              uuid.UUID  `json:"id"`
	Type            string     `json:"type"`
	Side            string     `json:"side"`
	FileID          uuid.UUID  `json:"file_id"`
	URL             string     `json:"url,omitempty"`
	Number          *string    `json:"number"`
	IssuedAt        *time.Time `json:"issued_at"`
	ExpiresAt       *time.Time `json:"expires_at"`
	Status          string     `json:"status"`
	RejectionReason *string    `json:"rejection_reason"`
	ReviewedAt      *time.Time `json:"reviewed_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

// ─── vehicles ───────────────────────────────────────────────────────

// SaveVehicleRequest creates or updates a truck.
type SaveVehicleRequest struct {
	Type                    string          `json:"type" validate:"required,oneof=tent refrigerator isothermal flatbed container tanker car_carrier grain dump van lowboy other"`
	Make                    string          `json:"make" validate:"required,max=64"`
	Model                   string          `json:"model" validate:"required,max=64"`
	Year                    *int            `json:"year" validate:"omitempty,gte=1950,lte=2100"`
	PlateNumber             string          `json:"plate_number" validate:"required,max=24"`
	VIN                     string          `json:"vin" validate:"omitempty,max=32"`
	Color                   string          `json:"color" validate:"omitempty,max=32"`
	CapacityKg              int             `json:"capacity_kg" validate:"required,gt=0,lte=100000"`
	VolumeM3                *float64        `json:"volume_m3" validate:"omitempty,gt=0,lte=500"`
	LengthCm                *int            `json:"length_cm" validate:"omitempty,gt=0,lte=3000"`
	WidthCm                 *int            `json:"width_cm" validate:"omitempty,gt=0,lte=500"`
	HeightCm                *int            `json:"height_cm" validate:"omitempty,gt=0,lte=500"`
	FuelType                string          `json:"fuel_type" validate:"omitempty,oneof=diesel petrol gas methane propane electric hybrid"`
	Axles                   *int            `json:"axles" validate:"omitempty,gte=2,lte=10"`
	HasADR                  bool            `json:"has_adr"`
	HasRefrigeration        bool            `json:"has_refrigeration"`
	TempMinC                *int            `json:"temp_min_c" validate:"omitempty,gte=-50,lte=50"`
	TempMaxC                *int            `json:"temp_max_c" validate:"omitempty,gte=-50,lte=50"`
	HasTailLift             bool            `json:"has_tail_lift"`
	HasGPS                  bool            `json:"has_gps"`
	InsuranceExpiresAt      *time.Time      `json:"insurance_expires_at" validate:"omitempty"`
	TechInspectionExpiresAt *time.Time      `json:"tech_inspection_expires_at" validate:"omitempty"`
	PhotoFileIDs            []uuid.UUID     `json:"photo_file_ids" validate:"omitempty,max=12"`
	Trailer                 *TrailerRequest `json:"trailer" validate:"omitempty"`
}

// TrailerRequest describes an attached semi-trailer.
type TrailerRequest struct {
	Type        string   `json:"type" validate:"required,max=24"`
	PlateNumber string   `json:"plate_number" validate:"required,max=24"`
	Make        string   `json:"make" validate:"omitempty,max=64"`
	Model       string   `json:"model" validate:"omitempty,max=64"`
	Year        *int     `json:"year" validate:"omitempty,gte=1950,lte=2100"`
	CapacityKg  *int     `json:"capacity_kg" validate:"omitempty,gt=0"`
	VolumeM3    *float64 `json:"volume_m3" validate:"omitempty,gt=0"`
	LengthCm    *int     `json:"length_cm" validate:"omitempty,gt=0"`
	WidthCm     *int     `json:"width_cm" validate:"omitempty,gt=0"`
	HeightCm    *int     `json:"height_cm" validate:"omitempty,gt=0"`
	Axles       *int     `json:"axles" validate:"omitempty,gte=1,lte=10"`
}

// VehicleResponse is a truck as returned to clients.
type VehicleResponse struct {
	ID                      uuid.UUID        `json:"id"`
	DriverID                uuid.UUID        `json:"driver_id"`
	Type                    string           `json:"type"`
	Make                    string           `json:"make"`
	Model                   string           `json:"model"`
	Year                    *int16           `json:"year"`
	PlateNumber             string           `json:"plate_number"`
	Color                   *string          `json:"color"`
	CapacityKg              int              `json:"capacity_kg"`
	VolumeM3                *float64         `json:"volume_m3"`
	LengthCm                *int             `json:"length_cm"`
	WidthCm                 *int             `json:"width_cm"`
	HeightCm                *int             `json:"height_cm"`
	FuelType                string           `json:"fuel_type"`
	Axles                   *int16           `json:"axles"`
	HasADR                  bool             `json:"has_adr"`
	HasRefrigeration        bool             `json:"has_refrigeration"`
	TempMinC                *int16           `json:"temp_min_c"`
	TempMaxC                *int16           `json:"temp_max_c"`
	HasTailLift             bool             `json:"has_tail_lift"`
	HasGPS                  bool             `json:"has_gps"`
	InsuranceExpiresAt      *time.Time       `json:"insurance_expires_at"`
	TechInspectionExpiresAt *time.Time       `json:"tech_inspection_expires_at"`
	Status                  string           `json:"status"`
	RejectionReason         *string          `json:"rejection_reason"`
	IsPrimary               bool             `json:"is_primary"`
	Photos                  []FileResponse   `json:"photos"`
	Trailer                 *TrailerResponse `json:"trailer"`
	CreatedAt               time.Time        `json:"created_at"`
}

// TrailerResponse is an attached trailer.
type TrailerResponse struct {
	ID          uuid.UUID `json:"id"`
	Type        string    `json:"type"`
	PlateNumber string    `json:"plate_number"`
	CapacityKg  *int      `json:"capacity_kg"`
	VolumeM3    *float64  `json:"volume_m3"`
	Axles       *int16    `json:"axles"`
}

// ─── availability & preferences ─────────────────────────────────────

// PublishAvailabilityRequest advertises free capacity on a corridor.
type PublishAvailabilityRequest struct {
	VehicleID          *uuid.UUID      `json:"vehicle_id" validate:"omitempty"`
	FromAddress        *AddressRequest `json:"from_address" validate:"omitempty"`
	AvailableFrom      time.Time       `json:"available_from" validate:"required"`
	AvailableTo        *time.Time      `json:"available_to" validate:"omitempty,gtfield=AvailableFrom"`
	PreferredRegionIDs []uuid.UUID     `json:"preferred_region_ids" validate:"omitempty,max=20"`
	MaxDistanceKm      *int            `json:"max_distance_km" validate:"omitempty,gt=0,lte=10000"`
	MinPriceAmount     *int64          `json:"min_price_amount" validate:"omitempty,gte=0"`
	Currency           string          `json:"currency" validate:"omitempty,len=3"`
	Notes              string          `json:"notes" validate:"omitempty,max=500"`
}

// AvailabilityResponse is a published availability window.
type AvailabilityResponse struct {
	ID                 uuid.UUID        `json:"id"`
	VehicleID          *uuid.UUID       `json:"vehicle_id"`
	FromAddress        *AddressResponse `json:"from_address"`
	AvailableFrom      time.Time        `json:"available_from"`
	AvailableTo        *time.Time       `json:"available_to"`
	PreferredRegionIDs []uuid.UUID      `json:"preferred_region_ids"`
	MaxDistanceKm      *int             `json:"max_distance_km"`
	MinPriceAmount     *int64           `json:"min_price_amount"`
	Currency           string           `json:"currency"`
	Notes              *string          `json:"notes"`
	IsActive           bool             `json:"is_active"`
	CreatedAt          time.Time        `json:"created_at"`
}

// SavePreferencesRequest configures matching and notifications.
type SavePreferencesRequest struct {
	MaxDistanceKm       int         `json:"max_distance_km" validate:"required,gt=0,lte=10000"`
	PreferredCargoTypes []uuid.UUID `json:"preferred_cargo_types" validate:"omitempty,max=20"`
	PreferredRegionIDs  []uuid.UUID `json:"preferred_region_ids" validate:"omitempty,max=20"`
	MinPriceAmount      *int64      `json:"min_price_amount" validate:"omitempty,gte=0"`
	NotifyNewLoads      bool        `json:"notify_new_loads"`
	NotifyRadiusKm      int         `json:"notify_radius_km" validate:"required,gt=0,lte=1000"`
	AutoAcceptOffers    bool        `json:"auto_accept_offers"`
}

// PreferencesResponse is the stored preference set.
type PreferencesResponse struct {
	MaxDistanceKm       int         `json:"max_distance_km"`
	PreferredCargoTypes []uuid.UUID `json:"preferred_cargo_types"`
	PreferredRegionIDs  []uuid.UUID `json:"preferred_region_ids"`
	MinPriceAmount      *int64      `json:"min_price_amount"`
	NotifyNewLoads      bool        `json:"notify_new_loads"`
	NotifyRadiusKm      int         `json:"notify_radius_km"`
	AutoAcceptOffers    bool        `json:"auto_accept_offers"`
}
