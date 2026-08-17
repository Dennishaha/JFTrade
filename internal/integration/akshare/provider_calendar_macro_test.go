package akshare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestClientCalendarMacroEndpointsEncodePathsAndQuery(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/providers/akshare/calendar/earnings":
			_ = json.NewEncoder(writer).Encode(map[string]any{"entries": []map[string]any{}})
		case "/providers/akshare/calendar/dividends":
			_ = json.NewEncoder(writer).Encode(map[string]any{"entries": []map[string]any{}})
		case "/providers/akshare/calendar/economic":
			_ = json.NewEncoder(writer).Encode(map[string]any{"entries": []map[string]any{}})
		case "/providers/akshare/calendar/ipos":
			_ = json.NewEncoder(writer).Encode(map[string]any{"entries": []map[string]any{}})
		case "/providers/akshare/macro/indicators":
			_ = json.NewEncoder(writer).Encode(map[string]any{"categories": []map[string]any{}})
		case "/providers/akshare/macro/indicator-history":
			_ = json.NewEncoder(writer).Encode(map[string]any{"indicator_id": "cpi_yoy", "entries": []map[string]any{}})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	if _, err := client.earningsCalendar(ctx, "2026-08-01", "2026-08-31"); err != nil {
		t.Fatalf("earningsCalendar: %v", err)
	}
	if _, err := client.dividendCalendar(ctx, "2026-08-15"); err != nil {
		t.Fatalf("dividendCalendar: %v", err)
	}
	if _, err := client.economicCalendar(ctx, "", ""); err != nil {
		t.Fatalf("economicCalendar: %v", err)
	}
	if _, err := client.ipoCalendar(ctx); err != nil {
		t.Fatalf("ipoCalendar: %v", err)
	}
	if _, err := client.macroIndicators(ctx); err != nil {
		t.Fatalf("macroIndicators: %v", err)
	}
	if _, err := client.macroIndicatorHistory(ctx, "cpi_yoy", 120); err != nil {
		t.Fatalf("macroIndicatorHistory: %v", err)
	}

	seen := requests()
	wantPaths := []string{
		"/providers/akshare/calendar/earnings", "/providers/akshare/calendar/dividends",
		"/providers/akshare/calendar/economic", "/providers/akshare/calendar/ipos",
		"/providers/akshare/macro/indicators", "/providers/akshare/macro/indicator-history",
	}
	if len(seen) != len(wantPaths) {
		t.Fatalf("requests = %#v", seen)
	}
	for index, want := range wantPaths {
		if seen[index].path != want {
			t.Fatalf("request %d path = %q, want %q", index, seen[index].path, want)
		}
	}
	if got := seen[0].query.Get("begin_date"); got != "2026-08-01" || seen[0].query.Get("end_date") != "2026-08-31" {
		t.Fatalf("earnings query = %v", seen[0].query)
	}
	if got := seen[1].query.Get("date"); got != "2026-08-15" {
		t.Fatalf("dividends query = %v", seen[1].query)
	}
	if got := seen[5].query.Get("indicator_id"); got != "cpi_yoy" || seen[5].query.Get("limit") != "120" {
		t.Fatalf("history query = %v", seen[5].query)
	}
}

func TestProviderCalendarConvertsEntriesAndKeepsNulls(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/providers/akshare/calendar/earnings":
			_ = json.NewEncoder(writer).Encode(map[string]any{"entries": []map[string]any{{
				"instrument_id": "sh.600519", "name": "贵州茅台", "symbol": "600519",
				"event_date": "2026-08-20", "period_text": "2025中报", "market_cap": nil, "price": 1680.5,
			}}})
		case "/providers/akshare/calendar/dividends":
			_ = json.NewEncoder(writer).Encode(map[string]any{"entries": []map[string]any{{
				"instrument_id": "SZ.000001", "name": "平安银行", "symbol": "000001",
				"statement": "10派2元", "ex_date": "2026-08-15", "record_date": "2026-08-14", "payable_date": nil,
			}}})
		case "/providers/akshare/calendar/economic":
			_ = json.NewEncoder(writer).Encode(map[string]any{"entries": []map[string]any{{
				"event_id": "econ-1", "title": "CPI同比", "region": "中国",
				"event_date": "2026-08-20", "event_timestamp": 1787000000,
				"importance": 3, "previous_value": "0.3%", "forecast_value": nil, "actual_value": nil,
			}}})
		case "/providers/akshare/calendar/ipos":
			_ = json.NewEncoder(writer).Encode(map[string]any{"entries": []map[string]any{{
				"instrument_id": "SZ.301999", "name": "新股示例", "symbol": "301999", "status": "pending",
				"listing_date": nil, "issue_volume": 4000, "issue_price": nil,
				"issue_price_min": 12.5, "issue_price_max": 15.0,
			}}})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()

	earnings, err := provider.EarningsCalendar(ctx, "2026-08-01", "2026-08-31")
	if err != nil || len(earnings.Entries) != 1 {
		t.Fatalf("earnings = %#v, err=%v", earnings, err)
	}
	event := earnings.Entries[0]
	if event.InstrumentID != "SH.600519" || event.EventDate != "2026-08-20" ||
		event.MarketCap != nil || event.Price == nil || *event.Price != "1680.5" {
		t.Fatalf("earnings entry = %#v", event)
	}
	if earnings.Source != "akshare-calendar" {
		t.Fatalf("earnings source = %q", earnings.Source)
	}

	dividends, err := provider.DividendCalendar(ctx, "2026-08-15")
	if err != nil || len(dividends.Entries) != 1 || dividends.Entries[0].PayableDate != nil {
		t.Fatalf("dividends = %#v, err=%v", dividends, err)
	}

	economic, err := provider.EconomicCalendar(ctx, "", "")
	if err != nil || len(economic.Entries) != 1 {
		t.Fatalf("economic = %#v, err=%v", economic, err)
	}
	econ := economic.Entries[0]
	if econ.EventID != "econ-1" || econ.EventDate != "2026-08-20" ||
		econ.EventTimestamp != 1787000000 ||
		econ.Importance == nil || *econ.Importance != 3 ||
		econ.PreviousValue == nil || *econ.PreviousValue != "0.3%" || econ.ForecastValue != nil {
		t.Fatalf("economic entry = %#v", econ)
	}

	ipos, err := provider.IpoCalendar(ctx)
	if err != nil || len(ipos.Entries) != 1 {
		t.Fatalf("ipos = %#v, err=%v", ipos, err)
	}
	ipo := ipos.Entries[0]
	if ipo.Status != "pending" || ipo.ListingDate != nil || ipo.IssuePrice != nil ||
		ipo.IssuePriceMin == nil || *ipo.IssuePriceMin != "12.5" {
		t.Fatalf("ipo entry = %#v", ipo)
	}
}

