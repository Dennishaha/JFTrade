package adk

import "testing"

func TestNormalizeRunAndResponsesReplaceNilSlices(t *testing.T) {
	run := NormalizeRun(Run{
		ID:               "run-normalize",
		ToolCalls:        nil,
		PendingApprovals: nil,
		ToolSummaries:    nil,
	})
	if run.ToolCalls == nil || len(run.ToolCalls) != 0 {
		t.Fatalf("toolCalls = %#v, want non-nil empty slice", run.ToolCalls)
	}
	if run.PendingApprovals == nil || len(run.PendingApprovals) != 0 {
		t.Fatalf("pendingApprovals = %#v, want non-nil empty slice", run.PendingApprovals)
	}
	if run.ToolSummaries == nil || len(run.ToolSummaries) != 0 {
		t.Fatalf("toolSummaries = %#v, want non-nil empty slice", run.ToolSummaries)
	}

	entry := NormalizeTimelineEntry(TimelineEntry{
		ID:        "entry-normalize",
		ToolCalls: nil,
		Approvals: nil,
	})
	if entry.ToolCalls == nil || len(entry.ToolCalls) != 0 {
		t.Fatalf("timeline toolCalls = %#v, want non-nil empty slice", entry.ToolCalls)
	}
	if entry.Approvals == nil || len(entry.Approvals) != 0 {
		t.Fatalf("timeline approvals = %#v, want non-nil empty slice", entry.Approvals)
	}

	response := NormalizeChatResponse(ChatResponse{
		Run:              Run{ID: "run-chat"},
		PendingApprovals: nil,
		Timeline:         nil,
	})
	if response.Run.ToolCalls == nil || response.Run.PendingApprovals == nil {
		t.Fatalf("normalized run = %+v, want non-nil slices", response.Run)
	}
	if response.PendingApprovals == nil || len(response.PendingApprovals) != 0 {
		t.Fatalf("response pendingApprovals = %#v, want non-nil empty slice", response.PendingApprovals)
	}
	if response.Timeline == nil || len(response.Timeline) != 0 {
		t.Fatalf("response timeline = %#v, want non-nil empty slice", response.Timeline)
	}

	resolution := NormalizeApprovalResolution(ApprovalResolution{
		Run: &Run{ID: "run-resolution"},
	})
	if resolution.Run == nil || resolution.Run.ToolCalls == nil || resolution.Run.PendingApprovals == nil {
		t.Fatalf("resolution run = %+v, want normalized run slices", resolution.Run)
	}

	sessionResponse := NormalizeSessionsResponse(SessionsResponse{})
	if sessionResponse.Timeline == nil || len(sessionResponse.Timeline) != 0 {
		t.Fatalf("session timeline = %#v, want non-nil empty slice", sessionResponse.Timeline)
	}
}

