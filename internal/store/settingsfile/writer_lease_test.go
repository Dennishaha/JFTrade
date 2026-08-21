package settingsfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/ownerlock"
)

func TestSettingsWritesRequireOwnerLeaseButOpeningRemainsReadOnlySafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"appearance":{"theme":"system"}}`), 0o600); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	held, err := ownerlock.Acquire(path, ownerlock.CurrentDiagnostic("rust-test", "lock-conflict"))
	if err != nil {
		t.Fatalf("hold external writer lease: %v", err)
	}
	store, err := New(path)
	if err != nil {
		t.Fatalf("read settings while lease held: %v", err)
	}
	if _, err := store.SaveAppearance(DefaultUIAppearanceSettings()); !errors.Is(err, ownerlock.ErrHeld) {
		t.Fatalf("settings write conflict error = %v", err)
	}
	if err := held.Close(); err != nil {
		t.Fatalf("release external writer lease: %v", err)
	}
	if _, err := store.SaveAppearance(DefaultUIAppearanceSettings()); err != nil {
		t.Fatalf("settings write after release: %v", err)
	}
	if _, err := os.Stat(ownerlock.LockPath(path)); err != nil {
		t.Fatalf("settings lock file must remain after release: %v", err)
	}
}
