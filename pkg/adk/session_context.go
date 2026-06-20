package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	adksession "google.golang.org/adk/session"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

const (
	ContextStatusUnknown   = "unknown"
	ContextStatusHealthy   = "healthy"
	ContextStatusWarning   = "warning"
	ContextStatusNearLimit = "near_limit"
	ContextStatusCritical  = "critical"

	contextWarnThreshold       = 0.70
	contextAutoCompactThresh   = 0.85
	contextAggressiveThreshold = 0.93

	adkSessionHandoffSummaryKey   = "jftrade:handoff_summary"
	adkSessionHandoffUpdatedAtKey = "jftrade:handoff_updated_at"
	adkSessionHandoffCountKey     = "jftrade:handoff_segment_count"

	adkSessionAppendMaxAttempts = 5
)

type SessionCompactRequest struct {
	Mode    string
	Trigger string
	Reason  string
}

type SessionContextManager struct {
	store       *Store
	rawService  adksession.Service
	openai      openAIClient
	tools       *ToolRegistry
	appendLocks *adkSessionAppendLockMap
}

type adkSessionAppendLockMap struct {
	mu    sync.Mutex
	locks map[string]*adkSessionAppendLock
}

type adkSessionAppendLock struct {
	mu   sync.Mutex
	refs int
}

type projectionState struct {
	snapshot         SessionContextSnapshot
	compactionCutoff int
}

type contextProjectionBucket int

const (
	contextProjectionBucketOtherVisible contextProjectionBucket = iota
	contextProjectionBucketRecentUser
	contextProjectionBucketProtectedTail
)

type projectedVisibleEvent struct {
	raw          *adksession.Event
	effective    *adksession.Event
	bucket       contextProjectionBucket
	trimmedCount int
}

type projectedVisibleSession struct {
	events                   []*adksession.Event
	rawBreakdown             SessionContextBreakdown
	effectiveBreakdown       SessionContextBreakdown
	trimmedToolResponseCount int
}

func ensureSessionContextRevision(state SessionContextState, sessionID string) SessionContextState {
	state.SessionID = strings.TrimSpace(defaultString(state.SessionID, sessionID))
	if strings.TrimSpace(state.ContextRevisionID) == "" {
		state.ContextRevisionID = newContextRevisionID()
	}
	if strings.TrimSpace(state.ContextRevisionCreatedAt) == "" {
		state.ContextRevisionCreatedAt = defaultString(state.CreatedAt, nowString())
	}
	return state
}

func newContextRevisionID() string {
	return "ctxrev-" + uuid.NewString()
}

func NewSessionContextManager(store *Store, rawService adksession.Service, openai openAIClient, tools *ToolRegistry) *SessionContextManager {
	if store == nil || rawService == nil {
		return nil
	}
	return &SessionContextManager{store: store, rawService: rawService, openai: openai, tools: tools, appendLocks: newADKSessionAppendLockMap()}
}

func (m *SessionContextManager) WrapService(service adksession.Service) adksession.Service {
	if m == nil || service == nil {
		return service
	}
	return &compactingSessionService{base: service, manager: m}
}

func (m *SessionContextManager) Snapshot(ctx context.Context, session Session, agent Agent) (SessionContextSnapshot, error) {
	return m.snapshotWithPending(ctx, session, agent, "")
}

func (m *SessionContextManager) ProjectedSnapshot(ctx context.Context, session Session, agent Agent, pendingUserText string) (SessionContextSnapshot, error) {
	return m.snapshotWithPending(ctx, session, agent, pendingUserText)
}

