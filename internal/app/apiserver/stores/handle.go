// Package stores owns the API application's persistent store aggregate.
package stores

import (
	"errors"

	appcomposition "github.com/jftrade/jftrade-main/internal/app/apiserver/application"
	backteststore "github.com/jftrade/jftrade-main/internal/store/backtest"
	researchstore "github.com/jftrade/jftrade-main/internal/store/research"
	strategystore "github.com/jftrade/jftrade-main/internal/store/strategy"
	tradingstore "github.com/jftrade/jftrade-main/internal/store/trading"
	watchliststore "github.com/jftrade/jftrade-main/internal/store/watchlist"
)

// Handle is the single persistent-store dependency held by the application
// composition root. Its zero value is safe for focused tests.
type Handle struct {
	StrategyCatalog strategystore.CatalogResource
	Design          strategystore.Resource
	BacktestRuns    backteststore.Resource
	BacktestTasks   backteststore.SyncTaskResource
	ExecutionOrders tradingstore.Resource
	Watchlist       *watchliststore.Store
	Research        *researchstore.Store

	resources *appcomposition.Resources
	setupErr  error
}

// Open opens and registers one store. A failure rolls back stores previously
// opened through the same handle and prevents later stores from opening.
func Open[T any](
	handle *Handle,
	name string,
	openFn func() (T, error),
	closeFn func(T) error,
) T {
	var zero T
	if handle == nil || handle.setupErr != nil {
		return zero
	}
	if handle.resources == nil {
		handle.resources = &appcomposition.Resources{}
	}
	value, err := appcomposition.Open(handle.resources, name, openFn, closeFn)
	if err != nil {
		handle.setupErr = errors.Join(handle.setupErr, err)
		return zero
	}
	return value
}

// SetupError returns the aggregated persistent-store startup and rollback error.
func (h *Handle) SetupError() error {
	if h == nil {
		return nil
	}
	return h.setupErr
}

// Close releases stores in reverse successful-open order.
func (h *Handle) Close() error {
	if h == nil {
		return nil
	}
	return h.resources.Close()
}
