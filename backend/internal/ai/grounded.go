// Package ai puts a model behind the rules, without letting it decide anything
// the rules could have decided.
//
// The arrangement is deliberate and narrow. The rules read the advert first. A
// model is consulted only when a required field is still empty *and* there are
// words nothing understood — otherwise there is nothing for it to find and no
// reason to spend a request. What comes back is treated as a pointer, not an
// answer: the model says which words are the origin or the rate, and the same
// rule that would have read those words computes the value. A name is resolved
// against the platform's own vocabulary, so an identifier is never invented,
// and a fragment that does not appear in the advert is discarded outright.
package ai

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/nlq"
	"github.com/loadify/backend/pkg/logger"
)

// GroundedParser reads an advert with the rules, then asks a model about what
// is left.
type GroundedParser struct {
	rules      domain.LoadAdParser
	extractor  domain.AdExtractor
	vocabulary domain.AdVocabulary
	log        logger.Logger
}

// NewGroundedParser wraps a rules parser. A nil extractor is not an error: the
// platform works without a model, and the fields the rules missed are simply
// reported for a person to fill in.
func NewGroundedParser(
	rules domain.LoadAdParser,
	extractor domain.AdExtractor,
	vocabulary domain.AdVocabulary,
	log logger.Logger,
) *GroundedParser {
	return &GroundedParser{rules: rules, extractor: extractor, vocabulary: vocabulary, log: log}
}

// Parse reads the advert.
func (p *GroundedParser) Parse(ctx context.Context, text string) (domain.AdDraft, error) {
	draft, err := p.rules.Parse(ctx, text)
	if err != nil {
		return draft, err
	}
	if p.extractor == nil {
		return draft, nil
	}
	if !worthAsking(draft) {
		draft.AIStatus = domain.AIStatusNotNeeded
		return draft, nil
	}

	result, err := p.extractor.Extract(ctx, domain.AdExtractRequest{
		Text:       text,
		Missing:    draft.Missing,
		Unresolved: draft.Dropped,
	})
	switch {
	case errors.Is(err, domain.ErrAIQuotaExhausted), errors.Is(err, domain.ErrAIBudgetExhausted):
		// Expected, and already recorded; the person is told the model sat
		// this one out.
		draft.AIStatus = domain.AIStatusLimited
		return draft, nil
	case err != nil:
		// A model outage degrades the reading; it never fails the request. The
		// person still sees what the rules understood and what is missing.
		p.log.Warn("advert extraction failed, keeping the rules result", logger.Err(err))
		draft.AIStatus = domain.AIStatusUnavailable
		return draft, nil
	}
	draft.AIStatus = domain.AIStatusNoResult
	if len(result.Fields) == 0 {
		return draft, nil
	}

	resolver, err := p.resolver(ctx)
	if err != nil {
		return draft, err
	}

	applied := p.apply(&draft, text, result, resolver)
	if applied == 0 {
		return draft, nil
	}

	draft.AIUsed = true
	draft.AIStatus = domain.AIStatusUsed
	draft.Ad.Fingerprint = draft.Ad.ComputeFingerprint()
	draft.Missing = draft.Ad.MissingFields()
	return draft, nil
}

// worthAsking keeps the model off adverts it cannot improve: one the rules read
// completely, and one where nothing was left unread for it to interpret.
func worthAsking(draft domain.AdDraft) bool {
	return len(draft.Missing) > 0 && len(draft.Dropped) > 0
}

func (p *GroundedParser) resolver(ctx context.Context) (*nlq.Resolver, error) {
	places, err := p.vocabulary.Places(ctx)
	if err != nil {
		return nil, err
	}
	vehicles, err := p.vocabulary.VehicleTypes(ctx)
	if err != nil {
		return nil, err
	}
	cargoTypes, err := p.vocabulary.CargoTypes(ctx)
	if err != nil {
		return nil, err
	}
	return nlq.NewResolver(places, vehicles, cargoTypes), nil
}

