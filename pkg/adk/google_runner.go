package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/workflowagent"
	adkmodel "google.golang.org/adk/v2/model"
	adkrunner "google.golang.org/adk/v2/runner"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
	"gorm.io/gorm"
)

const googleADKUserID = "jftrade-user"

var errUserGoalPauseRequested = errors.New("user goal pause requested")

type googleADKExecution struct {
	mu                       sync.Mutex
	runner                   *adkrunner.Runner
	sessionService           adksession.Service
	sessionID                string
	appName                  string
	agent                    Agent
	runID                    string
	runIDByAgentName         map[string]string
	runSnapshotBaseByID      map[string]Run
	descriptors              map[string]ToolDescriptor
	calls                    []ToolCall
	summaries                []string
	replyByRunID             map[string]*strings.Builder
	reasoningByRunID         map[string]*strings.Builder
	bufferedReplyByRunID     map[string]*strings.Builder
	bufferedReasoningByRunID map[string]*strings.Builder
	toolResponseSeenByRunID  map[string]bool
	postToolTextByRunID      map[string]bool
	toolResponseSeqByRunID   map[string]int
	postToolTextSeqByRunID   map[string]int
	reply                    strings.Builder
	reasoning                strings.Builder
	preToolContent           strings.Builder
	preToolReasoning         strings.Builder
	bufferedReply            strings.Builder
	bufferedReasoning        strings.Builder
	onDelta                  func(ChatDelta) error
	sawPartialText           bool
	runBlocking              func(context.Context, *genai.Content) error
	loadRun                  func(context.Context, string) (Run, bool, error)
	persistRunSnapshot       func(Run) (Run, error)
	processedConfirmationIDs map[string]struct{}
}

func (r *Runtime) executeGoogleADK(
	ctx context.Context,
	agent Agent,
	session Session,
	runID string,
	text string,
	onDelta func(ChatDelta) error,
) (toolExecutionContext, []Approval, openAIChatResult, string, string, error) {
	execution, err := r.newGoogleADKExecution(ctx, agent, session, runID, onDelta)
	if err != nil {
		return toolExecutionContext{}, nil, openAIChatResult{}, "", "", err
	}
	if err := execution.run(ctx, genai.NewContentFromText(text, genai.RoleUser)); err != nil {
		preToolContent, preToolReasoning := execution.preToolState()
		return execution.toolContext(), nil, execution.result(), preToolContent, preToolReasoning, err
	}
	approvals, err := execution.pendingApprovals(ctx, r.store)
	if err != nil {
		preToolContent, preToolReasoning := execution.preToolState()
		return execution.toolContext(), nil, execution.result(), preToolContent, preToolReasoning, err
	}
	if len(approvals) > 0 {
		execution.detachDeltaSink()
		r.adkMu.Lock()
		r.adkRuns[runID] = execution
		r.adkMu.Unlock()
	}
	if len(approvals) == 0 {
		if err := r.ensureGoogleADKFinalReply(ctx, agent, session, execution, runID, text); err != nil {
			preToolContent, preToolReasoning := execution.preToolState()
			return execution.toolContext(), nil, execution.result(), preToolContent, preToolReasoning, err
		}
	}
	preToolContent, preToolReasoning := execution.preToolState()
	return execution.toolContext(), approvals, execution.result(), preToolContent, preToolReasoning, nil
}

func (r *Runtime) resumeGoogleADK(ctx context.Context, run Run) (Run, *Message, bool, error) {
	r.adkMu.Lock()
	execution := r.adkRuns[run.ID]
	r.adkMu.Unlock()
	if execution == nil {
		var err error
		execution, err = r.rehydrateGoogleADKExecution(ctx, run)
		if err != nil {
			return run, nil, true, err
		}
		if execution == nil {
			return run, nil, false, nil
		}
	}
	execution.detachDeltaSink()
	parts := approvalResolutionParts(run.PendingApprovals)
	if len(parts) == 0 {
		return run, nil, false, nil
	}
	// Only reset reply buffers when this is a fresh (rehydrated) execution.
	// When the execution carries over from a previous approval round we
	// keep accumulating text so the final assistant message contains the
	// full conversation.
	rehydrated := execution.reply.Len() == 0 && execution.reasoning.Len() == 0
	if rehydrated {
		execution.reply.Reset()
		execution.reasoning.Reset()
	}
	if err := r.maybeAutoCompactSessionDuringWorkflow(ctx, Session{ID: run.SessionID, AgentID: run.AgentID}, execution.agent, run.UserMessage, nil); err != nil {
		return run, nil, true, err
	}
	if err := execution.run(ctx, genai.NewContentFromParts(parts, genai.RoleUser)); err != nil {
		return run, nil, true, err
	}

	// The resumed execution may have produced *new* tool calls that require
	// confirmation.  Seed the execution's dedup map with every
	// ConfirmationCallID that already has a corresponding approval so
	// pendingApprovals only creates fresh records.
	if execution.processedConfirmationIDs == nil {
		execution.processedConfirmationIDs = make(map[string]struct{})
	}
	for _, approval := range run.PendingApprovals {
		if approval.ConfirmationCallID != "" {
			execution.processedConfirmationIDs[approval.ConfirmationCallID] = struct{}{}
		}
	}

	newApprovals, approvalErr := execution.pendingApprovals(ctx, r.store)
	if approvalErr != nil {
		return run, nil, true, approvalErr
	}
	if len(newApprovals) > 0 {
		// Save the assistant message accumulated so far so the timeline
		// renders the LLM's analysis between approval rounds.
		result := execution.resultForRun(run.ID)
		if strings.TrimSpace(result.Reply) != "" || strings.TrimSpace(result.ReasoningContent) != "" {
			message, msgErr := r.ensureAssistantMessage(ctx, Session{ID: run.SessionID, AgentID: run.AgentID}, run, result)
			if msgErr == nil {
				run.FinalMessageID = message.ID
			}
		}
		run = hydrateResumedRunWithApprovals(run, execution, newApprovals)
		run.Status = RunStatusPending
		run.ResumeState = "waiting_approval"
		run.Message = "等待用户审批后继续执行。"
		if err := r.store.SaveRun(ctx, run); err != nil {
			return run, nil, true, err
		}
		r.audit(ctx, "run.awaiting_approval", run.ID, "Agent run is waiting for approval.", map[string]any{
			"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "pendingApprovals": len(run.PendingApprovals),
		})
		return run, nil, true, nil
	}

	denied := runHasDeniedApproval(run.PendingApprovals)
	if !denied {
		if err := r.ensureGoogleADKFinalReply(ctx, execution.agent, Session{ID: run.SessionID, AgentID: run.AgentID}, execution, run.ID, run.UserMessage); err != nil {
			run, _ = hydrateResumedRun(run, execution)
			run = markFailedChatRun(ctx, run, err)
			if persistErr := r.persistRunTerminalState(ctx, run); persistErr != nil {
				return run, nil, true, persistErr
			}
			r.adkMu.Lock()
			delete(r.adkRuns, run.ID)
			r.adkMu.Unlock()
			return run, nil, true, nil
		}
	}
	run, denied = hydrateResumedRun(run, execution)
	result := execution.resultForRun(run.ID)
	if denied {
		result.Reply = approvalResolutionSummary(run, run.PendingApprovals[0], false)
		result.ReasoningContent = ""
	} else if result.Reply == "" {
		result.Reply = approvalResolutionSummary(run, run.PendingApprovals[0], !denied)
	}
	run.CompletedAt = new(nowString())
	message, err := r.persistResumedRunResult(ctx, run, result)
	if err != nil {
		return run, nil, true, err
	}
	r.auditResumedRun(ctx, run)
	r.adkMu.Lock()
	delete(r.adkRuns, run.ID)
	r.adkMu.Unlock()
	return run, message, true, nil
}

