package adk

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	adkagent "google.golang.org/adk/v2/agent"
	adkmodel "google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

func TestGoogleHelperBoundaryCoverage(t *testing.T) {
	t.Run("new chat request marshal error", func(t *testing.T) {
		model := &openAICompatibleADKModel{provider: Provider{BaseURL: "https://example.test"}}
		_, err := model.newChatRequest(context.Background(), openAIChatRequest{
			Model:      "test-model",
			ToolChoice: make(chan int),
		})
		if err == nil {
			t.Fatal("newChatRequest accepted unsupported tool_choice payload")
		}
	})

	t.Run("decode chat response read error", func(t *testing.T) {
		model := &openAICompatibleADKModel{}
		readErr := errors.New("read failed")
		_, err := model.decodeChatResponse(streamErrorReader{err: readErr})
		if !errors.Is(err, readErr) {
			t.Fatalf("decodeChatResponse err = %v, want %v", err, readErr)
		}
	})

	t.Run("stream aggregation state handles cancellation and reasoning only final replies", func(t *testing.T) {
		var state openAIStreamAggregationState
		err := state.consume(openAIChatStreamDelta{Content: "partial"}, func(*adkmodel.LLMResponse, error) bool {
			return false
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("consume cancel err = %v, want context.Canceled", err)
		}

		err = state.consumeMessage(openAIChatMessage{Content: "message"}, func(*adkmodel.LLMResponse, error) bool {
			return false
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("consumeMessage cancel err = %v, want context.Canceled", err)
		}

		var empty openAIStreamAggregationState
		final, err := empty.finalResponse()
		if err != nil || final != nil {
			t.Fatalf("empty finalResponse = %#v, %v, want nil,nil", final, err)
		}

		var reasoningOnly openAIStreamAggregationState
		reasoningOnly.reasoning.WriteString("need more thought")
		final, err = reasoningOnly.finalResponse()
		if err != nil {
			t.Fatalf("reasoningOnly.finalResponse: %v", err)
		}
		if final == nil || final.Content == nil || len(final.Content.Parts) != 1 || !final.Content.Parts[0].Thought {
			t.Fatalf("reasoning-only final response = %#v", final)
		}
	})

	t.Run("genai conversion helpers cover nil and malformed payload fallbacks", func(t *testing.T) {
		if got := openAIMessagesFromGenAI(nil); got != nil {
			t.Fatalf("openAIMessagesFromGenAI(nil) = %#v, want nil", got)
		}

		content := genai.NewContentFromParts([]*genai.Part{
			{Text: " visible "},
			{Text: "hidden", Thought: true},
			{FunctionCall: &genai.FunctionCall{
				ID:   "call-bad",
				Name: "tool.bad",
				Args: map[string]any{"broken": make(chan int)},
			}},
			{FunctionResponse: &genai.FunctionResponse{
				ID:       "call-bad",
				Name:     "tool.bad",
				Response: map[string]any{"broken": make(chan int)},
			}},
		}, genai.RoleModel)
		messages := openAIMessagesFromGenAI(content)
		if len(messages) != 2 {
			t.Fatalf("openAIMessagesFromGenAI mixed messages = %#v, want assistant+tool", messages)
		}
		if messages[0].Role != "assistant" || messages[0].ReasoningContent != "hidden" || len(messages[0].ToolCalls) != 1 {
			t.Fatalf("assistant message = %#v", messages[0])
		}
		if messages[1].Role != "tool" || messages[1].ToolCallID != "call-bad" || messages[1].Name != "tool.bad" {
			t.Fatalf("tool message = %#v", messages[1])
		}

		if got := genAIContentText(nil); got != "" {
			t.Fatalf("genAIContentText(nil) = %q, want empty", got)
		}
		if got := genAIContentText(genai.NewContentFromParts([]*genai.Part{
			{Text: " one "},
			{Text: "two "},
		}, genai.RoleUser)); got != "one two" {
			t.Fatalf("genAIContentText = %q, want one two", got)
		}
	})

	t.Run("schema sanitization preserves supported fields and strips unsupported ones", func(t *testing.T) {
		if sanitizeSchemaForOpenAI(nil) != nil {
			t.Fatal("sanitizeSchemaForOpenAI(nil) should be nil")
		}

		sanitized := sanitizeSchemaForOpenAI(map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"properties": map[string]any{
				"strict": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
				},
				"raw": "value",
			},
			"items": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"examples": []string{"keep"},
		})
		if _, ok := sanitized["additionalProperties"]; ok {
			t.Fatalf("sanitizeSchemaForOpenAI kept unsupported additionalProperties: %#v", sanitized)
		}
		properties, ok := sanitized["properties"].(map[string]any)
		if !ok {
			t.Fatalf("properties = %#v, want object", sanitized["properties"])
		}
		strict, ok := properties["strict"].(map[string]any)
		if !ok || strict["additionalProperties"] != false {
			t.Fatalf("strict property = %#v, want nested additionalProperties=false kept", properties["strict"])
		}
		if properties["raw"] != "value" {
			t.Fatalf("raw property = %#v, want value", properties["raw"])
		}
		items, ok := sanitized["items"].(map[string]any)
		if !ok {
			t.Fatalf("items = %#v, want object", sanitized["items"])
		}
		if _, ok := items["additionalProperties"]; ok {
			t.Fatalf("items additionalProperties should be stripped: %#v", items)
		}
		if _, ok := sanitized["examples"]; !ok {
			t.Fatalf("examples unexpectedly removed: %#v", sanitized)
		}

		nonObject := sanitizeSchemaForOpenAI(map[string]any{
			"properties": "raw-properties",
			"items":      "raw-items",
		})
		if nonObject["properties"] != "raw-properties" || nonObject["items"] != "raw-items" {
			t.Fatalf("non-object schema fields = %#v", nonObject)
		}
	})
}

