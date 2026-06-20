package backtest_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	apibacktest "github.com/jftrade/jftrade-main/internal/api/backtest"
	srvbacktest "github.com/jftrade/jftrade-main/internal/backtest"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestSyncRouteClassifiesRequestErrorsAsBadRequest(t *testing.T) {
	router := newBacktestRouter(srvbacktest.NewService())

	tests := []string{
		`{"symbol":"bad symbol"}`,
		`{"market":"HK","code":"00700","since":"bad"}`,
		`{"market":"HK","code":"00700","since":"2024-01-03T00:00:00Z","until":"2024-01-02T00:00:00Z"}`,
	}
	for _, body := range tests {
		recorder := performJSONRequest(router, http.MethodPost, "/api/v1/backtests/sync", body)
		assertRouteError(t, recorder, http.StatusBadRequest, "BAD_REQUEST")
	}
}

func TestSyncRouteClassifiesAdapterFailureAsInternalServerError(t *testing.T) {
	service := srvbacktest.NewService(
		srvbacktest.WithNewKLineSyncerFn(func(string) (srvbacktest.KLineSyncer, error) {
			return nil, errors.New("sqlite unavailable")
		}),
	)
	router := newBacktestRouter(service)

	recorder := performJSONRequest(
		router,
		http.MethodPost,
		"/api/v1/backtests/sync",
		`{"market":"HK","code":"00700","since":"2024-01-02T00:00:00Z","until":"2024-01-03T00:00:00Z"}`,
	)
	assertRouteError(t, recorder, http.StatusInternalServerError, "SYNC_FAILED")
}

func TestSyncRouteRejectsMalformedJSON(t *testing.T) {
	router := newBacktestRouter(srvbacktest.NewService())
	recorder := performJSONRequest(router, http.MethodPost, "/api/v1/backtests/sync", `{`)
	assertRouteError(t, recorder, http.StatusBadRequest, "BAD_REQUEST")
}

func TestStartRouteClassifiesRequestAndInternalErrors(t *testing.T) {
	t.Run("invalid instrument", func(t *testing.T) {
		router := newBacktestRouter(srvbacktest.NewService())
		recorder := performJSONRequest(
			router,
			http.MethodPost,
			"/api/v1/backtests",
			`{"definitionId":"def-1","symbol":"bad symbol","startTime":"2024-01-02T00:00:00Z","endTime":"2024-01-03T00:00:00Z"}`,
		)
		assertRouteError(t, recorder, http.StatusBadRequest, "BAD_REQUEST")
	})

	t.Run("strategy provider failure", func(t *testing.T) {
		service := srvbacktest.NewService(
			srvbacktest.WithStrategyProvider(routeStrategyProvider{err: errors.New("database unavailable")}),
		)
		router := newBacktestRouter(service)
		recorder := performJSONRequest(
			router,
			http.MethodPost,
			"/api/v1/backtests",
			`{"definitionId":"def-1","market":"US","code":"AAPL","startTime":"2024-01-02T00:00:00Z","endTime":"2024-01-03T00:00:00Z"}`,
		)
		assertRouteError(t, recorder, http.StatusInternalServerError, "BACKTEST_START_FAILED")
	})
}

func TestStartRoutePreservesQueuedResponseShape(t *testing.T) {
	runs := newRouteRunStore()
	service := srvbacktest.NewService(
		srvbacktest.WithRunStore(runs),
		srvbacktest.WithStrategyProvider(routeStrategyProvider{
			found: true,
			def: srvbacktest.StrategyDef{
				ID:           "def-1",
				Version:      "v1",
				SourceFormat: strategydefinition.SourceFormatPineV6,
				Script: `//@version=6
strategy("Route Test", overlay=true, initial_capital=25000)
strategy.entry("Long", strategy.long, qty=1)`,
			},
		}),
		srvbacktest.WithRunBacktestFn(func(context.Context, bt.RunConfig) *bt.RunResult {
			return &bt.RunResult{}
		}),
	)
	t.Cleanup(func() { jftradeErr1 := service.Close(); jftradeCheckTestError(t, jftradeErr1) })
	router := newBacktestRouter(service)

	recorder := performJSONRequest(
		router,
		http.MethodPost,
		"/api/v1/backtests",
		`{"definitionId":"def-1","market":"US","code":"AAPL","startTime":"2024-01-02T00:00:00Z","endTime":"2024-01-03T00:00:00Z"}`,
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.OK || envelope.Data.ID == "" || envelope.Data.Status != "queued" || envelope.Data.Message != "backtest queued" {
		t.Fatalf("envelope = %#v, want queued response shape", envelope)
	}
}

func newBacktestRouter(service *srvbacktest.Service) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	apibacktest.RegisterRoutes(router.Group("/api/v1"), service)
	return router
}

func performJSONRequest(router http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertRouteError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, status, recorder.Body.String())
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.OK || envelope.Error.Code != code {
		t.Fatalf("envelope = %#v, want error code %q", envelope, code)
	}
}

type routeStrategyProvider struct {
	err   error
	def   srvbacktest.StrategyDef
	found bool
}

func (p routeStrategyProvider) Definition(string) (srvbacktest.StrategyDef, bool, error) {
	if p.err != nil {
		return srvbacktest.StrategyDef{}, false, p.err
	}
	return p.def, p.found, nil
}

type routeRunStore struct {
	mu      sync.Mutex
	runs    map[string]*srvbacktest.RunState
	cancels map[string]context.CancelFunc
}

func newRouteRunStore() *routeRunStore {
	return &routeRunStore{
		runs:    map[string]*srvbacktest.RunState{},
		cancels: map[string]context.CancelFunc{},
	}
}

func (s *routeRunStore) Add(run *srvbacktest.RunState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = new(*run)
	return nil
}

func (s *routeRunStore) Get(runID string) (*srvbacktest.RunState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return nil, false
	}
	clone := *run
	clone.Result = nil
	return &clone, true
}

func (s *routeRunStore) GetFull(runID string) (*srvbacktest.RunState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return nil, false, nil
	}
	return new(*run), true, nil
}

func (s *routeRunStore) List() []*srvbacktest.RunState {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := make([]*srvbacktest.RunState, 0, len(s.runs))
	for _, run := range s.runs {
		runs = append(runs, new(*run))
	}
	return runs
}

func (s *routeRunStore) ListLightweight() []*srvbacktest.RunState {
	runs := s.List()
	for _, run := range runs {
		run.Result = nil
	}
	return runs
}

func (s *routeRunStore) Update(runID string, mutate func(*srvbacktest.RunState)) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return false, nil
	}
	mutate(run)
	return true, nil
}

func (s *routeRunStore) UpdateMemoryOnly(runID string, mutate func(*srvbacktest.RunState)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return false
	}
	mutate(run)
	return true
}

func (s *routeRunStore) Delete(runID string) (*srvbacktest.RunState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return nil, false, nil
	}
	delete(s.runs, runID)
	delete(s.cancels, runID)
	return new(*run), true, nil
}

func (s *routeRunStore) SetCancel(runID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel == nil {
		delete(s.cancels, runID)
		return
	}
	s.cancels[runID] = cancel
}

func (s *routeRunStore) Cancel(runID string) bool {
	s.mu.Lock()
	cancel, ok := s.cancels[runID]
	s.mu.Unlock()
	if !ok || cancel == nil {
		return false
	}
	cancel()
	return true
}

func (s *routeRunStore) Close() error {
	return nil
}

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
