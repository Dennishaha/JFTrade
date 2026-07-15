package servercore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/shopspring/decimal"
)

func TestMarketSnapshotResponseUsesFreshCache(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	instrumentID := "HK.00700"
	now := time.Now().UTC().Truncate(time.Second)
	seedCachedTickSample(server, marketTickSample{
		InstrumentID:       instrumentID,
		Market:             "HK",
		Symbol:             "00700",
		Price:              decimal.RequireFromString("321.4"),
		Bid:                decimal.RequireFromString("321.3"),
		Ask:                decimal.RequireFromString("321.5"),
		PreviousClosePrice: decimalPointer(new(318.9)),
		Volume:             1282100,
		Turnover:           decimal.RequireFromString("411020000"),
		QuoteAt:            now.Format(time.RFC3339Nano),
		ObservedAt:         now.Format(time.RFC3339Nano),
		Source:             "bbgo:futu:stream",
		Session:            "regular",
	})

	response, err := server.marketSnapshotResponse(
		t.Context(),
		"/api/v1/market-data/snapshots/HK/00700",
		map[string][]string{},
	)
	if err != nil {
		t.Fatalf("marketSnapshotResponse: %v", err)
	}

	assertSnapshotResponse(t, response, instrumentID, true, "bbgo:futu:stream")
	if got := jftradeCheckedTypeAssertion[map[string]any](response["snapshot"])["at"]; got != now.Format(time.RFC3339Nano) {
		t.Fatalf("snapshot at = %v", got)
	}
	if got := jftradeCheckedTypeAssertion[map[string]any](response["snapshot"])["turnover"]; got != "411020000" {
		t.Fatalf("snapshot turnover = %v", got)
	}
}

func decimalPointer(v *float64) *decimal.Decimal {
	if v == nil {
		return nil
	}
	return new(decimal.NewFromFloat(*v))
}

func TestMarketSnapshotResponseQueriesQuoteSnapshotOnCacheMiss(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	acquireMarketDataTestSubscription(t, server, mdsrv.InstrumentRef{Channel: "SNAPSHOT", Market: "HK", Symbol: "00700"})
	response, err := server.marketSnapshotResponse(
		t.Context(),
		"/api/v1/market-data/snapshots/HK/00700",
		map[string][]string{},
	)
	if err != nil {
		t.Fatalf("marketSnapshotResponse: %v", err)
	}

	assertSnapshotResponse(t, response, "HK.00700", false, "bbgo:futu")
	if got := quoteServer.basicQotCallCount(); got != 1 {
		t.Fatalf("expected one GetBasicQot call, got %d", got)
	}
	if got := jftradeCheckedTypeAssertion[map[string]any](response["snapshot"])["price"]; got != "321.4" {
		t.Fatalf("snapshot price = %v", got)
	}
}

func TestMarketSnapshotResponseForceRefreshBypassesCache(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	acquireMarketDataTestSubscription(t, server, mdsrv.InstrumentRef{Channel: "SNAPSHOT", Market: "HK", Symbol: "00700"})
	seedCachedTickSample(server, marketTickSample{
		InstrumentID: "HK.00700",
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("999.9"),
		Bid:          decimal.RequireFromString("999.8"),
		Ask:          decimal.RequireFromString("1000.0"),
		Volume:       1,
		QuoteAt:      time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
		ObservedAt:   time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
		Source:       "bbgo:futu:stream",
		Session:      "regular",
	})

	response, err := server.marketSnapshotResponse(
		t.Context(),
		"/api/v1/market-data/snapshots/HK/00700",
		map[string][]string{"refresh": {"true"}},
	)
	if err != nil {
		t.Fatalf("marketSnapshotResponse: %v", err)
	}

	assertSnapshotResponse(t, response, "HK.00700", false, "bbgo:futu")
	if got := quoteServer.basicQotCallCount(); got != 1 {
		t.Fatalf("expected one forced GetBasicQot call, got %d", got)
	}
	if got := jftradeCheckedTypeAssertion[map[string]any](response["snapshot"])["price"]; got != "321.4" {
		t.Fatalf("forced refresh snapshot price = %v", got)
	}
}

