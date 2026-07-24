package futu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

func TestTranslateResearchScreenParamsBuildsStrictStockScreenRequest(t *testing.T) {
	definition := broker.ScreenDefinitionV2{
		Market:             "HK",
		CatalogVersion:     researchscreen.CatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		Pool: broker.ResearchScreenPool{
			Plates:            []broker.ResearchScreenPlate{{ParentPlateID: "BK1000", PlateIDs: []string{"BK1001"}}},
			WatchlistStockIDs: []string{"12"},
		},
		Conditions: []broker.ScreenCondition{
			{
				ID: "price", Factor: broker.FactorRef{InstanceID: "price", FactorKey: "simple.price"},
				Operator: "between", Value: map[string]any{"min": 10.0},
			},
			{
				ID: "change", Factor: broker.FactorRef{
					InstanceID: "change", FactorKey: "cumulative.price_change_pct",
					Params: broker.ResearchScreenFactorParams{Days: 5},
				}, Operator: "between", Value: map[string]any{"min": 10.0, "continuousPeriod": 2},
			},
			{
				ID: "profit", Factor: broker.FactorRef{
					InstanceID: "profit", FactorKey: "financial.net_profit",
					Params: broker.ResearchScreenFactorParams{Term: 10, Year: 2025},
				}, Operator: "between", Value: map[string]any{"min": 10.0},
			},
			{
				ID: "ma", Factor: broker.FactorRef{
					InstanceID: "ma", FactorKey: "indicator.ma", Params: broker.ResearchScreenFactorParams{
						Period: 11, IndicatorParams: []int64{6},
					},
				}, Operator: "position", Value: map[string]any{"position": 1, "secondValue": 0},
			},
			{
				ID: "pattern", Factor: broker.FactorRef{
					InstanceID: "pattern", FactorKey: "pattern.macd_gold_cross",
					Params: broker.ResearchScreenFactorParams{Period: 11},
				}, Operator: "pattern", Value: map[string]any{"match": true},
			},
			{
				ID: "chips", Factor: broker.FactorRef{InstanceID: "chips", FactorKey: "featured.chips_profit_ratio"},
				Operator: "between", Value: map[string]any{"min": 10.0},
			},
			{
				ID: "broker", Factor: broker.FactorRef{
					InstanceID: "broker", FactorKey: "broker.concentrated_distribution",
					Params: broker.ResearchScreenFactorParams{Days: 5},
				}, Operator: "between", Value: map[string]any{"min": 10.0},
			},
			{
				ID: "iv", Factor: broker.FactorRef{InstanceID: "iv", FactorKey: "option.stock_iv"},
				Operator: "between", Value: map[string]any{"min": 10.0},
			},
			{
				ID: "shape", Factor: broker.FactorRef{
					InstanceID: "shape", FactorKey: "kline_shape.shape_type",
					Params: broker.ResearchScreenFactorParams{Period: 11},
				}, Operator: "in", Value: []int64{1},
			},
		},
		Columns: []broker.ScreenColumn{{
			ID: "roe", Factor: broker.FactorRef{InstanceID: "roe", FactorKey: "financial.roe"},
		}},
		Sorts: []broker.ScreenSort{{
			ID: "market-cap", Factor: broker.FactorRef{InstanceID: "market-cap", FactorKey: "simple.market_cap"},
			Direction: "desc",
		}},
	}
	params := map[string]any{
		researchScreenDefinitionParam: definition,
		"pageFrom":                    50,
		"pageCount":                   50,
	}
	query := broker.FeatureQuery{Market: "HK"}
	if err := translateResearchScreenParams(params, query); err != nil {
		t.Fatal(err)
	}
	if err := opend.ValidateAdvancedC2S("Qot_StockScreen", params); err != nil {
		t.Fatalf("strict protobuf validation failed: %v\nparams=%#v", err, params)
	}
	filters, _ := params["filterList"].([]any)
	if len(filters) != len(definition.Conditions)+2 {
		t.Fatalf("filter count = %d, want %d", len(filters), len(definition.Conditions)+2)
	}
	retrieve, _ := params["retrieveList"].([]any)
	if len(retrieve) != len(definition.Columns) {
		t.Fatalf("retrieve count = %d", len(retrieve))
	}
}

