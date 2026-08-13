package servercore

import (
	"context"
	"testing"

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
			Services: Services{
				marketdataSvc: service,
				strategySvc:   stratsrv.NewService(nil, nil, runtime),
			},
		},
	}
	got := server.activeLiveStreamInstrumentIDs([]string{"US.AAPL", "HK.00700", "SH.600000", "SH.600000"})
	if len(got) != 3 || got[0] != "HK.00700" || got[1] != "SH.600000" || got[2] != "US.AAPL" {
		t.Fatalf("deduplicated active instruments = %#v", got)
	}
}
