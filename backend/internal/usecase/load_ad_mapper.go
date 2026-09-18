package usecase

import (
	"time"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
)

// dateLayout is the calendar-day format the API speaks. A loading day is a day,
// not a moment, so it never carries a time or an offset over the wire.
const dateLayout = "2006-01-02"

// adFromRequest builds the entity a person confirmed.
//
// Provenance is not taken from the client: whatever the parser suggested, the
// values that arrive here were shown to someone who pressed save, so every
// field present is recorded as theirs.
func adFromRequest(req dto.SaveLoadAdRequest) domain.LoadAd {
	ad := domain.LoadAd{
		RawText:         req.RawText,
		OriginCityID:    req.OriginCityID,
		OriginRegionID:  req.OriginRegionID,
		OriginCountry:   req.OriginCountry,
		OriginText:      req.OriginText,
		DestCityID:      req.DestCityID,
		DestRegionID:    req.DestRegionID,
		DestCountry:     req.DestCountry,
		DestText:        req.DestText,
		CargoTypeID:     req.CargoTypeID,
		CargoText:       req.CargoText,
		WeightKg:        req.WeightKg,
		VolumeM3:        req.VolumeM3,
		VehicleType:     req.VehicleType,
		LoadingDate:     parseCalendarDay(req.LoadingDate),
		LoadingDateText: req.LoadingDateText,
		PriceAmount:     req.PriceAmount,
		PriceCurrency:   req.PriceCurrency,
		PriceNegotiable: req.PriceNegotiable,
		PaymentTerms:    req.PaymentTerms,
		PaymentMethod:   req.PaymentMethod,
		ContactPhone:    req.ContactPhone,
		ContactName:     req.ContactName,
		Notes:           req.Notes,
	}
	ad.FieldSources = confirmedSources(&ad)
	ad.Fingerprint = ad.ComputeFingerprint()
	return ad
}

// confirmedSources marks every field that has a value as confirmed by a person.
func confirmedSources(ad *domain.LoadAd) domain.FieldSources {
	sources := domain.FieldSources{}
	mark := func(field string, present bool) {
		if present {
			sources[field] = domain.FieldProvenance{Source: domain.FieldFromUser}
		}
	}

	mark(domain.FieldOrigin, ad.OriginCityID != nil || ad.OriginRegionID != nil || ad.OriginText != "")
	mark(domain.FieldDestination, ad.DestCityID != nil || ad.DestRegionID != nil || ad.DestText != "")
	mark(domain.FieldCargoType, ad.CargoTypeID != nil || ad.CargoText != nil)
	mark(domain.FieldWeight, ad.WeightKg != nil)
	mark(domain.FieldVolume, ad.VolumeM3 != nil)
	mark(domain.FieldVehicleType, ad.VehicleType != nil)
	mark(domain.FieldLoadingDate, ad.LoadingDate != nil)
	mark(domain.FieldPrice, ad.PriceAmount != nil)
	mark(domain.FieldPaymentTerms, ad.PaymentTerms != nil)
	mark(domain.FieldContactPhone, ad.ContactPhone != nil)
	return sources
}

// requestFromAd is the draft in the shape the create endpoint accepts, so a
// client can edit what the parser showed it and post it straight back.
func requestFromAd(ad domain.LoadAd) dto.SaveLoadAdRequest {
	return dto.SaveLoadAdRequest{
		RawText:         ad.RawText,
		OriginCityID:    ad.OriginCityID,
		OriginRegionID:  ad.OriginRegionID,
		OriginCountry:   ad.OriginCountry,
		OriginText:      ad.OriginText,
		DestCityID:      ad.DestCityID,
		DestRegionID:    ad.DestRegionID,
		DestCountry:     ad.DestCountry,
		DestText:        ad.DestText,
		CargoTypeID:     ad.CargoTypeID,
		CargoText:       ad.CargoText,
		WeightKg:        ad.WeightKg,
		VolumeM3:        ad.VolumeM3,
		VehicleType:     ad.VehicleType,
		LoadingDate:     formatCalendarDay(ad.LoadingDate),
		LoadingDateText: ad.LoadingDateText,
		PriceAmount:     ad.PriceAmount,
		PriceCurrency:   ad.PriceCurrency,
		PriceNegotiable: ad.PriceNegotiable,
		PaymentTerms:    ad.PaymentTerms,
		PaymentMethod:   ad.PaymentMethod,
		ContactPhone:    ad.ContactPhone,
		ContactName:     ad.ContactName,
		Notes:           ad.Notes,
	}
}

