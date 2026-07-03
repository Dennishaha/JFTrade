package service

import (
	"context"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

// BackTestable is copied from bbgo's backtest store boundary and narrowed here
// to the interface JFTrade uses for replay and sync.
type BackTestable interface {
	Verify(sourceExchange types.Exchange, symbols []string, startTime time.Time, endTime time.Time) error
	Sync(ctx context.Context, ex types.Exchange, symbol string, intervals []types.Interval, since, until time.Time) error
	QueryKLine(ex types.Exchange, symbol string, interval types.Interval, orderBy string, limit int) (*types.KLine, error)
	QueryKLinesForward(ex types.Exchange, symbol string, interval types.Interval, startTime time.Time, limit int) ([]types.KLine, error)
	QueryKLinesBackward(ex types.Exchange, symbol string, interval types.Interval, endTime time.Time, limit int) ([]types.KLine, error)
	QueryKLinesCh(since, until time.Time, exchange types.Exchange, symbols []string, intervals []types.Interval) (chan types.KLine, chan error)
}
