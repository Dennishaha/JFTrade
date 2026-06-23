package calendar

import (
	"testing"
	"time"
)

func TestSnapshotHelpersRespectMarketDateAndCoverageWindow(t *testing.T) {
	template := MarketTemplate{MarketCode: "US", Timezone: "America/New_York"}
	targetDay := time.Date(2026, 12, 24, 12, 0, 0, 0, time.UTC)
	otherDay := time.Date(2026, 12, 25, 12, 0, 0, 0, time.UTC)

	snapshot := CalendarSnapshot{
		From: time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC),
		Schedules: []TradingDaySchedule{
			{
				MarketCode: "us",
				Date:       DayStart(template, targetDay),
				Status:     TradingDayEarlyClose,
			},
			{
				MarketCode: "HK",
				Date:       DayStart(MarketTemplate{MarketCode: "HK", Timezone: "Asia/Hong_Kong"}, targetDay),
				Status:     TradingDayOpen,
			},
		},
	}

	schedule, ok := SnapshotSchedule(snapshot, "US", targetDay)
	if !ok {
		t.Fatal("SnapshotSchedule() did not find US day")
	}
	if schedule.Status != TradingDayEarlyClose {
		t.Fatalf("SnapshotSchedule() status = %s, want %s", schedule.Status, TradingDayEarlyClose)
	}
	if _, ok := SnapshotSchedule(snapshot, "US", otherDay); ok {
		t.Fatal("SnapshotSchedule() unexpectedly found a different day")
	}

	if !SnapshotCoversDay(snapshot, template, targetDay) {
		t.Fatal("SnapshotCoversDay() = false, want true inside snapshot window")
	}
	if SnapshotCoversDay(snapshot, template, time.Date(2027, 1, 2, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("SnapshotCoversDay() = true, want false outside snapshot window")
	}
	if SnapshotCoversDay(CalendarSnapshot{}, template, targetDay) {
		t.Fatal("SnapshotCoversDay(zero snapshot) = true, want false")
	}
}

func TestTradingDaySessionHelpersReflectBusinessState(t *testing.T) {
	schedule := TradingDaySchedule{
		Status: TradingDaySpecial,
		Sessions: []SessionWindow{
			{Kind: SessionPre, StartMinute: 8 * 60, EndMinute: 9*60 + 30},
			{Kind: SessionRegular, StartMinute: 9*60 + 30, EndMinute: 16 * 60},
			{Kind: SessionAfter, StartMinute: 16 * 60, EndMinute: 20 * 60},
		},
	}

	if !TradingDayHasSessions(schedule) {
		t.Fatal("TradingDayHasSessions(open special day) = false, want true")
	}
	if TradingDayHasSessions(TradingDaySchedule{Status: TradingDayClosed, Sessions: schedule.Sessions}) {
		t.Fatal("TradingDayHasSessions(closed day) = true, want false")
	}

	tests := []struct {
		minute int
		want   SessionKind
		ok     bool
	}{
		{minute: 8 * 60, want: SessionPre, ok: true},
		{minute: 9*60 + 45, want: SessionRegular, ok: true},
		{minute: 16*60 + 30, want: SessionAfter, ok: true},
		{minute: 20 * 60, want: SessionUnknown, ok: false},
	}
	for _, tc := range tests {
		got, ok := SessionForMinute(schedule, tc.minute)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("SessionForMinute(%d) = (%s, %v), want (%s, %v)", tc.minute, got, ok, tc.want, tc.ok)
		}
	}
}

func TestBuiltinUSChristmasEveEarlyCloseAndTemplateCopies(t *testing.T) {
	resolver := NewBuiltinResolver()
	usTemplate, ok := resolver.Template("US")
	if !ok {
		t.Fatal("expected US template")
	}

	christmasEve := time.Date(2026, 12, 24, 12, 0, 0, 0, LoadLocation(usTemplate))
	schedule, ok := resolver.Schedule("US", christmasEve)
	if !ok {
		t.Fatal("expected Christmas Eve schedule")
	}
	if schedule.Status != TradingDayEarlyClose {
		t.Fatalf("schedule status = %s, want %s", schedule.Status, TradingDayEarlyClose)
	}
	if schedule.Reason != "christmas_eve_early_close" {
		t.Fatalf("schedule reason = %q, want christmas_eve_early_close", schedule.Reason)
	}
	regular, ok := SessionWindowByKind(schedule, SessionRegular)
	if !ok || regular.EndMinute != 13*60 {
		t.Fatalf("regular session = %#v, want early close at 13:00", regular)
	}
	if !isChristmasEveEarlyClose(christmasEve) {
		t.Fatal("isChristmasEveEarlyClose() = false, want true")
	}
	if isChristmasEveEarlyClose(time.Date(2027, 12, 25, 12, 0, 0, 0, LoadLocation(usTemplate))) {
		t.Fatal("isChristmasEveEarlyClose(Christmas Day) = true, want false")
	}

	templates := MarketTemplates()
	usCopy := templates["US"]
	if len(usCopy.RegularSessions) == 0 || len(usCopy.ExtendedSessions) == 0 {
		t.Fatalf("MarketTemplates()[US] = %#v, want regular and extended sessions", usCopy)
	}

	originalRegularStart := usCopy.RegularSessions[0].StartMinute
	originalExtendedStart := usCopy.ExtendedSessions[0].StartMinute
	usCopy.RegularSessions[0].StartMinute = 1
	usCopy.ExtendedSessions[0].StartMinute = 2

	templatesAgain := MarketTemplates()
	if templatesAgain["US"].RegularSessions[0].StartMinute != originalRegularStart {
		t.Fatalf("regular sessions leaked mutation: got %d want %d", templatesAgain["US"].RegularSessions[0].StartMinute, originalRegularStart)
	}
	if templatesAgain["US"].ExtendedSessions[0].StartMinute != originalExtendedStart {
		t.Fatalf("extended sessions leaked mutation: got %d want %d", templatesAgain["US"].ExtendedSessions[0].StartMinute, originalExtendedStart)
	}
}
