package futu

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

type basicQuoteNonFiniteCorpus struct {
	Version string `json:"version"`
	Cases   []struct {
		Name         string `json:"name"`
		Price        string `json:"price"`
		Instrument   string `json:"instrument"`
		GoBehavior   string `json:"goBehavior"`
		RustBehavior string `json:"rustBehavior"`
	} `json:"cases"`
}

func TestQuoteSnapshotNonFinitePriceCorpusRecordsGoFailureBoundary(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "fixtures", "rust-migration", "stage9", "basic-quote-nonfinite.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	var corpus basicQuoteNonFiniteCorpus
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatalf("Unmarshal fixture: %v", err)
	}
	if corpus.Version != "stage9.basic-quote-nonfinite.v1" {
		t.Fatalf("version = %q", corpus.Version)
	}
	if len(corpus.Cases) == 0 {
		t.Fatal("non-finite price corpus is empty")
	}

	for _, testCase := range corpus.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			if testCase.GoBehavior != "panic" {
				t.Fatalf("fixture goBehavior = %q, want panic", testCase.GoBehavior)
			}
			price := parseBasicQuoteNonFinitePrice(t, testCase.Price)
			marketCode, symbol, ok := splitBasicQuoteNonFiniteInstrument(testCase.Instrument)
			if !ok {
				t.Fatalf("invalid instrument %q", testCase.Instrument)
			}
			panicked := false
			func() {
				defer func() {
					if recover() != nil {
						panicked = true
					}
				}()
				quoteSnapshotFromBasicQotAt(
					&qotcommonpb.BasicQot{
						Security: &qotcommonpb.Security{Market: new(marketCode), Code: new(symbol)},
						CurPrice: &price,
					},
					testCase.Instrument,
					time.Unix(0, 0).UTC(),
				)
			}()
			if !panicked {
				t.Fatalf("Go quote snapshot did not panic for %q", testCase.Price)
			}
		})
	}
}

func parseBasicQuoteNonFinitePrice(t *testing.T, value string) float64 {
	t.Helper()
	switch value {
	case "NaN":
		return math.NaN()
	case "+Inf":
		return math.Inf(1)
	case "-Inf":
		return math.Inf(-1)
	default:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			t.Fatalf("ParseFloat(%q): %v", value, err)
		}
		return parsed
	}
}

func splitBasicQuoteNonFiniteInstrument(value string) (int32, string, bool) {
	const prefix = "US."
	if len(value) <= len(prefix) || value[:len(prefix)] != prefix {
		return 0, "", false
	}
	return 11, value[len(prefix):], true
}
