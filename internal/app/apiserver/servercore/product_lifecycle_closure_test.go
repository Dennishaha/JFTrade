package servercore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tradingstore "github.com/jftrade/jftrade-main/internal/store/trading"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestProductLifecycleOrderUpdateSourceAggregatesBrokersAndFees(t *testing.T) {
	server := newTradingCancellationTestServer(t)
	reader := &lifecycleMarketDataReader{
		orders: []broker.OrderSnapshot{{
			BrokerOrderID: "order-1", AccountID: "account-1", Market: "US",
		}},
		history: []broker.OrderSnapshot{{
			BrokerOrderID: "history-1", AccountID: "account-1", Market: "US",
		}},
		fees: []broker.OrderFeeSnapshot{{BrokerOrderIDEx: "order-ex-1"}},
	}
	server.runtimes.Brokers().Replace(&lifecycleBroker{
		id: "partial", reader: reader,
		accounts: []broker.Account{{ID: "account-1", TradingEnvironment: "SIMULATE"}},
	})
	server.runtimes.Brokers().Replace(&lifecycleBroker{id: "failed", discoverErr: errors.New("accounts failed")})
	source := &tradingOrderUpdateSource{server: server}

	accounts, err := source.DiscoverAccounts(t.Context())
	if err != nil || len(accounts) != 1 || accounts[0].BrokerID != "partial" {
		t.Fatalf("aggregated accounts = %#v, %v", accounts, err)
	}
	query := trdsrv.OrderQuery{
		BrokerID: "partial", AccountID: "account-1",
		TradingEnvironment: "SIMULATE", Market: "US",
	}
	orders, err := source.CurrentOrders(t.Context(), query)
	if err != nil || len(orders) != 1 || orders[0].BrokerOrderID != "order-1" {
		t.Fatalf("current orders = %#v, %v", orders, err)
	}
	history, err := source.HistoryOrders(
		t.Context(),
		query,
		time.Now().Add(-time.Hour),
		time.Now(),
	)
	if err != nil || len(history) != 1 || history[0].BrokerOrderID != "history-1" {
		t.Fatalf("history orders = %#v, %v", history, err)
	}
	fees, err := source.OrderFees(t.Context(), query, []string{"order-ex-1"})
	if err != nil || len(fees) != 1 ||
		len(reader.feeQuery.OrderIDExList) != 1 ||
		reader.feeQuery.BrokerID != "partial" {
		t.Fatalf("order fees = %#v, query=%#v, err=%v", fees, reader.feeQuery, err)
	}

	reader.err = errors.New("broker read failed")
	if _, err := source.CurrentOrders(t.Context(), query); !errors.Is(err, reader.err) {
		t.Fatalf("current order failure = %v", err)
	}
	if _, err := source.HistoryOrders(
		t.Context(),
		query,
		time.Now().Add(-time.Hour),
		time.Now(),
	); !errors.Is(err, reader.err) {
		t.Fatalf("history order failure = %v", err)
	}
	if _, err := source.OrderFees(
		t.Context(),
		query,
		[]string{"order-ex-1"},
	); !errors.Is(err, reader.err) {
		t.Fatalf("fee failure = %v", err)
	}
	if fees, err := source.OrderFees(
		t.Context(),
		trdsrv.OrderQuery{BrokerID: "missing"},
		nil,
	); err != nil || fees != nil {
		t.Fatalf("missing broker fees = %#v, %v", fees, err)
	}

	onlyFailures := newTradingCancellationTestServer(t)
	onlyFailures.runtimes.Brokers().Replace(&lifecycleBroker{
		id: "failed", discoverErr: errors.New("only failure"),
	})
	if _, err := (&tradingOrderUpdateSource{server: onlyFailures}).DiscoverAccounts(
		t.Context(),
	); err == nil || !strings.Contains(err.Error(), "only failure") {
		t.Fatalf("all-broker account failure = %v", err)
	}
}

