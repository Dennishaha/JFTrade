package marketdata

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type switchablePushProvider struct {
	Provider
	push atomic.Bool
}

func newSwitchablePushProvider(push bool) *switchablePushProvider {
	provider := &switchablePushProvider{}
	provider.push.Store(push)
	return provider
}

func (p *switchablePushProvider) PushAvailable() bool {
	return p.push.Load()
}

type blockingSubscriptionReconciler struct {
	started chan struct{}
	release chan struct{}
}

type blockingCloseProviderStream struct {
	started chan struct{}
	release chan struct{}
}

func (*blockingCloseProviderStream) Connect(context.Context) error {
	return nil
}

func (s *blockingCloseProviderStream) Close() error {
	close(s.started)
	<-s.release
	return nil
}

func (r *blockingSubscriptionReconciler) ReconcileSubscriptions(
	context.Context,
	[]InstrumentRef,
) error {
	select {
	case <-r.started:
	default:
		close(r.started)
	}
	<-r.release
	return nil
}

func (*blockingSubscriptionReconciler) SubscriptionState() map[string]any {
	return nil
}

func TestProviderChangeAndManagedLeaseAreMutuallyExclusive(t *testing.T) {
	provider := newSwitchablePushProvider(true)
	service := NewService(provider)
	refs := []InstrumentRef{{
		Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "1m",
	}}
	lease, err := service.AcquireManagedSubscription(
		t.Context(), "strategy-runtime:one", refs,
	)
	if err != nil {
		t.Fatalf("AcquireManagedSubscription: %v", err)
	}
	changeCalled := false
	err = service.ChangeProvider(func() error {
		changeCalled = true
		provider.push.Store(false)
		return nil
	})
	if !errors.Is(err, ErrManagedSubscriptionsActive) || changeCalled {
		t.Fatalf("managed provider change = called %v, err %v", changeCalled, err)
	}
	lease.Release()
	if err := service.ChangeProvider(func() error {
		provider.push.Store(false)
		return nil
	}); err != nil {
		t.Fatalf("provider change after lease release: %v", err)
	}
	if lease, err := service.AcquireManagedSubscription(
		t.Context(), "strategy-runtime:two", refs,
	); lease != nil || !errors.Is(err, ErrManagedSubscriptionsUnavailable) {
		t.Fatalf("poll-only managed lease = %#v, %v", lease, err)
	}
}

