package model

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const interactionRequestUserTool = "interaction.request_user"

// TimelineStore is the read surface SessionTimeline needs from the ADK
// persistence layer. Engine/persistence StoreCore and the engine composition
// Store both satisfy it.
type TimelineStore interface {
	Session(ctx context.Context, sessionID string) (Session, bool, error)
	SessionNotices(ctx context.Context, sessionID string) ([]TimelineEntry, error)
	SessionProjection(ctx context.Context, sessionID string) (SessionProjection, bool, error)
	ListRunsPage(ctx context.Context, status string, agentID string, sessionID string, limit int, offset int) ([]Run, int, error)
}

type TimelinePrimitive struct {
	ID            string
	SessionID     string
	RunID         string
	Kind          string
	CreatedAt     string
	UpdatedAt     string
	Order         int
	Status        string
	Text          string
	OriginalText  string
	ProcessedText string
	ToolCall      *ToolCall
	Approval      *Approval
	InputRequest  *InputRequest
}

func SessionTimeline(ctx context.Context, store TimelineStore, sessionID string) ([]TimelineEntry, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || store == nil {
		return nil, false, nil
	}
	session, ok, err := store.Session(ctx, sessionID)
	if err != nil || !ok {
		return nil, false, err
	}
	notices, err := store.SessionNotices(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	projection, ok, err := store.SessionProjection(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	runs, err := SessionRuns(ctx, store, sessionID)
	if err != nil {
		return nil, false, err
	}
	if !ok && len(runs) == 0 && len(notices) == 0 {
		return nil, false, nil
	}
	timeline := BuildSessionTimeline(session, projection.Messages, runs, notices)
	if len(timeline) == 0 {
		return nil, false, nil
	}
	return NormalizeTimelineEntries(timeline), true, nil
}

func SessionRuns(ctx context.Context, store TimelineStore, sessionID string) ([]Run, error) {
	if store == nil {
		return nil, fmt.Errorf("adk store is unavailable")
	}
	const limit = 100
	runs := make([]Run, 0, limit)
	offset := 0
	for {
		page, _, err := store.ListRunsPage(ctx, "", "", sessionID, limit, offset)
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
		return CompareTimelineKeys(runs[i].CreatedAt, 0, runs[i].ID, runs[j].CreatedAt, 0, runs[j].ID)
	})
	return runs, nil
}

func BuildSessionTimeline(session Session, messages []TranscriptEntry, runs []Run, notices []TimelineEntry) []TimelineEntry {
	sortedMessages := sortTimelineMessages(messages)
	sortedRuns := sortTimelineRuns(runs)
	runsByID, runsByFinalMessageID := indexTimelineRuns(sortedRuns)
	raw := make([]TimelinePrimitive, 0, len(sortedMessages)+(len(sortedRuns)*3)+len(notices))
	appendTimelineNotices(&raw, session.ID, notices)
	processedRuns := appendTimelineMessages(&raw, session.ID, sortedMessages, sortedRuns, runsByID, runsByFinalMessageID)
	appendTimelineOrphanRuns(&raw, session.ID, sortedRuns, processedRuns)
	return NormalizeTimelineEntries(GroupTimelinePrimitives(raw))
}

func sortTimelineMessages(messages []TranscriptEntry) []TranscriptEntry {
	sorted := append([]TranscriptEntry(nil), messages...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return CompareTimelineKeys(sorted[i].CreatedAt, 0, sorted[i].ID, sorted[j].CreatedAt, 0, sorted[j].ID)
	})
	return sorted
}

func sortTimelineRuns(runs []Run) []Run {
	sorted := append([]Run(nil), runs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return CompareTimelineKeys(sorted[i].CreatedAt, 0, sorted[i].ID, sorted[j].CreatedAt, 0, sorted[j].ID)
	})
	return sorted
}

func indexTimelineRuns(runs []Run) (map[string]Run, map[string]Run) {
	runsByID := make(map[string]Run, len(runs))
	runsByFinalMessageID := make(map[string]Run, len(runs))
	for _, run := range runs {
		runsByID[run.ID] = run
		if finalID := strings.TrimSpace(run.FinalMessageID); finalID != "" {
			runsByFinalMessageID[finalID] = run
		}
	}
	return runsByID, runsByFinalMessageID
}

