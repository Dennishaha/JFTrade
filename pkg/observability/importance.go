package observability

import (
	"strings"
	"sync/atomic"
)

const FieldImportance = "importance"

type Importance string

const (
	ImportanceLow      Importance = "low"
	ImportanceNormal   Importance = "normal"
	ImportanceHigh     Importance = "high"
	ImportanceCritical Importance = "critical"
)

var minimumImportanceRank atomic.Int64

func init() {
	minimumImportanceRank.Store(int64(importanceRank(ImportanceLow)))
}

func ParseImportance(value string) (Importance, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "debug", "trace":
		return ImportanceLow, true
	case "normal", "info", "default":
		return ImportanceNormal, true
	case "high", "warn", "warning", "error":
		return ImportanceHigh, true
	case "critical", "fatal", "panic":
		return ImportanceCritical, true
	default:
		return "", false
	}
}

func NormalizeImportance(value string) Importance {
	if importance, ok := ParseImportance(value); ok {
		return importance
	}
	return ImportanceNormal
}

func NormalizeMinimumImportance(value string) Importance {
	if importance, ok := ParseImportance(value); ok {
		return importance
	}
	return ImportanceLow
}

func SetMinimumImportance(importance Importance) {
	minimumImportanceRank.Store(int64(importanceRank(NormalizeMinimumImportance(string(importance)))))
}

func MinimumImportance() Importance {
	return importanceByRank(int(minimumImportanceRank.Load()))
}

func (importance Importance) String() string {
	if normalized, ok := ParseImportance(string(importance)); ok {
		return string(normalized)
	}
	return string(ImportanceNormal)
}

func (importance Importance) meets(minimum Importance) bool {
	return importanceRank(importance) >= importanceRank(minimum)
}

func importanceRank(importance Importance) int {
	switch NormalizeImportance(string(importance)) {
	case ImportanceLow:
		return 0
	case ImportanceNormal:
		return 1
	case ImportanceHigh:
		return 2
	case ImportanceCritical:
		return 3
	default:
		return 1
	}
}

func importanceByRank(rank int) Importance {
	switch rank {
	case 0:
		return ImportanceLow
	case 1:
		return ImportanceNormal
	case 2:
		return ImportanceHigh
	case 3:
		return ImportanceCritical
	default:
		return ImportanceLow
	}
}