func TestOpenAIHelperBoundaryCoverage(t *testing.T) {
	t.Run("truncateBytes preserves utf8 rune boundaries", func(t *testing.T) {
		const marker = "\n...(truncated)"
		got := truncateBytes(strings.Repeat("€", 10), len(marker)+2)
		if !strings.HasSuffix(got, marker) {
			t.Fatalf("truncateBytes suffix = %q, want truncation marker", got)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("truncateBytes returned invalid utf8: %q", got)
		}
	})

	t.Run("readStreamingResponse surfaces scanner errors", func(t *testing.T) {
		readErr := errors.New("stream failed")
		_, err := (openAIClient{}).readStreamingResponse(streamErrorReader{err: readErr}, nil)
		if !errors.Is(err, readErr) {
			t.Fatalf("readStreamingResponse err = %v, want %v", err, readErr)
		}
	})

	t.Run("readStreamingResponse flush tail delta errors", func(t *testing.T) {
		body := strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"<thi\"}}]}\n")
		_, err := (openAIClient{}).readStreamingResponse(body, func(delta ChatDelta) error {
			if delta.Reply != "<thi" {
				t.Fatalf("tail delta = %+v, want reply <thi", delta)
			}
			return errors.New("tail delta failed")
		})
		if err == nil || !strings.Contains(err.Error(), "tail delta failed") {
			t.Fatalf("readStreamingResponse tail err = %v, want tail delta failed", err)
		}
	})
}

