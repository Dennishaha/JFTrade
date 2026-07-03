package exchangecalendar

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	calendarstore "github.com/jftrade/jftrade-main/internal/store/exchangecalendar"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func TestManagerBackgroundRefreshFollowsSettingsReload(t *testing.T) {
	now := time.Date(2026, time.July, 2, 16, 0, 0, 0, time.UTC)
	fetched := make(chan struct{}, 2)
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id: "reload-source", markets: []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			fetched <- struct{}{}
			return marketcalendar.CalendarSnapshot{
				SourceID: "reload-source", MarketCode: "US",
				From: now.AddDate(0, -1, 0), To: now.AddDate(1, 0, 0),
				Schedules: []marketcalendar.TradingDaySchedule{{
					MarketCode: "US", Date: now, Status: marketcalendar.TradingDayClosed, Reason: "reload test",
				}},
				FetchedAt: now, ValidUntil: now.Add(24 * time.Hour),
			}, nil
		},
	})

	var settingsMu sync.RWMutex
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled: false, RefreshIntervalHours: 24, WarmupMarkets: []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{{
			Market: "US", EnabledSourceIDs: []string{"reload-source"}, PreferredSourceIDs: []string{"reload-source"},
		}},
	}
	manager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings {
		settingsMu.RLock()
		defer settingsMu.RUnlock()
		return settings
	}, WithRegistry(registry), WithClock(func() time.Time { return now }))
	manager.Start()
	t.Cleanup(func() { _ = manager.Close() })

	select {
	case <-fetched:
		t.Fatal("background refresh ran while auto refresh was disabled")
	case <-time.After(50 * time.Millisecond):
	}

	settingsMu.Lock()
	settings.AutoRefreshEnabled = true
	settingsMu.Unlock()
	deadline := time.After(2 * time.Second)
	for {
		manager.NotifySettingsChanged()
		select {
		case <-fetched:
			goto refreshed
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			t.Fatal("settings reload did not trigger background calendar refresh")
		}
	}

refreshed:
	if schedule, ok := manager.Schedule("US", now); !ok || schedule.SourceID != "reload-source" || schedule.Reason != "reload test" {
		t.Fatalf("schedule after background refresh = %#v, %v", schedule, ok)
	}
}

func TestManagerRefreshKeepsValidSnapshotWhenPersistenceFails(t *testing.T) {
	now := time.Date(2026, time.July, 2, 16, 0, 0, 0, time.UTC)
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id: "ephemeral-source", markets: []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{
				From: now.AddDate(0, -1, 0), To: now.AddDate(1, 0, 0),
				Schedules: []marketcalendar.TradingDaySchedule{{MarketCode: "US", Date: now, Status: marketcalendar.TradingDayClosed, Reason: "remote emergency closure"}},
				FetchedAt: now, ValidUntil: now.Add(24 * time.Hour),
			}, nil
		},
	})
	rootFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(rootFile, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("prepare unavailable persistence root: %v", err)
	}
	settings := jfsettings.ExchangeCalendarSettings{
		WarmupMarkets: []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{{
			Market: "US", EnabledSourceIDs: []string{"ephemeral-source"}, PreferredSourceIDs: []string{"ephemeral-source"},
		}},
	}
	manager := NewManager(calendarstore.New(rootFile), func() jfsettings.ExchangeCalendarSettings { return settings }, WithRegistry(registry), WithClock(func() time.Time { return now }))

	result := manager.RefreshAll(context.Background())
	if result["updated"] != 0 || result["failures"] != 1 {
		t.Fatalf("refresh result = %#v", result)
	}
	schedule, ok := manager.Schedule("US", now)
	if !ok || schedule.SourceID != "ephemeral-source" || schedule.MarketCode != "US" || schedule.Reason != "remote emergency closure" {
		t.Fatalf("in-memory schedule after persistence failure = %#v, %v", schedule, ok)
	}
	status := manager.sourceStatus("ephemeral-source")
	if !strings.Contains(status.LastError, "create exchange calendar snapshot directory") {
		t.Fatalf("source persistence error = %q", status.LastError)
	}
}

func TestManagerRestoreReportsMalformedCachedSnapshot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "US", "2026", "broken.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"sourceId":`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	manager := NewManager(calendarstore.New(root), nil)
	status := manager.sourceStatus(BuiltinSourceID)
	if !strings.Contains(status.LastError, "decode") || !strings.Contains(status.LastError, "broken.json") {
		t.Fatalf("cached snapshot restore error = %q", status.LastError)
	}
}

func TestManagerStatusReportsManualAndRemoteOverrideModes(t *testing.T) {
	now := time.Date(2026, time.July, 2, 16, 0, 0, 0, time.UTC)
	manualSettings := jfsettings.ExchangeCalendarSettings{
		WarmupMarkets: []string{"US"},
		ManualOverrides: []jfsettings.ExchangeCalendarManualOverride{{
			Market: "US", Date: "2026-07-02", Status: "closed", Reason: "operator closure",
		}},
	}
	manualManager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings { return manualSettings }, WithClock(func() time.Time { return now }))
	assertCalendarEffectiveMode(t, manualManager.Status(), ManualOverrideSourceID, "manual_override")

	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id: "official-source", markets: []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{
				SourceID: "official-source", MarketCode: "US",
				From: now.AddDate(0, -1, 0), To: now.AddDate(1, 0, 0),
				Schedules: []marketcalendar.TradingDaySchedule{{MarketCode: "US", Date: now, Status: marketcalendar.TradingDayClosed}},
				FetchedAt: now, ValidUntil: now.Add(24 * time.Hour),
			}, nil
		},
	})
	remoteSettings := jfsettings.ExchangeCalendarSettings{
		WarmupMarkets: []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{{
			Market: "US", EnabledSourceIDs: []string{"official-source"}, PreferredSourceIDs: []string{"official-source"},
		}},
	}
	remoteManager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings { return remoteSettings }, WithRegistry(registry), WithClock(func() time.Time { return now }))
	remoteManager.RefreshAll(context.Background())
	assertCalendarEffectiveMode(t, remoteManager.Status(), "official-source", "remote_override")
}

func assertCalendarEffectiveMode(t *testing.T, status map[string]any, sourceID string, mode string) {
	t.Helper()
	markets, ok := status["markets"].([]map[string]any)
	if !ok || len(markets) != 1 {
		t.Fatalf("calendar markets = %#v", status["markets"])
	}
	if markets[0]["effectiveSource"] != sourceID || markets[0]["effectiveMode"] != mode {
		t.Fatalf("calendar effective row = %#v", markets[0])
	}
}
