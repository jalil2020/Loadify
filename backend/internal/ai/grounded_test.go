package ai_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/ai"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/nlq"
	"github.com/loadify/backend/pkg/logger"
)

var (
	tashkentID = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	moscowID   = uuid.MustParse("44444444-4444-4444-8444-444444444444")
	qarshiID   = uuid.MustParse("77777777-7777-4777-8777-777777777777")
)

var zone = time.FixedZone("UTC+5", 5*60*60)

func clock() time.Time { return time.Date(2026, 9, 15, 12, 0, 0, 0, zone) }

type vocabulary struct{}

func (vocabulary) Places(context.Context) ([]domain.Place, error) {
	return []domain.Place{
		{ID: tashkentID, Kind: domain.PlaceCity, CountryCode: "UZ", Aliases: []string{"Toshkent", "Ташкент"}},
		{ID: moscowID, Kind: domain.PlaceCity, CountryCode: "RU", Aliases: []string{"Moskva", "Москва"}},
		{ID: qarshiID, Kind: domain.PlaceCity, CountryCode: "UZ", Aliases: []string{"Qarshi", "Карши"}},
	}, nil
}

func (vocabulary) VehicleTypes(context.Context) ([]domain.Term, error) {
	return []domain.Term{{ID: string(domain.VehicleTent), Aliases: []string{"Tent"}}}, nil
}

func (vocabulary) CargoTypes(context.Context) ([]domain.Term, error) { return nil, nil }

// fakeExtractor stands in for the model: it returns whatever the test says a
// model returned, and counts how often it was asked.
type fakeExtractor struct {
	fields map[string]domain.AdExtractedValue
	err    error
	calls  int
}

func (f *fakeExtractor) Extract(context.Context, domain.AdExtractRequest) (domain.AdExtractResult, error) {
	f.calls++
	if f.err != nil {
		return domain.AdExtractResult{}, f.err
	}
	return domain.AdExtractResult{Fields: f.fields, Provider: "fake", Model: "fake-1"}, nil
}

func parserWith(extractor domain.AdExtractor) *ai.GroundedParser {
	rules := nlq.NewAdParser(vocabulary{}, clock, zone)
	return ai.NewGroundedParser(rules, extractor, vocabulary{}, logger.New("test", "error"))
}

func parse(t *testing.T, parser *ai.GroundedParser, text string) domain.AdDraft {
	t.Helper()
	draft, err := parser.Parse(context.Background(), text)
	if err != nil {
		t.Fatalf("Parse(%q): %v", text, err)
	}
	return draft
}

// The useful case: a place the rules could not match because of a typo, which
// the model recognises. The name still has to resolve against our own
// vocabulary before it becomes an id.
func TestModelFillsWhatTheRulesMissed(t *testing.T) {
	extractor := &fakeExtractor{fields: map[string]domain.AdExtractedValue{
		domain.FieldOrigin: {Value: "Карши", Evidence: "Кашри"},
	}}
	draft := parse(t, parserWith(extractor), "Кашри → Москва 20 тонн")

	if draft.Ad.OriginCityID == nil || *draft.Ad.OriginCityID != qarshiID {
		t.Fatalf("origin = %v, want Qarshi", draft.Ad.OriginCityID)
	}
	if got := draft.Sources[domain.FieldOrigin]; got.Source != domain.FieldFromAI || got.Evidence != "Кашри" {
		t.Errorf("origin provenance = %+v, want the model and the words it pointed at", got)
	}
	if !draft.AIUsed {
		t.Error("ai_used must say a model was consulted")
	}
	for _, word := range draft.Dropped {
		if word == "Кашри" {
			t.Error("a word the model explained is still listed as not understood")
		}
	}
}

// Evidence is the whole mechanism: a value whose words are not in the advert is
// a value nobody wrote.
func TestUngroundedValuesAreDiscarded(t *testing.T) {
	extractor := &fakeExtractor{fields: map[string]domain.AdExtractedValue{
		domain.FieldWeight:       {Value: "20000", Evidence: "20 тонн"},
		domain.FieldContactPhone: {Value: "+998901234567", Evidence: "тел 90 123 45 67"},
	}}
	draft := parse(t, parserWith(extractor), "Кашри → Москва срочно")

	if draft.Ad.WeightKg != nil {
		t.Errorf("weight = %v, want nothing: the advert never says twenty tonnes", draft.Ad.WeightKg)
	}
	if draft.Ad.ContactPhone != nil {
		t.Errorf("phone = %v, want nothing: those digits are not in the advert", draft.Ad.ContactPhone)
	}
	if draft.AIUsed {
		t.Error("nothing was applied, so nothing should claim a model contributed")
	}
}

