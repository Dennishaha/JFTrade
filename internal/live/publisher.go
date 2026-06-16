package live

import (
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	defaultRetention = 24 * time.Hour
	defaultCapacity  = 200
)

var ErrClosed = errors.New("live publisher is closed")

type PublishFunc func(Notification) *Event
type StopFunc func() error

// Source registers an external notification producer with a publisher.
// The returned stop function must detach the producer and may be called once.
type Source interface {
	Start(PublishFunc) (StopFunc, error)
}

type SourceFunc func(PublishFunc) (StopFunc, error)

func (f SourceFunc) Start(publish PublishFunc) (StopFunc, error) {
	return f(publish)
}

// ReplayPublisher sequences notifications and retains a bounded replay window.
type ReplayPublisher struct {
	mu        sync.Mutex
	now       func() time.Time
	retention time.Duration
	capacity  int
	sequence  uint64
	events    []Event
	stops     []StopFunc
	starting  sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
	closed    bool
}

func NewReplayPublisher() *ReplayPublisher {
	return newReplayPublisher(time.Now, defaultRetention, defaultCapacity)
}

func newReplayPublisher(now func() time.Time, retention time.Duration, capacity int) *ReplayPublisher {
	return &ReplayPublisher{
		now:       now,
		retention: retention,
		capacity:  capacity,
	}
}

// Publish records a notification unless the publisher has been closed.
func (p *ReplayPublisher) Publish(note Notification) *Event {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}

	recordedAt := p.now().UTC()
	at := strings.TrimSpace(note.At)
	if at == "" {
		at = recordedAt.Format(time.RFC3339Nano)
	}

	p.sequence++
	event := Event{
		Sequence:   p.sequence,
		RecordedAt: recordedAt,
		At:         at,
		Level:      note.Level,
		Title:      note.Title,
		Message:    note.Message,
		Source:     note.Source,
		BrokerID:   note.BrokerID,
		Category:   note.Category,
	}

	cutoff := recordedAt.Add(-p.retention)
	filtered := make([]Event, 0, len(p.events)+1)
	for _, existing := range p.events {
		if existing.RecordedAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, existing)
	}
	filtered = append(filtered, event)
	if len(filtered) > p.capacity {
		filtered = filtered[len(filtered)-p.capacity:]
	}
	p.events = filtered

	return new(event)
}

// After returns a snapshot of retained events with a greater sequence.
func (p *ReplayPublisher) After(sequence uint64) []Event {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.events) == 0 {
		return nil
	}
	result := make([]Event, 0, len(p.events))
	for _, event := range p.events {
		if event.Sequence > sequence {
			result = append(result, event)
		}
	}
	return result
}

// Start attaches a source. Close stops every successfully attached source.
func (p *ReplayPublisher) Start(source Source) error {
	if p == nil || source == nil {
		return nil
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrClosed
	}
	p.starting.Add(1)
	p.mu.Unlock()
	defer p.starting.Done()

	stop, err := source.Start(p.Publish)
	if err != nil {
		return err
	}
	if stop == nil {
		stop = func() error { return nil }
	}

	p.mu.Lock()
	if !p.closed {
		p.stops = append(p.stops, stop)
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()
	return errors.Join(ErrClosed, stop())
}

// Close prevents further publication and stops all attached sources once.
func (p *ReplayPublisher) Close() error {
	if p == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		stops := append([]StopFunc(nil), p.stops...)
		p.stops = nil
		p.mu.Unlock()

		p.starting.Wait()
		var errs []error
		for i := len(stops) - 1; i >= 0; i-- {
			if err := stops[i](); err != nil {
				errs = append(errs, err)
			}
		}
		p.closeErr = errors.Join(errs...)
	})
	return p.closeErr
}