func (r *Runtime) rehydrateGoogleADKExecution(ctx context.Context, run Run) (*googleADKExecution, error) {
	agentDefinition, err := r.resolveAgent(ctx, run.AgentID)
	if err != nil {
		return nil, err
	}
	agentDefinition, err = r.prepareAgent(ctx, agentDefinition)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(run.ProviderID) != "" {
		agentDefinition.ProviderID = strings.TrimSpace(run.ProviderID)
	}
	if strings.TrimSpace(run.Model) != "" {
		agentDefinition.Model = strings.TrimSpace(run.Model)
	}
	if validPermissionMode(run.PermissionMode) {
		agentDefinition.PermissionMode = normalizePermissionMode(run.PermissionMode)
	}
	productSession, ok, err := r.store.Session(ctx, run.SessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	execution, err := r.newGoogleADKExecution(ctx, agentDefinition, productSession, run.ID, nil)
	if err != nil {
		return nil, err
	}
	execution.calls = append([]ToolCall(nil), run.ToolCalls...)
	execution.summaries = append([]string(nil), run.ToolSummaries...)
	parts := make([]*genai.Part, 0, len(run.PendingApprovals))
	for _, approval := range run.PendingApprovals {
		if approval.ConfirmationCallID == "" || approval.FunctionCallID == "" {
			continue
		}
		original := &genai.FunctionCall{
			ID: approval.FunctionCallID, Name: approval.ToolName, Args: approval.Input,
		}
		parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{
			ID: approval.ConfirmationCallID, Name: toolconfirmation.FunctionCallName,
			Args: map[string]any{
				"originalFunctionCall": original,
				"toolConfirmation": toolconfirmation.ToolConfirmation{
					Hint: "请批准或拒绝 JFTrade 工具调用 " + approval.ToolName,
				},
			},
		}})
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return execution, nil
}

func approvalResolutionParts(approvals []Approval) []*genai.Part {
	parts := make([]*genai.Part, 0, len(approvals))
	for _, approval := range approvals {
		if approval.ConfirmationCallID == "" {
			continue
		}
		parts = append(parts, &genai.Part{FunctionResponse: &genai.FunctionResponse{
			Name: toolconfirmation.FunctionCallName,
			ID:   approval.ConfirmationCallID,
			Response: map[string]any{
				"confirmed": approval.Status == ApprovalStatusApproved,
			},
		}})
	}
	return parts
}

func hydrateResumedRun(run Run, execution *googleADKExecution) (Run, bool) {
	toolContext := execution.toolContextForRun(run.ID)
	run.ToolCalls = toolContext.calls
	run.ToolSummaries = toolContext.summaries
	run.PreToolContent, run.PreToolReasoning = execution.preToolState()
	run.OptimizationTaskID = optimizationTaskID(toolContext.calls)
	run.ResumeState = "adk_confirmation_resolved"
	run.Status = RunStatusCompleted
	run.Message = "completed"

	denied := runHasDeniedApproval(run.PendingApprovals)
	if denied {
		run = markDeniedResumedRun(run)
	} else {
		run = markFailedResumedRunIfNeeded(run)
	}
	return run, denied
}

// hydrateResumedRunWithApprovals updates the run with the execution state
// while keeping the run in a pending-approval state so further confirmation
// rounds can be processed.
func hydrateResumedRunWithApprovals(run Run, execution *googleADKExecution, newApprovals []Approval) Run {
	toolContext := execution.toolContextForRun(run.ID)
	run.ToolCalls = toolContext.calls
	run.ToolSummaries = toolContext.summaries
	run.PreToolContent, run.PreToolReasoning = execution.preToolState()
	run.OptimizationTaskID = optimizationTaskID(toolContext.calls)
	run.PendingApprovals = newApprovals
	run.ResumeState = "waiting_approval"
	return run
}

func runHasDeniedApproval(approvals []Approval) bool {
	for _, approval := range approvals {
		if approval.Status == ApprovalStatusDenied {
			return true
		}
	}
	return false
}

func markDeniedResumedRun(run Run) Run {
	run.Status = RunStatusDenied
	run.ResumeState = "approval_denied"
	run.Message = "approval denied"
	run.ErrorCode = ""
	run.FailureReason = ""
	for index := range run.ToolCalls {
		call := &run.ToolCalls[index]
		if call.Status == "FAILED" && call.Error != nil &&
			strings.Contains(*call.Error, adktool.ErrConfirmationRejected.Error()) {
			call.Status = "DENIED"
			call.Error = nil
			call.RequiresUser = false
		}
	}
	return run
}

func markFailedResumedRunIfNeeded(run Run) Run {
	run.FailureReason = ""
	run.ErrorCode = ""
	run.Degraded = firstToolCallFailure(&run) != ""
	return run
}

func classifyToolExecutionError(err error) (string, string) {
	if err == nil {
		return "SUCCEEDED", ""
	}
	return classifyToolErrorText(err.Error())
}

func classifyToolErrorText(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "context deadline exceeded"):
		return "TIMED_OUT", prefixedToolError(trimmed, "tool execution timed out")
	case strings.Contains(lower, "context canceled"):
		return "CANCELLED", prefixedToolError(trimmed, "tool execution cancelled")
	default:
		return "FAILED", trimmed
	}
}

func prefixedToolError(text string, prefix string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return prefix
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, prefix) {
		return trimmed
	}
	return prefix + ": " + trimmed
}

func firstToolCallByStatus(calls []ToolCall, statuses ...string) *ToolCall {
	if len(calls) == 0 || len(statuses) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		allowed[strings.ToUpper(strings.TrimSpace(status))] = struct{}{}
	}
	for index := range calls {
		if _, ok := allowed[strings.ToUpper(strings.TrimSpace(calls[index].Status))]; ok {
			return &calls[index]
		}
	}
	return nil
}

func toolCallFailureMessage(call *ToolCall) string {
	if call == nil {
		return ""
	}
	if call.Error != nil && strings.TrimSpace(*call.Error) != "" {
		return strings.TrimSpace(*call.Error)
	}
	switch strings.ToUpper(strings.TrimSpace(call.Status)) {
	case "TIMED_OUT":
		return "tool execution timed out"
	case "CANCELLED":
		return "tool execution cancelled"
	default:
		return "tool execution failed"
	}
}

func firstToolCallFailure(run *Run) string {
	if run == nil {
		return ""
	}
	call := firstToolCallByStatus(run.ToolCalls, "TIMED_OUT", "FAILED", "CANCELLED")
	return toolCallFailureMessage(call)
}

