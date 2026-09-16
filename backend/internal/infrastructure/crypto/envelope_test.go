package crypto

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"testing"
)

const accountAAD = "telegram_account:9f1c:2b70"

func randomKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatalf("random key: %v", err)
	}
	return key
}

func newTestVault(t *testing.T) Vault {
	t.Helper()
	vault, err := NewVault(map[int][]byte{1: randomKey(t)}, 1)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	return vault
}

func TestSealOpenRoundTrip(t *testing.T) {
	vault := newTestVault(t)
	session := []byte("mtproto auth key, 256 bytes in real life")

	sealed, err := vault.Seal(session, accountAAD)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if sealed.KeyVersion != 1 {
		t.Errorf("KeyVersion = %d, want 1", sealed.KeyVersion)
	}
	if bytes.Contains(sealed.Ciphertext, session) {
		t.Error("ciphertext contains the plaintext")
	}

	opened, err := vault.Open(sealed, accountAAD)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(opened, session) {
		t.Errorf("Open returned %q, want %q", opened, session)
	}
}

// A ciphertext moved to another row must not open. This is what stops someone
// with write access to the table from swapping sessions between accounts and
// posting as a different logistician.
func TestOpenRejectsAnotherRowsAAD(t *testing.T) {
	vault := newTestVault(t)
	sealed, err := vault.Seal([]byte("session"), accountAAD)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	if _, err := vault.Open(sealed, "telegram_account:0000:2b70"); !errors.Is(err, ErrNotAuthentic) {
		t.Errorf("Open with a foreign aad returned %v, want ErrNotAuthentic", err)
	}
}

func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	vault := newTestVault(t)
	sealed, err := vault.Seal([]byte("session"), accountAAD)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	for name, index := range map[string]int{
		"nonce":       0,
		"wrapped key": nonceSize + 2,
		"payload":     headerSize,
	} {
		t.Run(name, func(t *testing.T) {
			tampered := Sealed{KeyVersion: sealed.KeyVersion, Ciphertext: bytes.Clone(sealed.Ciphertext)}
			tampered.Ciphertext[index] ^= 0x01
			if _, err := vault.Open(tampered, accountAAD); !errors.Is(err, ErrNotAuthentic) {
				t.Errorf("Open returned %v, want ErrNotAuthentic", err)
			}
		})
	}
}

func TestOpenRejectsShortCiphertext(t *testing.T) {
	vault := newTestVault(t)

	if _, err := vault.Open(Sealed{KeyVersion: 1, Ciphertext: []byte("short")}, accountAAD); !errors.Is(err, ErrMalformed) {
		t.Errorf("Open returned %v, want ErrMalformed", err)
	}
}

func TestOpenRejectsUnknownKeyVersion(t *testing.T) {
	vault := newTestVault(t)
	sealed, err := vault.Seal([]byte("session"), accountAAD)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	sealed.KeyVersion = 7

	if _, err := vault.Open(sealed, accountAAD); !errors.Is(err, ErrUnknownKeyVersion) {
		t.Errorf("Open returned %v, want ErrUnknownKeyVersion", err)
	}
}

// Rotation is only useful if it does not strand what the old key sealed.
func TestRotatedKeyStillOpensOlderSecrets(t *testing.T) {
	oldKey, newKey := randomKey(t), randomKey(t)

	before, err := NewVault(map[int][]byte{1: oldKey}, 1)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	sealed, err := before.Seal([]byte("session"), accountAAD)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	after, err := NewVault(map[int][]byte{1: oldKey, 2: newKey}, 2)
	if err != nil {
		t.Fatalf("NewVault after rotation: %v", err)
	}
	opened, err := after.Open(sealed, accountAAD)
	if err != nil {
		t.Fatalf("Open a secret sealed by the previous key: %v", err)
	}
	if string(opened) != "session" {
		t.Errorf("Open returned %q", opened)
	}

	fresh, err := after.Seal([]byte("session"), accountAAD)
	if err != nil {
		t.Fatalf("Seal after rotation: %v", err)
	}
	if fresh.KeyVersion != 2 {
		t.Errorf("new secrets sealed with version %d, want 2", fresh.KeyVersion)
	}
}

func TestSealIsNotDeterministic(t *testing.T) {
	vault := newTestVault(t)

	first, err := vault.Seal([]byte("session"), accountAAD)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	second, err := vault.Seal([]byte("session"), accountAAD)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Equal(first.Ciphertext, second.Ciphertext) {
		t.Error("sealing the same plaintext twice produced the same ciphertext")
	}
}

func TestNewVaultRejectsUnusableKeys(t *testing.T) {
	cases := map[string]struct {
		keks    map[int][]byte
		current int
	}{
		"no keys":            {map[int][]byte{}, 1},
		"current is missing": {map[int][]byte{1: randomKey(t)}, 2},
		"key is too short":   {map[int][]byte{1: []byte("too short")}, 1},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewVault(tc.keks, tc.current); err == nil {
				t.Error("NewVault accepted an unusable configuration")
			}
		})
	}
}
