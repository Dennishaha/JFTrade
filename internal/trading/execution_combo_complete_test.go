package trading

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestExecutionComboCompletePreviewPlaceCancelAndBuyingPower(t *testing.T) {
	selected := &completeComboBroker{id: "combo-complete"}
	previewStore := &comboPreviewStore{}
	placed := 0
	canceled := 0
	risk := &capturingRiskGateway{decision: PreTradeRiskDecision{Decision: RiskDecisionAllow}}
	service := NewService(
		WithActiveBroker(func() broker.Broker { return selected }),
		WithExecutionPreviewStore(previewStore),
		WithComboOrderGateway(&comboOrderGatewayFunctions{
			place: func(_ context.Context, intent broker.ComboOrderIntent) (ExecutionOrder, error) {
				placed++
				if intent.BrokerID != selected.id || intent.PreviewID == "" {
					t.Fatalf("place combo intent = %#v", intent)
				}
				return ExecutionOrder{InternalOrderID: "internal-combo"}, nil
			},
			cancel: func(_ context.Context, id string) (ExecutionOrder, error) {
				canceled++
				return ExecutionOrder{InternalOrderID: id, Status: "CANCEL_SUBMITTED"}, nil
			},
		}),
		WithPreTradeRiskGateway(risk),
	)

	buyingPower, err := service.PreviewExecutionBuyingPower(t.Context(), broker.ProductRuleQuery{
		ReadQuery:  broker.ReadQuery{BrokerID: selected.id, Market: "US"},
		Instrument: broker.Instrument{ProductClass: broker.ProductClassOption},
	})
	if err != nil || !buyingPower.Allowed || selected.ruleQuery.FeatureID != broker.FeatureExecutionBuyingPower {
		t.Fatalf("buying power = %#v query=%#v err=%v", buyingPower, selected.ruleQuery, err)
	}

	quantity := 2.0
	price := 1.25
	spread := 10.0
	earlyExpiry := time.Now().Add(2 * time.Minute)
	request := ExecutionComboRequest{
		BrokerID: selected.id, AccountID: "account-1", Market: "us",
		ClientOrderID: "client-combo-1", OrderKind: broker.OrderKindOptionCombo,
		UnderlyingID: "US.AAPL", OptionStrategy: "vertical",
		NearExpiry: "2026-07-17", Spread: &spread,
		QuoteExpiresAt: &earlyExpiry, Price: &price,
		Legs: []broker.OrderLegIntent{
			{InstrumentID: " us.aapl260717c00200000 ", Side: " buy ", Ratio: 1, Quantity: &quantity},
			{InstrumentID: "us.aapl260717c00210000", Side: "sell", Ratio: 1, Quantity: &quantity},
		},
	}
	preview, err := service.PreviewExecutionCombo(t.Context(), request)
	if err != nil || !preview.Allowed || preview.ProductClass != broker.ProductClassOption ||
		previewStore.saved.PreviewID != preview.PreviewID ||
		!strings.Contains(previewStore.saved.NormalizedRequest, "option_combo") {
		t.Fatalf("combo preview = %#v saved=%#v err=%v", preview, previewStore.saved, err)
	}
	request.PreviewID = preview.PreviewID
	placedResponse, err := service.CreateExecutionCombo(t.Context(), request)
	if err != nil || placedResponse.Operation != "PLACE_COMBO" || placed != 1 ||
		previewStore.consumed != 1 || risk.command.Query.Quantity != quantity ||
		risk.command.Query.QuantityMode != broker.QuantityModeContracts {
		t.Fatalf("place response=%#v placed=%d store=%#v risk=%#v err=%v",
			placedResponse, placed, previewStore, risk.command, err)
	}
	cancelResponse, err := service.CancelExecutionCombo(t.Context(), " internal-combo ")
	if err != nil || cancelResponse.Operation != "CANCEL_COMBO" || canceled != 1 {
		t.Fatalf("cancel response=%#v canceled=%d err=%v", cancelResponse, canceled, err)
	}
}

