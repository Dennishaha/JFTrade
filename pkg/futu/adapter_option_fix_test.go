package futu

import (
	"encoding/json"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
)

func TestFutuRootAdapterForwardsBatchSnapshotsThroughProtocol3203(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	server.setSecuritySnapshots([]*qotgetsecuritysnapshotpb.Snapshot{
		testTencentSecuritySnapshot(),
	})
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)

	result, err := adapter.QuerySecuritySnapshot(t.Context(), broker.SecuritySnapshotQuery{
		Symbols: []string{"HK.00700"},
	})
	if err != nil || result == nil || len(result.Snapshots) != 1 {
		t.Fatalf("root batch snapshot = %#v, %v", result, err)
	}
	if got := server.securitySnapshotCalls.Load(); got != 1 {
		t.Fatalf("protocol 3203 call count = %d, want 1", got)
	}
}

func TestFutuDeclaredCapabilitiesHaveExecutableAdapterInterfaces(t *testing.T) {
	adapter := NewBrokerAdapter(nil)
	seen := make(map[broker.FeatureID]struct{})
	for _, market := range adapter.Descriptor().Capabilities {
		for _, capability := range market.Features {
			if _, duplicate := seen[capability.ID]; duplicate {
				continue
			}
			seen[capability.ID] = struct{}{}
			definition, ok := broker.BuiltinCapabilityCatalog.Definition(capability.ID)
			if !ok {
				t.Errorf("declared feature %q is absent from catalog", capability.ID)
				continue
			}
			if !broker.ImplementsAdapterInterface(adapter, definition.AdapterInterface) {
				t.Errorf(
					"declared feature %q does not implement %s",
					capability.ID,
					definition.AdapterInterface,
				)
			}
		}
	}
}

func TestFutuOptionEventRequestsPassStrictOpenDValidation(t *testing.T) {
	cases := []struct {
		name       string
		protocol   string
		market     string
		instrument string
		params     map[string]any
		wantMarket int
		wantSeller int
	}{
		{
			name: "US equity unusual", protocol: "Qot_GetOptionEvent",
			market: "US", instrument: "US.BABA",
			params:     map[string]any{"underlyingProductClass": "equity"},
			wantMarket: 1,
		},
		{
			name: "US index zero dte", protocol: "Qot_GetOptionZeroDteScreener",
			market: "US", instrument: "US.SPX",
			params:     map[string]any{"underlyingProductClass": "index"},
			wantMarket: 2,
		},
		{
			name: "HK index earnings", protocol: "Qot_GetOptionEarningsScreener",
			market: "HK", instrument: "HK.HSI",
			params:     map[string]any{"underlyingProductClass": "index"},
			wantMarket: 4,
		},
		{
			name: "covered call", protocol: "Qot_GetOptionSellerScreener",
			market: "US", instrument: "US.BABA",
			params:     map[string]any{"sellerStrategy": "covered_call"},
			wantMarket: 1, wantSeller: 1,
		},
		{
			name: "cash secured put", protocol: "Qot_GetOptionSellerScreener",
			market: "US", instrument: "US.BABA",
			params:     map[string]any{"sellerStrategy": "cash_secured_put"},
			wantMarket: 1, wantSeller: 2,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			query := broker.FeatureQuery{
				Market: test.market, InstrumentID: test.instrument,
			}
			if err := injectAdvancedDefaults(test.params, test.protocol, query); err != nil {
				t.Fatalf("inject defaults: %v", err)
			}
			if got := int(floatParam(test.params["optionMarket"])); got != test.wantMarket {
				t.Fatalf("optionMarket = %d, want %d", got, test.wantMarket)
			}
			if test.wantSeller != 0 {
				if got := int(floatParam(test.params["sellerType"])); got != test.wantSeller {
					t.Fatalf("sellerType = %d, want %d", got, test.wantSeller)
				}
			}
			if err := opend.ValidateAdvancedC2S(test.protocol, test.params); err != nil {
				t.Fatalf("strict OpenD validation failed: %v params=%#v", err, test.params)
			}
		})
	}
}

