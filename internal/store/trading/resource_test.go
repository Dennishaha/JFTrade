package trading

import (
	"path/filepath"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestConstructorsReturnExecutionResourcesWithIdempotentClose(t *testing.T) {
	durableConstructor := requireExecutionResourceConstructor(New)
	degradedConstructor := requireInMemoryExecutionResourceConstructor(NewInMemory)

	resource, err := durableConstructor(filepath.Join(t.TempDir(), "execution.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var orderConsumer trdsrv.OrderStore = resource
	var previewConsumer trdsrv.ExecutionPreviewStore = resource
	if orderConsumer == nil || previewConsumer == nil || !resource.Available() {
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

func requireExecutionResourceConstructor(
	constructor func(string) (Resource, error),
) func(string) (Resource, error) {
	return constructor
}

func requireInMemoryExecutionResourceConstructor(constructor func() Resource) func() Resource {
	return constructor
}
