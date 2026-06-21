package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
			"order": 1, "title": title, "message": message, "description": message, "modeHint": WorkModeTask, "agentRole": "执行子 Agent",
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
		start := strings.Index(tag, prefix)
		if start < 0 {
			continue
		}
		rest := tag[start+len(prefix):]
		end := strings.Index(rest, quote)
		if end < 0 {
			return strings.TrimSpace(rest)
		}
		return strings.TrimSpace(rest[:end])
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
		index := strings.Index(text, marker)
		if index >= 0 {
			rest := "task-" + text[index+len(marker):]
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
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func jftradePanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
