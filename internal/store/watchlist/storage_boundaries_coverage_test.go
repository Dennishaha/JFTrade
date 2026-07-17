package watchlist

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

func TestCommitImportRejectsStaleAndBrokenStorageStates(t *testing.T) {
	t.Run("missing preview", func(t *testing.T) {
		store := openImportCoverageStore(t)
		_, err := store.CommitImport(t.Context(), domain.CommitImportStoreInput{Preview: domain.ImportPreview{ID: "missing", RemoteHash: "hash"}})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("CommitImport(missing preview) = %v", err)
		}
	})

	t.Run("missing local group", func(t *testing.T) {
		store := openImportCoverageStore(t)
		preview := importCoveragePreview("missing-local-group", "deleted-group", 1, "hash")
		if err := store.SaveImportPreview(t.Context(), preview); err != nil {
			t.Fatal(err)
		}
		_, err := store.CommitImport(t.Context(), domain.CommitImportStoreInput{Preview: preview})
		if !errors.Is(err, domain.ErrStalePreview) {
			t.Fatalf("CommitImport(missing local group) = %v", err)
		}
	})

	t.Run("membership lookup failure", func(t *testing.T) {
		store, input, _ := importCommitFailureFixture(t)
		mustStoreExec(t, store, `DROP TABLE watchlist_memberships`)
		if _, err := store.CommitImport(t.Context(), input); err == nil {
			t.Fatal("CommitImport succeeded without the membership table")
		}
	})

	t.Run("security alias upsert failure", func(t *testing.T) {
		store, input, _ := importCommitFailureFixture(t)
		input.RemoteMembers[0].BrokerCode = ""
		installAbortTrigger(t, store, "watchlist_instrument_aliases", "INSERT", "fail_security_alias")
		if _, err := store.CommitImport(t.Context(), input); err == nil {
			t.Fatal("CommitImport succeeded after security alias persistence failed")
		}
	})

	t.Run("local deletion failure", func(t *testing.T) {
		store, input, groupID := importCommitFailureFixture(t)
		if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
			InstrumentID: "US.TSLA", GroupIDs: []string{groupID}, ExpectedRevision: 0,
		}); err != nil {
			t.Fatalf("seed local membership: %v", err)
		}
		group, err := store.GetGroup(t.Context(), groupID)
		if err != nil {
			t.Fatal(err)
		}
		input.Preview = importCoveragePreview("local-delete-failure", group.ID, group.Revision, "fault-hash")
		input.RemoteMembers = nil
		input.DeleteInstrumentIDs = []string{"US.TSLA"}
		if err := store.SaveImportPreview(t.Context(), input.Preview); err != nil {
			t.Fatal(err)
		}
		installAbortTrigger(t, store, "watchlist_memberships", "DELETE", "fail_local_delete")
		if _, err := store.CommitImport(t.Context(), input); err == nil {
			t.Fatal("CommitImport succeeded after membership deletion failed")
		}
	})

	t.Run("origin cleanup failure", func(t *testing.T) {
		store, input, groupID := importCommitFailureFixture(t)
		if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
			InstrumentID: "US.TSLA", GroupIDs: []string{groupID}, ExpectedRevision: 0,
		}); err != nil {
			t.Fatalf("seed local membership: %v", err)
		}
		group, err := store.GetGroup(t.Context(), groupID)
		if err != nil {
			t.Fatal(err)
		}
		input.Preview = importCoveragePreview("origin-cleanup-failure", group.ID, group.Revision, "fault-hash")
		input.RemoteMembers = nil
		input.DeleteInstrumentIDs = []string{"US.TSLA"}
		if err := store.SaveImportPreview(t.Context(), input.Preview); err != nil {
			t.Fatal(err)
		}
		mustStoreExec(t, store, `INSERT INTO watchlist_membership_origins
			(group_id, instrument_id, source_id, remote_group_id, last_imported_at)
			VALUES (?, 'US.TSLA', 'broker:main', 'favorites', '2026-01-01T00:00:00Z')`, groupID)
		installAbortTrigger(t, store, "watchlist_membership_origins", "DELETE", "fail_local_origin_cleanup")
		if _, err := store.CommitImport(t.Context(), input); err == nil {
			t.Fatal("CommitImport succeeded after origin cleanup failed")
		}
	})

	t.Run("revision update failure", func(t *testing.T) {
		store, input, _ := importCommitFailureFixture(t)
		installAbortTrigger(t, store, "watchlist_instruments", "UPDATE", "fail_revision")
		if _, err := store.CommitImport(t.Context(), input); err == nil {
			t.Fatal("CommitImport succeeded after membership revision update failed")
		}
	})
}

