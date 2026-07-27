package servercore

import (
	"context"
	"testing"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

type heartbeatRuntimeStub struct {
	activeInstrumentIDs []string
}

func (s *heartbeatRuntimeStub) Start(context.Context, stratsrv.ManagedInstance) error { return nil }
func (s *heartbeatRuntimeStub) Stop(string)                                           {}
func (s *heartbeatRuntimeStub) GetObservation(string) (stratsrv.RuntimeObservation, bool) {
	return stratsrv.RuntimeObservation{}, false
}
func (s *heartbeatRuntimeStub) RuntimeSummary() stratsrv.RuntimeSummary {
	return stratsrv.RuntimeSummary{}
}
func (s *heartbeatRuntimeStub) ActiveInstrumentIDs() []string {
	return append([]string(nil), s.activeInstrumentIDs...)
}

func TestLiveHeartbeatSampleAndReasonBoundaries(t *testing.T) {
	now := time.Now().UTC()
	service := mdsrv.NewService(nil)
	server := &Server{serverApplication: serverApplication{
		marketdataSvc: service,
	}}
	service.Seed(mdsrv.Tick{InstrumentID: "US.FRESH", ObservedAt: now.Add(-time.Second).Format(time.RFC3339Nano)})
	service.Seed(mdsrv.Tick{InstrumentID: "US.STALE", ObservedAt: now.Add(-(liveHeartbeatStaleThreshold + time.Second)).Format(time.RFC3339Nano)})
	service.Seed(mdsrv.Tick{InstrumentID: "US.INVALID", ObservedAt: "invalid"})

	summary := server.summarizeLiveHeartbeatSamples(now, []string{"US.MISSING", "US.STALE", "US.FRESH", "US.INVALID"})
	if summary.freshCount != 1 || summary.staleCount != 3 || summary.latestObservedAtText == nil {
		t.Fatalf("heartbeat summary = %#v", summary)
	}
	if _, ok := server.liveHeartbeatObservedAt("US.MISSING"); ok {
		t.Fatal("missing observation reported as present")
	}
	if _, ok := server.liveHeartbeatObservedAt("US.INVALID"); ok {
		t.Fatal("invalid observation reported as present")
	}

	reasons := liveHeartbeatStaleReasons(1, 1, false, true, true)
	if len(reasons) != 4 {
		t.Fatalf("all stale reasons = %#v", reasons)
	}
	if got := liveHeartbeatRefreshTime(now); got == nil {
		t.Fatal("nonzero refresh time returned nil")
	}
	if mode := liveHeartbeatTransportMode(0, true); mode != "push-stream" {
		t.Fatalf("connected transport mode = %q", mode)
	}
	if mode := liveHeartbeatTransportMode(1, false); mode != "snapshot-poll-fallback" {
		t.Fatalf("fallback transport mode = %q", mode)
	}
}

func TestLiveHeartbeatActiveInstrumentDeduplicationBoundaries(t *testing.T) {
	if got := (&Server{}).activeMarketInstrumentIDs(); got != nil {
		t.Fatalf("nil market service instruments = %#v", got)
	}

	service := mdsrv.NewService(nil)
	if _, err := service.AcquireSubscription(context.Background(), "chart", []mdsrv.InstrumentRef{{Market: "US", Symbol: "AAPL"}}); err != nil {
		t.Fatalf("AcquireSubscription: %v", err)
	}
	runtime := &heartbeatRuntimeStub{activeInstrumentIDs: []string{"US.AAPL", "HK.00700"}}
	server := &Server{
		serverApplication: serverApplication{
			marketdataSvc: service,
			strategySvc:   stratsrv.NewService(nil, nil, runtime),
		},
	}
	got := server.activeLiveStreamInstrumentIDs([]string{"US.AAPL", "HK.00700", "SH.600000", "SH.600000"})
	if len(got) != 3 || got[0] != "HK.00700" || got[1] != "SH.600000" || got[2] != "US.AAPL" {
		t.Fatalf("deduplicated active instruments = %#v", got)
	}
}