func TestTranslateResearchScreenParamsValidatesStableKeysAndMarket(t *testing.T) {
	base := broker.ScreenDefinitionV2{
		Market:             "HK",
		CatalogVersion:     researchscreen.CatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
	}
	testCases := []broker.ScreenDefinitionV2{
		func() broker.ScreenDefinitionV2 {
			value := base
			value.Market = "SG"
			return value
		}(),
		func() broker.ScreenDefinitionV2 {
			value := base
			value.Conditions = []broker.ScreenCondition{{
				ID: "missing", Factor: broker.FactorRef{InstanceID: "missing", FactorKey: "missing.factor"},
				Operator: "between", Value: map[string]any{"min": 1},
			}}
			return value
		}(),
		func() broker.ScreenDefinitionV2 {
			value := base
			value.Conditions = []broker.ScreenCondition{{
				ID: "market", Factor: broker.FactorRef{InstanceID: "market", FactorKey: "field.market"},
				Operator: "in", Value: []int64{2},
			}}
			return value
		}(),
	}
	for _, definition := range testCases {
		params := map[string]any{researchScreenDefinitionParam: definition}
		if err := translateResearchScreenParams(params, broker.FeatureQuery{}); err == nil {
			t.Fatalf("definition unexpectedly accepted: %#v", definition)
		}
	}
}

func TestStockScreenFeatureResultNormalizesIdentityCellsAndOffset(t *testing.T) {
	result := stockScreenFeatureResult(broker.FeatureQuery{
		Market: "US", Cursor: "50", FeatureID: broker.FeatureResearchScreen,
	}, map[string]any{
		"allCount": float64(100), "lastPage": float64(0),
		"dataList": []any{map[string]any{
			"stockId": "123",
			"results": []any{
				map[string]any{"basicPropertyResult": map[string]any{
					"property": map[string]any{"name": float64(1101)}, "valueType": float64(1), "sval": "AAPL",
				}},
				map[string]any{"basicPropertyResult": map[string]any{
					"property": map[string]any{"name": float64(1102)}, "valueType": float64(1), "sval": "Apple",
				}},
				map[string]any{"simplePropertyResult": map[string]any{
					"property": map[string]any{"name": float64(2201)}, "valueType": float64(4), "dval": 190.25,
				}},
				map[string]any{"financialPropertyResult": map[string]any{
					"property": map[string]any{"name": float64(4101)}, "valueType": float64(2),
				}},
			},
		}},
	})
	if len(result.Entries) != 1 || result.NextCursor != "51" ||
		result.HasMore == nil || !*result.HasMore || result.Total == nil || *result.Total != 100 {
		t.Fatalf("result pagination = %#v", result)
	}
	row := result.Entries[0]
	if row["instrumentId"] != "US.AAPL" || row["name"] != "Apple" || row["stockId"] != "123" {
		t.Fatalf("row identity = %#v", row)
	}
	if _, exists := row["values"]; exists {
		t.Fatal("legacy values map was included in a result row")
	}
	cells := row["cells"].(map[string]broker.ScreenResultCell)
	if value := cells["simple.price"].Value; value.Type != "number" || value.Number == nil || *value.Number != 190.25 {
		t.Fatalf("price value = %#v", value)
	}
	if value := cells["financial.net_profit"].Value; value.Type != "missing" {
		t.Fatalf("missing value = %#v", value)
	}
	if _, leaked := result.Entries[0]["lastPage"]; leaked {
		t.Fatal("lastPage leaked into a result row")
	}
}

func TestNormalizeStockScreenRowPreservesParameterizedInstanceIdentity(t *testing.T) {
	factor, ok := researchscreen.Lookup("indicator.ma")
	if !ok {
		t.Fatal("indicator.ma is missing from catalog")
	}
	query := broker.FeatureQuery{
		Market: "US",
		Params: map[string]any{researchScreenDefinitionParam: broker.ScreenDefinitionV2{
			Market: "US", CatalogVersion: researchscreen.CatalogVersion,
			QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
			Columns: []broker.ScreenColumn{
				{ID: "ma20-column", Factor: broker.FactorRef{InstanceID: "ma20", FactorKey: factor.Key, Params: broker.ResearchScreenFactorParams{Period: 11, IndicatorParams: []int64{20}}}},
				{ID: "ma60-column", Factor: broker.FactorRef{InstanceID: "ma60", FactorKey: factor.Key, Params: broker.ResearchScreenFactorParams{Period: 11, IndicatorParams: []int64{60}}}},
			},
		}},
	}
	row := normalizeStockScreenRow(query, map[string]any{
		"stockId":  float64(1),
		"security": map[string]any{"market": "US", "code": "AAPL", "instrumentId": "US.AAPL"},
		"results": []any{
			map[string]any{"indicatorPropertyResult": map[string]any{
				"property":  map[string]any{"name": float64(factor.ProviderID), "period": float64(11), "indicatorParams": []any{float64(20)}},
				"valueType": float64(4), "dval": float64(180),
			}},
			map[string]any{"indicatorPropertyResult": map[string]any{
				"property":  map[string]any{"name": float64(factor.ProviderID), "period": float64(11), "indicatorParams": []any{float64(60)}},
				"valueType": float64(4), "dval": float64(170),
			}},
		},
	})
	cells := row["cells"].(map[string]broker.ScreenResultCell)
	if len(cells) != 2 || cells["ma20-column"].InstanceID != "ma20" || cells["ma60-column"].InstanceID != "ma60" {
		t.Fatalf("instance-aware cells = %#v", cells)
	}
	if cells["ma20-column"].Value.Number == nil || *cells["ma20-column"].Value.Number != 180 ||
		cells["ma60-column"].Value.Number == nil || *cells["ma60-column"].Value.Number != 170 {
		t.Fatalf("column-aware values = %#v", cells)
	}
}

