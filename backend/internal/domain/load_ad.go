package domain

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LoadAdStatus is where an advertisement stands on its way to the groups.
type LoadAdStatus string

// Load advertisement states.
const (
	LoadAdDraft      LoadAdStatus = "draft"
	LoadAdReady      LoadAdStatus = "ready"
	LoadAdPublishing LoadAdStatus = "publishing"
	LoadAdPublished  LoadAdStatus = "published"
	LoadAdArchived   LoadAdStatus = "archived"
)

// FieldSource says who produced a value.
type FieldSource string

// Where a parsed value came from. The distinction is what lets the interface
// show a person which parts of their sentence were read by a rule and which by
// a model, and what lets us refuse a model value that nothing in the text
// supports.
const (
	FieldFromRule FieldSource = "rule"
	FieldFromAI   FieldSource = "ai"
	FieldFromUser FieldSource = "user"
)

// FieldOrigin is the provenance of one field: who resolved it and the words it
// was resolved from.
type FieldOrigin struct {
	Source   FieldSource `json:"source"`
	Evidence string      `json:"evidence,omitempty"`
}

// FieldSources maps a field name to its provenance. It is stored as JSONB.
type FieldSources map[string]FieldOrigin

// Value implements driver.Valuer.
func (f FieldSources) Value() (driver.Value, error) {
	if f == nil {
		return []byte("{}"), nil
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("encode field sources: %w", err)
	}
	return raw, nil
}

// Scan implements sql.Scanner.
func (f *FieldSources) Scan(src any) error {
	if src == nil {
		*f = FieldSources{}
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("field sources: unsupported type %T", src)
	}
	if len(raw) == 0 {
		*f = FieldSources{}
		return nil
	}
	return json.Unmarshal(raw, f)
}

// LoadAd is a freight offer as a logistician wrote it, resolved as far as the
// platform could resolve it.
//
// Nearly every field is optional on purpose. A Telegram advert routinely leaves
// out the volume, the cargo type or the price, and filling those in would be
// the parser inventing facts. What was not said stays nil, and the *Text fields
// keep the words the person used beside whatever we matched them to.
type LoadAd struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID       uuid.UUID `gorm:"type:uuid;not null;index"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;not null"`
	RawText         string    `gorm:"not null;default:''"`

	OriginCityID   *uuid.UUID `gorm:"type:uuid"`
	OriginRegionID *uuid.UUID `gorm:"type:uuid"`
	OriginCountry  *string    `gorm:"type:char(2)"`
	OriginText     string     `gorm:"size:120;not null;default:''"`
	DestCityID     *uuid.UUID `gorm:"type:uuid"`
	DestRegionID   *uuid.UUID `gorm:"type:uuid"`
	DestCountry    *string    `gorm:"type:char(2)"`
	DestText       string     `gorm:"size:120;not null;default:''"`

	CargoTypeID     *uuid.UUID `gorm:"type:uuid"`
	CargoText       *string    `gorm:"size:120"`
	WeightKg        *int
	VolumeM3        *float64   `gorm:"type:numeric(8,2)"`
	VehicleType     *string    `gorm:"size:24"`
	LoadingDate     *time.Time `gorm:"type:date"`
	LoadingDateText *string    `gorm:"size:64"`

	PriceAmount     *int64
	PriceCurrency   *string `gorm:"type:char(3)"`
	PriceNegotiable bool    `gorm:"not null;default:false"`
	PaymentTerms    *string `gorm:"size:16"`
	PaymentMethod   *string `gorm:"size:16"`

	ContactPhone *string `gorm:"size:20"`
	ContactName  *string `gorm:"size:120"`
	Notes        *string

	FieldSources      FieldSources `gorm:"type:jsonb;not null;default:'{}'"`
	Fingerprint       string       `gorm:"type:char(64);not null"`
	Status            LoadAdStatus `gorm:"size:16;not null;default:'draft'"`
	MarketplaceLoadID *uuid.UUID   `gorm:"type:uuid"`

	PublishedAt *time.Time
	ExpiresAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// TableName pins the advertisement table.
