package exchangecalendar

import (
	"context"
	"errors"
	"testing"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

// TestCoverage98RefreshAndProbeKeepPerSourceHealthTruthful exercises the
// operational path with three independently configured providers. A calendar
// refresh must retain the usable source while making both transport and parser
// failures visible to the operator; a probe must report the same distinction.
func TestCoverage98RefreshAndProbeKeepPerSourceHealthTruthful(t *testing.T) {
	now := time.Date(2026, time.July, 2, 16, 0, 0, 0, time.UTC)
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id: "coverage98-network-failure", markets: []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{}, errors.New("official endpoint unavailable")
		},
	})
	registry.Register(stubSource{
		id: "coverage98-structure-failure", markets: []string{"US"},
		fetch: func(_ context.Context, market string, from time.Time, to time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{
				SourceID: "coverage98-structure-failure", MarketCode: market, From: from, To: to,
				FetchedAt: now, ValidUntil: now.Add(24 * time.Hour),
			}, nil
		},
	})
	registry.Register(stubSource{
		id: "coverage98-official", markets: []string{"US"},
		fetch: func(_ context.Context, market string, from time.Time, to time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{
				SourceID: "coverage98-official", MarketCode: market, From: from, To: to,
				FetchedAt: now, ValidUntil: now.Add(24 * time.Hour), Checksum: "coverage98-checksum",
				Schedules: []marketcalendar.TradingDaySchedule{{
					MarketCode: market, Date: now, Status: marketcalendar.TradingDayClosed, Reason: "official test closure",
				}},
			}, nil
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		WarmupMarkets: []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{{
			Market: "US",
			PreferredSourceIDs: []string{
				"coverage98-network-failure", "coverage98-structure-failure", "coverage98-official",
			},
			EnabledSourceIDs: []string{
				"coverage98-network-failure", "coverage98-structure-failure", "coverage98-official",
			},
			FallbackToBuiltin: true,
		}},
	}
	manager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings { return settings }, WithRegistry(registry), WithClock(func() time.Time { return now }))

	refresh := manager.RefreshMarket(context.Background(), "US")
	if refresh["updated"] != 1 || refresh["failures"] != 2 {
		t.Fatalf("RefreshMarket must keep the usable source and count failures: %#v", refresh)
	}
	if status := manager.sourceStatus("coverage98-network-failure"); status.LastError == "" || status.ConsecutiveFailures != 1 {
		t.Fatalf("network source status = %#v", status)
	}
	if status := manager.sourceStatus("coverage98-structure-failure"); status.LastError == "" || status.ConsecutiveFailures != 1 {
		t.Fatalf("structure source status = %#v", status)
	}
	if status := manager.sourceStatus("coverage98-official"); status.LastSuccessAt.IsZero() || status.LastSnapshotFetchedAt.IsZero() {
		t.Fatalf("healthy source status = %#v", status)
	}
	if schedule, ok := manager.Schedule("US", now); !ok || schedule.SourceID != "coverage98-official" || schedule.Status != marketcalendar.TradingDayClosed {
		t.Fatalf("usable remote calendar was not served: %#v/%v", schedule, ok)
	}

	probe := manager.ProbeMarket(context.Background(), "US")
	if probe["healthy"] != 1 || probe["failures"] != 2 {
		t.Fatalf("ProbeMarket must retain provider-level health: %#v", probe)
	}
	results, ok := probe["results"].([]map[string]any)
	if !ok || len(results) != 3 {
		t.Fatalf("probe results = %#v", probe["results"])
	}

	// An unsupported operator target should be accepted as a no-op, not treated
	// as a successful remote refresh for an unrelated market.
	if result := manager.RefreshMarket(context.Background(), "MARS"); result["updated"] != 0 || result["failures"] != 0 || result["market"] != "MARS" {
		t.Fatalf("unsupported target refresh = %#v", result)
	}
	if result := manager.ProbeMarket(context.Background(), "MARS"); result["healthy"] != 0 || result["failures"] != 0 || result["market"] != "MARS" {
		t.Fatalf("unsupported target probe = %#v", result)
	}
}

func TestCoverage98StatusDistinguishesRemoteCoverageFromRemoteOverride(t *testing.T) {
	now := time.Date(2026, time.July, 6, 14, 0, 0, 0, time.UTC)
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id: "coverage98-covered", markets: []string{"US"},
		fetch: func(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
			return marketcalendar.CalendarSnapshot{}, nil
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		WarmupMarkets: []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{{
			Market: "US", EnabledSourceIDs: []string{"coverage98-covered"}, PreferredSourceIDs: []string{"coverage98-covered"},
			FallbackToBuiltin: true,
		}},
	}
	manager := NewManager(nil, func() jfsettings.ExchangeCalendarSettings { return settings }, WithRegistry(registry), WithClock(func() time.Time { return now }))
	manager.cacheSnapshot(marketcalendar.CalendarSnapshot{
		SourceID: "coverage98-covered", MarketCode: "US",
		From: now.AddDate(0, -1, 0), To: now.AddDate(0, 1, 0),
		FetchedAt: now.Add(-time.Hour), ValidUntil: now.Add(time.Hour),
		// The special schedule is deliberately on another covered day. The
		// source is fresh for today but does not override today's builtin result.
		Schedules: []marketcalendar.TradingDaySchedule{{
			MarketCode: "US", Date: now.AddDate(0, 0, 1), Status: marketcalendar.TradingDayEarlyClose,
		}},
	})

	status := manager.Status()
	markets, ok := status["markets"].([]map[string]any)
	if !ok || len(markets) != 1 || markets[0]["effectiveSource"] != "coverage98-covered" || markets[0]["effectiveMode"] != "remote_covered_day" {
		t.Fatalf("fresh source coverage status = %#v", status)
	}
}
