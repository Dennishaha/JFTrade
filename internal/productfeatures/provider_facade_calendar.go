package productfeatures

import (
	"context"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// embeddedCalendar routes the calendar operations the console exposes
// (apps/web/src/components/research/EarningsCalendarView.vue,
// DividendCalendarView.vue, EconCalendarView.vue, IpoCenterView.vue) to the
// embedded market-data provider. trade_dates has no embedded feed and resolves
// to a capability-unavailable error instead of falling through to broker
// routing. Calendar data is cross-market, so the market argument is envelope
// context only and never filters.
func (s *Service) embeddedCalendar(
	ctx context.Context,
	reader EmbeddedResearchReader,
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	market string,
	now time.Time,
) (*broker.FeatureResult, error) {
	operation := strings.ToLower(stringParam(query.Params, "operation"))
	switch operation {
	case "earnings":
		response, err := reader.GetEarningsCalendar(
			ctx, stringParam(query.Params, "beginDate"), stringParam(query.Params, "endDate"),
		)
		if err != nil {
			return nil, err
		}
		return projectProviderEarningsCalendar(descriptor, query, response, market, now), nil
	case "dividends":
		response, err := reader.GetDividendCalendar(ctx, stringParam(query.Params, "date"))
		if err != nil {
			return nil, err
		}
		return projectProviderDividendCalendar(descriptor, query, response, market, now), nil
	case "economic":
		response, err := reader.GetEconomicCalendar(
			ctx, stringParam(query.Params, "beginDate"), stringParam(query.Params, "endDate"),
		)
		if err != nil {
			return nil, err
		}
		return projectProviderEconomicCalendar(descriptor, query, response, market, now), nil
	case "ipos":
		response, err := reader.GetIpoCalendar(ctx)
		if err != nil {
			return nil, err
		}
		return projectProviderIpoCalendar(descriptor, query, response, market, now), nil
	default:
		return nil, errEmbeddedResearchOperation(query.FeatureID, operation)
	}
}

// embeddedMacro routes the macro operations with an embedded feed (indicator
// catalog and history) to the embedded market-data provider; fed_target_rate
// and fed_dot_plot stay capability-unavailable
// (apps/web/src/components/research/MacroResearchView.vue:68-98,139-194).
func (s *Service) embeddedMacro(
	ctx context.Context,
	reader EmbeddedResearchReader,
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	market string,
	rawPageSize int,
	now time.Time,
) (*broker.FeatureResult, error) {
	operation := strings.ToLower(stringParam(query.Params, "operation"))
	switch operation {
	case "indicators":
		response, err := reader.GetMacroIndicators(ctx)
		if err != nil {
			return nil, err
		}
		return projectProviderMacroIndicators(descriptor, query, response, market, now), nil
	case "indicator_history":
		indicatorID := stringParam(query.Params, "indicatorId")
		if indicatorID == "" {
			return nil, errEmbeddedResearchOperation(query.FeatureID, operation+" (missing indicatorId)")
		}
		response, err := reader.GetMacroIndicatorHistory(
			ctx, indicatorID, embeddedMacroHistoryLimit(query, rawPageSize),
		)
		if err != nil {
			return nil, err
		}
		return projectProviderMacroIndicatorHistory(descriptor, query, response, market, now), nil
	default:
		return nil, errEmbeddedResearchOperation(query.FeatureID, operation)
	}
}

// embeddedMacroHistoryLimit mirrors embeddedRankingsLimit with the macro
// history bounds the market-data service accepts.
func embeddedMacroHistoryLimit(query *broker.FeatureQuery, rawPageSize int) int {
	limit := rawPageSize
	if limit <= 0 {
		limit = int(int32Param(query.Params, "limit", 0))
	}
	if limit <= 0 {
		limit = marketdata.DefaultMacroHistoryLimit
	}
	return min(max(limit, 1), marketdata.MaxMacroHistoryLimit)
}
