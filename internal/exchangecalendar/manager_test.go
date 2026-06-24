package exchangecalendar

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	calendarstore "github.com/jftrade/jftrade-main/internal/store/exchangecalendar"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}

type stubSource struct {
	id        string
	kind      string
	authority string
	markets   []string
	fetch     func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error)
}

func TestManagerFailureBackoffUsesHoursAndCapsAtTwentyFour(t *testing.T) {
	now := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	manager := NewManager(nil, nil, WithClock(func() time.Time { return now }))

	manager.recordOperationFailure("operation-source", errors.New("operation failed"))
	if got, want := manager.statuses["operation-source"].NextRefreshAt, now.Add(time.Hour); !got.Equal(want) {
		t.Fatalf("first operation retry = %s, want %s", got, want)
	}

	for range 30 {
		manager.recordSourceFailure("calendar-source", "US", errors.New("refresh failed"), "fetch_failed")
	}
	if got, want := manager.statuses["calendar-source"].NextRefreshAt, now.Add(24*time.Hour); !got.Equal(want) {
		t.Fatalf("capped source retry = %s, want %s", got, want)
	}
}

func TestDefaultWarmupRefreshTimeoutCoversSequentialRemoteSources(t *testing.T) {
	if defaultWarmupRefreshTimeout < 3*defaultHTTPTimeout {
		t.Fatalf("defaultWarmupRefreshTimeout = %s, want at least three source fetch windows of %s", defaultWarmupRefreshTimeout, defaultHTTPTimeout)
	}
}

func (s stubSource) ID() string        { return s.id }
func (s stubSource) Kind() string      { return s.kind }
func (s stubSource) Markets() []string { return append([]string(nil), s.markets...) }
func (s stubSource) Authority() string { return s.authority }
func (s stubSource) Fetch(ctx context.Context, market string, from time.Time, to time.Time) (marketcalendar.CalendarSnapshot, error) {
	return s.fetch(ctx, market, from, to)
}

func TestManagerFallsBackToBuiltinWhenOfficialRefreshFails(t *testing.T) {
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id:        "nyse_official",
		kind:      "official_html",
		authority: "NYSE",
		markets:   []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{}, errors.New("boom")
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "US",
				PreferredSourceIDs: []string{"nyse_official"},
				EnabledSourceIDs:   []string{"nyse_official"},
				FallbackToBuiltin:  true,
				StaleAfterHours:    24,
			},
		},
	}
	manager := NewManager(calendarstore.New(t.TempDir()), func() jfsettings.ExchangeCalendarSettings { return settings }, WithRegistry(registry), WithClock(func() time.Time {
		return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	}))

	result := manager.RefreshAll(context.Background())
	if result["failures"] != 1 {
		t.Fatalf("refresh result = %#v", result)
	}

	schedule, ok := manager.Schedule("US", time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("expected schedule")
	}
	if schedule.Status != marketcalendar.TradingDayClosed || schedule.SourceID != BuiltinSourceID {
		t.Fatalf("schedule = %#v", schedule)
	}
	sources := manager.Sources()
	var nyseSource map[string]any
	for _, source := range sources {
		if source["id"] == "nyse_official" {
			nyseSource = source
			break
		}
	}
	if nyseSource == nil || nyseSource["lastError"] == "" {
		t.Fatalf("sources = %#v, want lastError after failed refresh", sources)
	}
}

