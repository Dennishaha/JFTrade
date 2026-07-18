package trading

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestDerivativeSingleLegRequiresBrokerPreviewAndStableClientID(t *testing.T) {
	selected := &advancedTradingBroker{id: "partial-options"}
	service := NewService(WithActiveBroker(func() broker.Broker { return selected }))
	price := 2.25

	_, err := service.PreviewExecutionOrderContext(t.Context(), ExecutionPlaceRequest{
		BrokerID: "partial-options", Market: "US", Symbol: "AAPL260717C00200000",
		ProductClass: broker.ProductClassOption, Side: "BUY", Quantity: 1, Price: &price,
	})
	if !IsRequestError(err) || !strings.Contains(err.Error(), "clientOrderId") {
		t.Fatalf("preview without clientOrderId error = %v", err)
	}

	preview, err := service.PreviewExecutionOrderContext(t.Context(), ExecutionPlaceRequest{
		BrokerID: "partial-options", Market: "US", Symbol: "AAPL260717C00200000",
		ProductClass: broker.ProductClassOption, Side: "BUY", Quantity: 1, Price: &price,
		ClientOrderID: "option-client-1",
	})
	if err != nil {
		t.Fatalf("option preview: %v", err)
	}
	if !preview.PreviewValid || selected.productRuleCalls != 1 ||
		preview.ProductClass != broker.ProductClassOption {
		t.Fatalf("preview=%#v productRuleCalls=%d", preview, selected.productRuleCalls)
	}

	_, err = service.CreateExecutionOrder(t.Context(), ExecutionPlaceRequest{
		BrokerID: "partial-options", Market: "US", Symbol: "AAPL260717C00200000",
		ProductClass: broker.ProductClassOption, Side: "BUY", Quantity: 1, Price: &price,
		ClientOrderID: "option-client-1",
	})
	if !IsRequestError(err) || !strings.Contains(err.Error(), "previewId") {
		t.Fatalf("place without locked preview error = %v", err)
	}
}

func TestPredictionSingleLegEligibilityUsesSecurityFirmAndUSAuthority(t *testing.T) {
	price := 0.62
	amount := 25.0
	hkFirm := "FUTUSECURITIES"
	ineligible := &advancedTradingBroker{id: "futu-hk", accounts: []broker.Account{{
		ID: "hk-1", SecurityFirm: &hkFirm, MarketAuthorities: []string{"HK"},
	}}}
	service := NewService(WithActiveBroker(func() broker.Broker { return ineligible }))
	request := ExecutionPlaceRequest{
		BrokerID: "futu-hk", AccountID: "hk-1", Market: "US", Code: "EC.TEST",
		ProductClass: broker.ProductClassEventContract, OrderKind: broker.OrderKindEventSingle,
		Side: "BUY", Amount: &amount, Price: &price, PredictionSide: "YES",
		ClientOrderID: "event-client-1",
	}
	if _, err := service.PreviewExecutionOrderContext(t.Context(), request); !IsRequestError(err) ||
		!strings.Contains(err.Error(), "Moomoo US") {
		t.Fatalf("ineligible prediction preview error = %v", err)
	}

	usFirm := "FUTUINC"
	eligible := &advancedTradingBroker{id: "moomoo-us", accounts: []broker.Account{{
		ID: "us-1", SecurityFirm: &usFirm, MarketAuthorities: []string{"US"},
	}}}
	service = NewService(WithActiveBroker(func() broker.Broker { return eligible }))
	request.BrokerID = "moomoo-us"
	request.AccountID = "us-1"
	preview, err := service.PreviewExecutionOrderContext(t.Context(), request)
	if err != nil {
		t.Fatalf("eligible prediction preview: %v", err)
	}
	if preview.OrderKind != broker.OrderKindEventSingle ||
		preview.QuantityMode != broker.QuantityModeAmount ||
		eligible.productRuleCalls != 1 {
		t.Fatalf("prediction preview=%#v calls=%d", preview, eligible.productRuleCalls)
	}
}

func TestRealFuturesPreviewRequiresFuturesAuthority(t *testing.T) {
	price := 5120.0
	selected := &advancedTradingBroker{id: "futures-broker", accounts: []broker.Account{{
		ID: "real-1", MarketAuthorities: []string{"US"},
	}}}
	service := NewService(WithActiveBroker(func() broker.Broker { return selected }))
	request := ExecutionPlaceRequest{
		BrokerID: "futures-broker", AccountID: "real-1", TradingEnvironment: "REAL",
		Market: "US", Symbol: "ESmain", ProductClass: broker.ProductClassFuture,
		Side: "BUY", Quantity: 1, Price: &price, ClientOrderID: "future-client-1",
	}
	if _, err := service.PreviewExecutionOrderContext(t.Context(), request); !IsRequestError(err) ||
		!strings.Contains(err.Error(), "FUTURES") {
		t.Fatalf("preview without futures authority error = %v", err)
	}
	selected.accounts[0].MarketAuthorities = append(selected.accounts[0].MarketAuthorities, "FUTURES")
	if _, err := service.PreviewExecutionOrderContext(t.Context(), request); err != nil {
		t.Fatalf("preview with futures authority: %v", err)
	}
}

