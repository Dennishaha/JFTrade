package assistant

import (
	"strings"
	"testing"
)

func TestServiceUnavailableWithoutRuntime(t *testing.T) {
	service := NewService(nil)

	if service.Available() {
		t.Fatal("Available() = true, want false")
	}
	if _, err := service.Snapshot(t.Context()); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Snapshot() error = %v, want unavailable", err)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
}

func TestServiceOptionsExposeRuntimeSettings(t *testing.T) {
	service := NewService(nil,
		WithStreamIdleTimeout(func() int { return 420000 }),
		WithRuntimeSettings(func() any { return map[string]any{"enabled": true} }),
	)

	if got := service.StreamIdleTimeoutMillis(); got != 420000 {
		t.Fatalf("StreamIdleTimeoutMillis() = %d, want 420000", got)
	}
}

func TestAgentTemplatesAvailableWithoutRuntime(t *testing.T) {
	service := NewService(nil)

	templates, err := service.AgentTemplates(t.Context())
	if err != nil {
		t.Fatalf("AgentTemplates() error = %v", err)
	}
	if templates == nil {
		t.Fatal("AgentTemplates() = nil")
	}
}