func TestManagerStatusIncludesSnapshotSummariesAndSampleSchedules(t *testing.T) {
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id:        "nyse_official",
		kind:      "official_html",
		authority: "NYSE",
		markets:   []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			fetchedAt := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
			return marketcalendar.CalendarSnapshot{
				MarketCode: "US",
				SourceID:   "nyse_official",
				From:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				To:         time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC),
				FetchedAt:  fetchedAt,
				ValidUntil: fetchedAt.Add(7 * 24 * time.Hour),
				Checksum:   "checksum-1",
				Schedules: []marketcalendar.TradingDaySchedule{
					{
						MarketCode: "US",
						Date:       time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
						Status:     marketcalendar.TradingDayOpen,
						SourceID:   "nyse_official",
					},
					{
						MarketCode: "US",
						Date:       time.Date(2026, 1, 19, 0, 0, 0, 0, time.UTC),
						Status:     marketcalendar.TradingDayClosed,
						Reason:     "holiday",
						SourceID:   "nyse_official",
						Observed:   true,
					},
					{
						MarketCode: "US",
						Date:       time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
						Status:     marketcalendar.TradingDayEarlyClose,
						Reason:     "early close",
						SourceID:   "nyse_official",
						Sessions: []marketcalendar.SessionWindow{
							{Kind: marketcalendar.SessionRegular, StartMinute: 570, EndMinute: 780},
						},
					},
				},
			}, nil
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "US",
				PreferredSourceIDs: []string{"nyse_official"},
				EnabledSourceIDs:   []string{"nyse_official"},
				FallbackToBuiltin:  true,
				StaleAfterHours:    72,
			},
		},
	}
	manager := NewManager(calendarstore.New(t.TempDir()), func() jfsettings.ExchangeCalendarSettings { return settings }, WithRegistry(registry), WithClock(func() time.Time {
		return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	}))

	if result := manager.RefreshAll(context.Background()); result["updated"] != 1 || result["failures"] != 0 {
		t.Fatalf("RefreshAll result = %#v", result)
	}
	status := manager.Status()
	snapshots := jftradeCheckedTypeAssertion[[]map[string]any](status["snapshots"])
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %#v", snapshots)
	}
	snapshot := snapshots[0]
	if snapshot["market"] != "US" || snapshot["sourceId"] != "nyse_official" || snapshot["schedulesParsed"] != 3 || snapshot["checksum"] != "checksum-1" {
		t.Fatalf("snapshot summary = %#v", snapshot)
	}
	samples := jftradeCheckedTypeAssertion[[]map[string]any](snapshot["sampleSchedules"])
	if len(samples) != 2 {
		t.Fatalf("sampleSchedules = %#v, want closed/early-close samples only", samples)
	}
	if samples[0]["date"] != "2026-01-19" || samples[0]["status"] != marketcalendar.TradingDayClosed || samples[0]["reason"] != "holiday" {
		t.Fatalf("first sample = %#v", samples[0])
	}
	if samples[1]["date"] != "2026-11-27" || samples[1]["status"] != marketcalendar.TradingDayEarlyClose {
		t.Fatalf("second sample = %#v", samples[1])
	}
}

func TestManagerManualOverridesBeatRemoteAndBuiltin(t *testing.T) {
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"US"},
		ManualOverrides: []jfsettings.ExchangeCalendarManualOverride{
			{
				Market: "US",
				Date:   "2026-06-19",
				Status: "open",
				Sessions: []jfsettings.ExchangeCalendarSessionWindow{
					{Kind: "regular", StartMinute: 570, EndMinute: 960},
				},
				Reason: "manual_reopen",
			},
		},
	}
	manager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings { return settings }, WithRegistry(NewSourceRegistry()))

	schedule, ok := manager.Schedule("US", time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("expected schedule")
	}
	if schedule.SourceID != ManualOverrideSourceID || schedule.Status != marketcalendar.TradingDayOpen {
		t.Fatalf("schedule = %#v", schedule)
	}
}

