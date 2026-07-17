package servercore

import (
	"fmt"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/adk"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestCoverage98BusinessNotificationAndSecurityBoundaries(t *testing.T) {
	if got := formatBBGONotifyText("risk %s", "limit"); got != "risk limit" {
		t.Fatalf("format formatted text = %q", got)
	}
	if got := formatBBGONotifyText(coverage98Stringer("provider")); got != "provider" {
		t.Fatalf("format Stringer = %q", got)
	}
	if got := formatBBGONotifyText(coverage98Stringer("provider"), "retry"); got != "provider retry" {
		t.Fatalf("format Stringer with args = %q", got)
	}
	if got := formatBBGONotifyText(42, "attempt", 2); got != "42 attempt 2" {
		t.Fatalf("format generic notification = %q", got)
	}
	if note := liveNotificationFromBBGONotify("   "); note != nil {
		t.Fatalf("blank BBGO notification = %#v, want nil", note)
	}
	if note := liveNotificationFromBBGONotify(fmt.Errorf("connection timeout")); note == nil || note.Level != "error" {
		t.Fatalf("BBGO error notification = %#v", note)
	}

	if note := liveNotificationFromExchangeCalendarAlert(exchangecalendar.SourceAlert{}); note != nil {
		t.Fatalf("untitled calendar alert = %#v, want nil", note)
	}
	for _, tc := range []struct {
		level string
		want  string
	}{
		{level: "success", want: "success"},
		{level: "error", want: "error"},
		{level: "WARN", want: "warn"},
		{level: "unknown", want: "info"},
	} {
		if got := normalizeExchangeCalendarNotificationLevel(tc.level); got != tc.want {
			t.Fatalf("level %q = %q, want %q", tc.level, got, tc.want)
		}
	}
	calendarNote := liveNotificationFromExchangeCalendarAlert(exchangecalendar.SourceAlert{
		Level: "warn", Title: "NYSE source stale", Market: "US", SourceID: "nyse",
	})
	if calendarNote == nil || calendarNote.Message != "US 市场日历源 nyse 状态发生变化。" {
		t.Fatalf("calendar fallback notification = %#v", calendarNote)
	}

	if got := extendedMarketQuoteSecurityMap(nil); got != nil {
		t.Fatalf("nil extended quote map = %#v", got)
	}
	if got := optionalFloat64JSON(nil); got != nil {
		t.Fatalf("nil float64 JSON = %#v", got)
	}
	if got := optionalInt64(nil); got != nil {
		t.Fatalf("nil int64 = %#v", got)
	}
	if got := optionalBool(nil); got != nil {
		t.Fatalf("nil bool = %#v", got)
	}
	if got := optionalString(nil); got != nil {
		t.Fatalf("nil string = %#v", got)
	}
	if got := securityRefMap(nil); got != nil {
		t.Fatalf("nil security ref map = %#v", got)
	}
	if got := securityDetailsMap(nil); got != nil {
		t.Fatalf("nil security details map = %#v", got)
	}
}

func TestCoverage98BusinessQueryAndExecutionFallbacks(t *testing.T) {
	if first, second := pathTail("/api/v1/market/HK/00700", "/api/v1/market/"); first != "HK" || second != "00700" {
		t.Fatalf("pathTail = %q, %q", first, second)
	}
	if first, second := pathTail("/api/v1/market/HK", "/api/v1/market/"); first != "" || second != "" {
		t.Fatalf("short pathTail = %q, %q", first, second)
	}
	if _, err := decodeMarketCandlesQuery(map[string][]string{"period": {"not-a-period"}}); err == nil {
		t.Fatal("invalid period should be rejected")
	}
	query, err := decodeMarketCandlesQuery(map[string][]string{
		"limit": {"0"}, "from": {"2026-07-15T10:00:00Z"}, "to": {"2026-07-15T09:00:00Z"},
	})
	if err != nil {
		t.Fatalf("decode market candles: %v", err)
	}
	if got := query.normalizedPeriod(); got != "1m" {
		t.Fatalf("normalized period = %q", got)
	}
	if got := query.limitOrDefault(20, 50); got != 1 {
		t.Fatalf("limit clamp = %d", got)
	}
	begin, end := kLineQueryWindow(query, time.Minute, 10)
	if !begin.Before(end) {
		t.Fatalf("invalid explicit range should fall back to a chronological window: %s >= %s", begin, end)
	}

	if got := marshalExecutionPayload(nil); got != "{}" {
		t.Fatalf("nil execution payload = %q", got)
	}
	if got := marshalExecutionPayload(make(chan int)); got != "{}" {
		t.Fatalf("unencodable execution payload = %q", got)
	}
	if got := firstNonEmptyString(" ", "", " usable "); got != "usable" {
		t.Fatalf("first non-empty = %q", got)
	}
	if got := firstNonEmptyString(" ", ""); got != "" {
		t.Fatalf("empty fallback = %q", got)
	}
	if pointer := executionStringPointerOrNil(" "); pointer != nil {
		t.Fatalf("blank pointer = %v", pointer)
	}
	if pointer := executionStringPointerOrNil(" order-1 "); pointer == nil || *pointer != "order-1" {
		t.Fatalf("trimmed pointer = %v", pointer)
	}
}

