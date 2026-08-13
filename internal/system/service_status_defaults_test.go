package system

import (
	"context"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/observability"
)

func TestStatusIncludesInjectedObservabilitySummaries(t *testing.T) {
	svc := NewService(
		WithLiveStats(func() *LiveStats {
			return &LiveStats{Connected: 3}
		}),
		WithMarketdataRuntimeSummary(func() *MarketDataRuntime {
			return &MarketDataRuntime{ActiveCount: 2, Status: "running"}
		}),
		WithExchangeCalendarStatus(func() *CalendarStatus {
			return &CalendarStatus{AutoRefreshEnabled: true, Sources: []CalendarSource{{ID: "builtin"}}}
		}),
		WithRequestObservability(func() observability.Snapshot {
			return observability.Snapshot{RecentErrors: []observability.Event{{Message: "failure"}}, SlowThresholdMS: 750}
		}),
	)

	status := svc.Status()
	if status.Observability.Live == nil || status.Observability.Live.Connected != 3 {
		t.Fatalf("live = %#v", status.Observability.Live)
	}
	if status.Observability.MarketData == nil || status.Observability.MarketData.ActiveCount != 2 || status.Observability.MarketData.Status != "running" {
		t.Fatalf("marketdata = %#v", status.Observability.MarketData)
	}
	if calendars := status.Observability.ExchangeCalendars; calendars == nil || !calendars.AutoRefreshEnabled || len(calendars.Sources) != 1 {
		t.Fatalf("exchangeCalendars = %#v", calendars)
	}
	if requests := status.Observability.Requests; requests.SlowThresholdMS != 750 || len(requests.RecentErrors) != 1 {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestStatusProvidesDefaultRequestObservabilitySummary(t *testing.T) {
	status := NewService().Status()
	requests := status.Observability.Requests
	if got := requests.SlowThresholdMS; got != 750 {
		t.Fatalf("slowThresholdMs = %#v, want 750", got)
	}
	if got := requests.MinimumImportance; got != "low" {
		t.Fatalf("minimumImportance = %#v, want low", got)
	}
	if requests.RecentErrors == nil || len(requests.RecentErrors) != 0 {
		t.Fatalf("recentErrors = %#v, want empty slice", requests.RecentErrors)
	}
	if requests.RecentSlowRequests == nil || len(requests.RecentSlowRequests) != 0 {
		t.Fatalf("recentSlowRequests = %#v, want empty slice", requests.RecentSlowRequests)
	}
	if requests.OpenD.TotalCalls != 0 || requests.OpenD.FailedCalls != 0 {
		t.Fatalf("openD = %#v, want zero call counters", requests.OpenD)
	}
}

func TestExchangeCalendarDelegatesAndFallbacks(t *testing.T) {
	svc := NewService()
	if got := svc.ExchangeCalendarStatus(); got.AutoRefreshEnabled || got.Markets != nil || got.Sources != nil {
		t.Fatalf("ExchangeCalendarStatus default = %#v, want zero value", got)
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
		WithExchangeCalendarStatus(func() *CalendarStatus {
			return &CalendarStatus{AutoRefreshEnabled: true}
		}),
		WithExchangeCalendarSources(func() []CalendarSource {
			return []CalendarSource{{ID: "nyse_official"}, {ID: "builtin_rules"}}
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

	if got := delegated.ExchangeCalendarStatus(); !got.AutoRefreshEnabled {
		t.Fatalf("ExchangeCalendarStatus delegated = %#v", got)
	}
	if got := delegated.ExchangeCalendarSources(); len(got) != 2 || got[0].ID != "nyse_official" {
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

	hardStops := realTradeJSONMap(t, svc.RealTradeHardStops())
	if hardStops["allowsCancel"] != true {
		t.Fatalf("RealTradeHardStops = %#v", hardStops)
	}
	assertSystemEmptyAnySlice(t, hardStops, "entries")

	hardStopEvents := realTradeJSONMap(t, svc.RealTradeHardStopEvents())
	if hardStopEvents["realTradingEnabled"] != false || hardStopEvents["allowsCancel"] != true {
		t.Fatalf("RealTradeHardStopEvents = %#v", hardStopEvents)
	}
	assertSystemEmptyAnySlice(t, hardStopEvents, "entries")

	killSwitchEvents := realTradeJSONMap(t, svc.RealTradeKillSwitchEvents())
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
