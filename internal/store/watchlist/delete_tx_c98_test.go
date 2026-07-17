package watchlist

import (
	"testing"

	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

func TestCoverage98DeleteGroupNeverPartiallyRemovesWatchlistState(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *Store, domain.Group)
	}{
		{
			name: "membership lookup failure",
			setup: func(t *testing.T, store *Store, _ domain.Group) {
				mustStoreExec(t, store, `DROP TABLE watchlist_memberships`)
			},
		},
		{
			name: "origin cleanup failure",
			setup: func(t *testing.T, store *Store, group domain.Group) {
				coverage98SeedGroupMembership(t, store, group.ID, "US.AAPL")
				mustStoreExec(t, store, `INSERT INTO watchlist_membership_origins
					(group_id, instrument_id, source_id, remote_group_id, last_imported_at)
					VALUES (?, 'US.AAPL', 'broker', 'favorites', '2026-01-01T00:00:00Z')`, group.ID)
				installAbortTrigger(t, store, "watchlist_membership_origins", "DELETE", "fail_delete_group_origin_cleanup")
			},
		},
		{
			name: "membership cleanup failure",
			setup: func(t *testing.T, store *Store, group domain.Group) {
				coverage98SeedGroupMembership(t, store, group.ID, "US.AAPL")
				installAbortTrigger(t, store, "watchlist_memberships", "DELETE", "fail_delete_group_membership_cleanup")
			},
		},
		{
			name: "binding cleanup failure",
			setup: func(t *testing.T, store *Store, group domain.Group) {
				mustStoreExec(t, store, `INSERT INTO watchlist_bindings
					(binding_id, source_id, remote_group_id, remote_name, local_group_id, created_at, updated_at)
					VALUES ('coverage-binding', 'broker', 'favorites', 'Favorites', ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, group.ID)
				installAbortTrigger(t, store, "watchlist_bindings", "DELETE", "fail_delete_group_binding_cleanup")
			},
		},
		{
			name: "group row removal failure",
			setup: func(t *testing.T, store *Store, _ domain.Group) {
				installAbortTrigger(t, store, "watchlist_groups", "DELETE", "fail_delete_group_row")
			},
		},
		{
			name: "instrument revision failure",
			setup: func(t *testing.T, store *Store, group domain.Group) {
				coverage98SeedGroupMembership(t, store, group.ID, "US.AAPL")
				installAbortTrigger(t, store, "watchlist_instruments", "UPDATE", "fail_delete_group_revision")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openImportCoverageStore(t)
			group, err := store.CreateGroup(t.Context(), "coverage-delete-"+test.name)
			if err != nil {
				t.Fatalf("CreateGroup: %v", err)
			}
			test.setup(t, store, group)
			if err := store.DeleteGroup(t.Context(), group.ID); err == nil {
				t.Fatal("DeleteGroup unexpectedly succeeded after a write failure")
			}

			var count int
			if err := store.DB().GetContext(t.Context(), &count, `SELECT COUNT(*) FROM watchlist_groups WHERE group_id = ?`, group.ID); err != nil {
				t.Fatalf("verify retained group: %v", err)
			}
			if count != 1 {
				t.Fatalf("failed DeleteGroup removed the group, count=%d", count)
			}
		})
	}
}

func TestCoverage98RemoteReplacementAndBindingRemovalKeepPriorStateOnStorageFailure(t *testing.T) {
	t.Run("remote group delete failure", func(t *testing.T) {
		store := openImportCoverageStore(t)
		initial := domain.RemoteGroup{RemoteGroupID: "favorites", Name: "Favorites", Type: "custom"}
		if err := store.ReplaceRemoteGroups(t.Context(), "broker", []domain.RemoteGroup{initial}); err != nil {
			t.Fatalf("seed remote groups: %v", err)
		}
		installAbortTrigger(t, store, "watchlist_remote_groups", "DELETE", "fail_remote_group_replace")
		if err := store.ReplaceRemoteGroups(t.Context(), "broker", []domain.RemoteGroup{{RemoteGroupID: "new", Name: "New", Type: "custom"}}); err == nil {
			t.Fatal("ReplaceRemoteGroups unexpectedly succeeded")
		}
		groups, err := store.ListRemoteGroups(t.Context(), "broker")
		if err != nil || len(groups) != 1 || groups[0].RemoteGroupID != initial.RemoteGroupID {
			t.Fatalf("failed replacement lost prior groups: %#v, %v", groups, err)
		}
	})

	t.Run("binding delete failure", func(t *testing.T) {
		store := openImportCoverageStore(t)
		mustStoreExec(t, store, `INSERT INTO watchlist_bindings
			(binding_id, source_id, remote_group_id, remote_name, local_group_id, created_at, updated_at)
			VALUES ('coverage-binding', 'broker', 'favorites', 'Favorites', 'default', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
		installAbortTrigger(t, store, "watchlist_bindings", "DELETE", "fail_binding_delete")
		if err := store.DeleteBinding(t.Context(), "coverage-binding"); err == nil {
			t.Fatal("DeleteBinding unexpectedly succeeded")
		}
		bindings, err := store.ListBindings(t.Context(), "broker")
		if err != nil || len(bindings) != 1 || bindings[0].ID != "coverage-binding" {
			t.Fatalf("failed binding removal lost persisted binding: %#v, %v", bindings, err)
		}
	})
}

func coverage98SeedGroupMembership(t *testing.T, store *Store, groupID, instrumentID string) {
	t.Helper()
	if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
		InstrumentID:     instrumentID,
		GroupIDs:         []string{groupID},
		ExpectedRevision: 0,
	}); err != nil {
		t.Fatalf("seed group membership: %v", err)
	}
}
