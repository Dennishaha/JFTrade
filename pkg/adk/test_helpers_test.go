package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

const testProviderID = "test-openai-compatible"

func ensureTestProvider(t *testing.T, runtime *Runtime) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(testProviderChatHandler))
	t.Cleanup(server.Close)
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:          testProviderID,
		DisplayName: "Test OpenAI Compatible",
		BaseURL:     server.URL,
		Model:       "test-model",
		APIKey:      "sk-test",
		Enabled:     true,
	})
}

func mustCreateSession(t *testing.T, runtime *Runtime, agentID string, title string) Session {
	t.Helper()
	session, err := runtime.Store().CreateSession(context.Background(), agentID, title)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return session
}

func mustSaveRun(t *testing.T, runtime *Runtime, run Run) Run {
	t.Helper()
	if err := runtime.Store().SaveRun(context.Background(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	return run
}

func mustMessages(t *testing.T, runtime *Runtime, sessionID string) []Message {
	t.Helper()
	messages, err := runtime.Store().Messages(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	return messages
}

func mustAssistantMessages(t *testing.T, runtime *Runtime, sessionID string) []Message {
	t.Helper()
	messages := mustMessages(t, runtime, sessionID)
	filtered := make([]Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == "assistant" {
			filtered = append(filtered, message)
		}
	}
	return filtered
}

func mustAuditEvents(t *testing.T, runtime *Runtime) []AuditEvent {
	t.Helper()
	events, err := runtime.Store().ListAuditEvents(context.Background())
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	return events
}

func mustSaveProvider(t *testing.T, runtime *Runtime, req ProviderWriteRequest) Provider {
	t.Helper()
	provider, err := runtime.Store().SaveProvider(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	return provider
}

func mustSaveAgent(t *testing.T, runtime *Runtime, req AgentWriteRequest) Agent {
	t.Helper()
	if strings.TrimSpace(req.ProviderID) == "" {
		req.ProviderID = testProviderID
	}
	agent, err := runtime.Store().SaveAgent(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	return agent
}

type workflowDeclaredTool interface {
	Declaration() *genai.FunctionDeclaration
}

func saveGoalWorkflowProvider(t *testing.T, runtime *Runtime, providerID string, responder func(openAIChatRequest) openAIChatMessage) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		defer func() { jftradePanicOnError(r.Body.Close()) }()
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		jftradeErr := json.NewEncoder(w).Encode(openAIChatResponse{
			Choices: []struct {
				Message openAIChatMessage `json:"message"`
			}{{Message: responder(req)}},
		})
		jftradePanicOnError(jftradeErr)
	}))
	t.Cleanup(server.Close)
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: providerID, DisplayName: providerID, BaseURL: server.URL, Model: "test-model", APIKey: "sk-test", Enabled: true,
	})
	return providerID
}

func newWorkflowApprovalRuntime(t *testing.T, mode string) (*Runtime, *atomic.Int64) {
	t.Helper()
	base := newTestRuntime(t)
	registry := NewToolRegistry()
	executions := &atomic.Int64{}
	registry.Register(ToolDescriptor{
		Name:               "approval.required",
		DisplayName:        "Approval Required",
		Description:        "test approval tool",
		Category:           "strategy",
		Permission:         "write_strategy",
		AllowedModes:       []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
		RequiresApprovalIn: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		executions.Add(1)
		return map[string]any{"saved": true, "mode": normalizeWorkMode(mode)}, nil
	})
	runtime := newRuntimeWithRegistry(t, base.Store(), registry)
	return runtime, executions
}

