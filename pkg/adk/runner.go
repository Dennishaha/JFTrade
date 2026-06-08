package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	adksession "google.golang.org/adk/session"
)

type Runtime struct {
	store          *Store
	tools          *ToolRegistry
	skills         *SkillRegistry
	sessionService adksession.Service
	openai         openAIClient
	limitsProvider RuntimeLimitsProvider
	activeMu       sync.Mutex
	activeRuns     map[string]context.CancelFunc
	adkMu          sync.Mutex
	adkRuns        map[string]*googleADKExecution
	runSem         chan struct{} // Concurrency limiter for active runs
}

func NewRuntime(store *Store, tools *ToolRegistry) *Runtime {
	return NewRuntimeWithSessionService(store, tools, nil)
}

func NewRuntimeWithSessionService(store *Store, tools *ToolRegistry, sessionService adksession.Service) *Runtime {
	if tools == nil {
		tools = NewToolRegistry()
	}
	skillsPath := ""
	if store != nil {
		skillsPath = store.SkillsPath()
	}
	r := &Runtime{
		store: store, tools: tools, skills: NewSkillRegistry(skillsPath), sessionService: sessionService, openai: newOpenAIClient(),
		activeRuns: map[string]context.CancelFunc{}, adkRuns: map[string]*googleADKExecution{},
		runSem: make(chan struct{}, MaxConcurrentRuns),
	}
	r.reconcileStaleRuns(context.Background())
	return r
}

func (r *Runtime) SetRuntimeLimitsProvider(provider RuntimeLimitsProvider) {
	if r == nil {
		return
	}
	r.limitsProvider = provider
}

func (r *Runtime) runtimeLimits() RuntimeLimits {
	limits := RuntimeLimits{RunTimeout: DefaultRunTimeout}
	if r == nil || r.limitsProvider == nil {
		return limits
	}
	updated := r.limitsProvider()
	if updated.RunTimeout > 0 {
		limits.RunTimeout = updated.RunTimeout
	}
	return limits
}

func runTimeoutForRun(run Run) time.Duration {
	if run.MaxDurationMs > 0 {
		return time.Duration(run.MaxDurationMs) * time.Millisecond
	}
	return DefaultRunTimeout
}

// reconcileStaleRuns reclassifies unfinished runs from a previous process
// lifecycle. RUNNING runs are marked orphaned. PENDING_APPROVAL runs remain
// pending only when they still have recoverable GO-ADK confirmation context.
func (r *Runtime) reconcileStaleRuns(ctx context.Context) {
	if r == nil || r.store == nil {
		return
	}
	runs, err := r.store.ListRuns(ctx)
	if err != nil {
		return
	}
	for _, run := range runs {
		if run.Status != RunStatusRunning && run.Status != RunStatusPending {
			continue
		}
		originalStatus := run.Status
		if originalStatus == RunStatusPending && runHasRecoverableApprovalContext(run) {
			continue
		}
		now := nowString()
		run.Status = RunStatusFailed
		run.ErrorCode = "RUN_ORPHANED"
		run.Message = "run was interrupted by server restart"
		run.FailureReason = "run was interrupted by server restart before completion"
		run.ResumeState = "restart_unrecoverable"
		run.CompletedAt = &now
		run.Degraded = true
		if originalStatus == RunStatusPending {
			run.FailureReason = "run was waiting for approval, but its ADK confirmation context could not be recovered after server restart"
			run.ResumeState = "approval_context_missing"
		}
		finalizeRunUsage(&run)
		_ = r.store.SaveRun(ctx, run)
		r.audit(ctx, "run.orphaned", run.ID, "Agent run became unrecoverable after server restart.", map[string]any{
			"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "resumeState": run.ResumeState,
		})
	}
}

