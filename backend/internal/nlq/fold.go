// Package nlq turns a freight request written the way a driver would say it
// into filters the marketplace already accepts.
//
// It is rules-first on purpose. The vocabulary is closed — fourteen regions,
// twelve vehicle types, numbers with units, a handful of date words — so the
// common utterances resolve deterministically, offline, in microseconds. That
// matters more than cleverness for someone holding a phone in a moving cab on a
// rural cell.
package nlq

import (
	"strings"
	"unicode"
)

// cyrillicToLatin maps Uzbek Cyrillic to the Latin the reference data uses.
//
// Uzbek is written in both scripts and drivers type in whichever their keyboard
// is set to, so "Тошкент" and "Toshkent" have to reach the same region. The
// multi-rune entries come first in Fold's replacement order because "ш" must
// become "sh" before any single-letter rule sees it.
var cyrillicToLatin = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo",
	'ж': "j", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "x", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sh",
	'ъ': "", 'ы': "i", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
	// Uzbek-specific Cyrillic letters.
	'қ': "q", 'ғ': "g", 'ҳ': "h", 'ў': "o",
}

// apostrophes are every character that shows up where Uzbek Latin wants the
// modifier letter in oʻ and gʻ.
//
// The correct character is U+02BB, but keyboards produce U+2018, U+2019, U+0027
// and U+0060 far more often, and plenty of people type nothing at all. Folding
// them all away means "Fargʻona", "Farg'ona", "Farg`ona" and "Fargona" are one
// word — which is the only way alias matching survives contact with real input.
var apostrophes = "'ʻʼ`‘’ʼ"

// Fold reduces text to the form aliases are matched on: lower case, Latin
// script, no apostrophes, no diacritics.
//
// It is deliberately lossy. Fold is never used to display anything — the
// original surface form is kept for that — only to decide whether two strings
// name the same thing.
func Fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for _, r := range strings.ToLower(s) {
		switch {
		case strings.ContainsRune(apostrophes, r):
			// Dropped rather than replaced: the whole point is that its
			// presence or absence must not change the match.
			continue
		case r == 'ʼ':
			continue
		default:
			if latin, ok := cyrillicToLatin[r]; ok {
				b.WriteString(latin)
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// IsWordRune reports whether a rune belongs inside a token.
//
// Digits count so "20t" survives as one token, and the hyphen does so a range
// written "10-20" is not split into two unrelated numbers.
func IsWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' ||
		strings.ContainsRune(apostrophes, r)
}
