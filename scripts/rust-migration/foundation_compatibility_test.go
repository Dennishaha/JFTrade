package rustmigration

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/shopspring/decimal"
)

type foundationCorpus struct {
	Version              int                        `json:"version"`
	Decimal              []decimalCase              `json:"decimal"`
	Fixed8               []fixed8Case               `json:"fixed8"`
	Timestamps           []timestampCase            `json:"timestamps"`
	Taxonomies           []taxonomyCase             `json:"taxonomies"`
	BrokerErrors         []brokerErrorCase          `json:"brokerErrors"`
	SnapshotAvailability []snapshotAvailabilityCase `json:"snapshotAvailability"`
}

type decimalCase struct {
	Input     string `json:"input"`
	Canonical string `json:"canonical"`
	JSON      string `json:"json"`
}

type fixed8Case struct {
	Input   string `json:"input"`
	Scaled  int64  `json:"scaled"`
	Storage string `json:"storage"`
	JSON    string `json:"json"`
}

type timestampCase struct {
	Input      string `json:"input"`
	JSON       string `json:"json"`
	UnixMillis int64  `json:"unixMillis"`
}

type taxonomyCase struct {
	Type  string `json:"type"`
	Input string `json:"input"`
	Known bool   `json:"known"`
}

type brokerErrorCase struct {
	BrokerID string `json:"brokerId"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Display  string `json:"display"`
}

type snapshotAvailabilityCase struct {
	Kind             broker.SnapshotAvailabilityKind `json:"kind"`
	FallbackEligible bool                            `json:"fallbackEligible"`
}

type stageManifest struct {
	Version int `json:"version"`
	Files   []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

func fixtureDirectory(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve compatibility test source")
	}
	return filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage2")
}

func loadFoundationCorpus(t *testing.T) foundationCorpus {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDirectory(t), "foundation.json"))
	if err != nil {
		t.Fatalf("read foundation corpus: %v", err)
	}
	var corpus foundationCorpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatalf("decode foundation corpus: %v", err)
	}
	if corpus.Version != 1 {
		t.Fatalf("foundation corpus version = %d, want 1", corpus.Version)
	}
	return corpus
}

func TestStage2ManifestPinsCompatibilityFixtures(t *testing.T) {
	directory := fixtureDirectory(t)
	data, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatalf("read stage 2 manifest: %v", err)
	}
	var manifest stageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode stage 2 manifest: %v", err)
	}
	if manifest.Version != 1 || len(manifest.Files) == 0 {
		t.Fatalf("stage 2 manifest is incomplete: version=%d files=%d", manifest.Version, len(manifest.Files))
	}
	for _, file := range manifest.Files {
		contents, err := os.ReadFile(filepath.Join(directory, file.Path))
		if err != nil {
			t.Fatalf("read pinned fixture %q: %v", file.Path, err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(contents)); got != file.SHA256 {
			t.Errorf("fixture %q sha256 = %s, want %s", file.Path, got, file.SHA256)
		}
	}
}

func TestFoundationCorpusPreservesDecimalAndFixed8Semantics(t *testing.T) {
	corpus := loadFoundationCorpus(t)
	for _, test := range corpus.Decimal {
		value, err := decimal.NewFromString(test.Input)
		if err != nil {
			t.Fatalf("decimal %q: %v", test.Input, err)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal decimal %q: %v", test.Input, err)
		}
		if value.String() != test.Canonical || string(encoded) != test.JSON {
			t.Errorf("decimal %q = (%q, %s), want (%q, %s)", test.Input, value.String(), encoded, test.Canonical, test.JSON)
		}
	}
	for _, test := range corpus.Fixed8 {
		value, err := fixedpoint.NewFromString(test.Input)
		if err != nil {
			t.Fatalf("fixed8 %q: %v", test.Input, err)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal fixed8 %q: %v", test.Input, err)
		}
		if int64(value) != test.Scaled || value.String() != test.Storage || string(encoded) != test.JSON {
			t.Errorf("fixed8 %q = (%d, %q, %s), want (%d, %q, %s)", test.Input, value, value.String(), encoded, test.Scaled, test.Storage, test.JSON)
		}
	}
}

func TestFoundationCorpusPreservesTimeTaxonomyAndErrorSemantics(t *testing.T) {
	corpus := loadFoundationCorpus(t)
	for _, test := range corpus.Timestamps {
		parsed, err := time.Parse(time.RFC3339Nano, test.Input)
		if err != nil {
			t.Fatalf("timestamp %q: %v", test.Input, err)
		}
		value := bbgotypes.Time(parsed)
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal timestamp %q: %v", test.Input, err)
		}
		if value.UnixMilli() != test.UnixMillis || string(encoded) != test.JSON {
			t.Errorf("timestamp %q = (%d, %s), want (%d, %s)", test.Input, value.UnixMilli(), encoded, test.UnixMillis, test.JSON)
		}
	}
	for _, test := range corpus.Taxonomies {
		if knownTaxonomyValue(test.Type, test.Input) != test.Known {
			t.Errorf("taxonomy %s %q known mismatch", test.Type, test.Input)
		}
	}
	for _, test := range corpus.BrokerErrors {
		if got := broker.NewBrokerError(test.BrokerID, test.Code, test.Message).Error(); got != test.Display {
			t.Errorf("BrokerError.Error() = %q, want %q", got, test.Display)
		}
	}
	for _, test := range corpus.SnapshotAvailability {
		err := broker.NewSnapshotAvailabilityError(test.Kind, errors.New("provider unavailable"))
		kind, ok := broker.SnapshotAvailability(err)
		if !ok || kind != test.Kind || broker.IsSnapshotFallbackEligible(err) != test.FallbackEligible {
			t.Errorf("availability %q = (%q, %t, %t)", test.Kind, kind, ok, broker.IsSnapshotFallbackEligible(err))
		}
	}
}

func knownTaxonomyValue(kind, value string) bool {
	switch kind {
	case "productClass":
		switch broker.ProductClass(value) {
		case broker.ProductClassEquity, broker.ProductClassFund, broker.ProductClassOption,
			broker.ProductClassWarrant, broker.ProductClassCBBC, broker.ProductClassFuture,
			broker.ProductClassEventContract, broker.ProductClassIndex, broker.ProductClassBond,
			broker.ProductClassPlate, broker.ProductClassUnknown:
			return true
		}
	case "marketSegment":
		switch broker.MarketSegment(value) {
		case broker.MarketSegmentSecurities, broker.MarketSegmentDerivatives, broker.MarketSegmentPrediction:
			return true
		}
	case "quantityMode":
		switch broker.QuantityMode(value) {
		case broker.QuantityModeUnits, broker.QuantityModeContracts, broker.QuantityModeAmount:
			return true
		}
	case "orderKind":
		switch broker.OrderKind(value) {
		case broker.OrderKindSingle, broker.OrderKindOptionCombo, broker.OrderKindEventSingle, broker.OrderKindEventParlay:
			return true
		}
	}
	return false
}
