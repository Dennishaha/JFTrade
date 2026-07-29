package trading

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestExecutionEventLoadUsesOrderIndexAndPreservesPerOrderChronology(t *testing.T) {
	persistence, err := newExecutionOrderSQLiteStore(filepath.Join(t.TempDir(), "execution-orders.db"))
	if err != nil {
		t.Fatalf("newExecutionOrderSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = persistence.Close() })

	for _, event := range []trdsrv.ExecutionOrderEvent{
		{ID: "evt-000004", InternalOrderID: "order-b", EventType: "b-late", CreatedAt: "2026-07-29T00:04:00Z"},
		{ID: "evt-000003", InternalOrderID: "order-a", EventType: "a-late", CreatedAt: "2026-07-29T00:03:00Z"},
		{ID: "evt-000001", InternalOrderID: "order-a", EventType: "a-early", CreatedAt: "2026-07-29T00:01:00Z"},
		{ID: "evt-000002", InternalOrderID: "order-b", EventType: "b-early", CreatedAt: "2026-07-29T00:02:00Z"},
	} {
		if err := persistence.persistEvent(event); err != nil {
			t.Fatalf("persistEvent(%s): %v", event.ID, err)
		}
	}

	store := newExecutionOrderStore()
	store.persistence = persistence
	if err := store.loadFromDB(); err != nil {
		t.Fatalf("loadFromDB: %v", err)
	}
	if got := eventIDs(store.Events("order-a").Events); !slices.Equal(got, []string{"evt-000001", "evt-000003"}) {
		t.Fatalf("order-a events = %#v", got)
	}
	if got := eventIDs(store.Events("order-b").Events); !slices.Equal(got, []string{"evt-000002", "evt-000004"}) {
		t.Fatalf("order-b events = %#v", got)
	}
	if store.nextEventSeq != 4 {
		t.Fatalf("nextEventSeq = %d, want 4", store.nextEventSeq)
	}

	assertExecutionEventLoadPlanUsesOrderIndex(t, persistence)
}

func assertExecutionEventLoadPlanUsesOrderIndex(t *testing.T, persistence *sqliteStore) {
	t.Helper()
	rows, err := persistence.db.Queryx(`EXPLAIN QUERY PLAN ` + loadExecutionOrderEventsQuery)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer func() { _ = rows.Close() }()

	foundOrderIndex := false
	for rows.Next() {
		var selectID, order, from int
		var detail string
		if err := rows.Scan(&selectID, &order, &from, &detail); err != nil {
			t.Fatalf("scan query plan: %v", err)
		}
		if strings.Contains(detail, "USE TEMP B-TREE") {
			t.Fatalf("event load query uses a temporary sort: %s", detail)
		}
		if strings.Contains(detail, "idx_execution_order_events_order") {
			foundOrderIndex = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("query plan rows: %v", err)
	}
	if !foundOrderIndex {
		t.Fatal("event load query did not use idx_execution_order_events_order")
	}
}

func eventIDs(events []trdsrv.ExecutionOrderEvent) []string {
	result := make([]string, 0, len(events))
	for _, event := range events {
		result = append(result, event.ID)
	}
	return result
}
