package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func baseConfig() *Config {
	return &Config{
		App:      AppConfig{Env: "development", Port: 8080},
		DB:       DBConfig{Name: "loadify", User: "loadify"},
		JWT:      JWTConfig{AccessSecret: strings.Repeat("a", 32), RefreshSecret: strings.Repeat("b", 32), AccessTTL: 15 * time.Minute},
		Security: SecurityConfig{BcryptCost: 12, OTPLength: 6},
		AI:       AIConfig{Provider: AIProviderNone},
	}
}

func workingTelegram() TelegramConfig {
	return TelegramConfig{
		APIID:           12345,
		APIHash:         "0123456789abcdef0123456789abcdef",
		SessionKEK:      base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))),
		KEKVersion:      1,
		AccountGap:      90 * time.Second,
		AccountHourly:   20,
		AccountDaily:    120,
		GroupCooldown:   time.Hour,
		GroupDaily:      3,
		DuplicateWindow: 24 * time.Hour,
		GlobalPerSecond: 10,
		MaxAttempts:     5,
		LeaseTTL:        2 * time.Minute,
	}
}

func TestValidateAllowsPostingToBeDisabled(t *testing.T) {
	if err := baseConfig().Validate(); err != nil {
		t.Errorf("a deployment without Telegram credentials must still boot: %v", err)
	}
}

// Half a client would fail at the first send instead of at start-up, which is
// the kind of failure a boot-time check exists to prevent.
func TestValidateRejectsPartialTelegramConfig(t *testing.T) {
	cfg := baseConfig()
	cfg.Telegram = TelegramConfig{APIID: 12345, KEKVersion: 1}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("a partial Telegram configuration was accepted")
	}
	if !strings.Contains(err.Error(), "must be set together") {
		t.Errorf("the error does not name the problem: %v", err)
	}
}

func TestValidateAcceptsCompleteTelegramConfig(t *testing.T) {
	cfg := baseConfig()
	cfg.Telegram = workingTelegram()

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if !cfg.Telegram.Enabled() {
		t.Error("Enabled() is false for a complete configuration")
	}
}

func TestValidateRejectsKEKOfTheWrongSize(t *testing.T) {
	cfg := baseConfig()
	cfg.Telegram = workingTelegram()
	cfg.Telegram.SessionKEK = base64.StdEncoding.EncodeToString([]byte("16 bytes exactly"))

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("Validate accepted a key-encryption key of the wrong size: %v", err)
	}
}

func TestKEKsCarriesBothVersionsThroughRotation(t *testing.T) {
	tg := workingTelegram()
	tg.KEKVersion = 2
	tg.PreviousKEK = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("p", 32)))
	tg.PreviousKEKVersion = 1

	keys, current, err := tg.KEKs()
	if err != nil {
		t.Fatalf("KEKs: %v", err)
	}
	if current != 2 {
		t.Errorf("current = %d, want 2", current)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(keys))
	}
	for version, key := range keys {
		if len(key) != 32 {
			t.Errorf("key %d is %d bytes, want 32", version, len(key))
		}
	}
}

func TestValidateRejectsUnknownAIProvider(t *testing.T) {
	cfg := baseConfig()
	cfg.AI.Provider = "gpt"

	if err := cfg.Validate(); err == nil {
		t.Error("Validate accepted an unknown AI provider")
	}
}
