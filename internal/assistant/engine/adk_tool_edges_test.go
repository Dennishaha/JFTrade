package adk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	adkagent "google.golang.org/adk/v2/agent"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

func TestSmallADKBoundaryTailBranches(t *testing.T) {
	if _, err := googleADKJSONSchemaFromMap(map[string]any{"type": 123}); err == nil {
		t.Fatal("googleADKJSONSchemaFromMap invalid schema err = nil, want decode error")
	}

	runtime := newTestRuntime(t)
	for index := range 2 {
		mustSaveProvider(t, runtime, ProviderWriteRequest{
			ID: fmt.Sprintf("limit-provider-%d", index), DisplayName: fmt.Sprintf("Limit Provider %d", index),
			BaseURL: "https://example.test/v1", Model: fmt.Sprintf("model-%d", index), APIKey: "sk-limit", Enabled: true,
		})
	}
	raw, err := runtime.modelsListTool(t.Context(), map[string]any{"limit": 1, "callableOnly": "yes"})
	if err != nil {
		t.Fatalf("modelsListTool limit: %v", err)
	}
	payload := raw.(map[string]any)
	if payload["totalReturned"] != 1 {
		t.Fatalf("modelsListTool limit payload = %#v", payload)
	}
	if !toolBoolValue(map[string]any{"flag": "true"}, "flag", false) {
		t.Fatal("toolBoolValue true string = false, want true")
	}

	var splitter legacyAssistantContentSplitter
	reply, reasoning := splitter.Push("visible <not-a-tag> tail")
	if !strings.Contains(reply, "<not-a-tag>") || reasoning != "" {
		t.Fatalf("legacy splitter unknown tag = reply:%q reasoning:%q", reply, reasoning)
	}

	execution := &googleADKExecution{
		runID: "plugin-run",
		agent: Agent{PermissionMode: PermissionModeApproval},
		descriptors: map[string]ToolDescriptor{
			"live.trade": {Name: "live.trade", Permission: "live_trading", AllowedModes: []string{PermissionModeAll}},
		},
	}
	ctx := newGoogleADKToolTestContext()
	if result, err := execution.beforeToolCallback(ctx, boundaryGoogleTool{name: "unknown.tool"}, map[string]any{}); result != nil || err != nil {
		t.Fatalf("beforeToolCallback unknown = %#v/%v, want nil/nil", result, err)
	}
	if result, err := execution.beforeToolCallback(ctx, boundaryGoogleTool{name: "live.trade"}, map[string]any{}); result != nil || err == nil || !strings.Contains(err.Error(), "permission mode") {
		t.Fatalf("beforeToolCallback disallowed = %#v/%v, want permission error", result, err)
	}
	if result, err := execution.afterToolCallback(ctx, boundaryGoogleTool{name: "unknown.tool"}, map[string]any{}, nil, nil); result != nil || err != nil {
		t.Fatalf("afterToolCallback unknown = %#v/%v, want nil/nil", result, err)
	}

	nodes := []adkworkflow.Node{newWorkflowCompilerTestNode("first")}
	edges, err := newWorkflowCompiler().CompileEdges([]workflowStep{}, nodes)
	if err != nil {
		t.Fatalf("CompileEdges fallback: %v", err)
	}
	if len(edges) != 1 || edges[0].To.Name() != "first" {
		t.Fatalf("CompileEdges fallback edges = %+v", edges)
	}
	edges, err = newWorkflowCompiler().CompileEdges([]workflowStep{{DependencyID: "first"}, {DependencyID: "ignored"}}, nodes)
	if err != nil || len(edges) != 1 || edges[0].To.Name() != "first" {
		t.Fatalf("CompileEdges fewer nodes = %+v/%v", edges, err)
	}
}

func TestProviderHTTPBoundaryTailBranches(t *testing.T) {
	if err := validateProviderHostname(" "); err == nil || !strings.Contains(err.Error(), "host is required") {
		t.Fatalf("blank provider host err = %v, want required", err)
	}
	if err := validateProviderIP(netip.Addr{}); err == nil || !strings.Contains(err.Error(), "unspecified") {
		t.Fatalf("invalid provider IP err = %v, want unspecified", err)
	}
	if err := validateProviderIP(netip.MustParseAddr("224.0.0.1")); err == nil || !strings.Contains(err.Error(), "multicast") {
		t.Fatalf("multicast provider IP err = %v, want multicast", err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("jftradeCheckedTypeAssertion panic = nil, want panic")
			}
		}()
		_ = jftradeCheckedTypeAssertion[*http.Transport]("not a transport")
	}()

	lookupErr := errors.New("lookup failed")
	client := newProviderHTTPClientWithResolver(time.Second, func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, lookupErr
	})
	transport := client.Transport.(*http.Transport)
	if _, err := transport.DialContext(t.Context(), "tcp", "missing-port"); err == nil {
		t.Fatal("provider DialContext split host port err = nil, want error")
	}
	if _, err := transport.DialContext(t.Context(), "tcp", "metadata:443"); err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("provider DialContext metadata err = %v, want metadata blocked", err)
	}
	if _, err := transport.DialContext(t.Context(), "tcp", "example.test:443"); !errors.Is(err, lookupErr) {
		t.Fatalf("provider DialContext lookup err = %v, want lookupErr", err)
	}

	blockedClient := newProviderHTTPClientWithResolver(time.Second, func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
	})
	if _, err := blockedClient.Transport.(*http.Transport).DialContext(t.Context(), "tcp", "example.test:443"); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("provider DialContext blocked IP err = %v, want blocked address", err)
	}
	emptyClient := newProviderHTTPClientWithResolver(time.Second, func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, nil
	})
	if _, err := emptyClient.Transport.(*http.Transport).DialContext(t.Context(), "tcp", "example.test:443"); err == nil || !strings.Contains(err.Error(), "no usable addresses") {
		t.Fatalf("provider DialContext empty addresses err = %v, want no usable", err)
	}
	if err := emptyClient.CheckRedirect(&http.Request{URL: &url.URL{Host: "example.test"}}, make([]*http.Request, 5)); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Fatalf("provider redirect limit err = %v, want redirect limit", err)
	}
	if err := emptyClient.CheckRedirect(&http.Request{URL: &url.URL{Host: "metadata"}}, nil); err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("provider redirect metadata err = %v, want metadata host", err)
	}
}

