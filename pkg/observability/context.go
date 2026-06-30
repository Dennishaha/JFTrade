// Package observability standardizes correlation fields used across JFTrade logs.
package observability

import (
	"context"
	"log/slog"
	"strings"
)

const (
	FieldRequestID    = "request_id"
	FieldSessionID    = "session_id"
	FieldRunID        = "run_id"
	FieldTaskID       = "task_id"
	FieldBrokerID     = "broker_id"
	FieldAccountID    = "account_id"
	FieldInstrumentID = "instrument_id"
	FieldProviderID   = "provider_id"
	FieldSource       = "source"
)

type Fields struct {
	RequestID    string `json:"requestId,omitempty"`
	SessionID    string `json:"sessionId,omitempty"`
	RunID        string `json:"runId,omitempty"`
	TaskID       string `json:"taskId,omitempty"`
	BrokerID     string `json:"brokerId,omitempty"`
	AccountID    string `json:"accountId,omitempty"`
	InstrumentID string `json:"instrumentId,omitempty"`
	ProviderID   string `json:"providerId,omitempty"`
	Source       string `json:"source,omitempty"`
}

type contextState struct {
	fields   Fields
	recorder *Recorder
}

type contextKey struct{}

func WithFields(ctx context.Context, patch Fields) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	state := stateFromContext(ctx)
	state.fields = mergeFields(state.fields, patch)
	return context.WithValue(ctx, contextKey{}, state)
}

func WithRecorder(ctx context.Context, recorder *Recorder) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	state := stateFromContext(ctx)
	state.recorder = recorder
	return context.WithValue(ctx, contextKey{}, state)
}

// Detach preserves only observability values while using base for cancellation.
func Detach(base context.Context, parent context.Context) context.Context {
	if base == nil {
		base = context.Background()
	}
	state := stateFromContext(parent)
	if state == (contextState{}) {
		return base
	}
	return context.WithValue(base, contextKey{}, state)
}

func FieldsFromContext(ctx context.Context) Fields {
	return stateFromContext(ctx).fields
}

func RecorderFromContext(ctx context.Context) *Recorder {
	return stateFromContext(ctx).recorder
}

func Info(ctx context.Context, message string, attrs ...any) {
	InfoWithImportance(ctx, ImportanceNormal, message, attrs...)
}

func InfoWithImportance(ctx context.Context, importance Importance, message string, attrs ...any) {
	importance = NormalizeImportance(string(importance))
	if !shouldWrite(ctx, importance) {
		return
	}
	logAttrs := append(fieldAttrs(FieldsFromContext(ctx)), FieldImportance, importance.String())
	slog.InfoContext(ctxOrBackground(ctx), message, append(logAttrs, attrs...)...)
}

func Error(ctx context.Context, message string, err error, attrs ...any) {
	ErrorWithImportance(ctx, ImportanceHigh, message, err, attrs...)
}

func ErrorWithImportance(ctx context.Context, importance Importance, message string, err error, attrs ...any) {
	importance = NormalizeImportance(string(importance))
	if !shouldWrite(ctx, importance) {
		return
	}
	fields := FieldsFromContext(ctx)
	logAttrs := fieldAttrs(fields)
	logAttrs = append(logAttrs, FieldImportance, importance.String())
	if err != nil {
		logAttrs = append(logAttrs, "error", err.Error())
	}
	logAttrs = append(logAttrs, attrs...)
	slog.ErrorContext(ctxOrBackground(ctx), message, logAttrs...)
	if recorder := RecorderFromContext(ctx); recorder != nil {
		recorder.recordError(newEvent("error", message, fields, err, importance))
	}
}

func fieldAttrs(fields Fields) []any {
	attrs := make([]any, 0, 18)
	appendField := func(key string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			attrs = append(attrs, key, value)
		}
	}
	appendField(FieldRequestID, fields.RequestID)
	appendField(FieldSessionID, fields.SessionID)
	appendField(FieldRunID, fields.RunID)
	appendField(FieldTaskID, fields.TaskID)
	appendField(FieldBrokerID, fields.BrokerID)
	appendField(FieldAccountID, fields.AccountID)
	appendField(FieldInstrumentID, fields.InstrumentID)
	appendField(FieldProviderID, fields.ProviderID)
	appendField(FieldSource, fields.Source)
	return attrs
}

func stateFromContext(ctx context.Context) contextState {
	if ctx == nil {
		return contextState{}
	}
	state, _ := ctx.Value(contextKey{}).(contextState)
	return state
}

func mergeFields(base Fields, patch Fields) Fields {
	set := func(target *string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			*target = value
		}
	}
	set(&base.RequestID, patch.RequestID)
	set(&base.SessionID, patch.SessionID)
	set(&base.RunID, patch.RunID)
	set(&base.TaskID, patch.TaskID)
	set(&base.BrokerID, patch.BrokerID)
	set(&base.AccountID, patch.AccountID)
	set(&base.InstrumentID, patch.InstrumentID)
	set(&base.ProviderID, patch.ProviderID)
	set(&base.Source, patch.Source)
	return base
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func shouldWrite(ctx context.Context, importance Importance) bool {
	recorder := RecorderFromContext(ctx)
	if recorder != nil {
		return recorder.Accepts(importance)
	}
	return importance.meets(MinimumImportance())
}