func TestExecutionEventParlayCompletePreviewAndAmountRisk(t *testing.T) {
	selected := &completeComboBroker{id: "event-complete"}
	risk := &capturingRiskGateway{decision: PreTradeRiskDecision{Decision: RiskDecisionAllow}}
	placedIntent := broker.ComboOrderIntent{}
	service := NewService(
		WithActiveBroker(func() broker.Broker { return selected }),
		WithPreTradeRiskGateway(risk),
		WithComboOrderGateway(&comboOrderGatewayFunctions{place: func(
			_ context.Context,
			intent broker.ComboOrderIntent,
		) (ExecutionOrder, error) {
			placedIntent = intent
			return ExecutionOrder{InternalOrderID: "parlay-order"}, nil
		}}),
	)
	amount := 25.0
	expires := time.Now().Add(time.Minute)
	request := ExecutionComboRequest{
		BrokerID: selected.id, AccountID: "us-1", Market: "US",
		ClientOrderID: "parlay-client", OrderKind: broker.OrderKindEventParlay,
		RFQID: "rfq-1", QuoteExpiresAt: &expires, Amount: &amount,
		Legs: []broker.OrderLegIntent{
			{InstrumentID: "US.EVENT.ONE", Side: "BUY", Ratio: 1, PredictionSide: "yes"},
			{InstrumentID: "US.EVENT.TWO", Side: "BUY", Ratio: 1, PredictionSide: "no"},
		},
	}
	preview, err := service.PreviewExecutionCombo(t.Context(), request)
	if err != nil || preview.ProductClass != broker.ProductClassEventContract {
		t.Fatalf("parlay preview = %#v, %v", preview, err)
	}
	request.PreviewID = preview.PreviewID
	if _, err := service.CreateExecutionCombo(t.Context(), request); err != nil {
		t.Fatalf("place parlay: %v", err)
	}
	if placedIntent.RFQID != "rfq-1" || risk.command.Query.Quantity != amount ||
		risk.command.Query.QuantityMode != broker.QuantityModeAmount {
		t.Fatalf("placed parlay=%#v risk=%#v", placedIntent, risk.command)
	}
}

func TestExecutionComboPreviewKeepsLegacyBuyingPowerCompatible(t *testing.T) {
	decrease := 42.0
	selected := &completeComboBroker{
		id:               "combo-impact",
		usePreviewResult: true,
		previewResult: &broker.ProductRuleResult{
			Allowed: true,
			AccountImpact: &broker.OptionComboAccountImpact{
				BuyingPowerDecrease: &decrease,
			},
		},
	}
	service := NewService(WithActiveBroker(func() broker.Broker { return selected }))
	quantity := 1.0
	request := ExecutionComboRequest{
		BrokerID: selected.id, AccountID: "account", Market: "US",
		ClientOrderID: "impact-client", OrderKind: broker.OrderKindOptionCombo,
		UnderlyingID: "US.AAPL", OptionStrategy: "straddle",
		NearExpiry: "2026-07-17",
		Legs: []broker.OrderLegIntent{
			{InstrumentID: "US.CALL", Side: "BUY", Ratio: 1, Quantity: &quantity},
			{InstrumentID: "US.PUT", Side: "BUY", Ratio: 1, Quantity: &quantity},
		},
	}
	preview, err := service.PreviewExecutionCombo(t.Context(), request)
	if err != nil || preview.BuyingPowerImpact == nil ||
		*preview.BuyingPowerImpact != decrease ||
		preview.AccountImpact == nil {
		t.Fatalf("combo impact preview = %#v, %v", preview, err)
	}
}

