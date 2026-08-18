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

func TestMarketDataProviderDefaultsToAKShareAndPersistsSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := store.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("default active provider = %q, want akshare", got)
	}
	if got := store.BacktestMarketDataProvider(); got != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("default backtest provider = %q, want akshare", got)
	}
	for _, value := range []string{"", "unknown"} {
		invalidPath := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(
			invalidPath,
			[]byte(`{"activeMarketDataProvider":"`+value+`"}`),
			0o600,
		); err != nil {
			t.Fatalf("write invalid provider %q: %v", value, err)
		}
		invalidStore, err := New(invalidPath)
		if err != nil {
			t.Fatalf("load invalid provider %q: %v", value, err)
		}
		if got := invalidStore.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderAKShare {
			t.Fatalf("invalid provider %q = %q, want akshare", value, got)
		}
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

func TestNormalizeActiveMarketDataProviderFallsBackToAKShare(t *testing.T) {
	if got := NormalizeActiveMarketDataProvider(" yfinance "); got != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("normalized yfinance provider = %q", got)
	}
	if got := NormalizeActiveMarketDataProvider(" AKSHARE "); got != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("normalized AKShare provider = %q", got)
	}
	for _, input := range []jfsettings.ActiveMarketDataProvider{"", "unknown"} {
		if got := NormalizeActiveMarketDataProvider(input); got != jfsettings.MarketDataProviderAKShare {
			t.Fatalf("normalized %q = %q, want akshare", input, got)
		}
	}
	if got := NormalizeActiveMarketDataProvider(" FUTU "); got != jfsettings.MarketDataProviderFutu {
		t.Fatalf("normalized futu provider = %q, want futu", got)
	}
}

func TestMarketDataProviderPersistsAKShareSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderAKShare); err != nil {
		t.Fatalf("SaveActiveMarketDataProvider: %v", err)
	}
	reloaded, err := New(path)
	if err != nil || reloaded.ActiveMarketDataProvider() != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("reloaded AKShare provider = %q, err=%v", reloaded.ActiveMarketDataProvider(), err)
	}
}

func TestBacktestProviderUpgradeCopiesGlobalSelectionOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderAKShare); err != nil {
		t.Fatalf("save global provider: %v", err)
	}
	if err := store.EnsureBacktestMarketDataProvider(); err != nil {
		t.Fatalf("upgrade backtest provider: %v", err)
	}
	if got := store.BacktestMarketDataProvider(); got != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("copied backtest provider = %q, want akshare", got)
	}
	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu); err != nil {
		t.Fatalf("switch global provider: %v", err)
	}
	if err := store.EnsureBacktestMarketDataProvider(); err != nil {
		t.Fatalf("repeat upgrade: %v", err)
	}
	if got := store.BacktestMarketDataProvider(); got != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("independent backtest provider = %q, want akshare", got)
	}

	reloaded, err := New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.BacktestMarketDataProvider(); got != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("reloaded backtest provider = %q, want akshare", got)
	}
}
