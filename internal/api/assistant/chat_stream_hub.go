package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

const (
	adkChatStreamEventLimit = 2000
	adkChatStreamRetention  = 30 * time.Minute
	adkChatStreamHeartbeat  = 15 * time.Second
)

type adkChatStreamHub struct {
	mu      sync.Mutex
	streams map[string]*adkChatStreamRecord
	byRunID map[string]string
}

type adkChatStreamRecord struct {
	mu           sync.Mutex
	id           string
	runID        string
	nextSequence int64
	events       []adkChatStreamEvent
	notify       chan struct{}
	terminal     bool
	startedAt    time.Time
	updatedAt    time.Time
	expiresAt    time.Time
}

func newADKChatStreamHub() *adkChatStreamHub {
	return &adkChatStreamHub{
		streams: make(map[string]*adkChatStreamRecord),
		byRunID: make(map[string]string),
	}
}

func (h *adkChatStreamHub) create() *adkChatStreamRecord {
	h.cleanup()
	now := time.Now()
	record := &adkChatStreamRecord{
		id:        "stream-" + uuid.NewString(),
		startedAt: now,
		updatedAt: now,
		events:    make([]adkChatStreamEvent, 0, 32),
		notify:    make(chan struct{}),
	}
	h.mu.Lock()
	h.streams[record.id] = record
	h.mu.Unlock()
	return record
}

func (h *adkChatStreamHub) get(streamID string) (*adkChatStreamRecord, bool) {
	h.cleanup()
	h.mu.Lock()
	defer h.mu.Unlock()
	record, ok := h.streams[strings.TrimSpace(streamID)]
	return record, ok
}

func (h *adkChatStreamHub) getByRunID(runID string) (*adkChatStreamRecord, bool) {
	h.cleanup()
	h.mu.Lock()
	defer h.mu.Unlock()
	streamID := h.byRunID[strings.TrimSpace(runID)]
	record, ok := h.streams[streamID]
	return record, ok
}

func (h *adkChatStreamHub) publish(record *adkChatStreamRecord, event adkChatStreamEvent) {
	if record == nil {
		return
	}
	event = cloneADKChatStreamEvent(event)
	record.mu.Lock()
	record.nextSequence++
	event.StreamID = record.id
	event.Sequence = record.nextSequence
	event.RunID = streamEventRunID(event)
	if event.RunID != "" {
		record.runID = event.RunID
	}
	record.updatedAt = time.Now()
	record.events = append(record.events, event)
	if len(record.events) > adkChatStreamEventLimit {
		record.events = append([]adkChatStreamEvent(nil), record.events[len(record.events)-adkChatStreamEventLimit:]...)
	}
	if event.Type == "final" || event.Type == "error" {
		record.terminal = true
		record.expiresAt = time.Now().Add(adkChatStreamRetention)
	}
	close(record.notify)
	record.notify = make(chan struct{})
	runID := record.runID
	record.mu.Unlock()

	if runID != "" {
		h.mu.Lock()
		h.byRunID[runID] = record.id
		h.mu.Unlock()
	}
}

func cloneADKChatStreamEvent(event adkChatStreamEvent) adkChatStreamEvent {
	data, err := json.Marshal(event)
	if err != nil {
		return event
	}
	var cloned adkChatStreamEvent
	if err := json.Unmarshal(data, &cloned); err != nil {
		return event
	}
	return cloned
}

func (h *adkChatStreamHub) cleanup() {
	h.cleanupWithRunLookup(nil)
}

func (h *adkChatStreamHub) cleanupWithRunLookup(runLookup func(string) (jfadk.Run, bool)) {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	for streamID, record := range h.streams {
		record.mu.Lock()
		expired := record.terminal && !record.expiresAt.IsZero() && now.After(record.expiresAt)
		runID := record.runID
		startedAt := record.startedAt
		updatedAt := record.updatedAt
		record.mu.Unlock()
		if !expired && runLookup != nil && runID != "" {
			if run, ok := runLookup(runID); ok {
				expired = streamRunExpired(now, run, updatedAt)
			}
		}
		if !expired && runID == "" && !startedAt.IsZero() && now.Sub(startedAt) > jfadk.DefaultRunTimeout+adkChatStreamRetention {
			expired = true
		}
		if !expired {
			continue
		}
		delete(h.streams, streamID)
		if h.byRunID[runID] == streamID {
			delete(h.byRunID, runID)
		}
	}
}

func (r *adkChatStreamRecord) snapshot(after int64) ([]adkChatStreamEvent, bool, <-chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := make([]adkChatStreamEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.Sequence > after {
			events = append(events, event)
		}
	}
	return events, r.terminal, r.notify
}

func (r *adkChatStreamRecord) currentRunID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runID
}

func (r *adkChatStreamRecord) currentSequence() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nextSequence
}

