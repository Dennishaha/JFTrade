package adk

import (
	"context"
	"fmt"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	adktool "google.golang.org/adk/v2/tool"
)

// NewGoogleADKTaskExecution builds a goal-loop ADK execution for the injected
// workflow executor. The task toolset is supplied by the executor so the
// engine root package does not own a workflow execution implementation.
func (r *Runtime) NewGoogleADKTaskExecution(
	ctx context.Context,
	definition Agent,
	productSession Session,
	parent Run,
	req workflowRequest,
	taskTools adktool.Toolset,
	onDelta func(ChatDelta) error,
) (WorkflowExecutionHandle, error) {
	llm, err := r.GoogleADKModelForAgent(ctx, definition)
	if err != nil {
		return nil, err
	}
	rootName := GoogleADKWorkflowRootName(parent.ID)
	engine := jfadkmodel.WorkflowEngineForMode(req.Mode)
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = engine
	}
	descriptors := toolDescriptorIndex(jfadkmodel.WorkflowTaskToolDescriptors())
	execution := &googleADKExecution{
		runtime:   r,
		sessionID: productSession.ID,
		appName:   GoogleADKAppName(definition.ID),
		agent:     definition,
		runID:     parent.ID,
		runIDByAgentName: map[string]string{
			rootName: parent.ID,
		},
		runSnapshotBaseByID: map[string]Run{
			parent.ID: parent,
		},
		descriptors:              descriptors,
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
	orchestratorName := rootName + "_iteration"
	execution.SetRunIDByAgentName(orchestratorName, parent.ID)
	orchestrator, err := llmagent.New(llmagent.Config{
		Name:        orchestratorName,
		Description: definition.Name + " goal orchestrator",
		InstructionProvider: func(ctx adkagent.ReadonlyContext) (string, error) {
			instruction := jfadkmodel.GoalOrchestratorInstruction(definition.Instruction)
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
		Toolsets:        []adktool.Toolset{taskTools},
		IncludeContents: llmagent.IncludeContentsDefault,
	})
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK goal orchestrator agent: %w", err)
	}
	root, err := newGoogleADKLoopWorkflowAgent(rootName, definition.Name+" goal loop", []adkagent.Agent{orchestrator}, 1)
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK goal loop agent: %w", err)
	}
	return r.attachGoogleADKRunner(ctx, execution, productSession, root)
}
