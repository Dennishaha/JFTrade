package calendar

import (
	"testing"
	"time"
)

func TestBuiltinResolverUSHolidayAndEarlyClose(t *testing.T) {
	resolver := NewBuiltinResolver()
	usTemplate, ok := resolver.Template("US")
	if !ok {
		t.Fatal("expected US template")
	}

	juneteenth := time.Date(2026, 6, 19, 12, 0, 0, 0, LoadLocation(usTemplate))
	holiday, ok := resolver.Schedule("US", juneteenth)
	if !ok {
		t.Fatal("expected holiday schedule")
	}
	if holiday.Status != TradingDayClosed {
		t.Fatalf("holiday status = %s, want %s", holiday.Status, TradingDayClosed)
	}

	blackFriday := time.Date(2026, 11, 27, 12, 0, 0, 0, LoadLocation(usTemplate))
	earlyClose, ok := resolver.Schedule("US", blackFriday)
	if !ok {
		t.Fatal("expected early close schedule")
	}
	if earlyClose.Status != TradingDayEarlyClose {
		t.Fatalf("early close status = %s, want %s", earlyClose.Status, TradingDayEarlyClose)
	}
	regular, ok := SessionWindowByKind(earlyClose, SessionRegular)
	if !ok || regular.EndMinute != 13*60 {
		t.Fatalf("regular session = %#v, want end minute 780", regular)
	}
}

func TestBuiltinResolverHKWeekdayFallbackUsesTemplateSessions(t *testing.T) {
	resolver := NewBuiltinResolver()
	hkTemplate, ok := resolver.Template("HK")
	if !ok {
		t.Fatal("expected HK template")
	}
	openDay := time.Date(2026, 6, 22, 10, 0, 0, 0, LoadLocation(hkTemplate))
	hkSchedule, ok := resolver.Schedule("HK", openDay)
	if !ok {
		t.Fatal("expected HK schedule")
	}
	if hkSchedule.Status != TradingDayOpen || len(hkSchedule.Sessions) != 2 {
		t.Fatalf("HK schedule = %#v", hkSchedule)
	}
}

func TestBuiltinResolverMainlandHolidayFallbackClosesKnownHoliday(t *testing.T) {
	resolver := NewBuiltinResolver()
	cnTemplate, ok := resolver.Template("CN")
	if !ok {
		t.Fatal("expected CN template")
	}
	dragonBoatHoliday := time.Date(2026, 6, 19, 10, 0, 0, 0, LoadLocation(cnTemplate))
	cnSchedule, ok := resolver.Schedule("CN", dragonBoatHoliday)
	if !ok {
		t.Fatal("expected CN holiday schedule")
	}
	if cnSchedule.Status != TradingDayClosed {
		t.Fatalf("CN holiday status = %s, want %s", cnSchedule.Status, TradingDayClosed)
	}
	if cnSchedule.Reason != "dragon_boat_festival_holiday" {
		t.Fatalf("CN holiday reason = %q", cnSchedule.Reason)
	}
}

func TestBuiltinResolverMainlandWeekdayFallbackStillOpensRegularDay(t *testing.T) {
	resolver := NewBuiltinResolver()
	cnTemplate, ok := resolver.Template("CN")
	if !ok {
		t.Fatal("expected CN template")
	}
	regularWeekday := time.Date(2026, 6, 22, 10, 0, 0, 0, LoadLocation(cnTemplate))
	cnSchedule, ok := resolver.Schedule("CN", regularWeekday)
	if !ok {
		t.Fatal("expected CN weekday schedule")
	}
	if cnSchedule.Status != TradingDayOpen {
		t.Fatalf("CN weekday status = %s, want %s", cnSchedule.Status, TradingDayOpen)
	}
	if len(cnSchedule.Sessions) != 2 {
		t.Fatalf("CN weekday sessions = %#v", cnSchedule.Sessions)
	}
}

func TestBuiltinResolverMainlandAliasesShareHolidayFallback(t *testing.T) {
	resolver := NewBuiltinResolver()
	shTemplate, ok := resolver.Template("SH")
	if !ok {
		t.Fatal("expected SH template")
	}
	nationalDay := time.Date(2026, 10, 1, 10, 0, 0, 0, LoadLocation(shTemplate))
	for _, market := range []string{"SH", "SZ"} {
		schedule, ok := resolver.Schedule(market, nationalDay)
		if !ok {
			t.Fatalf("expected %s schedule", market)
		}
		if schedule.Status != TradingDayClosed {
			t.Fatalf("%s holiday status = %s, want %s", market, schedule.Status, TradingDayClosed)
		}
		if schedule.Reason != "national_day_holiday" {
			t.Fatalf("%s holiday reason = %q", market, schedule.Reason)
		}
	}
}