func TestProjectionAndReasoningHelperBoundaryBranches(t *testing.T) {
	if got := projectionRunID(nil); got != "" {
		t.Fatalf("nil projectionRunID = %q, want empty", got)
	}
	if got := projectionRunID(&adksession.Event{InvocationID: " invocation "}); got != "invocation" {
		t.Fatalf("invocation projectionRunID = %q", got)
	}
	if got := projectionRunID(&adksession.Event{ID: " event-id "}); got != "event-id" {
		t.Fatalf("event projectionRunID = %q", got)
	}
	timestamp := time.Date(2026, 7, 5, 1, 2, 3, 4, time.FixedZone("CST", 8*60*60))
	if got := projectionRunID(&adksession.Event{Timestamp: timestamp}); got != timestamp.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("timestamp projectionRunID = %q", got)
	}
	if got := eventTimeString(&adksession.Event{}); got == "" {
		t.Fatal("zero eventTimeString should fall back to nowString")
	}
	var builder strings.Builder
	mergeProjectedText(&builder, "hello", false)
	mergeProjectedText(&builder, "hello world", false)
	if got := builder.String(); got != "hello world" {
		t.Fatalf("prefix merge = %q", got)
	}
	mergeProjectedText(&builder, "world", false)
	if got := builder.String(); got != "hello world" {
		t.Fatalf("suffix merge = %q", got)
	}
	mergeProjectedText(&builder, "!", false)
	if got := builder.String(); got != "hello world!" {
		t.Fatalf("append merge = %q", got)
	}

	var splitter legacyAssistantContentSplitter
	splitter.tagBuffer.WriteString("<think>")
	if reply, reasoning := splitter.Flush(); reply != "" || reasoning != "" || splitter.mode != reasoningModeReasoning {
		t.Fatalf("flush opening = (%q,%q) mode=%v", reply, reasoning, splitter.mode)
	}
	splitter.tagBuffer.WriteString("</think>")
	if reply, reasoning := splitter.Flush(); reply != "" || reasoning != "" || splitter.mode != reasoningModeReply {
		t.Fatalf("flush closing = (%q,%q) mode=%v", reply, reasoning, splitter.mode)
	}
	splitter = legacyAssistantContentSplitter{mode: reasoningModeReasoning}
	splitter.tagBuffer.WriteString("<partial")
	if reply, reasoning := splitter.Flush(); reply != "" || reasoning != "<partial" {
		t.Fatalf("flush reasoning partial = (%q,%q)", reply, reasoning)
	}
}

func TestNormalizeAndWorkflowModelToolBoundaryBranches(t *testing.T) {
	run := Run{ID: "run-resolution", ToolCalls: nil}
	parent := Run{ID: "parent-resolution", ToolCalls: nil}
	resolution := NormalizeApprovalResolution(ApprovalResolution{Run: &run, ParentRun: &parent})
	if resolution.Run == &run || resolution.ParentRun == &parent {
		t.Fatal("NormalizeApprovalResolution should copy run pointers")
	}
	if got := normalizeAnyMap(map[string]any{" ": "ignored"}); len(got) != 0 {
		t.Fatalf("normalizeAnyMap blank-only = %#v, want empty", got)
	}
	runtime := newTestRuntime(t)
	toolset := &workflowTaskToolset{executor: runtime.workflowExecutor()}
	modelTool, err := toolset.modelsListTool()
	if err != nil {
		t.Fatalf("modelsListTool: %v", err)
	}
	if modelTool.Name() != workflowModelsListTool {
		t.Fatalf("models tool name = %q", modelTool.Name())
	}
	jftradeCheckTestError(t, runtime.Store().Close())
	if _, err := toolset.modelsList(map[string]any{"query": "test"}); err == nil {
		t.Fatal("modelsList closed runtime err = nil, want error")
	}
}

