package secret

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const code = "51724"

// Every printing path has to redact, because the leak always happens on the
// path nobody thought about.
func TestValueRedactsOnEveryPrintingPath(t *testing.T) {
	value := Value(code)

	for _, verb := range []string{"%v", "%s", "%q", "%#v", "%+v"} {
		if out := fmt.Sprintf(verb, value); strings.Contains(out, code) {
			t.Errorf("%s printed the secret: %s", verb, out)
		}
	}
	if out := fmt.Errorf("login failed for %v", value).Error(); strings.Contains(out, code) {
		t.Errorf("wrapping in an error printed the secret: %s", out)
	}
}

func TestValueRedactsInsideAStruct(t *testing.T) {
	payload := struct {
		Phone string `json:"phone"`
		Code  Value  `json:"code"`
	}{Phone: "+998901234567", Code: Value(code)}

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), code) {
		t.Errorf("JSON contains the secret: %s", encoded)
	}
	if !strings.Contains(string(encoded), mask) {
		t.Errorf("JSON is missing the mask: %s", encoded)
	}
}

func TestRevealReturnsTheValue(t *testing.T) {
	if got := Value(code).Reveal(); got != code {
		t.Errorf("Reveal() = %q, want %q", got, code)
	}
	if !Value("").IsZero() {
		t.Error("an empty secret is not reported as zero")
	}
}

func TestMaskPhone(t *testing.T) {
	cases := map[string]string{
		"+998901234567":    "+99890 *** ** 67",
		"+998 90 123 4567": "+99890 *** ** 67",
		"+7 495 123 45 67": "+74951 *** ** 67",
		"12345":            mask,
		"":                 mask,
	}

	for in, want := range cases {
		if got := MaskPhone(in); got != want {
			t.Errorf("MaskPhone(%q) = %q, want %q", in, got, want)
		}
	}
}