func TestStockScreenFeatureResultUsesPerRowMainlandIdentityAndFiltersExactMarkets(t *testing.T) {
	rows := []map[string]any{
		stockScreenIdentityTestRow("101", "600519", "贵州茅台"),
		stockScreenIdentityTestRow("202", "000001", "平安银行"),
	}
	candidates := stockScreenIdentityCandidates(rows)
	if len(candidates) != 4 {
		t.Fatalf("identity candidate count = %d, want 4", len(candidates))
	}
	if candidates[0].GetMarket() != int32(qotcommonpb.QotMarket_QotMarket_CNSH_Security) ||
		candidates[1].GetMarket() != int32(qotcommonpb.QotMarket_QotMarket_CNSZ_Security) ||
		candidates[0].GetCode() != "600519" || candidates[2].GetCode() != "000001" {
		t.Fatalf("identity candidates = %#v", candidates)
	}

	shMarket := int32(qotcommonpb.QotMarket_QotMarket_CNSH_Security)
	szMarket := int32(qotcommonpb.QotMarket_QotMarket_CNSZ_Security)
	shID, szID := int64(101), int64(202)
	shCode, szCode := "600519", "000001"
	staticInfo := []*qotcommonpb.SecurityStaticInfo{
		{Basic: &qotcommonpb.SecurityStaticBasic{
			Id: &shID, Security: &qotcommonpb.Security{Market: &shMarket, Code: &shCode},
		}},
		{Basic: &qotcommonpb.SecurityStaticBasic{
			Id: &szID, Security: &qotcommonpb.Security{Market: &szMarket, Code: &szCode},
		}},
	}
	if err := applyStockScreenIdentities(rows, staticInfo); err != nil {
		t.Fatal(err)
	}
	for index, expectedMarket := range []string{"SH", "SZ"} {
		security, ok := rows[index]["security"].(map[string]any)
		if !ok || security["market"] != expectedMarket {
			t.Fatalf("row %d security = %#v", index, rows[index]["security"])
		}
		if _, leaked := security["providerMarket"]; leaked {
			t.Fatalf("row %d leaked provider market id: %#v", index, security)
		}
	}

	payload := normalizeOpenDMap(map[string]any{
		"allCount": float64(20),
		"lastPage": float64(0),
		"dataList": []any{rows[0], rows[1]},
	})
	combined := stockScreenFeatureResult(broker.FeatureQuery{
		Market: "CN", FeatureID: broker.FeatureResearchScreen,
	}, payload)
	if len(combined.Entries) != 2 ||
		combined.Entries[0]["instrumentId"] != "SH.600519" ||
		combined.Entries[1]["instrumentId"] != "SZ.000001" {
		t.Fatalf("combined mainland identities = %#v", combined.Entries)
	}

	for _, testCase := range []struct {
		market       string
		instrumentID string
	}{
		{market: "SH", instrumentID: "SH.600519"},
		{market: "SZ", instrumentID: "SZ.000001"},
	} {
		t.Run(testCase.market, func(t *testing.T) {
			result := stockScreenFeatureResult(broker.FeatureQuery{
				Market: testCase.market, Cursor: "10",
				FeatureID: broker.FeatureResearchScreen,
			}, payload)
			if len(result.Entries) != 1 ||
				result.Entries[0]["instrumentId"] != testCase.instrumentID ||
				result.Entries[0]["market"] != testCase.market {
				t.Fatalf("%s entries = %#v", testCase.market, result.Entries)
			}
			if result.NextCursor != "12" || result.HasMore == nil || !*result.HasMore {
				t.Fatalf("%s pagination = next %q hasMore %#v", testCase.market, result.NextCursor, result.HasMore)
			}
			if result.Total != nil ||
				len(result.Warnings) != 1 ||
				result.Warnings[0] != exactMainlandStockScreenTotalWarning {
				t.Fatalf(
					"%s total/warnings = %#v / %#v",
					testCase.market,
					result.Total,
					result.Warnings,
				)
			}
		})
	}
}

func TestStockScreenMainlandIdentityMustBeAuthoritative(t *testing.T) {
	row := stockScreenIdentityTestRow("101", "600519", "贵州茅台")
	entry := normalizeStockScreenRow(broker.FeatureQuery{Market: "SH"}, row)
	if entry["market"] != "" || entry["instrumentId"] != "" {
		t.Fatalf("unresolved mainland row was relabelled from request market: %#v", entry)
	}
	if err := applyStockScreenIdentities([]map[string]any{row}, nil); err == nil {
		t.Fatal("unresolved mainland identity unexpectedly succeeded")
	}
}

