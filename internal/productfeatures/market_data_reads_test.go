package productfeatures

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestWorkspaceMarketDataReadsPreserveExplicitProviderAndResponseShape(t *testing.T) {
	open := 100.0
	high := 102.0
	low := 99.0
	closePrice := 101.5
	volume := 1234.0
	reader := &featureMarketDataReader{
		snapshot: &broker.KLineSnapshot{
			Symbol:        "US.AAPL",
			Period:        "5m",
			ExtendedHours: true,
			Session:       "all",
			Pagination: broker.KLinePagination{
				HasMore:    true,
				NextBefore: "2026-07-18T13:35:00Z",
			},
			KLines: []broker.KLineItem{{
				Time:   "2026-07-18T13:35:00Z",
				Open:   &open,
				High:   &high,
				Low:    &low,
				Close:  &closePrice,
				Volume: &volume,
			}},
		},
	}
	adapter := &featureBroker{
		id: "alpha",
		features: []broker.FeatureID{
			broker.FeatureMarketSnapshot,
			broker.FeatureMarketSnapshots,
			broker.FeatureInstrumentProfile,
			broker.FeatureMarketCandles,
			broker.FeatureMarketDepth,
		},
		marketData: reader,
	}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	service := NewService(registry, "alpha", nil, nil)
	service.now = func() time.Time {
		return time.Date(2026, 7, 18, 13, 36, 0, 0, time.UTC)
	}

	snapshotResult, err := service.ReadMarketSnapshot(
		t.Context(), "alpha", "us", "aapl", true,
	)
	if err != nil {
		t.Fatalf("ReadMarketSnapshot: %v", err)
	}
	assertWorkspaceProviderMeta(t, snapshotResult, "alpha", "US.AAPL")
	snapshot := snapshotResult["snapshot"].(map[string]any)
	if snapshot["price"] != 215.5 {
		t.Fatalf("snapshot = %#v", snapshot)
	}

	securityResult, err := service.ReadMarketSecurityDetails(
		t.Context(), "alpha", "US", "AAPL",
	)
	if err != nil {
		t.Fatalf("ReadMarketSecurityDetails: %v", err)
	}
	assertWorkspaceProviderMeta(t, securityResult, "alpha", "US.AAPL")

	candleResult, err := service.ReadMarketCandles(
		t.Context(), "alpha", "US", "AAPL", "5m", 20, "", "", "2026-07-18T13:40:00Z",
	)
	if err != nil {
		t.Fatalf("ReadMarketCandles: %v", err)
	}
	assertWorkspaceProviderMeta(t, candleResult, "alpha", "US.AAPL")
	candles := candleResult["candles"].([]map[string]any)
	if len(candles) != 1 || candles[0]["period"] != "5m" ||
		candles[0]["at"] != "2026-07-18T13:35:00Z" {
		t.Fatalf("candles = %#v", candles)
	}
	if reader.query.BrokerID != "alpha" || reader.query.Symbol != "US.AAPL" {
		t.Fatalf("broker candle query = %#v", reader.query)
	}
	if reader.query.BeforeTime != "2026-07-18T13:40:00Z" {
		t.Fatalf("before cursor = %q", reader.query.BeforeTime)
	}
	pagination := candleResult["pagination"].(map[string]any)
	if pagination["hasMore"] != true || pagination["nextBefore"] != "2026-07-18T13:35:00Z" {
		t.Fatalf("pagination = %#v", pagination)
	}
	meta := candleResult["meta"].(map[string]any)
	if meta["extendedHours"] != true || meta["session"] != "all" {
		t.Fatalf("candle meta = %#v", meta)
	}

	depthResult, err := service.ReadMarketDepth(
		t.Context(), "alpha", "US", "AAPL", 12,
	)
	if err != nil {
		t.Fatalf("ReadMarketDepth: %v", err)
	}
	assertWorkspaceProviderMeta(t, depthResult, "alpha", "US.AAPL")
	depth := depthResult["depth"].(map[string]any)
	if depth["symbol"] != "US.AAPL" {
		t.Fatalf("depth = %#v", depth)
	}
}

func TestWorkspaceMarketDataReadsRejectInvalidInstrument(t *testing.T) {
	service := NewService(broker.NewRegistry(), "", nil, nil)
	if _, err := service.ReadMarketSnapshot(
		t.Context(), "alpha", "", "", false,
	); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("ReadMarketSnapshot error = %v, want ErrInvalidQuery", err)
	}
	if _, err := service.ReadMarketSecurityDetails(t.Context(), "alpha", "", ""); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("ReadMarketSecurityDetails error = %v, want ErrInvalidQuery", err)
	}
	if _, err := service.ReadMarketCandles(t.Context(), "alpha", "", "", "1m", 10, "", "", ""); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("ReadMarketCandles error = %v, want ErrInvalidQuery", err)
	}
	if _, err := service.ReadMarketDepth(t.Context(), "alpha", "", "", 10); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("ReadMarketDepth error = %v, want ErrInvalidQuery", err)
	}
}

