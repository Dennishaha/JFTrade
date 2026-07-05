package adk

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func TestToolOpenAIAndWorkflowHelperAdditionalBoundaries(t *testing.T) {
	t.Run("tool registry helpers cover nil panic timeout and formatting branches", func(t *testing.T) {
		var nilRegistry *ToolRegistry
		nilRegistry.Register(ToolDescriptor{Name: "ignored"}, func(context.Context, map[string]any) (any, error) { return nil, nil })
		if got := nilRegistry.List(); len(got) != 0 {
			t.Fatalf("nil List = %#v, want empty", got)
		}
		if _, ok := nilRegistry.Get("tool"); ok {
			t.Fatal("nil Get unexpectedly found a tool")
		}
		if name, ok := nilRegistry.CanonicalName("tool"); ok || name != "" {
			t.Fatalf("nil CanonicalName = %q/%v, want empty/false", name, ok)
		}
		if descriptors := ToolDescriptorsForAgent(Agent{}, nil); descriptors != nil {
			t.Fatalf("ToolDescriptorsForAgent(nil registry) = %#v, want nil", descriptors)
		}

		registry := NewToolRegistry()
		registry.Register(ToolDescriptor{Name: "live.trade", Permission: "live_trading", DisplayName: "Live Trade"}, func(context.Context, map[string]any) (any, error) {
			return "ok", nil
		})
		live, ok := registry.Get("live.trade")
		if !ok || len(live.Descriptor.AllowedModes) != 1 || live.Descriptor.AllowedModes[0] != PermissionModeAll {
			t.Fatalf("live.trade descriptor = %+v, want all-mode only", live.Descriptor)
		}
		if !ToolRequiresApproval(ToolDescriptor{Name: "instance.create", Permission: "create_strategy_instance"}, PermissionModeLessApproval) {
			t.Fatal("create_strategy_instance should require approval outside all mode")
		}
		if ToolRequiresApproval(ToolDescriptor{Name: "instance.create", Permission: "create_strategy_instance"}, PermissionModeAll) {
			t.Fatal("create_strategy_instance should not require approval in all mode")
		}
		if ToolRequiresApproval(ToolDescriptor{Name: "live.trade", Permission: "live_trading"}, PermissionModeApproval) {
			t.Fatal("live_trading should not require approval here")
		}
		if got := defaultToolRiskLevel("mystery"); got != "medium" {
			t.Fatalf("defaultToolRiskLevel(default) = %q, want medium", got)
		}
		if got := normalizeToolAlias(" @JFTrade  market//snapshot::latest "); got != "market.snapshot.latest" {
			t.Fatalf("normalizeToolAlias = %q, want market.snapshot.latest", got)
		}

		fallbackRegistry := &ToolRegistry{tools: map[string]RegisteredTool{}}
		fallbackRegistry.Register(ToolDescriptor{Name: "tools.search.helper", DisplayName: "Tools Search Helper", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
			return "ok", nil
		})
		if name, ok := fallbackRegistry.CanonicalName("tools search helper"); !ok || name != "tools.search.helper" {
			t.Fatalf("CanonicalName(display name) = %q/%v, want tools.search.helper/true", name, ok)
		}

		ctx := contextWithToolAgent(context.Background(), Agent{Tools: []string{"live.trade"}})
		tool, ok := registry.Get("tools.search")
		if !ok {
			t.Fatal("tools.search missing")
		}
		output, err := tool.Handler(ctx, map[string]any{"category": "missing"})
		if err != nil {
			t.Fatalf("tools.search handler: %v", err)
		}
		if output.(map[string]any)["totalReturned"] != 0 {
			t.Fatalf("tools.search category filter output = %#v, want zero matches", output)
		}

		panicTool := RegisteredTool{Handler: func(context.Context, map[string]any) (any, error) {
			panic("boom")
		}}
		if _, err := executeRegisteredTool(context.Background(), panicTool, nil); err == nil || !strings.Contains(err.Error(), "tool panic") {
			t.Fatalf("executeRegisteredTool panic err = %v", err)
		}
		timeoutTool := RegisteredTool{Handler: func(ctx context.Context, _ map[string]any) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		timeoutCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		time.Sleep(2 * time.Millisecond)
		if _, err := executeRegisteredTool(timeoutCtx, timeoutTool, nil); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("executeRegisteredTool timeout err = %v, want DeadlineExceeded", err)
		}

		var nilCall *ToolCall
		finishToolCall(nilCall)
		call := &ToolCall{StartedAt: "not-a-time"}
		finishToolCall(call)
		if call.CompletedAt == nil || call.DurationMs != 0 {
			t.Fatalf("finishToolCall invalid timestamp = %+v, want completion without duration", call)
		}

		if got := summarizeToolOutput("bad", make(chan int)); !strings.Contains(got, "bad:") {
			t.Fatalf("summarizeToolOutput marshal fallback = %q", got)
		}
		longSummary := summarizeToolOutput("long", map[string]any{"body": strings.Repeat("x", 4000)})
		if !strings.Contains(longSummary, "...(truncated)") {
			t.Fatalf("summarizeToolOutput long = %q, want truncation marker", longSummary)
		}

		if err := rejectUnsafeHost(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "host is required") {
			t.Fatalf("rejectUnsafeHost(blank) err = %v", err)
		}
	})

	t.Run("openai helpers cover truncation normalization and request build errors", func(t *testing.T) {
		truncated := truncateBytes("hello世界", 5)
		if truncated != "\n...(truncated)" {
			t.Fatalf("truncateBytes small max = %q, want marker only", truncated)
		}
		if got := normalizeMessagesForProvider(nil); got != nil {
			t.Fatalf("normalizeMessagesForProvider(nil) = %#v, want nil", got)
		}
		normalized := normalizeMessagesForProvider([]openAIChatMessage{
			{Role: "assistant", ToolCalls: []openAIToolCall{{ID: "call-1"}}},
			{Role: "tool", ToolCallID: ""},
			{Role: "tool", ToolCallID: "call-2"},
			{Role: " assistant ", Content: "ok"},
		})
		if len(normalized) != 1 || normalized[0].Content != "ok" {
			t.Fatalf("normalizeMessagesForProvider = %#v, want orphan tool drops and trimmed role", normalized)
		}

		removeToolCallFromMessage(nil, "call")
		message := openAIChatMessage{ToolCalls: []openAIToolCall{{ID: "keep"}, {ID: "drop"}}}
		removeToolCallFromMessage(&message, "drop")
		if len(message.ToolCalls) != 1 || message.ToolCalls[0].ID != "keep" {
			t.Fatalf("removeToolCallFromMessage = %#v, want keep only", message.ToolCalls)
		}

		client := openAIClient{}
		descriptors := []ToolDescriptor{{Name: "bad.tool", InputSchema: map[string]any{"nan": math.NaN()}}}
		if _, err := client.selectTools(context.Background(), Provider{BaseURL: "https://example.com"}, "", "", nil, descriptors); err == nil {
			t.Fatal("selectTools accepted non-JSON schema payload")
		}
		if _, err := client.selectTools(context.Background(), Provider{BaseURL: "http://%zz"}, "", "", nil, []ToolDescriptor{{Name: "ok.tool"}}); err == nil {
			t.Fatal("selectTools accepted invalid provider URL")
		}
		if _, err := client.chatStream(context.Background(), Provider{BaseURL: "http://%zz"}, "", "", nil, nil); err == nil {
			t.Fatal("chatStream accepted invalid provider URL")
		}
	})

	t.Run("workflow helpers cover direct boundary branches", func(t *testing.T) {
		step := sanitizeWorkflowPlanStep(workflowStep{
			Title:       "same request",
			Description: "same request",
			Message:     "same request",
		}, "same request", 1)
		if step.Title != "执行计划步骤 2" || step.Description != "" || step.Message != "推进计划中的第 2 步。" {
			t.Fatalf("sanitizeWorkflowPlanStep = %+v", step)
		}
		if got := sanitizeWorkflowPlanStep(workflowStep{Title: "keep"}, "", 0); got.Title != "keep" {
			t.Fatalf("sanitizeWorkflowPlanStep blank request = %+v", got)
		}

		if workflowTasksComplete(nil) {
			t.Fatal("workflowTasksComplete(nil) = true, want false")
		}
		if _, ok := firstTerminalWorkflowTask([]Task{{Status: "DONE"}}); ok {
			t.Fatal("firstTerminalWorkflowTask unexpectedly found terminal task")
		}
		ready := executableWorkflowTasks([]Task{
			{ID: "a", Status: "DONE"},
			{ID: "b", Status: "TODO", DependsOn: []string{"a"}, Order: 1},
			{ID: "c", Status: "TODO", DependsOn: []string{"a"}, Order: 2},
		}, "")
		if len(ready) != 1 || ready[0].ID != "b" {
			t.Fatalf("executableWorkflowTasks = %#v, want earliest ready task", ready)
		}

		if got := workflowStepFromTask(Task{Title: "Title", Description: "Desc", Message: ""}); got.Message != "Desc" {
			t.Fatalf("workflowStepFromTask fallback message = %+v", got)
		}
		if got := workflowTaskIteration(Task{}); got != 1 {
			t.Fatalf("workflowTaskIteration = %d, want 1", got)
		}
		if got := workflowPlanIndexForTask(nil, "missing"); got != -1 {
			t.Fatalf("workflowPlanIndexForTask(nil) = %d, want -1", got)
		}
		if got := approvalsForRun([]Approval{{RunID: "run", Status: ApprovalStatusApproved}, {RunID: "run", Status: ApprovalStatusPending}}, " "); got != nil {
			t.Fatalf("approvalsForRun(blank) = %#v, want nil", got)
		}

		parent := Run{WorkflowPlan: []WorkflowStepState{{TaskID: "task-1"}, {TaskID: "task-2"}}}
		child := Run{ID: "child", Status: RunStatusRunning, ProviderID: "provider", Model: "model", Iteration: 3}
		updated := updateWorkflowPlanForChildAt(parent, child, 1)
		if updated.WorkflowCursor != 1 || updated.WorkflowPlan[1].Status != "IN_PROGRESS" {
			t.Fatalf("updateWorkflowPlanForChildAt indexed = %+v", updated.WorkflowPlan[1])
		}
		noMatch := updateWorkflowPlanForChild(parent, Run{ID: "other"})
		if noMatch.WorkflowCursor != 0 && noMatch.WorkflowCursor != parent.WorkflowCursor {
			t.Fatalf("updateWorkflowPlanForChild no match cursor = %d", noMatch.WorkflowCursor)
		}

		var nilStep *WorkflowStepState
		applyWorkflowChildState(nilStep, child)
		blocked := WorkflowStepState{}
		applyWorkflowChildState(&blocked, Run{ID: "child-pending", Status: RunStatusPending})
		if blocked.Status != "BLOCKED" {
			t.Fatalf("applyWorkflowChildState pending = %+v", blocked)
		}
		other := WorkflowStepState{}
		applyWorkflowChildState(&other, Run{ID: "child-failed", Status: RunStatusFailed})
		if other.Status != "BLOCKED" {
			t.Fatalf("applyWorkflowChildState default = %+v", other)
		}

		agent := workflowChildAgentForStep(Agent{ProviderID: "parent-provider", Model: "parent-model", WorkMode: WorkModeLoop}, workflowStep{
			ChildProviderID: "child-provider",
			ChildModel:      "child-model",
		})
		if agent.ProviderID != "child-provider" || agent.Model != "child-model" || agent.WorkMode != WorkModeChat {
			t.Fatalf("workflowChildAgentForStep = %+v", agent)
		}

		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		parentRun := mustSaveRun(t, runtime, Run{
			ID:             "workflow-helper-parent",
			SessionID:      "session",
			AgentID:        "agent",
			Status:         RunStatusRunning,
			WorkMode:       WorkModeLoop,
			WorkflowStatus: workflowStatusRunning,
			Objective:      "目标",
			CreatedAt:      nowString(),
			UpdatedAt:      nowString(),
		})
		for i := range maxRuntimeWorkflowTasks {
			if _, err := runtime.Store().SaveTask(context.Background(), TaskWriteRequest{
				ID:           fmt.Sprintf("runtime-task-%d", i),
				Title:        fmt.Sprintf("task-%d", i),
				Status:       "TODO",
				AgentID:      parentRun.AgentID,
				RunID:        parentRun.ID,
				Order:        i + 1,
				PlanSource:   workflowPlanSourceRuntime,
				WorkflowMode: parentRun.WorkMode,
				Objective:    parentRun.Objective,
			}); err != nil {
				t.Fatalf("SaveTask runtime %d: %v", i, err)
			}
		}
		if _, err := executor.addRuntimeWorkflowTask(context.Background(), parentRun, Task{}, workflowRuntimeTaskRequest{Title: "overflow"}); err == nil || !strings.Contains(err.Error(), "limit reached") {
			t.Fatalf("addRuntimeWorkflowTask limit err = %v", err)
		}
	})
}
