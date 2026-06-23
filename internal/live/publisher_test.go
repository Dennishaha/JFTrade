package live

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReplayPublisherSequencesAndRetainsWindow(t *testing.T) {
	now := time.Date(2026, 6, 14, 8, 0, 0, 0, time.FixedZone("test", 8*60*60))
	publisher := newReplayPublisher(func() time.Time { return now }, 24*time.Hour, 2)

	first := publisher.Publish(Notification{Title: "first"})
	now = now.Add(time.Hour)
	second := publisher.Publish(Notification{At: " explicit ", Title: "second"})
	now = now.Add(time.Hour)
	third := publisher.Publish(Notification{Title: "third"})

	if first.Sequence != 1 || second.Sequence != 2 || third.Sequence != 3 {
		t.Fatalf("sequences = %d, %d, %d", first.Sequence, second.Sequence, third.Sequence)
	}
	if first.RecordedAt.Location() != time.UTC || first.At != first.RecordedAt.Format(time.RFC3339Nano) {
		t.Fatalf("first timestamp = %#v", first)
	}
	if second.At != second.RecordedAt.Format(time.RFC3339Nano) {
		t.Fatalf("invalid explicit timestamp did not fall back to recordedAt: %#v", second)
	}
	events := publisher.After(0)
	if len(events) != 2 || events[0].Sequence != 2 || events[1].Sequence != 3 {
		t.Fatalf("retained events = %#v", events)
	}
	if got := publisher.After(2); len(got) != 1 || got[0].Sequence != 3 {
		t.Fatalf("After(2) = %#v", got)
	}
}

func TestReplayPublisherNormalizesEventTimeToUTC(t *testing.T) {
	publisher := newReplayPublisher(func() time.Time {
		return time.Date(2026, time.June, 20, 0, 0, 0, 0, time.UTC)
	}, time.Hour, 10)

	event := publisher.Publish(Notification{
		At:    "2026-06-20T09:30:00+08:00",
		Title: "offset event",
	})
	if event == nil {
		t.Fatal("expected event")
	}
	if event.At != "2026-06-20T01:30:00Z" {
		t.Fatalf("event.At = %q, want UTC", event.At)
	}
}

func TestReplayPublisherExpiresOldEventsWithoutResettingSequence(t *testing.T) {
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	publisher := newReplayPublisher(func() time.Time { return now }, 24*time.Hour, 200)
	publisher.Publish(Notification{Title: "old"})

	now = now.Add(24*time.Hour + time.Nanosecond)
	event := publisher.Publish(Notification{Title: "new"})
	if event.Sequence != 2 {
		t.Fatalf("sequence = %d", event.Sequence)
	}
	events := publisher.After(0)
	if len(events) != 1 || events[0].Title != "new" {
		t.Fatalf("events = %#v", events)
	}
}

func TestReplayPublisherCloseStopsSourcesOnce(t *testing.T) {
	publisher := NewReplayPublisher()
	var stopped atomic.Int32
	errStop := errors.New("stop failed")
	err := publisher.Start(SourceFunc(func(publish PublishFunc) (StopFunc, error) {
		publish(Notification{Title: "started"})
		return func() error {
			stopped.Add(1)
			return errStop
		}, nil
	}))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := publisher.Close(); !errors.Is(err, errStop) {
		t.Fatalf("Close error = %v", err)
	}
	if err := publisher.Close(); !errors.Is(err, errStop) {
		t.Fatalf("second Close error = %v", err)
	}
	if stopped.Load() != 1 {
		t.Fatalf("stop calls = %d", stopped.Load())
	}
	if event := publisher.Publish(Notification{Title: "late"}); event != nil {
		t.Fatalf("publish after close = %#v", event)
	}
	if err := publisher.Start(SourceFunc(func(PublishFunc) (StopFunc, error) {
		t.Fatal("closed publisher started source")
		return nil, nil
	})); !errors.Is(err, ErrClosed) {
		t.Fatalf("Start after close error = %v", err)
	}
}

func TestReplayPublisherConcurrentPublishAndAfter(t *testing.T) {
	publisher := newReplayPublisher(time.Now, time.Hour, 2000)
	const writers = 8
	const perWriter = 100

	var wg sync.WaitGroup
	wg.Add(writers)
	for range writers {
		go func() {
			defer wg.Done()
			for range perWriter {
				publisher.Publish(Notification{Title: "event"})
				_ = publisher.After(0)
			}
		}()
	}
	wg.Wait()

	events := publisher.After(0)
	if len(events) != writers*perWriter {
		t.Fatalf("event count = %d", len(events))
	}
	for i, event := range events {
		if event.Sequence != uint64(i+1) {
			t.Fatalf("events[%d].Sequence = %d", i, event.Sequence)
		}
	}
}
