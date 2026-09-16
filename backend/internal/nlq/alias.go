package nlq

import (
	"sort"
	"strings"
)

// AliasIndex matches the names people type against a closed vocabulary.
//
// Three rules make it work on real input, and all three are here rather than in
// each extractor: the longest phrase wins, so "Toshkent viloyati" is never
// broken up by a rule looking for "Toshkent"; a recognised case ending may be
// glued to the last word, so "Toshkentdan" still resolves; and a loose key
// comparison catches the Russian habit of naming a place with an inflected
// adjective, so "Бухары" reaches Buxoro.
//
// The loose form is withheld when two entries would collapse onto it. Their
// full names still match exactly — only the ambiguous short form is refused,
// because resolving it would mean picking one of two places at random.
type AliasIndex[T comparable] struct {
	entries  []aliasEntry[T]
	keyOwner map[string]T
	sorted   bool
}

type aliasEntry[T comparable] struct {
	id     T
	folded string
	words  int
	key    string
}

// NewAliasIndex builds an empty index.
func NewAliasIndex[T comparable]() *AliasIndex[T] {
	return &AliasIndex[T]{keyOwner: make(map[string]T)}
}

// Add registers every spelling of one entry. Aliases are folded here, so
// callers pass the words as they are written.
//
// The first entry to claim a loose key keeps it, which is why the caller's
// order matters: the vocabulary is sorted so the more likely meaning is added
// first — Tashkent the city before Tashkent the region, since freight
// originates in the city far more often.
func (x *AliasIndex[T]) Add(id T, aliases []string) {
	for _, alias := range aliases {
		folded := Fold(alias)
		if folded == "" {
			continue
		}
		x.addFolded(id, folded)

		// "Fergana Region" is also registered as "Fergana", because that is
		// what someone types.
		if short, stripped := StripQualifiers(folded); stripped {
			x.addFolded(id, short)
		}
	}
	x.sorted = false
}

func (x *AliasIndex[T]) addFolded(id T, folded string) {
	key := RegionKey(folded)
	if owner, taken := x.keyOwner[key]; taken && owner != id {
		key = ""
	} else if key != "" {
		x.keyOwner[key] = id
	}
	x.entries = append(x.entries, aliasEntry[T]{
		id:     id,
		folded: folded,
		words:  len(strings.Fields(folded)),
		key:    key,
	})
}

// Len reports how many spellings are registered.
func (x *AliasIndex[T]) Len() int { return len(x.entries) }

// MatchAt finds the longest alias starting at token start. It returns the
// entry, the token after the match, and the case ending that was glued on,
// which is what tells a caller whether the name was marked as an origin or a
// destination.
func (x *AliasIndex[T]) MatchAt(scan *Scan, start int) (id T, end int, suffix string, ok bool) {
	x.ensureSorted()

	for _, entry := range x.entries {
		stop := start + entry.words
		if !scan.Free(start, stop) {
			continue
		}

		phrase := foldedPhrase(scan, start, stop)
		if phrase == entry.folded {
			return entry.id, stop, "", true
		}
		// Only a recognised ending counts, so "toshkentskiy" does not match
		// Tashkent while "toshkentdan" does.
		if trimmed, found := strings.CutPrefix(phrase, entry.folded); found && isCaseSuffix(trimmed) {
			return entry.id, stop, trimmed, true
		}
		// Loosest, and therefore last.
		if entry.key != "" && RegionKey(phrase) == entry.key {
			return entry.id, stop, caseSuffixOf(phrase), true
		}
	}
	var zero T
	return zero, 0, "", false
}

// ensureSorted orders the vocabulary longest-first, which is what makes "the
// longest phrase wins" true rather than dependent on insertion order.
func (x *AliasIndex[T]) ensureSorted() {
	if x.sorted {
		return
	}
	sort.SliceStable(x.entries, func(i, j int) bool {
		if x.entries[i].words != x.entries[j].words {
			return x.entries[i].words > x.entries[j].words
		}
		return len(x.entries[i].folded) > len(x.entries[j].folded)
	})
	x.sorted = true
}