func TestGoogleTransportBoundaryCoverage(t *testing.T) {
	req := &adkmodel.LLMRequest{
		Model:    "gpt-test",
		Contents: []*genai.Content{genai.NewContentFromText("hello", genai.RoleUser)},
	}

	t.Run("generateStream surfaces transport errors after request creation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := server.URL
		server.Close()
		model := newOpenAICompatibleADKModel(Provider{BaseURL: url, Model: "gpt-test"}, "test-key", "gpt-test")
		err := model.(*openAICompatibleADKModel).generateStream(context.Background(), req, func(*adkmodel.LLMResponse, error) bool {
			return true
		})
		if err == nil {
			t.Fatal("generateStream transport err = nil, want dial error")
		}

		invalidURLModel := &openAICompatibleADKModel{provider: Provider{BaseURL: "://bad url", Model: "gpt-test"}, apiKey: "test-key", model: "gpt-test"}
		err = invalidURLModel.generateStream(context.Background(), req, func(*adkmodel.LLMResponse, error) bool {
			return true
		})
		if err == nil {
			t.Fatal("generateStream accepted invalid request url")
		}
	})

	t.Run("generateStream falls back to decode json responses and surfaces malformed bodies", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{bad-json"))
		}))
		defer server.Close()
		model := newOpenAICompatibleADKModel(Provider{BaseURL: server.URL, Model: "gpt-test"}, "test-key", "gpt-test")
		err := model.(*openAICompatibleADKModel).generateStream(context.Background(), req, func(*adkmodel.LLMResponse, error) bool {
			return true
		})
		if err == nil || !strings.Contains(err.Error(), "decode OpenAI-compatible ADK response") {
			t.Fatalf("generateStream fallback err = %v, want decode error", err)
		}
	})

	t.Run("generateStream ignores blank stream payloads and reports trailing malformed chunks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data:   \n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}))
		defer server.Close()
		model := newOpenAICompatibleADKModel(Provider{BaseURL: server.URL, Model: "gpt-test"}, "test-key", "gpt-test")
		var count int
		err := model.(*openAICompatibleADKModel).generateStream(context.Background(), req, func(*adkmodel.LLMResponse, error) bool {
			count++
			return true
		})
		if err != nil || count == 0 {
			t.Fatalf("generateStream blank event err=%v count=%d, want success with output", err, count)
		}

		badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {bad-json\n"))
		}))
		defer badServer.Close()
		model = newOpenAICompatibleADKModel(Provider{BaseURL: badServer.URL, Model: "gpt-test"}, "test-key", "gpt-test")
		err = model.(*openAICompatibleADKModel).generateStream(context.Background(), req, func(*adkmodel.LLMResponse, error) bool {
			return true
		})
		if err == nil || !strings.Contains(err.Error(), "decode OpenAI-compatible ADK stream chunk") {
			t.Fatalf("generateStream trailing err = %v, want trailing decode error", err)
		}

		messageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"message\":{\"content\":\"stop here\"}}]}\n\n"))
		}))
		defer messageServer.Close()
		model = newOpenAICompatibleADKModel(Provider{BaseURL: messageServer.URL, Model: "gpt-test"}, "test-key", "gpt-test")
		err = model.(*openAICompatibleADKModel).generateStream(context.Background(), req, func(*adkmodel.LLMResponse, error) bool {
			return false
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("generateStream message cancel err = %v, want context.Canceled", err)
		}
	})

	t.Run("finalResponse and doGenerate surface malformed tool and request errors", func(t *testing.T) {
		state := openAIStreamAggregationState{
			toolCalls: []openAIToolCall{{
				ID: "call-1",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      "tool.bad",
					Arguments: `{"symbol":`,
				},
			}},
		}
		if _, err := state.finalResponse(); err == nil || !strings.Contains(err.Error(), "decode tool arguments") {
			t.Fatalf("finalResponse err = %v, want malformed tool args", err)
		}

		model := &openAICompatibleADKModel{provider: Provider{BaseURL: "https://example.test"}}
		if _, err := model.doGenerate(context.Background(), openAIChatRequest{
			Model:      "test-model",
			ToolChoice: make(chan int),
		}); err == nil {
			t.Fatal("doGenerate accepted unsupported payload")
		}

		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer errorServer.Close()
		model = &openAICompatibleADKModel{provider: Provider{BaseURL: errorServer.URL, Model: "gpt-test"}}
		if _, err := model.doGenerate(context.Background(), openAIChatRequest{Model: "gpt-test"}); err == nil || !strings.Contains(err.Error(), "provider returned 502") {
			t.Fatalf("doGenerate status err = %v, want 502", err)
		}

		closedServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		closedURL := closedServer.URL
		closedServer.Close()
		model = &openAICompatibleADKModel{provider: Provider{BaseURL: closedURL, Model: "gpt-test"}}
		if _, err := model.doGenerate(context.Background(), openAIChatRequest{Model: "gpt-test"}); err == nil {
			t.Fatal("doGenerate transport err = nil, want dial error")
		}
	})

	t.Run("newChatRequest reports invalid endpoints", func(t *testing.T) {
		model := &openAICompatibleADKModel{provider: Provider{BaseURL: "://bad url"}}
		_, err := model.newChatRequest(context.Background(), openAIChatRequest{Model: "test-model"})
		if err == nil {
			t.Fatal("newChatRequest accepted invalid base url")
		}
	})
}

