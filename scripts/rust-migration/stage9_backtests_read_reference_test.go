package rustmigration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	apibacktest "github.com/jftrade/jftrade-main/internal/api/backtest"
	srv "github.com/jftrade/jftrade-main/internal/backtest"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/chart"
)

const stage9BacktestsReadFixtureVersion = "stage9.backtests-read.v1"

type stage9BacktestsReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9BacktestsReadFixture struct {
	Version string                    `json:"version"`
	Cases   []stage9BacktestsReadCase `json:"cases"`
}

// TestStage9BacktestsReadFixtureMatchesCurrentGoOwner freezes the three
// stable backtest-run GET projections together. The fake run store is only a
// deterministic producer for the Go wire fixture; Rust receives snapshots in
// test-cutover wiring and never opens the production database.
func TestStage9BacktestsReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 backtests fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/backtests-read.json")
	cases := []struct {
		name  string
		path  string
		store *stage9BacktestRunStore
	}{
		{name: "list", path: "/api/v1/backtests", store: stage9BacktestStoreWithRun()},
		{name: "status-existing", path: "/api/v1/backtests/fixture-run/status", store: stage9BacktestStoreWithRun()},
		{name: "result-existing", path: "/api/v1/backtests/fixture-run", store: stage9BacktestStoreWithRun()},
		{name: "status-missing", path: "/api/v1/backtests/missing-run/status", store: stage9BacktestStoreWithRun()},
		{name: "result-missing", path: "/api/v1/backtests/missing-run", store: stage9BacktestStoreWithRun()},
		{name: "result-store-failure", path: "/api/v1/backtests/store-failure", store: &stage9BacktestRunStore{getFullErr: true}},
	}
	want := stage9BacktestsReadFixture{Version: stage9BacktestsReadFixtureVersion, Cases: make([]stage9BacktestsReadCase, 0, len(cases))}
	for _, testCase := range cases {
		gin.SetMode(gin.TestMode)
		service := srv.NewService(srv.WithRunStore(testCase.store))
		router := gin.New()
		apibacktest.RegisterRoutes(router.Group("/api/v1"), service)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9BacktestsReadCase{
			Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path, ExpectedStatus: recorder.Code,
		}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v", testCase.name, err)
		}
		if envelope.Error != nil {
			entry.ErrorCode, entry.ErrorMessage = envelope.Error.Code, envelope.Error.Message
		} else {
			entry.Data = compactBacktestsReadJSON(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode backtests fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write backtests fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read backtests fixture: %v", err)
	}
	var got stage9BacktestsReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode backtests fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactBacktestsReadJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactBacktestsReadJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 backtests read fixture drifted from the Go owner: got=%#v want=%#v", got, want)
	}
}

func compactBacktestsReadJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

type stage9BacktestsSyncReadFixture struct {
	Version string                    `json:"version"`
	Cases   []stage9BacktestsReadCase `json:"cases"`
}

// TestStage9BacktestsSyncReadFixtureMatchesCurrentGoOwner freezes the one
// mutable sync-progress projection without starting a real provider worker.
func TestStage9BacktestsSyncReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 backtests sync fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/backtests-sync-read.json")
	cases := []struct {
		name  string
		path  string
		store *stage9BacktestSyncTaskStore
	}{
		{name: "queued", path: "/api/v1/backtests/sync/fixture-queued", store: stage9BacktestSyncStore("fixture-queued", "queued")},
		{name: "running", path: "/api/v1/backtests/sync/fixture-running", store: stage9BacktestSyncStore("fixture-running", "running")},
		{name: "failed", path: "/api/v1/backtests/sync/fixture-failed", store: stage9BacktestSyncStore("fixture-failed", "failed")},
		{name: "missing", path: "/api/v1/backtests/sync/missing-task", store: stage9BacktestSyncStore("fixture-queued", "queued")},
		{name: "blank", path: "/api/v1/backtests/sync/%20", store: stage9BacktestSyncStore("fixture-queued", "queued")},
	}
	want := stage9BacktestsSyncReadFixture{Version: "stage9.backtests-sync-read.v1", Cases: make([]stage9BacktestsReadCase, 0, len(cases))}
	for _, testCase := range cases {
		gin.SetMode(gin.TestMode)
		service := srv.NewService(srv.WithSyncTaskStore(testCase.store))
		router := gin.New()
		apibacktest.RegisterRoutes(router.Group("/api/v1"), service)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9BacktestsReadCase{
			Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path, ExpectedStatus: recorder.Code,
		}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v", testCase.name, err)
		}
		if envelope.Error != nil {
			entry.ErrorCode, entry.ErrorMessage = envelope.Error.Code, envelope.Error.Message
		} else {
			entry.Data = compactBacktestsReadJSON(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode backtests sync fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write backtests sync fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read backtests sync fixture: %v", err)
	}
	var got stage9BacktestsSyncReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode backtests sync fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactBacktestsReadJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactBacktestsReadJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 backtests sync read fixture drifted from the Go owner: got=%#v want=%#v", got, want)
	}
}

