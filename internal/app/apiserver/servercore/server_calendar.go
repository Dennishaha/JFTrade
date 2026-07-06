package servercore

import (
	"context"
	"strings"

	"github.com/jftrade/jftrade-main/internal/system"
)

func exchangeCalendarOperationContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), exchangeCalendarOperationTimeout)
}

func (s *Server) systemCalendarOptions() []system.Option {
	return []system.Option{
		system.WithExchangeCalendarStatus(func() map[string]any {
			if s.exchangeCalendars == nil {
				return map[string]any{}
			}
			return s.exchangeCalendars.Status()
		}),
		system.WithExchangeCalendarSources(func() []map[string]any {
			if s.exchangeCalendars == nil {
				return nil
			}
			return s.exchangeCalendars.Sources()
		}),
		system.WithRefreshExchangeCalendars(func(ctx context.Context, market string) map[string]any {
			return s.handleExchangeCalendarOperation(ctx, market, true)
		}),
		system.WithProbeExchangeCalendars(func(ctx context.Context, market string) map[string]any {
			return s.handleExchangeCalendarOperation(ctx, market, false)
		}),
	}
}

func (s *Server) handleExchangeCalendarOperation(ctx context.Context, market string, refresh bool) map[string]any {
	if s.exchangeCalendars == nil {
		return map[string]any{"accepted": false}
	}
	operationCtx, cancel := exchangeCalendarOperationContext(ctx)
	defer cancel()
	if strings.TrimSpace(market) == "" {
		if refresh {
			return s.exchangeCalendars.RefreshAll(operationCtx)
		}
		return s.exchangeCalendars.ProbeAll(operationCtx)
	}
	if refresh {
		return s.exchangeCalendars.RefreshMarket(operationCtx, market)
	}
	return s.exchangeCalendars.ProbeMarket(operationCtx, market)
}
