package watchlist

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

func TestCommitImportPersistsNewAndExistingBindings(t *testing.T) {
	store := openImportCoverageStore(t)
	ctx := t.Context()
	if err := store.ReplaceRemoteGroups(ctx, "broker:main", []domain.RemoteGroup{{
		RemoteGroupID: "favorites", Name: "Favorites", Type: "custom",
	}}); err != nil {
		t.Fatalf("ReplaceRemoteGroups: %v", err)
	}

	first := importCoveragePreview("preview-new", "", 0, "hash-one")
	if err := store.SaveImportPreview(ctx, first); err != nil {
		t.Fatalf("SaveImportPreview(new): %v", err)
	}
	firstRun, err := store.CommitImport(ctx, domain.CommitImportStoreInput{
		Preview: first,
		RemoteMembers: []domain.RemoteMember{{
			InstrumentID: "US.AAPL", Name: "Apple", Type: "equity", BrokerCode: "AAPL", SecurityID: "security-aapl",
		}},
	})
	if err != nil {
		t.Fatalf("CommitImport(new group): %v", err)
	}
	if firstRun.AddedCount != 1 || firstRun.UnchangedCount != 0 || firstRun.LocalGroupID == "" {
		t.Fatalf("new-group run = %#v", firstRun)
	}
	if _, err := store.GetImportPreview(ctx, first.ID); !errors.Is(err, domain.ErrStalePreview) {
		t.Fatalf("committed preview = %v, want stale", err)
	}
	var aliasCount int
	if err := store.DB().GetContext(ctx, &aliasCount, `SELECT COUNT(*) FROM watchlist_instrument_aliases WHERE source_id = 'broker:main'`); err != nil || aliasCount != 2 {
		t.Fatalf("stored aliases = %d, %v", aliasCount, err)
	}

	group, err := store.GetGroup(ctx, firstRun.LocalGroupID)
	if err != nil {
		t.Fatalf("GetGroup(new import group): %v", err)
	}
	mismatch := importCoveragePreview("preview-mismatch", group.ID, group.Revision, "hash-two")
	if err := store.SaveImportPreview(ctx, mismatch); err != nil {
		t.Fatalf("SaveImportPreview(mismatch): %v", err)
	}
	mismatch.RemoteHash = "different-hash"
	if _, err := store.CommitImport(ctx, domain.CommitImportStoreInput{Preview: mismatch}); !errors.Is(err, domain.ErrStalePreview) {
		t.Fatalf("CommitImport(remote hash mismatch) = %v", err)
	}

	second := importCoveragePreview("preview-existing", group.ID, group.Revision, "hash-two")
	if err := store.SaveImportPreview(ctx, second); err != nil {
		t.Fatalf("SaveImportPreview(existing): %v", err)
	}
	secondRun, err := store.CommitImport(ctx, domain.CommitImportStoreInput{
		Preview: second,
		RemoteMembers: []domain.RemoteMember{
			{InstrumentID: "US.AAPL", Name: "Apple"},
			{InstrumentID: "US.MSFT", Name: "Microsoft", BrokerCode: "MSFT"},
		},
	})
	if err != nil {
		t.Fatalf("CommitImport(existing group): %v", err)
	}
	if secondRun.AddedCount != 1 || secondRun.UnchangedCount != 1 {
		t.Fatalf("existing-group run = %#v", secondRun)
	}
	bindings, err := store.ListBindings(ctx, "broker:main")
	if err != nil || len(bindings) != 1 || bindings[0].LocalGroupID != group.ID {
		t.Fatalf("upserted binding = %#v, %v", bindings, err)
	}

	group, err = store.GetGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetGroup(after import): %v", err)
	}
	blocked := importCoveragePreview("preview-remote-delete", group.ID, group.Revision, "hash-three")
	if err := store.SaveImportPreview(ctx, blocked); err != nil {
		t.Fatalf("SaveImportPreview(remote delete): %v", err)
	}
	if _, err := store.CommitImport(ctx, domain.CommitImportStoreInput{
		Preview:             blocked,
		RemoteMembers:       []domain.RemoteMember{{InstrumentID: "US.AAPL"}, {InstrumentID: "US.MSFT"}},
		DeleteInstrumentIDs: []string{"US.AAPL"},
	}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("CommitImport(remote member deletion) = %v", err)
	}
	if _, err := store.GetImportPreview(ctx, blocked.ID); err != nil {
		t.Fatalf("failed import should leave preview pending: %v", err)
	}

	if _, err := store.ReplaceMemberships(ctx, domain.ReplaceMembershipsInput{
		InstrumentID: "US.TSLA", GroupIDs: []string{group.ID}, ExpectedRevision: 0,
	}); err != nil {
		t.Fatalf("seed local-only membership: %v", err)
	}
	group, err = store.GetGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetGroup(after local seed): %v", err)
	}
	deleteLocal := importCoveragePreview("preview-local-delete", group.ID, group.Revision, "hash-four")
	if err := store.SaveImportPreview(ctx, deleteLocal); err != nil {
		t.Fatalf("SaveImportPreview(local delete): %v", err)
	}
	removedRun, err := store.CommitImport(ctx, domain.CommitImportStoreInput{
		Preview:             deleteLocal,
		RemoteMembers:       []domain.RemoteMember{{InstrumentID: "US.AAPL"}, {InstrumentID: "US.MSFT"}},
		DeleteInstrumentIDs: []string{"US.TSLA", "US.UNKNOWN"},
	})
	if err != nil {
		t.Fatalf("CommitImport(local-only deletion): %v", err)
	}
	if removedRun.RemovedCount != 1 || removedRun.UnchangedCount != 2 {
		t.Fatalf("local-delete run = %#v", removedRun)
	}
	ids, err := store.GroupInstrumentIDs(ctx, group.ID)
	if err != nil || len(ids) != 2 || ids[0] != "US.AAPL" || ids[1] != "US.MSFT" {
		t.Fatalf("membership after local deletion = %#v, %v", ids, err)
	}
}

