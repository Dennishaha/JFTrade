package servercore

import (
	"fmt"

	livecore "github.com/jftrade/jftrade-main/internal/live"
)

type liveWebSocketClientMessage struct {
	Type          string                     `json:"type"`
	Subscriptions liveWebSocketSubscriptions `json:"subscriptions"`
}

type liveWebSocketSubscriptions = livecore.Subscriptions
type liveWebSocketSecurityDetailsSubscription = livecore.SecurityDetailsSubscription
type liveWebSocketDepthSubscription = livecore.DepthSubscription

func liveNotificationEventMap(event liveNotificationEvent) map[string]any {
	payload := map[string]any{
		"type":     "system.notification",
		"id":       fmt.Sprintf("system-notification-%d", event.Sequence),
		"at":       event.At,
		"level":    event.Level,
		"title":    event.Title,
		"source":   event.Source,
		"brokerId": event.BrokerID,
		"category": event.Category,
	}
	if event.Message != "" {
		payload["message"] = event.Message
	}
	return payload
}
