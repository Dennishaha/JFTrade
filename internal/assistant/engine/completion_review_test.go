package adk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/completionreview"
)

func TestCompletionReviewCompleteLeavesReplyUnchanged(t *testing.T) {
	runtime, agent, run, execution := newCompletionReviewFixture(t)
	var calls atomic.Int64
	runtime.completionReviewText = func(context.Context, Provider, string, string, string, string) (generatedTextResult, error) {
		calls.Add(1)
		return generatedTextResult{Text: `{"decision":"complete","confidence":0.98,"reasonCode":"answer_complete","continuation":""}`}, nil
	}

	updated, appended := runtime.maybeReviewChatCompletion(t.Context(), agent, run, execution)
	if appended || updated.ID != run.ID || execution.ResultForRun(run.ID).Reply != "已有完整回复。" {
		t.Fatalf("review updated=%+v appended=%v result=%+v", updated, appended, execution.ResultForRun(run.ID))
	}
	if calls.Load() != 1 {
		t.Fatalf("review calls=%d, want 1", calls.Load())
	}
	assertCompletionReviewAudit(t, runtime, run.ID, "complete", completionReviewReasonComplete, false)
}

func TestCompletionReviewHighConfidenceAppendUsesSameRunAndSyntheticFinalMessage(t *testing.T) {
	runtime, agent, run, execution := newCompletionReviewFixture(t)
	session := mustCreateSession(t, runtime, agent.ID, "Completion review")
	run.SessionID = session.ID
	mustSaveRun(t, runtime, run)
	deltas := make([]ChatDelta, 0, 1)
	execution.onDelta = func(delta ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	}
	runtime.completionReviewText = func(context.Context, Provider, string, string, string, string) (generatedTextResult, error) {
		return generatedTextResult{Text: `{"decision":"append","confidence":0.93,"reasonCode":"missing_action_plan","continuation":"直接行动方案：先核对风险敞口，再设置观察清单。"}`}, nil
	}

	run, appended := runtime.maybeReviewChatCompletion(t.Context(), agent, run, execution)
	if !appended {
		t.Fatal("high-confidence continuation was not appended")
	}
	if len(deltas) != 1 || deltas[0].Reply != "\n\n直接行动方案：先核对风险敞口，再设置观察清单。" {
		t.Fatalf("completion deltas=%+v", deltas)
	}
	result := reviewedExecutionResult(execution, run.ID, appended)
	if result.SourceEventID != "" || result.SyntheticKind != "completion_review" ||
		result.Reply != "已有完整回复。\n\n直接行动方案：先核对风险敞口，再设置观察清单。" {
		t.Fatalf("reviewed result=%+v", result)
	}
	run, _ = markCompletedChatRun(run)
	if err := runtime.PersistRunTerminalState(t.Context(), run); err != nil {
		t.Fatalf("PersistRunTerminalState: %v", err)
	}
	run, err := runtime.AttachFinalAssistantMessage(t.Context(), session, run, result)
	if err != nil {
		t.Fatalf("AttachFinalAssistantMessage: %v", err)
	}
	if run.FinalMessageID == "" {
		t.Fatal("completion review did not attach a final message")
	}
	messages := mustAssistantMessages(t, runtime, session.ID)
	if len(messages) != 1 || messages[0].RunID != run.ID || messages[0].Content != result.Reply {
		t.Fatalf("assistant messages=%+v, want one combined message", messages)
	}
	assertCompletionReviewAudit(t, runtime, run.ID, "append", completionReviewReasonMissingActionPlan, true)
}

