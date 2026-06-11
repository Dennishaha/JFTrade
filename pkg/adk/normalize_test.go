package adk

import "testing"

func TestNormalizeRunAndResponsesReplaceNilSlices(t *testing.T) {
	run := NormalizeRun(Run{
		ID:               "run-normalize",
		ToolCalls:        nil,
		PendingApprovals: nil,
		ToolSummaries:    nil,
	})
	if run.ToolCalls == nil || len(run.ToolCalls) != 0 {
		t.Fatalf("toolCalls = %#v, want non-nil empty slice", run.ToolCalls)
	}
	if run.PendingApprovals == nil || len(run.PendingApprovals) != 0 {
		t.Fatalf("pendingApprovals = %#v, want non-nil empty slice", run.PendingApprovals)
	}
	if run.ToolSummaries == nil || len(run.ToolSummaries) != 0 {
		t.Fatalf("toolSummaries = %#v, want non-nil empty slice", run.ToolSummaries)
	}

	entry := NormalizeTimelineEntry(TimelineEntry{
		ID:        "entry-normalize",
		ToolCalls: nil,
		Approvals: nil,
	})
	if entry.ToolCalls == nil || len(entry.ToolCalls) != 0 {
		t.Fatalf("timeline toolCalls = %#v, want non-nil empty slice", entry.ToolCalls)
	}
	if entry.Approvals == nil || len(entry.Approvals) != 0 {
		t.Fatalf("timeline approvals = %#v, want non-nil empty slice", entry.Approvals)
	}

	response := NormalizeChatResponse(ChatResponse{
		Run:              Run{ID: "run-chat"},
		PendingApprovals: nil,
		Timeline:         nil,
	})
	if response.Run.ToolCalls == nil || response.Run.PendingApprovals == nil {
		t.Fatalf("normalized run = %+v, want non-nil slices", response.Run)
	}
	if response.PendingApprovals == nil || len(response.PendingApprovals) != 0 {
		t.Fatalf("response pendingApprovals = %#v, want non-nil empty slice", response.PendingApprovals)
	}
	if response.Timeline == nil || len(response.Timeline) != 0 {
		t.Fatalf("response timeline = %#v, want non-nil empty slice", response.Timeline)
	}

	resolution := NormalizeApprovalResolution(ApprovalResolution{
		Run: &Run{ID: "run-resolution"},
	})
	if resolution.Run == nil || resolution.Run.ToolCalls == nil || resolution.Run.PendingApprovals == nil {
		t.Fatalf("resolution run = %+v, want normalized run slices", resolution.Run)
	}

	sessionResponse := NormalizeSessionsResponse(SessionsResponse{})
	if sessionResponse.Timeline == nil || len(sessionResponse.Timeline) != 0 {
		t.Fatalf("session timeline = %#v, want non-nil empty slice", sessionResponse.Timeline)
	}
}