func TestGoogleWorkflowAgentConcurrencyResumeCoverage(t *testing.T) {
	asker := adkworkflow.NewEmittingFunctionNode("asker", func(ctx adkagent.Context, _ any, emit func(*adksession.Event) error) (any, error) {
		if reply, ok := ctx.ResumedInput("ask-capped"); ok {
			return reply, nil
		}
		if err := emit(adkworkflow.NewRequestInputEvent(ctx, adksession.RequestInput{
			InterruptID: "ask-capped",
			Message:     "approve capped workflow?",
		})); err != nil {
			return nil, err
		}
		return nil, adkworkflow.ErrNodeInterrupted
	}, adkworkflow.NodeConfig{RerunOnResume: &googleADKWorkflowRerunOnResume})
	handler := adkworkflow.NewFunctionNode("handler", func(_ adkagent.Context, input any) (any, error) {
		return map[string]any{"handled": input}, nil
	}, adkworkflow.NodeConfig{})
	root, err := newGoogleADKWorkflowAgent(googleADKWorkflowAgentConfig{
		Name:           "capped_workflow",
		Description:    "capped workflow",
		MaxConcurrency: 1,
		Edges: []adkworkflow.Edge{
			{From: adkworkflow.Start, To: asker},
			{From: asker, To: handler},
		},
	})
	if err != nil {
		t.Fatalf("newGoogleADKWorkflowAgent capped: %v", err)
	}

	ctx := context.Background()
	service := adksession.InMemoryService()
	created, err := service.Create(ctx, &adksession.CreateRequest{AppName: "app", UserID: "user", SessionID: "session"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	freshCtx := &googleADKWorkflowAgentTestContext{
		StrictContextMock: adkagent.NewStrictContextMock(ctx),
		agent:             root,
		session:           created.Session,
		invocationID:      "workflow-invocation",
		userContent:       genai.NewContentFromText("start", genai.RoleUser),
	}

	var requestID string
	for event, err := range root.Run(freshCtx) {
		if err != nil {
			t.Fatalf("fresh capped workflow run: %v", err)
		}
		if event != nil && event.RequestedInput != nil {
			requestID = event.RequestedInput.InterruptID
		}
		if event != nil {
			if appendErr := service.AppendEvent(ctx, created.Session, event); appendErr != nil {
				t.Fatalf("AppendEvent fresh workflow: %v", appendErr)
			}
		}
	}
	if requestID != "ask-capped" {
		t.Fatalf("requestID = %q, want ask-capped", requestID)
	}

	resumeCtx := &googleADKWorkflowAgentTestContext{
		StrictContextMock: adkagent.NewStrictContextMock(ctx),
		agent:             root,
		session:           created.Session,
		invocationID:      "workflow-invocation",
		userContent: genai.NewContentFromParts([]*genai.Part{{
			FunctionResponse: &genai.FunctionResponse{
				ID:       "ask-capped",
				Name:     adkworkflow.WorkflowInputFunctionCallName,
				Response: map[string]any{"response": "approved"},
			},
		}}, genai.RoleUser),
	}
	var sawHandled bool
	for event, err := range root.Run(resumeCtx) {
		if err != nil {
			t.Fatalf("resume capped workflow run: %v", err)
		}
		if event != nil && event.Output != nil {
			if output, ok := event.Output.(map[string]any); ok && output["handled"] == "approved" {
				sawHandled = true
			}
		}
	}
	if !sawHandled {
		t.Fatal("capped workflow resume did not deliver handled output")
	}
}

func TestGoogleWorkflowAgentAdditionalBranchCoverage(t *testing.T) {
	t.Run("node error returns explicit message", func(t *testing.T) {
		if got := (&googleADKWorkflowNodeError{message: "boom"}).Error(); got != "boom" {
			t.Fatalf("node error text = %q, want boom", got)
		}
	})

	t.Run("node runner pause and generic agent run errors are surfaced", func(t *testing.T) {
		testCtx := &googleADKWorkflowAgentTestContext{
			StrictContextMock: adkagent.NewStrictContextMock(context.Background()),
			invocationID:      "invocation",
		}

		pausingRunner := &googleADKWorkflowNodeRunnerErrorDouble{
			events: []*adksession.Event{{LongRunningToolIDs: []string{"approval"}}},
		}
		if _, err := googleADKWorkflowRunNode(pausingRunner, testCtx, "task", false, func(*adksession.Event) error { return nil }); !errors.Is(err, adkworkflow.ErrNodeInterrupted) {
			t.Fatalf("googleADKWorkflowRunNode pause err = %v, want ErrNodeInterrupted", err)
		}

		agent, err := adkagent.New(adkagent.Config{
			Name: "generic-error",
			Run: func(adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
				return func(yield func(*adksession.Event, error) bool) {
					yield(nil, errors.New("generic agent failed"))
				}
			},
		})
		if err != nil {
			t.Fatalf("agent.New: %v", err)
		}
		if _, err := googleADKWorkflowRunGenericAgent(agent, testCtx, "task", false, func(*adksession.Event) error { return nil }); err == nil || err.Error() != "generic agent failed" {
			t.Fatalf("googleADKWorkflowRunGenericAgent err = %v, want generic agent failed", err)
		}
	})
}

func TestGoogleRunnerBoundaryCoverage(t *testing.T) {
	ctx := t.Context()

	t.Run("persistResumedRunResult surfaces assistant message creation failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		runtime.rawSessionService = createErrorSessionService{err: errors.New("create failed")}
		session := mustCreateSession(t, runtime, "persist-message-error-agent", "persist message error")
		run := mustSaveRun(t, runtime, Run{
			ID:        "persist-message-error",
			SessionID: session.ID,
			AgentID:   "persist-message-error-agent",
			Status:    RunStatusRunning,
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
			Usage:     &RunUsage{},
		})
		if _, err := runtime.persistResumedRunResult(ctx, run, openAIChatResult{Reply: "hello"}); err == nil || !strings.Contains(err.Error(), "create failed") {
			t.Fatalf("persistResumedRunResult message err = %v, want create failed", err)
		}
	})

	t.Run("persistResumedRunResult surfaces save failures after message append", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "persist-save-error-agent", "persist save error")
		run := mustSaveRun(t, runtime, Run{
			ID:        "persist-save-error",
			SessionID: session.ID,
			AgentID:   "persist-save-error-agent",
			Status:    RunStatusRunning,
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
			Usage:     &RunUsage{},
		})
		runtime.store.sessions = nil
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table: %v", err)
		}
		if _, err := runtime.persistResumedRunResult(ctx, run, openAIChatResult{Reply: "hello"}); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("persistResumedRunResult save err = %v, want runs table failure", err)
		}
	})

	t.Run("ensureGoogleADKFinalReply reports missing synthesized reply", func(t *testing.T) {
		runtime := newTestRuntime(t)
		providerID := saveGoalWorkflowProvider(t, runtime, "missing-final-reply-provider", func(openAIChatRequest) openAIChatMessage {
			return openAIChatMessage{Role: "assistant", Content: "   "}
		})
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "missing-final-reply-agent", Name: "Missing Final Reply", ProviderID: providerID, Status: AgentStatusEnabled,
		})
		session := mustCreateSession(t, runtime, agent.ID, "missing final reply")
		mustCreateADKSessionForAgent(t, runtime, agent.ID, session.ID)

		execution := &googleADKExecution{
			appName:        googleADKAppName(agent.ID),
			sessionID:      session.ID,
			sessionService: runtime.rawSessionService,
			agent:          agent,
			runID:          "missing-final-reply-run",
			runIDByAgentName: map[string]string{
				googleADKAgentName(agent.ID): "missing-final-reply-run",
			},
			calls: []ToolCall{{
				ID: "call-1", RunID: "missing-final-reply-run", ToolName: "tools.search", Status: "SUCCEEDED",
			}},
			postToolTextByRunID:     map[string]bool{},
			toolResponseSeenByRunID: map[string]bool{},
		}
		err := runtime.ensureGoogleADKFinalReply(ctx, agent, session, execution, "missing-final-reply-run", "summarize the result")
		if err == nil || !strings.Contains(err.Error(), errADKMissingFinalReply().Error()) {
			t.Fatalf("ensureGoogleADKFinalReply err = %v, want missing final reply", err)
		}
	})

	t.Run("rehydrate surfaces prepareAgent and session lookup errors", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:     "rehydrate-skill-error-agent",
			Name:   "Rehydrate Skill Error",
			Status: AgentStatusEnabled,
			Skills: []string{"missing-skill"},
		})
		_, err := runtime.rehydrateGoogleADKExecution(ctx, Run{
			ID: "rehydrate-skill-error-run", AgentID: agent.ID, SessionID: "missing-session", ProviderID: testProviderID, Model: "test-model",
		})
		if err == nil || !strings.Contains(err.Error(), "skill not found") {
			t.Fatalf("rehydrate prepareAgent err = %v, want skill not found", err)
		}

		runtime = newTestRuntime(t)
		agent = mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:     "rehydrate-session-error-agent",
			Name:   "Rehydrate Session Error",
			Status: AgentStatusEnabled,
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableSessions); err != nil {
			t.Fatalf("drop sessions table: %v", err)
		}
		_, err = runtime.rehydrateGoogleADKExecution(ctx, Run{
			ID: "rehydrate-session-error-run", AgentID: agent.ID, SessionID: "session-id", ProviderID: testProviderID, Model: "test-model",
		})
		if err == nil || !strings.Contains(err.Error(), tableSessions) {
			t.Fatalf("rehydrate session err = %v, want sessions table failure", err)
		}
	})
}

