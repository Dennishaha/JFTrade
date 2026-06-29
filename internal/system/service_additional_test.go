package system

import (
	"context"
	"testing"
)

func TestStatusIncludesInjectedObservabilitySummaries(t *testing.T) {
	svc := NewService(
		WithLiveStats(func() map[string]any {
			return map[string]any{"connections": 3}
		}),
		WithMarketdataRuntimeSummary(func() map[string]any {
			return map[string]any{"workers": 2, "status": "running"}
		}),
		WithExchangeCalendarStatus(func() map[string]any {
			return map[string]any{"healthy": true, "sources": 4}
		}),
	)

	status := svc.Status()
	observability, ok := status["observability"].(map[string]any)
	if !ok {
		t.Fatalf("observability = %#v, want map", status["observability"])
	}
	live, ok := observability["live"].(map[string]any)
	if !ok || live["connections"] != 3 {
		t.Fatalf("live = %#v", observability["live"])
	}
	marketdata, ok := observability["marketdata"].(map[string]any)
	if !ok || marketdata["workers"] != 2 || marketdata["status"] != "running" {
		t.Fatalf("marketdata = %#v", observability["marketdata"])
	}
	calendars, ok := observability["exchangeCalendars"].(map[string]any)
	if !ok || calendars["healthy"] != true || calendars["sources"] != 4 {
		t.Fatalf("exchangeCalendars = %#v", observability["exchangeCalendars"])
	}
}

func TestExchangeCalendarDelegatesAndFallbacks(t *testing.T) {
	svc := NewService()
	if got := svc.ExchangeCalendarStatus(); len(got) != 0 {
		t.Fatalf("ExchangeCalendarStatus default = %#v, want empty map", got)
	}
	if got := svc.ExchangeCalendarSources(); got != nil {
		t.Fatalf("ExchangeCalendarSources default = %#v, want nil", got)
	}
	if got := svc.RefreshExchangeCalendars(context.Background(), "US"); got["accepted"] != false || got["reason"] != "exchange calendar manager not configured" {
		t.Fatalf("RefreshExchangeCalendars default = %#v", got)
	}
	if got := svc.ProbeExchangeCalendars(context.Background(), "HK"); got["accepted"] != false || got["reason"] != "exchange calendar probe not configured" {
		t.Fatalf("ProbeExchangeCalendars default = %#v", got)
	}

	var refreshedMarket string
	var probedMarket string
	delegated := NewService(
		WithExchangeCalendarStatus(func() map[string]any {
			return map[string]any{"state": "ready"}
		}),
		WithExchangeCalendarSources(func() []map[string]any {
			return []map[string]any{{"id": "nyse_official"}, {"id": "builtin_rules"}}
		}),
		WithRefreshExchangeCalendars(func(ctx context.Context, market string) map[string]any {
			refreshedMarket = market
			return map[string]any{"accepted": true, "market": market, "ctx": ctx != nil}
		}),
		WithProbeExchangeCalendars(func(ctx context.Context, market string) map[string]any {
			probedMarket = market
			return map[string]any{"accepted": true, "market": market, "ctx": ctx != nil}
		}),
	)

	if got := delegated.ExchangeCalendarStatus(); got["state"] != "ready" {
		t.Fatalf("ExchangeCalendarStatus delegated = %#v", got)
	}
	if got := delegated.ExchangeCalendarSources(); len(got) != 2 || got[0]["id"] != "nyse_official" {
		t.Fatalf("ExchangeCalendarSources delegated = %#v", got)
	}
	if got := delegated.RefreshExchangeCalendars(context.Background(), "US"); got["accepted"] != true || got["market"] != "US" || refreshedMarket != "US" {
		t.Fatalf("RefreshExchangeCalendars delegated = %#v, refreshedMarket = %q", got, refreshedMarket)
	}
	if got := delegated.ProbeExchangeCalendars(context.Background(), "HK"); got["accepted"] != true || got["market"] != "HK" || probedMarket != "HK" {
		t.Fatalf("ProbeExchangeCalendars delegated = %#v, probedMarket = %q", got, probedMarket)
	}
}

func TestStorageAndRealTradeDefaultsExposeFrontendShape(t *testing.T) {
	svc := NewService()

	storage := svc.StorageOverview()
	assertSystemEmptyAnySlice(t, storage, "pendingOutbox")
	assertSystemEmptyAnySlice(t, storage, "recentJobs")
	assertSystemEmptyAnySlice(t, storage, "recentAuditLogs")
	assertSystemEmptyAnySlice(t, storage, "recentExecutionCommands")

	hardStops := svc.RealTradeHardStops()
	if hardStops["allowsCancel"] != true {
		t.Fatalf("RealTradeHardStops = %#v", hardStops)
	}
	assertSystemEmptyAnySlice(t, hardStops, "entries")

	hardStopEvents := svc.RealTradeHardStopEvents()
	if hardStopEvents["realTradingEnabled"] != false || hardStopEvents["allowsCancel"] != true {
		t.Fatalf("RealTradeHardStopEvents = %#v", hardStopEvents)
	}
	assertSystemEmptyAnySlice(t, hardStopEvents, "entries")

	killSwitchEvents := svc.RealTradeKillSwitchEvents()
	if killSwitchEvents["killSwitchActive"] != false || killSwitchEvents["allowsCancel"] != true {
		t.Fatalf("RealTradeKillSwitchEvents = %#v", killSwitchEvents)
	}
	assertSystemEmptyAnySlice(t, killSwitchEvents, "entries")
}

func TestFutuDefaultsExposeEmptyGuideAndSnapshot(t *testing.T) {
	svc := NewService()
	if got := svc.FutuOpenDInstallGuide(); len(got) != 0 {
		t.Fatalf("FutuOpenDInstallGuide default = %#v, want empty map", got)
	}
	if got := svc.BrokerOrderUpdatesSnapshot(); len(got) != 0 {
		t.Fatalf("BrokerOrderUpdatesSnapshot default = %#v, want empty map", got)
	}
	dependencies := svc.RuntimeDependencies(context.Background())
	if dependencies["allRequiredSatisfied"] != true {
		t.Fatalf("RuntimeDependencies default = %#v, want satisfied", dependencies)
	}
}

func TestRuntimeDependenciesDelegates(t *testing.T) {
	called := false
	svc := NewService(
		WithRuntimeDependencies(func(ctx context.Context) map[string]any {
			called = ctx != nil
			return map[string]any{
				"checkedAt":            "2026-06-29T00:00:00Z",
				"allRequiredSatisfied": false,
				"dependencies":         []any{map[string]any{"id": "node"}},
			}
		}),
	)

	got := svc.RuntimeDependencies(context.Background())
	if !called || got["allRequiredSatisfied"] != false {
		t.Fatalf("RuntimeDependencies delegated = %#v called=%v", got, called)
	}
}
