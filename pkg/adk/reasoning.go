package adk

import "strings"

// Legacy providers may inline reasoning inside assistant text using custom tags.
// The ADK-native path should rely on genai.Part.Thought instead; this parser is
// retained only as a narrow provider compatibility shim.
var legacyReasoningTags = map[string]reasoningMode{
	"<think>":      reasoningModeReasoning,
	"</think>":     reasoningModeReply,
	"<reasoning>":  reasoningModeReasoning,
	"</reasoning>": reasoningModeReply,
}

const maxReasoningTagLength = len("</reasoning>")

type reasoningMode int

const (
	reasoningModeReply reasoningMode = iota
	reasoningModeReasoning
)

type legacyAssistantContentSplitter struct {
	mode      reasoningMode
	tagBuffer strings.Builder
}

func (splitter *legacyAssistantContentSplitter) Push(chunk string) (string, string) {
	if chunk == "" {
		return "", ""
	}
	var reply strings.Builder
	var reasoning strings.Builder
	for _, r := range chunk {
		if splitter.tagBuffer.Len() > 0 || r == '<' {
			if splitter.tagBuffer.Len() == 0 {
				splitter.tagBuffer.WriteRune(r)
				continue
			}
			splitter.tagBuffer.WriteRune(r)
			candidate := splitter.tagBuffer.String()
			lowered := strings.ToLower(candidate)
			if mode, ok := legacyReasoningTags[lowered]; ok && strings.HasSuffix(candidate, ">") {
				splitter.mode = mode
				splitter.tagBuffer.Reset()
				continue
			}
			if !isReasoningTagPrefix(lowered) {
				appendByMode(&reply, &reasoning, splitter.mode, candidate)
				splitter.tagBuffer.Reset()
			}
			continue
		}
		appendRuneByMode(&reply, &reasoning, splitter.mode, r)
	}
	if splitter.tagBuffer.Len() > maxReasoningTagLength {
		appendByMode(&reply, &reasoning, splitter.mode, splitter.tagBuffer.String())
		splitter.tagBuffer.Reset()
	}
	return reply.String(), reasoning.String()
}

func (splitter *legacyAssistantContentSplitter) Flush() (string, string) {
	if splitter.tagBuffer.Len() == 0 {
		return "", ""
	}
	value := splitter.tagBuffer.String()
	splitter.tagBuffer.Reset()
	if strings.EqualFold(value, "<think>") || strings.EqualFold(value, "<reasoning>") {
		splitter.mode = reasoningModeReasoning
		return "", ""
	}
	if strings.EqualFold(value, "</think>") || strings.EqualFold(value, "</reasoning>") {
		splitter.mode = reasoningModeReply
		return "", ""
	}
	if splitter.mode == reasoningModeReasoning {
		return "", value
	}
	return value, ""
}

func splitLegacyAssistantContent(content string) (string, string) {
	var splitter legacyAssistantContentSplitter
	reply, reasoning := splitter.Push(content)
	replyTail, reasoningTail := splitter.Flush()
	return reply + replyTail, reasoning + reasoningTail
}

func extractVisibleAndReasoningText(content string, reasoningParts ...string) (string, string) {
	replyText := strings.TrimSpace(content)
	reasoningText := mergeReasoningBlocks(reasoningParts...)
	if reasoningText != "" {
		return replyText, reasoningText
	}
	replyText, reasoningText = splitLegacyAssistantContent(content)
	return strings.TrimSpace(replyText), strings.TrimSpace(reasoningText)
}

func mergeReasoningBlocks(parts ...string) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return strings.Join(values, "\n")
}

func isReasoningTagPrefix(candidate string) bool {
	for tag := range legacyReasoningTags {
		if strings.HasPrefix(tag, candidate) {
			return true
		}
	}
	return false
}

func appendByMode(reply *strings.Builder, reasoning *strings.Builder, mode reasoningMode, value string) {
	if mode == reasoningModeReasoning {
		reasoning.WriteString(value)
		return
	}
	reply.WriteString(value)
}

func appendRuneByMode(reply *strings.Builder, reasoning *strings.Builder, mode reasoningMode, value rune) {
	if mode == reasoningModeReasoning {
		reasoning.WriteRune(value)
		return
	}
	reply.WriteRune(value)
}
