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
	base := t.Context()
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

func TestObservabilityBoundaryDefaultsAndGlobalOpenDLogging(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	recorder := NewRecorderWithConfig(RecorderConfig{})
	snapshot := recorder.Snapshot()
	if snapshot.SlowThresholdMS != defaultSlowThreshold.Milliseconds() || snapshot.MinimumImportance != ImportanceLow.String() {
		t.Fatalf("default recorder snapshot = %#v", snapshot)
	}
	if !recorder.Accepts(ImportanceNormal) {
		t.Fatal("default recorder should accept normal importance")
	}
	if !(*Recorder)(nil).Accepts(ImportanceCritical) {
		t.Fatal("nil recorder should accept all importance levels")
	}

	var nilCtx context.Context
	ctx := WithRecorder(nilCtx, recorder)
	ctx = WithFields(ctx, Fields{RequestID: " request-boundary ", Source: " api "})
	Error(ctx, strings.Repeat("x ", 400), errors.New(strings.Repeat("boom ", 200)))
	snapshot = recorder.Snapshot()
	if len(snapshot.RecentErrors) != 1 {
		t.Fatalf("recent errors = %#v", snapshot.RecentErrors)
	}
	if len(snapshot.RecentErrors[0].Message) > maxSummaryTextBytes+3 || !strings.HasSuffix(snapshot.RecentErrors[0].Message, "...") {
		t.Fatalf("long message was not sanitized: %q", snapshot.RecentErrors[0].Message)
	}

	RecordOpenDCall(context.Background(), "global-qot", time.Millisecond, errors.New("global failure"))
	if !strings.Contains(output.String(), `"source":"opend"`) || !strings.Contains(output.String(), "global-qot") {
		t.Fatalf("global OpenD log missing source/operation: %s", output.String())
	}

	if NormalizeImportance("unknown").String() != ImportanceNormal.String() {
		t.Fatal("unknown importance should normalize to normal")
	}
	if NormalizeMinimumImportance("unknown").String() != ImportanceLow.String() {
		t.Fatal("unknown minimum importance should normalize to low")
	}
	if Importance("fatal").String() != ImportanceCritical.String() {
		t.Fatal("fatal importance alias should stringify as critical")
	}
	if Importance("weird").String() != ImportanceNormal.String() {
		t.Fatal("unknown direct importance should stringify as normal")
	}
	if got := importanceByRank(99); got != ImportanceLow {
		t.Fatalf("importanceByRank(99) = %s, want low", got)
	}
}

func TestObservabilityNilContextAndOpenDSuccessBoundaries(t *testing.T) {
	var nilCtx context.Context
	if fields := FieldsFromContext(nilCtx); fields != (Fields{}) {
		t.Fatalf("nil context fields = %#v", fields)
	}
	if recorder := RecorderFromContext(nilCtx); recorder != nil {
		t.Fatalf("nil context recorder = %#v", recorder)
	}

	base := context.Background()
	detached := Detach(base, context.Background())
	if detached != base {
		t.Fatal("Detach without parent observability state should return base context")
	}

	recorder := NewRecorder(3, time.Second)
	ctx := WithRecorder(WithFields(context.Background(), Fields{RequestID: "request-success"}), recorder)
	recorder.RecordOpenDCall(ctx, "proto_3007", time.Millisecond, nil)
	snapshot := recorder.Snapshot()
	if snapshot.OpenD.TotalCalls != 1 || snapshot.OpenD.FailedCalls != 0 || snapshot.OpenD.LastSuccessAt == "" {
		t.Fatalf("OpenD success health = %#v", snapshot.OpenD)
	}
	if snapshot.OpenD.LastOperation != "proto_3007" || snapshot.OpenD.LastRequestID != "request-success" {
		t.Fatalf("OpenD success correlation = %#v", snapshot.OpenD)
	}

	var nilRecorder *Recorder
	nilRecorder.RecordHTTPRequest(context.Background(), "GET", "/ignored", 500, time.Second)
	nilRecorder.RecordOpenDCall(context.Background(), "ignored", time.Second, nil)
	nilRecorder.recordError(Event{Message: "ignored"})
}