func TestRunnerChatHelperCoverage(t *testing.T) {
	ctx := t.Context()

	t.Run("resolveChatWorkflowOptions rejects invalid overrides", func(t *testing.T) {
		if _, _, _, err := resolveChatWorkflowOptions(ChatRequest{WorkModeOverride: "invalid"}, Agent{WorkMode: WorkModeChat}); err == nil || !strings.Contains(err.Error(), "invalid work mode") {
			t.Fatalf("resolveChatWorkflowOptions err = %v, want invalid work mode", err)
		}
	})

	t.Run("runChat surfaces session lookup and run persistence errors", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:     "run-chat-helper-agent",
			Name:   "Run Chat Helper",
			Status: AgentStatusEnabled,
		})
		if _, err := runtime.runChat(ctx, ChatRequest{
			AgentID:                agent.ID,
			Message:                "hello",
			SessionID:              "missing-session",
			PermissionModeOverride: PermissionModeApproval,
			WorkModeOverride:       WorkModeChat,
		}, nil, false); err == nil || !strings.Contains(err.Error(), "session not found") {
			t.Fatalf("runChat missing session err = %v, want session not found", err)
		}

		runtime = newTestRuntime(t)
		agent = mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:     "run-chat-save-run-helper-agent",
			Name:   "Run Chat Save Run Helper",
			Status: AgentStatusEnabled,
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table: %v", err)
		}
		if _, err := runtime.runChat(ctx, ChatRequest{
			AgentID:                agent.ID,
			Message:                "hello",
			PermissionModeOverride: PermissionModeApproval,
		}, nil, false); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("runChat save-run err = %v, want runs table failure", err)
		}
	})

	t.Run("persist and authoritative snapshots tolerate store failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		snapshot := Run{ID: "snapshot-run", Status: RunStatusRunning}
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table: %v", err)
		}
		if _, err := runtime.persistRunActivitySnapshot(ctx, snapshot); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("persistRunActivitySnapshot err = %v, want runs table error", err)
		}
		if authoritative := runtime.authoritativeRunSnapshot(ctx, snapshot); authoritative.ID != snapshot.ID || authoritative.Status != snapshot.Status {
			t.Fatalf("authoritativeRunSnapshot = %+v, want fallback snapshot %+v", authoritative, snapshot)
		}
	})

	t.Run("assistant message helpers surface projection and save errors", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "ensure-message-error-agent", "ensure message error")
		run := mustSaveRun(t, runtime, Run{
			ID:        "ensure-message-error-run",
			SessionID: session.ID,
			AgentID:   "ensure-message-error-agent",
			Status:    RunStatusRunning,
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
			Usage:     &RunUsage{},
		})
		mustCreateADKSessionForAgent(t, runtime, "ensure-message-error-agent", session.ID)
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table for projection: %v", err)
		}
		if _, err := runtime.ensureAssistantMessage(ctx, session, run, openAIChatResult{Reply: "hello"}); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("ensureAssistantMessage err = %v, want projection failure", err)
		}

		runtime = newTestRuntime(t)
		session = mustCreateSession(t, runtime, "attach-message-save-error-agent", "attach message save error")
		run = mustSaveRun(t, runtime, Run{
			ID:        "attach-message-save-error-run",
			SessionID: session.ID,
			AgentID:   "attach-message-save-error-agent",
			Status:    RunStatusRunning,
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
			Usage:     &RunUsage{},
		})
		runtime.store.sessions = nil
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table for save: %v", err)
		}
		if _, err := runtime.attachFinalAssistantMessage(ctx, session, run, openAIChatResult{Reply: "hello"}); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("attachFinalAssistantMessage err = %v, want save failure", err)
		}
	})

	t.Run("projectedChatResponse and context snapshot helpers degrade safely without dependencies", func(t *testing.T) {
		response := (&Runtime{}).projectedChatResponse(ctx, Session{ID: "session", AgentID: "agent"}, Run{ID: "run"}, openAIChatResult{Reply: "hello"})
		if response.Reply != "hello" || response.Run.ID != "run" {
			t.Fatalf("projectedChatResponse without store = %+v", response)
		}
		if snapshot := (*Runtime)(nil).contextSnapshotOrNil(ctx, "session"); snapshot != nil {
			t.Fatalf("contextSnapshotOrNil = %#v, want nil", snapshot)
		}
		if snapshot := (&Runtime{}).contextSnapshotForRunOrNil(ctx, Session{ID: "session", AgentID: "agent"}, Run{}); snapshot != nil {
			t.Fatalf("contextSnapshotForRunOrNil = %#v, want nil", snapshot)
		}
	})

	t.Run("completeChatRun completed branch and appendAssistantMessageEvent surface direct failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "complete-chat-run-helper-agent", "complete chat helper")
		run := mustSaveRun(t, runtime, Run{
			ID:        "complete-chat-run-helper",
			SessionID: session.ID,
			AgentID:   "complete-chat-run-helper-agent",
			Status:    RunStatusRunning,
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
			Usage:     &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table: %v", err)
		}
		if _, err := runtime.completeChatRun(ctx, session, run, "hello", toolExecutionContext{}, nil, openAIChatResult{Reply: "done"}, nil); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("completeChatRun success-path err = %v, want runs table failure", err)
		}

		runtime = newTestRuntime(t)
		base := adksession.InMemoryService()
		runtime.rawSessionService = &appendErrorSessionService{Service: base, err: errors.New("append failed")}
		session = mustCreateSession(t, runtime, "append-message-helper-agent", "append helper")
		created, err := base.Create(ctx, &adksession.CreateRequest{
			AppName: googleADKAppName("append-message-helper-agent"), UserID: googleADKUserID, SessionID: session.ID,
		})
		if err != nil {
			t.Fatalf("Create raw session: %v", err)
		}
		run = Run{ID: "append-message-helper-run", SessionID: session.ID, AgentID: "append-message-helper-agent"}
		_, err = runtime.appendAssistantMessageEvent(ctx, session, run, openAIChatResult{Reply: "hello"})
		if err == nil || !strings.Contains(err.Error(), "append failed") {
			t.Fatalf("appendAssistantMessageEvent err = %v, want append failed", err)
		}
		_ = created
	})

	t.Run("auto compaction and context snapshot fallbacks handle manager failures", func(t *testing.T) {
		if err := (*Runtime)(nil).maybeAutoCompactSessionWithOptions(ctx, Session{ID: "session"}, Agent{}, "", nil, false); err != nil {
			t.Fatalf("nil runtime maybeAutoCompactSessionWithOptions err = %v", err)
		}

		runtime := newTestRuntime(t)
		runtime.contextManager = NewSessionContextManager(runtime.Store(), getErrorADKSessionService{Service: runtime.rawSessionService, err: errors.New("raw unavailable")}, runtime.openai, runtime.Tools())
		if err := runtime.maybeAutoCompactSessionWithOptions(ctx, Session{ID: "session", AgentID: "agent"}, Agent{}, "hello", nil, false); err != nil {
			t.Fatalf("maybeAutoCompactSessionWithOptions manager error should be swallowed, got %v", err)
		}

		runtime = newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:     "context-run-agent",
			Name:   "Context Run Agent",
			Status: AgentStatusEnabled,
		})
		session := mustCreateSession(t, runtime, agent.ID, "context snapshot")
		mustCreateADKSessionForAgent(t, runtime, agent.ID, session.ID)
		run := Run{ProviderID: testProviderID, Model: "test-model"}
		if snapshot := runtime.contextSnapshotForRunOrNil(ctx, session, run); snapshot == nil {
			t.Fatal("contextSnapshotForRunOrNil = nil, want snapshot")
		}
	})
}

