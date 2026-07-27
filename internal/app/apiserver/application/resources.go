// Package application provides process-local application composition helpers.
package application

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ErrResourcesClosed reports an attempt to register a resource after shutdown.
var ErrResourcesClosed = errors.New("application resources are already closed")

type resource struct {
	name    string
	closeFn func() error
}

// Resources owns application resources in successful startup order.
//
// The zero value is ready to use. Close releases resources in reverse startup
// order, aggregates every named error, and is safe to call repeatedly or
// concurrently.
type Resources struct {
	mu        sync.Mutex
	closeOnce sync.Once
	entries   []resource
	closed    bool
	closeErr  error
}

// Register records a successfully opened resource.
//
// When registration races with or follows shutdown, closeFn is executed
// immediately so the caller cannot leak the newly opened resource.
func (r *Resources) Register(name string, closeFn func() error) error {
	if closeFn == nil {
		return nil
	}
	entry := resource{name: resourceName(name), closeFn: closeFn}
	r.mu.Lock()
	if !r.closed {
		r.entries = append(r.entries, entry)
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()
	return errors.Join(ErrResourcesClosed, closeResource(entry))
}

// Close releases every registered resource in reverse startup order.
func (r *Resources) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		entries := append([]resource(nil), r.entries...)
		r.entries = nil
		r.mu.Unlock()

		errs := make([]error, 0, len(entries))
		for index := len(entries) - 1; index >= 0; index-- {
			if err := closeResource(entries[index]); err != nil {
				errs = append(errs, err)
			}
		}
		r.closeErr = errors.Join(errs...)
	})
	return r.closeErr
}

// Rollback combines a startup failure with reverse-order resource cleanup.
func (r *Resources) Rollback(startupErr error) error {
	return errors.Join(startupErr, r.Close())
}

// Open opens one named resource and registers its close function. If opening
// fails, all resources previously opened through the same Resources are rolled
// back in reverse order.
func Open[T any](
	resources *Resources,
	name string,
	openFn func() (T, error),
	closeFn func(T) error,
) (T, error) {
	var zero T
	value, err := openFn()
	if err != nil {
		return zero, resources.Rollback(fmt.Errorf("open %s: %w", resourceName(name), err))
	}
	if closeFn == nil {
		return value, nil
	}
	if err := resources.Register(name, func() error { return closeFn(value) }); err != nil {
		return zero, err
	}
	return value, nil
}

func closeResource(entry resource) error {
	if err := entry.closeFn(); err != nil {
		return fmt.Errorf("close %s: %w", entry.name, err)
	}
	return nil
}

func resourceName(name string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return "unnamed resource"
}
