package broker

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestFactorRefIdentityAndStableConstruction(t *testing.T) {
	explicit := NewFactorRef(" Simple.Price ", ResearchScreenFactorParams{Period: 11}, " price ")
	if explicit.Identity() != "price" || explicit.FactorKey != "simple.price" || explicit.Params.Period != 11 {
		t.Fatalf("explicit ref = %#v", explicit)
	}

	first := NewFactorRef("indicator.rsi", map[string]any{"period": 11}, "")
	second := NewFactorRef(" INDICATOR.RSI ", map[string]any{"period": float64(11)}, "")
	different := NewFactorRef("indicator.rsi", map[string]any{"period": 21}, "")
	if first.InstanceID == "" || first.InstanceID != second.InstanceID {
		t.Fatalf("stable identities differ: %#v %#v", first, second)
	}
	if first.InstanceID == different.InstanceID {
		t.Fatalf("different params share identity %q", first.InstanceID)
	}
	if (FactorRef{FactorKey: " simple.price "}).Identity() != "simple.price" {
		t.Fatal("factor-key fallback identity was not normalized")
	}
}

func TestNewFactorRefRejectsUnrepresentableOrInvalidParameters(t *testing.T) {
	invalidJSON := NewFactorRef("simple.price", func() {}, "")
	if !reflect.DeepEqual(invalidJSON.Params, ResearchScreenFactorParams{}) {
		t.Fatalf("function params = %#v", invalidJSON.Params)
	}
	invalidShape := NewFactorRef("simple.price", "not-an-object", "")
	if !reflect.DeepEqual(invalidShape.Params, ResearchScreenFactorParams{}) {
		t.Fatalf("string params = %#v", invalidShape.Params)
	}
	nullParams := NewFactorRef("simple.price", nil, "")
	if !reflect.DeepEqual(nullParams.Params, ResearchScreenFactorParams{}) {
		t.Fatalf("nil params = %#v", nullParams.Params)
	}
}

func TestResearchScreenRateLimitErrorContract(t *testing.T) {
	var nilError *ResearchScreenRateLimitError
	if nilError.Error() != ErrResearchScreenRateLimited.Error() {
		t.Fatalf("nil error text = %q", nilError.Error())
	}

	defaulted := NewResearchScreenRateLimitError(0)
	if !errors.Is(defaulted, ErrResearchScreenRateLimited) {
		t.Fatalf("errors.Is(%v) = false", defaulted)
	}
	retryAfter, ok := ResearchScreenRetryAfter(fmt.Errorf("wrapped: %w", defaulted))
	if !ok || retryAfter != time.Second {
		t.Fatalf("default retry = %s, %v", retryAfter, ok)
	}
	if retryAfter, ok := ResearchScreenRetryAfter(errors.New("other")); ok || retryAfter != 0 {
		t.Fatalf("unrelated retry = %s, %v", retryAfter, ok)
	}

	explicit := NewResearchScreenRateLimitError(1500 * time.Millisecond)
	if explicit.Error() != "research stock screen rate limited; retry after 1.5s" {
		t.Fatalf("explicit error = %q", explicit.Error())
	}
}
