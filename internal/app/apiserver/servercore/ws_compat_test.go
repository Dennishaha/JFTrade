package servercore

import livecore "github.com/jftrade/jftrade-main/internal/live"

type liveWebSocketClientMessage struct {
	Type          string                     `json:"type"`
	Subscriptions liveWebSocketSubscriptions `json:"subscriptions"`
}

type liveWebSocketSubscriptions = livecore.Subscriptions
type liveWebSocketSecurityDetailsSubscription = livecore.SecurityDetailsSubscription
type liveWebSocketDepthSubscription = livecore.DepthSubscription