func (LoadAd) TableName() string { return "load_ads" }

// RequiredForPublish lists what a group needs to see for the post to be worth
// sending: where from, where to, and someone to call.
var RequiredForPublish = []string{"origin", "destination", "contact_phone"}

// MissingForPublish returns the required fields that are still empty, in the
// order a form would ask for them.
func (a *LoadAd) MissingForPublish() []string {
	var missing []string
	if a.OriginCityID == nil && a.OriginRegionID == nil && strings.TrimSpace(a.OriginText) == "" {
		missing = append(missing, "origin")
	}
	if a.DestCityID == nil && a.DestRegionID == nil && strings.TrimSpace(a.DestText) == "" {
		missing = append(missing, "destination")
	}
	if a.ContactPhone == nil || strings.TrimSpace(*a.ContactPhone) == "" {
		missing = append(missing, "contact_phone")
	}
	return missing
}

// CanPublish reports whether the advertisement may go to the queue.
func (a *LoadAd) CanPublish() bool {
	if len(a.MissingForPublish()) > 0 {
		return false
	}
	// An unsaved advertisement has no status yet; the column defaults to draft.
	switch a.Status {
	case "", LoadAdDraft, LoadAdReady, LoadAdPublished:
		return true
	default:
		return false
	}
}

// ComputeFingerprint digests what the advertisement means rather than how it
// was written, so re-typing the same offer with different wording, spacing or
// emoji still counts as the same advert at the duplicate check.
//
// Only the facts a reader would use to recognise a repeat are included: the
// corridor, the tonnage, the body, the loading day, the money and the phone
// number.
func (a *LoadAd) ComputeFingerprint() string {
	parts := []string{
		placeKey(a.OriginCityID, a.OriginRegionID, a.OriginText),
		placeKey(a.DestCityID, a.DestRegionID, a.DestText),
		optionalInt(a.WeightKg),
		normalise(stringOrEmpty(a.VehicleType)),
		dateKey(a.LoadingDate),
		optionalInt64(a.PriceAmount) + normalise(stringOrEmpty(a.PriceCurrency)),
		digitsOnly(stringOrEmpty(a.ContactPhone)),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func placeKey(cityID, regionID *uuid.UUID, text string) string {
	// A resolved place is the strongest key; the typed words are the fallback so
	// two unresolved spellings of the same city still collide when identical.
	switch {
	case cityID != nil:
		return "city:" + cityID.String()
	case regionID != nil:
		return "region:" + regionID.String()
	default:
		return "text:" + normalise(text)
	}
}

func normalise(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func digitsOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

func dateKey(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.DateOnly)
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func optionalInt(v *int) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(*v)
}

func optionalInt64(v *int64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(*v)
}

// ─── ports ──────────────────────────────────────────────────────────

// LoadAdFilter narrows a listing.
type LoadAdFilter struct {
	Statuses  []LoadAdStatus
	CreatedBy *uuid.UUID
	Search    string
	From      *time.Time
	To        *time.Time
}

// LoadAdRepository persists advertisements.
//
// Every method takes the company explicitly rather than reading it from the
// entity, so a missing tenant filter is a compile error rather than a leak.
type LoadAdRepository interface {
	Create(ctx context.Context, ad *LoadAd) error
	Update(ctx context.Context, ad *LoadAd) error
	UpdateColumns(ctx context.Context, companyID, id uuid.UUID, columns map[string]any) error
	FindByID(ctx context.Context, companyID, id uuid.UUID) (*LoadAd, error)
	List(ctx context.Context, companyID uuid.UUID, filter LoadAdFilter, offset, limit int) ([]LoadAd, int64, error)
	Archive(ctx context.Context, companyID, id uuid.UUID) error
	CountCreatedBetween(ctx context.Context, companyID uuid.UUID, from, to time.Time) (int64, error)
}
