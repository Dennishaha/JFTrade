package adk

import (
	"errors"
	"strings"
	"testing"
)

func newAutoCompactionFixture(t *testing.T, agentID string) (*Runtime, Agent, Session) {
	t.Helper()
	ctx := t.Context()

	runtime := newTestRuntime(t)
	provider, ok, err := runtime.Store().Provider(ctx, testProviderID)
	if err != nil || !ok {
		t.Fatalf("Provider: ok=%v err=%v", ok, err)
	}
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:                  testProviderID,
		DisplayName:         provider.DisplayName,
		BaseURL:             provider.BaseURL,
		Model:               provider.Model,
		APIKey:              "sk-test",
		ContextWindowTokens: 80,
		RequestTimeoutMs:    5000,
		Enabled:             true,
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:               agentID,
		Name:             "Coverage Auto Compact Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 1,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	session, raw := mustCreateSessionWithRaw(t, runtime, agent.ID, "Coverage Auto Compact Session")
	appendLargeContextEvents(t, runtime.rawSessionService, raw, 0, 80)

	snapshot, err := runtime.contextManager.ProjectedSnapshot(ctx, session, agent, strings.Repeat("pending input ", 200))
	if err != nil {
		t.Fatalf("ProjectedSnapshot: %v", err)
	}
	if _, shouldCompact := runtime.contextManager.ShouldAutoCompact(snapshot); !shouldCompact {
		t.Fatalf("snapshot = %+v, want auto compaction precondition", snapshot)
	}
	return runtime, agent, session
}

