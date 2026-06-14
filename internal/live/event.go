package live

import "time"

// Notification is the source-neutral payload accepted by ReplayPublisher.
type Notification struct {
	At       string
	Level    string
	Title    string
	Message  string
	Source   string
	BrokerID string
	Category string
}

// Event is a sequenced notification retained for replay.
type Event struct {
	Sequence   uint64
	RecordedAt time.Time
	At         string
	Level      string
	Title      string
	Message    string
	Source     string
	BrokerID   string
	Category   string
}
