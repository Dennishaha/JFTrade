package futu_test

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu"
)

// TestFutuAdapterCompileTimeChecks validates that the Futu adapter implements
// all required broker interfaces at compile time.
func TestFutuAdapterCompileTimeChecks(t *testing.T) {
	// These compile-time checks are in adapter_convert.go:
	//   var _ broker.Broker = (*futuAdapter)(nil)
	//   var _ broker.TradingService = (*futuTradingService)(nil)
	//   var _ broker.MarketDataReader = (*futuMarketDataReader)(nil)
	// If they compile, the interfaces are satisfied.
	// We just verify that the NewBrokerAdapter function returns a broker.Broker.
	exchange := futu.NewExchange(futu.DefaultOpenDAddr)
	adapter := futu.NewBrokerAdapter(exchange)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}

	// Verify it implements broker.Broker.
	var b broker.Broker = adapter
	if b.ID() != "futu" {
		t.Fatalf("expected ID=futu, got %s", b.ID())
	}

	desc := b.Descriptor()
	if desc.ID != "futu" {
		t.Fatalf("expected descriptor ID=futu, got %s", desc.ID)
	}
	if desc.DisplayName == "" {
		t.Fatal("expected non-empty display name")
	}
	if len(desc.Environments) == 0 {
		t.Fatal("expected at least one environment")
	}
	if len(desc.Capabilities) == 0 {
		t.Fatal("expected at least one capability")
	}

	// Verify MarketData and Trading return non-nil implementations.
	if b.MarketData() == nil {
		t.Fatal("expected non-nil MarketData")
	}
	if b.Trading() == nil {
		t.Fatal("expected non-nil Trading")
	}
}

// TestBrokerRegistryWithFutu validates that the Futu broker can be registered
// and discovered through the broker registry.
func TestBrokerRegistryWithFutu(t *testing.T) {
	r := broker.NewRegistry()
	exchange := futu.NewExchange(futu.DefaultOpenDAddr)
	adapter := futu.NewBrokerAdapter(exchange)
	r.Register(adapter)

	if b := r.Lookup("futu"); b == nil {
		t.Fatal("expected to find futu broker in registry")
	}
	if b := r.ActiveBroker(); b == nil || b.ID() != "futu" {
		t.Fatal("expected futu as active broker")
	}
	if ids := r.IDs(); len(ids) != 1 || ids[0] != "futu" {
		t.Fatalf("expected IDs [futu], got %v", ids)
	}
}