func TestCoverage98RealTradeControlPathUsesExplicitOverride(t *testing.T) {
	t.Setenv("JFTRADE_REAL_TRADE_CONTROL_PATH", "/tmp/jftrade-control.json")
	if got := deriveRealTradeControlPath("/var/lib/jftrade/settings.json"); got != "/tmp/jftrade-control.json" {
		t.Fatalf("explicit control path = %q", got)
	}
	t.Setenv("JFTRADE_REAL_TRADE_CONTROL_PATH", "")
	if got := deriveRealTradeControlPath("settings.json"); got != defaultRealTradeControlFilename {
		t.Fatalf("relative settings control path = %q", got)
	}
	if got := deriveRealTradeControlPath("/var/lib/jftrade/settings.json"); got != "/var/lib/jftrade/"+defaultRealTradeControlFilename {
		t.Fatalf("derived control path = %q", got)
	}
}

func TestCoverage98WorkflowAndMarketContractBoundaries(t *testing.T) {
	market, symbol, ok := splitWorkflowInstrumentID(" us.aapl ")
	if !ok || market != "US" || symbol != "AAPL" {
		t.Fatalf("split workflow instrument = %q, %q, %v", market, symbol, ok)
	}
	for _, value := range []string{"", "AAPL", "US.", ".AAPL"} {
		if _, _, valid := splitWorkflowInstrumentID(value); valid {
			t.Fatalf("invalid workflow instrument %q was accepted", value)
		}
	}
	if market, symbol, ok := splitWorkflowInstrumentID("US.AAPL.EXTRA"); !ok || market != "US" || symbol != "AAPL.EXTRA" {
		t.Fatalf("canonical workflow symbol with suffix = %q, %q, %v", market, symbol, ok)
	}
	var nilServer *Server
	if _, err := nilServer.workflowMarketSnapshot(t.Context(), "US.AAPL"); err == nil {
		t.Fatal("nil server should not provide a market snapshot")
	}
	if got := nilServer.workflowWatchedInstruments(); got != nil {
		t.Fatalf("nil server watched instruments = %#v", got)
	}
	nilServer.emitWorkflowEvent(adk.WorkflowEvent{})
	server := &Server{}
	if _, err := server.workflowMarketSnapshot(t.Context(), "invalid"); err == nil {
		t.Fatal("server without market service should reject snapshots")
	}
	server.emitWorkflowEvent(adk.WorkflowEvent{})

	request := marketCandlesRequest("US", "AAPL", "US.AAPL", "1h", 200)
	if request["period"] != "1h" || request["limit"] != 200 {
		t.Fatalf("market candles request = %#v", request)
	}
	meta := candleMeta("US.AAPL", false, true, true)
	if meta["session"] != "all" || meta["extendedHours"] != true {
		t.Fatalf("US intraday candle meta = %#v", meta)
	}
	if shouldAnnotateHistoricalKLineSession("US", bbgotypes.Interval("1h")) != true {
		t.Fatal("US hourly candles should carry session metadata")
	}
	if shouldAnnotateHistoricalKLineSession("HK", bbgotypes.Interval("1h")) || shouldAnnotateHistoricalKLineSession("US", bbgotypes.Interval("1d")) {
		t.Fatal("only US intraday candles should carry extended-session metadata")
	}
	if read := brokerReadQuery("HK.00700"); read.Market != "HK" {
		t.Fatalf("broker read market = %q", read.Market)
	}

	nilServer.ensureLiveMarketStream(t.Context(), []string{"US.AAPL"})
	nilServer.handlePushMarketdataTick(mdsrv.Tick{Kind: mdsrv.TickKindQuote})
	server.handlePushMarketdataTick(mdsrv.Tick{Kind: mdsrv.TickKindTrade})
	if got := httpTime("not-a-timestamp"); !got.IsZero() {
		t.Fatalf("invalid HTTP time = %s", got)
	}
	if got := httpTime("2026-07-16T10:00:00Z"); got != (time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("parsed HTTP time = %s", got)
	}
	if limit := (liveWebSocketBackend{}).ConnectionLimit(); limit != defaultMaxWebSocketClients {
		t.Fatalf("nil live backend limit = %d", limit)
	}
	if count, limit, atLimit := nilServer.liveStreamStats(); count != 0 || limit != defaultMaxWebSocketClients || atLimit {
		t.Fatalf("nil live stats = %d, %d, %v", count, limit, atLimit)
	}
}

type coverage98Stringer string

func (value coverage98Stringer) String() string { return string(value) }
