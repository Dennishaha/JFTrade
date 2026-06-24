package exchangecalendar

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestDefaultRegistryRegistersExpectedSources(t *testing.T) {
	registry := DefaultRegistry(nil)
	descriptors := registry.Descriptors()
	got := map[string][]string{}
	for _, descriptor := range descriptors {
		got[descriptor.ID] = descriptor.Markets
	}

	want := map[string][]string{
		"nyse_official":            {"US"},
		"nasdaq_verifier":          {"US"},
		"hk_gov_1823_ical":         {"HK"},
		"mainland_official_notice": {"CN", "SH", "SZ"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("descriptors = %#v, want %#v", got, want)
	}
}

func TestDefaultRegistryUsesCalendarFetchTimeout(t *testing.T) {
	registry := DefaultRegistry(nil)
	source, ok := registry.Source("nyse_official")
	if !ok {
		t.Fatal("missing nyse_official source")
	}
	httpSource, ok := source.(*HTTPCalendarSource)
	if !ok {
		t.Fatalf("source type = %T, want *HTTPCalendarSource", source)
	}
	if httpSource.client == nil || httpSource.client.Timeout != defaultHTTPTimeout {
		t.Fatalf("default timeout = %v, want %v", httpSource.client, defaultHTTPTimeout)
	}
}

func TestHTTPCalendarSourceFetchBuildsSnapshotMetadata(t *testing.T) {
	source := &HTTPCalendarSource{
		id:        "nyse_official",
		kind:      "official_html",
		authority: "NYSE",
		markets:   []string{"US"},
		url:       "https://example.test/nyse",
		client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<table><tr><td>Juneteenth</td><td>June 19, 2026</td><td>Closed</td></tr></table>`)),
				Header:     make(http.Header),
			}, nil
		})},
		parse:    defaultHolidayOverrideParser("US"),
		validFor: 12 * time.Hour,
	}

	snapshot, err := source.Fetch(context.Background(), "US", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if snapshot.SourceID != "nyse_official" || snapshot.MarketCode != "US" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(snapshot.Schedules) != 1 || snapshot.Schedules[0].Status != marketcalendar.TradingDayClosed {
		t.Fatalf("snapshot schedules = %#v", snapshot.Schedules)
	}
	if snapshot.Checksum == "" || snapshot.ValidUntil.IsZero() || snapshot.FetchedAt.IsZero() {
		t.Fatalf("snapshot metadata = %#v", snapshot)
	}
}

func TestHTTPCalendarSourceFetchReturnsStatusErrors(t *testing.T) {
	source := &HTTPCalendarSource{
		id:        "hk_gov_1823_ical",
		kind:      "official_ical",
		authority: "GovHK 1823",
		markets:   []string{"HK"},
		url:       "https://example.test/hk.ics",
		client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader("bad gateway")),
				Header:     make(http.Header),
			}, nil
		})},
	}

	if _, err := source.Fetch(context.Background(), "HK", time.Time{}, time.Time{}); err == nil {
		t.Fatal("expected status error")
	}
}

func TestHTTPCalendarSourceFetchRejectsSparseAnnualSchedules(t *testing.T) {
	source := &HTTPCalendarSource{
		id:        "hk_gov_1823_ical",
		kind:      "official_ical",
		authority: "GovHK 1823",
		markets:   []string{"HK"},
		url:       "https://example.test/hk.ics",
		client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<div>Holiday Notice: Securities Market will be closed on 19 June 2026.</div>`)),
				Header:     make(http.Header),
			}, nil
		})},
		parse:    defaultHolidayOverrideParser("HK"),
		validate: minimumAnchorYearSchedulesValidator(8),
		validFor: 12 * time.Hour,
	}

	if _, err := source.Fetch(context.Background(), "HK", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC)); err == nil {
		t.Fatal("expected sparse schedule validation error")
	}
}

