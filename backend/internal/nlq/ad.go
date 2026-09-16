package nlq

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
)

// adExtractor reads one dimension of a written advertisement.
//
// It is the same shape as the search Extractor and for the same reason: adding
// a dimension means adding a file, not editing one that already works. The sink
// differs because an advertisement is a thing to be published, while a search
// is a filter to be applied.
type adExtractor interface {
	Name() string
	Extract(scan *Scan, out *domain.AdDraft)
}

// AdParser turns a freight advertisement as someone typed it into a draft.
//
// Nothing here guesses. A field the rules cannot resolve is left empty and
// reported as missing, and every word nothing understood comes back in Dropped,
// because an advert that quietly lost its tonnage is worse than one that says
// it could not read it.
type AdParser struct {
	vocabulary domain.AdVocabulary
	clock      Clock
	location   *time.Location
}

// NewAdParser builds a parser over a vocabulary. A nil clock means time.Now,
// and a nil location means Asia/Tashkent, because "ertaga" has to mean the
// person's tomorrow rather than the server's.
func NewAdParser(vocabulary domain.AdVocabulary, clock Clock, location *time.Location) *AdParser {
	if clock == nil {
		clock = time.Now
	}
	if location == nil {
		location = defaultLocation()
	}
	return &AdParser{vocabulary: vocabulary, clock: clock, location: location}
}

func defaultLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Tashkent"); err == nil {
		return loc
	}
	// Windows and scratch containers often ship without a zone database. The
	// offset is stable and has no daylight saving, so a fixed zone is correct
	// rather than merely convenient.
	return time.FixedZone("UTC+5", 5*60*60)
}

// Parse reads the text and returns what it understood.
func (p *AdParser) Parse(ctx context.Context, text string) (domain.AdDraft, error) {
	places, err := p.vocabulary.Places(ctx)
	if err != nil {
		return domain.AdDraft{}, err
	}
	vehicles, err := p.vocabulary.VehicleTypes(ctx)
	if err != nil {
		return domain.AdDraft{}, err
	}
	cargoTypes, err := p.vocabulary.CargoTypes(ctx)
	if err != nil {
		return domain.AdDraft{}, err
	}

	scan := NewScan(text)
	draft := domain.AdDraft{
		Ad:      domain.LoadAd{RawText: text},
		Sources: domain.FieldSources{},
	}

	for _, extractor := range p.extractors(text, places, vehicles, cargoTypes) {
		extractor.Extract(scan, &draft)
	}

	draft.Dropped = scan.Unclaimed()
	draft.Ad.Fingerprint = draft.Ad.ComputeFingerprint()
	draft.Missing = draft.Ad.MissingForPublish()
	return draft, nil
}

// extractors is the ordered pipeline, and the order is the design: each entry
// may rely on earlier ones having claimed their tokens, and on nothing later
// having run.
func (p *AdParser) extractors(raw string, places []domain.Place, vehicles, cargoTypes []domain.Term) []adExtractor {
	return []adExtractor{
		// Names first, longest-first, so "Toshkent viloyati" is claimed whole.
		NewPlaceExtractor(places),
		NewAdVehicleExtractor(vehicles),
		NewAdCargoExtractor(cargoTypes),
		// A phone number is a long run of digits, so it has to be claimed
		// before any rule that reads a figure.
		NewContactExtractor(),
		// Dates before the numeric rules: "3 kun" must not become a tonnage.
		NewAdDateExtractor(raw, p.clock, p.location),
		NewAdWeightExtractor(),
		NewAdVolumeExtractor(),
		NewAdPriceExtractor(raw),
		NewPaymentExtractor(),
	}
}

// PlaceExtractor resolves the two ends of the corridor.
//
// Unlike the marketplace search, an advert routinely crosses a border, so the
// vocabulary is cities and regions across the region rather than the fourteen
// Uzbek ones. A resolved city carries its region and country with it, which is
// what lets a group whose rule says "anything to Russia" match an advert that
// named only Moscow.
type PlaceExtractor struct {
	index  *AliasIndex[uuid.UUID]
	byID   map[uuid.UUID]domain.Place
	claims []placeMention
}

