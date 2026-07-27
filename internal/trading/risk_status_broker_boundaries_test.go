package trading

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestRiskDecisionAndHardStopBoundarySemantics(t *testing.T) {
	if !(PreTradeRiskDecision{Decision: " require_approval "}).RequiresApproval() {
		t.Fatal("REQUIRE_APPROVAL decision was not recognized")
	}
	if (PreTradeRiskDecision{Decision: "reject"}).RequiresApproval() {
		t.Fatal("REJECT decision unexpectedly requires approval")
	}
	for _, test := range []struct {
		name string
		err  RiskRejectedError
		want string
	}{
		{name: "message", err: RiskRejectedError{Decision: PreTradeRiskDecision{ReasonMessage: "reason"}}, want: "reason"},
		{name: "code", err: RiskRejectedError{Decision: PreTradeRiskDecision{ReasonCode: "CODE"}}, want: "CODE"},
		{name: "default", err: RiskRejectedError{}, want: "pre-trade risk rejected the execution order"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want || !IsRiskRejected(test.err) {
				t.Fatalf("RiskRejectedError = %q / %v", got, IsRiskRejected(test.err))
			}
		})
	}
	if IsRiskRejected(errors.New("other")) {
		t.Fatal("ordinary error was classified as risk rejection")
	}

	for _, test := range []struct {
		entry, market, symbol string
		want                  bool
	}{
		{entry: "", market: "US", symbol: "AAPL", want: false},
		{entry: "AAPL", market: "US", symbol: "", want: false},
		{entry: "aapl", market: "US", symbol: "AAPL", want: true},
		{entry: "US.AAPL", market: "US", symbol: "AAPL", want: true},
		{entry: "AAPL", market: "US", symbol: "US.AAPL", want: true},
		{entry: "MSFT", market: "US", symbol: "AAPL", want: false},
	} {
		if got := symbolMatches(test.entry, test.market, test.symbol); got != test.want {
			t.Fatalf("symbolMatches(%q, %q, %q) = %v, want %v", test.entry, test.market, test.symbol, got, test.want)
		}
	}

	market, symbol := "US", "AAPL"
	base := ExecutionOrderCommand{BrokerID: "futu", Symbol: "AAPL", Query: broker.PlaceOrderQuery{
		ReadQuery: broker.ReadQuery{TradingEnvironment: "REAL", AccountID: "acc", Market: "US"},
	}}
	matching := RealTradeHardStopEntry{BrokerID: "futu", TradingEnvironment: "REAL", AccountID: "acc", Market: &market, Symbol: &symbol}
	if !hardStopMatches(matching, base) || matchHardStop(PreTradeRiskConfig{RuntimeHardStops: []RealTradeHardStopEntry{matching}}, base) == nil {
		t.Fatal("matching hard stop was not detected")
	}
	for _, entry := range []RealTradeHardStopEntry{
		{BrokerID: "other"},
		{BrokerID: "futu", TradingEnvironment: "SIMULATE"},
		{BrokerID: "futu", TradingEnvironment: "REAL", AccountID: "other"},
		{BrokerID: "futu", TradingEnvironment: "REAL", AccountID: "*", Market: new("HK")},
		{BrokerID: "futu", TradingEnvironment: "REAL", AccountID: "*", Market: new("US"), Symbol: new("MSFT")},
	} {
		if hardStopMatches(entry, base) {
			t.Fatalf("nonmatching hard stop = %#v", entry)
		}
	}
	if matchHardStop(PreTradeRiskConfig{}, base) != nil {
		t.Fatal("empty hard-stop config matched")
	}
	if got := matchHardStop(PreTradeRiskConfig{RuntimeHardStops: []RealTradeHardStopEntry{{BrokerID: "other"}, matching}}, base); got == nil || got.ID != matching.ID {
		t.Fatalf("hard-stop matching did not skip the nonmatching entry: %#v", got)
	}
}

