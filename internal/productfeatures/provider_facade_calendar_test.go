package productfeatures

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func calendarMacroBrokerAdapter() *featureBroker {
	return &featureBroker{
		id:       "futu",
		features: []broker.FeatureID{broker.FeatureResearchCalendar, broker.FeatureResearchMacro},
	}
}

func newEmbeddedCalendarService(adapter *featureBroker, reader *embeddedReaderStub) *Service {
	registry := broker.NewRegistry()
	registry.Register(adapter)
	return NewService(registry, adapter.id, nil, nil,
		WithEmbeddedProviderResearch(
			func() EmbeddedResearchReader { return reader },
			activeProviderStub(marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"}),
		),
	)
}

// The operation strings are the exact values the frontend calendar views send;
// market is accepted but never filters (calendar data is cross-market).
func TestEmbeddedProviderServesCalendarOperations(t *testing.T) {
	price := json.Number("1680.5")
	reader := &embeddedReaderStub{
		earningsResult: marketdata.EarningsCalendarResponse{
			Source: "akshare-calendar",
			Entries: []marketdata.EarningsEvent{{
				InstrumentID: "SH.600519", Name: "贵州茅台", Symbol: "600519",
				EventDate: "2026-08-20", PeriodText: "2025中报", Price: &price,
			}},
		},
		dividendResult: marketdata.DividendCalendarResponse{
			Source: "akshare-calendar",
			Entries: []marketdata.DividendEvent{{
				InstrumentID: "SZ.000001", Statement: "10派2元", ExDate: "2026-08-15",
			}},
		},
		economicResult: marketdata.EconomicCalendarResponse{
			Source: "akshare-calendar",
			Entries: []marketdata.EconomicEvent{{
				EventID: "econ-1", Title: "CPI同比", Region: "中国", EventTimestamp: 1787000000,
			}},
		},
		ipoResult: marketdata.IpoCalendarResponse{
			Source: "akshare-calendar",
			Entries: []marketdata.IpoEntry{{
				InstrumentID: "SZ.301999", Name: "新股示例", Status: "pending",
			}},
		},
	}
	adapter := calendarMacroBrokerAdapter()
	svc := newEmbeddedCalendarService(adapter, reader)

	cases := []struct {
		name      string
		operation string
		params    map[string]any
		calls     func() int
		check     func(t *testing.T, result *broker.FeatureResult)
	}{
		{
			name: "earnings", operation: "earnings",
			params: map[string]any{"beginDate": "2026-08-01", "endDate": "2026-08-31"},
			calls:  func() int { return reader.earningsCalls },
			check: func(t *testing.T, result *broker.FeatureResult) {
				entry := result.Entries[0]
				if entry["instrumentId"] != "SH.600519" || entry["market"] != "SH" ||
					entry["symbol"] != "600519" || entry["price"] != price {
					t.Fatalf("earnings entry = %#v", entry)
				}
			},
		},
		{
			name: "dividends", operation: "dividends",
			params: map[string]any{"date": "2026-08-15"},
			calls:  func() int { return reader.dividendCalls },
			check: func(t *testing.T, result *broker.FeatureResult) {
				if result.Entries[0]["statement"] != "10派2元" {
					t.Fatalf("dividend entry = %#v", result.Entries[0])
				}
			},
		},
		{
			name: "economic", operation: "economic",
			params: map[string]any{"beginDate": "2026-08-01", "endDate": "2026-08-07"},
			calls:  func() int { return reader.economicCalls },
			check: func(t *testing.T, result *broker.FeatureResult) {
				entry := result.Entries[0]
				if entry["eventId"] != "econ-1" || entry["eventTimestamp"] != int64(1787000000) {
					t.Fatalf("economic entry = %#v", entry)
				}
				if result.HasMore == nil || *result.HasMore || result.NextCursor != "" {
					t.Fatalf("economic pagination must stay flat: %#v", result.HasMore)
				}
			},
		},
		{
			name: "ipos", operation: "ipos", params: map[string]any{},
			calls: func() int { return reader.ipoCalls },
			check: func(t *testing.T, result *broker.FeatureResult) {
				if result.Entries[0]["status"] != "pending" {
					t.Fatalf("ipo entry = %#v", result.Entries[0])
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := broker.FeatureQuery{
				Market: "CN", FeatureID: broker.FeatureResearchCalendar,
				Params: map[string]any{"operation": tc.operation},
			}
			for key, value := range tc.params {
				query.Params[key] = value
			}
			result, err := svc.Query(t.Context(), query)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if tc.calls() != 1 || adapter.queryCalls != 0 {
				t.Fatalf("reader calls = %d, broker calls = %d", tc.calls(), adapter.queryCalls)
			}
			if result.ResolvedInstrument != nil {
				t.Fatalf("calendar result must not resolve an instrument: %#v", result.ResolvedInstrument)
			}
			tc.check(t, result)
		})
	}
	if reader.earningsBegin != "2026-08-01" || reader.earningsEnd != "2026-08-31" ||
		reader.dividendDate != "2026-08-15" {
		t.Fatalf("calendar params forwarded = %q/%q/%q",
			reader.earningsBegin, reader.earningsEnd, reader.dividendDate)
	}
}

func TestEmbeddedProviderServesMacroOperations(t *testing.T) {
	unitType := json.Number("1")
	value := json.Number("0.5")
	reader := &embeddedReaderStub{
		indicatorsResult: marketdata.MacroIndicatorsResponse{
			Source: "akshare-macro",
			Categories: []marketdata.MacroIndicatorCategory{{
				CategoryName: "价格",
				Indicators: []marketdata.MacroIndicator{{
					IndicatorID: "cpi_yoy", Name: "CPI同比", Region: "中国", UnitType: &unitType,
				}},
			}},
		},
		historyResult: marketdata.MacroIndicatorHistoryResponse{
			IndicatorID: "cpi_yoy", Source: "akshare-macro",
			Entries: []marketdata.MacroIndicatorPoint{{DataTime: "2026-07", Value: &value, UnitType: &unitType}},
		},
	}
	adapter := calendarMacroBrokerAdapter()
	svc := newEmbeddedCalendarService(adapter, reader)

	indicators, err := svc.Query(t.Context(), broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeatureResearchMacro,
		Params: map[string]any{"operation": "indicators"},
	})
	if err != nil {
		t.Fatalf("indicators query: %v", err)
	}
	entry := indicators.Entries[0]
	if entry["categoryName"] != "价格" {
		t.Fatalf("indicator category = %#v", entry)
	}
	list, ok := entry["indicatorList"].([]map[string]any)
	if !ok || len(list) != 1 || list[0]["indicatorId"] != "cpi_yoy" {
		t.Fatalf("indicatorList = %#v", entry["indicatorList"])
	}

	history, err := svc.Query(t.Context(), broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeatureResearchMacro,
		Params:   map[string]any{"operation": "indicator_history", "indicatorId": "cpi_yoy"},
		PageSize: 60,
	})
	if err != nil {
		t.Fatalf("history query: %v", err)
	}
	if reader.historyID != "cpi_yoy" || reader.historyLimit != 60 {
		t.Fatalf("history read = %q/%d", reader.historyID, reader.historyLimit)
	}
	if history.Entries[0]["dataTime"] != "2026-07" || history.Entries[0]["value"] != value {
		t.Fatalf("history entry = %#v", history.Entries[0])
	}
	if adapter.queryCalls != 0 {
		t.Fatalf("macro leaked to the broker: %d calls", adapter.queryCalls)
	}
}

