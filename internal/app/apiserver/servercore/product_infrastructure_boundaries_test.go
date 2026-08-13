package servercore

import (
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	tradingstore "github.com/jftrade/jftrade-main/internal/store/trading"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
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
	if got := defaultTradingEnvironment((*serverApplication)(nil)); got != "SIMULATE" {
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
	if summary := marketdataRuntimeSummary(&server.serverApplication); summary.Status != "unavailable" {
		t.Fatalf("nil marketdata runtime summary = %#v", summary)
	}
	if summary := strategyRuntimeSummary(&server.serverApplication); summary.Status != "idle" {
		t.Fatalf("nil strategy runtime summary = %#v", summary)
	}
	if watched := server.workflowWatchedInstruments(); watched != nil {
		t.Fatalf("nil assistant watched instruments = %#v", watched)
	}
}
