package adk

import (
	"context"
	"sort"
	"strings"
	"time"
)

type timelinePrimitive struct {
	id        string
	sessionID string
	runID     string
	kind      string
	createdAt string
	order     int
	text      string
	toolCall  *ToolCall
	approval  *Approval
}

func (s *Store) SessionTimeline(ctx context.Context, sessionID string) ([]TimelineEntry, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || s == nil {
		return nil, false, nil
	}
	session, ok, err := s.Session(ctx, sessionID)
	if err != nil || !ok {
		return nil, false, err
	}
	projection, ok, err := s.SessionProjection(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	runs, err := s.sessionRuns(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	timeline := buildSessionTimeline(session, projection.Messages, runs)
	if len(timeline) == 0 {
		return nil, false, nil
	}
	return normalizeTimelineEntries(timeline), true, nil
}

func (s *Store) sessionRuns(ctx context.Context, sessionID string) ([]Run, error) {
	const limit = 100
	runs := make([]Run, 0, limit)
	offset := 0
	for {
		page, _, err := s.ListRunsPage(ctx, "", "", sessionID, limit, offset)
		if err != nil {
			return nil, err
		}
		runs = append(runs, page...)
		if len(page) < limit {
			break
		}
		offset += len(page)
	}
	sort.SliceStable(runs, func(i, j int) bool {
		return compareTimelineKeys(runs[i].CreatedAt, 0, runs[i].ID, runs[j].CreatedAt, 0, runs[j].ID)
	})
	return runs, nil
}

func buildSessionTimeline(session Session, messages []TranscriptEntry, runs []Run) []TimelineEntry {
	sortedMessages := append([]TranscriptEntry(nil), messages...)
	sort.SliceStable(sortedMessages, func(i, j int) bool {
		return compareTimelineKeys(sortedMessages[i].CreatedAt, 0, sortedMessages[i].ID, sortedMessages[j].CreatedAt, 0, sortedMessages[j].ID)
	})

	sortedRuns := append([]Run(nil), runs...)
	sort.SliceStable(sortedRuns, func(i, j int) bool {
		return compareTimelineKeys(sortedRuns[i].CreatedAt, 0, sortedRuns[i].ID, sortedRuns[j].CreatedAt, 0, sortedRuns[j].ID)
	})

	runsByID := make(map[string]Run, len(sortedRuns))
	runsByFinalMessageID := make(map[string]Run, len(sortedRuns))
	for _, run := range sortedRuns {
		runsByID[run.ID] = run
		if finalID := strings.TrimSpace(run.FinalMessageID); finalID != "" {
			runsByFinalMessageID[finalID] = run
		}
	}

	raw := make([]timelinePrimitive, 0, len(sortedMessages)+(len(sortedRuns)*3))
	processedRuns := map[string]struct{}{}
	for _, message := range sortedMessages {
		switch strings.ToLower(strings.TrimSpace(message.Role)) {
		case "user":
			raw = append(raw, timelinePrimitive{
				id:        message.ID,
				sessionID: session.ID,
				runID:     strings.TrimSpace(message.RunID),
				kind:      TimelineKindUserMessage,
				createdAt: message.CreatedAt,
				order:     10,
				text:      strings.TrimSpace(message.Content),
			})
		default:
			run, ok := runsByID[strings.TrimSpace(message.RunID)]
			if !ok {
				run, ok = runsByFinalMessageID[strings.TrimSpace(message.ID)]
			}
			if ok {
				processedRuns[run.ID] = struct{}{}
				raw = append(raw, timelinePrimitivesForRunMessage(session.ID, run, message)...)
				continue
			}
			raw = append(raw, timelinePrimitivesForLooseAssistantMessage(session.ID, message)...)
		}
	}

	for _, run := range sortedRuns {
		if _, ok := processedRuns[run.ID]; ok {
			continue
		}
		raw = append(raw, timelinePrimitivesForOrphanRun(session.ID, run)...)
	}

	return normalizeTimelineEntries(groupTimelinePrimitives(raw))
}

func timelinePrimitivesForRunMessage(sessionID string, run Run, message TranscriptEntry) []timelinePrimitive {
	primitives := make([]timelinePrimitive, 0, len(run.ToolCalls)+len(run.PendingApprovals)+4)
	preTextTime := runTextAnchor(run, message.CreatedAt)
	if preReasoning := strings.TrimSpace(run.PreToolReasoning); preReasoning != "" {
		primitives = append(primitives, timelinePrimitive{
			id:        message.ID + ":pre-reasoning",
			sessionID: sessionID,
			runID:     run.ID,
			kind:      TimelineKindAssistantReasoning,
			createdAt: preTextTime,
			order:     20,
			text:      preReasoning,
		})
	}
	if preContent := strings.TrimSpace(run.PreToolContent); preContent != "" {
		primitives = append(primitives, timelinePrimitive{
			id:        message.ID + ":pre-message",
			sessionID: sessionID,
			runID:     run.ID,
			kind:      TimelineKindAssistantMessage,
			createdAt: preTextTime,
			order:     30,
			text:      preContent,
		})
	}
	primitives = append(primitives, timelinePrimitivesForRunActivity(sessionID, run)...)

	finalReasoning := stripTimelinePrefix(message.ReasoningContent, run.PreToolReasoning)
	if finalReasoning != "" {
		primitives = append(primitives, timelinePrimitive{
			id:        message.ID + ":reasoning",
			sessionID: sessionID,
			runID:     run.ID,
			kind:      TimelineKindAssistantReasoning,
			createdAt: message.CreatedAt,
			order:     60,
			text:      finalReasoning,
		})
	}
	finalContent := stripTimelinePrefix(message.Content, run.PreToolContent)
	if finalContent != "" {
		primitives = append(primitives, timelinePrimitive{
			id:        message.ID,
			sessionID: sessionID,
			runID:     run.ID,
			kind:      TimelineKindAssistantMessage,
			createdAt: message.CreatedAt,
			order:     70,
			text:      finalContent,
		})
	}
	return primitives
}

func timelinePrimitivesForLooseAssistantMessage(sessionID string, message TranscriptEntry) []timelinePrimitive {
	primitives := make([]timelinePrimitive, 0, 2)
	if reasoning := strings.TrimSpace(message.ReasoningContent); reasoning != "" {
		primitives = append(primitives, timelinePrimitive{
			id:        message.ID + ":reasoning",
			sessionID: sessionID,
			runID:     strings.TrimSpace(message.RunID),
			kind:      TimelineKindAssistantReasoning,
			createdAt: message.CreatedAt,
			order:     60,
			text:      reasoning,
		})
	}
	if content := strings.TrimSpace(message.Content); content != "" {
		primitives = append(primitives, timelinePrimitive{
			id:        message.ID,
			sessionID: sessionID,
			runID:     strings.TrimSpace(message.RunID),
			kind:      TimelineKindAssistantMessage,
			createdAt: message.CreatedAt,
			order:     70,
			text:      content,
		})
	}
	return primitives
}

func timelinePrimitivesForOrphanRun(sessionID string, run Run) []timelinePrimitive {
	primitives := make([]timelinePrimitive, 0, len(run.ToolCalls)+len(run.PendingApprovals)+2)
	preTextTime := runTextAnchor(run, run.UpdatedAt)
	if preReasoning := strings.TrimSpace(run.PreToolReasoning); preReasoning != "" {
		primitives = append(primitives, timelinePrimitive{
			id:        "run-pre-reasoning:" + run.ID,
			sessionID: sessionID,
			runID:     run.ID,
			kind:      TimelineKindAssistantReasoning,
			createdAt: preTextTime,
			order:     20,
			text:      preReasoning,
		})
	}
	if preContent := strings.TrimSpace(run.PreToolContent); preContent != "" {
		primitives = append(primitives, timelinePrimitive{
			id:        "run-pre-message:" + run.ID,
			sessionID: sessionID,
			runID:     run.ID,
			kind:      TimelineKindAssistantMessage,
			createdAt: preTextTime,
			order:     30,
			text:      preContent,
		})
	}
	return append(primitives, timelinePrimitivesForRunActivity(sessionID, run)...)
}

func timelinePrimitivesForRunActivity(sessionID string, run Run) []timelinePrimitive {
	approvals := pendingApprovalsOnly(run.PendingApprovals)
	primitives := make([]timelinePrimitive, 0, len(run.ToolCalls)+len(approvals))
	toolCalls := append([]ToolCall(nil), run.ToolCalls...)
	sort.SliceStable(toolCalls, func(i, j int) bool {
		return compareTimelineKeys(toolCalls[i].CreatedAt, 40, toolCalls[i].ID, toolCalls[j].CreatedAt, 40, toolCalls[j].ID)
	})
	for _, toolCall := range toolCalls {
		call := toolCall
		primitives = append(primitives, timelinePrimitive{
			id:        "tool:" + call.ID,
			sessionID: sessionID,
			runID:     run.ID,
			kind:      TimelineKindToolGroup,
			createdAt: firstNonEmpty(call.CreatedAt, call.UpdatedAt, run.UpdatedAt, run.CreatedAt),
			order:     40,
			toolCall:  &call,
		})
	}
	approvals = append([]Approval(nil), approvals...)
	sort.SliceStable(approvals, func(i, j int) bool {
		return compareTimelineKeys(approvals[i].CreatedAt, 50, approvals[i].ID, approvals[j].CreatedAt, 50, approvals[j].ID)
	})
	for _, approval := range approvals {
		item := approval
		primitives = append(primitives, timelinePrimitive{
			id:        "approval:" + item.ID,
			sessionID: sessionID,
			runID:     run.ID,
			kind:      TimelineKindApprovalGroup,
			createdAt: firstNonEmpty(item.CreatedAt, item.UpdatedAt, run.UpdatedAt, run.CreatedAt),
			order:     50,
			approval:  &item,
		})
	}
	return primitives
}

func groupTimelinePrimitives(primitives []timelinePrimitive) []TimelineEntry {
	if len(primitives) == 0 {
		return []TimelineEntry{}
	}
	sort.SliceStable(primitives, func(i, j int) bool {
		return compareTimelineKeys(primitives[i].createdAt, primitives[i].order, primitives[i].id, primitives[j].createdAt, primitives[j].order, primitives[j].id)
	})

	result := make([]TimelineEntry, 0, len(primitives))
	for _, primitive := range primitives {
		switch {
		case primitive.toolCall != nil:
			if len(result) > 0 && result[len(result)-1].Kind == TimelineKindToolGroup && result[len(result)-1].RunID == primitive.runID {
				result[len(result)-1].ToolCalls = append(result[len(result)-1].ToolCalls, *primitive.toolCall)
				continue
			}
			result = append(result, TimelineEntry{
				ID:        primitive.id,
				SessionID: primitive.sessionID,
				RunID:     primitive.runID,
				Kind:      TimelineKindToolGroup,
				CreatedAt: primitive.createdAt,
				Status:    TimelineStatusFinal,
				ToolCalls: []ToolCall{*primitive.toolCall},
			})
		case primitive.approval != nil:
			if len(result) > 0 && result[len(result)-1].Kind == TimelineKindApprovalGroup && result[len(result)-1].RunID == primitive.runID {
				result[len(result)-1].Approvals = append(result[len(result)-1].Approvals, *primitive.approval)
				continue
			}
			result = append(result, TimelineEntry{
				ID:        primitive.id,
				SessionID: primitive.sessionID,
				RunID:     primitive.runID,
				Kind:      TimelineKindApprovalGroup,
				CreatedAt: primitive.createdAt,
				Status:    TimelineStatusFinal,
				Approvals: []Approval{*primitive.approval},
			})
		default:
			if strings.TrimSpace(primitive.text) == "" {
				continue
			}
			result = append(result, TimelineEntry{
				ID:        primitive.id,
				SessionID: primitive.sessionID,
				RunID:     primitive.runID,
				Kind:      primitive.kind,
				CreatedAt: primitive.createdAt,
				Status:    TimelineStatusFinal,
				Text:      strings.TrimSpace(primitive.text),
			})
		}
	}
	for index := range result {
		result[index].Sequence = index + 1
	}
	return result
}

func runTextAnchor(run Run, preferredTime string) string {
	candidates := []string{firstRunToolTime(run), firstRunApprovalTime(run), preferredTime, run.UpdatedAt, run.CreatedAt}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return nowString()
}

func firstRunToolTime(run Run) string {
	earliest := ""
	for _, call := range run.ToolCalls {
		candidate := firstNonEmpty(call.CreatedAt, call.UpdatedAt)
		if candidate == "" {
			continue
		}
		if earliest == "" || compareTimelineKeys(candidate, 0, "", earliest, 0, "") {
			earliest = candidate
		}
	}
	return earliest
}

func firstRunApprovalTime(run Run) string {
	earliest := ""
	for _, approval := range pendingApprovalsOnly(run.PendingApprovals) {
		candidate := firstNonEmpty(approval.CreatedAt, approval.UpdatedAt)
		if candidate == "" {
			continue
		}
		if earliest == "" || compareTimelineKeys(candidate, 0, "", earliest, 0, "") {
			earliest = candidate
		}
	}
	return earliest
}

func stripTimelinePrefix(value string, prefix string) string {
	normalizedValue := strings.TrimSpace(value)
	normalizedPrefix := strings.TrimSpace(prefix)
	if normalizedValue == "" || normalizedPrefix == "" {
		return normalizedValue
	}
	if normalizedValue == normalizedPrefix {
		return ""
	}
	if strings.HasPrefix(normalizedValue, normalizedPrefix) {
		return strings.TrimSpace(normalizedValue[len(normalizedPrefix):])
	}
	return normalizedValue
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func compareTimelineKeys(leftAt string, leftOrder int, leftID string, rightAt string, rightOrder int, rightID string) bool {
	leftTime, leftOK := parseTimelineTime(leftAt)
	rightTime, rightOK := parseTimelineTime(rightAt)
	switch {
	case leftOK && rightOK:
		if !leftTime.Equal(rightTime) {
			return leftTime.Before(rightTime)
		}
	case leftOK:
		return true
	case rightOK:
		return false
	default:
		if leftAt != rightAt {
			return leftAt < rightAt
		}
	}
	if leftOrder != rightOrder {
		return leftOrder < rightOrder
	}
	return leftID < rightID
}

func parseTimelineTime(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}