func (r *Runtime) persistResumedRunResult(ctx context.Context, run Run, result openAIChatResult) (*Message, error) {
	message, err := r.ensureAssistantMessage(ctx, Session{ID: run.SessionID, AgentID: run.AgentID}, run, result)
	if err != nil {
		return nil, err
	}
	run.FinalMessageID = message.ID
	if err := r.store.SaveRun(ctx, run); err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *Runtime) auditResumedRun(ctx context.Context, run Run) {
	r.audit(ctx, "run.resumed", run.ID, "Agent run resumed after approval resolution.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "resumeState": run.ResumeState,
	})
	r.audit(ctx, runLifecycleAuditKind(run.Status), run.ID, "Agent run reached a terminal state after approval resolution.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "resumeState": run.ResumeState, "failureReason": run.FailureReason,
	})
}

func (r *Runtime) newGoogleADKExecution(
	ctx context.Context,
	definition Agent,
	productSession Session,
	runID string,
	onDelta func(ChatDelta) error,
) (*googleADKExecution, error) {
	llm, err := r.googleADKModelForAgent(ctx, definition)
	if err != nil {
		return nil, err
	}

	execution := &googleADKExecution{
		sessionID: productSession.ID,
		appName:   googleADKAppName(definition.ID),
		agent:     definition,
		runID:     runID,
		runIDByAgentName: map[string]string{
			googleADKAgentName(definition.ID): runID,
		},
		runSnapshotBaseByID: map[string]Run{
			runID: {
				ID: runID, SessionID: productSession.ID, AgentID: definition.ID,
				Status: RunStatusRunning, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{},
			},
		},
		descriptors:              toolDescriptorIndex(ToolDescriptorsForAgent(definition, r.tools)),
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
		onDelta:                  onDelta,
		loadRun: func(ctx context.Context, runID string) (Run, bool, error) {
			if r.store == nil {
				return Run{}, false, nil
			}
			return r.store.Run(ctx, runID)
		},
		persistRunSnapshot: func(snapshot Run) (Run, error) {
			return r.persistRunActivitySnapshot(context.Background(), snapshot)
		},
	}
	adkAgent, err := r.newGoogleADKLLMAgent(ctx, googleADKAgentName(definition.ID), definition.Name, definition, llm, execution)
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK agent: %w", err)
	}
	return r.attachGoogleADKRunner(ctx, execution, productSession, adkAgent)
}

func (r *Runtime) newGoogleADKWorkflowExecution(
	ctx context.Context,
	definition Agent,
	productSession Session,
	parent Run,
	childRuns []Run,
	steps []workflowStep,
	mode string,
	options RunOptions,
	onDelta func(ChatDelta) error,
) (*googleADKExecution, error) {
	rootName := googleADKWorkflowRootName(parent.ID)
	execution := &googleADKExecution{
		sessionID: productSession.ID,
		appName:   googleADKAppName(definition.ID),
		agent:     definition,
		runID:     parent.ID,
		runIDByAgentName: map[string]string{
			rootName: parent.ID,
		},
		runSnapshotBaseByID: map[string]Run{
			parent.ID: parent,
		},
		descriptors:              toolDescriptorIndex(ToolDescriptorsForAgent(definition, r.tools)),
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
		onDelta:                  onDelta,
		loadRun: func(ctx context.Context, runID string) (Run, bool, error) {
			if r.store == nil {
				return Run{}, false, nil
			}
			return r.store.Run(ctx, runID)
		},
		persistRunSnapshot: func(snapshot Run) (Run, error) {
			return r.persistRunActivitySnapshot(context.Background(), snapshot)
		},
	}
	childNodes := make([]adkworkflow.Node, 0, len(childRuns))
	for index, child := range childRuns {
		if index >= len(steps) {
			break
		}
		name := googleADKWorkflowChildName(parent.ID, index)
		execution.runIDByAgentName[name] = child.ID
		execution.runSnapshotBaseByID[child.ID] = child
		childDefinition := definition
		childDefinition = workflowChildAgentForStep(childDefinition, steps[index])
		childDefinition.WorkMode = WorkModeChat
		childDefinition.Instruction = workflowChildInstruction(definition.Instruction, workflowChildInstructionTask(steps[index]))
		childLLM, err := r.googleADKModelForAgent(ctx, childDefinition)
		if err != nil {
			return nil, err
		}
		childAgent, err := r.newGoogleADKLLMAgent(ctx, name, steps[index].Title, childDefinition, childLLM, execution)
		if err != nil {
			return nil, err
		}
		childNode, err := adkworkflow.NewAgentNode(childAgent, adkworkflow.NodeConfig{})
		if err != nil {
			return nil, fmt.Errorf("create GO-ADK workflow node: %w", err)
		}
		childNodes = append(childNodes, childNode)
	}
	if len(childNodes) == 0 {
		return nil, fmt.Errorf("workflow requires at least one sub-agent")
	}
	edges := compileGoogleADKWorkflowEdges(steps, childNodes)
	root, err := workflowagent.New(workflowagent.Config{
		Name:        rootName,
		Description: definition.Name + " task workflow",
		Edges:       edges,
	})
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK workflow agent: %w", err)
	}
	return r.attachGoogleADKRunner(ctx, execution, productSession, root)
}

func compileGoogleADKWorkflowEdges(steps []workflowStep, nodes []adkworkflow.Node) []adkworkflow.Edge {
	edges := make([]adkworkflow.Edge, 0, len(nodes)*2)
	nodeByStepID := make(map[string]adkworkflow.Node, len(nodes))
	for index, node := range nodes {
		if index >= len(steps) {
			break
		}
		if stepID := strings.TrimSpace(steps[index].DependencyID); stepID != "" {
			nodeByStepID[stepID] = node
		}
	}
	for index, node := range nodes {
		if index >= len(steps) {
			break
		}
		dependencies := compileGoogleADKWorkflowDependencies(steps[index], nodeByStepID)
		switch len(dependencies) {
		case 0:
			edges = append(edges, adkworkflow.Edge{From: adkworkflow.Start, To: node})
		case 1:
			edges = append(edges, adkworkflow.Edge{From: dependencies[0], To: node})
		default:
			join := adkworkflow.NewJoinNode(fmt.Sprintf("%s_join", node.Name()))
			for _, dep := range dependencies {
				edges = append(edges, adkworkflow.Edge{From: dep, To: join})
			}
			edges = append(edges, adkworkflow.Edge{From: join, To: node})
		}
	}
	if len(edges) == 0 && len(nodes) > 0 {
		edges = append(edges, adkworkflow.Edge{From: adkworkflow.Start, To: nodes[0]})
	}
	return edges
}

func compileGoogleADKWorkflowDependencies(step workflowStep, nodeByStepID map[string]adkworkflow.Node) []adkworkflow.Node {
	if len(step.DependsOn) == 0 {
		return nil
	}
	dependencies := make([]adkworkflow.Node, 0, len(step.DependsOn))
	seen := make(map[string]struct{}, len(step.DependsOn))
	for _, dependencyID := range step.DependsOn {
		dependencyID = strings.TrimSpace(dependencyID)
		if dependencyID == "" {
			continue
		}
		if _, ok := seen[dependencyID]; ok {
			continue
		}
		node, ok := nodeByStepID[dependencyID]
		if !ok || node == nil {
			continue
		}
		seen[dependencyID] = struct{}{}
		dependencies = append(dependencies, node)
	}
	return dependencies
}

