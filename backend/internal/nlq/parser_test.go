package nlq_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/nlq"
)

// The vocabulary is fixed so a parse assertion is about the rules, not about
// whatever happens to be seeded. The ids are stable per run and looked up by
// name in the assertions.
var (
	tashkentCity   = uuid.New()
	tashkentRegion = uuid.New()
	samarkand      = uuid.New()
	bukhara        = uuid.New()
	fergana        = uuid.New()
	karakalpak     = uuid.New()
)

type fakeVocabulary struct{}

// The aliases are the names the seeded reference data actually returns,
// qualifiers and Russian adjectival forms included. Using tidier names here
// would have hidden the very thing that broke against the live API.
func (fakeVocabulary) Regions(context.Context) ([]domain.Term, error) {
	return []domain.Term{
		{ID: tashkentCity.String(), Aliases: []string{"Toshkent shahri", "город Ташкент", "Tashkent City"}},
		{ID: tashkentRegion.String(), Aliases: []string{"Toshkent viloyati", "Ташкентская область", "Tashkent Region"}},
		{ID: samarkand.String(), Aliases: []string{"Samarqand", "Самаркандская область", "Samarkand Region"}},
		{ID: bukhara.String(), Aliases: []string{"Buxoro", "Бухарская область", "Bukhara Region"}},
		{ID: fergana.String(), Aliases: []string{"Farg'ona", "Ферганская область", "Fergana Region"}},
		{ID: karakalpak.String(), Aliases: []string{"Qoraqalpog'iston", "Каракалпакстан", "Karakalpakstan"}},
	}, nil
}

func (fakeVocabulary) VehicleTypes(context.Context) ([]domain.Term, error) {
	return []domain.Term{
		{ID: "tent", Aliases: []string{"Tent", "Тент"}},
		{ID: "refrigerator", Aliases: []string{"Refrijerator", "Рефрижератор"}},
		{ID: "grain", Aliases: []string{"Don tashuvchi", "Зерновоз"}},
	}, nil
}

// A fixed clock, so "ertaga" asserts against a known day rather than against
// whenever the suite runs.
func fixedClock() time.Time {
	return time.Date(2026, 8, 6, 15, 30, 0, 0, time.UTC)
}

func parse(t *testing.T, utterance string) domain.SearchIntent {
	t.Helper()
	parser := nlq.NewParser(fakeVocabulary{}, fixedClock)
	intent, err := parser.Parse(context.Background(), utterance)
	if err != nil {
		t.Fatalf("parse(%q): %v", utterance, err)
	}
	return intent
}

func TestAblativeAndDativeCarryDirection(t *testing.T) {
	// The whole design rests on this: Uzbek marks direction on the noun, so the
	// one thing a keyword matcher usually gets wrong is stated outright.
	intent := parse(t, "Toshkent shahridan Samarqandga")

	assertRegions(t, "origin", intent.OriginRegionIDs, tashkentCity)
	assertRegions(t, "destination", intent.DestinationRegionIDs, samarkand)
}

func TestDirectionSurvivesReversedWordOrder(t *testing.T) {
	// Uzbek word order is free; the suffix, not the position, is the signal.
	intent := parse(t, "Samarqandga Buxorodan yuk")

	assertRegions(t, "origin", intent.OriginRegionIDs, bukhara)
	assertRegions(t, "destination", intent.DestinationRegionIDs, samarkand)
}

func TestRussianAndEnglishPrepositions(t *testing.T) {
	ru := parse(t, "из Бухары в Самарканд")
	assertRegions(t, "ru origin", ru.OriginRegionIDs, bukhara)
	assertRegions(t, "ru destination", ru.DestinationRegionIDs, samarkand)

	en := parse(t, "from Bukhara to Samarkand")
	assertRegions(t, "en origin", en.OriginRegionIDs, bukhara)
	assertRegions(t, "en destination", en.DestinationRegionIDs, samarkand)
}

func TestOfficialNamesResolveFromWhatPeopleType(t *testing.T) {
	// Nobody types "Ферганская область" or "Fergana Region"; the reference data
	// stores nothing else. Every one of these reached the live API and came
	// back unresolved before the short forms and the key comparison existed.
	for _, tc := range []struct {
		utterance string
		want      uuid.UUID
	}{
		{"Fergana", fergana},
		{"из Бухары", bukhara},
		{"Бухара", bukhara},
		{"Самарканд", samarkand},
		{"Bukhara", bukhara},
	} {
		intent := parse(t, tc.utterance)
		if len(intent.OriginRegionIDs) != 1 || intent.OriginRegionIDs[0] != tc.want {
			t.Errorf("%q: want %s, got %v (dropped %v)",
				tc.utterance, tc.want, intent.OriginRegionIDs, intent.Dropped)
		}
	}
}

