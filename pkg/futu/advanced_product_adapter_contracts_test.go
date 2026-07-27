package futu

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	eventcontractsnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgeteventcontractsnapshot"
	qotgetorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetorderbook"
	qotgetsearchquotepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsearchquote"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	qotgetusersecuritygrouppb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecuritygroup"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestFutuAdvancedSpecializedReadersAndCustomizationSuccess(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	server.securitySnapshots = []*qotgetsecuritysnapshotpb.Snapshot{testTencentSecuritySnapshot()}
	server.staticInfos = []*qotcommonpb.SecurityStaticInfo{testTencentStaticInfo()}
	server.searchQuotes = []*qotgetsearchquotepb.SearchQuote{{
		Market: new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)),
		Code:   new("AAPL"),
		Name:   new("Apple"),
	}}
	server.watchlistGroups = []*qotgetusersecuritygrouppb.GroupData{{
		GroupName: new("Favorites"),
		GroupType: new(int32(qotgetusersecuritygrouppb.GroupType_GroupType_Custom)),
	}}
	server.orderBookSnapshot = &qotgetorderbookpb.S2C{
		Security: &qotcommonpb.Security{
			Market: new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)),
			Code:   new("AAPL"),
		},
	}
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	ctx := t.Context()

	if _, err := adapter.QueryInstrumentProfile(ctx, broker.FeatureQuery{
		FeatureID: broker.FeatureInstrumentProfile,
	}); err == nil {
		t.Fatal("instrument profile without id succeeded")
	}
	profile, err := adapter.QueryInstrumentProfile(ctx, broker.FeatureQuery{
		Market: "HK", InstrumentID: "HK.00700", FeatureID: broker.FeatureInstrumentProfile,
	})
	if err != nil || len(profile.Entries) != 1 {
		t.Fatalf("instrument profile = %#v, %v", profile, err)
	}
	search, err := adapter.QueryInstrumentProfile(ctx, broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeatureMarketSearch, PageSize: 10,
		Params: map[string]any{"keyword": "Apple"},
	})
	if err != nil || len(search.Entries) != 1 {
		t.Fatalf("instrument search = %#v, %v", search, err)
	}
	watchlists, err := adapter.QueryCustomization(ctx, broker.FeatureQuery{
		FeatureID: broker.FeatureRemoteWatchlistList,
	})
	if err != nil || len(watchlists.Entries) != 1 {
		t.Fatalf("remote watchlists = %#v, %v", watchlists, err)
	}
	if _, err := adapter.QueryCustomization(ctx, broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeaturePredictionDiscover,
		Params: map[string]any{"operation": "categories"},
	}); err != nil {
		t.Fatalf("generic customization read: %v", err)
	}
	written, err := adapter.ApplyCustomization(ctx, broker.CustomizationAction{
		FeatureID: broker.FeatureRemoteWatchlistModify,
		Action:    "modify",
		Payload: map[string]any{
			"groupName": "Favorites",
			"op":        1,
			"securityList": []any{map[string]any{
				"market": 11, "code": "AAPL",
			}},
		},
	})
	if err != nil || written == nil {
		t.Fatalf("remote watchlist write = %#v, %v", written, err)
	}

	if err := adapter.exchange.SubscribeOrderBook(ctx, "US.AAPL", false); err != nil {
		t.Fatalf("subscribe order book: %v", err)
	}
	depth, err := adapter.QueryMarketMicrostructure(ctx, broker.FeatureQuery{
		Market: "US", InstrumentID: "US.AAPL", FeatureID: broker.FeatureMarketDepth,
		Params: map[string]any{"num": float64(5)},
	})
	if err != nil || len(depth.Entries) != 1 {
		t.Fatalf("depth = %#v, %v", depth, err)
	}

	withPagination, err := adapter.queryAdvancedFeatureWithProtocols(ctx, broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeaturePredictionDiscover,
		Cursor: "cursor", PageSize: 10, Params: map[string]any{},
	}, map[string]string{"categories": "Qot_GetEventContractCategory"})
	if err != nil || withPagination == nil {
		t.Fatalf("advanced query default operation = %#v, %v", withPagination, err)
	}
	result := featureResultFromPayload(broker.FeatureQuery{
		FeatureID: broker.FeaturePredictionComboQuote,
	}, map[string]any{})
	if result.Metadata == nil || result.Metadata["quoteExpiresAt"] == nil {
		t.Fatalf("empty quote payload result = %#v", result)
	}
	if normalizeOpenDValue(42) != 42 {
		t.Fatal("default OpenD normalization changed scalar")
	}
	if mapped, err := structMap(map[string]any{"ok": true}); err != nil || mapped["ok"] != true {
		t.Fatalf("structMap success = %#v, %v", mapped, err)
	}
	if _, err := adapter.queryAdvancedFeatureWithProtocols(ctx, broker.FeatureQuery{
		Market: "US", InstrumentID: "INVALID", FeatureID: broker.FeatureMarketIntraday,
		Params: map[string]any{"operation": "intraday"},
	}, featureProtocols[broker.FeatureMarketIntraday]); err == nil {
		t.Fatal("advanced query with invalid instrument succeeded")
	}

	server.setSearchQuoteError(1, 1, "search failed")
	if _, err := adapter.QueryInstrumentProfile(ctx, broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeatureMarketSearch, PageSize: 10,
		Params: map[string]any{"keyword": "Apple"},
	}); err == nil {
		t.Fatal("search upstream error was hidden")
	}
	server.setSecuritySnapshotError(1, 1, "snapshot failed")
	failedSnapshotAdapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	if _, err := failedSnapshotAdapter.QueryInstrumentProfile(ctx, broker.FeatureQuery{
		Market: "HK", InstrumentID: "HK.00700", FeatureID: broker.FeatureInstrumentProfile,
	}); err == nil {
		t.Fatal("instrument snapshot upstream error was hidden")
	}
	server.setWatchlistGroupError(1, 1, "watchlist failed")
	failedWatchlistAdapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	if _, err := failedWatchlistAdapter.QueryCustomization(ctx, broker.FeatureQuery{
		FeatureID: broker.FeatureRemoteWatchlistList,
	}); err == nil {
		t.Fatal("remote watchlist upstream error was hidden")
	}
}

