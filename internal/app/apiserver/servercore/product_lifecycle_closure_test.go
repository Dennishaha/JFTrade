package servercore

import (
	"errors"
	"strings"
	"testing"
	"time"

	tradingstore "github.com/jftrade/jftrade-main/internal/store/trading"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestProductLifecycleFeeUpdatesPersistOnlyOnLiveOrderLedger(t *testing.T) {
	newTradingExecutionOrderUpdates(nil).ApplyFees(
		t.Context(),
		"partial",
		[]broker.OrderFeeSnapshot{{BrokerOrderIDEx: "order-ex-1"}},
	)

	server := newTradingCancellationTestServer(t)
	order := server.stores.ExecutionOrders.RecordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord{
		BrokerID: "partial", BrokerOrderID: "order-1", BrokerOrderIDEx: "order-ex-1",
		AccountID: "account-1", TradingEnvironment: "SIMULATE", Market: "US",
		Status: "SUBMITTED", EventType: "COMMAND_PLACE_ACCEPTED",
	})
	fee := 1.25
	updates := newTradingExecutionOrderUpdates(server)
	updates.ApplyFees(t.Context(), "partial", []broker.OrderFeeSnapshot{{
		BrokerOrderIDEx: "order-ex-1", AccountID: "account-1",
		TradingEnvironment: "SIMULATE", Market: "US", FeeAmount: &fee,
	}})
	updated, ok := server.stores.ExecutionOrders.Order(order.InternalOrderID)
	if !ok || updated.Fees == nil || *updated.Fees != fee {
		t.Fatalf("persisted parent fee = %#v", updated)
	}
}

func TestPredictionAndPreviewPersistenceRejectsStaleOrChangedBindings(t *testing.T) {
	store, err := newExecutionOrderStoreWithDB(t.TempDir() + "/closure.db")
	if err != nil {
		t.Fatalf("newExecutionOrderStoreWithDB: %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()
	now := time.Now().UTC()

	preview := trdsrv.ExecutionPreviewRecord{
		PreviewID: "preview-valid", RequestHash: "hash", BrokerID: "partial",
		CapabilityVersion: broker.BuiltinCapabilityCatalog.Version, AccountID: "account-1",
		ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano), CreatedAt: now.Format(time.RFC3339Nano),
	}
	if err := store.SavePreview(preview); err != nil {
		t.Fatalf("save preview: %v", err)
	}
	if err := store.ConsumePreview(
		preview.PreviewID,
		"partial",
		"account-1",
		"hash",
		"client-1",
	); err != nil {
		t.Fatalf("consume preview: %v", err)
	}
	if err := store.ConsumePreview("", "", "", "", " "); err == nil {
		t.Fatal("blank clientOrderId succeeded")
	}
	if err := store.ConsumePreview(
		"missing",
		"partial",
		"account-1",
		"hash",
		"client",
	); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing preview error = %v", err)
	}

	wrongBroker := preview
	wrongBroker.PreviewID = "preview-wrong-broker"
	if err := store.SavePreview(wrongBroker); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumePreview(
		wrongBroker.PreviewID,
		"other",
		"account-1",
		"hash",
		"client",
	); err == nil || !strings.Contains(err.Error(), "broker or account") {
		t.Fatalf("changed preview binding error = %v", err)
	}
	wrongVersion := preview
	wrongVersion.PreviewID = "preview-wrong-version"
	wrongVersion.CapabilityVersion = "old"
	if err := store.SavePreview(wrongVersion); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumePreview(
		wrongVersion.PreviewID,
		"partial",
		"account-1",
		"hash",
		"client",
	); err == nil || !strings.Contains(err.Error(), "capability version") {
		t.Fatalf("changed capability error = %v", err)
	}
	badExpiry := preview
	badExpiry.PreviewID = "preview-bad-expiry"
	badExpiry.ExpiresAt = "invalid"
	if err := store.SavePreview(badExpiry); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumePreview(
		badExpiry.PreviewID,
		"partial",
		"account-1",
		"hash",
		"client",
	); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("invalid preview expiry error = %v", err)
	}
	badQuoteExpiry := preview
	badQuoteExpiry.PreviewID = "preview-bad-quote-expiry"
	badQuoteExpiry.QuoteExpiresAt = "invalid"
	if err := store.SavePreview(badQuoteExpiry); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumePreview(
		badQuoteExpiry.PreviewID,
		"partial",
		"account-1",
		"hash",
		"client",
	); err == nil || !strings.Contains(err.Error(), "quote expired") {
		t.Fatalf("invalid quote expiry error = %v", err)
	}

	var nilStore *tradingstore.Store
	if err := nilStore.SavePredictionQuote(
		t.Context(),
		broker.PredictionQuoteRecord{},
	); err == nil {
		t.Fatal("nil quote persistence save succeeded")
	}
	if _, err := nilStore.ValidatePredictionQuote(
		t.Context(),
		"",
		"",
		"",
		"",
		"",
		"",
	); err == nil {
		t.Fatal("nil quote persistence validation succeeded")
	}
	if err := nilStore.ConsumePredictionQuote(
		t.Context(),
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
	); err == nil {
		t.Fatal("nil quote persistence consume succeeded")
	}
	if _, err := store.ValidatePredictionQuote(t.Context(),
		"missing",
		"partial",
		"account-1",
		"SIMULATE",
		"mvc",
		"hash",
	); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing RFQ error = %v", err)
	}
	expired := broker.PredictionQuoteRecord{
		QuoteID: "expired", BrokerID: "partial", AccountID: "account-1",
		TradingEnvironment: "SIMULATE", MVC: "mvc", LegsHash: "hash",
		ReceivedAt: now.Add(-time.Minute), ExpiresAt: now.Add(-time.Second),
		ExpirySource: "jftrade_policy", Status: "active",
	}
	if err := store.SavePredictionQuote(t.Context(), expired); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ValidatePredictionQuote(t.Context(),
		"expired",
		"partial",
		"account-1",
		"SIMULATE",
		"mvc",
		"hash",
	); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired RFQ error = %v", err)
	}
}