func (r *Runtime) ReconcileExpiredRuns(ctx context.Context) {
	if r == nil || r.store == nil {
		return
	}
	runs, err := r.store.ListRuns(ctx)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, run := range runs {
		if run.Status != RunStatusRunning {
			continue
		}
		startedAt := strings.TrimSpace(run.StartedAt)
		if startedAt == "" {
			startedAt = strings.TrimSpace(run.CreatedAt)
		}
		started, parseErr := time.Parse(time.RFC3339Nano, startedAt)
		timeout := runTimeoutForRun(run)
		if parseErr != nil || now.Sub(started) < timeout {
			continue
		}
		r.activeMu.Lock()
		cancel := r.activeRuns[run.ID]
		delete(r.activeRuns, run.ID)
		r.activeMu.Unlock()
		if cancel != nil {
			cancel()
		}
		for index := range run.ToolCalls {
			call := &run.ToolCalls[index]
			if call.Status != "RUNNING" {
				continue
			}
			call.Status = "FAILED"
			errText := "run timed out while waiting for model or tool completion"
			call.Error = &errText
			finishToolCall(call)
		}
		completedAt := nowString()
		timeoutText := timeout.String()
		run.Status = RunStatusTimedOut
		run.Message = "run timed out"
		run.FailureReason = "run exceeded maximum duration of " + timeoutText
		run.ErrorCode = runErrorCode(RunStatusTimedOut)
		run.Degraded = true
		run.CompletedAt = &completedAt
		finalizeRunUsage(&run)
		_ = r.store.SaveRun(ctx, run)
		r.audit(ctx, "run.timed_out", run.ID, "Agent run timed out.", map[string]any{
			"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "errorCode": run.ErrorCode, "failureReason": run.FailureReason,
		})
	}
}

func (r *Runtime) Store() *Store {
	if r == nil {
		return nil
	}
	return r.store
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.activeMu.Lock()
	for id, cancel := range r.activeRuns {
		cancel()
		delete(r.activeRuns, id)
	}
	r.activeMu.Unlock()
	return errors.Join(CloseSessionService(r.sessionService), r.store.Close())
}

func (r *Runtime) Tools() *ToolRegistry {
	if r == nil {
		return nil
	}
	return r.tools
}

func (r *Runtime) Skills() *SkillRegistry {
	if r == nil {
		return nil
	}
	return r.skills
}

func (r *Runtime) Snapshot(ctx context.Context) (Snapshot, error) {
	providers, err := r.store.ListProviders(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	agents, err := r.store.ListAgents(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	skills, err := r.skills.List(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Providers: providers, Agents: agents, Skills: skills, Tools: r.tools.List()}, nil
}

func (r *Runtime) TestProvider(ctx context.Context, providerID string) (map[string]any, error) {
	provider, ok, err := r.store.Provider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("provider not found")
	}
	apiKey, _, err := r.store.ProviderAPIKey(provider.ID)
	if err != nil {
		return nil, err
	}
	reply, err := r.openai.chat(ctx, provider, apiKey, provider.Model, []openAIChatMessage{
		{Role: "system", Content: "Reply with a short health check sentence."},
		{Role: "user", Content: "JFTrade ADK provider connectivity test."},
	})
	if err != nil {
		return nil, err
	}
	_, toolErr := r.openai.selectTools(ctx, provider, apiKey, provider.Model, []openAIChatMessage{
		{Role: "user", Content: "Do not call a tool."},
	}, []ToolDescriptor{{
		Name: "system.health_probe", DisplayName: "Health probe", Description: "Provider capability probe.", Permission: "read_internal",
	}})
	capabilities := map[string]bool{
		"streaming": true,
		"tools":     toolErr == nil,
		"reasoning": false,
	}
	updated, updateErr := r.store.UpdateProviderCapabilities(ctx, provider.ID, capabilities)
	if updateErr != nil {
		return nil, updateErr
	}
	r.audit(ctx, "provider.tested", provider.ID, "Provider capability test completed.", map[string]any{"capabilities": capabilities})
	return map[string]any{"ok": true, "reply": reply, "capabilities": updated.Capabilities, "checkedAt": nowString()}, nil
}

func (r *Runtime) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return r.runChat(ctx, req, nil, false)
}

func (r *Runtime) ChatStream(ctx context.Context, req ChatRequest, onDelta func(ChatDelta) error) (ChatResponse, error) {
	return r.runChat(ctx, req, onDelta, true)
}

type toolExecutionContext struct {
	calls     []ToolCall
	summaries []string
}

// executeSelectedTools is a legacy compatibility path retained for deterministic
// tests and fallback experiments. Production chat execution must continue to
// flow through executeGoogleADK via Chat/ChatStream.
func (r *Runtime) executeSelectedTools(ctx context.Context, agent Agent, runID string, text string, history []openAIChatMessage) (toolExecutionContext, []Approval) {
	selected := r.selectToolInvocations(ctx, agent, text, history)
	return r.executeToolInvocations(ctx, agent, runID, text, selected)
}

