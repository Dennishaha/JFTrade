package assistant

import (
	"context"
	"testing"
	"time"
)

func TestHandlerCloseCancelsAndJoinsBackgroundExecutions(t *testing.T) {
	handler := newHandler(nil)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})

	if !handler.startBackground(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		close(finished)
	}) {
		t.Fatal("background execution was rejected before shutdown")
	}
	waitForAssistantLifecycleSignal(t, started, "background execution start")

	closeResult := make(chan error, 1)
	go func() {
		closeResult <- handler.Close()
	}()
	waitForAssistantLifecycleSignal(t, cancelled, "background context cancellation")

	select {
	case err := <-closeResult:
		t.Fatalf("Close returned before the background execution exited: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	waitForAssistantLifecycleSignal(t, finished, "background execution exit")
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not return after the background execution exited")
	}

	if handler.startBackground(func(context.Context) {
		t.Error("background execution started after shutdown")
	}) {
		t.Fatal("background execution was accepted after shutdown")
	}
	if err := handler.Close(); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}
}

func waitForAssistantLifecycleSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
