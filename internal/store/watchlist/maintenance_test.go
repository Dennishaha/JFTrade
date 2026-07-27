package watchlist

import (
	"path/filepath"
	"testing"
)

func TestWatchlistMaintenanceCompactsLiveStoreAndFailsClosed(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "watchlists.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompactMaintenanceResource(t.Context()); err != nil {
		t.Fatalf("CompactMaintenanceResource: %v", err)
	}
	if groups, err := store.ListGroups(t.Context()); err != nil || len(groups) == 0 {
		t.Fatalf("ListGroups after compact = %#v, %v", groups, err)
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
