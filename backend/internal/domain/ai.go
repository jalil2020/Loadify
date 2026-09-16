package domain

import "context"

// AdExtractRequest is what a model is allowed to see: the advert the person
// typed, which fields the rules could not fill, and the words nothing
// understood.
//
// It carries no vocabulary and no identifiers. The model proposes words; the
// server decides whether those words name anything. That is what keeps an
// invented region id out of a search and an invented city out of a post.
type AdExtractRequest struct {
	Text       string
	Missing    []string
	Unresolved []string
}

// AdExtractedValue is one field a model proposed.
//
// Evidence is not decoration: a value whose evidence does not appear in the
// advert is discarded, which is the mechanism that stops a model from filling
// in a tonnage nobody wrote.
type AdExtractedValue struct {
	Value    string
	Evidence string
}

// AdExtractResult is a model's reading, plus what it cost.
type AdExtractResult struct {
	Fields    map[string]AdExtractedValue
	Provider  string
	Model     string
	TokensIn  int
	TokensOut int
}

// AdExtractor reads what the rules could not.
//
// It is optional everywhere: with no extractor configured the platform still
// parses, stores and posts adverts, and the fields the rules missed are simply
// reported as missing for a person to fill in.
type AdExtractor interface {
	Extract(ctx context.Context, req AdExtractRequest) (AdExtractResult, error)
}
