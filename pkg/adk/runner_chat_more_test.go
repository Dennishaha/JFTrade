package adk

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	adkagent "google.golang.org/adk/v2/agent"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	"google.golang.org/genai"
)

type notFoundCreateErrorSessionService struct {
	adksession.Service
	err error
}

func (service notFoundCreateErrorSessionService) Get(context.Context, *adksession.GetRequest) (*adksession.GetResponse, error) {
	return nil, errors.New("record not found")
}

func (service notFoundCreateErrorSessionService) Create(context.Context, *adksession.CreateRequest) (*adksession.CreateResponse, error) {
	return nil, service.err
}

func newNoopADKAgent(t *testing.T, name string) adkagent.Agent {
	t.Helper()
	agent, err := adkagent.New(adkagent.Config{
		Name:        name,
		Description: "noop",
		Run: func(adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
			return func(func(*adksession.Event, error) bool) {}
		},
	})
	if err != nil {
		t.Fatalf("adkagent.New(%s): %v", name, err)
	}
	return agent
}

func mustCreateADKSessionForAgent(t *testing.T, runtime *Runtime, agentID string, sessionID string) adksession.Session {
	t.Helper()
	created, err := runtime.rawSessionService.Create(t.Context(), &adksession.CreateRequest{
		AppName: googleADKAppName(agentID), UserID: googleADKUserID, SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("Create ADK session: %v", err)
	}
	return created.Session
}

func TestGoogleADKExecuteAdditionalBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("missing provider fails before execution starts", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "execute-missing-provider-agent", "execute missing provider")

		_, _, _, _, _, err := runtime.executeGoogleADK(ctx, Agent{
			ID: "execute-missing-provider-agent", Name: "Execute Missing Provider", ProviderID: "missing-provider",
			Model: "test-model", Status: AgentStatusEnabled,
		}, session, "run-execute-missing-provider", "hello", nil)
		if err == nil || !strings.Contains(err.Error(), "provider") {
			t.Fatalf("executeGoogleADK missing provider err = %v", err)
		}
	})

	t.Run("stream delta error is surfaced as run failure", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "execute-delta-agent", Name: "Execute Delta", Status: AgentStatusEnabled,
		})
		session := mustCreateSession(t, runtime, agent.ID, "execute delta")

		_, approvals, result, _, _, err := runtime.executeGoogleADK(ctx, agent, session, "run-execute-delta", "hello", func(ChatDelta) error {
			return errors.New("delta sink failed")
		})
		if err == nil || !strings.Contains(err.Error(), "delta sink failed") {
			t.Fatalf("executeGoogleADK delta err = %v", err)
		}
		if len(approvals) != 0 {
			t.Fatalf("approvals = %+v, want none", approvals)
		}
		if strings.TrimSpace(result.Reply) == "" {
			t.Fatalf("result = %+v, want accumulated reply even when delta sink fails late", result)
		}
	})

	t.Run("pending approval lookup errors from raw ADK session bubble up", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "execute-pending-agent", Name: "Execute Pending Lookup", Status: AgentStatusEnabled,
		})
		session := mustCreateSession(t, runtime, agent.ID, "execute pending lookup")
		runtime.rawSessionService = getErrorADKSessionService{Service: runtime.rawSessionService, err: errors.New("pending approvals failed")}

		_, _, _, _, _, err := runtime.executeGoogleADK(ctx, agent, session, "run-execute-pending-lookup", "hello", nil)
		if err == nil || !strings.Contains(err.Error(), "pending approvals failed") {
			t.Fatalf("executeGoogleADK pending approvals err = %v", err)
		}
	})

	t.Run("approval-required tool keeps execution attached for resume", func(t *testing.T) {
		runtime := newTestRuntime(t)
		runtime.tools.Register(ToolDescriptor{
			Name:         "approval.branch",
			DisplayName:  "Approval Branch",
			Description:  "requires confirmation",
			Category:     "test",
			Permission:   "write_strategy",
			AllowedModes: []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
		}, func(context.Context, map[string]any) (any, error) {
			return map[string]any{"ok": true}, nil
		})
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:             "execute-approval-agent",
			Name:           "Execute Approval",
			Status:         AgentStatusEnabled,
			Tools:          []string{"approval.branch"},
			PermissionMode: PermissionModeApproval,
		})
		session := mustCreateSession(t, runtime, agent.ID, "execute approval")

		toolContext, approvals, _, _, _, err := runtime.executeGoogleADK(ctx, agent, session, "run-execute-approval", "@approval.branch", nil)
		if err != nil {
			t.Fatalf("executeGoogleADK approval err = %v", err)
		}
		if len(toolContext.calls) != 1 || len(approvals) != 1 {
			t.Fatalf("toolContext=%+v approvals=%+v, want one pending approval tool call", toolContext, approvals)
		}
		if _, ok := runtime.adkRuns["run-execute-approval"]; !ok {
			t.Fatal("approval execution was not retained for resume")
		}
	})

	t.Run("missing synthesized final reply is surfaced after tool execution", func(t *testing.T) {
		runtime := newTestRuntime(t)
		runtime.tools.Register(ToolDescriptor{
			Name:         "strategy.save_draft",
			DisplayName:  "Save Draft",
			Description:  "save strategy draft",
			Category:     "test",
			Permission:   "write_strategy",
			AllowedModes: []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
		}, func(context.Context, map[string]any) (any, error) {
			return map[string]any{"saved": true}, nil
		})
		providerID := saveGoalWorkflowProvider(t, runtime, "execute-missing-final-reply-provider", func(req openAIChatRequest) openAIChatMessage {
			if len(testProviderToolResponseNames(req.Messages)) > 0 {
				return openAIChatMessage{Role: "assistant", Content: "   "}
			}
			return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
				testProviderToolCall("call-save-draft", "strategy.save_draft", map[string]any{"name": "draft"}),
			}}
		})
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:         "execute-missing-final-reply-agent",
			Name:       "Execute Missing Final Reply",
			ProviderID: providerID,
			Status:     AgentStatusEnabled,
			Tools:      []string{"strategy.save_draft"},
		})
		session := mustCreateSession(t, runtime, agent.ID, "execute missing final reply")

		toolContext, approvals, result, _, _, err := runtime.executeGoogleADK(ctx, agent, session, "run-execute-missing-final-reply", "保存策略草稿", nil)
		if err == nil || !strings.Contains(err.Error(), errADKMissingFinalReply().Error()) {
			t.Fatalf("executeGoogleADK missing final reply err = %v, want %v", err, errADKMissingFinalReply())
		}
		if len(approvals) != 0 || len(toolContext.calls) != 1 || toolContext.calls[0].Status != "SUCCEEDED" {
			t.Fatalf("toolContext=%+v approvals=%+v, want one succeeded tool call and no approvals", toolContext, approvals)
		}
		if strings.TrimSpace(result.Reply) != "" {
			t.Fatalf("result = %+v, want no synthesized reply after the missing-final-reply failure", result)
		}
	})

}

func TestGoogleADKResumeAdditionalBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "resume-more-agent", Name: "Resume More", Status: AgentStatusEnabled,
		WorkMode: WorkModeChat, PermissionMode: PermissionModeApproval,
	})

	newExecution := func(sessionID string, runID string, runBlocking func(*googleADKExecution, context.Context, *genai.Content) error) *googleADKExecution {
		execution := &googleADKExecution{
			sessionID: sessionID,
			appName:   googleADKAppName(agent.ID),
			agent:     agent,
			runID:     runID,
			runIDByAgentName: map[string]string{
				googleADKAgentName(agent.ID): runID,
			},
			runSnapshotBaseByID: map[string]Run{
				runID: {ID: runID, SessionID: sessionID, AgentID: agent.ID, Status: RunStatusRunning, Usage: &RunUsage{}},
			},
			descriptors:              map[string]ToolDescriptor{},
			calls:                    []ToolCall{},
			summaries:                []string{},
			replyByRunID:             map[string]*strings.Builder{},
			reasoningByRunID:         map[string]*strings.Builder{},
			bufferedReplyByRunID:     map[string]*strings.Builder{},
			bufferedReasoningByRunID: map[string]*strings.Builder{},
			toolResponseSeenByRunID:  map[string]bool{},
			postToolTextByRunID:      map[string]bool{},
			toolResponseSeqByRunID:   map[string]int{},
			postToolTextSeqByRunID:   map[string]int{},
			sessionService:           runtime.rawSessionService,
			loadRun: func(ctx context.Context, id string) (Run, bool, error) {
				return runtime.Store().Run(ctx, id)
			},
			persistRunSnapshot: func(snapshot Run) (Run, error) {
				return runtime.persistRunActivitySnapshot(context.Background(), snapshot)
			},
		}
		execution.runBlocking = func(ctx context.Context, content *genai.Content) error {
			return runBlocking(execution, ctx, content)
		}
		return execution
	}
	saveResumeRun := func(sessionID string, id string, status string) Run {
		return mustSaveRun(t, runtime, Run{
			ID: id, SessionID: sessionID, AgentID: agent.ID, Status: RunStatusPending,
			UserMessage: "please resume", ResumeState: "waiting_approval",
			PendingApprovals: []Approval{{
				ID: id + "-approval", RunID: id, AgentID: agent.ID, ToolName: "write",
				Status: status, FunctionCallID: id + "-call", ConfirmationCallID: id + "-confirmation",
				Input: map[string]any{"x": id}, CreatedAt: nowString(), UpdatedAt: nowString(),
			}},
			ToolCalls: []ToolCall{{ID: id + "-call", RunID: id, ToolName: "write", Status: "PENDING"}},
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
	}

	t.Run("rehydrate errors are surfaced when no live execution exists", func(t *testing.T) {
		run := Run{
			ID: "resume-rehydrate-missing-session", SessionID: "missing-session", AgentID: agent.ID, ProviderID: testProviderID,
			Model: "test-model", PendingApprovals: []Approval{{
				ID: "approval", RunID: "resume-rehydrate-missing-session", ToolName: "write",
				Status: ApprovalStatusApproved, FunctionCallID: "call-1", ConfirmationCallID: "confirm-1",
			}},
		}
		updated, message, handled, err := runtime.resumeGoogleADK(ctx, run)
		if err == nil || !handled || message != nil || updated.ID != run.ID {
			t.Fatalf("resume rehydrate error run=%+v message=%+v handled=%v err=%v", updated, message, handled, err)
		}
	})

	t.Run("rehydrate with no usable approvals is ignored", func(t *testing.T) {
		session := mustCreateSession(t, runtime, agent.ID, "resume rehydrate empty")
		run := Run{
			ID: "resume-rehydrate-empty", SessionID: session.ID, AgentID: agent.ID, ProviderID: testProviderID, Model: "test-model",
			PendingApprovals: []Approval{{ID: "approval-empty", Status: ApprovalStatusApproved}},
		}
		updated, message, handled, err := runtime.resumeGoogleADK(ctx, run)
		if err != nil || handled || message != nil || updated.ID != run.ID {
			t.Fatalf("resume rehydrate empty run=%+v message=%+v handled=%v err=%v", updated, message, handled, err)
		}
	})

	t.Run("follow-up confirmation round stays pending and persists interim assistant text", func(t *testing.T) {
		session := mustCreateSession(t, runtime, agent.ID, "resume follow up approval")
		rawSession := mustCreateADKSessionForAgent(t, runtime, agent.ID, session.ID)
		run := saveResumeRun(session.ID, "resume-follow-up-approval", ApprovalStatusApproved)
		runtime.adkRuns[run.ID] = newExecution(session.ID, run.ID, func(execution *googleADKExecution, ctx context.Context, content *genai.Content) error {
			if !googleADKWorkflowHasFunctionResponse(content) {
				t.Fatalf("resume follow-up content = %+v, want function response", content)
			}
			if err := execution.appendVisibleTextForRun(run.ID, "需要第二轮审批。", "继续分析"); err != nil {
				return err
			}
			execution.ensureCall("resume-follow-up-call", ToolDescriptor{Name: "approval.required.followup"}, map[string]any{"step": 2})
			event := adksession.NewEvent(ctx, "resume-follow-up")
			event.Author = googleADKAgentName(agent.ID)
			event.Content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
				ID: "resume-follow-up-confirm", Name: toolconfirmation.FunctionCallName,
				Args: map[string]any{
					"originalFunctionCall": &genai.FunctionCall{
						ID: "resume-follow-up-call", Name: "approval.required.followup", Args: map[string]any{"step": 2},
					},
					"toolConfirmation": toolconfirmation.ToolConfirmation{Hint: "approve second step"},
				},
			}}}, genai.RoleModel)
			return appendADKEventWithStaleRetry(ctx, runtimeAppendLocks(runtime), runtime.rawSessionService, rawSession, event)
		})

		updated, message, handled, err := runtime.resumeGoogleADK(ctx, run)
		if err != nil || !handled || message != nil {
			t.Fatalf("resume follow-up run=%+v message=%+v handled=%v err=%v", updated, message, handled, err)
		}
		if updated.Status != RunStatusPending || updated.ResumeState != "waiting_approval" || updated.FinalMessageID == "" {
			t.Fatalf("updated run = %+v, want pending waiting_approval with interim assistant message", updated)
		}
		if len(updated.PendingApprovals) != 1 || updated.PendingApprovals[0].ConfirmationCallID != "resume-follow-up-confirm" {
			t.Fatalf("updated approvals = %+v, want only the new follow-up approval", updated.PendingApprovals)
		}
	})

	t.Run("final synthesis failure marks resumed run failed and clears active execution", func(t *testing.T) {
		session := mustCreateSession(t, runtime, agent.ID, "resume final synthesis failure")
		mustCreateADKSessionForAgent(t, runtime, agent.ID, session.ID)
		run := saveResumeRun(session.ID, "resume-final-synthesis-failure", ApprovalStatusApproved)
		execution := newExecution(session.ID, run.ID, func(*googleADKExecution, context.Context, *genai.Content) error {
			return nil
		})
		execution.agent = Agent{ID: agent.ID, Name: "Broken Final Synthesis", ProviderID: "missing-provider"}
		call := execution.ensureCall("resume-final-tool-call", ToolDescriptor{Name: "branch.read"}, map[string]any{"ok": true})
		execution.finishCall(call.ID, map[string]any{"ok": true}, nil)
		runtime.adkRuns[run.ID] = execution

		updated, message, handled, err := runtime.resumeGoogleADK(ctx, run)
		if err != nil || !handled || message != nil {
			t.Fatalf("resume final synthesis failure run=%+v message=%+v handled=%v err=%v", updated, message, handled, err)
		}
		if updated.Status != RunStatusFailed || !strings.Contains(updated.FailureReason, "provider") {
			t.Fatalf("updated run = %+v, want failed resumed run with provider error", updated)
		}
		if _, ok := runtime.adkRuns[run.ID]; ok {
			t.Fatal("failed resumed execution should be removed from active ADK runs")
		}
	})
}

