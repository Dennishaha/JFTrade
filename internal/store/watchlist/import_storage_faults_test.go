package watchlist

import (
	"fmt"
	"testing"

	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

func TestCommitImportRollsBackEachPersistenceStage(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, store *Store)
	}{
		{
			name: "instrument upsert",
			setup: func(t *testing.T, store *Store) {
				installAbortTrigger(t, store, "watchlist_instruments", "INSERT", "fail_instrument")
			},
		},
		{
			name: "alias upsert",
			setup: func(t *testing.T, store *Store) {
				installAbortTrigger(t, store, "watchlist_instrument_aliases", "INSERT", "fail_alias")
			},
		},
		{
			name: "membership insert",
			setup: func(t *testing.T, store *Store) {
				installAbortTrigger(t, store, "watchlist_memberships", "INSERT", "fail_membership")
			},
		},
		{
			name: "remote provenance deletion",
			setup: func(t *testing.T, store *Store) {
				mustStoreExec(t, store, `INSERT INTO watchlist_remote_memberships
					(source_id, remote_group_id, instrument_id, remote_hash, observed_at)
					VALUES ('broker:main', 'favorites', 'US.OLD', 'old', '2026-01-01T00:00:00Z')`)
				installAbortTrigger(t, store, "watchlist_remote_memberships", "DELETE", "fail_remote_delete")
			},
		},
		{
			name: "membership origin deletion",
			setup: func(t *testing.T, store *Store) {
				mustStoreExec(t, store, `INSERT INTO watchlist_membership_origins
					(group_id, instrument_id, source_id, remote_group_id, last_imported_at)
					VALUES ('fault-group', 'US.OLD', 'broker:main', 'favorites', '2026-01-01T00:00:00Z')`)
				installAbortTrigger(t, store, "watchlist_membership_origins", "DELETE", "fail_origin_delete")
			},
		},
		{
			name: "remote provenance insert",
			setup: func(t *testing.T, store *Store) {
				installAbortTrigger(t, store, "watchlist_remote_memberships", "INSERT", "fail_remote_insert")
			},
		},
		{
			name: "membership origin insert",
			setup: func(t *testing.T, store *Store) {
				installAbortTrigger(t, store, "watchlist_membership_origins", "INSERT", "fail_origin_insert")
			},
		},
		{
			name: "binding lookup",
			setup: func(t *testing.T, store *Store) {
				mustStoreExec(t, store, `DROP TABLE watchlist_bindings`)
			},
		},
		{
			name: "import run record",
			setup: func(t *testing.T, store *Store) {
				installAbortTrigger(t, store, "watchlist_import_runs", "INSERT", "fail_run")
			},
		},
		{
			name: "preview completion",
			setup: func(t *testing.T, store *Store) {
				installAbortTrigger(t, store, "watchlist_import_previews", "UPDATE", "fail_preview")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, input, groupID := importCommitFailureFixture(t)
			test.setup(t, store)
			if _, err := store.CommitImport(t.Context(), input); err == nil {
				t.Fatal("CommitImport unexpectedly succeeded despite a storage failure")
			}
			if _, err := store.GetImportPreview(t.Context(), input.Preview.ID); err != nil {
				t.Fatalf("failed commit should leave its preview pending: %v", err)
			}
			ids, err := store.GroupInstrumentIDs(t.Context(), groupID)
			if err != nil || len(ids) != 0 {
				t.Fatalf("failed commit should roll back memberships: %#v, %v", ids, err)
			}
		})
	}
}

func importCommitFailureFixture(t *testing.T) (*Store, domain.CommitImportStoreInput, string) {
	t.Helper()
	store := openImportCoverageStore(t)
	group, err := store.CreateGroup(t.Context(), "fault-group")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	preview := importCoveragePreview("fault-preview", group.ID, group.Revision, "fault-hash")
	if err := store.SaveImportPreview(t.Context(), preview); err != nil {
		t.Fatalf("SaveImportPreview: %v", err)
	}
	return store, domain.CommitImportStoreInput{
		Preview: preview,
		RemoteMembers: []domain.RemoteMember{{
			InstrumentID: "US.AAPL", Name: "Apple", BrokerCode: "AAPL", SecurityID: "security-aapl",
		}},
	}, group.ID
}

func installAbortTrigger(t *testing.T, store *Store, table, operation, name string) {
	t.Helper()
	mustStoreExec(t, store, fmt.Sprintf(`CREATE TRIGGER %s BEFORE %s ON %s BEGIN
		SELECT RAISE(ABORT, '%s');
	END`, name, operation, table, name))
}

func mustStoreExec(t *testing.T, store *Store, statement string, args ...any) {
	t.Helper()
	if _, err := store.DB().ExecContext(t.Context(), statement, args...); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}
