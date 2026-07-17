package exchangecalendar

import (
	"errors"
	"reflect"
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

func TestManagerValidateCachedSnapshotRejectsCorruptSnapshots(t *testing.T) {
	manager := NewManager(nil, nil)
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.December, 31, 23, 59, 59, 0, time.UTC)
	validSchedule := marketcalendar.TradingDaySchedule{MarketCode: "US", Date: time.Date(2026, time.July, 3, 0, 0, 0, 0, time.UTC), Status: marketcalendar.TradingDayClosed}
	validSnapshot := marketcalendar.CalendarSnapshot{
		SourceID:   "cache_probe",
		MarketCode: "US",
		From:       from,
		To:         to,
		Schedules:  []marketcalendar.TradingDaySchedule{validSchedule},
	}

	cases := []struct {
		name     string
		snapshot marketcalendar.CalendarSnapshot
		want     string
	}{
		{name: "missing source", snapshot: withCalendarSnapshot(validSnapshot, func(snapshot *marketcalendar.CalendarSnapshot) { snapshot.SourceID = " " }), want: "missing sourceId"},
		{name: "missing market", snapshot: withCalendarSnapshot(validSnapshot, func(snapshot *marketcalendar.CalendarSnapshot) { snapshot.MarketCode = " " }), want: "missing marketCode"},
		{name: "unsupported market", snapshot: withCalendarSnapshot(validSnapshot, func(snapshot *marketcalendar.CalendarSnapshot) { snapshot.MarketCode = "MARS" }), want: `unsupported market "MARS"`},
		{name: "missing range", snapshot: withCalendarSnapshot(validSnapshot, func(snapshot *marketcalendar.CalendarSnapshot) { snapshot.From = time.Time{} }), want: "missing snapshot range"},
		{name: "backward range", snapshot: withCalendarSnapshot(validSnapshot, func(snapshot *marketcalendar.CalendarSnapshot) { snapshot.From = to; snapshot.To = from }), want: "invalid snapshot range"},
		{name: "empty schedule date", snapshot: withCalendarSnapshot(validSnapshot, func(snapshot *marketcalendar.CalendarSnapshot) {
			snapshot.Schedules = []marketcalendar.TradingDaySchedule{{MarketCode: "US", Status: marketcalendar.TradingDayClosed}}
		}), want: "schedule has empty date"},
		{name: "schedule outside snapshot range", snapshot: withCalendarSnapshot(validSnapshot, func(snapshot *marketcalendar.CalendarSnapshot) {
			snapshot.Schedules = []marketcalendar.TradingDaySchedule{{MarketCode: "US", Date: time.Date(2029, time.January, 1, 0, 0, 0, 0, time.UTC), Status: marketcalendar.TradingDayClosed}}
		}), want: "outside snapshot range"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.validateCachedSnapshot(tt.snapshot)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateCachedSnapshot err = %v, want containing %q", err, tt.want)
			}
		})
	}

	if err := manager.validateCachedSnapshot(validSnapshot); err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
}

func TestManagerValidateCachedSnapshotUsesRegisteredSourceValidator(t *testing.T) {
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.December, 31, 23, 59, 59, 0, time.UTC)
	wantErr := errors.New("missing official holiday")
	validatorCalled := false
	registry := NewSourceRegistry()
	registry.Register(validatingSnapshotSource{
		stubSource: stubSource{id: "nyse_official", markets: []string{"US"}},
		validate: func(market string, schedules []marketcalendar.TradingDaySchedule, gotFrom time.Time, gotTo time.Time) error {
			validatorCalled = true
			if market != "US" || len(schedules) != 1 || !gotFrom.Equal(from) || !gotTo.Equal(to) {
				t.Fatalf("validator input = market %q schedules %#v from %s to %s", market, schedules, gotFrom, gotTo)
			}
			return wantErr
		},
	})
	manager := NewManager(nil, nil, WithRegistry(registry))

	err := manager.validateCachedSnapshot(marketcalendar.CalendarSnapshot{
		SourceID:   "nyse_official",
		MarketCode: "US",
		From:       from,
		To:         to,
		Schedules: []marketcalendar.TradingDaySchedule{
			{MarketCode: "US", Date: time.Date(2026, time.July, 3, 0, 0, 0, 0, time.UTC), Status: marketcalendar.TradingDayClosed},
		},
	})
	if !validatorCalled || !errors.Is(err, wantErr) {
		t.Fatalf("validateCachedSnapshot called=%v err=%v, want registered validator error", validatorCalled, err)
	}
}

