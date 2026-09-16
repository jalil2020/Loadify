// Package matching decides which groups an advert belongs in, and in what
// order.
//
// The decision is deterministic and explainable on purpose. SQL narrows the
// company's groups to the ones whose stated rules fit the advert; this package
// ranks what came back and says why each one scored what it did. A model, where
// one is configured at all, may only reorder this list or drop from it — never
// add to it — so the set of groups an advert can reach is always one a person
// could have arrived at themselves.
package matching

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
)

// Points a candidate can earn. They are small integers rather than weights
// because the ranking has to be explainable to the person about to press
// publish: "both ends of the route match, and the group asks for tents".
const (
	pointsRegionRoute  = 40
	pointsCountryRoute = 25
	pointsHalfRoute    = 10
	pointsVehicle      = 15
	pointsCargo        = 10
	pointsWeightBand   = 5
	pointsBusyGroup    = 5
	penaltyJustPosted  = 15
)

// Reason is one line of why a group scored what it did, shown beside it in the
// publish preview.
type Reason string

// The reasons a candidate can carry.
const (
	ReasonRegionRoute  Reason = "route_region"
	ReasonCountryRoute Reason = "route_country"
	ReasonHalfRoute    Reason = "route_one_end"
	ReasonVehicle      Reason = "vehicle_listed"
	ReasonCargo        Reason = "cargo_listed"
	ReasonWeightBand   Reason = "weight_in_band"
	ReasonBusyGroup    Reason = "large_group"
	ReasonJustPosted   Reason = "posted_recently"
)

// Ranked is a candidate with its score and the reasons behind it.
type Ranked struct {
	Candidate domain.GroupCandidate
	Score     int
	Reasons   []Reason
}

// Excluded is a candidate that will not be offered at all, and why.
type Excluded struct {
	Candidate domain.GroupCandidate
	Reason    domain.SkipReason
}

// Result separates what is worth offering from what is not.
type Result struct {
	Ranked   []Ranked
	Excluded []Excluded
}

// Rank scores every candidate against the advert.
//
// Exclusions are checked here as well as in SQL, because the two run at
// different times: a group's rules can change between the query and the moment
// someone presses publish, and quiet hours depend on the clock rather than on
// anything a query could have filtered.
func Rank(ad *domain.LoadAd, candidates []domain.GroupCandidate, now time.Time) Result {
	var result Result

	for _, candidate := range candidates {
		if reason, excluded := exclude(ad, candidate, now); excluded {
			result.Excluded = append(result.Excluded, Excluded{Candidate: candidate, Reason: reason})
			continue
		}
		score, reasons := score(ad, candidate, now)
		result.Ranked = append(result.Ranked, Ranked{Candidate: candidate, Score: score, Reasons: reasons})
	}

	// Ties break on the group's title so the order is stable: the same advert
	// previewed twice must list the same groups in the same order, or nobody
	// can tell whether anything changed.
	sort.SliceStable(result.Ranked, func(i, j int) bool {
		if result.Ranked[i].Score != result.Ranked[j].Score {
			return result.Ranked[i].Score > result.Ranked[j].Score
		}
		return strings.ToLower(result.Ranked[i].Candidate.Group.Title) <
			strings.ToLower(result.Ranked[j].Candidate.Group.Title)
	})
	return result
}

// exclude answers the questions that have nothing to do with ranking: may this
// group be posted to at all, and does it accept this kind of freight.
func exclude(ad *domain.LoadAd, candidate domain.GroupCandidate, now time.Time) (domain.SkipReason, bool) {
	group, rules := candidate.Group, candidate.Rules

	if !group.Postable() {
		return domain.SkipGroupUnavailable, true
	}
	if !rules.AcceptsVehicle(ad.VehicleType) {
		return domain.SkipGroupUnavailable, true
	}
	if !rules.AcceptsWeight(ad.WeightKg) {
		return domain.SkipGroupUnavailable, true
	}
	if rules.InQuietHours(now) {
		return domain.SkipQuietHours, true
	}
	// A corridor nothing matches is not a near miss: posting a Tashkent–Moscow
	// load into a group that only ever carries Fergana–Andijan is exactly the
	// noise this product exists to stop.
	if len(candidate.Routes) > 0 && routeScore(ad, candidate.Routes) == 0 {
		return domain.SkipGroupUnavailable, true
	}
	return "", false
}