func TestDefaultChatCompletionReviewAppendsAfterToolReplyInSSEOrder(t *testing.T) {
	runtime := newTestRuntime(t)
	var reviews atomic.Int64
	runtime.completionReviewText = func(context.Context, Provider, string, string, string, string) (generatedTextResult, error) {
		reviews.Add(1)
		return generatedTextResult{Text: `{"decision":"append","confidence":0.96,"reasonCode":"missing_direct_deliverable","continuation":"补齐的直接结论与行动清单。"}`}, nil
	}
	deltas := make([]ChatDelta, 0)
	response, err := runtime.ChatStream(t.Context(), ChatRequest{
		Message: `<execute-tool name="workflow.wait" parameters='{"durationMs":1}'> <execute-tool name="tools.search" parameters='{"query":"行情"}'>`,
	}, func(delta ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if reviews.Load() != 1 || response.Run.Status != RunStatusCompleted || len(response.Run.ToolCalls) != 2 {
		t.Fatalf("response=%+v reviews=%d", response, reviews.Load())
	}
	if !strings.Contains(response.Reply, "已完成 ADK 分析") || !strings.HasSuffix(response.Reply, "补齐的直接结论与行动清单。") {
		t.Fatalf("combined reply=%q", response.Reply)
	}
	lastReplyDelta := ""
	for _, delta := range deltas {
		if delta.Reply != "" {
			lastReplyDelta = delta.Reply
		}
	}
	if lastReplyDelta != "\n\n补齐的直接结论与行动清单。" {
		t.Fatalf("last reply delta=%q; all deltas=%+v", lastReplyDelta, deltas)
	}
	messages := mustAssistantMessages(t, runtime, response.Session.ID)
	if len(messages) != 1 || messages[0].Content != response.Reply || response.Run.FinalMessageID != messages[0].ID {
		t.Fatalf("messages=%+v run=%+v", messages, response.Run)
	}
	runs, total, err := runtime.Store().ListRunsPage(t.Context(), "", "", response.Session.ID, 10, 0)
	if err != nil || total != 1 || len(runs) != 1 || runs[0].ID != response.Run.ID {
		t.Fatalf("runs=%+v total=%d err=%v", runs, total, err)
	}
}

func TestCompletionReviewFailsOpen(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		err        error
		wantReason string
	}{
		{name: "low confidence", response: `{"decision":"append","confidence":0.84,"reasonCode":"deferred_safe_work","continuation":"More"}`, wantReason: "low_confidence"},
		{name: "invalid json", response: `{`, wantReason: "invalid_response"},
		{name: "unknown field", response: `{"decision":"complete","confidence":0.9,"reasonCode":"answer_complete","continuation":"","extra":true}`, wantReason: "invalid_response"},
		{name: "continuation too long", response: `{"decision":"append","confidence":0.99,"reasonCode":"deferred_safe_work","continuation":"` + strings.Repeat("x", completionreview.MaxCharacters+1) + `"}`, wantReason: "continuation_too_long"},
		{name: "timeout", err: context.DeadlineExceeded, wantReason: "timeout"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, agent, run, execution := newCompletionReviewFixture(t)
			runtime.completionReviewText = func(context.Context, Provider, string, string, string, string) (generatedTextResult, error) {
				return generatedTextResult{Text: test.response}, test.err
			}
			_, appended := runtime.maybeReviewChatCompletion(t.Context(), agent, run, execution)
			if appended || execution.ResultForRun(run.ID).Reply != "已有完整回复。" {
				t.Fatalf("fail-open appended=%v result=%+v", appended, execution.ResultForRun(run.ID))
			}
			wantOutcome := "failed"
			if test.wantReason == "low_confidence" {
				wantOutcome = "skipped"
			}
			assertCompletionReviewAudit(t, runtime, run.ID, wantOutcome, test.wantReason, false)
		})
	}
}

