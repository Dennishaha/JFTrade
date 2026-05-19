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
	ex, err := exchange.New(types.ExchangeName("futu"), exchange.Options{"OPEND_ADDR": "127.0.0.1:11111"})
	if err != nil {
		t.Fatalf("exchange.New: %v", err)
	}
	if ex.Name() != Name {
		t.Fatalf("ex.Name() = %s", ex.Name())
	}
}

func TestConstructorRequiresAddress(t *testing.T) {
	t.Setenv("FUTU_OPEND_ADDR", "")
	_, err := exchange.New(types.ExchangeName("futu"), exchange.Options{})
	if err == nil {
		t.Fatal("expected error for empty OpenD address")
	}
}
