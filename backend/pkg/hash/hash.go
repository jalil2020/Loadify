// Package hash centralises password hashing and the one-way hashing used for
// tokens and OTP codes stored at rest.
package hash

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"

	"golang.org/x/crypto/bcrypt"
)

// Hasher hashes and verifies passwords.
type Hasher interface {
	Hash(plain string) (string, error)
	Verify(plain, hashed string) bool
}

type bcryptHasher struct{ cost int }

// NewBcrypt returns a bcrypt-backed hasher. Cost is clamped to the range bcrypt accepts.
func NewBcrypt(cost int) Hasher {
	if cost < bcrypt.MinCost {
		cost = bcrypt.DefaultCost
	}
	if cost > bcrypt.MaxCost {
		cost = bcrypt.MaxCost
	}
	return &bcryptHasher{cost: cost}
}

func (b *bcryptHasher) Hash(plain string) (string, error) {
	// bcrypt silently truncates beyond 72 bytes; reject instead of hashing a prefix.
	if len(plain) > 72 {
		return "", fmt.Errorf("password exceeds 72 bytes")
	}
	out, err := bcrypt.GenerateFromPassword([]byte(plain), b.cost)
	if err != nil {
		return "", fmt.Errorf("bcrypt: %w", err)
	}
	return string(out), nil
}

func (b *bcryptHasher) Verify(plain, hashed string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
}

// SHA256Hex returns the lowercase hex SHA-256 of s. Used for refresh tokens and
// OTP codes, where the input already has full entropy and bcrypt would only add latency.
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ConstantTimeEqual compares two strings without leaking length-independent timing.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// RandomToken returns a URL-safe random string carrying nBytes of entropy.
func RandomToken(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// NumericCode returns a cryptographically random decimal code of the given length.
func NumericCode(digits int) (string, error) {
	if digits <= 0 {
		return "", fmt.Errorf("digits must be positive")
	}
	out := make([]byte, digits)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("read random: %w", err)
		}
		out[i] = byte('0' + n.Int64())
	}
	return string(out), nil
}