func waitForRunStatus(t *testing.T, runtime *Runtime, runID string, status string) Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		run, ok, err := runtime.Store().Run(context.Background(), runID)
		if err != nil {
			t.Fatalf("Run(%s): %v", runID, err)
		}
		if ok && run.Status == status {
			return run
		}
		if time.Now().After(deadline) {
			t.Fatalf("Run(%s) did not reach status %s; last=%+v ok=%v", runID, status, run, ok)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func appendADKEvent(t *testing.T, runtime *Runtime, agentID string, sessionID string, event *adksession.Event) {
	t.Helper()
	ctx := context.Background()
	if _, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName: googleADKAppName(agentID), UserID: googleADKUserID, SessionID: sessionID,
	}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "already") && !strings.Contains(strings.ToLower(err.Error()), "unique constraint") && !strings.Contains(strings.ToLower(err.Error()), "constraint failed") {
		t.Fatalf("Create ADK session: %v", err)
	}
	response, err := runtime.rawSessionService.Get(context.Background(), &adksession.GetRequest{
		AppName: googleADKAppName(agentID), UserID: googleADKUserID, SessionID: sessionID,
	})
	if err != nil || response == nil || response.Session == nil {
		t.Fatalf("Get ADK session: response=%#v err=%v", response, err)
	}
	if err := appendADKEventWithStaleRetry(context.Background(), runtimeAppendLocks(runtime), runtime.rawSessionService, response.Session, event); err != nil {
		t.Fatalf("appendADKEventWithStaleRetry: %v", err)
	}
}

func newAssistantEvent(runID string, parts []*genai.Part, at time.Time) *adksession.Event {
	event := adksession.NewEvent(context.Background(), runID)
	event.Author = googleADKAgentName("agent")
	event.Content = &genai.Content{Role: genai.RoleModel, Parts: parts}
	event.Timestamp = at
	return event
}

func newToolCallEvent(runID string, callID string, toolName string, at time.Time) *adksession.Event {
	event := newAssistantEvent(runID, []*genai.Part{{FunctionCall: &genai.FunctionCall{
		ID: callID, Name: toolName, Args: map[string]any{},
	}}}, at)
	return event
}

func newToolResponseEvent(runID string, callID string, toolName string, response map[string]any, at time.Time) *adksession.Event {
	event := adksession.NewEvent(context.Background(), runID)
	event.Author = googleADKAgentName("agent")
	event.Content = &genai.Content{Role: genai.RoleUser, Parts: []*genai.Part{{FunctionResponse: &genai.FunctionResponse{
		ID: callID, Name: toolName, Response: response,
	}}}}
	event.Timestamp = at
	return event
}

func runHasToolCall(run Run, toolName string) bool {
	for _, call := range run.ToolCalls {
		if call.ToolName == toolName {
			return true
		}
	}
	return false
}

func testGoalWorkflowLastUserMessage(req openAIChatRequest) string {
	for index := len(req.Messages) - 1; index >= 0; index-- {
		if req.Messages[index].Role == "user" {
			return req.Messages[index].Content
		}
	}
	return ""
}

func testGoalWorkflowToolResponsesSinceLastUser(messages []openAIChatMessage) map[string]bool {
	seen := map[string]bool{}
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role == "user" {
			break
		}
		if message.Role == "tool" {
			name := restoreToolNameFromOpenAI(message.Name)
			if name == "" {
				name = message.Name
			}
			seen[name] = true
		}
	}
	return seen
}

func testGoalWorkflowTaskProgressCalls(req openAIChatRequest) []openAIToolCall {
	toolNames := testProviderToolNames(req)
	seen := testGoalWorkflowToolResponsesSinceLastUser(req.Messages)
	if containsTool(toolNames, workflowTasksListTool) && !seen[workflowTasksListTool] {
		return []openAIToolCall{testProviderToolCall("call-workflow-tasks-list", workflowTasksListTool, map[string]any{})}
	}
	if containsTool(toolNames, workflowTaskClaimTool) && !seen[workflowTaskClaimTool] {
		return []openAIToolCall{testProviderToolCall("call-workflow-task-claim", workflowTaskClaimTool, map[string]any{})}
	}
	if containsTool(toolNames, workflowTaskCompleteTool) && !seen[workflowTaskCompleteTool] {
		return []openAIToolCall{testProviderToolCall("call-workflow-task-complete", workflowTaskCompleteTool, map[string]any{
			"taskId": "", "summary": "任务已推进。",
		})}
	}
	return nil
}

func testProviderChatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		http.NotFound(w, r)
		return
	}
	defer func() { jftradePanicOnError(r.Body.Close()) }()
	var req openAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	jftradeErr1 := json.NewEncoder(w).Encode(openAIChatResponse{
		Choices: []struct {
			Message openAIChatMessage `json:"message"`
		}{{Message: testProviderMessage(req)}},
	})
	jftradePanicOnError(jftradeErr1)
}