func TestGoogleADKRunnerConstructionAndSynthesisBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("attach runner handles missing session service, raw override and setup errors", func(t *testing.T) {
		execution := &googleADKExecution{appName: "attach-app", runID: "attach-run", agent: Agent{ID: "agent"}}
		productSession := Session{ID: "attach-session", AgentID: "agent"}
		noopAgent := newNoopADKAgent(t, "attach_noop")

		runtime := &Runtime{}
		attached, err := runtime.attachGoogleADKRunner(ctx, execution, productSession, noopAgent)
		if err != nil {
			t.Fatalf("attachGoogleADKRunner default service: %v", err)
		}
		if attached.runner == nil || attached.sessionService == nil {
			t.Fatalf("attached execution = %#v, want runner and session service", attached)
		}

		raw := adksession.InMemoryService()
		attached, err = (&Runtime{sessionService: adksession.InMemoryService(), rawSessionService: raw}).attachGoogleADKRunner(ctx, &googleADKExecution{
			appName: "attach-app-raw", runID: "attach-run-raw", agent: Agent{ID: "agent"},
		}, Session{ID: "attach-session-raw", AgentID: "agent"}, noopAgent)
		if err != nil {
			t.Fatalf("attachGoogleADKRunner raw override: %v", err)
		}
		if attached.sessionService != raw {
			t.Fatalf("attached session service = %#v, want raw override %#v", attached.sessionService, raw)
		}

		if _, err := (&Runtime{sessionService: getErrorADKSessionService{Service: adksession.InMemoryService(), err: errors.New("session unavailable")}}).attachGoogleADKRunner(
			ctx,
			&googleADKExecution{appName: "attach-app-get-error", runID: "attach-run-get-error", agent: Agent{ID: "agent"}},
			Session{ID: "attach-session-get-error", AgentID: "agent"},
			noopAgent,
		); err == nil || !strings.Contains(err.Error(), "get GO-ADK session") {
			t.Fatalf("attachGoogleADKRunner get error = %v", err)
		}

		if _, err := (&Runtime{sessionService: notFoundCreateErrorSessionService{Service: adksession.InMemoryService(), err: errors.New("create session failed")}}).attachGoogleADKRunner(
			ctx,
			&googleADKExecution{appName: "attach-app-create-error", runID: "attach-run-create-error", agent: Agent{ID: "agent"}},
			Session{ID: "attach-session-create-error", AgentID: "agent"},
			noopAgent,
		); err == nil || !strings.Contains(err.Error(), "create GO-ADK session") {
			t.Fatalf("attachGoogleADKRunner create error = %v", err)
		}

		if _, err := (&Runtime{sessionService: adksession.InMemoryService()}).attachGoogleADKRunner(
			ctx,
			&googleADKExecution{appName: "attach-app-runner-error", runID: "attach-runner-error", agent: Agent{ID: "agent"}},
			Session{ID: "attach-session-runner-error", AgentID: "agent"},
			nil,
		); err == nil || !strings.Contains(err.Error(), "create GO-ADK runner") {
			t.Fatalf("attachGoogleADKRunner runner error = %v", err)
		}
	})

	t.Run("final synthesis surfaces mapping and event projection errors", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "final-synthesis-extra-agent", Name: "Final Synthesis Extra", Status: AgentStatusEnabled,
		})
		session := mustCreateSession(t, runtime, agent.ID, "final synthesis extra")
		mustCreateADKSessionForAgent(t, runtime, agent.ID, session.ID)

		err := runtime.runGoogleADKFinalSynthesis(ctx, agent, session, &googleADKExecution{
			appName:        googleADKAppName(agent.ID),
			sessionID:      session.ID,
			sessionService: runtime.rawSessionService,
			agent:          agent,
			runID:          "final-synthesis-missing-mapping",
		}, "final-synthesis-missing-mapping", "summarize results")
		if err == nil || !strings.Contains(err.Error(), "mapping missing") {
			t.Fatalf("runGoogleADKFinalSynthesis mapping err = %v", err)
		}

		err = runtime.runGoogleADKFinalSynthesis(ctx, agent, session, &googleADKExecution{
			appName:        googleADKAppName(agent.ID),
			sessionID:      session.ID,
			sessionService: nil,
			agent:          agent,
			runID:          "final-synthesis-runner-error",
			runIDByAgentName: map[string]string{
				googleADKAgentName(agent.ID): "final-synthesis-runner-error",
			},
		}, "final-synthesis-runner-error", "summarize results")
		if err == nil || !strings.Contains(err.Error(), "create GO-ADK final synthesis runner") {
			t.Fatalf("runGoogleADKFinalSynthesis runner err = %v", err)
		}

		providerID := saveGoalWorkflowProvider(t, runtime, "final-synthesis-delta-provider", func(openAIChatRequest) openAIChatMessage {
			return openAIChatMessage{Role: "assistant", Content: "final synthesis content"}
		})
		agent.ProviderID = providerID
		err = runtime.runGoogleADKFinalSynthesis(ctx, agent, session, &googleADKExecution{
			appName:        googleADKAppName(agent.ID),
			sessionID:      session.ID,
			sessionService: runtime.rawSessionService,
			agent:          agent,
			runID:          "final-synthesis-delta-error",
			runIDByAgentName: map[string]string{
				googleADKAgentName(agent.ID): "final-synthesis-delta-error",
			},
			onDelta: func(ChatDelta) error { return errors.New("delta projection failed") },
		}, "final-synthesis-delta-error", "summarize results")
		if err == nil || !strings.Contains(err.Error(), "delta projection failed") {
			t.Fatalf("runGoogleADKFinalSynthesis delta err = %v", err)
		}
	})

	t.Run("model, llm agent and workflow execution surface direct boundary errors", func(t *testing.T) {
		runtime := newTestRuntime(t)
		mustSaveProvider(t, runtime, ProviderWriteRequest{
			ID:          "no-key-provider",
			DisplayName: "No Key",
			BaseURL:     "https://example.test/v1",
			Model:       "test-model",
			Enabled:     true,
		})
		if _, err := runtime.googleADKModelForAgent(ctx, Agent{
			ID: "model-no-key", Name: "Model No Key", ProviderID: "no-key-provider", Model: "test-model",
		}); err == nil || !strings.Contains(err.Error(), "API key is not configured") {
			t.Fatalf("googleADKModelForAgent no-key err = %v", err)
		}

		llm, err := runtime.googleADKModelForAgent(ctx, Agent{
			ID: "llm-skill-error", Name: "LLM Skill Error", ProviderID: testProviderID, Model: "test-model",
		})
		if err != nil {
			t.Fatalf("googleADKModelForAgent test provider: %v", err)
		}
		if _, err := runtime.newGoogleADKLLMAgent(ctx, "llm_skill_error", "LLM Skill Error", Agent{
			ID: "llm-skill-error", Name: "LLM Skill Error", ProviderID: testProviderID, Model: "test-model", Skills: []string{"missing-skill"},
		}, llm, &googleADKExecution{}); err == nil || !strings.Contains(err.Error(), "skill not found") {
			t.Fatalf("newGoogleADKLLMAgent skill err = %v", err)
		}

		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-exec-boundary-agent", Name: "Workflow Exec Boundary", Status: AgentStatusEnabled,
			WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow exec boundary")
		parent := mustSaveRun(t, runtime, Run{
			ID: "workflow-exec-boundary-parent", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning,
			WorkMode: WorkModeLoop, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.newGoogleADKWorkflowExecution(ctx, agent, session, parent, nil, nil, WorkModeLoop, RunOptions{}, nil); err == nil || !strings.Contains(err.Error(), "requires at least one sub-agent") {
			t.Fatalf("newGoogleADKWorkflowExecution no-child err = %v", err)
		}
		child := mustSaveRun(t, runtime, Run{
			ID: "workflow-exec-boundary-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
			Status: RunStatusRunning, UserMessage: "child step", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.newGoogleADKWorkflowExecution(ctx, agent, session, parent, []Run{child}, []workflowStep{{
			DependencyID: "step-1", Title: "child step", Message: "child step", DependsOn: []string{"missing-step"},
		}}, WorkModeLoop, RunOptions{}, nil); err == nil || !strings.Contains(err.Error(), "unknown dependency") {
			t.Fatalf("newGoogleADKWorkflowExecution bad dependency err = %v", err)
		}
	})
}

func TestRunChatAdditionalBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("request preparation and agent validation errors surface directly", func(t *testing.T) {
		runtime := newTestRuntime(t)
		if _, err := runtime.runChat(ctx, ChatRequest{Message: "   "}, nil, false); err == nil || err.Error() != "message is required" {
			t.Fatalf("runChat blank message err = %v", err)
		}

		badProviderAgent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
			ID: "run-chat-bad-provider", Name: "Run Chat Bad Provider", ProviderID: "missing-provider", Status: AgentStatusEnabled,
		})
		if err != nil {
			t.Fatalf("SaveAgent bad provider: %v", err)
		}
		if _, err := runtime.runChat(ctx, ChatRequest{
			AgentID: badProviderAgent.ID, Message: "hello",
		}, nil, false); err == nil || !strings.Contains(err.Error(), "provider") {
			t.Fatalf("runChat bad provider err = %v", err)
		}

		skillAgent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "run-chat-missing-skill", Name: "Run Chat Missing Skill", Status: AgentStatusEnabled, Skills: []string{"missing-skill"},
		})
		if _, err := runtime.runChat(ctx, ChatRequest{
			AgentID: skillAgent.ID, Message: "hello",
		}, nil, false); err == nil || !strings.Contains(err.Error(), "skill not found") {
			t.Fatalf("runChat missing skill err = %v", err)
		}
	})

	t.Run("emitRun delta errors stop chat before model execution", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "run-chat-emit-run", Name: "Run Chat Emit Run", Status: AgentStatusEnabled,
		})
		if _, err := runtime.runChat(ctx, ChatRequest{
			AgentID: agent.ID, Message: "hello",
		}, func(delta ChatDelta) error {
			if delta.Run == nil {
				t.Fatalf("delta = %+v, want initial run snapshot", delta)
			}
			return errors.New("emit run failed")
		}, true); err == nil || !strings.Contains(err.Error(), "emit run failed") {
			t.Fatalf("runChat emitRun err = %v", err)
		}
	})

	t.Run("pending and terminal persistence failures are surfaced", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "run-chat-save-error-agent", "run chat save error")
		run := mustSaveRun(t, runtime, Run{
			ID: "run-chat-save-error", SessionID: session.ID, AgentID: "run-chat-save-error-agent",
			Status: RunStatusRunning, CreatedAt: nowString(), UpdatedAt: nowString(), StartedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table: %v", err)
		}
		if _, err := runtime.finishPendingApprovalRun(ctx, session, run, []Approval{{ID: "approval-save-error", Status: ApprovalStatusPending}}); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("finishPendingApprovalRun save err = %v", err)
		}
		if _, err := runtime.completeChatRun(ctx, session, run, "hello", toolExecutionContext{}, nil, openAIChatResult{}, errors.New("boom")); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("completeChatRun terminal save err = %v", err)
		}
	})

	t.Run("assistant message persistence failures are surfaced after run completion", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "run-chat-message-error-agent", "run chat message error")
		run := mustSaveRun(t, runtime, Run{
			ID: "run-chat-message-error", SessionID: session.ID, AgentID: "run-chat-message-error-agent",
			Status: RunStatusRunning, CreatedAt: nowString(), UpdatedAt: nowString(), StartedAt: nowString(), Usage: &RunUsage{},
		})
		runtime.rawSessionService = createErrorSessionService{err: errors.New("create failed")}
		if _, err := runtime.completeChatRun(ctx, session, run, "hello", toolExecutionContext{}, nil, openAIChatResult{Reply: "final"}, nil); err == nil || !strings.Contains(err.Error(), "create failed") {
			t.Fatalf("completeChatRun attach message err = %v", err)
		}
	})
}
