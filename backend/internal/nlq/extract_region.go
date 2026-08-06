package nlq

import (
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/loadify/backend/internal/domain"
)

// direction is which end of the corridor a region mention refers to.
type direction int

const (
	directionUnknown direction = iota
	directionOrigin
	directionDestination
)

// originSuffixes are the Uzbek ablative. "Toshkentdan" is unambiguously *from*
// Tashkent — the grammar states the direction that a keyword matcher would
// otherwise have to guess at, which is the single biggest reason this parses
// reliably without a model.
var originSuffixes = []string{"dan", "den", "tan", "ten"}

// destinationSuffixes are the dative and the terminative. The -ka and -qa forms
// appear after k and q stems; -gacha is "as far as".
var destinationSuffixes = []string{"gacha", "qacha", "kacha", "ga", "qa", "ka"}

// originWords and destinationWords cover Russian and English, which mark
// direction with a preposition before the name rather than a suffix after it.
var originWords = map[string]bool{
	"from": true, "iz": true, "s": true, "so": true, "ot": true,
}

var destinationWords = map[string]bool{
	"to": true, "v": true, "vo": true, "na": true, "do": true, "gacha": true,
}

// regionAlias is one spelling of one region, pre-folded for matching.
type regionAlias struct {
	id     uuid.UUID
	folded string
	words  int
	// key is the loose form every spelling collapses onto; see RegionKey.
	key string
}

// RegionExtractor resolves region names and decides which end of the corridor
// each belongs to.
type RegionExtractor struct {
	aliases []regionAlias
}

// NewRegionExtractor pre-folds the vocabulary and orders it longest-first, so a
// two-word region always wins over a one-word region sharing its first word.
func NewRegionExtractor(regions []domain.Term) *RegionExtractor {
	var aliases []regionAlias

	// keyOwner records which region first claimed a loose key. Tashkent City
	// and Tashkent Region both reduce to "toshkent", and someone typing that
	// alone has to get one of them: the first, which is the lowest sort order,
	// which is the city. Freight originates in the city far more often than in
	// the surrounding region, so that is also the better guess.
	keyOwner := make(map[string]uuid.UUID)

	for _, region := range regions {
		id, err := uuid.Parse(region.ID)
		if err != nil {
			// A vocabulary entry without a usable id cannot be resolved to, and
			// silently matching it would produce a filter the API rejects.
			continue
		}

		for _, alias := range region.Aliases {
			folded := Fold(alias)
			if folded == "" {
				continue
			}

			key := RegionKey(folded)
			if owner, taken := keyOwner[key]; taken && owner != id {
				// Another region already owns this loose form. The full name
				// still matches exactly; only the ambiguous short form is
				// withheld, so "Toshkent viloyati" keeps working.
				key = ""
			} else if key != "" {
				keyOwner[key] = id
			}

			aliases = append(aliases, regionAlias{
				id:     id,
				folded: folded,
				words:  len(strings.Fields(folded)),
				key:    key,
			})

			// "Fergana Region" is also registered as "Fergana", because that is
			// what someone types.
			if short, stripped := StripQualifiers(folded); stripped {
				aliases = append(aliases, regionAlias{
					id:     id,
					folded: short,
					words:  len(strings.Fields(short)),
					key:    key,
				})
			}
		}
	}

	sort.SliceStable(aliases, func(i, j int) bool {
		if aliases[i].words != aliases[j].words {
			return aliases[i].words > aliases[j].words
		}
		return len(aliases[i].folded) > len(aliases[j].folded)
	})

	return &RegionExtractor{aliases: aliases}
}

func (e *RegionExtractor) Name() string { return "region" }

// mention is a resolved region together with how it was marked.
type mention struct {
	id  uuid.UUID
	dir direction
}

func (e *RegionExtractor) Extract(scan *Scan, out *domain.SearchIntent) {
	var mentions []mention

	for start := 0; start < len(scan.Tokens); start++ {
		if scan.IsClaimed(start) {
			continue
		}
		alias, end, suffix, ok := e.matchAt(scan, start)
		if !ok {
			continue
		}

		scan.Claim(start, end, e.Name())
		dir := directionFromSuffix(suffix)
		if dir == directionUnknown {
			dir = directionFromNeighbour(scan, start, end)
		}
		mentions = append(mentions, mention{id: alias.id, dir: dir})
	}

	e.assign(mentions, out)
}

// matchAt finds the longest alias starting at token start, allowing a case
// suffix to be glued onto the final word.
func (e *RegionExtractor) matchAt(scan *Scan, start int) (regionAlias, int, string, bool) {
	for _, alias := range e.aliases {
		end := start + alias.words
		if !scan.Free(start, end) {
			continue
		}

		phrase := foldedPhrase(scan, start, end)
		if phrase == alias.folded {
			return alias, end, "", true
		}
		// "toshkentdan" is the alias plus an ablative. Only a recognised suffix
		// counts, so "toshkentskiy" does not match Tashkent.
		if suffix, ok := strings.CutPrefix(phrase, alias.folded); ok && isCaseSuffix(suffix) {
			return alias, end, suffix, true
		}
		// Loosest and therefore last: Russian inflects the noun and names the
		// region with an adjective, so "Бухары" and "Бухарская область" only
		// meet once both are reduced to a key. A blank key means the form was
		// ambiguous across regions and was deliberately withheld.
		if alias.key != "" && RegionKey(phrase) == alias.key {
			return alias, end, caseSuffixOf(phrase), true
		}
	}
	return regionAlias{}, 0, "", false
}

func foldedPhrase(scan *Scan, from, to int) string {
	parts := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		parts = append(parts, scan.Folded(i))
	}
	return strings.Join(parts, " ")
}

func isCaseSuffix(suffix string) bool {
	return directionFromSuffix(suffix) != directionUnknown
}

// caseSuffixOf recovers the direction-bearing ending from a phrase matched
// loosely, so "Buxorodan" still reads as an origin even when it reached the key
// comparison rather than the exact one.
func caseSuffixOf(phrase string) string {
	_, suffix := StripCaseSuffix(phrase)
	if isCaseSuffix(suffix) {
		return suffix
	}
	return ""
}

func directionFromSuffix(suffix string) direction {
	switch {
	case suffix == "":
		return directionUnknown
	case slices.Contains(originSuffixes, suffix):
		return directionOrigin
	case slices.Contains(destinationSuffixes, suffix):
		return directionDestination
	}
	return directionUnknown
}

// directionFromNeighbour reads a Russian or English preposition sitting before
// the name, and claims it so it is not later reported as unrecognised.
func directionFromNeighbour(scan *Scan, start, end int) direction {
	before := scan.Folded(start - 1)
	switch {
	case originWords[before]:
		scan.Claim(start-1, start, "region")
		return directionOrigin
	case destinationWords[before]:
		scan.Claim(start-1, start, "region")
		return directionDestination
	}

	// Uzbek can also place "gacha" as a separate word after the name.
	if destinationWords[scan.Folded(end)] {
		scan.Claim(end, end+1, "region")
		return directionDestination
	}
	return directionUnknown
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
