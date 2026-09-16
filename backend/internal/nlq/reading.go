package nlq

import (
	"strings"

	"github.com/loadify/backend/internal/domain"
)

// The Read helpers run one rule over a fragment of an advert.
//
// They exist for the model-backed pass, and they are what keeps a model from
// ever producing a number. A model is asked only which words are the tonnage,
// the rate or the phone number; the value itself is then computed here, by the
// same rule that would have read it if the rule had found the words on its own.
// So "20 тонн" becomes 20000 kilograms because our rule says a tonne is a
// thousand kilograms, not because a model said so.

// ReadWeight returns the weight in kilograms named by a fragment.
func ReadWeight(fragment string) (int, bool) {
	var draft domain.AdDraft
	NewAdWeightExtractor().Extract(NewScan(fragment), &draft)
	if draft.Ad.WeightKg == nil {
		return 0, false
	}
	return *draft.Ad.WeightKg, true
}

// ReadVolume returns the cubic metres named by a fragment.
func ReadVolume(fragment string) (float64, bool) {
	var draft domain.AdDraft
	NewAdVolumeExtractor().Extract(NewScan(fragment), &draft)
	if draft.Ad.VolumeM3 == nil {
		return 0, false
	}
	return *draft.Ad.VolumeM3, true
}

// ReadPrice returns the amount in minor units and its currency.
//
// The whole advert is passed as well as the fragment: a currency usually
// arrives as punctuation the tokeniser drops, so the marks are read off the
// original text.
func ReadPrice(fragment, whole string) (int64, string, bool) {
	var draft domain.AdDraft
	NewAdPriceExtractor(whole).Extract(NewScan(fragment), &draft)
	if draft.Ad.PriceAmount == nil || draft.Ad.PriceCurrency == nil {
		return 0, "", false
	}
	return *draft.Ad.PriceAmount, *draft.Ad.PriceCurrency, true
}

// ReadPhone returns the phone number named by a fragment.
func ReadPhone(fragment string) (string, bool) {
	var draft domain.AdDraft
	NewContactExtractor().Extract(NewScan(fragment), &draft)
	if draft.Ad.ContactPhone == nil {
		return "", false
	}
	return *draft.Ad.ContactPhone, true
}

// ReadPaymentTerms returns the payment terms named by a fragment.
func ReadPaymentTerms(fragment string) (string, bool) {
	var draft domain.AdDraft
	NewPaymentExtractor().Extract(NewScan(fragment), &draft)
	if draft.Ad.PaymentTerms == nil {
		return "", false
	}
	return *draft.Ad.PaymentTerms, true
}

// Grounded reports whether a fragment really appears in the advert.
//
// The comparison is on folded text, so a model that returns "Toshkentdan" for
// an advert written "тошкентдан" still counts as pointing at real words, while
// one that returns a tonnage nobody wrote does not.
func Grounded(whole, fragment string) bool {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return false
	}
	return strings.Contains(Fold(whole), Fold(fragment))
}
