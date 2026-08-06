package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/logger"
)

// referenceCacheTTL is generous because this data changes only on deploy.
const referenceCacheTTL = 6 * time.Hour

// ReferenceUsecase serves the static vocabularies every client needs: countries,
// regions, cities and truck types.
type ReferenceUsecase struct {
	geo   domain.GeoRepository
	cache cache.Cache
	log   logger.Logger
}

// NewReferenceUsecase wires the reference usecase.
func NewReferenceUsecase(geoRepo domain.GeoRepository, c cache.Cache, log logger.Logger) *ReferenceUsecase {
	return &ReferenceUsecase{geo: geoRepo, cache: c, log: log}
}

// Countries lists the markets the platform serves.
func (u *ReferenceUsecase) Countries(ctx context.Context, locale domain.Locale) ([]dto.CountryResponse, error) {
	key := "ref:countries:" + string(locale)
	var cached []dto.CountryResponse
	if err := u.cache.Get(ctx, key, &cached); err == nil {
		return cached, nil
	}

	countries, err := u.geo.ListCountries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.CountryResponse, 0, len(countries))
	for _, c := range countries {
		out = append(out, dto.CountryResponse{
			ID:           c.ID,
			Code:         c.Code,
			Name:         domain.LocalisedName(c.NameUZ, c.NameRU, c.NameEN, locale),
			PhoneCode:    c.PhoneCode,
			CurrencyCode: c.CurrencyCode,
			FlagEmoji:    c.FlagEmoji,
		})
	}
	u.store(ctx, key, out)
	return out, nil
}

// Regions lists the administrative divisions of a country.
func (u *ReferenceUsecase) Regions(ctx context.Context, countryCode string, locale domain.Locale) ([]dto.RegionResponse, error) {
	if countryCode == "" {
		countryCode = "UZ"
	}
	key := "ref:regions:" + countryCode + ":" + string(locale)
	var cached []dto.RegionResponse
	if err := u.cache.Get(ctx, key, &cached); err == nil {
		return cached, nil
	}

	regions, err := u.geo.ListRegions(ctx, countryCode)
	if err != nil {
		return nil, err
	}
	out := make([]dto.RegionResponse, 0, len(regions))
	for _, r := range regions {
		item := dto.RegionResponse{
			ID:        r.ID,
			CountryID: r.CountryID,
			Code:      r.Code,
			Name:      domain.LocalisedName(r.NameUZ, r.NameRU, r.NameEN, locale),
			NameUZ:    r.NameUZ,
			NameRU:    r.NameRU,
			NameEN:    r.NameEN,
		}
		if r.Center != nil && r.Center.Valid() {
			item.Center = &dto.PointResponse{Lat: r.Center.Lat, Lng: r.Center.Lng}
		}
		out = append(out, item)
	}
	u.store(ctx, key, out)
	return out, nil
}

// Cities lists settlements, optionally filtered by region or a search term.
// Search results are not cached: the key space would be unbounded.
func (u *ReferenceUsecase) Cities(ctx context.Context, regionID *uuid.UUID, search string, locale domain.Locale) ([]dto.CityResponse, error) {
	cacheable := search == ""
	key := "ref:cities:all:" + string(locale)
	if regionID != nil {
		key = "ref:cities:" + regionID.String() + ":" + string(locale)
	}

	if cacheable {
		var cached []dto.CityResponse
		if err := u.cache.Get(ctx, key, &cached); err == nil {
			return cached, nil
		}
	}

	cities, err := u.geo.ListCities(ctx, regionID, search, 200)
	if err != nil {
		return nil, err
	}
	out := make([]dto.CityResponse, 0, len(cities))
	for _, c := range cities {
		item := dto.CityResponse{
			ID:        c.ID,
			RegionID:  c.RegionID,
			Name:      domain.LocalisedName(c.NameUZ, c.NameRU, c.NameEN, locale),
			NameUZ:    c.NameUZ,
			NameRU:    c.NameRU,
			NameEN:    c.NameEN,
			IsCapital: c.IsCapital,
		}
		if c.Location != nil && c.Location.Valid() {
			item.Location = &dto.PointResponse{Lat: c.Location.Lat, Lng: c.Location.Lng}
		}
		out = append(out, item)
	}
	if cacheable {
		u.store(ctx, key, out)
	}
	return out, nil
}

// VehicleTypes returns the truck-type vocabulary shared by loads and vehicles.
func (u *ReferenceUsecase) VehicleTypes(locale domain.Locale) []dto.VehicleTypeResponse {
	out := make([]dto.VehicleTypeResponse, 0, len(domain.AllVehicleTypes))
	for _, t := range domain.AllVehicleTypes {
		out = append(out, dto.VehicleTypeResponse{
			Code: string(t),
			Name: vehicleTypeName(t, locale),
		})
	}
	return out
}

func (u *ReferenceUsecase) store(ctx context.Context, key string, value any) {
	if err := u.cache.Set(ctx, key, value, referenceCacheTTL); err != nil {
		u.log.Warn("reference cache write failed",
			logger.String("key", key), logger.Err(err))
	}
}

// vehicleTypeName localises a truck type. These labels live in code rather than
// the database because the enum itself lives in code — adding a type means a
// migration and a release anyway.
func vehicleTypeName(t domain.VehicleType, locale domain.Locale) string {
	names := map[domain.VehicleType][3]string{
		domain.VehicleTent:         {"Tent", "Тент", "Tilt / curtainsider"},
		domain.VehicleRefrigerator: {"Refrijerator", "Рефрижератор", "Refrigerated"},
		domain.VehicleIsothermal:   {"Izotermik", "Изотермический", "Isothermal"},
		domain.VehicleFlatbed:      {"Ochiq platforma", "Бортовой", "Flatbed"},
		domain.VehicleContainer:    {"Konteyner tashuvchi", "Контейнеровоз", "Container carrier"},
		domain.VehicleTanker:       {"Sisterna", "Цистерна", "Tanker"},
		domain.VehicleCarCarrier:   {"Avtovoz", "Автовоз", "Car carrier"},
		domain.VehicleGrain:        {"Don tashuvchi", "Зерновоз", "Grain hopper"},
		domain.VehicleDump:         {"Samosval", "Самосвал", "Tipper"},
		domain.VehicleVan:          {"Furgon", "Фургон", "Van"},
		domain.VehicleLowboy:       {"Tral", "Трал", "Lowboy"},
		domain.VehicleOther:        {"Boshqa", "Другое", "Other"},
	}
	entry, ok := names[t]
	if !ok {
		return string(t)
	}
	switch locale {
	case domain.LocaleRU:
		return entry[1]
	case domain.LocaleEN:
		return entry[2]
	default:
		return entry[0]
	}
}
