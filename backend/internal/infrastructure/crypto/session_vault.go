package crypto

import (
	"fmt"

	"github.com/loadify/backend/internal/domain"
)

// sessionVault adapts the envelope vault to the domain port, which speaks in
// the two columns a row actually stores: the ciphertext and its key version.
type sessionVault struct{ vault Vault }

// NewSessionVault builds the domain-facing vault over the configured keys.
func NewSessionVault(keks map[int][]byte, current int) (domain.SessionVault, error) {
	vault, err := NewVault(keks, current)
	if err != nil {
		return nil, err
	}
	return sessionVault{vault: vault}, nil
}

func (s sessionVault) Seal(plaintext []byte, aad string) ([]byte, int, error) {
	sealed, err := s.vault.Seal(plaintext, aad)
	if err != nil {
		return nil, 0, fmt.Errorf("seal session: %w", err)
	}
	return sealed.Ciphertext, sealed.KeyVersion, nil
}

func (s sessionVault) Open(ciphertext []byte, keyVersion int, aad string) ([]byte, error) {
	plaintext, err := s.vault.Open(Sealed{KeyVersion: keyVersion, Ciphertext: ciphertext}, aad)
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	return plaintext, nil
}
