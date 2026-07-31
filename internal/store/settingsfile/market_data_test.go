package settingsfile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestMarketDataProviderDefaultsToYFinanceAndPersistsSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := store.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("default active provider = %q, want yfinance", got)
	}

	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu); err != nil {
		t.Fatalf("SaveActiveMarketDataProvider futu: %v", err)
	}
	reloaded, err := New(path)
	if err != nil {
		t.Fatalf("reload futu: %v", err)
	}
	if got := reloaded.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderFutu {
		t.Fatalf("reloaded futu provider = %q", got)
	}

	if err := reloaded.SaveActiveMarketDataProvider(" YFINANCE "); err != nil {
		t.Fatalf("SaveActiveMarketDataProvider yfinance: %v", err)
	}
	reloaded, err = New(path)
	if err != nil {
		t.Fatalf("reload yfinance: %v", err)
	}
	if got := reloaded.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("reloaded yfinance provider = %q", got)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted map[string]json.RawMessage
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("decode persisted settings: %v", err)
	}
	if _, ok := persisted["yfinance"]; ok {
		t.Fatalf("persisted settings unexpectedly contain legacy yfinance block: %s", raw)
	}
	if string(persisted["activeMarketDataProvider"]) != `"yfinance"` {
		t.Fatalf("persisted active provider = %s", persisted["activeMarketDataProvider"])
	}
}

func TestLegacyYFinanceSettingsMigrateToFutuAndAreRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := map[string]any{
		"activeMarketDataProvider": "yfinance",
		"yfinance": map[string]any{
			"enabled":        true,
			"host":           "127.0.0.1",
			"port":           7788,
			"pythonBin":      "python3",
			"timeoutSeconds": 15,
		},
	}
	rawLegacy, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy settings: %v", err)
	}
	if err := os.WriteFile(path, rawLegacy, 0o600); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}

	store, err := New(path)
	if err != nil {
		t.Fatalf("New legacy settings: %v", err)
	}
	if got := store.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderFutu {
		t.Fatalf("migrated provider = %q, want futu", got)
	}

	rawMigrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated settings: %v", err)
	}
	var migrated map[string]json.RawMessage
	if err := json.Unmarshal(rawMigrated, &migrated); err != nil {
		t.Fatalf("decode migrated settings: %v", err)
	}
	if _, ok := migrated["yfinance"]; ok {
		t.Fatalf("migrated settings still contain legacy yfinance block: %s", rawMigrated)
	}
	if string(migrated["activeMarketDataProvider"]) != `"futu"` {
		t.Fatalf("migrated active provider = %s", migrated["activeMarketDataProvider"])
	}
}

func TestMarketDataProviderSaveRollsBackOnAtomicReplaceFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderYFinance); err != nil {
		t.Fatalf("save original provider: %v", err)
	}
	rawBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read original settings: %v", err)
	}

	replaceErr := errors.New("replace failed")
	store.replaceFile = func(string, string) error { return replaceErr }
	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu); !errors.Is(err, replaceErr) {
		t.Fatalf("SaveActiveMarketDataProvider error = %v", err)
	}
	if got := store.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("failed save changed active provider = %q", got)
	}
	rawAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings after failure: %v", err)
	}
	if !reflect.DeepEqual(rawAfter, rawBefore) {
		t.Fatalf("failed save changed settings file:\nbefore=%s\nafter=%s", rawBefore, rawAfter)
	}
}

func TestNormalizeActiveMarketDataProviderFallsBackToFutu(t *testing.T) {
	if got := NormalizeActiveMarketDataProvider(" yfinance "); got != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("normalized yfinance provider = %q", got)
	}
	for _, input := range []jfsettings.ActiveMarketDataProvider{"", "unknown", "futu"} {
		if got := NormalizeActiveMarketDataProvider(input); got != jfsettings.MarketDataProviderFutu {
			t.Fatalf("normalized %q = %q, want futu", input, got)
		}
	}
}