func TestExecutionComboRejectsEveryUnsafeBoundary(t *testing.T) {
	selected := &completeComboBroker{id: "combo-boundaries"}
	service := NewService(WithActiveBroker(func() broker.Broker { return selected }))
	quantity := 1.0
	validLegs := []broker.OrderLegIntent{
		{InstrumentID: "US.OPTION.ONE", Side: "BUY", Ratio: 1, Quantity: &quantity},
		{InstrumentID: "US.OPTION.TWO", Side: "SELL", Ratio: 1, Quantity: &quantity},
	}
	valid := ExecutionComboRequest{
		BrokerID: selected.id, Market: "US", ClientOrderID: "stable-client",
		OrderKind: broker.OrderKindOptionCombo, Legs: validLegs,
		UnderlyingID: "US.AAPL", OptionStrategy: "vertical",
		NearExpiry: "2026-07-17", Spread: &quantity,
	}
	cases := []struct {
		name   string
		mutate func(*ExecutionComboRequest)
		part   string
	}{
		{"unknown kind", func(r *ExecutionComboRequest) { r.OrderKind = broker.OrderKindSingle }, "orderKind"},
		{"too few legs", func(r *ExecutionComboRequest) { r.Legs = r.Legs[:1] }, "at least two"},
		{"missing client", func(r *ExecutionComboRequest) { r.ClientOrderID = "" }, "clientOrderId"},
		{"mixed product", func(r *ExecutionComboRequest) {
			r.ProductClass = broker.ProductClassOption
			r.Legs[1].ProductClass = broker.ProductClassEventContract
		}, "cannot mix"},
		{"bad leg", func(r *ExecutionComboRequest) { r.Legs[0].Ratio = 0 }, "each combo leg"},
	}
	for _, test := range cases {
		request := valid
		request.Legs = append([]broker.OrderLegIntent(nil), valid.Legs...)
		test.mutate(&request)
		if _, err := service.PreviewExecutionCombo(t.Context(), request); !IsRequestError(err) ||
			!strings.Contains(err.Error(), test.part) {
			t.Errorf("%s error = %v", test.name, err)
		}
	}

	amount := 10.0
	future := time.Now().Add(time.Minute)
	event := ExecutionComboRequest{
		BrokerID: selected.id, Market: "US", ClientOrderID: "parlay",
		OrderKind: broker.OrderKindEventParlay, RFQID: "rfq", Amount: &amount,
		QuoteExpiresAt: &future,
		Legs: []broker.OrderLegIntent{
			{InstrumentID: "US.ONE", Side: "BUY", Ratio: 1, PredictionSide: "YES"},
			{InstrumentID: "US.TWO", Side: "BUY", Ratio: 1, PredictionSide: "NO"},
		},
	}
	eventCases := []struct {
		name   string
		mutate func(*ExecutionComboRequest)
		part   string
	}{
		{"wrong market", func(r *ExecutionComboRequest) { r.Market = "HK" }, "market US"},
		{"missing expiry", func(r *ExecutionComboRequest) { r.QuoteExpiresAt = nil }, "quote expired"},
		{"missing rfq", func(r *ExecutionComboRequest) { r.RFQID = "" }, "rfqId"},
		{"zero amount", func(r *ExecutionComboRequest) {
			zero := 0.0
			r.Amount = &zero
		}, "positive amount"},
		{"missing prediction side", func(r *ExecutionComboRequest) {
			r.Legs[1].PredictionSide = ""
		}, "predictionSide"},
	}
	for _, test := range eventCases {
		request := event
		request.Legs = append([]broker.OrderLegIntent(nil), event.Legs...)
		test.mutate(&request)
		if _, err := service.PreviewExecutionCombo(t.Context(), request); !IsRequestError(err) ||
			!strings.Contains(err.Error(), test.part) {
			t.Errorf("%s error = %v", test.name, err)
		}
	}

	valid.PreviewID = ""
	if _, err := service.CreateExecutionCombo(t.Context(), valid); !IsRequestError(err) ||
		!strings.Contains(err.Error(), "previewId") {
		t.Fatalf("place without preview error = %v", err)
	}
}

