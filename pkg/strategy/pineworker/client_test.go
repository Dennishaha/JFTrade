package pineworker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestClientRunScriptSuccessAppliesMetadataDefaults(t *testing.T) {
	now := time.Unix(100, 0)
	transport := fakeTransport{run: func(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
		return RunScriptResponse{
			JobID: request.JobID,
			Plots: []PlotOutput{{
				Name:   "signal",
				Values: []float64{1},
			}},
			Metadata: WorkerMetadata{WorkerID: "worker-1"},
		}, nil
	}}
	client := newTestClient(t, transport, WithNow(func() time.Time {
		now = now.Add(10 * time.Millisecond)
		return now
	}))

	response, err := client.RunScript(context.Background(), validClientRequest())
	if err != nil {
		t.Fatalf("RunScript error = %v", err)
	}
	if response.JobID != "job-1" || response.Metadata.WorkerID != "worker-1" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.Metadata.Duration <= 0 || response.Metadata.RequestBytes <= 0 || response.Metadata.ResponseBytes <= 0 {
		t.Fatalf("metadata defaults were not applied: %#v", response.Metadata)
	}
}

func TestClientRunScriptRejectsInvalidRequestBeforeTransport(t *testing.T) {
	transport := &countingTransport{}
	client := newTestClient(t, transport)
	request := validClientRequest()
	request.Candles = nil

	err := runExpectError(t, client, request)
	if !strings.Contains(err.Error(), "candles are required") {
		t.Fatalf("error = %v, want candles validation", err)
	}
	if transport.calls != 0 {
		t.Fatalf("transport calls = %d, want 0", transport.calls)
	}
}

func TestClientRunScriptRejectsOversizedRequest(t *testing.T) {
	transport := &countingTransport{}
	config := DefaultWorkerConfig(4)
	config.MaxMessageBytes = 10
	client, err := NewClient(transport, config, WithPerformanceGate(relaxedGate()))
	if err != nil {
		t.Fatal(err)
	}

	err = runExpectError(t, client, validClientRequest())
	if !strings.Contains(err.Error(), "request bytes") {
		t.Fatalf("error = %v, want request bytes", err)
	}
	if transport.calls != 0 {
		t.Fatalf("transport calls = %d, want 0", transport.calls)
	}
}

func TestClientRunScriptMapsTransportError(t *testing.T) {
	client := newTestClient(t, fakeTransport{run: func(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
		return RunScriptResponse{}, errors.New("connection refused")
	}})

	err := runExpectError(t, client, validClientRequest())
	if !strings.Contains(err.Error(), "transport error") {
		t.Fatalf("error = %v, want transport error", err)
	}
}

func TestClientRunScriptMapsTimeout(t *testing.T) {
	client := newTestClient(t, fakeTransport{run: func(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
		return RunScriptResponse{}, context.DeadlineExceeded
	}})

	err := runExpectError(t, client, validClientRequest())
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want timeout", err)
	}
}

func TestClientRunScriptMapsWorkerError(t *testing.T) {
	client := newTestClient(t, fakeTransport{run: func(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
		return RunScriptResponse{JobID: request.JobID, Error: "compile failed"}, nil
	}})

	err := runExpectError(t, client, validClientRequest())
	if !strings.Contains(err.Error(), "compile failed") {
		t.Fatalf("error = %v, want worker error", err)
	}
}

func TestClientRunScriptRejectsMismatchedJobID(t *testing.T) {
	client := newTestClient(t, fakeTransport{run: func(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
		return RunScriptResponse{JobID: "other", Metadata: WorkerMetadata{Duration: time.Millisecond}}, nil
	}})

	err := runExpectError(t, client, validClientRequest())
	if !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v, want job id mismatch", err)
	}
}

func TestClientRunScriptRejectsPerformanceGateFailure(t *testing.T) {
	client := newTestClient(t, fakeTransport{run: func(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
		return RunScriptResponse{
			JobID: request.JobID,
			Metadata: WorkerMetadata{
				Duration:      2 * time.Second,
				RequestBytes:  100,
				ResponseBytes: 100,
			},
		}, nil
	}}, WithPerformanceGate(PerformanceGate{MaxDuration: time.Second}))

	err := runExpectError(t, client, validClientRequest())
	if !strings.Contains(err.Error(), "performance gate failed") {
		t.Fatalf("error = %v, want performance gate failure", err)
	}
}

func TestNewClientRequiresTransport(t *testing.T) {
	_, err := NewClient(nil, DefaultWorkerConfig(1))
	if err == nil || !strings.Contains(err.Error(), "transport is required") {
		t.Fatalf("error = %v, want transport required", err)
	}
}

type fakeTransport struct {
	run func(context.Context, RunScriptRequest) (RunScriptResponse, error)
}

func (transport fakeTransport) RunScript(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
	return transport.run(ctx, request)
}

type countingTransport struct {
	calls int
}

func (transport *countingTransport) RunScript(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
	transport.calls++
	return RunScriptResponse{JobID: request.JobID}, nil
}

func newTestClient(t *testing.T, transport Transport, options ...ClientOption) *Client {
	t.Helper()
	client, err := NewClient(transport, DefaultWorkerConfig(4), append([]ClientOption{WithPerformanceGate(relaxedGate())}, options...)...)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func relaxedGate() PerformanceGate {
	return PerformanceGate{
		MaxDuration:       time.Minute,
		MaxDurationPerBar: time.Minute,
		MinCandlesPerSec:  0,
		MaxRequestBytes:   0,
		MaxResponseBytes:  0,
		MaxPeakRSSBytes:   0,
	}
}

func validClientRequest() RunScriptRequest {
	return RunScriptRequest{
		JobID:    "job-1",
		ScriptID: "script-1",
		Source: `//@version=6
indicator("worker smoke")
plot(close, "close")`,
		Symbol:    "US.AAPL",
		Timeframe: "1",
		Mode:      ModeBacktest,
		Candles: []Candle{{
			OpenTime:  1_700_000_000_000,
			CloseTime: 1_700_000_060_000,
			Open:      10,
			High:      12,
			Low:       9,
			Close:     11,
			Volume:    100,
		}},
		Params: map[string]string{"threshold": "10"},
	}
}

func runExpectError(t *testing.T, client *Client, request RunScriptRequest) error {
	t.Helper()
	_, err := client.RunScript(context.Background(), request)
	if err == nil {
		t.Fatal("RunScript error = nil, want error")
	}
	return err
}
