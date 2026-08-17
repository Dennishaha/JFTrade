package productfeatures

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// Calendar entry keys are consumed by the console calendar views:
// apps/web/src/components/research/EarningsCalendarView.vue:112-140,
// DividendCalendarView.vue:33-66, EconCalendarView.vue:94-184,
// IpoCenterView.vue:36-94.
func TestProviderEarningsCalendarProjectionMapsFrontendKeys(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	number := func(value string) *json.Number { n := json.Number(value); return &n }
	response := marketdata.EarningsCalendarResponse{
		Source: "akshare-calendar",
		Entries: []marketdata.EarningsEvent{
			{
				InstrumentID: "SH.600519", Name: "贵州茅台", Symbol: "600519",
				EventDate: "2026-08-20", PeriodText: "2025中报",
				MarketCap: number("2.1e12"), Price: number("1680.5"),
			},
			{InstrumentID: "SZ.000001", Name: "平安银行", EventDate: "2026-08-21"},
		},
	}
	result := projectProviderEarningsCalendar(
		marketdata.ProviderDescriptor{BrokerID: "akshare"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchCalendar},
		response, "CN", now,
	)
	if len(result.Entries) != 2 {
		t.Fatalf("entries = %#v", result.Entries)
	}
	first := result.Entries[0]
	for _, key := range []string{
		"instrumentId", "market", "symbol", "name", "eventDate", "periodText", "marketCap", "price",
	} {
		if _, ok := first[key]; !ok {
			t.Fatalf("first entry missing key %q: %#v", key, first)
		}
	}
	if first["instrumentId"] != "SH.600519" || first["market"] != "SH" || first["symbol"] != "600519" ||
		first["marketCap"] != json.Number("2.1e12") {
		t.Fatalf("first entry = %#v", first)
	}
	second := result.Entries[1]
	for _, key := range []string{"marketCap", "price", "periodText"} {
		if _, ok := second[key]; ok {
			t.Fatalf("nil/empty field %q must be omitted: %#v", key, second)
		}
	}
	if result.ResolvedInstrument != nil || result.Total == nil || *result.Total != 2 ||
		result.HasMore == nil || *result.HasMore {
		t.Fatalf("envelope instrument=%#v Total=%#v HasMore=%#v",
			result.ResolvedInstrument, result.Total, result.HasMore)
	}
}

func TestProviderDividendCalendarProjectionMapsFrontendKeys(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	payable := "2026-08-22"
	response := marketdata.DividendCalendarResponse{
		Source: "akshare-calendar",
		Entries: []marketdata.DividendEvent{
			{
				InstrumentID: "SH.600519", Name: "贵州茅台", Symbol: "600519",
				Statement: "10派308.76元", ExDate: "2026-08-15", RecordDate: "2026-08-14",
				PayableDate: &payable,
			},
			{InstrumentID: "SZ.000001", Statement: "10派2元", ExDate: "2026-08-15"},
		},
	}
	result := projectProviderDividendCalendar(
		marketdata.ProviderDescriptor{BrokerID: "akshare"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchCalendar},
		response, "CN", now,
	)
	first := result.Entries[0]
	for _, key := range []string{
		"instrumentId", "name", "symbol", "statement", "exDate", "recordDate", "dividendPayableDate",
	} {
		if _, ok := first[key]; !ok {
			t.Fatalf("first entry missing key %q: %#v", key, first)
		}
	}
	if first["dividendPayableDate"] != "2026-08-22" {
		t.Fatalf("dividendPayableDate = %#v", first)
	}
	if _, ok := result.Entries[1]["dividendPayableDate"]; ok {
		t.Fatalf("nil payable date must be omitted: %#v", result.Entries[1])
	}
}

func TestProviderEconomicCalendarProjectionDerivesDateAndTime(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	importance := 3
	previous := "0.3%"
	response := marketdata.EconomicCalendarResponse{
		Source: "akshare-calendar",
		Entries: []marketdata.EconomicEvent{
			{
				EventID: "econ-1", Title: "中国7月CPI同比", Region: "中国",
				EventTimestamp: 1787004000, Importance: &importance, PreviousValue: &previous,
			},
			{EventID: "econ-2", Title: "无时间事件", EventDate: "2026-08-20"},
		},
	}
	result := projectProviderEconomicCalendar(
		marketdata.ProviderDescriptor{BrokerID: "akshare"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchCalendar},
		response, "SH", now,
	)
	first := result.Entries[0]
	for _, key := range []string{
		"eventId", "title", "region", "eventTimestamp", "eventDate", "eventTime",
		"importance", "previousValue",
	} {
		if _, ok := first[key]; !ok {
			t.Fatalf("first entry missing key %q: %#v", key, first)
		}
	}
	wantMoment := time.Unix(1787004000, 0).In(calendarEventLocation)
	if first["eventDate"] != wantMoment.Format(time.DateOnly) ||
		first["eventTime"] != wantMoment.Format("15:04") {
		t.Fatalf("derived date/time = %#v", first)
	}
	if first["importance"] != 3 {
		t.Fatalf("importance = %#v", first["importance"])
	}
	for _, key := range []string{"forecastValue", "actualValue"} {
		if _, ok := first[key]; ok {
			t.Fatalf("nil field %q must be omitted: %#v", key, first)
		}
	}
	second := result.Entries[1]
	for _, key := range []string{"eventTimestamp", "eventTime", "importance"} {
		if _, ok := second[key]; ok {
			t.Fatalf("all-day event must omit %q: %#v", key, second)
		}
	}
	if second["eventDate"] != "2026-08-20" {
		t.Fatalf("all-day event date = %#v", second["eventDate"])
	}
	if result.HasMore == nil || *result.HasMore || result.NextCursor != "" {
		t.Fatalf("economic calendar returns the full range: HasMore=%#v cursor=%q",
			result.HasMore, result.NextCursor)
	}
}