func TestFutuComboAdapterErrorPropagationBranches(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	ctx := t.Context()
	quantity := 1.0
	validLegs := []broker.OrderLegIntent{
		{InstrumentID: "US.ONE", ProductClass: broker.ProductClassOption, Side: "BUY", Ratio: 1, Quantity: &quantity},
		{InstrumentID: "US.TWO", ProductClass: broker.ProductClassOption, Side: "SELL", Ratio: 1, Quantity: &quantity},
	}
	if _, err := adapter.PreviewComboOrder(ctx, broker.ComboOrderIntent{}); err == nil {
		t.Fatal("invalid combo preview succeeded")
	}
	badSymbol := broker.ComboOrderIntent{
		OrderKind: broker.OrderKindOptionCombo,
		Legs: []broker.OrderLegIntent{
			{InstrumentID: "BAD", ProductClass: broker.ProductClassOption, Side: "BUY", Ratio: 1},
			validLegs[1],
		},
	}
	if _, err := adapter.PreviewComboOrder(ctx, badSymbol); err == nil {
		t.Fatal("bad-symbol combo preview succeeded")
	}
	if _, err := adapter.PreviewComboOrder(ctx, broker.ComboOrderIntent{
		ReadQuery: broker.ReadQuery{AccountID: "missing", Market: "US"},
		OrderKind: broker.OrderKindOptionCombo, Legs: validLegs,
	}); err == nil {
		t.Fatal("combo preview without account succeeded")
	}
	if _, err := adapter.PlaceComboOrder(ctx, broker.ComboOrderIntent{}); err == nil {
		t.Fatal("invalid combo place succeeded")
	}
	badSymbol.PreviewID = "preview"
	if _, err := adapter.PlaceComboOrder(ctx, badSymbol); err == nil {
		t.Fatal("bad-symbol combo place succeeded")
	}
	if _, err := adapter.PlaceComboOrder(ctx, broker.ComboOrderIntent{
		ReadQuery: broker.ReadQuery{AccountID: "missing", Market: "US"},
		PreviewID: "preview", OrderKind: broker.OrderKindOptionCombo, Legs: validLegs,
	}); err == nil {
		t.Fatal("combo place without account succeeded")
	}
	if err := adapter.CancelComboOrder(ctx, broker.ReadQuery{
		AccountID: "missing", Market: "US",
	}, "order"); err == nil {
		t.Fatal("combo cancel without account succeeded")
	}

	amount := 10.0
	expires := time.Now().Add(time.Minute)
	event := broker.ComboOrderIntent{
		OrderKind: broker.OrderKindEventParlay, PreviewID: "preview", RFQID: "rfq",
		QuoteExpiresAt: &expires, Amount: &amount,
		Legs: []broker.OrderLegIntent{
			{InstrumentID: "US.EVENT", ProductClass: broker.ProductClassEventContract, Side: "BUY", Ratio: 1, PredictionSide: "YES"},
			{InstrumentID: "US.OTHER", ProductClass: broker.ProductClassEventContract, Side: "BUY", Ratio: 1, PredictionSide: "NO"},
		},
	}
	if preview, err := adapter.PreviewComboOrder(ctx, event); err != nil || preview.Allowed {
		t.Fatalf("inactive event preview = %#v, %v", preview, err)
	}
	if _, err := adapter.PlaceComboOrder(ctx, event); err == nil {
		t.Fatal("inactive event place succeeded")
	}
}

