package backtest

import (
	"path/filepath"
	"testing"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
)

func TestConstructorsReturnRunResourcesWithIdempotentClose(t *testing.T) {
	durableConstructor := requireRunResourceConstructor(New)
	degradedConstructor := requireInMemoryRunResourceConstructor(NewInMemory)

	resource, err := durableConstructor(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var consumer btsrv.RunStore = resource
	if consumer == nil || !resource.Available() {
		t.Fatalf("durable resource = %#v", resource)
	}
	if err := resource.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := resource.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	degraded := degradedConstructor()
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

func requireRunResourceConstructor(
	constructor func(string) (Resource, error),
) func(string) (Resource, error) {
	return constructor
}

func requireInMemoryRunResourceConstructor(constructor func() Resource) func() Resource {
	return constructor
}
