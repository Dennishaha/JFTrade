package adk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	adkmodel "google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	adkskill "google.golang.org/adk/v2/tool/skilltoolset/skill"
	"google.golang.org/genai"
)

func TestFilteredSkillSourceForAgentHonorsAllowedToolsAndPermissionMode(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithRegistry(t, newTestRuntime(t).Store(), NewToolRegistry())
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name:         "restricted.tool",
		DisplayName:  "Restricted",
		Description:  "high-auto only tool",
		Permission:   "write_strategy",
		AllowedModes: []string{PermissionModeAll},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)

	writeSkillDir := filepath.Join(runtime.Store().SkillsPath(), "write-skill")
	if err := os.MkdirAll(writeSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll write-skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(writeSkillDir, "SKILL.md"), []byte("---\nname: write-skill\ndescription: write skill\nallowed-tools: [restricted.tool]\n---\nUse the restricted tool.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile write-skill: %v", err)
	}
	readSkillDir := filepath.Join(runtime.Store().SkillsPath(), "read-skill")
	if err := os.MkdirAll(readSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll read-skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(readSkillDir, "SKILL.md"), []byte("---\nname: read-skill\ndescription: read skill\nallowed-tools: [tools.search]\n---\nUse the read tool.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile read-skill: %v", err)
	}

	source, err := runtime.Skills().Source(ctx, []string{"write-skill", "read-skill"})
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	filtered, err := runtime.filteredSkillSourceForAgent(ctx, source, Agent{
		ID:             "agent",
		Tools:          []string{"restricted.tool", "tools.search"},
		Skills:         []string{"write-skill", "read-skill"},
		PermissionMode: PermissionModeApproval,
	})
	if err != nil {
		t.Fatalf("filteredSkillSourceForAgent: %v", err)
	}

	frontmatters, err := filtered.ListFrontmatters(ctx)
	if err != nil {
		t.Fatalf("ListFrontmatters: %v", err)
	}
	if len(frontmatters) != 1 || frontmatters[0].Name != "read-skill" {
		t.Fatalf("frontmatters = %#v, want only read-skill", frontmatters)
	}
	if _, err := filtered.LoadFrontmatter(ctx, "write-skill"); !errors.Is(err, adkskill.ErrSkillNotFound) {
		t.Fatalf("LoadFrontmatter(write-skill) err = %v, want ErrSkillNotFound", err)
	}
}

