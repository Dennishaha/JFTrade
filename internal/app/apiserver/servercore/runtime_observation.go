package servercore

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	"github.com/jftrade/jftrade-main/internal/strategy/runtimecontrol"
)

func (m *strategyRuntimeManager) runtimeObservation(instanceID string) (strategyRuntimeObservation, bool) {
	runtime := m.runtime(instanceID)
	if runtime == nil {
		return strategyRuntimeObservation{}, false
	}
	return runtime.observation(), true
}

func (m *strategyRuntimeManager) runtimeSummary() map[string]any {
	summary := m.typedRuntimeSummary()
	return map[string]any{
		"status":                 summary.Status,
		"activeStrategies":       summary.ActiveStrategies,
		"supportsBacktestParity": summary.SupportsBacktestParity,
		"activeInstances":        summary.ActiveInstances,
	}
}

func (m *strategyRuntimeManager) typedRuntimeSummary() stratsrv.RuntimeSummary {
	m.mu.RLock()
	runtimes := make([]*managedStrategyRuntime, 0, len(m.runtimes))
	for _, runtime := range m.runtimes {
		runtimes = append(runtimes, runtime)
	}
	m.mu.RUnlock()

	activeInstances := make([]strategyRuntimeActiveInstanceSummary, 0, len(runtimes))
	for _, runtime := range runtimes {
		observation := runtime.observation()
		activeInstances = append(activeInstances, strategyRuntimeActiveInstanceSummary{
			InstanceID:        runtime.instanceID,
			DefinitionName:    strings.TrimSpace(runtime.definition.Name),
			ActualStatus:      observation.ActualStatus,
			ActiveSymbols:     observation.ActiveSymbols,
			LastClosedKLineAt: observation.LastClosedKLineAt,
			LastSignalAt:      observation.LastSignalAt,
			LastOrderAt:       observation.LastOrderAt,
			LastErrorAt:       observation.LastErrorAt,
			LastError:         observation.LastError,
			UpdatedAt:         observation.UpdatedAt,
		})
	}
	sort.Slice(activeInstances, func(i int, j int) bool {
		return activeInstances[i].InstanceID < activeInstances[j].InstanceID
	})

	status := "idle"
	if len(activeInstances) > 0 {
		status = "active"
	}
	return stratsrv.RuntimeSummary{
		Status:                 status,
		ActiveStrategies:       len(activeInstances),
		SupportsBacktestParity: true,
		ActiveInstances:        activeInstances,
	}
}

func (m *strategyRuntimeManager) runtime(instanceID string) *managedStrategyRuntime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runtimes[instanceID]
}

func (m *strategyRuntimeManager) recordClosedKLine(instanceID string, at time.Time) {
	if runtime := m.runtime(instanceID); runtime != nil {
		runtime.recordClosedKLine(at)
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusRunning))
	}
}

func (m *strategyRuntimeManager) recordSignal(instanceID string, at time.Time) {
	if runtime := m.runtime(instanceID); runtime != nil {
		runtime.recordSignal(at)
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusRunning))
	}
}

func (m *strategyRuntimeManager) recordOrder(instanceID string, at time.Time) {
	if runtime := m.runtime(instanceID); runtime != nil {
		runtime.recordOrder(at)
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusRunning))
	}
}

func (m *strategyRuntimeManager) recordError(instanceID string, message string, at time.Time) {
	if runtime := m.runtime(instanceID); runtime != nil {
		runtime.recordError(message, at)
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusRunning))
	}
}

func (m *strategyRuntimeManager) handleRuntimePanic(instanceID string, symbol string, recovered any) {
	detail := fmt.Sprintf("strategy runtime panic on %s: %v", symbol, recovered)
	m.recordError(instanceID, detail, time.Now().UTC())
	m.stopStrategy(instanceID)
	jftradeErr1 := m.reconcileRuntimeFailure(instanceID, detail)
	jftradeLogError(jftradeErr1)
	m.recordNotification(strategyRuntimeNotification{
		At:       time.Now().UTC().Format(time.RFC3339Nano),
		Level:    "error",
		Title:    "策略运行异常退出",
		Message:  detail,
		Source:   "strategy.runtime",
		Category: "strategy.runtime.exit",
	})
	m.wakeMarketDataCollector()
}