// The model points at words; the rules compute the value. A model that says
// those words mean thirty tonnes does not get to make them mean that.
func TestValuesComeFromTheRulesNotTheModel(t *testing.T) {
	extractor := &fakeExtractor{fields: map[string]domain.AdExtractedValue{
		domain.FieldWeight: {Value: "30000", Evidence: "20 тонн"},
	}}
	draft := parse(t, parserWith(extractor), "Кашри → Москва 20 тонн")

	if draft.Ad.WeightKg == nil || *draft.Ad.WeightKg != 20000 {
		t.Errorf("weight = %v, want 20000 read from the words", draft.Ad.WeightKg)
	}
}

func TestNamesMustResolveAgainstOurVocabulary(t *testing.T) {
	extractor := &fakeExtractor{fields: map[string]domain.AdExtractedValue{
		domain.FieldOrigin: {Value: "Atlantis", Evidence: "Кашри"},
	}}
	draft := parse(t, parserWith(extractor), "Кашри → Москва 20 тонн")

	if draft.Ad.OriginCityID != nil || draft.Ad.OriginRegionID != nil {
		t.Error("a place the platform has no row for must not become an origin")
	}
}

// A confident wrong answer must not displace a right one.
func TestTheModelNeverOverwritesARule(t *testing.T) {
	extractor := &fakeExtractor{fields: map[string]domain.AdExtractedValue{
		domain.FieldDestination: {Value: "Ташкент", Evidence: "Москва"},
	}}
	draft := parse(t, parserWith(extractor), "Кашри → Москва 20 тонн")

	if draft.Ad.DestCityID == nil || *draft.Ad.DestCityID != moscowID {
		t.Errorf("destination = %v, want Moscow as the rules read it", draft.Ad.DestCityID)
	}
	if draft.Sources[domain.FieldDestination].Source != domain.FieldFromRule {
		t.Error("the destination provenance changed under a model's suggestion")
	}
}

func TestTheModelIsNotAskedWhenThereIsNothingToFind(t *testing.T) {
	cases := map[string]string{
		"the rules read everything":  "Ташкент → Москва тел 901234567",
		"nothing was left unread":    "Ташкент → Москва",
		"the advert is only a place": "Ташкент",
	}

	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			extractor := &fakeExtractor{}
			draft := parse(t, parserWith(extractor), text)

			if len(draft.Dropped) == 0 && extractor.calls != 0 {
				t.Errorf("the model was called %d times with nothing to interpret", extractor.calls)
			}
			if draft.AIUsed {
				t.Error("ai_used must stay false when no model contributed")
			}
		})
	}
}

// A model outage degrades the reading; it never fails the request.
func TestAModelFailureLeavesTheRulesResult(t *testing.T) {
	extractor := &fakeExtractor{err: errors.New("upstream timeout")}
	draft := parse(t, parserWith(extractor), "Кашри → Москва 20 тонн")

	if draft.Ad.WeightKg == nil || *draft.Ad.WeightKg != 20000 {
		t.Errorf("weight = %v, want what the rules read", draft.Ad.WeightKg)
	}
	if draft.AIUsed {
		t.Error("a failed call must not be reported as a model contribution")
	}
}

// With no model configured at all the platform still reads adverts.
func TestNoExtractorIsNotAnError(t *testing.T) {
	draft := parse(t, parserWith(nil), "Ташкент → Москва 20 тонн")

	if draft.Ad.DestCityID == nil || *draft.Ad.DestCityID != moscowID {
		t.Errorf("destination = %v, want Moscow", draft.Ad.DestCityID)
	}
}

// The person is told how the model took part, including when it did not.
func TestTheParserSaysHowTheModelTookPart(t *testing.T) {
	typo := "Кашри → Москва 20 тонн"
	cases := []struct {
		name      string
		extractor domain.AdExtractor
		text      string
		want      string
	}{
		{"no model configured", nil, typo, ""},
		{"nothing to ask", &fakeExtractor{}, "Ташкент → Москва", domain.AIStatusNotNeeded},
		{"the model filled a field", &fakeExtractor{fields: map[string]domain.AdExtractedValue{
			domain.FieldOrigin: {Value: "Карши", Evidence: "Кашри"},
		}}, typo, domain.AIStatusUsed},
		{"the model found nothing", &fakeExtractor{}, typo, domain.AIStatusNoResult},
		{"the plan is used up", &fakeExtractor{err: domain.ErrAIQuotaExhausted}, typo, domain.AIStatusLimited},
		{"the day's budget is spent", &fakeExtractor{err: domain.ErrAIBudgetExhausted}, typo, domain.AIStatusLimited},
		{"the provider is down", &fakeExtractor{err: errors.New("503")}, typo, domain.AIStatusUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			draft := parse(t, parserWith(tc.extractor), tc.text)
			if draft.AIStatus != tc.want {
				t.Errorf("ai status = %q, want %q", draft.AIStatus, tc.want)
			}
		})
	}
}
