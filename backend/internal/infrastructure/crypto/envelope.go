// Package crypto seals the secrets that have to live in the database but must
// stay unreadable in a dump: today that means the MTProto session of a
// logistician's own Telegram account, which grants full access to that account.
//
// Every value gets its own data key. The data key is wrapped with a
// key-encryption key that lives in the environment or a KMS and never in the
// database, so the table alone yields nothing. Both layers are bound to the row
// through additional authenticated data, so a ciphertext lifted out of one row
// cannot be pasted into another.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// Failures a caller may want to distinguish. Everything a tamperer could cause
// collapses into ErrNotAuthentic: which of the two layers rejected the value is
// not something the caller needs, and not something worth leaking.
var (
	ErrUnknownKeyVersion = errors.New("crypto: unknown key version")
	ErrMalformed         = errors.New("crypto: malformed ciphertext")
	ErrNotAuthentic      = errors.New("crypto: ciphertext failed authentication")
)

// Sealed is one encrypted value and the key version that wrapped it. The
// version travels with the row so a rotated key can still open old rows.
type Sealed struct {
	KeyVersion int
	Ciphertext []byte
}

// Vault seals and opens secrets.
type Vault interface {
	Seal(plaintext []byte, aad string) (Sealed, error)
	Open(sealed Sealed, aad string) ([]byte, error)
	CurrentVersion() int
}

const (
	keySize     = 32
	nonceSize   = 12
	gcmTagSize  = 16
	wrappedSize = keySize + gcmTagSize
	headerSize  = nonceSize + wrappedSize + nonceSize
)

type envelopeVault struct {
	keks    map[int]cipher.AEAD
	current int
}

// NewVault builds a vault over the key-encryption keys, keyed by version.
// current names the version new values are sealed with; every other version
// stays available for opening what it sealed before.
func NewVault(keks map[int][]byte, current int) (Vault, error) {
	if len(keks) == 0 {
		return nil, errors.New("crypto: no key-encryption key configured")
	}
	if _, ok := keks[current]; !ok {
		return nil, fmt.Errorf("crypto: current key version %d is not among the configured keys", current)
	}
	aeads := make(map[int]cipher.AEAD, len(keks))
	for version, key := range keks {
		aead, err := newAEAD(key)
		if err != nil {
			return nil, fmt.Errorf("crypto: key version %d: %w", version, err)
		}
		aeads[version] = aead
	}
	return &envelopeVault{keks: aeads, current: current}, nil
}

func (v *envelopeVault) CurrentVersion() int { return v.current }

// Seal encrypts plaintext under a fresh data key. aad binds the result to the
// row it belongs to, so pass something stable and unique such as
// "telegram_account:<id>:<company_id>".
func (v *envelopeVault) Seal(plaintext []byte, aad string) (Sealed, error) {
	dataKey := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, dataKey); err != nil {
		return Sealed{}, fmt.Errorf("crypto: read random: %w", err)
	}
	defer zero(dataKey)

	payloadAEAD, err := newAEAD(dataKey)
	if err != nil {
		return Sealed{}, fmt.Errorf("crypto: data key: %w", err)
	}
	kekNonce, err := randomNonce()
	if err != nil {
		return Sealed{}, err
	}
	dataNonce, err := randomNonce()
	if err != nil {
		return Sealed{}, err
	}

	wrapped := v.keks[v.current].Seal(nil, kekNonce, dataKey, []byte(aad))
	payload := payloadAEAD.Seal(nil, dataNonce, plaintext, []byte(aad))

	out := make([]byte, 0, headerSize+len(payload))
	out = append(out, kekNonce...)
	out = append(out, wrapped...)
	out = append(out, dataNonce...)
	out = append(out, payload...)
	return Sealed{KeyVersion: v.current, Ciphertext: out}, nil
}

// Open reverses Seal. A wrong aad, a changed byte or a key version this vault
// does not hold all fail, and the plaintext is never returned partially.
func (v *envelopeVault) Open(sealed Sealed, aad string) ([]byte, error) {
	kek, ok := v.keks[sealed.KeyVersion]
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownKeyVersion, sealed.KeyVersion)
	}
	if len(sealed.Ciphertext) < headerSize {
		return nil, ErrMalformed
	}
	kekNonce := sealed.Ciphertext[:nonceSize]
	wrapped := sealed.Ciphertext[nonceSize : nonceSize+wrappedSize]
	dataNonce := sealed.Ciphertext[nonceSize+wrappedSize : headerSize]
	payload := sealed.Ciphertext[headerSize:]

	dataKey, err := kek.Open(nil, kekNonce, wrapped, []byte(aad))
	if err != nil {
		return nil, ErrNotAuthentic
	}
	defer zero(dataKey)

	payloadAEAD, err := newAEAD(dataKey)
	if err != nil {
		return nil, ErrMalformed
	}
	plaintext, err := payloadAEAD.Open(nil, dataNonce, payload, []byte(aad))
	if err != nil {
		return nil, ErrNotAuthentic
	}
	return plaintext, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d", keySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func randomNonce() ([]byte, error) {
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: read random: %w", err)
	}
	return nonce, nil
}

// zero wipes a key as soon as it is no longer needed. It is not a guarantee —
// the runtime may have copied the slice — but it shortens the window in which a
// core dump would contain it.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