func TestExecutionComboProviderStoreRiskAndGatewayFailures(t *testing.T) {
	selected := &completeComboBroker{id: "combo-failures"}
	quantity := 1.0
	request := ExecutionComboRequest{
		BrokerID: selected.id, Market: "US", ClientOrderID: "client",
		OrderKind:    broker.OrderKindOptionCombo,
		UnderlyingID: "US.AAPL", OptionStrategy: "vertical",
		NearExpiry: "2026-07-17", Spread: &quantity,
		Legs: []broker.OrderLegIntent{
			{InstrumentID: "US.ONE", Side: "BUY", Ratio: 1, Quantity: &quantity},
			{InstrumentID: "US.TWO", Side: "SELL", Ratio: 1, Quantity: &quantity},
		},
	}

	noBroker := NewService()
	if _, err := noBroker.PreviewExecutionBuyingPower(t.Context(), broker.ProductRuleQuery{}); !IsRequestError(err) {
		t.Fatalf("buying power without broker error = %v", err)
	}
	bare := &comboBareBroker{id: "bare-combo"}
	unsupported := NewService(WithActiveBroker(func() broker.Broker { return bare }))
	if _, err := unsupported.PreviewExecutionBuyingPower(t.Context(), broker.ProductRuleQuery{}); !IsRequestError(err) {
		t.Fatalf("buying power unsupported error = %v", err)
	}
	if _, err := unsupported.PreviewExecutionCombo(t.Context(), ExecutionComboRequest{
		BrokerID: bare.id, Market: "US", ClientOrderID: "client",
		OrderKind: broker.OrderKindOptionCombo, Legs: request.Legs,
		UnderlyingID: request.UnderlyingID, OptionStrategy: request.OptionStrategy,
		NearExpiry: request.NearExpiry, Spread: request.Spread,
	}); !IsRequestError(err) {
		t.Fatalf("combo unsupported error = %v", err)
	}

	selected.previewErr = errors.New("preview upstream failed")
	service := NewService(WithActiveBroker(func() broker.Broker { return selected }))
	if _, err := service.PreviewExecutionCombo(t.Context(), request); err == nil {
		t.Fatal("preview upstream error was hidden")
	}
	selected.previewErr = nil
	selected.previewResult = nil
	selected.usePreviewResult = true
	if _, err := service.PreviewExecutionCombo(t.Context(), request); !IsRequestError(err) {
		t.Fatalf("nil rejected preview error = %v", err)
	}
	selected.previewResult = &broker.ProductRuleResult{Reason: "illegal spread"}
	if _, err := service.PreviewExecutionCombo(t.Context(), request); !IsRequestError(err) ||
		!strings.Contains(err.Error(), "illegal spread") {
		t.Fatalf("reasoned rejected preview error = %v", err)
	}
	selected.previewResult = &broker.ProductRuleResult{Allowed: true}
	store := &comboPreviewStore{saveErr: errors.New("save failed")}
	service = NewService(
		WithActiveBroker(func() broker.Broker { return selected }),
		WithExecutionPreviewStore(store),
	)
	if _, err := service.PreviewExecutionCombo(t.Context(), request); err == nil {
		t.Fatal("preview save error was hidden")
	}

	store.saveErr = nil
	preview, err := service.PreviewExecutionCombo(t.Context(), request)
	if err != nil {
		t.Fatalf("preview for failure tests: %v", err)
	}
	request.PreviewID = preview.PreviewID
	store.consumeErr = errors.New("already consumed")
	if _, err := service.CreateExecutionCombo(t.Context(), request); !IsRequestError(err) {
		t.Fatalf("consume error = %v", err)
	}
	store.consumeErr = nil
	rejectingRisk := &capturingRiskGateway{decision: PreTradeRiskDecision{
		Decision: RiskDecisionReject, ReasonCode: "DENY",
	}}
	service = NewService(
		WithActiveBroker(func() broker.Broker { return selected }),
		WithExecutionPreviewStore(store),
		WithPreTradeRiskGateway(rejectingRisk),
	)
	if _, err := service.CreateExecutionCombo(t.Context(), request); !IsRiskRejected(err) {
		t.Fatalf("risk rejection error = %v", err)
	}
	service = NewService(WithActiveBroker(func() broker.Broker { return selected }))
	if _, err := service.CreateExecutionCombo(t.Context(), request); !errors.Is(err, ErrOrderGatewayUnavailable) {
		t.Fatalf("missing combo gateway error = %v", err)
	}
	if _, err := service.CancelExecutionCombo(t.Context(), "id"); !errors.Is(err, ErrOrderGatewayUnavailable) {
		t.Fatalf("missing cancel gateway error = %v", err)
	}
}

func TestExecutionComboHelperBranches(t *testing.T) {
	quantity := 3.0
	amount := 15.0
	if got := comboRiskQuantity(broker.ComboOrderIntent{Amount: &amount}); got != amount {
		t.Fatalf("amount risk quantity = %v", got)
	}
	if got := comboRiskQuantity(broker.ComboOrderIntent{Legs: []broker.OrderLegIntent{{
		Quantity: &quantity,
	}}}); got != quantity {
		t.Fatalf("leg risk quantity = %v", got)
	}
	if got := comboRiskQuantity(broker.ComboOrderIntent{}); got != 1 {
		t.Fatalf("default risk quantity = %v", got)
	}
	if comboQuantityMode(broker.OrderKindEventParlay) != broker.QuantityModeAmount ||
		comboQuantityMode(broker.OrderKindOptionCombo) != broker.QuantityModeContracts {
		t.Fatal("combo quantity mode mismatch")
	}
	if got := normalizedComboIntent(broker.ComboOrderIntent{ClientOrderID: "client"}); !strings.Contains(got, "client") {
		t.Fatalf("normalized combo intent = %s", got)
	}
	if _, err := (&comboOrderGatewayFunctions{}).PlaceCombo(t.Context(), broker.ComboOrderIntent{}); !errors.Is(err, ErrOrderGatewayUnavailable) {
		t.Fatalf("empty place gateway error = %v", err)
	}
	if _, err := (*comboOrderGatewayFunctions)(nil).CancelCombo(t.Context(), "id"); !errors.Is(err, ErrOrderGatewayUnavailable) {
		t.Fatalf("nil cancel gateway error = %v", err)
	}
}

