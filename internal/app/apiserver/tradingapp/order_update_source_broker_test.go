package tradingapp

import (
	"context"
	"errors"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestOrderUpdateSourceActivatesThenDiscoversAccounts(t *testing.T) {
	activated := false
	var brokers []broker.Broker
	source := NewOrderUpdateSource(OrderUpdateSourceOptions{
		Brokers: func() []broker.Broker { return brokers },
		ActivateBroker: func() {
			activated = true
			brokers = []broker.Broker{&lifecycleBroker{
				id: "futu",
				accounts: []broker.Account{{
					ID: "acct-1", BrokerID: "", MarketAuthorities: []string{"HK"},
				}},
			}}
		},
	})
	accounts, err := source.DiscoverAccounts(context.Background())
	if err != nil || !activated || len(accounts) != 1 || accounts[0].BrokerID != "futu" {
		t.Fatalf("activated discovery = %#v, %v", accounts, err)
	}
}

func TestOrderUpdateSourceSubscribeFiltersFutuAccounts(t *testing.T) {
	subscribed := false
	source := NewOrderUpdateSource(OrderUpdateSourceOptions{
		SubscribeOrders: func(
			_ context.Context,
			accounts []trdsrv.Account,
			_ trdsrv.OrderUpdateHandler,
		) (trdsrv.OrderUpdateSubscription, error) {
			subscribed = true
			if len(accounts) != 1 || accounts[0].BrokerID != "futu" {
				t.Fatalf("subscribe accounts = %#v", accounts)
			}
			return &lifecycleSubscription{id: "sub"}, nil
		},
	})
	subscription, err := source.Subscribe(context.Background(), []trdsrv.Account{
		{BrokerID: "futu", ID: "futu-1"},
		{BrokerID: "bbgo", ID: "bbgo-1"},
	}, nil, nil)
	if err != nil || !subscribed || subscription == nil {
		t.Fatalf("subscribe = %#v, %v", subscription, err)
	}
	if got, ok := subscription.(*lifecycleSubscription); !ok || got.id != "sub" {
		t.Fatalf("subscription = %#v", subscription)
	}

	noFutu, err := source.Subscribe(context.Background(), []trdsrv.Account{{BrokerID: "bbgo"}}, nil, nil)
	if err != nil || noFutu == nil {
		t.Fatalf("non-futu subscribe = %#v, %v", noFutu, err)
	}
	if err := noFutu.Stop(); err != nil {
		t.Fatalf("no-op stop: %v", err)
	}

	failing := NewOrderUpdateSource(OrderUpdateSourceOptions{
		SubscribeOrders: func(context.Context, []trdsrv.Account, trdsrv.OrderUpdateHandler) (trdsrv.OrderUpdateSubscription, error) {
			return nil, errors.New("subscribe failed")
		},
	})
	if _, err := failing.Subscribe(context.Background(), []trdsrv.Account{{BrokerID: "futu"}}, nil, nil); err == nil {
		t.Fatal("subscribe error was not propagated")
	}

	nilResult := NewOrderUpdateSource(OrderUpdateSourceOptions{
		SubscribeOrders: func(context.Context, []trdsrv.Account, trdsrv.OrderUpdateHandler) (trdsrv.OrderUpdateSubscription, error) {
			return nil, nil
		},
	})
	got, err := nilResult.Subscribe(context.Background(), []trdsrv.Account{{BrokerID: "futu"}}, nil, nil)
	if err != nil || got == nil {
		t.Fatalf("nil subscribe result = %#v, %v", got, err)
	}
}

type lifecycleSubscription struct {
	id string
}

func (s *lifecycleSubscription) Stop() error { return nil }