func (r *Runtime) runGoogleADKWorkflowChildFinalSynthesis(
	ctx context.Context,
	definition Agent,
	productSession Session,
	execution *googleADKExecution,
	child Run,
) error {
	return r.runGoogleADKFinalSynthesis(ctx, definition, productSession, execution, child.ID, child.UserMessage)
}

func (r *Runtime) ensureGoogleADKFinalReply(
	ctx context.Context,
	definition Agent,
	productSession Session,
	execution *googleADKExecution,
	runID string,
	task string,
) error {
	if !execution.runNeedsFinalSynthesis(runID) {
		return nil
	}
	if err := r.runGoogleADKFinalSynthesis(ctx, definition, productSession, execution, runID, task); err != nil {
		return err
	}
	if execution.runNeedsFinalSynthesis(runID) || !execution.runHasPostToolText(runID) {
		return errADKMissingFinalReply()
	}
	return nil
}

func (r *Runtime) runGoogleADKFinalSynthesis(
	ctx context.Context,
	definition Agent,
	productSession Session,
	execution *googleADKExecution,
	runID string,
	task string,
) error {
	noToolDefinition := definition
	noToolDefinition.WorkMode = WorkModeChat
	noToolDefinition.Tools = nil
	noToolDefinition.Skills = nil
	if err := r.maybeAutoCompactSessionDuringWorkflow(ctx, productSession, noToolDefinition, task, nil); err != nil {
		return err
	}
	llm, err := r.googleADKModelForAgent(ctx, noToolDefinition)
	if err != nil {
		return err
	}
	agentName := execution.agentNameForRunID(runID)
	if agentName == "" {
		return fmt.Errorf("GO-ADK agent mapping missing for run %s", runID)
	}
	instruction := workflowFinalSynthesisInstruction(definition.Instruction, task)
	synthesisAgent, err := llmagent.New(llmagent.Config{
		Name:        agentName,
		Description: "Finalize ADK run response",
		InstructionProvider: func(ctx adkagent.ReadonlyContext) (string, error) {
			if r.contextManager == nil || ctx == nil {
				return instruction, nil
			}
			suffix, suffixErr := r.contextManager.InstructionSuffix(ctx, ctx.SessionID())
			if suffixErr != nil || strings.TrimSpace(suffix) == "" {
				return instruction, nil
			}
			return instruction + "\n\n" + suffix, nil
		},
		Model:           llm,
		IncludeContents: llmagent.IncludeContentsDefault,
	})
	if err != nil {
		return fmt.Errorf("create GO-ADK final synthesis agent: %w", err)
	}
	synthesisRunner, err := adkrunner.New(adkrunner.Config{
		AppName: execution.appName, Agent: synthesisAgent, SessionService: execution.sessionService,
	})
	if err != nil {
		return fmt.Errorf("create GO-ADK final synthesis runner: %w", err)
	}
	execution.markToolResponseSeenForRun(runID)
	for event, runErr := range synthesisRunner.Run(ctx, googleADKUserID, execution.sessionID, nil, adkagent.RunConfig{
		StreamingMode: adkagent.StreamingModeSSE,
	}) {
		if runErr != nil {
			return runErr
		}
		if err := execution.consumeEvent(event); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) googleADKModelForAgent(ctx context.Context, definition Agent) (adkmodel.LLM, error) {
	definition, err := r.resolveAgentProvider(ctx, definition)
	if err != nil {
		return nil, err
	}
	provider, err := r.effectiveProvider(ctx, definition.ProviderID)
	if err != nil {
		return nil, err
	}
	apiKey, hasKey, err := r.store.ProviderAPIKey(provider.ID)
	if err != nil {
		return nil, err
	}
	if !hasKey || strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("agent provider API key is not configured")
	}
	return newOpenAICompatibleADKModel(provider, apiKey, definition.Model), nil
}

func (r *Runtime) newGoogleADKLLMAgent(
	ctx context.Context,
	name string,
	description string,
	definition Agent,
	llm adkmodel.LLM,
	execution *googleADKExecution,
) (adkagent.Agent, error) {
	toolsets, err := r.googleADKToolsets(ctx, definition)
	if err != nil {
		return nil, err
	}
	return llmagent.New(llmagent.Config{
		Name:        name,
		Description: description,
		InstructionProvider: func(ctx adkagent.ReadonlyContext) (string, error) {
			instruction := strings.TrimSpace(definition.Instruction)
			if r.contextManager == nil || ctx == nil {
				return instruction, nil
			}
			suffix, err := r.contextManager.InstructionSuffix(ctx, ctx.SessionID())
			if err != nil || strings.TrimSpace(suffix) == "" {
				return instruction, nil
			}
			if instruction == "" {
				return suffix, nil
			}
			return instruction + "\n\n" + suffix, nil
		},
		Model: llm,
		BeforeToolCallbacks: []llmagent.BeforeToolCallback{
			execution.beforeToolCallback,
		},
		AfterToolCallbacks: []llmagent.AfterToolCallback{
			execution.afterToolCallback,
		},
		Toolsets:        toolsets,
		IncludeContents: llmagent.IncludeContentsDefault,
	})
}

func (r *Runtime) attachGoogleADKRunner(
	ctx context.Context,
	execution *googleADKExecution,
	productSession Session,
	adkAgent adkagent.Agent,
) (*googleADKExecution, error) {
	service := r.sessionService
	if service == nil {
		service = adksession.InMemoryService()
	}
	if _, err := service.Get(ctx, &adksession.GetRequest{
		AppName: execution.appName, UserID: googleADKUserID, SessionID: productSession.ID,
	}); err != nil {
		lowerErr := strings.ToLower(err.Error())
		if !errors.Is(err, gorm.ErrRecordNotFound) && !strings.Contains(lowerErr, "record not found") && !strings.Contains(lowerErr, "not found") {
			return nil, fmt.Errorf("get GO-ADK session: %w", err)
		}
		if _, createErr := service.Create(ctx, &adksession.CreateRequest{
			AppName: execution.appName, UserID: googleADKUserID, SessionID: productSession.ID,
		}); createErr != nil {
			return nil, fmt.Errorf("create GO-ADK session: %w", createErr)
		}
	}
	adkRunner, err := adkrunner.New(adkrunner.Config{
		AppName: execution.appName, Agent: adkAgent, SessionService: service,
	})
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK runner: %w", err)
	}
	execution.runner = adkRunner
	execution.sessionService = service
	if r.rawSessionService != nil {
		execution.sessionService = r.rawSessionService
	}
	return execution, nil
}