func (m *SessionContextManager) snapshotWithPending(ctx context.Context, session Session, agent Agent, pendingUserText string) (SessionContextSnapshot, error) {
	raw, err := m.rawSession(ctx, session.AgentID, session.ID)
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	state, ok, err := m.store.SessionContext(ctx, session.ID)
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	if !ok {
		state = SessionContextState{SessionID: session.ID}
	}
	state = ensureSessionContextRevision(state, session.ID)
	segments, err := m.store.HandoffSegmentsForRevision(ctx, session.ID, state.ContextRevisionID, true)
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	projection := m.projectSnapshot(raw.Session, state, agent, segments, pendingUserText)
	state = mergeStateMetrics(state, projection.snapshot)
	state.SessionID = session.ID
	state.RecentUserWindow = normalizeRecentUserWindow(agent.RecentUserWindow)
	if _, err := m.store.SaveSessionContext(ctx, state); err != nil {
		return SessionContextSnapshot{}, err
	}
	jftradeErr2 := m.syncHandoffStateForSession(ctx, session)
	jftradeLogError(jftradeErr2)
	return projection.snapshot, nil
}

func (m *SessionContextManager) Compact(ctx context.Context, session Session, agent Agent, request SessionCompactRequest) (SessionContextSnapshot, error) {
	if m == nil {
		return SessionContextSnapshot{}, fmt.Errorf("session context manager is unavailable")
	}
	raw, err := m.rawSession(ctx, session.AgentID, session.ID)
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	state, ok, err := m.store.SessionContext(ctx, session.ID)
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	if !ok {
		state = SessionContextState{SessionID: session.ID}
	}
	state = ensureSessionContextRevision(state, session.ID)
	segments, err := m.store.HandoffSegmentsForRevision(ctx, session.ID, state.ContextRevisionID, true)
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	projection := m.projectSnapshot(raw.Session, state, agent, segments, "")
	events := eventSlice(raw.Session.Events())
	cutoff := projection.compactionCutoff
	if cutoff > len(events) {
		cutoff = len(events)
	}
	mode := normalizeCompactMode(request.Mode)
	activeEnd := maxActiveSegmentEnd(segments)
	degraded := state.DegradedSummary
	previousRevisionID := state.ContextRevisionID
	nextRevisionID := newContextRevisionID()
	nextRevisionCreatedAt := nowString()
	wroteRevision := false
	if mode == "aggressive" {
		if cutoff > 0 || len(segments) > 0 {
			deterministic := buildHandoffSummary(segments, events[:cutoff], mode)
			merged, nextDegraded := m.mergeSummary(ctx, agent, deterministic, joinSegmentSummaries(segments), mode)
			next := HandoffSegment{
				SessionID:         session.ID,
				ContextRevisionID: nextRevisionID,
				Sequence:          nextHandoffSequence(segments),
				StartEventIndex:   0,
				EndEventIndex:     cutoff,
				Summary:           merged,
				Mode:              "aggressive",
				Reason:            strings.TrimSpace(request.Reason),
				EstimatedTokens:   estimateTextTokens(merged),
				Active:            true,
			}
			if _, err := m.store.ReplaceActiveHandoffSegments(ctx, session.ID, next, segments); err != nil {
				return SessionContextSnapshot{}, err
			}
			degraded = nextDegraded
			wroteRevision = true
		}
	} else if cutoff > activeEnd {
		deterministic := buildHandoffSummary(nil, events[activeEnd:cutoff], mode)
		merged, nextDegraded := m.mergeSummary(ctx, agent, deterministic, joinSegmentSummaries(segments), mode)
		next := HandoffSegment{
			SessionID:         session.ID,
			ContextRevisionID: nextRevisionID,
			Sequence:          nextHandoffSequence(segments),
			StartEventIndex:   0,
			EndEventIndex:     cutoff,
			Summary:           merged,
			Mode:              "manual",
			Reason:            strings.TrimSpace(request.Reason),
			EstimatedTokens:   estimateTextTokens(merged),
			Active:            true,
		}
		if _, err := m.store.SaveHandoffSegment(ctx, next); err != nil {
			return SessionContextSnapshot{}, err
		}
		degraded = nextDegraded
		wroteRevision = true
	}

	now := nowString()
	state.SessionID = session.ID
	if wroteRevision {
		state.PreviousContextRevisionID = previousRevisionID
		state.ContextRevisionID = nextRevisionID
		state.ContextRevisionCreatedAt = nextRevisionCreatedAt
	}
	state.RecentUserWindow = normalizeRecentUserWindow(agent.RecentUserWindow)
	state.LastCompactedAt = now
	state.LastCompactionMode = compactionModeLabel(request)
	state.LastCompactionReason = strings.TrimSpace(request.Reason)
	state.AutoCompacted = request.Trigger == "auto"
	state.DegradedSummary = degraded
	state.UpdatedAt = now
	if _, err := m.store.SaveSessionContext(ctx, state); err != nil {
		return SessionContextSnapshot{}, err
	}
	jftradeErr1 := m.syncHandoffStateForSession(ctx, session)
	jftradeLogError(jftradeErr1)
	return m.snapshotWithPending(ctx, session, agent, "")
}

