package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestStructuredLogIncludesCanonicalCorrelationFields(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	ctx := WithFields(context.Background(), Fields{
		RequestID: "request-1", SessionID: "session-1", RunID: "run-1",
		TaskID: "task-1", BrokerID: "futu", AccountID: "account-1",
		InstrumentID: "HK.00700", ProviderID: "provider-1", Source: "adk",
	})
	Info(ctx, "correlated event")

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("decode slog output: %v", err)
	}
	for key, want := range map[string]string{
		FieldRequestID: "request-1", FieldSessionID: "session-1", FieldRunID: "run-1",
		FieldTaskID: "task-1", FieldBrokerID: "futu", FieldAccountID: "account-1",
		FieldInstrumentID: "HK.00700", FieldProviderID: "provider-1", FieldSource: "adk",
	} {
		if got := event[key]; got != want {
			t.Fatalf("%s = %#v, want %q; event=%#v", key, got, want, event)
		}
	}
	if got := event[FieldImportance]; got != ImportanceNormal.String() {
		t.Fatalf("importance = %#v, want %q; event=%#v", got, ImportanceNormal, event)
	}
}

func TestImportanceThresholdSuppressesLowerImportanceLogs(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	recorder := NewRecorderWithConfig(RecorderConfig{
		EventLimit:        5,
		SlowThreshold:     time.Second,
		MinimumImportance: ImportanceHigh,
	})
	ctx := WithRecorder(context.Background(), recorder)

	InfoWithImportance(ctx, ImportanceLow, "low event")
	if strings.Contains(output.String(), "low event") {
		t.Fatalf("low importance log was written: %s", output.String())
	}
	ErrorWithImportance(ctx, ImportanceCritical, "critical event", errors.New("boom"))

	raw := output.String()
	if !strings.Contains(raw, "critical event") || !strings.Contains(raw, `"importance":"critical"`) {
		t.Fatalf("critical log missing importance: %s", raw)
	}
	snapshot := recorder.Snapshot()
	if len(snapshot.RecentErrors) != 1 || snapshot.RecentErrors[0].Importance != ImportanceCritical.String() {
		t.Fatalf("recent errors = %#v", snapshot.RecentErrors)
	}
	if snapshot.MinimumImportance != ImportanceHigh.String() {
		t.Fatalf("minimum importance = %q", snapshot.MinimumImportance)
	}
}

func TestGlobalImportanceThresholdAppliesWithoutRecorder(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	previousMinimum := MinimumImportance()
	SetMinimumImportance(ImportanceHigh)
	t.Cleanup(func() { SetMinimumImportance(previousMinimum) })

	InfoWithImportance(context.Background(), ImportanceLow, "low event")
	if strings.Contains(output.String(), "low event") {
		t.Fatalf("low importance log was written without recorder: %s", output.String())
	}
	ErrorWithImportance(context.Background(), ImportanceHigh, "high event", errors.New("boom"))
	if !strings.Contains(output.String(), `"importance":"high"`) {
		t.Fatalf("high importance log missing importance: %s", output.String())
	}
}

func TestRecorderBoundsErrorsSlowRequestsAndOpenDHealth(t *testing.T) {
	recorder := NewRecorder(2, 10*time.Millisecond)
	ctx := WithRecorder(WithFields(context.Background(), Fields{RequestID: "request-1"}), recorder)

	recorder.RecordHTTPRequest(ctx, "GET", "/slow", 200, 15*time.Millisecond)
	recorder.RecordHTTPRequest(ctx, "POST", "/failed", 503, time.Millisecond)
	recorder.RecordOpenDCall(ctx, "proto_3006", 3*time.Millisecond, errors.New("quote permission denied"))
	recorder.RecordHTTPRequest(ctx, "GET", "/failed-again", 500, time.Millisecond)

	snapshot := recorder.Snapshot()
	if len(snapshot.RecentErrors) != 2 {
		t.Fatalf("recent errors = %d, want bounded 2: %#v", len(snapshot.RecentErrors), snapshot.RecentErrors)
	}
	if len(snapshot.RecentSlowRequests) != 1 || snapshot.RecentSlowRequests[0].Path != "/slow" {
		t.Fatalf("slow requests = %#v", snapshot.RecentSlowRequests)
	}
	if snapshot.RecentSlowRequests[0].Importance != ImportanceLow.String() {
		t.Fatalf("slow request importance = %q", snapshot.RecentSlowRequests[0].Importance)
	}
	for _, event := range snapshot.RecentErrors {
		if event.Importance != ImportanceHigh.String() {
			t.Fatalf("error importance = %q in %#v", event.Importance, snapshot.RecentErrors)
		}
	}
	if snapshot.OpenD.TotalCalls != 1 || snapshot.OpenD.FailedCalls != 1 {
		t.Fatalf("OpenD health = %#v", snapshot.OpenD)
	}
	if snapshot.OpenD.LastOperation != "proto_3006" || snapshot.OpenD.LastRequestID != "request-1" {
		t.Fatalf("OpenD correlation = %#v", snapshot.OpenD)
	}
	if snapshot.MinimumImportance != ImportanceLow.String() {
		t.Fatalf("minimum importance = %q", snapshot.MinimumImportance)
	}
}

func TestDetachPreservesCorrelationWithoutParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(WithFields(context.Background(), Fields{RequestID: "request-1", Source: "api"}))
	base, baseCancel := context.WithCancel(context.Background())
	defer baseCancel()
	detached := Detach(base, parent)
	cancel()

	if err := detached.Err(); err != nil {
		t.Fatalf("detached context inherited request cancellation: %v", err)
	}
	fields := FieldsFromContext(detached)
	if fields.RequestID != "request-1" || fields.Source != "api" {
		t.Fatalf("detached fields = %#v", fields)
	}
}