func (r *Runtime) selectToolInvocations(ctx context.Context, agent Agent, text string, history []openAIChatMessage) []ToolInvocation {
	if strings.TrimSpace(agent.ProviderID) != "" {
		if provider, ok, err := r.store.Provider(ctx, agent.ProviderID); err == nil && ok && provider.Enabled {
			if apiKey, _, keyErr := r.store.ProviderAPIKey(provider.ID); keyErr == nil {
				messages := buildPromptMessages(agent, text, nil, history)
				selected, selectErr := r.openai.selectTools(ctx, provider, apiKey, defaultString(agent.Model, provider.Model), messages, ToolDescriptorsForAgent(agent, r.tools))
				if selectErr == nil && len(selected) > 0 {
					return normalizeToolInvocations(selected, agent, r.tools, text)
				}
			}
		}
	}
	return SelectToolInvocations(text, agent, r.tools)
}

func (r *Runtime) executeToolInvocations(ctx context.Context, agent Agent, runID string, text string, selected []ToolInvocation) (toolExecutionContext, []Approval) {
	result := toolExecutionContext{calls: []ToolCall{}, summaries: []string{}}
	approvals := []Approval{}
	for _, invocation := range selected {
		name := invocation.Name
		registered, ok := r.tools.Get(name)
		if !ok {
			continue
		}
		input := invocation.Input
		if input == nil {
			input = inferToolInput(name, text)
		}
		call := buildToolCall(runID, registered.Descriptor, input, "RUNNING")
		if !ToolAllowedInMode(registered.Descriptor, agent.PermissionMode) {
			errText := "tool is not allowed in permission mode " + agent.PermissionMode
			call.Status = "FAILED"
			call.Error = &errText
			finishToolCall(&call)
			result.calls = append(result.calls, call)
			continue
		}
		if ToolRequiresApproval(registered.Descriptor, agent.PermissionMode) {
			call.Status = "PENDING_APPROVAL"
			call.RequiresUser = true
			call.UpdatedAt = nowString()
			approval := Approval{
				ID:        "approval-" + uuid.NewString(),
				RunID:     runID,
				AgentID:   agent.ID,
				ToolName:  name,
				Input:     input,
				Status:    ApprovalStatusPending,
				Reason:    "当前权限模式要求用户审批该工具调用。",
				CreatedAt: nowString(),
				UpdatedAt: nowString(),
			}
			_ = r.store.SaveApproval(ctx, approval)
			approvals = append(approvals, approval)
			result.calls = append(result.calls, call)
			continue
		}
		output, err := executeRegisteredTool(contextWithToolAgent(ctx, agent), registered, input)
		if err != nil {
			errText := err.Error()
			call.Status = "FAILED"
			call.Error = &errText
		} else {
			call.Status = "SUCCEEDED"
			call.Output = limitToolOutput(output)
			result.summaries = append(result.summaries, summarizeToolOutput(name, output))
		}
		finishToolCall(&call)
		r.audit(ctx, "tool.executed", runID, "Agent tool execution completed.", map[string]any{
			"toolName": name, "status": call.Status, "durationMs": call.DurationMs,
		})
		result.calls = append(result.calls, call)
	}
	return result, approvals
}

func (r *Runtime) resolveAgent(ctx context.Context, agentID string) (Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return r.store.DefaultAgent(ctx)
	}
	agent, ok, err := r.store.Agent(ctx, agentID)
	if err != nil {
		return Agent{}, err
	}
	if !ok {
		return Agent{}, fmt.Errorf("agent not found")
	}
	if agent.Status == AgentStatusDisabled {
		return Agent{}, fmt.Errorf("agent is disabled")
	}
	if agent.DeletedAt != nil {
		return Agent{}, fmt.Errorf("agent is deleted")
	}
	if strings.TrimSpace(agent.ProviderID) != "" {
		provider, providerOK, providerErr := r.store.Provider(ctx, agent.ProviderID)
		if providerErr != nil {
			return Agent{}, providerErr
		}
		if !providerOK || !provider.Enabled {
			return Agent{}, fmt.Errorf("agent provider is unavailable")
		}
		if _, hasKey, keyErr := r.store.ProviderAPIKey(provider.ID); keyErr != nil {
			return Agent{}, keyErr
		} else if !hasKey {
			return Agent{}, fmt.Errorf("agent provider API key is not configured")
		}
	}
	return agent, nil
}

func (r *Runtime) resolveSession(ctx context.Context, sessionID string, agent Agent, text string) (Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		session, ok, err := r.store.Session(ctx, sessionID)
		if err != nil {
			return Session{}, err
		}
		if ok {
			if session.AgentID != "" && session.AgentID != agent.ID {
				return Session{}, fmt.Errorf("session belongs to a different agent")
			}
			return session, nil
		}
		return Session{}, fmt.Errorf("session not found")
	}
	title := text
	if len([]rune(title)) > 28 {
		title = string([]rune(title)[:28])
	}
	return r.store.CreateSession(ctx, agent.ID, title)
}

