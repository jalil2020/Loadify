package nlq_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/nlq"
)

// The vocabulary is fixed rather than loaded, so a failing test means the
// parser changed and not that someone renamed a city in the seed data.
var (
	tashkentCityID   = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	tashkentRegionID = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	samarkandCityID  = uuid.MustParse("33333333-3333-4333-8333-333333333333")
	moscowCityID     = uuid.MustParse("44444444-4444-4444-8444-444444444444")
	moscowRegionID   = uuid.MustParse("55555555-5555-4555-8555-555555555555")
	generalCargoID   = uuid.MustParse("66666666-6666-4666-8666-666666666666")
)

var tashkentZone = time.FixedZone("UTC+5", 5*60*60)

// noon on 15 September 2026, so "завтра" is the sixteenth.
func adClock() time.Time {
	return time.Date(2026, 9, 15, 12, 0, 0, 0, tashkentZone)
}

type adVocabulary struct{}

func (adVocabulary) Places(context.Context) ([]domain.Place, error) {
	return []domain.Place{
		// The city is listed before the region on purpose: both reduce to
		// "toshkent", and someone who types that alone means the city.
		{ID: tashkentCityID, Kind: domain.PlaceCity, CountryCode: "UZ", RegionID: &tashkentRegionID,
			Aliases: []string{"Toshkent", "Ташкент", "Tashkent", "Toshkent shahri"}},
		{ID: tashkentRegionID, Kind: domain.PlaceRegion, CountryCode: "UZ",
			Aliases: []string{"Toshkent viloyati", "Ташкентская область"}},
		{ID: samarkandCityID, Kind: domain.PlaceCity, CountryCode: "UZ",
			Aliases: []string{"Samarqand", "Самарканд", "Samarkand"}},
		{ID: moscowCityID, Kind: domain.PlaceCity, CountryCode: "RU", RegionID: &moscowRegionID,
			Aliases: []string{"Moskva", "Москва", "Moscow"}},
	}, nil
}

func (adVocabulary) VehicleTypes(context.Context) ([]domain.Term, error) {
	return []domain.Term{
		{ID: string(domain.VehicleTent), Aliases: []string{"Tentli", "Тентованный"}},
		{ID: string(domain.VehicleRefrigerator), Aliases: []string{"Refrijerator", "Рефрижератор"}},
	}, nil
}

func (adVocabulary) CargoTypes(context.Context) ([]domain.Term, error) {
	return []domain.Term{
		{ID: generalCargoID.String(), Aliases: []string{"Umumiy yuk", "Генеральный груз", "General cargo"}},
	}, nil
}

func parseAd(t *testing.T, text string) domain.AdDraft {
	t.Helper()
	parser := nlq.NewAdParser(adVocabulary{}, adClock, tashkentZone)
	draft, err := parser.Parse(context.Background(), text)
	if err != nil {
		t.Fatalf("Parse(%q): %v", text, err)
	}
	return draft
}

// The advertisement from the specification, written the way it arrives in a
// Telegram group: four lines, two scripts, no punctuation to help.
func TestParsesTheAdvertFromTheSpecification(t *testing.T) {
	draft := parseAd(t, "Ташкент → Москва\n20 тонн\nтент\nзагрузка завтра\n2500$")
	ad := draft.Ad

	if ad.OriginCityID == nil || *ad.OriginCityID != tashkentCityID {
		t.Errorf("origin = %v, want Tashkent", ad.OriginCityID)
	}
	if ad.DestCityID == nil || *ad.DestCityID != moscowCityID {
		t.Errorf("destination = %v, want Moscow", ad.DestCityID)
	}
	if ad.DestCountry == nil || *ad.DestCountry != "RU" {
		t.Errorf("destination country = %v, want RU — a group that takes anything to Russia depends on it", ad.DestCountry)
	}
	if ad.WeightKg == nil || *ad.WeightKg != 20000 {
		t.Errorf("weight = %v, want 20000 kg", ad.WeightKg)
	}
	if ad.VehicleType == nil || *ad.VehicleType != string(domain.VehicleTent) {
		t.Errorf("vehicle = %v, want tent", ad.VehicleType)
	}
	if ad.LoadingDate == nil || ad.LoadingDate.Format(time.DateOnly) != "2026-09-16" {
		t.Errorf("loading date = %v, want 2026-09-16", ad.LoadingDate)
	}
	if ad.PriceAmount == nil || *ad.PriceAmount != 250000 {
		t.Errorf("price = %v, want 250000 minor units", ad.PriceAmount)
	}
	if ad.PriceCurrency == nil || *ad.PriceCurrency != "USD" {
		t.Errorf("currency = %v, want USD", ad.PriceCurrency)
	}
	if len(draft.Dropped) != 0 {
		t.Errorf("nothing should be left over, got %v", draft.Dropped)
	}
	if len(draft.Missing) != 1 || draft.Missing[0] != domain.FieldContactPhone {
		t.Errorf("missing = %v, want only the phone number", draft.Missing)
	}
	if ad.Fingerprint == "" {
		t.Error("a parsed advert must carry its fingerprint")
	}
}