func TestComboPreviewRejectsMixedProductsAndExpiredParlayRFQ(t *testing.T) {
	selected := &advancedTradingBroker{id: "combo-broker"}
	service := NewService(WithActiveBroker(func() broker.Broker { return selected }))
	quantity := 1.0
	optionLegs := []broker.OrderLegIntent{
		{InstrumentID: "US.AAPL260717C00200000", ProductClass: broker.ProductClassOption, Side: "BUY", Ratio: 1, Quantity: &quantity},
		{InstrumentID: "US.AAPL260717C00210000", ProductClass: broker.ProductClassOption, Side: "SELL", Ratio: 1, Quantity: &quantity},
	}
	preview, err := service.PreviewExecutionCombo(t.Context(), ExecutionComboRequest{
		BrokerID: "combo-broker", Market: "US", ClientOrderID: "combo-client-1",
		OrderKind: broker.OrderKindOptionCombo, ProductClass: broker.ProductClassOption, Legs: optionLegs,
		UnderlyingID: "US.AAPL", OptionStrategy: "vertical", NearExpiry: "2026-07-17",
		Spread: new(10.0),
	})
	if err != nil || !preview.Allowed || selected.comboPreviewCalls != 1 {
		t.Fatalf("option combo preview=%#v calls=%d err=%v", preview, selected.comboPreviewCalls, err)
	}

	mixed := append([]broker.OrderLegIntent(nil), optionLegs...)
	mixed[1].ProductClass = broker.ProductClassEventContract
	if _, err := service.PreviewExecutionCombo(t.Context(), ExecutionComboRequest{
		BrokerID: "combo-broker", Market: "US", ClientOrderID: "combo-client-mixed",
		OrderKind: broker.OrderKindOptionCombo, ProductClass: broker.ProductClassOption, Legs: mixed,
		UnderlyingID: "US.AAPL", OptionStrategy: "vertical", NearExpiry: "2026-07-17",
		Spread: new(10.0),
	}); !IsRequestError(err) || !strings.Contains(err.Error(), "cannot mix") {
		t.Fatalf("mixed combo error = %v", err)
	}

	expired := time.Now().Add(-time.Second)
	amount := 20.0
	eventLegs := []broker.OrderLegIntent{
		{InstrumentID: "US.EC.ONE", ProductClass: broker.ProductClassEventContract, Side: "BUY", Ratio: 1, PredictionSide: "YES"},
		{InstrumentID: "US.EC.TWO", ProductClass: broker.ProductClassEventContract, Side: "BUY", Ratio: 1, PredictionSide: "NO"},
	}
	if _, err := service.PreviewExecutionCombo(t.Context(), ExecutionComboRequest{
		BrokerID: "combo-broker", Market: "US", ClientOrderID: "parlay-client-1",
		OrderKind: broker.OrderKindEventParlay, ProductClass: broker.ProductClassEventContract,
		RFQID: "rfq-expired", QuoteExpiresAt: &expired, Amount: &amount, Legs: eventLegs,
	}); !IsRequestError(err) || !strings.Contains(err.Error(), "request a new RFQ") {
		t.Fatalf("expired Parlay error = %v", err)
	}
}

func TestPreviewHashesBindStableClientOrderID(t *testing.T) {
	price := 12.0
	service := newExecutionTestService()
	first, err := service.normalizeExecutionOrder(ExecutionPlaceRequest{
		Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 1, Price: &price,
		ClientOrderID: "client-a",
	})
	if err != nil {
		t.Fatalf("normalize first order: %v", err)
	}
	second, err := service.normalizeExecutionOrder(ExecutionPlaceRequest{
		Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 1, Price: &price,
		ClientOrderID: "client-b",
	})
	if err != nil {
		t.Fatalf("normalize second order: %v", err)
	}
	if executionCommandHash(first) == executionCommandHash(second) {
		t.Fatal("single-order preview hash does not bind clientOrderId")
	}

	intent := broker.ComboOrderIntent{ClientOrderID: "client-a", OrderKind: broker.OrderKindOptionCombo}
	other := intent
	other.ClientOrderID = "client-b"
	if comboIntentHash(intent) == comboIntentHash(other) {
		t.Fatal("combo preview hash does not bind clientOrderId")
	}
}

type advancedTradingBroker struct {
	id                string
	accounts          []broker.Account
	productRuleCalls  int
	comboPreviewCalls int
}

func (b *advancedTradingBroker) ID() string                    { return b.id }
func (b *advancedTradingBroker) Descriptor() broker.Descriptor { return broker.Descriptor{ID: b.id} }
func (b *advancedTradingBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return append([]broker.Account(nil), b.accounts...), nil
}
func (b *advancedTradingBroker) Trading() broker.TradingService      { return nil }
func (b *advancedTradingBroker) MarketData() broker.MarketDataReader { return nil }
func (b *advancedTradingBroker) ValidateProductOrder(
	context.Context,
	broker.ProductRuleQuery,
) (*broker.ProductRuleResult, error) {
	b.productRuleCalls++
	return &broker.ProductRuleResult{Allowed: true}, nil
}
func (b *advancedTradingBroker) PreviewComboOrder(
	context.Context,
	broker.ComboOrderIntent,
) (*broker.ProductRuleResult, error) {
	b.comboPreviewCalls++
	return &broker.ProductRuleResult{Allowed: true}, nil
}
func (b *advancedTradingBroker) PlaceComboOrder(
	context.Context,
	broker.ComboOrderIntent,
) (*broker.ComboOrderResult, error) {
	return &broker.ComboOrderResult{BrokerOrderID: "combo-1", Status: "SUBMITTED"}, nil
}
func (b *advancedTradingBroker) CancelComboOrder(context.Context, broker.ReadQuery, string) error {
	return nil
}
