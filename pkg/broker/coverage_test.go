package broker_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestApplyMarketRulesMatchesAndOverridesConstraints(t *testing.T) {
	market := types.Market{Symbol: " hk.00700 ", MinQuantity: fixedpoint.One, StepSize: fixedpoint.One}
	lotSize := int32(100)
	minimum := 200.0
	step := 50.0
	rules := []broker.MarketRuleItem{
		{Symbol: "US.AAPL", LotSize: &lotSize},
		{Symbol: " HK.00700 ", LotSize: &lotSize, MinQuantity: &minimum, StepSize: &step},
	}
	got := broker.ApplyMarketRules(market, rules)
	if got.MinQuantity.Float64() != minimum || got.StepSize.Float64() != step {
		t.Fatalf("constraints = %s/%s, want %.0f/%.0f", got.MinQuantity, got.StepSize, minimum, step)
	}
	unmatched := broker.ApplyMarketRules(market, rules[:1])
	if unmatched.MinQuantity != market.MinQuantity || unmatched.StepSize != market.StepSize {
		t.Fatalf("unmatched market changed: %#v", unmatched)
	}
}

func TestApplyMarketRuleIgnoresInvalidExplicitConstraints(t *testing.T) {
	market := types.Market{MinQuantity: fixedpoint.One, StepSize: fixedpoint.One}
	zero := 0.0
	negative := -1.0
	got := broker.ApplyMarketRule(market, broker.MarketRuleItem{MinQuantity: &zero, StepSize: &negative})
	if got.MinQuantity != market.MinQuantity || got.StepSize != market.StepSize {
		t.Fatalf("invalid constraints changed market: %#v", got)
	}
}

func TestSymbolScopedSnapshotError(t *testing.T) {
	if got := broker.NewSymbolScopedSnapshotError(nil); got != nil {
		t.Fatalf("NewSymbolScopedSnapshotError(nil) = %v", got)
	}
	cause := errors.New("bad symbol")
	wrapped := broker.NewSymbolScopedSnapshotError(cause)
	if wrapped.Error() != cause.Error() {
		t.Fatalf("Error() = %q", wrapped.Error())
	}
	if !errors.Is(wrapped, cause) || !broker.IsSymbolScopedSnapshotError(wrapped) {
		t.Fatalf("wrapped error is not discoverable: %v", wrapped)
	}
	if !broker.IsSymbolScopedSnapshotError(fmt.Errorf("outer: %w", wrapped)) {
		t.Fatal("nested symbol-scoped error was not detected")
	}
	if broker.IsSymbolScopedSnapshotError(cause) {
		t.Fatal("plain error reported as symbol-scoped")
	}
}