func TestFutuZeroDteContractRebuildsBrokerNeutralChainContext(t *testing.T) {
	params := map[string]any{
		"expiryTimestamp": int64(1784332800),
		"chainLocator": broker.OptionZeroDteChainLocator{
			ProductCode: "BABA", Multiplier: 100, ContractSize: 100,
			ExpirationType: 2,
		},
		"sort": "open_interest", "optionType": "put",
	}
	query := broker.FeatureQuery{Market: "US", InstrumentID: "US.BABA"}
	if err := injectAdvancedDefaults(params, "Qot_GetOptionZeroDteContract", query); err != nil {
		t.Fatalf("inject 0DTE contract: %v", err)
	}
	if err := opend.ValidateAdvancedC2S("Qot_GetOptionZeroDteContract", params); err != nil {
		t.Fatalf("strict 0DTE contract validation: %v params=%#v", err, params)
	}
	chain, ok := params["chainInfo"].(map[string]any)
	if !ok || chain["productCode"] != "BABA" ||
		floatParam(chain["contractShareSize"]) != 100 {
		t.Fatalf("rebuilt chainInfo = %#v", params["chainInfo"])
	}
	if params["sortType"] != 2 {
		t.Fatalf("sortType = %#v, want 2", params["sortType"])
	}

	normalized := featureResultFromProtocolPayload(
		broker.FeatureQuery{FeatureID: broker.FeatureOptionEvents},
		"Qot_GetOptionZeroDteScreener",
		map[string]any{"itemList": []any{map[string]any{
			"owner": map[string]any{"market": "QotMarket_US", "code": "BABA"},
			"chainInfo": map[string]any{
				"strikeDateTimestamp": "1784332800",
				"productCode":         "BABA",
				"multiplier":          100.0,
				"contractShareSize":   100.0,
				"expirationType":      2.0,
			},
		}}},
	)
	contextValue, ok := normalized.Entries[0]["drilldownContext"].(map[string]any)
	if !ok || contextValue["underlyingInstrumentId"] != "US.BABA" {
		t.Fatalf("normalized drilldown context = %#v", normalized.Entries[0])
	}
	if _, leaked := normalized.Entries[0]["chainInfo"]; leaked {
		t.Fatal("raw OpenD chainInfo leaked into public entry")
	}
}

func TestFutuOptionEventValidationRejectsUnsupportedInputs(t *testing.T) {
	for _, test := range []struct {
		protocol string
		query    broker.FeatureQuery
		params   map[string]any
	}{
		{
			protocol: "Qot_GetOptionZeroDteScreener",
			query:    broker.FeatureQuery{Market: "HK", InstrumentID: "HK.00700"},
			params:   map[string]any{},
		},
		{
			protocol: "Qot_GetOptionSellerScreener",
			query:    broker.FeatureQuery{Market: "US", InstrumentID: "US.BABA"},
			params:   map[string]any{"sellerStrategy": "unknown"},
		},
		{
			protocol: "Qot_GetOptionZeroDteContract",
			query:    broker.FeatureQuery{Market: "US", InstrumentID: "US.BABA"},
			params:   map[string]any{},
		},
		{
			protocol: "Qot_GetOptionEvent",
			query:    broker.FeatureQuery{Market: "SH", InstrumentID: "SH.600000"},
			params:   map[string]any{"underlyingProductClass": "equity"},
		},
		{
			protocol: "Qot_GetOptionEvent",
			query:    broker.FeatureQuery{Market: "US", InstrumentID: "invalid"},
			params:   map[string]any{"underlyingProductClass": "equity"},
		},
	} {
		if err := injectAdvancedDefaults(test.params, test.protocol, test.query); err == nil {
			t.Errorf("%s accepted unsupported params %#v", test.protocol, test.params)
		}
	}
}

func TestFutuOptionRequestTranslationBoundaries(t *testing.T) {
	if _, err := futuOptionMarket("SH", "equity"); err == nil {
		t.Fatal("unsupported option market was accepted")
	}
	if _, err := optionSecurityMap("missing-market-prefix"); err == nil {
		t.Fatal("invalid option security was accepted")
	}

	params := map[string]any{"filterList": []any{"existing"}}
	appendOptionFilter(params, 1, map[string]any{"valueList": []any{1}})
	if got := len(params["filterList"].([]any)); got != 2 {
		t.Fatalf("appended filter count = %d, want 2", got)
	}
	params["filterList"] = "legacy"
	appendOptionFilter(params, 1, map[string]any{"valueList": []any{2}})
	if got := len(params["filterList"].([]any)); got != 2 {
		t.Fatalf("wrapped filter count = %d, want 2", got)
	}

	mapLocator, err := optionZeroDteLocator(map[string]any{
		"productCode": "BABA", "multiplier": float32(100),
		"contractSize": 100, "expirationType": int32(2),
	})
	if err != nil || mapLocator.ProductCode != "BABA" || mapLocator.ExpirationType != 2 {
		t.Fatalf("map locator = %#v, %v", mapLocator, err)
	}
	if _, err := optionZeroDteLocator(make(chan int)); err == nil {
		t.Fatal("unmarshalable locator was accepted")
	}
	if _, err := optionZeroDteLocator("not-an-object"); err == nil {
		t.Fatal("non-object locator was accepted")
	}

	for input, want := range map[string]int{
		"": 0, "default": 0, "volume": 1, "open_interest": 2, "iv": 3, "delta": 4,
	} {
		got, sortErr := zeroDteContractSort(input)
		if sortErr != nil || got != want {
			t.Fatalf("sort %q = %d, %v; want %d", input, got, sortErr, want)
		}
	}
	if _, err := zeroDteContractSort("unsupported"); err == nil {
		t.Fatal("unsupported 0DTE sort was accepted")
	}
	for input, want := range map[string]int{"": 0, "all": 0, "call": 1, "put": 2} {
		got, typeErr := zeroDteOptionType(input)
		if typeErr != nil || got != want {
			t.Fatalf("option type %q = %d, %v; want %d", input, got, typeErr, want)
		}
	}
	if _, err := zeroDteOptionType("unsupported"); err == nil {
		t.Fatal("unsupported 0DTE option type was accepted")
	}
}

