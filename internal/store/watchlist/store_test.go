package watchlist_test

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	store "github.com/jftrade/jftrade-main/internal/store/watchlist"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

func TestStoreDefaultGroupMembershipsAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchlists.db")
	repository, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	service := domain.NewService(repository)

	groups, err := service.ListGroups(t.Context())
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != domain.DefaultGroupName || !groups[0].IsDefault || !groups[0].Protected {
		t.Fatalf("default groups = %#v", groups)
	}
	defaultID := groups[0].ID
	if err := service.DeleteGroup(t.Context(), defaultID); !errors.Is(err, domain.ErrProtectedGroup) {
		t.Fatalf("DeleteGroup(default) = %v", err)
	}
	if _, err := service.UpdateGroup(t.Context(), defaultID, domain.UpdateGroupInput{Name: "重命名", ExpectedRevision: groups[0].Revision}); !errors.Is(err, domain.ErrProtectedGroup) {
		t.Fatalf("UpdateGroup(default) = %v", err)
	}

	created, err := service.CreateGroup(t.Context(), domain.CreateGroupInput{Name: "  科技  "})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if created.Name != "科技" {
		t.Fatalf("created name = %q", created.Name)
	}
	if _, err := service.CreateGroup(t.Context(), domain.CreateGroupInput{Name: "科技"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate CreateGroup = %v", err)
	}

	memberships, err := service.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
		InstrumentID: "us:aapl", GroupIDs: []string{defaultID, created.ID}, NewGroupNames: []string{"长期"}, ExpectedRevision: 0,
	})
	if err != nil {
		t.Fatalf("ReplaceMemberships(first): %v", err)
	}
	if memberships.InstrumentID != "US.AAPL" || memberships.Revision != 1 || len(memberships.Groups) != 3 {
		t.Fatalf("first memberships = %#v", memberships)
	}
	idempotent, err := service.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
		InstrumentID: "US.AAPL", GroupIDs: groupIDs(memberships.Groups), ExpectedRevision: memberships.Revision,
	})
	if err != nil {
		t.Fatalf("ReplaceMemberships(idempotent): %v", err)
	}
	if idempotent.Revision != memberships.Revision {
		t.Fatalf("idempotent revision = %d", idempotent.Revision)
	}
	if _, err := service.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{InstrumentID: "US.AAPL", GroupIDs: []string{defaultID}, ExpectedRevision: 0}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale ReplaceMemberships = %v", err)
	}

	cleared, err := service.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{InstrumentID: "US.AAPL", GroupIDs: []string{}, ExpectedRevision: memberships.Revision})
	if err != nil {
		t.Fatalf("ReplaceMemberships(clear): %v", err)
	}
	if cleared.Revision != 2 || len(cleared.Groups) != 0 {
		t.Fatalf("cleared = %#v", cleared)
	}
	page, err := service.ListItems(t.Context(), domain.ListItemsOptions{Limit: 10})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("ListItems after clear = %#v, %v", page, err)
	}

	var schemaVersion int
	if err := repository.DB().GetContext(t.Context(), &schemaVersion, `SELECT version FROM jftrade_schema_meta WHERE component_id = 'watchlist'`); err != nil || schemaVersion != store.SchemaVersion {
		t.Fatalf("schema metadata = %d, %v", schemaVersion, err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restored, err := domain.NewService(reopened).GetMemberships(t.Context(), "US.AAPL")
	if err != nil || restored.Revision != 2 || len(restored.Groups) != 0 {
		t.Fatalf("restored memberships = %#v, %v", restored, err)
	}
}

func TestReplaceMembershipsRollsBackNewGroupsAndInstrumentOnConflict(t *testing.T) {
	repository := openStore(t)
	service := domain.NewService(repository)
	groups, err := service.ListGroups(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateGroup(t.Context(), domain.CreateGroupInput{Name: "已有"}); err != nil {
		t.Fatal(err)
	}
	_, err = service.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
		InstrumentID: "US.TSLA", GroupIDs: []string{groups[0].ID}, NewGroupNames: []string{"已有"}, ExpectedRevision: 0,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("ReplaceMemberships conflict = %v", err)
	}
	memberships, err := service.GetMemberships(t.Context(), "US.TSLA")
	if err != nil || memberships.Revision != 0 || len(memberships.Groups) != 0 {
		t.Fatalf("rolled back memberships = %#v, %v", memberships, err)
	}
}

func TestSnapshotMetadataEnrichesExistingInstrumentWithoutChangingMembershipRevision(t *testing.T) {
	repository := openStore(t)
	service := domain.NewService(repository)
	groups, err := service.ListGroups(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	membership, err := service.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
		InstrumentID: "US.AAPL", GroupIDs: []string{groups[0].ID}, ExpectedRevision: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateInstrumentMetadata(t.Context(), []domain.InstrumentMetadata{{
		InstrumentID: "US.AAPL", Name: "Apple Inc.", Type: "SecurityType_Eqty",
	}}); err != nil {
		t.Fatal(err)
	}
	page, err := service.ListItems(t.Context(), domain.ListItemsOptions{Query: "Apple", Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].Name != "Apple Inc." || page.Items[0].Type != "SecurityType_Eqty" {
		t.Fatalf("enriched page = %#v, %v", page, err)
	}
	after, err := service.GetMemberships(t.Context(), "US.AAPL")
	if err != nil || after.Revision != membership.Revision {
		t.Fatalf("membership revision changed after metadata enrichment: %#v, %v", after, err)
	}
}

func TestListItemsBatchHydratesGroupsAndSources(t *testing.T) {
	repository := openStore(t)
	service := domain.NewService(repository)
	groups, err := service.ListGroups(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	technology, err := service.CreateGroup(t.Context(), domain.CreateGroupInput{Name: "科技"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []domain.ReplaceMembershipsInput{
		{InstrumentID: "US.AAPL", GroupIDs: []string{groups[0].ID, technology.ID}, ExpectedRevision: 0},
		{InstrumentID: "US.MSFT", GroupIDs: []string{technology.ID}, ExpectedRevision: 0},
	} {
		if _, err := service.ReplaceMemberships(t.Context(), input); err != nil {
			t.Fatal(err)
		}
	}
	for _, sourceID := range []string{"futu:default", "broker-b:default"} {
		if _, err := repository.DB().ExecContext(t.Context(), `INSERT INTO watchlist_membership_origins
			(group_id, instrument_id, source_id, remote_group_id, last_imported_at)
			VALUES (?, 'US.AAPL', ?, ?, '2026-07-11T00:00:00Z')`,
			technology.ID, sourceID, sourceID+":remote"); err != nil {
			t.Fatal(err)
		}
	}

	page, err := service.ListItems(t.Context(), domain.ListItemsOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items = %#v", page.Items)
	}
	apple, microsoft := page.Items[0], page.Items[1]
	if apple.ID != "US.AAPL" || !slices.Equal(apple.GroupIDs, []string{groups[0].ID, technology.ID}) ||
		!slices.Equal(apple.SourceIDs, []string{"broker-b:default", "futu:default"}) {
		t.Fatalf("Apple item = %#v", apple)
	}
	if microsoft.ID != "US.MSFT" || !slices.Equal(microsoft.GroupIDs, []string{technology.ID}) ||
		len(microsoft.SourceIDs) != 0 {
		t.Fatalf("Microsoft item = %#v", microsoft)
	}
}

func TestStoreGroupUpdateAndDeletePreserveMembershipRevisionContract(t *testing.T) {
	repository := openStore(t)
	service := domain.NewService(repository)
	if repository.Path() == "" || repository.DB() == nil {
		t.Fatalf("opened store path/db = %q/%p", repository.Path(), repository.DB())
	}
	first, err := service.CreateGroup(t.Context(), domain.CreateGroupInput{Name: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateGroup(t.Context(), domain.CreateGroupInput{Name: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdateGroup(t.Context(), first.ID, domain.UpdateGroupInput{Name: "Renamed", ExpectedRevision: first.Revision})
	if err != nil || updated.Name != "Renamed" || updated.Revision != first.Revision+1 {
		t.Fatalf("updated group = %#v, %v", updated, err)
	}
	if _, err := service.UpdateGroup(t.Context(), first.ID, domain.UpdateGroupInput{Name: "Stale", ExpectedRevision: first.Revision}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale update = %v", err)
	}
	if _, err := service.UpdateGroup(t.Context(), first.ID, domain.UpdateGroupInput{Name: second.Name, ExpectedRevision: updated.Revision}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate-name update = %v", err)
	}
	if _, err := service.UpdateGroup(t.Context(), "missing", domain.UpdateGroupInput{Name: "Missing", ExpectedRevision: 1}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing update = %v", err)
	}

	membership, err := service.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
		InstrumentID: "US.AAPL", GroupIDs: []string{first.ID, second.ID}, ExpectedRevision: 0,
	})
	if err != nil || membership.Revision != 1 || len(membership.Groups) != 2 {
		t.Fatalf("seed membership = %#v, %v", membership, err)
	}
	if err := service.DeleteGroup(t.Context(), first.ID); err != nil {
		t.Fatal(err)
	}
	after, err := service.GetMemberships(t.Context(), "US.AAPL")
	if err != nil || after.Revision != 2 || len(after.Groups) != 1 || after.Groups[0].ID != second.ID {
		t.Fatalf("membership after group delete = %#v, %v", after, err)
	}
	if _, err := repository.GetGroup(t.Context(), first.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted group lookup = %v", err)
	}
	if err := service.DeleteGroup(t.Context(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing delete = %v", err)
	}

	var nilStore *store.Store
	if nilStore.Path() != "" || nilStore.DB() != nil || nilStore.Close() != nil {
		t.Fatal("nil store accessors should be safe")
	}
	if _, err := store.Open(t.Context(), " "); err == nil {
		t.Fatal("Open accepted an empty database path")
	}
}

func TestStoreRoundTripsSourceAndRemoteGroupSnapshots(t *testing.T) {
	repository := openStore(t)
	now := time.Date(2026, time.July, 11, 13, 0, 0, 0, time.UTC)
	if err := repository.UpsertSource(t.Context(), domain.Source{
		ID: "futu:default", Broker: "futu", DisplayName: "OpenD", Status: "ready", UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpsertSource(t.Context(), domain.Source{
		ID: "futu:default", Broker: "futu", DisplayName: "OpenD", Status: "unavailable", Error: "offline",
	}); err != nil {
		t.Fatal(err)
	}
	sources, err := repository.ListSources(t.Context())
	if err != nil || len(sources) != 1 || sources[0].Status != "unavailable" || sources[0].Error != "offline" || sources[0].UpdatedAt.IsZero() {
		t.Fatalf("sources = %#v, %v", sources, err)
	}

	groups := []domain.RemoteGroup{
		{SourceID: "futu:default", RemoteGroupID: "two", Name: "Beta", Type: "custom", MemberCount: 2, RemoteHash: "hash-2", ObservedAt: now},
		{SourceID: "futu:default", RemoteGroupID: "one", Name: "Alpha", Type: "custom", Ambiguous: true, MemberCount: 1, RemoteHash: "hash-1"},
	}
	if err := repository.ReplaceRemoteGroups(t.Context(), "futu:default", groups); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.ListRemoteGroups(t.Context(), "futu:default")
	if err != nil || len(stored) != 2 {
		t.Fatalf("remote groups = %#v, %v", stored, err)
	}
	if stored[0].RemoteGroupID != "one" || !stored[0].Ambiguous || stored[0].ObservedAt.IsZero() || stored[1].RemoteGroupID != "two" || !stored[1].ObservedAt.Equal(now) {
		t.Fatalf("round-tripped remote groups = %#v", stored)
	}
	if err := repository.ReplaceRemoteGroups(t.Context(), "futu:default", nil); err != nil {
		t.Fatal(err)
	}
	stored, err = repository.ListRemoteGroups(t.Context(), "futu:default")
	if err != nil || stored == nil || len(stored) != 0 {
		t.Fatalf("cleared remote groups = %#v, %v", stored, err)
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	repository, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "watchlists.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	return repository
}

func groupIDs(groups []domain.GroupRef) []string {
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		result = append(result, group.ID)
	}
	return result
}
