package sqliteconn

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
)

var writeCoordinators = writeCoordinatorRegistry{entries: make(map[string]*writeCoordinatorEntry)}

type writeCoordinatorRegistry struct {
	mu      sync.Mutex
	entries map[string]*writeCoordinatorEntry
}

type writeCoordinatorEntry struct {
	coordinator      *writeCoordinator
	references       int
	cleanupScheduled bool
}

type writeCoordinator struct {
	mu   sync.Mutex
	tail <-chan struct{}
}

type writeBarrier struct {
	done <-chan struct{}
}

type writeTicket struct {
	previous <-chan struct{}
	done     chan struct{}
	once     sync.Once
}

func newWriteCoordinator() *writeCoordinator {
	ready := make(chan struct{})
	close(ready)
	return &writeCoordinator{tail: ready}
}

func coordinatorForPath(path string) *writeCoordinator {
	key := coordinatorKey(path)
	writeCoordinators.mu.Lock()
	defer writeCoordinators.mu.Unlock()
	if existing := writeCoordinators.entries[key]; existing != nil {
		existing.references++
		return existing.coordinator
	}
	coordinator := newWriteCoordinator()
	writeCoordinators.entries[key] = &writeCoordinatorEntry{coordinator: coordinator, references: 1}
	return coordinator
}

func releaseCoordinatorForPath(path string, coordinator *writeCoordinator) {
	if coordinator == nil {
		return
	}
	key := coordinatorKey(path)
	writeCoordinators.mu.Lock()
	entry := writeCoordinators.entries[key]
	if entry == nil || entry.coordinator != coordinator {
		writeCoordinators.mu.Unlock()
		return
	}
	if entry.references > 0 {
		entry.references--
	}
	if entry.references != 0 {
		writeCoordinators.mu.Unlock()
		return
	}
	if coordinator.idle() {
		delete(writeCoordinators.entries, key)
		writeCoordinators.mu.Unlock()
		return
	}
	scheduleCleanup := !entry.cleanupScheduled
	entry.cleanupScheduled = true
	writeCoordinators.mu.Unlock()
	if scheduleCleanup {
		go removeCoordinatorWhenIdle(key, coordinator)
	}
}

func removeCoordinatorWhenIdle(key string, coordinator *writeCoordinator) {
	for {
		<-coordinator.currentTail()

		writeCoordinators.mu.Lock()
		entry := writeCoordinators.entries[key]
		if entry == nil || entry.coordinator != coordinator {
			writeCoordinators.mu.Unlock()
			return
		}
		if entry.references != 0 {
			entry.cleanupScheduled = false
			writeCoordinators.mu.Unlock()
			return
		}
		if coordinator.idle() {
			delete(writeCoordinators.entries, key)
			writeCoordinators.mu.Unlock()
			return
		}
		writeCoordinators.mu.Unlock()
	}
}

func coordinatorKey(path string) string {
	key := strings.TrimSpace(path)
	if query := strings.IndexByte(key, '?'); query >= 0 {
		key = key[:query]
	}
	key = strings.TrimPrefix(strings.TrimPrefix(key, "file:"), "FILE:")
	if key == "" || key == ":memory:" {
		return strings.TrimSpace(path)
	}
	if absolute, err := filepath.Abs(key); err == nil {
		key = absolute
	}
	return filepath.Clean(key)
}

func (c *writeCoordinator) enqueueWrite() *writeTicket {
	done := make(chan struct{})
	c.mu.Lock()
	ticket := &writeTicket{previous: c.tail, done: done}
	c.tail = done
	c.mu.Unlock()
	return ticket
}

func (c *writeCoordinator) readBarrier() writeBarrier {
	c.mu.Lock()
	barrier := writeBarrier{done: c.tail}
	c.mu.Unlock()
	return barrier
}

func (c *writeCoordinator) currentTail() <-chan struct{} {
	c.mu.Lock()
	tail := c.tail
	c.mu.Unlock()
	return tail
}

func (c *writeCoordinator) idle() bool {
	select {
	case <-c.currentTail():
		return true
	default:
		return false
	}
}

func (b writeBarrier) wait(ctx context.Context) error {
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *writeTicket) wait(ctx context.Context) error {
	select {
	case <-t.previous:
		return nil
	case <-ctx.Done():
		go func() {
			<-t.previous
			t.finish()
		}()
		return ctx.Err()
	}
}

func (t *writeTicket) finish() {
	t.once.Do(func() { close(t.done) })
}
