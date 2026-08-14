package application

import (
	"context"
	"reflect"
	"testing"

	"github.com/jftrade/jftrade-main/internal/system"
)

func TestRuntimeDependenciesDelegatesAndReportsUnavailableService(t *testing.T) {
	if got := RuntimeDependencies(nil)(context.Background()); !reflect.DeepEqual(got, map[string]any{"status": "unavailable"}) {
		t.Fatalf("RuntimeDependencies(nil) = %#v", got)
	}
	service := system.NewService(system.WithRuntimeDependencies(func(context.Context) map[string]any {
		return map[string]any{"python": "ready"}
	}))
	if got := RuntimeDependencies(service)(context.Background()); !reflect.DeepEqual(got, map[string]any{"python": "ready"}) {
		t.Fatalf("RuntimeDependencies(service) = %#v", got)
	}
}
