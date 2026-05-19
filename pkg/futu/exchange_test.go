package futu

import (
	"testing"

	"github.com/c9s/bbgo/pkg/exchange"
	"github.com/c9s/bbgo/pkg/types"
)

func TestRegistration(t *testing.T) {
	if !types.ExchangeName("futu").IsValid() {
		t.Fatal("futu should be registered as a valid bbgo exchange via init()")
	}
	ex, err := exchange.New(types.ExchangeName("futu"), exchange.Options{"OPEND_ADDR": "127.0.0.1:11110"})
	if err != nil {
		t.Fatalf("exchange.New: %v", err)
	}
	if ex.Name() != Name {
		t.Fatalf("ex.Name() = %s", ex.Name())
	}
}

func TestConstructorFallsBackToDefaultAddress(t *testing.T) {
	t.Setenv("FUTU_OPEND_ADDR", "")
	ex, err := exchange.New(types.ExchangeName("futu"), exchange.Options{})
	if err != nil {
		t.Fatalf("expected default OpenD address fallback, got error: %v", err)
	}
	if ex.Name() != Name {
		t.Fatalf("ex.Name() = %s", ex.Name())
	}
}

func TestQueryMarketsReturnsBootstrapMarket(t *testing.T) {
	ex := NewExchange("127.0.0.1:11110")
	markets, err := ex.QueryMarkets(t.Context())
	if err != nil {
		t.Fatalf("QueryMarkets: %v", err)
	}
	market, ok := markets["HK.00700"]
	if !ok {
		t.Fatalf("expected bootstrap market HK.00700, got %#v", markets)
	}
	if market.Exchange != Name || market.QuoteCurrency != "HKD" {
		t.Fatalf("unexpected bootstrap market: %#v", market)
	}
}
