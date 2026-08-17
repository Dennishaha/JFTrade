package productfeatures

import (
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// calendarEventLocation formats economic event date/time keys; the akshare
// economic calendar publishes CN timezones, so Asia/Shanghai is the display
// frame with UTC as the fallback when the zone database is unavailable.
var calendarEventLocation = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.UTC
	}
	return location
}()

// projectProviderEarningsCalendar keys match the earnings table at
// apps/web/src/components/research/EarningsCalendarView.vue:112-140
// (instrumentId/market/symbol/name/eventDate/periodText/marketCap/price).
func projectProviderEarningsCalendar(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.EarningsCalendarResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0, len(response.Entries))
	for _, event := range response.Entries {
		projected := map[string]any{}
		putCalendarIdentity(projected, event.InstrumentID)
		if symbol := strings.TrimSpace(event.Symbol); symbol != "" {
			projected["symbol"] = strings.ToUpper(symbol)
		}
		putCalendarText(projected, "name", event.Name)
		putCalendarText(projected, "eventDate", event.EventDate)
		putCalendarText(projected, "periodText", event.PeriodText)
		putProviderNumber(projected, "marketCap", event.MarketCap)
		putProviderNumber(projected, "price", event.Price)
		entries = append(entries, projected)
	}
	return embeddedFeatureResult(descriptor, query, "", market, now, now, entries, response.Source)
}

// projectProviderDividendCalendar keys match the dividend table at
// apps/web/src/components/research/DividendCalendarView.vue:33-66; the
// upstream payable_date maps to dividendPayableDate.
func projectProviderDividendCalendar(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.DividendCalendarResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0, len(response.Entries))
	for _, event := range response.Entries {
		projected := map[string]any{}
		putCalendarIdentity(projected, event.InstrumentID)
		if symbol := strings.TrimSpace(event.Symbol); symbol != "" {
			projected["symbol"] = strings.ToUpper(symbol)
		}
		putCalendarText(projected, "name", event.Name)
		putCalendarText(projected, "statement", event.Statement)
		putCalendarText(projected, "exDate", event.ExDate)
		putCalendarText(projected, "recordDate", event.RecordDate)
		if event.PayableDate != nil {
			putCalendarText(projected, "dividendPayableDate", *event.PayableDate)
		}
		entries = append(entries, projected)
	}
	return embeddedFeatureResult(descriptor, query, "", market, now, now, entries, response.Source)
}

// projectProviderEconomicCalendar keys match the economic event feed at
// apps/web/src/components/research/EconCalendarView.vue:94-184. eventDate and
// eventTime derive from eventTimestamp in the Asia/Shanghai display frame;
// all-day events carry only eventDate and keep eventTime absent. The akshare
// feed returns the full requested range, so hasMore stays false and no cursor
// is issued.
func projectProviderEconomicCalendar(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.EconomicCalendarResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0, len(response.Entries))
	for _, event := range response.Entries {
		projected := map[string]any{}
		putCalendarText(projected, "eventId", event.EventID)
		putCalendarText(projected, "title", event.Title)
		putCalendarText(projected, "region", event.Region)
		if event.EventTimestamp != 0 {
			projected["eventTimestamp"] = event.EventTimestamp
			moment := time.Unix(event.EventTimestamp, 0).In(calendarEventLocation)
			projected["eventDate"] = moment.Format(time.DateOnly)
			projected["eventTime"] = moment.Format("15:04")
		} else {
			putCalendarText(projected, "eventDate", event.EventDate)
		}
		if event.Importance != nil {
			projected["importance"] = *event.Importance
		}
		putOptionalCalendarText(projected, "previousValue", event.PreviousValue)
		putOptionalCalendarText(projected, "forecastValue", event.ForecastValue)
		putOptionalCalendarText(projected, "actualValue", event.ActualValue)
		entries = append(entries, projected)
	}
	return embeddedFeatureResult(descriptor, query, "", market, now, now, entries, response.Source)
}

