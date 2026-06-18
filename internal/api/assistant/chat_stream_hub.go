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
	ctx := context.Background()
	sessionSent := false
	contextSent := false
	timelineState := &adkTimelineStreamState{}
	publish := func(event adkChatStreamEvent) {
		h.streams.publish(record, event)
	}

	if session, err := h.service.PreviewSession(ctx, payload); err == nil && strings.TrimSpace(session.ID) != "" {
		publish(adkChatStreamEvent{Type: "session", Session: &session})
		sessionSent = true
		timelineState.observeSession(session)
	}

	result, err := h.service.ChatStream(ctx, payload, func(delta jfadk.ChatDelta) error {
		if delta.Timeline != nil {
			timeline := jfadk.NormalizeTimelineEntry(*delta.Timeline)
			timelineState.observeTimeline(timeline)
			publish(adkChatStreamEvent{Type: "timeline", Timeline: &timeline})
			if delta.Run == nil && delta.Context == nil && delta.Reply == "" && delta.ReasoningContent == "" {
				return nil
			}
		}
		if delta.Run != nil {
			delta.Run = new(jfadk.NormalizeRun(*delta.Run))
			timelineState.observeRun(delta.Run)
			publish(adkChatStreamEvent{Type: "run", Run: delta.Run})
			if timeline := timelineState.toolGroupSnapshot(); timeline != nil {
				publish(adkChatStreamEvent{Type: "timeline", Timeline: timeline})
			}
			return nil
		}
		if delta.Context != nil {
			contextSent = true
			publish(adkChatStreamEvent{Type: "context", Context: delta.Context})
		}
		if !sessionSent {
			session, sessionErr := h.service.PreviewSession(ctx, payload)
			if sessionErr == nil && strings.TrimSpace(session.ID) != "" {
				publish(adkChatStreamEvent{Type: "session", Session: &session})
				sessionSent = true
				timelineState.observeSession(session)
				if !contextSent {
					if snapshot, snapshotErr := h.service.GetSessionContext(ctx, session.ID); snapshotErr == nil {
						publish(adkChatStreamEvent{Type: "context", Context: &snapshot})
						contextSent = true
					}
				}
			}
		}
		if reasoningTimeline := timelineState.appendReasoning(delta.Run, delta.ReasoningContent); reasoningTimeline != nil {
			publish(adkChatStreamEvent{Type: "timeline", Timeline: reasoningTimeline})
		}
		if messageTimeline := timelineState.appendMessage(delta.Run, delta.Reply); messageTimeline != nil {
			publish(adkChatStreamEvent{Type: "timeline", Timeline: messageTimeline})
		}
		return nil
	})
	if err != nil {
		if finalResponse, finalErr := h.service.RecoverTerminalChatResponse(ctx, record.currentRunID()); finalErr == nil && finalResponse != nil {
			publish(adkChatStreamEvent{Type: "final", Response: finalResponse})
			return
		}
		publish(adkChatStreamEvent{Type: "error", Message: err.Error()})
		return
	}

	response := result
	if !sessionSent {
		publish(adkChatStreamEvent{Type: "session", Session: &response.Session})
	}
	if !contextSent && response.Context != nil {
		publish(adkChatStreamEvent{Type: "context", Context: response.Context})
	}
	trimmedRun := response.Run
	for i := range trimmedRun.ToolCalls {
		trimmedRun.ToolCalls[i].Output = nil
	}
	publish(adkChatStreamEvent{Type: "final", Response: new(jfadk.NormalizeChatResponse(jfadk.ChatResponse{
		Reply:            response.Reply,
		ReasoningContent: response.ReasoningContent,
		Session:          response.Session,
		Run:              trimmedRun,
		PendingApprovals: response.PendingApprovals,
		Timeline:         response.Timeline,
		Context:          response.Context,
	}))})
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
