package exchangecalendar

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

type coverageFailingReadCloser struct {
	closed bool
}

func (r *coverageFailingReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("response body interrupted")
}

func (r *coverageFailingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestHTTPCalendarSourceFetchPreservesDistinctTransportAndParsingFailures(t *testing.T) {
	var nilSource *HTTPCalendarSource
	if _, err := nilSource.Fetch(context.Background(), "US", time.Time{}, time.Time{}); err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("nil source Fetch error=%v", err)
	}

	invalidURL := &HTTPCalendarSource{url: "://invalid"}
	if _, err := invalidURL.Fetch(context.Background(), "US", time.Time{}, time.Time{}); err == nil {
		t.Fatal("Fetch accepted an invalid source URL")
	}

	transportErr := errors.New("calendar endpoint unreachable")
	transportFailure := &HTTPCalendarSource{
		id: "transport-failure", url: "https://example.test/calendar",
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, transportErr
		})},
	}
	if _, err := transportFailure.Fetch(context.Background(), "US", time.Time{}, time.Time{}); !errors.Is(err, transportErr) {
		t.Fatalf("transport Fetch error=%v, want %v", err, transportErr)
	}

	body := &coverageFailingReadCloser{}
	readFailure := &HTTPCalendarSource{
		id: "read-failure", url: "https://example.test/calendar",
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
		})},
	}
	if _, err := readFailure.Fetch(context.Background(), "US", time.Time{}, time.Time{}); err == nil || !strings.Contains(err.Error(), "interrupted") || !body.closed {
		t.Fatalf("read Fetch error=%v bodyClosed=%v", err, body.closed)
	}

	parseErr := errors.New("malformed authority document")
	parseFailure := newCoverageHTTPSource(func(string, []byte, time.Time, time.Time) ([]marketcalendar.TradingDaySchedule, error) {
		return nil, parseErr
	}, nil)
	if _, err := parseFailure.Fetch(context.Background(), "US", time.Time{}, time.Time{}); !errors.Is(err, parseErr) {
		t.Fatalf("parse Fetch error=%v, want %v", err, parseErr)
	}

	validateErr := errors.New("schedule failed authority validation")
	validateFailure := newCoverageHTTPSource(nil, func(string, []marketcalendar.TradingDaySchedule, time.Time, time.Time) error {
		return validateErr
	})
	if _, err := validateFailure.Fetch(context.Background(), "US", time.Time{}, time.Time{}); !errors.Is(err, validateErr) {
		t.Fatalf("validation Fetch error=%v, want %v", err, validateErr)
	}
}

func newCoverageHTTPSource(parse ParseFunc, validate ValidateFunc) *HTTPCalendarSource {
	return &HTTPCalendarSource{
		id: "coverage-source", url: "https://example.test/calendar", parse: parse, validate: validate,
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("calendar document")), Header: make(http.Header)}, nil
		})},
	}
}

