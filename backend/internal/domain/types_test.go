package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestStringArrayRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   StringArray
	}{
		{"empty", StringArray{}},
		{"simple", StringArray{"uz", "ru", "en"}},
		{"with spaces", StringArray{"B", "C", "C1 E"}},
		{"with comma", StringArray{"a,b", "c"}},
		{"with quote", StringArray{`say "hi"`}},
		{"with backslash", StringArray{`a\b`}},
		{"literal NULL", StringArray{"NULL"}},
		{"braces", StringArray{"{nested}"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := tc.in.Value()
			if err != nil {
				t.Fatalf("Value() error: %v", err)
			}
			var decoded StringArray
			if err := decoded.Scan(encoded); err != nil {
				t.Fatalf("Scan() error: %v", err)
			}
			if len(decoded) != len(tc.in) {
				t.Fatalf("length = %d, want %d (%v)", len(decoded), len(tc.in), decoded)
			}
			for i := range tc.in {
				if decoded[i] != tc.in[i] {
					t.Errorf("element %d = %q, want %q", i, decoded[i], tc.in[i])
				}
			}
		})
	}
}

func TestStringArrayScanPostgresLiterals(t *testing.T) {
	cases := map[string][]string{
		`{}`:                {},
		`{a,b,c}`:           {"a", "b", "c"},
		`{"a b","c"}`:       {"a b", "c"},
		`{a,NULL,c}`:        {"a", "", "c"},
		`{"NULL"}`:          {"NULL"},
		`{"with,comma",x}`:  {"with,comma", "x"},
		`{"quote\"inside"}`: {`quote"inside`},
	}
	for literal, want := range cases {
		var got StringArray
		if err := got.Scan(literal); err != nil {
			t.Errorf("Scan(%s) error: %v", literal, err)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("Scan(%s) = %v, want %v", literal, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("Scan(%s)[%d] = %q, want %q", literal, i, got[i], want[i])
			}
		}
	}
}

func TestStringArrayScanNilAndBytes(t *testing.T) {
	var a StringArray
	if err := a.Scan(nil); err != nil || a != nil {
		t.Errorf("Scan(nil) = %v, %v; want nil, nil", a, err)
	}
	if err := a.Scan([]byte(`{x,y}`)); err != nil {
		t.Fatalf("Scan([]byte) error: %v", err)
	}
	if len(a) != 2 || a[0] != "x" || a[1] != "y" {
		t.Errorf("Scan([]byte) = %v", a)
	}
	if err := a.Scan(42); err == nil {
		t.Error("Scan(int) returned no error")
	}
	if err := a.Scan("not an array"); err == nil {
		t.Error("Scan(garbage) returned no error")
	}
}

func TestStringArrayContains(t *testing.T) {
	a := StringArray{"uz", "ru"}
	if !a.Contains("ru") || a.Contains("en") {
		t.Errorf("Contains behaved unexpectedly on %v", a)
	}
}

func TestUUIDArrayRoundTrip(t *testing.T) {
	ids := UUIDArray{uuid.New(), uuid.New(), uuid.New()}

	encoded, err := ids.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	var decoded UUIDArray
	if err := decoded.Scan(encoded); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}
	if len(decoded) != len(ids) {
		t.Fatalf("length = %d, want %d", len(decoded), len(ids))
	}
	for i := range ids {
		if decoded[i] != ids[i] {
			t.Errorf("element %d = %v, want %v", i, decoded[i], ids[i])
		}
	}
	if !decoded.Contains(ids[1]) || decoded.Contains(uuid.New()) {
		t.Error("Contains behaved unexpectedly")
	}
}

func TestUUIDArrayScanEmptyAndInvalid(t *testing.T) {
	var a UUIDArray
	if err := a.Scan("{}"); err != nil {
		t.Fatalf("Scan({}) error: %v", err)
	}
	if len(a) != 0 {
		t.Errorf("Scan({}) = %v, want empty", a)
	}
	if err := a.Scan("{not-a-uuid}"); err == nil {
		t.Error("Scan with a non-uuid element returned no error")
	}
}

func TestJSONMapRoundTrip(t *testing.T) {
	m := JSONMap{"active_loads": float64(50), "driver_map": true, "tier": "business"}

	encoded, err := m.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	var decoded JSONMap
	if err := decoded.Scan(encoded); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}
	if len(decoded) != len(m) {
		t.Fatalf("length = %d, want %d", len(decoded), len(m))
	}
	if decoded["tier"] != "business" || decoded["driver_map"] != true {
		t.Errorf("decoded = %v", decoded)
	}

	var empty JSONMap
	if err := empty.Scan(nil); err != nil || empty != nil {
		t.Errorf("Scan(nil) = %v, %v", empty, err)
	}
	if err := empty.Scan("{not json"); err == nil {
		t.Error("Scan(malformed json) returned no error")
	}
}

func TestLocalisedName(t *testing.T) {
	uz, ru, en := "Toshkent", "Ташкент", "Tashkent"
	if got := LocalisedName(uz, ru, en, LocaleRU); got != ru {
		t.Errorf("ru = %q, want %q", got, ru)
	}
	if got := LocalisedName(uz, ru, en, LocaleEN); got != en {
		t.Errorf("en = %q, want %q", got, en)
	}
	if got := LocalisedName(uz, ru, en, LocaleUZ); got != uz {
		t.Errorf("uz = %q, want %q", got, uz)
	}
	// An unknown locale falls back to the platform default rather than blank.
	if got := LocalisedName(uz, ru, en, Locale("de")); got != uz {
		t.Errorf("unknown locale = %q, want %q", got, uz)
	}
}
