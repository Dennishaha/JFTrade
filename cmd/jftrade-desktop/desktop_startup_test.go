package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver"
	"github.com/jftrade/jftrade-main/internal/live"
)

func TestDesktopStartupWindowStatePrecedesAPIStartup(t *testing.T) {
	t.Parallel()
	state := newDesktopAppState(func() {})
	startup := newDesktopStartupService(state, time.Unix(100, 0))

	snapshot := startup.Snapshot()
	if snapshot.State != desktopStartupStateStarting || snapshot.Phase != desktopStartupPhaseNativeReady {
		t.Fatalf("initial snapshot = %+v", snapshot)
	}
	if state.startupStarted.Load() {
		t.Fatal("API startup began before the application-started callback")
	}
}

func TestDesktopStartupPublishesReadyAndClosesResources(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	state := newDesktopAppState(cancel)
	startup := newDesktopStartupService(state, time.Now())
	entered := make(chan struct{})
	release := make(chan struct{})
	var shutdownCalls atomic.Int32

	startDesktopAPIAsyncWith(ctx, state, startup, nil, apiserver.DesktopRuntimeConfig{}, func(
		context.Context,
		apiserver.DesktopRuntimeConfig,
		func(live.Event) live.NotificationDelivery,
	) (func(context.Context) error, error) {
		close(entered)
		<-release
		return func(context.Context) error {
			shutdownCalls.Add(1)
			return nil
		}, nil
	})

	waitDesktopStartupTestSignal(t, entered, "API starter")
	if snapshot := startup.Snapshot(); snapshot.Phase != desktopStartupPhaseAPIStarting {
		t.Fatalf("starting snapshot = %+v", snapshot)
	}
	close(release)
	waitDesktopStartupTestSignal(t, state.startupDone, "API startup completion")
	if snapshot := startup.Snapshot(); snapshot.State != desktopStartupStateReady {
		t.Fatalf("ready snapshot = %+v", snapshot)
	}

	state.shutdownApp()
	if got := shutdownCalls.Load(); got != 1 {
		t.Fatalf("shutdown calls = %d, want 1", got)
	}
}

func TestDesktopStartupFailureUsesSafeMessage(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	state := newDesktopAppState(cancel)
	startup := newDesktopStartupService(state, time.Now())
	secret := "provider token abc123"

	startDesktopAPIAsyncWith(ctx, state, startup, nil, apiserver.DesktopRuntimeConfig{}, func(
		context.Context,
		apiserver.DesktopRuntimeConfig,
		func(live.Event) live.NotificationDelivery,
	) (func(context.Context) error, error) {
		return nil, errors.New(secret)
	})
	waitDesktopStartupTestSignal(t, state.startupDone, "failed API startup")

	snapshot := startup.Snapshot()
	if snapshot.State != desktopStartupStateFailed || snapshot.Phase != desktopStartupPhaseAPIFailed {
		t.Fatalf("failed snapshot = %+v", snapshot)
	}
	if snapshot.Message == secret {
		t.Fatal("startup snapshot exposed the internal error")
	}
	state.shutdownApp()
}

func TestDesktopShutdownCancelsStartupAndReclaimsLateResources(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	state := newDesktopAppState(cancel)
	startup := newDesktopStartupService(state, time.Now())
	entered := make(chan struct{})
	var shutdownCalls atomic.Int32

	startDesktopAPIAsyncWith(ctx, state, startup, nil, apiserver.DesktopRuntimeConfig{}, func(
		startCtx context.Context,
		_ apiserver.DesktopRuntimeConfig,
		_ func(live.Event) live.NotificationDelivery,
	) (func(context.Context) error, error) {
		close(entered)
		<-startCtx.Done()
		return func(context.Context) error {
			shutdownCalls.Add(1)
			return nil
		}, nil
	})
	waitDesktopStartupTestSignal(t, entered, "API starter")

	state.shutdownApp()
	if got := shutdownCalls.Load(); got != 1 {
		t.Fatalf("late resource shutdown calls = %d, want 1", got)
	}
	if snapshot := startup.Snapshot(); snapshot.State == desktopStartupStateReady {
		t.Fatalf("shutdown startup must not publish ready: %+v", snapshot)
	}
}

func TestDesktopShutdownIsIdempotent(t *testing.T) {
	t.Parallel()
	state := newDesktopAppState(func() {})
	var shutdownCalls atomic.Int32
	state.shutdown = func(context.Context) error {
		shutdownCalls.Add(1)
		return nil
	}

	var callers sync.WaitGroup
	callers.Add(2)
	go func() {
		defer callers.Done()
		state.shutdownApp()
	}()
	go func() {
		defer callers.Done()
		state.shutdownApp()
	}()
	callers.Wait()

	if got := shutdownCalls.Load(); got != 1 {
		t.Fatalf("shutdown calls = %d, want 1", got)
	}
}

func waitDesktopStartupTestSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
