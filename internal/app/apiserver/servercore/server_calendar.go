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

func (s *serverApplication) systemCalendarOptions() []system.Option {
	return []system.Option{
		system.WithExchangeCalendarStatus(func() map[string]any {
			calendars := s.runtimes.ExchangeCalendars()
			if calendars == nil {
				return map[string]any{}
			}
			return calendars.Status()
		}),
		system.WithExchangeCalendarSources(func() []map[string]any {
			calendars := s.runtimes.ExchangeCalendars()
			if calendars == nil {
				return nil
			}
			return calendars.Sources()
		}),
		system.WithRefreshExchangeCalendars(func(ctx context.Context, market string) map[string]any {
			return s.handleExchangeCalendarOperation(ctx, market, true)
		}),
		system.WithProbeExchangeCalendars(func(ctx context.Context, market string) map[string]any {
			return s.handleExchangeCalendarOperation(ctx, market, false)
		}),
	}
}

func (s *serverApplication) handleExchangeCalendarOperation(ctx context.Context, market string, refresh bool) map[string]any {
	calendars := s.runtimes.ExchangeCalendars()
	if calendars == nil {
		return map[string]any{"accepted": false}
	}
	operationCtx, cancel := exchangeCalendarOperationContext(ctx)
	defer cancel()
	if strings.TrimSpace(market) == "" {
		if refresh {
			return calendars.RefreshAll(operationCtx)
		}
		return calendars.ProbeAll(operationCtx)
	}
	if refresh {
		return calendars.RefreshMarket(operationCtx, market)
	}
	return calendars.ProbeMarket(operationCtx, market)
}
