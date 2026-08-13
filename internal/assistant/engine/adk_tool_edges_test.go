package adk

import (
	"context"
	"errors"
	"fmt"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	adkagent "google.golang.org/adk/v2/agent"
	adksession "google.golang.org/adk/v2/session"
	adkworkflow "google.golang.org/adk/v2/workflow"
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
	raw, err := runtime.ModelsListTool(t.Context(), map[string]any{"limit": 1, "callableOnly": "yes"})
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
	if err := providers.ValidateHostname(" "); err == nil || !strings.Contains(err.Error(), "host is required") {
		t.Fatalf("blank provider host err = %v, want required", err)
	}
	if err := providers.ValidateIP(netip.Addr{}); err == nil || !strings.Contains(err.Error(), "unspecified") {
		t.Fatalf("invalid provider IP err = %v, want unspecified", err)
	}
	if err := providers.ValidateIP(netip.MustParseAddr("224.0.0.1")); err == nil || !strings.Contains(err.Error(), "multicast") {
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
	client := providers.NewHTTPClientWithResolver(time.Second, func(context.Context, string, string) ([]netip.Addr, error) {
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

	blockedClient := providers.NewHTTPClientWithResolver(time.Second, func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
	})
	if _, err := blockedClient.Transport.(*http.Transport).DialContext(t.Context(), "tcp", "example.test:443"); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("provider DialContext blocked IP err = %v, want blocked address", err)
	}
	emptyClient := providers.NewHTTPClientWithResolver(time.Second, func(context.Context, string, string) ([]netip.Addr, error) {
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

}

func TestNormalizeAndApprovalResolutionBoundaryBranches(t *testing.T) {
	run := Run{ID: "run-resolution", ToolCalls: nil}
	parent := Run{ID: "parent-resolution", ToolCalls: nil}
	resolution := NormalizeApprovalResolution(ApprovalResolution{Run: &run, ParentRun: &parent})
	if resolution.Run == &run || resolution.ParentRun == &parent {
		t.Fatal("NormalizeApprovalResolution should copy run pointers")
	}
	if got := normalizeAnyMap(map[string]any{" ": "ignored"}); len(got) != 0 {
		t.Fatalf("normalizeAnyMap blank-only = %#v, want empty", got)
	}
}

func TestWorkflowTaskLocalHelperBoundaryBranches(t *testing.T) {
	var nilDecision *workflowGoalDecision
	nilDecision.Reset()
	nilDecision.BeginDecision()
	nilDecision.SetComplete("ignored")
	nilDecision.SetContinue("ignored")
	if nilDecision.DecisionPhase() {
		t.Fatal("nil decision should not be in decision phase")
	}
	if snap := nilDecision.Snapshot(); snap.Status != "" || snap.Summary != "" || snap.Reason != "" {
		t.Fatalf("nil decision snapshot status=%q summary=%q reason=%q, want empty", snap.Status, snap.Summary, snap.Reason)
	}
	decision := &workflowGoalDecision{}
	decision.BeginDecision()
	if !decision.DecisionPhase() {
		t.Fatal("decision should be in decision phase")
	}
	decision.SetComplete(" complete summary ")
	if snap := decision.Snapshot(); snap.Status != "complete" || snap.Summary != "complete summary" || snap.Reason != "" {
		t.Fatalf("complete decision snapshot status=%q summary=%q reason=%q", snap.Status, snap.Summary, snap.Reason)
	}
	decision.SetContinue(" continue reason ")
	if snap := decision.Snapshot(); snap.Status != "continue" || snap.Reason != "continue reason" || snap.Summary != "" {
		t.Fatalf("continue decision snapshot status=%q summary=%q reason=%q", snap.Status, snap.Summary, snap.Reason)
	}
	decision.Reset()
	if decision.DecisionPhase() {
		t.Fatal("reset decision should leave decision phase")
	}

	if run, changed := assistantmodel.PruneInterruptedGoalWorkflowToolCalls(Run{}); changed || len(run.ToolCalls) != 0 {
		t.Fatalf("empty prune = %+v changed=%v", run, changed)
	}
	pauseErr := assistantmodel.ErrUserGoalPauseRequested.Error()
	run, changed := assistantmodel.PruneInterruptedGoalWorkflowToolCalls(Run{
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

func TestWorkflowPlannerAdditionalBoundaryBranches(t *testing.T) {
	tool, err := NewWorkflowMapFunctionTool(WorkflowMapToolSpec{
		Name:        "workflow.coverage.nil",
		Description: "coverage",
		Schema:      assistantmodel.EmptyObjectSchema(),
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

	if got := assistantmodel.PlannerStringArg(map[string]any{"x": nil}, "x"); got != "" {
		t.Fatalf("plannerStringArg nil = %q, want empty", got)
	}
	if got := assistantmodel.PlannerStringArg(map[string]any{"x": "<nil>"}, "x"); got != "" {
		t.Fatalf("plannerStringArg <nil> = %q, want empty", got)
	}
	if got := assistantmodel.PlannerStringArg(map[string]any{"x": "  value  "}, "x"); got != "value" {
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
		if got := assistantmodel.PlannerIntArg(tc.args, "x"); got != tc.want {
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
	prompt := assistantmodel.ClassifyWorkflowUserPrompt("请推进这个目标。\n总体目标：ship\n用户请求：build it")
	if !prompt.IsInternal || prompt.IsHidden || prompt.UserMessage != "build it" || prompt.Objective != "ship" {
		t.Fatalf("goal workflow prompt = %+v", prompt)
	}
	hidden := assistantmodel.ClassifyWorkflowUserPrompt("请判断是否完成目标")
	if !hidden.IsInternal || !hidden.IsHidden {
		t.Fatalf("hidden prompt = %+v, want hidden internal", hidden)
	}
	if got := assistantmodel.ExtractWorkflowPromptField("no marker", "missing:", ""); got != "" {
		t.Fatalf("missing prompt field = %q, want empty", got)
	}
	runs := []Run{
		{ID: "old", UserMessage: "build it", Objective: "ship", CreatedAt: t1, UpdatedAt: t1},
		{ID: "new", UserMessage: "build it", Objective: "ship", CreatedAt: t2, UpdatedAt: t2},
	}
	if run, ok := assistantmodel.MatchWorkflowPromptRun(prompt, runs); !ok || run.ID != "new" {
		t.Fatalf("matched run = %+v ok=%v, want newest", run, ok)
	}
	if _, ok := assistantmodel.MatchWorkflowPromptRun(assistantmodel.WorkflowUserPrompt{IsInternal: true, IsHidden: true, UserMessage: "build it"}, runs); ok {
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
	entries := assistantmodel.BuildSessionTimeline(session, messages, runs, []TimelineEntry{notice, TimelineEntry{ID: "blank", Text: "   "}})
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
	orphan := assistantmodel.TimelinePrimitivesForOrphanRun(session.ID, run)
	grouped := assistantmodel.GroupTimelinePrimitives(orphan)
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
	merged := assistantmodel.GroupTimelinePrimitives([]assistantmodel.TimelinePrimitive{
		{ID: "tool:a", SessionID: session.ID, RunID: "merge", Kind: TimelineKindToolGroup, CreatedAt: t1, Order: 40, ToolCall: &ToolCall{ID: "a"}},
		{ID: "tool:b", SessionID: session.ID, RunID: "merge", Kind: TimelineKindToolGroup, CreatedAt: t1, Order: 40, ToolCall: &ToolCall{ID: "b"}},
		{ID: "approval:a", SessionID: session.ID, RunID: "merge", Kind: TimelineKindApprovalGroup, CreatedAt: t1, Order: 50, Approval: &Approval{ID: "a"}},
		{ID: "approval:b", SessionID: session.ID, RunID: "merge", Kind: TimelineKindApprovalGroup, CreatedAt: t1, Order: 50, Approval: &Approval{ID: "b"}},
	})
	if len(merged) != 2 || len(merged[0].ToolCalls) != 2 || len(merged[1].Approvals) != 2 {
		t.Fatalf("merged primitives = %#v, want grouped tools and approvals", merged)
	}
	if got := assistantmodel.RunTextAnchor(Run{}, ""); got == "" {
		t.Fatal("empty runTextAnchor should fall back to nowString")
	}
	if got := assistantmodel.StripTimelinePrefix("prefix rest", "prefix"); got != "rest" {
		t.Fatalf("stripTimelinePrefix partial = %q, want rest", got)
	}
	if got := assistantmodel.StripTimelinePrefix("same", "same"); got != "" {
		t.Fatalf("stripTimelinePrefix exact = %q, want empty", got)
	}
	if !assistantmodel.CompareTimelineKeys("bad-a", 2, "b", "bad-b", 1, "a") {
		t.Fatal("invalid time keys should fall back to lexical time before order")
	}
	if assistantmodel.CompareTimelineKeys("", 1, "b", t1, 1, "a") {
		t.Fatal("valid right timestamp should sort before empty left timestamp")
	}
}
