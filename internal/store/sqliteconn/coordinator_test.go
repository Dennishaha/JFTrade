package sqliteconn

import (
	"context"
	"errors"
	"path/filepath"
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
	uriCoordinator := coordinatorForPath("file:store.db?mode=ro")
	plainCoordinator := coordinatorForPath("store.db")
	t.Cleanup(func() {
		releaseCoordinatorForPath("file:store.db?mode=ro", uriCoordinator)
		releaseCoordinatorForPath("store.db", plainCoordinator)
	})
	if uriCoordinator != plainCoordinator {
		t.Fatal("file URI and plain path did not share a coordinator")
	}
	if got := coordinatorKey(":memory:"); got != ":memory:" {
		t.Fatalf("coordinatorKey(:memory:) = %q", got)
	}
	if got := coordinatorKey("  "); got != "" {
		t.Fatalf("coordinatorKey(blank) = %q", got)
	}
}

func TestCoordinatorRegistryReleasesLastDatabaseReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	first, err := Open(path)
	if err != nil {
		t.Fatalf("Open(first): %v", err)
	}
	second, err := Open(path)
	if err != nil {
		_ = first.Close()
		t.Fatalf("Open(second): %v", err)
	}
	coordinator := first.coordinator
	if second.coordinator != coordinator {
		t.Fatal("open databases did not share a write coordinator")
	}

	key := coordinatorKey(path)
	assertCoordinatorReferences(t, key, coordinator, 2)
	if err := first.Close(); err != nil {
		t.Fatalf("Close(first): %v", err)
	}
	assertCoordinatorReferences(t, key, coordinator, 1)
	if err := second.Close(); err != nil {
		t.Fatalf("Close(second): %v", err)
	}
	assertCoordinatorReferences(t, key, nil, 0)

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open(reopened): %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if reopened.coordinator == coordinator {
		t.Fatal("reopened database reused a released coordinator")
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close(reopened): %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("second Close(reopened): %v", err)
	}
	assertCoordinatorReferences(t, key, nil, 0)
}

func TestCoordinatorRegistryDefersRemovalUntilOutstandingWriteFinishes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.db")
	coordinator := coordinatorForPath(path)
	ticket := coordinator.enqueueWrite()
	if err := ticket.wait(context.Background()); err != nil {
		t.Fatalf("write ticket wait: %v", err)
	}
	releaseCoordinatorForPath(path, coordinator)
	assertCoordinatorReferences(t, coordinatorKey(path), coordinator, 0)

	ticket.finish()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		writeCoordinators.mu.Lock()
		entry := writeCoordinators.entries[coordinatorKey(path)]
		writeCoordinators.mu.Unlock()
		if entry == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("coordinator registry did not remove the idle zero-reference entry")
}

func assertCoordinatorReferences(t *testing.T, key string, wantCoordinator *writeCoordinator, wantReferences int) {
	t.Helper()
	writeCoordinators.mu.Lock()
	defer writeCoordinators.mu.Unlock()
	entry := writeCoordinators.entries[key]
	if wantCoordinator == nil {
		if entry != nil {
			t.Fatalf("coordinator registry entry %q still exists with %d references", key, entry.references)
		}
		return
	}
	if entry == nil || entry.coordinator != wantCoordinator || entry.references != wantReferences {
		t.Fatalf("coordinator registry entry %q = %#v, want coordinator %p with %d references", key, entry, wantCoordinator, wantReferences)
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
