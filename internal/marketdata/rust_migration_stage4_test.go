package marketdata

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type stage4MarketDataCorpus struct {
	Version              string                      `json:"version"`
	MarketDataOperations []stage4MarketDataOperation `json:"marketdataOperations"`
}

type stage4MarketDataOperation struct {
	Op         string          `json:"op"`
	ConsumerID string          `json:"consumerId"`
	Refs       []InstrumentRef `json:"refs"`
	Managed    bool            `json:"managed"`
	NowMS      int64           `json:"nowMs"`
}

func TestRustMigrationStage4DemandAndProviderLifecycleMatchesCorpus(t *testing.T) {
	var corpus stage4MarketDataCorpus
	readStage4MarketDataFixture(t, "provider-lifecycle-corpus.json", &corpus)
	if corpus.Version != "stage4.v1" {
		t.Fatalf("stage 4 corpus version = %q", corpus.Version)
	}

	registry := newSubscriptionRegistry()
	registry.externalTTL = 50 * time.Millisecond
	var now time.Time
	registry.now = func() time.Time { return now }
	for _, operation := range corpus.MarketDataOperations {
		if operation.Op != "acquire" || len(operation.Refs) == 0 {
			continue
		}
		now = time.UnixMilli(operation.NowMS)
		registry.acquireWithMode(operation.ConsumerID, operation.Refs, operation.Managed)
		if operation.Managed && !registry.hasManagedConsumers() {
			t.Fatalf("managed consumer %q was not retained", operation.ConsumerID)
		}
		if operation.ConsumerID == "chart" {
			refs := registry.activeSubscriptions()
			if len(refs) != 1 || refs[0].Channel != "SNAPSHOT" ||
				refs[0].Market != "US" || refs[0].Symbol != "AAPL" {
				t.Fatalf("normalized chart demand = %#v", refs)
			}
		}
	}

	provider := newSwitchablePushProvider(true)
	service := NewService(provider)
	service.Seed(Tick{InstrumentID: "US.AAPL"})
	service.subscriptions.acquireManaged("strategy", []InstrumentRef{{
		Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "1m",
	}})
	called := false
	if err := service.ChangeProvider(func() error { called = true; return nil }); !errors.Is(err, ErrManagedSubscriptionsActive) || called {
		t.Fatalf("managed switch gate = called %v, err %v", called, err)
	}
	service.subscriptions.clear("strategy")
	if err := service.ChangeProvider(func() error { return nil }); err != nil {
		t.Fatalf("provider switch after release: %v", err)
	}
	if service.CachedCount("US.AAPL") != 0 {
		t.Fatal("provider switch retained stale cache")
	}
}

func readStage4MarketDataFixture(t *testing.T, name string, target any) {
	t.Helper()
	directory := os.Getenv("JFTRADE_STAGE4_FIXTURE_ROOT")
	if directory == "" {
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("resolve stage 4 market-data test source")
		}
		directory = filepath.Join(filepath.Dir(source), "..", "..", "tests", "fixtures", "rust-migration", "stage4")
	}
	path := filepath.Join(directory, name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, target); err != nil {
		t.Fatal(err)
	}
}
