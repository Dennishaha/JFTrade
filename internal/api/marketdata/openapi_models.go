package marketdata

// SubscriptionInstrument documents a market-data subscription target.
type SubscriptionInstrument struct {
	Channel  string `json:"channel,omitempty"`
	Market   string `json:"market"`
	Symbol   string `json:"symbol"`
	Interval string `json:"interval,omitempty"`
}

// SubscriptionRequest documents the market-data subscription payload.
type SubscriptionRequest struct {
	ConsumerID  string                   `json:"consumerId"`
	Instruments []SubscriptionInstrument `json:"instruments,omitempty"`
}

// SubscriptionHeartbeatRequest documents the subscription heartbeat payload.
type SubscriptionHeartbeatRequest struct {
	ConsumerID string `json:"consumerId"`
}
