package backtest_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	srvbacktest "github.com/jftrade/jftrade-main/internal/backtest"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

type routeSyncTaskStore struct {
	progress map[string]*bt.SyncProgress
	cancelOK map[string]bool
}

func newRouteSyncTaskStore() *routeSyncTaskStore {
	return &routeSyncTaskStore{
		progress: map[string]*bt.SyncProgress{},
		cancelOK: map[string]bool{},
	}
}

func (s *routeSyncTaskStore) Add(taskID string, progress *bt.SyncProgress, cancel context.CancelFunc) {
	s.progress[taskID] = progress
}
func (s *routeSyncTaskStore) Get(taskID string) (*bt.SyncProgress, bool) {
	progress, ok := s.progress[taskID]
	return progress, ok
}
func (s *routeSyncTaskStore) Finish(taskID string) {}
func (s *routeSyncTaskStore) Cancel(taskID string, cancelledAt time.Time) (*bt.SyncProgress, bool) {
	progress, ok := s.progress[taskID]
	if !ok || !s.cancelOK[taskID] {
		return nil, false
	}
	progress.MarkCancelled(cancelledAt)
	return progress, true
}

type routeErrorRunStore struct {
	*routeRunStore
	getFullErr error
	deleteErr  error
}

func (s *routeErrorRunStore) GetFull(runID string) (*srvbacktest.RunState, bool, error) {
	if s.getFullErr != nil {
		return nil, false, s.getFullErr
	}
	return s.routeRunStore.GetFull(runID)
}

func (s *routeErrorRunStore) Delete(runID string) (*srvbacktest.RunState, bool, error) {
	if s.deleteErr != nil {
		return nil, false, s.deleteErr
	}
	return s.routeRunStore.Delete(runID)
}

type routeDeleteMissRunStore struct {
	*routeRunStore
}

func (s *routeDeleteMissRunStore) Delete(runID string) (*srvbacktest.RunState, bool, error) {
	return nil, false, nil
}

func TestSyncProgressAndCancelRoutesHandleSuccessAndNotFound(t *testing.T) {
	progress := bt.NewSyncProgress("task-1", "HK.00700", time.Date(2026, time.June, 22, 9, 0, 0, 0, time.UTC))
	progress.SetRunning(1, time.Date(2026, time.June, 22, 9, 0, 1, 0, time.UTC))
	store := newRouteSyncTaskStore()
	store.progress["task-1"] = progress
	store.cancelOK["task-1"] = true

	router := newBacktestRouter(srvbacktest.NewService(srvbacktest.WithSyncTaskStore(store)))

	progressRec := performJSONRequest(router, http.MethodGet, "/api/v1/backtests/sync/task-1", "")
	if progressRec.Code != http.StatusOK {
		t.Fatalf("sync progress status=%d body=%s", progressRec.Code, progressRec.Body.String())
	}

	cancelRec := performJSONRequest(router, http.MethodDelete, "/api/v1/backtests/sync/task-1", "")
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("sync cancel status=%d body=%s", cancelRec.Code, cancelRec.Body.String())
	}

	missingProgressRec := performJSONRequest(router, http.MethodGet, "/api/v1/backtests/sync/missing", "")
	assertRouteError(t, missingProgressRec, http.StatusNotFound, "NOT_FOUND")

	missingCancelRec := performJSONRequest(router, http.MethodDelete, "/api/v1/backtests/sync/missing", "")
	assertRouteError(t, missingCancelRec, http.StatusNotFound, "NOT_FOUND")
}