func TestProductLifecycleOrderUpdateSourceSkipsFundOnlyAccounts(t *testing.T) {
	server := newTradingCancellationTestServer(t)
	server.runtimes.Brokers().Replace(&lifecycleBroker{
		id: "futu",
		accounts: []broker.Account{
			{ID: "generic", MarketAuthorities: []string{"HK"}},
			{
				ID: "mixed", MarketAuthorities: []string{"US", "HK"},
				OrderMarketAuthorities: []string{"US"},
			},
			{
				ID: "fund-only", MarketAuthorities: []string{"US"},
				OrderMarketAuthorities: []string{},
			},
		},
	})

	accounts, err := (&tradingOrderUpdateSource{server: server}).DiscoverAccounts(t.Context())
	if err != nil {
		t.Fatalf("DiscoverAccounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("order accounts = %#v, want generic and mixed only", accounts)
	}
	if accounts[0].ID != "generic" || len(accounts[0].MarketAuthorities) != 1 || accounts[0].MarketAuthorities[0] != "HK" {
		t.Fatalf("generic account = %#v", accounts[0])
	}
	if accounts[1].ID != "mixed" || len(accounts[1].MarketAuthorities) != 1 || accounts[1].MarketAuthorities[0] != "US" {
		t.Fatalf("mixed account = %#v", accounts[1])
	}

	fundOnlyServer := newTradingCancellationTestServer(t)
	fundOnlyServer.runtimes.Brokers().Replace(&lifecycleBroker{
		id: "futu",
		accounts: []broker.Account{{
			ID: "fund-only", MarketAuthorities: []string{"US"},
			OrderMarketAuthorities: []string{},
		}},
	})
	if _, err := (&tradingOrderUpdateSource{server: fundOnlyServer}).DiscoverAccounts(
		t.Context(),
	); !errors.Is(err, trdsrv.ErrOrderUpdateSourceInactive) {
		t.Fatalf("fund-only discovery error = %v, want inactive without fallback queries", err)
	}
}

func TestProductLifecycleFeeUpdatesPersistOnlyOnLiveOrderLedger(t *testing.T) {
	var nilUpdates *tradingExecutionOrderUpdates
	nilUpdates.ApplyFees(t.Context(), "partial", []broker.OrderFeeSnapshot{{
		BrokerOrderIDEx: "order-ex-1",
	}})
	(&tradingExecutionOrderUpdates{}).ApplyFees(
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
	updates := &tradingExecutionOrderUpdates{server: server}
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
	if _, err := server.placeExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{
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
	if _, err := server.placeExecutionOrder(t.Context(), command); err == nil ||
		!strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing execution broker error = %v", err)
	}
	replayed, err := server.placeExecutionOrder(t.Context(), command)
	if err != nil || replayed.Status != trdsrv.OrderStatusSubmissionUnknown {
		t.Fatalf("unknown submission replay = %#v, %v", replayed, err)
	}
	if missing := server.stores.ExecutionOrders.MarkSubmissionUnknown("missing-order", errors.New("late")); missing.InternalOrderID != "" {
		t.Fatalf("missing submission update = %#v", missing)
	}

	source := &tradingOrderUpdateSource{server: server}
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

type lifecycleBroker struct {
	id          string
	reader      broker.MarketDataReader
	accounts    []broker.Account
	discoverErr error
}

func (b *lifecycleBroker) ID() string { return b.id }
func (b *lifecycleBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: b.id}
}
func (b *lifecycleBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return b.accounts, b.discoverErr
}
func (b *lifecycleBroker) Trading() broker.TradingService      { return nil }
func (b *lifecycleBroker) MarketData() broker.MarketDataReader { return b.reader }

type lifecycleMarketDataReader struct {
	broker.MarketDataReader
	orders   []broker.OrderSnapshot
	history  []broker.OrderSnapshot
	fees     []broker.OrderFeeSnapshot
	feeQuery broker.OrderFeeQuery
	err      error
}

func (r *lifecycleMarketDataReader) QueryOrders(
	context.Context,
	broker.ReadQuery,
	string,
) ([]broker.OrderSnapshot, error) {
	return r.orders, r.err
}

func (r *lifecycleMarketDataReader) QueryHistoryOrders(
	context.Context,
	broker.OrderHistoryQuery,
) ([]broker.OrderSnapshot, error) {
	return r.history, r.err
}

func (r *lifecycleMarketDataReader) QueryOrderFees(
	_ context.Context,
	query broker.OrderFeeQuery,
) ([]broker.OrderFeeSnapshot, error) {
	r.feeQuery = query
	return r.fees, r.err
}