func TestProviderIpoCalendarProjectionMapsFrontendKeys(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	number := func(value string) *json.Number { n := json.Number(value); return &n }
	listing := "2026-08-25"
	response := marketdata.IpoCalendarResponse{
		Source: "akshare-calendar",
		Entries: []marketdata.IpoEntry{
			{
				InstrumentID: "SZ.301999", Name: "新股示例", Symbol: "301999", Status: "pending",
				IssueVolume: number("4000"), IssuePriceMin: number("12.5"), IssuePriceMax: number("15"),
			},
			{InstrumentID: "SH.688999", Name: "已上市", Status: "listed", ListingDate: &listing},
		},
	}
	result := projectProviderIpoCalendar(
		marketdata.ProviderDescriptor{BrokerID: "akshare"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchCalendar},
		response, "CN", now,
	)
	first := result.Entries[0]
	for _, key := range []string{
		"instrumentId", "name", "symbol", "status", "issueVolume", "issuePriceMin", "issuePriceMax",
	} {
		if _, ok := first[key]; !ok {
			t.Fatalf("pending entry missing key %q: %#v", key, first)
		}
	}
	for _, key := range []string{"listingDate", "issuePrice"} {
		if _, ok := first[key]; ok {
			t.Fatalf("nil field %q must be omitted: %#v", key, first)
		}
	}
	if result.Entries[1]["listingDate"] != "2026-08-25" {
		t.Fatalf("listed entry = %#v", result.Entries[1])
	}
}

// Macro keys are consumed by apps/web/src/components/research/
// MacroResearchView.vue:68-98 (categoryName + nested indicatorList) and
// :139-194 (dataTime/value/predictValue/previousValue/unit/unitType).
func TestProviderMacroIndicatorsProjectionNestsIndicatorList(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	unitType := json.Number("1")
	response := marketdata.MacroIndicatorsResponse{
		Source: "akshare-macro",
		Categories: []marketdata.MacroIndicatorCategory{
			{
				CategoryName: "价格",
				Indicators: []marketdata.MacroIndicator{
					{IndicatorID: "cpi_yoy", Name: "CPI同比", Region: "中国", Unit: "%", UnitType: &unitType, Frequency: "月"},
					{IndicatorID: "ppi_yoy", Name: "PPI同比"},
				},
			},
			{CategoryName: "景气", Indicators: []marketdata.MacroIndicator{}},
		},
	}
	result := projectProviderMacroIndicators(
		marketdata.ProviderDescriptor{BrokerID: "akshare"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchMacro},
		response, "US", now,
	)
	if len(result.Entries) != 2 {
		t.Fatalf("entries = %#v", result.Entries)
	}
	first := result.Entries[0]
	if first["categoryName"] != "价格" {
		t.Fatalf("category entry = %#v", first)
	}
	list, ok := first["indicatorList"].([]map[string]any)
	if !ok || len(list) != 2 {
		t.Fatalf("indicatorList = %#v", first["indicatorList"])
	}
	if list[0]["indicatorId"] != "cpi_yoy" || list[0]["name"] != "CPI同比" ||
		list[0]["region"] != "中国" || list[0]["unitType"] != json.Number("1") ||
		list[0]["frequency"] != "月" {
		t.Fatalf("indicator = %#v", list[0])
	}
	if _, ok := list[1]["unitType"]; ok {
		t.Fatalf("nil unitType must be omitted: %#v", list[1])
	}
	if empty, ok := result.Entries[1]["indicatorList"].([]map[string]any); !ok || len(empty) != 0 {
		t.Fatalf("empty category must project empty indicatorList: %#v", result.Entries[1])
	}
	if result.Total == nil || *result.Total != 2 {
		t.Fatalf("Total = %#v", result.Total)
	}
}

func TestProviderMacroIndicatorHistoryProjectionMapsFrontendKeys(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	number := func(value string) *json.Number { n := json.Number(value); return &n }
	response := marketdata.MacroIndicatorHistoryResponse{
		IndicatorID: "cpi_yoy", Source: "akshare-macro",
		Entries: []marketdata.MacroIndicatorPoint{
			{DataTime: "2026-07", Value: number("0.5"), PreviousValue: number("0.3"), Unit: "%", UnitType: number("1")},
			{DataTime: "2026-08", Value: number("0.6"), PredictValue: number("0.55")},
		},
	}
	result := projectProviderMacroIndicatorHistory(
		marketdata.ProviderDescriptor{BrokerID: "akshare"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchMacro},
		response, "US", now,
	)
	first := result.Entries[0]
	for _, key := range []string{"dataTime", "value", "previousValue", "unit", "unitType"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("first point missing key %q: %#v", key, first)
		}
	}
	if _, ok := first["predictValue"]; ok {
		t.Fatalf("nil predictValue must be omitted: %#v", first)
	}
	second := result.Entries[1]
	if second["predictValue"] != json.Number("0.55") {
		t.Fatalf("second point = %#v", second)
	}
	for _, key := range []string{"previousValue", "unit", "unitType"} {
		if _, ok := second[key]; ok {
			t.Fatalf("absent field %q must be omitted: %#v", key, second)
		}
	}
	if result.Metadata["source"] != "akshare-macro" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}
