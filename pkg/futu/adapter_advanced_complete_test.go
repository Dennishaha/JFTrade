package futu

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	eventcontractsnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgeteventcontractsnapshot"
	optionstrategyspreadpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetoptionstrategyspread"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestFutuAdvancedAdapterReaderSurfaceAndPredictionSubscriptions(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	ctx := t.Context()

	categoryQuery := broker.FeatureQuery{
		BrokerID: "futu", Market: "US", FeatureID: broker.FeaturePredictionDiscover,
		Params: map[string]any{"operation": "categories"},
	}
	if result, err := adapter.QueryPredictionMarket(ctx, categoryQuery); err != nil || result.Entries == nil {
		t.Fatalf("prediction categories = %#v, %v", result, err)
	}
	if _, err := adapter.QueryPredictionMarket(ctx, broker.FeatureQuery{
		Market: "HK", FeatureID: broker.FeaturePredictionDiscover,
	}); err == nil {
		t.Fatal("HK prediction query succeeded")
	}

	calls := []struct {
		name string
		call func() (*broker.FeatureResult, error)
	}{
		{"microstructure", func() (*broker.FeatureResult, error) {
			return adapter.QueryMarketMicrostructure(ctx, broker.FeatureQuery{
				Market: "US", InstrumentID: "US.AAPL", FeatureID: broker.FeatureMarketIntraday,
				Params: map[string]any{},
			})
		}},
		{"search", func() (*broker.FeatureResult, error) {
			return adapter.QueryInstrumentProfile(ctx, broker.FeatureQuery{
				Market: "US", FeatureID: broker.FeatureMarketSearch, PageSize: 10,
				Params: map[string]any{"keyword": "Apple"},
			})
		}},
		{"profile", func() (*broker.FeatureResult, error) {
			return adapter.QueryInstrumentProfile(ctx, categoryQuery)
		}},
		{"derivatives", func() (*broker.FeatureResult, error) {
			return adapter.QueryDerivativeCatalog(ctx, categoryQuery)
		}},
		{"option analytics", func() (*broker.FeatureResult, error) {
			return adapter.QueryOptionAnalytics(ctx, categoryQuery)
		}},
		{"instrument research", func() (*broker.FeatureResult, error) {
			return adapter.QueryInstrumentResearch(ctx, categoryQuery)
		}},
		{"market research", func() (*broker.FeatureResult, error) {
			return adapter.QueryMarketResearch(ctx, categoryQuery)
		}},
		{"technical indicator", func() (*broker.FeatureResult, error) {
			return adapter.QueryTechnicalIndicator(ctx, categoryQuery)
		}},
		{"customization read", func() (*broker.FeatureResult, error) {
			return adapter.QueryCustomization(ctx, broker.FeatureQuery{
				FeatureID: broker.FeatureRemoteWatchlistList,
			})
		}},
	}
	for _, test := range calls {
		if _, err := test.call(); err != nil {
			t.Errorf("%s: %v", test.name, err)
		}
	}
	if _, err := adapter.QueryMarketMicrostructure(ctx, broker.FeatureQuery{
		Market: "US", InstrumentID: "US.AAPL", FeatureID: broker.FeatureMarketDepth,
		Params: map[string]any{"num": float64(5)},
	}); err == nil {
		t.Fatal("depth query without a visible subscription lease succeeded")
	}

	subscription := broker.PredictionSubscription{
		InstrumentID: "US.EVENT.ONE",
		DataTypes:    []string{"ORDER_BOOK", "KLINE", "TICKER"},
	}
	if err := adapter.SubscribePredictionMarket(ctx, subscription); err != nil {
		t.Fatalf("subscribe prediction: %v", err)
	}
	if server.predictionSubCalls.Load() != 1 {
		t.Fatalf("prediction subscribe calls = %d", server.predictionSubCalls.Load())
	}
	adapter.exchange.invalidateClient()
	if _, err := adapter.QueryPredictionMarket(ctx, categoryQuery); err != nil {
		t.Fatalf("prediction query after reconnect: %v", err)
	}
	if server.predictionSubCalls.Load() != 2 {
		t.Fatalf("prediction subscription was not replayed after reconnect: calls=%d", server.predictionSubCalls.Load())
	}
	if err := adapter.UnsubscribePredictionMarket(ctx, subscription); err != nil {
		t.Fatalf("unsubscribe prediction: %v", err)
	}
	if server.predictionSubCalls.Load() != 3 || len(adapter.predictionSubscriptions) != 0 {
		t.Fatalf("prediction unsubscribe lifecycle calls=%d active=%d",
			server.predictionSubCalls.Load(), len(adapter.predictionSubscriptions))
	}
	if err := adapter.SubscribePredictionMarket(ctx, broker.PredictionSubscription{}); err == nil {
		t.Fatal("blank prediction subscription succeeded")
	}
	if err := adapter.SubscribePredictionMarket(ctx, broker.PredictionSubscription{
		InstrumentID: "EVENT", DataTypes: []string{"BAD"},
	}); err == nil {
		t.Fatal("unknown prediction subscription type succeeded")
	}

	if _, err := adapter.ApplyCustomization(ctx, broker.CustomizationAction{
		FeatureID: broker.FeaturePriceAlertSet,
		Action:    "unsupported",
		Payload:   map[string]any{"value": 1},
	}); err == nil {
		t.Fatal("unsupported customization action succeeded")
	}
}

