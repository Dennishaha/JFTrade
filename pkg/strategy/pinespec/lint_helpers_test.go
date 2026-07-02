package pinespec

import "testing"

func TestOptionalTypeAssertionReturnsZeroForUnexpectedType(t *testing.T) {
	if got := jftradeOptionalTypeAssertion[string](123); got != "" {
		t.Fatalf("unexpected string fallback: %q", got)
	}
	if got := jftradeOptionalTypeAssertion[int](42); got != 42 {
		t.Fatalf("typed assertion = %d", got)
	}
}