func streamEventRunID(event adkChatStreamEvent) string {
	if event.Run != nil {
		return strings.TrimSpace(event.Run.ID)
	}
	if event.Response != nil {
		return strings.TrimSpace(event.Response.Run.ID)
	}
	if event.Timeline != nil {
		return strings.TrimSpace(event.Timeline.RunID)
	}
	return strings.TrimSpace(event.RunID)
}

func (h *Handler) startADKChatStream(payload jfadk.ChatRequest) *adkChatStreamRecord {
	h.cleanupADKChatStreams()
	record := h.streams.create()
	go h.executeADKChatStream(record, payload)
	return record
}

func (h *Handler) cleanupADKChatStreams() {
	if h == nil || h.streams == nil {
		return
	}
	h.streams.cleanupWithRunLookup(func(runID string) (jfadk.Run, bool) {
		run, err := h.service.GetRun(context.Background(), runID)
		if err != nil {
			return jfadk.Run{}, false
		}
		return run, true
	})
}

func (h *Handler) executeADKChatStream(record *adkChatStreamRecord, payload jfadk.ChatRequest) {
	execution := newADKChatStreamExecution(h, record, payload)
	execution.previewSession()
	result, err := h.service.ChatStream(execution.ctx, payload, execution.handleDelta)
	if err != nil {
		execution.publishTerminalError(err)
		return
	}
	execution.publishFinal(result)
}

type adkChatStreamExecution struct {
	handler       *Handler
	record        *adkChatStreamRecord
	payload       jfadk.ChatRequest
	ctx           context.Context
	sessionSent   bool
	contextSent   bool
	timelineState *adkTimelineStreamState
}

func newADKChatStreamExecution(h *Handler, record *adkChatStreamRecord, payload jfadk.ChatRequest) *adkChatStreamExecution {
	return &adkChatStreamExecution{
		handler:       h,
		record:        record,
		payload:       payload,
		ctx:           context.Background(),
		timelineState: &adkTimelineStreamState{},
	}
}

func (e *adkChatStreamExecution) publish(event adkChatStreamEvent) {
	e.handler.streams.publish(e.record, event)
}

func (e *adkChatStreamExecution) previewSession() {
	session, ok := e.fetchPreviewSession()
	if !ok {
		return
	}
	e.publishSession(session)
}

func (e *adkChatStreamExecution) fetchPreviewSession() (jfadk.Session, bool) {
	session, err := e.handler.service.PreviewSession(e.ctx, e.payload)
	if err != nil || strings.TrimSpace(session.ID) == "" {
		return jfadk.Session{}, false
	}
	return session, true
}

func (e *adkChatStreamExecution) publishSession(session jfadk.Session) {
	e.publish(adkChatStreamEvent{Type: "session", Session: &session})
	e.sessionSent = true
	e.timelineState.observeSession(session)
}

func (e *adkChatStreamExecution) handleDelta(delta jfadk.ChatDelta) error {
	if e.publishTimelineDelta(delta) {
		return nil
	}
	if e.publishRunDelta(&delta) {
		return nil
	}
	e.publishContextDelta(delta.Context)
	e.ensureSessionAndContext()
	e.publishNarrativeDeltas(delta)
	return nil
}

func (e *adkChatStreamExecution) publishTimelineDelta(delta jfadk.ChatDelta) bool {
	if delta.Timeline == nil {
		return false
	}
	timeline := jfadk.NormalizeTimelineEntry(*delta.Timeline)
	e.timelineState.observeTimeline(timeline)
	e.publish(adkChatStreamEvent{Type: "timeline", Timeline: &timeline})
	return delta.Run == nil && delta.Context == nil && delta.Reply == "" && delta.ReasoningContent == ""
}

func (e *adkChatStreamExecution) publishRunDelta(delta *jfadk.ChatDelta) bool {
	if delta.Run == nil {
		return false
	}
	normalizedRun := jfadk.NormalizeRun(*delta.Run)
	delta.Run = &normalizedRun
	e.timelineState.observeRun(delta.Run)
	e.publish(adkChatStreamEvent{Type: "run", Run: delta.Run})
	if timeline := e.timelineState.toolGroupSnapshot(); timeline != nil {
		e.publish(adkChatStreamEvent{Type: "timeline", Timeline: timeline})
	}
	return true
}

func (e *adkChatStreamExecution) publishContextDelta(snapshot *jfadk.SessionContextSnapshot) {
	if snapshot == nil {
		return
	}
	e.publish(adkChatStreamEvent{Type: "context", Context: snapshot})
	e.contextSent = true
}

func (e *adkChatStreamExecution) ensureSessionAndContext() {
	if e.sessionSent {
		return
	}
	session, ok := e.fetchPreviewSession()
	if !ok {
		return
	}
	e.publishSession(session)
	if e.contextSent {
		return
	}
	snapshot, err := e.handler.service.GetSessionContext(e.ctx, session.ID)
	if err != nil {
		return
	}
	e.publish(adkChatStreamEvent{Type: "context", Context: &snapshot})
	e.contextSent = true
}