// Uzbek states the direction in the grammar, which is the one thing a keyword
// matcher normally gets wrong.
func TestUzbekGrammarSetsTheDirection(t *testing.T) {
	draft := parseAd(t, "Samarqandga Toshkentdan ertaga 20 tonna tent, tel 90 123 45 67, 5 mln so'm")
	ad := draft.Ad

	if ad.OriginCityID == nil || *ad.OriginCityID != tashkentCityID {
		t.Errorf("origin = %v, want Tashkent — the ablative says so even though it is written second", ad.OriginCityID)
	}
	if ad.DestCityID == nil || *ad.DestCityID != samarkandCityID {
		t.Errorf("destination = %v, want Samarkand", ad.DestCityID)
	}
	if ad.ContactPhone == nil || *ad.ContactPhone != "+998901234567" {
		t.Errorf("phone = %v, want +998901234567", ad.ContactPhone)
	}
	if ad.PriceAmount == nil || *ad.PriceAmount != 500_000_000 {
		t.Errorf("price = %v, want 5 million so'm in minor units", ad.PriceAmount)
	}
	if ad.PriceCurrency == nil || *ad.PriceCurrency != "UZS" {
		t.Errorf("currency = %v, want UZS", ad.PriceCurrency)
	}
	if ad.LoadingDate == nil || ad.LoadingDate.Format(time.DateOnly) != "2026-09-16" {
		t.Errorf("loading date = %v, want tomorrow", ad.LoadingDate)
	}
	if len(draft.Missing) != 0 {
		t.Errorf("missing = %v, want none", draft.Missing)
	}
}

// A place nobody could resolve is named rather than swallowed, and the fields
// it would have filled stay empty.
func TestUnresolvedWordsAreReportedNotGuessed(t *testing.T) {
	draft := parseAd(t, "Qarshidan Nukusga 10 tonna")

	if draft.Ad.OriginCityID != nil || draft.Ad.OriginRegionID != nil {
		t.Error("an unresolved name must not fill the origin")
	}
	if draft.Ad.WeightKg == nil || *draft.Ad.WeightKg != 10000 {
		t.Errorf("weight = %v, want 10000 kg — the rest of the advert still parses", draft.Ad.WeightKg)
	}
	if len(draft.Dropped) != 2 {
		t.Errorf("dropped = %v, want both unknown names", draft.Dropped)
	}
	if len(draft.Missing) != 3 {
		t.Errorf("missing = %v, want origin, destination and phone", draft.Missing)
	}
}

// A figure with no currency beside it is the most consequential thing in the
// advert to get wrong, so it is left unset and reported.
func TestPriceWithoutACurrencyIsNotAssumed(t *testing.T) {
	draft := parseAd(t, "Ташкент Москва 2500")

	if draft.Ad.PriceAmount != nil {
		t.Errorf("price = %v, want nothing", draft.Ad.PriceAmount)
	}
	if len(draft.Dropped) != 1 || draft.Dropped[0] != "2500" {
		t.Errorf("dropped = %v, want the bare figure", draft.Dropped)
	}
}

func TestPhoneNumberIsNotReadAsMoney(t *testing.T) {
	draft := parseAd(t, "Ташкент Москва +998901234567 2500$")

	if draft.Ad.ContactPhone == nil || *draft.Ad.ContactPhone != "+998901234567" {
		t.Errorf("phone = %v", draft.Ad.ContactPhone)
	}
	if draft.Ad.PriceAmount == nil || *draft.Ad.PriceAmount != 250000 {
		t.Errorf("price = %v, want the dollar figure", draft.Ad.PriceAmount)
	}
}

// "16.09" is a date; "20 12" is two numbers. The separator in the raw text is
// the only thing that tells them apart once the tokeniser has split them.
func TestNumericDateNeedsItsSeparator(t *testing.T) {
	dated := parseAd(t, "Ташкент Москва загрузка 16.09")
	if dated.Ad.LoadingDate == nil || dated.Ad.LoadingDate.Format(time.DateOnly) != "2026-09-16" {
		t.Errorf("loading date = %v, want 2026-09-16", dated.Ad.LoadingDate)
	}

	bare := parseAd(t, "Ташкент Москва 20 12")
	if bare.Ad.LoadingDate != nil {
		t.Errorf("loading date = %v, want nothing", bare.Ad.LoadingDate)
	}
}