func TestExecutionProductPreviewAndSubmissionFailureContractsComplete(t *testing.T) {
	ctx := t.Context()
	price := 2.5
	optionRequest := ExecutionPlaceRequest{
		BrokerID: "rules", Market: "US", Symbol: "AAPL260717C00200000",
		ProductClass: broker.ProductClassOption, Side: "BUY",
		Quantity: 1, Price: &price, ClientOrderID: "option-client",
	}

	noDefault := NewService()
	if _, _, err := noDefault.resolveExecutionBroker(""); !IsRequestError(err) {
		t.Fatalf("missing default broker error = %v", err)
	}
	other := &completeComboBroker{id: "other"}
	mismatch := NewService(WithActiveBroker(func() broker.Broker { return other }))
	if _, _, err := mismatch.resolveExecutionBroker("rules"); !IsRequestError(err) {
		t.Fatalf("explicit mismatched broker error = %v", err)
	}
	if _, err := mismatch.PreviewExecutionBuyingPower(ctx, broker.ProductRuleQuery{
		ReadQuery: broker.ReadQuery{BrokerID: "rules"},
	}); !IsRequestError(err) {
		t.Fatalf("mismatched buying power error = %v", err)
	}

	bare := &comboBareBroker{id: "rules"}
	service := NewService(WithActiveBroker(func() broker.Broker { return bare }))
	if _, err := service.PreviewExecutionOrderContext(ctx, optionRequest); !IsRequestError(err) ||
		!strings.Contains(err.Error(), "product rule") {
		t.Fatalf("unsupported product rule error = %v", err)
	}

	selected := &completeComboBroker{id: "rules"}
	service = NewService(WithActiveBroker(func() broker.Broker { return selected }))
	selected.ruleErr = errors.New("rule service failed")
	if _, err := service.PreviewExecutionOrderContext(ctx, optionRequest); !errors.Is(err, selected.ruleErr) {
		t.Fatalf("product rule upstream error = %v", err)
	}
	selected.ruleErr = nil
	selected.useRuleResult = true
	selected.ruleResult = nil
	if _, err := service.PreviewExecutionOrderContext(ctx, optionRequest); !IsRequestError(err) ||
		!strings.Contains(err.Error(), "rejected") {
		t.Fatalf("nil product rule result error = %v", err)
	}
	selected.ruleResult = &broker.ProductRuleResult{Reason: "unsupported option session"}
	if _, err := service.PreviewExecutionOrderContext(ctx, optionRequest); !IsRequestError(err) ||
		!strings.Contains(err.Error(), "unsupported option session") {
		t.Fatalf("reasoned product rule result error = %v", err)
	}

	optionRequest.PreviewID = "preview-locked"
	optionRequest.ClientOrderID = ""
	if _, err := service.CreateExecutionOrder(ctx, optionRequest); !IsRequestError(err) ||
		!strings.Contains(err.Error(), "clientOrderId") {
		t.Fatalf("locked preview without client id error = %v", err)
	}
	equityWithPreview := ExecutionPlaceRequest{
		BrokerID: "rules", Market: "US", Symbol: "AAPL", Side: "BUY",
		Quantity: 1, Price: &price, PreviewID: "preview-equity",
	}
	if _, err := service.CreateExecutionOrder(ctx, equityWithPreview); !IsRequestError(err) ||
		!strings.Contains(err.Error(), "clientOrderId") {
		t.Fatalf("equity preview without client id error = %v", err)
	}
	store := &comboPreviewStore{consumeErr: errors.New("preview expired")}
	service = NewService(
		WithActiveBroker(func() broker.Broker { return selected }),
		WithExecutionPreviewStore(store),
	)
	equityWithPreview.ClientOrderID = "equity-client"
	if _, err := service.CreateExecutionOrder(ctx, equityWithPreview); !IsRequestError(err) ||
		!strings.Contains(err.Error(), "preview expired") {
		t.Fatalf("single-order preview consumption error = %v", err)
	}

	selected.accountErr = errors.New("accounts unavailable")
	if err := validateFuturesTradingAuthority(ctx, selected, "future-account"); !IsRequestError(err) {
		t.Fatalf("futures account discovery error = %v", err)
	}
	if err := validatePredictionTradingAccount(ctx, selected, "event-account"); !IsRequestError(err) {
		t.Fatalf("prediction account discovery error = %v", err)
	}
	selected.accountErr = nil
	firm := "FUTUINC"
	selected.accounts = []broker.Account{{
		ID: "other-account", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
	}}
	if err := validateFuturesTradingAuthority(ctx, selected, "future-account"); !IsRequestError(err) {
		t.Fatalf("mismatched futures account error = %v", err)
	}
	if err := validatePredictionTradingAccount(ctx, selected, "event-account"); !IsRequestError(err) {
		t.Fatalf("mismatched prediction account error = %v", err)
	}

	usInstrument := market.Instrument{Market: "US", Symbol: "US.EVENT"}
	amount := 10.0
	predictionCases := []ExecutionPlaceRequest{
		{Amount: nil, PredictionSide: "YES", Price: new(0.5)},
		{Amount: &amount, PredictionSide: "MAYBE", Price: new(0.5)},
		{Amount: &amount, PredictionSide: "YES", Price: nil},
	}
	for index := range predictionCases {
		if err := normalizePredictionOrder(&predictionCases[index], usInstrument); !IsRequestError(err) {
			t.Errorf("prediction validation case %d error = %v", index, err)
		}
	}
	if err := normalizePredictionOrder(
		&ExecutionPlaceRequest{Amount: &amount, PredictionSide: "YES", Price: new(0.5)},
		market.Instrument{Market: "HK", Symbol: "HK.EVENT"},
	); !IsRequestError(err) {
		t.Fatalf("non-US prediction error = %v", err)
	}
}

