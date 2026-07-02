package adk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/genai"
)

func TestGoogleADKExecutionDescriptorRunMappingAndContentBoundaries(t *testing.T) {
	execution := &googleADKExecution{
		runID:            "root-run",
		runIDByAgentName: map[string]string{"child-agent": "child-run"},
		descriptors: map[string]ToolDescriptor{
			"market.candles": {Name: "market.candles", Permission: "read"},
		},
	}

	if contentHasText(nil) {
		t.Fatal("nil content unexpectedly has text")
	}
	if contentHasText(genai.NewContentFromParts([]*genai.Part{{Text: ""}, nil}, genai.RoleModel)) {
		t.Fatal("empty content unexpectedly has text")
	}
	if !contentHasText(genai.NewContentFromText("ready", genai.RoleModel)) {
		t.Fatal("text content was not detected")
	}

	if descriptor, ok := execution.descriptorForTool(boundaryGoogleTool{name: "market.candles"}); !ok || descriptor.Permission != "read" {
		t.Fatalf("descriptor fallback = %#v/%v", descriptor, ok)
	}
	if descriptor, ok := execution.descriptorForTool(boundaryGoogleTool{name: "missing"}); ok || descriptor.Name != "" {
		t.Fatalf("missing descriptor = %#v/%v", descriptor, ok)
	}
	if descriptor, ok := (&googleADKExecution{}).descriptorForTool(nil); ok || descriptor.Name != "" {
		t.Fatalf("nil descriptor = %#v/%v", descriptor, ok)
	}

	if got := execution.runIDForAgentName(" child-agent "); got != "child-run" {
		t.Fatalf("runIDForAgentName(child-agent) = %q", got)
	}
	if got := execution.runIDForAgentName("unknown"); got != "root-run" {
		t.Fatalf("runIDForAgentName(unknown) = %q", got)
	}
	if got := execution.agentNameForRunID(" child-run "); got != "child-agent" {
		t.Fatalf("agentNameForRunID(child-run) = %q", got)
	}
	if got := execution.agentNameForRunID(" "); got != "" {
		t.Fatalf("agentNameForRunID(empty) = %q", got)
	}
}

func TestGoogleADKExecutionToolCallReuseAndCompletionBoundaries(t *testing.T) {
	execution := &googleADKExecution{
		runID:            "root-run",
		runIDByAgentName: map[string]string{"child-agent": "child-run"},
	}
	execution.reply.WriteString("pre tool reply")
	execution.reasoning.WriteString("pre tool reasoning")

	call := execution.ensureCallForAgent("function-1", ToolDescriptor{Name: "market.candles", Permission: "read"}, map[string]any{"symbol": "TME"}, "child-agent")
	if call.RunID != "child-run" || call.ToolName != "market.candles" || call.Status != "RUNNING" {
		t.Fatalf("created call = %#v", call)
	}
	reused := execution.ensureCallForAgent("function-1", ToolDescriptor{Name: "ignored"}, map[string]any{"symbol": "BABA"}, "child-agent")
	if reused.ID != call.ID || len(execution.calls) != 1 {
		t.Fatalf("reused call = %#v calls=%#v", reused, execution.calls)
	}
	if execution.preToolContent.String() != "pre tool reply" || execution.preToolReasoning.String() != "pre tool reasoning" {
		t.Fatalf("pre-tool buffers = %q/%q", execution.preToolContent.String(), execution.preToolReasoning.String())
	}

	execution.finishCall("missing-call", nil, errors.New("ignored"))
	if execution.calls[0].Status != "RUNNING" {
		t.Fatalf("missing finish mutated calls: %#v", execution.calls)
	}
	execution.finishCall(call.ID, map[string]any{"close": 10.2}, nil)
	if execution.calls[0].Status != "SUCCEEDED" || execution.calls[0].Output == nil || len(execution.summaries) == 0 {
		t.Fatalf("successful finish = calls %#v summaries %#v", execution.calls, execution.summaries)
	}

	failed := execution.ensureCallForRun("function-2", ToolDescriptor{Name: "strategy.backtest"}, nil, "")
	execution.finishCall(failed.ID, nil, context.DeadlineExceeded)
	if execution.calls[1].Status != "TIMED_OUT" || execution.calls[1].Error == nil {
		t.Fatalf("timeout finish = %#v", execution.calls[1])
	}
}

func TestGoogleADKFinalReplySynthesisBoundaries(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	session, err := runtime.Store().CreateSession(ctx, "agent-final", "final synthesis")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	definition := Agent{
		ID: "agent-final", Name: "Final Agent", ProviderID: "missing-provider",
		Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	}
	execution := &googleADKExecution{
		runID:                   "run-final",
		runIDByAgentName:        map[string]string{googleADKAgentName(definition.ID): "run-final"},
		calls:                   []ToolCall{},
		postToolTextByRunID:     map[string]bool{},
		toolResponseSeenByRunID: map[string]bool{},
	}

	if err := runtime.ensureGoogleADKFinalReply(ctx, definition, session, execution, "run-final", "summarize the result"); err != nil {
		t.Fatalf("ensureGoogleADKFinalReply without tool calls = %v", err)
	}

	execution.calls = []ToolCall{{ID: "call-1", RunID: "run-final", ToolName: "market.candles", Status: "SUCCEEDED"}}
	execution.markToolResponseSeenForRun("run-final")
	execution.markPostToolTextForRun("run-final")
	if err := runtime.ensureGoogleADKFinalReply(ctx, definition, session, execution, "run-final", "summarize the result"); err != nil {
		t.Fatalf("ensureGoogleADKFinalReply with post-tool text = %v", err)
	}

	execution.postToolTextByRunID["run-final"] = false
	err = runtime.ensureGoogleADKFinalReply(ctx, definition, session, execution, "run-final", "summarize the result")
	if err == nil || !strings.Contains(err.Error(), "agent provider is unavailable") {
		t.Fatalf("ensureGoogleADKFinalReply missing provider err = %v", err)
	}
}

type boundaryGoogleTool struct {
	name string
}

func (tool boundaryGoogleTool) Name() string        { return tool.name }
func (tool boundaryGoogleTool) Description() string { return "boundary test tool" }
func (tool boundaryGoogleTool) IsLongRunning() bool { return false }
