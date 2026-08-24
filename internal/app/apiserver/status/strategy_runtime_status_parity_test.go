package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

type fixtureStrategyRuntimeSummary struct {
	summary stratsrv.RuntimeSummary
}

func (f fixtureStrategyRuntimeSummary) RuntimeSummary() stratsrv.RuntimeSummary {
	return f.summary
}

func TestStrategyRuntimeStatusMatchesRustMigrationCorpus(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "tests", "fixtures", "rust-migration", "stage9", "strategy-runtime-status.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	var corpus struct {
		Version string `json:"version"`
		Cases   []struct {
			Name          string                  `json:"name"`
			PortAvailable bool                    `json:"portAvailable"`
			State         stratsrv.RuntimeSummary `json:"state"`
			Expected      json.RawMessage         `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatalf("Unmarshal fixture: %v", err)
	}
	if corpus.Version != "stage9.strategy-runtime-status.v1" {
		t.Fatalf("version = %q", corpus.Version)
	}
	for _, testCase := range corpus.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			var source StrategyRuntimeSummarySource
			if testCase.PortAvailable {
				source = fixtureStrategyRuntimeSummary{summary: testCase.State}
			}
			actualJSON, err := json.Marshal(StrategyRuntimeSummary(source))
			if err != nil {
				t.Fatalf("Marshal actual: %v", err)
			}
			var actualValue any
			if err := json.Unmarshal(actualJSON, &actualValue); err != nil {
				t.Fatalf("Unmarshal actual: %v", err)
			}
			var expected any
			if err := json.Unmarshal(testCase.Expected, &expected); err != nil {
				t.Fatalf("Unmarshal expected: %v", err)
			}
			if !reflect.DeepEqual(actualValue, expected) {
				t.Fatalf("actual = %s, want %s", actualJSON, testCase.Expected)
			}
		})
	}
}
