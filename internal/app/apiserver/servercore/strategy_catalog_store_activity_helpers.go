package servercore

import (
	"fmt"
	"strings"
	"time"
)

func buildStrategyRuntimeLogEntry(at time.Time, logMessage string) string {
	logMessage = strings.TrimSpace(logMessage)
	if logMessage == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", at.UTC().Format(time.RFC3339Nano), logMessage)
}

func strategyLogLevelForKind(kind string, logMessage string) string {
	switch strings.TrimSpace(kind) {
	case "runtime_error", "order_submit_failed", "runtime_exited":
		return "error"
	case "risk_rejected", "risk_monitor":
		return "warning"
	case "reconciled":
		return "warning"
	}
	message := strings.ToLower(strings.TrimSpace(logMessage))
	if strings.Contains(message, "error") || strings.Contains(message, "failed") || strings.Contains(message, "panic") {
		return "error"
	}
	return "info"
}

func sliceStrategyActivityPage[T any](items []T, limit int, offset int) ([]T, int, bool) {
	total := len(items)
	if offset >= total {
		return []T{}, total, false
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := append([]T(nil), items[offset:end]...)
	return page, total, end < total
}

func (s *strategyCatalogStore) runtimeSummary() map[string]any {
	strategies := s.strategies()
	activeCount := 0
	for _, strategy := range strategies {
		if strategy.Status == strategyStatusRunning || strategy.Status == strategyStatusPaused {
			activeCount++
		}
	}
	status := "idle"
	if activeCount > 0 {
		status = "active"
	}
	return map[string]any{
		"status":                 status,
		"activeStrategies":       activeCount,
		"supportsBacktestParity": true,
	}
}
