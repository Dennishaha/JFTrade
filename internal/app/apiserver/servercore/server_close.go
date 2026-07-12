package servercore

import (
	"context"
	"errors"
	"fmt"

	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
)

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.closeErr = s.collectCloseError()
	})
	return s.closeErr
}

func (s *Server) collectCloseError() error {
	var errs []error
	s.closeCoreServices(&errs)
	s.closePersistentStores(&errs)
	s.closeRuntimeManagers(&errs)
	s.restoreCalendarResolver()
	return errors.Join(errs...)
}

func (s *Server) closeCoreServices(errs *[]error) {
	if s.auth != nil {
		s.auth.close()
	}
	s.appendCloseError(errs, "trading order updates close", s.stopTradingOrderUpdates)
	s.appendCloseError(errs, "liveWebSocket close", s.closeLiveWebSocket)
	s.appendCloseError(errs, "marketdata close", s.closeMarketdataService)
	s.appendCloseError(errs, "liveNotifications close", s.closeLiveNotifications)
	s.appendCloseError(errs, "backtestSvc close", s.closeBacktestService)
	s.closePineWorkerRunner(errs, "backtestPineWorkerRunner", s.backtestPineWorkerRunner)
	s.closePineWorkerRunner(errs, "instancePineWorkerRunner", s.instancePineWorkerRunner)
}

func (s *Server) closePersistentStores(errs *[]error) {
	s.appendCloseError(errs, "watchlist store close", s.closeWatchlistStore)
	s.appendCloseError(errs, "backtestRuns close", s.closeBacktestRuns)
	s.appendCloseError(errs, "executionOrders close", s.closeExecutionOrders)
	s.appendCloseError(errs, "strategyStore close", s.closeStrategyStore)
	s.appendCloseError(errs, "designStore close", s.closeDesignStore)
}

func (s *Server) closeWatchlistStore() error {
	if s.watchlistStore == nil {
		return nil
	}
	return s.watchlistStore.Close()
}

func (s *Server) closeRuntimeManagers(errs *[]error) {
	s.appendCloseError(errs, "local MCP server close", s.closeMCPServer)
	s.appendCloseError(errs, "assistantSvc close", s.closeAssistantService)
	s.appendCloseError(errs, "futu marketdata runtime close", s.closeMarketdataRuntime)
	s.appendCloseError(errs, "exchange calendar manager close", s.closeExchangeCalendars)
}

func (s *Server) closeMCPServer() error {
	if s.mcpServer == nil {
		return nil
	}
	return s.mcpServer.Close()
}

func (s *Server) appendCloseError(errs *[]error, name string, closeFn func() error) {
	if closeFn == nil {
		return
	}
	if err := closeFn(); err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", name, err))
	}
}

func (s *Server) closePineWorkerRunner(errs *[]error, name string, runner pineWorkerRunner) {
	if runner == nil {
		return
	}
	closer, ok := runner.(interface{ Close(context.Context) error })
	if !ok {
		return
	}
	if err := closer.Close(context.Background()); err != nil {
		*errs = append(*errs, fmt.Errorf("%s close: %w", name, err))
	}
}

func (s *Server) stopTradingOrderUpdates() error {
	if s.tradingSvc == nil {
		return nil
	}
	return s.tradingSvc.StopOrderUpdates()
}

func (s *Server) closeLiveWebSocket() error {
	if s.liveWebSocket == nil {
		return nil
	}
	return s.liveWebSocket.Close()
}

func (s *Server) closeMarketdataService() error {
	if s.marketdataSvc == nil {
		return nil
	}
	return s.marketdataSvc.Close()
}

func (s *Server) closeLiveNotifications() error {
	if s.liveNotifications == nil {
		return nil
	}
	return s.liveNotifications.Close()
}

func (s *Server) closeBacktestService() error {
	if s.backtestSvc == nil {
		return nil
	}
	return s.backtestSvc.Close()
}

func (s *Server) closeBacktestRuns() error {
	if s.backtestRuns == nil {
		return nil
	}
	return s.backtestRuns.Close()
}

func (s *Server) closeExecutionOrders() error {
	if s.executionOrders == nil {
		return nil
	}
	return s.executionOrders.Close()
}

func (s *Server) closeStrategyStore() error {
	if s.strategyStore == nil {
		return nil
	}
	return s.strategyStore.Close()
}

func (s *Server) closeDesignStore() error {
	if s.designStore == nil {
		return nil
	}
	return s.designStore.Close()
}

func (s *Server) closeAssistantService() error {
	if s.assistantSvc == nil {
		return nil
	}
	return s.assistantSvc.Close()
}

func (s *Server) closeMarketdataRuntime() error {
	if s.marketdataRuntime == nil {
		return nil
	}
	return s.marketdataRuntime.Close()
}

func (s *Server) closeExchangeCalendars() error {
	if s.exchangeCalendars == nil {
		return nil
	}
	return s.exchangeCalendars.Close()
}

func (s *Server) restoreCalendarResolver() {
	if s.previousCalendarResolver != nil {
		marketpkg.SetCalendarResolver(s.previousCalendarResolver)
		return
	}
	marketpkg.ResetCalendarResolver()
}
