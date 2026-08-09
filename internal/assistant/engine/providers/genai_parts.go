package providers

import (
	"strings"

	"google.golang.org/genai"
)

func RawVisibleTextFromParts(parts []*genai.Part) (string, string) {
	var reply strings.Builder
	var reasoning strings.Builder
	for _, part := range parts {
		if part == nil || part.Text == "" {
			continue
		}
		if part.Thought {
			reasoning.WriteString(part.Text)
			continue
		}
		reply.WriteString(part.Text)
	}
	return reply.String(), reasoning.String()
}

func PartsFromReplyAndReasoning(reply string, reasoning string) []*genai.Part {
	parts := make([]*genai.Part, 0, 2)
	if trimmedReasoning := strings.TrimSpace(reasoning); trimmedReasoning != "" {
		parts = append(parts, &genai.Part{Text: trimmedReasoning, Thought: true})
	}
	if trimmedReply := strings.TrimSpace(reply); trimmedReply != "" {
		parts = append(parts, &genai.Part{Text: trimmedReply})
	}
	return parts
}

func RawPartsFromReplyAndReasoning(reply string, reasoning string) []*genai.Part {
	parts := make([]*genai.Part, 0, 2)
	if reasoning != "" {
		parts = append(parts, &genai.Part{Text: reasoning, Thought: true})
	}
	if reply != "" {
		parts = append(parts, &genai.Part{Text: reply})
	}
	return parts
}
