package matching_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/matching"
)

var (
	tashkentRegion = uuid.New()
	moscowRegion   = uuid.New()
	ferganaRegion  = uuid.New()
	generalCargo   = uuid.New()
)

func ptr[T any](v T) *T { return &v }

var now = time.Date(2026, 9, 16, 11, 0, 0, 0, time.UTC)

// The advert every test ranks against: Tashkent to Moscow, twenty tonnes, tent.
func advert() *domain.LoadAd {
	return &domain.LoadAd{
		OriginRegionID: &tashkentRegion,
		OriginCountry:  ptr("UZ"),
		DestRegionID:   &moscowRegion,
		DestCountry:    ptr("RU"),
		WeightKg:       ptr(20000),
		VehicleType:    ptr(string(domain.VehicleTent)),
		CargoTypeID:    &generalCargo,
	}
}

type candidateOption func(*domain.GroupCandidate)

func candidate(title string, options ...candidateOption) domain.GroupCandidate {
	c := domain.GroupCandidate{
		Group: domain.TelegramGroup{
			ID: uuid.New(), Title: title,
			CanSend: true, PostingEnabled: true, Active: true,
		},
		Rules: domain.TelegramGroupRules{HourlyLimit: 2, DailyLimit: 3, CooldownMinutes: 60},
	}
	for _, option := range options {
		option(&c)
	}
	return c
}

func withRoute(route domain.TelegramGroupRoute) candidateOption {
	return func(c *domain.GroupCandidate) { c.Routes = append(c.Routes, route) }
}

func regionRoute() domain.TelegramGroupRoute {
	return domain.TelegramGroupRoute{OriginRegionID: &tashkentRegion, DestRegionID: &moscowRegion}
}

func countryRoute() domain.TelegramGroupRoute {
	return domain.TelegramGroupRoute{OriginCountry: ptr("UZ"), DestCountry: ptr("RU")}
}

func titlesOf(ranked []matching.Ranked) []string {
	titles := make([]string, 0, len(ranked))
	for _, r := range ranked {
		titles = append(titles, r.Candidate.Group.Title)
	}
	return titles
}

