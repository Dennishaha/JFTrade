package marketdata

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

type pollOnlyProviderStub struct {
	*dataProviderStub
}

func (*pollOnlyProviderStub) PushAvailable() bool {
	return false
}

type generationTickerProvider struct {
	*dataProviderStub
	afterQuery func()
}

func (p *generationTickerProvider) QueryTicker(
	ctx context.Context,
	instrumentID string,
) (*Tick, error) {
	tick, err := p.dataProviderStub.QueryTicker(ctx, instrumentID)
	if p.afterQuery != nil {
		p.afterQuery()
	}
	return tick, err
}

func TestProviderSwitchHelpersHandleNilAndResetBoundaries(t *testing.T) {
	var nilService *Service
	if err := nilService.ChangeProvider(nil); err == nil {
		t.Fatal("nil ChangeProvider returned nil error")
	}
	nilService.NotifyProviderChanged()
	if runtime := nilService.ProviderRuntime(); runtime != nil {
		t.Fatalf("nil ProviderRuntime = %#v", runtime)
	}

	var nilResolver *MarketSubsetInstrumentResolver
	nilResolver.Reset()
	var nilRegistry *subscriptionRegistry
	if nilRegistry.hasManagedConsumers() {
		t.Fatal("nil subscription registry reported managed consumers")
	}
}

func TestProviderSwitchRetainsOnlyCurrentGenerationTickCandles(t *testing.T) {
	sample := tickAt("US.AAPL", "188.5", 10, time.Now().UTC())
	provider := &generationTickerProvider{
		dataProviderStub: &dataProviderStub{
			descriptor: ProviderDescriptor{
				ProviderID: "test-polling",
				Source:     "test-polling",
				Capabilities: ProviderCapabilities{
					TickCandles: true,
				},
			},
			ticker: &sample,
		},
	}
	service := NewService(provider)
	provider.afterQuery = func() { service.providerGeneration.Add(1) }

	_, err := service.GetCandles(context.Background(), "US", "AAPL", "tick", 1, "", "")
	if !errors.Is(err, ErrProviderChanged) {
		t.Fatalf("stale ticker response error = %v", err)
	}
	if service.CachedCount("US.AAPL") != 0 {
		t.Fatal("stale ticker response was cached")
	}
}

func TestPollOnlyProviderHealthUsesPollingModes(t *testing.T) {
	healthErr := errors.New("helper unavailable")
	degraded := NewService(&pollOnlyProviderStub{
		dataProviderStub: &dataProviderStub{healthErr: healthErr},
	})
	status, err := degraded.ProviderStatus(context.Background())
	if err != nil || status.Health.StreamMode != "snapshot-poll-delayed" ||
		status.Health.LastError != healthErr.Error() {
		t.Fatalf("degraded poll-only provider status = %#v, err=%v", status.Health, err)
	}

	pollOnly := NewService(&pollOnlyProviderStub{
		dataProviderStub: &dataProviderStub{},
	})
	if _, err := pollOnly.AcquireSubscription(context.Background(), "chart", []InstrumentRef{{
		Market: "US", Symbol: "AAPL",
	}}); err != nil {
		t.Fatalf("AcquireSubscription: %v", err)
	}
	health, err := pollOnly.Health(context.Background())
	if err != nil || health.StreamMode != "snapshot-poll-fallback" || health.ActiveCount != 1 {
		t.Fatalf("poll-only health = %#v, err=%v", health, err)
	}
}

func TestPollOnlyProviderReadsDoNotRequireLogicalLease(t *testing.T) {
	sample := tickAt("US.AAPL", "188.5", 10, time.Now().UTC())
	provider := &pollOnlyProviderStub{dataProviderStub: &dataProviderStub{
		descriptor: ProviderDescriptor{
			ProviderID: "poll-only", Source: "poll-only",
			Capabilities: ProviderCapabilities{Snapshots: true},
		},
		snapshot: &sample,
	}}
	service := NewService(provider)
	service.SetSubscriptionReconciler(&fakeSubscriptionReconciler{})

	if _, err := service.GetSnapshot(context.Background(), "US", "AAPL", true); err != nil {
		t.Fatalf("poll-only snapshot without logical lease: %v", err)
	}
}

func TestProviderSwitchCacheUtilitiesCoverConcreteValuesAndCNRejection(t *testing.T) {
	left := decimal.RequireFromString("12.5")
	right := decimal.RequireFromString("12.5")
	if !decimalPointerEqual(&left, &right) {
		t.Fatal("equal concrete quote volumes compared unequal")
	}
	if market, symbol := normalizeCNAggregateRead("CN", "600519"); market != "CN" || symbol != "600519" {
		t.Fatalf("bare CN symbol was rewritten to %s.%s", market, symbol)
	}

	service := NewService(&dataProviderStub{})
	sample := tickAt("US.AAPL", "188.5", 10, time.Now().UTC())
	service.Seed(sample)
	if candles := service.TickCandles("US.AAPL", "", "", 1); len(candles) != 1 {
		t.Fatalf("TickCandles = %#v", candles)
	}
}

func TestCollectorCloseHelpersRejectElapsedDeadlines(t *testing.T) {
	deadline := time.Now().Add(-time.Millisecond)
	if err := closeStreamUntil(&blockingLifecycleStream{}, deadline); err == nil {
		t.Fatal("closeStreamUntil accepted an elapsed deadline")
	}
	var group sync.WaitGroup
	group.Add(1)
	if err := waitGroupUntil(&group, deadline); err == nil {
		t.Fatal("waitGroupUntil accepted an elapsed deadline")
	}
	group.Done()
}