func (e *googleADKExecution) beforeToolCallback(ctx adkagent.Context, tool adktool.Tool, args map[string]any) (map[string]any, error) {
	if e.shouldInterruptForUserGoalPause(e.runIDForAgentName(ctx.AgentName())) {
		return nil, errUserGoalPauseRequested
	}
	descriptor, ok := e.descriptorForTool(tool)
	if !ok {
		return nil, nil
	}
	call := e.ensureCallForAgent(ctx.FunctionCallID(), descriptor, args, ctx.AgentName())
	e.emitToolProgress(call.ID, tool.Name())
	if !ToolAllowedInMode(descriptor, e.agent.PermissionMode) {
		return nil, fmt.Errorf("tool is not allowed in permission mode %s", e.agent.PermissionMode)
	}
	return nil, nil
}

func (e *googleADKExecution) shouldInterruptForUserGoalPause(runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" || runID != e.runID || e.loadRun == nil {
		return false
	}
	run, ok, err := e.loadRun(context.Background(), runID)
	if err != nil || !ok {
		return false
	}
	return userPauseRequestedGoalParent(run) || userPausedGoalParent(run)
}

func (e *googleADKExecution) afterToolCallback(
	ctx adkagent.Context,
	tool adktool.Tool,
	args map[string]any,
	result map[string]any,
	err error,
) (map[string]any, error) {
	descriptor, ok := e.descriptorForTool(tool)
	if !ok {
		return nil, nil
	}
	call := e.ensureCallForAgent(ctx.FunctionCallID(), descriptor, args, ctx.AgentName())
	switch {
	case err == nil:
		if structuredErr, ok := structuredToolError(result); ok {
			e.finishCall(call.ID, nil, errors.New(structuredErr))
			// Return the result with the error so the ADK includes it in the
			// function response content.  This lets the LLM see the failure and
			// decide whether to retry, use a different tool or report to the user.
			return result, nil
		}
		e.finishCall(call.ID, result, nil)
		return result, nil
	case errors.Is(err, adktool.ErrConfirmationRequired):
		// ADK will emit a tool confirmation function response that transitions the
		// tracked call into PENDING_APPROVAL; keep the call open until then.
	case errors.Is(err, adktool.ErrConfirmationRejected):
		e.finishCall(call.ID, nil, err)
	default:
		e.finishCall(call.ID, result, err)
	}
	return nil, nil
}

func (e *googleADKExecution) descriptorForTool(tool adktool.Tool) (ToolDescriptor, bool) {
	if descriptor, ok := descriptorFromADKTool(tool); ok {
		return descriptor, true
	}
	if tool == nil || len(e.descriptors) == 0 {
		return ToolDescriptor{}, false
	}
	descriptor, ok := e.descriptors[tool.Name()]
	return descriptor, ok
}

func (e *googleADKExecution) run(ctx context.Context, content *genai.Content) error {
	runBlocking := e.runBlocking
	if runBlocking == nil {
		runBlocking = e.runBlockingWithRunner
	}
	done := make(chan error, 1)
	go func() {
		done <- runBlocking(ctx, content)
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *googleADKExecution) runBlockingWithRunner(ctx context.Context, content *genai.Content) error {
	for event, err := range e.runner.Run(ctx, googleADKUserID, e.sessionID, content, adkagent.RunConfig{
		StreamingMode: adkagent.StreamingModeSSE,
	}) {
		if err != nil {
			return err
		}
		if err := e.consumeEvent(event); err != nil {
			return err
		}
	}
	return nil
}

func (e *googleADKExecution) consumeEvent(event *adksession.Event) error {
	if event == nil || event.Content == nil {
		if event != nil && !event.Partial {
			e.sawPartialText = false
		}
		return nil
	}
	emitText := true
	if event.Partial {
		e.sawPartialText = e.sawPartialText || contentHasText(event.Content)
	} else if e.sawPartialText {
		emitText = false
	}
	for _, part := range event.Content.Parts {
		if part.FunctionCall != nil {
			if part.FunctionCall.Name == toolconfirmation.FunctionCallName {
				continue
			}
			descriptor := ToolDescriptor{Name: part.FunctionCall.Name}
			e.ensureCallForAgent(part.FunctionCall.ID, descriptor, part.FunctionCall.Args, event.Author)
		}
		if part.FunctionResponse != nil {
			e.consumeFunctionResponse(part.FunctionResponse)
		}
		if emitText && part.Text != "" {
			reply, reasoning := visibleTextFromParts([]*genai.Part{part})
			if err := e.appendVisibleTextForRun(e.runIDForAgentName(event.Author), reply, reasoning); err != nil {
				return err
			}
		}
	}
	if !event.Partial {
		e.sawPartialText = false
	}
	if err := e.flushBufferedTextIfReady(); err != nil {
		return err
	}
	return nil
}

func contentHasText(content *genai.Content) bool {
	if content == nil {
		return false
	}
	for _, part := range content.Parts {
		if part != nil && part.Text != "" {
			return true
		}
	}
	return false
}

func (e *googleADKExecution) ensureCall(functionCallID string, descriptor ToolDescriptor, input map[string]any) *ToolCall {
	return e.ensureCallForRun(functionCallID, descriptor, input, e.runID)
}

func (e *googleADKExecution) ensureCallForAgent(functionCallID string, descriptor ToolDescriptor, input map[string]any, agentName string) *ToolCall {
	return e.ensureCallForRun(functionCallID, descriptor, input, e.runIDForAgentName(agentName))
}

func (e *googleADKExecution) runIDForAgentName(agentName string) string {
	normalized := strings.TrimSpace(agentName)
	if normalized != "" && e.runIDByAgentName != nil {
		if runID := strings.TrimSpace(e.runIDByAgentName[normalized]); runID != "" {
			return runID
		}
	}
	return e.runID
}

func (e *googleADKExecution) agentNameForRunID(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ""
	}
	for agentName, mappedRunID := range e.runIDByAgentName {
		if strings.TrimSpace(mappedRunID) == runID {
			return agentName
		}
	}
	return ""
}

func (e *googleADKExecution) ensureCallForRun(functionCallID string, descriptor ToolDescriptor, input map[string]any, runID string) *ToolCall {
	e.mu.Lock()
	defer e.mu.Unlock()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	for index := range e.calls {
		if e.calls[index].IdempotencyKey == functionCallID {
			if e.calls[index].ToolName == "" {
				e.calls[index].ToolName = descriptor.Name
			}
			if e.calls[index].Permission == "" {
				e.calls[index].Permission = descriptor.Permission
			}
			return &e.calls[index]
		}
	}
	now := nowString()
	call := ToolCall{
		ID: "tool-" + uuid.NewString(), RunID: runID, ToolName: descriptor.Name,
		Permission: descriptor.Permission, Status: "RUNNING", Input: input,
		IdempotencyKey: functionCallID, CreatedAt: now, StartedAt: now, UpdatedAt: now,
	}
	if len(e.calls) == 0 {
		e.preToolContent.Reset()
		e.preToolReasoning.Reset()
		e.preToolContent.WriteString(strings.TrimSpace(e.reply.String()))
		e.preToolReasoning.WriteString(strings.TrimSpace(e.reasoning.String()))
	}
	e.calls = append(e.calls, call)
	e.emitRunSnapshotLocked()
	return &e.calls[len(e.calls)-1]
}

