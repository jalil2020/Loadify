package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/pkg/geo"
)

// DriverStatus is the availability state a driver publishes to the marketplace.
type DriverStatus string

const (
	DriverStatusOffline DriverStatus = "offline"
	DriverStatusOnline  DriverStatus = "online"
	DriverStatusBusy    DriverStatus = "busy"
)

// DocumentType enumerates the papers a driver must supply.
type DocumentType string

const (
	DocDrivingLicense DocumentType = "driving_license"
	DocPassport       DocumentType = "passport"
	DocIDCard         DocumentType = "id_card"
	DocInsurance      DocumentType = "insurance"
	DocTechPassport   DocumentType = "tech_passport"
	DocMedical        DocumentType = "medical"
	DocOther          DocumentType = "other"
)

// RequiredDriverDocuments is the set that must be approved before a driver is
// verified. Everything else is optional and only strengthens their profile.
var RequiredDriverDocuments = []DocumentType{DocDrivingLicense, DocPassport}

// VehicleType is the body type of a truck. These values are the vocabulary
// shared by load requirements and driver capabilities — matching depends on
// both sides using exactly this list.
type VehicleType string

const (
	VehicleTent         VehicleType = "tent"
	VehicleRefrigerator VehicleType = "refrigerator"
	VehicleIsothermal   VehicleType = "isothermal"
	VehicleFlatbed      VehicleType = "flatbed"
	VehicleContainer    VehicleType = "container"
	VehicleTanker       VehicleType = "tanker"
	VehicleCarCarrier   VehicleType = "car_carrier"
	VehicleGrain        VehicleType = "grain"
	VehicleDump         VehicleType = "dump"
	VehicleVan          VehicleType = "van"
	VehicleLowboy       VehicleType = "lowboy"
	VehicleOther        VehicleType = "other"
)

// AllVehicleTypes is the canonical ordered list, used by reference endpoints.
var AllVehicleTypes = []VehicleType{
	VehicleTent, VehicleRefrigerator, VehicleIsothermal, VehicleFlatbed,
	VehicleContainer, VehicleTanker, VehicleCarCarrier, VehicleGrain,
	VehicleDump, VehicleVan, VehicleLowboy, VehicleOther,
}

