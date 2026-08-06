package hash

import (
	"strings"
	"testing"
)

func TestBcryptHashAndVerify(t *testing.T) {
	// Cost 10 keeps the test fast while staying inside the accepted range.
	h := NewBcrypt(10)

	hashed, err := h.Hash("Str0ngPassword")
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if strings.Contains(hashed, "Str0ngPassword") {
		t.Error("the hash contains the plaintext password")
	}
	if !h.Verify("Str0ngPassword", hashed) {
		t.Error("Verify() rejected the correct password")
	}
	if h.Verify("wrong-password", hashed) {
		t.Error("Verify() accepted a wrong password")
	}
	if h.Verify("Str0ngPassword", "not-a-bcrypt-hash") {
		t.Error("Verify() accepted a malformed hash")
	}
}

func TestBcryptSaltsEachHash(t *testing.T) {
	h := NewBcrypt(10)
	first, err := h.Hash("same-password")
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	second, err := h.Hash("same-password")
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if first == second {
		t.Error("hashing the same password twice produced identical output")
	}
}

func TestBcryptRejectsOverlongPassword(t *testing.T) {
	h := NewBcrypt(10)
	// bcrypt silently truncates past 72 bytes, which would make two different
	// long passwords interchangeable. We reject instead.
	if _, err := h.Hash(strings.Repeat("a", 73)); err == nil {
		t.Error("Hash() accepted a password longer than 72 bytes")
	}
}

func TestSHA256HexIsStable(t *testing.T) {
	got := SHA256Hex("loadify")
	if len(got) != 64 {
		t.Fatalf("length = %d, want 64", len(got))
	}
	if got != SHA256Hex("loadify") {
		t.Error("SHA256Hex is not deterministic")
	}
	if got == SHA256Hex("loadify ") {
		t.Error("SHA256Hex collided on different inputs")
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !ConstantTimeEqual("abc", "abc") {
		t.Error("ConstantTimeEqual rejected equal strings")
	}
	if ConstantTimeEqual("abc", "abd") || ConstantTimeEqual("abc", "abcd") {
		t.Error("ConstantTimeEqual accepted different strings")
	}
}

func TestRandomToken(t *testing.T) {
	a, err := RandomToken(32)
	if err != nil {
		t.Fatalf("RandomToken() error: %v", err)
	}
	b, err := RandomToken(32)
	if err != nil {
		t.Fatalf("RandomToken() error: %v", err)
	}
	if a == b {
		t.Error("RandomToken() repeated itself")
	}
	if len(a) < 40 {
		t.Errorf("token %q is shorter than 32 bytes of entropy implies", a)
	}
	// base64url output must be safe to place in a URL unencoded.
	if strings.ContainsAny(a, "+/=") {
		t.Errorf("token %q is not URL-safe", a)
	}
}

func TestNumericCode(t *testing.T) {
	code, err := NumericCode(6)
	if err != nil {
		t.Fatalf("NumericCode() error: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("length = %d, want 6", len(code))
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			t.Fatalf("code %q contains a non-digit", code)
		}
	}
	if _, err := NumericCode(0); err == nil {
		t.Error("NumericCode(0) returned no error")
	}

	// Over many draws every digit position must vary; a constant one would mean
	// the generator is broken in a way a single-sample test would miss.
	seen := map[string]bool{}
	for range 50 {
		c, err := NumericCode(6)
		if err != nil {
			t.Fatalf("NumericCode() error: %v", err)
		}
		seen[c] = true
	}
	if len(seen) < 45 {
		t.Errorf("only %d distinct codes in 50 draws, generator looks biased", len(seen))
	}
}