func (e *googleADKExecution) finishCall(callID string, output any, err error) {
	e.mu.Lock()
	changed := false
	for index := range e.calls {
		call := &e.calls[index]
		if call.ID != callID {
			continue
		}
		if err != nil {
			var errText string
			call.Status, errText = classifyToolExecutionError(err)
			call.Error = &errText
			call.RequiresUser = false
		} else {
			call.Status = "SUCCEEDED"
			call.Output = limitToolOutput(output)
			call.Error = nil
			call.RequiresUser = false
			e.summaries = append(e.summaries, summarizeToolOutput(call.ToolName, output))
		}
		finishToolCall(call)
		changed = true
		break
	}
	e.emitRunSnapshotLocked()
	e.mu.Unlock()
	if changed {
		jftradeErr1 := e.flushBufferedTextIfReady()
		jftradeLogError(jftradeErr1)
	}
}

func (e *googleADKExecution) consumeFunctionResponse(response *genai.FunctionResponse) {
	if response == nil {
		return
	}
	e.mu.Lock()
	changed := false
	for index := range e.calls {
		call := &e.calls[index]
		if call.IdempotencyKey != response.ID {
			continue
		}
		e.markToolResponseSeenLocked(call.RunID)
		if call.Status != "RUNNING" && call.Status != "PENDING" {
			break
		}
		// Detect failure via the new {"success":false} contract or the legacy
		// {"error":"..."} key so that ADK-built-in tools keep working.
		if isToolResponseError(response.Response) {
			errText := toolResponseErrorMessage(response.Response)
			if strings.Contains(errText, adktool.ErrConfirmationRequired.Error()) {
				call.Status = "PENDING_APPROVAL"
				call.RequiresUser = true
				call.UpdatedAt = nowString()
				changed = true
				break
			}
			call.Status, errText = classifyToolErrorText(errText)
			call.Error = &errText
			call.RequiresUser = false
			finishToolCall(call)
			changed = true
		} else {
			call.Status = "SUCCEEDED"
			call.Output = limitToolOutput(response.Response)
			call.Error = nil
			call.RequiresUser = false
			e.summaries = append(e.summaries, summarizeToolOutput(call.ToolName, response.Response))
			finishToolCall(call)
			changed = true
		}
		break
	}
	e.emitRunSnapshotLocked()
	e.mu.Unlock()
	if changed {
		jftradeErr2 := e.flushBufferedTextIfReady()
		jftradeLogError(jftradeErr2)
	}
}

func (e *googleADKExecution) pendingApprovals(ctx context.Context, store *Store) ([]Approval, error) {
	response, err := e.sessionService.Get(ctx, &adksession.GetRequest{
		AppName: e.appName, UserID: googleADKUserID, SessionID: e.sessionID,
	})
	if err != nil {
		return nil, err
	}
	var approvals []Approval
	for event := range response.Session.Events().All() {
		if event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			call := part.FunctionCall
			if call == nil || call.Name != toolconfirmation.FunctionCallName {
				continue
			}
			original, err := toolconfirmation.OriginalCallFrom(call)
			if err != nil {
				return nil, err
			}
			if e.hasApprovalForConfirmation(call.ID) {
				continue
			}
			runID, tracked := e.trackedRunIDForFunctionCall(original.ID)
			if !tracked {
				// The ADK session is shared by the parent and delegated children.
				// Confirmation calls from another execution must not be projected
				// into this run's approval queue.
				continue
			}
			now := nowString()
			approval := Approval{
				ID: "approval-" + uuid.NewString(), RunID: runID, AgentID: e.agent.ID,
				ToolName: original.Name, Input: original.Args, Status: ApprovalStatusPending,
				Reason:         "GO-ADK HITL 要求用户审批该工具调用。",
				FunctionCallID: original.ID, ConfirmationCallID: call.ID,
				CreatedAt: now, UpdatedAt: now,
			}
			saved, created, err := store.SaveApprovalIfConfirmationAbsent(ctx, approval)
			if err != nil {
				return nil, err
			}
			e.markConfirmationProcessed(call.ID)
			if !created {
				// A prior execution or recovery pass already owns this exact ADK
				// confirmation. Never create a second actionable approval.
				_ = saved
				continue
			}
			e.markCallPending(original.ID)
			approvals = append(approvals, saved)
		}
	}
	return approvals, nil
}

func (e *googleADKExecution) hasApprovalForConfirmation(id string) bool {
	if id == "" {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.processedConfirmationIDs != nil {
		_, ok := e.processedConfirmationIDs[id]
		return ok
	}
	return false
}

func (e *googleADKExecution) markConfirmationProcessed(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.processedConfirmationIDs == nil {
		e.processedConfirmationIDs = make(map[string]struct{})
	}
	e.processedConfirmationIDs[id] = struct{}{}
}

func (e *googleADKExecution) markCallPending(functionCallID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for index := range e.calls {
		if e.calls[index].IdempotencyKey == functionCallID {
			e.calls[index].Status = "PENDING_APPROVAL"
			e.calls[index].RequiresUser = true
			e.calls[index].UpdatedAt = nowString()
		}
	}
	e.emitRunSnapshotLocked()
}

func (e *googleADKExecution) toolContext() toolExecutionContext {
	return e.toolContextForRun("")
}

func (e *googleADKExecution) toolContextForRun(runID string) toolExecutionContext {
	e.mu.Lock()
	defer e.mu.Unlock()
	runID = strings.TrimSpace(runID)
	calls := make([]ToolCall, 0, len(e.calls))
	summaries := make([]string, 0, len(e.summaries))
	for _, call := range e.calls {
		if runID != "" && call.RunID != runID {
			continue
		}
		calls = append(calls, call)
		if summary := summarizeToolCall(call); summary != "" {
			summaries = append(summaries, summary)
		}
	}
	if runID == "" {
		summaries = append([]string(nil), e.summaries...)
	}
	return toolExecutionContext{
		calls: calls, summaries: summaries,
	}
}

func (e *googleADKExecution) result() openAIChatResult {
	return e.resultForRun(e.runID)
}

func (e *googleADKExecution) resultForRun(runID string) openAIChatResult {
	e.ensureTextMaps()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	if runID == e.runID {
		return openAIChatResult{
			Reply: strings.TrimSpace(e.reply.String()), ReasoningContent: strings.TrimSpace(e.reasoning.String()),
		}
	}
	reply := e.replyByRunID[runID]
	reasoning := e.reasoningByRunID[runID]
	var replyText, reasoningText string
	if reply != nil {
		replyText = reply.String()
	}
	if reasoning != nil {
		reasoningText = reasoning.String()
	}
	return openAIChatResult{
		Reply: strings.TrimSpace(replyText), ReasoningContent: strings.TrimSpace(reasoningText),
	}
}

func (e *googleADKExecution) trackedRunIDForFunctionCall(functionCallID string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, call := range e.calls {
		if call.IdempotencyKey == functionCallID && strings.TrimSpace(call.RunID) != "" {
			return call.RunID, true
		}
	}
	return "", false
}

func (e *googleADKExecution) preToolState() (string, string) {
	return strings.TrimSpace(e.preToolContent.String()), strings.TrimSpace(e.preToolReasoning.String())
}

func (e *googleADKExecution) detachDeltaSink() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onDelta = nil
}

