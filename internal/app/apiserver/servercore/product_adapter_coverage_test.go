package servercore

import (
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu"
)

func TestADKProductToolInputHelpersCompleteBranches(t *testing.T) {
	var decoded struct {
		Value string `json:"value"`
	}
	if err := decodeToolInput(map[string]any{"value": "ok"}, &decoded); err != nil || decoded.Value != "ok" {
		t.Fatalf("decodeToolInput success = %#v, %v", decoded, err)
	}
	if err := decodeToolInput(map[string]any{"bad": make(chan int)}, &decoded); err == nil {
		t.Fatal("decodeToolInput marshaled channel")
	}
	if err := decodeToolInput(map[string]any{"value": "ok"}, nil); err == nil {
		t.Fatal("decodeToolInput decoded into nil")
	}

	if got := toolInstrumentID(map[string]any{"instrumentId": " us.aapl "}); got != "US.AAPL" {
		t.Fatalf("explicit tool instrument = %q", got)
	}
	if got := toolInstrumentID(map[string]any{"market": "us", "symbol": "aapl"}); got != "US.AAPL" {
		t.Fatalf("market symbol instrument = %q", got)
	}
	if got := toolInstrumentID(map[string]any{"symbol": "aapl"}); got != "AAPL" {
		t.Fatalf("bare symbol instrument = %q", got)
	}
	if got := toolMapString(map[string]any{}, "missing"); got != "" {
		t.Fatalf("missing tool string = %q", got)
	}
	if got := toolMapString(map[string]any{"nil": nil}, "nil"); got != "" {
		t.Fatalf("nil tool string = %q", got)
	}
	if got := toolMapString(map[string]any{"number": 12}, "number"); got != "12" {
		t.Fatalf("numeric tool string = %q", got)
	}
	if got := toolMapInt(map[string]any{"page": "25"}, "page", 5); got != 25 {
		t.Fatalf("tool integer = %d", got)
	}
	if got := toolMapInt(map[string]any{"page": "bad"}, "page", 5); got != 5 {
		t.Fatalf("fallback tool integer = %d", got)
	}
	if got := toolMapStrings(map[string]any{
		"symbols": []any{" A ", "", 2},
	}, "symbols"); len(got) != 2 || got[0] != "A" || got[1] != "2" {
		t.Fatalf("interface tool strings = %#v", got)
	}
	direct := []string{"A", "B"}
	gotDirect := toolMapStrings(map[string]any{"symbols": direct}, "symbols")
	if len(gotDirect) != 2 || &gotDirect[0] == &direct[0] {
		t.Fatalf("direct tool strings = %#v", gotDirect)
	}
	if got := toolMapStrings(map[string]any{"symbols": 1}, "symbols"); got != nil {
		t.Fatalf("invalid tool strings = %#v", got)
	}
	cloned := cloneToolInput(map[string]any{"a": 1})
	if cloned["a"] != 1 {
		t.Fatalf("cloned tool input = %#v", cloned)
	}
}

func TestADKProductAndExecutionDispatchFailureBoundaries(t *testing.T) {
	server := &Server{}
	if _, err := server.invokeADKProductTool(t.Context(), "unknown.product.tool", nil); err == nil {
		t.Fatal("unknown product tool succeeded")
	}
	if _, err := server.invokeADKExecutionTool(t.Context(), "unknown.execution.tool", nil); err == nil {
		t.Fatal("unknown execution tool succeeded")
	}
	for _, name := range []string{
		"execution.order_preview",
		"execution.order_place",
		"execution.combo_preview",
		"execution.combo_place",
	} {
		if _, err := server.invokeADKExecutionTool(t.Context(), name, map[string]any{
			"invalid": make(chan int),
		}); err == nil {
			t.Errorf("%s accepted unmarshalable input", name)
		}
	}
	if _, err := server.adkProductSnapshots(
		t.Context(), map[string]any{}, broker.FeatureMarketSnapshots,
	); err == nil {
		t.Fatal("snapshot tool without symbols succeeded")
	}
	if _, err := server.adkProductBuyingPower(t.Context(), map[string]any{
		"invalid": make(chan int),
	}); err == nil {
		t.Fatal("buying-power tool accepted unmarshalable input")
	}
}

