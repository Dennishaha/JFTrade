package backtest

import (
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

// Result and synchronization contracts are re-exported by the business layer
// so application and transport code do not depend on the execution package.
type (
	Candle         = bt.Candle
	DrawdownPoint  = bt.DrawdownPoint
	OrderBookEntry = bt.OrderBookEntry
	PnLPoint       = bt.PnLPoint
	RunResult      = bt.RunResult
	SyncProgress   = bt.SyncProgress
	TradeEvent     = bt.TradeEvent
)

func NewSyncProgress(taskID string, symbol string, queuedAt time.Time) *SyncProgress {
	return bt.NewSyncProgress(taskID, symbol, queuedAt)
}
