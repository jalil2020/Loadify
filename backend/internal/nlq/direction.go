package nlq

import (
	"slices"
	"strings"
)

// direction is which end of the corridor a place mention refers to.
//
// It lives beside the grammar that reveals it rather than inside one extractor,
// because both the marketplace search and the advertisement parser need the
// same answer from the same endings.
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
// the name, and claims it under the calling extractor's name so it is not later
// reported as unrecognised.
func directionFromNeighbour(scan *Scan, start, end int, claimant string) direction {
	before := scan.Folded(start - 1)
	switch {
	case originWords[before]:
		scan.Claim(start-1, start, claimant)
		return directionOrigin
	case destinationWords[before]:
		scan.Claim(start-1, start, claimant)
		return directionDestination
	}

	// Uzbek can also place "gacha" as a separate word after the name.
	if destinationWords[scan.Folded(end)] {
		scan.Claim(end, end+1, claimant)
		return directionDestination
	}
	return directionUnknown
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

// foldedPhrase joins the folded tokens in [from, to).
func foldedPhrase(scan *Scan, from, to int) string {
	parts := make([]string, 0, max(to-from, 0))
	for i := from; i < to; i++ {
		parts = append(parts, scan.Folded(i))
	}
	return strings.Join(parts, " ")
}

// surfacePhrase joins what the person actually typed in [from, to). It is the
// evidence shown beside a parsed field, so it must be their words rather than
// the folded form matching happened on.
func surfacePhrase(scan *Scan, from, to int) string {
	parts := make([]string, 0, max(to-from, 0))
	for i := from; i < to && i < len(scan.Tokens); i++ {
		if i >= 0 {
			parts = append(parts, scan.Tokens[i].Surface)
		}
	}
	return strings.Join(parts, " ")
}

// arrowMarks are the ways an advert draws a corridor. They are punctuation, so
// the tokeniser drops them; the text between two tokens is where they survive.
var arrowMarks = []string{"→", "⟶", "➡", "->", "=>", "-->", ">"}

// directionFromArrow reads an arrow beside a place: one pointing into the name
// makes it the destination, one leading away from it makes it the origin.
//
// It matters more than it looks. "Ташкент → Москва" names two places and the
// word order resolves it, but "Кашри → Москва" names one the platform knows and
// one it does not, and without the arrow the only place left would be read as
// the origin — the exact opposite of what the advert says.
func directionFromArrow(scan *Scan, start, end int) direction {
	if containsArrow(scan.Before(start)) {
		return directionDestination
	}
	if containsArrow(scan.After(end - 1)) {
		return directionOrigin
	}
	return directionUnknown
}

func containsArrow(gap string) bool {
	for _, mark := range arrowMarks {
		if strings.Contains(gap, mark) {
			return true
		}
	}
	return false
}
