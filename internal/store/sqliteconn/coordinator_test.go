package sqliteconn

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWriteCoordinatorOrdersWritersAndReadBarriers(t *testing.T) {
	coordinator := newWriteCoordinator()
	first := coordinator.enqueueWrite()
	second := coordinator.enqueueWrite()

	if err := first.wait(context.Background()); err != nil {
		t.Fatalf("first.wait: %v", err)
	}

	secondReady := make(chan error, 1)
	go func() { secondReady <- second.wait(context.Background()) }()
	assertNotSignalled(t, secondReady, "second writer")

	readReady := make(chan error, 1)
	barrier := coordinator.readBarrier()
	go func() { readReady <- barrier.wait(context.Background()) }()
	assertNotSignalled(t, readReady, "read barrier")

	first.finish()
	if err := waitForSignal(t, secondReady, "second writer"); err != nil {
		t.Fatalf("second.wait: %v", err)
	}
	assertNotSignalled(t, readReady, "read barrier before second write")

	second.finish()
	if err := waitForSignal(t, readReady, "read barrier"); err != nil {
		t.Fatalf("read barrier: %v", err)
	}

	// Completion is idempotent so error cleanup and caller cleanup can safely overlap.
	second.finish()
}

func TestWriteCoordinatorAllowsAdmittedReadsToOverlapLaterWrites(t *testing.T) {
	coordinator := newWriteCoordinator()
	barrier := coordinator.readBarrier()
	laterWrite := coordinator.enqueueWrite()

	if err := barrier.wait(context.Background()); err != nil {
		t.Fatalf("barrier.wait: %v", err)
	}
	if err := laterWrite.wait(context.Background()); err != nil {
		t.Fatalf("laterWrite.wait: %v", err)
	}
	laterWrite.finish()
}

func TestWriteCoordinatorCancellationDoesNotBlockFollowingWork(t *testing.T) {
	coordinator := newWriteCoordinator()
	first := coordinator.enqueueWrite()
	cancelled := coordinator.enqueueWrite()
	last := coordinator.enqueueWrite()

	if err := first.wait(context.Background()); err != nil {
		t.Fatalf("first.wait: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cancelled.wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled.wait error = %v, want context.Canceled", err)
	}

	lastReady := make(chan error, 1)
	go func() { lastReady <- last.wait(context.Background()) }()
	assertNotSignalled(t, lastReady, "last writer")
	first.finish()
	if err := waitForSignal(t, lastReady, "last writer"); err != nil {
		t.Fatalf("last.wait: %v", err)
	}
	last.finish()
}

func TestWriteCoordinatorReadBarrierHonorsCancellation(t *testing.T) {
	coordinator := newWriteCoordinator()
	write := coordinator.enqueueWrite()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := coordinator.readBarrier().wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("barrier.wait error = %v, want context.Canceled", err)
	}
	write.finish()
}

func TestCoordinatorRegistryNormalizesDatabasePaths(t *testing.T) {
	if coordinatorForPath("file:store.db?mode=ro") != coordinatorForPath("store.db") {
		t.Fatal("file URI and plain path did not share a coordinator")
	}
	if got := coordinatorKey(":memory:"); got != ":memory:" {
		t.Fatalf("coordinatorKey(:memory:) = %q", got)
	}
	if got := coordinatorKey("  "); got != "" {
		t.Fatalf("coordinatorKey(blank) = %q", got)
	}
}

func assertNotSignalled[T any](t *testing.T, ch <-chan T, name string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s signalled too early", name)
	case <-time.After(30 * time.Millisecond):
	}
}

func waitForSignal[T any](t *testing.T, ch <-chan T, name string) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
		var zero T
		return zero
	}
}