func TestRunnerLifecycleHelperCoverage(t *testing.T) {
	ctx := t.Context()

	t.Run("runModelSnapshot handles missing and existing providers", func(t *testing.T) {
		if providerName, modelName := (*Runtime)(nil).runModelSnapshot(ctx, Agent{Model: "fallback-model"}); providerName != "" || modelName != "fallback-model" {
			t.Fatalf("nil runtime snapshot = %q/%q", providerName, modelName)
		}

		runtime := newTestRuntime(t)
		agent := Agent{ProviderID: "missing-provider", Model: "fallback-model"}
		if providerName, modelName := runtime.runModelSnapshot(ctx, agent); providerName != "" || modelName != "fallback-model" {
			t.Fatalf("missing provider snapshot = %q/%q", providerName, modelName)
		}

		provider := mustSaveProvider(t, runtime, ProviderWriteRequest{
			ID:          "snapshot-provider",
			DisplayName: "Snapshot Provider",
			BaseURL:     "https://example.test/v1",
			Model:       "provider-model",
			APIKey:      "sk-test",
			Enabled:     true,
		})
		if providerName, modelName := runtime.runModelSnapshot(ctx, Agent{ProviderID: provider.ID}); providerName != "Snapshot Provider" || modelName != "provider-model" {
			t.Fatalf("provider snapshot = %q/%q", providerName, modelName)
		}
	})

	t.Run("recentOpenAIMessages trims content and skips approval placeholders", func(t *testing.T) {
		messages := recentOpenAIMessages([]Message{
			{Role: "user", Content: "   "},
			{Role: "assistant", Content: "绛夊緟鐢ㄦ埛瀹℃壒"},
			{Role: "assistant", Content: "assistant reply"},
			{Role: "user", Content: "123456789"},
		}, 10, 6)
		if len(messages) != 1 {
			t.Fatalf("recentOpenAIMessages = %#v, want one truncated assistant message", messages)
		}
		if messages[0].Role != "assistant" || messages[0].Content != "assist" {
			t.Fatalf("recentOpenAIMessages[0] = %#v, want truncated assistant", messages[0])
		}
	})

	t.Run("run status and audit helpers map timeout and cancellation", func(t *testing.T) {
		if status := runStatusForContext(context.Background(), nil); status != RunStatusCompleted {
			t.Fatalf("runStatusForContext(nil) = %q, want %q", status, RunStatusCompleted)
		}
		timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 0)
		defer timeoutCancel()
		<-timeoutCtx.Done()
		if status := runStatusForContext(timeoutCtx, errors.New("boom")); status != RunStatusTimedOut {
			t.Fatalf("runStatusForContext timeout = %q, want %q", status, RunStatusTimedOut)
		}

		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()
		if status := runStatusForContext(cancelCtx, errors.New("boom")); status != RunStatusCancelled {
			t.Fatalf("runStatusForContext cancel = %q, want %q", status, RunStatusCancelled)
		}

		if kind := runLifecycleAuditKind(RunStatusTimedOut); kind != "run.timed_out" {
			t.Fatalf("runLifecycleAuditKind(timeout) = %q", kind)
		}
		if kind := runLifecycleAuditKind(RunStatusCancelled); kind != "run.cancelled" {
			t.Fatalf("runLifecycleAuditKind(cancelled) = %q", kind)
		}
	})
}