func testProviderMessage(req openAIChatRequest) openAIChatMessage {
	if len(req.Tools) > 0 {
		if calls := testProviderToolCalls(req); len(calls) > 0 {
			return openAIChatMessage{Role: "assistant", ToolCalls: calls}
		}
	}
	return openAIChatMessage{Role: "assistant", Content: testProviderFinalReply(req)}
}

func testProviderToolResponseNames(messages []openAIChatMessage) map[string]bool {
	names := map[string]bool{}
	for _, message := range messages {
		if message.Role == "tool" {
			name := restoreToolNameFromOpenAI(message.Name)
			if name == "" {
				name = message.Name
			}
			names[name] = true
		}
	}
	return names
}

func testProviderToolCalls(req openAIChatRequest) []openAIToolCall {
	toolNames := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		name := restoreToolNameFromOpenAI(tool.Function.Name)
		if name == "" {
			name = tool.Function.Name
		}
		toolNames = append(toolNames, name)
	}
	rawText := testProviderConversationText(req.Messages)
	text := strings.ToLower(rawText)
	switch {
	case containsTool(toolNames, workflowPlanFinishTool):
		return testProviderWorkflowPlanCalls(req, text)
	case containsTool(toolNames, workflowTaskCompleteTool):
		return testProviderWorkflowTaskCalls(req, text)
	case containsTool(toolNames, workflowGoalCompleteTool):
		seen := testProviderToolResponseNames(req.Messages)
		if !seen[workflowGoalCompleteTool] && !seen[workflowGoalContinueTool] {
			return []openAIToolCall{testProviderToolCall("call-goal-complete", workflowGoalCompleteTool, map[string]any{
				"summary": "目标已完成。",
			})}
		}
		return nil
	default:
		if len(testProviderToolResponseNames(req.Messages)) > 0 {
			return nil
		}
		return testProviderBusinessToolCalls(toolNames, rawText)
	}
}

func testProviderWorkflowPlanCalls(req openAIChatRequest, text string) []openAIToolCall {
	seen := testProviderToolResponseNames(req.Messages)
	title := "推进任务"
	message := strings.TrimSpace(testProviderLastUserText(text))
	if message == "" {
		message = "完成用户请求"
	}
	if strings.Contains(text, "整理一个执行清单") {
		title = "整理执行清单"
		message = "整理一个执行清单"
	}
	if strings.Contains(text, "创建子智能体") {
		title = "委派子智能体"
		message = "请创建子智能体完成任务"
		if strings.Contains(text, "approval.required") {
			message = "请 @approval.required 保存策略"
		} else if strings.Contains(text, "strategy.save_draft") {
			message = "请 @strategy.save_draft 保存策略"
		} else if strings.Contains(text, "行情分析") {
			message = "请创建子智能体完成行情分析"
		}
	}
	switch {
	case !seen[workflowPlanResetTool]:
		return []openAIToolCall{testProviderToolCall("call-plan-reset", workflowPlanResetTool, map[string]any{})}
	case !seen[workflowPlanAddStepTool]:
		return []openAIToolCall{testProviderToolCall("call-plan-add", workflowPlanAddStepTool, map[string]any{
			"order": 1, "title": title, "message": message, "description": message, "modeHint": WorkModeLoop, "agentRole": "执行子 Agent",
		})}
	case !seen[workflowPlanFinishTool]:
		return []openAIToolCall{testProviderToolCall("call-plan-finish", workflowPlanFinishTool, map[string]any{})}
	default:
		return nil
	}
}

