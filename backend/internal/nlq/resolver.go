package nlq

import (
	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
)

// Resolver answers one question: does this string name something the platform
// knows?
//
// The model-backed pass needs exactly that and nothing more. A model proposes
// words — "Москва", "реф" — and the server looks them up here; an identifier is
// never taken from a model, so a name nothing recognises becomes a reported gap
// rather than a filter pointing at a place that does not exist.
type Resolver struct {
	places   *AliasIndex[uuid.UUID]
	byID     map[uuid.UUID]domain.Place
	vehicles *AliasIndex[domain.VehicleType]
	cargo    *AliasIndex[uuid.UUID]
}

// NewResolver indexes the vocabularies a resolution may draw on.
func NewResolver(places []domain.Place, vehicles, cargoTypes []domain.Term) *Resolver {
	r := &Resolver{
		places:   NewAliasIndex[uuid.UUID](),
		byID:     make(map[uuid.UUID]domain.Place, len(places)),
		vehicles: NewAliasIndex[domain.VehicleType](),
		cargo:    NewAliasIndex[uuid.UUID](),
	}

	for _, place := range places {
		if place.ID == uuid.Nil {
			continue
		}
		r.byID[place.ID] = place
		r.places.Add(place.ID, place.Aliases)
	}
	for _, term := range vehicles {
		r.vehicles.Add(domain.VehicleType(term.ID), term.Aliases)
	}
	for code, aliases := range colloquialAliases {
		r.vehicles.Add(code, aliases)
	}
	for _, term := range cargoTypes {
		if id, err := uuid.Parse(term.ID); err == nil {
			r.cargo.Add(id, term.Aliases)
		}
	}
	return r
}

// Place resolves a place name.
func (r *Resolver) Place(name string) (domain.Place, bool) {
	id, ok := resolveWhole(r.places, name)
	if !ok {
		return domain.Place{}, false
	}
	place, known := r.byID[id]
	return place, known
}

// Vehicle resolves a body type.
func (r *Resolver) Vehicle(name string) (domain.VehicleType, bool) {
	return resolveWhole(r.vehicles, name)
}

// Cargo resolves a cargo type.
func (r *Resolver) Cargo(name string) (uuid.UUID, bool) {
	return resolveWhole(r.cargo, name)
}

// resolveWhole requires the match to consume the entire string.
//
// A partial match would be the loose end a model exploits without meaning to:
// "Moscow region warehouse 4" must not resolve to Moscow, because the person
// who wrote it may well have meant something the platform has no row for.
func resolveWhole[T comparable](index *AliasIndex[T], name string) (T, bool) {
	var zero T
	scan := NewScan(name)
	if len(scan.Tokens) == 0 {
		return zero, false
	}

	id, end, suffix, ok := index.MatchAt(scan, 0)
	if !ok || end != len(scan.Tokens) {
		return zero, false
	}
	// A case ending is part of the name as it was written — "Moskvaga" is still
	// Moscow — so it does not disqualify the match.
	_ = suffix
	return id, true
}