func TestImportStoreDetectsStaleCompletionAndCursorStorageFailure(t *testing.T) {
	store := openImportCoverageStore(t)
	preview := importCoveragePreview("completion-race", "", 0, "hash")
	if err := store.SaveImportPreview(t.Context(), preview); err != nil {
		t.Fatal(err)
	}
	tx, err := store.DB().BeginWrite(t.Context(), nil)
	if err != nil {
		t.Fatalf("BeginWrite: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(t.Context(), `UPDATE watchlist_import_previews SET status = 'committed' WHERE preview_id = ?`, preview.ID); err != nil {
		t.Fatal(err)
	}
	if err := markImportPreviewCommitted(t.Context(), tx, preview.ID); !errors.Is(err, domain.ErrStalePreview) {
		t.Fatalf("markImportPreviewCommitted(raced) = %v", err)
	}

	store = openImportCoverageStore(t)
	mustStoreExec(t, store, `DROP TABLE watchlist_import_runs`)
	if _, err := store.ListImportRuns(t.Context(), "", "missing-cursor", 10); err == nil {
		t.Fatal("ListImportRuns succeeded without import-run storage")
	}
}

func TestItemStorageBoundaryPaths(t *testing.T) {
	t.Run("pagination and empty metadata", func(t *testing.T) {
		store := openImportCoverageStore(t)
		groups, err := store.ListGroups(t.Context())
		if err != nil || len(groups) != 1 {
			t.Fatalf("ListGroups = %#v, %v", groups, err)
		}
		for _, instrumentID := range []string{"US.AAPL", "US.MSFT"} {
			if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
				InstrumentID: instrumentID, GroupIDs: []string{groups[0].ID}, ExpectedRevision: 0,
			}); err != nil {
				t.Fatalf("seed %s: %v", instrumentID, err)
			}
		}
		page, err := store.ListItems(t.Context(), domain.ListItemsOptions{Limit: 1})
		if err != nil || len(page.Items) != 1 || page.NextCursor != "US.AAPL" {
			t.Fatalf("paginated items = %#v, %v", page, err)
		}
		importedAt := "2026-01-01T00:00:00Z"
		items, err := store.hydrateItems(t.Context(), []instrumentRow{{
			ID: "US.EMPTY", Market: "US", Symbol: "EMPTY", ImportedAt: &importedAt,
		}})
		if err != nil || len(items) != 1 || items[0].Groups == nil || items[0].SourceIDs == nil || items[0].LastImportedAt == nil {
			t.Fatalf("sparse hydrated item = %#v, %v", items, err)
		}
	})

	t.Run("group hydration failure", func(t *testing.T) {
		store := openImportCoverageStore(t)
		groups, _ := store.ListGroups(t.Context())
		if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
			InstrumentID: "US.AAPL", GroupIDs: []string{groups[0].ID}, ExpectedRevision: 0,
		}); err != nil {
			t.Fatal(err)
		}
		mustStoreExec(t, store, `DROP TABLE watchlist_groups`)
		if _, err := store.ListItems(t.Context(), domain.ListItemsOptions{Limit: 10}); err == nil {
			t.Fatal("ListItems succeeded after group hydration storage was removed")
		}
	})

	t.Run("source hydration and membership lookup failures", func(t *testing.T) {
		store := openImportCoverageStore(t)
		mustStoreExec(t, store, `DROP TABLE watchlist_membership_origins`)
		if _, err := store.hydrateItems(t.Context(), []instrumentRow{{ID: "US.AAPL"}}); err == nil {
			t.Fatal("hydrateItems succeeded without membership origin storage")
		}

		store = openImportCoverageStore(t)
		groups, _ := store.ListGroups(t.Context())
		if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
			InstrumentID: "US.AAPL", GroupIDs: []string{groups[0].ID}, ExpectedRevision: 0,
		}); err != nil {
			t.Fatal(err)
		}
		mustStoreExec(t, store, `DROP TABLE watchlist_memberships`)
		if _, err := store.GetMemberships(t.Context(), "US.AAPL"); err == nil {
			t.Fatal("GetMemberships succeeded without membership storage")
		}
	})
}

