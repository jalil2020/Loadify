package nlq

import "strings"

// caseSuffixes are the endings that can be glued onto a noun without changing
// which thing it names.
//
// The Uzbek ones also carry direction and are listed again in the region rule,
// which reads them for that; here they matter only so the stem can be found.
// The list is ordered longest-first so "gacha" is stripped before "ga" and the
// remainder is not left holding "cha".
var caseSuffixes = []string{
	"gacha", "qacha", "kacha", "ning", "dagi", "lar",
	"dan", "den", "tan", "ten", "ga", "qa", "ka", "da", "ta", "ni", "nи",
}

// StripCaseSuffix splits a folded token into its stem and the case ending glued
// onto it, or returns the token unchanged when there is none.
//
// The stem must stay long enough to be a word: without that guard "dan" itself
// would strip to nothing and match every alias.
func StripCaseSuffix(folded string) (stem, suffix string) {
	for _, candidate := range caseSuffixes {
		trimmed, found := strings.CutSuffix(folded, candidate)
		if found && len(trimmed) >= minStemLen {
			return trimmed, candidate
		}
	}
	return folded, ""
}

// russianVowels are the vowels a Russian noun ending can consist of, already
// folded to Latin.
const russianVowels = "aeiouy"

// Stem drops a trailing vowel so Russian case endings collapse together.
//
// Russian marks direction with a preposition but still inflects the noun after
// it — "из Бухары", "в Бухару", "о Бухаре" — so prefix matching against the
// nominative "Бухара" fails on every one of them. Comparing stems instead makes
// all four the same word.
//
// Only one vowel is dropped, and only from a word long enough to survive it:
// dropping more would start merging genuinely different names.
func Stem(folded string) string {
	if len(folded) <= minStemLen {
		return folded
	}
	if strings.ContainsRune(russianVowels, rune(folded[len(folded)-1])) {
		return folded[:len(folded)-1]
	}
	return folded
}

// minStemLen is the shortest thing still treated as a name. Four characters
// keeps "Buxoro" and "Nawoi" intact while refusing to let a two-letter residue
// match anything.
const minStemLen = 4

// qualifiers are the administrative words attached to a region's official name.
//
// The reference data stores full names — "Fergana Region", "Бухарская область",
// "Toshkent viloyati" — and nobody types them. Stripping the qualifier is what
// connects the name in the database to the name in someone's head. Russian puts
// it before the noun and the other two after, so position is not assumed.
var qualifiers = map[string]bool{
	"viloyati": true, "viloyat": true, "shahri": true, "shahar": true,
	"respublikasi": true, "avtonom": true,
	"oblast": true, "oblasti": true, "gorod": true, "goroda": true,
	"respublika": true, "respubliki": true, "avtonomnaya": true,
	"region": true, "city": true, "republic": true, "province": true,
}

// adjectivalSuffixes are the Russian relational-adjective endings.
//
// Russian names a region with an adjective rather than the settlement's own
// name — Бухара becomes Бухарская — so stripping the qualifier alone still
// leaves a word that shares no ending with what a driver types.
var adjectivalSuffixes = []string{"skaya", "skoye", "skogo", "skoy", "skie", "skiy", "sk"}

// StripQualifiers removes the administrative words from a folded name and
// reports whether anything was removed.
func StripQualifiers(folded string) (string, bool) {
	words := strings.Fields(folded)
	kept := make([]string, 0, len(words))
	for _, word := range words {
		if !qualifiers[word] {
			kept = append(kept, word)
		}
	}
	if len(kept) == 0 || len(kept) == len(words) {
		return folded, false
	}
	return strings.Join(kept, " "), true
}

// RegionKey reduces a region name to the form every spelling of it collapses
// onto: no qualifier, no adjectival ending, no trailing vowel.
//
// It is the loosest comparison in the parser and therefore the last one tried.
// Two genuinely different regions must not share a key, which is checked when
// the vocabulary is built rather than assumed here.
func RegionKey(folded string) string {
	stripped, _ := StripQualifiers(folded)

	for _, suffix := range adjectivalSuffixes {
		if trimmed, found := strings.CutSuffix(stripped, suffix); found && len(trimmed) >= minStemLen {
			return trimmed
		}
	}
	return Stem(stripped)
}