func TestSessionProjectionRestoresMessagesFromADKEvents(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "agent-events",
		Name:           "Agent",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "事件恢复")
	appendADKEvent(t, runtime, agent.ID, session.ID, newUserEvent("run-1", "用户问题", time.Unix(10, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newAssistantEvent("run-1", []*genai.Part{
		{Text: "先分析上下文。", Thought: true},
		{Text: "这是答案。"},
	}, time.Unix(11, 0)))

	restarted := newRuntimeWithRegistry(t, runtime.Store(), NewToolRegistry())
	messages := mustMessages(t, restarted, session.ID)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v, want 2", messages)
	}
	if messages[0].Role != "user" || messages[0].Content != "用户问题" {
		t.Fatalf("user message = %+v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "这是答案。" || messages[1].ReasoningContent != "先分析上下文。" {
		t.Fatalf("assistant message = %+v", messages[1])
	}
}

func TestProjectedToolResponsesRecoverApprovalFailureAndSuccessState(t *testing.T) {
	timestamp := "2026-07-02T00:00:00Z"
	state := &projectedRunState{
		runID:         "run-tools",
		toolCalls:     map[string]*ToolCall{},
		toolCallOrder: []string{},
	}
	state.reply.WriteString("准备调用工具")
	state.reasoning.WriteString("先检查账户")

	projectedToolResponse(state, &genai.FunctionResponse{
		ID:   "call-approval",
		Name: "account.place_order",
		Response: map[string]any{
			"error": "execution blocked: " + adktool.ErrConfirmationRequired.Error(),
		},
	}, timestamp)
	approvalCall := state.toolCalls["call-approval"]
	if approvalCall == nil || approvalCall.Status != "PENDING_APPROVAL" || !approvalCall.RequiresUser || approvalCall.CompletedAt != nil {
		t.Fatalf("approval tool call = %#v", approvalCall)
	}
	if state.preToolContent != "准备调用工具" || state.preToolReasoning != "先检查账户" {
		t.Fatalf("pre-tool content/reasoning = %q/%q", state.preToolContent, state.preToolReasoning)
	}

	projectedToolResponse(state, &genai.FunctionResponse{
		ID:   "call-failed",
		Name: "market.candles",
		Response: map[string]any{
			"error": "symbol is required",
		},
	}, timestamp)
	failedCall := state.toolCalls["call-failed"]
	if failedCall == nil || failedCall.Status != "FAILED" || failedCall.Error == nil || *failedCall.Error != "symbol is required" || failedCall.CompletedAt == nil {
		t.Fatalf("failed tool call = %#v", failedCall)
	}

	projectedToolResponse(state, &genai.FunctionResponse{
		ID:   "call-ok",
		Name: "market.snapshot",
		Response: map[string]any{
			"symbol": "US.AAPL",
			"price":  200,
		},
	}, timestamp)
	successCall := state.toolCalls["call-ok"]
	if successCall == nil || successCall.Status != "SUCCEEDED" || successCall.Output == nil || successCall.CompletedAt == nil {
		t.Fatalf("success tool call = %#v", successCall)
	}

	if projectedToolResponse(nil, &genai.FunctionResponse{ID: "nil"}, timestamp); len(state.toolCalls) != 3 {
		t.Fatalf("nil state should not mutate calls: %#v", state.toolCalls)
	}
	if projectedToolProgress("") != "🔧 执行工具 unknown..." || projectionRunID(nil) != "" {
		t.Fatalf("progress/run fallback failed")
	}
}

func TestTranscriptProjectionFallsBackToStableIDsAndSkipsEmptyContent(t *testing.T) {
	if _, ok := transcriptEntryFromADKEvent(nil); ok {
		t.Fatal("nil event produced transcript entry")
	}
	if _, ok := transcriptEntryFromADKEvent(&adksession.Event{ID: "empty"}); ok {
		t.Fatal("empty event produced transcript entry")
	}

	at := time.Unix(200, 0).UTC()
	userEvent := newUserEvent("run-fallback", "用户输入", at)
	userEvent.ID = ""
	userEvent.InvocationID = ""
	entry, ok := transcriptEntryFromADKEvent(userEvent)
	if !ok {
		t.Fatal("expected user transcript entry")
	}
	if entry.ID != "event-message-"+at.Format(time.RFC3339Nano) || entry.Role != "user" || entry.Content != "用户输入" {
		t.Fatalf("fallback transcript entry = %#v", entry)
	}

	assistantEvent := newAssistantEvent("run-assistant", []*genai.Part{
		{Text: "隐藏推理", Thought: true},
		{Text: "可见回答"},
		nil,
	}, at)
	assistantEvent.ID = "event-assistant"
	assistantEntry, ok := transcriptEntryFromADKEvent(assistantEvent)
	if !ok || assistantEntry.ID != "event-assistant" || assistantEntry.RunID != "run-assistant" || assistantEntry.Content != "可见回答" || assistantEntry.ReasoningContent != "隐藏推理" {
		t.Fatalf("assistant transcript entry = %#v ok=%v", assistantEntry, ok)
	}
}

func TestSessionTimelineIncludesContextNoticeWithoutProjection(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "agent-context-notice",
		Name:           "Context Notice Agent",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context Notice")
	notice, err := runtime.Store().SaveSessionNotice(ctx, TimelineEntry{
		ID:        "notice-context-compaction",
		SessionID: session.ID,
		Kind:      TimelineKindContextNotice,
		Status:    TimelineStatusFinal,
		Text:      contextCompactionDoneText,
		CreatedAt: "2026-06-17T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("SaveSessionNotice: %v", err)
	}

	timeline, ok, err := runtime.Store().SessionTimeline(ctx, session.ID)
	if err != nil || !ok {
		t.Fatalf("SessionTimeline ok=%v err=%v", ok, err)
	}
	if len(timeline) != 1 {
		t.Fatalf("timeline len = %d, want 1: %+v", len(timeline), timeline)
	}
	if timeline[0].ID != notice.ID || timeline[0].Kind != TimelineKindContextNotice || timeline[0].Text != contextCompactionDoneText {
		t.Fatalf("timeline notice = %+v, want saved context notice", timeline[0])
	}

	messages, err := runtime.Store().TranscriptEntries(ctx, session.ID)
	if err != nil {
		t.Fatalf("TranscriptEntries: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages = %+v, want notice excluded from transcript", messages)
	}
}

func TestSessionTimelineUsesRunUserMessageAsOriginalPrompt(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "agent-timeline-original",
		Name:           "Timeline Original Agent",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Timeline Original")
	run := mustSaveRun(t, runtime, Run{
		ID:          "run-timeline-original",
		SessionID:   session.ID,
		AgentID:     agent.ID,
		Status:      RunStatusCompleted,
		UserMessage: "设计个适合 tme 的策略",
		CreatedAt:   "2026-06-17T00:00:00Z",
		UpdatedAt:   "2026-06-17T00:00:01Z",
	})
	appendADKEvent(t, runtime, agent.ID, session.ID, newUserEvent(run.ID, "请推进这个目标。\n\n用户原始目标：设计个适合 tme 的策略", time.Unix(10, 0)))

	timeline, ok, err := runtime.Store().SessionTimeline(ctx, session.ID)
	if err != nil || !ok {
		t.Fatalf("SessionTimeline ok=%v err=%v", ok, err)
	}
	if len(timeline) != 1 {
		t.Fatalf("timeline len = %d, want 1: %+v", len(timeline), timeline)
	}
	entry := timeline[0]
	if entry.Kind != TimelineKindUserMessage || entry.Text != run.UserMessage || entry.OriginalText != run.UserMessage {
		t.Fatalf("timeline user entry = %+v, want original prompt", entry)
	}
	if entry.ProcessedText != "请推进这个目标。\n\n用户原始目标：设计个适合 tme 的策略" {
		t.Fatalf("processedText = %q, want ADK event prompt", entry.ProcessedText)
	}
}

func TestSessionTimelineRestoresGoalPromptWhenInvocationMismatchesRun(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "agent-timeline-goal-mismatch",
		Name:           "Timeline Goal Mismatch Agent",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Timeline Goal Mismatch")
	run := mustSaveRun(t, runtime, Run{
		ID:          "run-timeline-nvda",
		SessionID:   session.ID,
		AgentID:     agent.ID,
		Status:      RunStatusCompleted,
		WorkMode:    WorkModeLoop,
		UserMessage: "编写个适合nvda的策略",
		Objective:   "编写个适合nvda的策略",
		CreatedAt:   "2026-06-18T00:00:00Z",
		UpdatedAt:   "2026-06-18T00:00:01Z",
	})
	processed := goalOrchestratorUserMessage(run)
	appendADKEvent(t, runtime, agent.ID, session.ID, newUserEvent("adk-invocation-nvda", processed, time.Unix(10, 0)))

	timeline, ok, err := runtime.Store().SessionTimeline(ctx, session.ID)
	if err != nil || !ok {
		t.Fatalf("SessionTimeline ok=%v err=%v", ok, err)
	}
	if len(timeline) != 1 {
		t.Fatalf("timeline len = %d, want 1: %+v", len(timeline), timeline)
	}
	entry := timeline[0]
	if entry.RunID != run.ID || entry.Text != run.UserMessage || entry.OriginalText != run.UserMessage {
		t.Fatalf("timeline user entry = %+v, want original prompt for run %s", entry, run.ID)
	}
	if entry.ProcessedText != processed {
		t.Fatalf("processedText = %q, want %q", entry.ProcessedText, processed)
	}
}

func TestSessionTimelineHidesGoalDecisionPrompts(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "agent-timeline-goal-decision",
		Name:           "Timeline Goal Decision Agent",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Timeline Goal Decision")
	run := mustSaveRun(t, runtime, Run{
		ID:          "run-timeline-goal-decision",
		SessionID:   session.ID,
		AgentID:     agent.ID,
		Status:      RunStatusRunning,
		WorkMode:    WorkModeLoop,
		UserMessage: "编写个适合nvda的策略",
		Objective:   "编写个适合nvda的策略",
		CreatedAt:   "2026-06-18T00:00:00Z",
		UpdatedAt:   "2026-06-18T00:00:01Z",
	})
	processed := goalOrchestratorUserMessage(run)
	appendADKEvent(t, runtime, agent.ID, session.ID, newUserEvent(run.ID, processed, time.Unix(10, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newUserEvent(run.ID, goalDecisionPrompt(run, "第一轮回复", false), time.Unix(11, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newUserEvent(run.ID, goalOrchestratorContinueNudge(run, "还要继续完善策略"), time.Unix(12, 0)))

	timeline, ok, err := runtime.Store().SessionTimeline(ctx, session.ID)
	if err != nil || !ok {
		t.Fatalf("SessionTimeline ok=%v err=%v", ok, err)
	}
	if len(timeline) != 1 {
		t.Fatalf("timeline len = %d, want only original user message: %+v", len(timeline), timeline)
	}
	entry := timeline[0]
	if entry.Text != run.UserMessage || entry.OriginalText != run.UserMessage || entry.ProcessedText != processed {
		t.Fatalf("timeline user entry = %+v, want original plus first processed prompt", entry)
	}
}

func TestSessionTimelineOmitsPromptVariantsWhenPromptUnchanged(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "agent-timeline-plain",
		Name:           "Timeline Plain Agent",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Timeline Plain")
	run := mustSaveRun(t, runtime, Run{
		ID:          "run-timeline-plain",
		SessionID:   session.ID,
		AgentID:     agent.ID,
		Status:      RunStatusCompleted,
		UserMessage: "普通问题",
		CreatedAt:   "2026-06-17T00:00:00Z",
		UpdatedAt:   "2026-06-17T00:00:01Z",
	})
	appendADKEvent(t, runtime, agent.ID, session.ID, newUserEvent(run.ID, "普通问题", time.Unix(10, 0)))

	timeline, ok, err := runtime.Store().SessionTimeline(ctx, session.ID)
	if err != nil || !ok {
		t.Fatalf("SessionTimeline ok=%v err=%v", ok, err)
	}
	if len(timeline) != 1 {
		t.Fatalf("timeline len = %d, want 1: %+v", len(timeline), timeline)
	}
	entry := timeline[0]
	if entry.Kind != TimelineKindUserMessage || entry.Text != "普通问题" {
		t.Fatalf("timeline user entry = %+v, want plain prompt", entry)
	}
	if entry.OriginalText != "" || entry.ProcessedText != "" {
		t.Fatalf("prompt variants = original %q processed %q, want omitted", entry.OriginalText, entry.ProcessedText)
	}
}

func TestSessionProjectionRecoversPreToolContentAndToolOrder(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "agent-tool-order",
		Name:           "Agent",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "工具恢复")

	appendADKEvent(t, runtime, agent.ID, session.ID, newUserEvent("run-2", "帮我执行", time.Unix(20, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newAssistantEvent("run-2", []*genai.Part{{Text: "先分析市场。"}}, time.Unix(21, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newToolCallEvent("run-2", "call-1", "market.snapshot", time.Unix(22, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newToolResponseEvent("run-2", "call-1", "market.snapshot", map[string]any{"ok": true}, time.Unix(23, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newToolCallEvent("run-2", "call-2", "portfolio.summary", time.Unix(24, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newToolResponseEvent("run-2", "call-2", "portfolio.summary", map[string]any{"ok": true}, time.Unix(25, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newAssistantEvent("run-2", []*genai.Part{{Text: "完成。"}}, time.Unix(26, 0)))

	projection, ok, err := runtime.Store().SessionProjection(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionProjection: %v", err)
	}
	if !ok {
		t.Fatal("expected session projection from events")
	}
	if projection.PreToolContent != "先分析市场。" {
		t.Fatalf("preToolContent = %q, want 先分析市场。", projection.PreToolContent)
	}
	if len(projection.ToolCalls) != 2 {
		t.Fatalf("tool calls = %#v, want 2", projection.ToolCalls)
	}
	if projection.ToolCalls[0].ToolName != "market.snapshot" || projection.ToolCalls[1].ToolName != "portfolio.summary" {
		t.Fatalf("tool call order = %#v", projection.ToolCalls)
	}
	if projection.ToolCalls[0].Status != "SUCCEEDED" || projection.ToolCalls[1].Status != "SUCCEEDED" {
		t.Fatalf("tool call statuses = %#v", projection.ToolCalls)
	}
	if projection.Reply != "先分析市场。完成。" {
		t.Fatalf("reply = %q, want 先分析市场。完成。", projection.Reply)
	}
}

func TestSessionProjectionIgnoresResolvedApprovalsAsReply(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "agent-approved-resolution",
		Name:           "Agent",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "approved resolution")
	run := mustSaveRun(t, runtime, Run{
		ID:        "run-approved-resolution",
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    RunStatusCompleted,
		PendingApprovals: []Approval{{
			ID:        "approval-approved",
			RunID:     "run-approved-resolution",
			AgentID:   agent.ID,
			ToolName:  "strategy.save_draft",
			Status:    ApprovalStatusApproved,
			Reason:    "resolved",
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
		}},
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
	})

	appendADKEvent(t, runtime, agent.ID, session.ID, newAssistantEvent(run.ID, []*genai.Part{{Text: "done"}}, time.Unix(40, 0)))

	projection, ok, err := runtime.Store().SessionProjection(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionProjection: %v", err)
	}
	if !ok {
		t.Fatal("expected session projection")
	}
	if len(projection.PendingApprovals) != 0 {
		t.Fatalf("projection pending approvals = %+v, want none", projection.PendingApprovals)
	}
}

func appendADKEvent(t *testing.T, runtime *Runtime, agentID string, sessionID string, event *adksession.Event) {
	t.Helper()
	response, err := runtime.rawSessionService.Get(context.Background(), &adksession.GetRequest{
		AppName:   googleADKAppName(agentID),
		UserID:    googleADKUserID,
		SessionID: sessionID,
	})
	if err != nil {
		created, createErr := runtime.rawSessionService.Create(context.Background(), &adksession.CreateRequest{
			AppName:   googleADKAppName(agentID),
			UserID:    googleADKUserID,
			SessionID: sessionID,
		})
		if createErr != nil {
			t.Fatalf("Create session service session: %v", createErr)
		}
		response = &adksession.GetResponse{Session: created.Session}
	}
	if err := runtime.rawSessionService.AppendEvent(context.Background(), response.Session, event); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
}

func newUserEvent(runID string, text string, ts time.Time) *adksession.Event {
	event := adksession.NewEvent(context.Background(), runID)
	event.ID = "user-" + runID + "-" + ts.UTC().Format(time.RFC3339Nano)
	event.Author = "user"
	event.Timestamp = ts
	event.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromText(text, genai.RoleUser),
	}
	return event
}

func newAssistantEvent(runID string, parts []*genai.Part, ts time.Time) *adksession.Event {
	event := adksession.NewEvent(context.Background(), runID)
	event.ID = "assistant-" + runID + "-" + ts.UTC().Format(time.RFC3339Nano)
	event.Author = googleADKAgentName("agent")
	event.Timestamp = ts
	event.LLMResponse = adkmodel.LLMResponse{
		Content:      genai.NewContentFromParts(parts, genai.RoleModel),
		TurnComplete: true,
	}
	return event
}

func newToolCallEvent(runID string, callID string, name string, ts time.Time) *adksession.Event {
	event := adksession.NewEvent(context.Background(), runID)
	event.ID = "tool-call-" + callID
	event.Author = googleADKAgentName("agent")
	event.Timestamp = ts
	event.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromParts([]*genai.Part{{
			FunctionCall: &genai.FunctionCall{ID: callID, Name: name, Args: map[string]any{"input": name}},
		}}, genai.RoleModel),
	}
	return event
}

func newToolResponseEvent(runID string, callID string, name string, response map[string]any, ts time.Time) *adksession.Event {
	event := adksession.NewEvent(context.Background(), runID)
	event.ID = "tool-response-" + callID
	event.Author = googleADKAgentName("agent")
	event.Timestamp = ts
	event.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromParts([]*genai.Part{{
			FunctionResponse: &genai.FunctionResponse{ID: callID, Name: name, Response: response},
		}}, genai.RoleModel),
	}
	return event
}
