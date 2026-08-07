package futu

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotstockscreenpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotstockscreen"
)

func TestStockScreenSnapshotParamsUseStrictDelayedQuoteFields(t *testing.T) {
	params := stockScreenSnapshotParams(3, []uint64{101, 202})
	if err := opend.ValidateAdvancedC2S("Qot_StockScreen", params); err != nil {
		t.Fatalf("ValidateAdvancedC2S: %v", err)
	}
	if stockIDs, ok := params["watchlistStockIds"].([]uint64); !ok || !slices.Equal(stockIDs, []uint64{101, 202}) {
		t.Fatalf("watchlistStockIds = %#v", params["watchlistStockIds"])
	}
	if pageCount, ok := params["pageCount"].(int32); !ok || pageCount != 2 {
		t.Fatalf("pageCount = %#v", params["pageCount"])
	}
	filters, ok := params["filterList"].([]any)
	if !ok || len(filters) != 2 {
		t.Fatalf("filterList = %#v", params["filterList"])
	}
	for index, want := range []struct {
		field int32
		value int64
	}{
		{field: 1, value: 3},
		{field: 4, value: 1},
	} {
		filter, ok := filters[index].(map[string]any)
		query, queryOK := filter["simpleFieldQuery"].(map[string]any)
		field, fieldOK := query["simpleField"].(int32)
		values, valuesOK := query["screenValueList"].([]int64)
		if !ok || !queryOK || !fieldOK || !valuesOK || field != want.field || !slices.Equal(values, []int64{want.value}) {
			t.Fatalf("filter[%d] = %#v", index, filters[index])
		}
	}

	simple, cumulative := stockScreenSnapshotProperties(t, params)
	if !slices.Equal(simple, []int32{2201, 2202, 2203, 2204, 2205, 2207, 2208}) {
		t.Fatalf("simple properties = %#v", simple)
	}
	if !slices.Equal(cumulative, []int32{3101}) {
		t.Fatalf("cumulative properties = %#v", cumulative)
	}
}

func TestFutuStockScreenSnapshotFallbackUsesStaticIDsWithoutSubscription(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{
		stockScreenFallbackStaticInfo(qotcommonpb.QotMarket_QotMarket_CNSH_Security, "600519", 101, "Kweichow Moutai"),
		stockScreenFallbackStaticInfo(qotcommonpb.QotMarket_QotMarket_CNSZ_Security, "000001", 202, "Ping An Bank"),
		stockScreenFallbackStaticInfo(qotcommonpb.QotMarket_QotMarket_US_Security, "AAPL", 303, "Apple"),
	})
	server.setAdvancedResponse(3252, stockScreenFallbackResponse(
		stockScreenFallbackRow(101, 1500, map[int32]float64{2203: 1490}),
		stockScreenFallbackRow(202, 12.3, map[int32]float64{3101: 0.3}),
	))

	fallback, ok := NewBrokerAdapter(exchange).(broker.SnapshotFallbackSource)
	if !ok {
		t.Fatal("Futu broker adapter does not expose delayed snapshots")
	}
	result, err := fallback.QuerySnapshotFallback(t.Context(), broker.SecuritySnapshotQuery{
		ReadQuery: broker.ReadQuery{AccountID: "account-1"},
		Symbols:   []string{"SH.600519", "SZ.000001", "US.AAPL"},
	})
	if err != nil {
		t.Fatalf("QuerySnapshotFallback: %v", err)
	}
	if result == nil || result.AccountID != "account-1" || len(result.Snapshots) != 2 {
		t.Fatalf("fallback result = %#v", result)
	}
	items := make(map[string]broker.SecuritySnapshotItem, len(result.Snapshots))
	for _, item := range result.Snapshots {
		items[item.Symbol] = item
	}
	assertStockScreenFallbackItem(t, items["SH.600519"], "Kweichow Moutai", 1500, 1490)
	assertStockScreenFallbackItem(t, items["SZ.000001"], "Ping An Bank", 12.3, 12)
	if _, found := items["US.AAPL"]; found {
		t.Fatalf("missing StockScreen row was synthesized: %#v", items["US.AAPL"])
	}
	if calls := server.subCallCount(); calls != 0 {
		t.Fatalf("delayed fallback created Qot_Sub calls = %d", calls)
	}
}

