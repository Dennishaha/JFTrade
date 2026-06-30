package observability

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultEventLimit    = 20
	defaultSlowThreshold = 750 * time.Millisecond
	maxSummaryTextBytes  = 500
)

type Event struct {
	At         string `json:"at"`
	Level      string `json:"level"`
	Importance string `json:"importance"`
	Message    string `json:"message"`
	Error      string `json:"error,omitempty"`
	Method     string `json:"method,omitempty"`
	Path       string `json:"path,omitempty"`
	Operation  string `json:"operation,omitempty"`
	Status     int    `json:"status,omitempty"`
	LatencyMS  int64  `json:"latencyMs,omitempty"`
	Fields
}

type OpenDHealth struct {
	TotalCalls    uint64 `json:"totalCalls"`
	FailedCalls   uint64 `json:"failedCalls"`
	LastCallAt    string `json:"lastCallAt,omitempty"`
	LastSuccessAt string `json:"lastSuccessAt,omitempty"`
	LastErrorAt   string `json:"lastErrorAt,omitempty"`
	LastError     string `json:"lastError,omitempty"`
	LastOperation string `json:"lastOperation,omitempty"`
	LastRequestID string `json:"lastRequestId,omitempty"`
}

type Snapshot struct {
	RecentErrors       []Event     `json:"recentErrors"`
	RecentSlowRequests []Event     `json:"recentSlowRequests"`
	OpenD              OpenDHealth `json:"openD"`
	SlowThresholdMS    int64       `json:"slowThresholdMs"`
	MinimumImportance  string      `json:"minimumImportance"`
}

type RecorderConfig struct {
	EventLimit        int
	SlowThreshold     time.Duration
	MinimumImportance Importance
}

type Recorder struct {
	mu                sync.RWMutex
	limit             int
	slowThreshold     time.Duration
	minimumImportance Importance
	errors            []Event
	slowRequests      []Event
	openD             OpenDHealth
}

func NewRecorder(limit int, slowThreshold time.Duration) *Recorder {
	return NewRecorderWithConfig(RecorderConfig{
		EventLimit:        limit,
		SlowThreshold:     slowThreshold,
		MinimumImportance: ImportanceLow,
	})
}

func NewRecorderWithConfig(config RecorderConfig) *Recorder {
	if config.EventLimit <= 0 {
		config.EventLimit = defaultEventLimit
	}
	if config.SlowThreshold <= 0 {
		config.SlowThreshold = defaultSlowThreshold
	}
	minimumImportance := NormalizeMinimumImportance(string(config.MinimumImportance))
	return &Recorder{
		limit:             config.EventLimit,
		slowThreshold:     config.SlowThreshold,
		minimumImportance: minimumImportance,
	}
}

func (r *Recorder) Accepts(importance Importance) bool {
	if r == nil {
		return true
	}
	r.mu.RLock()
	minimumImportance := r.minimumImportance
	r.mu.RUnlock()
	return NormalizeImportance(string(importance)).meets(minimumImportance)
}

func (r *Recorder) RecordHTTPRequest(ctx context.Context, method string, path string, status int, latency time.Duration) {
	if r == nil {
		return
	}
	event := newEvent("info", "api request", FieldsFromContext(ctx), nil, ImportanceLow)
	event.Method = strings.TrimSpace(method)
	event.Path = strings.TrimSpace(path)
	event.Status = status
	event.LatencyMS = latency.Milliseconds()

	r.mu.Lock()
	defer r.mu.Unlock()
	if latency >= r.slowThreshold && eventImportance(event).meets(r.minimumImportance) {
		r.slowRequests = prependBounded(r.slowRequests, event, r.limit)
	}
	if status >= 500 {
		event.Level = "error"
		event.Importance = ImportanceHigh.String()
		event.Message = "api request failed"
		event.Error = fmt.Sprintf("HTTP %d", status)
		if eventImportance(event).meets(r.minimumImportance) {
			r.errors = prependBounded(r.errors, event, r.limit)
		}
	}
}

func (r *Recorder) RecordOpenDCall(ctx context.Context, operation string, latency time.Duration, err error) {
	fields := FieldsFromContext(ctx)
	fields.Source = "opend"
	ctx = WithFields(ctx, fields)
	if err != nil {
		ErrorWithImportance(ctx, ImportanceHigh, "opend call failed", err, "operation", operation, "latency_ms", latency.Milliseconds())
	}
	if r == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.openD.TotalCalls++
	r.openD.LastCallAt = now
	r.openD.LastOperation = sanitizeSummaryText(operation)
	r.openD.LastRequestID = fields.RequestID
	if err == nil {
		r.openD.LastSuccessAt = now
		return
	}
	r.openD.FailedCalls++
	r.openD.LastErrorAt = now
	r.openD.LastError = sanitizeSummaryText(err.Error())
}

func RecordOpenDCall(ctx context.Context, operation string, latency time.Duration, err error) {
	if recorder := RecorderFromContext(ctx); recorder != nil {
		recorder.RecordOpenDCall(ctx, operation, latency, err)
		return
	}
	if err != nil {
		ctx = WithFields(ctx, Fields{Source: "opend"})
		ErrorWithImportance(ctx, ImportanceHigh, "opend call failed", err, "operation", operation, "latency_ms", latency.Milliseconds())
	}
}

func (r *Recorder) Snapshot() Snapshot {
	if r == nil {
		return Snapshot{
			RecentErrors:       []Event{},
			RecentSlowRequests: []Event{},
			MinimumImportance:  MinimumImportance().String(),
		}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return Snapshot{
		RecentErrors:       append([]Event(nil), r.errors...),
		RecentSlowRequests: append([]Event(nil), r.slowRequests...),
		OpenD:              r.openD,
		SlowThresholdMS:    r.slowThreshold.Milliseconds(),
		MinimumImportance:  r.minimumImportance.String(),
	}
}

func (r *Recorder) recordError(event Event) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.errors = prependBounded(r.errors, event, r.limit)
	r.mu.Unlock()
}

func newEvent(level string, message string, fields Fields, err error, importance Importance) Event {
	event := Event{
		At:         time.Now().UTC().Format(time.RFC3339Nano),
		Level:      level,
		Importance: NormalizeImportance(string(importance)).String(),
		Message:    sanitizeSummaryText(message),
		Fields:     fields,
	}
	if err != nil {
		event.Error = sanitizeSummaryText(err.Error())
	}
	return event
}

func prependBounded(events []Event, event Event, limit int) []Event {
	events = append(events, Event{})
	copy(events[1:], events[:len(events)-1])
	events[0] = event
	if len(events) > limit {
		events = events[:limit]
	}
	return events
}

func eventImportance(event Event) Importance {
	return NormalizeImportance(event.Importance)
}

func sanitizeSummaryText(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) <= maxSummaryTextBytes {
		return value
	}
	return value[:maxSummaryTextBytes] + "..."
}
