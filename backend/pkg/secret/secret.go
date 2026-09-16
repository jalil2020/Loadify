// Package secret holds values that must never be printed: a Telegram login
// code, a two-factor password, an MTProto session, an API key.
//
// The type redacts itself on every path a value normally escapes through —
// fmt, JSON, and the structured logger, which encodes unknown types as JSON —
// so a forgotten call site can no longer leak one. Reading the real value takes
// an explicit Reveal, which a reviewer can grep for.
package secret

import "strings"

const mask = "***"

// Value is a secret string.
type Value string

// String, GoString, MarshalText and MarshalJSON all yield the mask, covering
// fmt verbs, encoding/json and zap's reflection-based encoder.
func (Value) String() string { return mask }

// GoString covers the %#v verb.
func (Value) GoString() string { return mask }

// MarshalText covers encoding.TextMarshaler users.
func (Value) MarshalText() ([]byte, error) { return []byte(mask), nil }

// MarshalJSON covers encoding/json and the structured logger.
func (Value) MarshalJSON() ([]byte, error) { return []byte(`"` + mask + `"`), nil }

// Reveal returns the value in the clear. Call it at the boundary that needs it
// — the MTProto client, the crypto vault — and never store the result.
func (v Value) Reveal() string { return string(v) }

// IsZero reports whether the secret is empty.
func (v Value) IsZero() bool { return v == "" }

// MaskPhone keeps a number recognisable to the person it belongs to without
// printing it in full: +998901234567 becomes "+99890 *** ** 67". Anything too
// short to mask meaningfully is replaced outright.
func MaskPhone(phone string) string {
	compact := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '(' || r == ')' {
			return -1
		}
		return r
	}, phone)

	if len([]rune(compact)) < 8 {
		return mask
	}
	runes := []rune(compact)
	return string(runes[:6]) + " *** ** " + string(runes[len(runes)-2:])
}