func (m *SessionContextManager) ShouldAutoCompact(snapshot SessionContextSnapshot) (string, bool) {
	if snapshot.ContextWindowTokens <= 0 {
		return "", false
	}
	projectedRatio := float64(snapshot.ProjectedNextTurnTokens) / float64(snapshot.ContextWindowTokens)
	switch {
	case projectedRatio >= contextAggressiveThreshold:
		return "aggressive", true
	case projectedRatio >= contextAutoCompactThresh:
		return "normal", true
	default:
		return "", false
	}
}

func (m *SessionContextManager) AutoCompactForModelContext(ctx context.Context, session Session, agent Agent, pendingUserText string) (SessionContextSnapshot, bool, error) {
	if m == nil || strings.TrimSpace(session.ID) == "" {
		return SessionContextSnapshot{}, false, nil
	}
	snapshot, err := m.ProjectedSnapshot(ctx, session, agent, pendingUserText)
	if err != nil {
		return SessionContextSnapshot{}, false, err
	}
	mode, shouldCompact := m.ShouldAutoCompact(snapshot)
	if !shouldCompact {
		return snapshot, false, nil
	}
	canAdvance, err := m.canAdvanceAutoCompaction(ctx, session, agent)
	if err != nil {
		return snapshot, false, err
	}
	if !canAdvance {
		return snapshot, false, nil
	}
	reason := "context usage exceeded automatic compaction threshold before model call"
	if mode == "aggressive" {
		reason = "context usage exceeded aggressive failsafe threshold before model call"
	}
	compacted, err := m.Compact(ctx, session, agent, SessionCompactRequest{
		Mode:    mode,
		Trigger: "auto",
		Reason:  reason,
	})
	if err != nil {
		return snapshot, false, err
	}
	return compacted, true, nil
}

func (m *SessionContextManager) canAdvanceAutoCompaction(ctx context.Context, session Session, agent Agent) (bool, error) {
	raw, err := m.rawSession(ctx, session.AgentID, session.ID)
	if err != nil {
		return false, err
	}
	state, ok, err := m.store.SessionContext(ctx, session.ID)
	if err != nil {
		return false, err
	}
	if !ok {
		state = SessionContextState{SessionID: session.ID}
	}
	state = ensureSessionContextRevision(state, session.ID)
	segments, err := m.store.HandoffSegmentsForRevision(ctx, session.ID, state.ContextRevisionID, true)
	if err != nil {
		return false, err
	}
	events := eventSlice(raw.Session.Events())
	cutoff := m.projectSnapshot(raw.Session, state, agent, segments, "").compactionCutoff
	if cutoff > len(events) {
		cutoff = len(events)
	}
	activeEnd := maxActiveSegmentEnd(segments)
	if activeEnd > len(events) {
		activeEnd = len(events)
	}
	return cutoff > activeEnd, nil
}

func (m *SessionContextManager) InstructionSuffix(ctx context.Context, sessionID string) (string, error) {
	if m == nil || m.store == nil {
		return "", nil
	}
	state, ok, err := m.store.SessionContext(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return "", err
	}
	if !ok {
		state = SessionContextState{SessionID: strings.TrimSpace(sessionID)}
	}
	state = ensureSessionContextRevision(state, sessionID)
	segments, err := m.store.HandoffSegmentsForRevision(ctx, strings.TrimSpace(sessionID), state.ContextRevisionID, true)
	if err != nil {
		return "", err
	}
	if len(segments) == 0 {
		return "", nil
	}
	text := joinSegmentSummaries(segments)
	if text == "" {
		return "", nil
	}
	jftradeErr3 := m.syncHandoffStateForSessionID(ctx, sessionID, segments)
	jftradeLogError(jftradeErr3)
	return "Session handoff summaries:\n" + text, nil
}

