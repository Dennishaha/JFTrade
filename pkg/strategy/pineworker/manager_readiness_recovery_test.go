package pineworker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// A worker process can be listening while its health endpoint is still
// unavailable. These startup paths ensure that the manager closes the stale
// transport and returns the actual readiness/cancellation reason instead of
// exposing a partly initialized worker.
func TestWorkerManagerReadinessFailuresCloseTransportAndRespectCancellation(t *testing.T) {
	t.Run("unhealthy transport is closed and reports readiness failure", func(t *testing.T) {
		transport := &fakeManagedTransport{healthy: false, status: HealthStatus{OK: false}}
		manager := &WorkerManager{
			config: ManagerConfig{HealthTimeout: time.Nanosecond},
			dialer: managerReadinessDialer{transport: transport},
		}

		_, _, err := manager.dialWorkerUntilReady(t.Context(), "127.0.0.1:50051")
		if err == nil || !strings.Contains(err.Error(), "worker unhealthy") {
			t.Fatalf("dialWorkerUntilReady unhealthy error = %v", err)
		}
		if transport.closes != 1 {
			t.Fatalf("unhealthy transport closes = %d, want 1", transport.closes)
		}
	})

	t.Run("startup returns caller cancellation before another retry", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		manager := &WorkerManager{
			config: ManagerConfig{HealthTimeout: time.Second},
			dialer: managerReadinessDialer{err: errors.New("worker socket unavailable")},
		}

		_, _, err := manager.dialWorkerUntilReady(ctx, "127.0.0.1:50051")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("dialWorkerUntilReady canceled error = %v, want context.Canceled", err)
		}
	})
}

type managerReadinessDialer struct {
	transport ManagedTransport
	err       error
}

func (d managerReadinessDialer) Dial(context.Context, string) (ManagedTransport, error) {
	if d.err != nil {
		return nil, d.err
	}
	return d.transport, nil
}