func TestAnchorYearSchedulesValidatorAllowsMissingFutureYearCoverage(t *testing.T) {
	source := &HTTPCalendarSource{
		id:        "hk_gov_1823_ical",
		kind:      "official_ical",
		authority: "GovHK 1823",
		markets:   []string{"HK"},
		url:       "https://example.test/hk.ics",
		client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body := strings.Join([]string{
				"BEGIN:VCALENDAR",
				"BEGIN:VEVENT",
				"DTSTART;VALUE=DATE:20270101",
				"SUMMARY:The first day of January",
				"END:VEVENT",
				"BEGIN:VEVENT",
				"DTSTART;VALUE=DATE:20270405",
				"SUMMARY:Ching Ming Festival",
				"END:VEVENT",
				"BEGIN:VEVENT",
				"DTSTART;VALUE=DATE:20270406",
				"SUMMARY:The day following Ching Ming Festival",
				"END:VEVENT",
				"BEGIN:VEVENT",
				"DTSTART;VALUE=DATE:20270501",
				"SUMMARY:Labour Day",
				"END:VEVENT",
				"BEGIN:VEVENT",
				"DTSTART;VALUE=DATE:20270519",
				"SUMMARY:Buddha's Birthday",
				"END:VEVENT",
				"BEGIN:VEVENT",
				"DTSTART;VALUE=DATE:20270614",
				"SUMMARY:Tuen Ng Festival",
				"END:VEVENT",
				"BEGIN:VEVENT",
				"DTSTART;VALUE=DATE:20270701",
				"SUMMARY:HKSAR Establishment Day",
				"END:VEVENT",
				"BEGIN:VEVENT",
				"DTSTART;VALUE=DATE:20271001",
				"SUMMARY:National Day",
				"END:VEVENT",
				"BEGIN:VEVENT",
				"DTSTART;VALUE=DATE:20271225",
				"SUMMARY:Christmas Day",
				"END:VEVENT",
				"END:VCALENDAR",
			}, "\r\n")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		})},
		parse:    hongKongHolidayICalParser(),
		validate: minimumAnchorYearSchedulesValidator(8),
		validFor: 12 * time.Hour,
	}

	snapshot, err := source.Fetch(context.Background(), "HK", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2028, 12, 31, 23, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(snapshot.Schedules) != 9 {
		t.Fatalf("snapshot schedules len = %d, want 9", len(snapshot.Schedules))
	}
}

