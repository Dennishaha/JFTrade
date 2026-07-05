package backtest

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/observability"
)

func TestSyncProgressAndCancelDelegateToSyncTaskStore(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	svc := NewService(WithSyncTaskStore(tasks))
	cancelled := false
	progress := bt.NewSyncProgress("task-1", "HK.00700", time.Now())
	tasks.Add("task-1", progress, func() { cancelled = true })

	got, ok := svc.GetSyncProgress("task-1")
	if !ok || got.TaskID != "task-1" || got.Status != "queued" {
		t.Fatalf("GetSyncProgress() = %#v, %v; want queued task", got, ok)
	}

	got, ok = svc.CancelSync("task-1")
	if !ok || got.Status != "cancelled" || !cancelled {
		t.Fatalf("CancelSync() = %#v, %v cancelled=%v; want cancelled task", got, ok, cancelled)
	}
	if _, ok := svc.GetSyncProgress("missing"); ok {
		t.Fatal("GetSyncProgress(missing) = true, want false")
	}
}

func TestSyncConvertsParamsAndClosesAdapter(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{done: make(chan struct{})}
	var gotDBPath string
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithDBPathFn(func() string { return "/tmp/sync.db" }),
		WithNewKLineSyncerFn(func(dbPath string) (KLineSyncer, error) {
			gotDBPath = dbPath
			return syncer, nil
		}),
	)

	requestContext := observability.WithFields(context.Background(), observability.Fields{RequestID: "request-sync-1"})
	started, err := svc.Sync(requestContext, SyncRequest{
		Market:       "US",
		Code:         "AAPL",
		Intervals:    []string{"1d", "1w"},
		Since:        "2024-01-02T00:00:00Z",
		Until:        "2024-01-03T00:00:00Z",
		RehabType:    "backward",
		SessionScope: "extended",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if gotDBPath != "/tmp/sync.db" {
		t.Fatalf("db path = %q, want /tmp/sync.db", gotDBPath)
	}
	if started.Symbol != "US.AAPL" {
		t.Fatalf("symbol = %q, want US.AAPL", started.Symbol)
	}

	select {
	case <-syncer.done:
	case <-time.After(2 * time.Second):
		t.Fatal("sync adapter was not called")
	}
	waitForSyncFinished(t, tasks, started.TaskID)

	syncer.mu.Lock()
	params := syncer.params
	closed := syncer.closed
	fields := syncer.fields
	syncer.mu.Unlock()
	if params.Symbol != "US.AAPL" {
		t.Fatalf("params symbol = %q, want US.AAPL", params.Symbol)
	}
	if !reflect.DeepEqual(params.Intervals, []bbgotypes.Interval{"1h"}) {
		t.Fatalf("params intervals = %#v, want [1h]", params.Intervals)
	}
	if params.RehabType != RehabTypeBackward {
		t.Fatalf("rehab type = %q, want backward", params.RehabType)
	}
	if params.SessionScope != "extended" {
		t.Fatalf("session scope = %q, want extended", params.SessionScope)
	}
	if !closed {
		t.Fatal("sync adapter was not closed")
	}
	if fields.RequestID != "request-sync-1" || fields.TaskID != started.TaskID || fields.InstrumentID != "US.AAPL" || fields.Source != "backtest" {
		t.Fatalf("sync observability fields = %#v", fields)
	}
	if !tasks.isFinished(started.TaskID) {
		t.Fatal("sync task was not marked finished")
	}
}

func TestSyncFailureMarksProgressFailedAndClosesAdapter(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{
		err:  errors.New("OpenD unavailable"),
		done: make(chan struct{}),
	}
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) {
			return syncer, nil
		}),
	)

	started, err := svc.Sync(context.Background(), SyncRequest{
		Market: "HK",
		Code:   "00700",
		Since:  "2024-01-02T00:00:00Z",
		Until:  "2024-01-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	select {
	case <-syncer.done:
	case <-time.After(2 * time.Second):
		t.Fatal("sync adapter was not called")
	}
	waitForSyncFinished(t, tasks, started.TaskID)

	progress, ok := tasks.Get(started.TaskID)
	if !ok || progress.Status != "failed" || progress.Error != "OpenD unavailable" {
		t.Fatalf("progress = %#v, %v; want failed OpenD error", progress, ok)
	}
	syncer.mu.Lock()
	closed := syncer.closed
	syncer.mu.Unlock()
	if !closed {
		t.Fatal("failed sync adapter was not closed")
	}
}

func TestCloseCancelsAndWaitsForActiveSync(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{
		waitForCancel: true,
		started:       make(chan struct{}),
	}
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) {
			return syncer, nil
		}),
	)

	started, err := svc.Sync(context.Background(), SyncRequest{
		Market: "HK",
		Code:   "00700",
		Since:  "2024-01-02T00:00:00Z",
		Until:  "2024-01-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	select {
	case <-syncer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("sync adapter was not called")
	}

	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	waitForSyncFinished(t, tasks, started.TaskID)
	progress, ok := tasks.Get(started.TaskID)
	if !ok || progress.Status != "cancelled" {
		t.Fatalf("progress = %#v, %v; want cancelled", progress, ok)
	}
	syncer.mu.Lock()
	closed := syncer.closed
	syncer.mu.Unlock()
	if !closed {
		t.Fatal("sync adapter was not closed before Close returned")
	}
}

