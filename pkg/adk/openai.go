package adk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type openAIClient struct {
}

type openAIChatMessage struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	Name             string           `json:"name,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	Reasoning        string           `json:"reasoning,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature float64             `json:"temperature,omitempty"`
	Stream      bool                `json:"stream,omitempty"`
	Tools       []openAITool        `json:"tools,omitempty"`
	ToolChoice  any                 `json:"tool_choice,omitempty"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	Index    int    `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message openAIChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

type openAIChatStreamDelta struct {
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	Reasoning        string           `json:"reasoning,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIChatStreamResponse struct {
	Choices []struct {
		Delta        openAIChatStreamDelta `json:"delta"`
		Message      openAIChatMessage     `json:"message"`
		FinishReason string                `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

type openAIChatResult struct {
	Reply            string
	ReasoningContent string
}

func newOpenAIClient() openAIClient {
	return openAIClient{}
}

func providerRequestTimeout(provider Provider) time.Duration {
	timeoutMs := normalizeProviderRequestTimeoutMs(provider.RequestTimeoutMs)
	return time.Duration(timeoutMs) * time.Millisecond
}

func (c openAIClient) httpClientForProvider(provider Provider) *http.Client {
	return newProviderHTTPClient(providerRequestTimeout(provider))
}

const maxProviderPayloadBytes = 256 << 10 // Trim message content to stay under ~256KB JSON payload

type pendingOpenAIToolCall struct {
	messageIndex int
	call         openAIToolCall
}

// estimateMessageBytes returns an approximate byte size of a message when
// serialized to JSON, accounting for Content, ReasoningContent, Reasoning,
// and ToolCalls arguments.
func estimateMessageBytes(m openAIChatMessage) int {
	n := len(m.Content) + len(m.ReasoningContent) + len(m.Reasoning)
	for _, tc := range m.ToolCalls {
		n += len(tc.ID) + len(tc.Function.Name) + len(tc.Function.Arguments)
	}
	// Add overhead for JSON keys, punctuation, etc. (~64 bytes per message).
	n += 64
	return n
}

// truncateBytes truncates a string to at most maxBytes UTF-8 bytes,
// appending a truncation marker if needed.
func truncateBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	const marker = "\n...(truncated)"
	lim := maxBytes - len(marker)
	if lim < 0 {
		lim = 0
	}
	// Avoid splitting a multi-byte UTF-8 character.
	for lim > 0 && !utf8.RuneStart(s[lim]) {
		lim--
	}
	return s[:lim] + marker
}

func normalizeMessagesForProvider(messages []openAIChatMessage) []openAIChatMessage {
	if len(messages) == 0 {
		return messages
	}
	out := make([]openAIChatMessage, 0, len(messages))
	pending := map[string]pendingOpenAIToolCall{}
	activeToolCallIDs := map[string]struct{}{}
	activeAssistantIndex := -1
	droppedTools := 0

	resetActive := func() {
		activeAssistantIndex = -1
		activeToolCallIDs = map[string]struct{}{}
	}

	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = message.Role
		}
		message.Role = role
		switch role {
		case "assistant":
			out = append(out, message)
			resetActive()
			if len(message.ToolCalls) > 0 {
				activeAssistantIndex = len(out) - 1
				for _, call := range message.ToolCalls {
					id := strings.TrimSpace(call.ID)
					if id == "" {
						continue
					}
					pending[id] = pendingOpenAIToolCall{messageIndex: activeAssistantIndex, call: call}
					activeToolCallIDs[id] = struct{}{}
				}
			}
		case "tool":
			id := strings.TrimSpace(message.ToolCallID)
			if id == "" {
				droppedTools++
				resetActive()
				continue
			}
			if activeAssistantIndex >= 0 {
				if _, ok := activeToolCallIDs[id]; ok {
					out = append(out, message)
					delete(pending, id)
					delete(activeToolCallIDs, id)
					continue
				}
			}
			pair, ok := pending[id]
			if !ok {
				droppedTools++
				resetActive()
				continue
			}
			removeToolCallFromMessage(&out[pair.messageIndex], id)
			out = append(out, openAIChatMessage{
				Role:      "assistant",
				ToolCalls: []openAIToolCall{pair.call},
			})
			out = append(out, message)
			delete(pending, id)
			activeAssistantIndex = len(out) - 2
			activeToolCallIDs = map[string]struct{}{id: {}}
		default:
			out = append(out, message)
			resetActive()
		}
	}

	for id, pair := range pending {
		if pair.messageIndex >= 0 && pair.messageIndex < len(out) {
			removeToolCallFromMessage(&out[pair.messageIndex], id)
		}
	}
	normalized := make([]openAIChatMessage, 0, len(out))
	for _, message := range out {
		if shouldDropEmptyAssistantToolCallMessage(message) {
			continue
		}
		normalized = append(normalized, message)
	}
	if droppedTools > 0 {
		log.Printf("[adk] dropped %d orphan OpenAI tool message(s) before provider request", droppedTools)
	}
	return normalized
}