func TestManagerSharedMainlandSourceAppliesToSHAndSZ(t *testing.T) {
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id:        "mainland_official_notice",
		kind:      "official_pdf_notice",
		authority: "Mainland Notice",
		markets:   []string{"CN", "SH", "SZ"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{
				MarketCode: "CN",
				SourceID:   "mainland_official_notice",
				From:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*3600)),
				To:         time.Date(2026, 12, 31, 0, 0, 0, 0, time.FixedZone("CST", 8*3600)),
				Schedules: []marketcalendar.TradingDaySchedule{
					{
						MarketCode: "CN",
						Date:       time.Date(2026, 10, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*3600)),
						Status:     marketcalendar.TradingDayClosed,
						Reason:     "national_day",
					},
				},
				FetchedAt:  time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC),
				ValidUntil: time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"CN"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "CN",
				PreferredSourceIDs: []string{"mainland_official_notice"},
				EnabledSourceIDs:   []string{"mainland_official_notice"},
				FallbackToBuiltin:  true,
				StaleAfterHours:    24 * 30,
			},
		},
	}
	manager := NewManager(calendarstore.New(t.TempDir()), func() jfsettings.ExchangeCalendarSettings { return settings }, WithRegistry(registry), WithClock(func() time.Time {
		return time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	}))
	manager.RefreshMarket(context.Background(), "CN")

	for _, market := range []string{"SH", "SZ"} {
		schedule, ok := manager.Schedule(market, time.Date(2026, 10, 1, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)))
		if !ok {
			t.Fatalf("%s expected schedule", market)
		}
		if schedule.Status != marketcalendar.TradingDayClosed || schedule.SourceID != "mainland_official_notice" {
			t.Fatalf("%s schedule = %#v", market, schedule)
		}
	}
}

func TestManagerIgnoresStaleRemoteSnapshots(t *testing.T) {
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id:        "nyse_official",
		kind:      "official_html",
		authority: "NYSE",
		markets:   []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{
				MarketCode: "US",
				SourceID:   "nyse_official",
				From:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("EST", -5*3600)),
				To:         time.Date(2026, 12, 31, 0, 0, 0, 0, time.FixedZone("EST", -5*3600)),
				Schedules: []marketcalendar.TradingDaySchedule{
					{
						MarketCode: "US",
						Date:       time.Date(2026, 6, 22, 0, 0, 0, 0, time.FixedZone("EST", -5*3600)),
						Status:     marketcalendar.TradingDayClosed,
						Reason:     "stale_remote",
					},
				},
				FetchedAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				ValidUntil: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	})
	now := time.Date(2026, 6, 22, 14, 0, 0, 0, time.UTC)
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "US",
				PreferredSourceIDs: []string{"nyse_official"},
				EnabledSourceIDs:   []string{"nyse_official"},
				FallbackToBuiltin:  true,
				StaleAfterHours:    24,
			},
		},
	}
	manager := NewManager(calendarstore.New(t.TempDir()), func() jfsettings.ExchangeCalendarSettings { return settings }, WithRegistry(registry), WithClock(func() time.Time {
		return now
	}))
	manager.RefreshAll(context.Background())

	schedule, ok := manager.Schedule("US", now)
	if !ok {
		t.Fatal("expected schedule")
	}
	if schedule.SourceID != BuiltinSourceID || schedule.Status != marketcalendar.TradingDayOpen {
		t.Fatalf("schedule = %#v", schedule)
	}
}

func TestManagerDiscardInvalidCachedSnapshotOnRestore(t *testing.T) {
	root := t.TempDir()
	store := calendarstore.New(root)
	loc := time.FixedZone("EST", -5*3600)
	invalid := marketcalendar.CalendarSnapshot{
		MarketCode: "US",
		SourceID:   "nyse_official",
		From:       time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
		To:         time.Date(2027, 12, 31, 23, 59, 59, 0, loc),
		Schedules: []marketcalendar.TradingDaySchedule{
			{
				MarketCode: "US",
				Date:       time.Date(2028, 7, 3, 0, 0, 0, 0, loc),
				Status:     marketcalendar.TradingDayEarlyClose,
			},
		},
		FetchedAt:  time.Date(2026, 6, 19, 15, 29, 25, 0, time.UTC),
		ValidUntil: time.Date(2026, 7, 3, 15, 29, 25, 0, time.UTC),
	}
	if err := store.SaveSnapshot(invalid); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	manager := NewManager(store, func() jfsettings.ExchangeCalendarSettings {
		return jfsettings.ExchangeCalendarSettings{
			AutoRefreshEnabled:   false,
			RefreshIntervalHours: 24,
			WarmupMarkets:        []string{"US"},
		}
	}, WithClock(func() time.Time {
		return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	}))

	schedule, ok := manager.Schedule("US", time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("expected builtin schedule")
	}
	if schedule.SourceID != BuiltinSourceID || schedule.Status != marketcalendar.TradingDayClosed {
		t.Fatalf("schedule = %#v", schedule)
	}

	snapshotPath := filepath.Join(root, "US", "2026", "nyse_official.json")
	if _, err := os.Stat(snapshotPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot path should be removed, stat err = %v", err)
	}
}