func TestStatusResultAndDeleteRoutesCoverTerminalAndStoreFailures(t *testing.T) {
	runs := newRouteRunStore()
	running := &srvbacktest.RunState{ID: "run-running", Status: "running"}
	completed := &srvbacktest.RunState{ID: "run-complete", Status: "completed"}
	failed := &srvbacktest.RunState{ID: "run-failed", Status: "failed"}
	cancelled := &srvbacktest.RunState{ID: "run-cancelled", Status: "cancelled"}
	jftradeCheckTestError(t, runs.Add(running))
	jftradeCheckTestError(t, runs.Add(completed))
	jftradeCheckTestError(t, runs.Add(failed))
	jftradeCheckTestError(t, runs.Add(cancelled))

	router := newBacktestRouter(srvbacktest.NewService(srvbacktest.WithRunStore(runs)))

	statusRec := performJSONRequest(router, http.MethodGet, "/api/v1/backtests/run-complete/status", "")
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status route status=%d body=%s", statusRec.Code, statusRec.Body.String())
	}

	resultRec := performJSONRequest(router, http.MethodGet, "/api/v1/backtests/run-complete", "")
	if resultRec.Code != http.StatusOK {
		t.Fatalf("result route status=%d body=%s", resultRec.Code, resultRec.Body.String())
	}

	deleteRunningRec := performJSONRequest(router, http.MethodDelete, "/api/v1/backtests/run-running", "")
	assertRouteError(t, deleteRunningRec, http.StatusBadRequest, "BAD_REQUEST")

	deleteCompleteRec := performJSONRequest(router, http.MethodDelete, "/api/v1/backtests/run-complete", "")
	if deleteCompleteRec.Code != http.StatusOK {
		t.Fatalf("delete completed status=%d body=%s", deleteCompleteRec.Code, deleteCompleteRec.Body.String())
	}
	deleteFailedRec := performJSONRequest(router, http.MethodDelete, "/api/v1/backtests/run-failed", "")
	if deleteFailedRec.Code != http.StatusOK {
		t.Fatalf("delete failed status=%d body=%s", deleteFailedRec.Code, deleteFailedRec.Body.String())
	}
	deleteCancelledRec := performJSONRequest(router, http.MethodDelete, "/api/v1/backtests/run-cancelled", "")
	if deleteCancelledRec.Code != http.StatusOK {
		t.Fatalf("delete cancelled status=%d body=%s", deleteCancelledRec.Code, deleteCancelledRec.Body.String())
	}

	missingStatusRec := performJSONRequest(router, http.MethodGet, "/api/v1/backtests/missing/status", "")
	assertRouteError(t, missingStatusRec, http.StatusNotFound, "NOT_FOUND")

	missingDeleteRec := performJSONRequest(router, http.MethodDelete, "/api/v1/backtests/missing", "")
	assertRouteError(t, missingDeleteRec, http.StatusNotFound, "NOT_FOUND")
}

func TestResultAndDeleteRoutesMapRunStoreErrorsToInternalServerError(t *testing.T) {
	store := &routeErrorRunStore{
		routeRunStore: newRouteRunStore(),
		getFullErr:    errors.New("load failed"),
		deleteErr:     errors.New("delete failed"),
	}
	jftradeCheckTestError(t, store.Add(&srvbacktest.RunState{ID: "run-1", Status: "completed"}))

	router := newBacktestRouter(srvbacktest.NewService(srvbacktest.WithRunStore(store)))

	resultRec := performJSONRequest(router, http.MethodGet, "/api/v1/backtests/run-1", "")
	assertRouteError(t, resultRec, http.StatusInternalServerError, "BACKTEST_RUN_STORE_FAILED")

	deleteRec := performJSONRequest(router, http.MethodDelete, "/api/v1/backtests/run-1", "")
	assertRouteError(t, deleteRec, http.StatusInternalServerError, "BACKTEST_RUN_STORE_FAILED")
}

func TestDeleteRouteReturnsNotFoundWhenTerminalRunDisappearsBeforeDelete(t *testing.T) {
	store := &routeDeleteMissRunStore{routeRunStore: newRouteRunStore()}
	jftradeCheckTestError(t, store.Add(&srvbacktest.RunState{ID: "run-race", Status: "completed"}))

	router := newBacktestRouter(srvbacktest.NewService(srvbacktest.WithRunStore(store)))

	deleteRec := performJSONRequest(router, http.MethodDelete, "/api/v1/backtests/run-race", "")
	assertRouteError(t, deleteRec, http.StatusNotFound, "NOT_FOUND")
}