func TestApplyExecutionLegSnapshotsCompleteMergeAppendAndLifecycle(t *testing.T) {
	now := "2026-07-17T12:00:00Z"
	summary := &executionOrderSummaryResponse{
		InternalOrderID: "order-1",
		Legs: []trdsrv.ExecutionOrderLeg{{
			ID: "order-1-leg-001", InternalOrderID: "order-1", Index: 0,
			InstrumentID: "US.ONE", ProductClass: broker.ProductClassOption,
			Side: "BUY", Ratio: 1, Status: trdsrv.OrderStatusSubmitted,
		}},
	}
	applyExecutionLegSnapshots(nil, []broker.OrderLegSnapshot{{InstrumentID: "US.ONE"}}, now)
	applyExecutionLegSnapshots(summary, nil, now)
	applyExecutionLegSnapshots(summary, []broker.OrderLegSnapshot{
		{
			BrokerLegID: "broker-leg-1", InstrumentID: "us.one",
			ProductClass: broker.ProductClassEventContract, Side: " sell ", Ratio: 2,
			PredictionSide: " yes ", RequestedQuantity: 3, RequestedAmount: 30,
			RequestedPrice: 0.6, Status: "FILLED_PART", FilledQuantity: 1,
			FilledAmount: 10, AveragePrice: 0.55, Fees: 0.1, Payout: 5,
		},
		{
			InstrumentID: "US.TWO", ProductClass: broker.ProductClassOption,
			Side: "BUY", Ratio: 0, Status: "CUSTOM_PENDING",
		},
		{
			InstrumentID: "US.THREE", ProductClass: broker.ProductClassFuture,
			Side: "SELL", Ratio: 3, Status: "FILLED_ALL",
		},
	}, now)
	if len(summary.Legs) != 3 {
		t.Fatalf("merged legs = %#v", summary.Legs)
	}
	first := summary.Legs[0]
	if first.BrokerLegID == nil || *first.BrokerLegID != "broker-leg-1" ||
		first.ProductClass != broker.ProductClassEventContract || first.Side != "SELL" ||
		first.Ratio != 2 || first.PredictionSide != "YES" ||
		first.RequestedQuantity == nil || *first.RequestedQuantity != 3 ||
		first.RequestedAmount == nil || *first.RequestedAmount != 30 ||
		first.RequestedPrice == nil || *first.RequestedPrice != 0.6 ||
		first.FilledQuantity == nil || *first.FilledQuantity != 1 ||
		first.FilledAmount == nil || *first.FilledAmount != 10 ||
		first.AveragePrice == nil || *first.AveragePrice != 0.55 ||
		first.Fees == nil || *first.Fees != 0.1 ||
		first.Payout == nil || *first.Payout != 5 {
		t.Fatalf("first merged leg = %#v", first)
	}
	if summary.Legs[1].Ratio != 1 || summary.Legs[1].Status != trdsrv.OrderStatusUnknown {
		t.Fatalf("fallback-index leg = %#v", summary.Legs[1])
	}
	if summary.Legs[2].Ratio != 3 || summary.Legs[2].Status != trdsrv.OrderStatusFilled {
		t.Fatalf("appended leg = %#v", summary.Legs[2])
	}
}

func TestServerTradingAdapterSmallBoundaries(t *testing.T) {
	store := (*serverTradingOrderStore)(nil)
	if err := store.SavePreview(trdsrv.ExecutionPreviewRecord{}); err != nil {
		t.Fatalf("nil preview save: %v", err)
	}
	if err := store.ConsumePreview("", "", "", "", ""); err != nil {
		t.Fatalf("nil preview consume: %v", err)
	}
	emptyStore := &serverTradingOrderStore{}
	if err := emptyStore.SavePreview(trdsrv.ExecutionPreviewRecord{}); err != nil {
		t.Fatalf("empty preview save: %v", err)
	}
	if err := emptyStore.ConsumePreview("", "", "", "", ""); err != nil {
		t.Fatalf("empty preview consume: %v", err)
	}
	if comboOrderQuantityMode(broker.OrderKindEventParlay) != broker.QuantityModeAmount ||
		comboOrderQuantityMode(broker.OrderKindOptionCombo) != broker.QuantityModeContracts {
		t.Fatal("server combo quantity mode mismatch")
	}
	if got := normalizedBrokerComboIntent(broker.ComboOrderIntent{
		ClientOrderID: "client",
	}); !strings.Contains(got, "client") {
		t.Fatalf("normalized broker combo = %s", got)
	}
	if got := (*Server)(nil).defaultTradingEnvironment(); got != "SIMULATE" {
		t.Fatalf("nil default trading environment = %q", got)
	}
}

func TestProductInfrastructureRemainingNilAndFallbackBoundaries(t *testing.T) {
	if got := connectivityFromBrokerReadError(nil); got != "connected" {
		t.Fatalf("nil broker read connectivity = %q", got)
	}
	if got := executionFillLookupKey("", "", "", "", "", nil); got != "" {
		t.Fatalf("empty fill lookup key = %q", got)
	}
	fillIDEx := "fill-ex-1"
	if got := executionFillLookupKey(
		"broker", "account", "simulate", "us", "", &fillIDEx,
	); !strings.HasSuffix(got, "|fill-ex-1") {
		t.Fatalf("extended fill lookup key = %q", got)
	}
	if got := canonicalPlacedRecordStatus(trdsrv.OrderStatusSubmissionUnknown); got !=
		trdsrv.OrderStatusSubmissionUnknown {
		t.Fatalf("canonical submission-unknown status = %q", got)
	}
	if got := extendedMarketQuoteSecurityMap((*futu.ExtendedMarketQuote)(nil)); got != nil {
		t.Fatalf("nil extended quote map = %#v", got)
	}

	server := &Server{}
	if server.IsWriteMethod(nil) {
		t.Fatal("nil request classified as write")
	}
	var closeErrors []error
	server.appendCloseError(&closeErrors, "nil", nil)
	server.appendCloseError(&closeErrors, "failure", func() error {
		return errors.New("close failed")
	})
	if len(closeErrors) != 1 || !strings.Contains(closeErrors[0].Error(), "failure") {
		t.Fatalf("collected close errors = %#v", closeErrors)
	}
	server.recordExchangeCalendarAlert(exchangecalendar.SourceAlert{})
	if summary := server.marketdataRuntimeSummary(); summary["status"] != "unavailable" {
		t.Fatalf("nil marketdata runtime summary = %#v", summary)
	}
	if summary := server.strategyRuntimeSummary(); summary["status"] != "idle" {
		t.Fatalf("nil strategy runtime summary = %#v", summary)
	}
	if _, err := server.workflowMarketSnapshot(t.Context(), "US.AAPL"); err == nil {
		t.Fatal("workflow snapshot without market service succeeded")
	}
	if watched := server.workflowWatchedInstruments(); watched != nil {
		t.Fatalf("nil assistant watched instruments = %#v", watched)
	}
}
