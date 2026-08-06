package domain

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/pkg/geo"
)

// LoadStatus is where a load sits in its lifecycle.
type LoadStatus string

const (
	LoadDraft     LoadStatus = "draft"
	LoadPublished LoadStatus = "published"
	LoadReserved  LoadStatus = "reserved"
	LoadAssigned  LoadStatus = "assigned"
	LoadInTransit LoadStatus = "in_transit"
	LoadDelivered LoadStatus = "delivered"
	LoadCompleted LoadStatus = "completed"
	LoadCancelled LoadStatus = "cancelled"
	LoadExpired   LoadStatus = "expired"
)

// loadTransitions is the whole load state machine in one place. A transition
// that is not listed here cannot happen, whatever the caller believes.
var loadTransitions = map[LoadStatus][]LoadStatus{
	LoadDraft:     {LoadPublished, LoadCancelled},
	LoadPublished: {LoadReserved, LoadAssigned, LoadDraft, LoadCancelled, LoadExpired},
	LoadReserved:  {LoadPublished, LoadAssigned, LoadCancelled},
	LoadAssigned:  {LoadInTransit, LoadPublished, LoadCancelled},
	LoadInTransit: {LoadDelivered, LoadCancelled},
	LoadDelivered: {LoadCompleted},
	LoadCompleted: {},
	LoadCancelled: {},
	LoadExpired:   {LoadPublished, LoadDraft},
}

// CanTransitionTo reports whether the load may move to the target status.
func (s LoadStatus) CanTransitionTo(target LoadStatus) bool {
	return slices.Contains(loadTransitions[s], target)
}

// IsOpen reports whether the load is still looking for a driver.
func (s LoadStatus) IsOpen() bool {
	return s == LoadPublished || s == LoadReserved
}

// IsActive reports whether the load is live — published or being carried. Used
// to count a company's concurrent load usage against its plan.
func (s LoadStatus) IsActive() bool {
	switch s {
	case LoadPublished, LoadReserved, LoadAssigned, LoadInTransit:
		return true
	default:
		return false
	}
}

// PriceType is how the shipper expressed the rate.
type PriceType string

const (
	PriceFixed      PriceType = "fixed"
	PriceNegotiable PriceType = "negotiable"
	PricePerKm      PriceType = "per_km"
)

// PaymentTerms is when the driver gets paid.
type PaymentTerms string

const (
	PayOnLoading  PaymentTerms = "on_loading"
	PayOnDelivery PaymentTerms = "on_delivery"
	PayPrepayment PaymentTerms = "prepayment"
	PayDeferred   PaymentTerms = "deferred"
)

// CargoType classifies what is being shipped.
type CargoType struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Code                string    `gorm:"size:32;not null;uniqueIndex"`
	NameUZ              string    `gorm:"size:100;not null"`
	NameRU              string    `gorm:"size:100;not null"`
	NameEN              string    `gorm:"size:100;not null"`
	Icon                *string   `gorm:"size:64"`
	RequiresADR         bool      `gorm:"column:requires_adr;not null;default:false"`
	RequiresTemperature bool      `gorm:"not null;default:false"`
	IsActive            bool      `gorm:"not null;default:true"`
	SortOrder           int16     `gorm:"not null;default:0"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Load is a shipment a company wants carried.
type Load struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID       uuid.UUID `gorm:"type:uuid;not null;index"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;not null"`
	ReferenceNo     string    `gorm:"size:24;not null"`
	Title           string    `gorm:"size:255;not null"`
	Description     *string
	CargoTypeID     *uuid.UUID `gorm:"type:uuid"`

	WeightKg  int      `gorm:"not null"`
	VolumeM3  *float64 `gorm:"type:numeric(8,2)"`
	LengthCm  *int
	WidthCm   *int
	HeightCm  *int
	Packaging string `gorm:"size:16;not null;default:'other'"`
	Quantity  *int

	PickupAddressID   uuid.UUID  `gorm:"type:uuid;not null"`
	DeliveryAddressID uuid.UUID  `gorm:"type:uuid;not null"`
	PickupLocation    geo.Point  `gorm:"type:geography(Point,4326);not null"`
	DeliveryLocation  geo.Point  `gorm:"type:geography(Point,4326);not null"`
	PickupRegionID    *uuid.UUID `gorm:"type:uuid"`
	DeliveryRegionID  *uuid.UUID `gorm:"type:uuid"`
	PickupFrom        time.Time  `gorm:"not null"`
	PickupTo          *time.Time
	DeliveryFrom      *time.Time
	DeliveryTo        *time.Time
	DistanceKm        *float64 `gorm:"type:numeric(10,2)"`

	RequiredVehicleType   *VehicleType `gorm:"size:24"`
	RequiredCapacityKg    *int
	RequiresADR           bool `gorm:"column:requires_adr;not null;default:false"`
	RequiresRefrigeration bool `gorm:"not null;default:false"`
	TempMinC              *int16
	TempMaxC              *int16
	RequiresTailLift      bool   `gorm:"not null;default:false"`
	LoadingType           string `gorm:"size:16;not null;default:'rear'"`
	SpecialRequirements   *string

	PriceAmount       int64        `gorm:"not null"`
	PriceCurrency     string       `gorm:"size:3;not null;default:'UZS'"`
	PriceType         PriceType    `gorm:"size:12;not null;default:'fixed'"`
	PaymentTerms      PaymentTerms `gorm:"size:16;not null;default:'on_delivery'"`
	PaymentMethod     string       `gorm:"size:16;not null;default:'cash'"`
	PrepaymentPercent *int16
	DeferredDays      *int16

	Status             LoadStatus `gorm:"size:16;not null;default:'draft'"`
	PublishedAt        *time.Time
	ExpiresAt          *time.Time
	CancelledAt        *time.Time
	CancellationReason *string

	ViewsCount        int     `gorm:"not null;default:0"`
	ReservationsCount int     `gorm:"not null;default:0"`
	IsFeatured        bool    `gorm:"not null;default:false"`
	IsUrgent          bool    `gorm:"not null;default:false"`
	DuplicateHash     *string `gorm:"size:64"`
	FraudScore        float64 `gorm:"type:numeric(5,2);not null;default:0"`

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	Company         *Company       `gorm:"foreignKey:CompanyID"`
	CargoType       *CargoType     `gorm:"foreignKey:CargoTypeID"`
	PickupAddress   *Address       `gorm:"foreignKey:PickupAddressID"`
	DeliveryAddress *Address       `gorm:"foreignKey:DeliveryAddressID"`
	Photos          []LoadPhoto    `gorm:"foreignKey:LoadID"`
	Documents       []LoadDocument `gorm:"foreignKey:LoadID"`
}

