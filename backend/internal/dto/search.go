package dto

import (
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
)

// SearchTarget is which side of the market a request is aimed at.
type SearchTarget string

const (
	// TargetLoads is a driver looking for freight.
	TargetLoads SearchTarget = "loads"
	// TargetDrivers is a shipper looking for capacity.
	TargetDrivers SearchTarget = "drivers"
)

// ParseSearchRequest is a written request plus which side is asking.
type ParseSearchRequest struct {
	Query  string       `json:"query" validate:"required,min=2,max=280"`
	Target SearchTarget `json:"target" validate:"omitempty,oneof=loads drivers"`
}

// ParsedChip is one understood filter, ready to render as a removable chip.
//
// Showing the parse before running it is the whole safety model here: the driver
// sees what was understood and can drop a wrong chip, rather than staring at a
// result set and wondering why their region is missing.
type ParsedChip struct {
	// Field is the query parameter this chip controls, so removing it in the UI
	// is a matter of dropping that key.
	Field string `json:"field"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// ParsedSearchResponse is the parse, in the shape the client needs to both show
// it and act on it.
type ParsedSearchResponse struct {
	Target SearchTarget `json:"target"`
	// Filters is ready to send verbatim to /loads or /drivers. Repeated keys
	// are expressed as a list, matching how those endpoints read them.
	Filters map[string][]string `json:"filters"`
	Chips   []ParsedChip        `json:"chips"`
	// Dropped holds words that were not understood, so the client can say so
	// rather than let them vanish.
	Dropped []string `json:"dropped"`
	// Empty reports that nothing was understood, which is the signal to leave
	// the existing filters alone rather than replace them with nothing.
	Empty bool `json:"empty"`
}

// NewParsedSearchResponse projects an intent onto one side of the market.
//
// The two projections are where the duality is spent: the same tonnage caps the
// freight worth showing a driver and floors the capacity worth showing a
// shipper, and the same pair of regions is a corridor to one and a catchment to
// the other.
func NewParsedSearchResponse(
	intent domain.SearchIntent,
	target SearchTarget,
	locale domain.Locale,
) ParsedSearchResponse {
	if target == "" {
		target = TargetLoads
	}

	builder := &filterBuilder{
		filters: make(map[string][]string),
		locale:  locale,
	}
	if target == TargetDrivers {
		builder.forDrivers(intent)
	} else {
		builder.forLoads(intent)
	}

	return ParsedSearchResponse{
		Target:  target,
		Filters: builder.filters,
		Chips:   builder.chips,
		Dropped: intent.Dropped,
		Empty:   intent.IsEmpty(),
	}
}

// filterBuilder accumulates the query parameters and their human labels
// together, so a filter can never be added without something to show for it.
type filterBuilder struct {
	filters map[string][]string
	chips   []ParsedChip
	locale  domain.Locale
}

func (b *filterBuilder) add(field, label, value string) {
	b.filters[field] = append(b.filters[field], value)
	b.chips = append(b.chips, ParsedChip{Field: field, Label: label, Value: value})
}

func (b *filterBuilder) addIDs(field, label string, ids []uuid.UUID) {
	for _, id := range ids {
		b.add(field, label, id.String())
	}
}

// forLoads projects onto GET /loads.
func (b *filterBuilder) forLoads(intent domain.SearchIntent) {
	b.addIDs("pickup_region_id", labelFrom(b.locale), intent.OriginRegionIDs)
	b.addIDs("delivery_region_id", labelTo(b.locale), intent.DestinationRegionIDs)

	for _, vehicle := range intent.VehicleTypes {
		b.add("vehicle_type", labelVehicle(b.locale), string(vehicle))
	}

	// A driver stating a tonnage is stating what they can carry, so it caps the
	// freight worth showing them. Anything heavier they simply cannot take.
	if intent.WeightKg != nil {
		b.add("max_weight_kg", labelWeight(b.locale), strconv.Itoa(*intent.WeightKg))
	}
	if intent.MinWeightKg != nil {
		b.add("min_weight_kg", labelWeight(b.locale), strconv.Itoa(*intent.MinWeightKg))
	}
	if intent.MaxWeightKg != nil {
		b.add("max_weight_kg", labelWeight(b.locale), strconv.Itoa(*intent.MaxWeightKg))
	}

	// A driver naming a rate is naming what they want to earn, so it floors.
	if intent.MinPrice != nil {
		b.add("min_price", labelPrice(b.locale), strconv.FormatInt(*intent.MinPrice, 10))
	}
	if intent.MaxPrice != nil {
		b.add("max_price", labelPrice(b.locale), strconv.FormatInt(*intent.MaxPrice, 10))
	}

	b.addFlags(intent)
	b.addWindow(intent, "pickup_from", "pickup_to")
}

// forDrivers projects onto GET /drivers.
func (b *filterBuilder) forDrivers(intent domain.SearchIntent) {
	// A shipper's corridor is the driver's catchment: both ends are places the
	// driver should be willing to work, and the driver endpoint takes one list.
	b.addIDs("region_id", labelFrom(b.locale), intent.OriginRegionIDs)
	b.addIDs("region_id", labelTo(b.locale), intent.DestinationRegionIDs)

	for _, vehicle := range intent.VehicleTypes {
		b.add("vehicle_type", labelVehicle(b.locale), string(vehicle))
	}

	// The mirror of the load projection: a shipper stating a tonnage is stating
	// the cargo they hold, so it floors the capacity worth showing them.
	if intent.WeightKg != nil {
		b.add("min_capacity_kg", labelWeight(b.locale), strconv.Itoa(*intent.WeightKg))
	}
	if intent.MinWeightKg != nil {
		b.add("min_capacity_kg", labelWeight(b.locale), strconv.Itoa(*intent.MinWeightKg))
	}

	b.addFlags(intent)

	// Drivers are filtered by when they become free, so only the opening edge
	// of the window applies.
	if intent.ReadyFrom != nil {
		b.add("available_from", labelWhen(b.locale), intent.ReadyFrom.Format(time.RFC3339))
	}
}

func (b *filterBuilder) addFlags(intent domain.SearchIntent) {
	if intent.RequiresADR != nil && *intent.RequiresADR {
		b.add("requires_adr", "ADR", "true")
	}
	if intent.RequiresRefrigeration != nil && *intent.RequiresRefrigeration {
		b.add("requires_refrigeration", labelRefrigerated(b.locale), "true")
	}
}

func (b *filterBuilder) addWindow(intent domain.SearchIntent, fromField, toField string) {
	if intent.ReadyFrom != nil {
		b.add(fromField, labelWhen(b.locale), intent.ReadyFrom.Format(time.RFC3339))
	}
	if intent.ReadyTo != nil {
		b.add(toField, labelWhen(b.locale), intent.ReadyTo.Format(time.RFC3339))
	}
}

// The chip labels. Short by design — a chip is read at a glance on a phone, and
// the value beside it already carries the detail.
func labelFrom(l domain.Locale) string {
	return domain.LocalisedName("Qayerdan", "Откуда", "From", l)
}

func labelTo(l domain.Locale) string {
	return domain.LocalisedName("Qayerga", "Куда", "To", l)
}

func labelVehicle(l domain.Locale) string {
	return domain.LocalisedName("Mashina", "Кузов", "Vehicle", l)
}

func labelWeight(l domain.Locale) string {
	return domain.LocalisedName("Ogʻirlik", "Вес", "Weight", l)
}

func labelPrice(l domain.Locale) string {
	return domain.LocalisedName("Narx", "Цена", "Price", l)
}

func labelWhen(l domain.Locale) string {
	return domain.LocalisedName("Qachon", "Когда", "When", l)
}

func labelRefrigerated(l domain.Locale) string {
	return domain.LocalisedName("Sovutgich", "Рефрижератор", "Refrigerated", l)
}