func TestCalendarParserHelpersHandleMalformedAndPartialAuthorityDocuments(t *testing.T) {
	builtin := marketcalendar.NewBuiltinResolver()
	usTemplate, ok := builtin.Template("US")
	if !ok {
		t.Fatal("US template is unavailable")
	}
	if got := unfoldICalLines("BEGIN:VEVENT\r\nSUMMARY:National\r\n Day\r\nEND:VEVENT\r\n"); len(got) != 4 || got[1] != "SUMMARY:NationalDay" {
		t.Fatalf("unfoldICalLines=%#v", got)
	}
	if fieldValue("SUMMARY without separator") != "" || fieldValue("SUMMARY: National Day ") != "National Day" {
		t.Fatal("fieldValue did not distinguish missing and present values")
	}
	if !parseICalDateValue("DTSTART:20260102T153000Z", usTemplate).IsZero() && parseICalDateValue("DTSTART:invalid", usTemplate).IsZero() {
		// Both date formats are intentionally parsed; this condition documents
		// the malformed value branch without accepting an invalid date.
	} else {
		t.Fatal("parseICalDateValue did not parse valid / reject invalid values")
	}

	if target, _, _, ok := resolveParseTemplate("", "US"); !ok || target != "US" {
		t.Fatalf("resolveParseTemplate default=%q/%v", target, ok)
	}
	if target, _, _, ok := resolveParseTemplate("SH", ""); !ok || target != "SH" {
		t.Fatalf("resolveParseTemplate SH fallback=%q/%v", target, ok)
	}
	if _, _, _, ok := resolveParseTemplate("MARS", ""); ok {
		t.Fatal("resolveParseTemplate accepted an unsupported market")
	}

	date := time.Date(2026, time.July, 3, 0, 0, 0, 0, time.UTC)
	seen := map[string]struct{}{}
	schedule := marketcalendar.TradingDaySchedule{MarketCode: "US", Date: date, Status: marketcalendar.TradingDayClosed}
	if schedules := appendSchedule(appendSchedule(nil, seen, schedule), seen, schedule); len(schedules) != 1 {
		t.Fatalf("appendSchedule duplicate=%#v", schedules)
	}
	if _, ok := parseMonthDayCellWithYear("TBD", 2026, usTemplate); ok {
		t.Fatal("parseMonthDayCellWithYear accepted a non-date cell")
	}
	if _, ok := parseStandaloneYear("calendar 2026"); ok {
		t.Fatal("parseStandaloneYear accepted embedded text")
	}
	if containsMonthName("exchange holiday notice") {
		t.Fatal("containsMonthName accepted text without a month")
	}
	if spans := extractSSEDateSpans("National Day December 31, 2026 - January 2, 2027, plus makeup workdays", 2026, usTemplate); len(spans) != 1 || spans[0][1].Year() != 2027 {
		t.Fatalf("extractSSEDateSpans=%#v", spans)
	}
	if _, ok := parseMonthDayWithOptionalYear("NotAMonth", "1", 2026, usTemplate); ok {
		t.Fatal("parseMonthDayWithOptionalYear accepted an invalid month")
	}
	from := time.Date(2026, time.July, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.July, 4, 0, 0, 0, 0, time.UTC)
	if dateWithinFetchRange(time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC), from, to, usTemplate) {
		t.Fatal("dateWithinFetchRange accepted a date before the fetch window")
	}
	if dateWithinFetchRange(time.Date(2026, time.July, 5, 0, 0, 0, 0, time.UTC), from, to, usTemplate) {
		t.Fatal("dateWithinFetchRange accepted a date after the fetch window")
	}

	if schedules, err := defaultHolidayOverrideParser("")("MARS", []byte("Closed January 1, 2026"), from, to); err != nil || schedules != nil {
		t.Fatalf("unsupported default parser schedules=%#v err=%v", schedules, err)
	}
	if schedules, err := nyseHolidayScheduleParser()("MARS", []byte("Holiday"), from, to); err != nil || schedules != nil {
		t.Fatalf("unsupported NYSE parser schedules=%#v err=%v", schedules, err)
	}
}

func TestCalendarAuthorityValidatorHandlesMissingAnchorsAndSparseYears(t *testing.T) {
	noMinimum := minimumAnchorYearSchedulesValidator(0)
	if err := noMinimum("US", nil, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("zero minimum validator error=%v", err)
	}
	validator := minimumAnchorYearSchedulesValidator(2)
	zeroYear := time.Date(0, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := validator("US", nil, zeroYear, time.Time{}); err == nil || !strings.Contains(err.Error(), "no anchor year") {
		t.Fatalf("missing anchor validator error=%v", err)
	}
	schedules := []marketcalendar.TradingDaySchedule{{MarketCode: "US", Date: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), Status: marketcalendar.TradingDayClosed}}
	if err := validator("US", schedules, time.Time{}, time.Time{}); err == nil || !strings.Contains(err.Error(), "too few") {
		t.Fatalf("sparse validator error=%v", err)
	}
	if err := minimumAnchorYearSchedulesValidator(1)("US", schedules, schedules[0].Date, time.Time{}); err != nil {
		t.Fatalf("single schedule validator error=%v", err)
	}
	if err := minimumAnchorYearSchedulesValidator(1)("US", schedules, zeroYear, time.Time{}); err != nil {
		t.Fatalf("fallback schedule anchor validator error=%v", err)
	}
	jftradeLogError(errors.New("best effort calendar cleanup"), nil, "not an error")
}

