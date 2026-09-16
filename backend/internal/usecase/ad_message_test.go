package usecase

import (
	"strings"
	"testing"
	"time"

	"github.com/loadify/backend/internal/domain"
)

func pointer[T any](v T) *T { return &v }

func fullAd() *domain.LoadAd {
	loading := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	return &domain.LoadAd{
		OriginText:      "Ташкент",
		DestText:        "Москва",
		WeightKg:        pointer(20000),
		VehicleType:     pointer("tent"),
		VolumeM3:        pointer(86.0),
		LoadingDate:     &loading,
		PriceAmount:     pointer(int64(250000)),
		PriceCurrency:   pointer("USD"),
		PriceNegotiable: true,
		PaymentTerms:    pointer("on_delivery"),
		PaymentMethod:   pointer("transfer"),
		ContactPhone:    pointer("+998901234567"),
		ContactName:     pointer("Ulugbek"),
	}
}

func TestRenderRussianAdvert(t *testing.T) {
	want := strings.Join([]string{
		"Ташкент → Москва",
		"20 т · tent · 86 м³",
		"Загрузка: 16.09.2026",
		"Цена: 2 500 USD (торг)",
		"Оплата: при выгрузке, перечисление",
		"Контакт: +998901234567 (Ulugbek)",
	}, "\n")

	if got := renderAd(fullAd(), "ru"); got != want {
		t.Errorf("rendered:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestRenderUzbekAdvert(t *testing.T) {
	got := renderAd(fullAd(), "uz")

	for _, want := range []string{"Yuklash: 16.09.2026", "Narx: 2 500 USD (kelishiladi)", "Aloqa: +998901234567"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered message is missing %q:\n%s", want, got)
		}
	}
}

// A message reading "Volume: —" says nothing and costs a line in a feed people
// skim, so a field nobody filled in is simply absent.
func TestUnstatedFieldsLeaveNoLine(t *testing.T) {
	ad := &domain.LoadAd{
		OriginText:   "Ташкент",
		DestText:     "Москва",
		ContactPhone: pointer("+998901234567"),
	}

	got := renderAd(ad, "ru")
	if strings.Count(got, "\n") != 1 {
		t.Errorf("a two-line advert rendered as:\n%s", got)
	}
	for _, absent := range []string{"Цена", "Загрузка", "Оплата"} {
		if strings.Contains(got, absent) {
			t.Errorf("the message carries an empty %q line:\n%s", absent, got)
		}
	}
}

// The trade says twenty tonnes, not twenty thousand kilograms.
func TestTonnesAreWrittenTheWayTheyAreSpoken(t *testing.T) {
	cases := map[int]string{
		20000: "20 т",
		1500:  "1.5 т",
		500:   "0.5 т",
	}

	for kilograms, want := range cases {
		ad := &domain.LoadAd{OriginText: "A", DestText: "B", WeightKg: pointer(kilograms)}
		if got := renderAd(ad, "ru"); !strings.Contains(got, want) {
			t.Errorf("%d kg rendered as %q, want %q", kilograms, got, want)
		}
	}
}

// Varying the text per group is how a duplicate check gets fooled, and fooling
// it is the opposite of what this product is for.
func TestTheSameAdvertRendersIdenticallyEveryTime(t *testing.T) {
	ad := fullAd()

	first := renderAd(ad, "ru")
	for range 5 {
		if renderAd(ad, "ru") != first {
			t.Fatal("the same advert rendered differently for two groups")
		}
	}
}

func TestALongAdvertIsTrimmedRatherThanRefused(t *testing.T) {
	ad := fullAd()
	ad.Notes = pointer(strings.Repeat("подробности ", 600))

	got := renderAd(ad, "ru")
	if len(got) > telegramMessageLimit {
		t.Errorf("message is %d bytes, over Telegram's limit", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("a trimmed message should say it was trimmed")
	}
}

func TestNegotiableWithoutAFigure(t *testing.T) {
	ad := &domain.LoadAd{OriginText: "A", DestText: "B", PriceNegotiable: true}

	if got := renderAd(ad, "ru"); !strings.Contains(got, "Цена: торг") {
		t.Errorf("rendered:\n%s", got)
	}
}
