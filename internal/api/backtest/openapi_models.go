package backtest

import srv "github.com/jftrade/jftrade-main/internal/backtest"

// BacktestRunsData documents the list response envelope payload.
type BacktestRunsData struct {
	Runs []*srv.RunState `json:"runs"`
}

// BacktestQueuedData documents the response returned after a run is queued.
type BacktestQueuedData struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// BacktestStatusData documents the lightweight run status response.
type BacktestStatusData struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// BacktestSyncCancelData documents a successful sync cancellation.
type BacktestSyncCancelData struct {
	TaskID string `json:"taskId"`
	Status string `json:"status"`
}

// BacktestSyncProgress documents the concurrency-safe progress snapshot wire shape.
type BacktestSyncProgress struct {
	TaskID             string `json:"taskId"`
	Status             string `json:"status"`
	Symbol             string `json:"symbol"`
	CurrentInterval    string `json:"currentInterval"`
	TotalIntervals     int    `json:"totalIntervals"`
	CompletedIntervals int    `json:"completedIntervals"`
	TotalBatches       int    `json:"totalBatches"`
	CompletedBatches   int    `json:"completedBatches"`
	Retries            int    `json:"retries"`
	Error              string `json:"error,omitempty"`
	StartedAt          string `json:"startedAt"`
	UpdatedAt          string `json:"updatedAt"`
}

// BacktestDeleteData documents a successful run deletion.
type BacktestDeleteData struct {
	Deleted bool   `json:"deleted"`
	ID      string `json:"id"`
}
