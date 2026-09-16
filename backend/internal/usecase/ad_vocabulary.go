package usecase

import (
	"context"
	"time"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/internal/nlq"
	"github.com/loadify/backend/pkg/logger"
)

// adVocabularyTTL is generous because this data changes only on deploy, and the
// parse runs on every keystroke in the compose box.
const adVocabularyTTL = 6 * time.Hour

// adVocabulary adapts the reference data to the advert parser's port.
//
// It exists for the same reason the search vocabulary does: the parser depends
// on "a list of names I can resolve against" rather than on the geography
// repository. What differs is the breadth — an advert crosses borders, so the
// list is every city and region the platform knows rather than the fourteen
// Uzbek regions the marketplace search filters by.
type adVocabulary struct {
	geo       domain.GeoRepository
	cargo     domain.CargoTypeRepository
	reference *ReferenceUsecase
	cache     cache.Cache
	log       logger.Logger
}

// Places returns every resolvable location, cached whole. One list rather than
// a query per region is what keeps a parse off the database entirely once the
// cache is warm.
func (v adVocabulary) Places(ctx context.Context) ([]domain.Place, error) {
	const key = "ref:places"

	var cached []domain.Place
	if err := v.cache.Get(ctx, key, &cached); err == nil && len(cached) > 0 {
		return cached, nil
	}

	places, err := v.geo.ListPlaces(ctx)
	if err != nil {
		return nil, err
	}
	if err := v.cache.Set(ctx, key, places, adVocabularyTTL); err != nil {
		v.log.Warn("place vocabulary cache write failed", logger.Err(err))
	}
	return places, nil
}

// VehicleTypes reuses the search vocabulary: a body type is a body type
// whichever side of the product is asking.
func (v adVocabulary) VehicleTypes(ctx context.Context) ([]domain.Term, error) {
	return referenceVocabulary{reference: v.reference}.VehicleTypes(ctx)
}

// CargoTypes offers every localisation of a cargo name, plus the code itself,
// which is what appears in adverts written by dispatch software.
func (v adVocabulary) CargoTypes(ctx context.Context) ([]domain.Term, error) {
	const key = "ref:cargo-terms"

	var cached []domain.Term
	if err := v.cache.Get(ctx, key, &cached); err == nil && len(cached) > 0 {
		return cached, nil
	}

	types, err := v.cargo.List(ctx)
	if err != nil {
		return nil, err
	}

	terms := make([]domain.Term, 0, len(types))
	for _, cargo := range types {
		terms = append(terms, domain.Term{
			ID:      cargo.ID.String(),
			Aliases: []string{cargo.NameUZ, cargo.NameRU, cargo.NameEN, cargo.Code},
		})
	}
	if err := v.cache.Set(ctx, key, terms, adVocabularyTTL); err != nil {
		v.log.Warn("cargo vocabulary cache write failed", logger.Err(err))
	}
	return terms, nil
}

// NewAdParser builds the advertisement parser over the platform's own reference
// data.
//
// It lives here rather than in the composition root because the vocabulary
// adapter is unexported: what the parser needs is a capability, and the only
// thing that should know how that capability is assembled is this package.
func NewAdParser(
	geoRepo domain.GeoRepository,
	cargo domain.CargoTypeRepository,
	reference *ReferenceUsecase,
	c cache.Cache,
	timezone string,
	log logger.Logger,
) domain.LoadAdParser {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		// "ertaga" has to mean the person's tomorrow; the parser falls back to a
		// fixed +5 zone, which is right for this market and never silently UTC.
		log.Warn("timezone unavailable, the advert parser will use its fallback",
			logger.String("timezone", timezone), logger.Err(err))
		location = nil
	}
	vocabulary := adVocabulary{geo: geoRepo, cargo: cargo, reference: reference, cache: c, log: log}
	return nlq.NewAdParser(vocabulary, nil, location)
}