func (r *Runtime) DeleteSession(ctx context.Context, sessionID string) error {
	if r == nil || r.store == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	session, ok, err := r.store.Session(ctx, sessionID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("session not found")
	}
	if r.sessionService != nil {
		if err := r.sessionService.Delete(ctx, &adksession.DeleteRequest{
			AppName:   googleADKAppName(session.AgentID),
			UserID:    googleADKUserID,
			SessionID: session.ID,
		}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
			return err
		}
	}
	return r.store.DeleteSession(ctx, sessionID)
}

func (r *Runtime) generateReply(ctx context.Context, agent Agent, question string, toolSummaries []string, history []openAIChatMessage) (openAIChatResult, error) {
	if strings.TrimSpace(agent.ProviderID) == "" {
		return openAIChatResult{Reply: localReply(question, toolSummaries, nil)}, nil
	}
	provider, ok, err := r.store.Provider(ctx, agent.ProviderID)
	if err != nil {
		return openAIChatResult{}, err
	}
	if !ok || !provider.Enabled {
		return openAIChatResult{Reply: localReply(question, toolSummaries, fmt.Errorf("provider unavailable"))}, nil
	}
	apiKey, _, err := r.store.ProviderAPIKey(provider.ID)
	if err != nil {
		return openAIChatResult{}, err
	}
	messages := buildPromptMessages(agent, question, toolSummaries, history)
	return r.openai.chatDetailed(ctx, provider, apiKey, defaultString(agent.Model, provider.Model), messages)
}

func (r *Runtime) generateReplyStream(
	ctx context.Context,
	agent Agent,
	question string,
	toolSummaries []string,
	history []openAIChatMessage,
	onDelta func(ChatDelta) error,
) (openAIChatResult, error) {
	if strings.TrimSpace(agent.ProviderID) == "" {
		result := openAIChatResult{Reply: localReply(question, toolSummaries, nil)}
		if onDelta != nil {
			if err := onDelta(ChatDelta{Reply: result.Reply, ReasoningContent: result.ReasoningContent}); err != nil {
				return openAIChatResult{}, err
			}
		}
		return result, nil
	}
	provider, ok, err := r.store.Provider(ctx, agent.ProviderID)
	if err != nil {
		return openAIChatResult{}, err
	}
	if !ok || !provider.Enabled {
		result := openAIChatResult{Reply: localReply(question, toolSummaries, fmt.Errorf("provider unavailable"))}
		if onDelta != nil {
			if err := onDelta(ChatDelta{Reply: result.Reply, ReasoningContent: result.ReasoningContent}); err != nil {
				return openAIChatResult{}, err
			}
		}
		return result, nil
	}
	apiKey, _, err := r.store.ProviderAPIKey(provider.ID)
	if err != nil {
		return openAIChatResult{}, err
	}
	messages := buildPromptMessages(agent, question, toolSummaries, history)
	return r.openai.chatStream(ctx, provider, apiKey, defaultString(agent.Model, provider.Model), messages, onDelta)
}

