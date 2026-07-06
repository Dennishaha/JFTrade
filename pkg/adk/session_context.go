package adk

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	adksession "google.golang.org/adk/v2/session"
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

type sessionContextProjectionData struct {
	raw        *adksession.GetResponse
	state      SessionContextState
	segments   []HandoffSegment
	projection projectionState
}

type sessionCompactionResult struct {
	wroteRevision         bool
	degradedSummary       bool
	previousRevisionID    string
	nextRevisionID        string
	nextRevisionCreatedAt string
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

func (m *SessionContextManager) WrapService(service adksession.Service, gates ...func(string) (func(), bool)) adksession.Service {
	if m == nil || service == nil {
		return service
	}
	var beginCompaction func(string) (func(), bool)
	if len(gates) > 0 {
		beginCompaction = gates[0]
	}
	return &compactingSessionService{base: service, manager: m, beginCompaction: beginCompaction}
}

func (m *SessionContextManager) Snapshot(ctx context.Context, session Session, agent Agent) (SessionContextSnapshot, error) {
	return m.snapshotWithPending(ctx, session, agent, "")
}

func (m *SessionContextManager) ProjectedSnapshot(ctx context.Context, session Session, agent Agent, pendingUserText string) (SessionContextSnapshot, error) {
	return m.snapshotWithPending(ctx, session, agent, pendingUserText)
}

func (m *SessionContextManager) snapshotWithPending(ctx context.Context, session Session, agent Agent, pendingUserText string) (SessionContextSnapshot, error) {
	data, err := m.loadSessionContextProjection(ctx, session, agent, pendingUserText)
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	data.state = mergeStateMetrics(data.state, data.projection.snapshot)
	data.state.SessionID = session.ID
	data.state.RecentUserWindow = normalizeRecentUserWindow(agent.RecentUserWindow)
	if _, err := m.store.SaveSessionContext(ctx, data.state); err != nil {
		return SessionContextSnapshot{}, err
	}
	jftradeErr2 := m.syncHandoffStateForSession(ctx, session)
	jftradeLogError(jftradeErr2)
	return data.projection.snapshot, nil
}

func (m *SessionContextManager) Compact(ctx context.Context, session Session, agent Agent, request SessionCompactRequest) (SessionContextSnapshot, error) {
	if m == nil {
		return SessionContextSnapshot{}, fmt.Errorf("session context manager is unavailable")
	}
	data, err := m.loadSessionContextProjection(ctx, session, agent, "")
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	compaction, err := m.compactSessionProjection(ctx, session, agent, request, data)
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	state := applySessionCompactionState(data.state, session.ID, agent, request, compaction, nowString())
	if _, err := m.store.SaveSessionContext(ctx, state); err != nil {
		return SessionContextSnapshot{}, err
	}
	jftradeErr1 := m.syncHandoffStateForSession(ctx, session)
	jftradeLogError(jftradeErr1)
	return m.snapshotWithPending(ctx, session, agent, "")
}

func (m *SessionContextManager) loadSessionContextProjection(ctx context.Context, session Session, agent Agent, pendingUserText string) (sessionContextProjectionData, error) {
	raw, err := m.rawSession(ctx, session.AgentID, session.ID)
	if err != nil {
		return sessionContextProjectionData{}, err
	}
	state, ok, err := m.store.SessionContext(ctx, session.ID)
	if err != nil {
		return sessionContextProjectionData{}, err
	}
	if !ok {
		state = SessionContextState{SessionID: session.ID}
	}
	state = ensureSessionContextRevision(state, session.ID)
	segments, err := m.store.HandoffSegmentsForRevision(ctx, session.ID, state.ContextRevisionID, true)
	if err != nil {
		return sessionContextProjectionData{}, err
	}
	return sessionContextProjectionData{
		raw:        raw,
		state:      state,
		segments:   segments,
		projection: m.projectSnapshot(raw.Session, state, agent, segments, pendingUserText),
	}, nil
}

func (m *SessionContextManager) compactSessionProjection(ctx context.Context, session Session, agent Agent, request SessionCompactRequest, data sessionContextProjectionData) (sessionCompactionResult, error) {
	events := eventSlice(data.raw.Session.Events())
	result := sessionCompactionResult{
		degradedSummary:       data.state.DegradedSummary,
		previousRevisionID:    data.state.ContextRevisionID,
		nextRevisionID:        newContextRevisionID(),
		nextRevisionCreatedAt: nowString(),
	}
	cutoff := min(data.projection.compactionCutoff, len(events))
	mode := normalizeCompactMode(request.Mode)
	activeEnd := maxActiveSegmentEnd(data.segments)
	switch {
	case mode == "aggressive":
		return m.writeAggressiveSessionCompaction(ctx, session, agent, request, data.segments, events, cutoff, result)
	case cutoff > activeEnd:
		return m.writeManualSessionCompaction(ctx, session, agent, request, data.segments, events, cutoff, activeEnd, result)
	default:
		return result, nil
	}
}

func (m *SessionContextManager) writeAggressiveSessionCompaction(ctx context.Context, session Session, agent Agent, request SessionCompactRequest, segments []HandoffSegment, events []*adksession.Event, cutoff int, result sessionCompactionResult) (sessionCompactionResult, error) {
	if cutoff == 0 && len(segments) == 0 {
		return result, nil
	}
	deterministic := buildHandoffSummary(segments, events[:cutoff], "aggressive")
	merged, degraded := m.mergeSummary(ctx, agent, deterministic, joinSegmentSummaries(segments), "aggressive")
	next := HandoffSegment{
		SessionID:         session.ID,
		ContextRevisionID: result.nextRevisionID,
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
		return sessionCompactionResult{}, err
	}
	result.wroteRevision = true
	result.degradedSummary = degraded
	return result, nil
}

func (m *SessionContextManager) writeManualSessionCompaction(ctx context.Context, session Session, agent Agent, request SessionCompactRequest, segments []HandoffSegment, events []*adksession.Event, cutoff int, activeEnd int, result sessionCompactionResult) (sessionCompactionResult, error) {
	deterministic := buildHandoffSummary(nil, events[activeEnd:cutoff], "normal")
	merged, degraded := m.mergeSummary(ctx, agent, deterministic, joinSegmentSummaries(segments), "normal")
	next := HandoffSegment{
		SessionID:         session.ID,
		ContextRevisionID: result.nextRevisionID,
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
		return sessionCompactionResult{}, err
	}
	result.wroteRevision = true
	result.degradedSummary = degraded
	return result, nil
}

func applySessionCompactionState(state SessionContextState, sessionID string, agent Agent, request SessionCompactRequest, result sessionCompactionResult, now string) SessionContextState {
	state.SessionID = sessionID
	if result.wroteRevision {
		state.PreviousContextRevisionID = result.previousRevisionID
		state.ContextRevisionID = result.nextRevisionID
		state.ContextRevisionCreatedAt = result.nextRevisionCreatedAt
	}
	state.RecentUserWindow = normalizeRecentUserWindow(agent.RecentUserWindow)
	state.LastCompactedAt = now
	state.LastCompactionMode = compactionModeLabel(request)
	state.LastCompactionReason = strings.TrimSpace(request.Reason)
	state.AutoCompacted = request.Trigger == "auto"
	state.DegradedSummary = result.degradedSummary
	state.UpdatedAt = now
	return state
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
	cutoff := min(m.projectSnapshot(raw.Session, state, agent, segments, "").compactionCutoff, len(events))
	activeEnd := min(maxActiveSegmentEnd(segments), len(events))
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
	if raw == nil || raw.Session == nil || raw.Session.State() == nil || isSyntheticADKSession(raw.Session) {
		return nil
	}
	state := raw.Session.State()
	summary := joinSegmentSummaries(segments)
	if stateTextValue(state, adkSessionHandoffSummaryKey) == summary &&
		stateTextValue(state, adkSessionHandoffCountKey) == fmt.Sprint(len(segments)) {
		return nil
	}
	event := adksession.NewEvent(ctx, "jftrade-handoff-state")
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
		// A run waiting for approval is quiescent. Its unresolved confirmation
		// events are protected by protectedTailStart, so context compaction can
		// safely advance up to (but never across) that approval boundary.
		if run.Status == RunStatusRunning {
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
	currentCutoff := min(maxActiveSegmentEnd(segments), len(events))
	protectedStart := max(protectedTailStart(events), currentCutoff)
	recentStart := max(recentUserEventStart(events, recentWindow), currentCutoff)

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