func TestFutuAdvancedAdapterProtocolValidationDefaultsAndPayloadHelpers(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	ctx := t.Context()

	if _, err := adapter.queryAdvancedFeature(ctx, broker.FeatureQuery{
		FeatureID: "unknown", Params: map[string]any{},
	}); err == nil {
		t.Fatal("unmapped feature succeeded")
	}
	if _, err := adapter.queryAdvancedFeatureWithProtocols(ctx, broker.FeatureQuery{
		FeatureID: broker.FeaturePredictionDiscover, Params: map[string]any{"operation": "bad"},
	}, featureProtocols[broker.FeaturePredictionDiscover]); err == nil {
		t.Fatal("unsupported feature operation succeeded")
	}
	if _, err := adapter.queryAdvancedFeatureWithProtocols(ctx, broker.FeatureQuery{
		Market: "XX", FeatureID: broker.FeaturePredictionDiscover,
		Params: map[string]any{"operation": "categories"},
	}, featureProtocols[broker.FeaturePredictionDiscover]); err == nil {
		t.Fatal("unsupported market succeeded")
	}
	if err := injectAdvancedProtocolDefaults(
		map[string]any{}, "Qot_GetSearchNews",
		broker.FeatureQuery{FeatureID: broker.FeatureResearchNews},
	); err == nil {
		t.Fatal("news defaults accepted no keyword")
	}

	defaultCases := []struct {
		protocol string
		market   string
	}{
		{"Qot_GetOptionChain", "US"},
		{"Qot_OptionScreen", "HK"},
		{"Qot_GetOptionEvent", "HK"},
		{"Qot_GetOptionMarketStatistic", "HK"},
		{"Qot_GetOptionUnderlyingHisStatistic", "US"},
		{"Qot_GetOptionUnderlyingHisVolatility", "US"},
		{"Qot_GetWarrant", "HK"},
		{"Qot_WarrantScreen", "HK"},
		{"Qot_GetMacroIndicatorList", "SH"},
		{"Qot_GetEventContractKline", "US"},
	}
	for _, test := range defaultCases {
		params := map[string]any{}
		if err := injectAdvancedProtocolDefaults(params, test.protocol, broker.FeatureQuery{
			Market: test.market, InstrumentID: test.market + ".TEST",
		}); err != nil || len(params) == 0 {
			t.Errorf("defaults %s = %#v, %v", test.protocol, params, err)
		}
	}
	if macroRegion("HK") != 1 || macroRegion("SZ") != 8 || macroRegion("US") != 2 {
		t.Fatal("macro region mapping mismatch")
	}

	cursorParams := map[string]any{}
	injectAdvancedCursor(cursorParams, "Qot_GetEventContractComboList", "next")
	pageParams := map[string]any{}
	injectAdvancedPageSize(pageParams, "Qot_GetEventContractComboList", 999)
	if len(cursorParams) == 0 && len(pageParams) == 0 {
		t.Fatal("advanced pagination fields were not injected")
	}
	existing := map[string]any{"securityList": "existing"}
	if err := injectFeatureInstrument(existing, "Qot_GetEventContractSnapshot", "US.EVENT"); err != nil ||
		existing["securityList"] != "existing" {
		t.Fatalf("existing instrument field = %#v, %v", existing, err)
	}
	if err := injectFeatureInstrument(map[string]any{}, "Qot_GetOptionQuote", "US.AAPL"); err != nil {
		t.Fatalf("option multi-leg instrument: %v", err)
	}
	if err := injectFeatureInstrument(map[string]any{}, "Qot_GetRT", "INVALID"); err == nil {
		t.Fatal("invalid instrument succeeded")
	}
	if err := injectFeatureInstrument(map[string]any{}, "Qot_GetEventContract", "US.EVENT"); err != nil {
		t.Fatalf("event instrument: %v", err)
	}

	payload := map[string]any{
		"snapshotList": []any{
			map[string]any{"status": "EC_Status_Active"},
			"discarded",
		},
		"nextPage": "cursor-2",
	}
	result := featureResultFromPayload(broker.FeatureQuery{
		FeatureID: broker.FeaturePredictionComboQuote,
	}, payload)
	if len(result.Entries) != 1 || result.NextCursor != "cursor-2" ||
		len(result.Warnings) != 1 || result.Metadata["quoteExpiresAt"] == nil {
		t.Fatalf("payload result = %#v", result)
	}
	entries, metadata := payloadEntries(map[string]any{"z": 1})
	if len(entries) != 1 || metadata != nil {
		t.Fatalf("object payload entries=%#v metadata=%#v", entries, metadata)
	}
	entries, metadata = payloadEntries(nil)
	if len(entries) != 0 || metadata != nil {
		t.Fatalf("empty payload entries=%#v metadata=%#v", entries, metadata)
	}
	entries, metadata = payloadEntries(map[string]any{
		"emptyFirst": []any{},
		"rows":       []any{map[string]any{"id": 7}},
	})
	if len(entries) != 1 || entries[0]["id"] != 7 ||
		metadata["emptyFirst"] == nil {
		t.Fatalf("multi-list payload entries=%#v metadata=%#v", entries, metadata)
	}
	if defaultOperation(map[string]string{"b": "B", "a": "A"}) != "a" ||
		defaultOperation(map[string]string{"only": "P"}) != "only" {
		t.Fatal("default operation selection mismatch")
	}

	normalized := normalizeOpenDMap(map[string]any{
		"market": "QotMarket_US",
		"nested": []any{map[string]any{"status": "EC_Status_Active"}},
	})
	if normalized["market"] != "us" {
		t.Fatalf("normalized OpenD map = %#v", normalized)
	}
	for _, value := range []string{
		"SecurityType_SecurityType_Option", "OptionType_OptionType_Call",
		"IndexOptionType_IndexOptionType_Unknown", "ExpirationCycle_ExpirationCycle_Weekly",
		"PredSide_PredSide_Yes", "TrdSide_TrdSide_Buy", "OrderStatus_OrderStatus_Submitted",
		"TrdEnv_TrdEnv_Real", "TrdMarket_TrdMarket_US", "KLType_KLType_Day",
		"RehabType_RehabType_Forward",
	} {
		if normalizeOpenDEnum(value) == value {
			t.Errorf("enum %q was not normalized", value)
		}
	}
	if normalizeOpenDEnum("plain") != "plain" || numberValue(1, 3) != 3 ||
		numberValue(float64(2), 3) != 2 || stringValue(1) != "" {
		t.Fatal("normalization scalar helper mismatch")
	}
	cloned := cloneMap(map[string]any{"a": 1})
	if cloned["a"] != 1 {
		t.Fatalf("clone map = %#v", cloned)
	}
	if _, err := structMap(make(chan int)); err == nil {
		t.Fatal("structMap marshaled channel")
	}
	if _, err := structMap("scalar"); err == nil {
		t.Fatal("structMap decoded scalar")
	}
}