func (m *SessionContextManager) syncHandoffStateForSession(ctx context.Context, session Session) error {
	if m == nil || m.store == nil {
		return nil
	}
	state, ok, err := m.store.SessionContext(ctx, session.ID)
	if err != nil {
		return err
	}
	if !ok {
		state = SessionContextState{SessionID: session.ID}
	}
	state = ensureSessionContextRevision(state, session.ID)
	segments, err := m.store.HandoffSegmentsForRevision(ctx, session.ID, state.ContextRevisionID, true)
	if err != nil {
		return err
	}
	return m.syncHandoffState(ctx, session, segments)
}

func (m *SessionContextManager) syncHandoffStateForSessionID(ctx context.Context, sessionID string, segments []HandoffSegment) error {
	if m == nil || m.store == nil {
		return nil
	}
	session, ok, err := m.store.Session(ctx, strings.TrimSpace(sessionID))
	if err != nil || !ok {
		return err
	}
	return m.syncHandoffState(ctx, session, segments)
}

func (m *SessionContextManager) syncHandoffState(ctx context.Context, session Session, segments []HandoffSegment) error {
	if m == nil || m.rawService == nil || strings.TrimSpace(session.ID) == "" {
		return nil
	}
	raw, err := m.rawSession(ctx, session.AgentID, session.ID)
	if err != nil {
		return err
	}
	if raw == nil || raw.Session == nil || raw.Session.State() == nil {
		return nil
	}
	state := raw.Session.State()
	summary := joinSegmentSummaries(segments)
	if stateTextValue(state, adkSessionHandoffSummaryKey) == summary &&
		stateTextValue(state, adkSessionHandoffCountKey) == fmt.Sprint(len(segments)) {
		return nil
	}
	event := adksession.NewEvent("jftrade-handoff-state")
	event.Author = "jftrade"
	event.Actions.SkipSummarization = true
	event.Actions.StateDelta = map[string]any{
		adkSessionHandoffSummaryKey:   summary,
		adkSessionHandoffUpdatedAtKey: nowString(),
		adkSessionHandoffCountKey:     len(segments),
	}
	return appendADKEventWithStaleRetry(ctx, m.appendLocks, m.rawService, raw.Session, event)
}

func stateTextValue(state adksession.State, key string) string {
	if state == nil {
		return ""
	}
	value, err := state.Get(key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func (m *SessionContextManager) HasActiveRun(ctx context.Context, sessionID string) (bool, error) {
	runs, err := m.store.ListRuns(ctx)
	if err != nil {
		return false, err
	}
	for _, run := range runs {
		if run.SessionID != sessionID {
			continue
		}
		if run.Status == RunStatusRunning || run.Status == RunStatusPending {
			return true, nil
		}
	}
	return false, nil
}

func (m *SessionContextManager) rawSession(ctx context.Context, agentID string, sessionID string) (*adksession.GetResponse, error) {
	response, err := m.rawService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agentID),
		UserID:    googleADKUserID,
		SessionID: sessionID,
	})
	if err == nil {
		return response, nil
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "not found") || strings.Contains(lower, "record not found") {
		return &adksession.GetResponse{
			Session: &emptySession{
				id:      sessionID,
				appName: googleADKAppName(agentID),
				userID:  googleADKUserID,
				state:   &emptyState{values: map[string]any{}},
				events:  &wrappedEvents{},
			},
		}, nil
	}
	return nil, err
}

