package desktop

import (
	"testing"

	"github.com/jftrade/jftrade-main/internal/live"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestShouldForwardSystemNotification(t *testing.T) {
	event := live.Event{Level: "warn", Category: "broker.connection"}
	if ShouldForwardSystemNotification(jfsettings.SystemNotificationSettings{Enabled: false, Mode: "all"}, event) {
		t.Fatal("disabled settings forwarded notification")
	}
	if !ShouldForwardSystemNotification(jfsettings.SystemNotificationSettings{Enabled: true, Mode: "all"}, live.Event{Level: "info"}) {
		t.Fatal("all mode did not forward info notification")
	}
	if !ShouldForwardSystemNotification(jfsettings.SystemNotificationSettings{Enabled: true, Mode: "custom", Levels: []string{"error"}}, live.Event{Level: "ERROR"}) {
		t.Fatal("custom level did not match case-insensitively")
	}
	if !ShouldForwardSystemNotification(jfsettings.SystemNotificationSettings{Enabled: true, Mode: "custom", Categories: []string{"strategy.order.signal"}}, live.Event{Level: "info", Category: "strategy.order.signal"}) {
		t.Fatal("custom category did not match")
	}
	if ShouldForwardSystemNotification(jfsettings.SystemNotificationSettings{Enabled: true, Mode: "custom", Levels: []string{"error"}}, live.Event{Level: "info", Category: "market.quota"}) {
		t.Fatal("custom settings forwarded unmatched notification")
	}
}

func TestNotificationMetadata(t *testing.T) {
	if got := NotificationThreadID(live.Event{Category: "broker.connection", Source: "futu"}); got != "broker.connection" {
		t.Fatalf("NotificationThreadID category = %q", got)
	}
	if got := NotificationThreadID(live.Event{Source: "bbgo.notify"}); got != "bbgo.notify" {
		t.Fatalf("NotificationThreadID source = %q", got)
	}
	if got := NotificationThreadID(live.Event{}); got != "jftrade.system" {
		t.Fatalf("NotificationThreadID fallback = %q", got)
	}
	if got := NotificationInterruptionLevel("error"); got != "timeSensitive" {
		t.Fatalf("error interruption = %q", got)
	}
	if got := NotificationInterruptionLevel("warn"); got != "active" {
		t.Fatalf("warn interruption = %q", got)
	}
	if got := NotificationInterruptionLevel("info"); got != "passive" {
		t.Fatalf("info interruption = %q", got)
	}
}