func appendTimelineNotices(raw *[]TimelinePrimitive, sessionID string, notices []TimelineEntry) {
	for _, notice := range notices {
		if strings.TrimSpace(notice.Text) == "" {
			continue
		}
		*raw = append(*raw, TimelinePrimitive{
			ID:        strings.TrimSpace(notice.ID),
			SessionID: sessionID,
			RunID:     strings.TrimSpace(notice.RunID),
			Kind:      DefaultString(strings.TrimSpace(notice.Kind), TimelineKindContextNotice),
			CreatedAt: notice.CreatedAt,
			UpdatedAt: notice.UpdatedAt,
			Order:     15,
			Status:    strings.TrimSpace(notice.Status),
			Text:      strings.TrimSpace(notice.Text),
		})
	}
}

func appendTimelineMessages(raw *[]TimelinePrimitive, sessionID string, messages []TranscriptEntry, runs []Run, runsByID map[string]Run, runsByFinalMessageID map[string]Run) map[string]struct{} {
	processedRuns := map[string]struct{}{}
	visibleUserRuns := map[string]struct{}{}
	for _, message := range messages {
		appendTimelineMessage(raw, sessionID, message, runs, runsByID, runsByFinalMessageID, visibleUserRuns, processedRuns)
	}
	return processedRuns
}

func appendTimelineMessage(raw *[]TimelinePrimitive, sessionID string, message TranscriptEntry, runs []Run, runsByID map[string]Run, runsByFinalMessageID map[string]Run, visibleUserRuns map[string]struct{}, processedRuns map[string]struct{}) {
	if strings.EqualFold(strings.TrimSpace(message.Role), "user") {
		if primitive, ok := timelinePrimitiveForUserMessage(sessionID, message, runs, runsByID, visibleUserRuns); ok {
			*raw = append(*raw, primitive)
		}
		return
	}
	run, ok := runsByID[strings.TrimSpace(message.RunID)]
	if !ok {
		run, ok = runsByFinalMessageID[strings.TrimSpace(message.ID)]
	}
	if ok {
		processedRuns[run.ID] = struct{}{}
		*raw = append(*raw, TimelinePrimitivesForRunMessage(sessionID, run, message)...)
		return
	}
	*raw = append(*raw, timelinePrimitivesForLooseAssistantMessage(sessionID, message)...)
}

func timelinePrimitiveForUserMessage(sessionID string, message TranscriptEntry, runs []Run, runsByID map[string]Run, visibleUserRuns map[string]struct{}) (TimelinePrimitive, bool) {
	text := strings.TrimSpace(message.Content)
	prompt := ClassifyWorkflowUserPrompt(text)
	run, runOK := runsByID[strings.TrimSpace(message.RunID)]
	if !runOK && prompt.IsInternal {
		run, runOK = MatchWorkflowPromptRun(prompt, runs)
	}
	if prompt.IsHidden {
		return TimelinePrimitive{}, false
	}
	originalText := ""
	processedText := ""
	runID := strings.TrimSpace(message.RunID)
	if runOK {
		runID = strings.TrimSpace(run.ID)
		if _, seen := visibleUserRuns[runID]; seen {
			return TimelinePrimitive{}, false
		}
		visibleUserRuns[runID] = struct{}{}
		if userMessage := strings.TrimSpace(run.UserMessage); userMessage != "" && userMessage != text {
			originalText = userMessage
			processedText = text
			text = userMessage
		}
	}
	return TimelinePrimitive{
		ID:            message.ID,
		SessionID:     sessionID,
		RunID:         runID,
		Kind:          TimelineKindUserMessage,
		CreatedAt:     message.CreatedAt,
		Order:         10,
		Text:          text,
		OriginalText:  originalText,
		ProcessedText: processedText,
	}, true
}

