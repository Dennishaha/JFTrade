package backtest

import (
	"time"

	internalstorage "github.com/jftrade/jftrade-main/pkg/futu/backtest/internal/storage"
)

type SyncProgress = internalstorage.SyncProgress

func NewSyncProgress(taskID string, symbol string, queuedAt time.Time) *SyncProgress {
	return internalstorage.NewSyncProgress(taskID, symbol, queuedAt)
}
