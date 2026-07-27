package application

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLifecycleClosesAdoptedResourcesInReverseOrderAndPreservesSetupError(t *testing.T) {
	startupErr := errors.New("runtime assembly failed")
	var resources Resources
	var closed []string
	for _, name := range []string{"persistent stores", "authentication"} {
		resourceName := name
		if err := resources.Register(resourceName, func() error {
			closed = append(closed, resourceName)
			return nil
		}); err != nil {
			t.Fatalf("register %s: %v", resourceName, err)
		}
	}
	lifecycle := NewLifecycle(&resources, startupErr, true, true)
	lifecycle.Register("application runtimes", func() error {
		closed = append(closed, "application runtimes")
		return nil
	})

	for range 2 {
		if err := lifecycle.Close(); !errors.Is(err, startupErr) {
			t.Fatalf("close error = %v, want startup error", err)
		}
	}
	want := []string{"application runtimes", "authentication", "persistent stores"}
	if !reflect.DeepEqual(closed, want) {
		t.Fatalf("close order = %v, want %v", closed, want)
	}
}

func TestLifecycleEnsuresOnlyMissingOwnershipGroupsOnce(t *testing.T) {
	const callers = 16
	var persistentCalls atomic.Int32
	var runtimeCalls atomic.Int32
	lifecycle := NewLifecycle(nil, nil, true, false)

	var callersDone sync.WaitGroup
	callersDone.Add(callers)
	for range callers {
		go func() {
			defer callersDone.Done()
			lifecycle.EnsureOwnedResources(
				func() { persistentCalls.Add(1) },
				func() { runtimeCalls.Add(1) },
			)
		}()
	}
	callersDone.Wait()

	if got := persistentCalls.Load(); got != 0 {
		t.Fatalf("persistent registration calls = %d, want 0", got)
	}
	if got := runtimeCalls.Load(); got != 1 {
		t.Fatalf("runtime registration calls = %d, want 1", got)
	}
}

func TestLifecycleRecordsLateRegistrationFailureAndClosesResource(t *testing.T) {
	var lifecycle Lifecycle
	if err := lifecycle.Close(); err != nil {
		t.Fatalf("initial close: %v", err)
	}
	closeErr := errors.New("late resource close failed")
	var closeCalls atomic.Int32
	lifecycle.Register("late runtime", func() error {
		closeCalls.Add(1)
		return closeErr
	})

	if got := closeCalls.Load(); got != 1 {
		t.Fatalf("late close calls = %d, want 1", got)
	}
	setupErr := lifecycle.SetupError()
	if !errors.Is(setupErr, ErrResourcesClosed) || !errors.Is(setupErr, closeErr) {
		t.Fatalf("setup error = %v, want closed and close failures", setupErr)
	}
	err := lifecycle.Close()
	if !errors.Is(err, ErrResourcesClosed) || !errors.Is(err, closeErr) {
		t.Fatalf("close error = %v, want late registration failures", err)
	}
}
