package adk

import (
	"strings"
	"testing"

	adkagent "google.golang.org/adk/v2/agent"
	adkmodel "google.golang.org/adk/v2/model"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/genai"
)

type workflowToolRequestProcessor interface {
	ProcessRequest(adkagent.Context, *adkmodel.LLMRequest) error
}

type workflowToolRunner interface {
	Run(adkagent.Context, any) (map[string]any, error)
}

func TestWorkflowPlannerToolsetDraftLifecycleAndRequestInjection(t *testing.T) {
	if tools, err := (&workflowPlannerToolset{}).Tools(nil); err != nil || tools != nil {
		t.Fatalf("nil draft toolset tools = %#v err=%v, want nil nil", tools, err)
	}

	draft := &workflowPlanDraft{Mode: WorkModeTask, Objective: "研究 AAPL 风险"}
	tools, err := newWorkflowPlannerToolset(draft).Tools(nil)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("planner tools len = %d, want reset/add/finish", len(tools))
	}

	ctx := newGoogleADKToolTestContext()
	addTool := tools[1]
	if addTool.Name() != workflowPlanAddStepTool || addTool.Description() == "" || addTool.IsLongRunning() {
		t.Fatalf("add tool metadata = %s %q long=%v", addTool.Name(), addTool.Description(), addTool.IsLongRunning())
	}
	if declaration := workflowPlannerToolDeclaration(t, addTool); declaration == nil || declaration.Name != workflowPlanAddStepTool || declaration.ParametersJsonSchema == nil {
		t.Fatalf("add tool declaration = %#v", declaration)
	}

	req := &adkmodel.LLMRequest{}
	addProcessor := workflowPlannerToolRequestProcessor(t, addTool)
	if err := addProcessor.ProcessRequest(ctx, req); err != nil {
		t.Fatalf("ProcessRequest: %v", err)
	}
	if req.Tools[workflowPlanAddStepTool] != addTool {
		t.Fatalf("request tools = %#v, want add tool registered", req.Tools)
	}
	if req.Config == nil || len(req.Config.Tools) != 1 || len(req.Config.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("request config after process = %#v", req.Config)
	}
	if err := addProcessor.ProcessRequest(ctx, req); err == nil || !strings.Contains(err.Error(), "duplicate tool") {
		t.Fatalf("duplicate ProcessRequest err = %v", err)
	}

	finishTool := tools[2]
	finishProcessor := workflowPlannerToolRequestProcessor(t, finishTool)
	req = &adkmodel.LLMRequest{Config: &genai.GenerateContentConfig{Tools: []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{}}}}}
	if err := finishProcessor.ProcessRequest(ctx, req); err != nil {
		t.Fatalf("finish ProcessRequest existing function tool: %v", err)
	}
	if len(req.Config.Tools) != 1 || len(req.Config.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("finish request config = %#v", req.Config.Tools)
	}
	finishRunner := workflowPlannerToolRunner(t, finishTool)
	if _, err := finishRunner.Run(ctx, "bad-input"); err == nil || !strings.Contains(err.Error(), "unexpected args type") {
		t.Fatalf("invalid Run input err = %v", err)
	}

	addRunner := workflowPlannerToolRunner(t, addTool)
	if _, err := addRunner.Run(ctx, map[string]any{
		"order":           2,
		"title":           " 收集约束 ",
		"message":         " 读取持仓和行情 ",
		"description":     " 用真实账户状态规划 ",
		"modeHint":        "task",
		"dependsOn":       []any{" 1 ", "", "收集约束"},
		"agentRole":       " researcher ",
		"childProviderId": " provider-a ",
		"childModel":      " model-a ",
	}); err != nil {
		t.Fatalf("add planner step: %v", err)
	}
	if len(draft.Steps) != 1 || draft.Steps[0].Order != 2 || draft.Steps[0].Title != "收集约束" || len(draft.Steps[0].DependsOn) != 2 {
		t.Fatalf("draft after add = %+v", draft)
	}

	resetTool := tools[0]
	resetRunner := workflowPlannerToolRunner(t, resetTool)
	resetResult, err := resetRunner.Run(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("reset after add: %v", err)
	}
	if ignored, _ := resetResult["ignored"].(bool); !ignored || len(draft.Steps) != 1 || len(draft.Warnings) != 1 {
		t.Fatalf("reset after unfinished add result=%#v draft=%+v", resetResult, draft)
	}

	finishResult, err := finishRunner.Run(ctx, map[string]any{
		"mode":      "loop",
		"objective": "更新为循环检查",
		"warnings":  []any{" 需要人工确认 ", ""},
	})
	if err != nil {
		t.Fatalf("finish planner draft: %v", err)
	}
	if workflowPlannerNumberArg(finishResult, "steps") != 1 || !draft.Finished || draft.Mode != WorkModeLoop || draft.Objective != "更新为循环检查" {
		t.Fatalf("finish result=%#v draft=%+v", finishResult, draft)
	}
	if len(draft.Warnings) != 2 || draft.Warnings[1] != "需要人工确认" {
		t.Fatalf("warnings after finish = %#v", draft.Warnings)
	}

	resetResult, err = resetRunner.Run(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("reset after finish: %v", err)
	}
	if ignored, _ := resetResult["ignored"].(bool); ignored || len(draft.Steps) != 0 || draft.Mode != WorkModeLoop || draft.Objective != "更新为循环检查" {
		t.Fatalf("reset after finish result=%#v draft=%+v", resetResult, draft)
	}
}

func workflowPlannerToolRequestProcessor(t *testing.T, tool adktool.Tool) workflowToolRequestProcessor {
	t.Helper()
	processor, ok := tool.(workflowToolRequestProcessor)
	if !ok {
		t.Fatalf("%s does not implement ProcessRequest", tool.Name())
	}
	return processor
}

func workflowPlannerToolRunner(t *testing.T, tool adktool.Tool) workflowToolRunner {
	t.Helper()
	runner, ok := tool.(workflowToolRunner)
	if !ok {
		t.Fatalf("%s does not implement Run", tool.Name())
	}
	return runner
}

func workflowPlannerToolDeclaration(t *testing.T, tool adktool.Tool) *genai.FunctionDeclaration {
	t.Helper()
	declared, ok := tool.(workflowDeclaredTool)
	if !ok {
		t.Fatalf("%s does not expose a declaration", tool.Name())
	}
	return declared.Declaration()
}

func workflowPlannerNumberArg(args map[string]any, key string) int {
	switch value := args[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	default:
		return 0
	}
}

func TestWorkflowPlannerArgumentAndDependencyHelpers(t *testing.T) {
	if got := plannerIntArg(map[string]any{"order": int64(7)}, "order"); got != 7 {
		t.Fatalf("plannerIntArg int64 = %d", got)
	}
	if got := plannerIntArg(map[string]any{"order": float32(3.9)}, "order"); got != 3 {
		t.Fatalf("plannerIntArg float32 = %d", got)
	}
	if got := plannerIntArg(map[string]any{"order": "not-number"}, "order"); got != 0 {
		t.Fatalf("plannerIntArg bad string = %d", got)
	}
	if got := plannerStringArg(map[string]any{"title": nil}, "title"); got != "" {
		t.Fatalf("plannerStringArg nil = %q", got)
	}
	if workflowStepsHaveDependencies([]workflowStep{{DependsOn: []string{" "}}}) {
		t.Fatal("blank dependency should not count as a planner dependency")
	}
	if !workflowStepsHaveDependencies([]workflowStep{{DependsOn: []string{"__planner_step_1"}}}) {
		t.Fatal("non-empty dependency should count")
	}
}
