package servercore

import (
	"context"
	"strings"
	"testing"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestExecutionSubmissionLedgerDeduplicatesAndNeverRetriesUnknown(t *testing.T) {
	store := newExecutionOrderStore()
	input := trdsrv.ExecutionPlacedOrderRecord{
		BrokerID: "partial-broker", TradingEnvironment: "REAL", AccountID: "acct-1",
		Market: "US", Symbol: "US.AAPL", Side: "BUY", OrderType: "LIMIT",
		RequestedQuantity: 1, ClientOrderID: "client-stable-1",
		ProductClass: broker.ProductClassEquity, OrderKind: broker.OrderKindSingle,
		QuantityMode: broker.QuantityModeUnits, NormalizedRequest: `{"symbol":"US.AAPL"}`,
	}
	prepared, fresh, err := store.prepareSubmission(input)
	if err != nil || !fresh || prepared.Status != trdsrv.OrderStatusSubmitting {
		t.Fatalf("first prepare = %#v, fresh=%v, err=%v", prepared, fresh, err)
	}
	accepted := input
	accepted.InternalOrderID = prepared.InternalOrderID
	accepted.BrokerOrderID = "broker-1"
	accepted.Status = "SUBMITTED"
	accepted.EventType = "COMMAND_PLACE_ACCEPTED"
	store.recordPlacedOrder(accepted)

	replayed, fresh, err := store.prepareSubmission(input)
	if err != nil || fresh {
		t.Fatalf("replay prepare = %#v, fresh=%v, err=%v", replayed, fresh, err)
	}
	if replayed.InternalOrderID != prepared.InternalOrderID ||
		replayed.Status != trdsrv.OrderStatusBrokerAccepted {
		t.Fatalf("replayed order = %#v, want original accepted order", replayed)
	}
	if events := store.orderEvents(prepared.InternalOrderID).Events; len(events) != 2 {
		t.Fatalf("replay created an extra submission event: %#v", events)
	}

	unknownInput := input
	unknownInput.ClientOrderID = "client-unknown-1"
	unknown, fresh, err := store.prepareSubmission(unknownInput)
	if err != nil || !fresh {
		t.Fatalf("unknown prepare = %#v, fresh=%v, err=%v", unknown, fresh, err)
	}
	store.markSubmissionUnknown(unknown.InternalOrderID, errTestBrokerTimeout)
	replayedUnknown, fresh, err := store.prepareSubmission(unknownInput)
	if err != nil || fresh || replayedUnknown.Status != trdsrv.OrderStatusSubmissionUnknown {
		t.Fatalf("unknown replay = %#v, fresh=%v, err=%v", replayedUnknown, fresh, err)
	}
	unknownEvents := store.orderEvents(unknown.InternalOrderID).Events
	if len(unknownEvents) != 2 || unknownEvents[1].EventType != "COMMAND_SUBMISSION_UNKNOWN" ||
		!strings.Contains(unknownEvents[1].PayloadJSON, `"retryAllowed":false`) {
		t.Fatalf("unknown lifecycle events = %#v", unknownEvents)
	}
}

func TestExecutionPreviewConsumptionIsIdempotentOnlyForIdenticalClientRequest(t *testing.T) {
	persistence, err := newExecutionOrderSQLiteStore(t.TempDir() + "/execution-previews.db")
	if err != nil {
		t.Fatalf("newExecutionOrderSQLiteStore: %v", err)
	}
	defer func() { jftradeCheckTestError(t, persistence.Close()) }()

	now := time.Now().UTC()
	record := trdsrv.ExecutionPreviewRecord{
		PreviewID: "preview-safe-1", RequestHash: "hash-client-1", BrokerID: "futu",
		CapabilityVersion: broker.BuiltinCapabilityCatalog.Version, AccountID: "acct-1",
		ExpiresAt:         now.Add(time.Minute).Format(time.RFC3339Nano),
		NormalizedRequest: `{"clientOrderId":"client-1"}`, CreatedAt: now.Format(time.RFC3339Nano),
	}
	if err := persistence.savePreview(record); err != nil {
		t.Fatalf("savePreview: %v", err)
	}
	if err := persistence.consumePreview(record.PreviewID, "futu", "acct-1", "hash-client-1", "client-1"); err != nil {
		t.Fatalf("first consumePreview: %v", err)
	}
	if err := persistence.consumePreview(record.PreviewID, "futu", "acct-1", "hash-client-1", "client-1"); err != nil {
		t.Fatalf("identical replay consumePreview: %v", err)
	}
	if err := persistence.consumePreview(record.PreviewID, "futu", "acct-1", "hash-client-2", "client-2"); err == nil ||
		!strings.Contains(err.Error(), "request changed") {
		t.Fatalf("different client replay error = %v", err)
	}

	expired := record
	expired.PreviewID = "preview-expired"
	expired.RequestHash = "expired-hash"
	expired.ExpiresAt = now.Add(-time.Second).Format(time.RFC3339Nano)
	if err := persistence.savePreview(expired); err != nil {
		t.Fatalf("save expired preview: %v", err)
	}
	if err := persistence.consumePreview(expired.PreviewID, "futu", "acct-1", "expired-hash", "client-expired"); err == nil ||
		!strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired preview error = %v", err)
	}

	quoteExpired := record
	quoteExpired.PreviewID = "preview-quote-expired"
	quoteExpired.RequestHash = "quote-expired-hash"
	quoteExpired.QuoteExpiresAt = now.Add(-time.Second).Format(time.RFC3339Nano)
	if err := persistence.savePreview(quoteExpired); err != nil {
		t.Fatalf("save quote-expired preview: %v", err)
	}
	if err := persistence.consumePreview(
		quoteExpired.PreviewID, "futu", "acct-1", "quote-expired-hash", "client-quote-expired",
	); err == nil || !strings.Contains(err.Error(), "broker quote expired") {
		t.Fatalf("quote-expired preview error = %v", err)
	}
}

