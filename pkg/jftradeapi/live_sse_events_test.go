package jftradeapi

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bbgo "github.com/c9s/bbgo/pkg/bbgo"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
)

func TestLiveSSESendsHeartbeat(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	response, err := liveSSERequest(t, srv.URL+"/api/v1/stream/live")
	if err != nil {
		t.Fatalf("GET live SSE: %v", err)
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
	event := readSSEEvent(t, reader)
	if event["type"] != "heartbeat" || event["at"] == "" {
		t.Fatalf("unexpected event: %+v", event)
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

	response, err := liveSSERequest(t, srv.URL+"/api/v1/stream/live")
	if err != nil {
		t.Fatalf("GET live SSE: %v", err)
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
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
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	bbgo.Notify("strategy %s started", "demo-grid")

	response, err := liveSSERequest(t, srv.URL+"/api/v1/stream/live")
	if err != nil {
		t.Fatalf("GET live SSE: %v", err)
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
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
