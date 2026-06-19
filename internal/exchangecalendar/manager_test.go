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

type stubSource struct {
	id        string
	kind      string
	authority string
	markets   []string
	fetch     func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error)
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
		id, _ := row["id"].(string)
		note, _ := row["availabilityNote"].(string)
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
	reason, _ := markets[0]["effectiveReason"].(string)
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
	reason, _ := markets[0]["effectiveReason"].(string)
	if reason != "a fresh source snapshot covers the checked trading day; builtin template supplies the standard session result because that date has no special override" {
		t.Fatalf("effectiveReason = %q", reason)
	}
}
