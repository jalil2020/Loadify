package nlq

import (
	"time"

	"github.com/loadify/backend/internal/domain"
)

// dayOffsets are the relative day words, in days from today.
//
// Uzbek has a single word for the day after tomorrow ("indinga") that both
// Russian and English need two for, which is the kind of thing a per-language
// keyword list handles and a shared one does not.
var dayOffsets = map[string]int{
	"bugun": 0, "segodnya": 0, "today": 0,
	"ertaga": 1, "erta": 1, "zavtra": 1, "tomorrow": 1,
	"indinga": 2, "poslezavtra": 2,
}

// dayUnits are the words for "day" that follow a count.
var dayUnits = map[string]bool{
	"kun": true, "kunda": true, "kunlik": true, "kungacha": true,
	"dnya": true, "dney": true, "den": true,
	"day": true, "days": true,
}

// DateExtractor resolves loading-time words into a window.
//
// It runs before the numeric rules so "3 kun" is not read as a tonnage.
type DateExtractor struct {
	now Clock
}

func NewDateExtractor(now Clock) *DateExtractor {
	if now == nil {
		now = time.Now
	}
	return &DateExtractor{now: now}
}

func (e *DateExtractor) Name() string { return "date" }

func (e *DateExtractor) Extract(scan *Scan, out *domain.SearchIntent) {
	for i := 0; i < len(scan.Tokens); i++ {
		if scan.IsClaimed(i) {
			continue
		}
		if from, to, ok := e.windowAt(scan, i); ok {
			out.ReadyFrom = &from
			out.ReadyTo = &to
			return
		}
	}
}

// windowAt reads either a day word or the "N kun" form, returning the half-open
// window it denotes.
func (e *DateExtractor) windowAt(scan *Scan, i int) (time.Time, time.Time, bool) {
	today := e.startOfToday()

	// "ertaga" names one specific day, so the window is that day alone —
	// someone saying it is not asking for loads at this hour tomorrow.
	if offset, found := dayOffsets[scan.Folded(i)]; found {
		scan.Claim(i, i+1, e.Name())
		start := today.AddDate(0, 0, offset)
		return start, start.AddDate(0, 0, 1), true
	}

	// "3 kun" names a span, so the window opens now and runs that many days.
	count, ok := parseNumber(scan.Folded(i))
	if !ok || count < 1 || count > maxDayspan {
		return time.Time{}, time.Time{}, false
	}
	if !dayUnits[scan.Folded(i+1)] || scan.IsClaimed(i+1) {
		return time.Time{}, time.Time{}, false
	}

	scan.Claim(i, i+2, e.Name())
	return today, today.AddDate(0, 0, int(count)), true
}

func (e *DateExtractor) startOfToday() time.Time {
	now := e.now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// maxDayspan bounds what counts as a day count. Beyond it the figure is far
// more likely a tonnage or a price that happened to sit next to the word.
const maxDayspan = 30
