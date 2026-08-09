package futu

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	updateklinepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractkline"
	updateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractorderbook"
	updatetickerpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractticker"
)

func TestFutuPredictionStreamListenersSequencesAndMalformedPushes(t *testing.T) {
	adapter := NewBrokerAdapter(nil).(*futuAdapter)
	noop := adapter.OnPredictionMarketUpdate(nil)
	noop()
	var updates []broker.PredictionMarketUpdate
	remove := adapter.OnPredictionMarketUpdate(func(update broker.PredictionMarketUpdate) {
		updates = append(updates, update)
	})

	adapter.emitPredictionPush("ORDER_BOOK", "Qot_GetEventContractOrderBook", map[string]any{
		"orderBookList": []any{
			map[string]any{
				"code": map[string]any{"market": "QotMarket_Event", "code": "EC.BOOK"},
			},
			map[string]any{"code": map[string]any{}},
		},
	})
	adapter.emitPredictionPush("TICKER", "Qot_GetEventContractTicker", map[string]any{
		"tickerList": []any{map[string]any{
			"contractSecurity": map[string]any{
				"market": "QotMarket_Event", "code": "EC.TICK",
			},
			"tickerList": []any{map[string]any{"sequence": "12"}},
		}},
	})
	adapter.emitPredictionPush("KLINE", "Qot_GetEventContractKline", map[string]any{
		"klineList": []any{map[string]any{
			"code": map[string]any{"market": "QotMarket_Event", "code": "EC.KLINE"},
			"klineList": []any{map[string]any{"sequence": nil, "timeKey": "2026-07-18 10:00:00"}},
		}},
	})
	adapter.emitPredictionPush("TICKER", "Qot_GetEventContractTicker", make(chan int))
	adapter.handlePredictionOrderBookPush(&updateorderbookpb.S2C{})
	adapter.handlePredictionKlinePush(&updateklinepb.S2C{})
	adapter.handlePredictionTickerPush(&updatetickerpb.S2C{})
	if len(updates) != 3 ||
		updates[0].InstrumentID != "US.EC.BOOK" ||
		updates[1].Sequence != "12" ||
		updates[2].Sequence != "2026-07-18 10:00:00" {
		t.Fatalf("prediction updates = %#v", updates)
	}
	remove()
	adapter.emitPredictionPush("ORDER_BOOK", "Qot_GetEventContractOrderBook", map[string]any{
		"orderBookList": []any{map[string]any{
			"code": map[string]any{"market": "QotMarket_Event", "code": "EC.REMOVED"},
		}},
	})
	if len(updates) != 3 {
		t.Fatal("removed prediction listener still received updates")
	}

	if got := predictionEntryInstrumentID(map[string]any{
		"contractSecurity": map[string]any{"instrumentId": "US.EC.DIRECT"},
	}); got != "US.EC.DIRECT" {
		t.Fatalf("contract-security instrument = %q", got)
	}
	if predictionEntryInstrumentID(map[string]any{}) != "" {
		t.Fatal("empty prediction entry resolved an instrument")
	}
	for _, test := range []struct {
		dataType string
		entry    map[string]any
		want     string
	}{
		{"TICKER", map[string]any{"tickerList": []any{map[string]any{"time": "10"}}}, "10"},
		{"KLINE", map[string]any{"klineList": []any{"invalid"}}, ""},
		{"ORDER_BOOK", map[string]any{}, ""},
	} {
		if got := predictionEntrySequence(test.dataType, test.entry); got != test.want {
			t.Errorf("sequence %s = %q, want %q", test.dataType, got, test.want)
		}
	}
}

