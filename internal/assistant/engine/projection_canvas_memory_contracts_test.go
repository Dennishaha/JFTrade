package adk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	adkmemory "google.golang.org/adk/v2/memory"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

func TestSessionProjectionRetainsUserReplyReasoningAndToolLifecycle(t *testing.T) {
	base := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	user := newProjectionEvent("user-message", "", "user", genai.RoleUser, []*genai.Part{{Text: "  analyze  "}}, base, false)
	partial := newProjectionEvent("assistant-partial", "run-projection", "agent", genai.RoleModel, []*genai.Part{
		{Text: "draft"}, {Text: "reason", Thought: true},
	}, base.Add(time.Second), true)
	call := newProjectionEvent("tool-call", "run-projection", "agent", genai.RoleModel, []*genai.Part{
		{FunctionCall: &genai.FunctionCall{ID: "price", Name: "market.price", Args: map[string]any{"symbol": "AAPL"}}},
		{FunctionCall: &genai.FunctionCall{ID: "ignored-confirmation", Name: toolconfirmation.FunctionCallName}},
		{FunctionCall: &genai.FunctionCall{ID: "ignored-input", Name: adkworkflow.WorkflowInputFunctionCallName}},
	}, base.Add(2*time.Second), false)
	success := newProjectionEvent("tool-success", "run-projection", "user", genai.RoleUser, []*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{ID: "price", Name: "market.price", Response: map[string]any{"last": 220.5}},
	}}, base.Add(3*time.Second), false)
	pending := newProjectionEvent("tool-pending", "run-projection", "user", genai.RoleUser, []*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{ID: "trade", Name: "trade.submit", Response: map[string]any{"error": adktool.ErrConfirmationRequired.Error()}},
	}}, base.Add(4*time.Second), false)
	failed := newProjectionEvent("tool-failed", "run-projection", "user", genai.RoleUser, []*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{ID: "failed", Name: "market.history", Response: map[string]any{"error": "provider unavailable"}},
	}}, base.Add(5*time.Second), false)
	interrupted := newProjectionEvent("tool-interrupted", "run-projection", "user", genai.RoleUser, []*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{ID: "interrupted", Name: "workflow.pause", Response: map[string]any{"error": adkworkflow.ErrNodeInterrupted.Error()}},
	}}, base.Add(6*time.Second), false)
	final := newProjectionEvent("assistant-final", "run-projection", "agent", genai.RoleModel, []*genai.Part{
		{Text: "draft answer"}, {Text: "reasoning", Thought: true},
	}, base.Add(7*time.Second), false)
	partialUser := newProjectionEvent("partial-user", "", "user", genai.RoleUser, []*genai.Part{{Text: "not persisted"}}, base.Add(8*time.Second), true)

	// Supply deliberately unsorted events: projection must use the durable event timeline,
	// rather than the order in which a session backend happened to return rows.
	projection := sessionProjectionFromADKEvents([]*adksession.Event{
		partialUser, final, interrupted, failed, pending, success, call, partial, user,
	})
	if len(projection.Messages) != 2 {
		t.Fatalf("projected messages = %#v, want user and assistant", projection.Messages)
	}
	if projection.Messages[0].Role != "user" || projection.Messages[0].Content != "analyze" {
		t.Fatalf("user entry = %#v", projection.Messages[0])
	}
	if projection.LatestAssistant == nil || projection.LatestAssistant.ID != "assistant-final" {
		t.Fatalf("latest assistant = %#v", projection.LatestAssistant)
	}
	if projection.Reply != "draft answer" || projection.ReasoningContent != "reasoning" || projection.FinalMessageID != "assistant-final" {
		t.Fatalf("assistant projection = %+v", projection)
	}
	if projection.PreToolContent != "draft" || projection.PreToolReasoning != "reason" {
		t.Fatalf("pre-tool projection = content:%q reasoning:%q", projection.PreToolContent, projection.PreToolReasoning)
	}
	if len(projection.ToolCalls) != 3 {
		t.Fatalf("tool calls = %#v, want succeeded, pending and failed", projection.ToolCalls)
	}
	byKey := make(map[string]ToolCall, len(projection.ToolCalls))
	for _, toolCall := range projection.ToolCalls {
		byKey[toolCall.IdempotencyKey] = toolCall
	}
	if call := byKey["price"]; call.Status != "SUCCEEDED" || call.Output == nil || call.Error != nil || call.CompletedAt == nil {
		t.Fatalf("successful tool call = %+v", call)
	}
	if call := byKey["trade"]; call.Status != "PENDING_APPROVAL" || !call.RequiresUser || call.CompletedAt != nil {
		t.Fatalf("approval tool call = %+v", call)
	}
	if call := byKey["failed"]; call.Status != "FAILED" || call.Error == nil || !strings.Contains(*call.Error, "provider unavailable") || call.CompletedAt == nil {
		t.Fatalf("failed tool call = %+v", call)
	}
	if _, found := byKey["interrupted"]; found {
		t.Fatalf("interrupted workflow tool should be pruned: %#v", byKey)
	}

	state := &projectedRunState{toolCalls: map[string]*ToolCall{"fallback:time": {}}, toolCallOrder: []string{"fallback:time"}}
	pruneProjectedToolCall(state, "", "fallback", "time")
	if len(state.toolCalls) != 0 || len(state.toolCallOrder) != 0 {
		t.Fatalf("fallback tool pruning = %+v", state)
	}
	if got := projectedToolProgress(" "); !strings.Contains(got, "unknown") {
		t.Fatalf("blank projected tool progress = %q", got)
	}
}