func (m *SessionContextManager) projectSnapshot(raw adksession.Session, state SessionContextState, agent Agent, segments []HandoffSegment, pendingUserText string) projectionState {
	events := eventSlice(raw.Events())
	recentWindow := normalizeRecentUserWindow(agent.RecentUserWindow)
	potentialCutoff := compactionCutoff(events, recentWindow)
	currentCutoff := maxActiveSegmentEnd(segments)
	if currentCutoff > len(events) {
		currentCutoff = len(events)
	}
	protectedStart := protectedTailStart(events)
	if protectedStart < currentCutoff {
		protectedStart = currentCutoff
	}
	recentStart := recentUserEventStart(events, recentWindow)
	if recentStart < currentCutoff {
		recentStart = currentCutoff
	}

	rawBreakdown := SessionContextBreakdown{
		InstructionTokens:     estimateInstructionTokens(agent),
		HandoffTokens:         estimateHandoffTokens(segments),
		PendingUserTokens:     estimatePendingUserTokens(pendingUserText),
		ToolDeclarationTokens: estimateToolDeclarationTokens(agent, m.tools),
	}
	effectiveBreakdown := rawBreakdown
	projectedEvents := projectVisibleSessionEvents(events, len(segments) > 0, currentCutoff, recentStart, protectedStart)
	rawBreakdown.RecentUserTokens += projectedEvents.rawBreakdown.RecentUserTokens
	rawBreakdown.ProtectedTailTokens += projectedEvents.rawBreakdown.ProtectedTailTokens
	rawBreakdown.OtherVisibleTokens += projectedEvents.rawBreakdown.OtherVisibleTokens
	effectiveBreakdown.RecentUserTokens += projectedEvents.effectiveBreakdown.RecentUserTokens
	effectiveBreakdown.ProtectedTailTokens += projectedEvents.effectiveBreakdown.ProtectedTailTokens
	effectiveBreakdown.OtherVisibleTokens += projectedEvents.effectiveBreakdown.OtherVisibleTokens

	rawCurrentInputTokens := rawBreakdown.InstructionTokens + rawBreakdown.HandoffTokens + rawBreakdown.RecentUserTokens + rawBreakdown.ProtectedTailTokens + rawBreakdown.OtherVisibleTokens + rawBreakdown.ToolDeclarationTokens
	rawProjectedNextTurnTokens := rawCurrentInputTokens + rawBreakdown.PendingUserTokens
	currentInputTokens := effectiveBreakdown.InstructionTokens + effectiveBreakdown.HandoffTokens + effectiveBreakdown.RecentUserTokens + effectiveBreakdown.ProtectedTailTokens + effectiveBreakdown.OtherVisibleTokens + effectiveBreakdown.ToolDeclarationTokens
	projectedNextTurnTokens := currentInputTokens + effectiveBreakdown.PendingUserTokens
	windowTokens := m.contextWindowTokens(agent)
	ratio := 0.0
	if windowTokens > 0 {
		ratio = float64(currentInputTokens) / float64(windowTokens)
	}

	return projectionState{
		compactionCutoff: potentialCutoff,
		snapshot: SessionContextSnapshot{
			SessionID:                  raw.ID(),
			ContextRevisionID:          state.ContextRevisionID,
			PreviousContextRevisionID:  state.PreviousContextRevisionID,
			ContextRevisionCreatedAt:   state.ContextRevisionCreatedAt,
			CurrentInputTokens:         currentInputTokens,
			ProjectedNextTurnTokens:    projectedNextTurnTokens,
			EstimatedInputTokens:       currentInputTokens,
			RawCurrentInputTokens:      rawCurrentInputTokens,
			RawProjectedNextTurnTokens: rawProjectedNextTurnTokens,
			ContextWindowTokens:        windowTokens,
			UsageRatio:                 ratio,
			Status:                     contextStatus(projectedNextTurnTokens, windowTokens),
			RecentUserWindow:           recentWindow,
			RetainedRecentUserCount:    retainedRecentUserCount(events, recentStart, protectedStart),
			ProtectedRecentCount:       retainedRecentUserCount(events, recentStart, protectedStart),
			ActiveHandoffCount:         len(segments),
			LatestHandoffPreview:       latestSegmentPreview(segments),
			SummaryPreview:             latestSegmentPreview(segments),
			RawEventCount:              len(events),
			CompactedEventCount:        currentCutoff,
			SummaryBoundaryEventIndex:  currentCutoff,
			Breakdown:                  effectiveBreakdown,
			RawBreakdown:               rawBreakdown,
			TrimmedToolResponseCount:   projectedEvents.trimmedToolResponseCount,
			LastCompactedAt:            state.LastCompactedAt,
			LastCompactionMode:         state.LastCompactionMode,
			LastCompactionReason:       state.LastCompactionReason,
			AutoCompacted:              state.AutoCompacted,
			DegradedSummary:            state.DegradedSummary,
		},
	}
}

