package observability

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestObservabilityCoverageForBackgroundContextsAndImportanceRanks(t *testing.T) {
	ctx := WithFields(context.Background(), Fields{RequestID: "request-coverage"})
	if fields := FieldsFromContext(ctx); fields.RequestID != "request-coverage" {
		t.Fatalf("WithFields(background) fields = %#v", fields)
	}
	Info(context.Background(), "background context logging is supported")
	Error(context.Background(), "background context error logging is supported", errors.New("coverage"))

	for rank, want := range map[int]Importance{
		0: ImportanceLow,
		1: ImportanceNormal,
		2: ImportanceHigh,
		3: ImportanceCritical,
	} {
		if got := importanceByRank(rank); got != want {
			t.Fatalf("importanceByRank(%d) = %q, want %q", rank, got, want)
		}
	}

	var nilRecorder *Recorder
	snapshot := nilRecorder.Snapshot()
	if snapshot.RecentErrors == nil || snapshot.RecentSlowRequests == nil {
		t.Fatalf("nil Recorder snapshot should expose empty slices: %#v", snapshot)
	}
	nilRecorder.RecordOpenDCall(context.Background(), "nil-recorder-error", time.Millisecond, errors.New("unavailable"))
}

func TestObservabilityCoverageForContextDetachWithBackgroundBase(t *testing.T) {
	parent := WithRecorder(WithFields(context.Background(), Fields{Source: "coverage"}), NewRecorder(1, time.Second))
	detached := Detach(context.Background(), parent)
	if detached == nil || FieldsFromContext(detached).Source != "coverage" || RecorderFromContext(detached) == nil {
		t.Fatalf("Detach(background, parent) = fields:%#v recorder:%#v", FieldsFromContext(detached), RecorderFromContext(detached))
	}
}
