package adk

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"
)

func TestBuildInputRequestAndValidateAnswers(t *testing.T) {
	request, err := buildInputRequest("run-1", "agent-1", "call-1", requestUserToolArgs{
		Title: "Choose a plan",
		Questions: []requestUserToolQuestion{
			{
				Question: "Deployment mode?", AllowOther: true,
				Options: []requestUserToolOption{{Label: "Safe", Recommended: true}, {Label: "Fast", Description: "Less validation"}},
			},
			{
				Question: "Output format?",
				Options:  []requestUserToolOption{{Label: "Markdown"}, {Label: "JSON"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildInputRequest: %v", err)
	}
	if request.Questions[0].ID != "q1" || request.Questions[0].Options[1].ID != "q1-o2" || request.Questions[1].ID != "q2" {
		t.Fatalf("generated ids = %+v", request.Questions)
	}
	answers, err := validateInputAnswers(*request, []InputAnswer{
		{QuestionID: "q2", OptionID: "q2-o1"},
		{QuestionID: "q1", OtherText: "Balanced"},
	})
	if err != nil {
		t.Fatalf("validateInputAnswers: %v", err)
	}
	if answers[0].QuestionID != "q1" || answers[0].OtherText != "Balanced" || answers[1].QuestionID != "q2" {
		t.Fatalf("canonical answers = %+v", answers)
	}

	invalidCases := [][]InputAnswer{
		{{QuestionID: "q1", OptionID: "q1-o1"}},
		{{QuestionID: "q1", OptionID: "missing"}, {QuestionID: "q2", OptionID: "q2-o1"}},
		{{QuestionID: "q1", OptionID: "q1-o1", OtherText: "both"}, {QuestionID: "q2", OptionID: "q2-o1"}},
		{{QuestionID: "q1", OptionID: "q1-o1"}, {QuestionID: "q2", OtherText: "not allowed"}},
	}
	for _, submitted := range invalidCases {
		if _, err := validateInputAnswers(*request, submitted); !errors.Is(err, errInputRequestInvalid) {
			t.Fatalf("answers %+v error = %v, want invalid", submitted, err)
		}
	}
	_, err = buildInputRequest("run-1", "agent-1", "call-too-many-options", requestUserToolArgs{
		Questions: []requestUserToolQuestion{{
			Question: "Too many choices?",
			Options: []requestUserToolOption{
				{Label: "A"}, {Label: "B"}, {Label: "C"}, {Label: "D"},
			},
		}},
	})
	if !errors.Is(err, errInputRequestInvalid) {
		t.Fatalf("four options error = %v, want invalid", err)
	}
}

func TestInputRequestValidationAndErrorEdges(t *testing.T) {
	t.Run("nil tool", func(t *testing.T) {
		var tool *googleADKInputTool
		if tool.Name() != interactionRequestUserTool || tool.Description() == "" || tool.IsLongRunning() || tool.Declaration() != nil {
			t.Fatalf("nil tool accessors returned unexpected values")
		}
		if _, err := tool.Run(nil, nil); err == nil {
			t.Fatal("nil tool Run error = nil")
		}
		if err := tool.ProcessRequest(nil, nil); err == nil {
			t.Fatal("nil tool ProcessRequest error = nil")
		}
		if descriptor := tool.googleADKToolDescriptor(); descriptor.Name != interactionRequestUserTool {
			t.Fatalf("nil tool descriptor = %+v", descriptor)
		}
	})

	t.Run("build validation", func(t *testing.T) {
		cases := []requestUserToolArgs{
			{},
			{Questions: []requestUserToolQuestion{{Question: " ", Options: []requestUserToolOption{{Label: "A"}, {Label: "B"}}}}},
			{Questions: []requestUserToolQuestion{{Question: "Pick", Options: []requestUserToolOption{{Label: "A"}}}}},
			{Questions: []requestUserToolQuestion{{Question: "Pick", Options: []requestUserToolOption{{Label: "A"}, {Label: " "}}}}},
		}
		if _, err := buildInputRequest("", "agent", "call", cases[0]); !errors.Is(err, errInputRequestInvalid) {
			t.Fatalf("missing run error = %v", err)
		}
		for _, args := range cases {
			if _, err := buildInputRequest("run", "agent", "call", args); !errors.Is(err, errInputRequestInvalid) {
				t.Fatalf("args %+v error = %v", args, err)
			}
		}
	})

	t.Run("call decoding", func(t *testing.T) {
		calls := []*genai.FunctionCall{
			nil,
			{Name: "other"},
			{Name: interactionRequestUserTool, Args: map[string]any{"invalid": func() {}}},
			{Name: interactionRequestUserTool, Args: map[string]any{"questions": "invalid"}},
		}
		for _, call := range calls {
			if _, err := requestUserToolArgsFromCall(call); !errors.Is(err, errInputRequestInvalid) {
				t.Fatalf("call %#v error = %v", call, err)
			}
		}
	})

	t.Run("answer validation", func(t *testing.T) {
		request := InputRequest{Questions: []InputQuestion{
			{ID: "q1", AllowOther: true, Options: []InputOption{{ID: "q1-o1"}, {ID: "q1-o2"}}},
			{ID: "q2", Options: []InputOption{{ID: "q2-o1"}, {ID: "q2-o2"}}},
		}}
		cases := [][]InputAnswer{
			{{QuestionID: "", OptionID: "q1-o1"}, {QuestionID: "q2", OptionID: "q2-o1"}},
			{{QuestionID: "q1", OptionID: "q1-o1"}, {QuestionID: "q1", OptionID: "q1-o2"}},
			{{QuestionID: "q1"}, {QuestionID: "q2", OptionID: "q2-o1"}},
			{{QuestionID: "unknown", OptionID: "q1-o1"}, {QuestionID: "q2", OptionID: "q2-o1"}},
		}
		for _, answers := range cases {
			if _, err := validateInputAnswers(request, answers); !errors.Is(err, errInputRequestInvalid) {
				t.Fatalf("answers %+v error = %v", answers, err)
			}
		}
	})

	t.Run("error classification", func(t *testing.T) {
		cases := []struct {
			err  error
			kind string
		}{
			{errInputRequestInvalid, "invalid"},
			{errInputRequestNotFound, "not_found"},
			{errInputRequestAlreadyAnswered, "conflict"},
			{errInputRequestConflict, "conflict"},
			{errors.New("other"), "internal"},
		}
		for _, tc := range cases {
			if kind := InputRequestErrorKind(tc.err); kind != tc.kind {
				t.Fatalf("InputRequestErrorKind(%v) = %q, want %q", tc.err, kind, tc.kind)
			}
		}
	})
}

func TestResolveRunInputStoreErrors(t *testing.T) {
	var nilStore *Store
	if _, _, err := nilStore.ResolveRunInput(t.Context(), "run", InputResponseRequest{RequestID: "request"}); err == nil {
		t.Fatal("nil store error = nil")
	}
	runtime := newTestRuntime(t)
	if _, _, err := runtime.Store().ResolveRunInput(t.Context(), "", InputResponseRequest{}); !errors.Is(err, errInputRequestInvalid) {
		t.Fatalf("empty IDs error = %v", err)
	}
	if _, _, err := runtime.Store().ResolveRunInput(t.Context(), "missing", InputResponseRequest{RequestID: "request"}); !errors.Is(err, errInputRequestNotFound) {
		t.Fatalf("missing run error = %v", err)
	}
	mustSaveRun(t, runtime, Run{ID: "mismatch-run", Status: RunStatusPendingInput, CreatedAt: nowString(), UpdatedAt: nowString()})
	if _, _, err := runtime.Store().ResolveRunInput(t.Context(), "mismatch-run", InputResponseRequest{RequestID: "request"}); !errors.Is(err, errInputRequestConflict) {
		t.Fatalf("mismatched request error = %v", err)
	}

	t.Run("closed store", func(t *testing.T) {
		closed := newTestRuntime(t)
		if err := closed.Close(); err != nil {
			t.Fatal(err)
		}
		if _, _, err := closed.Store().ResolveRunInput(t.Context(), "run", InputResponseRequest{RequestID: "request"}); err == nil {
			t.Fatal("closed store error = nil")
		}
	})

	t.Run("missing table", func(t *testing.T) {
		broken := newTestRuntime(t)
		if _, err := broken.Store().db.ExecContext(t.Context(), `DROP TABLE `+tableRuns); err != nil {
			t.Fatal(err)
		}
		if _, _, err := broken.Store().ResolveRunInput(t.Context(), "run", InputResponseRequest{RequestID: "request"}); err == nil {
			t.Fatal("missing table error = nil")
		}
	})

	t.Run("invalid stored payload", func(t *testing.T) {
		broken := newTestRuntime(t)
		mustSaveRun(t, broken, Run{ID: "invalid-json-run", CreatedAt: nowString(), UpdatedAt: nowString()})
		if _, err := broken.Store().db.ExecContext(t.Context(), `UPDATE `+tableRuns+` SET payload_json = '{' WHERE id = ?`, "invalid-json-run"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := broken.Store().ResolveRunInput(t.Context(), "invalid-json-run", InputResponseRequest{RequestID: "request"}); err == nil {
			t.Fatal("invalid payload error = nil")
		}
	})

	for _, tc := range []struct {
		name    string
		trigger string
	}{
		{name: "update failure", trigger: `CREATE TRIGGER fail_input_update BEFORE UPDATE ON ` + tableRuns + ` BEGIN SELECT RAISE(FAIL, 'input update failed'); END`},
		{name: "ignored update", trigger: `CREATE TRIGGER ignore_input_update BEFORE UPDATE ON ` + tableRuns + ` BEGIN SELECT RAISE(IGNORE); END`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken := newTestRuntime(t)
			request, err := buildInputRequest("trigger-run", "agent", "call", requestUserToolArgs{
				Questions: []requestUserToolQuestion{{Question: "Pick", Options: []requestUserToolOption{{Label: "A"}, {Label: "B"}}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			mustSaveRun(t, broken, Run{
				ID: "trigger-run", Status: RunStatusPendingInput, InputRequest: request,
				CreatedAt: nowString(), UpdatedAt: nowString(),
			})
			if _, err := broken.Store().db.ExecContext(t.Context(), tc.trigger); err != nil {
				t.Fatal(err)
			}
			if _, _, err := broken.Store().ResolveRunInput(t.Context(), "trigger-run", InputResponseRequest{
				RequestID: request.ID, Answers: []InputAnswer{{QuestionID: "q1", OptionID: "q1-o1"}},
			}); err == nil {
				t.Fatal("triggered update error = nil")
			}
		})
	}
}

func TestPendingInputRequestConflictEdges(t *testing.T) {
	runtime := newTestRuntime(t)
	if requests, err := runtime.pendingInputRequests(t.Context(), nil); err != nil || requests != nil {
		t.Fatalf("nil execution requests=%v err=%v", requests, err)
	}
	session := mustCreateSession(t, runtime, "input-edge-agent", "Input edges")
	validArgs := map[string]any{
		"questions": []any{map[string]any{
			"question": "Pick", "options": []any{map[string]any{"label": "A"}, map[string]any{"label": "B"}},
		}},
	}

	t.Run("filters irrelevant and invalid events", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "input-filter-agent", "Filter input")
		mustSaveRun(t, runtime, Run{
			ID: "input-existing-run", SessionID: session.ID, AgentID: "input-filter-agent", Status: RunStatusRunning,
			InputRequest: &InputRequest{ID: "existing", RunID: "input-existing-run", FunctionCallID: "existing-call", Status: InputRequestStatusAnswered},
			CreatedAt:    nowString(), UpdatedAt: nowString(),
		})
		emptyEvent := newAssistantEvent("input-filter-run", nil, time.Now())
		emptyEvent.Content = nil
		appendADKEvent(t, runtime, "input-filter-agent", session.ID, emptyEvent)
		event := newAssistantEvent("input-filter-run", []*genai.Part{
			{Text: "ignore"},
			{FunctionCall: &genai.FunctionCall{ID: "untracked-call", Name: interactionRequestUserTool, Args: validArgs}},
			{FunctionCall: &genai.FunctionCall{ID: "malformed-call", Name: interactionRequestUserTool, Args: map[string]any{"questions": "invalid"}}},
			{FunctionCall: &genai.FunctionCall{ID: "invalid-call", Name: interactionRequestUserTool, Args: map[string]any{"questions": []any{}}}},
			{FunctionCall: &genai.FunctionCall{ID: "existing-call", Name: interactionRequestUserTool, Args: validArgs}},
		}, time.Now())
		event.LongRunningToolIDs = []string{"untracked-call", "malformed-call", "invalid-call", "existing-call"}
		appendADKEvent(t, runtime, "input-filter-agent", session.ID, event)
		execution := &googleADKExecution{
			sessionService: runtime.rawSessionService, appName: googleADKAppName("input-filter-agent"), sessionID: session.ID,
			agent: Agent{ID: "input-filter-agent"},
			calls: []ToolCall{
				{RunID: "input-filter-run", IdempotencyKey: "malformed-call"},
				{RunID: "input-filter-run", IdempotencyKey: "invalid-call"},
				{RunID: "input-existing-run", IdempotencyKey: "existing-call"},
			},
		}
		requests, err := runtime.pendingInputRequests(t.Context(), execution)
		if err != nil || len(requests) != 0 {
			t.Fatalf("filtered requests=%v err=%v", requests, err)
		}
	})

	t.Run("missing session", func(t *testing.T) {
		execution := &googleADKExecution{
			sessionService: runtime.rawSessionService, appName: googleADKAppName("input-edge-agent"), sessionID: "missing-session",
		}
		if _, err := runtime.pendingInputRequests(t.Context(), execution); err == nil {
			t.Fatal("missing session error = nil")
		}
	})

	t.Run("existing pending request", func(t *testing.T) {
		storedRequest, err := buildInputRequest("input-edge-run", "input-edge-agent", "old-call", requestUserToolArgs{
			Questions: []requestUserToolQuestion{{Question: "Existing", Options: []requestUserToolOption{{Label: "A"}, {Label: "B"}}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		mustSaveRun(t, runtime, Run{
			ID: "input-edge-run", SessionID: session.ID, AgentID: "input-edge-agent", Status: RunStatusPendingInput,
			InputRequest: storedRequest, CreatedAt: nowString(), UpdatedAt: nowString(),
		})
		event := newAssistantEvent("input-edge-run", []*genai.Part{{FunctionCall: &genai.FunctionCall{
			ID: "new-call", Name: interactionRequestUserTool, Args: validArgs,
		}}}, time.Now())
		event.LongRunningToolIDs = []string{"new-call"}
		appendADKEvent(t, runtime, "input-edge-agent", session.ID, event)
		execution := &googleADKExecution{
			sessionService: runtime.rawSessionService, appName: googleADKAppName("input-edge-agent"), sessionID: session.ID,
			agent: Agent{ID: "input-edge-agent"}, calls: []ToolCall{{RunID: "input-edge-run", IdempotencyKey: "new-call"}},
		}
		if _, err := runtime.pendingInputRequests(t.Context(), execution); !errors.Is(err, errInputRequestConflict) {
			t.Fatalf("existing pending request error = %v", err)
		}
	})

	t.Run("simultaneous requests", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "input-parallel-agent", "Parallel input")
		mustSaveRun(t, runtime, Run{
			ID: "input-parallel-run", SessionID: session.ID, AgentID: "input-parallel-agent", Status: RunStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(),
		})
		event := newAssistantEvent("input-parallel-run", []*genai.Part{
			{FunctionCall: &genai.FunctionCall{ID: "call-a", Name: interactionRequestUserTool, Args: validArgs}},
			{FunctionCall: &genai.FunctionCall{ID: "call-b", Name: interactionRequestUserTool, Args: validArgs}},
		}, time.Now())
		event.LongRunningToolIDs = []string{"call-a", "call-b"}
		appendADKEvent(t, runtime, "input-parallel-agent", session.ID, event)
		execution := &googleADKExecution{
			sessionService: runtime.rawSessionService, appName: googleADKAppName("input-parallel-agent"), sessionID: session.ID,
			agent: Agent{ID: "input-parallel-agent"},
			calls: []ToolCall{
				{RunID: "input-parallel-run", IdempotencyKey: "call-a"},
				{RunID: "input-parallel-run", IdempotencyKey: "call-b"},
			},
		}
		if _, err := runtime.pendingInputRequests(t.Context(), execution); !errors.Is(err, errInputRequestConflict) {
			t.Fatalf("simultaneous request error = %v", err)
		}
	})
}

func TestInputContinuationFailureIsPersisted(t *testing.T) {
	runtime := newTestRuntime(t)
	run := mustSaveRun(t, runtime, Run{
		ID: "input-continuation-failure", SessionID: "session", AgentID: "agent", Status: RunStatusRunning,
		InputRequest: &InputRequest{ID: "request", RunID: "input-continuation-failure", Status: InputRequestStatusAnswered},
		CreatedAt:    nowString(), UpdatedAt: nowString(),
	})
	runtime.failInputContinuation(t.Context(), run, nil)
	stored, ok, err := runtime.Store().Run(t.Context(), run.ID)
	if err != nil || !ok {
		t.Fatalf("stored failed run ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusFailed || stored.ResumeState != "input_resume_failed" || stored.ErrorCode == "" {
		t.Fatalf("stored failed run = %+v", stored)
	}
	sentinel := errors.New("sentinel")
	if defaultError(sentinel, "fallback") != sentinel || defaultError(nil, "fallback").Error() != "fallback" {
		t.Fatal("defaultError did not preserve the supplied error or fallback")
	}
	if _, err := runtime.ResolveInputAsync(t.Context(), "missing", InputResponseRequest{RequestID: "request"}); !errors.Is(err, errInputRequestNotFound) {
		t.Fatalf("missing ResolveInputAsync error = %v", err)
	}
	recovering := mustSaveRun(t, runtime, Run{
		ID: "input-missing-context", SessionID: "session", AgentID: "agent", Status: RunStatusRunning,
		InputRequest: &InputRequest{ID: "request", RunID: "input-missing-context", Status: InputRequestStatusAnswered},
		CreatedAt:    nowString(), UpdatedAt: nowString(),
	})
	runtime.continueResolvedInput(t.Context(), recovering.ID)
	failed, ok, err := runtime.Store().Run(t.Context(), recovering.ID)
	if err != nil || !ok || failed.Status != RunStatusFailed || failed.ResumeState != "input_resume_failed" {
		t.Fatalf("unrecoverable continuation run=%+v ok=%v err=%v", failed, ok, err)
	}
}

func TestResolveRunInputIsValidatedAndIdempotent(t *testing.T) {
	runtime := newTestRuntime(t)
	request, err := buildInputRequest("input-run", "agent", "call", requestUserToolArgs{
		Questions: []requestUserToolQuestion{{
			Question: "Pick one", AllowOther: true,
			Options: []requestUserToolOption{{Label: "A"}, {Label: "B"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustSaveRun(t, runtime, Run{
		ID: "input-run", SessionID: "session", AgentID: "agent", Status: RunStatusPendingInput,
		InputRequest: request, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, CreatedAt: nowString(), UpdatedAt: nowString(),
	})

	if _, changed, err := runtime.Store().ResolveRunInput(context.Background(), "input-run", InputResponseRequest{
		RequestID: request.ID, Answers: []InputAnswer{{QuestionID: "q1", OptionID: "missing"}},
	}); changed || !errors.Is(err, errInputRequestInvalid) {
		t.Fatalf("invalid resolve changed=%v err=%v", changed, err)
	}

	payload := InputResponseRequest{RequestID: request.ID, Answers: []InputAnswer{{QuestionID: "q1", OptionID: "q1-o2"}}}
	resolved, changed, err := runtime.Store().ResolveRunInput(context.Background(), "input-run", payload)
	if err != nil || !changed {
		t.Fatalf("resolve changed=%v err=%v", changed, err)
	}
	if resolved.Status != RunStatusRunning || resolved.ResumeState != "input_resuming" || resolved.InputRequest.Status != InputRequestStatusAnswered {
		t.Fatalf("resolved run = %+v", resolved)
	}
	if _, changed, err := runtime.Store().ResolveRunInput(context.Background(), "input-run", payload); err != nil || changed {
		t.Fatalf("idempotent resolve changed=%v err=%v", changed, err)
	}
	if _, _, err := runtime.Store().ResolveRunInput(context.Background(), "input-run", InputResponseRequest{
		RequestID: request.ID, Answers: []InputAnswer{{QuestionID: "q1", OtherText: "different"}},
	}); !errors.Is(err, errInputRequestAlreadyAnswered) {
		t.Fatalf("different second answer err=%v", err)
	}
}

func TestCancelPendingInputRunCancelsRequestAndRejectsLateAnswer(t *testing.T) {
	runtime := newTestRuntime(t)
	request, err := buildInputRequest("input-cancel-run", "agent", "call", requestUserToolArgs{
		Questions: []requestUserToolQuestion{{
			Question: "Pick one",
			Options:  []requestUserToolOption{{Label: "A"}, {Label: "B"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustSaveRun(t, runtime, Run{
		ID: "input-cancel-run", SessionID: "session", AgentID: "agent", Status: RunStatusPendingInput,
		InputRequest: request, ToolCalls: []ToolCall{{ID: "call", RunID: "input-cancel-run", Status: RunStatusPendingInput}},
		PendingApprovals: []Approval{}, CreatedAt: nowString(), UpdatedAt: nowString(),
	})

	cancelled, err := runtime.CancelRun(t.Context(), "input-cancel-run")
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if cancelled.Status != RunStatusCancelled || cancelled.InputRequest == nil || cancelled.InputRequest.Status != InputRequestStatusCancelled {
		t.Fatalf("cancelled run = %+v", cancelled)
	}
	stored, ok, err := runtime.Store().Run(t.Context(), cancelled.ID)
	if err != nil || !ok {
		t.Fatalf("stored cancelled run ok=%v err=%v", ok, err)
	}
	if len(stored.InputRequests) != 1 || stored.InputRequests[0].Status != InputRequestStatusCancelled {
		t.Fatalf("stored input history = %+v", stored.InputRequests)
	}
	if _, _, err := runtime.Store().ResolveRunInput(t.Context(), cancelled.ID, InputResponseRequest{
		RequestID: request.ID,
		Answers:   []InputAnswer{{QuestionID: "q1", OptionID: "q1-o1"}},
	}); !errors.Is(err, errInputRequestConflict) {
		t.Fatalf("late answer error = %v, want conflict", err)
	}
}

func TestInputRequestTimelinePersistsAnsweredCard(t *testing.T) {
	request, err := buildInputRequest("timeline-run", "agent", "call", requestUserToolArgs{
		Questions: []requestUserToolQuestion{{Question: "Pick", Options: []requestUserToolOption{{Label: "A"}, {Label: "B"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request.Status = InputRequestStatusAnswered
	request.Answers = []InputAnswer{{QuestionID: "q1", OptionID: "q1-o1"}}
	second := *normalizeInputRequest(request)
	second.ID = "input-second"
	second.FunctionCallID = "call-second"
	second.Status = InputRequestStatusPending
	second.Answers = []InputAnswer{}
	second.CreatedAt = "9999-01-01T00:00:00Z"
	second.UpdatedAt = second.CreatedAt
	entries := groupTimelinePrimitives(timelinePrimitivesForRunActivity("session", Run{
		ID: "timeline-run", InputRequest: &second, InputRequests: []InputRequest{*request, second},
		ToolCalls: []ToolCall{{ID: "call", RunID: "timeline-run", ToolName: interactionRequestUserTool, Status: RunStatusPendingInput}},
	}))
	if len(entries) != 2 || entries[0].Kind != TimelineKindInputRequest || entries[0].InputRequest == nil || entries[0].InputRequest.Status != InputRequestStatusAnswered || entries[1].InputRequest == nil || entries[1].InputRequest.Status != InputRequestStatusPending {
		t.Fatalf("timeline entries = %+v", entries)
	}
}

func TestRequestUserToolIsLongRunning(t *testing.T) {
	registry := NewToolRegistry()
	registered, ok := registry.Get(interactionRequestUserTool)
	if !ok {
		t.Fatalf("registry is missing %q", interactionRequestUserTool)
	}
	options, ok := registered.Descriptor.InputSchema["properties"].(map[string]any)["questions"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["options"].(map[string]any)
	if !ok || options["maxItems"] != maxInputRequestOptions {
		t.Fatalf("input tool options schema = %#v, want maxItems %d", options, maxInputRequestOptions)
	}
	tool, err := newGoogleADKInputTool()
	if err != nil {
		t.Fatalf("newGoogleADKInputTool: %v", err)
	}
	if tool.Name() != interactionRequestUserTool || !tool.IsLongRunning() {
		t.Fatalf("tool name=%q long=%v", tool.Name(), tool.IsLongRunning())
	}
	declaration := tool.Declaration()
	if declaration == nil || !strings.Contains(declaration.Description, "two or three options") {
		t.Fatalf("input tool declaration = %#v, want three-option guidance", declaration)
	}
	declarationSchema, err := json.Marshal(declaration.ParametersJsonSchema)
	if err != nil || !strings.Contains(string(declarationSchema), `"maxItems":3`) {
		t.Fatalf("input tool declaration schema = %s, err=%v, want maxItems 3", declarationSchema, err)
	}
}

func TestRequestUserToolPausesAndResumesChatRun(t *testing.T) {
	runtime := newTestRuntime(t)
	agent, err := runtime.Store().SaveAgent(t.Context(), AgentWriteRequest{
		ID: "input-chat-agent", Name: "Input Chat", ProviderID: testProviderID,
		PermissionMode: PermissionModeAll, WorkMode: WorkModeChat, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(t.Context(), ChatRequest{AgentID: agent.ID, Message: "@input.required decide"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if response.Run.Status != RunStatusPendingInput || response.InputRequest == nil || len(response.InputRequest.Questions) != 2 {
		t.Fatalf("pending response = %+v", response)
	}
	if response.InputRequest.Questions[0].Options[0].ID != "q1-o1" || !response.InputRequest.Questions[0].Options[0].Recommended {
		t.Fatalf("questions = %+v", response.InputRequest.Questions)
	}
	_, err = runtime.ResolveInputAsync(t.Context(), response.Run.ID, InputResponseRequest{
		RequestID: response.InputRequest.ID,
		Answers: []InputAnswer{
			{QuestionID: "q1", OtherText: "Balanced"},
			{QuestionID: "q2", OptionID: "q2-o1"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveInputAsync: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, ok, loadErr := runtime.Store().Run(t.Context(), response.Run.ID)
		if loadErr != nil {
			t.Fatalf("Run: %v", loadErr)
		}
		if ok && run.Status == RunStatusCompleted {
			if run.InputRequest == nil || run.InputRequest.Status != InputRequestStatusAnswered || len(run.InputRequest.Answers) != 2 {
				t.Fatalf("completed input request = %+v", run.InputRequest)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	run, _, _ := runtime.Store().Run(t.Context(), response.Run.ID)
	t.Fatalf("run did not complete after input: %+v", run)
}

func TestRequestUserToolSupportsSequentialQuestionsInOneRun(t *testing.T) {
	runtime := newTestRuntime(t)
	agent, err := runtime.Store().SaveAgent(t.Context(), AgentWriteRequest{
		ID: "input-twice-agent", Name: "Input Twice", ProviderID: testProviderID,
		PermissionMode: PermissionModeAll, WorkMode: WorkModeChat, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := runtime.Chat(t.Context(), ChatRequest{AgentID: agent.ID, Message: "@input.twice decide"})
	if err != nil || response.Run.Status != RunStatusPendingInput || response.InputRequest == nil {
		t.Fatalf("first question response=%+v err=%v", response, err)
	}
	firstID := response.InputRequest.ID
	if _, err := runtime.ResolveInputAsync(t.Context(), response.Run.ID, InputResponseRequest{
		RequestID: firstID, Answers: []InputAnswer{{QuestionID: "q1", OptionID: "q1-o1"}},
	}); err != nil {
		t.Fatalf("resolve first: %v", err)
	}

	second := waitForInputRequest(t, runtime, response.Run.ID, firstID)
	if second.ID == firstID || second.Status != InputRequestStatusPending {
		t.Fatalf("second request = %+v", second)
	}
	if _, err := runtime.ResolveInputAsync(t.Context(), response.Run.ID, InputResponseRequest{
		RequestID: second.ID, Answers: []InputAnswer{{QuestionID: "q1", OtherText: "custom"}},
	}); err != nil {
		t.Fatalf("resolve second: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, ok, loadErr := runtime.Store().Run(t.Context(), response.Run.ID)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if ok && run.Status == RunStatusCompleted {
			if len(run.InputRequests) != 2 || run.InputRequests[0].Status != InputRequestStatusAnswered || run.InputRequests[1].Status != InputRequestStatusAnswered {
				t.Fatalf("input history = %+v", run.InputRequests)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("run did not complete after second answer")
}

func TestRequestUserToolCanTransitionToApproval(t *testing.T) {
	runtime, executions := newWorkflowApprovalRuntime(t, WorkModeChat)
	agent, err := runtime.Store().SaveAgent(t.Context(), AgentWriteRequest{
		ID: "input-approval-agent", Name: "Input Approval", ProviderID: testProviderID,
		PermissionMode: PermissionModeApproval, WorkMode: WorkModeChat, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := runtime.Chat(t.Context(), ChatRequest{AgentID: agent.ID, Message: "@input.approval decide"})
	if err != nil || response.Run.Status != RunStatusPendingInput || response.InputRequest == nil {
		t.Fatalf("input response=%+v err=%v", response, err)
	}
	if _, err := runtime.ResolveInputAsync(t.Context(), response.Run.ID, InputResponseRequest{
		RequestID: response.InputRequest.ID,
		Answers:   []InputAnswer{{QuestionID: "q1", OptionID: "q1-o1"}},
	}); err != nil {
		t.Fatalf("ResolveInputAsync: %v", err)
	}
	var pending Run
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		candidate, ok, loadErr := runtime.Store().Run(t.Context(), response.Run.ID)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if ok && candidate.Status == RunStatusPending && len(candidate.PendingApprovals) == 1 {
			pending = candidate
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pending.InputRequest == nil || pending.InputRequest.Status != InputRequestStatusAnswered || len(pending.PendingApprovals) != 1 || executions.Load() != 0 {
		t.Fatalf("pending approval run=%+v executions=%d", pending, executions.Load())
	}
	if _, err := runtime.ResolveApprovalAsync(t.Context(), pending.PendingApprovals[0].ID, true); err != nil {
		t.Fatalf("ResolveApprovalAsync: %v", err)
	}
	completed := waitForRunStatus(t, runtime, response.Run.ID, RunStatusCompleted)
	if completed.InputRequest == nil || completed.InputRequest.Status != InputRequestStatusAnswered || executions.Load() != 1 {
		t.Fatalf("completed run=%+v executions=%d", completed, executions.Load())
	}
}

func TestRequestUserToolResumesAfterRuntimeRestart(t *testing.T) {
	runtime := newTestRuntime(t)
	agent, err := runtime.Store().SaveAgent(t.Context(), AgentWriteRequest{
		ID: "input-restart-agent", Name: "Input Restart", ProviderID: testProviderID,
		PermissionMode: PermissionModeAll, WorkMode: WorkModeChat, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := runtime.Chat(t.Context(), ChatRequest{AgentID: agent.ID, Message: "@input.required decide"})
	if err != nil || response.InputRequest == nil {
		t.Fatalf("Chat response=%+v err=%v", response, err)
	}
	restarted := newRuntimeWithRegistry(t, runtime.Store(), NewToolRegistry())
	if _, err := restarted.ResolveInputAsync(t.Context(), response.Run.ID, InputResponseRequest{
		RequestID: response.InputRequest.ID,
		Answers:   []InputAnswer{{QuestionID: "q1", OptionID: "q1-o1"}, {QuestionID: "q2", OptionID: "q2-o2"}},
	}); err != nil {
		t.Fatalf("ResolveInputAsync after restart: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, ok, loadErr := restarted.Store().Run(t.Context(), response.Run.ID)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if ok && run.Status == RunStatusCompleted {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	run, _, _ := restarted.Store().Run(t.Context(), response.Run.ID)
	t.Fatalf("restarted run did not complete: %+v", run)
}

func waitForInputRequest(t *testing.T, runtime *Runtime, runID string, previousID string) InputRequest {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, ok, err := runtime.Store().Run(t.Context(), runID)
		if err != nil {
			t.Fatal(err)
		}
		if ok && run.Status == RunStatusPendingInput && run.InputRequest != nil && run.InputRequest.ID != previousID {
			return *run.InputRequest
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("next input request did not appear")
	return InputRequest{}
}