func appendTimelineOrphanRuns(raw *[]TimelinePrimitive, sessionID string, runs []Run, processedRuns map[string]struct{}) {
	for _, run := range runs {
		if _, ok := processedRuns[run.ID]; ok {
			continue
		}
		*raw = append(*raw, TimelinePrimitivesForOrphanRun(sessionID, run)...)
	}
}

type WorkflowUserPrompt struct {
	IsInternal  bool
	IsHidden    bool
	UserMessage string
	Objective   string
}

func ClassifyWorkflowUserPrompt(text string) WorkflowUserPrompt {
	text = strings.TrimSpace(text)
	if text == "" {
		return WorkflowUserPrompt{}
	}
	switch {
	case strings.HasPrefix(text, "请推进这个目标。") && strings.Contains(text, "\n用户请求："):
		return WorkflowUserPrompt{
			IsInternal:  true,
			UserMessage: ExtractWorkflowPromptField(text, "用户请求：", ""),
			Objective:   ExtractWorkflowPromptField(text, "总体目标：", "\n用户请求："),
		}
	case strings.HasPrefix(text, "请推进这个任务编排。") && strings.Contains(text, "\n用户请求："):
		return WorkflowUserPrompt{
			IsInternal:  true,
			UserMessage: ExtractWorkflowPromptField(text, "用户请求：", ""),
			Objective:   ExtractWorkflowPromptField(text, "总体目标：", "\n用户请求："),
		}
	case strings.HasPrefix(text, "请判断是否完成目标"),
		strings.HasPrefix(text, "上一次没有调用目标裁决工具。"),
		strings.HasPrefix(text, "目标尚未完成，原因："),
		strings.HasPrefix(text, "仍有未完成 TODO。"):
		return WorkflowUserPrompt{IsInternal: true, IsHidden: true}
	default:
		return WorkflowUserPrompt{}
	}
}

func ExtractWorkflowPromptField(text string, startMarker string, endMarker string) string {
	_, after, ok := strings.Cut(text, startMarker)
	if !ok {
		return ""
	}
	value := after
	if endMarker != "" {
		if end := strings.Index(value, endMarker); end >= 0 {
			value = value[:end]
		}
	}
	return strings.TrimSpace(value)
}

func MatchWorkflowPromptRun(prompt WorkflowUserPrompt, runs []Run) (Run, bool) {
	if !prompt.IsInternal || prompt.IsHidden {
		return Run{}, false
	}
	userMessage := strings.TrimSpace(prompt.UserMessage)
	objective := strings.TrimSpace(prompt.Objective)
	if userMessage == "" && objective == "" {
		return Run{}, false
	}
	for index := len(runs) - 1; index >= 0; index-- {
		run := runs[index]
		if userMessage != "" && strings.TrimSpace(run.UserMessage) != userMessage {
			continue
		}
		if objective != "" && strings.TrimSpace(run.Objective) != "" && strings.TrimSpace(run.Objective) != objective {
			continue
		}
		return run, true
	}
	return Run{}, false
}

func TimelinePrimitivesForRunMessage(sessionID string, run Run, message TranscriptEntry) []TimelinePrimitive {
	primitives := make([]TimelinePrimitive, 0, len(run.ToolCalls)+len(run.PendingApprovals)+5)
	preTextTime := RunTextAnchor(run, message.CreatedAt)
	if preReasoning := strings.TrimSpace(run.PreToolReasoning); preReasoning != "" {
		primitives = append(primitives, TimelinePrimitive{
			ID:        message.ID + ":pre-reasoning",
			SessionID: sessionID,
			RunID:     run.ID,
			Kind:      TimelineKindAssistantReasoning,
			CreatedAt: preTextTime,
			Order:     20,
			Text:      preReasoning,
		})
	}
	if preContent := strings.TrimSpace(run.PreToolContent); preContent != "" {
		primitives = append(primitives, TimelinePrimitive{
			ID:        message.ID + ":pre-message",
			SessionID: sessionID,
			RunID:     run.ID,
			Kind:      TimelineKindAssistantMessage,
			CreatedAt: preTextTime,
			Order:     30,
			Text:      preContent,
		})
	}
	primitives = append(primitives, TimelinePrimitivesForRunActivity(sessionID, run)...)

	finalReasoning := StripTimelinePrefix(message.ReasoningContent, run.PreToolReasoning)
	if finalReasoning != "" {
		primitives = append(primitives, TimelinePrimitive{
			ID:        message.ID + ":reasoning",
			SessionID: sessionID,
			RunID:     run.ID,
			Kind:      TimelineKindAssistantReasoning,
			CreatedAt: message.CreatedAt,
			Order:     60,
			Text:      finalReasoning,
		})
	}
	finalContent := StripTimelinePrefix(message.Content, run.PreToolContent)
	if finalContent != "" {
		primitives = append(primitives, TimelinePrimitive{
			ID:        message.ID,
			SessionID: sessionID,
			RunID:     run.ID,
			Kind:      TimelineKindAssistantMessage,
			CreatedAt: message.CreatedAt,
			Order:     70,
			Text:      finalContent,
		})
	}
	return primitives
}

