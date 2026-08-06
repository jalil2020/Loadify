package nlq

import "github.com/loadify/backend/internal/domain"

// adrWords mark dangerous goods. ADR is the European agreement on their carriage
// and the acronym travels untranslated, but the plain-language phrases do not.
var adrWords = map[string]bool{
	"adr": true, "xavfli": true, "opasnyy": true, "opasnye": true,
	"dangerous": true, "hazardous": true,
}

// refrigerationWords mark a temperature requirement stated as a property of the
// cargo rather than as a body type.
//
// The refrigerator body itself is claimed earlier by the vehicle rule, so what
// reaches here is someone describing the goods — "muzlatilgan" — which is a
// different filter: it wants any vehicle that can hold temperature, not that
// one code.
var refrigerationWords = map[string]bool{
	"muzlatilgan": true, "sovutilgan": true, "sovuq": true,
	"zamorozhennyy": true, "ohlazhdennyy": true, "moroz": true,
	"frozen": true, "chilled": true, "refrigerated": true,
}

// FlagExtractor reads the boolean requirements.
//
// It runs last: every one of these words is a single token with no arguments,
// so nothing else needs them, and leaving it last means a word that a richer
// rule could have used is never stolen from it.
type FlagExtractor struct{}

func NewFlagExtractor() *FlagExtractor { return &FlagExtractor{} }

func (e *FlagExtractor) Name() string { return "flag" }

func (e *FlagExtractor) Extract(scan *Scan, out *domain.SearchIntent) {
	yes := true

	for i := range scan.Tokens {
		if scan.IsClaimed(i) {
			continue
		}
		switch {
		case adrWords[scan.Folded(i)]:
			scan.Claim(i, i+1, e.Name())
			out.RequiresADR = &yes
		case refrigerationWords[scan.Folded(i)]:
			scan.Claim(i, i+1, e.Name())
			out.RequiresRefrigeration = &yes
		}
	}
}
