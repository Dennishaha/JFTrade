package servercore

import (
	"context"
	"errors"
	"testing"
	"time"

	productsrv "github.com/jftrade/jftrade-main/internal/productfeatures"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type completeADKProductBroker struct {
	id            string
	placeComboErr error
	cancelErr     error
	placedCombos  int
	canceled      int
}

func (b *completeADKProductBroker) ID() string { return b.id }

func (b *completeADKProductBroker) Descriptor() broker.Descriptor {
	features := make([]broker.FeatureCapability, 0, len(broker.BuiltinCapabilityCatalog.Features))
	for _, definition := range broker.BuiltinCapabilityCatalog.Features {
		features = append(features, broker.FeatureCapability{
			ID: definition.ID, Markets: []string{"US"}, Access: definition.Access,
			State: broker.CapabilityAvailable,
		})
	}
	return broker.Descriptor{
		ID: b.id, SecurityFirm: "FUTUINC",
		CapabilityVersion: broker.BuiltinCapabilityCatalog.Version,
		Capabilities: []broker.MarketCapability{{
			Market: "US", SupportsQuote: true, SupportsTrade: true, Features: features,
		}},
	}
}

func (b *completeADKProductBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	firm := "FUTUINC"
	return []broker.Account{{
		ID: "account-us", BrokerID: b.id, SecurityFirm: &firm,
		MarketAuthorities: []string{"US", "FUTURES"},
	}}, nil
}

func (b *completeADKProductBroker) Trading() broker.TradingService { return b }

func (b *completeADKProductBroker) MarketData() broker.MarketDataReader { return nil }

func (b *completeADKProductBroker) PlaceOrder(
	_ context.Context,
	query broker.PlaceOrderQuery,
) (*broker.PlaceOrderResult, error) {
	brokerOrderIDEx := "coverage-ex-1001"
	return &broker.PlaceOrderResult{
		BrokerOrderID: "1001", BrokerOrderIDEx: &brokerOrderIDEx,
		TradingEnvironment: query.TradingEnvironment, AccountID: query.AccountID,
		Market: query.Market, Status: "SUBMITTED",
	}, nil
}

func (b *completeADKProductBroker) CancelOrders(
	context.Context,
	broker.ReadQuery,
	...broker.CancelOrder,
) error {
	b.canceled++
	return b.cancelErr
}

func (b *completeADKProductBroker) featureResult(query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return &broker.FeatureResult{
		Entries: []map[string]any{{"featureId": query.FeatureID, "instrumentId": query.InstrumentID}},
	}, nil
}

func (b *completeADKProductBroker) QueryMarketMicrostructure(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}

func (b *completeADKProductBroker) QueryInstrumentProfile(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}

func (b *completeADKProductBroker) QueryDerivativeCatalog(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}

func (b *completeADKProductBroker) QueryOptionAnalytics(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}

func (b *completeADKProductBroker) QueryInstrumentResearch(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}

func (b *completeADKProductBroker) QueryMarketResearch(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}

func (b *completeADKProductBroker) QueryPredictionMarket(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}

func (b *completeADKProductBroker) SubscribePredictionMarket(
	context.Context,
	broker.PredictionSubscription,
) error {
	return nil
}

func (b *completeADKProductBroker) UnsubscribePredictionMarket(
	context.Context,
	broker.PredictionSubscription,
) error {
	return nil
}

func (b *completeADKProductBroker) QueryTechnicalIndicator(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}

func (b *completeADKProductBroker) QueryCustomization(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}

func (b *completeADKProductBroker) ApplyCustomization(
	_ context.Context,
	action broker.CustomizationAction,
) (*broker.CustomizationResult, error) {
	return &broker.CustomizationResult{
		Entries: []map[string]any{{"action": action.Action, "payload": action.Payload}},
	}, nil
}

func (b *completeADKProductBroker) QuerySecuritySnapshot(
	_ context.Context,
	query broker.SecuritySnapshotQuery,
) (*broker.SecuritySnapshotResult, error) {
	snapshots := make([]broker.SecuritySnapshotItem, 0, len(query.Symbols))
	for _, symbol := range query.Symbols {
		price := 200.0
		snapshots = append(snapshots, broker.SecuritySnapshotItem{
			Symbol: symbol, LastPrice: &price, ObservedAt: time.Now().UTC(),
		})
	}
	return &broker.SecuritySnapshotResult{AccountID: query.AccountID, Snapshots: snapshots}, nil
}

func (b *completeADKProductBroker) ValidateProductOrder(
	context.Context,
	broker.ProductRuleQuery,
) (*broker.ProductRuleResult, error) {
	impact := 125.0
	return &broker.ProductRuleResult{Allowed: true, BuyingPowerImpact: &impact}, nil
}

func (b *completeADKProductBroker) PreviewComboOrder(
	context.Context,
	broker.ComboOrderIntent,
) (*broker.ProductRuleResult, error) {
	impact := 250.0
	return &broker.ProductRuleResult{Allowed: true, BuyingPowerImpact: &impact}, nil
}

func (b *completeADKProductBroker) PlaceComboOrder(
	_ context.Context,
	intent broker.ComboOrderIntent,
) (*broker.ComboOrderResult, error) {
	b.placedCombos++
	if b.placeComboErr != nil {
		return nil, b.placeComboErr
	}
	result := &broker.ComboOrderResult{BrokerOrderID: "combo-1001", Status: "SUBMITTED"}
	if len(intent.Legs) > 0 {
		result.Legs = []broker.OrderLegSnapshot{{
			BrokerLegID: "leg-1", InstrumentID: intent.Legs[0].InstrumentID,
			Status: "SUBMITTED",
		}}
	}
	return result, nil
}

func (b *completeADKProductBroker) CancelComboOrder(
	context.Context,
	broker.ReadQuery,
	string,
) error {
	b.canceled++
	return b.cancelErr
}

func newCompleteADKProductServer(t *testing.T) (*Server, *completeADKProductBroker) {
	t.Helper()
	adapter := &completeADKProductBroker{id: "coverage"}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	server := &Server{
		brokers:         registry,
		executionOrders: newExecutionOrderStore(),
	}
	server.productFeaturesSvc = productsrv.NewService(registry, adapter.id, nil, nil)
	runtime := &serverTradingBrokerRuntimeProvider{server: server}
	gateway := &serverTradingOrderGateway{server: server}
	server.tradingSvc = trdsrv.NewService(
		trdsrv.WithBrokerRuntimeProvider(runtime),
		trdsrv.WithDefaultTradingEnvironment(func() string { return "SIMULATE" }),
		trdsrv.WithOrderGateway(gateway),
		trdsrv.WithComboOrderGateway(gateway),
	)
	return server, adapter
}

func TestADKProductToolCompleteDispatchThroughBrokerNeutralService(t *testing.T) {
	server, _ := newCompleteADKProductServer(t)
	ctx := t.Context()
	cases := []struct {
		name  string
		input map[string]any
	}{
		{"market.capabilities", map[string]any{}},
		{"market.search", map[string]any{
			"brokerId": "coverage", "market": "us", "query": "apple", "pageSize": 500,
		}},
		{"market.snapshot", map[string]any{
			"brokerId": "coverage", "market": "us", "symbol": "aapl",
		}},
		{"market.snapshots", map[string]any{
			"brokerId": "coverage", "market": "us", "symbols": []any{"US.AAPL", "US.MSFT"},
		}},
		{"derivatives.option_chain", map[string]any{
			"brokerId": "coverage", "market": "us", "instrumentId": "us.aapl",
			"cursor": "next", "pageSize": -1, "operation": "chain",
		}},
		{"prediction.discover", map[string]any{
			"brokerId": "coverage", "accountId": "account-us", "operation": "categories",
		}},
		{"alerts.price.set", map[string]any{
			"brokerId": "coverage", "accountId": "account-us",
			"payload": map[string]any{"instrumentId": "US.AAPL", "price": 210},
		}},
		{"watchlist.remote.modify", map[string]any{
			"brokerId": "coverage", "group": "Favorites",
		}},
		{"execution.buying_power", map[string]any{
			"brokerId": "coverage", "accountId": "account-us", "market": "US",
			"instrument": map[string]any{"instrumentId": "US.AAPL", "productClass": "option"},
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, err := server.invokeADKProductTool(ctx, test.name, test.input)
			if err != nil || result == nil {
				t.Fatalf("%s = %#v, %v", test.name, result, err)
			}
		})
	}
}

func TestADKExecutionToolsCompleteOrderAndComboLifecycle(t *testing.T) {
	server, adapter := newCompleteADKProductServer(t)
	ctx := t.Context()
	price := 200.0
	orderInput := map[string]any{
		"brokerId": "coverage", "tradingEnvironment": "SIMULATE",
		"accountId": "account-us", "market": "US", "symbol": "AAPL",
		"side": "BUY", "orderType": "LIMIT", "quantity": 1,
		"price": price, "clientOrderId": "stock-client-1",
	}
	if _, err := server.invokeADKExecutionTool(ctx, "execution.order_preview", orderInput); err != nil {
		t.Fatalf("order preview: %v", err)
	}
	placedValue, err := server.invokeADKExecutionTool(ctx, "execution.order_place", orderInput)
	if err != nil {
		t.Fatalf("order place: %v", err)
	}
	placed := placedValue.(trdsrv.ExecutionCommandResponse)
	if placed.InternalOrderID == nil {
		t.Fatalf("placed order = %#v", placed)
	}
	if _, err := server.invokeADKExecutionTool(ctx, "execution.order_cancel", map[string]any{
		"internalOrderId": *placed.InternalOrderID,
	}); err != nil {
		t.Fatalf("order cancel: %v", err)
	}

	quantity := 1.0
	comboInput := map[string]any{
		"brokerId": "coverage", "tradingEnvironment": "SIMULATE",
		"accountId": "account-us", "market": "US", "clientOrderId": "combo-client-1",
		"orderKind": "option_combo", "productClass": "option",
		"underlyingInstrumentId": "US.AAPL", "optionStrategy": "vertical",
		"nearExpiry": "2026-07-17", "spread": 10.0,
		"legs": []any{
			map[string]any{
				"instrumentId": "US.AAPL260717C00200000", "productClass": "option",
				"side": "BUY", "ratio": 1, "quantity": quantity,
			},
			map[string]any{
				"instrumentId": "US.AAPL260717C00210000", "productClass": "option",
				"side": "SELL", "ratio": 1, "quantity": quantity,
			},
		},
	}
	previewValue, err := server.invokeADKExecutionTool(ctx, "execution.combo_preview", comboInput)
	if err != nil {
		t.Fatalf("combo preview: %v", err)
	}
	preview := previewValue.(trdsrv.ExecutionComboPreview)
	comboInput["previewId"] = preview.PreviewID
	comboValue, err := server.invokeADKExecutionTool(ctx, "execution.combo_place", comboInput)
	if err != nil {
		t.Fatalf("combo place: %v", err)
	}
	combo := comboValue.(trdsrv.ExecutionCommandResponse)
	if combo.InternalOrderID == nil || adapter.placedCombos != 1 {
		t.Fatalf("combo response = %#v, calls=%d", combo, adapter.placedCombos)
	}
	replayed, err := server.invokeADKExecutionTool(ctx, "execution.combo_place", comboInput)
	if err != nil || replayed.(trdsrv.ExecutionCommandResponse).InternalOrderID == nil ||
		adapter.placedCombos != 1 {
		t.Fatalf("combo replay = %#v, calls=%d, err=%v", replayed, adapter.placedCombos, err)
	}
	if _, err := server.invokeADKExecutionTool(ctx, "execution.combo_cancel", map[string]any{
		"internalOrderId": *combo.InternalOrderID,
	}); err != nil {
		t.Fatalf("combo cancel: %v", err)
	}
}

func TestServerComboGatewayFailureAndCancelBoundaries(t *testing.T) {
	server, adapter := newCompleteADKProductServer(t)
	gateway := &serverTradingOrderGateway{server: server}
	quantity := 1.0
	intent := broker.ComboOrderIntent{
		ReadQuery: broker.ReadQuery{
			BrokerID: adapter.id, AccountID: "account-us", Market: "US",
			TradingEnvironment: "SIMULATE",
		},
		ClientOrderID: "failure-combo", PreviewID: "preview-failure",
		OrderKind: broker.OrderKindOptionCombo, ProductClass: broker.ProductClassOption,
		Legs: []broker.OrderLegIntent{{
			InstrumentID: "US.OPTION", ProductClass: broker.ProductClassOption,
			Side: "BUY", Ratio: 1, Quantity: &quantity,
		}},
	}
	adapter.placeComboErr = errors.New("submission outcome unknown")
	if _, err := gateway.PlaceCombo(t.Context(), intent); !errors.Is(err, adapter.placeComboErr) {
		t.Fatalf("combo place error = %v", err)
	}
	unknown := server.executionOrders.listOrders().Orders[0]
	if unknown.Status != trdsrv.OrderStatusSubmissionUnknown {
		t.Fatalf("unknown combo state = %#v", unknown)
	}
	replayed, err := gateway.PlaceCombo(t.Context(), intent)
	if err != nil || replayed.InternalOrderID != unknown.InternalOrderID || adapter.placedCombos != 1 {
		t.Fatalf("submission-unknown replay = %#v, calls=%d, err=%v", replayed, adapter.placedCombos, err)
	}

	if _, err := gateway.CancelCombo(t.Context(), "missing"); err == nil {
		t.Fatal("missing combo cancel succeeded")
	}
	missingBrokerID := server.executionOrders.recordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord{
		BrokerID: adapter.id, AccountID: "account-us", Market: "US",
		OrderKind: broker.OrderKindOptionCombo, ProductClass: broker.ProductClassOption,
		ClientOrderID: "missing-broker-id", Status: "SUBMITTED",
	})
	if _, err := gateway.CancelCombo(t.Context(), missingBrokerID.InternalOrderID); err == nil {
		t.Fatal("combo without broker order id canceled")
	}

	adapter.placeComboErr = nil
	intent.ClientOrderID = "cancel-error-combo"
	placed, err := gateway.PlaceCombo(t.Context(), intent)
	if err != nil {
		t.Fatalf("place cancel-error combo: %v", err)
	}
	adapter.cancelErr = errors.New("cancel failed")
	if _, err := gateway.CancelCombo(t.Context(), placed.InternalOrderID); !errors.Is(err, adapter.cancelErr) {
		t.Fatalf("combo cancel error = %v", err)
	}

	unsupportedRegistry := broker.NewRegistry()
	unsupportedRegistry.Register(servercoreFakeBroker{})
	unsupported := &serverTradingOrderGateway{server: &Server{
		brokers: unsupportedRegistry, executionOrders: newExecutionOrderStore(),
	}}
	intent.BrokerID = "fake"
	intent.ClientOrderID = "unsupported-combo"
	if _, err := unsupported.PlaceCombo(t.Context(), intent); err == nil {
		t.Fatal("unsupported combo broker succeeded")
	}

	unsupportedOrder := unsupported.server.executionOrders.recordPlacedOrder(
		trdsrv.ExecutionPlacedOrderRecord{
			BrokerID: "fake", BrokerOrderID: "unsupported-order", Status: "SUBMITTED",
			OrderKind: broker.OrderKindOptionCombo, ProductClass: broker.ProductClassOption,
		},
	)
	if _, err := unsupported.CancelCombo(t.Context(), unsupportedOrder.InternalOrderID); err == nil {
		t.Fatal("unsupported combo cancel succeeded")
	}

	noLegIntent := broker.ComboOrderIntent{
		ReadQuery: broker.ReadQuery{
			BrokerID: adapter.id, AccountID: "account-us", Market: "US",
		},
		ClientOrderID: "no-leg-gateway-boundary", PreviewID: "preview-no-leg",
		OrderKind: broker.OrderKindOptionCombo, ProductClass: broker.ProductClassOption,
	}
	if _, err := gateway.PlaceCombo(t.Context(), noLegIntent); err != nil {
		t.Fatalf("broker gateway no-leg persistence: %v", err)
	}

	changed := intent
	changed.BrokerID = adapter.id
	changed.ClientOrderID = "changed-request-combo"
	changed.PreviewID = "preview-changed"
	if _, err := gateway.PlaceCombo(t.Context(), changed); err != nil {
		t.Fatalf("initial changed-request combo: %v", err)
	}
	placedCalls := adapter.placedCombos
	changed.Price = new(9.0)
	if replayed, err := gateway.PlaceCombo(t.Context(), changed); err != nil ||
		replayed.InternalOrderID == "" || adapter.placedCombos != placedCalls {
		t.Fatalf("changed idempotent combo replay = %#v, calls=%d, err=%v",
			replayed, adapter.placedCombos, err)
	}
}

