package exchangecalendar

import (
	"context"
	"testing"
	"time"

	calendarstore "github.com/jftrade/jftrade-main/internal/store/exchangecalendar"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func TestManagerProbeMarketWarmupAndSnapshotOrdering(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	fetches := map[string]int{}
	registry := NewSourceRegistry()
	registry.Register(stubSource{
		id:        "nyse_official",
		kind:      "official_html",
		authority: "NYSE",
		markets:   []string{"US", "HK"},
		fetch: func(ctx context.Context, market string, from time.Time, to time.Time) (marketcalendar.CalendarSnapshot, error) {
			fetches[market]++
			if ctx.Err() != nil {
				return marketcalendar.CalendarSnapshot{}, ctx.Err()
			}
			return marketcalendar.CalendarSnapshot{
				MarketCode: market,
				SourceID:   "nyse_official",
				From:       from,
				To:         to,
				Schedules: []marketcalendar.TradingDaySchedule{{
					MarketCode: market,
					Date:       time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
					Status:     marketcalendar.TradingDayClosed,
					Reason:     "juneteenth",
				}},
				FetchedAt:  now,
				ValidUntil: now.Add(7 * 24 * time.Hour),
			}, nil
		},
	})
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   true,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"HK"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "US",
				PreferredSourceIDs: []string{"nyse_official"},
				EnabledSourceIDs:   []string{"nyse_official"},
				FallbackToBuiltin:  true,
				StaleAfterHours:    24,
			},
			{
				Market:             "HK",
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

	probe := manager.ProbeMarket(context.Background(), "US")
	if probe["market"] != "US" || probe["healthy"] != 1 || probe["failures"] != 0 {
		t.Fatalf("ProbeMarket result = %#v", probe)
	}
	if fetches["US"] != 1 || fetches["HK"] != 0 {
		t.Fatalf("probe fetches = %#v, want only US", fetches)
	}

	manager.refreshWarmup()
	if fetches["HK"] != 1 {
		t.Fatalf("warmup fetches = %#v, want HK warmed from settings", fetches)
	}

	manager.mu.Lock()
	manager.snapshots["z"] = marketcalendar.CalendarSnapshot{
		MarketCode: "US", SourceID: "z-source", From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Schedules: []marketcalendar.TradingDaySchedule{{Status: marketcalendar.TradingDayClosed}},
	}
	manager.snapshots["a"] = marketcalendar.CalendarSnapshot{
		MarketCode: "HK", SourceID: "a-source", From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Schedules: []marketcalendar.TradingDaySchedule{{Status: marketcalendar.TradingDayClosed}},
	}
	manager.snapshots["builtin"] = marketcalendar.CalendarSnapshot{MarketCode: "US", SourceID: BuiltinSourceID}
	manager.mu.Unlock()

	summaries := manager.snapshotSummaries()
	if len(summaries) < 2 {
		t.Fatalf("snapshot summaries = %#v", summaries)
	}
	if summaries[0]["market"] != "HK" {
		t.Fatalf("first sorted snapshot = %#v, want HK before US", summaries[0])
	}
	if got := snapshotSortKey(marketcalendar.CalendarSnapshot{MarketCode: " us ", SourceID: " source ", From: now}); got != "US|source|2026-06-19T12:00:00Z" {
		t.Fatalf("snapshotSortKey = %q", got)
	}
}
