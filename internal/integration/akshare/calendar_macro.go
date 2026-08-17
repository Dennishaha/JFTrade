package akshare

import (
	"context"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

var (
	_ marketdata.CalendarSource = (*Provider)(nil)
	_ marketdata.MacroSource    = (*Provider)(nil)
)

// EarningsCalendar returns the cross-market earnings calendar; the sidecar
// owns the default window when the range is empty.
func (p *Provider) EarningsCalendar(
	ctx context.Context,
	beginDate, endDate string,
) (marketdata.EarningsCalendarResponse, error) {
	response, err := p.client.earningsCalendar(ctx, beginDate, endDate)
	if err != nil {
		return marketdata.EarningsCalendarResponse{}, err
	}
	entries := make([]marketdata.EarningsEvent, 0, len(response.Entries))
	for index, entry := range response.Entries {
		instrumentID, err := requireCalendarInstrumentID(entry.InstrumentID, "earnings", index)
		if err != nil {
			return marketdata.EarningsCalendarResponse{}, err
		}
		entries = append(entries, marketdata.EarningsEvent{
			InstrumentID: instrumentID,
			Name:         strings.TrimSpace(entry.Name),
			Symbol:       strings.TrimSpace(entry.Symbol),
			EventDate:    strings.TrimSpace(entry.EventDate),
			PeriodText:   strings.TrimSpace(entry.PeriodText),
			MarketCap:    entry.MarketCap, Price: entry.Price,
		})
	}
	return marketdata.EarningsCalendarResponse{
		BeginDate: beginDate, EndDate: endDate, Entries: entries, Source: "akshare-calendar",
	}, nil
}

// DividendCalendar returns the single-day cross-market dividend calendar.
func (p *Provider) DividendCalendar(ctx context.Context, date string) (marketdata.DividendCalendarResponse, error) {
	response, err := p.client.dividendCalendar(ctx, date)
	if err != nil {
		return marketdata.DividendCalendarResponse{}, err
	}
	entries := make([]marketdata.DividendEvent, 0, len(response.Entries))
	for index, entry := range response.Entries {
		instrumentID, err := requireCalendarInstrumentID(entry.InstrumentID, "dividends", index)
		if err != nil {
			return marketdata.DividendCalendarResponse{}, err
		}
		entries = append(entries, marketdata.DividendEvent{
			InstrumentID: instrumentID,
			Name:         strings.TrimSpace(entry.Name),
			Symbol:       strings.TrimSpace(entry.Symbol),
			Statement:    strings.TrimSpace(entry.Statement),
			ExDate:       strings.TrimSpace(entry.ExDate),
			RecordDate:   strings.TrimSpace(entry.RecordDate),
			PayableDate:  optionalText(entry.PayableDate),
		})
	}
	return marketdata.DividendCalendarResponse{
		Date: strings.TrimSpace(date), Entries: entries, Source: "akshare-calendar",
	}, nil
}

// EconomicCalendar returns the cross-market economic event calendar.
func (p *Provider) EconomicCalendar(
	ctx context.Context,
	beginDate, endDate string,
) (marketdata.EconomicCalendarResponse, error) {
	response, err := p.client.economicCalendar(ctx, beginDate, endDate)
	if err != nil {
		return marketdata.EconomicCalendarResponse{}, err
	}
	entries := make([]marketdata.EconomicEvent, 0, len(response.Entries))
	for index, entry := range response.Entries {
		eventID := strings.TrimSpace(entry.EventID)
		if eventID == "" {
			return marketdata.EconomicCalendarResponse{}, fmt.Errorf(
				"%w: economic calendar entry %d event_id is required", ErrInvalidResponse, index,
			)
		}
		entries = append(entries, marketdata.EconomicEvent{
			EventID:        eventID,
			Title:          strings.TrimSpace(entry.Title),
			Region:         strings.TrimSpace(entry.Region),
			EventDate:      strings.TrimSpace(entry.EventDate),
			EventTimestamp: entry.EventTimestamp,
			Importance:     entry.Importance,
			PreviousValue:  optionalText(entry.PreviousValue),
			ForecastValue:  optionalText(entry.ForecastValue),
			ActualValue:    optionalText(entry.ActualValue),
		})
	}
	return marketdata.EconomicCalendarResponse{
		BeginDate: beginDate, EndDate: endDate, Entries: entries, Source: "akshare-calendar",
	}, nil
}

// IpoCalendar returns the cross-market IPO calendar (listed and pending).
func (p *Provider) IpoCalendar(ctx context.Context) (marketdata.IpoCalendarResponse, error) {
	response, err := p.client.ipoCalendar(ctx)
	if err != nil {
		return marketdata.IpoCalendarResponse{}, err
	}
	entries := make([]marketdata.IpoEntry, 0, len(response.Entries))
	for index, entry := range response.Entries {
		instrumentID, err := requireCalendarInstrumentID(entry.InstrumentID, "ipos", index)
		if err != nil {
			return marketdata.IpoCalendarResponse{}, err
		}
		status := strings.ToLower(strings.TrimSpace(entry.Status))
		if status != "listed" && status != "pending" {
			return marketdata.IpoCalendarResponse{}, fmt.Errorf(
				"%w: ipo entry %d status %q", ErrInvalidResponse, index, entry.Status,
			)
		}
		entries = append(entries, marketdata.IpoEntry{
			InstrumentID: instrumentID, Name: strings.TrimSpace(entry.Name),
			Symbol: strings.TrimSpace(entry.Symbol), Status: status,
			ListingDate: optionalText(entry.ListingDate),
			IssueVolume: entry.IssueVolume, IssuePrice: entry.IssuePrice,
			IssuePriceMin: entry.IssuePriceMin, IssuePriceMax: entry.IssuePriceMax,
		})
	}
	return marketdata.IpoCalendarResponse{Entries: entries, Source: "akshare-calendar"}, nil
}

// MacroIndicators returns the macro indicator catalog grouped by category.
func (p *Provider) MacroIndicators(ctx context.Context) (marketdata.MacroIndicatorsResponse, error) {
	response, err := p.client.macroIndicators(ctx)
	if err != nil {
		return marketdata.MacroIndicatorsResponse{}, err
	}
	categories := make([]marketdata.MacroIndicatorCategory, 0, len(response.Categories))
	for index, category := range response.Categories {
		indicators := make([]marketdata.MacroIndicator, 0, len(category.Indicators))
		for offset, indicator := range category.Indicators {
			indicatorID := strings.TrimSpace(indicator.IndicatorID)
			if indicatorID == "" {
				return marketdata.MacroIndicatorsResponse{}, fmt.Errorf(
					"%w: macro indicator %d.%d indicator_id is required", ErrInvalidResponse, index, offset,
				)
			}
			indicators = append(indicators, marketdata.MacroIndicator{
				IndicatorID: indicatorID, Name: strings.TrimSpace(indicator.Name),
				Region: strings.TrimSpace(indicator.Region), Unit: strings.TrimSpace(indicator.Unit),
				UnitType: indicator.UnitType, Frequency: strings.TrimSpace(indicator.Frequency),
			})
		}
		categories = append(categories, marketdata.MacroIndicatorCategory{
			CategoryName: strings.TrimSpace(category.CategoryName), Indicators: indicators,
		})
	}
	return marketdata.MacroIndicatorsResponse{Categories: categories, Source: "akshare-macro"}, nil
}

// MacroIndicatorHistory returns one indicator's history series.
func (p *Provider) MacroIndicatorHistory(
	ctx context.Context,
	indicatorID string,
	limit int,
) (marketdata.MacroIndicatorHistoryResponse, error) {
	limit = normalizeLimit(limit, marketdata.DefaultMacroHistoryLimit, marketdata.MaxMacroHistoryLimit)
	response, err := p.client.macroIndicatorHistory(ctx, indicatorID, limit)
	if err != nil {
		return marketdata.MacroIndicatorHistoryResponse{}, err
	}
	if echoed := strings.TrimSpace(response.IndicatorID); echoed != "" && echoed != indicatorID {
		return marketdata.MacroIndicatorHistoryResponse{}, fmt.Errorf(
			"%w: macro history indicator %q does not match %q", ErrInvalidResponse, response.IndicatorID, indicatorID,
		)
	}
	entries := make([]marketdata.MacroIndicatorPoint, 0, len(response.Entries))
	for _, entry := range response.Entries {
		entries = append(entries, marketdata.MacroIndicatorPoint{
			DataTime: strings.TrimSpace(entry.DataTime),
			Value:    entry.Value, PredictValue: entry.PredictValue, PreviousValue: entry.PreviousValue,
			Unit: strings.TrimSpace(entry.Unit), UnitType: entry.UnitType,
		})
	}
	return marketdata.MacroIndicatorHistoryResponse{
		IndicatorID: indicatorID, Entries: entries, Source: "akshare-macro",
	}, nil
}

// requireCalendarInstrumentID pins the identity signal for calendar entries;
// the sidecar calendar payload carries no separate market echo.
func requireCalendarInstrumentID(instrumentID, kind string, index int) (string, error) {
	instrumentID = strings.ToUpper(strings.TrimSpace(instrumentID))
	if instrumentID == "" {
		return "", fmt.Errorf(
			"%w: %s calendar entry %d instrument_id is required", ErrInvalidResponse, kind, index,
		)
	}
	return instrumentID, nil
}
