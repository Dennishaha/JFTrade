package liveruntime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/strategy/pineruntime"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type closeTestSession struct {
	err   error
	count atomic.Int32
}

func (session *closeTestSession) Append(
	context.Context,
	pineworker.RunScriptRequest,
) (pineworker.RunScriptResponse, error) {
	return pineworker.RunScriptResponse{}, nil
}

func (session *closeTestSession) Close(context.Context) error {
	session.count.Add(1)
	return session.err
}

type closeTestLease struct {
	count atomic.Int32
}

func (lease *closeTestLease) Release() {
	lease.count.Add(1)
}

func TestManagerCloseAggregatesNamedSessionErrorsOnce(t *testing.T) {
	t.Parallel()

	firstSession := &closeTestSession{err: errors.New("first close failed")}
	secondSession := &closeTestSession{err: errors.New("second close failed")}
	lease := &closeTestLease{}
	manager := NewManager(Dependencies{})
	manager.runtimes["instance-a"] = closeTestRuntime(
		"instance-a",
		lease,
		map[string]pineruntime.LiveSession{
			"US.AAPL": firstSession,
			"US.MSFT": secondSession,
		},
	)

	const callers = 12
	results := make(chan error, callers)
	var callersWG sync.WaitGroup
	for range callers {
		callersWG.Add(1)
		go func() {
			defer callersWG.Done()
			results <- manager.Close()
		}()
	}
	callersWG.Wait()
	close(results)

	for err := range results {
		if err == nil {
			t.Fatal("Close() error = nil, want aggregated session errors")
		}
		message := err.Error()
		for _, expected := range []string{
			"strategy runtime instance-a symbol US.AAPL pine session close",
			"first close failed",
			"strategy runtime instance-a symbol US.MSFT pine session close",
			"second close failed",
		} {
			if !strings.Contains(message, expected) {
				t.Fatalf("Close() error %q does not contain %q", message, expected)
			}
		}
	}
	if got := firstSession.count.Load(); got != 1 {
		t.Fatalf("first session close count = %d, want 1", got)
	}
	if got := secondSession.count.Load(); got != 1 {
		t.Fatalf("second session close count = %d, want 1", got)
	}
	if got := lease.count.Load(); got != 1 {
		t.Fatalf("subscription release count = %d, want 1", got)
	}
}

func TestManagerCloseWaitsForInFlightStartAndCollectsItsCloseError(t *testing.T) {
	t.Parallel()

	manager := NewManager(Dependencies{})
	releaseStart, err := manager.reserveRuntimeStart("instance-race")
	if err != nil {
		t.Fatalf("reserveRuntimeStart() error = %v", err)
	}

	closeResult := make(chan error, 1)
	go func() {
		closeResult <- manager.Close()
	}()
	waitForManagerClosed(t, manager)

	session := &closeTestSession{err: errors.New("racing session close failed")}
	activationErr := manager.activateStrategyRuntime(
		"instance-race",
		closeTestRuntime(
			"instance-race",
			&closeTestLease{},
			map[string]pineruntime.LiveSession{"HK.00700": session},
		),
	)
	if !errors.Is(activationErr, ErrClosed) {
		t.Fatalf("activateStrategyRuntime() error = %v, want ErrClosed", activationErr)
	}

	select {
	case err := <-closeResult:
		t.Fatalf("Close() returned before start reservation release: %v", err)
	default:
	}
	releaseStart()

	select {
	case closeErr := <-closeResult:
		if closeErr == nil || !strings.Contains(closeErr.Error(), "instance-race symbol HK.00700") {
			t.Fatalf("Close() error = %v, want named racing session error", closeErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not finish after start reservation release")
	}
	if got := session.count.Load(); got != 1 {
		t.Fatalf("racing session close count = %d, want 1", got)
	}
}

func closeTestRuntime(
	instanceID string,
	lease SubscriptionLease,
	sessions map[string]pineruntime.LiveSession,
) *managedRuntime {
	runtime := &managedRuntime{
		instanceID:        instanceID,
		cancel:            func() {},
		symbols:           make(map[string]*symbolRuntime, len(sessions)),
		subscriptionLease: lease,
	}
	for symbol, session := range sessions {
		runtime.symbols[symbol] = &symbolRuntime{
			symbol:         symbol,
			pineWorkerLive: &pineWorkerLive{session: session},
		}
	}
	return runtime
}

func waitForManagerClosed(t *testing.T, manager *Manager) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		manager.mu.RLock()
		closed := manager.closed
		manager.mu.RUnlock()
		if closed {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("manager did not enter closed state")
		}
		time.Sleep(time.Millisecond)
	}
}
