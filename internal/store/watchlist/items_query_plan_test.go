package watchlist

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

func TestGroupedListItemsPreservesOrderingPaginationAndFilters(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "watchlists.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	groups, err := store.ListGroups(t.Context())
	if err != nil || len(groups) != 1 {
		t.Fatalf("ListGroups = %#v, %v", groups, err)
	}
	custom, err := store.CreateGroup(t.Context(), "Technology")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	for _, input := range []domain.ReplaceMembershipsInput{
		{InstrumentID: "US.AAPL", GroupIDs: []string{groups[0].ID, custom.ID}, ExpectedRevision: 0},
		{InstrumentID: "US.MSFT", GroupIDs: []string{custom.ID}, ExpectedRevision: 0},
		{InstrumentID: "US.TSLA", GroupIDs: []string{groups[0].ID}, ExpectedRevision: 0},
	} {
		if _, err := store.ReplaceMemberships(t.Context(), input); err != nil {
			t.Fatalf("ReplaceMemberships(%s): %v", input.InstrumentID, err)
		}
	}
	if err := store.UpdateInstrumentMetadata(t.Context(), []domain.InstrumentMetadata{
		{InstrumentID: "US.AAPL", Name: "Apple"},
		{InstrumentID: "US.MSFT", Name: "Microsoft"},
		{InstrumentID: "US.TSLA", Name: "Tesla"},
	}); err != nil {
		t.Fatalf("UpdateInstrumentMetadata: %v", err)
	}

	first, err := store.ListItems(t.Context(), domain.ListItemsOptions{GroupID: custom.ID, Market: "US", Limit: 1})
	if err != nil {
		t.Fatalf("ListItems first page: %v", err)
	}
	if len(first.Items) != 1 || first.Items[0].ID != "US.AAPL" || first.NextCursor != "US.AAPL" {
		t.Fatalf("first page = %#v", first)
	}
	if !slices.Contains(first.Items[0].GroupIDs, custom.ID) {
		t.Fatalf("first page groups = %#v", first.Items[0].GroupIDs)
	}

	second, err := store.ListItems(t.Context(), domain.ListItemsOptions{
		GroupID: custom.ID,
		Cursor:  first.NextCursor,
		Query:   "Microsoft",
		Market:  "US",
		Limit:   1,
	})
	if err != nil {
		t.Fatalf("ListItems second page: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "US.MSFT" || second.NextCursor != "" {
		t.Fatalf("second page = %#v", second)
	}
}

func TestGroupedListItemsQueryUsesMembershipPrimaryKeyRange(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "watchlists.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	query, args := buildListItemsQuery(domain.ListItemsOptions{
		GroupID: "group-a",
		Cursor:  "US.AAAA",
		Market:  "US",
		Limit:   50,
	})
	rows, err := store.db.QueryxContext(t.Context(), `EXPLAIN QUERY PLAN `+query, args...)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer func() { _ = rows.Close() }()

	foundMembershipRange := false
	for rows.Next() {
		var selectID, order, from int
		var detail string
		if err := rows.Scan(&selectID, &order, &from, &detail); err != nil {
			t.Fatalf("scan query plan: %v", err)
		}
		if strings.Contains(detail, "USE TEMP B-TREE") || strings.Contains(detail, "SCAN i") {
			t.Fatalf("grouped list query uses avoidable scan/sort: %s", detail)
		}
		if strings.Contains(detail, "SEARCH member USING COVERING INDEX") &&
			strings.Contains(detail, "group_id=? AND instrument_id>?") {
			foundMembershipRange = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("query plan rows: %v", err)
	}
	if !foundMembershipRange {
		t.Fatal("grouped list query did not use the membership primary-key range")
	}
}