func TestStockScreenSnapshotCoordinatorCachesRowsAndNegativeResults(t *testing.T) {
	now := time.Date(2026, time.August, 7, 2, 3, 4, 0, time.UTC)
	coordinator := newStockScreenSnapshotCoordinator()
	coordinator.now = func() time.Time { return now }
	var calls int
	var requests [][]string
	name := "Moutai"
	price := 1500.0
	fetch := func(_ context.Context, symbols []string) (map[string]broker.SecuritySnapshotItem, error) {
		calls++
		requests = append(requests, append([]string(nil), symbols...))
		return map[string]broker.SecuritySnapshotItem{
			"SH.600519": {Symbol: "SH.600519", Name: &name, LastPrice: &price},
		}, nil
	}

	first, err := coordinator.query(t.Context(), []string{"SZ.000001", "SH.600519"}, fetch)
	if err != nil || len(first) != 1 || calls != 1 {
		t.Fatalf("first query = %#v, %v; calls=%d", first, err, calls)
	}
	if !slices.Equal(requests[0], []string{"SH.600519", "SZ.000001"}) {
		t.Fatalf("first fetch symbols = %#v", requests[0])
	}
	*first["SH.600519"].Name = "mutated"
	second, err := coordinator.query(t.Context(), []string{"SH.600519", "SZ.000001"}, fetch)
	if err != nil || calls != 1 || second["SH.600519"].Name == nil || *second["SH.600519"].Name != "Moutai" {
		t.Fatalf("cached query = %#v, %v; calls=%d", second, err, calls)
	}

	now = now.Add(stockScreenSnapshotCacheTTL + time.Nanosecond)
	if _, err := coordinator.query(t.Context(), []string{"SH.600519", "SZ.000001"}, fetch); err != nil || calls != 2 {
		t.Fatalf("expired query error = %v; calls=%d", err, calls)
	}
	if !slices.Equal(requests[1], []string{"SH.600519", "SZ.000001"}) {
		t.Fatalf("expired fetch symbols = %#v", requests[1])
	}

	failing := newStockScreenSnapshotCoordinator()
	attempts := 0
	_, err = failing.query(t.Context(), []string{"SH.600519"}, func(context.Context, []string) (map[string]broker.SecuritySnapshotItem, error) {
		attempts++
		return nil, errors.New("StockScreen unavailable")
	})
	if err == nil {
		t.Fatal("failed delayed snapshot fetch returned nil")
	}
	_, err = failing.query(t.Context(), []string{"SH.600519"}, func(context.Context, []string) (map[string]broker.SecuritySnapshotItem, error) {
		attempts++
		return map[string]broker.SecuritySnapshotItem{}, nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("failed fetch was cached: err=%v attempts=%d", err, attempts)
	}
}

func TestFutuStockScreenSnapshotFallbackReportsScreenErrors(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{
		stockScreenFallbackStaticInfo(qotcommonpb.QotMarket_QotMarket_CNSH_Security, "600519", 101, "Kweichow Moutai"),
	})
	retType, errCode := int32(-1), int32(9)
	server.setAdvancedResponse(3252, &qotstockscreenpb.Response{
		RetType: &retType, ErrCode: &errCode, RetMsg: new("StockScreen unavailable"),
	})

	fallback := NewBrokerAdapter(exchange).(broker.SnapshotFallbackSource)
	_, err := fallback.QuerySnapshotFallback(t.Context(), broker.SecuritySnapshotQuery{Symbols: []string{"SH.600519"}})
	if err == nil || !strings.Contains(err.Error(), "Qot_StockScreen") {
		t.Fatalf("QuerySnapshotFallback error = %v", err)
	}
	if calls := server.subCallCount(); calls != 0 {
		t.Fatalf("failed delayed fallback created Qot_Sub calls = %d", calls)
	}
}

