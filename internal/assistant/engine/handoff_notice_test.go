package adk

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestHandoffSegmentsReplaceActiveChainAndFilterByRevision(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "agent-handoff", "handoff lifecycle")

	if _, err := runtime.Store().SaveHandoffSegment(ctx, HandoffSegment{Summary: "missing session"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SaveHandoffSegment missing session err = %v, want os.ErrNotExist", err)
	}
	if segments, err := runtime.Store().HandoffSegmentsForRevision(ctx, session.ID, "   ", true); err != nil || len(segments) != 0 {
		t.Fatalf("blank revision segments=%+v err=%v, want empty nil-error", segments, err)
	}

	first, err := runtime.Store().SaveHandoffSegment(ctx, HandoffSegment{
		SessionID:       session.ID,
		Sequence:        1,
		StartEventIndex: 0,
		EndEventIndex:   3,
		Summary:         "Initial handoff summary",
		Mode:            "auto",
		EstimatedTokens: 48,
		Active:          true,
	})
	if err != nil {
		t.Fatalf("SaveHandoffSegment first: %v", err)
	}
	if first.ID == "" || first.ContextRevisionID == "" {
		t.Fatalf("first handoff = %+v, want generated id and revision", first)
	}

	second, err := runtime.Store().ReplaceActiveHandoffSegments(ctx, session.ID, HandoffSegment{
		SessionID:         session.ID,
		ContextRevisionID: "ctx-revision-2",
		Sequence:          2,
		StartEventIndex:   4,
		EndEventIndex:     8,
		Summary:           "Replacement handoff summary",
		Mode:              "manual",
		Reason:            "compacted after provider response grew",
		EstimatedTokens:   64,
		Active:            true,
	}, []HandoffSegment{first})
	if err != nil {
		t.Fatalf("ReplaceActiveHandoffSegments: %v", err)
	}
	if second.ID == "" || second.ID == first.ID {
		t.Fatalf("replacement handoff = %+v, want distinct generated id", second)
	}

	all, err := runtime.Store().HandoffSegments(ctx, session.ID, false)
	if err != nil {
		t.Fatalf("HandoffSegments all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all handoff segments len = %d, want 2; segments=%+v", len(all), all)
	}
	if all[0].ID != first.ID || all[1].ID != second.ID {
		t.Fatalf("handoff order = [%s %s], want [%s %s]", all[0].ID, all[1].ID, first.ID, second.ID)
	}
	if all[0].Active || all[0].SupersededBy != second.ID {
		t.Fatalf("superseded segment = %+v, want inactive supersededBy=%s", all[0], second.ID)
	}
	if !all[1].Active || all[1].SupersededBy != "" {
		t.Fatalf("replacement segment = %+v, want active and not superseded", all[1])
	}

	activeOnly, err := runtime.Store().HandoffSegments(ctx, session.ID, true)
	if err != nil {
		t.Fatalf("HandoffSegments active: %v", err)
	}
	if len(activeOnly) != 1 || activeOnly[0].ID != second.ID {
		t.Fatalf("active handoff segments = %+v, want only replacement", activeOnly)
	}

	firstRevision, err := runtime.Store().HandoffSegmentsForRevision(ctx, session.ID, first.ContextRevisionID, false)
	if err != nil {
		t.Fatalf("HandoffSegmentsForRevision first: %v", err)
	}
	if len(firstRevision) != 1 || firstRevision[0].ID != first.ID || firstRevision[0].Active {
		t.Fatalf("first revision segments = %+v, want inactive original segment", firstRevision)
	}

	secondRevision, err := runtime.Store().HandoffSegmentsForRevision(ctx, session.ID, second.ContextRevisionID, true)
	if err != nil {
		t.Fatalf("HandoffSegmentsForRevision second active: %v", err)
	}
	if len(secondRevision) != 1 || secondRevision[0].ID != second.ID || !secondRevision[0].Active {
		t.Fatalf("second revision segments = %+v, want active replacement segment", secondRevision)
	}
}

func TestSessionNoticesPersistNormalizedEntriesAndHandleMissingState(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "agent-notice", "notice session")

	var nilStore *Store
	if _, err := nilStore.SaveSessionNotice(ctx, TimelineEntry{SessionID: session.ID}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nil store SaveSessionNotice err = %v, want os.ErrNotExist", err)
	}
	if notices, err := nilStore.SessionNotices(ctx, session.ID); err != nil || len(notices) != 0 {
		t.Fatalf("nil store SessionNotices notices=%+v err=%v, want empty nil-error", notices, err)
	}
	if _, err := runtime.Store().SaveSessionNotice(ctx, TimelineEntry{Text: "missing session"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SaveSessionNotice missing session err = %v, want os.ErrNotExist", err)
	}

	first, err := runtime.Store().SaveSessionNotice(ctx, TimelineEntry{
		ID:        "notice-a",
		SessionID: session.ID,
		Text:      "  context compacted  ",
		ToolCalls: []ToolCall{{ToolName: "strategy.save_draft", Status: "SUCCEEDED"}},
		Approvals: []Approval{{ToolName: "strategy.save_draft", Status: ApprovalStatusPending}},
		CreatedAt: "2026-06-22T08:00:00Z",
	})
	if err != nil {
		t.Fatalf("SaveSessionNotice first: %v", err)
	}
	if first.Kind != TimelineKindContextNotice || first.Status != TimelineStatusFinal || first.Text != "context compacted" {
		t.Fatalf("normalized first notice = %+v", first)
	}
	if len(first.ToolCalls) != 0 || len(first.Approvals) != 0 {
		t.Fatalf("first notice should not persist tool or approval payloads: %+v", first)
	}

	second, err := runtime.Store().SaveSessionNotice(ctx, TimelineEntry{
		ID:        "notice-b",
		SessionID: session.ID,
		RunID:     "run-1",
		Kind:      TimelineKindContextNotice,
		Status:    TimelineStatusError,
		Text:      "second notice",
		CreatedAt: "2026-06-22T08:00:01Z",
	})
	if err != nil {
		t.Fatalf("SaveSessionNotice second: %v", err)
	}

	notices, err := runtime.Store().SessionNotices(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionNotices: %v", err)
	}
	if len(notices) != 2 {
		t.Fatalf("notices len = %d, want 2; notices=%+v", len(notices), notices)
	}
	if notices[0].ID != first.ID || notices[1].ID != second.ID {
		t.Fatalf("notice order = [%s %s], want [%s %s]", notices[0].ID, notices[1].ID, first.ID, second.ID)
	}
	if len(notices[0].ToolCalls) != 0 || len(notices[0].Approvals) != 0 {
		t.Fatalf("stored notice should not include tool/approval payloads: %+v", notices[0])
	}
	if notices[1].RunID != "run-1" || notices[1].Status != TimelineStatusError {
		t.Fatalf("second notice = %+v, want run id and error status preserved", notices[1])
	}
	if empty, err := runtime.Store().SessionNotices(ctx, "   "); err != nil || len(empty) != 0 {
		t.Fatalf("blank session notices=%+v err=%v, want empty nil-error", empty, err)
	}
}