func TestEventProjectionAdditionalCoverage(t *testing.T) {
	ctx := t.Context()

	t.Run("TranscriptEntries surfaces projection errors", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "projection-error-agent", "projection error")
		mustCreateADKSessionForAgent(t, runtime, "projection-error-agent", session.ID)
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table: %v", err)
		}
		if _, err := runtime.Store().TranscriptEntries(ctx, session.ID); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("TranscriptEntries err = %v, want runs table failure", err)
		}
	})

	t.Run("SessionProjection derives final message id from latest assistant", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "projection-final-id-agent", "projection final id")
		rawSession := mustCreateADKSessionForAgent(t, runtime, "projection-final-id-agent", session.ID)
		event := adksession.NewEvent(ctx, "assistant-final-id")
		event.Author = "agent"
		event.Content = genai.NewContentFromText("assistant reply", genai.RoleModel)
		if err := appendADKEventWithStaleRetry(ctx, runtimeAppendLocks(runtime), runtime.rawSessionService, rawSession, event); err != nil {
			t.Fatalf("append final assistant event: %v", err)
		}
		projection, ok, err := runtime.Store().SessionProjection(ctx, session.ID)
		if err != nil || !ok || projection.LatestAssistant == nil || projection.FinalMessageID != projection.LatestAssistant.ID {
			t.Fatalf("SessionProjection = %+v ok=%v err=%v", projection, ok, err)
		}
	})

	t.Run("event projection helpers cover nil ordering, nil parts and fallback entry ids", func(t *testing.T) {
		_ = sessionProjectionFromADKEvents([]*adksession.Event{nil, nil})
		nilPartsEvent := &adksession.Event{}
		nilPartsEvent.Content = &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{nil}}
		_ = sessionProjectionFromADKEvents([]*adksession.Event{nilPartsEvent})

		runStates := map[string]*projectedRunState{}
		runOrder := []string{}
		entries := []TranscriptEntry{}
		assistantEvent := &adksession.Event{Author: "agent"}
		assistantEvent.Content = &genai.Content{
			Role:  genai.RoleModel,
			Parts: []*genai.Part{{Text: "assistant text"}},
		}
		state := ensureProjectedRunState(runStates, &runOrder, &entries, assistantEvent)
		if state == nil || len(entries) != 1 || entries[0].ID == "" || !strings.HasPrefix(entries[0].ID, "event-message-") {
			t.Fatalf("ensureProjectedRunState fallback entry = state:%#v entries:%#v", state, entries)
		}
	})
}