func TestImportPreviewStorageRejectsCorruptionAndMissingRecords(t *testing.T) {
	store := openImportCoverageStore(t)
	ctx := t.Context()
	if _, err := store.GetImportPreview(ctx, "missing-preview"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetImportPreview(missing) = %v", err)
	}

	for _, test := range []struct {
		name   string
		column string
		value  string
	}{
		{name: "added", column: "added_json", value: "{"},
		{name: "unchanged", column: "unchanged_json", value: "{"},
		{name: "local only", column: "local_only_json", value: "{"},
	} {
		t.Run(test.name, func(t *testing.T) {
			preview := importCoveragePreview("corrupt-"+test.name, "", 0, "hash")
			if err := store.SaveImportPreview(ctx, preview); err != nil {
				t.Fatalf("SaveImportPreview: %v", err)
			}
			if _, err := store.DB().ExecContext(ctx, `UPDATE watchlist_import_previews SET `+test.column+` = ? WHERE preview_id = ?`, test.value, preview.ID); err != nil {
				t.Fatalf("corrupt %s JSON: %v", test.column, err)
			}
			if _, err := store.GetImportPreview(ctx, preview.ID); err == nil {
				t.Fatalf("GetImportPreview accepted corrupt %s JSON", test.column)
			}
		})
	}

	committed := importCoveragePreview("non-pending-preview", "", 0, "hash")
	if err := store.SaveImportPreview(ctx, committed); err != nil {
		t.Fatalf("SaveImportPreview(non-pending): %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `UPDATE watchlist_import_previews SET status = 'committed' WHERE preview_id = ?`, committed.ID); err != nil {
		t.Fatalf("update preview status: %v", err)
	}
	if _, err := store.GetImportPreview(ctx, committed.ID); !errors.Is(err, domain.ErrStalePreview) {
		t.Fatalf("GetImportPreview(non-pending) = %v", err)
	}
}

func TestImportStorageRollsBackRemoteGroupConflictsAndInvalidOpenPath(t *testing.T) {
	store := openImportCoverageStore(t)
	ctx := t.Context()
	initial := domain.RemoteGroup{RemoteGroupID: "existing", Name: "Existing", Type: "custom"}
	if err := store.ReplaceRemoteGroups(ctx, "source", []domain.RemoteGroup{initial}); err != nil {
		t.Fatalf("seed remote groups: %v", err)
	}
	duplicate := []domain.RemoteGroup{
		{RemoteGroupID: "duplicate", Name: "one", Type: "custom"},
		{RemoteGroupID: "duplicate", Name: "two", Type: "custom"},
	}
	if err := store.ReplaceRemoteGroups(ctx, "source", duplicate); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("ReplaceRemoteGroups(duplicate) = %v", err)
	}
	groups, err := store.ListRemoteGroups(ctx, "source")
	if err != nil || len(groups) != 1 || groups[0].RemoteGroupID != initial.RemoteGroupID {
		t.Fatalf("failed replacement should roll back: %#v, %v", groups, err)
	}
	if err := store.DeleteBinding(ctx, "missing-binding"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("DeleteBinding(missing) = %v", err)
	}

	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
		t.Fatalf("create blocking path: %v", err)
	}
	if _, err := Open(ctx, filepath.Join(blocked, "watchlist.db")); err == nil {
		t.Fatal("Open accepted a database path below a regular file")
	}
}

func openImportCoverageStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "watchlist.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func importCoveragePreview(id, localGroupID string, localRevision int64, remoteHash string) domain.ImportPreview {
	now := time.Now().UTC()
	return domain.ImportPreview{
		ID: id, SourceID: "broker:main", RemoteGroupID: "favorites", RemoteGroupName: "Favorites",
		LocalGroupID: localGroupID, NewGroupName: "Imported favorites", LocalGroupRevision: localRevision,
		RemoteHash: remoteHash, Added: []domain.ImportDiffItem{}, Unchanged: []domain.ImportDiffItem{}, LocalOnly: []domain.ImportDiffItem{},
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
}
