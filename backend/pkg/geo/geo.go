// Package geo carries coordinates between Go and PostGIS.
//
// Point implements driver.Valuer / sql.Scanner using hex EWKB, which is exactly
// what PostgreSQL emits for a geography column and accepts as input — so a
// geography(Point,4326) column maps to this type with no SQL functions in the
// hot path and no separate lat/lng columns to keep in sync.
package geo

import (
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
)

// SRID4326 is WGS 84, the SRID used across the platform.
const SRID4326 uint32 = 4326

// EarthRadiusKm is the mean Earth radius used by the haversine helper.
const EarthRadiusKm = 6371.0088

// Point is a WGS 84 coordinate.
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// NewPoint builds a Point.
func NewPoint(lat, lng float64) Point { return Point{Lat: lat, Lng: lng} }

// Valid reports whether the coordinate is inside the legal WGS 84 range and is
// not the null island placeholder that broken GPS fixes produce.
func (p Point) Valid() bool {
	if math.IsNaN(p.Lat) || math.IsNaN(p.Lng) {
		return false
	}
	if p.Lat < -90 || p.Lat > 90 || p.Lng < -180 || p.Lng > 180 {
		return false
	}
	return p.Lat != 0 || p.Lng != 0
}

// IsZero reports whether the point is unset.
func (p Point) IsZero() bool { return p.Lat == 0 && p.Lng == 0 }

// Value encodes the point as hex EWKB for PostGIS.
func (p Point) Value() (driver.Value, error) {
	buf := make([]byte, 25)
	buf[0] = 1 // little endian
	// Geometry type 1 (point) with the SRID flag set.
	binary.LittleEndian.PutUint32(buf[1:5], 1|0x20000000)
	binary.LittleEndian.PutUint32(buf[5:9], SRID4326)
	binary.LittleEndian.PutUint64(buf[9:17], math.Float64bits(p.Lng))  // X
	binary.LittleEndian.PutUint64(buf[17:25], math.Float64bits(p.Lat)) // Y
	return hex.EncodeToString(buf), nil
}

// Scan decodes hex EWKB (or raw EWKB bytes) coming back from PostGIS.
func (p *Point) Scan(src any) error {
	if src == nil {
		*p = Point{}
		return nil
	}

	var raw []byte
	switch v := src.(type) {
	case string:
		decoded, err := hex.DecodeString(v)
		if err != nil {
			return fmt.Errorf("geo: decode hex ewkb: %w", err)
		}
		raw = decoded
	case []byte:
		// Drivers may hand back either the hex text or the binary itself.
		if decoded, err := hex.DecodeString(string(v)); err == nil {
			raw = decoded
		} else {
			raw = v
		}
	default:
		return fmt.Errorf("geo: unsupported scan type %T", src)
	}

	if len(raw) < 21 {
		return fmt.Errorf("geo: ewkb too short (%d bytes)", len(raw))
	}

	var order binary.ByteOrder
	switch raw[0] {
	case 0:
		order = binary.BigEndian
	case 1:
		order = binary.LittleEndian
	default:
		return fmt.Errorf("geo: unknown byte order flag %d", raw[0])
	}

	geomType := order.Uint32(raw[1:5])
	hasSRID := geomType&0x20000000 != 0
	if geomType&0xFF != 1 {
		return fmt.Errorf("geo: expected a point, got geometry type %d", geomType&0xFF)
	}

	offset := 5
	if hasSRID {
		offset += 4
	}
	if len(raw) < offset+16 {
		return fmt.Errorf("geo: ewkb truncated")
	}

	p.Lng = math.Float64frombits(order.Uint64(raw[offset : offset+8]))
	p.Lat = math.Float64frombits(order.Uint64(raw[offset+8 : offset+16]))
	return nil
}

// GormDataType tells GORM the column type for auto-migration and casting.
func (Point) GormDataType() string { return "geography(Point,4326)" }

// DistanceKm returns the great-circle distance between two points.
// Server-side filtering uses ST_DWithin; this is for response payloads and the
// in-memory scoring used by the matching service.
func DistanceKm(a, b Point) float64 {
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * EarthRadiusKm * math.Asin(math.Sqrt(h))
}

// BoundingBox returns the min/max latitude and longitude of a circle of the
// given radius around the point. Useful as a cheap pre-filter.
func BoundingBox(center Point, radiusKm float64) (minLat, minLng, maxLat, maxLng float64) {
	latDelta := radiusKm / 111.32
	cos := math.Cos(center.Lat * math.Pi / 180)
	if math.Abs(cos) < 1e-9 {
		cos = 1e-9
	}
	lngDelta := radiusKm / (111.32 * cos)
	return center.Lat - latDelta, center.Lng - lngDelta,
		center.Lat + latDelta, center.Lng + lngDelta
}