func TestFutuComboAdapterCompleteOptionAndEventLifecycle(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	account := testSimulateHKCashAccount()
	account.TrdMarketAuthList = []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)}
	server.setAccounts([]*trdcommonpb.TrdAcc{account})
	active := qotcommonpb.EC_Status_EC_Status_Active
	server.setAdvancedResponse(3445, &eventcontractsnapshotpb.Response{
		RetType: new(int32(0)),
		S2C: &eventcontractsnapshotpb.S2C{SnapshotList: []*eventcontractsnapshotpb.SnapshotItem{
			{Code: &qotcommonpb.Security{Market: new(int32(101)), Code: new("EVENT.ONE")}, Status: &active},
			{Code: &qotcommonpb.Security{Market: new(int32(101)), Code: new("EVENT.TWO")}, Status: &active},
		}},
	})
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	ctx := t.Context()
	quantity := 2.0
	spread := 10.0
	server.setAdvancedResponse(3258, &optionstrategyspreadpb.Response{
		RetType: new(int32(0)),
		S2C:     &optionstrategyspreadpb.S2C{SpreadList: []float64{spread}},
	})
	option := broker.ComboOrderIntent{
		ReadQuery:     broker.ReadQuery{AccountID: "1001", Market: "US", TradingEnvironment: "SIMULATE"},
		ClientOrderID: "option-client", PreviewID: "preview-option",
		OrderKind: broker.OrderKindOptionCombo, ProductClass: broker.ProductClassOption,
		UnderlyingID: "US.AAPL", OptionStrategy: "vertical",
		NearExpiry: "2026-07-17", Spread: &spread,
		Legs: []broker.OrderLegIntent{
			{InstrumentID: "US.OPTION.ONE", ProductClass: broker.ProductClassOption, Side: "BUY", Ratio: 1, Quantity: &quantity},
			{InstrumentID: "US.OPTION.TWO", ProductClass: broker.ProductClassOption, Side: "SELL", Ratio: 1, Quantity: &quantity},
		},
	}
	if preview, err := adapter.PreviewComboOrder(ctx, option); err != nil || !preview.Allowed {
		t.Fatalf("option combo preview = %#v, %v", preview, err)
	}
	placed, err := adapter.PlaceComboOrder(ctx, option)
	if err != nil || placed.Status != "SUBMITTED" || len(placed.Legs) != 2 {
		t.Fatalf("option combo place = %#v, %v", placed, err)
	}
	if err := adapter.CancelComboOrder(ctx, option.ReadQuery, "broker-combo"); err != nil {
		t.Fatalf("option combo cancel: %v", err)
	}

	amount := 20.0
	price := 0.6
	expires := time.Now().Add(time.Minute)
	event := broker.ComboOrderIntent{
		ReadQuery:     broker.ReadQuery{AccountID: "1001", Market: "US", TradingEnvironment: "SIMULATE"},
		ClientOrderID: "event-client", PreviewID: "preview-event",
		RFQID: "rfq-1", QuoteExpiresAt: &expires, Amount: &amount, Price: &price,
		OrderKind: broker.OrderKindEventParlay, ProductClass: broker.ProductClassEventContract,
		Legs: []broker.OrderLegIntent{
			{InstrumentID: "US.EVENT.ONE", ProductClass: broker.ProductClassEventContract, Side: "BUY", Ratio: 1, PredictionSide: "YES", Amount: &amount, Price: &price},
			{InstrumentID: "US.EVENT.TWO", ProductClass: broker.ProductClassEventContract, Side: "BUY", Ratio: 1, PredictionSide: "NO", Amount: &amount, Price: &price},
		},
	}
	if preview, err := adapter.PreviewEventOrder(ctx, event); err != nil || !preview.Allowed {
		t.Fatalf("event preview = %#v, %v", preview, err)
	}
	placed, err = adapter.PlaceEventOrder(ctx, event)
	if err != nil || len(placed.Legs) != 2 || placed.Legs[0].RequestedAmount != amount {
		t.Fatalf("event place = %#v, %v", placed, err)
	}
	if err := adapter.CancelEventOrder(ctx, event.ReadQuery, "event-order"); err != nil {
		t.Fatalf("event cancel: %v", err)
	}
}

