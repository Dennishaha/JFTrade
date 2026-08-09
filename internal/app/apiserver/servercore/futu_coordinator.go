package servercore

import (
	"context"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/futuapp"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

const liveQuoteTransportMode = futuapp.LiveQuoteTransportMode

func newFutuRuntimeCoordinator(s *serverApplication) *futuapp.Coordinator {
	if s == nil {
		return futuapp.New(futuapp.Options{})
	}
	return futuapp.New(futuapp.Options{
		Settings:          s.store,
		Registry:          s.runtimes.Brokers(),
		MarketDataRuntime: func() futuapp.MarketDataRuntime { return s.runtimes.MarketData() },
		RuntimeDependencies: func(ctx context.Context) map[string]any {
			return apiruntime.Dependencies(ctx, s.pineWorkerSettings())
		},
		LiveStreamStats: func() (int, int, bool) { return liveStreamStats(s) },
		MarketDataState: func() mdsrv.RuntimeState {
			if s.marketdataSvc == nil {
				return mdsrv.RuntimeState{}
			}
			return s.marketdataSvc.RuntimeState()
		},
		StopOrderUpdates: func() error {
			if s.tradingSvc == nil {
				return nil
			}
			return s.tradingSvc.StopOrderUpdates()
		},
		ResetCollector: func() {
			if s.marketdataSvc != nil {
				s.marketdataSvc.ResetCollector()
			}
		},
		ResumeCollector: func() {
			if s.marketdataSvc != nil {
				s.marketdataSvc.ResumeCollector()
			}
		},
	})
}

func (s *serverApplication) futuCoordinator() *futuapp.Coordinator {
	if s == nil {
		return newFutuRuntimeCoordinator(s)
	}
	if coordinator := s.runtimes.FutuCoordinator(); coordinator != nil {
		return coordinator
	}
	return newFutuRuntimeCoordinator(s)
}
