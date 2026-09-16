package nlq

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
)

// ─── body type ──────────────────────────────────────────────────────

// AdVehicleExtractor resolves the body type an advert asks for.
type AdVehicleExtractor struct {
	index *AliasIndex[domain.VehicleType]
}

// NewAdVehicleExtractor folds the server's labels together with the words
// people actually use. When the vocabulary is unavailable the colloquial names
// alone still resolve, because "tent" and "реф" are what most adverts say.
func NewAdVehicleExtractor(vehicles []domain.Term) *AdVehicleExtractor {
	index := NewAliasIndex[domain.VehicleType]()
	for _, term := range vehicles {
		index.Add(domain.VehicleType(term.ID), term.Aliases)
	}
	for code, aliases := range colloquialAliases {
		index.Add(code, aliases)
	}
	return &AdVehicleExtractor{index: index}
}

func (e *AdVehicleExtractor) Name() string { return "vehicle" }

func (e *AdVehicleExtractor) Extract(scan *Scan, out *domain.AdDraft) {
	for start := range scan.Tokens {
		if scan.IsClaimed(start) {
			continue
		}
		code, end, _, ok := e.index.MatchAt(scan, start)
		if !ok {
			continue
		}

		evidence := surfacePhrase(scan, start, end)
		scan.Claim(start, end, e.Name())
		value := string(code)
		out.Ad.VehicleType = &value
		out.Note(domain.FieldVehicleType, domain.FieldFromRule, evidence)
		return
	}
}

// ─── cargo ──────────────────────────────────────────────────────────

// AdCargoExtractor resolves what is being carried.
type AdCargoExtractor struct{ index *AliasIndex[uuid.UUID] }

// NewAdCargoExtractor indexes the cargo vocabulary.
func NewAdCargoExtractor(cargoTypes []domain.Term) *AdCargoExtractor {
	index := NewAliasIndex[uuid.UUID]()
	for _, term := range cargoTypes {
		id, err := uuid.Parse(term.ID)
		if err != nil {
			continue
		}
		index.Add(id, term.Aliases)
	}
	return &AdCargoExtractor{index: index}
}

func (e *AdCargoExtractor) Name() string { return "cargo" }

func (e *AdCargoExtractor) Extract(scan *Scan, out *domain.AdDraft) {
	for start := range scan.Tokens {
		if scan.IsClaimed(start) {
			continue
		}
		id, end, _, ok := e.index.MatchAt(scan, start)
		if !ok {
			continue
		}

		evidence := surfacePhrase(scan, start, end)
		scan.Claim(start, end, e.Name())
		cargoID := id
		out.Ad.CargoTypeID = &cargoID
		out.Ad.CargoText = &evidence
		out.Note(domain.FieldCargoType, domain.FieldFromRule, evidence)
		return
	}
}

// ─── contact ────────────────────────────────────────────────────────

// phoneWords introduce a number that is a phone rather than a figure. Folding
// turns "тел" into "tel", so one entry covers both scripts.
var phoneWords = map[string]bool{
	"tel": true, "telefon": true, "telefonlar": true, "aloqa": true,
	"kontakt": true, "kontakti": true, "contact": true, "phone": true,
	"whatsapp": true, "vatsap": true, "mob": true, "mobil": true, "nomer": true,
}

// ContactExtractor claims the phone number.
//
// It runs before every rule that reads a figure, because a phone number is a
// long run of digits and would otherwise be read as a price.
type ContactExtractor struct{}

// NewContactExtractor builds the extractor.
func NewContactExtractor() *ContactExtractor { return &ContactExtractor{} }

func (e *ContactExtractor) Name() string { return "contact" }

func (e *ContactExtractor) Extract(scan *Scan, out *domain.AdDraft) {
	for i := range scan.Tokens {
		if scan.IsClaimed(i) {
			continue
		}

		// A number written in one piece: +998901234567, 998901234567.
		if digits := digitsOf(scan.Folded(i)); len(digits) >= 9 && len(digits) <= 15 && isAllDigits(scan.Folded(i)) {
			e.claim(scan, out, i, i+1, digits)
			return
		}

		// A number written in groups after a label: "tel 90 123 45 67". The
		// label is what makes this safe; without it, four small numbers in a
		// row are far more likely to be a tonnage and a price.
		if !phoneWords[scan.Folded(i)] {
			continue
		}
		digits, end := collectDigits(scan, i+1)
		if len(digits) >= 7 && len(digits) <= 15 {
			e.claim(scan, out, i, end, digits)
			return
		}
	}
}

