package live

import (
	"reflect"
	"testing"
)

func TestNormalizeSubscriptions(t *testing.T) {
	got := NormalizeSubscriptions(Subscriptions{
		ActiveInstruments: []string{" us.aapl ", "HK.00700", "US.AAPL", ""},
		SecurityDetails: []SecurityDetailsSubscription{
			{Market: " hk ", Symbol: " 00700 ", InstrumentID: " hk.00700 "},
			{Market: "HK", Symbol: "IGNORED", InstrumentID: "HK.00700"},
			{Market: "", Symbol: "AAPL", InstrumentID: "US.AAPL"},
		},
		Depth: []DepthSubscription{
			{Market: " us ", Symbol: " tme ", InstrumentID: " us.tme ", Num: 0},
			{Market: "US", Symbol: "TME", InstrumentID: "US.TME", Num: 100},
			{Market: "US", Symbol: "TME", InstrumentID: "US.TME", Num: 50},
		},
		ConsoleRefresh: true,
	})

	want := Subscriptions{
		ActiveInstruments: []string{"HK.00700", "US.AAPL"},
		SecurityDetails: []SecurityDetailsSubscription{
			{Market: "HK", Symbol: "00700", InstrumentID: "HK.00700"},
		},
		Depth: []DepthSubscription{
			{Market: "US", Symbol: "TME", InstrumentID: "US.TME", Num: 1},
			{Market: "US", Symbol: "TME", InstrumentID: "US.TME", Num: 50},
		},
		ConsoleRefresh: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeSubscriptions() = %#v, want %#v", got, want)
	}
}

func TestClientRegistryTracksActiveInstruments(t *testing.T) {
	var registry ClientRegistry
	first := registry.Register()
	second := registry.Register()
	first.SetSubscriptions(Subscriptions{ActiveInstruments: []string{"US.AAPL", "HK.00700"}})
	second.SetSubscriptions(Subscriptions{ActiveInstruments: []string{"US.AAPL", "US.MSFT"}})

	if got, want := registry.ActiveInstrumentIDs(), []string{"HK.00700", "US.AAPL", "US.MSFT"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveInstrumentIDs() = %v, want %v", got, want)
	}
	registry.Unregister(first.ID())
	if got, want := registry.ActiveInstrumentIDs(), []string{"US.AAPL", "US.MSFT"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after unregister = %v, want %v", got, want)
	}
}

func TestClientSnapshotIsIsolatedAndUpdateIsCoalesced(t *testing.T) {
	client := newClient(1)
	client.SetSubscriptions(Subscriptions{ActiveInstruments: []string{"US.AAPL"}})
	client.SetSubscriptions(Subscriptions{ActiveInstruments: []string{"US.MSFT"}})

	select {
	case <-client.Updated():
	default:
		t.Fatal("expected subscription update signal")
	}
	select {
	case <-client.Updated():
		t.Fatal("expected update signals to be coalesced")
	default:
	}

	snapshot := client.Snapshot()
	snapshot.ActiveInstruments[0] = "CHANGED"
	if got := client.Snapshot().ActiveInstruments[0]; got != "US.MSFT" {
		t.Fatalf("stored subscription changed through snapshot: %q", got)
	}
}
