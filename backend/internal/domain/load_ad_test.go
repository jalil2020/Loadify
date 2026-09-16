package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func ptr[T any](v T) *T { return &v }

func tashkentMoscow() *LoadAd {
	loading := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	return &LoadAd{
		OriginText:    "Ташкент",
		DestText:      "Москва",
		WeightKg:      ptr(20000),
		VehicleType:   ptr("tent"),
		LoadingDate:   &loading,
		PriceAmount:   ptr(int64(250000)),
		PriceCurrency: ptr("USD"),
		ContactPhone:  ptr("+998901234567"),
	}
}

// The duplicate check is only worth having if re-typing the same offer still
// collides. Spacing, case and punctuation are wording, not meaning.
func TestFingerprintIgnoresWording(t *testing.T) {
	original := tashkentMoscow()
	retyped := tashkentMoscow()
	retyped.OriginText = "  ташкент  "
	retyped.DestText = "МОСКВА"
	retyped.ContactPhone = ptr("+998 90 123 45 67")
	retyped.Notes = ptr("срочно!")
	retyped.RawText = "completely different text"

	if original.ComputeFingerprint() != retyped.ComputeFingerprint() {
		t.Error("the same offer typed differently produced a different fingerprint")
	}
}

func TestFingerprintSeparatesDifferentOffers(t *testing.T) {
	base := tashkentMoscow()

	cases := map[string]func(*LoadAd){
		"another tonnage":    func(a *LoadAd) { a.WeightKg = ptr(22000) },
		"another body":       func(a *LoadAd) { a.VehicleType = ptr("ref") },
		"another day":        func(a *LoadAd) { a.LoadingDate = ptr(time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)) },
		"another price":      func(a *LoadAd) { a.PriceAmount = ptr(int64(270000)) },
		"another currency":   func(a *LoadAd) { a.PriceCurrency = ptr("UZS") },
		"another phone":      func(a *LoadAd) { a.ContactPhone = ptr("+998901111111") },
		"another city":       func(a *LoadAd) { a.DestText = "Казань" },
		"resolved city":      func(a *LoadAd) { a.DestCityID = ptr(uuid.New()) },
		"tonnage now absent": func(a *LoadAd) { a.WeightKg = nil },
	}

	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			other := tashkentMoscow()
			change(other)
			if base.ComputeFingerprint() == other.ComputeFingerprint() {
				t.Error("a materially different offer produced the same fingerprint")
			}
		})
	}
}

// A resolved city must not depend on how it was typed, or the same load posted
// from the form and from a sentence would look like two adverts.
func TestFingerprintPrefersResolvedPlaces(t *testing.T) {
	cityID := uuid.New()

	fromForm := tashkentMoscow()
	fromForm.DestCityID = &cityID
	fromForm.DestText = "Москва"

	fromSentence := tashkentMoscow()
	fromSentence.DestCityID = &cityID
	fromSentence.DestText = "мск"

	if fromForm.ComputeFingerprint() != fromSentence.ComputeFingerprint() {
		t.Error("a resolved city still depends on the typed spelling")
	}
}

func TestMissingForPublish(t *testing.T) {
	empty := &LoadAd{}
	if got := empty.MissingForPublish(); len(got) != len(RequiredForPublish) {
		t.Errorf("MissingForPublish() = %v, want all of %v", got, RequiredForPublish)
	}
	if empty.CanPublish() {
		t.Error("an empty advertisement may not be published")
	}

	ready := tashkentMoscow()
	if got := ready.MissingForPublish(); len(got) != 0 {
		t.Errorf("MissingForPublish() = %v, want none", got)
	}
	if !ready.CanPublish() {
		t.Error("a complete draft may be published")
	}

	noPhone := tashkentMoscow()
	noPhone.ContactPhone = nil
	if got := noPhone.MissingForPublish(); len(got) != 1 || got[0] != "contact_phone" {
		t.Errorf("MissingForPublish() = %v, want [contact_phone]", got)
	}

	archived := tashkentMoscow()
	archived.Status = LoadAdArchived
	if archived.CanPublish() {
		t.Error("an archived advertisement may not be published")
	}
}