func TestManagerProbeMarksHealthySources(t *testing.T) {
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id:        "nyse_official",
		kind:      "official_html",
		authority: "NYSE",
		markets:   []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{
				MarketCode: "US",
				SourceID:   "nyse_official",
				Schedules: []marketcalendar.TradingDaySchedule{
					{
						MarketCode: "US",
						Date:       time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
						Status:     marketcalendar.TradingDayClosed,
					},
				},
				FetchedAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				ValidUntil: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "US",
				PreferredSourceIDs: []string{"nyse_official"},
				EnabledSourceIDs:   []string{"nyse_official"},
				FallbackToBuiltin:  true,
			},
		},
	}
	manager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings { return settings }, WithRegistry(registry), WithClock(func() time.Time {
		return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	}))

	result := manager.ProbeAll(context.Background())
	if result["healthy"] != 1 || result["failures"] != 0 {
		t.Fatalf("probe result = %#v", result)
	}

	status := manager.sourceStatus("nyse_official")
	if status.LastProbeStatus != "healthy" || status.LastProbeSchedules != 1 || status.LastProbeMarket != "US" {
		t.Fatalf("source status = %#v", status)
	}
}

func TestManagerProbeMarksEmptyParsesUnhealthy(t *testing.T) {
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id:        "hk_gov_1823_ical",
		kind:      "official_ical",
		authority: "GovHK 1823",
		markets:   []string{"HK"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{
				MarketCode: "HK",
				SourceID:   "hk_gov_1823_ical",
				FetchedAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				ValidUntil: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"HK"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "HK",
				PreferredSourceIDs: []string{"hk_gov_1823_ical"},
				EnabledSourceIDs:   []string{"hk_gov_1823_ical"},
				FallbackToBuiltin:  true,
			},
		},
	}
	manager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings { return settings }, WithRegistry(registry), WithClock(func() time.Time {
		return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	}))

	result := manager.ProbeAll(context.Background())
	if result["healthy"] != 0 || result["failures"] != 1 {
		t.Fatalf("probe result = %#v", result)
	}

	status := manager.sourceStatus("hk_gov_1823_ical")
	if status.LastProbeStatus != "unhealthy" || status.LastProbeError != "no schedules parsed" {
		t.Fatalf("source status = %#v", status)
	}
}

func TestManagerRefreshTreatsEmptyParsesAsFailureAndAlerts(t *testing.T) {
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id:        "nyse_official",
		kind:      "official_html",
		authority: "NYSE",
		markets:   []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{
				MarketCode: "US",
				SourceID:   "nyse_official",
				FetchedAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				ValidUntil: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "US",
				PreferredSourceIDs: []string{"nyse_official"},
				EnabledSourceIDs:   []string{"nyse_official"},
				FallbackToBuiltin:  true,
			},
		},
	}
	var alerts []SourceAlert
	manager := NewManager(
		nil,
		func() jfsettings.ExchangeCalendarSettings { return settings },
		WithRegistry(registry),
		WithAlertSink(func(alert SourceAlert) {
			alerts = append(alerts, alert)
		}),
		WithClock(func() time.Time {
			return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
		}),
	)

	result := manager.RefreshAll(context.Background())
	if result["updated"] != 0 || result["failures"] != 1 {
		t.Fatalf("refresh result = %#v", result)
	}
	status := manager.sourceStatus("nyse_official")
	if status.HealthState != "unhealthy" || status.LastAlertStatus != "triggered" {
		t.Fatalf("source status = %#v", status)
	}
	if len(alerts) != 1 || alerts[0].Kind != "structure_changed" {
		t.Fatalf("alerts = %#v", alerts)
	}
}

