package domain

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/pkg/geo"
)

// Country is a market the platform operates in.
type Country struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Code         string    `gorm:"size:2;not null;uniqueIndex"`
	Code3        string    `gorm:"size:3;not null"`
	NameUZ       string    `gorm:"size:100;not null"`
	NameRU       string    `gorm:"size:100;not null"`
	NameEN       string    `gorm:"size:100;not null"`
	PhoneCode    string    `gorm:"size:8;not null"`
	CurrencyCode string    `gorm:"size:3;not null"`
	FlagEmoji    *string   `gorm:"size:16"`
	IsActive     bool      `gorm:"not null;default:true"`
	SortOrder    int16     `gorm:"not null;default:0"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Region is an administrative division — the primary unit drivers and companies
// think in when describing a route ("Tashkent → Samarkand").
type Region struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CountryID uuid.UUID  `gorm:"type:uuid;not null;index"`
	Code      string     `gorm:"size:16;not null"`
	NameUZ    string     `gorm:"size:100;not null"`
	NameRU    string     `gorm:"size:100;not null"`
	NameEN    string     `gorm:"size:100;not null"`
	Center    *geo.Point `gorm:"type:geography(Point,4326)"`
	IsActive  bool       `gorm:"not null;default:true"`
	SortOrder int16      `gorm:"not null;default:0"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Country *Country `gorm:"foreignKey:CountryID"`
}

// City is a settlement inside a region.
type City struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	RegionID   uuid.UUID  `gorm:"type:uuid;not null;index"`
	NameUZ     string     `gorm:"size:100;not null"`
	NameRU     string     `gorm:"size:100;not null"`
	NameEN     string     `gorm:"size:100;not null"`
	Location   *geo.Point `gorm:"type:geography(Point,4326)"`
	IsCapital  bool       `gorm:"not null;default:false"`
	Population *int
	IsActive   bool `gorm:"not null;default:true"`
	CreatedAt  time.Time
	UpdatedAt  time.Time

	Region *Region `gorm:"foreignKey:RegionID"`
}

// Address is a resolved pickup or delivery point.
type Address struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CountryID     *uuid.UUID `gorm:"type:uuid"`
	RegionID      *uuid.UUID `gorm:"type:uuid;index"`
	CityID        *uuid.UUID `gorm:"type:uuid;index"`
	Line1         string     `gorm:"size:255;not null;default:''"`
	Line2         *string    `gorm:"size:255"`
	PostalCode    *string    `gorm:"size:20"`
	Formatted     string     `gorm:"size:512;not null;default:''"`
	GooglePlaceID *string    `gorm:"size:255"`
	Location      geo.Point  `gorm:"type:geography(Point,4326);not null"`
	ContactName   *string    `gorm:"size:120"`
	ContactPhone  *string    `gorm:"size:20"`
	Notes         *string
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Region *Region `gorm:"foreignKey:RegionID"`
	City   *City   `gorm:"foreignKey:CityID"`
}

// LocalisedName picks the field matching the requested locale.
func LocalisedName(uz, ru, en string, locale Locale) string {
	switch locale {
	case LocaleRU:
		return ru
	case LocaleEN:
		return en
	default:
		return uz
	}
}

// GeoRepository serves the address book and reference geography.
type GeoRepository interface {
	ListCountries(ctx context.Context) ([]Country, error)
	ListRegions(ctx context.Context, countryCode string) ([]Region, error)
	ListCities(ctx context.Context, regionID *uuid.UUID, search string, limit int) ([]City, error)
	FindRegionByID(ctx context.Context, id uuid.UUID) (*Region, error)
	FindCityByID(ctx context.Context, id uuid.UUID) (*City, error)
	// NearestRegion resolves free-form coordinates to the region they fall in,
	// so a load pinned on a map is still filterable by region.
	NearestRegion(ctx context.Context, point geo.Point) (*Region, error)
	CreateAddress(ctx context.Context, address *Address) error
	FindAddressByID(ctx context.Context, id uuid.UUID) (*Address, error)
}
