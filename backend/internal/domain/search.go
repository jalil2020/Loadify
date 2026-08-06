package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SearchIntent is what someone meant, resolved against the platform's own
// vocabulary.
//
// It is deliberately neutral about which side of the market is asking. A load
// search and a driver search are duals of each other — the same sentence
// ("Toshkentdan Samarqandga, 20 tonna, tent") describes a corridor either as
// freight to be carried or as capacity to carry it. Storing the meaning once
// and projecting it twice is what keeps the two sides from drifting apart.
//
// Every identifier here came from a lookup, never from a model: an id the
// parser could not resolve is reported in Dropped instead of guessed.
type SearchIntent struct {
	OriginRegionIDs      []uuid.UUID
	DestinationRegionIDs []uuid.UUID
	VehicleTypes         []VehicleType

	// WeightKg is a tonnage the speaker stated without saying which way it
	// bounds the search, and it is the clearest case of the duality: for a
	// driver it is the capacity they have, so it caps the loads worth showing;
	// for a shipper it is the cargo they hold, so it floors the vehicles worth
	// showing. The projection decides, not the parser.
	WeightKg *int
	// MinWeightKg and MaxWeightKg are set only when the speaker was explicit
	// ("20 tonnadan ortiq", "10 tonnagacha").
	MinWeightKg *int
	MaxWeightKg *int

	// Money in minor units, matching every other amount on the platform.
	MinPrice *int64
	MaxPrice *int64
	Currency string

	RequiresADR           *bool
	RequiresRefrigeration *bool

	ReadyFrom *time.Time
	ReadyTo   *time.Time

	// Dropped holds the words that were understood as meaningful but could not
	// be resolved. Surfacing them is what lets someone see that "Qarshi" was
	// ignored rather than silently widening their search.
	Dropped []string
}

// IsEmpty reports whether nothing at all was understood, which is the signal to
// leave the existing filters alone rather than replace them with nothing.
func (i SearchIntent) IsEmpty() bool {
	return len(i.OriginRegionIDs) == 0 &&
		len(i.DestinationRegionIDs) == 0 &&
		len(i.VehicleTypes) == 0 &&
		i.WeightKg == nil &&
		i.MinWeightKg == nil &&
		i.MaxWeightKg == nil &&
		i.MinPrice == nil &&
		i.MaxPrice == nil &&
		i.RequiresADR == nil &&
		i.RequiresRefrigeration == nil &&
		i.ReadyFrom == nil &&
		i.ReadyTo == nil
}

// Term is one entry of a closed vocabulary the parser resolves names against.
//
// Aliases carries every spelling the platform accepts for the same thing —
// all three languages, both scripts, and the informal names drivers actually
// use. Matching is done on folded forms, so the list holds the words rather
// than their normalisations.
type Term struct {
	ID      string
	Aliases []string
}

// Vocabulary supplies the closed lists that make this parseable without a
// model: fourteen regions and twelve vehicle types, not an open catalogue.
//
// It is an interface so the parser depends on the capability rather than on the
// geography repository, and so tests can hand it a fixed list instead of a
// database.
type Vocabulary interface {
	Regions(ctx context.Context) ([]Term, error)
	VehicleTypes(ctx context.Context) ([]Term, error)
}
