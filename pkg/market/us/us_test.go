package us

import (
	"testing"
	"time"
)

func TestUSProfileUsesNewYorkTimezoneAndRegularSession(t *testing.T) {
	if got := Location().String(); got != LocationName {
		t.Fatalf("Location = %q, want %q", got, LocationName)
	}
	if len(RegularWindows) != 1 || RegularWindows[0] != [2]int{9*60 + 30, 16 * 60} {
		t.Fatalf("RegularWindows = %#v", RegularWindows)
	}
}

func TestUSTradingDayAndEarlyCloseRules(t *testing.T) {
	loc := Location()

	holiday := time.Date(2026, time.January, 1, 12, 0, 0, 0, loc)
	if IsTradingDay(holiday) {
		t.Fatal("New Year's Day should be a holiday")
	}

	saturday := time.Date(2026, time.January, 3, 12, 0, 0, 0, loc)
	if IsTradingDay(saturday) {
		t.Fatal("Saturday should not be a trading day")
	}

	earlyClose := time.Date(2025, time.July, 3, 12, 0, 0, 0, loc)
	if !IsTradingDay(earlyClose) {
		t.Fatal("July 3, 2025 should be a trading day")
	}
	if !IsEarlyCloseDay(earlyClose) {
		t.Fatal("July 3, 2025 should be an early close day")
	}
	if RegularSessionEndMinute(earlyClose) != EarlyRegularEndMinute {
		t.Fatalf("RegularSessionEndMinute = %d, want %d", RegularSessionEndMinute(earlyClose), EarlyRegularEndMinute)
	}
	if AfterSessionEndMinute(earlyClose) != EarlyAfterEndMinute {
		t.Fatalf("AfterSessionEndMinute = %d, want %d", AfterSessionEndMinute(earlyClose), EarlyAfterEndMinute)
	}

	regular := time.Date(2026, time.July, 6, 12, 0, 0, 0, loc)
	if IsEarlyCloseDay(regular) {
		t.Fatal("July 6, 2026 should not be an early close day")
	}
	if RegularSessionEndMinute(regular) != RegularEndMinute {
		t.Fatalf("RegularSessionEndMinute regular = %d, want %d", RegularSessionEndMinute(regular), RegularEndMinute)
	}
}
