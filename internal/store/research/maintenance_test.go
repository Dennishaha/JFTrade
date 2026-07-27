package research

import (
	"path/filepath"
	"testing"
)

func TestResearchMaintenanceCompactsLiveStoreAndFailsClosed(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "research.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompactMaintenanceResource(t.Context()); err != nil {
		t.Fatalf("CompactMaintenanceResource: %v", err)
	}
	if presets, err := store.ListScreenPresets(t.Context()); err != nil || len(presets) != 0 {
		t.Fatalf("ListScreenPresets after compact = %#v, %v", presets, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}
	if err := store.CompactMaintenanceResource(t.Context()); err == nil {
		t.Fatal("compact after close succeeded")
	}
	if err := (*Store)(nil).CompactMaintenanceResource(t.Context()); err == nil {
		t.Fatal("nil store compact succeeded")
	}
}