func (runtime *managedStrategyRuntime) observation() strategyRuntimeObservation {
	snapshot := runtime.snapshot(strategyStatusRunning)
	return strategyRuntimeObservationFromSnapshot(snapshot, strategyStatusRunning)
}

func (runtime *managedStrategyRuntime) snapshot(actualStatus string) runtimeactivity.ObservationSnapshot {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return runtimeactivity.ObservationSnapshot{
		InstanceID:        runtime.instanceID,
		ActualStatus:      strings.TrimSpace(actualStatus),
		ActiveSymbols:     strategyRuntimeSortedSymbols(runtime.symbols),
		LastClosedKLineAt: strategyRuntimeOptionalTime(runtime.lastClosedKLineAt),
		LastSignalAt:      strategyRuntimeOptionalTime(runtime.lastSignalAt),
		LastOrderAt:       strategyRuntimeOptionalTime(runtime.lastOrderAt),
		LastErrorAt:       strategyRuntimeOptionalTime(runtime.lastErrorAt),
		LastError:         strings.TrimSpace(runtime.lastError),
		UpdatedAt:         strategyRuntimeOptionalTime(runtime.updatedAt),
	}
}

func (runtime *managedStrategyRuntime) recordClosedKLine(at time.Time) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if at.After(runtime.lastClosedKLineAt) {
		runtime.lastClosedKLineAt = at
	}
	runtime.updatedAt = strategyRuntimeMaxTime(runtime.updatedAt, at)
}

func (runtime *managedStrategyRuntime) recordSignal(at time.Time) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if at.After(runtime.lastSignalAt) {
		runtime.lastSignalAt = at
	}
	runtime.updatedAt = strategyRuntimeMaxTime(runtime.updatedAt, at)
}

func (runtime *managedStrategyRuntime) recordOrder(at time.Time) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if at.After(runtime.lastOrderAt) {
		runtime.lastOrderAt = at
	}
	runtime.updatedAt = strategyRuntimeMaxTime(runtime.updatedAt, at)
}

func (runtime *managedStrategyRuntime) recordError(message string, at time.Time) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if at.After(runtime.lastErrorAt) {
		runtime.lastErrorAt = at
	}
	runtime.lastError = strings.TrimSpace(message)
	runtime.updatedAt = strategyRuntimeMaxTime(runtime.updatedAt, at)
}

func (m *strategyRuntimeManager) persistObservationSnapshot(snapshot runtimeactivity.ObservationSnapshot) {
	if m == nil || m.deps.upsertObservation == nil {
		return
	}
	if err := m.deps.upsertObservation(context.Background(), snapshot); err != nil {
		log.Printf("JFTrade persist strategy runtime observation degraded: %v", err)
	}
}

func strategyRuntimeObservationFromSnapshot(snapshot runtimeactivity.ObservationSnapshot, actualStatus string) strategyRuntimeObservation {
	observation := runtimecontrol.ObservationFromSnapshot(snapshot, actualStatus, strategyStatusStopped)
	return strategyRuntimeObservation{
		ActualStatus:      observation.ActualStatus,
		ActiveSymbols:     observation.ActiveSymbols,
		LastClosedKLineAt: observation.LastClosedKLineAt,
		LastSignalAt:      observation.LastSignalAt,
		LastOrderAt:       observation.LastOrderAt,
		LastErrorAt:       observation.LastErrorAt,
		LastError:         observation.LastError,
		UpdatedAt:         observation.UpdatedAt,
	}
}

func strategyRuntimeOptionalTime(value time.Time) *time.Time {
	return runtimecontrol.OptionalTime(value)
}
