package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeFallbacksAndEnvironmentOverridesRemainUsable(t *testing.T) {
	if got := defaultRuntimeRoot(true, " \t "); got != DefaultRuntimeDir {
		t.Fatalf("defaultRuntimeRoot blank executable dir = %q", got)
	}
	if got := LaunchDefaultsForExecutableDir(true, " "); got.SettingsPath != filepath.Join(DefaultRuntimeDir, DefaultSettingsFilename) {
		t.Fatalf("embedded blank executable dir defaults = %#v", got)
	}

	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if path := DeriveDesktopLogPath(settingsPath, time.Time{}); filepath.Dir(path) != DeriveDesktopLogDir(settingsPath) || !strings.HasPrefix(filepath.Base(path), "desktop-") {
		t.Fatalf("zero-day desktop log path = %q", path)
	}

	for key, value := range map[string]string{
		"JFTRADE_ADK_SKILLS_DIR":          filepath.Join(t.TempDir(), "skills"),
		"JFTRADE_ADK_SESSION_DB":          filepath.Join(t.TempDir(), "sessions.db"),
		"JFTRADE_EXCHANGE_CALENDAR_DIR":   filepath.Join(t.TempDir(), "calendars"),
		"JFTRADE_REAL_TRADE_CONTROL_PATH": filepath.Join(t.TempDir(), "control.json"),
	} {
		t.Setenv(key, value)
	}
	if got := DeriveADKSkillsDir(settingsPath); got != getenvForTest(t, "JFTRADE_ADK_SKILLS_DIR") {
		t.Fatalf("skills dir = %q", got)
	}
	if got := DeriveADKSessionDBPath(settingsPath); got != getenvForTest(t, "JFTRADE_ADK_SESSION_DB") {
		t.Fatalf("session DB = %q", got)
	}
	if got := DeriveExchangeCalendarDir(settingsPath); got != getenvForTest(t, "JFTRADE_EXCHANGE_CALENDAR_DIR") {
		t.Fatalf("calendar dir = %q", got)
	}
	if got := deriveRealTradeControlPath(settingsPath); got != getenvForTest(t, "JFTRADE_REAL_TRADE_CONTROL_PATH") {
		t.Fatalf("real-trade control path = %q", got)
	}

	if got := envOrDefault("JFTRADE_RUNTIME_TEST_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("envOrDefault fallback = %q", got)
	}
	if got := firstNonEmpty(" ", "", "selected", "later"); got != "selected" {
		t.Fatalf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty(" ", ""); got != "" {
		t.Fatalf("empty firstNonEmpty = %q", got)
	}
	jftradeLogError(errors.New("best effort failure"), "not an error")
}

func getenvForTest(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Fatalf("%s unexpectedly empty", key)
	}
	return value
}