func TestRunnerApprovalHelperCoverage(t *testing.T) {
	ctx := t.Context()

	t.Run("resolve approval surfaces store errors", func(t *testing.T) {
		runtime := newTestRuntime(t)
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableApprovals); err != nil {
			t.Fatalf("drop approvals table: %v", err)
		}
		if _, err := runtime.ResolveApproval(ctx, "approval-id", true); err == nil || !strings.Contains(err.Error(), tableApprovals) {
			t.Fatalf("ResolveApproval err = %v, want approvals table failure", err)
		}
		if _, err := runtime.ResolveApprovalAsync(ctx, "approval-id", true); err == nil || !strings.Contains(err.Error(), tableApprovals) {
			t.Fatalf("ResolveApprovalAsync err = %v, want approvals table failure", err)
		}
	})

	t.Run("stageResolvedApproval returns embedded run when approval is no longer embedded", func(t *testing.T) {
		runtime := newTestRuntime(t)
		run := mustSaveRun(t, runtime, Run{
			ID:        "stage-missing-embedded",
			SessionID: "session",
			AgentID:   "agent",
			Status:    RunStatusPending,
			PendingApprovals: []Approval{{
				ID:     "different-approval",
				Status: ApprovalStatusPending,
			}},
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
			Usage:     &RunUsage{},
		})
		resolution, shouldContinue, err := runtime.stageResolvedApproval(ctx, Approval{
			ID:     "missing-approval",
			RunID:  run.ID,
			Status: ApprovalStatusApproved,
		}, true)
		if err != nil || shouldContinue || resolution.Run == nil || resolution.Run.ID != run.ID {
			t.Fatalf("stageResolvedApproval resolution=%+v shouldContinue=%v err=%v", resolution, shouldContinue, err)
		}
	})

	t.Run("approval continuation helpers short-circuit for blank, closing, duplicate and non-resumable runs", func(t *testing.T) {
		runtime := newTestRuntime(t)
		runtime.enqueueResolvedApprovalContinuation(" ")
		if len(runtime.approvalRuns) != 0 {
			t.Fatalf("blank enqueue mutated approvalRuns: %#v", runtime.approvalRuns)
		}

		runtime.closing = true
		runtime.enqueueResolvedApprovalContinuation("run-1")
		if len(runtime.approvalRuns) != 0 {
			t.Fatalf("closing enqueue mutated approvalRuns: %#v", runtime.approvalRuns)
		}

		runtime.closing = false
		runtime.approvalRuns["run-1"] = struct{}{}
		runtime.enqueueResolvedApprovalContinuation("run-1")
		if len(runtime.approvalRuns) != 1 {
			t.Fatalf("duplicate enqueue mutated approvalRuns: %#v", runtime.approvalRuns)
		}

		runtime = newTestRuntime(t)
		if runHasRecoverableResolvedApprovalContext(Run{ResumeState: "", PendingApprovals: []Approval{{
			Status: ApprovalStatusApproved, FunctionCallID: "call", ConfirmationCallID: "confirm",
		}}}) {
			t.Fatal("runHasRecoverableResolvedApprovalContext should require approval_resuming")
		}
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table: %v", err)
		}
		runtime.ReconcileResolvedApprovals(ctx)
	})
}

func (c *googleADKWorkflowAgentTestContext) Branch() string {
	return c.path
}

func (c *googleADKWorkflowAgentTestContext) ResumedInput(string) (any, bool) {
	return nil, false
}