func TestManagerSourceAlertsDeduplicateAndRecover(t *testing.T) {
	registry := NewSourceRegistry()
	callCount := 0
	registry.Register(stubSource{
		id:        "hk_gov_1823_ical",
		kind:      "official_ical",
		authority: "GovHK 1823",
		markets:   []string{"HK"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			callCount++
			if callCount < 3 {
				return marketcalendar.CalendarSnapshot{}, errors.New("temporary fetch failure")
			}
			return marketcalendar.CalendarSnapshot{
				MarketCode: "HK",
				SourceID:   "hk_gov_1823_ical",
				Schedules: []marketcalendar.TradingDaySchedule{
					{
						MarketCode: "HK",
						Date:       time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
						Status:     marketcalendar.TradingDayClosed,
					},
				},
				FetchedAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				ValidUntil: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"HK"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "HK",
				PreferredSourceIDs: []string{"hk_gov_1823_ical"},
				EnabledSourceIDs:   []string{"hk_gov_1823_ical"},
				FallbackToBuiltin:  true,
			},
		},
	}
	var alerts []SourceAlert
	manager := NewManager(
		nil,
		func() jfsettings.ExchangeCalendarSettings { return settings },
		WithRegistry(registry),
		WithAlertSink(func(alert SourceAlert) {
			alerts = append(alerts, alert)
		}),
		WithClock(func() time.Time {
			return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
		}),
	)

	manager.ProbeAll(context.Background())
	manager.ProbeAll(context.Background())
	manager.ProbeAll(context.Background())

	if len(alerts) != 2 {
		t.Fatalf("alerts = %#v", alerts)
	}
	if alerts[0].Kind != "fetch_failed" || alerts[1].Kind != "recovered" {
		t.Fatalf("alerts = %#v", alerts)
	}
	status := manager.sourceStatus("hk_gov_1823_ical")
	if status.HealthState != "healthy" || status.LastAlertStatus != "recovered" {
		t.Fatalf("source status = %#v", status)
	}
}

func TestManagerSourceAlertsDeduplicateNetworkTimeoutVariants(t *testing.T) {
	registry := NewSourceRegistry()
	errorsByCall := []error{
		context.DeadlineExceeded,
		context.Canceled,
		errors.New(`Get "https://www.nyse.com/trade/hours-calendars": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`),
	}
	callCount := 0
	registry.Register(stubSource{
		id:        "nyse_official",
		kind:      "official_html",
		authority: "NYSE",
		markets:   []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			err := errorsByCall[callCount%len(errorsByCall)]
			callCount++
			return marketcalendar.CalendarSnapshot{}, err
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "US",
				PreferredSourceIDs: []string{"nyse_official"},
				EnabledSourceIDs:   []string{"nyse_official"},
				FallbackToBuiltin:  true,
			},
		},
	}
	var alerts []SourceAlert
	manager := NewManager(
		nil,
		func() jfsettings.ExchangeCalendarSettings { return settings },
		WithRegistry(registry),
		WithAlertSink(func(alert SourceAlert) {
			alerts = append(alerts, alert)
		}),
		WithClock(func() time.Time {
			return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
		}),
	)

	manager.RefreshAll(context.Background())
	manager.RefreshAll(context.Background())
	manager.RefreshAll(context.Background())

	if len(alerts) != 1 {
		t.Fatalf("alerts = %#v, want one network timeout alert", alerts)
	}
	if got, want := alerts[0].Fingerprint, "nyse_official|US|fetch_failed|network_timeout_or_cancelled"; got != want {
		t.Fatalf("fingerprint = %q, want %q", got, want)
	}
}