func TestCompletionReviewAnonymousPortfolioScenarioFinishesInOneRun(t *testing.T) {
	runtime, agent, run, execution := newCompletionReviewFixture(t)
	run.ToolCalls = append(run.ToolCalls, ToolCall{
		ID: "call-research", RunID: run.ID, ToolName: "research.instrument", Permission: "read_external", Status: "SUCCEEDED",
		Output: map[string]any{"raw": "must-not-enter-review-prompt"},
	})
	execution.calls = append([]ToolCall(nil), run.ToolCalls...)
	execution.reply.Reset()
	execution.reply.WriteString("账户判断：证券账户可查询持仓，基金账户不支持该查询。重点标的诊断：当前集中度偏高。你想先看哪项？如果需要我可以继续给行动方案。")
	runtime.completionReviewText = func(_ context.Context, _ Provider, _ string, _ string, _ string, prompt string) (generatedTextResult, error) {
		for _, expected := range []string{"诊断账户并给出直接行动方案", "portfolio.accounts", "portfolio.positions", "research.instrument", "SUCCEEDED"} {
			if !strings.Contains(prompt, expected) {
				t.Fatalf("review prompt missing %q: %s", expected, prompt)
			}
		}
		if strings.Contains(prompt, "must-not-enter-review-prompt") {
			t.Fatalf("review prompt included raw tool output: %s", prompt)
		}
		return generatedTextResult{Text: `{"decision":"append","confidence":0.97,"reasonCode":"deferred_safe_work","continuation":"行动方案：降低单一标的集中度，设置价格与风险观察清单，并在执行前复核账户可交易范围。"}`}, nil
	}

	_, appended := runtime.maybeReviewChatCompletion(t.Context(), agent, run, execution)
	result := reviewedExecutionResult(execution, run.ID, appended)
	if !appended || !strings.Contains(result.Reply, "账户判断") || !strings.Contains(result.Reply, "重点标的诊断") ||
		!strings.Contains(result.Reply, "行动方案") || result.SyntheticKind != "completion_review" {
		t.Fatalf("anonymous scenario result=%+v appended=%v", result, appended)
	}
	if run.Status != RunStatusRunning || run.InputRequest != nil || len(run.InputRequests) != 0 {
		t.Fatalf("review changed run control state: %+v", run)
	}
}

func TestCompletionReviewResponsesRequestIsBoundedStructuredAndToolFree(t *testing.T) {
	payloads := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer func() { jftradePanicOnError(request.Body.Close()) }()
		payload := map[string]any{}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		payloads <- payload
		writeTestResponsesMessage(w, openAIChatMessage{
			Role: "assistant", Content: `{"decision":"complete","confidence":0.99,"reasonCode":"answer_complete","continuation":""}`,
		}, false)
	}))
	defer server.Close()

	result, err := newResponsesClient().generateCompletionReview(t.Context(), Provider{
		ID: "review-provider", BaseURL: server.URL, Model: "review-model", Enabled: true,
	}, "sk-test", "review-model", "system", "prompt")
	if err != nil || !strings.Contains(result.Text, `"decision":"complete"`) {
		t.Fatalf("generateCompletionReview result=%+v err=%v", result, err)
	}
	payload := <-payloads
	if payload["model"] != "review-model" || payload["max_output_tokens"] != float64(1200) {
		t.Fatalf("completion review request model/bound=%+v", payload)
	}
	if tools, exists := payload["tools"]; exists && tools != nil {
		t.Fatalf("completion review request declared tools: %#v", tools)
	}
	if reasoning, exists := payload["reasoning"]; exists && reasoning != nil {
		t.Fatalf("completion review request declared reasoning: %#v", reasoning)
	}
	textConfig, ok := payload["text"].(map[string]any)
	if !ok {
		t.Fatalf("completion review text config=%#v", payload["text"])
	}
	format, ok := textConfig["format"].(map[string]any)
	if !ok || format["type"] != "json_schema" {
		t.Fatalf("completion review structured format=%#v", textConfig)
	}
}