func score(ad *domain.LoadAd, candidate domain.GroupCandidate, now time.Time) (int, []Reason) {
	var (
		total   int
		reasons []Reason
	)
	add := func(points int, reason Reason) {
		total += points
		reasons = append(reasons, reason)
	}

	switch points := routeScore(ad, candidate.Routes); points {
	case pointsRegionRoute:
		add(points, ReasonRegionRoute)
	case pointsCountryRoute:
		add(points, ReasonCountryRoute)
	case pointsHalfRoute:
		add(points, ReasonHalfRoute)
	}

	// A group that names body types and names this one is a better fit than one
	// that names none, even though both accept the advert.
	if ad.VehicleType != nil && len(candidate.Rules.VehicleTypes) > 0 &&
		candidate.Rules.AcceptsVehicle(ad.VehicleType) {
		add(pointsVehicle, ReasonVehicle)
	}
	if ad.CargoTypeID != nil && len(candidate.Rules.CargoTypeCodes) > 0 {
		add(pointsCargo, ReasonCargo)
	}
	if ad.WeightKg != nil && (candidate.Rules.MinWeightKg != nil || candidate.Rules.MaxWeightKg != nil) {
		add(pointsWeightBand, ReasonWeightBand)
	}
	if candidate.Group.MemberCount != nil && *candidate.Group.MemberCount >= 1000 {
		add(pointsBusyGroup, ReasonBusyGroup)
	}

	// Recency is a nudge, not a rule: the guard decides whether a group may be
	// posted to. This only says that a group posted to an hour ago is a worse
	// use of today's allowance than one that has not seen us for a week.
	if lastPosted := candidate.Group.LastPostedAt; lastPosted != nil {
		cooldown := candidate.Rules.Cooldown(candidate.Group.SlowmodeSeconds, 0)
		if now.Sub(*lastPosted) < 2*cooldown {
			total -= penaltyJustPosted
			reasons = append(reasons, ReasonJustPosted)
		}
	}

	return total, reasons
}

// routeScore is how well the advert's corridor matches any of the group's.
//
// Both ends must match for a route to count at all. One end matching is a
// weaker signal worth keeping — a group for everything leaving Tashkent is a
// reasonable destination for a Tashkent load — but it never outranks a group
// that serves the whole corridor.
func routeScore(ad *domain.LoadAd, routes []domain.TelegramGroupRoute) int {
	best := 0
	for _, route := range routes {
		origin := endScore(route.OriginCountry, route.OriginRegionID, ad.OriginCountry, ad.OriginRegionID)
		destination := endScore(route.DestCountry, route.DestRegionID, ad.DestCountry, ad.DestRegionID)

		points := 0
		switch {
		case origin == 0 && destination == 0:
			continue
		case origin == 0 || destination == 0:
			points = pointsHalfRoute
		case origin == 2 && destination == 2:
			points = pointsRegionRoute
		default:
			points = pointsCountryRoute
		}
		if points > best {
			best = points
		}
	}
	return best
}

// endScore is 2 for a region match, 1 for a country match, 0 for neither.
func endScore(routeCountry *string, routeRegion *uuid.UUID, adCountry *string, adRegion *uuid.UUID) int {
	if routeRegion != nil {
		if adRegion != nil && *routeRegion == *adRegion {
			return 2
		}
		return 0
	}
	if routeCountry != nil && adCountry != nil && strings.EqualFold(*routeCountry, *adCountry) {
		return 1
	}
	return 0
}
