package adk

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	"google.golang.org/genai"
)

func projectVisibleSessionEvents(events []*adksession.Event, compacted bool, currentCutoff int, recentStart int, protectedStart int) projectedVisibleSession {
	if len(events) == 0 {
		return projectedVisibleSession{}
	}
	if currentCutoff < 0 {
		currentCutoff = 0
	}
	if recentStart < currentCutoff {
		recentStart = currentCutoff
	}
	if protectedStart < currentCutoff {
		protectedStart = currentCutoff
	}

	projected := make([]projectedVisibleEvent, 0, len(events))
	appendEvent := func(bucket contextProjectionBucket, event *adksession.Event) {
		effective, trimmedCount := limitVisibleEventForContext(event)
		projected = append(projected, projectedVisibleEvent{raw: event, effective: effective, bucket: bucket, trimmedCount: trimmedCount})
	}

	if !compacted {
		for _, event := range events {
			appendEvent(contextProjectionBucketOtherVisible, event)
		}
	} else {
		for index := recentStart; index < minInt(protectedStart, len(events)); index++ {
			if isUserEvent(events[index]) {
				appendEvent(contextProjectionBucketRecentUser, events[index])
			}
		}
		if protectedStart < len(events) {
			for _, event := range events[protectedStart:] {
				appendEvent(contextProjectionBucketProtectedTail, event)
			}
		}
	}

	result := projectedVisibleSession{events: make([]*adksession.Event, 0, len(projected))}
	for _, item := range projected {
		result.events = append(result.events, item.effective)
		addProjectedEventTokens(&result.rawBreakdown, item.bucket, item.raw)
		addProjectedEventTokens(&result.effectiveBreakdown, item.bucket, item.effective)
		result.trimmedToolResponseCount += item.trimmedCount
	}
	return result
}

func addProjectedEventTokens(breakdown *SessionContextBreakdown, bucket contextProjectionBucket, event *adksession.Event) {
	if breakdown == nil || event == nil {
		return
	}
	tokens := estimateEventTokens(event)
	switch bucket {
	case contextProjectionBucketRecentUser:
		breakdown.RecentUserTokens += tokens
	case contextProjectionBucketProtectedTail:
		breakdown.ProtectedTailTokens += tokens
	default:
		breakdown.OtherVisibleTokens += tokens
	}
}

func limitVisibleEventForContext(event *adksession.Event) (*adksession.Event, int) {
	if event == nil || event.Content == nil || len(event.Content.Parts) == 0 {
		return event, 0
	}
	parts := make([]*genai.Part, len(event.Content.Parts))
	trimmedCount := 0
	needsClone := false
	for index, part := range event.Content.Parts {
		if part == nil {
			continue
		}
		partCopy := *part
		if part.FunctionCall != nil {
			partCopy.FunctionCall = new(*part.FunctionCall)
		}
		if part.FunctionResponse != nil {
			functionResponseCopy := *part.FunctionResponse
			limitedResponse, trimmed := limitToolOutputWithMetadata(part.FunctionResponse.Response)
			if trimmed {
				functionResponseCopy.Response = asToolResponseMap(limitedResponse)
				trimmedCount++
				needsClone = true
			}
			partCopy.FunctionResponse = &functionResponseCopy
		}
		parts[index] = &partCopy
	}
	if !needsClone {
		return event, 0
	}
	contentCopy := *event.Content
	contentCopy.Parts = parts
	eventCopy := *event
	eventCopy.Content = &contentCopy
	return &eventCopy, trimmedCount
}

func asToolResponseMap(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{"result": value}
}

func contextStatus(tokens int, windowTokens int) string {
	if windowTokens <= 0 {
		return ContextStatusUnknown
	}
	ratio := float64(tokens) / float64(windowTokens)
	switch {
	case ratio >= contextAggressiveThreshold:
		return ContextStatusCritical
	case ratio >= contextAutoCompactThresh:
		return ContextStatusNearLimit
	case ratio >= contextWarnThreshold:
		return ContextStatusWarning
	default:
		return ContextStatusHealthy
	}
}

func compactionModeLabel(request SessionCompactRequest) string {
	if request.Trigger == "auto" && request.Mode != "aggressive" {
		return "auto"
	}
	if request.Mode == "aggressive" {
		return "aggressive"
	}
	return "manual"
}

func normalizeCompactMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "aggressive") {
		return "aggressive"
	}
	return "normal"
}

func compactionCutoff(events []*adksession.Event, recentWindow int) int {
	if len(events) == 0 {
		return 0
	}
	recentStart := recentUserEventStart(events, recentWindow)
	protectedStart := protectedTailStart(events)
	if protectedStart < recentStart {
		return protectedStart
	}
	return recentStart
}

func recentUserEventStart(events []*adksession.Event, recentWindow int) int {
	if len(events) == 0 {
		return 0
	}
	userHits := 0
	for index := len(events) - 1; index >= 0; index-- {
		if !isUserEvent(events[index]) {
			continue
		}
		userHits++
		if userHits >= recentWindow {
			return index
		}
	}
	return 0
}

func retainedRecentUserCount(events []*adksession.Event, recentStart int, protectedStart int) int {
	if recentStart >= len(events) {
		return 0
	}
	if protectedStart < recentStart {
		protectedStart = recentStart
	}
	count := 0
	for index := recentStart; index < minInt(protectedStart, len(events)); index++ {
		if isUserEvent(events[index]) {
			count++
		}
	}
	return count
}

func protectedTailStart(events []*adksession.Event) int {
	resolvedApprovalCallIDs := resolvedApprovalIDs(events)
	protectedStart := len(events)
	for index, event := range events {
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part == nil || part.FunctionCall == nil {
				continue
			}
			call := part.FunctionCall
			if call.Name != toolconfirmation.FunctionCallName {
				continue
			}
			if _, ok := resolvedApprovalCallIDs[strings.TrimSpace(call.ID)]; ok {
				continue
			}
			candidate := index
			if original, err := toolconfirmation.OriginalCallFrom(call); err == nil {
				if originalIndex := functionCallEventIndex(events, original.ID, index); originalIndex >= 0 {
					candidate = originalIndex
				}
			}
			if candidate < protectedStart {
				protectedStart = candidate
			}
		}
	}
	return protectedStart
}

func isUserEvent(event *adksession.Event) bool {
	if event == nil || event.Content == nil {
		return false
	}
	role := strings.ToLower(strings.TrimSpace(event.Content.Role))
	if role == strings.ToLower(genai.RoleUser) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(event.Author), "user")
}

func resolvedApprovalIDs(events []*adksession.Event) map[string]struct{} {
	resolved := map[string]struct{}{}
	for _, event := range events {
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part == nil || part.FunctionResponse == nil {
				continue
			}
			response := part.FunctionResponse
			if response.Name != toolconfirmation.FunctionCallName {
				continue
			}
			if id := strings.TrimSpace(response.ID); id != "" {
				resolved[id] = struct{}{}
			}
		}
	}
	return resolved
}

func functionCallEventIndex(events []*adksession.Event, functionCallID string, through int) int {
	functionCallID = strings.TrimSpace(functionCallID)
	if functionCallID == "" {
		return -1
	}
	if through >= len(events) {
		through = len(events) - 1
	}
	for index := 0; index <= through; index++ {
		event := events[index]
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part != nil && part.FunctionCall != nil && strings.TrimSpace(part.FunctionCall.ID) == functionCallID {
				return index
			}
		}
	}
	return -1
}

