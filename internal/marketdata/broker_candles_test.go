package marketdata

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestBrokerKLineCandlesResponseProjectsStrictPage(t *testing.T) {
	first := time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	response, err := BrokerKLineCandlesResponse(
		"US", "AAPL", "US.AAPL", "1h", 2,
		HistoricalCandlesQuery{Limit: 2, BeforeTime: second.Add(time.Hour).Format(time.RFC3339Nano)},
		[]CandleSession{CandleSessionRegular}, true,
		&broker.KLineSnapshot{
			KLines: []broker.KLineItem{
				brokerKLineItem(first, 100, "regular"),
				brokerKLineItem(second, 101, "regular"),
			},
			Pagination: broker.KLinePagination{HasMore: true, NextBefore: first.Format(time.RFC3339Nano)},
		},
		" broker:futu ",
	)
	if err != nil {
		t.Fatalf("BrokerKLineCandlesResponse: %v", err)
	}
	if response["totalReturned"] != 2 {
		t.Fatalf("totalReturned = %#v", response["totalReturned"])
	}
	candles, ok := response["candles"].([]map[string]any)
	if !ok || len(candles) != 2 {
		t.Fatalf("candles = %#v", response["candles"])
	}
	if candles[0]["at"] != first.Format(time.RFC3339Nano) || candles[0]["close"] != "100" || candles[0]["session"] != "regular" {
		t.Fatalf("first candle = %#v", candles[0])
	}
	pagination, ok := response["pagination"].(map[string]any)
	if !ok || pagination["hasMore"] != true || pagination["nextBefore"] != first.Format(time.RFC3339Nano) {
		t.Fatalf("pagination = %#v", response["pagination"])
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok || meta["source"] != "broker:futu" || meta["extendedHours"] != false || meta["session"] != "regular" {
		t.Fatalf("meta = %#v", response["meta"])
	}
}

func TestBrokerKLineCandlesResponseHandlesTerminalAndBoundedPages(t *testing.T) {
	at := time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		request HistoricalCandlesQuery
	}{
		{name: "terminal", request: HistoricalCandlesQuery{Limit: 1}},
		{name: "bounded", request: HistoricalCandlesQuery{Limit: 1, FromTime: at.Add(-time.Hour).Format(time.RFC3339Nano)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := BrokerKLineCandlesResponse(
				"US", "AAPL", "US.AAPL", "1h", 1, test.request,
				[]CandleSession{CandleSessionRegular}, false,
				&broker.KLineSnapshot{KLines: []broker.KLineItem{brokerKLineItem(at, 100, "")}}, "futu",
			)
			if err != nil {
				t.Fatalf("BrokerKLineCandlesResponse: %v", err)
			}
			pagination := response["pagination"].(map[string]any)
			if pagination["hasMore"] != false || len(pagination) != 1 {
				t.Fatalf("pagination = %#v", pagination)
			}
		})
	}
}