func TestResearchScreenQuoteCurrencyUsesSecurityCounterIdentity(t *testing.T) {
	testCases := []struct {
		name         string
		market       string
		symbol       string
		securityName string
		wantCurrency string
	}{
		{name: "US", market: "US", symbol: "AAPL", securityName: "Apple", wantCurrency: "USD"},
		{name: "Shanghai", market: "SH", symbol: "600519", securityName: "贵州茅台", wantCurrency: "CNY"},
		{name: "Shenzhen", market: "SZ", symbol: "000001", securityName: "平安银行", wantCurrency: "CNY"},
		{name: "HKD counter", market: "HK", symbol: "00700", securityName: "腾讯控股", wantCurrency: "HKD"},
		{name: "RMB counter", market: "HK", symbol: "80700", securityName: "腾讯控股-R", wantCurrency: "CNY"},
		{name: "RMB BYD counter", market: "HK", symbol: "81211", securityName: "比亚迪股份-R", wantCurrency: "CNY"},
		{name: "RMB code without marker", market: "HK", symbol: "80700", securityName: "腾讯控股"},
		{name: "RMB marker on HKD code", market: "HK", symbol: "00700", securityName: "腾讯控股-R"},
		{name: "missing name", market: "HK", symbol: "00700"},
		{name: "missing symbol", market: "US", securityName: "Apple"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := researchScreenQuoteCurrency(
				testCase.market,
				testCase.symbol,
				testCase.securityName,
			); got != testCase.wantCurrency {
				t.Fatalf("quote currency = %q, want %q", got, testCase.wantCurrency)
			}
		})
	}

	row := normalizeStockScreenRow(
		broker.FeatureQuery{Market: "HK"},
		stockScreenIdentityTestRow("80700", "80700", "腾讯控股-R"),
	)
	if row["quoteCurrency"] != "CNY" {
		t.Fatalf("normalized RMB counter = %#v", row)
	}
}

func TestResearchScreenLimiterAllowsTenPerThirtySeconds(t *testing.T) {
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	limiter := newResearchScreenLimiter()
	limiter.now = func() time.Time { return now }
	for index := range researchScreenLimit {
		if retry := limiter.retryAfter(); retry != 0 {
			t.Fatalf("request %d retry=%s", index+1, retry)
		}
	}
	if retry := limiter.retryAfter(); retry != researchScreenWindow {
		t.Fatalf("11th request retry=%s", retry)
	}
	now = now.Add(researchScreenWindow)
	if retry := limiter.retryAfter(); retry != 0 {
		t.Fatalf("request after window retry=%s", retry)
	}
}

func TestResearchScreenRateLimitErrorRoundTrip(t *testing.T) {
	err := broker.NewResearchScreenRateLimitError(2500 * time.Millisecond)
	if !errors.Is(err, broker.ErrResearchScreenRateLimited) {
		t.Fatal("rate-limit sentinel was not preserved")
	}
	if delay, ok := broker.ResearchScreenRetryAfter(err); !ok || delay != 2500*time.Millisecond {
		t.Fatalf("retry after = %s, %v", delay, ok)
	}
}