func timelinePrimitivesForLooseAssistantMessage(sessionID string, message TranscriptEntry) []TimelinePrimitive {
	primitives := make([]TimelinePrimitive, 0, 2)
	if reasoning := strings.TrimSpace(message.ReasoningContent); reasoning != "" {
		primitives = append(primitives, TimelinePrimitive{
			ID:        message.ID + ":reasoning",
			SessionID: sessionID,
			RunID:     strings.TrimSpace(message.RunID),
			Kind:      TimelineKindAssistantReasoning,
			CreatedAt: message.CreatedAt,
			Order:     60,
			Text:      reasoning,
		})
	}
	if content := strings.TrimSpace(message.Content); content != "" {
		primitives = append(primitives, TimelinePrimitive{
			ID:        message.ID,
			SessionID: sessionID,
			RunID:     strings.TrimSpace(message.RunID),
			Kind:      TimelineKindAssistantMessage,
			CreatedAt: message.CreatedAt,
			Order:     70,
			Text:      content,
		})
	}
	return primitives
}

func TimelinePrimitivesForOrphanRun(sessionID string, run Run) []TimelinePrimitive {
	primitives := make([]TimelinePrimitive, 0, len(run.ToolCalls)+len(run.PendingApprovals)+3)
	preTextTime := RunTextAnchor(run, run.UpdatedAt)
	if preReasoning := strings.TrimSpace(run.PreToolReasoning); preReasoning != "" {
		primitives = append(primitives, TimelinePrimitive{
			ID:        "run-pre-reasoning:" + run.ID,
			SessionID: sessionID,
			RunID:     run.ID,
			Kind:      TimelineKindAssistantReasoning,
			CreatedAt: preTextTime,
			Order:     20,
			Text:      preReasoning,
		})
	}
	if preContent := strings.TrimSpace(run.PreToolContent); preContent != "" {
		primitives = append(primitives, TimelinePrimitive{
			ID:        "run-pre-message:" + run.ID,
			SessionID: sessionID,
			RunID:     run.ID,
			Kind:      TimelineKindAssistantMessage,
			CreatedAt: preTextTime,
			Order:     30,
			Text:      preContent,
		})
	}
	return append(primitives, TimelinePrimitivesForRunActivity(sessionID, run)...)
}