func TestWorkflowTaskLocalHelperBoundaryBranches(t *testing.T) {
	var nilDecision *workflowGoalDecision
	nilDecision.reset()
	nilDecision.beginDecision()
	nilDecision.setComplete("ignored")
	nilDecision.setContinue("ignored")
	if nilDecision.decisionPhase() {
		t.Fatal("nil decision should not be in decision phase")
	}
	if snap := nilDecision.snapshot(); snap.status != "" || snap.summary != "" || snap.reason != "" {
		t.Fatalf("nil decision snapshot status=%q summary=%q reason=%q, want empty", snap.status, snap.summary, snap.reason)
	}
	decision := &workflowGoalDecision{}
	decision.beginDecision()
	if !decision.decisionPhase() {
		t.Fatal("decision should be in decision phase")
	}
	decision.setComplete(" complete summary ")
	if snap := decision.snapshot(); snap.status != "complete" || snap.summary != "complete summary" || snap.reason != "" {
		t.Fatalf("complete decision snapshot status=%q summary=%q reason=%q", snap.status, snap.summary, snap.reason)
	}
	decision.setContinue(" continue reason ")
	if snap := decision.snapshot(); snap.status != "continue" || snap.reason != "continue reason" || snap.summary != "" {
		t.Fatalf("continue decision snapshot status=%q summary=%q reason=%q", snap.status, snap.summary, snap.reason)
	}
	decision.reset()
	if decision.decisionPhase() {
		t.Fatal("reset decision should leave decision phase")
	}

	if run, changed := pruneInterruptedGoalWorkflowToolCalls(Run{}); changed || len(run.ToolCalls) != 0 {
		t.Fatalf("empty prune = %+v changed=%v", run, changed)
	}
	pauseErr := errUserGoalPauseRequested.Error()
	run, changed := pruneInterruptedGoalWorkflowToolCalls(Run{
		ID: "parent-run",
		ToolCalls: []ToolCall{
			{ID: "keep-other-run", RunID: "child-run", ToolName: workflowTasksListTool, Status: "RUNNING"},
			{ID: "keep-business", RunID: "parent-run", ToolName: "market.candles", Status: "RUNNING"},
			{ID: "keep-failed-other", RunID: "parent-run", ToolName: workflowTasksListTool, Status: "FAILED"},
			{ID: "drop-running", RunID: "parent-run", ToolName: workflowTasksListTool, Status: "RUNNING"},
			{ID: "drop-pending", ToolName: workflowTaskAddTool, Status: "PENDING"},
			{ID: "drop-failed-pause", ToolName: workflowTaskClaimTool, Status: "FAILED", Error: &pauseErr},
		},
	})
	if !changed {
		t.Fatal("workflow tool prune changed = false, want true")
	}
	if len(run.ToolCalls) != 3 {
		t.Fatalf("pruned tool calls = %+v, want three kept calls", run.ToolCalls)
	}
	for _, call := range run.ToolCalls {
		if strings.HasPrefix(call.ID, "drop-") {
			t.Fatalf("interrupted call was not pruned: %+v", run.ToolCalls)
		}
	}
}

func TestWorkflowTaskToolsetLookupBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-helper-parent", SessionID: "workflow-helper-session", AgentID: "workflow-helper-agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{
			{TaskID: "task-current", ChildRunID: ""},
			{TaskID: "task-missing-child", ChildRunID: "missing-child"},
			{TaskID: "task-foreign-child", ChildRunID: "foreign-child"},
			{TaskID: "task-pending-child", ChildRunID: "pending-child"},
		},
		ChildRunIDs: []string{"", "workflow-helper-parent"},
		CreatedAt:   now, UpdatedAt: now,
	})
	current, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-current", Title: "Current", Status: "IN_PROGRESS", AgentID: parent.AgentID, RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask current: %v", err)
	}
	ready, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-ready", Title: "Ready", Status: "TODO", AgentID: parent.AgentID, RunID: parent.ID, Order: 2, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask ready: %v", err)
	}
	mustSaveRun(t, runtime, Run{
		ID: "foreign-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: "other-parent",
		Status: RunStatusRunning, CreatedAt: now, UpdatedAt: now,
	})
	mustSaveRun(t, runtime, Run{
		ID: "pending-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusPending, CreatedAt: now, UpdatedAt: now,
	})
	toolset := &workflowTaskToolset{
		executor:      runtime.workflowExecutor(),
		parentID:      parent.ID,
		currentTaskID: current.ID,
		req:           workflowRequest{Mode: WorkModeLoop},
	}
	if _, _, err := (&workflowTaskToolset{executor: runtime.workflowExecutor(), parentID: "missing-parent"}).parentAndTasks(ctx); err == nil || !strings.Contains(err.Error(), "parent run not found") {
		t.Fatalf("missing parentAndTasks err = %v", err)
	}
	if task, ok := toolset.taskByID(ctx, " "); ok || task.ID != "" {
		t.Fatalf("blank taskByID = %+v/%v, want missing", task, ok)
	}
	if task, err := toolset.resolveTask(ctx, parent, []Task{ready}, "missing-task", true); err == nil || task.ID != "" || !strings.Contains(err.Error(), "task not found") {
		t.Fatalf("explicit missing resolveTask = %+v/%v", task, err)
	}
	if task, err := toolset.resolveTask(ctx, parent, []Task{ready}, "", false); err != nil || task.ID != current.ID {
		t.Fatalf("current resolveTask = %+v/%v, want current", task, err)
	}
	toolset.currentTaskID = ""
	if task, err := toolset.resolveTask(ctx, parent, []Task{{ID: "in-progress", Status: "IN_PROGRESS"}}, "", false); err != nil || task.ID != "in-progress" {
		t.Fatalf("in-progress resolveTask = %+v/%v", task, err)
	}
	if task, err := toolset.resolveTask(ctx, parent, []Task{ready}, "", true); err != nil || task.ID != ready.ID {
		t.Fatalf("ready resolveTask = %+v/%v", task, err)
	}
	if _, err := toolset.resolveTask(ctx, parent, []Task{{ID: "blocked-ready", Status: "TODO", DependsOn: []string{"missing"}}}, "", true); err == nil || !strings.Contains(err.Error(), "no executable workflow task") {
		t.Fatalf("no executable resolveTask err = %v", err)
	}

	child, index, ok := runtime.workflowExecutor().firstBlockingTaskChild(ctx, parent)
	if !ok || child.ID != "pending-child" || index != 3 {
		t.Fatalf("firstBlockingTaskChild = %+v index=%d ok=%v, want pending child at index 3", child, index, ok)
	}
	cleanParent := parent
	cleanParent.WorkflowPlan = []WorkflowStepState{{TaskID: "blank-child"}, {TaskID: "done-child", ChildRunID: "done-child"}}
	mustSaveRun(t, runtime, Run{
		ID: "done-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusCompleted, CreatedAt: now, UpdatedAt: now,
	})
	if child, index, ok := runtime.workflowExecutor().firstBlockingTaskChild(ctx, cleanParent); ok || child.ID != "" || index != -1 {
		t.Fatalf("clean firstBlockingTaskChild = %+v index=%d ok=%v, want none", child, index, ok)
	}

	blockers := toolset.workflowCompletionBlockers(ctx, parent, []Task{{ID: "done", Status: "DONE"}})
	if len(blockers) != 0 {
		t.Fatalf("blank/self child IDs should not block completion: %+v", blockers)
	}

	dir := t.TempDir()
	closedStore, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore closed workflow task lookup: %v", err)
	}
	closedRuntime := NewRuntime(closedStore, NewToolRegistry())
	closedToolset := &workflowTaskToolset{executor: closedRuntime.workflowExecutor(), parentID: parent.ID}
	jftradeCheckTestError(t, closedStore.Close())
	if _, _, err := closedToolset.parentAndTasks(ctx); err == nil {
		t.Fatal("closed parentAndTasks err = nil, want error")
	}
	if task, ok := closedToolset.taskByID(ctx, current.ID); ok || task.ID != "" {
		t.Fatalf("closed taskByID = %+v/%v, want missing", task, ok)
	}
	if err := closedToolset.saveParentPlan(ctx, parent, nil); err == nil {
		t.Fatal("closed saveParentPlan err = nil, want error")
	}
	for _, tc := range []struct {
		name string
		call func() (map[string]any, error)
	}{
		{name: "list", call: func() (map[string]any, error) { return closedToolset.list(nil) }},
		{name: "add", call: func() (map[string]any, error) { return closedToolset.add(map[string]any{"title": "x"}) }},
		{name: "claim", call: func() (map[string]any, error) { return closedToolset.claim(map[string]any{"taskId": current.ID}) }},
		{name: "complete", call: func() (map[string]any, error) { return closedToolset.complete(map[string]any{"taskId": current.ID}) }},
		{name: "block", call: func() (map[string]any, error) { return closedToolset.block(map[string]any{"taskId": current.ID}) }},
		{name: "delegate", call: func() (map[string]any, error) { return closedToolset.delegate(map[string]any{"taskId": current.ID}) }},
		{name: "goalComplete", call: func() (map[string]any, error) { return closedToolset.goalComplete(map[string]any{"summary": "done"}) }},
	} {
		t.Run("closed "+tc.name, func(t *testing.T) {
			if result, err := tc.call(); err == nil || result != nil {
				t.Fatalf("%s closed result = %#v err=%v, want nil/error", tc.name, result, err)
			}
		})
	}
}

func TestWorkflowTaskToolsetMethodErrorAndFallbackBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-method-branches-parent", SessionID: "workflow-method-branches-session", AgentID: "workflow-method-agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})
	done, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "workflow-method-done", Title: "Done task", Status: "DONE", AgentID: parent.AgentID, RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask done: %v", err)
	}
	ready, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "workflow-method-ready", Title: "Ready task", Status: "TODO", AgentID: parent.AgentID, RunID: parent.ID, Order: 2, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask ready: %v", err)
	}
	parent.WorkflowPlan = workflowPlanFromTasks([]Task{done, ready}, nil)
	mustSaveRun(t, runtime, parent)
	toolset := &workflowTaskToolset{
		executor: runtime.workflowExecutor(),
		parentID: parent.ID,
		req:      workflowRequest{Mode: WorkModeLoop, GoalDecision: &workflowGoalDecision{}},
	}
	for _, tc := range []struct {
		name string
		call func() (map[string]any, error)
	}{
		{name: "claim missing", call: func() (map[string]any, error) { return toolset.claim(map[string]any{"taskId": "missing-task"}) }},
		{name: "complete missing", call: func() (map[string]any, error) { return toolset.complete(map[string]any{"taskId": "missing-task"}) }},
		{name: "block missing", call: func() (map[string]any, error) { return toolset.block(map[string]any{"taskId": "missing-task"}) }},
		{name: "delegate missing", call: func() (map[string]any, error) { return toolset.delegate(map[string]any{"taskId": "missing-task"}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if result, err := tc.call(); err == nil || result != nil || !strings.Contains(err.Error(), "task not found") {
				t.Fatalf("%s result = %#v err=%v, want task not found", tc.name, result, err)
			}
		})
	}
	complete, err := toolset.goalComplete(map[string]any{"resultSummary": "done via result summary"})
	if err != nil {
		t.Fatalf("goalComplete resultSummary: %v", err)
	}
	if complete["success"] != false || complete["status"] != "blocked" {
		t.Fatalf("goalComplete with open task = %#v, want blocked", complete)
	}
	doneStatus := "DONE"
	if _, err := runtime.Store().UpdateTask(ctx, ready.ID, TaskPatchRequest{Status: &doneStatus}); err != nil {
		t.Fatalf("UpdateTask ready done: %v", err)
	}
	complete, err = toolset.goalComplete(map[string]any{"resultSummary": "done via result summary"})
	if err != nil {
		t.Fatalf("goalComplete success resultSummary: %v", err)
	}
	if complete["success"] != true || complete["summary"] != "done via result summary" {
		t.Fatalf("goalComplete success = %#v, want resultSummary fallback", complete)
	}
	if snap := toolset.req.GoalDecision.snapshot(); snap.status != "complete" || snap.summary != "done via result summary" {
		t.Fatalf("goal decision = status:%q summary:%q", snap.status, snap.summary)
	}
}

