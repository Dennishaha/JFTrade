package persistence

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestApprovalByConfirmationCallIDQueryUsesPartialIndex(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStoreCore(
		filepath.Join(dir, "adk.db"),
		filepath.Join(dir, "secrets", "adk.json"),
		filepath.Join(dir, "skills"),
	)
	if err != nil {
		t.Fatalf("NewStoreCore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rows, err := store.DB().QueryxContext(
		t.Context(),
		`EXPLAIN QUERY PLAN `+ApprovalByConfirmationCallIDQuery,
		"confirmation-plan",
	)
	if err != nil {
		t.Fatalf("explain approval lookup: %v", err)
	}
	defer func() { _ = rows.Close() }()

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
