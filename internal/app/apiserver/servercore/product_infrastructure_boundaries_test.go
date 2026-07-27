package servercore

import (
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	tradingstore "github.com/jftrade/jftrade-main/internal/store/trading"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestServerTradingStoreAndGatewaySmallBoundaries(t *testing.T) {
	store := (*tradingstore.Store)(nil)
	if err := store.SavePreview(trdsrv.ExecutionPreviewRecord{}); err != nil {
		t.Fatalf("nil preview save: %v", err)
	}
	if err := store.ConsumePreview("", "", "", "", ""); err != nil {
		t.Fatalf("nil preview consume: %v", err)
	}
	emptyStore := newExecutionOrderStore()
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
	if got := futuintegration.ExtendedMarketQuoteSecurityMap(nil); got != nil {
		t.Fatalf("nil extended quote map = %#v", got)
	}

	server := &Server{}
	if server.IsWriteMethod(nil) {
		t.Fatal("nil request classified as write")
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
