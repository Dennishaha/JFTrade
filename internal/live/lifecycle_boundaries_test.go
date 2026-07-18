package live

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestNilClientAndPublisherBoundaries(t *testing.T) {
	var client *Client
	if client.ID() != 0 {
		t.Fatalf("nil client ID = %d, want 0", client.ID())
	}
	if client.Updated() != nil {
		t.Fatalf("nil client Updated should be nil")
	}
	if got := client.Snapshot(); !reflect.DeepEqual(got, Subscriptions{}) {
		t.Fatalf("nil client Snapshot = %#v", got)
	}
	client.SetSubscriptions(Subscriptions{ActiveInstruments: []string{"US.AAPL"}})

	var publisher *ReplayPublisher
	if event := publisher.Publish(Notification{Title: "ignored"}); event != nil {
		t.Fatalf("nil publisher Publish = %#v", event)
	}
	if events := publisher.After(0); events != nil {
		t.Fatalf("nil publisher After = %#v", events)
	}
	if err := publisher.Start(nil); err != nil {
		t.Fatalf("nil publisher Start = %v", err)
	}
	if err := publisher.Close(); err != nil {
		t.Fatalf("nil publisher Close = %v", err)
	}
}

func TestReplayPublisherStartErrorAndNilSourceAreNoops(t *testing.T) {
	publisher := NewReplayPublisher()
	if events := publisher.After(0); events != nil {
		t.Fatalf("empty publisher events = %#v", events)
	}
	if err := publisher.Start(nil); err != nil {
		t.Fatalf("Start nil source: %v", err)
	}
	upstream := errors.New("source failed")
	err := publisher.Start(SourceFunc(func(PublishFunc) (StopFunc, error) {
		return nil, upstream
	}))
	if !errors.Is(err, upstream) {
		t.Fatalf("Start source error = %v, want upstream", err)
	}
	if err := publisher.Start(SourceFunc(func(PublishFunc) (StopFunc, error) {
		return nil, nil
	})); err != nil {
		t.Fatalf("Start source with nil stop: %v", err)
	}
	if err := publisher.Close(); err != nil {
		t.Fatalf("Close after failed source: %v", err)
	}
}

func TestReplayPublisherStartRacingCloseStopsLateSource(t *testing.T) {
	publisher := NewReplayPublisher()
	started := make(chan struct{})
	release := make(chan struct{})
	stopped := make(chan struct{}, 1)
	done := make(chan error, 1)

	go func() {
		done <- publisher.Start(SourceFunc(func(PublishFunc) (StopFunc, error) {
			close(started)
			<-release
			return func() error {
				stopped <- struct{}{}
				return nil
			}, nil
		}))
	}()

	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- publisher.Close() }()
	for {
		publisher.mu.Lock()
		closed := publisher.closed
		publisher.mu.Unlock()
		if closed {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(release)

	if err := <-done; !errors.Is(err, ErrClosed) {
		t.Fatalf("late Start error = %v, want ErrClosed", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("late source stop was not called")
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("Close while start racing: %v", err)
	}
}
