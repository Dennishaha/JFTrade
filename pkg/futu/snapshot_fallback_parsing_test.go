package futu

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestStockScreenFallbackWireValueHelpers(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
		want  float64
		ok    bool
	}{
		{name: "float64", value: float64(1.5), want: 1.5, ok: true},
		{name: "float32", value: float32(2.5), want: 2.5, ok: true},
		{name: "int", value: int(3), want: 3, ok: true},
		{name: "int32", value: int32(4), want: 4, ok: true},
		{name: "int64", value: int64(5), want: 5, ok: true},
		{name: "uint64", value: uint64(6), want: 6, ok: true},
		{name: "string", value: " 7.5 ", want: 7.5, ok: true},
		{name: "nan", value: math.NaN()},
		{name: "bad string", value: "not-a-number"},
		{name: "unsupported", value: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := stockScreenFloat(test.value)
			if ok != test.ok || (ok && got != test.want) {
				t.Fatalf("stockScreenFloat(%#v) = %v, %t", test.value, got, ok)
			}
		})
	}
	for _, test := range []struct {
		value any
		want  uint64
		ok    bool
	}{
		{value: " 12 ", want: 12, ok: true},
		{value: float64(13), want: 13, ok: true},
		{value: int64(-1)},
		{value: float64(1.5)},
		{value: math.Inf(1)},
		{value: "bad"},
	} {
		got, ok := stockScreenUint64(test.value)
		if ok != test.ok || (ok && got != test.want) {
			t.Fatalf("stockScreenUint64(%#v) = %d, %t", test.value, got, ok)
		}
	}
	for _, test := range []struct {
		value any
		want  int32
		ok    bool
	}{
		{value: int64(12), want: 12, ok: true},
		{value: "-13", want: -13, ok: true},
		{value: float64(1.5)},
		{value: int64(math.MaxInt32) + 1},
		{value: "bad"},
	} {
		got, ok := stockScreenInt32(test.value)
		if ok != test.ok || (ok && got != test.want) {
			t.Fatalf("stockScreenInt32(%#v) = %d, %t", test.value, got, ok)
		}
	}
	if got, ok := stockScreenResultNumber(map[string]any{"dval": "bad", "ival": int32(9)}); !ok || got != 9 {
		t.Fatalf("stockScreenResultNumber fallback = %v, %t", got, ok)
	}
	if _, ok := stockScreenResultNumber(map[string]any{"dval": math.Inf(1)}); ok {
		t.Fatal("non-finite result was accepted")
	}
	values := stockScreenFallbackValues{cumulative: map[int32]float64{3101: 2}}
	if got := stockScreenPreviousClose(10, values); got == nil || *got != 8 {
		t.Fatalf("stockScreenPreviousClose = %v", got)
	}
	if got := stockScreenPreviousClose(1, values); got != nil {
		t.Fatalf("non-positive previous close = %v", got)
	}
	if got := stockScreenOptional(map[int32]float64{1: math.Inf(1)}, 1); got != nil {
		t.Fatalf("infinite optional = %v", got)
	}
	if got := stockScreenOptional(map[int32]float64{1: 3}, 1); got == nil || *got != 3 {
		t.Fatalf("valid optional = %v", got)
	}
}

func TestStockScreenFallbackParsesRowsAndMarketGroups(t *testing.T) {
	for _, test := range []struct {
		market string
		want   int64
		ok     bool
	}{
		{market: "HK", want: 1, ok: true}, {market: "US", want: 2, ok: true},
		{market: "CN", want: 3, ok: true}, {market: "SH", want: 3, ok: true}, {market: "SZ", want: 3, ok: true},
		{market: "SG", want: 4, ok: true}, {market: "CA", want: 5, ok: true}, {market: "AU", want: 6, ok: true},
		{market: "JP", want: 7, ok: true}, {market: "MY", want: 8, ok: true}, {market: "OTHER"},
	} {
		got, ok := stockScreenMarketValue(test.market)
		if ok != test.ok || (ok && got != test.want) {
			t.Fatalf("stockScreenMarketValue(%q) = %d, %t", test.market, got, ok)
		}
	}
	if stockScreenSymbolMarket(" us.aapl ") != "US" || stockScreenSymbolMarket("invalid") != "" {
		t.Fatal("stock-screen symbol market parsing is incorrect")
	}
	observedAt := time.Date(2026, time.August, 7, 14, 0, 0, 0, time.UTC)
	items := stockScreenSnapshotItems(map[string]any{"dataList": []any{
		"not-a-row",
		map[string]any{"stockId": "invalid"},
		map[string]any{"stockId": uint64(2), "results": []any{stockScreenTestSimpleResult(2201, 0)}},
		map[string]any{
			"stockId": "1",
			"results": []any{
				stockScreenTestSimpleResult(2201, "100"),
				stockScreenTestSimpleResult(2202, 99.0),
				stockScreenTestSimpleResult(2204, 101.0),
				stockScreenTestSimpleResult(2205, 98.0),
				stockScreenTestSimpleResult(2207, 99.5),
				stockScreenTestSimpleResult(2208, 100.5),
				map[string]any{"cumulativePropertyResult": map[string]any{
					"property": map[string]any{"name": int32(3101)}, "ival": int64(2),
				}},
				map[string]any{"simplePropertyResult": map[string]any{"property": "bad"}},
			},
		},
	}}, map[uint64]string{1: "US.AAPL"}, map[string]*string{"US.AAPL": fallbackTestString("Apple")}, observedAt)
	item, ok := items["US.AAPL"]
	if !ok || len(items) != 1 || item.LastPrice == nil || *item.LastPrice != 100 || item.PreviousClose == nil || *item.PreviousClose != 98 {
		t.Fatalf("stockScreenSnapshotItems = %#v", items)
	}
	if item.Name == nil || *item.Name != "Apple" || item.Session == nil || *item.Session != "regular" || item.BidPrice == nil || item.AskPrice == nil {
		t.Fatalf("stock-screen item metadata = %#v", item)
	}
	values := stockScreenValues(map[string]any{"results": []any{stockScreenTestSimpleResult(2201, 100)}})
	if values.simple[2201] != 100 || len(values.cumulative) != 0 {
		t.Fatalf("stockScreenValues = %#v", values)
	}
}