func TestManagerCachedSnapshotMainlandFallbackAndFreshnessBoundaries(t *testing.T) {
	now := time.Date(2026, time.October, 1, 8, 0, 0, 0, time.UTC)
	registry := NewSourceRegistry()
	registry.Register(stubSource{id: "mainland-cache", markets: []string{"CN"}})
	settings := jfsettings.ExchangeCalendarSettings{SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{{
		Market: "CN", EnabledSourceIDs: []string{"mainland-cache"}, PreferredSourceIDs: []string{"mainland-cache"}, StaleAfterHours: 24,
	}}}
	manager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings { return settings }, WithRegistry(registry), WithClock(func() time.Time { return now }))

	// Cached CN data is allowed to validate and serve the SH/SZ templates, so a
	// persisted shared mainland source remains usable after a restart.
	snapshot := marketcalendar.CalendarSnapshot{
		SourceID: "mainland-cache", MarketCode: "SH",
		From: now.AddDate(0, -1, 0), To: now.AddDate(0, 1, 0),
		FetchedAt: now.Add(-time.Hour), ValidUntil: now.Add(time.Hour),
		Schedules: []marketcalendar.TradingDaySchedule{{
			MarketCode: "SH", Date: now, Status: marketcalendar.TradingDayClosed, Reason: "national_day",
		}},
	}
	if err := manager.validateCachedSnapshot(snapshot); err != nil {
		t.Fatalf("shared mainland snapshot rejected: %v", err)
	}
	manager.cacheSnapshot(snapshot)
	if sourceID, ok := manager.coverageSource("SH", now); !ok || sourceID != "mainland-cache" {
		t.Fatalf("coverageSource(SH) = %q/%v, want mainland-cache/true", sourceID, ok)
	}
	if schedule, sourceID, ok := manager.overrideSchedule("SH", now); !ok || sourceID != "mainland-cache" || schedule.Status != marketcalendar.TradingDayClosed {
		t.Fatalf("overrideSchedule(SH) = %#v/%q/%v", schedule, sourceID, ok)
	}

	// An expired remote snapshot must never remain an effective coverage source.
	if snapshotFresh(marketcalendar.CalendarSnapshot{FetchedAt: now.Add(-25 * time.Hour)}, jfsettings.ExchangeCalendarSourcePolicy{StaleAfterHours: 24}, now) {
		t.Fatal("stale snapshot reported fresh")
	}
	if _, ok := manager.coverageSource("MARS", now); ok {
		t.Fatal("unknown market unexpectedly has remote coverage")
	}
}

func TestSourceRegistryNilDuplicateAndMarketNormalizationBoundaries(t *testing.T) {
	var nilRegistry *SourceRegistry
	if source, ok := nilRegistry.Source("anything"); ok || source != nil {
		t.Fatalf("nil registry Source = %#v/%v", source, ok)
	}
	if got := nilRegistry.OrderedSources("US", jfsettings.ExchangeCalendarSourcePolicy{}); got != nil {
		t.Fatalf("nil registry OrderedSources = %#v, want nil", got)
	}
	if got := nilRegistry.Descriptors(); got != nil {
		t.Fatalf("nil registry Descriptors = %#v, want nil", got)
	}

	registry := NewSourceRegistry()
	var nilSource Source
	registry.Register(nilSource)
	registry.Register(stubSource{id: " ", markets: []string{"US"}})
	registry.Register(stubSource{id: "secondary", markets: []string{"HK"}})
	registry.Register(stubSource{id: "primary", kind: "remote", authority: " First ", markets: []string{" us ", "US", "", "cn"}})
	registry.Register(stubSource{id: "primary", kind: "remote", authority: " Replacement ", markets: []string{"CN"}})

	source, ok := registry.Source(" primary ")
	if !ok || strings.TrimSpace(source.Authority()) != "Replacement" {
		t.Fatalf("replaced source = %#v/%v", source, ok)
	}
	ordered := registry.OrderedSources(" sz ", jfsettings.ExchangeCalendarSourcePolicy{
		PreferredSourceIDs: []string{"secondary", "primary", "primary"},
		EnabledSourceIDs:   []string{"primary"},
	})
	if len(ordered) != 1 || ordered[0].ID() != "primary" {
		t.Fatalf("ordered SZ sources = %#v, want primary through CN market support", ordered)
	}
	descriptors := registry.Descriptors()
	if got, want := sourceDescriptorIDs(descriptors), []string{"primary", "secondary"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("descriptor ids = %#v, want %#v", got, want)
	}
	if got, want := describeSource(registry.sources["primary"]).Markets, []string{"CN"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("primary descriptor markets = %#v, want %#v", got, want)
	}
	if got := describeSource(nil); got.ID != "" || got.Kind != "" || got.Authority != "" || got.Markets != nil {
		t.Fatalf("nil source descriptor = %#v", got)
	}
	if got, want := candidateSnapshotMarkets(" cn "), []string{"CN", "SH", "SZ"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("candidateSnapshotMarkets(CN) = %#v, want %#v", got, want)
	}
}

func TestExtractNYSEHeaderYearsSkipsMalformedRowsBeforeValidHeader(t *testing.T) {
	rows := [][]string{
		{"Holiday"},
		{"Not Holiday", "2026"},
		{"<strong>Holiday</strong>", "2026", "TBD"},
		{" Holiday ", "<span>2027</span>", "2028"},
	}

	years, rowIndex := extractNYSEHeaderYears(rows)
	if rowIndex != 3 || !reflect.DeepEqual(years, []int{2027, 2028}) {
		t.Fatalf("extractNYSEHeaderYears = years %#v row %d, want [2027 2028]/3", years, rowIndex)
	}
	if years, rowIndex := extractNYSEHeaderYears([][]string{{"Holiday", "TBD"}}); years != nil || rowIndex != -1 {
		t.Fatalf("extractNYSEHeaderYears(invalid only) = years %#v row %d, want nil/-1", years, rowIndex)
	}
}

func withCalendarSnapshot(snapshot marketcalendar.CalendarSnapshot, mutate func(*marketcalendar.CalendarSnapshot)) marketcalendar.CalendarSnapshot {
	mutate(&snapshot)
	return snapshot
}

type validatingSnapshotSource struct {
	stubSource
	validate func(string, []marketcalendar.TradingDaySchedule, time.Time, time.Time) error
}

func (source validatingSnapshotSource) ValidateSnapshot(market string, schedules []marketcalendar.TradingDaySchedule, from time.Time, to time.Time) error {
	if source.validate == nil {
		return nil
	}
	return source.validate(market, schedules, from, to)
}

func sourceDescriptorIDs(descriptors []SourceDescriptor) []string {
	ids := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		ids = append(ids, descriptor.ID)
	}
	return ids
}
