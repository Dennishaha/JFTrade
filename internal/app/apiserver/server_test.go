package apiserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestStartForRunArgsInitializesRuntimeLayout(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "var", "jftrade-api")
	settingsPath := filepath.Join(runtimeDir, "settings.json")
	backtestDBPath := filepath.Join(runtimeDir, "backtest.db")
	t.Setenv("JFTRADE_API_DISABLED", "")
	t.Setenv("JFTRADE_SETTINGS_PATH", settingsPath)
	t.Setenv("JFTRADE_BACKTEST_DB", backtestDBPath)
	t.Setenv("JFTRADE_API_BIND", "127.0.0.1:0")
	t.Setenv("JFTRADE_GUI_BIND", "127.0.0.1:0")
	t.Setenv("FUTU_OPEND_ADDR", "before-startup")

	store, err := settingsfile.New(settingsPath)
	if err != nil {
		t.Fatalf("settingsfile.New: %v", err)
	}
	if _, err := store.SaveIntegration(jfsettings.BrokerIntegration{
		Config: jfsettings.FutuIntegrationConfig{
			Host:    "127.0.0.4",
			APIPort: 24444,
		},
	}); err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	if got := os.Getenv("FUTU_OPEND_ADDR"); got != "before-startup" {
		t.Fatalf("pure store changed FUTU_OPEND_ADDR to %q", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdown, err := StartForRunArgs(ctx, []string{"api"})
	if err != nil {
		t.Fatalf("StartForRunArgs() error = %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown() error = %v", err)
		}
	}()

	if _, err := os.Stat(runtimeDir); err != nil {
		t.Fatalf("runtime dir not initialized: %v", err)
	}
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings file not initialized: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(backtestDBPath)); err != nil {
		t.Fatalf("backtest db dir not initialized: %v", err)
	}
	if got := os.Getenv("FUTU_OPEND_ADDR"); got != "127.0.0.4:24444" {
		t.Fatalf("startup FUTU_OPEND_ADDR = %q", got)
	}
}

func TestStartForRunArgsDisabledReturnsNoop(t *testing.T) {
	t.Setenv("JFTRADE_API_DISABLED", "true")

	shutdown, err := StartForRunArgs(context.Background(), []string{"api"})
	if err != nil {
		t.Fatalf("StartForRunArgs() error = %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
}