func (e *adkChatStreamExecution) publishNarrativeDeltas(delta jfadk.ChatDelta) {
	if reasoningTimeline := e.timelineState.appendReasoning(delta.Run, delta.ReasoningContent); reasoningTimeline != nil {
		e.publish(adkChatStreamEvent{Type: "timeline", Timeline: reasoningTimeline})
	}
	if messageTimeline := e.timelineState.appendMessage(delta.Run, delta.Reply); messageTimeline != nil {
		e.publish(adkChatStreamEvent{Type: "timeline", Timeline: messageTimeline})
	}
}

func (e *adkChatStreamExecution) publishTerminalError(err error) {
	finalResponse, finalErr := e.handler.service.RecoverTerminalChatResponse(e.ctx, e.record.currentRunID())
	if finalErr == nil && finalResponse != nil {
		e.publish(adkChatStreamEvent{Type: "final", Response: finalResponse})
		return
	}
	e.publish(adkChatStreamEvent{Type: "error", Message: err.Error()})
}

func (e *adkChatStreamExecution) publishFinal(response jfadk.ChatResponse) {
	if !e.sessionSent {
		e.publish(adkChatStreamEvent{Type: "session", Session: &response.Session})
	}
	if !e.contextSent && response.Context != nil {
		e.publish(adkChatStreamEvent{Type: "context", Context: response.Context})
	}
	finalResponse := trimFinalChatResponse(response)
	e.publish(adkChatStreamEvent{Type: "final", Response: &finalResponse})
}

func trimFinalChatResponse(response jfadk.ChatResponse) jfadk.ChatResponse {
	trimmedRun := response.Run
	for i := range trimmedRun.ToolCalls {
		trimmedRun.ToolCalls[i].Output = nil
	}
	return jfadk.NormalizeChatResponse(jfadk.ChatResponse{
		Reply:            response.Reply,
		ReasoningContent: response.ReasoningContent,
		Session:          response.Session,
		Run:              trimmedRun,
		PendingApprovals: response.PendingApprovals,
		Timeline:         response.Timeline,
		Context:          response.Context,
	})
}

func (h *Handler) streamADKChatRecord(c *gin.Context, writer httpserver.SSEWriter, record *adkChatStreamRecord, after int64, replay bool) {
	current := after
	replayUntil := int64(0)
	if replay {
		replayUntil = record.currentSequence()
	}
	for {
		events, terminal, notify := record.snapshot(current)
		for _, event := range events {
			if replay && event.Sequence <= replayUntil {
				event.Replay = true
			}
			eventID := fmt.Sprintf("%s:%d", record.id, event.Sequence)
			if err := writer.WriteEventID(eventID, event); err != nil {
				return
			}
			current = event.Sequence
		}
		if terminal {
			return
		}
		timer := time.NewTimer(adkChatStreamHeartbeat)
		select {
		case <-c.Request.Context().Done():
			timer.Stop()
			return
		case <-notify:
			timer.Stop()
		case <-timer.C:
			if err := writer.WriteComment("keepalive"); err != nil {
				return
			}
		}
	}
}

func parseADKStreamAfter(c *gin.Context) (int64, error) {
	value := strings.TrimSpace(c.Query("after"))
	if value == "" {
		return 0, nil
	}
	after, err := strconv.ParseInt(value, 10, 64)
	if err != nil || after < 0 {
		return 0, fmt.Errorf("after is invalid")
	}
	return after, nil
}

func streamRunExpired(now time.Time, run jfadk.Run, lastEventAt time.Time) bool {
	if isStreamRunTerminal(run) {
		return !lastEventAt.IsZero() && now.Sub(lastEventAt) > adkChatStreamRetention
	}
	startedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(run.StartedAt))
	if err != nil {
		startedAt, err = time.Parse(time.RFC3339Nano, strings.TrimSpace(run.CreatedAt))
	}
	if err != nil {
		return !lastEventAt.IsZero() && now.Sub(lastEventAt) > jfadk.DefaultRunTimeout+adkChatStreamRetention
	}
	timeout := jfadk.DefaultRunTimeout
	if run.MaxDurationMs > 0 {
		timeout = time.Duration(run.MaxDurationMs) * time.Millisecond
	}
	return now.After(startedAt.Add(timeout).Add(adkChatStreamRetention))
}

func isStreamRunTerminal(run jfadk.Run) bool {
	switch run.Status {
	case jfadk.RunStatusCompleted, jfadk.RunStatusFailed, jfadk.RunStatusCancelled, jfadk.RunStatusTimedOut, jfadk.RunStatusDenied:
		return true
	default:
		return false
	}
}