func TestExecutionProductRemainingLifecycleAndUpdateHelpers(t *testing.T) {
	ctx := t.Context()
	price := 100.0
	selected := &completeComboBroker{id: "helper-broker"}
	store := &comboPreviewStore{saveErr: errors.New("preview storage unavailable")}
	service := NewService(
		WithActiveBroker(func() broker.Broker { return selected }),
		WithExecutionPreviewStore(store),
	)
	if _, err := service.PreviewExecutionOrderContext(ctx, ExecutionPlaceRequest{
		BrokerID: selected.id, Market: "US", Symbol: "AAPL",
		Side: "BUY", Quantity: 1, Price: &price,
	}); !errors.Is(err, store.saveErr) {
		t.Fatalf("single preview storage error = %v", err)
	}

	if _, _, _, err := normalizeExecutionProduct(
		&ExecutionPlaceRequest{OrderKind: broker.OrderKindOptionCombo},
		market.Instrument{Market: "US", Symbol: "US.AAPL"},
	); !IsRequestError(err) {
		t.Fatalf("combo through single endpoint error = %v", err)
	}
	if _, _, _, err := normalizeExecutionProduct(
		&ExecutionPlaceRequest{
			ProductClass: broker.ProductClassOption, Quantity: 1.5,
		},
		market.Instrument{Market: "US", Symbol: "US.OPTION"},
	); !IsRequestError(err) {
		t.Fatalf("fractional option quantity error = %v", err)
	}
	if _, _, _, _, err := service.normalizeExecutionTerms(
		ExecutionPlaceRequest{Session: "ETH"},
		broker.ProductClassOption,
		"US",
		"LIMIT",
	); !IsRequestError(err) {
		t.Fatalf("option extended-hours error = %v", err)
	}

	queries := BuildOrderUpdateQueries([]Account{
		{ID: "one", BrokerID: "broker", MarketAuthorities: nil},
		{ID: "one", BrokerID: "broker", MarketAuthorities: []string{"US", "US"}},
	}, "broker", "US")
	if len(queries) != 1 || queries[0].Market != "US" {
		t.Fatalf("deduplicated order update queries = %#v", queries)
	}
	if CanonicalStoredOrderStatus("SUBMISSION_UNKNOWN") != OrderStatusSubmissionUnknown {
		t.Fatal("submission-unknown canonical status changed")
	}
	if CanonicalStoredOrderStatus("not-a-broker-status") != OrderStatusUnknown {
		t.Fatal("unknown stored status did not use broker fallback")
	}
	if status, accepted := ReconcileCanonicalOrderStatus(
		OrderStatusSubmitted,
		OrderStatusSubmitted,
	); !accepted || status != OrderStatusSubmitted {
		t.Fatalf("same-state reconciliation = %q, %v", status, accepted)
	}
	if snapshot := (*OrderUpdatesWorker)(nil).SnapshotResponse(); len(snapshot) != 0 {
		t.Fatalf("nil order update snapshot = %#v", snapshot)
	}
	jftradeLogError("ignored", errors.New("expected test log"))
}

