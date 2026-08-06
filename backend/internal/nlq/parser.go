package nlq

import (
	"context"
	"time"

	"github.com/loadify/backend/internal/domain"
)

// Extractor understands one dimension of a request.
//
// The interface is this small on purpose: an extractor claims the tokens it
// recognises and writes what it found onto the intent. Adding a new dimension —
// cargo type, payment terms, a loading side — means adding an Extractor and
// registering it, never editing one that already works.
type Extractor interface {
	// Name identifies the extractor when a token is claimed, which is what
	// makes a mis-parse traceable to the rule that caused it.
	Name() string
	Extract(scan *Scan, out *domain.SearchIntent)
}

// Clock is injected so date words resolve against a fixed instant in tests
// rather than against whenever the suite happens to run.
type Clock func() time.Time

// Parser turns an utterance into a resolved intent.
//
// It holds no state between calls; the vocabulary is fetched per parse because
// the reference repositories already cache it, and a parser that cached it
// again would serve stale region names after an admin edit.
type Parser struct {
	vocabulary domain.Vocabulary
	clock      Clock
}

// NewParser builds a parser over a vocabulary. A nil clock means time.Now.
func NewParser(vocabulary domain.Vocabulary, clock Clock) *Parser {
	if clock == nil {
		clock = time.Now
	}
	return &Parser{vocabulary: vocabulary, clock: clock}
}

// Parse resolves an utterance.
//
// Extractors run most-specific first. Region names are matched before anything
// else because they are multi-word and would otherwise be broken up by a rule
// looking for a single token — "Toshkent shahri" must be claimed whole before
// a stray matcher can take "shahri".
func (p *Parser) Parse(ctx context.Context, utterance string) (domain.SearchIntent, error) {
	regions, err := p.vocabulary.Regions(ctx)
	if err != nil {
		return domain.SearchIntent{}, err
	}
	vehicles, err := p.vocabulary.VehicleTypes(ctx)
	if err != nil {
		return domain.SearchIntent{}, err
	}

	scan := NewScan(utterance)
	intent := domain.SearchIntent{}

	for _, extractor := range p.extractors(regions, vehicles) {
		extractor.Extract(scan, &intent)
	}

	intent.Dropped = scan.Unclaimed()
	return intent, nil
}

// extractors is the ordered pipeline. The order is the design: each entry may
// depend on earlier ones having already claimed their tokens, and on nothing
// later having run.
func (p *Parser) extractors(regions, vehicles []domain.Term) []Extractor {
	return []Extractor{
		// Longest phrases first, so "Toshkent viloyati" beats "Toshkent".
		NewRegionExtractor(regions),
		NewVehicleExtractor(vehicles),
		// Dates before the numeric rules: "3 kun" must not be read as a weight.
		NewDateExtractor(p.clock),
		// Weight before price: both consume a number, and weight's units are
		// the narrower signal.
		NewWeightExtractor(),
		NewPriceExtractor(),
		NewFlagExtractor(),
	}
}
