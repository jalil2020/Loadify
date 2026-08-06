package nlq

import "strings"

// Token is one word of the utterance, kept alongside the folded form the
// extractors match on.
type Token struct {
	// Surface is what the speaker actually typed, preserved for diagnostics.
	Surface string
	// Folded is the normalised form; see Fold.
	Folded string
	// claimedBy names the extractor that consumed this token, or is empty.
	claimedBy string
}

// Scan is the mutable state a parse runs over.
//
// Tokens are claimed rather than deleted, which gives two things: an extractor
// cannot consume a word another already used, and whatever is still unclaimed
// at the end is exactly the set of words nothing understood. That leftover is
// what gets reported instead of being silently ignored.
type Scan struct {
	Tokens []Token
}

// NewScan splits an utterance into tokens.
func NewScan(utterance string) *Scan {
	fields := strings.FieldsFunc(utterance, func(r rune) bool {
		return !IsWordRune(r)
	})

	tokens := make([]Token, 0, len(fields))
	for _, field := range fields {
		folded := Fold(field)
		if folded == "" {
			continue
		}
		tokens = append(tokens, Token{Surface: field, Folded: folded})
	}
	return &Scan{Tokens: tokens}
}

// Claim marks tokens [from, to) as consumed by name. Claiming an already
// claimed token is a no-op, so an earlier, more specific extractor always wins.
func (s *Scan) Claim(from, to int, name string) {
	for i := from; i < to && i < len(s.Tokens); i++ {
		if s.Tokens[i].claimedBy == "" {
			s.Tokens[i].claimedBy = name
		}
	}
}

// IsClaimed reports whether a token has already been consumed.
func (s *Scan) IsClaimed(i int) bool {
	return i >= 0 && i < len(s.Tokens) && s.Tokens[i].claimedBy != ""
}

// Free reports whether every token in [from, to) is still unclaimed, which is
// what an extractor checks before matching a multi-word phrase.
func (s *Scan) Free(from, to int) bool {
	if from < 0 || to > len(s.Tokens) || from >= to {
		return false
	}
	for i := from; i < to; i++ {
		if s.Tokens[i].claimedBy != "" {
			return false
		}
	}
	return true
}

// Folded returns the folded token at i, or "" when out of range. Extractors
// look ahead and behind freely, so bounds are handled here rather than at every
// call site.
func (s *Scan) Folded(i int) string {
	if i < 0 || i >= len(s.Tokens) {
		return ""
	}
	return s.Tokens[i].Folded
}

// Unclaimed returns the surface forms nothing understood, in order.
//
// Noise words are not filtered here: a word the parser cannot place is worth
// showing even when it turns out to be filler, because the alternative is a
// search that quietly ignored half the request.
func (s *Scan) Unclaimed() []string {
	var out []string
	for _, token := range s.Tokens {
		if token.claimedBy == "" {
			out = append(out, token.Surface)
		}
	}
	return out
}
