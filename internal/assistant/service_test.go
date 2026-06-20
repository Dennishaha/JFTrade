package assistant

import (
	"errors"
	"strings"
	"testing"
)

func TestWrapSessionTimelineErrorPreservesErrorChain(t *testing.T) {
	cause := errors.New("timeline storage failed")
	err := wrapSessionTimelineError(cause)

	if !errors.Is(err, ErrSessionTimelineFailed) {
		t.Fatalf("errors.Is(%v, ErrSessionTimelineFailed) = false", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(%v, cause) = false", err)
	}
	if got, want := err.Error(), "adk session timeline failed: timeline storage failed"; got != want {
		t.Fatalf("error text = %q, want %q", got, want)
	}
}

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
