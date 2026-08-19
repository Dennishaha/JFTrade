package pineworker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type stage4PineCorpus struct {
	Version string `json:"version"`
	Pine    struct {
		Workers    []struct{ WorkerID, Address string }
		Operations []map[string]any `json:"operations"`
	} `json:"pine"`
}

func TestRustMigrationStage4PineLifecycleMatchesCorpus(t *testing.T) {
	directory := os.Getenv("JFTRADE_STAGE4_FIXTURE_ROOT")
	if directory == "" {
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("resolve stage 4 Pine test source")
		}
		directory = filepath.Join(filepath.Dir(source), "..", "..", "..", "tests", "fixtures", "rust-migration", "stage4")
	}
	path := filepath.Join(directory, "provider-lifecycle-corpus.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var corpus stage4PineCorpus
	if err := json.Unmarshal(content, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Version != "stage4.v1" || len(corpus.Pine.Workers) != 2 || len(corpus.Pine.Operations) != 7 {
		t.Fatalf("unexpected Pine stage 4 fixture: %#v", corpus.Pine)
	}

	launcher := &fakeWorkerLauncher{}
	dialer := newFakeManagerDialer()
	manager := newTestManager(t, ManagerConfig{Workers: 2, StartPort: 45051}, launcher, dialer)
	if err := manager.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	request := validClientRequest()
	request.Mode = ModeLive
	request.SessionID = "session-a"
	request.SessionOperation = SessionOperationOpen
	if _, err := manager.RunScript(t.Context(), request); err != nil {
		t.Fatalf("open session: %v", err)
	}
	if _, err := manager.RunScript(context.Background(), validClientRequest()); err != nil {
		t.Fatalf("round robin request: %v", err)
	}
	request.SessionOperation = SessionOperationAppend
	request.ExpectedRevision = 1
	if _, err := manager.RunScript(t.Context(), request); err != nil {
		t.Fatalf("append pinned session: %v", err)
	}
	if dialer.transports["127.0.0.1:45051"].runs != 2 ||
		dialer.transports["127.0.0.1:45052"].runs != 1 {
		t.Fatal("Go Pine manager no longer preserves round-robin plus session pinning")
	}
	request.SessionOperation = SessionOperationClose
	request.ExpectedRevision = 2
	request.Source, request.Symbol, request.Timeframe, request.Candles = "", "", "", nil
	if _, err := manager.RunScript(t.Context(), request); err != nil {
		t.Fatalf("close session: %v", err)
	}
	request.SessionOperation = SessionOperationAppend
	if _, err := manager.RunScript(t.Context(), request); err == nil || !strings.Contains(err.Error(), "not pinned") {
		t.Fatalf("append after close error = %v", err)
	}
}