// adResponse renders a stored advertisement.
func adResponse(ad *domain.LoadAd) dto.LoadAdResponse {
	// Nothing missing is sent as [], which is what the contract promises and
	// what a client reading its length expects.
	missing := ad.MissingForPublish()
	if missing == nil {
		missing = []string{}
	}
	return dto.LoadAdResponse{
		ID:        ad.ID,
		CompanyID: ad.CompanyID,
		Status:    string(ad.Status),
		RawText:   ad.RawText,
		Origin: dto.PlaceRefResponse{
			CityID:   ad.OriginCityID,
			RegionID: ad.OriginRegionID,
			Country:  ad.OriginCountry,
			Text:     ad.OriginText,
		},
		Destination: dto.PlaceRefResponse{
			CityID:   ad.DestCityID,
			RegionID: ad.DestRegionID,
			Country:  ad.DestCountry,
			Text:     ad.DestText,
		},
		CargoTypeID:     ad.CargoTypeID,
		CargoText:       ad.CargoText,
		WeightKg:        ad.WeightKg,
		VolumeM3:        ad.VolumeM3,
		VehicleType:     ad.VehicleType,
		LoadingDate:     formatCalendarDay(ad.LoadingDate),
		LoadingDateText: ad.LoadingDateText,
		PriceAmount:     ad.PriceAmount,
		PriceCurrency:   ad.PriceCurrency,
		PriceNegotiable: ad.PriceNegotiable,
		PaymentTerms:    ad.PaymentTerms,
		PaymentMethod:   ad.PaymentMethod,
		ContactPhone:    ad.ContactPhone,
		ContactName:     ad.ContactName,
		Notes:           ad.Notes,
		Missing:         missing,
		Fingerprint:     ad.Fingerprint,
		PublishedAt:     ad.PublishedAt,
		CreatedAt:       ad.CreatedAt,
		UpdatedAt:       ad.UpdatedAt,
	}
}

// parseResponse renders what the parser understood, field by field, each with
// the words it was read from.
func parseResponse(draft domain.AdDraft) dto.ParseAdResponse {
	fields := map[string]dto.ParsedField{}
	values := map[string]any{
		domain.FieldOrigin: dto.PlaceRefResponse{
			CityID: draft.Ad.OriginCityID, RegionID: draft.Ad.OriginRegionID,
			Country: draft.Ad.OriginCountry, Text: draft.Ad.OriginText,
		},
		domain.FieldDestination: dto.PlaceRefResponse{
			CityID: draft.Ad.DestCityID, RegionID: draft.Ad.DestRegionID,
			Country: draft.Ad.DestCountry, Text: draft.Ad.DestText,
		},
		domain.FieldCargoType:    draft.Ad.CargoTypeID,
		domain.FieldWeight:       draft.Ad.WeightKg,
		domain.FieldVolume:       draft.Ad.VolumeM3,
		domain.FieldVehicleType:  draft.Ad.VehicleType,
		domain.FieldLoadingDate:  formatCalendarDay(draft.Ad.LoadingDate),
		domain.FieldPrice:        priceValue(draft.Ad),
		domain.FieldPaymentTerms: draft.Ad.PaymentTerms,
		domain.FieldContactPhone: draft.Ad.ContactPhone,
	}

	for field, provenance := range draft.Sources {
		fields[field] = dto.ParsedField{
			Value:    values[field],
			Source:   string(provenance.Source),
			Evidence: provenance.Evidence,
		}
	}
	// A field nobody resolved is reported as an explicit null rather than left
	// out, so the client can render every slot the form has.
	for field := range values {
		if _, found := fields[field]; !found {
			fields[field] = dto.ParsedField{}
		}
	}

	aiStatus := draft.AIStatus
	if aiStatus == "" {
		aiStatus = domain.AIStatusOff
	}
	// Empty lists are sent as [], which is what the contract promises and what
	// a client iterating them expects.
	missing, dropped := draft.Missing, draft.Dropped
	if missing == nil {
		missing = []string{}
	}
	if dropped == nil {
		dropped = []string{}
	}
	return dto.ParseAdResponse{
		Fields:   fields,
		Missing:  missing,
		Dropped:  dropped,
		AIUsed:   draft.AIUsed,
		AIStatus: aiStatus,
		Draft:    requestFromAd(draft.Ad),
	}
}

// priceMoney is how an amount and its currency travel together.
type priceMoney struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	Negotiable  bool   `json:"negotiable"`
}

func priceValue(ad domain.LoadAd) any {
	if ad.PriceAmount == nil || ad.PriceCurrency == nil {
		return nil
	}
	return priceMoney{AmountMinor: *ad.PriceAmount, Currency: *ad.PriceCurrency, Negotiable: ad.PriceNegotiable}
}

func parseCalendarDay(value *string) *time.Time {
	if value == nil || *value == "" {
		return nil
	}
	day, err := time.Parse(dateLayout, *value)
	if err != nil {
		// The validator already rejected anything malformed; a value that gets
		// here and still fails is not worth failing the whole save over.
		return nil
	}
	return &day
}

func formatCalendarDay(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(dateLayout)
	return &formatted
}