func TimelinePrimitivesForRunActivity(sessionID string, run Run) []TimelinePrimitive {
	approvals := PendingApprovalsOnly(run.PendingApprovals)
	primitives := make([]TimelinePrimitive, 0, len(run.ToolCalls)+len(approvals))
	toolCalls := append([]ToolCall(nil), run.ToolCalls...)
	sort.SliceStable(toolCalls, func(i, j int) bool {
		return CompareTimelineKeys(toolCalls[i].CreatedAt, 40, toolCalls[i].ID, toolCalls[j].CreatedAt, 40, toolCalls[j].ID)
	})
	for _, toolCall := range toolCalls {
		if toolCall.ToolName == interactionRequestUserTool {
			continue
		}
		call := toolCall
		primitives = append(primitives, TimelinePrimitive{
			ID:        "tool:" + call.ID,
			SessionID: sessionID,
			RunID:     run.ID,
			Kind:      TimelineKindToolGroup,
			CreatedAt: FirstNonEmpty(call.CreatedAt, call.UpdatedAt, run.UpdatedAt, run.CreatedAt),
			Order:     40,
			ToolCall:  &call,
		})
	}
	inputRequests := normalizeInputRequests(run.InputRequests)
	if run.InputRequest != nil {
		inputRequests = AppendInputRequestIfMissing(inputRequests, *run.InputRequest)
	}
	for requestIndex := range inputRequests {
		if strings.TrimSpace(inputRequests[requestIndex].RunID) != strings.TrimSpace(run.ID) {
			continue
		}
		item := *NormalizeInputRequest(&inputRequests[requestIndex])
		primitives = append(primitives, TimelinePrimitive{
			ID: "input:" + item.ID, SessionID: sessionID, RunID: item.RunID,
			Kind: TimelineKindInputRequest, CreatedAt: FirstNonEmpty(item.CreatedAt, item.UpdatedAt, run.UpdatedAt, run.CreatedAt),
			UpdatedAt: item.UpdatedAt, Order: 50, InputRequest: &item,
		})
	}
	approvals = append([]Approval(nil), approvals...)
	sort.SliceStable(approvals, func(i, j int) bool {
		return CompareTimelineKeys(approvals[i].CreatedAt, 50, approvals[i].ID, approvals[j].CreatedAt, 50, approvals[j].ID)
	})
	for _, approval := range approvals {
		item := approval
		primitives = append(primitives, TimelinePrimitive{
			ID:        "Approval:" + item.ID,
			SessionID: sessionID,
			RunID:     run.ID,
			Kind:      TimelineKindApprovalGroup,
			CreatedAt: FirstNonEmpty(item.CreatedAt, item.UpdatedAt, run.UpdatedAt, run.CreatedAt),
			Order:     50,
			Approval:  &item,
		})
	}
	return primitives
}

func GroupTimelinePrimitives(primitives []TimelinePrimitive) []TimelineEntry {
	if len(primitives) == 0 {
		return []TimelineEntry{}
	}
	sort.SliceStable(primitives, func(i, j int) bool {
		return CompareTimelineKeys(primitives[i].CreatedAt, primitives[i].Order, primitives[i].ID, primitives[j].CreatedAt, primitives[j].Order, primitives[j].ID)
	})

	result := make([]TimelineEntry, 0, len(primitives))
	for _, primitive := range primitives {
		switch {
		case primitive.ToolCall != nil:
			if len(result) > 0 && result[len(result)-1].Kind == TimelineKindToolGroup && result[len(result)-1].RunID == primitive.RunID {
				result[len(result)-1].ToolCalls = append(result[len(result)-1].ToolCalls, *primitive.ToolCall)
				continue
			}
			result = append(result, TimelineEntry{
				ID:        primitive.ID,
				SessionID: primitive.SessionID,
				RunID:     primitive.RunID,
				Kind:      TimelineKindToolGroup,
				CreatedAt: primitive.CreatedAt,
				Status:    TimelineStatusFinal,
				ToolCalls: []ToolCall{*primitive.ToolCall},
			})
		case primitive.Approval != nil:
			if len(result) > 0 && result[len(result)-1].Kind == TimelineKindApprovalGroup && result[len(result)-1].RunID == primitive.RunID {
				result[len(result)-1].Approvals = append(result[len(result)-1].Approvals, *primitive.Approval)
				continue
			}
			result = append(result, TimelineEntry{
				ID:        primitive.ID,
				SessionID: primitive.SessionID,
				RunID:     primitive.RunID,
				Kind:      TimelineKindApprovalGroup,
				CreatedAt: primitive.CreatedAt,
				Status:    TimelineStatusFinal,
				Approvals: []Approval{*primitive.Approval},
			})
		case primitive.InputRequest != nil:
			result = append(result, TimelineEntry{
				ID: primitive.ID, SessionID: primitive.SessionID, RunID: primitive.RunID,
				Kind: TimelineKindInputRequest, CreatedAt: primitive.CreatedAt, UpdatedAt: primitive.UpdatedAt,
				Status: TimelineStatusFinal, InputRequest: NormalizeInputRequest(primitive.InputRequest),
			})
		default:
			if strings.TrimSpace(primitive.Text) == "" {
				continue
			}
			result = append(result, TimelineEntry{
				ID:            primitive.ID,
				SessionID:     primitive.SessionID,
				RunID:         primitive.RunID,
				Kind:          primitive.Kind,
				CreatedAt:     primitive.CreatedAt,
				UpdatedAt:     primitive.UpdatedAt,
				Status:        DefaultString(strings.TrimSpace(primitive.Status), TimelineStatusFinal),
				Text:          strings.TrimSpace(primitive.Text),
				OriginalText:  strings.TrimSpace(primitive.OriginalText),
				ProcessedText: strings.TrimSpace(primitive.ProcessedText),
			})
		}
	}
	for index := range result {
		result[index].Sequence = index + 1
	}
	return result
}