func TestCalendarParsersDiscardIncompleteOrOutOfRangeAuthorityRows(t *testing.T) {
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	nyseBody := []byte(`
		<table>
			<tr><th>Holiday</th><th>2026</th></tr>
			<tr><td>Incomplete row</td></tr>
			<tr><td></td><td>June 19</td></tr>
			<tr><td>Invalid date</td><td>February 30</td></tr>
			<tr><td>Outside range</td><td>July 3</td></tr>
		</table>`)
	if schedules, err := nyseHolidayScheduleParser()("US", nyseBody, from, time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC)); err != nil || len(schedules) != 0 {
		t.Fatalf("NYSE incomplete rows schedules=%#v err=%v", schedules, err)
	}

	if schedules, err := hongKongHolidayICalParser()("MARS", []byte("BEGIN:VEVENT\nEND:VEVENT"), from, to); err != nil || schedules != nil {
		t.Fatalf("HK parser unsupported market schedules=%#v err=%v", schedules, err)
	}
	if schedules, err := hongKongHolidayICalParser()("HK", []byte("BEGIN:VEVENT\nSUMMARY:No date\nEND:VEVENT"), from, to); err != nil || len(schedules) != 0 {
		t.Fatalf("HK parser incomplete event schedules=%#v err=%v", schedules, err)
	}

	if schedules, err := sseTradingScheduleParser()("MARS", []byte("2026\nJanuary 1"), from, to); err != nil || schedules != nil {
		t.Fatalf("SSE parser unsupported market schedules=%#v err=%v", schedules, err)
	}
	sseBody := []byte(strings.Join([]string{
		"notice before year",
		"## 2026",
		"plain text without a month",
		"Reverse span December 31 - January 2",
		"Trading hours",
		"National Day October 1 - October 8",
	}, "\n"))
	if schedules, err := sseTradingScheduleParser()("CN", sseBody, from, to); err != nil || len(schedules) != 0 {
		t.Fatalf("SSE parser should stop at trading-hours marker schedules=%#v err=%v", schedules, err)
	}
	if parseICalDateValue("DTSTART:", marketTemplateForCoverage(t, "HK")) != (time.Time{}) {
		t.Fatal("parseICalDateValue accepted an empty DTSTART value")
	}
}

func marketTemplateForCoverage(t *testing.T, market string) marketcalendar.MarketTemplate {
	t.Helper()
	template, ok := marketcalendar.NewBuiltinResolver().Template(market)
	if !ok {
		t.Fatalf("missing market template %s", market)
	}
	return template
}

