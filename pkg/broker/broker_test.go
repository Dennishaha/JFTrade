package broker_test

import (
	"context"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestRegistryBasic(t *testing.T) {
	r := broker.NewRegistry()

	if ids := r.IDs(); len(ids) != 0 {
		t.Fatalf("expected empty registry, got %d IDs", len(ids))
	}
	if b := r.ActiveBroker(); b != nil {
		t.Fatalf("expected nil active broker, got %v", b)
	}
	if b := r.Lookup("futu"); b != nil {
		t.Fatalf("expected nil lookup, got %v", b)
	}
}

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := broker.NewRegistry()
	mock := &mockBroker{id: "test-broker"}
	r.Register(mock)

	if b := r.Lookup("test-broker"); b == nil {
		t.Fatal("expected to find registered broker")
	}
	if b := r.ActiveBroker(); b == nil || b.ID() != "test-broker" {
		t.Fatal("expected active broker to be the registered one")
	}
	if ids := r.IDs(); len(ids) != 1 || ids[0] != "test-broker" {
		t.Fatalf("expected IDs [test-broker], got %v", ids)
	}
	if all := r.All(); len(all) != 1 {
		t.Fatalf("expected 1 broker, got %d", len(all))
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	r := broker.NewRegistry()
	r.Register(&mockBroker{id: "dup"})

	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	r.Register(&mockBroker{id: "dup"})
}

func TestBrokerTypesFieldCompatibility(t *testing.T) {
	// Verify that broker.ReadQuery has the expected fields.
	q := broker.ReadQuery{
		BrokerID:           "futu",
		AccountID:          "123",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
	}
	if q.BrokerID != "futu" {
		t.Fatalf("expected BrokerID=futu, got %s", q.BrokerID)
	}
}

func TestDescriptorFields(t *testing.T) {
	d := broker.Descriptor{
		ID:          "test",
		DisplayName: "Test Broker",
		Environments: []string{"SIMULATE", "REAL"},
		Capabilities: []broker.MarketCapability{{
			Market:        "HK",
			SupportsQuote: true,
			SupportsTrade: true,
		}},
	}
	if d.ID != "test" {
		t.Fatalf("expected ID=test, got %s", d.ID)
	}
	if len(d.Capabilities) != 1 {
		t.Fatalf("expected 1 capability, got %d", len(d.Capabilities))
	}
}

func TestBrokerError(t *testing.T) {
	err := broker.NewBrokerError("futu", broker.ErrCodeNotConnected, "connection refused")
	if err.BrokerID != "futu" {
		t.Fatalf("expected BrokerID=futu, got %s", err.BrokerID)
	}
	if err.Code != broker.ErrCodeNotConnected {
		t.Fatalf("expected Code=%s, got %s", broker.ErrCodeNotConnected, err.Code)
	}
	msg := err.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestHelperFunctions(t *testing.T) {
	f := broker.Float64Ptr(3.14)
	if f == nil || *f != 3.14 {
		t.Fatalf("expected *3.14, got %v", f)
	}
	s := broker.StringPtr("hello")
	if s == nil || *s != "hello" {
		t.Fatalf("expected *hello, got %v", s)
	}
	b := broker.BoolPtr(true)
	if b == nil || !*b {
		t.Fatalf("expected *true, got %v", b)
	}
}

type mockBroker struct {
	id string
}

func (m *mockBroker) ID() string                                         { return m.id }
func (m *mockBroker) Descriptor() broker.Descriptor                      { return broker.Descriptor{ID: m.id} }
func (m *mockBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) { return nil, nil }
func (m *mockBroker) Trading() broker.TradingService                     { return nil }
func (m *mockBroker) MarketData() broker.MarketDataReader                { return nil }