func TestProductLifecycleSnapshotIdentityAndRuntimeHelpers(t *testing.T) {
	boolValue := true
	if optionalBool(&boolValue) != true || optionalBool(nil) != nil {
		t.Fatal("optional bool normalization failed")
	}
}

func TestProductLifecycleExecutionGatewayGuardsAndSubscriptionFallback(t *testing.T) {
	server := newTradingCancellationTestServer(t)
	if _, err := placeExecutionOrder(&server.serverApplication, t.Context(), trdsrv.ExecutionOrderCommand{
		BrokerID: "first",
		Query: broker.PlaceOrderQuery{
			ReadQuery: broker.ReadQuery{BrokerID: "second"},
		},
	}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched execution broker error = %v", err)
	}
	command := trdsrv.ExecutionOrderCommand{
		BrokerID: "missing",
		Query: broker.PlaceOrderQuery{
			ReadQuery:     broker.ReadQuery{BrokerID: "missing", Market: "US"},
			Symbol:        "US.AAPL",
			Side:          "BUY",
			OrderType:     "LIMIT",
			Quantity:      1,
			ClientOrderID: "missing-broker-client",
		},
		Symbol: "US.AAPL", Side: "BUY", OrderType: "LIMIT",
	}
	if _, err := placeExecutionOrder(&server.serverApplication, t.Context(), command); err == nil ||
		!strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing execution broker error = %v", err)
	}
	replayed, err := placeExecutionOrder(&server.serverApplication, t.Context(), command)
	if err != nil || replayed.Status != trdsrv.OrderStatusSubmissionUnknown {
		t.Fatalf("unknown submission replay = %#v, %v", replayed, err)
	}
	if missing := server.stores.ExecutionOrders.MarkSubmissionUnknown("missing-order", errors.New("late")); missing.InternalOrderID != "" {
		t.Fatalf("missing submission update = %#v", missing)
	}

	source := newTradingOrderUpdateSource(server)
	subscription, err := source.Subscribe(
		t.Context(),
		[]trdsrv.Account{{BrokerID: "futu"}},
		nil,
		nil,
	)
	if err != nil || subscription == nil {
		t.Fatalf("missing Futu exchange subscription = %#v, %v", subscription, err)
	}
	if err := subscription.Stop(); err != nil {
		t.Fatalf("no-op subscription stop: %v", err)
	}
}

func TestProductLifecycleStartupBoundaries(t *testing.T) {
	t.Setenv("JFTRADE_API_DISABLED", "1")
	if shouldStartForArgs([]string{"api"}) {
		t.Fatal("disabled API startup was accepted")
	}
	t.Setenv("JFTRADE_API_DISABLED", "")
	if shouldStartForArgs([]string{"--help", "api"}) {
		t.Fatal("help startup was accepted")
	}
}
