package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResearchDatabasePathAndRuntimeResource(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "runtime", "settings.json")
	want := filepath.Join(filepath.Dir(settingsPath), "research.db")
	if got := DeriveResearchDBPath(settingsPath); got != want {
		t.Fatalf("DeriveResearchDBPath() = %q, want %q", got, want)
	}
	override := filepath.Join(t.TempDir(), "custom", "research.sqlite")
	t.Setenv("JFTRADE_RESEARCH_DB", override)
	if got := DeriveResearchDBPath(settingsPath); got != override {
		t.Fatalf("DeriveResearchDBPath() override = %q, want %q", got, override)
	}
	if err := EnsureRuntimeLayout(settingsPath, filepath.Join(t.TempDir(), "backtest.db")); err != nil {
		t.Fatalf("EnsureRuntimeLayout: %v", err)
	}
	if info, err := os.Stat(filepath.Dir(override)); err != nil || !info.IsDir() {
		t.Fatalf("research directory info=%v err=%v", info, err)
	}
	resources := RuntimeResources(settingsPath, filepath.Join(t.TempDir(), "backtest.db"))
	for _, resource := range resources {
		if resource.ID != "research-db" {
			continue
		}
		if resource.Owner != "research" || resource.Path != override || resource.EnvironmentOverride != "JFTRADE_RESEARCH_DB" || !resource.Critical {
			t.Fatalf("research resource = %+v", resource)
		}
		return
	}
	t.Fatal("research-db runtime resource missing")
}