func TestFutuPredictionPushHandlerInstallationReplayAndFailure(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	if err := adapter.ensurePredictionPushHandlers(t.Context(), nil); err != nil {
		t.Fatalf("nil client handlers: %v", err)
	}
	adapter.predictionSubscriptions["one"] = broker.PredictionSubscription{
		InstrumentID: "US.EC.ONE", DataTypes: []string{"TICKER"},
	}
	adapter.predictionSubscriptions["two"] = broker.PredictionSubscription{
		InstrumentID: "US.EC.TWO", DataTypes: []string{"ORDER_BOOK"},
	}
	var client *opend.Client
	if err := adapter.exchange.withClient(t.Context(), func(value *opend.Client) error {
		client = value
		return nil
	}); err != nil {
		t.Fatalf("acquire OpenD client: %v", err)
	}
	adapter.predictionStreamClients = make(map[*opend.Client]struct{})
	if err := adapter.ensurePredictionPushHandlers(t.Context(), client); err != nil {
		t.Fatalf("install prediction handlers: %v", err)
	}
	if err := adapter.ensurePredictionPushHandlers(t.Context(), client); err != nil {
		t.Fatalf("idempotent prediction handlers: %v", err)
	}

	invalid := newTestBrokerAdapter(t, server).(*futuAdapter)
	invalid.predictionSubscriptions["invalid"] = broker.PredictionSubscription{}
	if err := invalid.exchange.withClient(t.Context(), func(value *opend.Client) error {
		invalid.predictionStreamClients = make(map[*opend.Client]struct{})
		return invalid.ensurePredictionPushHandlers(t.Context(), value)
	}); err == nil {
		t.Fatal("invalid replay subscription succeeded")
	}

	failingServer := startQuoteOpenDServer(t)
	defer failingServer.stop()
	failingServer.setDropProto(opend.ProtoQotSubEventContract)
	failing := newTestBrokerAdapter(t, failingServer).(*futuAdapter)
	failing.predictionSubscriptions["failing"] = broker.PredictionSubscription{
		InstrumentID: "US.EC.FAIL", DataTypes: []string{"ORDER_BOOK"},
	}
	if err := failing.exchange.withClient(t.Context(), func(value *opend.Client) error {
		failing.predictionStreamClients = make(map[*opend.Client]struct{})
		return failing.ensurePredictionPushHandlers(t.Context(), value)
	}); err == nil {
		t.Fatal("prediction subscription replay transport failure was hidden")
	}
}

