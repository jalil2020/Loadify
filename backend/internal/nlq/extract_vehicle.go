package nlq

import (
	"sort"
	"strings"

	"github.com/loadify/backend/internal/domain"
)

// vehicleAlias is one spelling of one vehicle type, pre-folded.
type vehicleAlias struct {
	code   domain.VehicleType
	folded string
	words  int
}

// VehicleExtractor resolves body types.
//
// Drivers name a body in shop language rather than in the platform's codes —
// "ref" and "izotermik" and "фура" — so the vocabulary the API returns is
// widened here with the informal names. Everything still resolves to one of the
// twelve codes; nothing new is invented.
type VehicleExtractor struct {
	aliases []vehicleAlias
}

// colloquialAliases are the words people actually use, mapped onto the codes.
//
// Kept beside the server's own localised labels rather than inside them: these
// are search synonyms, not names, and showing "furgon" as the label for a van
// on a load card would be wrong.
var colloquialAliases = map[domain.VehicleType][]string{
	domain.VehicleTent:         {"tent", "tentli", "тент", "тентованный", "tilt", "fura", "фура"},
	domain.VehicleRefrigerator: {"ref", "реф", "refrijerator", "рефрижератор", "muzlatgich", "холодильник", "reefer"},
	domain.VehicleIsothermal:   {"izoterm", "izotermik", "изотерм", "изотермический"},
	domain.VehicleFlatbed:      {"ochiq", "bortli", "открытый", "бортовой", "platforma", "площадка"},
	domain.VehicleContainer:    {"konteyner", "контейнер", "konteynervoz"},
	domain.VehicleTanker:       {"sisterna", "цистерна", "bochka", "tanker"},
	domain.VehicleCarCarrier:   {"avtovoz", "автовоз"},
	domain.VehicleGrain:        {"zerno", "зерновоз", "donvoz", "gallavoz"},
	domain.VehicleDump:         {"samosval", "самосвал", "agdargich"},
	domain.VehicleVan:          {"furgon", "фургон", "van"},
	domain.VehicleLowboy:       {"tral", "трал", "past ramali"},
}

// NewVehicleExtractor folds the server's labels together with the colloquial
// names and orders them longest-first.
func NewVehicleExtractor(vehicles []domain.Term) *VehicleExtractor {
	seen := make(map[string]bool)
	var aliases []vehicleAlias

	add := func(code domain.VehicleType, alias string) {
		folded := Fold(alias)
		key := string(code) + "\x00" + folded
		if folded == "" || seen[key] {
			return
		}
		seen[key] = true
		aliases = append(aliases, vehicleAlias{
			code:   code,
			folded: folded,
			words:  len(strings.Fields(folded)),
		})
	}

	for _, term := range vehicles {
		code := domain.VehicleType(term.ID)
		add(code, term.ID)
		for _, alias := range term.Aliases {
			add(code, alias)
		}
	}
	for code, extra := range colloquialAliases {
		for _, alias := range extra {
			add(code, alias)
		}
	}

	sort.SliceStable(aliases, func(i, j int) bool {
		if aliases[i].words != aliases[j].words {
			return aliases[i].words > aliases[j].words
		}
		return len(aliases[i].folded) > len(aliases[j].folded)
	})

	return &VehicleExtractor{aliases: aliases}
}

func (e *VehicleExtractor) Name() string { return "vehicle" }

func (e *VehicleExtractor) Extract(scan *Scan, out *domain.SearchIntent) {
	seen := make(map[domain.VehicleType]bool)

	for start := 0; start < len(scan.Tokens); start++ {
		if scan.IsClaimed(start) {
			continue
		}
		for _, alias := range e.aliases {
			end := start + alias.words
			if !scan.Free(start, end) {
				continue
			}
			if foldedPhrase(scan, start, end) != alias.folded {
				continue
			}
			scan.Claim(start, end, e.Name())
			if !seen[alias.code] {
				seen[alias.code] = true
				out.VehicleTypes = append(out.VehicleTypes, alias.code)
			}
			break
		}
	}
}
