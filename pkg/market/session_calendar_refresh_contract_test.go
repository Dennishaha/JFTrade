package market

import (
	"strings"
	"testing"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

// refreshWindowResolver models a calendar refresh that happens between the
// session classification and the subsequent window lookup. A missing window
// must fail closed instead of manufacturing an extended-hours bucket.
type refreshWindowResolver struct {
	template  marketcalendar.MarketTemplate
	schedules []marketcalendar.TradingDaySchedule
	calls     int
}

func (r *refreshWindowResolver) Template(marketCode string) (marketcalendar.MarketTemplate, bool) {
	if strings.EqualFold(marketCode, r.template.MarketCode) {
		return r.template, true
	}
	return marketcalendar.MarketTemplate{}, false
}

func (r *refreshWindowResolver) Schedule(marketCode string, _ time.Time) (marketcalendar.TradingDaySchedule, bool) {
	if !strings.EqualFold(marketCode, r.template.MarketCode) || len(r.schedules) == 0 {
		return marketcalendar.TradingDaySchedule{}, false
	}
	index := min(r.calls, len(r.schedules)-1)
	r.calls++
	return r.schedules[index], true
}

func TestCoverage98ExtendedWindowFailsClosedWhenCalendarRefreshRemovesClassifiedWindow(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	template := marketcalendar.MarketTemplate{
		MarketCode:             "US",
		Timezone:               "America/New_York",
		SupportsExtendedHours:  true,
		OvernightCarryStartMin: 20 * 60,
	}
	cases := []struct {
		name            string
		at              time.Time
		classified      marketcalendar.SessionWindow
		refreshedWindow marketcalendar.SessionWindow
	}{
		{
			name:            "overnight window disappears",
			at:              time.Date(2026, time.June, 15, 2, 0, 0, 0, nyLoc),
			classified:      marketcalendar.SessionWindow{Kind: marketcalendar.SessionOvernight, StartMinute: 0, EndMinute: 4 * 60},
			refreshedWindow: marketcalendar.SessionWindow{Kind: marketcalendar.SessionRegular, StartMinute: 9*60 + 30, EndMinute: 16 * 60},
		},
		{
			name:            "pre-market window disappears",
			at:              time.Date(2026, time.June, 15, 5, 0, 0, 0, nyLoc),
			classified:      marketcalendar.SessionWindow{Kind: marketcalendar.SessionPre, StartMinute: 4 * 60, EndMinute: 9*60 + 30},
			refreshedWindow: marketcalendar.SessionWindow{Kind: marketcalendar.SessionRegular, StartMinute: 9*60 + 30, EndMinute: 16 * 60},
		},
		{
			name:            "regular window disappears",
			at:              time.Date(2026, time.June, 15, 10, 0, 0, 0, nyLoc),
			classified:      marketcalendar.SessionWindow{Kind: marketcalendar.SessionRegular, StartMinute: 9*60 + 30, EndMinute: 16 * 60},
			refreshedWindow: marketcalendar.SessionWindow{Kind: marketcalendar.SessionPre, StartMinute: 4 * 60, EndMinute: 9*60 + 30},
		},
		{
			name:            "after-hours window disappears",
			at:              time.Date(2026, time.June, 15, 17, 0, 0, 0, nyLoc),
			classified:      marketcalendar.SessionWindow{Kind: marketcalendar.SessionAfter, StartMinute: 16 * 60, EndMinute: 20 * 60},
			refreshedWindow: marketcalendar.SessionWindow{Kind: marketcalendar.SessionRegular, StartMinute: 9*60 + 30, EndMinute: 16 * 60},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			initial := marketcalendar.ScheduleForDate(template, marketcalendar.TradingDayOpen, tc.at, "initial", "", false, []marketcalendar.SessionWindow{tc.classified})
			refreshed := marketcalendar.ScheduleForDate(template, marketcalendar.TradingDayOpen, tc.at, "refresh", "", false, []marketcalendar.SessionWindow{tc.refreshedWindow})
			resolver := &refreshWindowResolver{template: template, schedules: []marketcalendar.TradingDaySchedule{initial, refreshed}}
			previous := SwapCalendarResolver(resolver)
			t.Cleanup(func() { SetCalendarResolver(previous) })

			if _, _, ok := usExtendedSessionWindowBounds("US.AAPL", tc.at); ok {
				t.Fatal("calendar refresh with no matching window produced extended-hours bounds")
			}
			if resolver.calls < 2 {
				t.Fatalf("calendar resolver calls = %d, want classification and refreshed lookup", resolver.calls)
			}
		})
	}
}
