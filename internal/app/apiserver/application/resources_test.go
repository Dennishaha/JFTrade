package application

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestResourcesClosesApplicationDependenciesInReverseStartupOrder(t *testing.T) {
	var resources Resources
	var closed []string
	for _, name := range []string{"settings store", "strategy store", "broker runtime"} {
		resourceName := name
		if err := resources.Register(resourceName, func() error {
			closed = append(closed, resourceName)
			return nil
		}); err != nil {
			t.Fatalf("register %s: %v", resourceName, err)
		}
	}

	if err := resources.Close(); err != nil {
		t.Fatalf("close resources: %v", err)
	}
	want := []string{"broker runtime", "strategy store", "settings store"}
	if !reflect.DeepEqual(closed, want) {
		t.Fatalf("close order = %v, want %v", closed, want)
	}
}

func TestOpenRollsBackEarlierResourcesWhenLaterStartupFails(t *testing.T) {
	startupErr := errors.New("dial broker")
	closeErr := errors.New("flush strategy state")
	var resources Resources
	var closed []string

	_, err := Open(&resources, "strategy store", func() (string, error) {
		return "strategy", nil
	}, func(string) error {
		closed = append(closed, "strategy store")
		return closeErr
	})
	if err != nil {
		t.Fatalf("open strategy store: %v", err)
	}
	_, err = Open(&resources, "market data runtime", func() (string, error) {
		return "market data", nil
	}, func(string) error {
		closed = append(closed, "market data runtime")
		return nil
	})
	if err != nil {
		t.Fatalf("open market data runtime: %v", err)
	}
	_, err = Open(&resources, "broker runtime", func() (string, error) {
		return "", startupErr
	}, func(string) error {
		t.Fatal("failed resource must not be closed")
		return nil
	})

	if !errors.Is(err, startupErr) || !errors.Is(err, closeErr) {
		t.Fatalf("rollback error = %v, want startup and close causes", err)
	}
	if !strings.Contains(err.Error(), "open broker runtime") ||
		!strings.Contains(err.Error(), "close strategy store") {
		t.Fatalf("rollback error lacks resource names: %v", err)
	}
	want := []string{"market data runtime", "strategy store"}
	if !reflect.DeepEqual(closed, want) {
		t.Fatalf("rollback order = %v, want %v", closed, want)
	}
}

func TestResourcesCloseIsIdempotentAndConcurrentSafe(t *testing.T) {
	var resources Resources
	closeErr := errors.New("shutdown timeout")
	var closeCalls atomic.Int32
	if err := resources.Register("assistant runtime", func() error {
		closeCalls.Add(1)
		return closeErr
	}); err != nil {
		t.Fatalf("register assistant runtime: %v", err)
	}

	const callers = 16
	results := make(chan error, callers)
	var callersDone sync.WaitGroup
	callersDone.Add(callers)
	for range callers {
		go func() {
			defer callersDone.Done()
			results <- resources.Close()
		}()
	}
	callersDone.Wait()
	close(results)

	for err := range results {
		if !errors.Is(err, closeErr) {
			t.Fatalf("close error = %v, want shutdown cause", err)
		}
	}
	if got := closeCalls.Load(); got != 1 {
		t.Fatalf("close calls = %d, want 1", got)
	}
}

func TestResourcesCloseAggregatesEveryNamedFailure(t *testing.T) {
	var resources Resources
	storeErr := errors.New("database busy")
	runtimeErr := errors.New("worker stuck")
	if err := resources.Register("strategy store", func() error { return storeErr }); err != nil {
		t.Fatalf("register strategy store: %v", err)
	}
	if err := resources.Register("pine runtime", func() error { return runtimeErr }); err != nil {
		t.Fatalf("register pine runtime: %v", err)
	}

	err := resources.Close()
	if !errors.Is(err, storeErr) || !errors.Is(err, runtimeErr) {
		t.Fatalf("close error = %v, want both causes", err)
	}
	message := err.Error()
	if !strings.Contains(message, "close pine runtime") ||
		!strings.Contains(message, "close strategy store") {
		t.Fatalf("close error lacks resource names: %v", err)
	}
	if strings.Index(message, "pine runtime") > strings.Index(message, "strategy store") {
		t.Fatalf("close errors do not follow reverse order: %v", err)
	}
}

func TestRegisterAfterShutdownClosesLateResourceImmediately(t *testing.T) {
	var resources Resources
	if err := resources.Close(); err != nil {
		t.Fatalf("initial close: %v", err)
	}
	var closeCalls int
	lateErr := errors.New("late close")
	err := resources.Register("late broker", func() error {
		closeCalls++
		return lateErr
	})
	if closeCalls != 1 {
		t.Fatalf("late resource close calls = %d, want 1", closeCalls)
	}
	if !errors.Is(err, ErrResourcesClosed) || !errors.Is(err, lateErr) {
		t.Fatalf("late registration error = %v", err)
	}
	if !strings.Contains(err.Error(), "close late broker") {
		t.Fatalf("late registration error lacks resource name: %v", err)
	}
}
