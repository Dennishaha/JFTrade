package watchlist

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

func TestNilStoreOperationsReturnUnavailable(t *testing.T) {
	var store *Store
	ctx := t.Context()
	checks := []func() error{
		func() error { _, err := store.ListGroups(ctx); return err },
		func() error { _, err := store.GetGroup(ctx, "group"); return err },
		func() error { _, err := store.CreateGroup(ctx, "group"); return err },
		func() error { _, err := store.UpdateGroup(ctx, "group", "name", 1); return err },
		func() error { return store.DeleteGroup(ctx, "group") },
		func() error { _, err := store.ListItems(ctx, domain.ListItemsOptions{}); return err },
		func() error { _, err := store.GetMemberships(ctx, "US.AAPL"); return err },
		func() error { _, err := store.ReplaceMemberships(ctx, domain.ReplaceMembershipsInput{}); return err },
		func() error { _, err := store.GroupInstrumentIDs(ctx, "group"); return err },
		func() error { return store.UpdateInstrumentMetadata(ctx, nil) },
		func() error { return store.UpsertSource(ctx, domain.Source{}) },
		func() error { _, err := store.ListSources(ctx); return err },
		func() error { return store.ReplaceRemoteGroups(ctx, "source", nil) },
		func() error { _, err := store.ListRemoteGroups(ctx, "source"); return err },
		func() error { _, err := store.ListBindings(ctx, "source"); return err },
		func() error { return store.DeleteBinding(ctx, "binding") },
		func() error { return store.SaveImportPreview(ctx, domain.ImportPreview{}) },
		func() error { _, err := store.GetImportPreview(ctx, "preview"); return err },
		func() error { _, err := store.CommitImport(ctx, domain.CommitImportStoreInput{}); return err },
		func() error { _, err := store.ListImportRuns(ctx, "source", "", 10); return err },
	}
	for index, check := range checks {
		if err := check(); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("operation %d error = %v", index, err)
		}
	}
}

func TestClosedStoreOperationsSurfaceDatabaseErrors(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "watchlist.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ctx := t.Context()
	checks := []func() error{
		func() error { _, err := store.ListGroups(ctx); return err },
		func() error { _, err := store.GetGroup(ctx, "group"); return err },
		func() error { _, err := store.CreateGroup(ctx, "group"); return err },
		func() error { _, err := store.UpdateGroup(ctx, "group", "name", 1); return err },
		func() error { return store.DeleteGroup(ctx, "group") },
		func() error { _, err := store.ListItems(ctx, domain.ListItemsOptions{Limit: 10}); return err },
		func() error { _, err := store.GetMemberships(ctx, "US.AAPL"); return err },
		func() error {
			_, err := store.ReplaceMemberships(ctx, domain.ReplaceMembershipsInput{InstrumentID: "US.AAPL"})
			return err
		},
		func() error { _, err := store.GroupInstrumentIDs(ctx, "group"); return err },
		func() error {
			return store.UpdateInstrumentMetadata(ctx, []domain.InstrumentMetadata{{InstrumentID: "US.AAPL", Name: "Apple"}})
		},
		func() error { return store.UpsertSource(ctx, domain.Source{ID: "source"}) },
		func() error { _, err := store.ListSources(ctx); return err },
		func() error {
			return store.ReplaceRemoteGroups(ctx, "source", []domain.RemoteGroup{{RemoteGroupID: "group"}})
		},
		func() error { _, err := store.ListRemoteGroups(ctx, "source"); return err },
		func() error { _, err := store.ListBindings(ctx, "source"); return err },
		func() error { return store.DeleteBinding(ctx, "binding") },
		func() error { return store.SaveImportPreview(ctx, domain.ImportPreview{ID: "preview"}) },
		func() error { _, err := store.GetImportPreview(ctx, "preview"); return err },
		func() error { _, err := store.CommitImport(ctx, domain.CommitImportStoreInput{}); return err },
		func() error { _, err := store.ListImportRuns(ctx, "source", "", 10); return err },
	}
	for index, check := range checks {
		if err := check(); err == nil {
			t.Fatalf("closed-store operation %d succeeded", index)
		}
	}
}