func TestOrderStatusMapsEveryBrokerLifecycleFamily(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want string
	}{
		{raw: "created", want: OrderStatusCreated},
		{raw: "precheck rejected", want: OrderStatusPrecheckReject},
		{raw: "waiting_submit", want: OrderStatusSubmitting},
		{raw: "submitted", want: OrderStatusBrokerAccepted},
		{raw: "new", want: OrderStatusBrokerAccepted},
		{raw: "filled_part", want: OrderStatusPartiallyFilled},
		{raw: "filled_all", want: OrderStatusFilled},
		{raw: "cancelling_all", want: OrderStatusCancelRequested},
		{raw: "canceled_part", want: OrderStatusCancelled},
		{raw: "submitfailed", want: OrderStatusRejected},
		{raw: "expired", want: OrderStatusExpired},
		{raw: "unexpected", want: OrderStatusUnknown},
	} {
		if got := CanonicalBrokerOrderStatus(test.raw); got != test.want {
			t.Fatalf("CanonicalBrokerOrderStatus(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
	if got := CanonicalStoredOrderStatus("submitted"); got != OrderStatusSubmitted {
		t.Fatalf("stored submitted = %q", got)
	}
	if got := CanonicalStoredOrderStatus("order_status_broker_accepted"); got != OrderStatusBrokerAccepted {
		t.Fatalf("stored canonical prefix = %q", got)
	}
	if got, advanced := ReconcileCanonicalOrderStatus(OrderStatusUnknown, OrderStatusSubmitted); got != OrderStatusSubmitted || !advanced {
		t.Fatalf("unknown reconciliation = %q / %v", got, advanced)
	}
	if got, advanced := ReconcileCanonicalOrderStatus(OrderStatusSubmitted, OrderStatusUnknown); got != OrderStatusSubmitted || advanced {
		t.Fatalf("unknown incoming reconciliation = %q / %v", got, advanced)
	}
	if IsCanonicalTerminalOrderStatus(OrderStatusSubmitted) {
		t.Fatal("submitted order was classified as terminal")
	}
	if got, advanced := ReconcileCanonicalOrderStatus(OrderStatusSubmitted, OrderStatusCreated); got != OrderStatusSubmitted || advanced {
		t.Fatalf("invalid regression reconciliation = %q / %v", got, advanced)
	}
}

func TestBrokerIdentityMismatchPropagatesAcrossReadAndWriteOperations(t *testing.T) {
	service := NewService(WithActiveBroker(func() broker.Broker { return &stubBroker{id: "other"} }))
	read := broker.ReadQuery{BrokerID: "futu", Market: "US"}
	cases := []struct {
		name string
		call func() error
	}{
		{"runtime", func() error { _, err := service.Runtime(t.Context(), "futu"); return err }},
		{"funds", func() error { _, err := service.Funds(t.Context(), read); return err }},
		{"positions", func() error { _, err := service.Positions(t.Context(), read); return err }},
		{"orders", func() error { _, err := service.Orders(t.Context(), OrdersQuery{ReadQuery: read}); return err }},
		{"fills", func() error { _, err := service.Fills(t.Context(), FillsQuery{ReadQuery: read}); return err }},
		{"cash flows", func() error {
			_, err := service.CashFlows(t.Context(), broker.CashFlowQuery{ReadQuery: read})
			return err
		}},
		{"fees", func() error {
			_, err := service.OrderFees(t.Context(), broker.OrderFeeQuery{ReadQuery: read})
			return err
		}},
		{"margin ratios", func() error {
			_, err := service.MarginRatios(t.Context(), broker.MarginRatioQuery{ReadQuery: read})
			return err
		}},
		{"max quantity", func() error {
			_, err := service.MaxTradeQuantity(t.Context(), broker.MaxTradeQuantityQuery{ReadQuery: read})
			return err
		}},
		{"quote", func() error { _, err := service.Quote(t.Context(), broker.QuoteQuery{ReadQuery: read}); return err }},
		{"klines", func() error { _, err := service.KLines(t.Context(), broker.KLineQuery{ReadQuery: read}); return err }},
		{"securities", func() error {
			_, err := service.Securities(t.Context(), broker.SecuritySnapshotQuery{ReadQuery: read})
			return err
		}},
		{"portfolio cash", func() error { _, err := service.PortfolioCashBalances(t.Context(), read); return err }},
		{"portfolio positions", func() error { _, err := service.PortfolioPositions(t.Context(), read); return err }},
		{"place", func() error {
			_, err := service.PlaceBrokerOrder(t.Context(), broker.PlaceOrderQuery{ReadQuery: read})
			return err
		}},
		{"cancel", func() error { _, err := service.CancelBrokerOrders(t.Context(), read, nil); return err }},
		{"unlock", func() error {
			_, err := service.UnlockTrade(t.Context(), broker.UnlockTradeRequest{ReadQuery: read})
			return err
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, ErrBrokerNotFound) {
				t.Fatalf("operation error = %v, want broker mismatch", err)
			}
		})
	}

	if runtime, err := (&Service{}).Runtime(t.Context(), ""); err != nil || runtime == nil || runtime.Session.Connectivity != "" {
		t.Fatalf("nil runtime provider fallback = %#v, %v", runtime, err)
	}
	service = &Service{orderStore: &testOrderStorePort{}, orderGateway: &testOrderGatewayPort{}, brokerRuntime: testBrokerRuntimePort{}}
	if ensureOrderStoreFunctions(service) == nil || ensureOrderGatewayFunctions(service) == nil || ensureBrokerRuntimeFunctions(service) == nil {
		t.Fatal("function adapters were not installed")
	}
}

func TestLowLevelTradingFallbackHelpers(t *testing.T) {
	if got := firstNonEmpty("", " ", "selected"); got != "selected" {
		t.Fatalf("firstNonEmpty = %q", got)
	}
	if firstNonEmpty("", " ") != "" || floatValue(nil) != 0 || firstFloatPointer(nil, nil) != nil {
		t.Fatal("empty primitive fallback did not retain its zero value")
	}
	value := 12.5
	if firstFloatPointer(nil, &value) != &value || firstFloat(nil, &value) != value {
		t.Fatal("first float helper did not choose the first populated value")
	}
	var store *orderStoreFunctions
	if _, err := store.OrderEvents(t.Context(), "order"); !errors.Is(err, ErrOrderStoreUnavailable) {
		t.Fatalf("nil order store events error = %v", err)
	}
	var gateway *orderGatewayFunctions
	if _, err := gateway.CancelOrder(t.Context(), "order"); !errors.Is(err, ErrOrderGatewayUnavailable) {
		t.Fatalf("nil order gateway cancel error = %v", err)
	}
	response := withTimeout(t.Context(), time.Second, "result", func(context.Context) (*BrokerPositionsResponse, error) {
		return nil, errors.New("upstream failed")
	}, positionsReadError)
	if response.Connectivity == "connected" {
		t.Fatalf("failed timeout wrapper response = %#v", response)
	}
	completed := make(chan struct{})
	timedOut := withTimeout(t.Context(), time.Millisecond, "result", func(ctx context.Context) (*BrokerPositionsResponse, error) {
		defer close(completed)
		<-ctx.Done()
		return nil, ctx.Err()
	}, positionsReadError)
	if timedOut.LastError == nil || !strings.Contains(*timedOut.LastError, "timed out after") {
		t.Fatalf("context-aware timeout response = %#v", timedOut)
	}
	select {
	case <-completed:
	default:
		t.Fatal("timeout helper returned before the broker query exited")
	}
}

func TestServiceBrokerResolutionAndPredictionStoreBoundaries(t *testing.T) {
	service := &Service{}
	if resolved, err := service.resolveBroker("", true); resolved != nil || !errors.Is(err, ErrNoBroker) {
		t.Fatalf("required broker resolution = (%#v, %v), want ErrNoBroker", resolved, err)
	}
	if resolved, err := service.resolveBroker("", false); resolved != nil || err != nil {
		t.Fatalf("optional broker resolution = (%#v, %v), want empty success", resolved, err)
	}

	active := &stubBroker{id: "futu"}
	service.brokerRuntime = testBrokerRuntimePort{active: active}
	if resolved, err := service.resolveBroker("", false); resolved != active || err != nil {
		t.Fatalf("active broker resolution = (%#v, %v), want futu", resolved, err)
	}
	service.brokerRuntime = testBrokerRuntimePort{}
	if resolved, err := service.resolveBroker("", false); resolved != nil || err != nil {
		t.Fatalf("optional empty runtime resolution = (%#v, %v), want empty success", resolved, err)
	}

	WithPredictionQuoteStore(nil)(service)
	if service.predictionQuotes != nil {
		t.Fatal("nil prediction quote store option changed the injected value")
	}
}