func TestWorkflowPlannerAdditionalBoundaryBranches(t *testing.T) {
	tool, err := newWorkflowMapFunctionTool(workflowMapToolSpec{
		name:        "workflow.coverage.nil",
		description: "coverage",
		schema:      emptyObjectSchema(),
	})
	if err != nil {
		t.Fatalf("newWorkflowMapFunctionTool: %v", err)
	}
	runnable, ok := tool.(interface {
		Run(adkagent.Context, any) (map[string]any, error)
	})
	if !ok {
		t.Fatalf("workflow map tool type = %T, want runnable", tool)
	}
	mock := newGoogleADKToolTestContext()
	if result, err := runnable.Run(mock, map[string]any{}); err == nil || result != nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil workflow tool result = %#v err=%v, want unavailable", result, err)
	}
	if result, err := runnable.Run(mock, "bad"); err == nil || result != nil || !strings.Contains(err.Error(), "unexpected args type") {
		t.Fatalf("bad workflow tool args result = %#v err=%v, want args type error", result, err)
	}

	if got := plannerStringArg(map[string]any{"x": nil}, "x"); got != "" {
		t.Fatalf("plannerStringArg nil = %q, want empty", got)
	}
	if got := plannerStringArg(map[string]any{"x": "<nil>"}, "x"); got != "" {
		t.Fatalf("plannerStringArg <nil> = %q, want empty", got)
	}
	if got := plannerStringArg(map[string]any{"x": "  value  "}, "x"); got != "value" {
		t.Fatalf("plannerStringArg trim = %q, want value", got)
	}
	for _, tc := range []struct {
		name string
		args map[string]any
		want int
	}{
		{name: "nil", args: nil, want: 0},
		{name: "int64", args: map[string]any{"x": int64(12)}, want: 12},
		{name: "float64", args: map[string]any{"x": float64(12.9)}, want: 12},
		{name: "float32", args: map[string]any{"x": float32(7.9)}, want: 7},
		{name: "string", args: map[string]any{"x": " 42 "}, want: 42},
		{name: "bad", args: map[string]any{"x": "not-a-number"}, want: 0},
		{name: "nil string", args: map[string]any{"x": "<nil>"}, want: 0},
	} {
		if got := plannerIntArg(tc.args, "x"); got != tc.want {
			t.Fatalf("plannerIntArg %s = %d, want %d", tc.name, got, tc.want)
		}
	}

	unfinished := workflowPlanDraft{Warnings: []string{"keep"}}
	if steps, warnings, err := compileWorkflowPlanDraft(unfinished, WorkModeLoop, "msg", "msg", RunOptions{}); err == nil || steps != nil || len(warnings) != 1 {
		t.Fatalf("unfinished draft = steps:%#v warnings:%#v err:%v, want warning/error", steps, warnings, err)
	}
	empty := workflowPlanDraft{Finished: true, Steps: []workflowPlanDraftStep{{Title: "empty"}}}
	if steps, _, err := compileWorkflowPlanDraft(empty, WorkModeLoop, "msg", "msg", RunOptions{}); err == nil || steps != nil || !strings.Contains(err.Error(), "no valid steps") {
		t.Fatalf("empty draft = steps:%#v err:%v, want no valid steps", steps, err)
	}
	duplicate := workflowPlanDraft{Finished: true, Steps: []workflowPlanDraftStep{
		{Order: 2, Title: "B", Message: "run B"},
		{Order: 2, Title: "A", Message: "run A"},
		{Order: 0, Title: "C", Message: "run C"},
	}}
	steps, warnings, err := compileWorkflowPlanDraft(duplicate, WorkModeLoop, "user message", "different objective", RunOptions{})
	if err != nil {
		t.Fatalf("compile duplicate draft: %v", err)
	}
	if len(steps) != 1 || steps[0].Order != 1 || len(warnings) != 2 || !strings.Contains(warnings[0], "duplicated") || !strings.Contains(warnings[1], "loop workflow") {
		t.Fatalf("duplicate normalization steps=%#v warnings=%#v", steps, warnings)
	}
	loop := workflowPlanDraft{Finished: true, Steps: []workflowPlanDraftStep{
		{Title: "one", Message: "first"},
		{Title: "two", Message: "second"},
	}}
	loopSteps, loopWarnings, err := compileWorkflowPlanDraft(loop, WorkModeLoop, "msg", "msg", RunOptions{})
	if err != nil {
		t.Fatalf("compile loop draft: %v", err)
	}
	if len(loopSteps) != 1 || len(loopWarnings) != 1 || !strings.Contains(loopWarnings[0], "first planner step") {
		t.Fatalf("loop truncation steps=%#v warnings=%#v", loopSteps, loopWarnings)
	}
	depLoop := workflowPlanDraft{Finished: true, Steps: []workflowPlanDraftStep{{Title: "one", Message: "first", DependsOn: []string{"x"}}}}
	if _, _, err := compileWorkflowPlanDraft(depLoop, WorkModeLoop, "msg", "msg", RunOptions{}); err == nil || !strings.Contains(err.Error(), "must not depend") {
		t.Fatalf("loop dependency err = %v, want dependency error", err)
	}
	ambiguous := []workflowStep{
		{Title: "same", Message: "first", DependencyID: "a"},
		{Title: "same", Message: "second", DependencyID: "b", DependsOn: []string{"same"}},
	}
	if err := normalizeSequentialPlannerDependencies(ambiguous); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous dependency err = %v, want ambiguous", err)
	}
	aliases := map[string]int{"first": 0, "second": 1}
	resolved, err := resolveWorkflowStepDependencies([]string{" first ", "first", ""}, aliases, []workflowStep{{DependencyID: "dep-1"}, {DependencyID: "dep-2"}}, 1)
	if err != nil || len(resolved) != 1 || resolved[0] != "dep-1" {
		t.Fatalf("resolved duplicate deps = %#v err=%v, want dep-1", resolved, err)
	}
	if _, err := resolveWorkflowStepDependencies([]string{"missing"}, aliases, []workflowStep{{DependencyID: "dep-1"}}, 1); err == nil || !strings.Contains(err.Error(), "known step") {
		t.Fatalf("missing dep err = %v, want known step", err)
	}
	if _, err := resolveWorkflowStepDependencies([]string{"second"}, aliases, []workflowStep{{DependencyID: "dep-1"}, {DependencyID: "dep-2"}}, 1); err == nil || !strings.Contains(err.Error(), "earlier step") {
		t.Fatalf("future dep err = %v, want earlier step", err)
	}
}