func testProviderWorkflowTaskCalls(req openAIChatRequest, text string) []openAIToolCall {
	seen := testProviderToolResponseNames(req.Messages)
	if seen[workflowTaskCompleteTool] || seen[workflowTaskDelegateTool] || seen[workflowTaskBlockTool] {
		if containsTool(testProviderToolNames(req), workflowGoalCompleteTool) && !seen[workflowGoalCompleteTool] && !seen[workflowGoalContinueTool] {
			return []openAIToolCall{testProviderToolCall("call-goal-complete", workflowGoalCompleteTool, map[string]any{
				"summary": "目标已完成。",
			})}
		}
		return nil
	}
	if !seen[workflowTasksListTool] {
		return []openAIToolCall{testProviderToolCall("call-task-list", workflowTasksListTool, map[string]any{})}
	}
	taskID := testProviderTaskIDFromText(text)
	if strings.Contains(text, "创建子智能体") || strings.Contains(text, "approval.required") || strings.Contains(text, "strategy.save_draft") {
		return []openAIToolCall{testProviderToolCall("call-task-delegate", workflowTaskDelegateTool, map[string]any{
			"taskId": taskID, "prompt": testProviderDelegatePrompt(text), "agentRole": "执行子 Agent",
		})}
	}
	return []openAIToolCall{testProviderToolCall("call-task-complete", workflowTaskCompleteTool, map[string]any{
		"taskId": taskID, "resultSummary": "已整理执行清单。",
	})}
}

func testProviderToolNames(req openAIChatRequest) []string {
	toolNames := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		name := restoreToolNameFromOpenAI(tool.Function.Name)
		if name == "" {
			name = tool.Function.Name
		}
		toolNames = append(toolNames, name)
	}
	return toolNames
}

func testProviderBusinessToolCalls(toolNames []string, text string) []openAIToolCall {
	if calls := testProviderExecuteToolCalls(toolNames, text); len(calls) > 0 {
		return calls
	}
	lowerText := strings.ToLower(text)
	var calls []openAIToolCall
	add := func(name string, args map[string]any) {
		if containsTool(toolNames, name) {
			calls = append(calls, testProviderToolCall(fmt.Sprintf("call-%d-%s", len(calls)+1, sanitizeToolNameForOpenAI(name)), name, args))
		}
	}
	for _, name := range toolNames {
		if strings.Contains(lowerText, "@"+strings.ToLower(name)) {
			add(name, testProviderArgsForTool(name, lowerText))
			return calls
		}
	}
	if strings.Contains(lowerText, "账户") || strings.Contains(lowerText, "订单") {
		add("account.orders", map[string]any{})
		add("portfolio.summary", map[string]any{})
	}
	if strings.Contains(lowerText, "行情") {
		add("market.subscriptions", map[string]any{})
	}
	if strings.Contains(lowerText, "回测") {
		add("backtest.runs", map[string]any{})
	}
	if strings.Contains(lowerText, "系统") {
		add("system.status", map[string]any{})
	}
	if strings.Contains(lowerText, "工具") {
		add("tools.search", map[string]any{"query": "工具"})
	}
	return calls
}

func testProviderExecuteToolCalls(toolNames []string, text string) []openAIToolCall {
	var calls []openAIToolCall
	remaining := text
	for {
		start := strings.Index(remaining, "<execute-tool")
		if start < 0 {
			break
		}
		remaining = remaining[start:]
		end := strings.Index(remaining, ">")
		if end < 0 {
			break
		}
		tag := remaining[:end+1]
		remaining = remaining[end+1:]
		rawName := testProviderTagAttr(tag, "name")
		name := testProviderCanonicalToolName(toolNames, rawName)
		if name == "" {
			continue
		}
		args := map[string]any{}
		if rawParams := testProviderTagAttr(tag, "parameters"); rawParams != "" {
			jftradeErr2 := json.Unmarshal([]byte(rawParams), &args)
			jftradePanicOnError(jftradeErr2)
		}
		for _, key := range []string{"title", "key", "value"} {
			if value := testProviderTagAttr(tag, key); value != "" {
				args[key] = value
			}
		}
		calls = append(calls, testProviderToolCall(fmt.Sprintf("call-tag-%d-%s", len(calls)+1, sanitizeToolNameForOpenAI(name)), name, args))
	}
	return calls
}

func testProviderCanonicalToolName(toolNames []string, raw string) string {
	normalized := normalizeToolAlias(raw)
	for _, name := range toolNames {
		if normalizeToolAlias(name) == normalized {
			return name
		}
	}
	return ""
}

func testProviderTagAttr(tag string, name string) string {
	for _, quote := range []string{`"`, `'`} {
		prefix := name + "=" + quote
		_, after, ok := strings.Cut(tag, prefix)
		if !ok {
			continue
		}
		rest := after
		before, _, ok := strings.Cut(rest, quote)
		if !ok {
			return strings.TrimSpace(rest)
		}
		return strings.TrimSpace(before)
	}
	return ""
}