func removeToolCallFromMessage(message *openAIChatMessage, id string) {
	if message == nil || id == "" || len(message.ToolCalls) == 0 {
		return
	}
	next := message.ToolCalls[:0]
	for _, call := range message.ToolCalls {
		if strings.TrimSpace(call.ID) == id {
			continue
		}
		next = append(next, call)
	}
	message.ToolCalls = next
}

func shouldDropEmptyAssistantToolCallMessage(message openAIChatMessage) bool {
	return message.Role == "assistant" &&
		strings.TrimSpace(message.Content) == "" &&
		strings.TrimSpace(message.ReasoningContent) == "" &&
		strings.TrimSpace(message.Reasoning) == "" &&
		len(message.ToolCalls) == 0
}

type providerMessageGroup struct {
	messages []openAIChatMessage
	system   bool
}

func groupMessagesForProvider(messages []openAIChatMessage) []providerMessageGroup {
	groups := make([]providerMessageGroup, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		message := messages[i]
		if message.Role == "assistant" && len(message.ToolCalls) > 0 {
			group := providerMessageGroup{messages: []openAIChatMessage{message}}
			for i+1 < len(messages) && messages[i+1].Role == "tool" {
				group.messages = append(group.messages, messages[i+1])
				i++
			}
			groups = append(groups, group)
			continue
		}
		groups = append(groups, providerMessageGroup{
			messages: []openAIChatMessage{message},
			system:   message.Role == "system",
		})
	}
	return groups
}

func estimateMessageGroupBytes(group providerMessageGroup) int {
	total := 0
	for _, message := range group.messages {
		total += estimateMessageBytes(message)
	}
	return total
}

// trimMessagesForProvider trims message content to keep the estimated JSON
// payload size within budget, preventing 413 Request Entity Too Large errors.
func trimMessagesForProvider(messages []openAIChatMessage, maxTotalBytes int) []openAIChatMessage {
	if len(messages) == 0 {
		return messages
	}
	// Work on a copy to avoid mutating the caller's slice.
	out := normalizeMessagesForProvider(messages)

	// First pass: truncate individual messages that are excessively long.
	const maxSingleMessageBytes = 40000
	truncatedCount := 0
	for i := range out {
		if estimateMessageBytes(out[i]) > maxSingleMessageBytes {
			out[i].Content = truncateBytes(out[i].Content, maxSingleMessageBytes)
			out[i].ReasoningContent = truncateBytes(out[i].ReasoningContent, maxSingleMessageBytes)
			out[i].Reasoning = truncateBytes(out[i].Reasoning, maxSingleMessageBytes)
			truncatedCount++
		}
	}
	if truncatedCount > 0 {
		log.Printf("[adk] trimmed %d oversized message(s) to %d bytes each", truncatedCount, maxSingleMessageBytes)
	}

	// Second pass: trim older messages if total still exceeds budget.
	total := 0
	for _, m := range out {
		total += estimateMessageBytes(m)
	}
	if total <= maxTotalBytes {
		return normalizeMessagesForProvider(out)
	}

	groups := groupMessagesForProvider(out)
	systemGroups := make([]providerMessageGroup, 0, len(groups))
	systemBytes := 0
	for _, group := range groups {
		if !group.system {
			continue
		}
		systemGroups = append(systemGroups, group)
		systemBytes += estimateMessageGroupBytes(group)
	}
	remaining := maxTotalBytes - systemBytes
	keptGroups := make([]providerMessageGroup, 0, len(groups))
	droppedCount := 0
	for i := len(groups) - 1; i >= 0; i-- {
		group := groups[i]
		if group.system {
			continue
		}
		groupBytes := estimateMessageGroupBytes(group)
		if groupBytes > remaining {
			droppedCount += len(group.messages)
			continue
		}
		remaining -= groupBytes
		keptGroups = append(keptGroups, group)
	}
	for i, j := 0, len(keptGroups)-1; i < j; i, j = i+1, j-1 {
		keptGroups[i], keptGroups[j] = keptGroups[j], keptGroups[i]
	}
	finalGroups := append(systemGroups, keptGroups...)
	result := make([]openAIChatMessage, 0, len(out))
	for _, group := range finalGroups {
		result = append(result, group.messages...)
	}
	if droppedCount > 0 {
		log.Printf("[adk] dropped %d older message(s) to keep payload under %d bytes (was %d)", droppedCount, maxTotalBytes, total)
	}
	return normalizeMessagesForProvider(result)
}