// A group that serves the whole corridor beats one that serves the countries,
// which beats one that only shares an end.
func TestTheCloserTheCorridorTheHigherItRanks(t *testing.T) {
	result := matching.Rank(advert(), []domain.GroupCandidate{
		candidate("countries", withRoute(countryRoute())),
		candidate("one end", withRoute(domain.TelegramGroupRoute{
			OriginRegionID: &tashkentRegion, DestCountry: ptr("KZ"),
		})),
		candidate("corridor", withRoute(regionRoute())),
	}, now)

	want := []string{"corridor", "countries", "one end"}
	got := titlesOf(result.Ranked)
	if len(got) != len(want) {
		t.Fatalf("ranked %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// Posting a Tashkent-Moscow load into a group that only carries Fergana-Andijan
// is exactly the noise this product exists to stop.
func TestAGroupOnAnotherCorridorIsNotOffered(t *testing.T) {
	elsewhere := candidate("fergana only", withRoute(domain.TelegramGroupRoute{
		OriginRegionID: &ferganaRegion, DestRegionID: &ferganaRegion,
	}))

	result := matching.Rank(advert(), []domain.GroupCandidate{elsewhere}, now)

	if len(result.Ranked) != 0 {
		t.Fatalf("ranked %v, want none", titlesOf(result.Ranked))
	}
	if len(result.Excluded) != 1 || result.Excluded[0].Reason != domain.SkipGroupUnavailable {
		t.Errorf("excluded = %+v", result.Excluded)
	}
}

func TestWhatKeepsAGroupOutOfTheList(t *testing.T) {
	cases := map[string]struct {
		option candidateOption
		reason domain.SkipReason
	}{
		"posting is switched off": {
			option: func(c *domain.GroupCandidate) { c.Group.PostingEnabled = false },
			reason: domain.SkipGroupUnavailable,
		},
		"the account cannot write there": {
			option: func(c *domain.GroupCandidate) { c.Group.CanSend = false },
			reason: domain.SkipGroupUnavailable,
		},
		"the group takes another body type": {
			option: func(c *domain.GroupCandidate) {
				c.Rules.VehicleTypes = domain.StringArray{string(domain.VehicleRefrigerator)}
			},
			reason: domain.SkipGroupUnavailable,
		},
		"the load is heavier than the group takes": {
			option: func(c *domain.GroupCandidate) { c.Rules.MaxWeightKg = ptr(10000) },
			reason: domain.SkipGroupUnavailable,
		},
		"it is the middle of the night there": {
			option: func(c *domain.GroupCandidate) {
				c.Rules.QuietFromHour, c.Rules.QuietToHour = ptr(int16(22)), ptr(int16(14))
			},
			reason: domain.SkipQuietHours,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			result := matching.Rank(advert(),
				[]domain.GroupCandidate{candidate("group", withRoute(regionRoute()), tc.option)}, now)

			if len(result.Ranked) != 0 {
				t.Fatalf("ranked %v, want none", titlesOf(result.Ranked))
			}
			if len(result.Excluded) != 1 || result.Excluded[0].Reason != tc.reason {
				t.Errorf("excluded = %+v, want %q", result.Excluded, tc.reason)
			}
		})
	}
}

// A group that names body types and names this one fits better than a group
// that names none, even though both accept the advert.
func TestStatedRulesOutrankSilence(t *testing.T) {
	specific := candidate("asks for tents", withRoute(regionRoute()), func(c *domain.GroupCandidate) {
		c.Rules.VehicleTypes = domain.StringArray{string(domain.VehicleTent)}
		c.Rules.CargoTypeCodes = domain.StringArray{"general"}
	})
	general := candidate("takes anything", withRoute(regionRoute()))

	result := matching.Rank(advert(), []domain.GroupCandidate{general, specific}, now)

	if got := titlesOf(result.Ranked); len(got) != 2 || got[0] != "asks for tents" {
		t.Errorf("ranked %v, want the more specific group first", got)
	}
	if result.Ranked[0].Score <= result.Ranked[1].Score {
		t.Errorf("scores %d and %d do not separate the two", result.Ranked[0].Score, result.Ranked[1].Score)
	}
}

// Recency is a nudge, not a rule: the guard decides whether a group may be
// posted to, this only says which is the better use of today's allowance.
func TestAGroupPostedToAnHourAgoRanksLower(t *testing.T) {
	recent := candidate("posted recently", withRoute(regionRoute()), func(c *domain.GroupCandidate) {
		c.Group.LastPostedAt = ptr(now.Add(-30 * time.Minute))
	})
	rested := candidate("rested", withRoute(regionRoute()), func(c *domain.GroupCandidate) {
		c.Group.LastPostedAt = ptr(now.Add(-7 * 24 * time.Hour))
	})

	result := matching.Rank(advert(), []domain.GroupCandidate{recent, rested}, now)

	if got := titlesOf(result.Ranked); len(got) != 2 || got[0] != "rested" {
		t.Errorf("ranked %v, want the rested group first", got)
	}
	var penalised bool
	for _, reason := range result.Ranked[1].Reasons {
		if reason == matching.ReasonJustPosted {
			penalised = true
		}
	}
	if !penalised {
		t.Error("the recent post is not given as a reason, so the preview cannot explain the order")
	}
}

// The same advert previewed twice must list the same groups in the same order,
// or nobody can tell whether anything changed.
func TestTiesBreakOnTheTitleSoTheOrderIsStable(t *testing.T) {
	candidates := []domain.GroupCandidate{
		candidate("Zamin yuklari", withRoute(regionRoute())),
		candidate("Avto yuk", withRoute(regionRoute())),
		candidate("Market", withRoute(regionRoute())),
	}

	first := titlesOf(matching.Rank(advert(), candidates, now).Ranked)
	second := titlesOf(matching.Rank(advert(), candidates, now).Ranked)

	want := []string{"Avto yuk", "Market", "Zamin yuklari"}
	for i := range want {
		if first[i] != want[i] || second[i] != want[i] {
			t.Fatalf("order = %v then %v, want %v", first, second, want)
		}
	}
}

// Every score carries the reasons behind it, because the person about to press
// publish is owed an explanation rather than a number.
func TestScoresCarryTheirReasons(t *testing.T) {
	result := matching.Rank(advert(), []domain.GroupCandidate{
		candidate("group", withRoute(regionRoute()), func(c *domain.GroupCandidate) {
			c.Rules.VehicleTypes = domain.StringArray{string(domain.VehicleTent)}
			c.Group.MemberCount = ptr(4200)
		}),
	}, now)

	if len(result.Ranked) != 1 {
		t.Fatalf("ranked %v", titlesOf(result.Ranked))
	}
	got := map[matching.Reason]bool{}
	for _, reason := range result.Ranked[0].Reasons {
		got[reason] = true
	}
	for _, want := range []matching.Reason{matching.ReasonRegionRoute, matching.ReasonVehicle, matching.ReasonBusyGroup} {
		if !got[want] {
			t.Errorf("reason %q is missing from %v", want, result.Ranked[0].Reasons)
		}
	}
}