func (m *SessionContextManager) contextWindowTokens(agent Agent) int {
	if m == nil || m.store == nil || strings.TrimSpace(agent.ProviderID) == "" {
		return 0
	}
	provider, ok, err := m.store.Provider(context.Background(), agent.ProviderID)
	if err != nil || !ok {
		return 0
	}
	return provider.ContextWindowTokens
}

func (m *SessionContextManager) mergeSummary(ctx context.Context, agent Agent, deterministic string, existing string, mode string) (string, bool) {
	deterministic = strings.TrimSpace(deterministic)
	if deterministic == "" {
		return strings.TrimSpace(existing), false
	}
	if strings.TrimSpace(agent.ProviderID) == "" || m.store == nil {
		return deterministic, true
	}
	provider, ok, err := m.store.Provider(ctx, agent.ProviderID)
	if err != nil || !ok || !provider.Enabled {
		return deterministic, true
	}
	apiKey, hasKey, err := m.store.ProviderAPIKey(provider.ID)
	if err != nil || !hasKey {
		return deterministic, true
	}
	targetStyle := "Produce a compact handoff summary that preserves durable facts, user goals, unfinished work, approvals, tool outcomes, and constraints."
	if mode == "aggressive" {
		targetStyle = "Compress aggressively. Keep only durable facts, user goals, approvals, critical tool outcomes, and unresolved work."
	}
	reply, err := m.openai.chat(ctx, provider, apiKey, defaultString(agent.Model, provider.Model), []openAIChatMessage{
		{Role: "system", Content: "You compress chat context for future model turns. Output plain text only. " + targetStyle},
		{Role: "user", Content: "Existing handoff:\n" + strings.TrimSpace(existing) + "\n\nCandidate handoff:\n" + deterministic},
	})
	if err != nil {
		return deterministic, true
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return deterministic, true
	}
	return reply, false
}

func mergeStateMetrics(state SessionContextState, snapshot SessionContextSnapshot) SessionContextState {
	state.SessionID = snapshot.SessionID
	state.ContextRevisionID = snapshot.ContextRevisionID
	state.PreviousContextRevisionID = snapshot.PreviousContextRevisionID
	state.ContextRevisionCreatedAt = snapshot.ContextRevisionCreatedAt
	state.RecentUserWindow = snapshot.RecentUserWindow
	state.RetainedRecentUserCount = snapshot.RetainedRecentUserCount
	state.ActiveHandoffCount = snapshot.ActiveHandoffCount
	state.CurrentInputTokens = snapshot.CurrentInputTokens
	state.ProjectedNextTurnTokens = snapshot.ProjectedNextTurnTokens
	state.ContextWindowTokens = snapshot.ContextWindowTokens
	state.UsageRatio = snapshot.UsageRatio
	state.LatestHandoffPreview = snapshot.LatestHandoffPreview
	state.Breakdown = snapshot.Breakdown
	state.DegradedSummary = snapshot.DegradedSummary
	return state
}

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
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if eventContainsUnresolvedApproval(event, resolvedApprovalCallIDs) {
			return index
		}
	}
	return len(events)
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

