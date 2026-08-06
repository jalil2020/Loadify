package geo

import (
	"math"
	"testing"
)

func TestPointEWKBRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		p    Point
	}{
		{"tashkent", Point{Lat: 41.2995, Lng: 69.2401}},
		{"samarkand", Point{Lat: 39.6270, Lng: 66.9750}},
		{"southern hemisphere", Point{Lat: -33.8688, Lng: 151.2093}},
		{"western hemisphere", Point{Lat: 40.7128, Lng: -74.0060}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := tc.p.Value()
			if err != nil {
				t.Fatalf("Value() error: %v", err)
			}
			hexStr, ok := encoded.(string)
			if !ok {
				t.Fatalf("Value() returned %T, want string", encoded)
			}
			// 1 byte order + 4 type + 4 srid + 16 coords = 25 bytes = 50 hex chars.
			if len(hexStr) != 50 {
				t.Fatalf("encoded length = %d, want 50 (%s)", len(hexStr), hexStr)
			}

			var decoded Point
			if err := decoded.Scan(hexStr); err != nil {
				t.Fatalf("Scan() error: %v", err)
			}
			if math.Abs(decoded.Lat-tc.p.Lat) > 1e-9 {
				t.Errorf("lat = %v, want %v", decoded.Lat, tc.p.Lat)
			}
			if math.Abs(decoded.Lng-tc.p.Lng) > 1e-9 {
				t.Errorf("lng = %v, want %v", decoded.Lng, tc.p.Lng)
			}
		})
	}
}

func TestPointScanNil(t *testing.T) {
	p := Point{Lat: 1, Lng: 2}
	if err := p.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}
	if !p.IsZero() {
		t.Errorf("Scan(nil) left %v, want zero point", p)
	}
}

func TestPointScanRejectsGarbage(t *testing.T) {
	var p Point
	if err := p.Scan("zzzz"); err == nil {
		t.Error("Scan(invalid hex) returned nil error")
	}
	if err := p.Scan(42); err == nil {
		t.Error("Scan(int) returned nil error")
	}
	if err := p.Scan("0101"); err == nil {
		t.Error("Scan(truncated ewkb) returned nil error")
	}
}

func TestPointValid(t *testing.T) {
	cases := []struct {
		name string
		p    Point
		want bool
	}{
		{"tashkent", Point{41.2995, 69.2401}, true},
		{"null island", Point{0, 0}, false},
		{"lat too high", Point{91, 10}, false},
		{"lat too low", Point{-91, 10}, false},
		{"lng too high", Point{10, 181}, false},
		{"lng too low", Point{10, -181}, false},
		{"nan", Point{math.NaN(), 10}, false},
		{"boundary", Point{90, 180}, true},
	}
	for _, tc := range cases {
		if got := tc.p.Valid(); got != tc.want {
			t.Errorf("%s: Valid() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDistanceKm(t *testing.T) {
	tashkent := Point{Lat: 41.2995, Lng: 69.2401}
	samarkand := Point{Lat: 39.6270, Lng: 66.9750}

	got := DistanceKm(tashkent, samarkand)
	// Great-circle distance Tashkent–Samarkand is roughly 270 km.
	if got < 260 || got > 285 {
		t.Errorf("DistanceKm = %.1f km, want roughly 270 km", got)
	}

	if d := DistanceKm(tashkent, tashkent); d > 1e-9 {
		t.Errorf("DistanceKm(p, p) = %v, want 0", d)
	}
}

func TestBoundingBox(t *testing.T) {
	center := Point{Lat: 41.2995, Lng: 69.2401}
	minLat, minLng, maxLat, maxLng := BoundingBox(center, 100)

	if minLat >= center.Lat || maxLat <= center.Lat {
		t.Errorf("latitude box [%v, %v] does not contain %v", minLat, maxLat, center.Lat)
	}
	if minLng >= center.Lng || maxLng <= center.Lng {
		t.Errorf("longitude box [%v, %v] does not contain %v", minLng, maxLng, center.Lng)
	}

	// Every corner must be at least the requested radius away, so the box is a
	// valid superset of the circle and never drops a candidate row.
	corners := []Point{{minLat, minLng}, {minLat, maxLng}, {maxLat, minLng}, {maxLat, maxLng}}
	for _, corner := range corners {
		if d := DistanceKm(center, corner); d < 100 {
			t.Errorf("corner %v is %.1f km away, closer than the 100 km radius", corner, d)
		}
	}
}
