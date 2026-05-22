package jftradeapi

import (
	"strings"
	"sync"
	"time"
)

type liveNotificationCache struct {
	mu     sync.Mutex
	seq    uint64
	events []liveNotificationEvent
}

func (c *liveNotificationCache) record(note liveNotification) *liveNotificationEvent {
	recordedAt := time.Now().UTC()
	at := strings.TrimSpace(note.At)
	if at == "" {
		at = recordedAt.Format(time.RFC3339Nano)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.seq++
	event := liveNotificationEvent{
		Sequence:   c.seq,
		RecordedAt: recordedAt,
		At:         at,
		Level:      note.Level,
		Title:      note.Title,
		Message:    note.Message,
		Source:     note.Source,
		BrokerID:   note.BrokerID,
		Category:   note.Category,
	}

	cutoff := recordedAt.Add(-liveNotificationRetention)
	filtered := make([]liveNotificationEvent, 0, len(c.events)+1)
	for _, existing := range c.events {
		if existing.RecordedAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, existing)
	}
	filtered = append(filtered, event)
	if len(filtered) > maxLiveNotificationEvents {
		filtered = filtered[len(filtered)-maxLiveNotificationEvents:]
	}
	c.events = filtered

	copyOfEvent := event
	return &copyOfEvent
}

func (c *liveNotificationCache) after(sequence uint64) []liveNotificationEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.events) == 0 {
		return nil
	}
	result := make([]liveNotificationEvent, 0, len(c.events))
	for _, event := range c.events {
		if event.Sequence <= sequence {
			continue
		}
		result = append(result, event)
	}
	return result
}