func TestCalendarSourceAlertLifecycleRecordsFailuresDeduplicatesAndRecovers(t *testing.T) {
	now := time.Date(2026, time.July, 2, 8, 0, 0, 0, time.UTC)
	alerts := make([]SourceAlert, 0)
	manager := NewManager(nil, nil,
		WithClock(func() time.Time { return now }),
		WithAlertSink(func(alert SourceAlert) { alerts = append(alerts, alert) }),
	)
	manager.recordOperationFailure("operation-source", errors.New("disk unavailable"))
	operation := manager.sourceStatus("operation-source")
	if operation.ConsecutiveFailures != 1 || operation.LastError != "disk unavailable" || !operation.NextRefreshAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("operation failure status=%+v", operation)
	}

	manager.recordSourceFailure("remote-source", "us", nil, "fetch_failed")
	manager.recordSourceFailure("remote-source", "US", nil, "fetch_failed")
	if len(alerts) != 1 || alerts[0].Level != "warn" || alerts[0].Message == "" {
		t.Fatalf("deduplicated source alerts=%#v", alerts)
	}
	manager.recordProbeFailure("probe-source", "HK", context.DeadlineExceeded, "fetch_failed")
	probe := manager.sourceStatus("probe-source")
	if probe.LastProbeStatus != "unhealthy" || probe.LastProbeError == "" || probe.HealthState != "unhealthy" {
		t.Fatalf("probe failure status=%+v", probe)
	}
	manager.recordProbeSuccess("probe-source", "HK", 9)
	manager.recordSuccess("remote-source", marketcalendar.CalendarSnapshot{MarketCode: "US", FetchedAt: now})
	if probe = manager.sourceStatus("probe-source"); probe.LastProbeStatus != "healthy" || probe.HealthState != "healthy" || probe.LastProbeSchedules != 9 {
		t.Fatalf("probe recovery status=%+v", probe)
	}
	if len(alerts) < 3 {
		t.Fatalf("expected unhealthy and recovery alerts, got %#v", alerts)
	}

	if recordHealthyStateLocked(nil, "US", now) != nil {
		t.Fatal("nil healthy status unexpectedly emitted an alert")
	}
	if alert := recordUnhealthyStateLocked(&SourceStatus{SourceID: "fallback-source"}, "CN", now, SourceAlert{}); alert == nil || alert.Fingerprint == "" {
		t.Fatalf("fallback unhealthy alert=%+v", alert)
	}
	if got := sourceAlertFingerprintDetail("fetch_failed", nil); got != "unknown_error" {
		t.Fatalf("nil alert fingerprint detail=%q", got)
	}
	if got := sourceAlertFingerprintDetail("fetch_failed", context.Canceled); got != "network_timeout_or_cancelled" {
		t.Fatalf("cancelled alert fingerprint detail=%q", got)
	}
	if defaultAlertDetail(" ") != "unknown error" {
		t.Fatal("defaultAlertDetail did not provide an unknown-error fallback")
	}
}

func TestCalendarParsersHonorNarrowFetchWindowsAndDiscardImpossibleDates(t *testing.T) {
	from := time.Date(2026, time.October, 3, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.October, 4, 0, 0, 0, 0, time.UTC)
	body := []byte("## 2026\nNational Day October 1 - October 8")
	schedules, err := sseTradingScheduleParser()("CN", body, from, to)
	if err != nil || len(schedules) != 2 || schedules[0].Date.Format("2006-01-02") != "2026-10-03" || schedules[1].Date.Format("2006-01-02") != "2026-10-04" {
		t.Fatalf("SSE narrow window schedules=%#v err=%v", schedules, err)
	}
	template := marketTemplateForCoverage(t, "US")
	if _, ok := parseMonthDayCellWithYear("February 30", 2026, template); ok {
		t.Fatal("parseMonthDayCellWithYear accepted an impossible calendar day")
	}
	if spans := extractSSEDateSpans("Holiday February 30", 2026, template); len(spans) != 0 {
		t.Fatalf("extractSSEDateSpans accepted impossible start date: %#v", spans)
	}
	if spans := extractSSEDateSpans("Holiday January 1 - February 30", 2026, template); len(spans) != 0 {
		t.Fatalf("extractSSEDateSpans accepted impossible end date: %#v", spans)
	}
}

func TestNilCalendarManagerOperationsRemainSafeDuringStartupAndShutdown(t *testing.T) {
	var nilManager *Manager
	nilManager.recordSuccess("source", marketcalendar.CalendarSnapshot{})
	nilManager.recordOperationFailure("source", errors.New("ignored"))
	nilManager.recordSourceFailure("source", "US", errors.New("ignored"), "fetch_failed")
	nilManager.recordProbeSuccess("source", "US", 1)
	nilManager.recordProbeFailure("source", "US", errors.New("ignored"), "fetch_failed")
	if settings := nilManager.settings(); len(settings.SourcePolicies) != 0 {
		t.Fatalf("nil manager settings=%+v", settings)
	}
	if nilManager.currentTime().IsZero() {
		t.Fatal("nil manager currentTime returned zero")
	}

	manager := NewManager(nil, nil)
	manager.now = nil
	if manager.currentTime().IsZero() {
		t.Fatal("manager without clock returned zero time")
	}
	if _, ok := manualOverrideSchedule(jfsettings.ExchangeCalendarSettings{}, nil, "US", time.Now()); ok {
		t.Fatal("manualOverrideSchedule accepted a nil builtin resolver")
	}
}