func buildHandoffSummary(existing []HandoffSegment, events []*adksession.Event, mode string) string {
	lines := make([]string, 0, len(events)+len(existing)+4)
	if len(existing) > 0 {
		lines = append(lines, "Prior handoff:")
		for _, segment := range existing {
			if text := strings.TrimSpace(segment.Summary); text != "" {
				lines = append(lines, "- "+clipSummaryLine(text, 220))
			}
		}
	}
	lines = append(lines, "Conversation material:")
	maxLineLen := 220
	maxLines := 24
	if mode == "aggressive" {
		maxLineLen = 140
		maxLines = 12
	}
	for _, event := range events {
		for _, line := range summarizeEvent(event, maxLineLen) {
			if strings.TrimSpace(line) == "" {
				continue
			}
			lines = append(lines, "- "+line)
			if len(lines) >= maxLines {
				break
			}
		}
		if len(lines) >= maxLines {
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func summarizeEvent(event *adksession.Event, maxLineLen int) []string {
	if event == nil || event.Content == nil {
		return nil
	}
	lines := []string{}
	role := strings.TrimSpace(event.Content.Role)
	for _, part := range event.Content.Parts {
		if part == nil {
			continue
		}
		if text := strings.TrimSpace(part.Text); text != "" {
			prefix := "Assistant"
			if strings.EqualFold(role, genai.RoleUser) {
				prefix = "User"
			}
			lines = append(lines, prefix+": "+clipSummaryLine(text, maxLineLen))
		}
		if part.FunctionCall != nil {
			if part.FunctionCall.Name == toolconfirmation.FunctionCallName {
				lines = append(lines, "Approval requested for a protected tool action.")
				continue
			}
			lines = append(lines, fmt.Sprintf("Tool call %s args=%s", part.FunctionCall.Name, clipSummaryLine(marshalCompactJSON(part.FunctionCall.Args), maxLineLen)))
		}
		if part.FunctionResponse != nil {
			lines = append(lines, fmt.Sprintf("Tool result %s => %s", part.FunctionResponse.Name, clipSummaryLine(marshalCompactJSON(part.FunctionResponse.Response), maxLineLen)))
		}
	}
	return lines
}

func clipSummaryLine(text string, maxLen int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if maxLen <= 0 || len([]rune(text)) <= maxLen {
		return text
	}
	return string([]rune(text)[:maxLen]) + "..."
}

func marshalCompactJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(raw)
}

func estimateTextTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return maxInt(1, int(math.Ceil(float64(len([]byte(text)))/4.0)))
}

func estimateInstructionTokens(agent Agent) int {
	return estimateTextTokens(strings.TrimSpace(agent.Instruction))
}

func estimateHandoffTokens(segments []HandoffSegment) int {
	if len(segments) == 0 {
		return 0
	}
	return estimateTextTokens("Session handoff summaries:\n" + joinSegmentSummaries(segments))
}

func estimatePendingUserTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return estimateTextTokens("User: " + strings.TrimSpace(text))
}

func estimateToolDeclarationTokens(agent Agent, tools *ToolRegistry) int {
	if tools == nil {
		return 0
	}
	total := 0
	for _, descriptor := range ToolDescriptorsForAgent(agent, tools) {
		payload, jftradeErr4 := json.Marshal(map[string]any{
			"name":        descriptor.Name,
			"description": descriptor.Description,
			"schema":      descriptor.InputSchema,
		})
		jftradeLogError(jftradeErr4)
		total += estimateTextTokens(string(payload))
	}
	return total
}

func estimateEventTokens(event *adksession.Event) int {
	if event == nil || event.Content == nil {
		return 0
	}
	total := 0
	for _, part := range event.Content.Parts {
		if part == nil {
			continue
		}
		if part.Text != "" {
			total += estimateTextTokens(part.Text)
		}
		if part.FunctionCall != nil {
			total += estimateTextTokens(part.FunctionCall.Name)
			total += estimateTextTokens(marshalCompactJSON(part.FunctionCall.Args))
		}
		if part.FunctionResponse != nil {
			total += estimateTextTokens(part.FunctionResponse.Name)
			total += estimateTextTokens(marshalCompactJSON(part.FunctionResponse.Response))
		}
	}
	return total
}

func joinSegmentSummaries(segments []HandoffSegment) string {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if text := strings.TrimSpace(segment.Summary); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func latestSegmentPreview(segments []HandoffSegment) string {
	if len(segments) == 0 {
		return ""
	}
	return strings.TrimSpace(segments[len(segments)-1].Summary)
}

func maxActiveSegmentEnd(segments []HandoffSegment) int {
	end := 0
	for _, segment := range segments {
		if segment.Active && segment.EndEventIndex > end {
			end = segment.EndEventIndex
		}
	}
	return end
}

func nextHandoffSequence(segments []HandoffSegment) int {
	next := 1
	for _, segment := range segments {
		if segment.Sequence >= next {
			next = segment.Sequence + 1
		}
	}
	return next
}

func eventSlice(events adksession.Events) []*adksession.Event {
	if events == nil {
		return nil
	}
	items := make([]*adksession.Event, 0, events.Len())
	for event := range events.All() {
		items = append(items, event)
	}
	return items
}
