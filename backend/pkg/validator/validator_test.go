package validator

import (
	"testing"

	"github.com/loadify/backend/pkg/apperror"
)

type sample struct {
	Phone    string `json:"phone" validate:"required,uzphone"`
	Password string `json:"password" validate:"required,password"`
	Locale   string `json:"locale" validate:"omitempty,locale"`
	Plate    string `json:"plate" validate:"omitempty,plate"`
}

func TestValidatorAcceptsValidInput(t *testing.T) {
	v := New()
	err := v.Struct(&sample{
		Phone:    "+998901234567",
		Password: "Str0ngPass",
		Locale:   "uz",
		Plate:    "01A123BC",
	})
	if err != nil {
		t.Fatalf("Struct() rejected valid input: %v", err)
	}
}

func TestUzPhoneRule(t *testing.T) {
	v := New()
	invalid := []string{
		"998901234567",   // missing +
		"+99890123456",   // too short
		"+9989012345678", // too long
		"+7 700 1234567", // wrong country
		"+998abcdefghi",  // not digits
	}
	for _, phone := range invalid {
		err := v.Struct(&sample{Phone: phone, Password: "Str0ngPass"})
		if err == nil {
			t.Errorf("phone %q was accepted", phone)
		}
	}
}

func TestPasswordRule(t *testing.T) {
	v := New()
	cases := map[string]bool{
		"Str0ngPass":  true,  // letters + digits, long enough
		"short1":      false, // under 8
		"alllettersx": false, // no digit
		"1234567890":  false, // no letter
	}
	for password, wantValid := range cases {
		err := v.Struct(&sample{Phone: "+998901234567", Password: password})
		if wantValid && err != nil {
			t.Errorf("password %q rejected: %v", password, err)
		}
		if !wantValid && err == nil {
			t.Errorf("password %q accepted", password)
		}
	}
}

func TestErrorsCarryJSONFieldNames(t *testing.T) {
	v := New()
	err := v.Struct(&sample{Phone: "bad", Password: "short"})
	if err == nil {
		t.Fatal("Struct() returned no error for invalid input")
	}

	appErr := apperror.As(err)
	if appErr.Code != apperror.ErrValidation.Code {
		t.Fatalf("code = %s, want %s", appErr.Code, apperror.ErrValidation.Code)
	}
	if len(appErr.Details) != 2 {
		t.Fatalf("details = %d, want 2: %+v", len(appErr.Details), appErr.Details)
	}

	fields := map[string]string{}
	for _, d := range appErr.Details {
		fields[d.Field] = d.Message
	}
	for _, want := range []string{"phone", "password"} {
		msg, ok := fields[want]
		if !ok {
			t.Errorf("no detail for field %q, got %+v", want, fields)
			continue
		}
		if msg == "" {
			t.Errorf("field %q has an empty message", want)
		}
	}
}

func TestOmitemptySkipsBlankOptionalFields(t *testing.T) {
	v := New()
	// Locale and Plate are optional; empty values must not fail.
	if err := v.Struct(&sample{Phone: "+998901234567", Password: "Str0ngPass"}); err != nil {
		t.Fatalf("blank optional fields rejected: %v", err)
	}
	if err := v.Struct(&sample{Phone: "+998901234567", Password: "Str0ngPass", Locale: "de"}); err == nil {
		t.Error("unsupported locale accepted")
	}
}