func TestCompletionReviewEligibilityAndMemoBoundaries(t *testing.T) {
	t.Run("memo invokes reviewer and appends once", func(t *testing.T) {
		runtime, agent, run, execution := newCompletionReviewFixture(t)
		var calls atomic.Int64
		runtime.completionReviewText = func(context.Context, Provider, string, string, string, string) (generatedTextResult, error) {
			calls.Add(1)
			return generatedTextResult{Text: `{"decision":"append","confidence":0.99,"reasonCode":"deferred_safe_work","continuation":"补齐内容。"}`}, nil
		}
		var wait sync.WaitGroup
		for range 8 {
			wait.Go(func() {
				_, _ = runtime.maybeReviewChatCompletion(t.Context(), agent, run, execution)
			})
		}
		wait.Wait()
		if calls.Load() != 1 {
			t.Fatalf("review calls=%d, want exactly 1", calls.Load())
		}
		if got := strings.Count(execution.ResultForRun(run.ID).Reply, "补齐内容。"); got != 1 {
			t.Fatalf("continuation count=%d, reply=%q", got, execution.ResultForRun(run.ID).Reply)
		}
	})

	tests := []struct {
		name   string
		mutate func(*Agent, *Run)
	}{
		{name: "custom agent", mutate: func(agent *Agent, _ *Run) { agent.ID = "custom" }},
		{name: "loop", mutate: func(_ *Agent, run *Run) { run.WorkMode = WorkModeLoop }},
		{name: "workflow child", mutate: func(_ *Agent, run *Run) { run.ParentRunID = "parent" }},
		{name: "write tool", mutate: func(_ *Agent, run *Run) { run.ToolCalls[0].Permission = "write_strategy" }},
		{name: "approval pending", mutate: func(_ *Agent, run *Run) { run.PendingApprovals = []Approval{{Status: ApprovalStatusPending}} }},
		{name: "failed tool", mutate: func(_ *Agent, run *Run) { run.ToolCalls[0].Status = "FAILED" }},
		{name: "degraded", mutate: func(_ *Agent, run *Run) { run.Degraded = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, agent, run, execution := newCompletionReviewFixture(t)
			test.mutate(&agent, &run)
			var calls atomic.Int64
			runtime.completionReviewText = func(context.Context, Provider, string, string, string, string) (generatedTextResult, error) {
				calls.Add(1)
				return generatedTextResult{}, errors.New("must not run")
			}
			_, appended := runtime.maybeReviewChatCompletion(t.Context(), agent, run, execution)
			if appended || calls.Load() != 0 {
				t.Fatalf("ineligible review appended=%v calls=%d", appended, calls.Load())
			}
		})
	}
}

func newCompletionReviewFixture(t *testing.T) (*Runtime, Agent, Run, *googleADKExecution) {
	t.Helper()
	runtime := newTestRuntime(t)
	agent, ok, err := runtime.Store().Agent(t.Context(), DefaultBuiltinAgentID)
	if err != nil || !ok {
		t.Fatalf("default Agent ok=%v err=%v", ok, err)
	}
	completed := nowString()
	run := Run{
		ID: "completion-review-run", AgentID: agent.ID, ProviderID: testProviderID,
		Model: "test-model", WorkMode: WorkModeChat, Status: RunStatusRunning,
		UserMessage: "诊断账户并给出直接行动方案。",
		ToolCalls: []ToolCall{
			{ID: "call-accounts", RunID: "completion-review-run", ToolName: "portfolio.accounts", Permission: "read_internal", Status: "SUCCEEDED", CompletedAt: &completed},
			{ID: "call-positions", RunID: "completion-review-run", ToolName: "portfolio.positions", Permission: "read_external", Status: "SUCCEEDED", CompletedAt: &completed},
		},
		PendingApprovals: []Approval{}, CreatedAt: nowString(), UpdatedAt: nowString(),
	}
	execution := &googleADKExecution{
		runID: run.ID, agent: agent, calls: append([]ToolCall(nil), run.ToolCalls...),
		replyByRunID: map[string]*strings.Builder{}, reasoningByRunID: map[string]*strings.Builder{},
		bufferedReplyByRunID: map[string]*strings.Builder{}, bufferedReasoningByRunID: map[string]*strings.Builder{},
		toolResponseSeenByRunID: map[string]bool{}, postToolTextByRunID: map[string]bool{},
		finalMessageIDByRunID: map[string]string{},
	}
	execution.reply.WriteString("已有完整回复。")
	return runtime, agent, run, execution
}

func assertCompletionReviewAudit(t *testing.T, runtime *Runtime, runID string, outcome string, reasonCode string, appended bool) {
	t.Helper()
	for _, event := range mustAuditEvents(t, runtime) {
		if event.Kind != "run.completion_review" || event.SubjectID != runID {
			continue
		}
		if event.Metadata["outcome"] != outcome || event.Metadata["reasonCode"] != reasonCode || event.Metadata["appended"] != appended {
			t.Fatalf("completion review audit=%+v", event)
		}
		if _, leaked := event.Metadata["continuation"]; leaked {
			t.Fatalf("completion review audit leaked content: %+v", event.Metadata)
		}
		return
	}
	t.Fatalf("completion review audit not found for %s", runID)
}
