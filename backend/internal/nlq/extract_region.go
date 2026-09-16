package nlq

import (
	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
)

// RegionExtractor resolves region names and decides which end of the corridor
// each belongs to.
type RegionExtractor struct {
	index *AliasIndex[uuid.UUID]
}

// NewRegionExtractor indexes the vocabulary.
//
// Entries are added in the order the vocabulary supplies them, which decides
// who owns an ambiguous short form: Tashkent City and Tashkent Region both
// reduce to "toshkent", and someone typing that alone gets the city, because it
// sorts first and because freight originates in the city far more often than in
// the surrounding region.
func NewRegionExtractor(regions []domain.Term) *RegionExtractor {
	index := NewAliasIndex[uuid.UUID]()
	for _, region := range regions {
		id, err := uuid.Parse(region.ID)
		if err != nil {
			// A vocabulary entry without a usable id cannot be resolved to, and
			// silently matching it would produce a filter the API rejects.
			continue
		}
		index.Add(id, region.Aliases)
	}
	return &RegionExtractor{index: index}
}

func (e *RegionExtractor) Name() string { return "region" }

// mention is a resolved region together with how it was marked.
type mention struct {
	id  uuid.UUID
	dir direction
}

func (e *RegionExtractor) Extract(scan *Scan, out *domain.SearchIntent) {
	var mentions []mention

	for start := range scan.Tokens {
		if scan.IsClaimed(start) {
			continue
		}
		id, end, suffix, ok := e.index.MatchAt(scan, start)
		if !ok {
			continue
		}

		scan.Claim(start, end, e.Name())
		dir := directionFromSuffix(suffix)
		if dir == directionUnknown {
			dir = directionFromNeighbour(scan, start, end, e.Name())
		}
		mentions = append(mentions, mention{id: id, dir: dir})
	}

	e.assign(mentions, out)
}

// assign turns mentions into the two id lists.
//
// Unmarked mentions fall back to word order, which is how people write a
// corridor when they drop the grammar: "Toshkent Samarqand" reads as from the
// first to the second. A single unmarked region becomes the origin, because a
// driver naming one place is almost always saying where they are.
func (e *RegionExtractor) assign(mentions []mention, out *domain.SearchIntent) {
	var unmarked []uuid.UUID

	for _, m := range mentions {
		switch m.dir {
		case directionOrigin:
			out.OriginRegionIDs = append(out.OriginRegionIDs, m.id)
		case directionDestination:
			out.DestinationRegionIDs = append(out.DestinationRegionIDs, m.id)
		default:
			unmarked = append(unmarked, m.id)
		}
	}

	for _, id := range unmarked {
		switch {
		case len(out.OriginRegionIDs) == 0:
			out.OriginRegionIDs = append(out.OriginRegionIDs, id)
		case len(out.DestinationRegionIDs) == 0:
			out.DestinationRegionIDs = append(out.DestinationRegionIDs, id)
		default:
			// Both ends already stated explicitly; an extra unmarked name is
			// more likely a via point than a correction, and widening either
			// end with it would change the corridor the speaker asked for.
			out.OriginRegionIDs = append(out.OriginRegionIDs, id)
		}
	}
}