func TestProviderCalendarRejectsMalformedEntries(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/providers/akshare/calendar/ipos" {
			_ = json.NewEncoder(writer).Encode(map[string]any{"entries": []map[string]any{{
				"instrument_id": "SH.600001", "status": "withdrawn",
			}}})
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"entries": []map[string]any{{
			"instrument_id": "", "name": "无名",
		}}})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()

	if _, err := provider.EarningsCalendar(ctx, "", ""); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("missing instrument_id error = %v", err)
	}
	if _, err := provider.IpoCalendar(ctx); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("unknown ipo status error = %v", err)
	}
}

func TestProviderMacroConvertsCatalogAndHistory(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/providers/akshare/macro/indicators":
			_ = json.NewEncoder(writer).Encode(map[string]any{"categories": []map[string]any{{
				"category_name": "价格",
				"indicators": []map[string]any{{
					"indicator_id": "cpi_yoy", "name": "CPI同比", "region": "中国",
					"unit": "%", "unit_type": 1, "frequency": "月",
				}},
			}}})
		case "/providers/akshare/macro/indicator-history":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"indicator_id": "cpi_yoy",
				"entries": []map[string]any{{
					"data_time": "2026-07", "value": 0.5, "predict_value": nil,
					"previous_value": nil, "unit": "%", "unit_type": 1,
				}},
			})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()

	catalog, err := provider.MacroIndicators(ctx)
	if err != nil || len(catalog.Categories) != 1 {
		t.Fatalf("indicators = %#v, err=%v", catalog, err)
	}
	category := catalog.Categories[0]
	if category.CategoryName != "价格" || len(category.Indicators) != 1 ||
		category.Indicators[0].IndicatorID != "cpi_yoy" ||
		category.Indicators[0].UnitType == nil || *category.Indicators[0].UnitType != "1" {
		t.Fatalf("category = %#v", category)
	}
	if catalog.Source != "akshare-macro" {
		t.Fatalf("indicators source = %q", catalog.Source)
	}

	history, err := provider.MacroIndicatorHistory(ctx, "cpi_yoy", 0)
	if err != nil || len(history.Entries) != 1 {
		t.Fatalf("history = %#v, err=%v", history, err)
	}
	point := history.Entries[0]
	if point.DataTime != "2026-07" || point.Value == nil || *point.Value != "0.5" ||
		point.PredictValue != nil || point.UnitType == nil {
		t.Fatalf("history point = %#v", point)
	}
	seen := requests()
	if got := seen[1].query.Get("limit"); got != "100" {
		t.Fatalf("default limit forwarded = %q", got)
	}
}

func TestProviderMacroHistoryRejectsMismatchedEcho(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"indicator_id": "ppi_yoy", "entries": []map[string]any{},
		})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.MacroIndicatorHistory(context.Background(), "cpi_yoy", 10); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("mismatched indicator echo error = %v", err)
	}
}

func TestProviderCalendarMacroPassesSidecarErrorsThrough(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"error":{"code":"not_found","message":"no calendar data"}}`))
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = provider.IpoCalendar(context.Background())
	var remoteErr *HTTPError
	if !errors.As(err, &remoteErr) || remoteErr.StatusCode != http.StatusNotFound {
		t.Fatalf("404 must surface as HTTPError, got %v", err)
	}
	if errors.Is(err, ErrUnsupported) {
		t.Fatalf("404 must not fold into capability errors: %v", err)
	}
}