func TestAmbiguousShortFormPrefersTheCity(t *testing.T) {
	// "Toshkent" alone could be the city or the surrounding region. Freight
	// originates in the city far more often, and the vocabulary lists it first,
	// so that is the one a bare mention resolves to — deterministically.
	intent := parse(t, "Toshkentdan Samarqandga")

	assertRegions(t, "origin", intent.OriginRegionIDs, tashkentCity)

	// The full name must still reach the region it actually names.
	explicit := parse(t, "Toshkent viloyatidan Samarqandga")
	assertRegions(t, "origin", explicit.OriginRegionIDs, tashkentRegion)
}

func TestLongerRegionNameWins(t *testing.T) {
	// "Toshkent viloyati" and "Toshkent shahri" are different regions sharing a
	// first word. Matching the short form first would silently pick the wrong
	// one on every Tashkent search.
	intent := parse(t, "Toshkent viloyatidan Buxoroga")

	assertRegions(t, "origin", intent.OriginRegionIDs, tashkentRegion)
	assertRegions(t, "destination", intent.DestinationRegionIDs, bukhara)
}

func TestApostropheAndScriptVariantsResolveAlike(t *testing.T) {
	// Four spellings of one region name, all of which a real keyboard produces.
	for _, spelling := range []string{
		"Farg'onadan", "Fargʻonadan", "Fargonadan", "Фергана",
	} {
		intent := parse(t, spelling+" Samarqandga")
		if len(intent.OriginRegionIDs) != 1 || intent.OriginRegionIDs[0] != fergana {
			t.Errorf("%q did not resolve to Fergana: %v", spelling, intent.OriginRegionIDs)
		}
	}
}

func TestUnmarkedPairFallsBackToWordOrder(t *testing.T) {
	// People drop the grammar when typing fast.
	intent := parse(t, "Buxoro Samarqand tent")

	assertRegions(t, "origin", intent.OriginRegionIDs, bukhara)
	assertRegions(t, "destination", intent.DestinationRegionIDs, samarkand)
}

func TestSingleUnmarkedRegionBecomesOrigin(t *testing.T) {
	// A driver naming one place is almost always saying where they are.
	intent := parse(t, "Buxoro")

	assertRegions(t, "origin", intent.OriginRegionIDs, bukhara)
	if len(intent.DestinationRegionIDs) != 0 {
		t.Errorf("destination should stay empty, got %v", intent.DestinationRegionIDs)
	}
}

func TestVehicleColloquialNames(t *testing.T) {
	for utterance, want := range map[string]domain.VehicleType{
		"ref kerak":     domain.VehicleRefrigerator,
		"фура":          domain.VehicleTent,
		"zernovoz":      domain.VehicleGrain,
		"Don tashuvchi": domain.VehicleGrain,
		"рефрижератор":  domain.VehicleRefrigerator,
	} {
		intent := parse(t, utterance)
		if len(intent.VehicleTypes) != 1 || intent.VehicleTypes[0] != want {
			t.Errorf("%q: want %s, got %v", utterance, want, intent.VehicleTypes)
		}
	}
}

func TestWeightIsNeutralUntilProjected(t *testing.T) {
	// A bare tonnage does not say which way it bounds the search — that is the
	// projection's call, so the parser must not decide it here.
	intent := parse(t, "20 tonna")

	if intent.WeightKg == nil || *intent.WeightKg != 20000 {
		t.Fatalf("want 20000 kg, got %v", intent.WeightKg)
	}
	if intent.MinWeightKg != nil || intent.MaxWeightKg != nil {
		t.Error("a bare tonnage must not set a bound")
	}
}

func TestComparativeSetsTheBound(t *testing.T) {
	over := parse(t, "20 tonnadan ortiq")
	if over.MinWeightKg == nil || *over.MinWeightKg != 20000 {
		t.Errorf("want min 20000, got %v", over.MinWeightKg)
	}

	under := parse(t, "10 tonnadan kam")
	if under.MaxWeightKg == nil || *under.MaxWeightKg != 10000 {
		t.Errorf("want max 10000, got %v", under.MaxWeightKg)
	}
}

func TestComparativeWrittenApartFromTheNoun(t *testing.T) {
	// "so'm dan ortiq" leaves a bare "dan" between the figure and the
	// comparative. Reported as unrecognised against the live API before the
	// bound rule learned to step over it.
	intent := parse(t, "500 ming so'm dan ortiq")

	if intent.MinPrice == nil || *intent.MinPrice != 50_000_000 {
		t.Fatalf("want min 50000000, got %v", intent.MinPrice)
	}
	if len(intent.Dropped) != 0 {
		t.Errorf("nothing should have been dropped, got %v", intent.Dropped)
	}
}

func TestGluedNumberAndUnit(t *testing.T) {
	// "20t" is what a thumb types.
	intent := parse(t, "20t Samarqandga")

	if intent.WeightKg == nil || *intent.WeightKg != 20000 {
		t.Fatalf("want 20000 kg, got %v", intent.WeightKg)
	}
	assertRegions(t, "destination", intent.DestinationRegionIDs, samarkand)
}