func TestStorePureHelpersCoverBoundaryInputs(t *testing.T) {
	if err := mapWriteError(nil); err != nil {
		t.Fatalf("mapWriteError(nil) = %v", err)
	}
	constraint := errors.New("UNIQUE constraint failed")
	if err := mapWriteError(constraint); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("constraint error = %v", err)
	}
	plain := errors.New("disk offline")
	if err := mapWriteError(plain); !errors.Is(err, plain) {
		t.Fatalf("plain error = %v", err)
	}

	if market, symbol := splitInstrumentID("AAPL"); market != "" || symbol != "AAPL" {
		t.Fatalf("split invalid = %q/%q", market, symbol)
	}
	if market, symbol := splitInstrumentID("US.AAPL"); market != "US" || symbol != "AAPL" {
		t.Fatalf("split valid = %q/%q", market, symbol)
	}
	if !parseTime("invalid").IsZero() {
		t.Fatal("invalid timestamp did not produce zero time")
	}

	encoded, err := jsonText(map[string]int{"value": 1})
	if err != nil || encoded != `{"value":1}` {
		t.Fatalf("jsonText = %q, %v", encoded, err)
	}
	if _, err := jsonText(make(chan int)); err == nil {
		t.Fatal("jsonText accepted a channel")
	}

	var values []string
	if err := scanJSON(" ", &values); err != nil || values == nil || len(values) != 0 {
		t.Fatalf("scanJSON blank = %#v, %v", values, err)
	}
	if err := scanJSON(`["a","b"]`, &values); err != nil || !reflect.DeepEqual(values, []string{"a", "b"}) {
		t.Fatalf("scanJSON valid = %#v, %v", values, err)
	}
	if err := scanJSON("{", &values); err == nil {
		t.Fatal("scanJSON accepted invalid JSON")
	}

	keys := sortedKeys(map[string]struct{}{"z": {}, "a": {}})
	if !reflect.DeepEqual(keys, []string{"a", "z"}) {
		t.Fatalf("sortedKeys = %#v", keys)
	}
	difference := setDifference(map[string]struct{}{"a": {}, "b": {}}, map[string]struct{}{"b": {}})
	if !reflect.DeepEqual(difference, []string{"a"}) {
		t.Fatalf("setDifference = %#v", difference)
	}
	if items, err := (*Store)(nil).hydrateItems(t.Context(), nil); err != nil || items == nil || len(items) != 0 {
		t.Fatalf("hydrateItems empty = %#v, %v", items, err)
	}
	if err := insertAlias(t.Context(), nil, "source", "code", " ", "US.AAPL"); err != nil {
		t.Fatalf("insertAlias blank = %v", err)
	}
}

func TestListItemsCoversMarketAndCursorFilters(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "watchlist.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	groups, err := store.ListGroups(t.Context())
	if err != nil || len(groups) == 0 {
		t.Fatalf("ListGroups = %#v, %v", groups, err)
	}
	if _, err := store.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
		InstrumentID: "US.AAPL", GroupIDs: []string{groups[0].ID}, ExpectedRevision: 0,
	}); err != nil {
		t.Fatalf("ReplaceMemberships: %v", err)
	}
	if err := store.UpdateInstrumentMetadata(t.Context(), []domain.InstrumentMetadata{
		{}, {InstrumentID: "US.AAPL"},
	}); err != nil {
		t.Fatalf("UpdateInstrumentMetadata blank entries: %v", err)
	}
	for _, options := range []domain.ListItemsOptions{
		{Limit: 10, Market: "CN"},
		{Limit: 10, Market: "US"},
		{Limit: 10, Cursor: "US.AAAA"},
	} {
		if _, err := store.ListItems(t.Context(), options); err != nil {
			t.Fatalf("ListItems(%#v): %v", options, err)
		}
	}
}