// apply takes what survives checking, and returns how many fields that was.
//
// Nothing the rules resolved is ever overwritten: a model's reading is only
// ever an addition, so a confident wrong answer cannot displace a right one.
func (p *GroundedParser) apply(
	draft *domain.AdDraft,
	text string,
	result domain.AdExtractResult,
	resolver *nlq.Resolver,
) int {
	applied := 0

	for _, field := range []string{
		domain.FieldOrigin, domain.FieldDestination, domain.FieldCargoType,
		domain.FieldVehicleType, domain.FieldWeight, domain.FieldVolume,
		domain.FieldPrice, domain.FieldContactPhone, domain.FieldPaymentTerms,
	} {
		if draft.Has(field) {
			continue
		}
		proposed, offered := result.Fields[field]
		if !offered {
			continue
		}

		// The evidence is the model's claim about where in the advert this
		// came from. If those words are not there, neither is the fact.
		evidence := strings.TrimSpace(proposed.Evidence)
		if !nlq.Grounded(text, evidence) {
			p.log.Warn("discarding an ungrounded value",
				logger.String("field", field),
				logger.String("evidence", evidence))
			continue
		}

		if p.set(draft, field, text, evidence, strings.TrimSpace(proposed.Value), resolver) {
			draft.Note(field, domain.FieldFromAI, evidence)
			draft.Dropped = withoutWords(draft.Dropped, evidence)
			applied++
		}
	}
	return applied
}

// set fills one field, computing every value from the words rather than from
// what the model said they meant.
func (p *GroundedParser) set(
	draft *domain.AdDraft,
	field, text, evidence, value string,
	resolver *nlq.Resolver,
) bool {
	switch field {
	case domain.FieldOrigin, domain.FieldDestination:
		// The name is looked up: a place the platform has no row for is not a
		// place an advert can be filtered by.
		place, found := resolver.Place(firstNonEmpty(value, evidence))
		if !found {
			return false
		}
		if field == domain.FieldOrigin {
			setPlace(&draft.Ad.OriginCityID, &draft.Ad.OriginRegionID, &draft.Ad.OriginCountry, &draft.Ad.OriginText, place, evidence)
		} else {
			setPlace(&draft.Ad.DestCityID, &draft.Ad.DestRegionID, &draft.Ad.DestCountry, &draft.Ad.DestText, place, evidence)
		}
		return true

	case domain.FieldVehicleType:
		code, found := resolver.Vehicle(firstNonEmpty(value, evidence))
		if !found {
			return false
		}
		body := string(code)
		draft.Ad.VehicleType = &body
		return true

	case domain.FieldCargoType:
		id, found := resolver.Cargo(firstNonEmpty(value, evidence))
		if !found {
			return false
		}
		cargoID := id
		draft.Ad.CargoTypeID = &cargoID
		draft.Ad.CargoText = &evidence
		return true

	case domain.FieldWeight:
		kilograms, ok := nlq.ReadWeight(evidence)
		if !ok {
			return false
		}
		draft.Ad.WeightKg = &kilograms
		return true

	case domain.FieldVolume:
		cubic, ok := nlq.ReadVolume(evidence)
		if !ok {
			return false
		}
		draft.Ad.VolumeM3 = &cubic
		return true

	case domain.FieldPrice:
		amount, currency, ok := nlq.ReadPrice(evidence, text)
		if !ok {
			return false
		}
		draft.Ad.PriceAmount = &amount
		draft.Ad.PriceCurrency = &currency
		return true

	case domain.FieldContactPhone:
		phone, ok := nlq.ReadPhone(evidence)
		if !ok {
			return false
		}
		draft.Ad.ContactPhone = &phone
		return true

	case domain.FieldPaymentTerms:
		terms, ok := nlq.ReadPaymentTerms(evidence)
		if !ok {
			return false
		}
		draft.Ad.PaymentTerms = &terms
		return true

	default:
		// Dates are absent on purpose: "tomorrow" has to be resolved against
		// the company's clock, and a date is exactly the kind of thing a model
		// gets subtly and unfalsifiably wrong.
		return false
	}
}

func setPlace(cityID, regionID **uuid.UUID, country **string, text *string, place domain.Place, evidence string) {
	if place.Kind == domain.PlaceCity {
		id := place.ID
		*cityID = &id
		*regionID = place.RegionID
	} else {
		id := place.ID
		*regionID = &id
	}
	if place.CountryCode != "" {
		code := place.CountryCode
		*country = &code
	}
	*text = evidence
}

// withoutWords drops the words a model explained from the unresolved list: they
// are understood now, and leaving them there would tell the person otherwise.
func withoutWords(dropped []string, evidence string) []string {
	claimed := make(map[string]bool)
	for _, word := range strings.Fields(nlq.Fold(evidence)) {
		claimed[word] = true
	}

	kept := dropped[:0]
	for _, word := range dropped {
		if !claimed[nlq.Fold(word)] {
			kept = append(kept, word)
		}
	}
	return kept
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