func TestKilogramsStayKilograms(t *testing.T) {
	intent := parse(t, "500 kg")
	if intent.WeightKg == nil || *intent.WeightKg != 500 {
		t.Fatalf("want 500 kg, got %v", intent.WeightKg)
	}
}

func TestPriceScalesToMinorUnits(t *testing.T) {
	// "500 ming" is 500 000 soʻm, stored as tiyin like every other amount.
	intent := parse(t, "500 ming so'm")

	if intent.MinPrice == nil || *intent.MinPrice != 50_000_000 {
		t.Fatalf("want 50000000 tiyin, got %v", intent.MinPrice)
	}
	if intent.Currency != "UZS" {
		t.Errorf("want UZS, got %q", intent.Currency)
	}
}

func TestBareNumberIsNotAPrice(t *testing.T) {
	// Guessing here would silently filter out most of the board.
	intent := parse(t, "Buxoro 5")

	if intent.MinPrice != nil || intent.MaxPrice != nil {
		t.Errorf("a bare number must not become a price: %v %v", intent.MinPrice, intent.MaxPrice)
	}
}

func TestDayWordsResolveAgainstTheClock(t *testing.T) {
	intent := parse(t, "ertaga Samarqandga")

	if intent.ReadyFrom == nil || intent.ReadyTo == nil {
		t.Fatal("ertaga produced no window")
	}
	wantFrom := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	if !intent.ReadyFrom.Equal(wantFrom) {
		t.Errorf("want from %s, got %s", wantFrom, intent.ReadyFrom)
	}
	// One specific day, not "from tomorrow onwards".
	if !intent.ReadyTo.Equal(wantFrom.AddDate(0, 0, 1)) {
		t.Errorf("want a one-day window, got %s..%s", intent.ReadyFrom, intent.ReadyTo)
	}
}

func TestDaySpanOpensNow(t *testing.T) {
	// "3 kun" is a span, not a single day.
	intent := parse(t, "3 kun ichida")

	if intent.ReadyFrom == nil || intent.ReadyTo == nil {
		t.Fatal("3 kun produced no window")
	}
	wantFrom := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	if !intent.ReadyFrom.Equal(wantFrom) || !intent.ReadyTo.Equal(wantFrom.AddDate(0, 0, 3)) {
		t.Errorf("want a three-day window from today, got %s..%s", intent.ReadyFrom, intent.ReadyTo)
	}
	// The count must not also have been read as a tonnage.
	if intent.WeightKg != nil {
		t.Errorf("3 kun leaked into the weight: %v", intent.WeightKg)
	}
}

func TestFlags(t *testing.T) {
	intent := parse(t, "xavfli yuk Samarqandga")
	if intent.RequiresADR == nil || !*intent.RequiresADR {
		t.Error("xavfli did not set the ADR flag")
	}
}

func TestTheFullSentence(t *testing.T) {
	intent := parse(t, "Toshkent shahridan Samarqandga ertaga 20 tonna tent 500 ming so'm")

	assertRegions(t, "origin", intent.OriginRegionIDs, tashkentCity)
	assertRegions(t, "destination", intent.DestinationRegionIDs, samarkand)
	if len(intent.VehicleTypes) != 1 || intent.VehicleTypes[0] != domain.VehicleTent {
		t.Errorf("vehicle: %v", intent.VehicleTypes)
	}
	if intent.WeightKg == nil || *intent.WeightKg != 20000 {
		t.Errorf("weight: %v", intent.WeightKg)
	}
	if intent.MinPrice == nil || *intent.MinPrice != 50_000_000 {
		t.Errorf("price: %v", intent.MinPrice)
	}
	if intent.ReadyFrom == nil {
		t.Error("no loading window")
	}
	if len(intent.Dropped) != 0 {
		t.Errorf("nothing should have been dropped, got %v", intent.Dropped)
	}
}

func TestUnresolvableRegionIsDropped(t *testing.T) {
	// Qarshi is not in this vocabulary. Silently widening the search to every
	// region is the failure worth guarding: the driver must see it was ignored.
	intent := parse(t, "Qarshidan Samarqandga")

	assertRegions(t, "destination", intent.DestinationRegionIDs, samarkand)
	if len(intent.OriginRegionIDs) != 0 {
		t.Errorf("an unknown region must not resolve: %v", intent.OriginRegionIDs)
	}
	if !slices.Contains(intent.Dropped, "Qarshidan") {
		t.Errorf("Qarshidan should be reported as dropped, got %v", intent.Dropped)
	}
}

func TestEmptyUtteranceIsEmpty(t *testing.T) {
	if !parse(t, "   ").IsEmpty() {
		t.Error("blank input produced a non-empty intent")
	}
}

func assertRegions(t *testing.T, label string, got []uuid.UUID, want ...uuid.UUID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: want %d region(s) %v, got %d %v", label, len(want), want, len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d]: want %s, got %s", label, i, want[i], got[i])
		}
	}
}
