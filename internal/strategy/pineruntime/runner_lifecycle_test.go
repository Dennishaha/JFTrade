package pineruntime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type fixedLiveTransportDialer struct {
	transport pineworker.ManagedTransport
}

func (dialer fixedLiveTransportDialer) Dial(
	ctx context.Context,
	_ string,
) (pineworker.ManagedTransport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return dialer.transport, nil
}

type closeRunnerBeforeRegistrationTransport struct {
	fakeTransport
	closeRunner func()
}

func (transport *closeRunnerBeforeRegistrationTransport) RunScript(
	ctx context.Context,
	request pineworker.RunScriptRequest,
) (pineworker.RunScriptResponse, error) {
	if request.SessionOperation == pineworker.SessionOperationOpen {
		transport.closeRunner()
	}
	return transport.fakeTransport.RunScript(ctx, request)
}

func TestLiveSessionContextWatcherExitsWhenSessionCloses(t *testing.T) {
	session := &liveSession{done: make(chan struct{})}
	watcherExited := make(chan struct{})
	go func() {
		session.closeWhenDone(context.Background())
		close(watcherExited)
	}()

	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-watcherExited:
	case <-time.After(time.Second):
		t.Fatal("context watcher remained blocked after session close")
	}
}

func TestOpenLiveSessionCleansUpWhenRunnerClosesBeforeRegistration(t *testing.T) {
	launcher := &fakeLauncher{}
	var runner *ephemeralRunner
	transport := &closeRunnerBeforeRegistrationTransport{
		closeRunner: func() {
			runner.mu.Lock()
			runner.closed = true
			runner.mu.Unlock()
		},
	}
	deps := applyOptions([]Option{
		WithLauncherFactory(func(Config, []byte) (pineworker.WorkerLauncher, error) {
			return launcher, nil
		}),
		WithDialerFactory(func(int) pineworker.TransportDialer {
			return fixedLiveTransportDialer{transport: transport}
		}),
	})
	var err error
	runner, err = newEphemeralRunner(
		Config{
			bundleData: []byte("worker"), Host: "127.0.0.1",
			HealthTimeout: time.Second, RequestTimeout: time.Second,
		},
		runnerInstance,
		deps,
	)
	if err != nil {
		t.Fatalf("newEphemeralRunner: %v", err)
	}

	session, _, err := runner.OpenLiveSession(context.Background(), validRequest("close-before-register"))

	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("OpenLiveSession error = %v, want closed runner", err)
	}
	if session != nil {
		t.Fatalf("OpenLiveSession returned session after registration failed: %#v", session)
	}
	if len(runner.busy) != 0 {
		t.Fatal("registration failure did not release runner capacity")
	}
	if launcher.startedCount() != 1 || launcher.stoppedCount() != 1 {
		t.Fatalf(
			"registration failure worker lifecycle = started %d stopped %d",
			launcher.startedCount(),
			launcher.stoppedCount(),
		)
	}
}

func TestLiveSessionContextWatcherCancellationBoundaries(t *testing.T) {
	t.Run("nil runner", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		session := &liveSession{}

		session.closeWhenDone(ctx)

		session.mu.Lock()
		done := session.done
		session.mu.Unlock()
		if done == nil {
			t.Fatal("context watcher did not initialize its session completion signal")
		}
	})

	t.Run("owned session", func(t *testing.T) {
		runner := &ephemeralRunner{
			config:   Config{RequestTimeout: time.Second},
			busy:     make(chan struct{}, 1),
			sessions: make(map[*liveSession]struct{}),
		}
		runner.busy <- struct{}{}
		session := &liveSession{runner: runner}
		runner.sessions[session] = struct{}{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		session.closeWhenDone(ctx)

		if !session.closed {
			t.Fatal("context cancellation did not close the owned session")
		}
		select {
		case <-session.done:
		default:
			t.Fatal("owned session completion signal remained open")
		}
		if len(runner.sessions) != 0 {
			t.Fatalf("closed session remained registered: %d", len(runner.sessions))
		}
		if len(runner.busy) != 0 {
			t.Fatal("closed session did not release runner capacity")
		}
	})
}

func TestLiveSessionCloseInitializesCompletionSignal(t *testing.T) {
	session := &liveSession{}

	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-session.done:
	default:
		t.Fatal("Close did not publish session completion")
	}
}
