package runtimecontrol

import (
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
)

type Observation struct {
	ActualStatus      string
	ActiveSymbols     []string
	LastClosedKLineAt *string
	LastSignalAt      *string
	LastOrderAt       *string
	LastErrorAt       *string
	LastError         *string
	UpdatedAt         *string
}

func ObservationFromSnapshot(snapshot runtimeactivity.ObservationSnapshot, actualStatus string, stoppedStatus string) Observation {
	status := strings.TrimSpace(actualStatus)
	if status == "" {
		status = strings.TrimSpace(snapshot.ActualStatus)
	}
	if status == "" {
		status = strings.TrimSpace(stoppedStatus)
	}
	return Observation{
		ActualStatus:      status,
		ActiveSymbols:     append([]string(nil), snapshot.ActiveSymbols...),
		LastClosedKLineAt: TimePointerToString(snapshot.LastClosedKLineAt),
		LastSignalAt:      TimePointerToString(snapshot.LastSignalAt),
		LastOrderAt:       TimePointerToString(snapshot.LastOrderAt),
		LastErrorAt:       TimePointerToString(snapshot.LastErrorAt),
		LastError:         OptionalString(snapshot.LastError),
		UpdatedAt:         TimePointerToString(snapshot.UpdatedAt),
	}
}

func ObservationTime(value time.Time) *time.Time {
	return OptionalTime(value)
}
