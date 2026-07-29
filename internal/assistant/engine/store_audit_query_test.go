package adk

import (
	"strings"
	"testing"
)

func TestApprovalByConfirmationCallIDQueryUsesPartialIndex(t *testing.T) {
	store := newBusinessStore(t)

	rows, err := store.db.QueryxContext(
		t.Context(),
		`EXPLAIN QUERY PLAN `+approvalByConfirmationCallIDQuery,
		"confirmation-plan",
	)
	if err != nil {
		t.Fatalf("explain approval lookup: %v", err)
	}
	defer func() { jftradeCheckTestError(t, rows.Close()) }()

	details := make([]string, 0, 2)
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan approval query plan: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read approval query plan: %v", err)
	}
	plan := strings.Join(details, "\n")
	t.Logf("approval lookup query plan:\n%s", plan)
	if !strings.Contains(plan, "idx_adk_approvals_confirmation_call") {
		t.Fatalf("approval query plan does not use confirmation-call index:\n%s", plan)
	}
	if strings.Contains(plan, "SCAN "+tableApprovals) {
		t.Fatalf("approval query plan performs a full table scan:\n%s", plan)
	}
}

func TestStoreListAuditEventsPageFiltersCountsAndOrdersInSQL(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	for _, event := range []AuditEvent{
		{ID: "audit-b", Kind: "agent.saved", SubjectID: "wanted", Detail: "second at same time", CreatedAt: "2026-01-02T03:04:07Z"},
		{ID: "audit-a", Kind: "agent.saved", SubjectID: "wanted", Detail: "first at same time", CreatedAt: "2026-01-02T03:04:07Z"},
		{ID: "audit-old", Kind: "agent.saved", SubjectID: "wanted", Detail: "older", CreatedAt: "2026-01-02T03:04:05Z"},
		{ID: "audit-other-subject", Kind: "agent.saved", SubjectID: "other", Detail: "excluded", CreatedAt: "2026-01-02T03:04:08Z"},
	} {
		if err := store.AddAuditEvent(ctx, event); err != nil {
			t.Fatalf("AddAuditEvent %s: %v", event.ID, err)
		}
	}
	if _, err := store.db.ExecContext(
		ctx,
		`INSERT INTO `+tableAudit+` (id, kind, subject_id, payload_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		"audit-corrupt-other-kind", "provider.saved", "wanted", "{not-json", "2026-01-02T03:04:09Z",
	); err != nil {
		t.Fatalf("insert unrelated corrupt audit payload: %v", err)
	}

	events, total, err := store.ListAuditEventsPage(ctx, "agent.saved", "wanted", 2, 0)
	if err != nil {
		t.Fatalf("ListAuditEventsPage first page: %v", err)
	}
	if total != 3 || len(events) != 2 || events[0].ID != "audit-a" || events[1].ID != "audit-b" {
		t.Fatalf("first page total=%d events=%#v, want audit-a then audit-b of 3", total, events)
	}

	events, total, err = store.ListAuditEventsPage(ctx, "agent.saved", "wanted", 1, 2)
	if err != nil {
		t.Fatalf("ListAuditEventsPage last page: %v", err)
	}
	if total != 3 || len(events) != 1 || events[0].ID != "audit-old" {
		t.Fatalf("last page total=%d events=%#v, want audit-old", total, events)
	}

	events, total, err = store.ListAuditEventsPage(ctx, "agent.saved", "wanted", 1, 100)
	if err != nil {
		t.Fatalf("ListAuditEventsPage beyond total: %v", err)
	}
	if total != 3 || len(events) != 0 {
		t.Fatalf("page beyond total total=%d events=%#v, want empty page of 3", total, events)
	}
}