func TestManagerProbeRecoveryClearsCurrentFetchError(t *testing.T) {
	registry := NewSourceRegistry()
	callCount := 0
	registry.Register(stubSource{
		id:        "nyse_official",
		kind:      "official_html",
		authority: "NYSE",
		markets:   []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			callCount++
			if callCount == 1 {
				return marketcalendar.CalendarSnapshot{}, errors.New("context deadline exceeded")
			}
			return marketcalendar.CalendarSnapshot{
				MarketCode: "US",
				SourceID:   "nyse_official",
				Schedules: []marketcalendar.TradingDaySchedule{
					{
						MarketCode: "US",
						Date:       time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
						Status:     marketcalendar.TradingDayClosed,
					},
				},
				FetchedAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				ValidUntil: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "US",
				PreferredSourceIDs: []string{"nyse_official"},
				EnabledSourceIDs:   []string{"nyse_official"},
				FallbackToBuiltin:  true,
			},
		},
	}
	manager := NewManager(
		nil,
		func() jfsettings.ExchangeCalendarSettings { return settings },
		WithRegistry(registry),
		WithClock(func() time.Time {
			return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
		}),
	)

	manager.RefreshAll(context.Background())
	failed := manager.sourceStatus("nyse_official")
	if failed.HealthState != "unhealthy" || failed.LastError == "" || failed.NextRefreshAt.IsZero() {
		t.Fatalf("failed source status = %#v", failed)
	}

	manager.ProbeAll(context.Background())
	recovered := manager.sourceStatus("nyse_official")
	if recovered.HealthState != "healthy" || recovered.LastError != "" || recovered.LastProbeError != "" {
		t.Fatalf("recovered source status = %#v", recovered)
	}
	if recovered.ConsecutiveFailures != 0 || !recovered.NextRefreshAt.IsZero() {
		t.Fatalf("recovered retry state = %#v", recovered)
	}
}

func TestSourceRegistryHonorsPreferredSourceOrder(t *testing.T) {
	registry := NewSourceRegistry()
	registry.Register(stubSource{id: "b", markets: []string{"US"}})
	registry.Register(stubSource{id: "a", markets: []string{"US"}})

	ordered := registry.OrderedSources("US", jfsettings.ExchangeCalendarSourcePolicy{
		PreferredSourceIDs: []string{"a"},
		EnabledSourceIDs:   []string{"a", "b"},
	})
	got := []string{ordered[0].ID(), ordered[1].ID()}
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("ordered = %#v", got)
	}
}

func TestManagerSourcesExposeAvailabilityNotes(t *testing.T) {
	manager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings {
		return jfsettings.ExchangeCalendarSettings{
			AutoRefreshEnabled:   false,
			RefreshIntervalHours: 24,
			WarmupMarkets:        []string{"HK", "CN"},
		}
	})

	rows := manager.Sources()
	notes := map[string]string{}
	for _, row := range rows {
		id := jftradeCheckedTypeAssertion[string](row["id"])
		note := jftradeCheckedTypeAssertion[string](row["availabilityNote"])
		notes[id] = note
	}

	if notes["hk_gov_1823_ical"] == "" {
		t.Fatalf("missing HK availability note: %#v", notes)
	}
	if notes["mainland_official_notice"] == "" {
		t.Fatalf("missing mainland availability note: %#v", notes)
	}
	if notes[BuiltinSourceID] == "" || notes[ManualOverrideSourceID] == "" {
		t.Fatalf("missing builtin/manual notes: %#v", notes)
	}
}

func TestManagerStatusExplainsBuiltinEffectiveReason(t *testing.T) {
	manager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings {
		return jfsettings.ExchangeCalendarSettings{
			AutoRefreshEnabled:   false,
			RefreshIntervalHours: 24,
			WarmupMarkets:        []string{"CN"},
			SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
				{
					Market:            "CN",
					EnabledSourceIDs:  []string{"builtin_rules"},
					FallbackToBuiltin: true,
				},
			},
		}
	}, WithClock(func() time.Time {
		return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	}))

	status := manager.Status()
	markets, ok := status["markets"].([]map[string]any)
	if !ok || len(markets) != 1 {
		t.Fatalf("markets = %#v", status["markets"])
	}
	if markets[0]["effectiveSource"] != BuiltinSourceID {
		t.Fatalf("effectiveSource = %#v", markets[0]["effectiveSource"])
	}
	reason := jftradeCheckedTypeAssertion[string](markets[0]["effectiveReason"])
	if reason == "" || reason != "current policy uses builtin_rules because no external source is enabled for this market" {
		t.Fatalf("effectiveReason = %q", reason)
	}
}