type stage9BacktestSyncTaskStore struct {
	progress *bt.SyncProgress
}

func stage9BacktestSyncStore(taskID, status string) *stage9BacktestSyncTaskStore {
	progress := bt.NewSyncProgress(taskID, "US.AAPL", time.Date(2026, 8, 15, 20, 0, 0, 0, time.UTC))
	progress.Status = status
	progress.MarketDataProvider = "futu"
	progress.CurrentInterval = "1d"
	progress.TotalIntervals = 2
	progress.CompletedIntervals = 1
	progress.TotalBatches = 4
	progress.CompletedBatches = 2
	progress.Retries = 1
	progress.UpdatedAt = "2026-08-15T20:01:00Z"
	if status == "queued" {
		progress.CurrentInterval = ""
		progress.TotalIntervals = 0
		progress.CompletedIntervals = 0
		progress.TotalBatches = 0
		progress.CompletedBatches = 0
		progress.Retries = 0
		progress.UpdatedAt = ""
	}
	if status == "failed" {
		progress.Error = "OpenD unavailable"
	}
	return &stage9BacktestSyncTaskStore{progress: progress}
}

func (s *stage9BacktestSyncTaskStore) Add(string, *bt.SyncProgress, context.CancelFunc) {}
func (s *stage9BacktestSyncTaskStore) Get(taskID string) (*bt.SyncProgress, bool) {
	if s.progress == nil || s.progress.TaskID != taskID {
		return nil, false
	}
	return s.progress.Snapshot(), true
}
func (s *stage9BacktestSyncTaskStore) Finish(string) {}
func (s *stage9BacktestSyncTaskStore) Cancel(string, time.Time) (*bt.SyncProgress, bool) {
	return nil, false
}

type stage9BacktestRunStore struct {
	mu         sync.Mutex
	run        *srv.RunState
	getFullErr bool
}

func stage9BacktestStoreWithRun() *stage9BacktestRunStore {
	useExtendedHours := false
	return &stage9BacktestRunStore{run: &srv.RunState{
		ID: "fixture-run", Status: "completed",
		Request: srv.StartRequest{
			DefinitionID: "fixture-strategy", DefinitionVersion: "v1", Market: "US", Code: "AAPL", Symbol: "US.AAPL",
			InstrumentType: "stock", Interval: "1d", StartDate: "2026-08-01", EndDate: "2026-08-15",
			MarketTimezone: "America/New_York", InitialBalance: 10000, RehabType: "none", UseExtendedHours: &useExtendedHours,
			ExecutionModel: "next_open", ChartType: chart.ChartTypeStandard,
		},
		Result: &bt.RunResult{
			Symbol: "US.AAPL", MarketDataProvider: "futu", Interval: "1d", StartTime: "2026-08-01T13:30:00Z", EndTime: "2026-08-15T20:00:00Z",
			QuoteCurrency: "USD", FinalBalance: 10500, PnL: 500, TotalTrades: 1, WinRate: 1, Logs: []string{"fixture complete"}, ChartType: chart.ChartTypeStandard,
		},
		CreatedAt: "2026-08-15T20:00:00Z", UpdatedAt: "2026-08-15T20:01:00Z", MarketDataProvider: "futu",
	}}
}

func (s *stage9BacktestRunStore) Add(run *srv.RunState) error { s.run = run; return nil }
func (s *stage9BacktestRunStore) Get(runID string) (*srv.RunState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.match(runID)
}
func (s *stage9BacktestRunStore) GetFull(runID string) (*srv.RunState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getFullErr {
		return nil, true, context.DeadlineExceeded
	}
	run, ok := s.match(runID)
	return run, ok, nil
}
func (s *stage9BacktestRunStore) List() []*srv.RunState {
	if s.run == nil {
		return nil
	}
	return []*srv.RunState{s.run}
}
func (s *stage9BacktestRunStore) ListLightweight() []*srv.RunState {
	run := s.run
	if run == nil {
		return nil
	}
	copy := *run
	copy.Result = nil
	return []*srv.RunState{&copy}
}
func (s *stage9BacktestRunStore) Update(string, func(*srv.RunState)) (bool, error)  { return false, nil }
func (s *stage9BacktestRunStore) UpdateMemoryOnly(string, func(*srv.RunState)) bool { return false }
func (s *stage9BacktestRunStore) Delete(string) (*srv.RunState, bool, error)        { return nil, false, nil }
func (s *stage9BacktestRunStore) SetCancel(string, context.CancelFunc)              {}
func (s *stage9BacktestRunStore) Cancel(string) bool                                { return false }
func (s *stage9BacktestRunStore) Close() error                                      { return nil }

func (s *stage9BacktestRunStore) match(runID string) (*srv.RunState, bool) {
	if s.run == nil || s.run.ID != runID {
		return nil, false
	}
	return s.run, true
}
