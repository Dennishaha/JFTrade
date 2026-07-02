package exchangecalendar

import (
	"errors"
	"strings"
	"testing"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func TestManagerLifecycleAndTemplateBoundaries(t *testing.T) {
	var nilManager *Manager
	nilManager.Start()
	nilManager.NotifySettingsChanged()
	if err := nilManager.Close(); err != nil {
		t.Fatalf("nil manager close = %v", err)
	}
	if _, ok := nilManager.Template("US"); ok {
		t.Fatalf("nil manager returned a template")
	}
	if _, ok := nilManager.Schedule("US", time.Now()); ok {
		t.Fatalf("nil manager returned a schedule")
	}

	manager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings {
		return jfsettings.ExchangeCalendarSettings{AutoRefreshEnabled: false, RefreshIntervalHours: 0}
	})
	if template, ok := manager.Template(" us "); !ok || template.MarketCode != "US" {
		t.Fatalf("Template(us) = %#v/%v", template, ok)
	}
	if _, ok := manager.Template("MARS"); ok {
		t.Fatalf("unknown market returned a template")
	}
	if _, ok := manager.Schedule("US", time.Time{}); ok {
		t.Fatalf("zero day returned a schedule")
	}

	manager.NotifySettingsChanged()
	manager.NotifySettingsChanged()
	manager.Start()
	manager.NotifySettingsChanged()
	if err := manager.Close(); err != nil {
		t.Fatalf("manager close = %v", err)
	}
}

func TestManualOverrideStatusAndSessionBoundaries(t *testing.T) {
	statusCases := map[string]marketcalendar.TradingDayStatus{
		" open ":       marketcalendar.TradingDayOpen,
		"CLOSED":       marketcalendar.TradingDayClosed,
		"early_close":  marketcalendar.TradingDayEarlyClose,
		"special":      marketcalendar.TradingDaySpecial,
		"not-a-status": marketcalendar.TradingDayUnknown,
	}
	for raw, want := range statusCases {
		if got := manualStatus(raw); got != want {
			t.Fatalf("manualStatus(%q) = %s, want %s", raw, got, want)
		}
	}

	sessions := manualSessions([]jfsettings.ExchangeCalendarSessionWindow{
		{Kind: "regular", StartMinute: 570, EndMinute: 960},
		{Kind: "pre", StartMinute: 240, EndMinute: 570},
		{Kind: "after", StartMinute: 960, EndMinute: 1200},
		{Kind: "overnight", StartMinute: 1200, EndMinute: 1440},
		{Kind: "closed", StartMinute: 0, EndMinute: 1},
		{Kind: "mystery", StartMinute: 1, EndMinute: 2},
		{Kind: "regular", StartMinute: 700, EndMinute: 700},
	})
	if len(sessions) != 6 {
		t.Fatalf("manualSessions len = %d, want invalid zero-width session dropped", len(sessions))
	}
	wantKinds := []marketcalendar.SessionKind{
		marketcalendar.SessionClosed,
		marketcalendar.SessionUnknown,
		marketcalendar.SessionPre,
		marketcalendar.SessionRegular,
		marketcalendar.SessionAfter,
		marketcalendar.SessionOvernight,
	}
	for i, want := range wantKinds {
		if sessions[i].Kind != want {
			t.Fatalf("sessions[%d].Kind = %s, want %s in %#v", i, sessions[i].Kind, want, sessions)
		}
	}

	settings := jfsettings.ExchangeCalendarSettings{
		ManualOverrides: []jfsettings.ExchangeCalendarManualOverride{
			{
				Market:   "CN",
				Date:     "2026-10-01",
				Status:   "special",
				Reason:   "national_day_override",
				Observed: true,
				Sessions: []jfsettings.ExchangeCalendarSessionWindow{
					{Kind: "regular", StartMinute: 570, EndMinute: 690},
				},
			},
		},
	}
	builtin := marketcalendar.NewBuiltinResolver()
	for _, marketCode := range []string{"SH", "SZ"} {
		schedule, ok := manualOverrideSchedule(settings, builtin, marketCode, time.Date(2026, time.October, 1, 12, 0, 0, 0, time.FixedZone("CST", 8*3600)))
		if !ok {
			t.Fatalf("%s manual CN override not applied", marketCode)
		}
		if schedule.MarketCode != marketCode || schedule.Status != marketcalendar.TradingDaySpecial || schedule.SourceID != ManualOverrideSourceID || !schedule.Observed {
			t.Fatalf("%s manual override schedule = %#v", marketCode, schedule)
		}
		if len(schedule.Sessions) != 1 || schedule.Sessions[0].Kind != marketcalendar.SessionRegular {
			t.Fatalf("%s manual sessions = %#v", marketCode, schedule.Sessions)
		}
	}
}

func TestHTTPCalendarSourceValidateSnapshotBoundary(t *testing.T) {
	if err := ((*HTTPCalendarSource)(nil)).ValidateSnapshot("US", nil, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("nil source ValidateSnapshot = %v", err)
	}
	if err := (&HTTPCalendarSource{}).ValidateSnapshot("US", nil, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("source without validator ValidateSnapshot = %v", err)
	}

	wantErr := errors.New("not enough official holidays")
	called := false
	source := &HTTPCalendarSource{
		validate: func(market string, schedules []marketcalendar.TradingDaySchedule, from time.Time, to time.Time) error {
			called = true
			if market != "US" || len(schedules) != 1 || from.IsZero() || to.IsZero() {
				t.Fatalf("validator input = market %q schedules %#v from %s to %s", market, schedules, from, to)
			}
			return wantErr
		},
	}
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.December, 31, 23, 59, 59, 0, time.UTC)
	err := source.ValidateSnapshot("US", []marketcalendar.TradingDaySchedule{{MarketCode: "US", Date: from, Status: marketcalendar.TradingDayClosed}}, from, to)
	if !called || !errors.Is(err, wantErr) {
		t.Fatalf("ValidateSnapshot called=%v err=%v, want validator error", called, err)
	}
}

func TestCalendarSourceAvailabilityNotesAndRefreshTargets(t *testing.T) {
	for _, sourceID := range []string{"nyse_official", "nasdaq_verifier", "hk_gov_1823_ical", "mainland_official_notice", BuiltinSourceID, ManualOverrideSourceID} {
		if note := sourceAvailabilityNote(sourceID); strings.TrimSpace(note) == "" {
			t.Fatalf("sourceAvailabilityNote(%q) is empty", sourceID)
		}
	}
	if note := sourceAvailabilityNote("custom_source"); note != "" {
		t.Fatalf("custom source note = %q, want empty", note)
	}

	targetCases := map[string][]string{
		"":   {"CN"},
		"CN": {"CN"},
		"SH": {"CN"},
		"SZ": {"CN"},
		"US": {"US"},
		"hk": {"HK"},
	}
	for raw, want := range targetCases {
		got := refreshMarketsForTarget(raw)
		if len(got) != len(want) || got[0] != want[0] {
			t.Fatalf("refreshMarketsForTarget(%q) = %#v, want %#v", raw, got, want)
		}
	}
}
