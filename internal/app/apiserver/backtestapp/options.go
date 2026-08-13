package backtestapp

import (
	"context"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	backtestservice "github.com/jftrade/jftrade-main/internal/backtest"
	backteststore "github.com/jftrade/jftrade-main/internal/store/backtest"
)

func ProviderOptions(
	runtime *marketdataapp.Runtime,
	dbPath func() string,
	providerID func() string,
) []backtestservice.Option {
	if runtime == nil {
		panic("assemble backtest service: market-data provider runtime is unavailable")
	}
	return []backtestservice.Option{
		backtestservice.WithDBPathFn(dbPath),
		backtestservice.WithBacktestProviderIDFn(providerID),
		backtestservice.WithProviderKLineSyncerFn(func(ctx context.Context, path, provider string) (backtestservice.KLineSyncer, error) {
			return NewKLineSyncer(ctx, runtime, path, provider)
		}),
		backtestservice.WithKLineSyncPreflight(NewKLineSyncPreflight(runtime)),
		backtestservice.WithProviderKLineCoverageCheckFn(backteststore.CheckKLineCoverageForProvider),
		backtestservice.WithInstrumentSpecResolver(NewInstrumentSpecResolver(runtime)),
	}
}