func TestFutuEventContractStatusBranches(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	inactive := qotcommonpb.EC_Status_EC_Status_Closed
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)

	// A protocol-compatible dynamic response includes an unrelated contract
	// before the requested closed contract, covering both filtering and status rejection.
	eventResponse := buildEventContractSnapshotResponse(t, []eventContractStatus{
		{code: "UNRELATED", status: qotcommonpb.EC_Status_EC_Status_Active},
		{code: "EVENT", status: inactive},
	})
	server.setAdvancedResponse(3445, eventResponse)
	err := adapter.validateActiveEventContracts(t.Context(), []string{"US.EVENT"})
	if err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("inactive event error = %v", err)
	}
}

func TestFutuWarrantsStayHKOnlyAndFuturesRemainDiscoverable(t *testing.T) {
	for _, market := range []string{"HK", "US", "SH", "SZ"} {
		features := futuFeatureCapabilities(market)
		hasWarrants := false
		hasFutures := false
		for _, feature := range features {
			switch feature.ID {
			case broker.FeatureWarrants:
				hasWarrants = true
				if len(feature.ProductClasses) != 2 ||
					feature.ProductClasses[0] != broker.ProductClassWarrant ||
					feature.ProductClasses[1] != broker.ProductClassCBBC {
					t.Fatalf("%s warrant product classes = %v", market, feature.ProductClasses)
				}
			case broker.FeatureFutures:
				hasFutures = true
			}
		}
		if hasWarrants != (market == "HK") {
			t.Errorf("%s warrants capability = %v", market, hasWarrants)
		}
		if hasFutures != (market == "HK" || market == "US") {
			t.Errorf("%s futures capability = %v", market, hasFutures)
		}
	}
}