func TestMarketCandlesTickResponseUsesFreshCache(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	instrumentID := "HK.00700"
	now := time.Now().UTC().Truncate(time.Second)
	seedCachedTickSample(server, marketTickSample{
		InstrumentID: instrumentID,
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("321.4"),
		Bid:          decimal.RequireFromString("321.3"),
		Ask:          decimal.RequireFromString("321.5"),
		Volume:       1282100,
		QuoteAt:      now.Format(time.RFC3339Nano),
		ObservedAt:   now.Format(time.RFC3339Nano),
		Source:       "bbgo:futu:stream",
		Session:      "regular",
	})

	response, err := server.marketCandlesResponse(
		t.Context(),
		"/api/v1/market-data/candles/HK/00700",
		map[string][]string{"period": {"tick"}, "limit": {"2"}},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse: %v", err)
	}

	assertTickCandlesResponse(t, response, instrumentID, true, 1)
	if got := jftradeCheckedTypeAssertion[[]map[string]any](response["candles"])[0]["at"]; got != now.Format(time.RFC3339Nano) {
		t.Fatalf("tick candle at = %v", got)
	}
}

func TestMarketCandlesTickResponseQueriesTickerOnCacheMiss(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	acquireMarketDataTestSubscription(t, server, mdsrv.InstrumentRef{Channel: "TICK", Market: "HK", Symbol: "00700"})
	response, err := server.marketCandlesResponse(
		t.Context(),
		"/api/v1/market-data/candles/HK/00700",
		map[string][]string{"period": {"tick"}, "limit": {"2"}},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse: %v", err)
	}

	assertTickCandlesResponse(t, response, "HK.00700", false, 1)
	if got := quoteServer.basicQotCallCount(); got != 1 {
		t.Fatalf("expected one GetBasicQot call, got %d", got)
	}
	if got := jftradeCheckedTypeAssertion[[]map[string]any](response["candles"])[0]["period"]; got != "tick" {
		t.Fatalf("tick candle period = %v", got)
	}
}

func acquireMarketDataTestSubscription(t *testing.T, server *Server, ref mdsrv.InstrumentRef) {
	t.Helper()
	_, err := server.marketdataSvc.AcquireSubscription(t.Context(), "market-snapshot-test", []mdsrv.InstrumentRef{ref})
	if err != nil {
		t.Fatalf("AcquireSubscription(%#v): %v", ref, err)
	}
	if err := server.marketdataRuntime.ReconcileSubscriptions(t.Context(), []mdsrv.InstrumentRef{ref}); err != nil {
		t.Fatalf("ReconcileSubscriptions(%#v): %v", ref, err)
	}
	if state := server.marketdataRuntime.SubscriptionState(); state["ownActiveCount"] != 1 {
		t.Fatalf("ReconcileSubscriptions(%#v) did not establish the physical lease: %#v", ref, state)
	}
}

func TestMarketCandlesTickResponseFallsBackToCachedCandlesOnTickerError(t *testing.T) {
	server := newMarketDataTestServerWithQuoteRuntime(t, "127.0.0.1:1")

	instrumentID := "HK.00700"
	observedAt := time.Now().UTC().Add(-1 * time.Minute).Truncate(time.Second)
	seedCachedTickSample(server, marketTickSample{
		InstrumentID: instrumentID,
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("321.4"),
		Bid:          decimal.RequireFromString("321.3"),
		Ask:          decimal.RequireFromString("321.5"),
		Volume:       1282100,
		QuoteAt:      observedAt.Format(time.RFC3339Nano),
		ObservedAt:   observedAt.Format(time.RFC3339Nano),
		Source:       "bbgo:futu:fallback",
		Session:      "regular",
	})

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	response, err := server.marketCandlesResponse(
		ctx,
		"/api/v1/market-data/candles/HK/00700",
		map[string][]string{"period": {"tick"}, "limit": {"2"}},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse fallback: %v", err)
	}

	assertTickCandlesResponse(t, response, instrumentID, true, 1)
	if got := jftradeCheckedTypeAssertion[[]map[string]any](response["candles"])[0]["at"]; got != observedAt.Format(time.RFC3339Nano) {
		t.Fatalf("fallback tick candle at = %v", got)
	}
}