func TestFutuComboAdapterProductRulesAndValidationFailures(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	ctx := t.Context()
	amount := 10.0
	price := 0.5
	quantity := 1.0
	eventInstrument := broker.Instrument{
		InstrumentID: "US.EVENT", ProductClass: broker.ProductClassEventContract,
		MarketSegment: broker.MarketSegmentPrediction, QuoteMarket: "US", TradeMarket: "US",
	}
	productCases := []struct {
		name  string
		query broker.ProductRuleQuery
		code  string
	}{
		{"event product", broker.ProductRuleQuery{OrderKind: broker.OrderKindEventSingle}, "PRODUCT_MISMATCH"},
		{"event market", broker.ProductRuleQuery{OrderKind: broker.OrderKindEventSingle, Instrument: broker.Instrument{
			ProductClass: broker.ProductClassEventContract, MarketSegment: broker.MarketSegmentPrediction,
		}}, "MARKET_MISMATCH"},
		{"event amount", broker.ProductRuleQuery{OrderKind: broker.OrderKindEventSingle, Instrument: eventInstrument}, "INVALID_AMOUNT"},
		{"event price", broker.ProductRuleQuery{OrderKind: broker.OrderKindEventSingle, Instrument: eventInstrument, Amount: &amount}, "INVALID_PRICE"},
		{"event order type", broker.ProductRuleQuery{OrderKind: broker.OrderKindEventSingle, Instrument: eventInstrument, Amount: &amount, Price: &price}, "INVALID_ORDER_TYPE"},
		{"event missing snapshot", broker.ProductRuleQuery{OrderKind: broker.OrderKindEventSingle, Instrument: eventInstrument, Amount: &amount, Price: &price, OrderType: "LIMIT"}, "EVENT_NOT_TRADABLE"},
		{"fractional option", broker.ProductRuleQuery{Instrument: broker.Instrument{ProductClass: broker.ProductClassOption}, Quantity: new(1.5)}, "INVALID_CONTRACT_QUANTITY"},
		{"missing future quantity", broker.ProductRuleQuery{Instrument: broker.Instrument{ProductClass: broker.ProductClassFuture}}, "INVALID_CONTRACT_QUANTITY"},
		{"option session", broker.ProductRuleQuery{Instrument: broker.Instrument{ProductClass: broker.ProductClassOption}, Quantity: &quantity, Session: "RTH"}, "INVALID_SESSION"},
	}
	for _, test := range productCases {
		result, err := adapter.ValidateProductOrder(ctx, test.query)
		if err != nil || result.Allowed || result.ReasonCode != test.code {
			t.Errorf("%s result=%#v err=%v", test.name, result, err)
		}
	}
	if result, err := adapter.ValidateProductOrder(ctx, broker.ProductRuleQuery{}); err != nil || !result.Allowed {
		t.Fatalf("ordinary product rule = %#v, %v", result, err)
	}

	validLegs := []broker.OrderLegIntent{
		{InstrumentID: "US.ONE", ProductClass: broker.ProductClassOption, Side: "BUY", Ratio: 1, Quantity: &quantity},
		{InstrumentID: "US.TWO", ProductClass: broker.ProductClassOption, Side: "SELL", Ratio: 1, Quantity: &quantity},
	}
	expired := time.Now().Add(-time.Second)
	validFuture := time.Now().Add(time.Minute)
	comboCases := []broker.ComboOrderIntent{
		{},
		{OrderKind: broker.OrderKindSingle, Legs: validLegs},
		{OrderKind: broker.OrderKindOptionCombo, Legs: []broker.OrderLegIntent{
			validLegs[0], {InstrumentID: "US.TWO", ProductClass: broker.ProductClassFuture, Side: "SELL", Ratio: 1},
		}},
		{OrderKind: broker.OrderKindOptionCombo, Legs: []broker.OrderLegIntent{
			validLegs[0], {InstrumentID: "US.TWO", ProductClass: broker.ProductClassOption, Side: "SELL", Ratio: 0},
		}},
		{OrderKind: broker.OrderKindEventParlay, Legs: validLegs},
		{OrderKind: broker.OrderKindEventParlay, RFQID: "rfq", QuoteExpiresAt: &expired, Amount: &amount, Legs: validLegs},
		{OrderKind: broker.OrderKindEventParlay, RFQID: "rfq", QuoteExpiresAt: &validFuture, Legs: validLegs},
		{OrderKind: broker.OrderKindEventParlay, RFQID: "rfq", QuoteExpiresAt: &validFuture, Amount: &amount, Legs: []broker.OrderLegIntent{
			{InstrumentID: "US.ONE", ProductClass: broker.ProductClassEventContract, Side: "BUY", Ratio: 1},
			{InstrumentID: "US.TWO", ProductClass: broker.ProductClassEventContract, Side: "BUY", Ratio: 1, PredictionSide: "YES"},
		}},
	}
	for index, intent := range comboCases {
		if _, err := validateComboIntent(intent); err == nil {
			t.Errorf("invalid combo case %d succeeded", index)
		}
	}
	if _, err := futuComboLegs(validLegs, false); err != nil {
		t.Fatalf("valid option legs: %v", err)
	}
	if _, err := futuComboLegs([]broker.OrderLegIntent{{
		InstrumentID: "bad", Side: "BUY", Ratio: 1,
	}}, false); err == nil {
		t.Fatal("invalid option symbol succeeded")
	}
	if _, err := futuComboLegs([]broker.OrderLegIntent{{
		InstrumentID: "US.ONE", Side: "HOLD", Ratio: 1,
	}}, false); err == nil {
		t.Fatal("invalid combo side succeeded")
	}
	if _, err := futuComboLegs([]broker.OrderLegIntent{{
		InstrumentID: "US.EVENT", Side: "BUY", Ratio: 1, PredictionSide: "MAYBE",
	}}, true); err == nil {
		t.Fatal("invalid prediction side succeeded")
	}
	if ids := comboInstrumentIDs(validLegs); len(ids) != 2 {
		t.Fatalf("combo instrument ids = %#v", ids)
	}
	if comboQuantity(broker.ComboOrderIntent{Amount: &amount}) != amount ||
		comboQuantity(broker.ComboOrderIntent{Legs: validLegs}) != quantity ||
		comboQuantity(broker.ComboOrderIntent{}) != 1 {
		t.Fatal("combo quantity mismatch")
	}
	if _, err := adapter.PlaceComboOrder(ctx, broker.ComboOrderIntent{
		OrderKind: broker.OrderKindOptionCombo, Legs: validLegs,
	}); err == nil || !strings.Contains(err.Error(), "previewId") {
		t.Fatalf("place without preview error = %v", err)
	}
	if err := adapter.CancelComboOrder(ctx, broker.ReadQuery{}, " "); err == nil {
		t.Fatal("blank combo cancel succeeded")
	}
	if err := adapter.validateActiveEventContracts(ctx, []string{""}); err == nil {
		t.Fatal("blank event contract succeeded")
	}
}

func TestFutuAdvancedProtocolTransportFailureIsReturned(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	server.setDropProto(3434)
	_, err := adapter.QueryPredictionMarket(t.Context(), broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeaturePredictionDiscover,
		Params: map[string]any{"operation": "categories"},
	})
	if err == nil {
		t.Fatalf("transport failure error = %v", err)
	}
}
