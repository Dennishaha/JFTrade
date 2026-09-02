package settings

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/live"
)

func TestServiceNotificationAndMCPStatusAccessors(t *testing.T) {
	store := &fakeStore{
		systemNotifications: jfsettings.SystemNotificationSettings{Enabled: true, Mode: "all"},
		mcpServer: jfsettings.MCPServerSettings{
			Enabled: true, Port: 7788, AuthMode: "none",
		},
	}
	status := jfsettings.MCPServerStatus{Running: true, Endpoint: "http://127.0.0.1:7788/mcp"}
	svc := NewService(store, WithMCPServerStatus(func() jfsettings.MCPServerStatus { return status }))

	if got := svc.GetSystemNotificationSettings(); !reflect.DeepEqual(got, store.systemNotifications) {
		t.Fatalf("GetSystemNotificationSettings = %#v", got)
	}
	updated := jfsettings.SystemNotificationSettings{Enabled: true, Mode: "custom", Levels: []string{"error"}}
	if got, err := svc.SaveSystemNotificationSettings(updated); err != nil || got.Mode != "custom" {
		t.Fatalf("SaveSystemNotificationSettings = %#v, %v", got, err)
	}
	snapshot := svc.GetMCPServerSettingsSnapshot()
	if snapshot.Settings.Port != 7788 || snapshot.Status != status {
		t.Fatalf("MCP snapshot = %#v", snapshot)
	}
}

func TestServiceSystemNotificationTestUsesNarrowPublisherAndFailsClosed(t *testing.T) {
	event := &live.Event{Sequence: 7, Title: "JFTrade", Category: "system.notification.test"}
	delivery := live.NotificationDelivered("sent")
	svc := NewService(&fakeStore{}, WithSystemNotificationTester(
		func() (*live.Event, live.NotificationDelivery) {
			return event, delivery
		},
	))

	result, err := svc.TestSystemNotification()
	if err != nil {
		t.Fatalf("TestSystemNotification: %v", err)
	}
	if result.Event != *event || result.Delivery != delivery {
		t.Fatalf("notification result = %#v", result)
	}
	if _, err := NewService(&fakeStore{}).TestSystemNotification(); !errors.Is(err, ErrNotificationUnavailable) {
		t.Fatalf("missing publisher error = %v", err)
	}
	if _, err := NewService(&fakeStore{}, WithSystemNotificationTester(
		func() (*live.Event, live.NotificationDelivery) { return nil, live.NotificationDelivery{} },
	)).TestSystemNotification(); !errors.Is(err, ErrNotificationUnavailable) {
		t.Fatalf("nil event error = %v", err)
	}
}

func TestServiceDefaultMCPStatusAndTokenGeneration(t *testing.T) {
	store := &fakeStore{mcpServer: jfsettings.MCPServerSettings{Port: 7799, AuthMode: "none"}}
	snapshot := NewService(store).GetMCPServerSettingsSnapshot()
	if snapshot.Status.Endpoint != "http://127.0.0.1:7799/mcp" {
		t.Fatalf("default MCP endpoint = %q", snapshot.Status.Endpoint)
	}
	token, err := newMCPServerToken()
	if err != nil {
		t.Fatalf("newMCPServerToken: %v", err)
	}
	if !strings.HasPrefix(token, "jft_mcp_") || len(token) <= len("jft_mcp_") {
		t.Fatalf("generated MCP token = %q", token)
	}
}

func TestServiceOptionsCaptureBrokerDescriptorAndDefaultTradingEnvironment(t *testing.T) {
	descriptor := map[string]any{"brokerId": "futu", "markets": []string{"HK", "US"}}
	svc := NewService(
		&fakeStore{},
		WithBrokerDescriptor(func() map[string]any { return descriptor }),
		WithDefaultTradingEnvironment("REAL"),
	)

	if svc.brokerDescriptor == nil {
		t.Fatal("broker descriptor option was not installed")
	}
	got := svc.brokerDescriptor()
	if got["brokerId"] != "futu" || len(got["markets"].([]string)) != 2 {
		t.Fatalf("broker descriptor = %#v", got)
	}
	if svc.defaultTradingEnv != "REAL" {
		t.Fatalf("default trading env = %q, want REAL", svc.defaultTradingEnv)
	}
}
