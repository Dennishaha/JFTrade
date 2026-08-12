package providers

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

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

type OpenAIClient struct {
}

type OpenAIChatMessage struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	Name             string           `json:"name,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	Reasoning        string           `json:"reasoning,omitempty"`
	ToolCalls        []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAIChatRequest struct {
	Model          string              `json:"model"`
	Messages       []OpenAIChatMessage `json:"messages"`
	ReasoningField string              `json:"-"`
	ReasoningValue string              `json:"-"`
	Temperature    float64             `json:"temperature,omitempty"`
	Stream         bool                `json:"stream,omitempty"`
	Tools          []OpenAITool        `json:"tools,omitempty"`
	ToolChoice     any                 `json:"tool_choice,omitempty"`
}

type OpenAITool struct {
	Type     string             `json:"type"`
	Function OpenAIToolFunction `json:"function"`
}

type OpenAIToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type OpenAIToolCall struct {
	Index    int    `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type OpenAIChatResponse struct {
	Choices []struct {
		Message OpenAIChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

type OpenAIChatStreamDelta struct {
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	Reasoning        string           `json:"reasoning,omitempty"`
	ToolCalls        []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAIChatStreamResponse struct {
	Choices []struct {
		Delta        OpenAIChatStreamDelta `json:"delta"`
		Message      OpenAIChatMessage     `json:"message"`
		FinishReason string                `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

type AssistantExecutionResult = jfadkmodel.AssistantExecutionResult

func NewOpenAIClient() OpenAIClient {
	return OpenAIClient{}
}

func ProviderRequestTimeout(provider jfadkmodel.Provider) time.Duration {
	return provider.RequestTimeout()
}

func (c OpenAIClient) HTTPClientForProvider(provider jfadkmodel.Provider) *http.Client {
	return NewHTTPClient(ProviderRequestTimeout(provider))
}

const MaxProviderPayloadBytes = 256 << 10 // Trim message content to stay under ~256KB JSON payload

type PendingOpenAIToolCall struct {
	messageIndex int
	call         OpenAIToolCall
}

type OpenAIMessageNormalizer struct {
	out                  []OpenAIChatMessage
	pending              map[string]PendingOpenAIToolCall
	activeToolCallIDs    map[string]struct{}
	activeAssistantIndex int
	droppedTools         int
}

type OpenAIStreamAccumulator struct {
	splitter         LegacyAssistantContentSplitter
	replyBuilder     strings.Builder
	reasoningBuilder strings.Builder
	dataLines        []string
}

// EstimateMessageBytes returns an approximate byte size of a message when
// serialized to JSON, accounting for Content, ReasoningContent, Reasoning,
// and ToolCalls arguments.
func EstimateMessageBytes(m OpenAIChatMessage) int {
	n := len(m.Content) + len(m.ReasoningContent) + len(m.Reasoning)
	for _, tc := range m.ToolCalls {
		n += len(tc.ID) + len(tc.Function.Name) + len(tc.Function.Arguments)
	}
	// Add overhead for JSON keys, punctuation, etc. (~64 bytes per message).
	n += 64
	return n
}

// TruncateBytes truncates a string to at most maxBytes UTF-8 bytes,
// appending a truncation marker if needed.
func TruncateBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	const marker = "\n...(truncated)"
	lim := max(maxBytes-len(marker), 0)
	// Avoid splitting a multi-byte UTF-8 character.
	for lim > 0 && !utf8.RuneStart(s[lim]) {
		lim--
	}
	return s[:lim] + marker
}

func NormalizeMessagesForProvider(messages []OpenAIChatMessage) []OpenAIChatMessage {
	if len(messages) == 0 {
		return messages
	}
	return NewOpenAIMessageNormalizer(len(messages)).normalize(messages)
}

func NewOpenAIMessageNormalizer(size int) *OpenAIMessageNormalizer {
	return &OpenAIMessageNormalizer{
		out:                  make([]OpenAIChatMessage, 0, size),
		pending:              map[string]PendingOpenAIToolCall{},
		activeToolCallIDs:    map[string]struct{}{},
		activeAssistantIndex: -1,
	}
}

func (n *OpenAIMessageNormalizer) normalize(messages []OpenAIChatMessage) []OpenAIChatMessage {
	for _, message := range messages {
		n.consumeMessage(message)
	}
	for id, pair := range n.pending {
		if pair.messageIndex >= 0 && pair.messageIndex < len(n.out) {
			RemoveToolCallFromMessage(&n.out[pair.messageIndex], id)
		}
	}
	normalized := make([]OpenAIChatMessage, 0, len(n.out))
	for _, message := range n.out {
		if !ShouldDropEmptyAssistantToolCallMessage(message) {
			normalized = append(normalized, message)
		}
	}
	if n.droppedTools > 0 {
		log.Printf("[adk] dropped %d orphan OpenAI tool message(s) before provider request", n.droppedTools)
	}
	return normalized
}

func (n *OpenAIMessageNormalizer) consumeMessage(message OpenAIChatMessage) {
	message.Role = NormalizeProviderMessageRole(message.Role)
	switch message.Role {
	case "assistant":
		n.appendAssistantMessage(message)
	case "tool":
		n.appendToolMessage(message)
	default:
		n.out = append(n.out, message)
		n.resetActive()
	}
}

func NormalizeProviderMessageRole(role string) string {
	trimmed := strings.TrimSpace(role)
	if trimmed == "" {
		return role
	}
	return trimmed
}

func (n *OpenAIMessageNormalizer) appendAssistantMessage(message OpenAIChatMessage) {
	n.out = append(n.out, message)
	n.resetActive()
	if len(message.ToolCalls) == 0 {
		return
	}
	n.activeAssistantIndex = len(n.out) - 1
	for _, call := range message.ToolCalls {
		id := strings.TrimSpace(call.ID)
		if id == "" {
			continue
		}
		n.pending[id] = PendingOpenAIToolCall{messageIndex: n.activeAssistantIndex, call: call}
		n.activeToolCallIDs[id] = struct{}{}
	}
}

func (n *OpenAIMessageNormalizer) appendToolMessage(message OpenAIChatMessage) {
	id := strings.TrimSpace(message.ToolCallID)
	if id == "" {
		n.dropToolMessage()
		return
	}
	if n.appendActiveToolResponse(id, message) {
		return
	}
	if !n.rebuildPendingToolPair(id, message) {
		n.dropToolMessage()
	}
}

func (n *OpenAIMessageNormalizer) appendActiveToolResponse(id string, message OpenAIChatMessage) bool {
	if n.activeAssistantIndex < 0 {
		return false
	}
	if _, ok := n.activeToolCallIDs[id]; !ok {
		return false
	}
	n.out = append(n.out, message)
	delete(n.pending, id)
	delete(n.activeToolCallIDs, id)
	return true
}

func (n *OpenAIMessageNormalizer) rebuildPendingToolPair(id string, message OpenAIChatMessage) bool {
	pair, ok := n.pending[id]
	if !ok {
		return false
	}
	RemoveToolCallFromMessage(&n.out[pair.messageIndex], id)
	n.out = append(n.out, OpenAIChatMessage{
		Role:      "assistant",
		ToolCalls: []OpenAIToolCall{pair.call},
	})
	n.out = append(n.out, message)
	delete(n.pending, id)
	n.activeAssistantIndex = len(n.out) - 2
	n.activeToolCallIDs = map[string]struct{}{id: {}}
	return true
}

func (n *OpenAIMessageNormalizer) dropToolMessage() {
	n.droppedTools++
	n.resetActive()
}

func (n *OpenAIMessageNormalizer) resetActive() {
	n.activeAssistantIndex = -1
	n.activeToolCallIDs = map[string]struct{}{}
}

func RemoveToolCallFromMessage(message *OpenAIChatMessage, id string) {
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

func ShouldDropEmptyAssistantToolCallMessage(message OpenAIChatMessage) bool {
	return message.Role == "assistant" &&
		strings.TrimSpace(message.Content) == "" &&
		strings.TrimSpace(message.ReasoningContent) == "" &&
		strings.TrimSpace(message.Reasoning) == "" &&
		len(message.ToolCalls) == 0
}

type ProviderMessageGroup struct {
	messages []OpenAIChatMessage
	system   bool
}

func GroupMessagesForProvider(messages []OpenAIChatMessage) []ProviderMessageGroup {
	groups := make([]ProviderMessageGroup, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		message := messages[i]
		if message.Role == "assistant" && len(message.ToolCalls) > 0 {
			group := ProviderMessageGroup{messages: []OpenAIChatMessage{message}}
			for i+1 < len(messages) && messages[i+1].Role == "tool" {
				group.messages = append(group.messages, messages[i+1])
				i++
			}
			groups = append(groups, group)
			continue
		}
		groups = append(groups, ProviderMessageGroup{
			messages: []OpenAIChatMessage{message},
			system:   message.Role == "system",
		})
	}
	return groups
}

func EstimateMessageGroupBytes(group ProviderMessageGroup) int {
	total := 0
	for _, message := range group.messages {
		total += EstimateMessageBytes(message)
	}
	return total
}

// TrimMessagesForProvider trims message content to keep the estimated JSON
// payload size within budget, preventing 413 Request Entity Too Large errors.
func TrimMessagesForProvider(messages []OpenAIChatMessage, maxTotalBytes int) []OpenAIChatMessage {
	if len(messages) == 0 {
		return messages
	}
	// Work on a copy to avoid mutating the caller's slice.
	out := NormalizeMessagesForProvider(messages)

	// First pass: truncate individual messages that are excessively long.
	const maxSingleMessageBytes = 40000
	truncatedCount := 0
	for i := range out {
		if EstimateMessageBytes(out[i]) > maxSingleMessageBytes {
			out[i].Content = TruncateBytes(out[i].Content, maxSingleMessageBytes)
			out[i].ReasoningContent = TruncateBytes(out[i].ReasoningContent, maxSingleMessageBytes)
			out[i].Reasoning = TruncateBytes(out[i].Reasoning, maxSingleMessageBytes)
			truncatedCount++
		}
	}
	if truncatedCount > 0 {
		log.Printf("[adk] trimmed %d oversized message(s) to %d bytes each", truncatedCount, maxSingleMessageBytes)
	}

	// Second pass: trim older messages if total still exceeds budget.
	total := 0
	for _, m := range out {
		total += EstimateMessageBytes(m)
	}
	if total <= maxTotalBytes {
		return NormalizeMessagesForProvider(out)
	}

	groups := GroupMessagesForProvider(out)
	systemGroups := make([]ProviderMessageGroup, 0, len(groups))
	systemBytes := 0
	for _, group := range groups {
		if !group.system {
			continue
		}
		systemGroups = append(systemGroups, group)
		systemBytes += EstimateMessageGroupBytes(group)
	}
	remaining := maxTotalBytes - systemBytes
	keptGroups := make([]ProviderMessageGroup, 0, len(groups))
	droppedCount := 0
	for i := len(groups) - 1; i >= 0; i-- {
		group := groups[i]
		if group.system {
			continue
		}
		groupBytes := EstimateMessageGroupBytes(group)
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
	result := make([]OpenAIChatMessage, 0, len(out))
	for _, group := range finalGroups {
		result = append(result, group.messages...)
	}
	if droppedCount > 0 {
		log.Printf("[adk] dropped %d older message(s) to keep payload under %d bytes (was %d)", droppedCount, maxTotalBytes, total)
	}
	return NormalizeMessagesForProvider(result)
}

func (c OpenAIClient) Chat(ctx context.Context, provider jfadkmodel.Provider, apiKey string, model string, messages []OpenAIChatMessage) (string, error) {
	result, err := c.ChatDetailed(ctx, provider, apiKey, model, messages)
	if err != nil {
		return "", err
	}
	return result.Reply, nil
}

func (c OpenAIClient) ChatDetailed(ctx context.Context, provider jfadkmodel.Provider, apiKey string, model string, messages []OpenAIChatMessage) (AssistantExecutionResult, error) {
	var result AssistantExecutionResult
	streamResult, err := c.ChatStream(ctx, provider, apiKey, model, messages, nil)
	if err != nil {
		return AssistantExecutionResult{}, err
	}
	result = streamResult
	return result, nil
}

func (c OpenAIClient) SelectTools(
	ctx context.Context,
	provider jfadkmodel.Provider,
	apiKey string,
	model string,
	messages []OpenAIChatMessage,
	tools []OpenAITool,
) ([]jfadkmodel.ToolInvocation, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/chat/completions"
	if strings.TrimSpace(model) == "" {
		model = provider.Model
	}
	selectionMessages := append([]OpenAIChatMessage{}, TrimMessagesForProvider(messages, MaxProviderPayloadBytes)...)
	selectionMessages = append(selectionMessages, OpenAIChatMessage{
		Role:    "system",
		Content: "Decide which JFTrade tools are needed before answering. If no tool is useful, answer normally without tool calls.",
	})
	payload := OpenAIChatRequest{
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
	resp, err := c.HTTPClientForProvider(provider).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { besteffort.LogError(resp.Body.Close()) }()
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
	var parsed OpenAIChatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode OpenAI-compatible tool selection: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("provider returned: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, nil
	}
	return ToolInvocationsFromOpenAI(parsed.Choices[0].Message.ToolCalls), nil
}

func (c OpenAIClient) ChatStream(
	ctx context.Context,
	provider jfadkmodel.Provider,
	apiKey string,
	model string,
	messages []OpenAIChatMessage,
	onDelta func(jfadkmodel.ChatDelta) error,
) (AssistantExecutionResult, error) {
	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/chat/completions"
	if strings.TrimSpace(model) == "" {
		model = provider.Model
	}
	payload := OpenAIChatRequest{Model: model, Messages: TrimMessagesForProvider(messages, MaxProviderPayloadBytes), Temperature: 0.2, Stream: true}
	raw, err := json.Marshal(payload)
	if err != nil {
		return AssistantExecutionResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return AssistantExecutionResult{}, err
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
	resp, err := c.HTTPClientForProvider(provider).Do(req)
	if err != nil {
		return AssistantExecutionResult{}, err
	}
	defer func() { besteffort.LogError(resp.Body.Close()) }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if readErr != nil {
			return AssistantExecutionResult{}, readErr
		}
		var parsed OpenAIChatResponse
		if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error != nil && parsed.Error.Message != "" {
			return AssistantExecutionResult{}, fmt.Errorf("provider returned %d: %s", resp.StatusCode, parsed.Error.Message)
		}
		return AssistantExecutionResult{}, fmt.Errorf("provider returned %d", resp.StatusCode)
	}

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return c.ReadStreamingResponse(resp.Body, onDelta)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return AssistantExecutionResult{}, err
	}
	var parsed OpenAIChatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return AssistantExecutionResult{}, fmt.Errorf("decode OpenAI-compatible response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return AssistantExecutionResult{}, fmt.Errorf("provider returned: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return AssistantExecutionResult{}, fmt.Errorf("provider returned no choices")
	}
	return c.EmitStructuredMessage(parsed.Choices[0].Message, onDelta)
}

func (c OpenAIClient) ReadStreamingResponse(body io.Reader, onDelta func(jfadkmodel.ChatDelta) error) (AssistantExecutionResult, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64<<10), 2<<20)
	stream := &OpenAIStreamAccumulator{}
	for scanner.Scan() {
		if err := stream.consumeLine(scanner.Text(), onDelta); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return AssistantExecutionResult{}, err
		}
	}
	if err := scanner.Err(); err != nil {
		return AssistantExecutionResult{}, err
	}
	if err := stream.flushEvent(onDelta); err != nil && !errors.Is(err, io.EOF) {
		return AssistantExecutionResult{}, err
	}
	if err := stream.flushTail(onDelta); err != nil {
		return AssistantExecutionResult{}, err
	}
	return stream.result()
}

func (s *OpenAIStreamAccumulator) consumeLine(line string, onDelta func(jfadkmodel.ChatDelta) error) error {
	if line == "" {
		return s.flushEvent(onDelta)
	}
	if after, ok := strings.CutPrefix(line, "data:"); ok {
		s.dataLines = append(s.dataLines, strings.TrimSpace(after))
	}
	return nil
}

func (s *OpenAIStreamAccumulator) flushEvent(onDelta func(jfadkmodel.ChatDelta) error) error {
	if len(s.dataLines) == 0 {
		return nil
	}
	payload := strings.Join(s.dataLines, "\n")
	s.dataLines = s.dataLines[:0]
	if strings.TrimSpace(payload) == "" {
		return nil
	}
	if strings.TrimSpace(payload) == "[DONE]" {
		return io.EOF
	}
	var parsed OpenAIChatStreamResponse
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return fmt.Errorf("decode OpenAI-compatible stream chunk: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return fmt.Errorf("provider returned: %s", parsed.Error.Message)
	}
	for _, choice := range parsed.Choices {
		if err := s.appendChoice(choice.Delta.Content, choice.Delta.ReasoningContent, choice.Delta.Reasoning, onDelta); err != nil {
			return err
		}
		if choice.Message.Content != "" || choice.Message.ReasoningContent != "" || choice.Message.Reasoning != "" {
			if err := s.appendChoice(choice.Message.Content, choice.Message.ReasoningContent, choice.Message.Reasoning, onDelta); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *OpenAIStreamAccumulator) appendChoice(content string, reasoningContent string, reasoning string, onDelta func(jfadkmodel.ChatDelta) error) error {
	return AppendStreamChoice(&s.splitter, &s.replyBuilder, &s.reasoningBuilder, content, reasoningContent, reasoning, onDelta)
}

func (s *OpenAIStreamAccumulator) flushTail(onDelta func(jfadkmodel.ChatDelta) error) error {
	replyTail, reasoningTail := s.splitter.Flush()
	if replyTail == "" && reasoningTail == "" {
		return nil
	}
	s.replyBuilder.WriteString(replyTail)
	s.reasoningBuilder.WriteString(reasoningTail)
	if onDelta != nil {
		return onDelta(jfadkmodel.ChatDelta{Reply: replyTail, ReasoningContent: reasoningTail})
	}
	return nil
}

func (s *OpenAIStreamAccumulator) result() (AssistantExecutionResult, error) {
	result := AssistantExecutionResult{
		Reply:            strings.TrimSpace(s.replyBuilder.String()),
		ReasoningContent: strings.TrimSpace(s.reasoningBuilder.String()),
	}
	if result.Reply == "" {
		return AssistantExecutionResult{}, fmt.Errorf("provider returned an empty reply")
	}
	return result, nil
}

func (c OpenAIClient) EmitStructuredMessage(message OpenAIChatMessage, onDelta func(jfadkmodel.ChatDelta) error) (AssistantExecutionResult, error) {
	reply, reasoning := ExtractVisibleAndReasoningText(message.Content, message.ReasoningContent, message.Reasoning)
	result := AssistantExecutionResult{
		Reply:            strings.TrimSpace(reply),
		ReasoningContent: strings.TrimSpace(reasoning),
	}
	if result.Reply == "" {
		return AssistantExecutionResult{}, fmt.Errorf("provider returned an empty reply")
	}
	if onDelta != nil {
		if err := onDelta(jfadkmodel.ChatDelta{Reply: result.Reply, ReasoningContent: result.ReasoningContent}); err != nil {
			return AssistantExecutionResult{}, err
		}
	}
	return result, nil
}

func AppendStreamChoice(
	splitter *LegacyAssistantContentSplitter,
	replyBuilder *strings.Builder,
	reasoningBuilder *strings.Builder,
	content string,
	reasoningContent string,
	reasoning string,
	onDelta func(jfadkmodel.ChatDelta) error,
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
	return onDelta(jfadkmodel.ChatDelta{
		Reply:            replyDelta,
		ReasoningContent: reasoningFromContent + reasoningDelta,
	})
}

func ToolInvocationsFromOpenAI(calls []OpenAIToolCall) []jfadkmodel.ToolInvocation {
	invocations := make([]jfadkmodel.ToolInvocation, 0, len(calls))
	for _, call := range calls {
		name := RestoreToolNameFromOpenAI(strings.TrimSpace(call.Function.Name))
		if name == "" {
			continue
		}
		input := map[string]any{}
		if strings.TrimSpace(call.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
				input = map[string]any{"rawParameters": call.Function.Arguments, "parseError": err.Error()}
			}
		}
		invocations = append(invocations, jfadkmodel.ToolInvocation{Name: name, Input: input})
		if len(invocations) >= 5 {
			break
		}
	}
	return invocations
}