func TestFutuPredictionNormalizationCatalogPaginationAndIdentity(t *testing.T) {
	protocols := map[string]string{
		"Qot_GetEventContractCategory":      "categoryList",
		"Qot_FilterCompetition":             "competitionList",
		"Qot_GetEventContractSeriesList":    "seriesList",
		"Qot_GetEventContractEventList":     "eventList",
		"Qot_GetEventContract":              "contractList",
		"Qot_GetEventContractMilestoneList": "milestoneList",
		"Qot_GetEventContractSnapshot":      "snapshotList",
		"Qot_GetEventContractOrderBook":     "orderBookList",
		"Qot_GetEventContractKline":         "klineList",
		"Qot_RequestHistoryEventContractKL": "klineList",
		"Qot_GetEventContractTicker":        "tickerList",
		"Qot_GetEventContractComboList":     "eventList",
		"Qot_GetEventContractComboRfq":      "comboLegList",
		"unknown":                           "",
	}
	for protocol, want := range protocols {
		if got := predictionListKey(protocol); got != want {
			t.Errorf("predictionListKey(%s) = %q", protocol, got)
		}
	}
	if !isPredictionProtocol("Qot_GetEventContract") ||
		!isPredictionProtocol("Qot_FilterCompetition") ||
		isPredictionProtocol("Qot_GetOptionChain") {
		t.Fatal("prediction protocol classification changed")
	}

	for _, test := range []struct {
		value any
		want  int
		ok    bool
	}{
		{float64(1), 1, true}, {int(2), 2, true},
		{int32(3), 3, true}, {int64(4), 4, true},
		{"5", 5, true}, {"bad", 0, false}, {nil, 0, false},
	} {
		got, ok := integerValue(test.value)
		if got != test.want || ok != test.ok {
			t.Errorf("integerValue(%#v) = %d, %v", test.value, got, ok)
		}
	}
	result := &broker.FeatureResult{}
	setPagination(result, map[string]any{"nextKey": "next", "totalCount": int32(7)}, 2)
	if result.NextCursor != "next" || result.HasMore == nil || !*result.HasMore ||
		result.Total == nil || *result.Total != 7 {
		t.Fatalf("pagination = %#v", result)
	}
	setPagination(result, map[string]any{"allCount": int64(8)}, 2)
	setPagination(result, map[string]any{"total": "9"}, 2)
	if result.Total == nil || *result.Total != 9 {
		t.Fatalf("string pagination total = %#v", result.Total)
	}

	query := broker.FeatureQuery{
		Market: "US", InstrumentID: "US.EC.ONE",
		MarketSegment: broker.MarketSegmentPrediction,
		ProductClass:  broker.ProductClassEventContract,
	}
	entry := map[string]any{
		"contractSecurity": map[string]any{"code": "ec.two"},
		"name":             "Contract two",
		"eventSecurity":    map[string]any{"instrumentId": "US.EVENT.TWO"},
		"seriesSecurity":   map[string]any{"instrumentId": "US.SERIES.TWO"},
		"status":           "active",
		"tickSize":         "0.02",
	}
	instrument := resolvedPredictionInstrument(query, "Qot_GetEventContractSnapshot", []map[string]any{entry})
	if instrument == nil || instrument.InstrumentID != "US.EC.TWO" ||
		instrument.PriceTick == nil || *instrument.PriceTick != 0.02 ||
		instrument.Event == nil || instrument.Event.EventID != "US.EVENT.TWO" {
		t.Fatalf("prediction instrument = %#v", instrument)
	}
	entry["eventCode"] = map[string]any{"instrumentId": "US.EVENT.DIRECT"}
	entry["tickSize"] = "invalid"
	instrument = resolvedPredictionInstrument(query, "Qot_GetEventContractSnapshot", []map[string]any{entry})
	if instrument == nil || *instrument.PriceTick != 0.01 ||
		instrument.Event.EventID != "US.EVENT.DIRECT" {
		t.Fatalf("prediction defaults = %#v", instrument)
	}
	for _, protocol := range []string{
		"Qot_GetEventContractCategory", "Qot_FilterCompetition",
		"Qot_GetEventContractSeriesList", "Qot_GetEventContractEventList",
	} {
		if resolvedPredictionInstrument(query, protocol, nil) != nil {
			t.Errorf("%s resolved a contract instrument", protocol)
		}
	}
	if resolvedPredictionInstrument(broker.FeatureQuery{}, "Qot_GetEventContractSnapshot", nil) != nil ||
		resolvedPredictionInstrument(broker.FeatureQuery{
			MarketSegment: broker.MarketSegmentPrediction,
		}, "Qot_GetEventContractSnapshot", nil) != nil {
		t.Fatal("invalid prediction identity resolved")
	}
	if securityInstrumentID(nil) != "" ||
		securityInstrumentID(map[string]any{"instrumentId": "US.EC.ONE"}) != "US.EC.ONE" {
		t.Fatal("security instrument normalization changed")
	}
}

func TestFutuAdvancedSecurityNormalizationAllPublicMarkets(t *testing.T) {
	cases := map[string]string{
		"QotMarket_Event":       "US",
		"QotMarket_Prediction":  "US",
		"QotMarket_US_Security": "US",
		"QotMarket_HK_Security": "HK",
		"QotMarket_SH_Security": "SH",
		"QotMarket_SZ_Security": "SZ",
	}
	for market, expected := range cases {
		value, ok := normalizeOpenDSecurity(map[string]any{
			"market": market, "code": "test",
		})
		if !ok || value["market"] != expected {
			t.Errorf("normalize security %s = %#v, %v", market, value, ok)
		}
	}
	if value, ok := normalizeOpenDSecurity(map[string]any{
		"market": float64(101), "code": "event",
	}); !ok || value["market"] != "US" {
		t.Fatalf("numeric prediction market = %#v, %v", value, ok)
	}
	for _, value := range []map[string]any{
		{"market": "QotMarket_US"}, {"code": "AAPL"},
		{"market": "QotMarket_Unknown", "code": "AAPL"},
	} {
		if _, ok := normalizeOpenDSecurity(value); ok {
			t.Errorf("invalid security normalized: %#v", value)
		}
	}
}
