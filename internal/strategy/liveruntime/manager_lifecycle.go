package liveruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

// Stop implements strategy.RuntimeManager.
func (m *Manager) Stop(instanceID string) {
	m.stopStrategy(instanceID)
}

func (m *Manager) stopStrategy(instanceID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	runtime, exists := m.runtimes[instanceID]
	if exists {
		delete(m.runtimes, instanceID)
	}
	m.mu.Unlock()
	if exists {
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusStopped))
	}
	if exists {
		if err := runtime.close(context.Background()); err != nil {
			besteffort.LogError(err)
		}
	}
	if exists {
		m.wakeMarketDataCollector()
	}
}

// Close stops all active and in-flight starts exactly once. Every Pine session
// close failure is returned with its instance and symbol name.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		runtimes := make([]*managedRuntime, 0, len(m.runtimes))
		for instanceID, runtime := range m.runtimes {
			delete(m.runtimes, instanceID)
			runtimes = append(runtimes, runtime)
		}
		m.mu.Unlock()

		closeErrors := make([]error, 0)
		for _, runtime := range runtimes {
			m.persistObservationSnapshot(runtime.snapshot(strategyStatusStopped))
			closeErrors = append(closeErrors, runtime.close(context.Background()))
		}
		m.startWG.Wait()
		closeErrors = append(closeErrors, m.takeCloseErrors()...)
		m.closeErr = errors.Join(closeErrors...)
		if len(runtimes) > 0 {
			m.wakeMarketDataCollector()
		}
	})
	return m.closeErr
}

func (m *Manager) recordCloseError(err error) {
	if err == nil {
		return
	}
	m.closeErrorsMu.Lock()
	m.closeErrors = append(m.closeErrors, err)
	m.closeErrorsMu.Unlock()
}

func (m *Manager) takeCloseErrors() []error {
	m.closeErrorsMu.Lock()
	defer m.closeErrorsMu.Unlock()
	result := append([]error(nil), m.closeErrors...)
	m.closeErrors = nil
	return result
}

func (runtime *managedRuntime) close(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	runtime.closeOnce.Do(func() {
		if runtime.cancel != nil {
			runtime.cancel()
		}
		runtime.backgroundWG.Wait()
		symbols := strategyRuntimeSortedSymbols(runtime.symbols)
		closeErrors := make([]error, 0, len(symbols))
		for _, symbol := range symbols {
			runner := runtime.symbols[symbol]
			if runner == nil || runner.pineWorkerLive == nil {
				continue
			}
			if err := runner.pineWorkerLive.closeSession(ctx); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf(
					"strategy runtime %s symbol %s pine session close: %w",
					runtime.instanceID,
					symbol,
					err,
				))
			}
		}
		if runtime.subscriptionLease != nil {
			runtime.subscriptionLease.Release()
		}
		runtime.closeErr = errors.Join(closeErrors...)
	})
	return runtime.closeErr
}

func (runtime *managedRuntime) startBackgroundLoops() {
	if runtime == nil {
		return
	}
	runners := make([]*symbolRuntime, 0, len(runtime.symbols))
	for _, runner := range runtime.symbols {
		if runner != nil && runner.closedKLineSyncInterval > 0 {
			runners = append(runners, runner)
		}
	}
	runtime.backgroundWG.Add(len(runners))
	for _, runner := range runners {
		go func() {
			defer runtime.backgroundWG.Done()
			runner.syncClosedKLinesLoop()
		}()
	}
}

func strategyKLineSubscriptionRefs(symbols []string, interval bbgotypes.Interval) []mdsrv.InstrumentRef {
	refs := make([]mdsrv.InstrumentRef, 0, len(symbols))
	for _, raw := range symbols {
		parts := strings.SplitN(strings.ToUpper(strings.TrimSpace(raw)), ".", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		refs = append(refs, mdsrv.InstrumentRef{
			Channel: "KLINE", Market: parts[0], Symbol: parts[1], Interval: string(interval),
		})
	}
	return refs
}