func TestTimelineAdditionalBoundaryBranches(t *testing.T) {
	t1 := "2026-01-01T00:00:00Z"
	t2 := "2026-01-01T00:00:01Z"
	prompt := classifyWorkflowUserPrompt("请推进这个目标。\n总体目标：ship\n用户请求：build it")
	if !prompt.isInternal || prompt.isHidden || prompt.userMessage != "build it" || prompt.objective != "ship" {
		t.Fatalf("goal workflow prompt = %+v", prompt)
	}
	hidden := classifyWorkflowUserPrompt("请判断是否完成目标")
	if !hidden.isInternal || !hidden.isHidden {
		t.Fatalf("hidden prompt = %+v, want hidden internal", hidden)
	}
	if got := extractWorkflowPromptField("no marker", "missing:", ""); got != "" {
		t.Fatalf("missing prompt field = %q, want empty", got)
	}
	runs := []Run{
		{ID: "old", UserMessage: "build it", Objective: "ship", CreatedAt: t1, UpdatedAt: t1},
		{ID: "new", UserMessage: "build it", Objective: "ship", CreatedAt: t2, UpdatedAt: t2},
	}
	if run, ok := matchWorkflowPromptRun(prompt, runs); !ok || run.ID != "new" {
		t.Fatalf("matched run = %+v ok=%v, want newest", run, ok)
	}
	if _, ok := matchWorkflowPromptRun(workflowUserPrompt{isInternal: true, isHidden: true, userMessage: "build it"}, runs); ok {
		t.Fatal("hidden workflow prompt should not match")
	}
	session := Session{ID: "timeline-session"}
	messages := []TranscriptEntry{
		{ID: "hidden", SessionID: session.ID, Role: "user", Content: "请判断是否完成目标", CreatedAt: t1},
		{ID: "internal", SessionID: session.ID, Role: "user", Content: "请推进这个目标。\n总体目标：ship\n用户请求：build it", CreatedAt: t1},
		{ID: "dup-visible", SessionID: session.ID, RunID: "new", Role: "user", Content: "processed", CreatedAt: t2},
		{ID: "assistant-loose", SessionID: session.ID, Role: "assistant", Content: " loose final ", ReasoningContent: " loose reasoning ", CreatedAt: t2},
	}
	notice := TimelineEntry{ID: "notice", Kind: "", Text: "notice text", CreatedAt: t1, Status: "streaming"}
	entries := buildSessionTimeline(session, messages, runs, []TimelineEntry{notice, TimelineEntry{ID: "blank", Text: "   "}})
	var sawNotice, sawOriginal, sawLooseReasoning, sawLooseFinal bool
	for _, entry := range entries {
		switch {
		case entry.ID == "notice" && entry.Kind == TimelineKindContextNotice && entry.Status == "streaming":
			sawNotice = true
		case entry.Kind == TimelineKindUserMessage && entry.RunID == "new" && entry.Text == "build it" && entry.ProcessedText != "":
			sawOriginal = true
		case entry.ID == "assistant-loose:reasoning" && entry.Text == "loose reasoning":
			sawLooseReasoning = true
		case entry.ID == "assistant-loose" && entry.Text == "loose final":
			sawLooseFinal = true
		case entry.ID == "hidden":
			t.Fatal("hidden prompt should not be emitted")
		case entry.ID == "dup-visible":
			t.Fatal("duplicate visible user message should not be emitted")
		}
	}
	if !sawNotice || !sawOriginal || !sawLooseReasoning || !sawLooseFinal {
		t.Fatalf("timeline entries missing expected items: notice=%v original=%v reasoning=%v final=%v entries=%#v", sawNotice, sawOriginal, sawLooseReasoning, sawLooseFinal, entries)
	}
	run := Run{
		ID: "activity", CreatedAt: t2, UpdatedAt: t2,
		ToolCalls: []ToolCall{
			{ID: "tool-2", CreatedAt: t2, ToolName: "b"},
			{ID: "tool-1", CreatedAt: t1, ToolName: "a"},
		},
		PendingApprovals: []Approval{
			{ID: "approval-2", CreatedAt: t2, Status: ApprovalStatusPending},
			{ID: "approval-1", CreatedAt: t1, Status: ApprovalStatusPending},
			{ID: "approval-done", CreatedAt: t1, Status: ApprovalStatusApproved},
		},
		PreToolContent: "pre content", PreToolReasoning: "pre reasoning",
	}
	orphan := timelinePrimitivesForOrphanRun(session.ID, run)
	grouped := groupTimelinePrimitives(orphan)
	var toolGroup, approvalGroup *TimelineEntry
	for index := range grouped {
		switch grouped[index].Kind {
		case TimelineKindToolGroup:
			if toolGroup == nil {
				toolGroup = &grouped[index]
			}
		case TimelineKindApprovalGroup:
			if approvalGroup == nil {
				approvalGroup = &grouped[index]
			}
		}
	}
	if toolGroup == nil || len(toolGroup.ToolCalls) != 1 || toolGroup.ToolCalls[0].ID != "tool-1" {
		t.Fatalf("first tool group = %+v, want earliest tool call", toolGroup)
	}
	if approvalGroup == nil || len(approvalGroup.Approvals) != 1 || approvalGroup.Approvals[0].ID != "approval-1" {
		t.Fatalf("first approval group = %+v, want earliest pending approval", approvalGroup)
	}
	merged := groupTimelinePrimitives([]timelinePrimitive{
		{id: "tool:a", sessionID: session.ID, runID: "merge", kind: TimelineKindToolGroup, createdAt: t1, order: 40, toolCall: &ToolCall{ID: "a"}},
		{id: "tool:b", sessionID: session.ID, runID: "merge", kind: TimelineKindToolGroup, createdAt: t1, order: 40, toolCall: &ToolCall{ID: "b"}},
		{id: "approval:a", sessionID: session.ID, runID: "merge", kind: TimelineKindApprovalGroup, createdAt: t1, order: 50, approval: &Approval{ID: "a"}},
		{id: "approval:b", sessionID: session.ID, runID: "merge", kind: TimelineKindApprovalGroup, createdAt: t1, order: 50, approval: &Approval{ID: "b"}},
	})
	if len(merged) != 2 || len(merged[0].ToolCalls) != 2 || len(merged[1].Approvals) != 2 {
		t.Fatalf("merged primitives = %#v, want grouped tools and approvals", merged)
	}
	if got := runTextAnchor(Run{}, ""); got == "" {
		t.Fatal("empty runTextAnchor should fall back to nowString")
	}
	if got := stripTimelinePrefix("prefix rest", "prefix"); got != "rest" {
		t.Fatalf("stripTimelinePrefix partial = %q, want rest", got)
	}
	if got := stripTimelinePrefix("same", "same"); got != "" {
		t.Fatalf("stripTimelinePrefix exact = %q, want empty", got)
	}
	if !compareTimelineKeys("bad-a", 2, "b", "bad-b", 1, "a") {
		t.Fatal("invalid time keys should fall back to lexical time before order")
	}
	if compareTimelineKeys("", 1, "b", t1, 1, "a") {
		t.Fatal("valid right timestamp should sort before empty left timestamp")
	}
}

