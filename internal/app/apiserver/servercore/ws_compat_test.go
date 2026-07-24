package servercore

import livecore "github.com/jftrade/jftrade-main/internal/live"

type liveWebSocketClientMessage struct {
	Type          string                 `json:"type"`
	Subscriptions livecore.Subscriptions `json:"subscriptions"`
}