// Requirements projects the load onto the shape Vehicle.CanCarry expects, so
// compatibility is decided by one rule set on both sides of the marketplace.
func (l *Load) Requirements() CargoRequirements {
	return CargoRequirements{
		WeightKg:              l.WeightKg,
		VolumeM3:              l.VolumeM3,
		LengthCm:              l.LengthCm,
		WidthCm:               l.WidthCm,
		HeightCm:              l.HeightCm,
		VehicleType:           l.RequiredVehicleType,
		RequiresADR:           l.RequiresADR,
		RequiresRefrigeration: l.RequiresRefrigeration,
		RequiresTailLift:      l.RequiresTailLift,
		TempMinC:              l.TempMinC,
		TempMaxC:              l.TempMaxC,
	}
}

// IsExpired reports whether a published load has passed its expiry.
func (l *Load) IsExpired(now time.Time) bool {
	return l.ExpiresAt != nil && now.After(*l.ExpiresAt)
}

// PricePerKm returns the rate per kilometre, or nil when the distance is unknown.
func (l *Load) PricePerKm() *float64 {
	if l.DistanceKm == nil || *l.DistanceKm <= 0 {
		return nil
	}
	rate := float64(l.PriceAmount) / *l.DistanceKm
	return &rate
}

// LoadPhoto is one image of the cargo.
type LoadPhoto struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	LoadID    uuid.UUID `gorm:"type:uuid;not null;index"`
	FileID    uuid.UUID `gorm:"type:uuid;not null"`
	SortOrder int16     `gorm:"not null;default:0"`
	CreatedAt time.Time

	File *File `gorm:"foreignKey:FileID"`
}

// LoadDocument is a paper attached to the shipment.
type LoadDocument struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	LoadID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	FileID     uuid.UUID  `gorm:"type:uuid;not null"`
	Type       string     `gorm:"size:32;not null;default:'other'"`
	Name       *string    `gorm:"size:255"`
	UploadedBy *uuid.UUID `gorm:"type:uuid"`
	CreatedAt  time.Time

	File *File `gorm:"foreignKey:FileID"`
}

// LoadView records that someone opened a load, for analytics and the view counter.
type LoadView struct {
	ID        int64      `gorm:"primaryKey"`
	LoadID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	UserID    *uuid.UUID `gorm:"type:uuid"`
	IPAddress *string    `gorm:"type:inet"`
	ViewedAt  time.Time
}

// TableName pins the table for the view log.
func (LoadView) TableName() string { return "load_views" }

// SavedLoad is a driver's bookmark.
type SavedLoad struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	LoadID    uuid.UUID `gorm:"type:uuid;not null"`
	CreatedAt time.Time

	Load *Load `gorm:"foreignKey:LoadID"`
}

