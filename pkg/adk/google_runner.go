package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	adkartifact "google.golang.org/adk/v2/artifact"
	adkmodel "google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/plugin"
	adkrunner "google.golang.org/adk/v2/runner"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
	"gorm.io/gorm"
)

const googleADKUserID = "jftrade-user"

var errUserGoalPauseRequested = errors.New("user goal pause requested")
var errADKInputUnsupported = errors.New("GO-ADK requested input is unsupported; configure the agent/workflow to collect required input before running")

type googleADKExecution struct {
	mu                       sync.Mutex
	runner                   *adkrunner.Runner
	sessionService           adksession.Service
	artifactService          adkartifact.Service
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
		toolContext := execution.toolContext()
		return toolContext, nil, execution.result(), preToolContent, preToolReasoning, err
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

func (r *Runtime) rehydrateGoogleADKExecution(ctx context.Context, run Run) (*googleADKExecution, error) {
	execution, err := r.newResumedGoogleADKExecution(ctx, run)
	if err != nil {
		return nil, err
	}
	seedResumedExecutionState(execution, run)
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

func (r *Runtime) newResumedGoogleADKExecution(ctx context.Context, run Run) (*googleADKExecution, error) {
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
	return execution, nil
}

func seedResumedExecutionState(execution *googleADKExecution, run Run) {
	if execution == nil || strings.TrimSpace(run.ID) == "" {
		return
	}
	if execution.runSnapshotBaseByID == nil {
		execution.runSnapshotBaseByID = map[string]Run{}
	}
	execution.runSnapshotBaseByID[run.ID] = run
	seenCalls := map[string]struct{}{}
	for _, call := range execution.calls {
		if strings.TrimSpace(call.ID) != "" {
			seenCalls["id:"+strings.TrimSpace(call.ID)] = struct{}{}
		}
		if strings.TrimSpace(call.IdempotencyKey) != "" {
			seenCalls["key:"+strings.TrimSpace(call.IdempotencyKey)] = struct{}{}
		}
	}
	for _, call := range run.ToolCalls {
		key := strings.TrimSpace(call.ID)
		if key != "" {
			key = "id:" + key
		} else if strings.TrimSpace(call.IdempotencyKey) != "" {
			key = "key:" + strings.TrimSpace(call.IdempotencyKey)
		}
		if key != "" {
			if _, ok := seenCalls[key]; ok {
				continue
			}
			seenCalls[key] = struct{}{}
		}
		execution.calls = append(execution.calls, call)
	}
	execution.summaries = append(execution.summaries, run.ToolSummaries...)
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
		sessionID:       productSession.ID,
		appName:         googleADKAppName(definition.ID),
		artifactService: r.artifactService,
		agent:           definition,
		runID:           runID,
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
	execution := r.newGoogleADKWorkflowExecutionState(definition, productSession, parent, rootName, onDelta)
	childNodes, err := r.newGoogleADKWorkflowChildNodes(ctx, definition, parent.ID, childRuns, steps, execution)
	if err != nil {
		return nil, err
	}
	if len(childNodes) == 0 {
		return nil, fmt.Errorf("workflow requires at least one sub-agent")
	}
	edges, err := compileGoogleADKWorkflowEdges(steps, childNodes)
	if err != nil {
		return nil, err
	}
	root, err := newGoogleADKWorkflowAgent(googleADKWorkflowAgentConfig{
		Name:           rootName,
		Description:    definition.Name + " task workflow",
		Edges:          edges,
		MaxConcurrency: MaxConcurrentRuns,
	})
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK workflow agent: %w", err)
	}
	return r.attachGoogleADKRunner(ctx, execution, productSession, root)
}

func (r *Runtime) newGoogleADKWorkflowExecutionState(
	definition Agent,
	productSession Session,
	parent Run,
	rootName string,
	onDelta func(ChatDelta) error,
) *googleADKExecution {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = WorkflowEngineADK2Loop
	}
	return &googleADKExecution{
		sessionID:       productSession.ID,
		appName:         googleADKAppName(definition.ID),
		artifactService: r.artifactService,
		agent:           definition,
		runID:           parent.ID,
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
		loadRun:                  r.googleADKExecutionRunLoader(),
		persistRunSnapshot:       r.googleADKExecutionSnapshotPersister(),
	}
}

func (r *Runtime) googleADKExecutionRunLoader() func(context.Context, string) (Run, bool, error) {
	return func(ctx context.Context, runID string) (Run, bool, error) {
		if r.store == nil {
			return Run{}, false, nil
		}
		return r.store.Run(ctx, runID)
	}
}

func (r *Runtime) googleADKExecutionSnapshotPersister() func(Run) (Run, error) {
	return func(snapshot Run) (Run, error) {
		return r.persistRunActivitySnapshot(context.Background(), snapshot)
	}
}

func (r *Runtime) newGoogleADKWorkflowChildNodes(
	ctx context.Context,
	definition Agent,
	parentID string,
	childRuns []Run,
	steps []workflowStep,
	execution *googleADKExecution,
) ([]adkworkflow.Node, error) {
	childNodes := make([]adkworkflow.Node, 0, len(childRuns))
	for index, child := range childRuns {
		if index >= len(steps) {
			break
		}
		childNode, err := r.newGoogleADKWorkflowChildNode(ctx, definition, parentID, child, steps[index], index, execution)
		if err != nil {
			return nil, err
		}
		childNodes = append(childNodes, childNode)
	}
	return childNodes, nil
}

func (r *Runtime) newGoogleADKWorkflowChildNode(
	ctx context.Context,
	definition Agent,
	parentID string,
	child Run,
	step workflowStep,
	index int,
	execution *googleADKExecution,
) (adkworkflow.Node, error) {
	name := googleADKWorkflowChildName(parentID, index)
	execution.runIDByAgentName[name] = child.ID
	execution.runSnapshotBaseByID[child.ID] = child
	childDefinition, err := r.workflowChildAgentForStep(ctx, definition, step)
	if err != nil {
		return nil, err
	}
	childDefinition.WorkMode = WorkModeChat
	childDefinition.Instruction = workflowChildInstruction(childDefinition.Instruction, workflowChildInstructionTask(step))
	childLLM, err := r.googleADKModelForAgent(ctx, childDefinition)
	if err != nil {
		return nil, err
	}
	childAgent, err := r.newGoogleADKLLMAgent(ctx, name, step.Title, childDefinition, childLLM, execution)
	if err != nil {
		return nil, err
	}
	return newGoogleADKWorkflowAgentNode(childAgent)
}

func compileGoogleADKWorkflowEdges(steps []workflowStep, nodes []adkworkflow.Node) ([]adkworkflow.Edge, error) {
	return newWorkflowCompiler().CompileEdges(steps, nodes)
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
		AppName:         execution.appName,
		Agent:           synthesisAgent,
		SessionService:  execution.sessionService,
		ArtifactService: r.artifactService,
		MemoryService:   r.memoryService,
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
		Model:           llm,
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
	executionPlugin, err := execution.plugin()
	if err != nil {
		return nil, err
	}
	adkRunner, err := adkrunner.New(adkrunner.Config{
		AppName:         execution.appName,
		Agent:           adkAgent,
		SessionService:  service,
		ArtifactService: r.artifactService,
		MemoryService:   r.memoryService,
		PluginConfig: adkrunner.PluginConfig{
			Plugins: []*plugin.Plugin{executionPlugin},
		},
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