func (r *Runtime) ResolveApproval(ctx context.Context, approvalID string, approved bool) (ApprovalResolution, error) {
	status := ApprovalStatusDenied
	if approved {
		status = ApprovalStatusApproved
	}
	approval, changed, err := r.store.ResolvePendingApproval(ctx, approvalID, status)
	if err != nil {
		return ApprovalResolution{}, err
	}
	if !changed {
		return ApprovalResolution{Approval: approval}, nil
	}
	r.audit(ctx, "approval.resolved", approval.ID, "Agent approval resolved.", map[string]any{
		"runId": approval.RunID, "toolName": approval.ToolName, "approved": approved,
	})
	var updatedRun *Run
	var createdMessage *Message
	run, ok, err := r.store.Run(ctx, approval.RunID)
	if err == nil && ok {
		for index := range run.PendingApprovals {
			if run.PendingApprovals[index].ID == approval.ID {
				run.PendingApprovals[index] = approval
			}
		}
		if !approved {
			for index := range run.PendingApprovals {
				item := &run.PendingApprovals[index]
				if item.Status != ApprovalStatusPending {
					continue
				}
				resolved, changed, resolveErr := r.store.ResolvePendingApproval(ctx, item.ID, ApprovalStatusDenied)
				if resolveErr == nil && changed {
					*item = resolved
				}
			}
			for index := range run.ToolCalls {
				call := &run.ToolCalls[index]
				if call.Status == "PENDING_APPROVAL" {
					call.Status = "DENIED"
					call.RequiresUser = false
					finishToolCall(call)
				}
			}
		}
		hasPending := false
		for _, item := range run.PendingApprovals {
			if item.Status == ApprovalStatusPending {
				hasPending = true
				break
			}
		}
		if hasPending {
			_ = r.store.SaveRun(ctx, run)
			updatedRun = &run
			return ApprovalResolution{Approval: approval, Run: updatedRun}, nil
		}
		if resumedRun, message, handled, resumeErr := r.resumeGoogleADK(ctx, run); handled {
			if resumeErr != nil {
				return ApprovalResolution{}, resumeErr
			}
			updatedRun = &resumedRun
			createdMessage = message
			return ApprovalResolution{Approval: approval, Run: updatedRun, Message: createdMessage}, nil
		}
		if approved {
			agent, agentOK, agentErr := r.store.Agent(ctx, run.AgentID)
			if agentErr != nil || !agentOK || agent.Status != AgentStatusEnabled || agent.DeletedAt != nil {
				for index := range run.ToolCalls {
					call := &run.ToolCalls[index]
					if call.Status == "PENDING_APPROVAL" {
						errText := "agent is no longer available"
						call.Status = "FAILED"
						call.Error = &errText
						call.RequiresUser = false
						finishToolCall(call)
					}
				}
			} else {
				for _, item := range run.PendingApprovals {
					if item.Status != ApprovalStatusApproved {
						continue
					}
					for index := range run.ToolCalls {
						call := &run.ToolCalls[index]
						if call.ToolName != item.ToolName || call.Status != "PENDING_APPROVAL" {
							continue
						}
						registered, registeredOK := r.tools.Get(item.ToolName)
						if !registeredOK {
							errText := "tool is no longer registered"
							call.Status = "FAILED"
							call.Error = &errText
						} else {
							output, execErr := executeRegisteredTool(contextWithToolAgent(ctx, agent), registered, item.Input)
							if execErr != nil {
								errText := execErr.Error()
								call.Status = "FAILED"
								call.Error = &errText
							} else {
								call.Status = "SUCCEEDED"
								call.Output = limitToolOutput(output)
							}
						}
						call.RequiresUser = false
						finishToolCall(call)
						break
					}
				}
			}
		}
		if !hasPending {
			completedAt := nowString()
			run.CompletedAt = &completedAt
			run.ToolSummaries = toolSummariesForRun(run)
			summary := approvalResolutionSummary(run, approval, approved)
			if approved {
				run.Status = RunStatusCompleted
				run.ResumeState = "approval_resolved"
				run.Message = "approvals resolved"
				for _, call := range run.ToolCalls {
					if call.Status == "FAILED" {
						run.Status = RunStatusFailed
						run.ErrorCode = "TOOL_EXECUTION_FAILED"
						run.Message = "approved tool execution failed"
						if call.Error != nil {
							run.FailureReason = *call.Error
						}
						break
					}
				}
				agent, agentErr := r.resolveAgent(ctx, run.AgentID)
				if agentErr == nil {
					agent, agentErr = r.prepareAgent(ctx, agent)
				}
				if agentErr == nil {
					history, _ := r.conversationHistory(ctx, run.SessionID, agent.MemoryEnabled)
					question := strings.TrimSpace(run.UserMessage)
					if question == "" {
						question = approval.ToolName
					}
					replyResult, replyErr := r.generateReply(ctx, agent, question, run.ToolSummaries, history)
					if replyErr != nil {
						run.Status = RunStatusFailed
						run.Message = replyErr.Error()
						run.FailureReason = replyErr.Error()
						replyResult = openAIChatResult{Reply: localReply(question, run.ToolSummaries, replyErr)}
					}
					summary = replyResult.Reply
					if message, msgErr := r.store.AddMessage(ctx, run.SessionID, "assistant", summary, replyResult.ReasoningContent); msgErr == nil {
						run.FinalMessageID = message.ID
						createdMessage = &message
					}
					_ = r.store.SaveRun(ctx, run)
					updatedRun = &run
					return ApprovalResolution{Approval: approval, Run: updatedRun, Message: createdMessage}, nil
				}
				run.Status = RunStatusFailed
				run.ErrorCode = "AGENT_UNAVAILABLE"
				run.FailureReason = agentErr.Error()
				run.Message = agentErr.Error()
				summary = localReply(run.UserMessage, run.ToolSummaries, agentErr)
			} else {
				run.Status = RunStatusDenied
				run.ResumeState = "approval_denied"
				run.Message = "approval denied"
			}
			if message, msgErr := r.store.AddMessage(ctx, run.SessionID, "assistant", summary, ""); msgErr == nil {
				run.FinalMessageID = message.ID
				createdMessage = &message
			}
			_ = r.store.SaveRun(ctx, run)
		}
		updatedRun = &run
	}
	return ApprovalResolution{Approval: approval, Run: updatedRun, Message: createdMessage}, nil
}

