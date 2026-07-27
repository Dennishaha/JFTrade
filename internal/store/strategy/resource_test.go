package strategy

import (
	"path/filepath"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

func TestConstructorsReturnDesignResourcesWithIdempotentClose(t *testing.T) {
	durableConstructor := requireStrategyResourceConstructor(New)
	degradedConstructor := requireUnavailableStrategyResourceConstructor(NewUnavailable)

	resource, err := durableConstructor(filepath.Join(t.TempDir(), "strategy.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var consumer stratsrv.DesignStore = resource
	if consumer == nil || !resource.Available() {
		t.Fatalf("durable resource = %#v", resource)
	}
	if err := resource.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := resource.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	degraded := degradedConstructor(filepath.Join(t.TempDir(), "unavailable.json"))
	if degraded == nil || degraded.Available() {
		t.Fatalf("degraded resource = %#v", degraded)
	}
	if err := degraded.Close(); err != nil {
		t.Fatalf("degraded Close: %v", err)
	}
	if err := degraded.Close(); err != nil {
		t.Fatalf("second degraded Close: %v", err)
	}
}

func requireStrategyResourceConstructor(
	constructor func(string) (Resource, error),
) func(string) (Resource, error) {
	return constructor
}

func requireUnavailableStrategyResourceConstructor(
	constructor func(string) Resource,
) func(string) Resource {
	return constructor
}
