package tradingapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

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

type lifecycleBrokerRegistry struct {
	brokers []broker.Broker
}

func (r *lifecycleBrokerRegistry) all() []broker.Broker {
	return r.brokers
}

func (r *lifecycleBrokerRegistry) lookup(id string) broker.Broker {
	for _, selected := range r.brokers {
		if selected.ID() == id {
			return selected
		}
	}
	return nil
}

func newLifecycleOrderUpdateSource(registry *lifecycleBrokerRegistry) *OrderUpdateSource {
	return NewOrderUpdateSource(OrderUpdateSourceOptions{
		Brokers:        registry.all,
		ActivateBroker: func() {},
		ResolveBroker:  registry.lookup,
	})
}

func TestProductLifecycleOrderUpdateSourceAggregatesBrokersAndFees(t *testing.T) {
	reader := &lifecycleMarketDataReader{
		orders: []broker.OrderSnapshot{{
			BrokerOrderID: "order-1", AccountID: "account-1", Market: "US",
		}},
		history: []broker.OrderSnapshot{{
			BrokerOrderID: "history-1", AccountID: "account-1", Market: "US",
		}},
		fees: []broker.OrderFeeSnapshot{{BrokerOrderIDEx: "order-ex-1"}},
	}
	registry := &lifecycleBrokerRegistry{brokers: []broker.Broker{
		&lifecycleBroker{
			id: "partial", reader: reader,
			accounts: []broker.Account{{ID: "account-1", TradingEnvironment: "SIMULATE"}},
		},
		&lifecycleBroker{id: "failed", discoverErr: errors.New("accounts failed")},
	}}
	source := newLifecycleOrderUpdateSource(registry)

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

	onlyFailures := &lifecycleBrokerRegistry{brokers: []broker.Broker{
		&lifecycleBroker{id: "failed", discoverErr: errors.New("only failure")},
	}}
	if _, err := newLifecycleOrderUpdateSource(onlyFailures).DiscoverAccounts(
		t.Context(),
	); err == nil || !strings.Contains(err.Error(), "only failure") {
		t.Fatalf("all-broker account failure = %v", err)
	}
}

func TestProductLifecycleOrderUpdateSourceSkipsFundOnlyAccounts(t *testing.T) {
	registry := &lifecycleBrokerRegistry{brokers: []broker.Broker{
		&lifecycleBroker{
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
		},
	}}

	accounts, err := newLifecycleOrderUpdateSource(registry).DiscoverAccounts(t.Context())
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

	fundOnlyRegistry := &lifecycleBrokerRegistry{brokers: []broker.Broker{
		&lifecycleBroker{
			id: "futu",
			accounts: []broker.Account{{
				ID: "fund-only", MarketAuthorities: []string{"US"},
				OrderMarketAuthorities: []string{},
			}},
		},
	}}
	if _, err := newLifecycleOrderUpdateSource(fundOnlyRegistry).DiscoverAccounts(
		t.Context(),
	); !errors.Is(err, trdsrv.ErrOrderUpdateSourceInactive) {
		t.Fatalf("fund-only discovery error = %v, want inactive without fallback queries", err)
	}
}