func eventContainsUnresolvedApproval(event *adksession.Event, resolvedApprovalCallIDs map[string]struct{}) bool {
	if event == nil || event.Content == nil {
		return false
	}
	for _, part := range event.Content.Parts {
		if part == nil || part.FunctionCall == nil {
			continue
		}
		call := part.FunctionCall
		if call.Name != toolconfirmation.FunctionCallName {
			continue
		}
		if _, ok := resolvedApprovalCallIDs[strings.TrimSpace(call.ID)]; !ok {
			return true
		}
	}
	return false
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

type compactingSessionService struct {
	base    adksession.Service
	manager *SessionContextManager
}

func (s *compactingSessionService) Create(ctx context.Context, req *adksession.CreateRequest) (*adksession.CreateResponse, error) {
	return s.base.Create(ctx, req)
}

func (s *compactingSessionService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	response, err := s.base.Get(ctx, req)
	if err != nil || s.manager == nil || response == nil {
		return response, err
	}
	session, ok, storeErr := s.manager.store.Session(ctx, req.SessionID)
	if storeErr != nil || !ok {
		return response, storeErr
	}
	agent, ok, storeErr := s.manager.store.Agent(ctx, session.AgentID)
	if storeErr != nil || !ok {
		return response, storeErr
	}
	if _, compacted, compactErr := s.manager.AutoCompactForModelContext(ctx, session, agent, ""); compactErr != nil {
		log.Printf("[adk] auto context compaction before model session read failed for session %s: %v", req.SessionID, compactErr)
	} else if compacted {
		response, err = s.base.Get(ctx, req)
		if err != nil || response == nil {
			return response, err
		}
	}
	state, stateOK, stateErr := s.manager.store.SessionContext(ctx, req.SessionID)
	if stateErr != nil {
		return response, stateErr
	}
	if !stateOK {
		state = SessionContextState{SessionID: req.SessionID}
	}
	state = ensureSessionContextRevision(state, req.SessionID)
	segments, stateErr := s.manager.store.HandoffSegmentsForRevision(ctx, req.SessionID, state.ContextRevisionID, true)
	if stateErr != nil {
		return response, stateErr
	}
	events := eventSlice(response.Session.Events())
	cutoff := minInt(maxActiveSegmentEnd(segments), len(events))
	recentStart := recentUserEventStart(events, normalizeRecentUserWindow(agent.RecentUserWindow))
	if recentStart < cutoff {
		recentStart = cutoff
	}
	protectedStart := protectedTailStart(events)
	if protectedStart < cutoff {
		protectedStart = cutoff
	}
	projected := projectVisibleSessionEvents(events, len(segments) > 0, cutoff, recentStart, protectedStart)
	if len(segments) == 0 && projected.trimmedToolResponseCount == 0 {
		return response, nil
	}
	filtered := filterEvents(projected.events, req.After, req.NumRecentEvents)
	response.Session = &wrappedSession{
		base:   response.Session,
		events: &wrappedEvents{items: filtered},
	}
	return response, nil
}

func (s *compactingSessionService) List(ctx context.Context, req *adksession.ListRequest) (*adksession.ListResponse, error) {
	return s.base.List(ctx, req)
}

func (s *compactingSessionService) Delete(ctx context.Context, req *adksession.DeleteRequest) error {
	return s.base.Delete(ctx, req)
}

func (s *compactingSessionService) AppendEvent(ctx context.Context, session adksession.Session, event *adksession.Event) error {
	if wrapped, ok := session.(*wrappedSession); ok && wrapped != nil {
		session = wrapped.base
	}
	return appendADKEventWithStaleRetry(ctx, serviceAppendLocks(s), s.base, session, event)
}

func appendADKEventWithStaleRetry(ctx context.Context, locks *adkSessionAppendLockMap, service adksession.Service, session adksession.Session, event *adksession.Event) error {
	if service == nil {
		return fmt.Errorf("adk session service is unavailable")
	}
	if session == nil {
		return fmt.Errorf("adk session is unavailable")
	}

	lock, release := locks.acquire(session)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	current := session
	var lastErr error
	for attempt := 0; attempt < adkSessionAppendMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := service.AppendEvent(ctx, current, event)
		if err == nil {
			return nil
		}
		if !isStaleADKSessionError(err) {
			return err
		}
		lastErr = err
		latest, getErr := service.Get(ctx, &adksession.GetRequest{
			AppName:   current.AppName(),
			UserID:    current.UserID(),
			SessionID: current.ID(),
		})
		if getErr != nil {
			return err
		}
		if latest == nil || latest.Session == nil {
			return err
		}
		current = latest.Session
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("failed to append ADK event after %d attempts", adkSessionAppendMaxAttempts)
}

func isStaleADKSessionError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "stale session error")
}

