package usecase

import (
	"strings"
	"testing"

	"github.com/loadify/backend/internal/domain"
)

func TestNormalisePhone(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"+998901234567", "+998901234567"},
		{"998901234567", "+998901234567"},
		{"+998 90 123 45 67", "+998901234567"},
		{"+998 (90) 123-45-67", "+998901234567"},
		{"  +998-90-123-45-67  ", "+998901234567"},
		{"+7 700 123 45 67", "+77001234567"},
	}
	for _, tc := range cases {
		if got := normalisePhone(tc.in); got != tc.want {
			t.Errorf("normalisePhone(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalisePhoneIsIdempotent(t *testing.T) {
	once := normalisePhone("+998 (90) 123-45-67")
	if twice := normalisePhone(once); twice != once {
		t.Errorf("normalisePhone is not idempotent: %q then %q", once, twice)
	}
}

func TestMaskPhoneHidesSubscriberDigits(t *testing.T) {
	masked := maskPhone("+998901234567")
	if strings.Contains(masked, "234567") {
		t.Errorf("maskPhone leaked digits: %q", masked)
	}
	if !strings.HasPrefix(masked, "+99890") {
		t.Errorf("maskPhone = %q, want the operator prefix preserved", masked)
	}
	if got := maskPhone("+998"); got != "***" {
		t.Errorf("maskPhone(short) = %q, want ***", got)
	}
}

func TestOTPMessageIsLocalisedAndCarriesTheCode(t *testing.T) {
	for _, locale := range []domain.Locale{domain.LocaleUZ, domain.LocaleRU, domain.LocaleEN} {
		msg := otpMessage("123456", locale, 5)
		if !strings.Contains(msg, "123456") {
			t.Errorf("%s message does not contain the code: %q", locale, msg)
		}
		if !strings.Contains(msg, "Loadify") {
			t.Errorf("%s message does not identify the sender: %q", locale, msg)
		}
	}

	if otpMessage("1", domain.LocaleRU, 5) == otpMessage("1", domain.LocaleEN, 5) {
		t.Error("Russian and English OTP messages are identical")
	}
	// An unknown locale must fall back to the platform default rather than blank.
	if fallback := otpMessage("1", domain.Locale("de"), 5); fallback != otpMessage("1", domain.LocaleUZ, 5) {
		t.Errorf("unknown locale did not fall back to Uzbek: %q", fallback)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "android", "ios"); got != "android" {
		t.Errorf("firstNonEmpty = %q, want android", got)
	}
	if got := firstNonEmpty("", "   "); got != "" {
		t.Errorf("firstNonEmpty(blanks) = %q, want empty", got)
	}
}

func TestNilIfBlank(t *testing.T) {
	if nilIfBlank("   ") != nil {
		t.Error("nilIfBlank(whitespace) should be nil")
	}
	got := nilIfBlank("value")
	if got == nil || *got != "value" {
		t.Errorf("nilIfBlank(value) = %v, want pointer to \"value\"", got)
	}
}

func TestDeref(t *testing.T) {
	if deref(nil) != "" {
		t.Error("deref(nil) should be empty")
	}
	s := "x"
	if deref(&s) != "x" {
		t.Error("deref(&s) should return s")
	}
}
