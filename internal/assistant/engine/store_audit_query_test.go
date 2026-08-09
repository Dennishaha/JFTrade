package adk

import (
	"testing"
)

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
	if _, err := store.DB().ExecContext(
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
