package adk

import (
	"context"
	"testing"
)

func TestToolRegistryOnChangeNotifiesAndUnsubscribesIdempotently(t *testing.T) {
	registry := NewToolRegistry()
	called := 0
	remove := registry.OnChange(func() { called++ })
	registry.Register(ToolDescriptor{Name: "market.test", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	if called != 1 {
		t.Fatalf("OnChange calls after registration = %d, want 1", called)
	}
	remove()
	remove()
	registry.Register(ToolDescriptor{Name: "market.test.two", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	})
	if called != 1 {
		t.Fatalf("OnChange calls after unsubscribe = %d, want 1", called)
	}

	var nilRegistry *ToolRegistry
	nilRegistry.OnChange(nil)()
	registry.OnChange(nil)()
}
