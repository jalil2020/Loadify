package nlq

// Token is one word of the utterance, kept alongside the folded form the
// extractors match on.
type Token struct {
	// Surface is what the speaker actually typed, preserved for diagnostics.
	Surface string
	// Folded is the normalised form; see Fold.
	Folded string
	// Start and End are byte offsets into the original text.
	//
	// They exist because some of the meaning lives in what sits *between* the
	// words: an arrow in "Ташкент → Москва" states the direction as plainly as
	// an Uzbek case ending does, and it is not a word, so the only way to read
	// it is to look back at the text the tokens came from.
	Start, End int
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
	// text is the utterance the tokens came from, kept so punctuation between
	// them can still be read.
	text string
}

// NewScan splits an utterance into tokens, remembering where each one sat.
func NewScan(utterance string) *Scan {
	var tokens []Token
	start := -1

	flush := func(end int) {
		if start < 0 {
			return
		}
		surface := utterance[start:end]
		if folded := Fold(surface); folded != "" {
			tokens = append(tokens, Token{Surface: surface, Folded: folded, Start: start, End: end})
		}
		start = -1
	}

	runes := []rune(utterance)
	offsets := make([]int, 0, len(runes))
	for offset := range utterance {
		offsets = append(offsets, offset)
	}

	for i, r := range runes {
		offset := offsets[i]
		// A hyphen belongs inside a token only between digits, where it writes a
		// range. Between names it joins two of them — "Ташкент-Москва" is a
		// corridor, not a place — so it separates.
		if r == '-' && !(i > 0 && i+1 < len(runes) && isDigit(runes[i-1]) && isDigit(runes[i+1])) {
			flush(offset)
			continue
		}
		if IsWordRune(r) {
			if start < 0 {
				start = offset
			}
			continue
		}
		flush(offset)
	}
	flush(len(utterance))

	return &Scan{Tokens: tokens, text: utterance}
}

// Between returns the text separating two tokens, which is where punctuation
// that carries meaning — an arrow, a dash — survives.
func (s *Scan) Between(left, right int) string {
	if left < 0 || right >= len(s.Tokens) || left >= right {
		return ""
	}
	return s.text[s.Tokens[left].End:s.Tokens[right].Start]
}

// Before returns the text preceding a token, up to the previous one.
func (s *Scan) Before(i int) string {
	if i <= 0 || i >= len(s.Tokens) {
		if i == 0 && len(s.Tokens) > 0 {
			return s.text[:s.Tokens[0].Start]
		}
		return ""
	}
	return s.Between(i-1, i)
}

// After returns the text following a token, up to the next one.
func (s *Scan) After(i int) string {
	switch {
	case i < 0 || i >= len(s.Tokens):
		return ""
	case i == len(s.Tokens)-1:
		return s.text[s.Tokens[i].End:]
	default:
		return s.Between(i, i+1)
	}
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

func isDigit(r rune) bool { return r >= '0' && r <= '9' }