func TestExecutionDetailsResolverAndOrderUpdateCacheFailureBranches(t *testing.T) {
	ctx := t.Context()
	if _, err := NewService().ExecutionOrderDetails(ctx, " "); !IsRequestError(err) {
		t.Fatalf("blank execution details id error = %v", err)
	}
	listErr := errors.New("order ledger unavailable")
	listFailure := NewService(WithListOrders(func(
		context.Context,
		ExecutionOrderFilter,
	) (ExecutionOrders, error) {
		return ExecutionOrders{}, listErr
	}))
	if _, err := listFailure.ExecutionOrderDetails(ctx, "order-1"); !errors.Is(err, listErr) {
		t.Fatalf("execution details list error = %v", err)
	}
	eventErr := errors.New("order events unavailable")
	eventFailure := NewService(
		WithListOrders(func(context.Context, ExecutionOrderFilter) (ExecutionOrders, error) {
			return ExecutionOrders{Orders: []ExecutionOrder{{
				InternalOrderID: "order-1", Status: OrderStatusFilled,
			}}}, nil
		}),
		WithGetOrderEvents(func(context.Context, string) (ExecutionOrderEvents, error) {
			return ExecutionOrderEvents{}, eventErr
		}),
	)
	if _, err := eventFailure.ExecutionOrderDetails(ctx, "order-1"); !errors.Is(err, eventErr) {
		t.Fatalf("execution details events error = %v", err)
	}

	selected := &completeComboBroker{id: "resolved"}
	runtime := &resolvingExecutionRuntime{active: selected, resolved: selected}
	service := NewService(WithBrokerRuntimeProvider(runtime))
	if id, resolved, err := service.resolveExecutionBroker("resolved"); err != nil ||
		id != "resolved" || resolved != selected {
		t.Fatalf("resolver-backed broker = %q, %#v, %v", id, resolved, err)
	}
	if _, err := service.PreviewExecutionBuyingPower(ctx, broker.ProductRuleQuery{
		ReadQuery: broker.ReadQuery{BrokerID: "resolved"},
	}); err != nil {
		t.Fatalf("resolver-backed buying power: %v", err)
	}
	runtime.resolved = nil
	command := ExecutionOrderCommand{
		BrokerID: "missing", ProductClass: broker.ProductClassOption,
		Query: broker.PlaceOrderQuery{
			ReadQuery: broker.ReadQuery{BrokerID: "missing", Market: "US"},
			Quantity:  1,
		},
	}
	if err := service.validateProductOrderPreview(ctx, command); !IsRequestError(err) {
		t.Fatalf("resolver returned nil product preview error = %v", err)
	}
	mismatched := NewService(WithActiveBroker(func() broker.Broker { return selected }))
	if err := mismatched.validateProductOrderPreview(ctx, command); !IsRequestError(err) {
		t.Fatalf("active broker mismatch product preview error = %v", err)
	}
	if _, err := NewService().normalizeExecutionOrder(ExecutionPlaceRequest{
		Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 1, Price: new(100.0),
	}); !IsRequestError(err) {
		t.Fatalf("normalized order without default broker error = %v", err)
	}
	if _, err := NewService().PreviewExecutionCombo(ctx, ExecutionComboRequest{}); !IsRequestError(err) {
		t.Fatalf("combo without default broker error = %v", err)
	}
	if _, _, _, err := normalizeExecutionProduct(
		&ExecutionPlaceRequest{
			ProductClass: broker.ProductClassEventContract,
			OrderKind:    broker.OrderKindEventSingle,
		},
		market.Instrument{Market: "US", Symbol: "US.EVENT"},
	); !IsRequestError(err) {
		t.Fatalf("invalid event product normalization error = %v", err)
	}

	now := time.Now()
	worker := NewOrderUpdatesWorker(
		&fakeOrderUpdateSource{},
		&fakeExecutionOrderUpdates{},
		OrderUpdatesConfig{Now: func() time.Time { return now }},
	)
	worker.activeOrdersCachedAt["missing-cache-entry"] = now
	if orders, ok := worker.cachedActiveOrders("missing-cache-entry"); ok || orders != nil {
		t.Fatalf("missing active-order cache entry = %#v, %v", orders, ok)
	}
	worker.removeActiveOrder("missing-subscription", "1", nil)
	worker.activeOrdersCache["orders"] = []Order{{BrokerOrderID: "keep"}}
	worker.removeActiveOrder("orders", "other", nil)
	if len(worker.activeOrdersCache["orders"]) != 1 {
		t.Fatalf("non-matching active order was removed: %#v", worker.activeOrdersCache)
	}
	worker.subscriptions["nil-state"] = nil
	if snapshot := worker.SnapshotResponse(); snapshot["subscriptions"] == nil {
		t.Fatalf("snapshot with nil subscription state = %#v", snapshot)
	}
	delete(worker.subscriptions, "nil-state")
	if err := worker.Stop(); err != nil {
		t.Fatalf("worker stop without subscription: %v", err)
	}
}

