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
}
