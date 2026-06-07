package adk

import "strings"

var reasoningTags = map[string]reasoningMode{
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

type assistantContentSplitter struct {
	mode      reasoningMode
	tagBuffer strings.Builder
}

func (splitter *assistantContentSplitter) Push(chunk string) (string, string) {
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
			if mode, ok := reasoningTags[lowered]; ok && strings.HasSuffix(candidate, ">") {
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

func (splitter *assistantContentSplitter) Flush() (string, string) {
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

func splitAssistantContent(content string) (string, string) {
	var splitter assistantContentSplitter
	reply, reasoning := splitter.Push(content)
	replyTail, reasoningTail := splitter.Flush()
	return reply + replyTail, reasoning + reasoningTail
}

func isReasoningTagPrefix(candidate string) bool {
	for tag := range reasoningTags {
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