func TestGoogleADKWorkflowInputResponseBoundaryBranches(t *testing.T) {
	if got := googleADKWorkflowInputToUserContent(nil); got != nil {
		t.Fatalf("nil input content = %#v, want nil", got)
	}
	if got := googleADKWorkflowInputToUserContent(""); got != nil {
		t.Fatalf("empty input content = %#v, want nil", got)
	}
	content := genai.NewContentFromText("hello", genai.RoleUser)
	if got := googleADKWorkflowInputToUserContent(content); got != content {
		t.Fatal("content input should be returned unchanged")
	}
	if got := googleADKWorkflowInputToUserContent(func() {}); got != nil {
		t.Fatalf("unmarshalable input content = %#v, want nil", got)
	}
	jsonContent := googleADKWorkflowInputToUserContent(map[string]any{"a": 1})
	if jsonContent == nil || len(jsonContent.Parts) != 1 || !strings.Contains(jsonContent.Parts[0].Text, `"a":1`) {
		t.Fatalf("json input content = %+v", jsonContent)
	}
	mixed := genai.NewContentFromParts([]*genai.Part{
		nil,
		{Text: "ignore"},
		{FunctionResponse: &genai.FunctionResponse{ID: "", Name: adkworkflow.WorkflowInputFunctionCallName, Response: map[string]any{"response": "ignored"}}},
		{FunctionResponse: &genai.FunctionResponse{ID: "ask", Name: adkworkflow.WorkflowInputFunctionCallName, Response: map[string]any{"response": `{"ok":true}`}}},
		{FunctionResponse: &genai.FunctionResponse{ID: "approval", Name: toolconfirmation.FunctionCallName, Response: map[string]any{"payload": map[string]any{"confirmed": true}}}},
	}, genai.RoleUser)
	if !googleADKWorkflowHasFunctionResponse(mixed) {
		t.Fatal("mixed content should have function response")
	}
	inputs := googleADKWorkflowInputResponses(mixed)
	if decoded, ok := inputs["ask"].(map[string]any); !ok || decoded["ok"] != true {
		t.Fatalf("workflow input responses = %#v, want decoded ask response", inputs)
	}
	state := &adkworkflow.RunState{Nodes: map[string]*adkworkflow.NodeState{
		"nil":       nil,
		"completed": {Status: adkworkflow.NodeCompleted, Interrupts: []string{"done"}},
		"waiting":   {Status: adkworkflow.NodeWaiting, Interrupts: []string{"ask", ""}},
	}}
	resume := googleADKWorkflowResumeResponses(mixed, state, nil)
	if _, ok := resume["ask"]; !ok {
		t.Fatalf("resume responses = %#v, want ask", resume)
	}
	if _, ok := resume["approval"]; ok {
		t.Fatalf("resume responses = %#v, did not expect approval without open session call", resume)
	}
	ctx := context.Background()
	service := adksession.InMemoryService()
	created, err := service.Create(ctx, &adksession.CreateRequest{AppName: "app", UserID: "user", SessionID: "session"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	request := adksession.NewEvent(ctx, "invocation")
	request.Content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{ID: "approval", Name: toolconfirmation.FunctionCallName}}}, genai.RoleModel)
	request.LongRunningToolIDs = []string{"approval", ""}
	if err := service.AppendEvent(ctx, created.Session, request); err != nil {
		t.Fatalf("Append request: %v", err)
	}
	sess := created.Session
	open := googleADKWorkflowOpenLongRunningCallIDs(sess)
	if _, ok := open["approval"]; !ok {
		t.Fatalf("open long-running ids = %#v, want approval", open)
	}
	resume = googleADKWorkflowResumeResponses(mixed, nil, sess)
	if decoded, ok := resume["approval"].(map[string]any); !ok || decoded["confirmed"] != true {
		t.Fatalf("long-running resume responses = %#v, want approval payload", resume)
	}
	answeredBefore := googleADKWorkflowAnsweredOpenInterrupts(sess)
	if answeredBefore["approval"] {
		t.Fatalf("answered before response = %#v, want false", answeredBefore)
	}
	response := adksession.NewEvent(ctx, "invocation")
	response.Content = genai.NewContentFromParts([]*genai.Part{{FunctionResponse: &genai.FunctionResponse{
		ID: "approval", Name: toolconfirmation.FunctionCallName, Response: map[string]any{"confirmed": true},
	}}}, genai.RoleUser)
	if err := service.AppendEvent(ctx, sess, response); err != nil {
		t.Fatalf("Append response: %v", err)
	}
	answered := googleADKWorkflowAnsweredOpenInterrupts(sess)
	if !answered["approval"] {
		t.Fatalf("answered ids = %#v, want approval", answered)
	}
	if open := googleADKWorkflowOpenLongRunningCallIDs(sess); len(open) != 0 {
		t.Fatalf("open long-running after response = %#v, want empty", open)
	}
	if got := googleADKDecodeWorkflowInputResponse(&genai.FunctionResponse{Response: map[string]any{"response": "plain text"}}); got != "plain text" {
		t.Fatalf("plain response decode = %#v, want plain text", got)
	}
	if got := googleADKDecodeWorkflowInputResponse(&genai.FunctionResponse{Response: map[string]any{"other": "value"}}); fmt.Sprint(got) == "" {
		t.Fatalf("fallback response decode = %#v, want response map", got)
	}
	if got := googleADKWorkflowInputResponses(nil); got != nil {
		t.Fatalf("nil input responses = %#v, want nil", got)
	}
	if got := googleADKWorkflowResumeResponses(genai.NewContentFromText("none", genai.RoleUser), nil, nil); got != nil {
		t.Fatalf("no pending resume responses = %#v, want nil", got)
	}
	var nilSession adksession.Session
	if open := googleADKWorkflowOpenLongRunningCallIDs(nilSession); len(open) != 0 {
		t.Fatalf("nil session open ids = %#v, want empty", open)
	}
	if answered := googleADKWorkflowAnsweredOpenInterrupts(nilSession); len(answered) != 0 {
		t.Fatalf("nil session answered ids = %#v, want empty", answered)
	}
}