// Driver is the professional profile attached to a user account.
type Driver struct {
	ID                 uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID             uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex"`
	CompanyID          *uuid.UUID  `gorm:"type:uuid;index"`
	LicenseNumber      *string     `gorm:"size:64"`
	LicenseIssuedAt    *time.Time  `gorm:"type:date"`
	LicenseExpiresAt   *time.Time  `gorm:"type:date"`
	LicenseCategories  StringArray `gorm:"type:text[]"`
	ExperienceYears    int16       `gorm:"not null;default:0"`
	HomeCityID         *uuid.UUID  `gorm:"type:uuid"`
	Languages          StringArray `gorm:"type:text[]"`
	Bio                *string
	Status             DriverStatus       `gorm:"size:16;not null;default:'offline'"`
	VerificationStatus VerificationStatus `gorm:"size:16;not null;default:'pending'"`
	RejectionReason    *string
	VerifiedAt         *time.Time
	VerifiedBy         *uuid.UUID `gorm:"type:uuid"`
	Rating             float64    `gorm:"type:numeric(3,2);not null;default:0"`
	RatingCount        int        `gorm:"not null;default:0"`
	CompletedTrips     int        `gorm:"not null;default:0"`
	CancelledTrips     int        `gorm:"not null;default:0"`
	AcceptanceRate     float64    `gorm:"type:numeric(5,2);not null;default:0"`
	AvgResponseTimeSec int        `gorm:"not null;default:0"`
	TotalDistanceKm    float64    `gorm:"type:numeric(12,2);not null;default:0"`
	CurrentLocation    *geo.Point `gorm:"type:geography(Point,4326)"`
	LocationUpdatedAt  *time.Time
	LastOnlineAt       *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`

	User     *User            `gorm:"foreignKey:UserID"`
	Vehicles []Vehicle        `gorm:"foreignKey:DriverID"`
	Docs     []DriverDocument `gorm:"foreignKey:DriverID"`
	HomeCity *City            `gorm:"foreignKey:HomeCityID"`
}

// IsVerified reports whether the driver may reserve loads.
func (d *Driver) IsVerified() bool { return d.VerificationStatus == VerificationVerified }

// IsAvailable reports whether the driver can take a new booking right now.
func (d *Driver) IsAvailable() bool { return d.IsVerified() && d.Status == DriverStatusOnline }

// DriverDocument is one uploaded and moderated paper.
type DriverDocument struct {
	ID              uuid.UUID          `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	DriverID        uuid.UUID          `gorm:"type:uuid;not null;index"`
	Type            DocumentType       `gorm:"size:32;not null"`
	Side            string             `gorm:"size:8;not null;default:'single'"`
	FileID          uuid.UUID          `gorm:"type:uuid;not null"`
	Number          *string            `gorm:"size:64"`
	IssuedAt        *time.Time         `gorm:"type:date"`
	ExpiresAt       *time.Time         `gorm:"type:date"`
	Status          VerificationStatus `gorm:"size:16;not null;default:'pending'"`
	RejectionReason *string
	ReviewedBy      *uuid.UUID `gorm:"type:uuid"`
	ReviewedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`

	File *File `gorm:"foreignKey:FileID"`
}

// IsExpired reports whether an approved document has passed its expiry date.
func (d *DriverDocument) IsExpired(now time.Time) bool {
	return d.ExpiresAt != nil && now.After(*d.ExpiresAt)
}

// Vehicle is a truck a driver can carry a load with.
type Vehicle struct {
	ID                      uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	DriverID                uuid.UUID   `gorm:"type:uuid;not null;index"`
	CompanyID               *uuid.UUID  `gorm:"type:uuid;index"`
	Type                    VehicleType `gorm:"size:24;not null"`
	Make                    string      `gorm:"size:64;not null"`
	Model                   string      `gorm:"size:64;not null"`
	Year                    *int16
	PlateNumber             string   `gorm:"size:24;not null"`
	VIN                     *string  `gorm:"size:32"`
	Color                   *string  `gorm:"size:32"`
	CapacityKg              int      `gorm:"not null"`
	VolumeM3                *float64 `gorm:"type:numeric(8,2)"`
	LengthCm                *int
	WidthCm                 *int
	HeightCm                *int
	FuelType                string `gorm:"size:16;not null;default:'diesel'"`
	Axles                   *int16
	HasADR                  bool `gorm:"column:has_adr;not null;default:false"`
	HasRefrigeration        bool `gorm:"not null;default:false"`
	TempMinC                *int16
	TempMaxC                *int16
	HasTailLift             bool               `gorm:"not null;default:false"`
	HasGPS                  bool               `gorm:"column:has_gps;not null;default:false"`
	InsuranceExpiresAt      *time.Time         `gorm:"type:date"`
	TechInspectionExpiresAt *time.Time         `gorm:"type:date"`
	Status                  VerificationStatus `gorm:"size:16;not null;default:'pending'"`
	RejectionReason         *string
	IsPrimary               bool `gorm:"not null;default:false"`
	CreatedAt               time.Time
	UpdatedAt               time.Time
	DeletedAt               gorm.DeletedAt `gorm:"index"`

	Photos  []VehiclePhoto `gorm:"foreignKey:VehicleID"`
	Trailer *Trailer       `gorm:"foreignKey:VehicleID"`
}

// IsApproved reports whether the vehicle may be used on a booking.
func (v *Vehicle) IsApproved() bool { return v.Status == VerificationApproved }

// CanCarry reports whether this vehicle satisfies a load's hard requirements.
// This is the single source of truth for compatibility — the load search filter,
// the reservation guard and the matching score all call it.
func (v *Vehicle) CanCarry(req CargoRequirements) bool {
	if !v.IsApproved() {
		return false
	}
	if v.CapacityKg < req.WeightKg {
		return false
	}
	if req.VolumeM3 != nil && v.VolumeM3 != nil && *v.VolumeM3 < *req.VolumeM3 {
		return false
	}
	if req.VehicleType != nil && *req.VehicleType != VehicleOther && v.Type != *req.VehicleType {
		return false
	}
	if req.RequiresADR && !v.HasADR {
		return false
	}
	if req.RequiresRefrigeration && !v.HasRefrigeration {
		return false
	}
	if req.RequiresTailLift && !v.HasTailLift {
		return false
	}
	// A refrigerated load also constrains the temperature range the unit can hold.
	if req.TempMinC != nil && v.TempMinC != nil && *v.TempMinC > *req.TempMinC {
		return false
	}
	if req.TempMaxC != nil && v.TempMaxC != nil && *v.TempMaxC < *req.TempMaxC {
		return false
	}
	if req.LengthCm != nil && v.LengthCm != nil && *v.LengthCm < *req.LengthCm {
		return false
	}
	if req.WidthCm != nil && v.WidthCm != nil && *v.WidthCm < *req.WidthCm {
		return false
	}
	if req.HeightCm != nil && v.HeightCm != nil && *v.HeightCm < *req.HeightCm {
		return false
	}
	return true
}

// CargoRequirements is what a load demands of a truck.
type CargoRequirements struct {
	WeightKg              int
	VolumeM3              *float64
	LengthCm              *int
	WidthCm               *int
	HeightCm              *int
	VehicleType           *VehicleType
	RequiresADR           bool
	RequiresRefrigeration bool
	RequiresTailLift      bool
	TempMinC              *int16
	TempMaxC              *int16
}

// VehiclePhoto is one image of a truck.
type VehiclePhoto struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	VehicleID uuid.UUID `gorm:"type:uuid;not null;index"`
	FileID    uuid.UUID `gorm:"type:uuid;not null"`
	Position  string    `gorm:"size:16;not null;default:'other'"`
	SortOrder int16     `gorm:"not null;default:0"`
	CreatedAt time.Time

	File *File `gorm:"foreignKey:FileID"`
}

// Trailer is an optional semi-trailer attached to a vehicle.
type Trailer struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	VehicleID   uuid.UUID `gorm:"type:uuid;not null;index"`
	Type        string    `gorm:"size:24;not null"`
	PlateNumber string    `gorm:"size:24;not null"`
	Make        *string   `gorm:"size:64"`
	Model       *string   `gorm:"size:64"`
	Year        *int16
	CapacityKg  *int
	VolumeM3    *float64 `gorm:"type:numeric(8,2)"`
	LengthCm    *int
	WidthCm     *int
	HeightCm    *int
	Axles       *int16
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// DriverAvailability is a driver advertising free capacity on a corridor.
type DriverAvailability struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	DriverID           uuid.UUID  `gorm:"type:uuid;not null;index"`
	VehicleID          *uuid.UUID `gorm:"type:uuid"`
	FromAddressID      *uuid.UUID `gorm:"type:uuid"`
	AvailableFrom      time.Time
	AvailableTo        *time.Time
	PreferredRegionIDs UUIDArray `gorm:"type:uuid[]"`
	MaxDistanceKm      *int
	MinPriceAmount     *int64
	Currency           string `gorm:"size:3;not null;default:'UZS'"`
	Notes              *string
	IsActive           bool `gorm:"not null;default:true"`
	CreatedAt          time.Time
	UpdatedAt          time.Time

	Vehicle     *Vehicle `gorm:"foreignKey:VehicleID"`
	FromAddress *Address `gorm:"foreignKey:FromAddressID"`
}

// DriverPreferences drives matching and push targeting.
type DriverPreferences struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	DriverID            uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	MaxDistanceKm       int       `gorm:"not null;default:500"`
	PreferredCargoTypes UUIDArray `gorm:"type:uuid[]"`
	PreferredRegionIDs  UUIDArray `gorm:"type:uuid[]"`
	MinPriceAmount      *int64
	NotifyNewLoads      bool `gorm:"not null;default:true"`
	NotifyRadiusKm      int  `gorm:"not null;default:100"`
	AutoAcceptOffers    bool `gorm:"not null;default:false"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// DriverLocation is one GPS sample.
type DriverLocation struct {
	ID           int64      `gorm:"primaryKey"`
	DriverID     uuid.UUID  `gorm:"type:uuid;not null"`
	TripID       *uuid.UUID `gorm:"type:uuid"`
	Location     geo.Point  `gorm:"type:geography(Point,4326);not null"`
	SpeedKmh     *float64   `gorm:"type:numeric(6,2)"`
	Heading      *float64   `gorm:"type:numeric(6,2)"`
	AccuracyM    *float64   `gorm:"type:numeric(8,2)"`
	AltitudeM    *float64   `gorm:"type:numeric(8,2)"`
	BatteryLevel *int16
	IsMoving     bool `gorm:"not null;default:true"`
	RecordedAt   time.Time
	CreatedAt    time.Time
}

// TableName pins the partitioned table name.
func (DriverLocation) TableName() string { return "driver_locations" }

// ─── ports ──────────────────────────────────────────────────────────

// DriverSearchFilter narrows a nearby-driver search.
type DriverSearchFilter struct {
	Near           *geo.Point
	RadiusKm       float64
	VehicleTypes   []VehicleType
	MinCapacityKg  int
	MinVolumeM3    float64
	RequiresADR    bool
	RequiresRefrig bool
	MinRating      float64
	Statuses       []DriverStatus
	CompanyID      *uuid.UUID
	Verification   []VerificationStatus
	Search         string
	AvailableFrom  *time.Time
	RegionIDs      []uuid.UUID
}

// DriverSearchResult pairs a driver with the distance that produced the match.
type DriverSearchResult struct {
	Driver     Driver
	DistanceKm *float64
}

// DriverRepository persists driver profiles.
type DriverRepository interface {
	Create(ctx context.Context, driver *Driver) error
	Update(ctx context.Context, driver *Driver) error
	UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error
	FindByID(ctx context.Context, id uuid.UUID) (*Driver, error)
	FindByUserID(ctx context.Context, userID uuid.UUID) (*Driver, error)
	FindWithRelations(ctx context.Context, id uuid.UUID) (*Driver, error)
	Search(ctx context.Context, filter DriverSearchFilter, offset, limit int, order string) ([]DriverSearchResult, int64, error)
	UpdateLocation(ctx context.Context, driverID uuid.UUID, point geo.Point, recordedAt time.Time) error
	SetStatus(ctx context.Context, driverID uuid.UUID, status DriverStatus) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	RecalculateRating(ctx context.Context, driverID uuid.UUID) error
}

// DriverDocumentRepository persists driver papers.
type DriverDocumentRepository interface {
	Upsert(ctx context.Context, doc *DriverDocument) error
	ListByDriver(ctx context.Context, driverID uuid.UUID) ([]DriverDocument, error)
	FindByID(ctx context.Context, id uuid.UUID) (*DriverDocument, error)
	Review(ctx context.Context, id uuid.UUID, status VerificationStatus, reason *string, reviewedBy uuid.UUID) error
	ListPending(ctx context.Context, offset, limit int) ([]DriverDocument, int64, error)
	CountApprovedByTypes(ctx context.Context, driverID uuid.UUID, types []DocumentType) (int64, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

// VehicleRepository persists trucks and trailers.
type VehicleRepository interface {
	Create(ctx context.Context, vehicle *Vehicle) error
	Update(ctx context.Context, vehicle *Vehicle) error
	UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error
	FindByID(ctx context.Context, id uuid.UUID) (*Vehicle, error)
	ListByDriver(ctx context.Context, driverID uuid.UUID) ([]Vehicle, error)
	ListByCompany(ctx context.Context, companyID uuid.UUID, offset, limit int) ([]Vehicle, int64, error)
	ListPending(ctx context.Context, offset, limit int) ([]Vehicle, int64, error)
	SetPrimary(ctx context.Context, driverID, vehicleID uuid.UUID) error
	Review(ctx context.Context, id uuid.UUID, status VerificationStatus, reason *string) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	AddPhoto(ctx context.Context, photo *VehiclePhoto) error
	DeletePhoto(ctx context.Context, photoID uuid.UUID) error
	UpsertTrailer(ctx context.Context, trailer *Trailer) error
	CountApproved(ctx context.Context, driverID uuid.UUID) (int64, error)
}

// DriverPreferenceRepository persists availability and matching preferences.
type DriverPreferenceRepository interface {
	GetPreferences(ctx context.Context, driverID uuid.UUID) (*DriverPreferences, error)
	SavePreferences(ctx context.Context, prefs *DriverPreferences) error
	CreateAvailability(ctx context.Context, availability *DriverAvailability) error
	ListAvailability(ctx context.Context, driverID uuid.UUID, activeOnly bool) ([]DriverAvailability, error)
	DeactivateAvailability(ctx context.Context, id uuid.UUID) error
	FindAvailabilityByID(ctx context.Context, id uuid.UUID) (*DriverAvailability, error)
}

// LocationRepository stores GPS traces.
type LocationRepository interface {
	// InsertBatch writes a buffered batch of points in one statement. Locations
	// arrive every 15 seconds per active driver, so per-point transactions would
	// dominate the write load.
	InsertBatch(ctx context.Context, points []DriverLocation) error
	ListByTrip(ctx context.Context, tripID uuid.UUID, limit int) ([]DriverLocation, error)
	LatestByDriver(ctx context.Context, driverID uuid.UUID) (*DriverLocation, error)
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)
}
