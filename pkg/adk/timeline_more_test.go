package adk

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestTimelineAdditionalHelperCoverageBranches(t *testing.T) {
	t1 := "2026-01-01T00:00:00Z"
	t2 := "2026-01-01T00:00:01Z"
	sessionID := "timeline-more-session"

	t.Run("run message emits final reasoning and content after pre tool prefixes", func(t *testing.T) {
		run := Run{
			ID: "timeline-more-run", CreatedAt: t1, UpdatedAt: t1,
			PreToolContent: "pre content", PreToolReasoning: "pre reasoning",
			ToolCalls:        []ToolCall{{ID: "timeline-more-tool", CreatedAt: t1}},
			PendingApprovals: []Approval{{ID: "timeline-more-approval", Status: ApprovalStatusPending, CreatedAt: t1}},
		}
		message := TranscriptEntry{
			ID: "timeline-more-assistant", RunID: run.ID, Role: "assistant",
			Content: "pre content\nfinal answer", ReasoningContent: "pre reasoning\nfinal reasoning",
			CreatedAt: t2,
		}
		entries := groupTimelinePrimitives(timelinePrimitivesForRunMessage(sessionID, run, message))
		seen := map[string]bool{}
		for _, entry := range entries {
			if entry.ID == "timeline-more-assistant:reasoning" && entry.Text == "final reasoning" {
				seen["reasoning"] = true
			}
			if entry.ID == "timeline-more-assistant" && entry.Text == "final answer" {
				seen["content"] = true
			}
			if entry.Kind == TimelineKindToolGroup && len(entry.ToolCalls) == 1 {
				seen["tool"] = true
			}
			if entry.Kind == TimelineKindApprovalGroup && len(entry.Approvals) == 1 {
				seen["approval"] = true
			}
		}
		for _, key := range []string{"reasoning", "content", "tool", "approval"} {
			if !seen[key] {
				t.Fatalf("timeline entries = %#v, missing %s", entries, key)
			}
		}
	})

	t.Run("prompt matching rejects mismatched objectives and blank internals", func(t *testing.T) {
		if _, ok := matchWorkflowPromptRun(workflowUserPrompt{isInternal: true}, []Run{{ID: "run"}}); ok {
			t.Fatal("blank internal workflow prompt should not match a run")
		}
		prompt := workflowUserPrompt{isInternal: true, userMessage: "build", objective: "target"}
		if _, ok := matchWorkflowPromptRun(prompt, []Run{{ID: "run", UserMessage: "build", Objective: "other"}}); ok {
			t.Fatal("workflow prompt with mismatched objective should not match")
		}
		taskPrompt := classifyWorkflowUserPrompt("请推进这个任务编排。\n总体目标：ship\n用户请求：build task")
		if !taskPrompt.isInternal || taskPrompt.objective != "ship" || taskPrompt.userMessage != "build task" {
			t.Fatalf("task workflow prompt = %+v", taskPrompt)
		}
	})

	t.Run("grouping drops blank text primitives and sorts invalid keys by id", func(t *testing.T) {
		entries := groupTimelinePrimitives([]timelinePrimitive{
			{id: "blank", sessionID: sessionID, kind: TimelineKindAssistantMessage, text: "   ", createdAt: t1},
			{id: "b", sessionID: sessionID, kind: TimelineKindAssistantMessage, text: "second", createdAt: "bad"},
			{id: "a", sessionID: sessionID, kind: TimelineKindAssistantMessage, text: "first", createdAt: "bad"},
		})
		if len(entries) != 2 || entries[0].ID != "a" || entries[0].Sequence != 1 || entries[1].Sequence != 2 {
			t.Fatalf("grouped entries = %#v", entries)
		}
	})

	t.Run("session runs paginates and sorts ascending", func(t *testing.T) {
		ctx := context.Background()
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "timeline-more-agent", "timeline more")
		for i := range 105 {
			id := fmt.Sprintf("timeline-more-run-page-%03d", 104-i)
			mustSaveRun(t, runtime, Run{
				ID: id, SessionID: session.ID, AgentID: session.AgentID,
				Status: RunStatusCompleted, CreatedAt: t1, UpdatedAt: t1,
			})
		}
		runs, err := runtime.Store().sessionRuns(ctx, session.ID)
		if err != nil {
			t.Fatalf("sessionRuns: %v", err)
		}
		if len(runs) != 105 {
			t.Fatalf("sessionRuns len = %d, want 105", len(runs))
		}
		for i := 1; i < len(runs); i++ {
			if runs[i-1].ID > runs[i].ID {
				t.Fatalf("runs not sorted at %d: %q > %q", i, runs[i-1].ID, runs[i].ID)
			}
		}
	})

	t.Run("timeline surfaces notice projection and run loading errors", func(t *testing.T) {
		ctx := context.Background()

		noticeRuntime := newTestRuntime(t)
		noticeSession := mustCreateSession(t, noticeRuntime, "timeline-notice-agent", "timeline notice")
		if _, err := noticeRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableSessionNotices); err != nil {
			t.Fatalf("drop session notices: %v", err)
		}
		if _, ok, err := noticeRuntime.Store().SessionTimeline(ctx, noticeSession.ID); err == nil || ok || !strings.Contains(err.Error(), tableSessionNotices) {
			t.Fatalf("SessionTimeline notices ok=%v err=%v, want %s failure", ok, err, tableSessionNotices)
		}

		projectionRuntime := newTestRuntime(t)
		projectionSession := mustCreateSession(t, projectionRuntime, "timeline-projection-agent", "timeline projection")
		mustCreateADKSessionForAgent(t, projectionRuntime, projectionSession.AgentID, projectionSession.ID)
		if _, err := projectionRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs for projection: %v", err)
		}
		if _, ok, err := projectionRuntime.Store().SessionTimeline(ctx, projectionSession.ID); err == nil || ok || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("SessionTimeline projection ok=%v err=%v, want %s failure", ok, err, tableRuns)
		}

		runRuntime := newTestRuntime(t)
		runRuntime.Store().SetSessionService(nil)
		runSession := mustCreateSession(t, runRuntime, "timeline-runs-agent", "timeline runs")
		if _, err := runRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs for sessionRuns: %v", err)
		}
		if _, ok, err := runRuntime.Store().SessionTimeline(ctx, runSession.ID); err == nil || ok || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("SessionTimeline sessionRuns ok=%v err=%v, want %s failure", ok, err, tableRuns)
		}
	})

	t.Run("timeline returns empty when only blank orphan runs remain and helpers cover fallback branches", func(t *testing.T) {
		ctx := context.Background()
		runtime := newTestRuntime(t)
		runtime.Store().SetSessionService(nil)
		session := mustCreateSession(t, runtime, "timeline-empty-agent", "timeline empty")
		mustSaveRun(t, runtime, Run{
			ID: "timeline-empty-run", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusCompleted, CreatedAt: t1, UpdatedAt: t1,
		})
		timeline, ok, err := runtime.Store().SessionTimeline(ctx, session.ID)
		if err != nil || ok || timeline != nil {
			t.Fatalf("SessionTimeline empty timeline=%+v ok=%v err=%v, want nil false nil", timeline, ok, err)
		}
		if got := stripTimelinePrefix("unchanged text", "prefix"); got != "unchanged text" {
			t.Fatalf("stripTimelinePrefix no prefix = %q, want unchanged text", got)
		}
		if !compareTimelineKeys(t1, 1, "left", "bad-time", 1, "right") {
			t.Fatal("compareTimelineKeys should prefer valid RFC3339 time over invalid time")
		}
		if prompt := classifyWorkflowUserPrompt(" "); prompt.isInternal || prompt.isHidden || prompt.userMessage != "" || prompt.objective != "" {
			t.Fatalf("blank classifyWorkflowUserPrompt = %+v, want zero value prompt", prompt)
		}
		if _, ok := matchWorkflowPromptRun(workflowUserPrompt{isInternal: true, userMessage: "wanted"}, []Run{{ID: "other", UserMessage: "different"}}); ok {
			t.Fatal("matchWorkflowPromptRun matched a run with a different user message")
		}
	})
}
