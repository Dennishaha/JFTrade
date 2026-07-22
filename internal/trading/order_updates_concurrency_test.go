package trading

import (
	"context"
	"sync"
	"testing"
)

func TestOrderUpdatesWorkerBrokerIDConcurrentInitializationAndPush(t *testing.T) {
	source := &fakeOrderUpdateSource{accounts: []Account{{
		ID: "1001", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"},
	}}}
	worker := NewOrderUpdatesWorker(source, &fakeExecutionOrderUpdates{}, OrderUpdatesConfig{})
	start := make(chan struct{})
	var wg sync.WaitGroup

	wg.Go(func() {
		<-start
		worker.Sync(context.Background(), true, true)
	})
	for range 8 {
		wg.Go(func() {
			<-start
			for range 100 {
				worker.HandleOrderUpdate(Order{
					AccountID: "1001", TradingEnvironment: "SIMULATE", Market: "HK",
					BrokerOrderID: "order-1", Status: "SUBMITTED",
				})
				worker.HandleFillUpdate(Fill{
					AccountID: "1001", TradingEnvironment: "SIMULATE", Market: "HK", BrokerFillID: "fill-1",
				})
			}
		})
	}
	close(start)
	wg.Wait()

	if got := worker.configuredBrokerID(); got != "futu" {
		t.Fatalf("configured broker ID = %q, want futu", got)
	}
}
