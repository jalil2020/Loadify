package usecase

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/loadify/backend/internal/domain"
)

// telegramMessageLimit is Telegram's own ceiling for a text message. An advert
// is nowhere near it, but a person can paste a page of notes into one.
const telegramMessageLimit = 4096

// adLabels are the words each line is introduced with. They live in code rather
// than in the database because they are part of the message format, and a
// deployment that wants to change the format is changing more than a word.
var adLabels = map[string]map[string]string{
	"uz": {
		"loading": "Yuklash", "weight": "t", "volume": "m³", "price": "Narx",
		"negotiable": "kelishiladi", "payment": "To‘lov", "contact": "Aloqa",
		"on_loading": "yuklashda", "on_delivery": "yetkazilganda",
		"prepayment": "oldindan", "deferred": "kechiktirilgan",
		"cash": "naqd", "transfer": "o‘tkazma", "card": "karta", "escrow": "eskrou",
	},
	"ru": {
		"loading": "Загрузка", "weight": "т", "volume": "м³", "price": "Цена",
		"negotiable": "торг", "payment": "Оплата", "contact": "Контакт",
		"on_loading": "при загрузке", "on_delivery": "при выгрузке",
		"prepayment": "предоплата", "deferred": "отсрочка",
		"cash": "наличные", "transfer": "перечисление", "card": "карта", "escrow": "эскроу",
	},
	"en": {
		"loading": "Loading", "weight": "t", "volume": "m³", "price": "Price",
		"negotiable": "negotiable", "payment": "Payment", "contact": "Contact",
		"on_loading": "on loading", "on_delivery": "on delivery",
		"prepayment": "prepayment", "deferred": "deferred",
		"cash": "cash", "transfer": "bank transfer", "card": "card", "escrow": "escrow",
	},
}

// messageFor is what a group receives for the advert.
//
// An advert a person wrote is posted exactly as they wrote it: the platform
// decides where it goes, never what it says. Only the surrounding blank space is
// dropped, which Telegram would drop anyway. An advert built from fields alone,
// with no words of its own, has its message composed by renderAd.
func messageFor(ad *domain.LoadAd, language string) string {
	if ad.HasText() {
		return strings.TrimSpace(ad.RawText)
	}
	return renderAd(ad, language)
}

// renderAd writes the message for an advert that has no text of its own.
//
// The same advert produces the same text for every group in a language: varying
// it per group is how a duplicate check gets fooled, and fooling a duplicate
// check is the opposite of what this product is for. Only the language changes,
// because a Russian-speaking board and an Uzbek one are different audiences
// rather than different adverts.
//
// Lines a person did not fill in are left out entirely. A message reading
// "Volume: —" says nothing and costs a line in a feed people skim.
func renderAd(ad *domain.LoadAd, language string) string {
	labels, known := adLabels[language]
	if !known {
		labels = adLabels["ru"]
	}

	var lines []string

	if route := routeLine(ad); route != "" {
		lines = append(lines, route)
	}
	if spec := specLine(ad, labels); spec != "" {
		lines = append(lines, spec)
	}
	if ad.LoadingDate != nil {
		lines = append(lines, labels["loading"]+": "+ad.LoadingDate.Format("02.01.2006"))
	}
	if price := priceLine(ad, labels); price != "" {
		lines = append(lines, price)
	}
	if payment := paymentLine(ad, labels); payment != "" {
		lines = append(lines, payment)
	}
	if ad.Notes != nil && strings.TrimSpace(*ad.Notes) != "" {
		lines = append(lines, strings.TrimSpace(*ad.Notes))
	}
	if ad.ContactPhone != nil {
		contact := labels["contact"] + ": " + *ad.ContactPhone
		if ad.ContactName != nil && strings.TrimSpace(*ad.ContactName) != "" {
			contact += " (" + strings.TrimSpace(*ad.ContactName) + ")"
		}
		lines = append(lines, contact)
	}

	return trimToLimit(strings.Join(lines, "\n"))
}

// trimToLimit keeps a message inside Telegram's ceiling.
//
// Trimming beats being refused by the API, and the cut lands on a character
// boundary rather than in the middle of one: Cyrillic is two bytes per letter
// here, so a byte-wise cut would end the message in half a word.
func trimToLimit(message string) string {
	const ellipsis = "…"
	if len(message) <= telegramMessageLimit {
		return message
	}

	cut := telegramMessageLimit - len(ellipsis)
	for cut > 0 && !utf8.RuneStart(message[cut]) {
		cut--
	}
	return strings.TrimSpace(message[:cut]) + ellipsis
}

func routeLine(ad *domain.LoadAd) string {
	origin := strings.TrimSpace(ad.OriginText)
	destination := strings.TrimSpace(ad.DestText)
	switch {
	case origin != "" && destination != "":
		return origin + " → " + destination
	case origin != "":
		return origin + " →"
	case destination != "":
		return "→ " + destination
	default:
		return ""
	}
}

// specLine puts the freight on one line, the way a dispatcher reads it:
// "20 т · тент · 86 м³".
func specLine(ad *domain.LoadAd, labels map[string]string) string {
	var parts []string

	if ad.WeightKg != nil {
		parts = append(parts, formatTonnes(*ad.WeightKg)+" "+labels["weight"])
	}
	if ad.VehicleType != nil && *ad.VehicleType != "" {
		parts = append(parts, *ad.VehicleType)
	}
	if ad.VolumeM3 != nil {
		parts = append(parts, strconv.FormatFloat(*ad.VolumeM3, 'f', -1, 64)+" "+labels["volume"])
	}
	if ad.CargoText != nil && strings.TrimSpace(*ad.CargoText) != "" {
		parts = append(parts, strings.TrimSpace(*ad.CargoText))
	}
	return strings.Join(parts, " · ")
}

// formatTonnes writes kilograms the way the trade says them: 20, not 20000, and
// 1.5 rather than 1500.
func formatTonnes(kilograms int) string {
	tonnes := float64(kilograms) / 1000
	if tonnes == float64(int(tonnes)) {
		return strconv.Itoa(int(tonnes))
	}
	return strconv.FormatFloat(tonnes, 'f', 1, 64)
}

func priceLine(ad *domain.LoadAd, labels map[string]string) string {
	if ad.PriceAmount == nil || ad.PriceCurrency == nil {
		if ad.PriceNegotiable {
			return labels["price"] + ": " + labels["negotiable"]
		}
		return ""
	}

	line := labels["price"] + ": " + formatMoney(*ad.PriceAmount) + " " + *ad.PriceCurrency
	if ad.PriceNegotiable {
		line += " (" + labels["negotiable"] + ")"
	}
	return line
}

// formatMoney turns minor units into something readable, grouped in threes the
// way every price in every one of these groups is written.
func formatMoney(minor int64) string {
	major := minor / 100
	digits := strconv.FormatInt(major, 10)

	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	if cents := minor % 100; cents != 0 {
		return fmt.Sprintf("%s.%02d", b.String(), cents)
	}
	return b.String()
}

func paymentLine(ad *domain.LoadAd, labels map[string]string) string {
	var parts []string
	if ad.PaymentTerms != nil {
		if label, known := labels[*ad.PaymentTerms]; known {
			parts = append(parts, label)
		}
	}
	if ad.PaymentMethod != nil {
		if label, known := labels[*ad.PaymentMethod]; known {
			parts = append(parts, label)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return labels["payment"] + ": " + strings.Join(parts, ", ")
}