func stockScreenSnapshotProperties(t *testing.T, params map[string]any) ([]int32, []int32) {
	t.Helper()
	retrieve, ok := params["retrieveList"].([]any)
	if !ok {
		t.Fatalf("retrieveList = %#v", params["retrieveList"])
	}
	simple := make([]int32, 0, len(retrieve))
	cumulative := make([]int32, 0, len(retrieve))
	for _, raw := range retrieve {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("retrieve entry = %#v", raw)
		}
		if property, ok := entry["simpleProperty"].(map[string]any); ok {
			name, ok := property["name"].(int32)
			if !ok {
				t.Fatalf("simple property = %#v", property)
			}
			simple = append(simple, name)
		}
		if property, ok := entry["cumulativeProperty"].(map[string]any); ok {
			name, ok := property["name"].(int32)
			if !ok {
				t.Fatalf("cumulative property = %#v", property)
			}
			cumulative = append(cumulative, name)
		}
	}
	return simple, cumulative
}

func stockScreenFallbackStaticInfo(
	market qotcommonpb.QotMarket,
	code string,
	id int64,
	name string,
) *qotcommonpb.SecurityStaticInfo {
	marketValue := int32(market)
	lotSize := int32(100)
	securityType := int32(qotcommonpb.SecurityType_SecurityType_Eqty)
	listTime := "2000-01-01"
	return &qotcommonpb.SecurityStaticInfo{Basic: &qotcommonpb.SecurityStaticBasic{
		Security: &qotcommonpb.Security{Market: &marketValue, Code: &code},
		Id:       &id,
		LotSize:  &lotSize,
		SecType:  &securityType,
		Name:     &name,
		ListTime: &listTime,
	}}
}

func stockScreenFallbackResponse(rows ...*qotstockscreenpb.StockScreenItem) *qotstockscreenpb.Response {
	retType := int32(0)
	return &qotstockscreenpb.Response{
		RetType: &retType,
		S2C:     &qotstockscreenpb.S2C{DataList: rows},
	}
}

func stockScreenFallbackRow(
	stockID uint64,
	price float64,
	values map[int32]float64,
) *qotstockscreenpb.StockScreenItem {
	results := []*qotstockscreenpb.RspItemResult{stockScreenSimpleResult(2201, price)}
	for _, property := range []int32{2202, 2203, 2204, 2205, 2207, 2208} {
		if value, ok := values[property]; ok {
			results = append(results, stockScreenSimpleResult(property, value))
		}
	}
	if change, ok := values[3101]; ok {
		results = append(results, stockScreenCumulativeResult(3101, change))
	}
	return &qotstockscreenpb.StockScreenItem{StockId: &stockID, Results: results}
}

func stockScreenSimpleResult(property int32, value float64) *qotstockscreenpb.RspItemResult {
	return &qotstockscreenpb.RspItemResult{SimplePropertyResult: &qotstockscreenpb.ResultPropertySimple{
		Property: &qotstockscreenpb.PropertySimple{Name: &property}, Dval: &value,
	}}
}

func stockScreenCumulativeResult(property int32, value float64) *qotstockscreenpb.RspItemResult {
	days := uint32(1)
	return &qotstockscreenpb.RspItemResult{CumulativePropertyResult: &qotstockscreenpb.ResultPropertyCumulative{
		Property: &qotstockscreenpb.PropertyCumulative{Name: &property, Days: &days}, Dval: &value,
	}}
}

func assertStockScreenFallbackItem(
	t *testing.T,
	item broker.SecuritySnapshotItem,
	wantName string,
	wantPrice float64,
	wantPreviousClose float64,
) {
	t.Helper()
	if item.Source != "futu:stock-screen-delayed" || item.Name == nil || *item.Name != wantName || item.LastPrice == nil || item.PreviousClose == nil {
		t.Fatalf("fallback item = %#v", item)
	}
	if math.Abs(*item.LastPrice-wantPrice) > 1e-9 || math.Abs(*item.PreviousClose-wantPreviousClose) > 1e-9 {
		t.Fatalf("fallback item prices = %#v, want %v/%v", item, wantPrice, wantPreviousClose)
	}
}