func TestProductToolRegistryHandlersAndSwaggerMarkersStayLinked(t *testing.T) {
	ctx := t.Context()
	registry := jfadk.NewToolRegistry()
	registerJFTradeProductTools(registry, ToolDeps{})
	for _, name := range []string{
		"market.search", "execution.order_place", "alerts.price.set",
	} {
		registered, ok := registry.Get(name)
		if !ok {
			t.Fatalf("registered product tool %q missing", name)
		}
		if _, err := registered.Handler(ctx, map[string]any{}); err == nil {
			t.Fatalf("product tool %q hid unavailable dependency", name)
		}
	}

	productCalls := 0
	executionCalls := 0
	registry = jfadk.NewToolRegistry()
	registerJFTradeProductTools(registry, ToolDeps{
		ProductTool: func(_ context.Context, name string, _ map[string]any) (any, error) {
			productCalls++
			return name, nil
		},
		ExecutionTool: func(_ context.Context, name string, _ map[string]any) (any, error) {
			executionCalls++
			return name, nil
		},
	})
	for _, name := range []string{
		"market.snapshot", "execution.combo_preview", "watchlist.remote.modify",
	} {
		registered, _ := registry.Get(name)
		if result, err := registered.Handler(ctx, nil); err != nil || result != name {
			t.Fatalf("product tool %q = %#v, %v", name, result, err)
		}
	}
	if productCalls != 2 || executionCalls != 1 {
		t.Fatalf("product/execution calls = %d/%d", productCalls, executionCalls)
	}

	documentationMarkers := []func() string{
		documentDataMigrationRoutes,
		documentDataManagementOverview,
		documentDataCleanupPreview,
		documentDataCleanupExecute,
		documentDatabaseCompact,
		documentDatabaseRebuild,
		documentAssistantCatalogRoutes,
		documentAssistantTaskMemoryRoutes,
		documentAssistantSessionRunRoutes,
		documentAssistantChatApprovalSkillRoutes,
		documentAssistantOptimizationRoutes,
		documentAssistantWorkflowRoutes,
		documentBacktestSyncTaskRoutes,
		documentMarketUtilityRoutes,
		documentPluginRoutes,
		documentPortfolioRoutes,
		documentBrokerFundsRoute,
		documentBrokerPositionsRoute,
		documentBrokerOrdersRoute,
		documentBrokerFillsRoute,
		documentBrokerCashFlowsRoute,
		documentBrokerOrderFeesRoute,
		documentBrokerMarginRatiosRoute,
		documentBrokerMaxTradeQuantityRoute,
		documentBrokerQuoteRoute,
		documentBrokerKLinesRoute,
		documentBrokerSecuritiesRoute,
		documentExecutionOrdersRoute,
		documentExecutionOrderDetailsRoute,
		documentExecutionPlaceRoute,
		documentExecutionCancelRoute,
		documentExecutionEventsRoute,
		documentSystemOperationalRoutes,
		documentExecutionPreviewRoute,
	}
	seenMarkers := map[string]struct{}{}
	for _, marker := range documentationMarkers {
		name := marker()
		if name == "" {
			t.Fatal("empty OpenAPI documentation marker")
		}
		if _, duplicate := seenMarkers[name]; duplicate {
			t.Fatalf("duplicate OpenAPI documentation marker %q", name)
		}
		seenMarkers[name] = struct{}{}
	}
}

func TestBrokerRuntimeResolutionAndCancelBridgeCompleteBranches(t *testing.T) {
	server, adapter := newCompleteADKProductServer(t)
	provider := &serverTradingBrokerRuntimeProvider{server: server}
	if provider.ActiveBroker() != adapter ||
		provider.ResolveBroker("") != adapter ||
		provider.ResolveBroker(adapter.id) != adapter ||
		provider.ResolveBroker("missing") != nil ||
		provider.ResolveBroker("futu") != nil {
		t.Fatal("broker runtime provider resolved an unexpected adapter")
	}

	bridge := &strategyRuntimeBrokerBridge{broker: adapter}
	if err := bridge.CancelBrokerOrder(t.Context(), broker.ReadQuery{}, broker.CancelOrder{}); err != nil {
		t.Fatalf("bridge cancel: %v", err)
	}
	bridge.broker = servercoreFakeBroker{}
	if err := bridge.CancelBrokerOrder(t.Context(), broker.ReadQuery{}, broker.CancelOrder{}); err == nil {
		t.Fatal("bridge cancel without trading service succeeded")
	}
}
