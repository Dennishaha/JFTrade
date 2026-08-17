package marketdataapp

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

// EarningsCalendar 向运行时 provider 转发财报日历读取，仅当提供者声明该可选能力。
func (r *Runtime) EarningsCalendar(
	ctx context.Context,
	beginDate string,
	endDate string,
) (marketdata.EarningsCalendarResponse, error) {
	source, err := r.calendarSource()
	if err != nil {
		return marketdata.EarningsCalendarResponse{}, err
	}
	return source.EarningsCalendar(ctx, beginDate, endDate)
}

// DividendCalendar 向运行时 provider 转发单日分红日历读取。
func (r *Runtime) DividendCalendar(ctx context.Context, date string) (marketdata.DividendCalendarResponse, error) {
	source, err := r.calendarSource()
	if err != nil {
		return marketdata.DividendCalendarResponse{}, err
	}
	return source.DividendCalendar(ctx, date)
}

// EconomicCalendar 向运行时 provider 转发财经事件日历读取。
func (r *Runtime) EconomicCalendar(
	ctx context.Context,
	beginDate string,
	endDate string,
) (marketdata.EconomicCalendarResponse, error) {
	source, err := r.calendarSource()
	if err != nil {
		return marketdata.EconomicCalendarResponse{}, err
	}
	return source.EconomicCalendar(ctx, beginDate, endDate)
}

// IpoCalendar 向运行时 provider 转发新股日历读取。
func (r *Runtime) IpoCalendar(ctx context.Context) (marketdata.IpoCalendarResponse, error) {
	source, err := r.calendarSource()
	if err != nil {
		return marketdata.IpoCalendarResponse{}, err
	}
	return source.IpoCalendar(ctx)
}

// MacroIndicators 向运行时 provider 转发宏观指标目录读取，仅当提供者声明该可选能力。
func (r *Runtime) MacroIndicators(ctx context.Context) (marketdata.MacroIndicatorsResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.MacroSource)
	if !ok {
		return marketdata.MacroIndicatorsResponse{}, fmt.Errorf(
			"%w: active provider %q does not support macro indicators",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.MacroIndicators(ctx)
}

// MacroIndicatorHistory 向运行时 provider 转发单指标历史序列读取。
func (r *Runtime) MacroIndicatorHistory(
	ctx context.Context,
	indicatorID string,
	limit int,
) (marketdata.MacroIndicatorHistoryResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.MacroSource)
	if !ok {
		return marketdata.MacroIndicatorHistoryResponse{}, fmt.Errorf(
			"%w: active provider %q does not support macro indicators",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.MacroIndicatorHistory(ctx, indicatorID, limit)
}

func (r *Runtime) calendarSource() (marketdata.CalendarSource, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.CalendarSource)
	if !ok {
		return nil, fmt.Errorf(
			"%w: active provider %q does not support event calendars",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source, nil
}