type resolvingExecutionRuntime struct {
	active   broker.Broker
	resolved broker.Broker
}

func (r *resolvingExecutionRuntime) ActiveBroker() broker.Broker { return r.active }

func (r *resolvingExecutionRuntime) ResolveBroker(string) broker.Broker { return r.resolved }

func (r *resolvingExecutionRuntime) Runtime(context.Context) map[string]any {
	return map[string]any{"resolved": r.resolved != nil}
}

type completeComboBroker struct {
	id               string
	accounts         []broker.Account
	accountErr       error
	ruleErr          error
	ruleResult       *broker.ProductRuleResult
	useRuleResult    bool
	ruleQuery        broker.ProductRuleQuery
	previewErr       error
	previewResult    *broker.ProductRuleResult
	usePreviewResult bool
}

func (b *completeComboBroker) ID() string { return b.id }
func (b *completeComboBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: b.id}
}
func (b *completeComboBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return append([]broker.Account(nil), b.accounts...), b.accountErr
}
func (b *completeComboBroker) Trading() broker.TradingService      { return nil }
func (b *completeComboBroker) MarketData() broker.MarketDataReader { return nil }
func (b *completeComboBroker) ValidateProductOrder(
	_ context.Context,
	query broker.ProductRuleQuery,
) (*broker.ProductRuleResult, error) {
	b.ruleQuery = query
	if b.ruleErr != nil {
		return nil, b.ruleErr
	}
	if b.useRuleResult {
		return b.ruleResult, nil
	}
	return &broker.ProductRuleResult{Allowed: true}, nil
}
func (b *completeComboBroker) PreviewComboOrder(
	context.Context,
	broker.ComboOrderIntent,
) (*broker.ProductRuleResult, error) {
	if b.previewErr != nil {
		return nil, b.previewErr
	}
	if b.usePreviewResult {
		return b.previewResult, nil
	}
	return &broker.ProductRuleResult{
		Allowed: true, Warnings: []string{"test warning"},
	}, nil
}
func (b *completeComboBroker) PlaceComboOrder(context.Context, broker.ComboOrderIntent) (*broker.ComboOrderResult, error) {
	return &broker.ComboOrderResult{}, nil
}
func (b *completeComboBroker) CancelComboOrder(context.Context, broker.ReadQuery, string) error {
	return nil
}

type comboBareBroker struct{ id string }

func (b *comboBareBroker) ID() string                    { return b.id }
func (b *comboBareBroker) Descriptor() broker.Descriptor { return broker.Descriptor{ID: b.id} }
func (b *comboBareBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (b *comboBareBroker) Trading() broker.TradingService      { return nil }
func (b *comboBareBroker) MarketData() broker.MarketDataReader { return nil }

type comboPreviewStore struct {
	saved      ExecutionPreviewRecord
	saveErr    error
	consumeErr error
	consumed   int
}

func (s *comboPreviewStore) SavePreview(record ExecutionPreviewRecord) error {
	s.saved = record
	return s.saveErr
}
func (s *comboPreviewStore) ConsumePreview(_, _, _, _, _ string) error {
	s.consumed++
	return s.consumeErr
}

type capturingRiskGateway struct {
	decision PreTradeRiskDecision
	command  ExecutionOrderCommand
}

func (g *capturingRiskGateway) EvaluatePlaceOrder(
	_ context.Context,
	command ExecutionOrderCommand,
) PreTradeRiskDecision {
	g.command = command
	return g.decision
}
func (g *capturingRiskGateway) Snapshot() map[string]any { return map[string]any{} }
