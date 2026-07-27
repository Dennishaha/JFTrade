package trading

import (
	"testing"
	"time"
)

func TestSeenFillRetentionPrunesExpiredKeysAndBoundsConfiguration(t *testing.T) {
	store := newExecutionOrderStore()
	store.seenFillKeys["old-fill"] = time.Now().UTC().Add(-120 * 24 * time.Hour).Format(time.RFC3339Nano)
	store.seenFillKeys["recent-fill"] = time.Now().UTC().Format(time.RFC3339Nano)
	store.seenFillKeys["invalid-fill"] = "not-a-time"

	store.ConfigureSeenFillRetention(0)
	if store.SeenFillRetentionDays() != 90 {
		t.Fatalf("default retention days = %d, want 90", store.SeenFillRetentionDays())
	}
	if store.HasSeenFill("old-fill") {
		t.Fatal("old fill key was not pruned")
	}
	if !store.HasSeenFill("recent-fill") {
		t.Fatal("recent fill key was unexpectedly pruned")
	}
	if !store.HasSeenFill("invalid-fill") {
		t.Fatal("invalid fill timestamp should be preserved for manual inspection")
	}

	store.ConfigureSeenFillRetention(5000)
	if store.SeenFillRetentionDays() != 3650 {
		t.Fatalf("max retention days = %d, want 3650", store.SeenFillRetentionDays())
	}
}
