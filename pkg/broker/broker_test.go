package broker_test

import (
	"context"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

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

func TestRegistryReplaceUpdatesActiveBroker(t *testing.T) {
	r := broker.NewRegistry()
	original := &mockBroker{id: "replaceable"}
	replacement := &mockBroker{id: "replaceable"}
	r.Register(original)

	r.Replace(replacement)
	if got := r.Lookup("replaceable"); got != replacement {
		t.Fatalf("Lookup after Replace = %#v, want replacement broker", got)
	}
	if got := r.ActiveBroker(); got != replacement {
		t.Fatalf("ActiveBroker after Replace = %#v, want replacement broker", got)
	}

	newBroker := &mockBroker{id: "new-broker"}
	r.Replace(newBroker)
	if got := r.Lookup("new-broker"); got != newBroker {
		t.Fatalf("Lookup new broker after Replace = %#v", got)
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

func TestConvertFutuReadQuery(t *testing.T) {
	q := broker.ConvertFutuReadQuery("123", "SIMULATE", "HK")

	if q.BrokerID != "futu" {
		t.Fatalf("BrokerID = %q, want futu", q.BrokerID)
	}
	if q.AccountID != "123" {
		t.Fatalf("AccountID = %q, want 123", q.AccountID)
	}
	if q.TradingEnvironment != "SIMULATE" {
		t.Fatalf("TradingEnvironment = %q, want SIMULATE", q.TradingEnvironment)
	}
	if q.Market != "HK" {
		t.Fatalf("Market = %q, want HK", q.Market)
	}
}

func TestPointerHelpersReturnStableIndependentValues(t *testing.T) {
	float64Ptr := broker.Float64Ptr
	stringPtr := broker.StringPtr
	boolPtr := broker.BoolPtr
	uint64Ptr := broker.Uint64Ptr

	floatPtr := float64Ptr(12.5)
	secondFloatPtr := float64Ptr(12.5)
	if floatPtr == nil || *floatPtr != 12.5 {
		t.Fatalf("Float64Ptr = %#v, want independent 12.5", floatPtr)
	}
	if floatPtr == secondFloatPtr {
		t.Fatal("Float64Ptr should return a fresh pointer")
	}

	stringValuePtr := stringPtr("HK.00700")
	secondStringValuePtr := stringPtr("HK.00700")
	if stringValuePtr == nil || *stringValuePtr != "HK.00700" {
		t.Fatalf("StringPtr = %#v, want independent HK.00700", stringValuePtr)
	}
	if stringValuePtr == secondStringValuePtr {
		t.Fatal("StringPtr should return a fresh pointer")
	}

	boolValuePtr := boolPtr(true)
	secondBoolValuePtr := boolPtr(true)
	if boolValuePtr == nil || *boolValuePtr != true {
		t.Fatalf("BoolPtr = %#v, want independent true", boolValuePtr)
	}
	if boolValuePtr == secondBoolValuePtr {
		t.Fatal("BoolPtr should return a fresh pointer")
	}

	uintPtr := uint64Ptr(42)
	secondUintPtr := uint64Ptr(42)
	if uintPtr == nil || *uintPtr != 42 {
		t.Fatalf("Uint64Ptr = %#v, want independent 42", uintPtr)
	}
	if uintPtr == secondUintPtr {
		t.Fatal("Uint64Ptr should return a fresh pointer")
	}
}

func TestApplyMarketRuleUsesLotSizeAsQuantityConstraints(t *testing.T) {
	lotSize := int32(100)
	market := types.Market{
		Symbol:      "HK.00700",
		MinQuantity: fixedpoint.One,
		StepSize:    fixedpoint.One,
	}

	enriched := broker.ApplyMarketRule(market, broker.MarketRuleItem{
		Symbol:  "HK.00700",
		LotSize: &lotSize,
	})

	if enriched.MinQuantity.Float64() != 100 || enriched.StepSize.Float64() != 100 {
		t.Fatalf("quantity constraints = min %s step %s, want 100/100", enriched.MinQuantity, enriched.StepSize)
	}
}

func TestApplyMarketRuleIgnoresMissingAndInvalidLotSize(t *testing.T) {
	market := types.Market{
		Symbol:      "HK.00700",
		MinQuantity: fixedpoint.NewFromFloat(5),
		StepSize:    fixedpoint.NewFromFloat(5),
	}
	for _, lotSize := range []*int32{nil, new(int32)} {
		enriched := broker.ApplyMarketRule(market, broker.MarketRuleItem{Symbol: "HK.00700", LotSize: lotSize})
		if enriched.MinQuantity.Float64() != 5 || enriched.StepSize.Float64() != 5 {
			t.Fatalf("quantity constraints changed for lotSize %#v: min %s step %s", lotSize, enriched.MinQuantity, enriched.StepSize)
		}
	}
	negative := int32(-100)
	enriched := broker.ApplyMarketRule(market, broker.MarketRuleItem{Symbol: "HK.00700", LotSize: &negative})
	if enriched.MinQuantity.Float64() != 5 || enriched.StepSize.Float64() != 5 {
		t.Fatalf("quantity constraints changed for negative lotSize: min %s step %s", enriched.MinQuantity, enriched.StepSize)
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

type mockBroker struct {
	id string
}

func (m *mockBroker) ID() string                                                 { return m.id }
func (m *mockBroker) Descriptor() broker.Descriptor                              { return broker.Descriptor{ID: m.id} }
func (m *mockBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) { return nil, nil }
func (m *mockBroker) Trading() broker.TradingService                             { return nil }
func (m *mockBroker) MarketData() broker.MarketDataReader                        { return nil }