func serviceAppendLocks(service *compactingSessionService) *adkSessionAppendLockMap {
	if service != nil && service.manager != nil && service.manager.appendLocks != nil {
		return service.manager.appendLocks
	}
	return newADKSessionAppendLockMap()
}

func runtimeAppendLocks(runtime *Runtime) *adkSessionAppendLockMap {
	if runtime != nil && runtime.contextManager != nil && runtime.contextManager.appendLocks != nil {
		return runtime.contextManager.appendLocks
	}
	return newADKSessionAppendLockMap()
}

func newADKSessionAppendLockMap() *adkSessionAppendLockMap {
	return &adkSessionAppendLockMap{locks: map[string]*adkSessionAppendLock{}}
}

func (m *adkSessionAppendLockMap) acquire(session adksession.Session) (*sync.Mutex, func()) {
	if m == nil {
		m = newADKSessionAppendLockMap()
	}
	key := strings.Join([]string{session.AppName(), session.UserID(), session.ID()}, "\x00")
	m.mu.Lock()
	lock := m.locks[key]
	if lock == nil {
		lock = &adkSessionAppendLock{}
		m.locks[key] = lock
	}
	lock.refs++
	m.mu.Unlock()
	return &lock.mu, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		lock.refs--
		if lock.refs <= 0 && m.locks[key] == lock {
			delete(m.locks, key)
		}
	}
}

func (m *adkSessionAppendLockMap) len() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.locks)
}

func filterEvents(events []*adksession.Event, after time.Time, numRecent int) []*adksession.Event {
	filtered := events[:0]
	for _, event := range events {
		if event == nil {
			continue
		}
		if !after.IsZero() && event.Timestamp.Before(after) {
			continue
		}
		filtered = append(filtered, event)
	}
	if numRecent > 0 && len(filtered) > numRecent {
		filtered = filtered[len(filtered)-numRecent:]
	}
	return filtered
}

type wrappedSession struct {
	base   adksession.Session
	events adksession.Events
}

func (s *wrappedSession) ID() string                { return s.base.ID() }
func (s *wrappedSession) AppName() string           { return s.base.AppName() }
func (s *wrappedSession) UserID() string            { return s.base.UserID() }
func (s *wrappedSession) State() adksession.State   { return s.base.State() }
func (s *wrappedSession) Events() adksession.Events { return s.events }
func (s *wrappedSession) LastUpdateTime() time.Time { return s.base.LastUpdateTime() }

type wrappedEvents struct {
	items []*adksession.Event
}

func (e *wrappedEvents) All() iter.Seq[*adksession.Event] {
	return func(yield func(*adksession.Event) bool) {
		for _, item := range e.items {
			if !yield(item) {
				return
			}
		}
	}
}

func (e *wrappedEvents) Len() int { return len(e.items) }

func (e *wrappedEvents) At(i int) *adksession.Event {
	if i < 0 || i >= len(e.items) {
		return nil
	}
	return e.items[i]
}

type emptySession struct {
	id             string
	appName        string
	userID         string
	state          adksession.State
	events         adksession.Events
	lastUpdateTime time.Time
}

func (s *emptySession) ID() string                { return s.id }
func (s *emptySession) AppName() string           { return s.appName }
func (s *emptySession) UserID() string            { return s.userID }
func (s *emptySession) State() adksession.State   { return s.state }
func (s *emptySession) Events() adksession.Events { return s.events }
func (s *emptySession) LastUpdateTime() time.Time { return s.lastUpdateTime }

type emptyState struct {
	values map[string]any
}

func (s *emptyState) Get(key string) (any, error) {
	value, ok := s.values[key]
	if !ok {
		return nil, adksession.ErrStateKeyNotExist
	}
	return value, nil
}

func (s *emptyState) Set(key string, value any) error {
	s.values[key] = value
	return nil
}

func (s *emptyState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for key, value := range s.values {
			if !yield(key, value) {
				return
			}
		}
	}
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
