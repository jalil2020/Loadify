package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// StringArray maps a PostgreSQL text[] column.
//
// The pure database/sql driver hands array columns back as the raw literal
// `{a,b,"c d"}`; these types parse and emit that format so entities can use
// ordinary Go slices without importing a driver-specific array wrapper.
type StringArray []string

// Value renders the slice as a PostgreSQL array literal.
func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return encodePGArray(a), nil
}

// Scan parses a PostgreSQL array literal.
func (a *StringArray) Scan(src any) error {
	if src == nil {
		*a = nil
		return nil
	}
	raw, err := toString(src)
	if err != nil {
		return fmt.Errorf("text[]: %w", err)
	}
	parsed, err := decodePGArray(raw)
	if err != nil {
		return fmt.Errorf("text[]: %w", err)
	}
	*a = parsed
	return nil
}

// GormDataType tells GORM the column type.
func (StringArray) GormDataType() string { return "text[]" }

// Contains reports whether the array holds the value.
func (a StringArray) Contains(v string) bool { return slices.Contains(a, v) }

// UUIDArray maps a PostgreSQL uuid[] column.
type UUIDArray []uuid.UUID

// Value renders the slice as a PostgreSQL array literal.
func (a UUIDArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	strs := make([]string, len(a))
	for i, id := range a {
		strs[i] = id.String()
	}
	return encodePGArray(strs), nil
}

// Scan parses a PostgreSQL array literal into UUIDs.
func (a *UUIDArray) Scan(src any) error {
	if src == nil {
		*a = nil
		return nil
	}
	raw, err := toString(src)
	if err != nil {
		return fmt.Errorf("uuid[]: %w", err)
	}
	parts, err := decodePGArray(raw)
	if err != nil {
		return fmt.Errorf("uuid[]: %w", err)
	}
	out := make(UUIDArray, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		id, err := uuid.Parse(p)
		if err != nil {
			return fmt.Errorf("uuid[]: %q is not a uuid: %w", p, err)
		}
		out = append(out, id)
	}
	*a = out
	return nil
}

// GormDataType tells GORM the column type.
func (UUIDArray) GormDataType() string { return "uuid[]" }

// Contains reports whether the array holds the id.
func (a UUIDArray) Contains(id uuid.UUID) bool { return slices.Contains(a, id) }

// JSONMap maps a PostgreSQL jsonb column to a Go map.
type JSONMap map[string]any

// Value serialises the map to JSON.
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("jsonb: encode: %w", err)
	}
	return string(raw), nil
}

// Scan deserialises JSON into the map.
func (m *JSONMap) Scan(src any) error {
	if src == nil {
		*m = nil
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("jsonb: unsupported scan type %T", src)
	}
	if len(raw) == 0 {
		*m = JSONMap{}
		return nil
	}
	parsed := JSONMap{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("jsonb: decode: %w", err)
	}
	*m = parsed
	return nil
}

// GormDataType tells GORM the column type.
func (JSONMap) GormDataType() string { return "jsonb" }

// JSONRaw holds a jsonb value whose shape is not fixed — a settings value may be
// a string, a number or an object, so it is kept as raw JSON and decoded by the
// typed accessors.
type JSONRaw []byte

// Value writes the raw JSON.
func (r JSONRaw) Value() (driver.Value, error) {
	if len(r) == 0 {
		return nil, nil
	}
	return string(r), nil
}

// Scan copies the raw JSON out of the driver's buffer.
func (r *JSONRaw) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*r = nil
	case []byte:
		// The driver may reuse its buffer, so the bytes must be copied.
		*r = append(JSONRaw(nil), v...)
	case string:
		*r = JSONRaw(v)
	default:
		return fmt.Errorf("jsonb: unsupported scan type %T", src)
	}
	return nil
}

// GormDataType tells GORM the column type.
func (JSONRaw) GormDataType() string { return "jsonb" }

// String returns the raw JSON text.
func (r JSONRaw) String() string { return string(r) }

// ─── array literal codec ────────────────────────────────────────────

func toString(src any) (string, error) {
	switch v := src.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return "", fmt.Errorf("unsupported scan type %T", src)
	}
}

// encodePGArray writes `{"a","b"}`. Every element is quoted, which removes any
// need to reason about which characters would otherwise need escaping.
func encodePGArray(values []string) string {
	if len(values) == 0 {
		return "{}"
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		for _, r := range v {
			if r == '"' || r == '\\' {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

// decodePGArray parses `{a,"b c",NULL}` into a slice, honouring quoting and
// backslash escapes.
func decodePGArray(literal string) ([]string, error) {
	s := strings.TrimSpace(literal)
	if s == "" || s == "{}" {
		return []string{}, nil
	}
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return nil, fmt.Errorf("malformed array literal %q", literal)
	}
	s = s[1 : len(s)-1]
	if s == "" {
		return []string{}, nil
	}

	var (
		out       []string
		current   strings.Builder
		inQuotes  bool
		wasQuoted bool
		escaped   bool
	)
	flush := func() {
		item := current.String()
		// An unquoted NULL is the SQL null element; a quoted "NULL" is the
		// four-letter string and must survive intact.
		if !wasQuoted && item == "NULL" {
			item = ""
		}
		out = append(out, item)
		current.Reset()
		wasQuoted = false
	}

	for _, r := range s {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
			wasQuoted = true
		case r == ',' && !inQuotes:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	if escaped || inQuotes {
		return nil, fmt.Errorf("unterminated element in array literal %q", literal)
	}
	flush()
	return out, nil
}
