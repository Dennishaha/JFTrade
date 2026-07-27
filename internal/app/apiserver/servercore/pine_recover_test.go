package servercore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestStartPineWorkerManagersRejectsInvalidRuntimeConfiguration(t *testing.T) {
	t.Setenv(envPineWorkerBundle, filepath.Join(t.TempDir(), "worker.mjs"))
	t.Setenv(envPineWorkerRequestTimeout, "not-a-duration")

	backtest, instance := (&Server{}).startPineWorkerManagers()
	if backtest != nil || instance != nil {
		t.Fatalf("runners = %#v/%#v, want both disabled for invalid runtime configuration", backtest, instance)
	}
}

func TestStartPineWorkerManagersDoesNotPublishPartialRunnerSet(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "worker.mjs")
	if err := os.WriteFile(bundlePath, []byte("worker"), 0o600); err != nil {
		t.Fatalf("write worker bundle: %v", err)
	}
	t.Setenv(envPineWorkerBundle, bundlePath)

	for _, failCall := range []int{1, 2} {
		t.Run(string(rune('0'+failCall)), func(t *testing.T) {
			previous := newPineWorkerLauncher
			calls := 0
			newPineWorkerLauncher = func(pineWorkerRuntimeConfig, []byte) (pineworker.WorkerLauncher, error) {
				calls++
				if calls == failCall {
					return nil, errors.New("launcher unavailable")
				}
				return &fakeServerPineWorkerLauncher{}, nil
			}
			t.Cleanup(func() { newPineWorkerLauncher = previous })

			backtest, instance := (&Server{}).startPineWorkerManagers()
			if backtest != nil || instance != nil {
				t.Fatalf("runners = %#v/%#v, want atomic startup failure", backtest, instance)
			}
			if calls != failCall {
				t.Fatalf("launcher factory calls = %d, want %d", calls, failCall)
			}
		})
	}
}