func (e *googleADKExecution) emitToolProgress(callID string, toolName string) {
	if e.onDelta == nil {
		return
	}
	jftradeErr4 := e.onDelta(ChatDelta{ToolProgress: projectedToolProgress(toolName)})
	jftradeLogError(jftradeErr4)
}

func (e *googleADKExecution) appendVisibleText(reply string, reasoning string) error {
	return e.appendVisibleTextForRun(e.runID, reply, reasoning)
}

func (e *googleADKExecution) appendVisibleTextForRun(runID string, reply string, reasoning string) error {
	if reply == "" && reasoning == "" {
		return nil
	}
	e.ensureTextMaps()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	if e.activeToolCallCountForRun(runID) > 0 {
		e.builderForRun(e.bufferedReplyByRunID, runID).WriteString(reply)
		e.builderForRun(e.bufferedReasoningByRunID, runID).WriteString(reasoning)
		if runID == e.runID {
			e.bufferedReply.WriteString(reply)
			e.bufferedReasoning.WriteString(reasoning)
		}
		return nil
	}
	e.builderForRun(e.replyByRunID, runID).WriteString(reply)
	e.builderForRun(e.reasoningByRunID, runID).WriteString(reasoning)
	if runID == e.runID {
		e.reply.WriteString(reply)
		e.reasoning.WriteString(reasoning)
	}
	if e.toolResponseSeenForRun(runID) {
		e.markPostToolTextForRun(runID)
	}
	if e.onDelta != nil && runID == e.runID {
		if err := e.onDelta(ChatDelta{Reply: reply, ReasoningContent: reasoning}); err != nil {
			return err
		}
	}
	return nil
}

func (e *googleADKExecution) flushBufferedTextIfReady() error {
	e.ensureTextMaps()
	for runID := range e.bufferedReplyByRunID {
		if err := e.flushBufferedTextForRunIfReady(runID); err != nil {
			return err
		}
	}
	return e.flushBufferedTextForRunIfReady(e.runID)
}

func (e *googleADKExecution) flushBufferedTextForRunIfReady(runID string) error {
	e.ensureTextMaps()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	if e.activeToolCallCountForRun(runID) > 0 {
		return nil
	}
	replyBuffer := e.builderForRun(e.bufferedReplyByRunID, runID)
	reasoningBuffer := e.builderForRun(e.bufferedReasoningByRunID, runID)
	reply := strings.TrimSpace(replyBuffer.String())
	reasoning := strings.TrimSpace(reasoningBuffer.String())
	if reply == "" && reasoning == "" {
		return nil
	}
	replyBuffer.Reset()
	reasoningBuffer.Reset()
	e.builderForRun(e.replyByRunID, runID).WriteString(reply)
	e.builderForRun(e.reasoningByRunID, runID).WriteString(reasoning)
	if runID == e.runID {
		e.bufferedReply.Reset()
		e.bufferedReasoning.Reset()
		e.reply.WriteString(reply)
		e.reasoning.WriteString(reasoning)
	}
	if e.toolResponseSeenForRun(runID) {
		e.markPostToolTextForRun(runID)
	}
	if e.onDelta != nil && runID == e.runID {
		if err := e.onDelta(ChatDelta{Reply: reply, ReasoningContent: reasoning}); err != nil {
			return err
		}
	}
	return nil
}

func (e *googleADKExecution) ensureTextMaps() {
	if e.replyByRunID == nil {
		e.replyByRunID = map[string]*strings.Builder{}
	}
	if e.reasoningByRunID == nil {
		e.reasoningByRunID = map[string]*strings.Builder{}
	}
	if e.bufferedReplyByRunID == nil {
		e.bufferedReplyByRunID = map[string]*strings.Builder{}
	}
	if e.bufferedReasoningByRunID == nil {
		e.bufferedReasoningByRunID = map[string]*strings.Builder{}
	}
	if e.toolResponseSeenByRunID == nil {
		e.toolResponseSeenByRunID = map[string]bool{}
	}
	if e.postToolTextByRunID == nil {
		e.postToolTextByRunID = map[string]bool{}
	}
	if e.toolResponseSeqByRunID == nil {
		e.toolResponseSeqByRunID = map[string]int{}
	}
	if e.postToolTextSeqByRunID == nil {
		e.postToolTextSeqByRunID = map[string]int{}
	}
}

func (e *googleADKExecution) builderForRun(store map[string]*strings.Builder, runID string) *strings.Builder {
	if store == nil {
		return &strings.Builder{}
	}
	builder := store[runID]
	if builder == nil {
		builder = &strings.Builder{}
		store[runID] = builder
	}
	return builder
}

func (e *googleADKExecution) activeToolCallCountForRun(runID string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	runID = strings.TrimSpace(runID)
	count := 0
	for _, call := range e.calls {
		if runID != "" && call.RunID != runID {
			continue
		}
		switch call.Status {
		case "RUNNING", "PENDING":
			count++
		}
	}
	return count
}

func (e *googleADKExecution) markToolResponseSeenLocked(runID string) {
	e.ensureTextMaps()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	e.toolResponseSeenByRunID[runID] = true
	e.toolResponseSeqByRunID[runID]++
}

func (e *googleADKExecution) markToolResponseSeenForRun(runID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.markToolResponseSeenLocked(runID)
}

func (e *googleADKExecution) toolResponseSeenForRun(runID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ensureTextMaps()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	return e.toolResponseSeenByRunID[runID] || e.toolResponseSeqByRunID[runID] > 0
}

func (e *googleADKExecution) markPostToolTextForRun(runID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ensureTextMaps()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	e.postToolTextByRunID[runID] = true
	e.postToolTextSeqByRunID[runID] = e.toolResponseSeqByRunID[runID]
}

func (e *googleADKExecution) emitRunSnapshotLocked() {
	for _, runID := range e.snapshotRunIDsLocked() {
		snapshot := e.runSnapshotLocked(runID, false)
		if e.persistRunSnapshot != nil {
			persisted := e.runSnapshotLocked(runID, true)
			sanitized := persisted
			if sanitized.Status == RunStatusRunning || sanitized.Status == RunStatusPending {
				sanitized.CompletedAt = nil
				sanitized.CancelledAt = nil
				sanitized.Degraded = false
				if sanitized.Status != RunStatusFailed {
					sanitized.Message = ""
					sanitized.FailureReason = ""
					sanitized.ErrorCode = ""
				}
			}
			if saved, err := e.persistRunSnapshot(sanitized); err == nil {
				snapshot = saved
			} else {
				snapshot = sanitized
			}
		}
		if e.onDelta != nil {
			snapshot = NormalizeRun(snapshot)
			jftradeErr3 := e.onDelta(ChatDelta{Run: &snapshot})
			jftradeLogError(jftradeErr3)
		}
	}
}