func (r *Runtime) conversationHistory(ctx context.Context, sessionID string, enabled bool) ([]openAIChatMessage, error) {
	if !enabled || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	messages, err := r.store.Messages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return recentOpenAIMessages(messages, 12, 12000), nil
}

func (r *Runtime) prepareAgent(ctx context.Context, agent Agent) (Agent, error) {
	for _, id := range agent.Skills {
		if _, ok, err := r.skills.Get(ctx, id); err != nil {
			return Agent{}, err
		} else if !ok {
			return Agent{}, fmt.Errorf("skill not found: %s", strings.TrimSpace(id))
		}
	}
	if agent.MemoryEnabled {
		memoryPrompt, err := r.agentMemoryPrompt(ctx, agent.ID)
		if err != nil {
			return Agent{}, err
		}
		if memoryPrompt != "" {
			agent.Instruction = strings.TrimSpace(agent.Instruction) + "\n\nJFTrade memory:\n" + memoryPrompt
		}
	}
	return agent, nil
}

func (r *Runtime) agentMemoryPrompt(ctx context.Context, agentID string) (string, error) {
	if r == nil || r.store == nil {
		return "", nil
	}
	entries, err := r.store.ListMemory(ctx, agentID)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	lines := make([]string, 0, len(entries))
	remaining := 4000
	for _, entry := range entries {
		line := fmt.Sprintf("- [%s] %s: %s", entry.Scope, entry.Key, strings.TrimSpace(entry.Value))
		if len([]rune(line)) > remaining {
			line = string([]rune(line)[:remaining])
		}
		lines = append(lines, line)
		remaining -= len([]rune(line))
		if remaining <= 0 {
			break
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (r *Runtime) startRun(ctx context.Context, sessionID string, agent Agent, text string) (Run, context.Context, func(), error) {
	now := nowString()
	timeout := r.runtimeLimits().RunTimeout
	run := Run{
		ID: "run-" + uuid.NewString(), SessionID: sessionID, AgentID: agent.ID, ProviderID: strings.TrimSpace(agent.ProviderID),
		MaxDurationMs: timeout.Milliseconds(),
		Status:        RunStatusRunning, UserMessage: text, Message: "running",
		CreatedAt: now, StartedAt: now, UpdatedAt: now,
		ToolCalls: []ToolCall{}, PendingApprovals: []Approval{},
		Usage: &RunUsage{},
	}
	if err := r.store.SaveRun(ctx, run); err != nil {
		return Run{}, nil, nil, err
	}
	r.audit(ctx, "run.started", run.ID, "Agent run started.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "providerId": run.ProviderID, "status": run.Status, "maxDurationMs": run.MaxDurationMs,
	})
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	r.activeMu.Lock()
	r.activeRuns[run.ID] = cancel
	r.activeMu.Unlock()
	finish := func() {
		cancel()
		r.activeMu.Lock()
		delete(r.activeRuns, run.ID)
		r.activeMu.Unlock()
	}
	return run, runCtx, finish, nil
}

func (r *Runtime) CancelRun(ctx context.Context, runID string) (Run, error) {
	r.ReconcileExpiredRuns(ctx)
	run, ok, err := r.store.Run(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	if !ok {
		return Run{}, fmt.Errorf("run not found")
	}
	if run.Status != RunStatusRunning && run.Status != RunStatusPending {
		return run, nil
	}
	r.activeMu.Lock()
	cancel := r.activeRuns[run.ID]
	r.activeMu.Unlock()
	if cancel != nil {
		cancel()
	}
	cancelledAt := nowString()
	run.Status = RunStatusCancelled
	run.CancelledAt = &cancelledAt
	run.CompletedAt = &cancelledAt
	run.Message = "cancelled"
	run.FailureReason = "run was cancelled by user"
	run.ErrorCode = "RUN_CANCELLED"
	for index := range run.PendingApprovals {
		if run.PendingApprovals[index].Status == ApprovalStatusPending {
			resolved, changed, resolveErr := r.store.ResolvePendingApproval(ctx, run.PendingApprovals[index].ID, ApprovalStatusDenied)
			if resolveErr == nil && changed {
				run.PendingApprovals[index] = resolved
			}
		}
	}
	for index := range run.ToolCalls {
		call := &run.ToolCalls[index]
		switch call.Status {
		case "RUNNING", "PENDING", "PENDING_APPROVAL":
			call.Status = "CANCELLED"
			call.RequiresUser = false
			finishToolCall(call)
		}
	}
	finalizeRunUsage(&run)
	if err := r.store.SaveRun(ctx, run); err != nil {
		return Run{}, err
	}
	r.audit(ctx, "run.cancelled", run.ID, "Agent run cancelled.", map[string]any{
		"runId": run.ID, "sessionId": run.SessionID, "agentId": run.AgentID, "status": run.Status,
	})
	return run, nil
}

func (r *Runtime) audit(ctx context.Context, kind string, subjectID string, detail string, metadata map[string]any) {
	if r == nil || r.store == nil {
		return
	}
	_ = r.store.AddAuditEvent(ctx, AuditEvent{
		Kind: kind, SubjectID: subjectID, Detail: detail, Metadata: metadata,
	})
}

func (r *Runtime) RecordAudit(ctx context.Context, kind string, subjectID string, detail string, metadata map[string]any) {
	r.audit(ctx, kind, subjectID, detail, metadata)
}

func recentOpenAIMessages(messages []Message, maxMessages int, maxChars int) []openAIChatMessage {
	if maxMessages <= 0 || maxChars <= 0 || len(messages) == 0 {
		return nil
	}
	start := 0
	if len(messages) > maxMessages {
		start = len(messages) - maxMessages
	}
	out := make([]openAIChatMessage, 0, len(messages)-start)
	remaining := maxChars
	for _, message := range messages[start:] {
		role := "assistant"
		if message.Role == "user" {
			role = "user"
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if role == "assistant" && isIntermediateApprovalMessage(content) {
			continue
		}
		if len([]rune(content)) > remaining {
			content = string([]rune(content)[:remaining])
		}
		out = append(out, openAIChatMessage{Role: role, Content: content})
		remaining -= len([]rune(content))
		if remaining <= 0 {
			break
		}
	}
	return out
}

func isIntermediateApprovalMessage(content string) bool {
	return strings.Contains(content, "等待用户审批") ||
		strings.Contains(content, "请先在 ADK 审批队列")
}

func runStatusForContext(ctx context.Context, err error) string {
	if err == nil {
		return RunStatusCompleted
	}
	if ctx != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return RunStatusTimedOut
		}
		if ctx.Err() == context.Canceled {
			return RunStatusCancelled
		}
	}
	return RunStatusFailed
}

func runErrorCode(status string) string {
	switch status {
	case RunStatusTimedOut:
		return "RUN_TIMED_OUT"
	case RunStatusCancelled:
		return "RUN_CANCELLED"
	default:
		return "MODEL_CALL_FAILED"
	}
}

func runHasRecoverableApprovalContext(run Run) bool {
	for _, approval := range run.PendingApprovals {
		if approval.Status != ApprovalStatusPending {
			continue
		}
		if strings.TrimSpace(approval.FunctionCallID) != "" && strings.TrimSpace(approval.ConfirmationCallID) != "" {
			return true
		}
	}
	return false
}

func runLifecycleAuditKind(status string) string {
	switch status {
	case RunStatusTimedOut:
		return "run.timed_out"
	case RunStatusCancelled:
		return "run.cancelled"
	case RunStatusDenied:
		return "run.denied"
	case RunStatusFailed:
		return "run.failed"
	default:
		return "run.completed"
	}
}

func finalizeRunUsage(run *Run) {
	if run.Usage == nil {
		return
	}
	if run.StartedAt != "" && run.CompletedAt != nil {
		if started, err := time.Parse(time.RFC3339Nano, run.StartedAt); err == nil {
			if completed, err := time.Parse(time.RFC3339Nano, *run.CompletedAt); err == nil {
				run.Usage.DurationMs = completed.Sub(started).Milliseconds()
			}
		}
	}
}

func buildPromptMessages(agent Agent, question string, toolSummaries []string, history []openAIChatMessage) []openAIChatMessage {
	system := strings.TrimSpace(agent.Instruction)
	if system == "" {
		system = defaultAgentInstruction()
	}
	messages := []openAIChatMessage{{Role: "system", Content: system}}
	messages = append(messages, history...)
	prompt := "用户问题：\n" + question
	if len(toolSummaries) > 0 {
		prompt += "\n\nJFTrade 工具输出：\n" + strings.Join(toolSummaries, "\n\n")
	}
	messages = append(messages, openAIChatMessage{Role: "user", Content: prompt})
	return messages
}

func normalizeToolInvocations(invocations []ToolInvocation, agent Agent, registry *ToolRegistry, question string) []ToolInvocation {
	out := make([]ToolInvocation, 0, len(invocations))
	allowed := map[string]struct{}{}
	for _, descriptor := range ToolDescriptorsForAgent(agent, registry) {
		allowed[descriptor.Name] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, invocation := range invocations {
		canonical, ok := registry.CanonicalName(invocation.Name)
		if !ok {
			continue
		}
		if _, ok := allowed[canonical]; !ok {
			continue
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		input := invocation.Input
		if len(input) == 0 {
			input = inferToolInput(canonical, question)
		}
		out = append(out, ToolInvocation{Name: canonical, Input: input})
		seen[canonical] = struct{}{}
		if len(out) >= 5 {
			break
		}
	}
	return out
}

func approvalResolutionSummary(run Run, approval Approval, approved bool) string {
	if !approved {
		return fmt.Sprintf("已拒绝工具调用 `%s`。本次 run 已结束，没有执行该操作。", approval.ToolName)
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("已批准并执行工具调用 `%s`。", approval.ToolName))
	for _, call := range run.ToolCalls {
		if call.ToolName != approval.ToolName {
			continue
		}
		if call.Status == "SUCCEEDED" {
			lines = append(lines, "执行结果：")
			lines = append(lines, summarizeToolOutput(call.ToolName, call.Output))
		}
		if call.Status == "FAILED" && call.Error != nil {
			lines = append(lines, "执行失败："+*call.Error)
		}
	}
	return strings.Join(lines, "\n")
}

func toolSummariesForRun(run Run) []string {
	summaries := make([]string, 0, len(run.ToolCalls))
	for _, call := range run.ToolCalls {
		if call.Status == "SUCCEEDED" {
			summaries = append(summaries, summarizeToolOutput(call.ToolName, call.Output))
		}
		if call.Status == "FAILED" && call.Error != nil {
			summaries = append(summaries, fmt.Sprintf("%s failed: %s", call.ToolName, *call.Error))
		}
		if call.Status == "DENIED" {
			summaries = append(summaries, fmt.Sprintf("%s denied by user", call.ToolName))
		}
	}
	return summaries
}

func optimizationTaskID(calls []ToolCall) string {
	for _, call := range calls {
		if call.ToolName != "strategy.optimize" || call.Status != "SUCCEEDED" {
			continue
		}
		if output, ok := call.Output.(map[string]any); ok {
			if taskID, ok := output["taskId"].(string); ok {
				return strings.TrimSpace(taskID)
			}
		}
	}
	return ""
}

func inferToolInput(name string, text string) map[string]any {
	input := map[string]any{"query": text}
	if name == "http.fetch" {
		fields := strings.Fields(text)
		for _, field := range fields {
			field = strings.Trim(field, "，。,.()[]{}<>\"'")
			if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
				input["url"] = field
				break
			}
		}
	}
	return input
}

func localReply(question string, toolSummaries []string, cause error) string {
	var builder strings.Builder
	builder.WriteString("已完成本地 ADK 分析。")
	if cause != nil {
		builder.WriteString(" 模型调用不可用，已使用本地兜底回复。原因：")
		builder.WriteString(cause.Error())
		builder.WriteString("。")
	}
	if len(toolSummaries) > 0 {
		builder.WriteString("\n\n使用的数据来源：\n")
		for _, summary := range toolSummaries {
			builder.WriteString("- ")
			builder.WriteString(summary)
			builder.WriteString("\n")
		}
	} else {
		builder.WriteString(" 目前没有触发内部工具；你可以询问行情、账户、策略、回测、系统状态，或使用 @toolName 指定工具。")
	}
	if strings.TrimSpace(question) != "" {
		builder.WriteString("\n\n问题摘要：")
		builder.WriteString(strings.TrimSpace(question))
	}
	return builder.String()
}
