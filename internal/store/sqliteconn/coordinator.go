package sqliteconn

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
)

var writeCoordinators sync.Map

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
	if existing, ok := writeCoordinators.Load(key); ok {
		return existing.(*writeCoordinator)
	}
	coordinator := newWriteCoordinator()
	actual, _ := writeCoordinators.LoadOrStore(key, coordinator)
	return actual.(*writeCoordinator)
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
