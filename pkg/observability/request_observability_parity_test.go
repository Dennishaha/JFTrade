package observability

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

type requestObservabilityParityFixture struct {
	EventLimit        int                             `json:"eventLimit"`
	SlowThresholdMS   int64                           `json:"slowThresholdMs"`
	MinimumImportance Importance                      `json:"minimumImportance"`
	Requests          []requestObservabilityRequest   `json:"requests"`
	OpenDCalls        []requestObservabilityOpenDCall `json:"openDCalls"`
	Expected          map[string]any                  `json:"expected"`
}

type requestObservabilityRequest struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	LatencyMS int64  `json:"latencyMs"`
	RequestID string `json:"requestId"`
}

type requestObservabilityOpenDCall struct {
	Operation string `json:"operation"`
	RequestID string `json:"requestId"`
	Error     string `json:"error"`
}

func TestRequestObservabilityMatchesRustMigrationCorpus(t *testing.T) {
	fixture := loadRequestObservabilityParityFixture(t)
	recorder := NewRecorderWithConfig(RecorderConfig{
		EventLimit: fixture.EventLimit, SlowThreshold: time.Duration(fixture.SlowThresholdMS) * time.Millisecond,
		MinimumImportance: fixture.MinimumImportance,
	})
	for _, request := range fixture.Requests {
		ctx := WithFields(context.Background(), Fields{RequestID: request.RequestID, Source: "api"})
		recorder.RecordHTTPRequest(ctx, request.Method, request.Path, request.Status, time.Duration(request.LatencyMS)*time.Millisecond)
	}
	for _, call := range fixture.OpenDCalls {
		ctx := WithFields(context.Background(), Fields{RequestID: call.RequestID})
		var callErr error
		if call.Error != "" {
			callErr = errors.New(call.Error)
		}
		recorder.RecordOpenDCall(ctx, call.Operation, 0, callErr)
	}

	actual := normalizeRequestObservabilitySnapshot(t, recorder.Snapshot())
	if !reflect.DeepEqual(actual, fixture.Expected) {
		t.Fatalf("request observability mismatch\nactual: %#v\nwant:   %#v", actual, fixture.Expected)
	}
}

func loadRequestObservabilityParityFixture(t *testing.T) requestObservabilityParityFixture {
	t.Helper()
	data, err := os.ReadFile("../../tests/fixtures/rust-migration/stage9/request-observability.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture requestObservabilityParityFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func normalizeRequestObservabilitySnapshot(t *testing.T, snapshot Snapshot) map[string]any {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(data, &normalized); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"recentErrors", "recentSlowRequests"} {
		for _, raw := range normalized[key].([]any) {
			delete(raw.(map[string]any), "at")
		}
	}
	openD := normalized["openD"].(map[string]any)
	for _, key := range []string{"lastCallAt", "lastSuccessAt", "lastErrorAt"} {
		delete(openD, key)
	}
	return normalized
}