func RunTextAnchor(run Run, preferredTime string) string {
	inputTime := ""
	if len(run.InputRequests) > 0 {
		inputTime = FirstNonEmpty(run.InputRequests[0].CreatedAt, run.InputRequests[0].UpdatedAt)
	} else if run.InputRequest != nil {
		inputTime = FirstNonEmpty(run.InputRequest.CreatedAt, run.InputRequest.UpdatedAt)
	}
	candidates := []string{FirstRunToolTime(run), FirstRunApprovalTime(run), inputTime, preferredTime, run.UpdatedAt, run.CreatedAt}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return NowString()
}

func FirstRunToolTime(run Run) string {
	earliest := ""
	for _, call := range run.ToolCalls {
		candidate := FirstNonEmpty(call.CreatedAt, call.UpdatedAt)
		if candidate == "" {
			continue
		}
		if earliest == "" || CompareTimelineKeys(candidate, 0, "", earliest, 0, "") {
			earliest = candidate
		}
	}
	return earliest
}

func FirstRunApprovalTime(run Run) string {
	earliest := ""
	for _, approval := range PendingApprovalsOnly(run.PendingApprovals) {
		candidate := FirstNonEmpty(approval.CreatedAt, approval.UpdatedAt)
		if candidate == "" {
			continue
		}
		if earliest == "" || CompareTimelineKeys(candidate, 0, "", earliest, 0, "") {
			earliest = candidate
		}
	}
	return earliest
}

func StripTimelinePrefix(value string, prefix string) string {
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

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func CompareTimelineKeys(leftAt string, leftOrder int, leftID string, rightAt string, rightOrder int, rightID string) bool {
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

// PendingApprovalsOnly returns the pending approvals from a run projection,
// deduplicating approvals that share an id, confirmation call id, or function
// call id.
func PendingApprovalsOnly(approvals []Approval) []Approval {
	if len(approvals) == 0 {
		return []Approval{}
	}
	filtered := make([]Approval, 0, len(approvals))
	seen := map[string]struct{}{}
	for _, approval := range approvals {
		if isPendingApprovalStatus(approval.Status) {
			if key := pendingApprovalKey(approval); key != "" {
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
			}
			filtered = append(filtered, approval)
		}
	}
	return filtered
}

func pendingApprovalKey(approval Approval) string {
	if id := strings.TrimSpace(approval.ID); id != "" {
		return "id:" + id
	}
	if id := strings.TrimSpace(approval.ConfirmationCallID); id != "" {
		return "confirmation:" + id
	}
	if id := strings.TrimSpace(approval.FunctionCallID); id != "" {
		return "function:" + id
	}
	return ""
}

func isPendingApprovalStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), ApprovalStatusPending)
}

func normalizeInputRequests(requests []InputRequest) []InputRequest {
	if len(requests) == 0 {
		return []InputRequest{}
	}
	result := make([]InputRequest, 0, len(requests))
	for index := range requests {
		result = append(result, *NormalizeInputRequest(&requests[index]))
	}
	return result
}
