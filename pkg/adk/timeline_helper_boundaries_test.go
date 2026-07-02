package adk

import "testing"

func TestTimelineHelperBoundaries(t *testing.T) {
	if got := stripTimelinePrefix(" run-1: objective", "run-1:"); got != "objective" {
		t.Fatalf("stripTimelinePrefix = %q, want objective", got)
	}
	if got := stripTimelinePrefix("run-1:", "run-1:"); got != "" {
		t.Fatalf("stripTimelinePrefix exact = %q, want empty", got)
	}
	if got := stripTimelinePrefix(" message ", " "); got != "message" {
		t.Fatalf("stripTimelinePrefix blank prefix = %q, want trimmed value", got)
	}

	if !compareTimelineKeys("2026-06-20T00:00:00Z", 2, "b", "2026-06-20T00:00:01Z", 1, "a") {
		t.Fatal("earlier valid timestamp should sort first")
	}
	if compareTimelineKeys("bad-time", 1, "a", "2026-06-20T00:00:01Z", 1, "a") {
		t.Fatal("valid right timestamp should sort before invalid left timestamp")
	}
	if !compareTimelineKeys("same", 1, "a", "same", 2, "a") {
		t.Fatal("lower timeline order should sort first when timestamps tie")
	}
	if !compareTimelineKeys("same", 1, "a", "same", 1, "b") {
		t.Fatal("lower timeline id should sort first when timestamp/order tie")
	}
	if got := firstNonEmpty(" ", "\t", " value "); got != "value" {
		t.Fatalf("firstNonEmpty = %q, want value", got)
	}

	run := Run{
		ToolCalls: []ToolCall{
			{ID: "late", UpdatedAt: "2026-06-20T00:00:03Z"},
			{ID: "early", CreatedAt: "2026-06-20T00:00:01Z"},
			{ID: "empty"},
		},
		PendingApprovals: []Approval{
			{ID: "done", Status: ApprovalStatusApproved, CreatedAt: "2026-06-20T00:00:00Z"},
			{ID: "pending-late", Status: ApprovalStatusPending, UpdatedAt: "2026-06-20T00:00:04Z"},
			{ID: "pending-early", Status: ApprovalStatusPending, CreatedAt: "2026-06-20T00:00:02Z"},
		},
	}
	if got := firstRunToolTime(run); got != "2026-06-20T00:00:01Z" {
		t.Fatalf("firstRunToolTime = %q", got)
	}
	if got := firstRunApprovalTime(run); got != "2026-06-20T00:00:02Z" {
		t.Fatalf("firstRunApprovalTime = %q", got)
	}
}