func TestRunnerChatCoveragePushesToward95(t *testing.T) {
	ctx := t.Context()

	t.Run("runChat returns invalid work mode after resolving a valid agent", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:     "coverage-invalid-mode-agent",
			Name:   "Coverage Invalid Mode Agent",
			Status: AgentStatusEnabled,
		})
		if _, err := runtime.runChat(ctx, ChatRequest{
			AgentID:          agent.ID,
			Message:          "hello",
			WorkModeOverride: "bad-mode",
		}, nil, false); err == nil || !strings.Contains(err.Error(), "invalid work mode") {
			t.Fatalf("runChat invalid work mode err = %v", err)
		}
	})

	t.Run("projectedChatResponse hydrates pending approvals from session projection", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:     "coverage-projected-approvals-agent",
			Name:   "Coverage Projected Approvals Agent",
			Status: AgentStatusEnabled,
		})
		session := mustCreateSession(t, runtime, agent.ID, "projected approvals")
		mustCreateADKSessionForAgent(t, runtime, agent.ID, session.ID)
		mustSaveRun(t, runtime, Run{
			ID:        "coverage-projected-approvals-run",
			SessionID: session.ID,
			AgentID:   agent.ID,
			Status:    RunStatusPending,
			PendingApprovals: []Approval{{
				ID: "approval-projected", RunID: "coverage-projected-approvals-run", Status: ApprovalStatusPending,
			}},
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
		})

		response := runtime.projectedChatResponse(ctx, session, Run{}, openAIChatResult{Reply: "draft"})
		if len(response.PendingApprovals) != 1 || response.PendingApprovals[0].ID != "approval-projected" {
			t.Fatalf("PendingApprovals = %+v, want projected pending approval", response.PendingApprovals)
		}
		if len(response.Run.PendingApprovals) != 1 || response.Run.PendingApprovals[0].ID != "approval-projected" {
			t.Fatalf("Run.PendingApprovals = %+v, want projected pending approval", response.Run.PendingApprovals)
		}
	})

	t.Run("context snapshot helpers cover success and fallback branches", func(t *testing.T) {
		runtime, agent, session := newAutoCompactionFixture(t, "coverage-context-snapshot-agent")
		if snapshot := runtime.contextSnapshotOrNil(ctx, session.ID); snapshot == nil {
			t.Fatal("contextSnapshotOrNil = nil, want snapshot")
		}

		runtime = newTestRuntime(t)
		agent = mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:     "coverage-context-skill-missing-agent",
			Name:   "Coverage Context Skill Missing Agent",
			Skills: []string{"missing-skill"},
			Status: AgentStatusEnabled,
		})
		session = mustCreateSession(t, runtime, agent.ID, "context skill missing")
		if snapshot := runtime.contextSnapshotForRunOrNil(ctx, session, Run{}); snapshot != nil {
			t.Fatalf("contextSnapshotForRunOrNil with missing skill = %#v, want nil", snapshot)
		}

		runtime, agent, session = newAutoCompactionFixture(t, "coverage-context-raw-error-agent")
		runtime.contextManager.rawService = &sessionContextGetErrorService{
			Service: runtime.rawSessionService,
			err:     errors.New("raw unavailable"),
		}
		if snapshot := runtime.contextSnapshotForRunOrNil(ctx, session, Run{AgentID: agent.ID}); snapshot != nil {
			t.Fatalf("contextSnapshotForRunOrNil raw failure = %#v, want nil", snapshot)
		}
	})

	t.Run("maybeAutoCompactSessionWithOptions covers notice and context error branches", func(t *testing.T) {
		t.Run("first notice error propagates", func(t *testing.T) {
			runtime, agent, session := newAutoCompactionFixture(t, "coverage-first-notice-agent")
			wantErr := errors.New("stop first notice")
			err := runtime.maybeAutoCompactSessionWithOptions(ctx, session, agent, strings.Repeat("pending input ", 200), func(ChatDelta) error {
				return wantErr
			}, false)
			if !errors.Is(err, wantErr) {
				t.Fatalf("maybeAutoCompactSessionWithOptions err = %v, want %v", err, wantErr)
			}
		})

		t.Run("runChat surfaces auto compaction delta errors", func(t *testing.T) {
			runtime, agent, session := newAutoCompactionFixture(t, "coverage-run-chat-compaction-agent")
			wantErr := errors.New("stop runChat compaction")
			_, err := runtime.runChat(ctx, ChatRequest{
				AgentID:   agent.ID,
				SessionID: session.ID,
				Message:   "hello after large context",
			}, func(ChatDelta) error {
				return wantErr
			}, false)
			if !errors.Is(err, wantErr) {
				t.Fatalf("runChat compaction err = %v, want %v", err, wantErr)
			}
		})

		t.Run("compaction failure emits an error notice and returns nil when sink accepts it", func(t *testing.T) {
			runtime, agent, session := newAutoCompactionFixture(t, "coverage-compact-failure-agent")
			installFailTrigger(t, runtime, "coverage_compact_fail_notice", tableHandoffSegments, "INSERT", "compact handoff failed")

			var deltas []ChatDelta
			err := runtime.maybeAutoCompactSessionWithOptions(ctx, session, agent, strings.Repeat("pending input ", 200), func(delta ChatDelta) error {
				deltas = append(deltas, delta)
				return nil
			}, false)
			if err != nil {
				t.Fatalf("maybeAutoCompactSessionWithOptions compact failure err = %v", err)
			}
			if len(deltas) != 2 || deltas[0].Timeline == nil || deltas[1].Timeline == nil {
				t.Fatalf("compact failure deltas = %+v, want two notice deltas", deltas)
			}
			if deltas[0].Timeline.Status != TimelineStatusStreaming || deltas[1].Timeline.Status != TimelineStatusError {
				t.Fatalf("compact failure notice statuses = %q/%q", deltas[0].Timeline.Status, deltas[1].Timeline.Status)
			}
		})

		t.Run("final notice error propagates after a successful compaction", func(t *testing.T) {
			runtime, agent, session := newAutoCompactionFixture(t, "coverage-final-notice-agent")
			wantErr := errors.New("stop final notice")
			calls := 0
			err := runtime.maybeAutoCompactSessionWithOptions(ctx, session, agent, strings.Repeat("pending input ", 200), func(ChatDelta) error {
				calls++
				if calls == 2 {
					return wantErr
				}
				return nil
			}, false)
			if !errors.Is(err, wantErr) {
				t.Fatalf("maybeAutoCompactSessionWithOptions final notice err = %v, want %v", err, wantErr)
			}
		})

		t.Run("context delta error propagates after notices succeed", func(t *testing.T) {
			runtime, agent, session := newAutoCompactionFixture(t, "coverage-context-delta-agent")
			wantErr := errors.New("stop context delta")
			sawContext := false
			err := runtime.maybeAutoCompactSessionWithOptions(ctx, session, agent, strings.Repeat("pending input ", 200), func(delta ChatDelta) error {
				if delta.Context != nil {
					sawContext = true
					return wantErr
				}
				return nil
			}, false)
			if !errors.Is(err, wantErr) {
				t.Fatalf("maybeAutoCompactSessionWithOptions context delta err = %v, want %v", err, wantErr)
			}
			if !sawContext {
				t.Fatal("context delta callback was not reached")
			}
		})
	})

	t.Run("CancelRun surfaces store lookup errors", func(t *testing.T) {
		runtime := &Runtime{store: newClosedStoreForLifecycle(t)}
		if _, err := runtime.CancelRun(ctx, "closed-store-run"); err == nil {
			t.Fatal("CancelRun closed store err = nil, want failure")
		}
	})
}