func (e *googleADKExecution) derivedRunStatusForRunLocked(runID string) string {
	calls := e.callsForRunLocked(runID)
	if len(calls) == 0 {
		if e.runHasTextLocked(runID) {
			return RunStatusCompleted
		}
		return RunStatusRunning
	}
	allCancelled := true
	allCompleted := true
	for _, call := range calls {
		switch call.Status {
		case "PENDING_APPROVAL":
			return RunStatusPending
		case "RUNNING", "PENDING":
			return RunStatusRunning
		case "FAILED", "TIMED_OUT", "DENIED":
			allCompleted = false
			allCancelled = false
		case "SUCCEEDED", "COMPLETED":
			allCancelled = false
		case "CANCELLED":
			allCompleted = false
		default:
			allCompleted = false
			allCancelled = false
		}
	}
	if allCancelled {
		return RunStatusCancelled
	}
	if allCompleted {
		if !e.runHasPostToolTextLocked(runID) {
			return RunStatusRunning
		}
		return RunStatusCompleted
	}
	return RunStatusRunning
}

func (e *googleADKExecution) persistedRunStatusForRunLocked(runID string) string {
	calls := e.callsForRunLocked(runID)
	for _, call := range calls {
		switch strings.ToUpper(strings.TrimSpace(call.Status)) {
		case "PENDING_APPROVAL":
			return RunStatusPending
		}
	}
	// These snapshots are emitted while GO-ADK still owns the invocation.
	// Post-tool text is not a terminal boundary: the same invocation may issue
	// another tool call afterwards. Only the explicit completion path may write
	// a terminal run status.
	return RunStatusRunning
}

func (e *googleADKExecution) runHasPostToolTextLocked(runID string) bool {
	e.ensureTextMaps()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	toolSeq := e.toolResponseSeqByRunID[runID]
	if toolSeq == 0 {
		return false
	}
	return e.postToolTextByRunID[runID] && e.postToolTextSeqByRunID[runID] >= toolSeq
}

func (e *googleADKExecution) runHasPostToolText(runID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runHasPostToolTextLocked(runID)
}

func (e *googleADKExecution) runNeedsFinalSynthesis(runID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	calls := e.callsForRunLocked(runID)
	if len(calls) == 0 || e.runHasPostToolTextLocked(runID) {
		return false
	}
	hasFinishedCall := false
	for _, call := range calls {
		switch strings.ToUpper(strings.TrimSpace(call.Status)) {
		case "RUNNING", "PENDING", "PENDING_APPROVAL":
			return false
		case "SUCCEEDED", "COMPLETED", "FAILED", "TIMED_OUT", "DENIED", "CANCELLED":
			hasFinishedCall = true
		}
	}
	return hasFinishedCall
}

func (e *googleADKExecution) runHasTextLocked(runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" || runID == e.runID {
		return strings.TrimSpace(e.reply.String()) != "" || strings.TrimSpace(e.reasoning.String()) != ""
	}
	if builder := e.replyByRunID[runID]; builder != nil && strings.TrimSpace(builder.String()) != "" {
		return true
	}
	if builder := e.reasoningByRunID[runID]; builder != nil && strings.TrimSpace(builder.String()) != "" {
		return true
	}
	return false
}

func (e *googleADKExecution) snapshotRunIDsLocked() []string {
	ids := []string{}
	seen := map[string]struct{}{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(e.runSnapshotBaseByID) > 0 {
		for id := range e.runSnapshotBaseByID {
			add(id)
		}
	}
	add(e.runID)
	for _, call := range e.calls {
		add(call.RunID)
	}
	return ids
}

func (e *googleADKExecution) runSnapshotLocked(runID string, persisted bool) Run {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	base := Run{
		ID: runID, SessionID: e.sessionID, AgentID: e.agent.ID,
		ToolCalls: []ToolCall{}, PendingApprovals: []Approval{},
	}
	if e.runSnapshotBaseByID != nil {
		if candidate, ok := e.runSnapshotBaseByID[runID]; ok {
			base = candidate
		}
	}
	calls := e.callsForRunLocked(runID)
	base.ToolCalls = calls
	base.ToolSummaries = toolSummariesForRun(Run{ToolCalls: calls})
	base.Status = e.derivedRunStatusForRunLocked(runID)
	if persisted {
		base.Status = e.persistedRunStatusForRunLocked(runID)
	}
	base.UpdatedAt = nowString()
	if base.CreatedAt == "" {
		base.CreatedAt = ""
	}
	return base
}

func (e *googleADKExecution) callsForRunLocked(runID string) []ToolCall {
	runID = strings.TrimSpace(runID)
	calls := make([]ToolCall, 0, len(e.calls))
	for _, call := range e.calls {
		if runID != "" && call.RunID != runID {
			continue
		}
		calls = append(calls, call)
	}
	return calls
}

func summarizeToolCall(call ToolCall) string {
	if call.Output != nil {
		return summarizeToolOutput(call.ToolName, call.Output)
	}
	if call.Error != nil && strings.TrimSpace(*call.Error) != "" {
		return call.ToolName + ": " + strings.TrimSpace(*call.Error)
	}
	return ""
}

func googleADKAgentName(id string) string {
	name := strings.ReplaceAll(normalizeID(id), "-", "_")
	if name == "" {
		return "jftrade_agent"
	}
	if name == "user" {
		return "jftrade_user_agent"
	}
	return name
}

func googleADKWorkflowRootName(parentRunID string) string {
	name := "workflow_" + strings.ReplaceAll(normalizeID(parentRunID), "-", "_")
	if name == "workflow_" {
		return "workflow_root"
	}
	return name
}

func googleADKWorkflowChildName(parentRunID string, index int) string {
	name := fmt.Sprintf("%s_child_%d", googleADKWorkflowRootName(parentRunID), index+1)
	if name == "user" {
		return "workflow_child_agent"
	}
	return name
}

func workflowChildInstruction(base string, task string) string {
	task = strings.TrimSpace(task)
	instruction := strings.TrimSpace(base)
	marker := "JFTRADE_WORKFLOW_TASK: " + task
	if instruction == "" {
		return marker
	}
	if task == "" {
		return instruction
	}
	return instruction + "\n\n" + marker + "\n请只完成上述 JFTRADE_WORKFLOW_TASK 指定的子任务。"
}

func workflowChildInstructionTask(step workflowStep) string {
	var builder strings.Builder
	if objective := strings.TrimSpace(step.Objective); objective != "" {
		builder.WriteString("总体目标：")
		builder.WriteString(objective)
	}
	if task := strings.TrimSpace(step.Message); task != "" {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("当前子任务：")
		builder.WriteString(task)
	}
	if description := strings.TrimSpace(step.Description); description != "" && description != strings.TrimSpace(step.Message) {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("子任务说明：")
		builder.WriteString(description)
	}
	if role := strings.TrimSpace(step.AgentRole); role != "" {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("子 Agent 角色：")
		builder.WriteString(role)
	}
	if builder.Len() == 0 {
		return strings.TrimSpace(step.Message)
	}
	builder.WriteString("\n\n请只基于以上明确给出的目标和子任务工作；不要假设自己能看到父对话的其他上下文。")
	return builder.String()
}

func workflowFinalSynthesisInstruction(base string, task string) string {
	instruction := workflowChildInstruction(base, task)
	return instruction + "\n\n工具调用已经完成。现在必须基于已有工具结果输出最终回复。不要再调用工具，不要请求审批，不要只说明准备继续。"
}

func googleADKAppName(id string) string {
	normalized := normalizeID(id)
	if normalized == "" {
		return "jftrade-default"
	}
	return "jftrade-" + normalized
}