// SavedSearch is a stored filter that can push new matching loads.
type SavedSearch struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index"`
	Name           string    `gorm:"size:100;not null"`
	Filters        JSONMap   `gorm:"type:jsonb;not null;default:'{}'"`
	NotifyEnabled  bool      `gorm:"not null;default:true"`
	LastNotifiedAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ─── ports ──────────────────────────────────────────────────────────

// LoadFilter is the full set of marketplace search criteria.
type LoadFilter struct {
	CompanyID             *uuid.UUID
	CreatedByUserID       *uuid.UUID
	Statuses              []LoadStatus
	CargoTypeIDs          []uuid.UUID
	PickupRegionIDs       []uuid.UUID
	DeliveryRegionIDs     []uuid.UUID
	PickupNear            *geo.Point
	PickupRadiusKm        float64
	DeliveryNear          *geo.Point
	DeliveryRadiusKm      float64
	MinWeightKg           int
	MaxWeightKg           int
	MinVolumeM3           float64
	MaxVolumeM3           float64
	VehicleTypes          []VehicleType
	RequiresADR           *bool
	RequiresRefrigeration *bool
	RequiresTailLift      *bool
	MinPriceAmount        int64
	MaxPriceAmount        int64
	Currency              string
	PriceTypes            []PriceType
	PaymentTerms          []PaymentTerms
	PickupFrom            *time.Time
	PickupTo              *time.Time
	FeaturedOnly          bool
	UrgentOnly            bool
	Search                string
	// FitsVehicleID narrows results to loads a specific truck can legally carry,
	// evaluated in SQL so paging stays correct.
	FitsVehicleID  *uuid.UUID
	ExcludeExpired bool
}

// LoadSearchResult pairs a load with distances computed by the query.
type LoadSearchResult struct {
	Load             Load
	PickupDistanceKm *float64
}

// LoadRepository persists loads and their attachments.
type LoadRepository interface {
	Create(ctx context.Context, load *Load) error
	Update(ctx context.Context, load *Load) error
	UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error
	FindByID(ctx context.Context, id uuid.UUID) (*Load, error)
	FindWithRelations(ctx context.Context, id uuid.UUID) (*Load, error)
	FindByReference(ctx context.Context, reference string) (*Load, error)
	Search(ctx context.Context, filter LoadFilter, offset, limit int, order string) ([]LoadSearchResult, int64, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error

	// TransitionStatus moves a load between states, failing when the current
	// status is not one of `from`. The guard is in the UPDATE so two concurrent
	// requests cannot both win.
	TransitionStatus(ctx context.Context, id uuid.UUID, from []LoadStatus, to LoadStatus, columns map[string]any) error
	CountActiveByCompany(ctx context.Context, companyID uuid.UUID) (int64, error)
	IncrementViews(ctx context.Context, id uuid.UUID) error
	RecordView(ctx context.Context, view *LoadView) error
	ExpireOverdue(ctx context.Context, now time.Time) (int64, error)
	FindDuplicate(ctx context.Context, companyID uuid.UUID, hash string) (*Load, error)

	AddPhoto(ctx context.Context, photo *LoadPhoto) error
	DeletePhoto(ctx context.Context, photoID uuid.UUID) error
	AddDocument(ctx context.Context, doc *LoadDocument) error
	DeleteDocument(ctx context.Context, docID uuid.UUID) error
	ReplacePhotos(ctx context.Context, loadID uuid.UUID, fileIDs []uuid.UUID) error
}

// CargoTypeRepository serves the cargo-type vocabulary.
type CargoTypeRepository interface {
	List(ctx context.Context) ([]CargoType, error)
	FindByID(ctx context.Context, id uuid.UUID) (*CargoType, error)
}

// SavedRepository persists bookmarks and stored searches.
type SavedRepository interface {
	SaveLoad(ctx context.Context, userID, loadID uuid.UUID) error
	UnsaveLoad(ctx context.Context, userID, loadID uuid.UUID) error
	ListSavedLoads(ctx context.Context, userID uuid.UUID, offset, limit int) ([]SavedLoad, int64, error)
	IsSaved(ctx context.Context, userID uuid.UUID, loadIDs []uuid.UUID) (map[uuid.UUID]bool, error)

	CreateSearch(ctx context.Context, search *SavedSearch) error
	UpdateSearch(ctx context.Context, search *SavedSearch) error
	ListSearches(ctx context.Context, userID uuid.UUID) ([]SavedSearch, error)
	FindSearchByID(ctx context.Context, id uuid.UUID) (*SavedSearch, error)
	DeleteSearch(ctx context.Context, id uuid.UUID) error
	// ListNotifiableSearches feeds the matcher that pushes new loads to drivers.
	ListNotifiableSearches(ctx context.Context, limit int) ([]SavedSearch, error)
	MarkNotified(ctx context.Context, id uuid.UUID, at time.Time) error
}