func TestResearchScreenTranslationEdges(t *testing.T) {
	if err := translateResearchScreenParams(map[string]any{}, broker.FeatureQuery{}); err == nil {
		t.Fatal("missing definition unexpectedly succeeded")
	}

	params := map[string]any{researchScreenDefinitionParam: map[string]any{
		"market": "US", "catalogVersion": researchscreen.CatalogVersion,
		"querySchemaVersion": broker.ScreenQuerySchemaVersionV2,
	}}
	if err := translateResearchScreenParams(params, broker.FeatureQuery{}); err != nil {
		t.Fatalf("generic definition: %v", err)
	}
	if _, exists := params[researchScreenDefinitionParam]; exists {
		t.Fatal("definition leaked into provider params")
	}
	if _, err := decodeResearchScreenDefinition(map[string]any{"invalid": make(chan int)}); err == nil {
		t.Fatal("unencodable definition unexpectedly succeeded")
	}
	if _, err := decodeResearchScreenDefinition("invalid"); err == nil {
		t.Fatal("non-object definition unexpectedly succeeded")
	}

	for _, testCase := range []struct {
		market string
		want   int64
	}{
		{market: " hk ", want: 1},
		{market: "us", want: 2},
		{market: "SH", want: 3},
		{market: "SZ", want: 3},
		{market: "CN", want: 3},
	} {
		got, err := researchScreenMarketValue(testCase.market)
		if err != nil || got != testCase.want {
			t.Fatalf("market %q = %d, %v; want %d", testCase.market, got, err, testCase.want)
		}
	}
	if _, err := researchScreenMarketValue("SG"); err == nil {
		t.Fatal("unsupported market unexpectedly succeeded")
	}

	filters, err := translateResearchScreenFilters(broker.ScreenDefinitionV2{}, "HK", 1)
	if err != nil || len(filters) != 1 {
		t.Fatalf("default market filter = %#v, %v", filters, err)
	}
	_, err = translateResearchScreenFilters(broker.ScreenDefinitionV2{
		Pool: broker.ResearchScreenPool{Plates: []broker.ResearchScreenPlate{{PlateIDs: []string{" ", ""}}}},
	}, "HK", 1)
	if err == nil {
		t.Fatal("empty plate group unexpectedly succeeded")
	}
	if _, err := translateResearchScreenFilters(broker.ScreenDefinitionV2{
		Conditions: []broker.ScreenCondition{{Factor: broker.FactorRef{FactorKey: "missing.factor"}}},
	}, "HK", 1); err == nil {
		t.Fatal("unknown filter factor unexpectedly succeeded")
	}
	if _, err := translateResearchScreenFilters(broker.ScreenDefinitionV2{
		Conditions: []broker.ScreenCondition{{Factor: broker.FactorRef{FactorKey: "field.market"}}},
	}, "HK", 1); err == nil {
		t.Fatal("invalid field filter unexpectedly succeeded")
	}

	duplicate := broker.ScreenColumn{Factor: broker.FactorRef{FactorKey: "simple.price"}}
	retrieve, err := translateResearchScreenRetrieve(broker.ScreenDefinitionV2{
		Columns: []broker.ScreenColumn{duplicate, duplicate},
	}, "US")
	if err != nil || len(retrieve) != 1 {
		t.Fatalf("deduplicated retrieve = %#v, %v", retrieve, err)
	}
	if _, err := translateResearchScreenRetrieve(broker.ScreenDefinitionV2{
		Columns: []broker.ScreenColumn{{Factor: broker.FactorRef{FactorKey: "missing.factor"}}},
	}, "US"); err == nil {
		t.Fatal("unknown retrieve factor unexpectedly succeeded")
	}
	if _, err := translateResearchScreenSorts(broker.ScreenDefinitionV2{
		Sorts: []broker.ScreenSort{{
			Factor: broker.FactorRef{FactorKey: "simple.price"}, Direction: "sideways",
		}},
	}, "US"); err == nil {
		t.Fatal("invalid sort direction unexpectedly succeeded")
	}
	if _, err := translateResearchScreenSorts(broker.ScreenDefinitionV2{
		Sorts: []broker.ScreenSort{{Factor: broker.FactorRef{FactorKey: "missing.factor"}}},
	}, "US"); err == nil {
		t.Fatal("unknown sort factor unexpectedly succeeded")
	}
	if _, err := decodeResearchScreenDefinition(map[string]any{
		"market": "SG", "catalogVersion": researchscreen.CatalogVersion,
		"querySchemaVersion": broker.ScreenQuerySchemaVersionV2,
	}); err == nil {
		t.Fatal("invalid generic definition unexpectedly succeeded")
	}

	if _, _, err := translateResearchScreenCondition(broker.ScreenCondition{
		Factor: broker.FactorRef{FactorKey: "field.market"},
	}); err == nil {
		t.Fatal("empty field condition unexpectedly succeeded")
	}
	if _, _, err := translateResearchScreenCondition(broker.ScreenCondition{
		Factor: broker.FactorRef{FactorKey: "missing.factor"},
	}); err == nil {
		t.Fatal("unknown condition factor unexpectedly succeeded")
	}
	fieldQuery, _, err := translateResearchScreenCondition(broker.ScreenCondition{
		Factor: broker.FactorRef{FactorKey: "field.market"}, Value: []any{float64(2), "3", "bad"},
	})
	if err != nil || fieldQuery["simpleFieldQuery"] == nil {
		t.Fatalf("field query = %#v, %v", fieldQuery, err)
	}

	indicator := broker.ScreenCondition{
		Factor: broker.FactorRef{FactorKey: "indicator.rsi"},
		Value:  map[string]any{"position": 0},
	}
	if _, _, err := translateResearchScreenCondition(indicator); err == nil {
		t.Fatal("invalid indicator position unexpectedly succeeded")
	}
	indicator.Value = map[string]any{"position": 1, "secondValue": "42"}
	query, _, err := translateResearchScreenCondition(indicator)
	if err != nil || query["indicatorPositionalQuery"] == nil {
		t.Fatalf("indicator query = %#v, %v", query, err)
	}
	badSecond := broker.FactorRef{FactorKey: "simple.price"}
	indicator.SecondFactor = &badSecond
	if _, _, err := translateResearchScreenCondition(indicator); err == nil {
		t.Fatal("non-indicator second factor unexpectedly succeeded")
	}
	goodSecond := broker.FactorRef{FactorKey: "indicator.ma"}
	indicator.SecondFactor = &goodSecond
	if _, _, err := translateResearchScreenCondition(indicator); err != nil {
		t.Fatalf("indicator comparison: %v", err)
	}

	patternQuery, _, err := translateResearchScreenCondition(broker.ScreenCondition{
		Factor: broker.FactorRef{FactorKey: "pattern.macd_gold_cross"},
		Value:  map[string]any{"match": false, "values": []any{1}},
	})
	if err != nil || patternQuery["indicatorPatternQuery"] == nil {
		t.Fatalf("pattern query = %#v, %v", patternQuery, err)
	}
	if _, _, err := translateResearchScreenCondition(broker.ScreenCondition{
		Factor: broker.FactorRef{FactorKey: "kline_shape.shape_type"},
	}); err == nil {
		t.Fatal("empty kline shape unexpectedly succeeded")
	}
	if _, _, err := translateResearchScreenProperty(
		broker.FactorRef{FactorKey: "field.market"}, false, false,
	); err == nil {
		t.Fatal("field factor unexpectedly translated as a property")
	}
	if _, _, err := translateResearchScreenProperty(
		broker.FactorRef{FactorKey: "missing.factor"}, false, false,
	); err == nil {
		t.Fatal("unknown factor unexpectedly translated as a property")
	}

	cumulative := mustResearchScreenFactor(t, "cumulative.price_change_pct")
	if property := researchScreenProperty(cumulative, broker.ResearchScreenFactorParams{}); property["days"] != int32(1) {
		t.Fatalf("default cumulative property = %#v", property)
	}
	indicatorFactor := mustResearchScreenFactor(t, "indicator.ma")
	if property := researchScreenProperty(indicatorFactor, broker.ResearchScreenFactorParams{}); property["period"] != int32(11) {
		t.Fatalf("default indicator property = %#v", property)
	}
	option := mustResearchScreenFactor(t, "option.stock_iv")
	optionProperty := researchScreenProperty(option, broker.ResearchScreenFactorParams{
		OptionParamType: 2, OptionParamInteger: 30,
	})
	if optionProperty["param"] == nil {
		t.Fatalf("option property = %#v", optionProperty)
	}

	boundary := researchScreenConditionBoundary(map[string]any{
		"min": int64(10), "minIncludes": false,
	}, "min")
	if boundary.(map[string]any)["includes"] != false {
		t.Fatalf("exclusive boundary = %#v", boundary)
	}
	if researchScreenConditionBoundary(map[string]any{}, "min") != nil {
		t.Fatal("missing boundary was not nil")
	}
	if intervals := researchScreenConditionIntervals(map[string]any{
		"intervals": []any{map[string]any{"min": "1", "max": 2, "unit": 3}},
	}); len(intervals) != 1 {
		t.Fatalf("explicit intervals = %#v", intervals)
	}
	if intervals := researchScreenConditionIntervals(map[string]any{"min": 1}); len(intervals) != 1 {
		t.Fatalf("inferred intervals = %#v", intervals)
	}
	if values := researchScreenConditionValues([]int64{1, 2}); len(values) != 2 {
		t.Fatalf("typed values = %#v", values)
	}
	if values := researchScreenConditionValues("invalid"); values != nil {
		t.Fatalf("invalid values = %#v", values)
	}

	for direction, want := range map[string]int32{
		"asc": 1, "desc": 2, "": 2, "abs_asc": 3, "abs_desc": 4,
	} {
		got, err := researchScreenSortDirection(direction)
		if err != nil || got != want {
			t.Fatalf("sort %q = %d, %v; want %d", direction, got, err, want)
		}
	}
	if cleaned := cleanStrings([]string{" A ", "", "  ", "B"}); len(cleaned) != 2 || cleaned[0] != "A" {
		t.Fatalf("clean strings = %#v", cleaned)
	}
	if stockScreenOffset(" -2 ") != 0 || stockScreenOffset("bad") != 0 || stockScreenOffset("3") != 3 {
		t.Fatal("stock screen offset normalization failed")
	}
}

