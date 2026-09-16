package repository

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
)

// placeRow is the flattened shape both halves of the union produce.
type placeRow struct {
	ID          uuid.UUID
	Kind        string
	CountryCode string
	RegionID    *uuid.UUID
	NameUZ      string
	NameRU      string
	NameEN      string
}

// listPlacesSQL reads every resolvable name in one round trip.
//
// The order is load-bearing rather than cosmetic. Cities come first because a
// city and its region share a short name — "Toshkent" means both — and whoever
// is added first owns that ambiguous form in the parser's index; freight
// originates in the city far more often. Capitals lead within a country for the
// same reason: someone writing "Moskva" means the city, not the oblast.
const listPlacesSQL = `
SELECT ci.id            AS id,
       'city'           AS kind,
       co.code          AS country_code,
       ci.region_id     AS region_id,
       ci.name_uz       AS name_uz,
       ci.name_ru       AS name_ru,
       ci.name_en       AS name_en,
       0                AS kind_order,
       CASE WHEN ci.is_capital THEN 0 ELSE 1 END AS rank_order
FROM cities ci
JOIN regions re ON re.id = ci.region_id
JOIN countries co ON co.id = re.country_id
WHERE ci.is_active AND re.is_active AND co.is_active

UNION ALL

SELECT re.id, 'region', co.code, NULL::uuid,
       re.name_uz, re.name_ru, re.name_en,
       1 AS kind_order,
       1 AS rank_order
FROM regions re
JOIN countries co ON co.id = re.country_id
WHERE re.is_active AND co.is_active

ORDER BY kind_order, rank_order, name_en`

// ListPlaces returns every city and region the parser may resolve a name to.
func (r *geoRepository) ListPlaces(ctx context.Context) ([]domain.Place, error) {
	var rows []placeRow
	if err := r.DB(ctx).Raw(listPlacesSQL).Scan(&rows).Error; err != nil {
		return nil, translate(err, "place")
	}

	places := make([]domain.Place, 0, len(rows))
	for _, row := range rows {
		kind := domain.PlaceRegion
		if row.Kind == string(domain.PlaceCity) {
			kind = domain.PlaceCity
		}
		places = append(places, domain.Place{
			ID:          row.ID,
			Kind:        kind,
			CountryCode: row.CountryCode,
			RegionID:    row.RegionID,
			// All three localisations are aliases of one place, so an advert
			// written in any of them resolves to the same id.
			Aliases: distinctNames(row.NameUZ, row.NameRU, row.NameEN),
		})
	}
	return places, nil
}

func distinctNames(names ...string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}
