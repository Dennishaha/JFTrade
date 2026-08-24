package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestMarketDataRuntimeStatusMatchesRustMigrationCorpus(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "tests", "fixtures", "rust-migration", "stage9", "market-data-runtime-status.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	var corpus struct {
		Version string `json:"version"`
		Cases   []struct {
			Name          string          `json:"name"`
			PortAvailable bool            `json:"portAvailable"`
			State         fixtureRuntime  `json:"state"`
			Expected      json.RawMessage `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatalf("Unmarshal fixture: %v", err)
	}
	if corpus.Version != "stage9.market-data-runtime-status.v1" {
		t.Fatalf("version = %q", corpus.Version)
	}
	for _, testCase := range corpus.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			actual := any(MarketDataRuntimeSummary(nil))
			if testCase.PortAvailable {
				actual = marketDataRuntimeSummary(testCase.State.runtimeState(t))
			}
			var expected any
			if err := json.Unmarshal(testCase.Expected, &expected); err != nil {
				t.Fatalf("Unmarshal expected: %v", err)
			}
			actualJSON, err := json.Marshal(actual)
			if err != nil {
				t.Fatalf("Marshal actual: %v", err)
			}
			var actualValue any
			if err := json.Unmarshal(actualJSON, &actualValue); err != nil {
				t.Fatalf("Unmarshal actual: %v", err)
			}
			if !reflect.DeepEqual(actualValue, expected) {
				t.Fatalf("actual = %s, want %s", actualJSON, testCase.Expected)
			}
		})
	}
}

type fixtureRuntime struct {
	Connected       bool    `json:"connected"`
	Closed          bool    `json:"closed"`
	Generation      uint64  `json:"generation"`
	ActiveCount     int     `json:"activeCount"`
	LastRefreshAt   *string `json:"lastRefreshAt"`
	QuoteRetryAt    *string `json:"quoteRetryAt"`
	QuoteFailures   int     `json:"quoteFailures"`
	QuoteLastError  *string `json:"quoteLastError"`
	StreamRetryAt   *string `json:"streamRetryAt"`
	StreamFailures  int     `json:"streamFailures"`
	StreamLastError *string `json:"streamLastError"`
}

func (f fixtureRuntime) runtimeState(t *testing.T) mdsrv.RuntimeState {
	t.Helper()
	return mdsrv.RuntimeState{
		Connected: f.Connected, Closed: f.Closed, Generation: f.Generation, ActiveCount: f.ActiveCount,
		LastRefreshAt: parseFixtureTime(t, f.LastRefreshAt), QuoteRetryAt: parseFixtureTime(t, f.QuoteRetryAt),
		QuoteFailures: f.QuoteFailures, QuoteLastError: fixtureString(f.QuoteLastError),
		StreamRetryAt: parseFixtureTime(t, f.StreamRetryAt), StreamFailures: f.StreamFailures,
		StreamLastError: fixtureString(f.StreamLastError),
	}
}

func parseFixtureTime(t *testing.T, value *string) time.Time {
	t.Helper()
	if value == nil {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		t.Fatalf("Parse time %q: %v", *value, err)
	}
	return parsed
}

func fixtureString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