func TestSecurityDetailsProductIdentityFallbacks(t *testing.T) {
	cases := map[string]broker.ProductClass{
		"SecurityType_Bond":    broker.ProductClassBond,
		"Eqty":                 broker.ProductClassEquity,
		"Equity":               broker.ProductClassEquity,
		"Stock":                broker.ProductClassEquity,
		"Trust":                broker.ProductClassFund,
		"Fund":                 broker.ProductClassFund,
		"ETF":                  broker.ProductClassFund,
		"Warrant":              broker.ProductClassWarrant,
		"Bwrt":                 broker.ProductClassWarrant,
		"Index":                broker.ProductClassIndex,
		"Plate":                broker.ProductClassPlate,
		"PlateSet":             broker.ProductClassPlate,
		"Drvt":                 broker.ProductClassOption,
		"Option":               broker.ProductClassOption,
		"Future":               broker.ProductClassFuture,
		"unsupported-security": broker.ProductClassUnknown,
	}
	for securityType, expected := range cases {
		details := &SecurityDetails{
			SecurityType: securityType,
			ProductClass: broker.ProductClassUnknown,
		}
		refreshSecurityDetailsProductIdentity(details)
		if details.ProductClass != expected {
			t.Errorf("%s product class = %s, want %s", securityType, details.ProductClass, expected)
		}
	}

	for _, warrantType := range []string{"Bull", "BEAR"} {
		details := &SecurityDetails{
			ProductClass: broker.ProductClassWarrant,
			Warrant:      &WarrantSecurityDetails{WarrantType: warrantType},
		}
		refreshSecurityDetailsProductIdentity(details)
		if details.ProductClass != broker.ProductClassCBBC ||
			details.MarketSegment != broker.MarketSegmentDerivatives {
			t.Errorf("%s warrant identity = %s/%s", warrantType, details.ProductClass, details.MarketSegment)
		}
	}

	if got := marketSegmentFromProductClass(broker.ProductClassEventContract); got != broker.MarketSegmentPrediction {
		t.Fatalf("event contract segment = %s", got)
	}
	refreshSecurityDetailsProductIdentity(nil)
}

type eventContractStatus struct {
	code   string
	status qotcommonpb.EC_Status
}

func buildEventContractSnapshotResponse(
	t *testing.T,
	statuses []eventContractStatus,
) *eventcontractsnapshotpb.Response {
	t.Helper()
	items := make([]*eventcontractsnapshotpb.SnapshotItem, 0, len(statuses))
	for _, status := range statuses {
		items = append(items, &eventcontractsnapshotpb.SnapshotItem{
			Code: &qotcommonpb.Security{
				Market: new(int32(qotcommonpb.QotMarket_QotMarket_EventContract)),
				Code:   new(status.code),
			},
			Status: &status.status,
		})
	}
	return &eventcontractsnapshotpb.Response{
		RetType: new(int32(0)),
		S2C:     &eventcontractsnapshotpb.S2C{SnapshotList: items},
	}
}