func TestPredictionRFQPersistsBindingExpiryAndSingleConsumption(t *testing.T) {
	persistence, err := newExecutionOrderSQLiteStore(t.TempDir() + "/prediction-rfq.db")
	if err != nil {
		t.Fatalf("newExecutionOrderSQLiteStore: %v", err)
	}
	defer func() { jftradeCheckTestError(t, persistence.Close()) }()

	now := time.Now().UTC()
	legs := []broker.OrderLegIntent{
		{InstrumentID: "US.EC.ONE", ProductClass: broker.ProductClassEventContract, Side: "BUY", Ratio: 1, PredictionSide: "YES"},
		{InstrumentID: "US.EC.TWO", ProductClass: broker.ProductClassEventContract, Side: "BUY", Ratio: 1, PredictionSide: "NO"},
	}
	hash := broker.PredictionQuoteLegsHash("mvc-1", legs)
	store := &serverTradingOrderStore{store: &executionOrderStore{persistence: persistence}}
	record := broker.PredictionQuoteRecord{
		QuoteID: "rfq-1", BrokerID: "futu", AccountID: "acct-1",
		TradingEnvironment: "REAL", MVC: "mvc-1", LegsHash: hash,
		ReceivedAt: now, ExpiresAt: now.Add(30 * time.Second),
		ExpirySource: "jftrade_policy", Status: "active",
	}
	if err := store.SavePredictionQuote(context.Background(), record); err != nil {
		t.Fatalf("SavePredictionQuote: %v", err)
	}
	validated, err := store.ValidatePredictionQuote(
		context.Background(), "rfq-1", "futu", "acct-1", "REAL", "mvc-1", hash,
	)
	if err != nil || validated.ExpirySource != "jftrade_policy" {
		t.Fatalf("ValidatePredictionQuote = %#v, %v", validated, err)
	}
	if _, err := store.ValidatePredictionQuote(
		context.Background(), "rfq-1", "futu", "acct-2", "REAL", "mvc-1", hash,
	); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("cross-account validation error = %v", err)
	}
	if err := store.ConsumePredictionQuote(
		context.Background(), "rfq-1", "futu", "acct-1", "REAL", "mvc-1", hash,
		"preview-1", "client-1",
	); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if err := store.ConsumePredictionQuote(
		context.Background(), "rfq-1", "futu", "acct-1", "REAL", "mvc-1", hash,
		"preview-1", "client-1",
	); err != nil {
		t.Fatalf("idempotent consume: %v", err)
	}
	if err := store.ConsumePredictionQuote(
		context.Background(), "rfq-1", "futu", "acct-1", "REAL", "mvc-1", hash,
		"preview-2", "client-2",
	); err == nil || !strings.Contains(err.Error(), "consumed") {
		t.Fatalf("reused quote error = %v", err)
	}
}

var errTestBrokerTimeout = &broker.BrokerError{
	Code: broker.ErrCodeTimeout, Message: "submission timed out",
}