func (c openAIClient) chat(ctx context.Context, provider Provider, apiKey string, model string, messages []openAIChatMessage) (string, error) {
	result, err := c.chatDetailed(ctx, provider, apiKey, model, messages)
	if err != nil {
		return "", err
	}
	return result.Reply, nil
}

func (c openAIClient) chatDetailed(ctx context.Context, provider Provider, apiKey string, model string, messages []openAIChatMessage) (openAIChatResult, error) {
	var result openAIChatResult
	streamResult, err := c.chatStream(ctx, provider, apiKey, model, messages, nil)
	if err != nil {
		return openAIChatResult{}, err
	}
	result = streamResult
	return result, nil
}

func (c openAIClient) selectTools(
	ctx context.Context,
	provider Provider,
	apiKey string,
	model string,
	messages []openAIChatMessage,
	descriptors []ToolDescriptor,
) ([]ToolInvocation, error) {
	tools := openAIToolsFromDescriptors(descriptors)
	if len(tools) == 0 {
		return nil, nil
	}
	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/chat/completions"
	if strings.TrimSpace(model) == "" {
		model = provider.Model
	}
	selectionMessages := append([]openAIChatMessage{}, trimMessagesForProvider(messages, maxProviderPayloadBytes)...)
	selectionMessages = append(selectionMessages, openAIChatMessage{
		Role:    "system",
		Content: "Decide which JFTrade tools are needed before answering. If no tool is useful, answer normally without tool calls.",
	})
	payload := openAIChatRequest{
		Model:       model,
		Messages:    selectionMessages,
		Temperature: 0,
		Stream:      false,
		Tools:       tools,
		ToolChoice:  "auto",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	for key, value := range provider.DefaultHeaders {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := c.httpClientForProvider(provider).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { jftradeLogError(resp.Body.Close()) }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errDetail := strings.TrimSpace(string(body))
		if errDetail == "" {
			errDetail = resp.Status
		}
		return nil, fmt.Errorf("provider returned %d during tool selection: %s", resp.StatusCode, errDetail)
	}
	var parsed openAIChatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode OpenAI-compatible tool selection: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("provider returned: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, nil
	}
	return toolInvocationsFromOpenAI(parsed.Choices[0].Message.ToolCalls), nil
}

func (c openAIClient) chatStream(
	ctx context.Context,
	provider Provider,
	apiKey string,
	model string,
	messages []openAIChatMessage,
	onDelta func(ChatDelta) error,
) (openAIChatResult, error) {
	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/chat/completions"
	if strings.TrimSpace(model) == "" {
		model = provider.Model
	}
	payload := openAIChatRequest{Model: model, Messages: trimMessagesForProvider(messages, maxProviderPayloadBytes), Temperature: 0.2, Stream: true}
	raw, err := json.Marshal(payload)
	if err != nil {
		return openAIChatResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return openAIChatResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	for key, value := range provider.DefaultHeaders {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := c.httpClientForProvider(provider).Do(req)
	if err != nil {
		return openAIChatResult{}, err
	}
	defer func() { jftradeLogError(resp.Body.Close()) }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if readErr != nil {
			return openAIChatResult{}, readErr
		}
		var parsed openAIChatResponse
		if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error != nil && parsed.Error.Message != "" {
			return openAIChatResult{}, fmt.Errorf("provider returned %d: %s", resp.StatusCode, parsed.Error.Message)
		}
		return openAIChatResult{}, fmt.Errorf("provider returned %d", resp.StatusCode)
	}

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return c.readStreamingResponse(resp.Body, onDelta)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return openAIChatResult{}, err
	}
	var parsed openAIChatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return openAIChatResult{}, fmt.Errorf("decode OpenAI-compatible response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return openAIChatResult{}, fmt.Errorf("provider returned: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return openAIChatResult{}, fmt.Errorf("provider returned no choices")
	}
	return c.emitStructuredMessage(parsed.Choices[0].Message, onDelta)
}

func (c openAIClient) readStreamingResponse(body io.Reader, onDelta func(ChatDelta) error) (openAIChatResult, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64<<10), 2<<20)

	var splitter legacyAssistantContentSplitter
	var replyBuilder strings.Builder
	var reasoningBuilder strings.Builder
	var dataLines []string

	flushEvent := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if strings.TrimSpace(payload) == "" {
			return nil
		}
		if strings.TrimSpace(payload) == "[DONE]" {
			return io.EOF
		}
		var parsed openAIChatStreamResponse
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			return fmt.Errorf("decode OpenAI-compatible stream chunk: %w", err)
		}
		if parsed.Error != nil && parsed.Error.Message != "" {
			return fmt.Errorf("provider returned: %s", parsed.Error.Message)
		}
		for _, choice := range parsed.Choices {
			if err := appendStreamChoice(&splitter, &replyBuilder, &reasoningBuilder, choice.Delta.Content, choice.Delta.ReasoningContent, choice.Delta.Reasoning, onDelta); err != nil {
				return err
			}
			if choice.Message.Content != "" || choice.Message.ReasoningContent != "" || choice.Message.Reasoning != "" {
				if err := appendStreamChoice(&splitter, &replyBuilder, &reasoningBuilder, choice.Message.Content, choice.Message.ReasoningContent, choice.Message.Reasoning, onDelta); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			err := flushEvent()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return openAIChatResult{}, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return openAIChatResult{}, err
	}
	if err := flushEvent(); err != nil && !errors.Is(err, io.EOF) {
		return openAIChatResult{}, err
	}
	replyTail, reasoningTail := splitter.Flush()
	if replyTail != "" || reasoningTail != "" {
		replyBuilder.WriteString(replyTail)
		reasoningBuilder.WriteString(reasoningTail)
		if onDelta != nil {
			if err := onDelta(ChatDelta{Reply: replyTail, ReasoningContent: reasoningTail}); err != nil {
				return openAIChatResult{}, err
			}
		}
	}

	result := openAIChatResult{
		Reply:            strings.TrimSpace(replyBuilder.String()),
		ReasoningContent: strings.TrimSpace(reasoningBuilder.String()),
	}
	if result.Reply == "" {
		return openAIChatResult{}, fmt.Errorf("provider returned an empty reply")
	}
	return result, nil
}