func TestConcurrentManagedLeaseWinsBeforeProviderChange(t *testing.T) {
	provider := newSwitchablePushProvider(true)
	service := NewService(provider)
	reconciler := &blockingSubscriptionReconciler{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	service.SetSubscriptionReconciler(reconciler)
	refs := []InstrumentRef{{
		Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "1m",
	}}
	leaseResult := make(chan *ManagedSubscription, 1)
	leaseErrors := make(chan error, 1)
	go func() {
		lease, err := service.AcquireManagedSubscription(
			context.Background(), "strategy-runtime:one", refs,
		)
		leaseResult <- lease
		leaseErrors <- err
	}()
	<-reconciler.started

	changeErrors := make(chan error, 1)
	go func() {
		changeErrors <- service.ChangeProvider(func() error {
			provider.push.Store(false)
			return nil
		})
	}()
	select {
	case err := <-changeErrors:
		t.Fatalf("provider change escaped managed lease gate: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(reconciler.release)
	lease := <-leaseResult
	if err := <-leaseErrors; err != nil || lease == nil {
		t.Fatalf("managed lease = %#v, %v", lease, err)
	}
	if err := <-changeErrors; !errors.Is(err, ErrManagedSubscriptionsActive) {
		t.Fatalf("concurrent provider change error = %v", err)
	}
	lease.Release()
}

func TestConcurrentProviderChangeWinsBeforeManagedLease(t *testing.T) {
	provider := newSwitchablePushProvider(true)
	service := NewService(provider)
	changeStarted := make(chan struct{})
	releaseChange := make(chan struct{})
	changeErrors := make(chan error, 1)
	go func() {
		changeErrors <- service.ChangeProvider(func() error {
			close(changeStarted)
			<-releaseChange
			provider.push.Store(false)
			return nil
		})
	}()
	<-changeStarted

	leaseErrors := make(chan error, 1)
	go func() {
		_, err := service.AcquireManagedSubscription(
			context.Background(),
			"strategy-runtime:one",
			[]InstrumentRef{{
				Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "1m",
			}},
		)
		leaseErrors <- err
	}()
	select {
	case err := <-leaseErrors:
		t.Fatalf("managed lease escaped provider change gate: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseChange)
	if err := <-changeErrors; err != nil {
		t.Fatalf("provider change: %v", err)
	}
	if err := <-leaseErrors; !errors.Is(err, ErrManagedSubscriptionsUnavailable) {
		t.Fatalf("post-change managed lease error = %v", err)
	}
}

func TestProviderChangeInvalidatesCacheBeforeUnblockingReaders(t *testing.T) {
	provider := newSwitchablePushProvider(true)
	service := NewService(provider)
	stream := &blockingCloseProviderStream{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	service.collector = &Collector{
		cache:        service.cache,
		stream:       stream,
		state:        RuntimeState{Generation: 1},
		wake:         make(chan struct{}, 1),
		closeTimeout: 20 * time.Millisecond,
	}
	service.Seed(Tick{InstrumentID: "US.AAPL"})

	changeStarted := make(chan struct{})
	changeErrors := make(chan error, 1)
	go func() {
		changeErrors <- service.ChangeProvider(func() error {
			close(changeStarted)
			provider.push.Store(false)
			return nil
		})
	}()

	<-changeStarted
	<-stream.started
	if err := <-changeErrors; err != nil {
		t.Fatalf("ChangeProvider: %v", err)
	}
	close(stream.release)
	if provider.PushAvailable() {
		t.Fatal("provider callback did not run")
	}
	if service.CachedCount("US.AAPL") != 0 {
		t.Fatal("provider change retained old cache")
	}
}

func TestProviderChangeBlocksReadsDuringActivation(t *testing.T) {
	sample := tickAt("US.AAPL", "188.5", 10, time.Now().UTC())
	provider := &blockingSnapshotProvider{
		dataProviderStub: &dataProviderStub{},
		started:          make(chan struct{}),
		release:          make(chan struct{}),
		snapshot:         &sample,
	}
	close(provider.release)
	service := NewService(provider)
	changeStarted := make(chan struct{})
	releaseChange := make(chan struct{})
	changeErrors := make(chan error, 1)
	go func() {
		changeErrors <- service.ChangeProvider(func() error {
			close(changeStarted)
			<-releaseChange
			return nil
		})
	}()
	<-changeStarted

	readErrors := make(chan error, 1)
	go func() {
		_, err := service.GetSnapshot(
			context.Background(), "US", "AAPL", true,
		)
		readErrors <- err
	}()
	select {
	case <-provider.started:
		t.Fatal("provider read escaped activation barrier")
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseChange)
	if err := <-changeErrors; err != nil {
		t.Fatalf("ChangeProvider: %v", err)
	}
	<-provider.started
	if err := <-readErrors; err != nil {
		t.Fatalf("post-change provider read: %v", err)
	}
}

func TestProviderChangeWaitsForInflightReadThenClearsItsCache(t *testing.T) {
	sample := tickAt("US.AAPL", "188.5", 10, time.Now().UTC())
	provider := &blockingSnapshotProvider{
		dataProviderStub: &dataProviderStub{},
		started:          make(chan struct{}),
		release:          make(chan struct{}),
		snapshot:         &sample,
	}
	service := NewService(provider)
	readErrors := make(chan error, 1)
	go func() {
		_, err := service.GetSnapshot(
			context.Background(), "US", "AAPL", true,
		)
		readErrors <- err
	}()
	<-provider.started

	changeStarted := make(chan struct{})
	changeErrors := make(chan error, 1)
	go func() {
		changeErrors <- service.ChangeProvider(func() error {
			close(changeStarted)
			return nil
		})
	}()
	select {
	case <-changeStarted:
		t.Fatal("provider change published while an old read was in flight")
	case <-time.After(20 * time.Millisecond):
	}
	close(provider.release)
	if err := <-readErrors; err != nil {
		t.Fatalf("in-flight read: %v", err)
	}
	<-changeStarted
	if err := <-changeErrors; err != nil {
		t.Fatalf("ChangeProvider: %v", err)
	}
	if service.CachedCount("US.AAPL") != 0 {
		t.Fatal("provider change retained cache from completed old read")
	}
}

func TestFailedProviderChangeKeepsOldCollectorAndCache(t *testing.T) {
	service := NewService(newSwitchablePushProvider(true))
	stream := &blockingCloseProviderStream{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	service.collector = &Collector{
		cache:  service.cache,
		stream: stream,
		state:  RuntimeState{Generation: 1},
		wake:   make(chan struct{}, 1),
	}
	service.Seed(Tick{InstrumentID: "US.AAPL"})
	changeErr := errors.New("activation failed")
	if err := service.ChangeProvider(func() error {
		return changeErr
	}); !errors.Is(err, changeErr) {
		t.Fatalf("failed ChangeProvider error = %v", err)
	}
	select {
	case <-stream.started:
		t.Fatal("failed provider change closed the old stream")
	default:
	}
	if service.CachedCount("US.AAPL") != 1 {
		t.Fatal("failed provider change cleared the old cache")
	}
}