func TestFutuSnapshotProductExtensionsAndSecurityTypeMapping(t *testing.T) {
	item := &broker.SecuritySnapshotItem{}
	contractSize := 100.5
	applyOptionSnapshotData(item, &qotgetsecuritysnapshotpb.OptionSnapshotExData{
		Type:              new(int32(qotcommonpb.OptionType_OptionType_Call)),
		Owner:             testUSSecurity("AAPL"),
		ContractSize:      new(int32(100)),
		ContractSizeFloat: &contractSize,
	})
	if item.Option == nil || item.Option.ContractSize != contractSize ||
		item.ProductClass != broker.ProductClassOption {
		t.Fatalf("option snapshot = %#v", item)
	}

	for _, warrantType := range []qotcommonpb.WarrantType{
		qotcommonpb.WarrantType_WarrantType_Buy,
		qotcommonpb.WarrantType_WarrantType_Bull,
		qotcommonpb.WarrantType_WarrantType_Bear,
	} {
		warrantItem := &broker.SecuritySnapshotItem{}
		applyWarrantSnapshotData(warrantItem, &qotgetsecuritysnapshotpb.WarrantSnapshotExData{
			WarrantType: new(int32(warrantType)),
			Owner:       testUSSecurity("AAPL"),
		})
		expected := broker.ProductClassWarrant
		if warrantType == qotcommonpb.WarrantType_WarrantType_Bull ||
			warrantType == qotcommonpb.WarrantType_WarrantType_Bear {
			expected = broker.ProductClassCBBC
		}
		if warrantItem.Warrant == nil || warrantItem.ProductClass != expected {
			t.Errorf("warrant type %v item = %#v", warrantType, warrantItem)
		}
	}
	futureItem := &broker.SecuritySnapshotItem{}
	applyFutureSnapshotData(futureItem, &qotgetsecuritysnapshotpb.FutureSnapshotExData{})
	if futureItem.Future == nil || futureItem.ProductClass != broker.ProductClassFuture {
		t.Fatalf("future snapshot = %#v", futureItem)
	}
	fundItem := &broker.SecuritySnapshotItem{}
	applyFundSnapshotData(fundItem, &qotgetsecuritysnapshotpb.TrustSnapshotExData{})
	if fundItem.Fund == nil || fundItem.ProductClass != broker.ProductClassFund {
		t.Fatalf("fund snapshot = %#v", fundItem)
	}
	applyOptionSnapshotData(item, nil)
	applyWarrantSnapshotData(item, nil)
	applyFutureSnapshotData(item, nil)
	applyFundSnapshotData(item, nil)

	classes := map[qotcommonpb.SecurityType]broker.ProductClass{
		qotcommonpb.SecurityType_SecurityType_Bond:     broker.ProductClassBond,
		qotcommonpb.SecurityType_SecurityType_Eqty:     broker.ProductClassEquity,
		qotcommonpb.SecurityType_SecurityType_Trust:    broker.ProductClassFund,
		qotcommonpb.SecurityType_SecurityType_Warrant:  broker.ProductClassWarrant,
		qotcommonpb.SecurityType_SecurityType_Index:    broker.ProductClassIndex,
		qotcommonpb.SecurityType_SecurityType_Plate:    broker.ProductClassPlate,
		qotcommonpb.SecurityType_SecurityType_PlateSet: broker.ProductClassPlate,
		qotcommonpb.SecurityType_SecurityType_Drvt:     broker.ProductClassOption,
		qotcommonpb.SecurityType_SecurityType_Future:   broker.ProductClassFuture,
		qotcommonpb.SecurityType(999):                  broker.ProductClassUnknown,
	}
	for securityType, expected := range classes {
		if got := productClassFromSecurityType(int32(securityType)); got != expected {
			t.Errorf("productClassFromSecurityType(%v) = %s, want %s", securityType, got, expected)
		}
	}
}

