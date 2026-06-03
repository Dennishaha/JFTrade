package jftradeapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bbgo "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
)

func TestLiveSSESendsHeartbeat(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	defer server.Close()
	srv := httptest.NewServer(server)
	defer srv.Close()

	response, err := liveSSERequest(t, srv.URL+"/api/sse/live")
	if err != nil {
		t.Fatalf("GET live SSE: %v", err)
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
	if retryMillis := readSSERetry(t, reader); retryMillis != int((defaultSSEClientRetry / time.Millisecond)) {
		t.Fatalf("retry = %d, want %d", retryMillis, int(defaultSSEClientRetry/time.Millisecond))
	}
	event := readSSEEvent(t, reader)
	if event["type"] != "heartbeat" || event["at"] == "" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if got := int64(event["intervalMs"].(float64)); got != int64(15*time.Second/time.Millisecond) {
		t.Fatalf("intervalMs = %d", got)
	}
	if got := int64(event["retryMs"].(float64)); got != int64(defaultSSEClientRetry/time.Millisecond) {
		t.Fatalf("retryMs = %d", got)
	}
	if stale, _ := event["stale"].(bool); stale {
		t.Fatalf("unexpected stale heartbeat: %+v", event)
	}
}

func TestLiveSSESendsSystemNotification(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	server.handleFutuSystemNotify(&notifypb.Response{
		RetType: proto.Int32(0),
		S2C: &notifypb.S2C{
			Type: proto.Int32(int32(notifypb.NotifyType_NotifyType_ProgramStatus)),
			ProgramStatus: &commonpb.ProgramStatus{
				Type:       commonpb.ProgramStatusType_ProgramStatusType_Ready.Enum(),
				StrExtDesc: proto.String("OpenD ready for requests"),
			},
		},
	})

	srv := httptest.NewServer(server)
	defer srv.Close()
	defer server.Close()

	response, err := liveSSERequest(t, srv.URL+"/api/sse/live")
	if err != nil {
		t.Fatalf("GET live SSE: %v", err)
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
	_ = readSSERetry(t, reader)
	heartbeat := readSSEEvent(t, reader)
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("unexpected first event: %+v", heartbeat)
	}

	event := readSSEEvent(t, reader)
	if event["type"] != "system.notification" {
		t.Fatalf("unexpected event type: %+v", event)
	}
	if event["title"] != "OpenD 已就绪" {
		t.Fatalf("title = %#v", event["title"])
	}
	if event["message"] != "已就绪：OpenD ready for requests" {
		t.Fatalf("message = %#v", event["message"])
	}
}

func TestLiveSSESendsBBGONotification(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	defer server.Close()
	srv := httptest.NewServer(server)
	defer srv.Close()

	bbgo.Notify("strategy %s started", "demo-grid")

	response, err := liveSSERequest(t, srv.URL+"/api/sse/live")
	if err != nil {
		t.Fatalf("GET live SSE: %v", err)
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
	_ = readSSERetry(t, reader)
	heartbeat := readSSEEvent(t, reader)
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("unexpected first event: %+v", heartbeat)
	}

	event := readSSEEvent(t, reader)
	if event["type"] != "system.notification" {
		t.Fatalf("unexpected event type: %+v", event)
	}
	if event["title"] != "BBGO 通知" {
		t.Fatalf("title = %#v", event["title"])
	}
	if event["message"] != "strategy demo-grid started" {
		t.Fatalf("message = %#v", event["message"])
	}
}

func TestLiveSSEHeartbeatReportsStaleMarketData(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	server.marketSubscriptions.acquire(marketSubscriptionInput{
		Key:        "SNAPSHOT:HK:00700",
		Channel:    "SNAPSHOT",
		Market:     "HK",
		Symbol:     "00700",
		ConsumerID: "test-live-heartbeat",
	})
	server.tickCache.seed(marketTickSample{
		InstrumentID: "HK.00700",
		Market:       "HK",
		Symbol:       "00700",
		Price:        mustDecimal("320.5"),
		Bid:          mustDecimal("320.4"),
		Ask:          mustDecimal("320.6"),
		ObservedAt:   time.Now().UTC().Add(-(liveHeartbeatStaleThreshold + 500*time.Millisecond)).Format(time.RFC3339Nano),
		QuoteAt:      time.Now().UTC().Add(-(liveHeartbeatStaleThreshold + 500*time.Millisecond)).Format(time.RFC3339Nano),
		Source:       "bbgo:futu",
	})

	srv := httptest.NewServer(server)
	defer srv.Close()
	defer server.Close()

	response, err := liveSSERequest(t, srv.URL+"/api/sse/live")
	if err != nil {
		t.Fatalf("GET live SSE: %v", err)
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
	_ = readSSERetry(t, reader)
	heartbeat := readSSEEvent(t, reader)
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("unexpected first event: %+v", heartbeat)
	}
	if stale, _ := heartbeat["stale"].(bool); !stale {
		t.Fatalf("expected stale heartbeat, got %+v", heartbeat)
	}
	reasons, ok := heartbeat["staleReasons"].([]any)
	if !ok || len(reasons) == 0 {
		t.Fatalf("staleReasons = %#v", heartbeat["staleReasons"])
	}
	if reasons[0] != "market-data-samples-stale" {
		t.Fatalf("unexpected stale reason: %#v", reasons)
	}
}

func liveSSERequest(t *testing.T, url string) (*http.Response, error) {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/event-stream")
	return client.Do(request)
}

func readSSERetry(t *testing.T, reader *bufio.Reader) int {
	t.Helper()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("ReadString retry: %v", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "retry: ") {
			var retryMillis int
			if _, err := fmt.Sscanf(strings.TrimPrefix(line, "retry: "), "%d", &retryMillis); err != nil {
				t.Fatalf("parse retry %q: %v", line, err)
			}
			return retryMillis
		}
		t.Fatalf("unexpected SSE prelude line: %q", line)
	}
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) map[string]any {
	t.Helper()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("ReadString: %v", err)
		}
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		return event
	}
}

func mustDecimal(value string) decimal.Decimal {
	return decimal.RequireFromString(value)
}
