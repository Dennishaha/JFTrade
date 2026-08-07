package broker_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

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

func TestSnapshotRateLimitErrorCarriesRetryDelay(t *testing.T) {
	err := broker.NewSnapshotRateLimitError(2500*time.Millisecond, errors.New("quota exhausted"))
	if !errors.Is(err, broker.ErrSnapshotRateLimited) || err.Error() != "quota exhausted" {
		t.Fatalf("rate limit error = %v", err)
	}
	retryAfter, ok := broker.SnapshotRetryAfter(fmt.Errorf("wrapped: %w", err))
	if !ok || retryAfter != 2500*time.Millisecond {
		t.Fatalf("retryAfter = %v, %t", retryAfter, ok)
	}
	defaultDelay, ok := broker.SnapshotRetryAfter(broker.NewSnapshotRateLimitError(0, nil))
	if !ok || defaultDelay != time.Second {
		t.Fatalf("default retryAfter = %v, %t", defaultDelay, ok)
	}
	var nilRateLimit *broker.SnapshotRateLimitError
	if nilRateLimit.Error() != broker.ErrSnapshotRateLimited.Error() {
		t.Fatalf("nil rate limit Error() = %q", nilRateLimit.Error())
	}
	if _, ok := broker.SnapshotRetryAfter(errors.New("plain")); ok {
		t.Fatal("plain error yielded retry delay")
	}
}

func TestSnapshotAvailabilityErrorsExposeFallbackEligibility(t *testing.T) {
	cause := errors.New("BasicQot entitlement is unavailable")
	for _, test := range []struct {
		name     string
		kind     broker.SnapshotAvailabilityKind
		eligible bool
	}{
		{name: "entitlement", kind: broker.SnapshotAvailabilityEntitlement, eligible: true},
		{name: "unsupported", kind: broker.SnapshotAvailabilityUnsupported, eligible: true},
		{name: "quota", kind: broker.SnapshotAvailabilityQuota, eligible: true},
		{name: "unknown", kind: broker.SnapshotAvailabilityKind("other"), eligible: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := broker.NewSnapshotAvailabilityError(test.kind, cause)
			if err == nil || err.Error() != cause.Error() || !errors.Is(err, cause) {
				t.Fatalf("availability error = %v", err)
			}
			kind, ok := broker.SnapshotAvailability(fmt.Errorf("wrapped: %w", err))
			if !ok || kind != test.kind {
				t.Fatalf("SnapshotAvailability = %q, %t", kind, ok)
			}
			if got := broker.IsSnapshotFallbackEligible(err); got != test.eligible {
				t.Fatalf("IsSnapshotFallbackEligible = %t, want %t", got, test.eligible)
			}
		})
	}
	if got := broker.NewSnapshotAvailabilityError(broker.SnapshotAvailabilityQuota, nil); got != nil {
		t.Fatalf("NewSnapshotAvailabilityError(nil) = %v", got)
	}
	if _, ok := broker.SnapshotAvailability(errors.New("plain")); ok {
		t.Fatal("plain error exposed availability")
	}
	if broker.IsSnapshotFallbackEligible(errors.New("plain")) {
		t.Fatal("plain error reported as fallback eligible")
	}
	var nilAvailability *broker.SnapshotAvailabilityError
	if nilAvailability.Error() != "broker snapshot availability is unavailable" {
		t.Fatalf("nil availability error = %q", nilAvailability.Error())
	}
}
