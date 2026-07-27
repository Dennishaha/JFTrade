package runtimeactivity

import (
	"context"
	"strings"
	"time"
)

const (
	DefaultPageSize = 50
	MaxPageSize     = 5000
)

type LogEvent struct {
	ID         int64
	InstanceID string
	At         time.Time
	Raw        string
	Level      string
	Source     string
}

type LogQuery struct {
	InstanceID string
	Limit      int
	Offset     int
	Level      string
	FromAt     *time.Time
	ToAt       *time.Time
}

type AuditEvent struct {
	ID         int64
	InstanceID string
	Kind       string
	Detail     string
	At         time.Time
}

type AuditQuery struct {
	InstanceID string
	Limit      int
	Offset     int
	Kind       string
	FromAt     *time.Time
	ToAt       *time.Time
}

type ObservationSnapshot struct {
	InstanceID        string
	ActualStatus      string
	ActiveSymbols     []string
	LastClosedKLineAt *time.Time
	LastSignalAt      *time.Time
	LastOrderAt       *time.Time
	LastErrorAt       *time.Time
	LastError         string
	UpdatedAt         *time.Time
}

// LogStore is the consumer-owned port for persisted strategy runtime logs.
type LogStore interface {
	AppendLog(context.Context, LogEvent) error
	ListLogs(context.Context, LogQuery) ([]LogEvent, error)
	CountLogs(context.Context, LogQuery) (int, error)
	ListRecentLogsTail(context.Context, string, int) ([]LogEvent, error)
}

// AuditStore is the consumer-owned port for persisted strategy runtime audit.
type AuditStore interface {
	AppendAudit(context.Context, AuditEvent) error
	ListAudit(context.Context, AuditQuery) ([]AuditEvent, error)
	CountAudit(context.Context, AuditQuery) (int, error)
}

// ObservationStore is the consumer-owned port for persisted runtime snapshots.
type ObservationStore interface {
	UpsertObservation(context.Context, ObservationSnapshot) error
	GetObservation(context.Context, string) (ObservationSnapshot, bool, error)
}

// Store combines the runtime activity capabilities used by catalog and live
// strategy services without exposing a database handle.
type Store interface {
	LogStore
	AuditStore
	ObservationStore
}

func NormalizePageSize(limit int) int {
	if limit <= 0 {
		return DefaultPageSize
	}
	if limit > MaxPageSize {
		return MaxPageSize
	}
	return limit
}

func NormalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func NormalizeLogQuery(query LogQuery) LogQuery {
	query.InstanceID = strings.TrimSpace(query.InstanceID)
	query.Level = strings.ToLower(strings.TrimSpace(query.Level))
	query.Limit = NormalizePageSize(query.Limit)
	query.Offset = NormalizeOffset(query.Offset)
	return query
}

func NormalizeAuditQuery(query AuditQuery) AuditQuery {
	query.InstanceID = strings.TrimSpace(query.InstanceID)
	query.Kind = strings.TrimSpace(query.Kind)
	query.Limit = NormalizePageSize(query.Limit)
	query.Offset = NormalizeOffset(query.Offset)
	return query
}