func TestStockScreenFallbackCoordinatesCopiesAndErrors(t *testing.T) {
	if got, err := (*stockScreenSnapshotCoordinator)(nil).query(t.Context(), nil, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty nil coordinator query = %#v, %v", got, err)
	}
	if _, err := (*stockScreenSnapshotCoordinator)(nil).query(t.Context(), []string{"HK.00700"}, nil); err == nil {
		t.Fatal("nil coordinator accepted a requested fallback")
	}
	if got, err := canonicalStockScreenSnapshotSymbols([]string{"HK.00700", "hk.00700", "US.AAPL"}); err != nil || strings.Join(got, ",") != "HK.00700,US.AAPL" {
		t.Fatalf("canonical symbols = %#v, %v", got, err)
	}
	if _, err := canonicalStockScreenSnapshotSymbols([]string{"invalid"}); err == nil {
		t.Fatal("invalid fallback symbol was accepted")
	}
	coordinator := newStockScreenSnapshotCoordinator()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := coordinator.query(ctx, []string{"HK.00700"}, func(context.Context, []string) (map[string]broker.SecuritySnapshotItem, error) {
		return map[string]broker.SecuritySnapshotItem{}, nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled fallback query = %v", err)
	}
	name, securityType, suspended, price, lot, session := "Tencent", "Equity", true, 100.0, int32(100), "regular"
	original := broker.SecuritySnapshotItem{
		Symbol: "HK.00700", Name: &name, SecurityType: &securityType, IsSuspended: &suspended,
		LastPrice: &price, BidPrice: &price, AskPrice: &price, PreviousClose: &price, OpenPrice: &price,
		HighPrice: &price, LowPrice: &price, Volume: &price, Turnover: &price, LotSize: &lot,
		UpdateTime: &name, Session: &session,
	}
	cloned := cloneStockScreenSnapshotMap(map[string]broker.SecuritySnapshotItem{"HK.00700": original})
	name, price, lot = "mutated", 1, 1
	if cloned["HK.00700"].Name == nil || *cloned["HK.00700"].Name != "Tencent" ||
		cloned["HK.00700"].LastPrice == nil || *cloned["HK.00700"].LastPrice != 100 ||
		cloned["HK.00700"].LotSize == nil || *cloned["HK.00700"].LotSize != 100 {
		t.Fatalf("cloned fallback item = %#v", cloned["HK.00700"])
	}
	var nilAdapter *futuAdapter
	if _, err := nilAdapter.QuerySnapshotFallback(t.Context(), broker.SecuritySnapshotQuery{Symbols: []string{"HK.00700"}}); err == nil {
		t.Fatal("nil adapter served a fallback snapshot")
	}
	adapter := &futuAdapter{exchange: NewExchange("127.0.0.1:1")}
	if _, err := adapter.QuerySnapshotFallback(t.Context(), broker.SecuritySnapshotQuery{}); err == nil {
		t.Fatal("empty fallback query was accepted")
	}
	if adapter.stockScreenSnapshotCoordinator() == nil {
		t.Fatal("adapter did not create a snapshot coordinator")
	}
	if _, err := adapter.queryStockScreenSnapshotPage(t.Context(), 1, nil); err == nil {
		t.Fatal("empty StockScreen page was accepted")
	}
	if _, err := adapter.queryStockScreenSnapshotPage(t.Context(), 1, make([]uint64, stockScreenSnapshotPageSize+1)); err == nil {
		t.Fatal("oversized StockScreen page was accepted")
	}
}

func stockScreenTestSimpleResult(property int32, value any) map[string]any {
	return map[string]any{"simplePropertyResult": map[string]any{
		"property": map[string]any{"name": property}, "dval": value,
	}}
}

func fallbackTestString(value string) *string { return &value }