func TestBrokerKLineCandlesResponseRejectsInvalidProviderRows(t *testing.T) {
	at := time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC)
	valid := brokerKLineItem(at, 100, "regular")
	for _, test := range []struct {
		name     string
		request  HistoricalCandlesQuery
		snapshot *broker.KLineSnapshot
		want     string
	}{
		{name: "nil snapshot", snapshot: nil, want: "empty K-line snapshot"},
		{name: "invalid before", request: HistoricalCandlesQuery{BeforeTime: "not-a-time"}, snapshot: &broker.KLineSnapshot{}, want: "invalid K-line before cursor"},
		{name: "cursor not strict", request: HistoricalCandlesQuery{BeforeTime: at.Format(time.RFC3339Nano)}, snapshot: &broker.KLineSnapshot{KLines: []broker.KLineItem{valid}}, want: "at or after the before cursor"},
		{name: "unordered", snapshot: &broker.KLineSnapshot{KLines: []broker.KLineItem{valid, brokerKLineItem(at, 101, "regular")}}, want: "not strictly ordered"},
		{name: "invalid time", snapshot: &broker.KLineSnapshot{KLines: []broker.KLineItem{{Time: "bad"}}}, want: "invalid K-line time"},
		{name: "missing number", snapshot: &broker.KLineSnapshot{KLines: []broker.KLineItem{{Time: at.Format(time.RFC3339Nano)}}}, want: "missing or non-finite"},
		{name: "non finite", snapshot: &broker.KLineSnapshot{KLines: []broker.KLineItem{brokerKLineItem(at, math.NaN(), "regular")}}, want: "missing or non-finite"},
		{name: "closed session", snapshot: &broker.KLineSnapshot{KLines: []broker.KLineItem{brokerKLineItem(time.Date(2026, time.July, 18, 14, 0, 0, 0, time.UTC), 100, "regular")}}, want: "unable to classify K-line session"},
		{name: "unrequested session", snapshot: &broker.KLineSnapshot{KLines: []broker.KLineItem{brokerKLineItem(time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC), 100, "pre")}}, want: "was not requested"},
		{name: "unknown session", snapshot: &broker.KLineSnapshot{KLines: []broker.KLineItem{brokerKLineItem(at, 100, "mystery")}}, want: "unable to classify K-line session"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := BrokerKLineCandlesResponse(
				"US", "AAPL", "US.AAPL", "1h", 1, test.request,
				[]CandleSession{CandleSessionRegular}, true, test.snapshot, "futu",
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("BrokerKLineCandlesResponse error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBrokerKLineHelpersClassifySessionsAndNumbers(t *testing.T) {
	at := time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC)
	if got, err := historicalCandleBeforeTime(""); err != nil || !got.IsZero() {
		t.Fatalf("empty before = %v, %v", got, err)
	}
	if got, err := historicalCandleBeforeTime("2026-07-15T22:00:00+08:00"); err != nil || got.Location() != time.UTC {
		t.Fatalf("parsed before = %v, %v", got, err)
	}
	if got, group, err := brokerKLineSession("", "US.AAPL", at); err != nil || got != "regular" || group != CandleSessionRegular {
		t.Fatalf("default session = %q, %q, %v", got, group, err)
	}
	if got, group, err := brokerKLineSession("after", "US.AAPL", at); err != nil || got != "after" || group != CandleSessionExtended {
		t.Fatalf("explicit session = %q, %q, %v", got, group, err)
	}
	value := 12.5
	if got, err := brokerKLineNumber(&value, "close"); err != nil || got != "12.5" {
		t.Fatalf("brokerKLineNumber = %q, %v", got, err)
	}
}

func brokerKLineItem(at time.Time, close float64, session string) broker.KLineItem {
	open, high, low, volume := close-0.5, close+1, close-1, 1000.0
	return broker.KLineItem{
		Time: at.Format(time.RFC3339Nano), Open: &open, High: &high, Low: &low,
		Close: &close, Volume: &volume, Session: session,
	}
}

func TestBrokerKLinePaginationRejectsInvalidBoundedAndPagedMetadata(t *testing.T) {
	candles := []map[string]any{{
		"at": "2026-07-15T01:00:00Z", "open": "100", "high": "102",
		"low": "99", "close": "101.5", "volume": "1000",
	}}
	twoCandles := append(append([]map[string]any(nil), candles...), map[string]any{
		"at": "2026-07-15T02:00:00Z", "open": "101", "high": "103",
		"low": "100", "close": "102.5", "volume": "1100",
	})

	for _, test := range []struct {
		name       string
		candles    []map[string]any
		pagination broker.KLinePagination
		request    HistoricalCandlesQuery
		want       string
	}{
		{
			name: "bounded has more", candles: candles,
			pagination: broker.KLinePagination{HasMore: true, NextBefore: "2026-07-15T01:00:00Z"},
			request: HistoricalCandlesQuery{
				FromTime: "2026-07-14T00:00:00Z", ToTime: "2026-07-16T00:00:00Z", Limit: 1,
			},
			want: "bounded K-line query returned hasMore=true",
		},
		{
			name: "bounded cursor", candles: candles,
			pagination: broker.KLinePagination{NextBefore: "2026-07-15T01:00:00Z"},
			request:    HistoricalCandlesQuery{FromTime: "2026-07-14T00:00:00Z", Limit: 1},
			want:       "bounded K-line query contains nextBefore",
		},
		{
			name:       "page exceeds limit",
			candles:    twoCandles,
			pagination: broker.KLinePagination{HasMore: false},
			request:    HistoricalCandlesQuery{Limit: 1},
			want:       "k-line page exceeds the requested limit",
		},
		{
			name:       "bounded page exceeds limit",
			candles:    twoCandles,
			pagination: broker.KLinePagination{HasMore: false},
			request: HistoricalCandlesQuery{
				FromTime: "2026-07-14T00:00:00Z", ToTime: "2026-07-16T00:00:00Z", Limit: 1,
			},
			want: "k-line page exceeds the requested limit",
		},
		{
			name:       "missing paged cursor",
			candles:    candles,
			pagination: broker.KLinePagination{HasMore: true},
			request:    HistoricalCandlesQuery{Limit: 1},
			want:       "paged K-line response is missing its cursor",
		},
		{
			name:       "terminal cursor",
			candles:    candles,
			pagination: broker.KLinePagination{NextBefore: "2026-07-15T01:00:00Z"},
			request:    HistoricalCandlesQuery{Limit: 1},
			want:       "terminal K-line page contains nextBefore",
		},
		{
			name:       "invalid next cursor",
			candles:    candles,
			pagination: broker.KLinePagination{HasMore: true, NextBefore: "invalid"},
			request:    HistoricalCandlesQuery{Limit: 1},
			want:       "invalid K-line nextBefore",
		},
		{
			name:       "mismatched next cursor",
			candles:    candles,
			pagination: broker.KLinePagination{HasMore: true, NextBefore: "2026-07-15T02:00:00Z"},
			request:    HistoricalCandlesQuery{Limit: 1},
			want:       "nextBefore does not equal the earliest candle",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := brokerKLinePagination(test.pagination, test.candles, test.request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("brokerKLinePagination error = %v, want %q", err, test.want)
			}
		})
	}
}