func TestWorkspaceMarketDataReadsSurfaceProviderFailuresAndNormalizeFallbacks(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	reader := &featureMarketDataReader{err: providerErr}
	adapter := &workspaceBoundaryBroker{
		featureBroker: &featureBroker{
			id: "workspace-errors",
			features: []broker.FeatureID{
				broker.FeatureMarketSnapshot,
				broker.FeatureMarketSnapshots,
				broker.FeatureInstrumentProfile,
				broker.FeatureMarketCandles,
				broker.FeatureMarketDepth,
			},
			snapshotErr: providerErr,
			marketData:  reader,
		},
		err: providerErr,
	}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	service := NewService(registry, adapter.id, nil, nil)

	if _, err := service.ReadMarketSnapshot(t.Context(), adapter.id, "US", "AAPL", true); !errors.Is(err, providerErr) {
		t.Fatalf("ReadMarketSnapshot provider error = %v", err)
	}
	if _, err := service.ReadMarketSecurityDetails(t.Context(), adapter.id, "US", "AAPL"); !errors.Is(err, providerErr) {
		t.Fatalf("ReadMarketSecurityDetails provider error = %v", err)
	}
	if _, err := service.ReadMarketCandles(t.Context(), adapter.id, "US", "AAPL", "1m", 10, "", "", ""); !errors.Is(err, providerErr) {
		t.Fatalf("ReadMarketCandles provider error = %v", err)
	}
	if _, err := service.ReadMarketDepth(t.Context(), adapter.id, "US", "AAPL", 10); !errors.Is(err, providerErr) {
		t.Fatalf("ReadMarketDepth provider error = %v", err)
	}

	adapter.err = nil
	adapter.result = &broker.FeatureResult{Entries: []map[string]any{{
		"symbol": "US.AAPL",
		"bids":   []any{map[string]any{"price": 100.0}},
		"asks":   []any{map[string]any{"price": 101.0}},
	}}}
	depthResult, err := service.ReadMarketDepth(t.Context(), adapter.id, "", "us.aapl", 10)
	if err != nil || depthResult["depth"].(map[string]any)["symbol"] != "US.AAPL" {
		t.Fatalf("qualified depth result = %#v, %v", depthResult, err)
	}
	securityResult, err := service.ReadMarketSecurityDetails(t.Context(), adapter.id, "", "us.aapl")
	if err != nil || securityResult["security"].(map[string]any)["symbol"] != "US.AAPL" {
		t.Fatalf("qualified security result = %#v, %v", securityResult, err)
	}

	if workspaceSnapshot(nil, time.Time{}) != nil {
		t.Fatal("nil workspace snapshot returned a value")
	}
	updated := workspaceSnapshot(map[string]any{"updateTime": "2026-07-18T12:00:00Z"}, time.Time{})
	if updated["observedAt"] != "2026-07-18T12:00:00Z" {
		t.Fatalf("update-time workspace snapshot = %#v", updated)
	}
	fallback := time.Date(2026, 7, 18, 12, 1, 0, 0, time.UTC)
	withFallback := workspaceSnapshot(map[string]any{}, fallback)
	if withFallback["observedAt"] != fallback.Format(time.RFC3339Nano) {
		t.Fatalf("fallback-time workspace snapshot = %#v", withFallback)
	}
}

func TestWorkspaceSnapshotRestoresRegularCloseComparisonSemantics(t *testing.T) {
	tests := []struct {
		name              string
		entry             map[string]any
		wantPreviousClose any
		wantLastClose     any
	}{
		{
			name: "US regular compares latest price with prior close",
			entry: map[string]any{
				"symbol": "US.AAPL", "session": "regular",
				"lastPrice": 195.50, "previousClose": 193.20,
			},
			wantPreviousClose: 193.20,
			wantLastClose:     193.20,
		},
		{
			name: "US after hours compares recent regular close with prior close",
			entry: map[string]any{
				"symbol": "US.AAPL", "session": "after",
				"lastPrice": 195.50, "previousClose": 193.20,
			},
			wantPreviousClose: 195.50,
			wantLastClose:     193.20,
		},
		{
			name: "non-US unknown session keeps raw prior close",
			entry: map[string]any{
				"symbol": "SZ.000858", "session": "unknown",
				"lastPrice": 74.10, "previousClose": 72.76,
			},
			wantPreviousClose: 72.76,
			wantLastClose:     72.76,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := workspaceSnapshot(tt.entry, time.Time{})
			if snapshot["previousClosePrice"] != tt.wantPreviousClose ||
				snapshot["lastClosePrice"] != tt.wantLastClose {
				t.Fatalf("workspace snapshot closes = %#v", snapshot)
			}
		})
	}
}

type workspaceBoundaryBroker struct {
	*featureBroker
	err    error
	result *broker.FeatureResult
}

func (b *workspaceBoundaryBroker) QueryInstrumentProfile(
	context.Context,
	broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.result, b.err
}

func (b *workspaceBoundaryBroker) QueryMarketMicrostructure(
	context.Context,
	broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.result, b.err
}

func assertWorkspaceProviderMeta(
	t *testing.T,
	result map[string]any,
	brokerID string,
	instrumentID string,
) {
	t.Helper()
	meta := result["meta"].(map[string]any)
	if meta["brokerId"] != brokerID || meta["source"] != brokerID ||
		meta["instrumentId"] != instrumentID || meta["resolvedAt"] == "" {
		t.Fatalf("provider meta = %#v", meta)
	}
}