func TestCurrencies(t *testing.T) {
	cases := map[string]struct {
		minor    int64
		currency string
	}{
		"2500$":         {250000, "USD"},
		"2500 доллар":   {250000, "USD"},
		"5 mln so'm":    {500_000_000, "UZS"},
		"50000 руб":     {5_000_000, "RUB"},
		"300000 тенге":  {30_000_000, "KZT"},
		"2 500 000 сум": {250_000_000, "UZS"},
	}

	for price, want := range cases {
		t.Run(price, func(t *testing.T) {
			draft := parseAd(t, "Ташкент Москва "+price)
			if draft.Ad.PriceAmount == nil || *draft.Ad.PriceAmount != want.minor {
				t.Errorf("amount = %v, want %d", draft.Ad.PriceAmount, want.minor)
			}
			if draft.Ad.PriceCurrency == nil || *draft.Ad.PriceCurrency != want.currency {
				t.Errorf("currency = %v, want %s", draft.Ad.PriceCurrency, want.currency)
			}
		})
	}
}

func TestPaymentWords(t *testing.T) {
	cases := map[string]struct{ terms, method string }{
		"предоплата наличными":    {"prepayment", "cash"},
		"оплата при выгрузке":     {"on_delivery", ""},
		"перечислением, отсрочка": {"deferred", "transfer"},
		"yuklashda naqd": {"on_loading", "cash"},
	}

	for text, want := range cases {
		t.Run(text, func(t *testing.T) {
			draft := parseAd(t, "Ташкент Москва "+text)
			if want.terms != "" {
				if draft.Ad.PaymentTerms == nil || *draft.Ad.PaymentTerms != want.terms {
					t.Errorf("terms = %v, want %s", draft.Ad.PaymentTerms, want.terms)
				}
			}
			if want.method != "" {
				if draft.Ad.PaymentMethod == nil || *draft.Ad.PaymentMethod != want.method {
					t.Errorf("method = %v, want %s", draft.Ad.PaymentMethod, want.method)
				}
			}
		})
	}
}

// Every value carries the words it came from, which is what the interface shows
// as a removable chip so a misread is correctable rather than invisible.
func TestEveryFieldCarriesItsEvidence(t *testing.T) {
	draft := parseAd(t, "Ташкент → Москва 20 тонн тент завтра 2500$")

	for _, field := range []string{
		domain.FieldOrigin, domain.FieldDestination, domain.FieldWeight,
		domain.FieldVehicleType, domain.FieldLoadingDate, domain.FieldPrice,
	} {
		origin, found := draft.Sources[field]
		if !found {
			t.Errorf("%s has no recorded source", field)
			continue
		}
		if origin.Source != domain.FieldFromRule {
			t.Errorf("%s came from %q, want a rule", field, origin.Source)
		}
		if origin.Evidence == "" {
			t.Errorf("%s has no evidence", field)
		}
	}
	if draft.AIUsed {
		t.Error("no model should have been called for an advert the rules understood")
	}
}

// The trade says "торг" as often as it says the figure.
func TestNegotiablePrice(t *testing.T) {
	draft := parseAd(t, "Ташкент Москва 2500$ торг")

	if !draft.Ad.PriceNegotiable {
		t.Error("торг must mark the price as negotiable")
	}
	if draft.Ad.PriceAmount == nil {
		t.Error("a negotiable price is still a price")
	}
}

func TestCargoTypeIsResolved(t *testing.T) {
	draft := parseAd(t, "Ташкент Москва Генеральный груз 20 тонн")

	if draft.Ad.CargoTypeID == nil || *draft.Ad.CargoTypeID != generalCargoID {
		t.Errorf("cargo = %v, want the general cargo type", draft.Ad.CargoTypeID)
	}
}

// A corridor written with a hyphen is as common as one written with an arrow,
// and the tokeniser has to take the hyphen apart without taking Rostov-na-Donu
// apart with it.
func TestHyphenatedCorridor(t *testing.T) {
	draft := parseAd(t, "Ташкент-Москва 20 тонн тент")

	if draft.Ad.OriginCityID == nil || *draft.Ad.OriginCityID != tashkentCityID {
		t.Errorf("origin = %v, want Tashkent", draft.Ad.OriginCityID)
	}
	if draft.Ad.DestCityID == nil || *draft.Ad.DestCityID != moscowCityID {
		t.Errorf("destination = %v, want Moscow", draft.Ad.DestCityID)
	}
}

// An arrow states the direction even when only one end resolves. Without it the
// single known place would be read as the origin, which is the opposite of what
// the advert says.
func TestArrowSurvivesAnUnknownName(t *testing.T) {
	draft := parseAd(t, "Кашри → Москва 20 тонн")

	if draft.Ad.DestCityID == nil || *draft.Ad.DestCityID != moscowCityID {
		t.Errorf("destination = %v, want Moscow", draft.Ad.DestCityID)
	}
	if draft.Ad.OriginCityID != nil {
		t.Error("the unresolved origin must stay empty rather than take Moscow")
	}
	if len(draft.Dropped) != 1 || draft.Dropped[0] != "Кашри" {
		t.Errorf("dropped = %v, want the name nothing understood", draft.Dropped)
	}
}