func TestManagerStatusUsesRemoteCoverageSourceForRegularDay(t *testing.T) {
	registry := NewSourceRegistry()
	loc := time.FixedZone("HKT", 8*3600)
	registry.Register(stubSource{
		id:        "hk_gov_1823_ical",
		kind:      "official_ical",
		authority: "GovHK 1823",
		markets:   []string{"HK"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{
				MarketCode: "HK",
				SourceID:   "hk_gov_1823_ical",
				From:       time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
				To:         time.Date(2027, 12, 31, 23, 59, 59, 0, loc),
				Schedules: []marketcalendar.TradingDaySchedule{
					{
						MarketCode: "HK",
						Date:       time.Date(2026, 6, 19, 0, 0, 0, 0, loc),
						Status:     marketcalendar.TradingDayClosed,
						Reason:     "tuen_ng_festival",
					},
				},
				FetchedAt:  time.Date(2026, 6, 19, 17, 10, 34, 0, time.UTC),
				ValidUntil: time.Date(2026, 7, 19, 17, 10, 34, 0, time.UTC),
			}, nil
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"HK"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "HK",
				PreferredSourceIDs: []string{"hk_gov_1823_ical"},
				EnabledSourceIDs:   []string{"hk_gov_1823_ical"},
				FallbackToBuiltin:  true,
				StaleAfterHours:    168,
			},
		},
	}
	manager := NewManager(calendarstore.New(t.TempDir()), func() jfsettings.ExchangeCalendarSettings {
		return settings
	}, WithRegistry(registry), WithClock(func() time.Time {
		return time.Date(2026, 6, 19, 17, 19, 42, 0, time.UTC)
	}))

	manager.RefreshAll(context.Background())
	status := manager.Status()
	markets, ok := status["markets"].([]map[string]any)
	if !ok || len(markets) != 1 {
		t.Fatalf("markets = %#v", status["markets"])
	}
	if markets[0]["effectiveSource"] != "hk_gov_1823_ical" {
		t.Fatalf("effectiveSource = %#v", markets[0]["effectiveSource"])
	}
	if markets[0]["effectiveMode"] != "remote_covered_day" {
		t.Fatalf("effectiveMode = %#v", markets[0]["effectiveMode"])
	}
	reason := jftradeCheckedTypeAssertion[string](markets[0]["effectiveReason"])
	if reason != "a fresh source snapshot covers the checked trading day; builtin template supplies the standard session result because that date has no special override" {
		t.Fatalf("effectiveReason = %q", reason)
	}
}

func TestSnapshotCacheKeyUsesMarketLocalYear(t *testing.T) {
	manager := NewManager(nil, nil)

	hongKongNewYear := time.Date(2025, time.December, 31, 16, 30, 0, 0, time.UTC)
	if got, want := manager.snapshotCacheKey("source", "HK", hongKongNewYear), "source|HK|2026"; got != want {
		t.Fatalf("HK snapshot cache key = %q, want %q", got, want)
	}

	newYorkPreviousYear := time.Date(2026, time.January, 1, 2, 0, 0, 0, time.UTC)
	if got, want := manager.snapshotCacheKey("source", "US", newYorkPreviousYear), "source|US|2025"; got != want {
		t.Fatalf("US snapshot cache key = %q, want %q", got, want)
	}
}

func TestManagerCurrentTimeNormalizesInjectedClockToUTC(t *testing.T) {
	local := time.FixedZone("injected", 8*60*60)
	manager := NewManager(nil, nil, WithClock(func() time.Time {
		return time.Date(2026, time.June, 20, 9, 30, 0, 0, local)
	}))

	got := manager.currentTime()
	want := time.Date(2026, time.June, 20, 1, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("currentTime() = %s, want %s", got, want)
	}
	if got.Location() != time.UTC {
		t.Fatalf("currentTime() location = %s, want UTC", got.Location())
	}
}