func testProviderToolCall(id string, name string, args map[string]any) openAIToolCall {
	rawArgs, jftradeErr3 := json.Marshal(args)
	jftradePanicOnError(jftradeErr3)
	call := openAIToolCall{ID: id, Type: "function"}
	call.Function.Name = sanitizeToolNameForOpenAI(name)
	call.Function.Arguments = string(rawArgs)
	return call
}

func testProviderArgsForTool(name string, text string) map[string]any {
	switch name {
	case "tools.search":
		return map[string]any{"query": strings.TrimSpace(text)}
	case "strategy.save_draft":
		return map[string]any{"name": "测试策略", "source": "strategy('test')"}
	default:
		return map[string]any{}
	}
}

func testProviderFinalReply(req openAIChatRequest) string {
	conversation := testProviderConversationText(req.Messages)
	var toolNames []string
	for _, message := range req.Messages {
		if message.Role == "tool" && strings.TrimSpace(message.Name) != "" {
			toolNames = append(toolNames, restoreToolNameFromOpenAI(message.Name))
		}
	}
	if len(toolNames) > 0 {
		return "已完成 ADK 分析：" + strings.Join(toolNames, ", ")
	}
	if task := testProviderValueAfterLabel(conversation, "当前子任务："); task != "" {
		return "已完成 ADK 分析：" + task
	}
	if task := testProviderValueAfterLabel(conversation, "子任务："); task != "" {
		return "已完成 ADK 分析：" + task
	}
	if task := testProviderValueAfterLabel(conversation, "JFTRADE_WORKFLOW_TASK:"); task != "" {
		return "已完成 ADK 分析：" + task
	}
	last := testProviderLastUserText(conversation)
	if strings.TrimSpace(last) == "" {
		last = "请求"
	}
	return "已完成 ADK 分析：" + strings.TrimSpace(last)
}

func testProviderValueAfterLabel(text string, label string) string {
	index := strings.LastIndex(text, label)
	if index < 0 {
		return ""
	}
	rest := strings.TrimSpace(text[index+len(label):])
	if rest == "" {
		return ""
	}
	if lineEnd := strings.Index(rest, "\n"); lineEnd >= 0 {
		rest = rest[:lineEnd]
	}
	return strings.TrimSpace(rest)
}

func testProviderConversationText(messages []openAIChatMessage) string {
	var parts []string
	for _, message := range messages {
		if strings.TrimSpace(message.Content) != "" {
			parts = append(parts, message.Content)
		}
		if len(message.ToolCalls) > 0 {
			raw, jftradeErr4 := json.Marshal(message.ToolCalls)
			jftradePanicOnError(jftradeErr4)
			parts = append(parts, string(raw))
		}
	}
	return strings.Join(parts, "\n")
}

func testProviderLastUserText(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func testProviderTaskIDFromText(text string) string {
	for _, marker := range []string{`"id":"task-`, `"id": "task-`} {
		_, after, ok := strings.Cut(text, marker)
		if ok {
			rest := "task-" + after
			end := strings.IndexAny(rest, `",}`)
			if end < 0 {
				return strings.TrimSpace(rest)
			}
			return strings.TrimSpace(rest[:end])
		}
	}
	const marker = `"id":`
	index := strings.LastIndex(text, marker)
	if index < 0 {
		return ""
	}
	rest := strings.TrimSpace(text[index+len(marker):])
	rest = strings.TrimLeft(rest, `" `)
	end := strings.IndexAny(rest, `",}`)
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

func testProviderDelegatePrompt(text string) string {
	if strings.Contains(text, "approval.required") {
		return "请 @approval.required 保存策略"
	}
	if strings.Contains(text, "strategy.save_draft") {
		return "请 @strategy.save_draft 保存策略"
	}
	if strings.Contains(text, "行情分析") {
		return "请创建子智能体完成行情分析"
	}
	return "请创建子智能体完成任务"
}

func containsTool(names []string, want string) bool {
	return slices.Contains(names, want)
}

func jftradePanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
