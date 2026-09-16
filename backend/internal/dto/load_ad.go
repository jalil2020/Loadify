package dto

import (
	"time"

	"github.com/google/uuid"
)

// ParseAdRequest carries an advertisement as it was written.
type ParseAdRequest struct {
	Text string `json:"text" validate:"required,min=3,max=4000"`
}

// ParsedField is one value the parser resolved, with the words it read it from.
//
// Provenance travels with the value because the interface shows it: a chip a
// person can see and remove is what makes a misread correctable before a
// hundred groups see it.
type ParsedField struct {
	Value    any    `json:"value"`
	Source   string `json:"source,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

// ParseAdResponse is what was understood, what is still needed, and what was
// not understood at all.
type ParseAdResponse struct {
	Fields  map[string]ParsedField `json:"fields"`
	Missing []string               `json:"missing"`
	Dropped []string               `json:"dropped"`
	AIUsed  bool                   `json:"ai_used"`
	// Draft is the same content in the shape the create endpoint accepts, so a
	// client edits what it was shown and posts it straight back.
	Draft SaveLoadAdRequest `json:"draft"`
}

// SaveLoadAdRequest is an advertisement a person has confirmed.
//
// Almost everything is optional because an advert that omits the volume or the
// cargo type is normal, and requiring a value would only invite an invented one.
// What is required is what a group needs to act on the post: where from, where
// to, and someone to call.
type SaveLoadAdRequest struct {
	RawText string `json:"raw_text" validate:"omitempty,max=4000"`

	OriginCityID   *uuid.UUID `json:"origin_city_id"`
	OriginRegionID *uuid.UUID `json:"origin_region_id"`
	OriginCountry  *string    `json:"origin_country" validate:"omitempty,len=2"`
	OriginText     string     `json:"origin_text" validate:"omitempty,max=120"`
	DestCityID     *uuid.UUID `json:"destination_city_id"`
	DestRegionID   *uuid.UUID `json:"destination_region_id"`
	DestCountry    *string    `json:"destination_country" validate:"omitempty,len=2"`
	DestText       string     `json:"destination_text" validate:"omitempty,max=120"`

	CargoTypeID     *uuid.UUID `json:"cargo_type_id"`
	CargoText       *string    `json:"cargo_text" validate:"omitempty,max=120"`
	WeightKg        *int       `json:"weight_kg" validate:"omitempty,gt=0,lte=100000"`
	VolumeM3        *float64   `json:"volume_m3" validate:"omitempty,gt=0,lte=200"`
	VehicleType     *string    `json:"vehicle_type" validate:"omitempty,max=24"`
	LoadingDate     *string    `json:"loading_date" validate:"omitempty,datetime=2006-01-02"`
	LoadingDateText *string    `json:"loading_date_text" validate:"omitempty,max=64"`

	PriceAmount     *int64  `json:"price_amount" validate:"omitempty,gte=0"`
	PriceCurrency   *string `json:"price_currency" validate:"omitempty,len=3"`
	PriceNegotiable bool    `json:"price_negotiable"`
	PaymentTerms    *string `json:"payment_terms" validate:"omitempty,oneof=on_loading on_delivery prepayment deferred"`
	PaymentMethod   *string `json:"payment_method" validate:"omitempty,oneof=cash transfer card escrow"`

	ContactPhone *string `json:"contact_phone" validate:"omitempty,phone"`
	ContactName  *string `json:"contact_name" validate:"omitempty,max=120"`
	Notes        *string `json:"notes" validate:"omitempty,max=2000"`
}

// LoadAdResponse is a stored advertisement.
type LoadAdResponse struct {
	ID        uuid.UUID `json:"id"`
	CompanyID uuid.UUID `json:"company_id"`
	Status    string    `json:"status"`
	RawText   string    `json:"raw_text,omitempty"`

	Origin      PlaceRefResponse `json:"origin"`
	Destination PlaceRefResponse `json:"destination"`

	CargoTypeID     *uuid.UUID `json:"cargo_type_id,omitempty"`
	CargoText       *string    `json:"cargo_text,omitempty"`
	WeightKg        *int       `json:"weight_kg,omitempty"`
	VolumeM3        *float64   `json:"volume_m3,omitempty"`
	VehicleType     *string    `json:"vehicle_type,omitempty"`
	LoadingDate     *string    `json:"loading_date,omitempty"`
	LoadingDateText *string    `json:"loading_date_text,omitempty"`

	PriceAmount     *int64  `json:"price_amount,omitempty"`
	PriceCurrency   *string `json:"price_currency,omitempty"`
	PriceNegotiable bool    `json:"price_negotiable"`
	PaymentTerms    *string `json:"payment_terms,omitempty"`
	PaymentMethod   *string `json:"payment_method,omitempty"`

	ContactPhone *string `json:"contact_phone,omitempty"`
	ContactName  *string `json:"contact_name,omitempty"`
	Notes        *string `json:"notes,omitempty"`

	// Missing is recomputed on read: a group's rules or a plan may change what
	// an advert still needs, and a stale list would be worse than none.
	Missing     []string   `json:"missing"`
	Fingerprint string     `json:"fingerprint"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// PlaceRefResponse is one end of the corridor: what we resolved, and the words
// the person wrote.
type PlaceRefResponse struct {
	CityID   *uuid.UUID `json:"city_id,omitempty"`
	RegionID *uuid.UUID `json:"region_id,omitempty"`
	Country  *string    `json:"country,omitempty"`
	Text     string     `json:"text,omitempty"`
}

// LoadAdQuery filters the advertisement listing.
type LoadAdQuery struct {
	Status string `query:"status" validate:"omitempty,oneof=draft ready publishing published archived"`
	Mine   bool   `query:"mine"`
	Search string `query:"search" validate:"omitempty,max=120"`
}
