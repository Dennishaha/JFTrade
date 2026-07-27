package catalog

import (
	"fmt"
	"strings"
	"time"
)

func buildRuntimeLogEntry(at time.Time, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", at.UTC().Format(time.RFC3339Nano), message)
}

func logLevelForKind(kind string, message string) string {
	switch strings.TrimSpace(kind) {
	case "runtime_error", "order_submit_failed", "runtime_exited":
		return "error"
	case "risk_rejected", "risk_monitor", "reconciled":
		return "warning"
	}
	message = strings.ToLower(strings.TrimSpace(message))
	if strings.Contains(message, "error") || strings.Contains(message, "failed") || strings.Contains(message, "panic") {
		return "error"
	}
	return "info"
}
