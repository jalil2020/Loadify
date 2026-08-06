package nlq

import (
	"strconv"
	"strings"

	"github.com/loadify/backend/internal/domain"
)

// bound is how a stated figure limits the search.
type bound int

const (
	boundExact bound = iota
	boundMin
	boundMax
)

// minWords and maxWords are the comparatives, in all three languages. They sit
// either side of the figure depending on the language — Uzbek puts them after
// ("20 tonnadan ortiq"), English before ("over 20 tonnes") — so both positions
// are checked.
var minWords = map[string]bool{
	"ortiq": true, "kop": true, "kore": true, "yuqori": true, "boshlab": true,
	"bolshe": true, "svishe": true, "ot": true,
	"over": true, "above": true, "min": true, "minimum": true, "at": true,
}

var maxWords = map[string]bool{
	"kam": true, "kamida": true, "pastroq": true,
	"menshe": true, "menee": true, "maks": true,
	"under": true, "below": true, "max": true, "maximum": true, "upto": true,
}

// standaloneCaseWords are case endings people write as separate words. They
// carry no meaning on their own here — only the comparative after them does.
var standaloneCaseWords = map[string]bool{
	"dan": true, "den": true, "gacha": true, "ot": true, "do": true,
}

// multipliers scale a bare figure. "500 ming" is half a million, and a driver
// quoting a price says it that way far more often than they write the zeros.
var multipliers = map[string]int64{
	"ming": 1_000, "тысяч": 1_000, "tys": 1_000, "k": 1_000,
	"mln": 1_000_000, "million": 1_000_000, "millon": 1_000_000, "mil": 1_000_000,
}

// tonneUnits and kiloUnits are the weight units, folded.
var tonneUnits = map[string]bool{
	"t": true, "tn": true, "tonna": true, "tona": true, "ton": true,
	"tonn": true, "tonny": true, "tonnes": true, "tons": true,
}

var kiloUnits = map[string]bool{
	"kg": true, "kilo": true, "kilogramm": true, "kgs": true,
}

// currencyUnits mark a figure as money rather than mass.
var currencyUnits = map[string]bool{
	"som": true, "sum": true, "sums": true, "uzs": true, "sm": true,
}