// trade_dates and the fed_* operations have no embedded feed: they must answer
// capability-unavailable instead of falling through to broker routing.
func TestEmbeddedProviderRejectsUnsupportedCalendarMacroOperations(t *testing.T) {
	cases := []struct {
		feature   broker.FeatureID
		operation string
	}{
		{broker.FeatureResearchCalendar, "trade_dates"},
		{broker.FeatureResearchCalendar, ""},
		{broker.FeatureResearchMacro, "fed_target_rate"},
		{broker.FeatureResearchMacro, "fed_dot_plot"},
		{broker.FeatureResearchMacro, "indicator_history"}, // missing indicatorId
		{broker.FeatureResearchMacro, ""},
	}
	for _, tc := range cases {
		adapter := calendarMacroBrokerAdapter()
		reader := &embeddedReaderStub{}
		svc := newEmbeddedCalendarService(adapter, reader)
		params := map[string]any{}
		if tc.operation != "" {
			params["operation"] = tc.operation
		}
		_, err := svc.Query(t.Context(), broker.FeatureQuery{
			Market: "CN", FeatureID: tc.feature, Params: params,
		})
		if !errors.Is(err, ErrCapabilityUnavailable) {
			t.Fatalf("%s operation %q error = %v", tc.feature, tc.operation, err)
		}
		if adapter.queryCalls != 0 {
			t.Fatalf("%s operation %q leaked to the broker", tc.feature, tc.operation)
		}
	}
}

func TestEmbeddedProviderPropagatesCalendarMacroErrors(t *testing.T) {
	adapter := calendarMacroBrokerAdapter()
	reader := &embeddedReaderStub{
		ipoErr: fmt.Errorf("%w: active provider %q does not support event calendars",
			marketdata.ErrCapabilityUnsupported, "yfinance"),
	}
	svc := newEmbeddedCalendarService(adapter, reader)
	_, err := svc.Query(t.Context(), broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeatureResearchCalendar,
		Params: map[string]any{"operation": "ipos"},
	})
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("unsupported calendar error = %v", err)
	}

	reader = &embeddedReaderStub{indicatorsErr: marketdata.ErrProviderBusy}
	svc = newEmbeddedCalendarService(adapter, reader)
	if _, err = svc.Query(t.Context(), broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeatureResearchMacro,
		Params: map[string]any{"operation": "indicators"},
	}); !errors.Is(err, marketdata.ErrProviderBusy) {
		t.Fatalf("busy error = %v", err)
	}
}

func TestEmbeddedProviderCalendarMacroStayOnBrokerPathForFutu(t *testing.T) {
	adapter := calendarMacroBrokerAdapter()
	reader := &embeddedReaderStub{}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	svc := NewService(registry, adapter.id, nil, nil,
		WithEmbeddedProviderResearch(
			func() EmbeddedResearchReader { return reader },
			activeProviderStub(marketdata.ProviderDescriptor{BrokerID: "futu", ProviderID: "futu"}),
		),
	)
	if _, err := svc.Query(t.Context(), broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeatureResearchMacro,
		Params: map[string]any{"operation": "indicators"},
	}); err != nil {
		t.Fatalf("futu-active macro query: %v", err)
	}
	if adapter.queryCalls != 1 || reader.indicatorsCalls != 0 {
		t.Fatalf("broker calls = %d, reader calls = %d", adapter.queryCalls, reader.indicatorsCalls)
	}
}
