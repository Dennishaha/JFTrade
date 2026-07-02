package backtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/backtest"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestBacktestListAndMissingResultRoutes(t *testing.T) {
	service := srv.NewService()
	router := backtestBoundaryRouter(service)

	list := backtestBoundaryRequest(router, http.MethodGet, "/api/v1/backtests", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"runs":null`) {
		t.Fatalf("list response = %d %s", list.Code, list.Body.String())
	}
	missingRouter := backtestBoundaryRouter(srv.NewService(srv.WithRunStore(boundaryEmptyRunStore{})))
	missing := backtestBoundaryRequest(missingRouter, http.MethodGet, "/api/v1/backtests/missing", "")
	backtestBoundaryAssertError(t, missing, http.StatusNotFound, "NOT_FOUND")
}

func TestBacktestStartRouteRejectsMalformedAndMissingStrategy(t *testing.T) {
	t.Run("malformed json", func(t *testing.T) {
		router := backtestBoundaryRouter(srv.NewService())
		response := backtestBoundaryRequest(router, http.MethodPost, "/api/v1/backtests", "{")
		backtestBoundaryAssertError(t, response, http.StatusBadRequest, "BAD_REQUEST")
	})

	t.Run("strategy not found", func(t *testing.T) {
		service := srv.NewService(srv.WithStrategyProvider(missingStrategyProvider{}))
		router := backtestBoundaryRouter(service)
		response := backtestBoundaryRequest(router, http.MethodPost, "/api/v1/backtests", `{
			"definitionId":"missing",
			"market":"US",
			"code":"AAPL",
			"startTime":"2024-01-02T00:00:00Z",
			"endTime":"2024-01-03T00:00:00Z"
		}`)
		backtestBoundaryAssertError(t, response, http.StatusNotFound, "NOT_FOUND")
	})
}

func TestBacktestSyncRouteReturnsTaskForValidRequest(t *testing.T) {
	service := srv.NewService(
		srv.WithSyncTaskStore(newBoundarySyncTaskStore()),
		srv.WithNewKLineSyncerFn(func(string) (srv.KLineSyncer, error) {
			return boundarySyncer{}, nil
		}),
	)
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("Close service: %v", err)
		}
	})
	router := backtestBoundaryRouter(service)
	response := backtestBoundaryRequest(router, http.MethodPost, "/api/v1/backtests/sync", `{
		"market":"US",
		"code":"AAPL",
		"intervals":["1m"],
		"since":"2024-01-02T00:00:00Z",
		"until":"2024-01-03T00:00:00Z"
	}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"taskId"`) || !strings.Contains(response.Body.String(), `"US.AAPL"`) {
		t.Fatalf("sync response = %d %s", response.Code, response.Body.String())
	}
}

func TestBacktestHandlersRejectMissingAndBlankURIParameters(t *testing.T) {
	service := srv.NewService()
	tests := []struct {
		name    string
		handler gin.HandlerFunc
		key     string
		value   *string
	}{
		{name: "sync progress missing", handler: handleSyncProgress(service), key: "taskId"},
		{name: "sync cancel missing", handler: handleSyncCancel(service), key: "taskId"},
		{name: "status missing", handler: handleStatus(service), key: "runId"},
		{name: "result missing", handler: handleResult(service), key: "runId"},
		{name: "delete missing", handler: handleDelete(service), key: "runId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			tt.handler(ctx)
			backtestBoundaryAssertError(t, response, http.StatusBadRequest, "BAD_REQUEST")
		})
	}

	blank := "   "
	for _, tt := range []struct {
		name    string
		handler gin.HandlerFunc
		key     string
	}{
		{name: "sync progress blank", handler: handleSyncProgress(service), key: "taskId"},
		{name: "delete blank", handler: handleDelete(service), key: "runId"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			ctx.Params = gin.Params{{Key: tt.key, Value: blank}}
			tt.handler(ctx)
			backtestBoundaryAssertError(t, response, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}

type missingStrategyProvider struct{}

func (missingStrategyProvider) Definition(string) (srv.StrategyDef, bool, error) {
	return srv.StrategyDef{}, false, nil
}

type boundaryEmptyRunStore struct{}

func (boundaryEmptyRunStore) Add(*srv.RunState) error                           { return nil }
func (boundaryEmptyRunStore) Get(string) (*srv.RunState, bool)                  { return nil, false }
func (boundaryEmptyRunStore) GetFull(string) (*srv.RunState, bool, error)       { return nil, false, nil }
func (boundaryEmptyRunStore) List() []*srv.RunState                             { return []*srv.RunState{} }
func (boundaryEmptyRunStore) ListLightweight() []*srv.RunState                  { return []*srv.RunState{} }
func (boundaryEmptyRunStore) Update(string, func(*srv.RunState)) (bool, error)  { return false, nil }
func (boundaryEmptyRunStore) UpdateMemoryOnly(string, func(*srv.RunState)) bool { return false }
func (boundaryEmptyRunStore) Delete(string) (*srv.RunState, bool, error)        { return nil, false, nil }
func (boundaryEmptyRunStore) SetCancel(string, context.CancelFunc)              {}
func (boundaryEmptyRunStore) Cancel(string) bool                                { return false }
func (boundaryEmptyRunStore) Close() error                                      { return nil }

type boundarySyncer struct{}

func (boundarySyncer) Sync(_ context.Context, params srv.KLineSyncParams, progress *bt.SyncProgress) error {
	progress.MarkCompleted(len(params.Intervals), time.Now())
	return nil
}

func (boundarySyncer) Close() error { return nil }

type boundarySyncTaskStore struct {
	mu       sync.Mutex
	progress map[string]*bt.SyncProgress
	cancels  map[string]context.CancelFunc
}

func newBoundarySyncTaskStore() *boundarySyncTaskStore {
	return &boundarySyncTaskStore{progress: map[string]*bt.SyncProgress{}, cancels: map[string]context.CancelFunc{}}
}

func (store *boundarySyncTaskStore) Add(taskID string, progress *bt.SyncProgress, cancel context.CancelFunc) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.progress[taskID] = progress
	store.cancels[taskID] = cancel
}

func (store *boundarySyncTaskStore) Get(taskID string) (*bt.SyncProgress, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	progress, ok := store.progress[taskID]
	return progress, ok
}

func (store *boundarySyncTaskStore) Finish(taskID string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.cancels, taskID)
}

func (store *boundarySyncTaskStore) Cancel(taskID string, cancelledAt time.Time) (*bt.SyncProgress, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	progress, ok := store.progress[taskID]
	if !ok {
		return nil, false
	}
	if cancel := store.cancels[taskID]; cancel != nil {
		cancel()
	}
	progress.MarkCancelled(cancelledAt)
	delete(store.cancels, taskID)
	return progress, true
}

func backtestBoundaryRouter(service *srv.Service) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)
	return router
}

func backtestBoundaryRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func backtestBoundaryAssertError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, status, response.Body.String())
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error.Code != code {
		t.Fatalf("error code = %q, want %q", envelope.Error.Code, code)
	}
}
