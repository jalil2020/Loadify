package domain

import (
	"context"

	"github.com/google/uuid"
)

// PlaceKind distinguishes the two things a corridor end can resolve to.
type PlaceKind string

// Kinds of place.
const (
	PlaceCity   PlaceKind = "city"
	PlaceRegion PlaceKind = "region"
)

// Place is one location the parser may resolve a name to.
//
// A corridor in this business crosses borders — Tashkent to Moscow, Almaty to
// Bishkek — so the vocabulary is not the fourteen Uzbek regions the marketplace
// search uses. A city carries its region and country so a group whose rule says
// "anything to Russia" still matches an advert that named only Moscow.
type Place struct {
	ID          uuid.UUID
	Kind        PlaceKind
	CountryCode string
	RegionID    *uuid.UUID
	Aliases     []string
}

// AdDraft is the result of reading a written advertisement.
//
// It is a draft rather than a load: nothing is stored until a person has seen
// what was understood. Sources says which rule or model produced each field and
// which words it came from, Missing says what still has to be supplied before
// the advert can be posted, and Dropped holds the words nothing understood —
// surfaced rather than swallowed, so a misread is visible.
type AdDraft struct {
	Ad      LoadAd
	Sources FieldSources
	Missing []string
	Dropped []string
	AIUsed  bool
}

// Field names used in Sources and Missing. They are the wire contract too, so
// the client can label a chip without a second vocabulary.
const (
	FieldOrigin       = "origin"
	FieldDestination  = "destination"
	FieldCargoType    = "cargo_type"
	FieldWeight       = "weight_kg"
	FieldVolume       = "volume_m3"
	FieldVehicleType  = "vehicle_type"
	FieldLoadingDate  = "loading_date"
	FieldPrice        = "price"
	FieldPaymentTerms = "payment_terms"
	FieldContactPhone = "contact_phone"
)

// Note records where a field came from.
func (d *AdDraft) Note(field string, source FieldSource, evidence string) {
	if d.Sources == nil {
		d.Sources = FieldSources{}
	}
	d.Sources[field] = FieldProvenance{Source: source, Evidence: evidence}
}

// Has reports whether a field was already filled by an earlier rule. The
// model-backed pass uses it to stay off anything the rules resolved.
func (d *AdDraft) Has(field string) bool {
	_, found := d.Sources[field]
	return found
}

// AdVocabulary supplies the closed lists an advertisement is resolved against.
//
// It is an interface for the same reason the search vocabulary is: the parser
// depends on the capability, not on the geography repository, and a test hands
// it a fixed list instead of a database.
type AdVocabulary interface {
	Places(ctx context.Context) ([]Place, error)
	VehicleTypes(ctx context.Context) ([]Term, error)
	CargoTypes(ctx context.Context) ([]Term, error)
}

// LoadAdParser turns what someone wrote into a draft.
type LoadAdParser interface {
	Parse(ctx context.Context, text string) (AdDraft, error)
}