func newProjectionEvent(id string, invocationID string, author string, role genai.Role, parts []*genai.Part, timestamp time.Time, partial bool) *adksession.Event {
	event := adksession.NewEvent(context.Background(), invocationID)
	event.ID = id
	event.Author = author
	event.Content = &genai.Content{Role: string(role), Parts: parts}
	event.Timestamp = timestamp
	event.Partial = partial
	return event
}

func TestProjectionApprovalMemoryAndCanvasBoundarySemantics(t *testing.T) {
	t.Run("approval projection only retains unique pending work", func(t *testing.T) {
		pending := pendingApprovalsOnly([]Approval{
			{ID: "first", Status: ApprovalStatusPending},
			{ID: " first ", Status: "pending"},
			{ConfirmationCallID: "confirmation", Status: " PENDING "},
			{FunctionCallID: "function", Status: ApprovalStatusPending},
			{Status: ApprovalStatusPending},
			{ID: "approved", Status: ApprovalStatusApproved},
		})
		if len(pending) != 4 {
			t.Fatalf("pending approvals = %#v, want four unique pending entries", pending)
		}
		if pendingApprovalKey(Approval{}) != "" || isPendingApprovalStatus("approved") {
			t.Fatal("approval helper accepted an empty or approved record as pending")
		}
	})

	t.Run("memory scores rank semantic matches and preserve metadata", func(t *testing.T) {
		if score := googleADKMemoryScore(MemoryEntry{Key: "Risk Budget", Value: "Use market stops", Scope: "agent"}, "risk market"); score != 5 {
			t.Fatalf("memory score = %d, want key + value score 5", score)
		}
		if !parseMemoryTime("not-a-timestamp").IsZero() {
			t.Fatal("invalid memory timestamp should fall back to zero time")
		}
		entry := googleADKMemoryEntry(MemoryEntry{
			ID: "memory-1", Scope: "workspace", Key: " Risk ", Value: " keep limits ", UpdatedAt: "not-a-timestamp",
		})
		if entry.ID != "memory-1" || entry.Author != "jftrade.memory.workspace" || entry.Timestamp.IsZero() == false {
			t.Fatalf("memory entry = %#v", entry)
		}
		if entry.CustomMetadata["key"] != " Risk " || entry.Content == nil || len(entry.Content.Parts) != 1 || entry.Content.Parts[0].Text != "Risk: keep limits" {
			t.Fatalf("memory entry metadata/content = %#v", entry)
		}

		runtime := newTestRuntime(t)
		for index := range 10 {
			_, err := runtime.Store().SaveMemory(context.Background(), MemoryWriteRequest{
				Scope: "workspace", Key: "market-note-" + string(rune('a'+index)), Value: "market context",
			})
			if err != nil {
				t.Fatalf("SaveMemory(%d): %v", index, err)
			}
		}
		response, err := runtime.memoryService.SearchMemory(context.Background(), &adkmemory.SearchRequest{AppName: "jftrade-default", Query: "market"})
		if err != nil || response == nil || len(response.Memories) != 8 {
			t.Fatalf("capped memory search = %#v, err=%v", response, err)
		}
		for _, memory := range response.Memories {
			if memory.CustomMetadata["scope"] != "workspace" || !strings.Contains(memory.Content.Parts[0].Text, "market") {
				t.Fatalf("workspace memory search leaked or lost content: %#v", memory)
			}
		}
	})

	t.Run("canvas compiler validates every graph boundary and data fallbacks", func(t *testing.T) {
		cases := []struct {
			name  string
			graph WorkflowCanvasGraph
			want  string
		}{
			{"blank node id", WorkflowCanvasGraph{Nodes: []WorkflowCanvasNode{canvasNode("", "start", nil)}}, "node id is required"},
			{"duplicate node", WorkflowCanvasGraph{Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("start", "agent", nil)}}, "duplicate node id"},
			{"blank edge endpoint", WorkflowCanvasGraph{Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("agent", "agent", nil)}, Edges: []WorkflowCanvasEdge{{ID: "missing-source", Target: "agent"}}}, "requires source and target"},
			{"unknown source", WorkflowCanvasGraph{Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("agent", "agent", nil)}, Edges: []WorkflowCanvasEdge{{ID: "missing-source", Source: "missing", Target: "agent"}}}, "unknown source"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := compileWorkflowCanvasSteps(WorkflowDefinition{AgentID: "parent", CanvasGraph: &tc.graph}, "message", "objective")
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("compile err = %v, want %q", err, tc.want)
				}
			})
		}
		workflow := WorkflowDefinition{AgentID: "parent", ProviderID: "provider", Model: "model", PermissionMode: PermissionModeApproval, CanvasGraph: &WorkflowCanvasGraph{
			Nodes: []WorkflowCanvasNode{
				canvasNode("start", "start", nil),
				canvasNode("worker", "", map[string]any{"type": " agent ", "label": "  Worker  ", "promptTemplate": "  prompt  ", "objectiveTemplate": "  objective  ", "agentRole": 7, "description": true}),
			},
			Edges: []WorkflowCanvasEdge{{ID: "start-worker", Source: "start", Target: "worker"}},
		}}
		steps, err := compileWorkflowCanvasSteps(workflow, "fallback message", "fallback objective")
		if err != nil || len(steps) != 1 {
			t.Fatalf("compile data fallback = %#v, err=%v", steps, err)
		}
		step := steps[0]
		if step.Title != "Worker" || step.Message != "prompt" || step.Objective != "objective" || step.AgentRole != "7" || step.Description != "true" || step.ChildAgentID != "parent" {
			t.Fatalf("canvas step data = %+v", step)
		}
		if workflowCanvasNodeDataString(WorkflowCanvasNode{}, "missing") != "" || workflowCanvasNodeType(canvasNode("x", "", nil)) != "" {
			t.Fatal("canvas node helper did not preserve empty data/type semantics")
		}
	})

	t.Run("canvas execution rejects invalid requests before starting workflow work", func(t *testing.T) {
		runtime := newTestRuntime(t)
		if _, err := runtime.RunCanvasWorkflow(context.Background(), WorkflowCanvasRunRequest{Message: " "}); err == nil {
			t.Fatal("blank canvas message should be rejected")
		}
		if _, err := runtime.RunCanvasWorkflow(context.Background(), WorkflowCanvasRunRequest{Workflow: WorkflowDefinition{}, Message: "run"}); err == nil || !strings.Contains(err.Error(), "graph is required") {
			t.Fatalf("missing canvas graph err = %v", err)
		}
		missingAgentWorkflow := WorkflowDefinition{AgentID: "missing", CanvasGraph: &WorkflowCanvasGraph{
			Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("worker", "agent", nil)},
			Edges: []WorkflowCanvasEdge{{ID: "edge", Source: "start", Target: "worker"}},
		}}
		if _, err := runtime.RunCanvasWorkflow(context.Background(), WorkflowCanvasRunRequest{Workflow: missingAgentWorkflow, Message: "run"}); err == nil {
			t.Fatal("missing canvas agent should be rejected")
		}

		ensureTestProvider(t, runtime)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{ID: "canvas-invalid-mode", Name: "Canvas Invalid Mode", Status: AgentStatusEnabled})
		invalidMode := missingAgentWorkflow
		invalidMode.AgentID = agent.ID
		invalidMode.PermissionMode = "not-a-mode"
		response, err := runtime.RunCanvasWorkflow(context.Background(), WorkflowCanvasRunRequest{Workflow: invalidMode, Message: "run"})
		if err != nil || response.Run.Status != RunStatusCompleted || response.Run.PermissionMode != agent.PermissionMode {
			t.Fatalf("invalid persisted canvas permission should normalize to agent default: response=%+v err=%v", response, err)
		}
	})
}

func TestDirectApprovalResumeErrorRequiresTerminalToolStates(t *testing.T) {
	if isIgnorableDirectApprovalResumeError(nil, toolExecutionContext{}) {
		t.Fatal("nil direct resume error should not be ignored")
	}
	err := errors.New("no function call event found for function responses ids [confirmation]")
	if isIgnorableDirectApprovalResumeError(err, toolExecutionContext{}) {
		t.Fatal("resume error without tool history should not be ignored")
	}
	terminal := toolExecutionContext{calls: []ToolCall{{Status: "SUCCEEDED"}, {Status: "denied"}, {Status: "TIMED_OUT"}}}
	if !isIgnorableDirectApprovalResumeError(err, terminal) {
		t.Fatal("terminal direct approval state should ignore a replay-only response error")
	}
	if isIgnorableDirectApprovalResumeError(err, toolExecutionContext{calls: []ToolCall{{Status: "RUNNING"}}}) {
		t.Fatal("active tool state must not ignore a direct approval resume error")
	}
}