func (c openAIClient) emitStructuredMessage(message openAIChatMessage, onDelta func(ChatDelta) error) (openAIChatResult, error) {
	reply, reasoning := extractVisibleAndReasoningText(message.Content, message.ReasoningContent, message.Reasoning)
	result := openAIChatResult{
		Reply:            strings.TrimSpace(reply),
		ReasoningContent: strings.TrimSpace(reasoning),
	}
	if result.Reply == "" {
		return openAIChatResult{}, fmt.Errorf("provider returned an empty reply")
	}
	if onDelta != nil {
		if err := onDelta(ChatDelta{Reply: result.Reply, ReasoningContent: result.ReasoningContent}); err != nil {
			return openAIChatResult{}, err
		}
	}
	return result, nil
}

func appendStreamChoice(
	splitter *legacyAssistantContentSplitter,
	replyBuilder *strings.Builder,
	reasoningBuilder *strings.Builder,
	content string,
	reasoningContent string,
	reasoning string,
	onDelta func(ChatDelta) error,
) error {
	replyDelta, reasoningFromContent := splitter.Push(content)
	reasoningDelta := reasoningContent + reasoning
	if replyDelta != "" {
		replyBuilder.WriteString(replyDelta)
	}
	if reasoningFromContent != "" {
		reasoningBuilder.WriteString(reasoningFromContent)
	}
	if reasoningDelta != "" {
		reasoningBuilder.WriteString(reasoningDelta)
	}
	if onDelta == nil {
		return nil
	}
	if replyDelta == "" && reasoningFromContent == "" && reasoningDelta == "" {
		return nil
	}
	return onDelta(ChatDelta{
		Reply:            replyDelta,
		ReasoningContent: reasoningFromContent + reasoningDelta,
	})
}