// parseNumber reads a bare integer, tolerating the separators people type.
func parseNumber(folded string) (int64, bool) {
	cleaned := strings.NewReplacer(" ", "", ",", "", ".", "", "-", "").Replace(folded)
	if cleaned == "" {
		return 0, false
	}
	value, err := strconv.ParseInt(cleaned, 10, 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

// splitNumberUnit handles the glued form, "20t" and "500kg", which is how
// people type on a phone keyboard when the space costs a tap.
func splitNumberUnit(folded string) (int64, string, bool) {
	split := 0
	for split < len(folded) && folded[split] >= '0' && folded[split] <= '9' {
		split++
	}
	if split == 0 || split == len(folded) {
		return 0, "", false
	}
	value, ok := parseNumber(folded[:split])
	if !ok {
		return 0, "", false
	}
	return value, folded[split:], true
}

// boundAround reads the comparative sitting either side of a figure.
//
// The Uzbek comparative attaches to the unit as a suffix — "tonnadan ortiq" —
// so the ablative on the preceding token is a signal too, but only when a
// comparative word follows it; on its own, "-dan" is a corridor marker.
func boundAround(scan *Scan, from, to int) bound {
	// The ablative is often written apart from the noun — "so'm dan ortiq" —
	// which leaves a bare "dan" between the figure and its comparative. Stepping
	// over it is the difference between reading the comparative and reporting
	// two perfectly meaningful words as not understood.
	if standaloneCaseWords[scan.Folded(to)] {
		scan.Claim(to, to+1, "bound")
		to++
	}

	switch {
	case minWords[scan.Folded(to)]:
		scan.Claim(to, to+1, "bound")
		return boundMin
	case maxWords[scan.Folded(to)]:
		scan.Claim(to, to+1, "bound")
		return boundMax
	case minWords[scan.Folded(from-1)]:
		scan.Claim(from-1, from, "bound")
		return boundMin
	case maxWords[scan.Folded(from-1)]:
		scan.Claim(from-1, from, "bound")
		return boundMax
	}
	return boundExact
}

// WeightExtractor reads a tonnage or a kilo figure.
type WeightExtractor struct{}

func NewWeightExtractor() *WeightExtractor { return &WeightExtractor{} }

func (e *WeightExtractor) Name() string { return "weight" }

func (e *WeightExtractor) Extract(scan *Scan, out *domain.SearchIntent) {
	for i := 0; i < len(scan.Tokens); i++ {
		if scan.IsClaimed(i) {
			continue
		}

		kg, end, ok := e.readAt(scan, i)
		if !ok {
			continue
		}

		scan.Claim(i, end, e.Name())
		value := kg
		switch boundAround(scan, i, end) {
		case boundMin:
			out.MinWeightKg = &value
		case boundMax:
			out.MaxWeightKg = &value
		default:
			// Neutral: which way it bounds the search depends on who is asking,
			// and that is the projection's decision, not the parser's.
			out.WeightKg = &value
		}
		return
	}
}

// readAt returns the weight in kilograms starting at token i.
func (e *WeightExtractor) readAt(scan *Scan, i int) (int, int, bool) {
	if value, unit, ok := splitNumberUnit(scan.Folded(i)); ok {
		if kg, ok := toKilograms(value, unit); ok {
			return kg, i + 1, true
		}
		return 0, 0, false
	}

	value, ok := parseNumber(scan.Folded(i))
	if !ok || scan.IsClaimed(i+1) {
		return 0, 0, false
	}
	if kg, ok := toKilograms(value, scan.Folded(i+1)); ok {
		return kg, i + 2, true
	}
	return 0, 0, false
}

func toKilograms(value int64, unit string) (int, bool) {
	// The Uzbek comparative attaches its ablative to the unit, not to the
	// figure: "20 tonnadan ortiq". Without stripping it the whole phrase parses
	// as no weight at all.
	stem, _ := StripCaseSuffix(unit)

	switch {
	case tonneUnits[unit] || tonneUnits[stem]:
		return int(value * 1000), true
	case kiloUnits[unit] || kiloUnits[stem]:
		return int(value), true
	}
	return 0, false
}

// PriceExtractor reads a soʻm figure.
//
// It runs after the weight rule because both consume a number and weight's
// units are the narrower signal; a figure reaching here with a currency word or
// a scale word beside it is money.
type PriceExtractor struct{}

func NewPriceExtractor() *PriceExtractor { return &PriceExtractor{} }

func (e *PriceExtractor) Name() string { return "price" }

func (e *PriceExtractor) Extract(scan *Scan, out *domain.SearchIntent) {
	for i := 0; i < len(scan.Tokens); i++ {
		if scan.IsClaimed(i) {
			continue
		}

		amount, end, ok := e.readAt(scan, i)
		if !ok {
			continue
		}

		scan.Claim(i, end, e.Name())
		// Money is stored in minor units everywhere on the platform, so a
		// spoken soʻm figure is scaled once, here.
		minor := amount * 100
		out.Currency = "UZS"

		switch boundAround(scan, i, end) {
		case boundMax:
			out.MaxPrice = &minor
		default:
			// A price without a comparative is a floor: someone naming a rate
			// is saying what they want to earn, not what they refuse to exceed.
			out.MinPrice = &minor
		}
		return
	}
}

// readAt returns the amount in soʻm starting at token i, consuming any scale
// word and currency word that follow.
func (e *PriceExtractor) readAt(scan *Scan, i int) (int64, int, bool) {
	amount, ok := parseNumber(scan.Folded(i))
	if !ok {
		return 0, 0, false
	}

	end := i + 1
	scaled := false
	if factor, found := multipliers[scan.Folded(end)]; found && !scan.IsClaimed(end) {
		amount *= factor
		scaled = true
		end++
	}

	currency := false
	if currencyUnits[scan.Folded(end)] && !scan.IsClaimed(end) {
		currency = true
		end++
	}

	// A bare number with neither a scale nor a currency word is not a price —
	// it is far more likely a quantity nothing else claimed, and guessing would
	// silently filter out most of the board.
	if !scaled && !currency {
		return 0, 0, false
	}
	return amount, end, true
}
