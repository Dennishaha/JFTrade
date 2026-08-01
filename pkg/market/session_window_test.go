package market

import (
	"testing"
	"time"
)

func TestResolveSessionWindowUsesTradingCalendarBoundaries(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	tests := []struct {
		name        string
		at          time.Time
		wantSession Session
		wantDate    string
		wantStart   time.Time
		wantEnd     time.Time
	}{
		{
			name:        "normal after market",
			at:          time.Date(2026, 7, 31, 19, 59, 51, 0, nyLoc),
			wantSession: SessionAfter,
			wantDate:    "2026-07-31",
			wantStart:   time.Date(2026, 7, 31, 20, 0, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "early close after market",
			at:          time.Date(2026, 11, 27, 16, 59, 0, 0, nyLoc),
			wantSession: SessionAfter,
			wantDate:    "2026-11-27",
			wantStart:   time.Date(2026, 11, 27, 18, 0, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 11, 27, 22, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			window, ok := ResolveSessionWindow("US.AAPL", tc.at)
			if !ok {
				t.Fatal("expected session window")
			}
			if window.Session != tc.wantSession || window.TradingDate != tc.wantDate ||
				window.Timezone != "America/New_York" || !window.StartAt.Equal(tc.wantStart) ||
				!window.EndAt.Equal(tc.wantEnd) {
				t.Fatalf("window = %#v", window)
			}
		})
	}
}

func TestResolveSessionWindowRejectsClosedAndResolvesOvernightTradingDate(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	if _, ok := ResolveSessionWindow(
		"US.AAPL",
		time.Date(2026, 8, 1, 12, 0, 0, 0, nyLoc),
	); ok {
		t.Fatal("weekend must not resolve a session window")
	}

	window, ok := ResolveSessionWindow(
		"US.AAPL",
		time.Date(2026, 8, 2, 21, 0, 0, 0, nyLoc),
	)
	if !ok || window.Session != SessionOvernight || window.TradingDate != "2026-08-03" {
		t.Fatalf("overnight window = %#v, ok=%v", window, ok)
	}
	if !window.StartAt.Equal(time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)) ||
		!window.EndAt.Equal(time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("overnight bounds = %s - %s", window.StartAt, window.EndAt)
	}

	window, ok = ResolveSessionWindow(
		"US.AAPL",
		time.Date(2026, 8, 3, 1, 0, 0, 0, nyLoc),
	)
	if !ok || window.TradingDate != "2026-08-03" {
		t.Fatalf("after-midnight overnight window = %#v, ok=%v", window, ok)
	}
}

func TestResolveTradingDaySessionWindowCoversNamedSessionsAndInvalidInputs(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	tradingDay := time.Date(2026, 7, 31, 12, 0, 0, 0, nyLoc)

	for _, tc := range []struct {
		session   Session
		wantStart time.Time
		wantEnd   time.Time
	}{
		{SessionPre, time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 13, 30, 0, 0, time.UTC)},
		{SessionRegular, time.Date(2026, 7, 31, 13, 30, 0, 0, time.UTC), time.Date(2026, 7, 31, 20, 0, 0, 0, time.UTC)},
		{SessionAfter, time.Date(2026, 7, 31, 20, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
	} {
		window, ok := ResolveTradingDaySessionWindow("US.AAPL", tradingDay, tc.session)
		if !ok || !window.StartAt.Equal(tc.wantStart) || !window.EndAt.Equal(tc.wantEnd) {
			t.Fatalf("%s window = %#v, ok=%v", tc.session, window, ok)
		}
	}

	invalidCases := []struct {
		symbol  string
		day     time.Time
		session Session
	}{
		{"US.AAPL", time.Time{}, SessionPre},
		{"US.AAPL", tradingDay, SessionClosed},
		{"US.AAPL", tradingDay, SessionUnknown},
		{"bad-symbol", tradingDay, SessionRegular},
		{"HK.00700", tradingDay, SessionPre},
		{"US.AAPL", time.Date(2026, 8, 1, 12, 0, 0, 0, nyLoc), SessionRegular},
		{"US.AAPL", tradingDay, Session("invalid")},
	}
	for _, tc := range invalidCases {
		if window, ok := ResolveTradingDaySessionWindow(tc.symbol, tc.day, tc.session); ok {
			t.Fatalf("unexpected %s window for %s: %#v", tc.session, tc.symbol, window)
		}
	}

	if _, ok := ResolveSessionWindow("US.AAPL", time.Time{}); ok {
		t.Fatal("zero timestamp must not resolve a session window")
	}
	if _, ok := ResolveSessionWindow("bad-symbol", tradingDay); ok {
		t.Fatal("unknown symbol must not resolve a session window")
	}
}

func TestResolveSessionWindowDoesNotInventMissingCalendarData(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	template := testCalendarResolver{}.template
	template.MarketCode = "HK"
	previous := SwapCalendarResolver(&testCalendarResolver{template: template})
	t.Cleanup(func() { SetCalendarResolver(previous) })

	at := time.Date(2026, 7, 31, 10, 0, 0, 0, nyLoc)
	if _, ok := ResolveSessionWindow("US.AAPL", at); ok {
		t.Fatal("missing US calendar template must not resolve a session window")
	}
	if _, ok := ResolveTradingDaySessionWindow("US.AAPL", at, SessionRegular); ok {
		t.Fatal("missing US calendar template must not resolve a trading-day window")
	}
}

func TestResolveTradingDaySessionWindowTracksDaylightSavingTime(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	tests := []struct {
		name      string
		day       time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "before spring transition",
			day:       time.Date(2026, time.March, 6, 12, 0, 0, 0, nyLoc),
			wantStart: time.Date(2026, time.March, 6, 21, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, time.March, 7, 1, 0, 0, 0, time.UTC),
		},
		{
			name:      "after spring transition",
			day:       time.Date(2026, time.March, 9, 12, 0, 0, 0, nyLoc),
			wantStart: time.Date(2026, time.March, 9, 20, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, time.March, 10, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			window, ok := ResolveTradingDaySessionWindow("US.AAPL", tc.day, SessionAfter)
			if !ok || !window.StartAt.Equal(tc.wantStart) || !window.EndAt.Equal(tc.wantEnd) {
				t.Fatalf("after-market window = %#v, ok=%v", window, ok)
			}
		})
	}
}