// projectProviderIpoCalendar keys match the IPO center table at
// apps/web/src/components/research/IpoCenterView.vue:36-94; nullable pricing
// fields are omitted when the terms are not yet published.
func projectProviderIpoCalendar(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.IpoCalendarResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0, len(response.Entries))
	for _, entry := range response.Entries {
		projected := map[string]any{}
		putCalendarIdentity(projected, entry.InstrumentID)
		if symbol := strings.TrimSpace(entry.Symbol); symbol != "" {
			projected["symbol"] = strings.ToUpper(symbol)
		}
		putCalendarText(projected, "name", entry.Name)
		putCalendarText(projected, "status", entry.Status)
		putOptionalCalendarText(projected, "listingDate", entry.ListingDate)
		putProviderNumber(projected, "issueVolume", entry.IssueVolume)
		putProviderNumber(projected, "issuePrice", entry.IssuePrice)
		putProviderNumber(projected, "issuePriceMin", entry.IssuePriceMin)
		putProviderNumber(projected, "issuePriceMax", entry.IssuePriceMax)
		entries = append(entries, projected)
	}
	return embeddedFeatureResult(descriptor, query, "", market, now, now, entries, response.Source)
}

// projectProviderMacroIndicators nests indicators under their category as
// {categoryName, indicatorList:[{indicatorId,name,...}]} to match the Futu
// shape the console reads at
// apps/web/src/components/research/MacroResearchView.vue:68-98.
func projectProviderMacroIndicators(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.MacroIndicatorsResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0, len(response.Categories))
	for _, category := range response.Categories {
		indicatorList := make([]map[string]any, 0, len(category.Indicators))
		for _, indicator := range category.Indicators {
			projected := map[string]any{}
			putCalendarText(projected, "indicatorId", indicator.IndicatorID)
			putCalendarText(projected, "name", indicator.Name)
			putCalendarText(projected, "region", indicator.Region)
			putCalendarText(projected, "unit", indicator.Unit)
			putProviderNumber(projected, "unitType", indicator.UnitType)
			putCalendarText(projected, "frequency", indicator.Frequency)
			indicatorList = append(indicatorList, projected)
		}
		entries = append(entries, map[string]any{
			"categoryName":  category.CategoryName,
			"indicatorList": indicatorList,
		})
	}
	return embeddedFeatureResult(descriptor, query, "", market, now, now, entries, response.Source)
}

// projectProviderMacroIndicatorHistory keys match the history chart at
// apps/web/src/components/research/MacroResearchView.vue:139-194; nil
// predict/previous values are omitted.
func projectProviderMacroIndicatorHistory(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.MacroIndicatorHistoryResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0, len(response.Entries))
	for _, point := range response.Entries {
		projected := map[string]any{}
		putCalendarText(projected, "dataTime", point.DataTime)
		putProviderNumber(projected, "value", point.Value)
		putProviderNumber(projected, "predictValue", point.PredictValue)
		putProviderNumber(projected, "previousValue", point.PreviousValue)
		putCalendarText(projected, "unit", point.Unit)
		putProviderNumber(projected, "unitType", point.UnitType)
		entries = append(entries, projected)
	}
	return embeddedFeatureResult(descriptor, query, "", market, now, now, entries, response.Source)
}

// putCalendarIdentity injects the uppercased instrumentId plus the market and
// symbol split from its prefix, mirroring rankingEntryDocuments.
func putCalendarIdentity(document map[string]any, instrumentID string) {
	id := strings.ToUpper(strings.TrimSpace(instrumentID))
	if id == "" {
		return
	}
	document["instrumentId"] = id
	if prefix, code, found := strings.Cut(id, "."); found {
		document["market"] = prefix
		document["symbol"] = code
	}
}

func putCalendarText(document map[string]any, key, value string) {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		document[key] = trimmed
	}
}

func putOptionalCalendarText(document map[string]any, key string, value *string) {
	if value != nil {
		putCalendarText(document, key, *value)
	}
}
