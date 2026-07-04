package servercore

import runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"

const (
	defaultStrategyRuntimeDBFilename = runtimeactivity.DefaultDBFilename

	strategyRuntimeLogTable         = runtimeactivity.LogTable
	strategyRuntimeAuditTable       = runtimeactivity.AuditTable
	strategyRuntimeObservationTable = runtimeactivity.ObservationTable

	defaultStrategyRuntimePageSize = runtimeactivity.DefaultPageSize
	maxStrategyRuntimePageSize     = runtimeactivity.MaxPageSize
)

type strategyRuntimeStore = runtimeactivity.Store
type strategyRuntimeLogEvent = runtimeactivity.LogEvent
type strategyRuntimeLogQuery = runtimeactivity.LogQuery
type strategyRuntimeAuditEvent = runtimeactivity.AuditEvent
type strategyRuntimeAuditQuery = runtimeactivity.AuditQuery
type strategyRuntimeObservationSnapshot = runtimeactivity.ObservationSnapshot

func NewStrategyRuntimeStore(dbPath string) (*strategyRuntimeStore, error) {
	return runtimeactivity.NewStore(dbPath)
}

func deriveStrategyRuntimeDBPath(settingsPath string) string {
	return runtimeactivity.DeriveDBPath(settingsPath)
}

func normalizeStrategyRuntimePageSize(limit int) int {
	return runtimeactivity.NormalizePageSize(limit)
}

func normalizeStrategyRuntimeOffset(offset int) int {
	return runtimeactivity.NormalizeOffset(offset)
}