func (e *ContactExtractor) claim(scan *Scan, out *domain.AdDraft, from, to int, digits string) {
	evidence := surfacePhrase(scan, from, to)
	scan.Claim(from, to, e.Name())
	phone := normalisePhone(digits)
	out.Ad.ContactPhone = &phone
	out.Note(domain.FieldContactPhone, domain.FieldFromRule, evidence)
}

// phoneLengths are the digit counts a complete number has here: nine for a
// local Uzbek mobile, eleven for a Russian number written with its domestic 8,
// twelve with a country code.
//
// Stopping at the first of them is what keeps the next figure in the advert out
// of the phone number: "tel 90 123 45 67 5 mln so`m" ends the number at 67.
var phoneLengths = map[int]bool{9: true, 11: true, 12: true}

// collectDigits gathers consecutive all-digit tokens, which is how a number is
// written when it is spaced into groups.
func collectDigits(scan *Scan, from int) (string, int) {
	var digits strings.Builder
	i := from
	for ; i < len(scan.Tokens); i++ {
		folded := scan.Folded(i)
		if scan.IsClaimed(i) || !isAllDigits(folded) || digits.Len()+len(folded) > 15 {
			break
		}
		digits.WriteString(digitsOf(folded))
		if phoneLengths[digits.Len()] {
			i++
			break
		}
	}
	return digits.String(), i
}