func TestFutuZeroDteContractTranslationRejectsEachInvalidBoundary(t *testing.T) {
	validLocator := broker.OptionZeroDteChainLocator{
		ProductCode: "BABA", Multiplier: 100, ContractSize: 100,
	}
	cases := []struct {
		name   string
		query  broker.FeatureQuery
		params map[string]any
	}{
		{
			name: "HK market",
			query: broker.FeatureQuery{
				Market: "HK", InstrumentID: "HK.00700",
			},
			params: map[string]any{},
		},
		{
			name: "invalid owner",
			query: broker.FeatureQuery{
				Market: "US", InstrumentID: "invalid",
			},
			params: map[string]any{},
		},
		{
			name: "invalid locator",
			query: broker.FeatureQuery{
				Market: "US", InstrumentID: "US.BABA",
			},
			params: map[string]any{"chainLocator": make(chan int)},
		},
		{
			name: "missing product code",
			query: broker.FeatureQuery{
				Market: "US", InstrumentID: "US.BABA",
			},
			params: map[string]any{
				"expiryTimestamp": int64(1),
				"chainLocator":    broker.OptionZeroDteChainLocator{},
			},
		},
		{
			name: "unsupported sort",
			query: broker.FeatureQuery{
				Market: "US", InstrumentID: "US.BABA",
			},
			params: map[string]any{
				"expiryTimestamp": int64(1), "chainLocator": validLocator,
				"sort": "unsupported",
			},
		},
		{
			name: "unsupported option type",
			query: broker.FeatureQuery{
				Market: "US", InstrumentID: "US.BABA",
			},
			params: map[string]any{
				"expiryTimestamp": int64(1), "chainLocator": validLocator,
				"optionType": "unsupported",
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := injectZeroDteContractParams(test.params, test.query); err == nil {
				t.Fatalf("invalid boundary accepted: %#v", test.params)
			}
		})
	}
}

func TestFutuOptionNumericParameterBoundaries(t *testing.T) {
	for input, want := range map[any]int64{
		int(1): 1, int32(2): 2, int64(3): 3, float64(4): 4,
		json.Number("5"): 5, "6": 6,
	} {
		got, ok := int64Param(input)
		if !ok || got != want {
			t.Fatalf("int64Param(%#v) = %d, %v; want %d", input, got, ok, want)
		}
	}
	for _, input := range []any{json.Number("bad"), "bad", true} {
		if _, ok := int64Param(input); ok {
			t.Fatalf("int64Param(%#v) unexpectedly succeeded", input)
		}
	}
	for input, want := range map[any]float64{
		float64(1): 1, float32(2): 2, int(3): 3, int32(4): 4,
		int64(5): 5, json.Number("6"): 6, "7": 7,
	} {
		if got := floatParam(input); got != want {
			t.Fatalf("floatParam(%#v) = %v, want %v", input, got, want)
		}
	}
	if got := floatParam(true); got != 0 {
		t.Fatalf("floatParam(true) = %v, want 0", got)
	}
	if got := int32Param("8"); got != 8 {
		t.Fatalf("int32Param = %d, want 8", got)
	}
}

func TestFutuZeroDteNormalizationHandlesAbsentAndNestedUnderlying(t *testing.T) {
	result := optionZeroDteFeatureResult(
		broker.FeatureQuery{FeatureID: broker.FeatureOptionEvents},
		map[string]any{"itemList": []any{
			map[string]any{"name": "without-chain"},
			map[string]any{
				"chainInfo": map[string]any{
					"underlying":          map[string]any{"instrumentId": "US.BABA"},
					"strikeDateTimestamp": int64(1),
				},
			},
		}},
	)
	if len(result.Entries) != 2 {
		t.Fatalf("normalized entry count = %d, want 2", len(result.Entries))
	}
	contextValue, ok := result.Entries[1]["drilldownContext"].(map[string]any)
	if !ok || contextValue["underlyingInstrumentId"] != "US.BABA" {
		t.Fatalf("nested underlying context = %#v", result.Entries[1])
	}
}