func TestSyncTaskIDsAreUnique(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) {
			return &fakeKLineSyncer{}, nil
		}),
	)
	request := SyncRequest{
		Market: "HK",
		Code:   "00700",
		Since:  "2024-01-02T00:00:00Z",
		Until:  "2024-01-03T00:00:00Z",
	}
	first, err := svc.Sync(context.Background(), request)
	if err != nil {
		t.Fatalf("first Sync() error = %v", err)
	}
	second, err := svc.Sync(context.Background(), request)
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if first.TaskID == second.TaskID {
		t.Fatalf("task IDs collided: %q", first.TaskID)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestSyncClosesAdapterWhenTaskStoreMissing(t *testing.T) {
	syncer := &fakeKLineSyncer{}
	svc := NewService(WithNewKLineSyncerFn(func(string) (KLineSyncer, error) {
		return syncer, nil
	}))

	_, err := svc.Sync(context.Background(), SyncRequest{
		Market: "HK",
		Code:   "00700",
		Since:  "2024-01-02T00:00:00Z",
		Until:  "2024-01-03T00:00:00Z",
	})
	if err == nil || !strings.Contains(err.Error(), "sync task store not configured") {
		t.Fatalf("Sync() error = %v, want missing task store", err)
	}
	syncer.mu.Lock()
	closed := syncer.closed
	syncer.mu.Unlock()
	if !closed {
		t.Fatal("adapter was not closed after setup failure")
	}
}

func TestSyncRequestErrors(t *testing.T) {
	svc := NewService()
	tests := []struct {
		name string
		req  SyncRequest
	}{
		{
			name: "invalid symbol",
			req:  SyncRequest{Symbol: "bad symbol"},
		},
		{
			name: "invalid since",
			req:  SyncRequest{Market: "HK", Code: "00700", Since: "bad"},
		},
		{
			name: "invalid until",
			req:  SyncRequest{Market: "HK", Code: "00700", Until: "bad"},
		},
		{
			name: "reversed range",
			req:  SyncRequest{Market: "HK", Code: "00700", Since: "2024-01-03T00:00:00Z", Until: "2024-01-02T00:00:00Z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Sync(context.Background(), tt.req)
			if err == nil || !IsRequestError(err) {
				t.Fatalf("Sync() error = %v, want RequestError", err)
			}
		})
	}
}

func TestSyncUnknownRehabTypeFallsBackToForward(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{done: make(chan struct{})}
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) {
			return syncer, nil
		}),
	)

	started, err := svc.Sync(context.Background(), SyncRequest{
		Market:    "HK",
		Code:      "00700",
		Since:     "2024-01-02T00:00:00Z",
		Until:     "2024-01-03T00:00:00Z",
		RehabType: "sideways",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	select {
	case <-syncer.done:
	case <-time.After(2 * time.Second):
		t.Fatal("sync adapter was not called")
	}
	waitForSyncFinished(t, tasks, started.TaskID)

	syncer.mu.Lock()
	rehabType := syncer.params.RehabType
	syncer.mu.Unlock()
	if rehabType != RehabTypeForward {
		t.Fatalf("rehabType = %v, want forward fallback", rehabType)
	}
}

func TestPlanSyncIntervals(t *testing.T) {
	tests := []struct {
		name         string
		symbol       string
		requested    []bbgotypes.Interval
		sessionScope string
		want         []bbgotypes.Interval
	}{
		{
			name:      "deduplicates planned intervals",
			symbol:    "HK.00700",
			requested: []bbgotypes.Interval{"1m", "1m", "3d"},
			want:      []bbgotypes.Interval{"1m", "1d"},
		},
		{
			name:      "downgrades unsupported multi day and sub daily intervals",
			symbol:    "HK.00700",
			requested: []bbgotypes.Interval{"3d", "2w", "2h"},
			want:      []bbgotypes.Interval{"1d", "1h"},
		},
		{
			name:         "uses hourly data for us extended daily sessions",
			symbol:       "US.AAPL",
			requested:    []bbgotypes.Interval{"1d", "1w"},
			sessionScope: "extended",
			want:         []bbgotypes.Interval{"1h"},
		},
		{
			name:         "keeps us regular daily sessions unchanged",
			symbol:       "US.AAPL",
			requested:    []bbgotypes.Interval{"1d"},
			sessionScope: "regular",
			want:         []bbgotypes.Interval{"1d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := planSyncIntervals(tt.symbol, tt.requested, tt.sessionScope); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("planSyncIntervals() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizeSessionScope(t *testing.T) {
	for input, want := range map[string]string{
		"regular":    "regular",
		" extended ": "extended",
		"":           "legacy",
		"unknown":    "legacy",
	} {
		if got := normalizeSessionScope(input); got != want {
			t.Fatalf("normalizeSessionScope(%q) = %q, want %q", input, got, want)
		}
	}
}