// normalisePhone puts a number into the shape the rest of the platform stores,
// adding the country code only where the local format makes it unambiguous.
func normalisePhone(digits string) string {
	switch {
	case len(digits) == 9:
		// A bare Uzbek mobile: 90 123 45 67.
		return "+998" + digits
	case strings.HasPrefix(digits, "998") && len(digits) == 12:
		return "+" + digits
	case strings.HasPrefix(digits, "8") && len(digits) == 11:
		// Russian numbers are written with a domestic 8; the international
		// form of the same number starts with 7.
		return "+7" + digits[1:]
	default:
		return "+" + digits
	}
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func digitsOf(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// ─── loading date ───────────────────────────────────────────────────

// loadingWords mark the date rather than carrying meaning of their own. They
// are claimed so a perfectly understood advert does not report "загрузка" as a
// word nothing understood.
var loadingWords = map[string]bool{
	"yuklash": true, "yuklama": true, "yuklanadi": true, "yuklash-": true,
	"zagruzka": true, "zagruzki": true, "pogruzka": true, "pogruzki": true,
	"otgruzka": true, "loading": true, "load": true, "data": true, "date": true,
}

// monthStems are the months in all three languages, folded and cut to the part
// every inflection shares: "сентября", "сентябрь" and "sentyabr" all begin with
// "sentyabr".
var monthStems = map[string]time.Month{
	"yanvar": time.January, "january": time.January, "jan": time.January,
	"fevral": time.February, "february": time.February, "feb": time.February,
	"mart": time.March, "march": time.March, "mar": time.March,
	"aprel": time.April, "april": time.April, "apr": time.April,
	"may": time.May, "maya": time.May,
	"iyun": time.June, "june": time.June, "jun": time.June,
	"iyul": time.July, "july": time.July, "jul": time.July,
	"avgust": time.August, "august": time.August, "aug": time.August,
	"sentyabr": time.September, "september": time.September, "sep": time.September,
	"oktyabr": time.October, "october": time.October, "oct": time.October,
	"noyabr": time.November, "november": time.November, "nov": time.November,
	"dekabr": time.December, "december": time.December, "dec": time.December,
}

// numericDate matches the dotted and slashed forms, which the tokeniser splits
// apart because a full stop is not part of a word. Reading them off the raw
// text is what keeps "16.09" a date rather than two numbers.
var numericDate = regexp.MustCompile(`\b(\d{1,2})[./](\d{1,2})(?:[./](\d{2,4}))?\b`)

// AdDateExtractor resolves the loading day.
//
// Relative words are resolved here, against the company's own clock, rather
// than being handed to a model: "ertaga" has to mean the person's tomorrow, and
// a date is exactly the kind of thing a model gets subtly wrong.
type AdDateExtractor struct {
	raw string
	now Clock
	loc *time.Location
}

// NewAdDateExtractor builds the extractor over the raw text and a clock.
func NewAdDateExtractor(raw string, now Clock, loc *time.Location) *AdDateExtractor {
	if now == nil {
		now = time.Now
	}
	if loc == nil {
		loc = defaultLocation()
	}
	return &AdDateExtractor{raw: raw, now: now, loc: loc}
}

func (e *AdDateExtractor) Name() string { return "date" }

func (e *AdDateExtractor) Extract(scan *Scan, out *domain.AdDraft) {
	e.claimLoadingWords(scan)

	for i := range scan.Tokens {
		if scan.IsClaimed(i) {
			continue
		}
		if day, end, ok := e.readAt(scan, i); ok {
			evidence := surfacePhrase(scan, i, end)
			scan.Claim(i, end, e.Name())
			date := day
			text := evidence
			out.Ad.LoadingDate = &date
			out.Ad.LoadingDateText = &text
			out.Note(domain.FieldLoadingDate, domain.FieldFromRule, evidence)
			return
		}
	}
}

func (e *AdDateExtractor) claimLoadingWords(scan *Scan) {
	for i := range scan.Tokens {
		if !scan.IsClaimed(i) && loadingWords[scan.Folded(i)] {
			scan.Claim(i, i+1, e.Name())
		}
	}
}

// readAt reads a day word, a "16 sentyabr" form, or a "16.09" one.
func (e *AdDateExtractor) readAt(scan *Scan, i int) (time.Time, int, bool) {
	today := e.startOfToday()

	if offset, found := dayOffsets[scan.Folded(i)]; found {
		return today.AddDate(0, 0, offset), i + 1, true
	}

	day, ok := parseNumber(scan.Folded(i))
	if !ok || day < 1 || day > 31 {
		return time.Time{}, 0, false
	}

	// "16 sentyabr" and "16 сентября".
	if month, found := monthFromWord(scan.Folded(i + 1)); found && !scan.IsClaimed(i+1) {
		return e.resolve(int(day), month, 0), i + 2, true
	}

	// "16.09", split by the tokeniser into two numbers. Only a pair that really
	// was written with a separator counts, so "20 12" stays a tonnage.
	if month, ok := parseNumber(scan.Folded(i + 1)); ok && !scan.IsClaimed(i+1) &&
		month >= 1 && month <= 12 && e.writtenAsDate(scan.Tokens[i].Surface, scan.Tokens[i+1].Surface) {
		return e.resolve(int(day), time.Month(month), 0), i + 2, true
	}
	return time.Time{}, 0, false
}

// writtenAsDate reports whether the two surfaces appear side by side in the raw
// text separated by a date separator.
func (e *AdDateExtractor) writtenAsDate(day, month string) bool {
	for _, match := range numericDate.FindAllStringSubmatch(e.raw, -1) {
		if match[1] == day && match[2] == month {
			return true
		}
	}
	return false
}

// resolve turns a day and month into a date, choosing the year the writer
// meant: a date that has already passed by more than a week is next year's,
// because an advert is never about last month.
func (e *AdDateExtractor) resolve(day int, month time.Month, year int) time.Time {
	today := e.startOfToday()
	if year == 0 {
		year = today.Year()
	}
	date := time.Date(year, month, day, 0, 0, 0, 0, e.loc)
	if date.Before(today.AddDate(0, 0, -7)) {
		date = date.AddDate(1, 0, 0)
	}
	return date
}

func (e *AdDateExtractor) startOfToday() time.Time {
	now := e.now().In(e.loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, e.loc)
}

func monthFromWord(folded string) (time.Month, bool) {
	for stem, month := range monthStems {
		if strings.HasPrefix(folded, stem) {
			return month, true
		}
	}
	return 0, false
}

// ─── weight and volume ──────────────────────────────────────────────

// AdWeightExtractor reads the tonnage.
type AdWeightExtractor struct{}

// NewAdWeightExtractor builds the extractor.
func NewAdWeightExtractor() *AdWeightExtractor { return &AdWeightExtractor{} }

func (e *AdWeightExtractor) Name() string { return "weight" }

func (e *AdWeightExtractor) Extract(scan *Scan, out *domain.AdDraft) {
	for i := range scan.Tokens {
		if scan.IsClaimed(i) {
			continue
		}
		kg, end, ok := readWeightAt(scan, i)
		if !ok {
			continue
		}

		evidence := surfacePhrase(scan, i, end)
		scan.Claim(i, end, e.Name())
		value := kg
		out.Ad.WeightKg = &value
		out.Note(domain.FieldWeight, domain.FieldFromRule, evidence)
		return
	}
}

func readWeightAt(scan *Scan, i int) (int, int, bool) {
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

// volumeUnits are the cubic-metre spellings, folded. "м3" folds to "m3".
var volumeUnits = map[string]bool{
	"m3": true, "kub": true, "kubm": true, "kubmetr": true, "kubatura": true,
	"cbm": true, "kubov": true, "kuba": true, "m": true,
}

// AdVolumeExtractor reads the cubic metres.
type AdVolumeExtractor struct{}

// NewAdVolumeExtractor builds the extractor.
func NewAdVolumeExtractor() *AdVolumeExtractor { return &AdVolumeExtractor{} }

func (e *AdVolumeExtractor) Name() string { return "volume" }

func (e *AdVolumeExtractor) Extract(scan *Scan, out *domain.AdDraft) {
	for i := range scan.Tokens {
		if scan.IsClaimed(i) {
			continue
		}
		cubic, end, ok := e.readAt(scan, i)
		if !ok {
			continue
		}

		evidence := surfacePhrase(scan, i, end)
		scan.Claim(i, end, e.Name())
		value := cubic
		out.Ad.VolumeM3 = &value
		out.Note(domain.FieldVolume, domain.FieldFromRule, evidence)
		return
	}
}

func (e *AdVolumeExtractor) readAt(scan *Scan, i int) (float64, int, bool) {
	if value, unit, ok := splitNumberUnit(scan.Folded(i)); ok && volumeUnits[unit] {
		return float64(value), i + 1, true
	}
	value, ok := parseNumber(scan.Folded(i))
	if !ok || scan.IsClaimed(i+1) || !volumeUnits[scan.Folded(i+1)] {
		return 0, 0, false
	}
	return float64(value), i + 2, true
}

// ─── price ──────────────────────────────────────────────────────────

// currencyPatterns read the money marks off the raw text, because the ones that
// matter most are punctuation the tokeniser drops: "2500$" arrives as the bare
// number 2500, and a price without its currency is not a price.
var currencyPatterns = []struct {
	re   *regexp.Regexp
	code string
}{
	{regexp.MustCompile(`(?i)(\d[\d.,]*(?:\s\d{3})*)\s*(?:\$|usd|у\.?\s?е\.?|дол\w*|доллар\w*|dollar\w*)`), "USD"},
	{regexp.MustCompile(`(?i)\$\s*(\d[\d.,]*(?:\s\d{3})*)`), "USD"},
	{regexp.MustCompile(`(?i)(\d[\d.,]*(?:\s\d{3})*)\s*(?:сум\w*|сўм\w*|so.?m\w*|sum\w*|uzs)`), "UZS"},
	{regexp.MustCompile(`(?i)(\d[\d.,]*(?:\s\d{3})*)\s*(?:руб\w*|rub\w*|₽|rur)`), "RUB"},
	{regexp.MustCompile(`(?i)(\d[\d.,]*(?:\s\d{3})*)\s*(?:тенге|tenge|kzt|₸)`), "KZT"},
}

// currencyWords catch the same marks when they survive tokenising.
var currencyWords = map[string]string{
	"usd": "USD", "dollar": "USD", "dollars": "USD", "dol": "USD", "ue": "USD",
	"som": "UZS", "sum": "UZS", "sums": "UZS", "uzs": "UZS", "sm": "UZS",
	"rub": "RUB", "rubl": "RUB", "rubley": "RUB", "rur": "RUB",
	"tenge": "KZT", "kzt": "KZT",
}

// negotiableWords mark a price as open to haggling, which in this trade is said
// as often as the figure itself.
var negotiableWords = map[string]bool{
	"torg": true, "dogovornaya": true, "dogovorn": true, "kelishamiz": true,
	"kelishilgan": true, "kelishiladi": true, "negotiable": true, "torgu": true,
}

// AdPriceExtractor reads the rate and the currency it was named in.
type AdPriceExtractor struct{ hints map[string]string }

// NewAdPriceExtractor precomputes the currency marks in the raw text.
func NewAdPriceExtractor(raw string) *AdPriceExtractor {
	hints := make(map[string]string)
	for _, pattern := range currencyPatterns {
		for _, match := range pattern.re.FindAllStringSubmatch(raw, -1) {
			if digits := digitsOf(match[1]); digits != "" {
				hints[digits] = pattern.code
			}
		}
	}
	return &AdPriceExtractor{hints: hints}
}

func (e *AdPriceExtractor) Name() string { return "price" }

func (e *AdPriceExtractor) Extract(scan *Scan, out *domain.AdDraft) {
	e.claimNegotiable(scan, out)

	for i := range scan.Tokens {
		if scan.IsClaimed(i) {
			continue
		}
		amount, currency, end, ok := e.readAt(scan, i)
		if !ok {
			continue
		}

		evidence := surfacePhrase(scan, i, end)
		scan.Claim(i, end, e.Name())
		// Money is stored in minor units everywhere on the platform.
		minor := amount * 100
		code := currency
		out.Ad.PriceAmount = &minor
		out.Ad.PriceCurrency = &code
		out.Note(domain.FieldPrice, domain.FieldFromRule, evidence)
		return
	}
}

func (e *AdPriceExtractor) claimNegotiable(scan *Scan, out *domain.AdDraft) {
	for i := range scan.Tokens {
		if scan.IsClaimed(i) {
			continue
		}
		if negotiableWords[scan.Folded(i)] {
			scan.Claim(i, i+1, e.Name())
			out.Ad.PriceNegotiable = true
		}
	}
}

// readAt returns the amount and the currency it was named in.
//
// A figure whose currency nobody stated is left alone: on a corridor priced in
// dollars, som and roubles by different people, assuming one would be inventing
// the most consequential number in the advert.
func (e *AdPriceExtractor) readAt(scan *Scan, i int) (int64, string, int, bool) {
	// A figure written in groups — "2 500" — reaches here as separate tokens,
	// so the marks found in the raw text are matched against the joined digits
	// as well as the single token.
	digits := ""
	for offset := range 3 {
		token := scan.Folded(i + offset)
		if offset > 0 && (scan.IsClaimed(i+offset) || !isAllDigits(token)) {
			break
		}
		if !isAllDigits(token) {
			break
		}
		digits += token
		if code, found := e.hints[digits]; found {
			amount, ok := parseNumber(digits)
			return amount, code, i + offset + 1, ok
		}
	}

	amount, ok := parseNumber(scan.Folded(i))
	if !ok || amount == 0 {
		return 0, "", 0, false
	}
	end := i + 1

	// "500 ming" is half a million, and a rate is quoted that way far more
	// often than the zeros are typed.
	if scale, found := multipliers[scan.Folded(end)]; found && !scan.IsClaimed(end) {
		amount *= scale
		end++
	}
	if code, found := currencyWords[scan.Folded(end)]; found && !scan.IsClaimed(end) {
		return amount, code, end + 1, true
	}
	if code, found := currencyWords[scan.Folded(i-1)]; found {
		return amount, code, end, true
	}
	return 0, "", 0, false
}

// ─── payment ────────────────────────────────────────────────────────

// paymentTermWords and paymentMethodWords are matched on the stem, because
// Russian inflects both ("предоплата", "предоплатой") and Uzbek suffixes them.
var paymentTermWords = map[string]string{
	"predoplat": "prepayment", "avans": "prepayment", "oldindan": "prepayment",
	"prepayment": "prepayment",
	"yuklashda":  "on_loading", "pogruzke": "on_loading", "zagruzke": "on_loading",
	"vigruzke": "on_delivery", "razgruzke": "on_delivery", "yetkazilganda": "on_delivery",
	"fakt": "on_delivery", "otsrochk": "deferred", "kechiktir": "deferred",
	"deferred": "deferred",
}

var paymentMethodWords = map[string]string{
	"nal": "cash", "nalichn": "cash", "naqd": "cash", "cash": "cash",
	"beznal": "transfer", "perechisl": "transfer", "bank": "transfer",
	"transfer": "transfer", "pereviod": "transfer",
	"karta": "card", "kartoy": "card", "card": "card", "plastik": "card",
}

// PaymentExtractor reads how and when the money arrives.
type PaymentExtractor struct{}

// NewPaymentExtractor builds the extractor.
func NewPaymentExtractor() *PaymentExtractor { return &PaymentExtractor{} }

func (e *PaymentExtractor) Name() string { return "payment" }

func (e *PaymentExtractor) Extract(scan *Scan, out *domain.AdDraft) {
	for i := range scan.Tokens {
		if scan.IsClaimed(i) {
			continue
		}
		folded := scan.Folded(i)

		if terms, found := matchStem(paymentTermWords, folded); found && out.Ad.PaymentTerms == nil {
			value := terms
			out.Ad.PaymentTerms = &value
			out.Note(domain.FieldPaymentTerms, domain.FieldFromRule, scan.Tokens[i].Surface)
			scan.Claim(i, i+1, e.Name())
			continue
		}
		if method, found := matchStem(paymentMethodWords, folded); found && out.Ad.PaymentMethod == nil {
			value := method
			out.Ad.PaymentMethod = &value
			scan.Claim(i, i+1, e.Name())
		}
	}
}

func matchStem(table map[string]string, folded string) (string, bool) {
	if value, found := table[folded]; found {
		return value, true
	}
	for stem, value := range table {
		if strings.HasPrefix(folded, stem) {
			return value, true
		}
	}
	return "", false
}
