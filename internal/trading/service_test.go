package trading

import (
	"testing"
)

func TestServiceReadQueryAppliesDefaultMarket(t *testing.T) {
	service := NewService(WithDefaultMarket(func() string { return "US" }))
	query := service.ReadQuery("futu", "REAL", "account-1", "")
	if query.BrokerID != "futu" || query.TradingEnvironment != "REAL" || query.AccountID != "account-1" {
		t.Fatalf("ReadQuery = %+v", query)
	}
	if query.Market != "US" {
		t.Fatalf("ReadQuery market = %q, want US", query.Market)
	}

	defaultQuery := NewService().ReadQuery("futu", "SIMULATE", "account-2", "")
	if defaultQuery.Market != "HK" {
		t.Fatalf("default ReadQuery market = %q, want HK", defaultQuery.Market)
	}
}

func TestServiceOrderUpdateDefaultsAreNoops(t *testing.T) {
	service := NewService(WithOrderUpdates(nil))

	service.SyncOrderUpdates(t.Context(), true, true)
	if snapshot := service.OrderUpdatesSnapshot(); len(snapshot) != 0 {
		t.Fatalf("OrderUpdatesSnapshot = %#v, want empty", snapshot)
	}
	if err := service.StopOrderUpdates(); err != nil {
		t.Fatalf("StopOrderUpdates: %v", err)
	}

	var nilService *Service
	nilService.SyncOrderUpdates(t.Context(), false, false)
	if snapshot := nilService.OrderUpdatesSnapshot(); len(snapshot) != 0 {
		t.Fatalf("nil OrderUpdatesSnapshot = %#v, want empty", snapshot)
	}
	if err := nilService.StopOrderUpdates(); err != nil {
		t.Fatalf("nil StopOrderUpdates: %v", err)
	}
}