func TestStockScreenNormalizationEdges(t *testing.T) {
	price := mustResearchScreenFactor(t, "simple.price")
	values := []struct {
		input map[string]any
		kind  string
	}{
		{input: map[string]any{"valueType": 1, "sval": "text", "endTime": "10"}, kind: "string"},
		{input: map[string]any{"valueType": 2, "ival": "11"}, kind: "integer"},
		{input: map[string]any{"valueType": 3, "aval": []any{1, "2", false}}, kind: "integer_array"},
		{input: map[string]any{"valueType": 4, "dval": "12.5"}, kind: "number"},
		{input: map[string]any{"valueType": 4, "dval": false}, kind: "missing"},
	}
	for _, testCase := range values {
		if got := normalizeStockScreenValue(price, testCase.input); got.Type != testCase.kind {
			t.Fatalf("normalized value %#v = %#v, want %s", testCase.input, got, testCase.kind)
		}
	}

	for _, testCase := range []struct {
		value any
		want  string
	}{
		{value: "12", want: "12"}, {value: "bad", want: ""},
		{value: float64(13), want: "13"}, {value: uint64(14), want: "14"},
		{value: 15, want: ""},
	} {
		if got := uint64String(testCase.value); got != testCase.want {
			t.Fatalf("uint64String(%#v) = %q, want %q", testCase.value, got, testCase.want)
		}
	}
	for _, value := range []any{"1", float64(2), 3, int32(4), int64(5)} {
		if _, ok := int64Value(value); !ok {
			t.Fatalf("int64Value(%#v) failed", value)
		}
	}
	if _, ok := int64Value("bad"); ok {
		t.Fatal("invalid integer string unexpectedly succeeded")
	}
	if _, ok := int64Value(false); ok {
		t.Fatal("invalid integer type unexpectedly succeeded")
	}
	for _, value := range []any{float64(1.5), "2.5"} {
		if _, ok := float64Value(value); !ok {
			t.Fatalf("float64Value(%#v) failed", value)
		}
	}
	if _, ok := float64Value("bad"); ok {
		t.Fatal("invalid float string unexpectedly succeeded")
	}
	if _, ok := float64Value(1); ok {
		t.Fatal("invalid float type unexpectedly succeeded")
	}
	if int64Slice([]int64{1}) != nil {
		t.Fatal("non-provider integer slice unexpectedly succeeded")
	}

	text := "value"
	integer := int64(2)
	number := 3.5
	for _, testCase := range []struct {
		value broker.ResearchScreenValue
		want  string
	}{
		{value: broker.ResearchScreenValue{String: &text}, want: "value"},
		{value: broker.ResearchScreenValue{Integer: &integer}, want: "2"},
		{value: broker.ResearchScreenValue{Number: &number}, want: "3.5"},
		{value: broker.ResearchScreenValue{Integers: []int64{4, 5}}, want: "[4,5]"},
	} {
		if got := researchScreenString(testCase.value); got != testCase.want {
			t.Fatalf("researchScreenString(%#v) = %q, want %q", testCase.value, got, testCase.want)
		}
	}

	property := map[string]any{
		"days": 1, "periodAverage": 2, "term": 3, "duration": 4,
		"year": 2026, "futureDuration": 5, "period": 11,
		"indicatorParams": []any{6}, "rangePeriod": 7,
		"firstCustomParam": 8, "param": "broker",
	}
	for _, key := range []string{
		"cumulative.price_change_pct", "financial.net_profit", "indicator.ma",
		"featured.chips_profit_ratio", "broker.concentrated_distribution",
		"option.stock_iv", "kline_shape.shape_type", "simple.price",
	} {
		researchScreenParamsFromProperty(mustResearchScreenFactor(t, key), property)
	}
	industry := mustResearchScreenFactor(t, "basic.industry")
	normalizedRow := normalizeStockScreenRow(broker.FeatureQuery{Market: "US"}, map[string]any{
		"results": []any{
			map[string]any{"simplePropertyResult": map[string]any{"property": map[string]any{}}},
			map[string]any{"simplePropertyResult": map[string]any{"property": map[string]any{"name": 999999}}},
			map[string]any{"basicPropertyResult": map[string]any{
				"property": map[string]any{"name": industry.ProviderID}, "valueType": 1, "sval": "Software",
			}},
		},
	})
	if normalizedRow["industry"] != "Software" {
		t.Fatalf("normalized industry row = %#v", normalizedRow)
	}

	if _, _, ok := stockScreenRequestedRef(broker.FeatureQuery{}, price, nil); ok {
		t.Fatal("request ref unexpectedly resolved without params")
	}
	if _, _, ok := stockScreenRequestedRef(broker.FeatureQuery{
		Params: map[string]any{researchScreenDefinitionParam: "invalid"},
	}, price, nil); ok {
		t.Fatal("request ref unexpectedly resolved from invalid definition")
	}
	definition := broker.ScreenDefinitionV2{
		Market: "US", CatalogVersion: researchscreen.CatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		Columns: []broker.ScreenColumn{{
			ID: "ma", Factor: broker.FactorRef{FactorKey: "indicator.ma", Params: broker.ResearchScreenFactorParams{Period: 11, IndicatorParams: []int64{20}}},
		}},
	}
	if ref, columnID, ok := stockScreenRequestedRef(broker.FeatureQuery{
		Params: map[string]any{researchScreenDefinitionParam: definition},
	}, mustResearchScreenFactor(t, "indicator.ma"), map[string]any{
		"period": 11, "indicatorParams": []any{60},
	}); !ok || ref.FactorKey != "indicator.ma" || columnID != "ma" {
		t.Fatalf("fallback request ref = %#v, %q, %v", ref, columnID, ok)
	}
	if _, _, ok := stockScreenRequestedRef(broker.FeatureQuery{
		Params: map[string]any{researchScreenDefinitionParam: definition},
	}, price, nil); ok {
		t.Fatal("unrequested factor unexpectedly resolved")
	}

	for _, testCase := range []struct {
		request string
		actual  string
		want    bool
	}{
		{request: "", actual: "US", want: true},
		{request: "CN", actual: "SH", want: true},
		{request: "CN", actual: "HK", want: false},
		{request: " us ", actual: "US", want: true},
		{request: "US", actual: "HK", want: false},
	} {
		if got := stockScreenMarketMatches(testCase.request, testCase.actual); got != testCase.want {
			t.Fatalf("market match %q/%q = %v, want %v", testCase.request, testCase.actual, got, testCase.want)
		}
	}
	if err := resolveStockScreenIdentities(context.Background(), nil,
		broker.FeatureQuery{Market: "US"}, map[string]any{}); err != nil {
		t.Fatalf("non-mainland identity resolution: %v", err)
	}
	if err := resolveStockScreenIdentities(context.Background(), nil,
		broker.FeatureQuery{Market: "CN"}, map[string]any{}); err != nil {
		t.Fatalf("empty mainland identity resolution: %v", err)
	}
	if err := resolveStockScreenIdentities(context.Background(), nil,
		broker.FeatureQuery{Market: "CN"}, map[string]any{"dataList": []any{map[string]any{}}}); err == nil {
		t.Fatal("mainland row without code unexpectedly resolved")
	}
	for _, market := range []string{"SH", "SZ", "CN"} {
		if !isMainlandStockScreenMarket(market) {
			t.Fatalf("%s should be a mainland market", market)
		}
	}
	if isMainlandStockScreenMarket("US") || isExactMainlandStockScreenMarket("CN") || !isExactMainlandStockScreenMarket("SH") {
		t.Fatal("mainland market classification failed")
	}

	duplicateRows := []map[string]any{
		stockScreenIdentityTestRow("1", "600000", "first"),
		stockScreenIdentityTestRow("2", "600000", "duplicate"),
		{"results": []any{"bad", map[string]any{"basicPropertyResult": "bad"}}},
	}
	if candidates := stockScreenIdentityCandidates(duplicateRows); len(candidates) != 2 {
		t.Fatalf("deduplicated identity candidates = %#v", candidates)
	}
	invalidID := int64(0)
	if err := applyStockScreenIdentities([]map[string]any{{"stockId": "1"}}, []*qotcommonpb.SecurityStaticInfo{
		nil,
		{Basic: &qotcommonpb.SecurityStaticBasic{Id: &invalidID}},
		{Basic: &qotcommonpb.SecurityStaticBasic{Id: func() *int64 { value := int64(1); return &value }()}},
	}); err == nil {
		t.Fatal("malformed identity data unexpectedly resolved")
	}
	invalidMarket := int32(999)
	invalidCode := "600000"
	invalidSecurityID := int64(2)
	if err := applyStockScreenIdentities([]map[string]any{{"stockId": "2"}}, []*qotcommonpb.SecurityStaticInfo{{
		Basic: &qotcommonpb.SecurityStaticBasic{
			Id:       &invalidSecurityID,
			Security: &qotcommonpb.Security{Market: &invalidMarket, Code: &invalidCode},
		},
	}}); err == nil {
		t.Fatal("invalid provider market unexpectedly resolved")
	}

	adapter := &futuAdapter{}
	for request := 1; request <= 2; request++ {
		if retry := adapter.researchScreenRetryAfter(nil); retry != 0 {
			t.Fatalf("adapter limiter request %d retry = %s", request, retry)
		}
	}
}

func mustResearchScreenFactor(t *testing.T, key string) researchscreen.FactorDescriptor {
	t.Helper()
	factor, ok := researchscreen.Lookup(key)
	if !ok {
		t.Fatalf("factor %q is missing from catalog", key)
	}
	return factor
}

func stockScreenIdentityTestRow(stockID, code, name string) map[string]any {
	return map[string]any{
		"stockId": stockID,
		"results": []any{
			map[string]any{"basicPropertyResult": map[string]any{
				"property":  map[string]any{"name": float64(1101)},
				"valueType": float64(1),
				"sval":      code,
			}},
			map[string]any{"basicPropertyResult": map[string]any{
				"property":  map[string]any{"name": float64(1102)},
				"valueType": float64(1),
				"sval":      name,
			}},
		},
	}
}