type placeMention struct {
	place    domain.Place
	dir      direction
	evidence string
}

// NewPlaceExtractor indexes the vocabulary.
func NewPlaceExtractor(places []domain.Place) *PlaceExtractor {
	index := NewAliasIndex[uuid.UUID]()
	byID := make(map[uuid.UUID]domain.Place, len(places))

	for _, place := range places {
		if place.ID == uuid.Nil {
			continue
		}
		byID[place.ID] = place
		index.Add(place.ID, place.Aliases)
	}
	return &PlaceExtractor{index: index, byID: byID}
}

func (e *PlaceExtractor) Name() string { return "place" }

func (e *PlaceExtractor) Extract(scan *Scan, out *domain.AdDraft) {
	e.claims = nil

	for start := range scan.Tokens {
		if scan.IsClaimed(start) {
			continue
		}
		id, end, suffix, ok := e.index.MatchAt(scan, start)
		if !ok {
			continue
		}
		place, known := e.byID[id]
		if !known {
			continue
		}

		evidence := surfacePhrase(scan, start, end)
		scan.Claim(start, end, e.Name())

		dir := directionFromSuffix(suffix)
		if dir == directionUnknown {
			dir = directionFromNeighbour(scan, start, end, e.Name())
		}
		e.claims = append(e.claims, placeMention{place: place, dir: dir, evidence: evidence})
	}

	e.assign(out)
}

// assign fills the two ends.
//
// Unmarked mentions fall back to word order, which is how an advert is written
// when the grammar is dropped: "Ташкент → Москва" is from the first to the
// second. A single unmarked place becomes the origin, because someone naming
// one place is saying where the freight is.
func (e *PlaceExtractor) assign(out *domain.AdDraft) {
	var unmarked []placeMention

	for _, mention := range e.claims {
		switch mention.dir {
		case directionOrigin:
			setOrigin(out, mention)
		case directionDestination:
			setDestination(out, mention)
		default:
			unmarked = append(unmarked, mention)
		}
	}

	for _, mention := range unmarked {
		switch {
		case !out.Has(domain.FieldOrigin):
			setOrigin(out, mention)
		case !out.Has(domain.FieldDestination):
			setDestination(out, mention)
		default:
			// Both ends are already stated. A third name is more likely a via
			// point than a correction, and overwriting either end would change
			// the corridor the person described, so it is left in Dropped.
		}
	}
}

func setOrigin(out *domain.AdDraft, mention placeMention) {
	if out.Has(domain.FieldOrigin) {
		return
	}
	place := mention.place
	if place.Kind == domain.PlaceCity {
		out.Ad.OriginCityID = &place.ID
		out.Ad.OriginRegionID = place.RegionID
	} else {
		out.Ad.OriginRegionID = &place.ID
	}
	if place.CountryCode != "" {
		country := place.CountryCode
		out.Ad.OriginCountry = &country
	}
	out.Ad.OriginText = mention.evidence
	out.Note(domain.FieldOrigin, domain.FieldFromRule, mention.evidence)
}

func setDestination(out *domain.AdDraft, mention placeMention) {
	if out.Has(domain.FieldDestination) {
		return
	}
	place := mention.place
	if place.Kind == domain.PlaceCity {
		out.Ad.DestCityID = &place.ID
		out.Ad.DestRegionID = place.RegionID
	} else {
		out.Ad.DestRegionID = &place.ID
	}
	if place.CountryCode != "" {
		country := place.CountryCode
		out.Ad.DestCountry = &country
	}
	out.Ad.DestText = mention.evidence
	out.Note(domain.FieldDestination, domain.FieldFromRule, mention.evidence)
}