func TestNormalizeWorkflowAndSessionResponsesPreserveBusinessContracts(t *testing.T) {
	agent := NormalizeAgent(Agent{
		ID:                DefaultBuiltinAgentID,
		Status:            "",
		Tools:             []string{" tool.b ", "", "tool.a", "tool.b"},
		Skills:            []string{" external-http ", "external-http"},
		PermissionMode:    "bad",
		RecentUserWindow:  -10,
		WorkMode:          "bad",
		LoopMaxIterations: 999,
	})
	if agent.Name != "默认助手" || !agent.Builtin || agent.Status != AgentStatusEnabled {
		t.Fatalf("builtin agent normalization = %#v", agent)
	}
	if len(agent.Tools) != 2 || agent.Tools[0] != "tool.a" || agent.Tools[1] != "tool.b" {
		t.Fatalf("agent tools = %#v", agent.Tools)
	}
	if len(agent.Skills) != len(BuiltinSkillIDs()) {
		t.Fatalf("builtin skills = %#v", agent.Skills)
	}
	if agent.RecentUserWindow <= 0 || agent.LoopMaxIterations != MaxLoopIterations || agent.WorkMode == "bad" {
		t.Fatalf("agent defaults = %#v", agent)
	}

	workflow := NormalizeWorkflowDefinition(WorkflowDefinition{
		ID:                " workflow-1 ",
		Name:              " Risk Review ",
		Description:       " check exposure ",
		Status:            "",
		AgentID:           " agent-1 ",
		WorkMode:          "loop",
		ProviderID:        " provider-1 ",
		Model:             " model-1 ",
		PermissionMode:    "all",
		PromptTemplate:    " {{.symbol}} ",
		DefaultInputs:     map[string]any{" symbol ": "US.AAPL", "": "ignored"},
		Tags:              []string{" risk ", "risk", "daily"},
		CanvasGraph:       &WorkflowCanvasGraph{Version: " 1 ", Nodes: []WorkflowCanvasNode{{ID: " n1 ", Type: " prompt ", Data: map[string]any{" title ": "Review"}}}, Edges: []WorkflowCanvasEdge{{ID: " e1 ", Source: " n1 ", Target: " n2 ", Type: " default ", Data: map[string]any{" label ": "next"}}}},
		ObjectiveTemplate: " objective ",
	})
	if workflow.ID != "workflow-1" || workflow.Status != WorkflowStatusEnabled || workflow.AgentID != "agent-1" {
		t.Fatalf("workflow identity/defaults = %#v", workflow)
	}
	if workflow.DefaultInputs["symbol"] != "US.AAPL" || len(workflow.Tags) != 2 {
		t.Fatalf("workflow inputs/tags = %#v / %#v", workflow.DefaultInputs, workflow.Tags)
	}
	if workflow.CanvasGraph == nil || workflow.CanvasGraph.Version != "1" || workflow.CanvasGraph.Nodes[0].ID != "n1" || workflow.CanvasGraph.Edges[0].Source != "n1" {
		t.Fatalf("workflow graph = %#v", workflow.CanvasGraph)
	}

	rawResponse := &ChatResponse{
		Run:              Run{ID: "run-workflow", ChildRunIDs: []string{" child-b ", "child-a", "child-b"}},
		PendingApprovals: nil,
		Timeline:         nil,
	}
	log := NormalizeWorkflowTriggerLog(WorkflowTriggerLog{
		ID:          " log-1 ",
		WorkflowID:  " workflow-1 ",
		TriggerID:   " trigger-1 ",
		TriggerType: " manual ",
		Status:      "",
		RunID:       " run-1 ",
		SessionID:   " session-1 ",
		Inputs:      map[string]any{" symbol ": "US.AAPL"},
		MatchedEvent: map[string]any{
			" event ": "manual",
		},
		Result: &WorkflowResult{
			Format:      " markdown ",
			Markdown:    " done ",
			JSON:        map[string]any{" status ": "ok"},
			RawResponse: rawResponse,
		},
		NodeRuns: []WorkflowNodeRun{
			{NodeID: " node-1 ", NodeType: " prompt ", Title: " Review ", Status: "", Inputs: map[string]any{" x ": 1}, Outputs: map[string]any{" y ": 2}},
			{NodeID: " ", Status: "FAILED"},
		},
		Error:      " ",
		StartedAt:  " start ",
		FinishedAt: " finish ",
	})
	if log.WorkflowID != "workflow-1" || log.Status != WorkflowTriggerLogStatusQueued || log.Inputs["symbol"] != "US.AAPL" {
		t.Fatalf("workflow trigger log = %#v", log)
	}
	if log.Result == nil || log.Result.Format != "markdown" || log.Result.Markdown != "done" || log.Result.JSON["status"] != "ok" {
		t.Fatalf("workflow result = %#v", log.Result)
	}
	if log.Result.RawResponse == nil || len(log.Result.RawResponse.Run.ChildRunIDs) != 2 || log.Result.RawResponse.Timeline == nil {
		t.Fatalf("raw response not normalized = %#v", log.Result.RawResponse)
	}
	if len(log.NodeRuns) != 1 || log.NodeRuns[0].NodeID != "node-1" || log.NodeRuns[0].Status != WorkflowTriggerLogStatusQueued || log.NodeRuns[0].Inputs["x"] != 1 {
		t.Fatalf("node runs = %#v", log.NodeRuns)
	}

	sessionResponse := NormalizeSessionsResponse(SessionsResponse{
		Session: Session{ID: "session-1"},
		Runs: []Run{{
			ID:               "run-session",
			ToolCalls:        nil,
			PendingApprovals: nil,
		}},
		ComposerState: SessionComposerState{SessionID: ""},
	})
	if len(sessionResponse.Runs) != 1 || sessionResponse.Runs[0].ToolCalls == nil || sessionResponse.Runs[0].PendingApprovals == nil {
		t.Fatalf("session runs = %#v", sessionResponse.Runs)
	}
	if sessionResponse.ComposerState.SessionID != "session-1" {
		t.Fatalf("composer state = %#v", sessionResponse.ComposerState)
	}
}