func TestDefaultHolidayOverrideParserUSParsesTableRows(t *testing.T) {
	body := []byte(`
		<table>
			<tr><td>Juneteenth National Independence Day</td><td>June 19, 2026</td><td>Closed</td></tr>
			<tr><td>Black Friday</td><td>November 27, 2026</td><td>1:00 p.m. early close</td></tr>
		</table>
	`)

	schedules, err := defaultHolidayOverrideParser("US")("US", body, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(schedules) != 2 {
		t.Fatalf("schedules len = %d, want 2 (%#v)", len(schedules), schedules)
	}
	if schedules[0].Status != marketcalendar.TradingDayClosed {
		t.Fatalf("first schedule = %#v", schedules[0])
	}
	if schedules[1].Status != marketcalendar.TradingDayEarlyClose {
		t.Fatalf("second schedule = %#v", schedules[1])
	}
}

func TestNYSEHolidayScheduleParserParsesMultiYearTableAndFootnotes(t *testing.T) {
	body := []byte(`
		<table>
			<tr>
				<th>Holiday</th>
				<th>2026</th>
				<th>2027</th>
				<th>2028</th>
			</tr>
			<tr>
				<td>Juneteenth National Independence Day</td>
				<td>Friday, June 19</td>
				<td>Friday, June 18 (Juneteenth National Independence Day observed)</td>
				<td>Monday, June 19</td>
			</tr>
			<tr>
				<td>Independence Day</td>
				<td>Friday, July 3 (Independence Day observed)</td>
				<td>Monday, July 5 (Independence Day observed)</td>
				<td>Tuesday, July 4**</td>
			</tr>
		</table>
		<div>** Each market will close early at 1:00 p.m. (1:15 p.m. for eligible options) on Monday, July 3, 2028. NYSE American Equities, NYSE Arca Equities, NYSE National, and NYSE Texas late trading sessions will close at 5:00 p.m. All times are Eastern Time.</div>
		<div>*** Each market will close early at 1:00 p.m. (1:15 p.m. for eligible options) on Friday, November 27, 2026, Friday, November 26, 2027, and Friday, November 24, 2028 (the day after Thanksgiving). NYSE American Equities, NYSE Arca Equities, NYSE National, and NYSE Texas late trading sessions will close at 5:00 p.m. All times are Eastern Time.</div>
	`)

	schedules, err := nyseHolidayScheduleParser()("US", body, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	got := map[string]marketcalendar.TradingDayStatus{}
	for _, schedule := range schedules {
		got[schedule.Date.Format("2006-01-02")] = schedule.Status
	}

	want := map[string]marketcalendar.TradingDayStatus{
		"2026-06-19": marketcalendar.TradingDayClosed,
		"2026-07-03": marketcalendar.TradingDayClosed,
		"2026-11-27": marketcalendar.TradingDayEarlyClose,
		"2027-06-18": marketcalendar.TradingDayClosed,
		"2027-07-05": marketcalendar.TradingDayClosed,
		"2027-11-26": marketcalendar.TradingDayEarlyClose,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schedules = %#v, want %#v", got, want)
	}
	if _, ok := got["2028-07-03"]; ok {
		t.Fatalf("unexpected out-of-range early close: %#v", schedules)
	}
}

func TestDefaultHolidayOverrideParserHKParsesEnglishList(t *testing.T) {
	body := []byte(`<ul><li>National Day - 1 October 2026 - Closed</li></ul>`)

	schedules, err := defaultHolidayOverrideParser("HK")("HK", body, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(schedules) != 1 || schedules[0].Status != marketcalendar.TradingDayClosed {
		t.Fatalf("schedules = %#v", schedules)
	}
}

func TestHongKongHolidayICalParserParsesClosedDays(t *testing.T) {
	body := []byte(strings.Join([]string{
		"BEGIN:VCALENDAR",
		"BEGIN:VEVENT",
		"DTSTART;VALUE=DATE:20260101",
		"SUMMARY:The first day of January",
		"END:VEVENT",
		"BEGIN:VEVENT",
		"DTSTART;VALUE=DATE:20271001",
		"SUMMARY:National Day",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n"))

	schedules, err := hongKongHolidayICalParser()("HK", body, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	got := map[string]marketcalendar.TradingDayStatus{}
	for _, schedule := range schedules {
		got[schedule.Date.Format("2006-01-02")] = schedule.Status
	}
	want := map[string]marketcalendar.TradingDayStatus{
		"2026-01-01": marketcalendar.TradingDayClosed,
		"2027-10-01": marketcalendar.TradingDayClosed,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schedules = %#v, want %#v", got, want)
	}
}

func TestSSETradingScheduleParserExpandsRangesAndSkipsMakeupDays(t *testing.T) {
	body := []byte(strings.Join([]string{
		"## 2026",
		"Chinese New Year January 28 (Wednesday) - February 4 (Wednesday), plus January 25 (Sunday) and February 7 (Saturday)",
		"National Day October 1 (Thursday) - October 8 (Thursday), plus September 27 (Sunday) and October 10 (Saturday)",
		"Trading hours",
	}, "\n"))

	schedules, err := sseTradingScheduleParser()("CN", body, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	got := map[string]marketcalendar.TradingDayStatus{}
	for _, schedule := range schedules {
		got[schedule.Date.Format("2006-01-02")] = schedule.Status
	}
	if got["2026-01-28"] != marketcalendar.TradingDayClosed || got["2026-02-04"] != marketcalendar.TradingDayClosed {
		t.Fatalf("missing Chinese New Year range: %#v", got)
	}
	if got["2026-10-01"] != marketcalendar.TradingDayClosed || got["2026-10-08"] != marketcalendar.TradingDayClosed {
		t.Fatalf("missing National Day range: %#v", got)
	}
	if _, ok := got["2026-01-25"]; ok {
		t.Fatalf("unexpected makeup day included: %#v", got)
	}
	if _, ok := got["2026-10-10"]; ok {
		t.Fatalf("unexpected makeup day included: %#v", got)
	}
}

func TestDefaultHolidayOverrideParserCNParsesChineseDateLine(t *testing.T) {
	body := []byte(`<div>国庆节休市安排：2026年10月1日 休市</div>`)

	schedules, err := defaultHolidayOverrideParser("CN")("CN", body, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(schedules) != 1 || schedules[0].Status != marketcalendar.TradingDayClosed {
		t.Fatalf("schedules = %#v", schedules)
	}
}

func TestDefaultHolidayOverrideParserRejectsOutOfRangeDates(t *testing.T) {
	body := []byte(`<div>Friday, July 3, 2028 - 1:00 p.m. early close</div>`)

	schedules, err := defaultHolidayOverrideParser("US")("US", body, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(schedules) != 0 {
		t.Fatalf("schedules = %#v, want none", schedules)
	}
}
