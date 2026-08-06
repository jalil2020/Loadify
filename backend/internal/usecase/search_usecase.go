package usecase

import (
	"context"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/nlq"
	"github.com/loadify/backend/pkg/logger"
)

// referenceVocabulary adapts the reference usecase to the parser's Vocabulary
// port.
//
// It exists so the parser depends on "a list of names I can resolve against"
// rather than on the geography repository, which keeps it a pure function of
// its inputs and testable without a database. Reusing the reference usecase
// rather than the repository also means the parse inherits its Redis cache: the
// vocabulary is read on every keystroke-triggered parse and must not be a query.
type referenceVocabulary struct {
	reference *ReferenceUsecase
}

// Regions offers every localisation of every region name as an alias, so a
// driver writing Uzbek, Russian or English all resolve to the same id.
func (v referenceVocabulary) Regions(ctx context.Context) ([]domain.Term, error) {
	var terms []domain.Term

	for _, locale := range domain.AllLocales {
		regions, err := v.reference.Regions(ctx, "", locale)
		if err != nil {
			return nil, err
		}
		terms = mergeTerms(terms, regions, func(r dto.RegionResponse) (string, string) {
			return r.ID.String(), r.Name
		})
	}
	return terms, nil
}

// VehicleTypes does the same for body types. The parser widens these further
// with the shop language drivers actually use.
func (v referenceVocabulary) VehicleTypes(ctx context.Context) ([]domain.Term, error) {
	var terms []domain.Term

	for _, locale := range domain.AllLocales {
		types := v.reference.VehicleTypes(locale)
		terms = mergeTerms(terms, types, func(t dto.VehicleTypeResponse) (string, string) {
			return t.Code, t.Name
		})
	}
	return terms, nil
}

// mergeTerms folds one locale's names into the accumulating term list, keyed by
// id so the three passes build one term per thing rather than three.
func mergeTerms[T any](terms []domain.Term, items []T, key func(T) (string, string)) []domain.Term {
	index := make(map[string]int, len(terms))
	for i, term := range terms {
		index[term.ID] = i
	}

	for _, item := range items {
		id, name := key(item)
		if id == "" || name == "" {
			continue
		}
		if at, found := index[id]; found {
			terms[at].Aliases = append(terms[at].Aliases, name)
			continue
		}
		index[id] = len(terms)
		terms = append(terms, domain.Term{ID: id, Aliases: []string{name}})
	}
	return terms
}

// SearchUsecase turns a written request into the filters both sides of the
// marketplace already accept.
//
// It deliberately does not run the search. Returning the filters lets the client
// show them as editable chips before anything is fetched, and keeps a single
// implementation of load and driver search rather than a second one behind a
// natural-language door.
type SearchUsecase struct {
	parser *nlq.Parser
	log    logger.Logger
}

// NewSearchUsecase wires the parser over the reference vocabulary.
func NewSearchUsecase(reference *ReferenceUsecase, log logger.Logger) *SearchUsecase {
	return &SearchUsecase{
		parser: nlq.NewParser(referenceVocabulary{reference: reference}, nil),
		log:    log,
	}
}

// Parse resolves an utterance and projects it onto the requested side of the
// market.
func (u *SearchUsecase) Parse(
	ctx context.Context,
	req dto.ParseSearchRequest,
	locale domain.Locale,
) (dto.ParsedSearchResponse, error) {
	intent, err := u.parser.Parse(ctx, req.Query)
	if err != nil {
		return dto.ParsedSearchResponse{}, err
	}

	if len(intent.Dropped) > 0 {
		// Worth a log line: a word nobody could resolve is either a region the
		// vocabulary is missing or a phrasing worth teaching the parser, and
		// both only show up in aggregate.
		u.log.Info("search terms went unresolved",
			logger.String("query", req.Query),
			logger.Any("dropped", intent.Dropped))
	}

	return dto.NewParsedSearchResponse(intent, req.Target, locale), nil
}