func TestMembershipStorageFaultsAndMigrationGuards(t *testing.T) {
	t.Run("new instrument with nonzero revision", func(t *testing.T) {
		store := openImportCoverageStore(t)
		if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
			InstrumentID: "US.AAPL", ExpectedRevision: 1,
		}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("ReplaceMemberships(nonzero new revision) = %v", err)
		}
	})

	t.Run("missing group and group lookup failure", func(t *testing.T) {
		store := openImportCoverageStore(t)
		if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
			InstrumentID: "US.AAPL", GroupIDs: []string{"missing"}, ExpectedRevision: 0,
		}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("ReplaceMemberships(missing group) = %v", err)
		}

		store = openImportCoverageStore(t)
		groups, err := store.ListGroups(t.Context())
		if err != nil || len(groups) != 1 {
			t.Fatalf("ListGroups = %#v, %v", groups, err)
		}
		mustStoreExec(t, store, `DROP TABLE watchlist_groups`)
		if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
			InstrumentID: "US.AAPL", GroupIDs: []string{groups[0].ID}, ExpectedRevision: 0,
		}); err == nil {
			t.Fatal("ReplaceMemberships succeeded without group storage")
		}
	})

	t.Run("membership diff and metadata write failures", func(t *testing.T) {
		store := openImportCoverageStore(t)
		groups, _ := store.ListGroups(t.Context())
		installAbortTrigger(t, store, "watchlist_memberships", "INSERT", "fail_membership_diff")
		if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
			InstrumentID: "US.AAPL", GroupIDs: []string{groups[0].ID}, ExpectedRevision: 0,
		}); err == nil {
			t.Fatal("ReplaceMemberships succeeded after membership insert failed")
		}

		store = openImportCoverageStore(t)
		groups, _ = store.ListGroups(t.Context())
		if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
			InstrumentID: "US.AAPL", GroupIDs: []string{groups[0].ID}, ExpectedRevision: 0,
		}); err != nil {
			t.Fatalf("seed metadata instrument: %v", err)
		}
		installAbortTrigger(t, store, "watchlist_instruments", "UPDATE", "fail_metadata")
		if err := store.UpdateInstrumentMetadata(t.Context(), []domain.InstrumentMetadata{{InstrumentID: "US.AAPL", Name: "Apple"}}); err == nil {
			t.Fatal("UpdateInstrumentMetadata succeeded after update failure")
		}
	})

	t.Run("unsupported schema version", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "newer.db")
		db, err := sqliteconn.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.ExecContext(t.Context(), `CREATE TABLE watchlist_schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), `INSERT INTO watchlist_schema_migrations (version, applied_at) VALUES (?, '2026-01-01T00:00:00Z')`, SchemaVersion+1); err != nil {
			t.Fatal(err)
		}
		if err := migrate(t.Context(), db); err == nil {
			t.Fatal("migrate accepted a newer schema version")
		}
	})

	t.Run("default group persistence failure", func(t *testing.T) {
		if err := ensureDefaultGroup(t.Context(), failingGroupExecutor{err: errors.New("write failed")}); err == nil {
			t.Fatal("ensureDefaultGroup succeeded with a failing executor")
		}
	})
}

func TestMembershipDiffStorageFailuresRollback(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, store *Store, group domain.Group)
	}{
		{
			name: "membership deletion",
			run: func(t *testing.T, store *Store, group domain.Group) {
				seedMembershipForFault(t, store, group.ID)
				installAbortTrigger(t, store, "watchlist_memberships", "DELETE", "fail_diff_delete")
				if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
					InstrumentID: "US.AAPL", ExpectedRevision: 1,
				}); err == nil {
					t.Fatal("ReplaceMemberships succeeded after membership deletion failed")
				}
			},
		},
		{
			name: "origin deletion",
			run: func(t *testing.T, store *Store, group domain.Group) {
				seedMembershipForFault(t, store, group.ID)
				mustStoreExec(t, store, `INSERT INTO watchlist_membership_origins
					(group_id, instrument_id, source_id, remote_group_id, last_imported_at)
					VALUES (?, 'US.AAPL', 'source', 'remote', '2026-01-01T00:00:00Z')`, group.ID)
				installAbortTrigger(t, store, "watchlist_membership_origins", "DELETE", "fail_diff_origin_delete")
				if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
					InstrumentID: "US.AAPL", ExpectedRevision: 1,
				}); err == nil {
					t.Fatal("ReplaceMemberships succeeded after origin deletion failed")
				}
			},
		},
		{
			name: "instrument revision update",
			run: func(t *testing.T, store *Store, group domain.Group) {
				installAbortTrigger(t, store, "watchlist_instruments", "UPDATE", "fail_diff_revision")
				if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
					InstrumentID: "US.AAPL", GroupIDs: []string{group.ID}, ExpectedRevision: 0,
				}); err == nil {
					t.Fatal("ReplaceMemberships succeeded after revision update failed")
				}
			},
		},
		{
			name: "group revision update",
			run: func(t *testing.T, store *Store, group domain.Group) {
				installAbortTrigger(t, store, "watchlist_groups", "UPDATE", "fail_group_revision")
				if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
					InstrumentID: "US.AAPL", GroupIDs: []string{group.ID}, ExpectedRevision: 0,
				}); err == nil {
					t.Fatal("ReplaceMemberships succeeded after group revision update failed")
				}
			},
		},
		{
			name: "membership lookup",
			run: func(t *testing.T, store *Store, group domain.Group) {
				mustStoreExec(t, store, `DROP TABLE watchlist_memberships`)
				if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
					InstrumentID: "US.AAPL", GroupIDs: []string{group.ID}, ExpectedRevision: 0,
				}); err == nil {
					t.Fatal("ReplaceMemberships succeeded without membership storage")
				}
			},
		},
		{
			name: "instrument lookup",
			run: func(t *testing.T, store *Store, group domain.Group) {
				mustStoreExec(t, store, `DROP TABLE watchlist_instruments`)
				if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
					InstrumentID: "US.AAPL", GroupIDs: []string{group.ID}, ExpectedRevision: 0,
				}); err == nil {
					t.Fatal("ReplaceMemberships succeeded without instrument storage")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openImportCoverageStore(t)
			groups, err := store.ListGroups(t.Context())
			if err != nil || len(groups) != 1 {
				t.Fatalf("ListGroups = %#v, %v", groups, err)
			}
			test.run(t, store, groups[0])
		})
	}
}

func seedMembershipForFault(t *testing.T, store *Store, groupID string) {
	t.Helper()
	if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
		InstrumentID: "US.AAPL", GroupIDs: []string{groupID}, ExpectedRevision: 0,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
}

type failingGroupExecutor struct{ err error }

func (executor failingGroupExecutor) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, executor.err
}
