package assistant

import (
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
)

func TestServiceGetAuditPagePreservesFilteredPagination(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()

	for _, event := range []jfadk.AuditEvent{
		{ID: "audit-new", Kind: "agent.saved", SubjectID: "wanted", CreatedAt: "2026-01-02T03:04:07Z"},
		{ID: "audit-old", Kind: "agent.saved", SubjectID: "wanted", CreatedAt: "2026-01-02T03:04:05Z"},
		{ID: "audit-other", Kind: "agent.saved", SubjectID: "other", CreatedAt: "2026-01-02T03:04:08Z"},
	} {
		if err := runtime.Store().AddAuditEvent(ctx, event); err != nil {
			t.Fatalf("AddAuditEvent %s: %v", event.ID, err)
		}
	}

	page, err := service.GetAuditPage(ctx, AuditQuery{
		Kind: "agent.saved", SubjectID: "wanted", Limit: 1, Offset: 1,
	})
	if err != nil {
		t.Fatalf("GetAuditPage: %v", err)
	}
	if page.Total != 2 || page.Limit != 1 || page.Offset != 1 ||
		len(page.Items) != 1 || page.Items[0].ID != "audit-old" {
		t.Fatalf("GetAuditPage = %#v, want second filtered event of 2", page)
	}

	page, err = service.GetAuditPage(ctx, AuditQuery{
		Kind: "agent.saved", SubjectID: "wanted", Limit: 1, Offset: 100,
	})
	if err != nil {
		t.Fatalf("GetAuditPage beyond total: %v", err)
	}
	if page.Total != 2 || page.Offset != 2 || len(page.Items) != 0 {
		t.Fatalf("GetAuditPage beyond total = %#v, want empty page clamped to 2", page)
	}
}