func TestFutuTradeProductRequestAndReadLifecycleBranches(t *testing.T) {
	account := resolvedTradeAccount{
		AccountID: "1001", Market: "US",
		protoAccountID: 1001,
		protoTrdEnv:    int32(trdcommonpb.TrdEnv_TrdEnv_Real),
		protoTrdMarket: int32(trdcommonpb.TrdMarket_TrdMarket_US),
	}
	amount := 20.0
	order := types.SubmitOrder{
		Symbol: "US.EVENT", Side: types.SideTypeBuy, Type: types.OrderTypeLimit,
		Quantity: fixedpoint.NewFromFloat(1), Price: fixedpoint.NewFromFloat(0.6),
	}
	if _, err := placeOrderRequestFromSubmitOrder(account, order, BrokerPlaceOrderQuery{
		Amount: new(-1.0), PredictionSide: "YES",
	}); err == nil {
		t.Fatal("negative event amount succeeded")
	}
	if _, err := placeOrderRequestFromSubmitOrder(account, order, BrokerPlaceOrderQuery{
		Amount: &amount, PredictionSide: "MAYBE",
	}); err == nil {
		t.Fatal("invalid event prediction side succeeded")
	}
	request, err := placeOrderRequestFromSubmitOrder(account, order, BrokerPlaceOrderQuery{
		Amount: &amount, PredictionSide: "YES",
	})
	if err != nil || request.GetAmount() != amount ||
		request.GetPredSide() != int32(commonpb.PredSide_PredSide_Yes) {
		t.Fatalf("event place request = %#v, %v", request, err)
	}

	comboPosition := brokerPositionSnapshotFromProto(account, &trdcommonpb.Position{
		Code: new("OPTION-COMBO"), ComboID: new(uint64(99)), Qty: new(1.0),
	})
	if comboPosition.ProductClass != broker.ProductClassOption || comboPosition.ComboID == nil {
		t.Fatalf("combo position = %#v", comboPosition)
	}
	futuresAccount := account
	futuresAccount.Market = "FUTURES"
	futurePosition := brokerPositionSnapshotFromProto(futuresAccount, &trdcommonpb.Position{
		Code: new("ES"), Qty: new(1.0),
	})
	if futurePosition.ProductClass != broker.ProductClassFuture {
		t.Fatalf("future position = %#v", futurePosition)
	}

	eventSingle := brokerOrderSnapshotFromProto(account, &trdcommonpb.Order{
		Code: new("EVENT"), OrderAmount: &amount,
	})
	if eventSingle.OrderKind != broker.OrderKindEventSingle {
		t.Fatalf("event single order = %#v", eventSingle)
	}
	single := brokerOrderSnapshotFromProto(account, &trdcommonpb.Order{Code: new("AAPL")})
	if single.OrderKind != broker.OrderKindSingle {
		t.Fatalf("single order = %#v", single)
	}
	legs := brokerOrderLegSnapshots(&trdcommonpb.Order{
		Qty: new(1.0),
		ComboLegs: []*qotcommonpb.ComboLeg{
			nil,
			{},
			{Security: &qotcommonpb.Security{Market: new(int32(999)), Code: new("BAD")}},
			{Security: &qotcommonpb.Security{
				Market: new(int32(qotcommonpb.QotMarket_QotMarket_EventContract)),
				Code:   new("EVENT"),
			}},
		},
	}, broker.ProductClassEventContract)
	if len(legs) != 1 || legs[0].InstrumentID != "US.EVENT" {
		t.Fatalf("filtered order legs = %#v", legs)
	}
}

func TestFutuComboProtocolTransportErrors(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	account := testSimulateHKCashAccount()
	account.TrdMarketAuthList = []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)}
	server.setAccounts([]*trdcommonpb.TrdAcc{account})
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	quantity := 1.0
	intent := broker.ComboOrderIntent{
		ReadQuery: broker.ReadQuery{AccountID: "1001", Market: "US", TradingEnvironment: "SIMULATE"},
		PreviewID: "preview", OrderKind: broker.OrderKindOptionCombo,
		Legs: []broker.OrderLegIntent{
			{InstrumentID: "US.ONE", ProductClass: broker.ProductClassOption, Side: "BUY", Ratio: 1, Quantity: &quantity},
			{InstrumentID: "US.TWO", ProductClass: broker.ProductClassOption, Side: "SELL", Ratio: 1, Quantity: &quantity},
		},
	}
	server.setDropProto(opend.ProtoTrdGetComboMaxTrdQtys)
	if _, err := adapter.PreviewComboOrder(t.Context(), intent); err == nil {
		t.Fatal("combo preview transport error was hidden")
	}

	placeServer := startQuoteOpenDServer(t)
	defer placeServer.stop()
	placeAccount := testSimulateHKCashAccount()
	placeAccount.TrdMarketAuthList = []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)}
	placeServer.setAccounts([]*trdcommonpb.TrdAcc{placeAccount})
	placeAdapter := newTestBrokerAdapter(t, placeServer).(*futuAdapter)
	placeServer.setDropProto(opend.ProtoTrdPlaceComboOrder)
	if _, err := placeAdapter.PlaceComboOrder(t.Context(), intent); err == nil {
		t.Fatal("combo place transport error was hidden")
	}

	eventServer := startQuoteOpenDServer(t)
	defer eventServer.stop()
	eventAdapter := newTestBrokerAdapter(t, eventServer).(*futuAdapter)
	eventServer.setDropProto(3445)
	if err := eventAdapter.validateActiveEventContracts(t.Context(), []string{"US.EVENT"}); err == nil {
		t.Fatal("event snapshot transport error was hidden")
	}
}
