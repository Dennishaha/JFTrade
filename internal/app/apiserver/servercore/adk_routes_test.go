package servercore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	adksession "google.golang.org/adk/v2/session"

	asst "github.com/jftrade/jftrade-main/internal/assistant"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestADKSessionDetailOmitsResolvedApprovalGroups(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{jfadk.PermissionModeApproval},
		RequiresApprovalIn: []string{jfadk.PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"saved": true}, nil
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "session-approval-agent",
		Name:           "Session Approval Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"approval.required"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := server.adkRuntime.Store().CreateSession(t.Context(), agent.ID, "approval cleanup")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	chatResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/chat", "application/json", strings.NewReader(`{"agentId":"`+agent.ID+`","sessionId":"`+session.ID+`","message":"<execute-tool name=\"approval.required\" />"}`))
	if err != nil {
		t.Fatalf("POST session chat: %v", err)
	}
	defer func() { jftradeCheckTestError(t, chatResp.Body.Close()) }()
	if chatResp.StatusCode != http.StatusOK {
		t.Fatalf("POST session chat status = %d", chatResp.StatusCode)
	}
	var chatEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Run              jfadk.Run        `json:"run"`
			PendingApprovals []jfadk.Approval `json:"pendingApprovals"`
		} `json:"data"`
	}
	if err := json.NewDecoder(chatResp.Body).Decode(&chatEnvelope); err != nil {
		t.Fatalf("decode session chat: %v", err)
	}
	if !chatEnvelope.OK || len(chatEnvelope.Data.PendingApprovals) != 1 {
		t.Fatalf("chat envelope = %+v, want one pending approval", chatEnvelope)
	}
	approvalID := chatEnvelope.Data.PendingApprovals[0].ID

	approveResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/approvals/"+approvalID+"/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST approve: %v", err)
	}
	defer func() { jftradeCheckTestError(t, approveResp.Body.Close()) }()
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d", approveResp.StatusCode)
	}

	deadline := time.Now().Add(2 * time.Second)
	terminal := false
	for time.Now().Before(deadline) {
		run, ok, err := server.adkRuntime.Store().Run(t.Context(), chatEnvelope.Data.Run.ID)
		if err != nil || !ok {
			t.Fatalf("Run lookup err=%v ok=%v", err, ok)
		}
		if run.Status != jfadk.RunStatusPending && run.Status != jfadk.RunStatusRunning {
			terminal = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !terminal {
		run, ok, err := server.adkRuntime.Store().Run(t.Context(), chatEnvelope.Data.Run.ID)
		t.Fatalf("run did not reach terminal status after approval: run=%+v ok=%v err=%v", run, ok, err)
	}

	getResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions/"+session.ID)
	if err != nil {
		t.Fatalf("GET session detail: %v", err)
	}
	defer func() { jftradeCheckTestError(t, getResp.Body.Close()) }()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET session detail status = %d", getResp.StatusCode)
	}
	var getEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Session  jfadk.Session         `json:"session"`
			Timeline []jfadk.TimelineEntry `json:"timeline"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&getEnvelope); err != nil {
		t.Fatalf("decode session detail: %v", err)
	}
	if !getEnvelope.OK {
		t.Fatalf("session detail envelope = %+v", getEnvelope)
	}
	for _, entry := range getEnvelope.Data.Timeline {
		if entry.Kind == jfadk.TimelineKindApprovalGroup {
			t.Fatalf("timeline approval group = %+v, want resolved approval omitted", entry)
		}
	}

	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/approvals?status=PENDING")
	if err != nil {
		t.Fatalf("GET pending approvals: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listResp.Body.Close()) }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("GET pending approvals status = %d", listResp.StatusCode)
	}
	var listEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Approvals []jfadk.Approval `json:"approvals"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listEnvelope); err != nil {
		t.Fatalf("decode pending approvals: %v", err)
	}
	if !listEnvelope.OK {
		t.Fatalf("pending approvals envelope = %+v", listEnvelope)
	}
	for _, approval := range listEnvelope.Data.Approvals {
		if approval.ID == approvalID {
			t.Fatalf("pending approvals = %+v, want resolved approval %q omitted", listEnvelope.Data.Approvals, approvalID)
		}
	}
}

func TestADKAuditRouteFiltersByKindAndSubjectID(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{jfadk.PermissionModeApproval},
		RequiresApprovalIn: []string{jfadk.PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"saved": true}, nil
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit", "saved", map[string]any{"idx": 1})
	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit", "saved-again", map[string]any{"idx": 2})
	server.adkRuntime.RecordAudit(t.Context(), "provider.saved", "provider-audit", "provider", map[string]any{"idx": 3})

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/audit?kind=agent.saved&subjectId=agent-audit&limit=1")
	if err != nil {
		t.Fatalf("GET audit: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Events []jfadk.AuditEvent `json:"events"`
			Page   struct {
				Total    int `json:"total"`
				Returned int `json:"returned"`
			} `json:"page"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	if !envelope.OK || envelope.Data.Page.Total != 2 || envelope.Data.Page.Returned != 1 || len(envelope.Data.Events) != 1 {
		t.Fatalf("audit envelope = %+v", envelope)
	}
	if envelope.Data.Events[0].Kind != "agent.saved" || envelope.Data.Events[0].SubjectID != "agent-audit" {
		t.Fatalf("audit event = %+v, want filtered agent.saved subject", envelope.Data.Events[0])
	}
}

func TestADKAuditRouteDefaultPagination(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit-1", "saved", nil)
	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit-2", "saved", nil)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/audit?limit=oops&offset=-10")
	if err != nil {
		t.Fatalf("GET audit with invalid page params: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Events []jfadk.AuditEvent `json:"events"`
			Page   struct {
				Limit    int  `json:"limit"`
				Offset   int  `json:"offset"`
				Total    int  `json:"total"`
				Returned int  `json:"returned"`
				HasMore  bool `json:"hasMore"`
			} `json:"page"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode audit default page: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("audit default page envelope = %+v", envelope)
	}
	if envelope.Data.Page.Limit != 100 || envelope.Data.Page.Offset != 0 || envelope.Data.Page.Total != 2 || envelope.Data.Page.Returned != 2 || envelope.Data.Page.HasMore {
		t.Fatalf("audit default page = %+v, want default pagination", envelope.Data.Page)
	}
}

func TestADKChatStreamEmitsSessionRunAndFinalEvents(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.store.SaveADKSettings(ADKRuntimeSettings{RunTimeoutMs: 720_000, StreamIdleTimeoutMs: 420_000}); err != nil {
		t.Fatalf("saveADKSettings: %v", err)
	}

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "stream-agent",
		Name:           "Stream Agent",
		ProviderID:     testADKProviderID,
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/adk/chat/stream", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat stream status = %d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-ADK-Stream-Idle-Timeout-Ms") != "420000" {
		t.Fatalf("stream idle timeout header = %q, want 420000", rec.Header().Get("X-ADK-Stream-Idle-Timeout-Ms"))
	}
	frames := parseADKStreamFrames(t, rec.Body.String())
	var eventTypes []string
	var finalEvent *adkChatStreamEvent
	for i := range frames {
		eventTypes = append(eventTypes, frames[i].Type)
		if frames[i].Type == "final" {
			finalEvent = &frames[i]
		}
	}
	sort.Strings(eventTypes)
	if !containsString(eventTypes, "run") || !containsString(eventTypes, "session") || !containsString(eventTypes, "final") {
		t.Fatalf("stream event types = %#v, want session/run/final; raw=%q", eventTypes, rec.Body.String())
	}
	if finalEvent == nil || finalEvent.Response == nil {
		t.Fatalf("final event = %+v, want response", finalEvent)
		return
	}
	if finalEvent.Response.Run.AgentID != agent.ID {
		t.Fatalf("final run agent = %q, want %q", finalEvent.Response.Run.AgentID, agent.ID)
	}
	if finalEvent.Response.Run.MaxDurationMs != 720_000 {
		t.Fatalf("final run maxDurationMs = %d, want 720000", finalEvent.Response.Run.MaxDurationMs)
	}
	streamID := rec.Header().Get("X-ADK-Stream-ID")
	if streamID == "" {
		t.Fatal("stream id header is empty")
	}
	replayReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/adk/streams/"+streamID+"?after=1", nil)
	replayRec := httptest.NewRecorder()
	server.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("stream replay status = %d body=%q", replayRec.Code, replayRec.Body.String())
	}
	replayed := parseADKStreamFrames(t, replayRec.Body.String())
	if len(replayed) == 0 {
		t.Fatal("stream replay returned no events")
	}
	for _, event := range replayed {
		if event.Sequence <= 1 || !event.Replay || event.StreamID != streamID {
			t.Fatalf("replayed event = %+v, want sequence > 1 with replay metadata", event)
		}
	}
	runReplayReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/adk/runs/"+finalEvent.Response.Run.ID+"/stream?after=1", nil)
	runReplayRec := httptest.NewRecorder()
	server.ServeHTTP(runReplayRec, runReplayReq)
	if runReplayRec.Code != http.StatusOK {
		t.Fatalf("run stream replay status = %d body=%q", runReplayRec.Code, runReplayRec.Body.String())
	}

	detachedCtx, cancelDetached := context.WithCancel(context.Background())
	detachedReq := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost,
		"/api/v1/adk/chat/stream",
		strings.NewReader(`{"agentId":"`+agent.ID+`","message":"detached execution"}`),
	).WithContext(detachedCtx)
	detachedReq.Header.Set("Content-Type", "application/json")
	cancelDetached()
	detachedRec := httptest.NewRecorder()
	server.ServeHTTP(detachedRec, detachedReq)

	deadline := time.Now().Add(2 * time.Second)
	for {
		runs, listErr := server.adkRuntime.Store().ListRuns(t.Context())
		if listErr != nil {
			t.Fatalf("ListRuns after detached stream: %v", listErr)
		}
		completed := false
		for _, run := range runs {
			if run.UserMessage == "detached execution" && run.Status == jfadk.RunStatusCompleted {
				completed = true
				break
			}
		}
		if completed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("detached stream did not complete after request cancellation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestADKChatReturnsCompletedEnvelopeWithVisibleToolFailure(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name: "strategy.save_draft", Permission: "write_strategy",
		AllowedModes: []string{jfadk.PermissionModeApproval, jfadk.PermissionModeLessApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return nil, errors.New("disk full")
	})

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "chat-failed-run-agent",
		Name:           "Failed Run Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: jfadk.PermissionModeLessApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/adk/chat", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"@strategy.save_draft 保存失败草稿"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat status = %d body=%q", rec.Code, rec.Body.String())
	}
	var envelope struct {
		OK   bool               `json:"ok"`
		Data jfadk.ChatResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode chat envelope: %v body=%q", err, rec.Body.String())
	}
	if !envelope.OK {
		t.Fatalf("chat envelope = %+v, want ok=true", envelope)
	}
	if envelope.Data.Run.Status != jfadk.RunStatusCompleted {
		t.Fatalf("run status = %q, want %q", envelope.Data.Run.Status, jfadk.RunStatusCompleted)
	}
	if envelope.Data.Run.FailureReason != "" {
		t.Fatalf("failureReason = %q, want empty", envelope.Data.Run.FailureReason)
	}
	if envelope.Data.Run.ErrorCode != "" {
		t.Fatalf("errorCode = %q, want empty", envelope.Data.Run.ErrorCode)
	}
	if !envelope.Data.Run.Degraded {
		t.Fatalf("run degraded = %v, want true", envelope.Data.Run.Degraded)
	}
	if len(envelope.Data.Run.ToolCalls) != 1 || envelope.Data.Run.ToolCalls[0].Error == nil || !strings.Contains(*envelope.Data.Run.ToolCalls[0].Error, "disk full") {
		t.Fatalf("toolCalls = %+v, want failed tool error containing 'disk full'", envelope.Data.Run.ToolCalls)
	}
}

func TestADKChatStreamReturnsFinalEventForCompletedRunWithToolFailure(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name: "strategy.save_draft", Permission: "write_strategy",
		AllowedModes: []string{jfadk.PermissionModeApproval, jfadk.PermissionModeLessApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return nil, errors.New("disk full")
	})

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "stream-failed-run-agent",
		Name:           "Stream Failed Run Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: jfadk.PermissionModeLessApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/adk/chat/stream", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"@strategy.save_draft 保存失败草稿"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat stream status = %d body=%q", rec.Code, rec.Body.String())
	}
	frames := parseADKStreamFrames(t, rec.Body.String())
	if len(frames) == 0 {
		t.Fatalf("expected SSE frames, raw=%q", rec.Body.String())
	}
	last := frames[len(frames)-1]
	if last.Type != "final" || last.Response == nil {
		t.Fatalf("last event = %+v, want final response", last)
	}
	if last.Response.Run.Status != jfadk.RunStatusCompleted {
		t.Fatalf("run status = %q, want completed", last.Response.Run.Status)
	}
	if last.Response.Run.FailureReason != "" {
		t.Fatalf("failureReason = %q, want empty", last.Response.Run.FailureReason)
	}
	if !last.Response.Run.Degraded {
		t.Fatalf("run degraded = %v, want true", last.Response.Run.Degraded)
	}
	if len(last.Response.Run.ToolCalls) != 1 || last.Response.Run.ToolCalls[0].Error == nil || !strings.Contains(*last.Response.Run.ToolCalls[0].Error, "disk full") {
		t.Fatalf("toolCalls = %+v, want failed tool error containing 'disk full'", last.Response.Run.ToolCalls)
	}
	for _, frame := range frames {
		if frame.Type == "error" {
			t.Fatalf("frames = %+v, want failed run to end with final instead of error", frames)
		}
	}
}

func TestADKChatStreamRecoversCompletedRunAsFinalEventWhenFinalMessageAppendFails(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	failingService := &failingAppendSessionService{
		base:     adksession.InMemoryService(),
		failText: "问题摘要：attach final failure",
	}
	if err := server.adkRuntime.CloseSessionServices(); err != nil {
		t.Fatalf("CloseSessionServices: %v", err)
	}
	server.adkRuntime = jfadk.NewRuntimeWithSessionService(server.adkRuntime.Store(), server.adkRuntime.Tools(), failingService)
	server.assistantSvc = asst.NewService(server.adkRuntime)
	server.router = server.buildRouter()
	t.Cleanup(func() {
		jftradeErr1 := server.adkRuntime.Close()
		jftradeCheckTestError(t, jftradeErr1)
	})
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name: "strategy.save_draft", Permission: "write_strategy",
		AllowedModes: []string{jfadk.PermissionModeApproval, jfadk.PermissionModeLessApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return nil, errors.New("disk full")
	})

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "stream-recover-failed-run-agent",
		Name:           "Stream Recover Failed Run Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: jfadk.PermissionModeLessApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/adk/chat/stream", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"@strategy.save_draft attach final failure"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat stream status = %d body=%q", rec.Code, rec.Body.String())
	}
	frames := parseADKStreamFrames(t, rec.Body.String())
	if len(frames) == 0 {
		t.Fatalf("expected SSE frames, raw=%q", rec.Body.String())
	}
	last := frames[len(frames)-1]
	if last.Type != "final" || last.Response == nil {
		t.Fatalf("last event = %+v, want recovered final response", last)
	}
	if last.Response.Run.Status != jfadk.RunStatusCompleted {
		t.Fatalf("run status = %q, want completed", last.Response.Run.Status)
	}
	if last.Response.Run.FailureReason != "" {
		t.Fatalf("failureReason = %q, want empty", last.Response.Run.FailureReason)
	}
	if !last.Response.Run.Degraded {
		t.Fatalf("run degraded = %v, want true", last.Response.Run.Degraded)
	}
	for _, frame := range frames {
		if frame.Type == "error" {
			t.Fatalf("frames = %+v, want recovered terminal run to emit final without error", frames)
		}
	}
}

func TestADKChatStreamReturnsErrorEventForInvalidPayload(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/chat/stream", "application/json", strings.NewReader(`{"message":`))
	if err != nil {
		t.Fatalf("POST invalid chat stream: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll invalid stream: %v", err)
	}
	frames := parseADKStreamFrames(t, string(body))
	if len(frames) == 0 {
		t.Fatalf("expected at least one SSE frame, raw=%q", string(body))
	}
	last := frames[len(frames)-1]
	if last.Type != "error" || !strings.HasPrefix(last.Message, "invalid chat payload") {
		t.Fatalf("last stream event = %+v, want invalid payload error", last)
	}
}

func TestADKProviderSaveRejectsInvalidPayload(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/providers", "application/json", strings.NewReader(`{"displayName":`))
	if err != nil {
		t.Fatalf("POST invalid provider payload: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	status, code, message := decodeAPIErrorEnvelope(t, resp)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "invalid provider payload" {
		t.Fatalf("provider validation error = %d/%s/%q, want 400/BAD_REQUEST/invalid provider payload", status, code, message)
	}
}

func TestADKProviderSaveReturnsRequestTimeoutMs(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/providers", "application/json", strings.NewReader(`{
		"displayName":"Slow Provider",
		"baseUrl":"https://api.openai.com/v1",
		"model":"gpt-4o-mini",
		"requestTimeoutMs":250000,
		"enabled":true
	}`))
	if err != nil {
		t.Fatalf("POST provider: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST provider status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool           `json:"ok"`
		Data jfadk.Provider `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode provider save: %v", err)
	}
	if !envelope.OK || envelope.Data.RequestTimeoutMs != 250_000 {
		t.Fatalf("provider save envelope = %+v", envelope)
	}
}

func TestADKAgentSaveValidationFailures(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	disabledProvider, err := server.adkRuntime.Store().SaveProvider(t.Context(), jfadk.ProviderWriteRequest{
		ID:          "provider-disabled",
		DisplayName: "Disabled Provider",
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		APIKey:      "sk-disabled",
		Enabled:     false,
	})
	if err != nil {
		t.Fatalf("SaveProvider disabled: %v", err)
	}
	if _, err := server.adkRuntime.Store().SaveProvider(t.Context(), jfadk.ProviderWriteRequest{
		ID:          "provider-no-key",
		DisplayName: "No Key Provider",
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("SaveProvider no-key: %v", err)
	}

	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{
			name:        "disabled provider",
			body:        `{"id":"agent-disabled-provider","name":"Agent","providerId":"` + disabledProvider.ID + `","permissionMode":"approval","status":"ENABLED"}`,
			wantMessage: "provider is disabled",
		},
		{
			name:        "missing provider key",
			body:        `{"id":"agent-no-key","name":"Agent","providerId":"provider-no-key","permissionMode":"approval","status":"ENABLED"}`,
			wantMessage: "provider API key is not configured",
		},
		{
			name:        "unknown tool",
			body:        `{"id":"agent-bad-tool","name":"Agent","permissionMode":"approval","status":"ENABLED","tools":["tool.does_not_exist"]}`,
			wantMessage: "unknown ADK tool: tool.does_not_exist",
		},
		{
			name:        "unknown skill",
			body:        `{"id":"agent-bad-skill","name":"Agent","permissionMode":"approval","status":"ENABLED","skills":["skill-does-not-exist"]}`,
			wantMessage: "unknown ADK skill: skill-does-not-exist",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/agents", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("POST agent payload: %v", err)
			}
			defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

			status, code, message := decodeAPIErrorEnvelope(t, resp)
			if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != tc.wantMessage {
				t.Fatalf("agent validation error = %d/%s/%q, want 400/BAD_REQUEST/%q", status, code, message, tc.wantMessage)
			}
		})
	}
}

func TestADKSkillInstallAndUninstallFailureRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	installResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/skills", "application/json", strings.NewReader(`{"url":"not-a-valid-url"}`))
	if err != nil {
		t.Fatalf("POST invalid skill install: %v", err)
	}
	defer func() { jftradeCheckTestError(t, installResp.Body.Close()) }()
	status, code, message := decodeAPIErrorEnvelope(t, installResp)
	if status != http.StatusBadRequest || code != "ADK_SKILL_INSTALL_FAILED" || strings.TrimSpace(message) == "" {
		t.Fatalf("skill install error = %d/%s/%q, want 400/ADK_SKILL_INSTALL_FAILED/non-empty", status, code, message)
	}

	deleteReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/adk/skills/jftrade-market", nil)
	if err != nil {
		t.Fatalf("NewRequest delete builtin skill: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE builtin skill: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteResp.Body.Close()) }()
	status, code, message = decodeAPIErrorEnvelope(t, deleteResp)
	if status != http.StatusInternalServerError || code != "ADK_SKILL_UNINSTALL_FAILED" || !strings.Contains(strings.ToLower(message), "builtin") {
		t.Fatalf("skill uninstall error = %d/%s/%q, want 500/ADK_SKILL_UNINSTALL_FAILED/*builtin*", status, code, message)
	}
}

func TestADKBindAgentWithPreinstalledNeodataFinancialSearch(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	skillDir := filepath.Join(server.adkRuntime.Store().SkillsPath(), "neodata-financial-search")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll skill dir: %v", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(`---
name: neodata-financial-search
description: Search NeoData financial filings and earnings materials.
allowed-tools: [http.fetch]
metadata:
  version: 2026.06
---
Use NeoData search results as reference material and cite the source URL.`), 0o644); err != nil {
		t.Fatalf("WriteFile skill doc: %v", err)
	}

	createBody := `{"id":"agent-neodata","name":"Agent NeoData","permissionMode":"approval","status":"ENABLED","tools":["http.fetch"],"skills":["neodata-financial-search"]}`
	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/agents", "application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST create agent: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST create agent status = %d", createResp.StatusCode)
	}
	var agentEnvelope struct {
		OK   bool        `json:"ok"`
		Data jfadk.Agent `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&agentEnvelope); err != nil {
		t.Fatalf("decode agent envelope: %v", err)
	}
	if !agentEnvelope.OK || !containsString(agentEnvelope.Data.Skills, "neodata-financial-search") {
		t.Fatalf("agent envelope = %+v", agentEnvelope)
	}
}

func TestADKSessionNegativeRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/sessions", "application/json", strings.NewReader(`{"agentId":"missing-agent","title":"x"}`))
	if err != nil {
		t.Fatalf("POST invalid session create: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
	status, code, message := decodeAPIErrorEnvelope(t, createResp)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "enabled agent is required" {
		t.Fatalf("session create error = %d/%s/%q, want 400/BAD_REQUEST/enabled agent is required", status, code, message)
	}

	invalidDetailReq := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/api/v1/adk/sessions/%zz"},
		Header: make(http.Header),
	}
	invalidDetailRec := httptest.NewRecorder()
	server.ServeHTTP(invalidDetailRec, invalidDetailReq)
	status, code, message = decodeAPIErrorRecorder(t, invalidDetailRec)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "sessionId is invalid" {
		t.Fatalf("session invalid id error = %d/%s/%q", status, code, message)
	}

	missingResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions/session-missing")
	if err != nil {
		t.Fatalf("GET missing session detail: %v", err)
	}
	defer func() { jftradeCheckTestError(t, missingResp.Body.Close()) }()
	status, code, message = decodeAPIErrorEnvelope(t, missingResp)
	if status != http.StatusNotFound || code != "NOT_FOUND" || message != "session not found" {
		t.Fatalf("session missing error = %d/%s/%q", status, code, message)
	}

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "session-negative-agent",
		Name:           "Session Negative Agent",
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := server.adkRuntime.Store().CreateSession(t.Context(), agent.ID, "valid")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	renameReq, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/adk/sessions/"+session.ID, strings.NewReader(`{"title":`))
	if err != nil {
		t.Fatalf("NewRequest invalid rename: %v", err)
	}
	renameReq.Header.Set("Content-Type", "application/json")
	renameResp, err := http.DefaultClient.Do(renameReq)
	if err != nil {
		t.Fatalf("PUT invalid rename: %v", err)
	}
	defer func() { jftradeCheckTestError(t, renameResp.Body.Close()) }()
	status, code, message = decodeAPIErrorEnvelope(t, renameResp)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "invalid session payload" {
		t.Fatalf("session rename payload error = %d/%s/%q", status, code, message)
	}
}

func TestADKRunNegativeRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	server := newTestServer(t, store)
	invalidCancelReq := &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Path: "/api/v1/adk/runs/%zz/cancel"},
		Header: make(http.Header),
	}
	invalidCancelRec := httptest.NewRecorder()
	server.ServeHTTP(invalidCancelRec, invalidCancelReq)
	status, code, message := decodeAPIErrorRecorder(t, invalidCancelRec)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "runId is invalid" {
		t.Fatalf("run cancel invalid error = %d/%s/%q", status, code, message)
	}

	cancelMissingResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/runs/run-missing/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST missing cancel run: %v", err)
	}
	defer func() { jftradeCheckTestError(t, cancelMissingResp.Body.Close()) }()
	status, code, message = decodeAPIErrorEnvelope(t, cancelMissingResp)
	if status != http.StatusNotFound || code != "ADK_RUN_CANCEL_FAILED" || message != "run not found" {
		t.Fatalf("run cancel missing error = %d/%s/%q", status, code, message)
	}

	invalidGetReq := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/api/v1/adk/runs/%zz"},
		Header: make(http.Header),
	}
	invalidGetRec := httptest.NewRecorder()
	server.ServeHTTP(invalidGetRec, invalidGetReq)
	status, code, message = decodeAPIErrorRecorder(t, invalidGetRec)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "runId is invalid" {
		t.Fatalf("run get invalid error = %d/%s/%q", status, code, message)
	}

	getMissingResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/runs/run-missing")
	if err != nil {
		t.Fatalf("GET missing run: %v", err)
	}
	defer func() { jftradeCheckTestError(t, getMissingResp.Body.Close()) }()
	status, code, message = decodeAPIErrorEnvelope(t, getMissingResp)
	if status != http.StatusNotFound || code != "NOT_FOUND" || message != "run not found" {
		t.Fatalf("run get missing error = %d/%s/%q", status, code, message)
	}
}

func TestADKApprovalNegativeAndIdempotentRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{jfadk.PermissionModeApproval},
		RequiresApprovalIn: []string{jfadk.PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"saved": true}, nil
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	invalidReq := &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Path: "/api/v1/adk/approvals/%zz/approve"},
		Header: make(http.Header),
	}
	invalidRec := httptest.NewRecorder()
	server.ServeHTTP(invalidRec, invalidReq)
	status, code, message := decodeAPIErrorRecorder(t, invalidRec)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "approvalId is invalid" {
		t.Fatalf("approval invalid error = %d/%s/%q", status, code, message)
	}

	missingResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/approvals/approval-missing/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST missing approval: %v", err)
	}
	defer func() { jftradeCheckTestError(t, missingResp.Body.Close()) }()
	if missingResp.StatusCode != http.StatusOK {
		t.Fatalf("missing approval status = %d, want 200", missingResp.StatusCode)
	}
	var missingEnvelope struct {
		OK   bool                     `json:"ok"`
		Data jfadk.ApprovalResolution `json:"data"`
	}
	if err := json.NewDecoder(missingResp.Body).Decode(&missingEnvelope); err != nil {
		t.Fatalf("decode missing approval envelope: %v", err)
	}
	if !missingEnvelope.OK || missingEnvelope.Data.Run != nil || missingEnvelope.Data.Message != nil || missingEnvelope.Data.Approval.ID != "" || missingEnvelope.Data.Approval.Status != "" {
		t.Fatalf("missing approval envelope = %+v, want idempotent empty success result", missingEnvelope)
	}

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "approval-idempotent-agent",
		Name:           "Approval Idempotent Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"approval.required"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	chatResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/chat", "application/json", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"<execute-tool name=\"approval.required\" />"}`))
	if err != nil {
		t.Fatalf("POST chat for approval: %v", err)
	}
	defer func() { jftradeCheckTestError(t, chatResp.Body.Close()) }()
	var chatEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			PendingApprovals []jfadk.Approval `json:"pendingApprovals"`
		} `json:"data"`
	}
	if err := json.NewDecoder(chatResp.Body).Decode(&chatEnvelope); err != nil {
		t.Fatalf("decode chat envelope: %v", err)
	}
	if len(chatEnvelope.Data.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1", len(chatEnvelope.Data.PendingApprovals))
	}
	approvalID := chatEnvelope.Data.PendingApprovals[0].ID

	firstResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/approvals/"+approvalID+"/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST first approval: %v", err)
	}
	defer func() { jftradeCheckTestError(t, firstResp.Body.Close()) }()
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("first approval status = %d", firstResp.StatusCode)
	}

	secondResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/approvals/"+approvalID+"/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST second approval: %v", err)
	}
	defer func() { jftradeCheckTestError(t, secondResp.Body.Close()) }()
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("second approval status = %d", secondResp.StatusCode)
	}
	var secondEnvelope struct {
		OK   bool                     `json:"ok"`
		Data jfadk.ApprovalResolution `json:"data"`
	}
	if err := json.NewDecoder(secondResp.Body).Decode(&secondEnvelope); err != nil {
		t.Fatalf("decode second approval envelope: %v", err)
	}
	if !secondEnvelope.OK || secondEnvelope.Data.Approval.Status != jfadk.ApprovalStatusApproved {
		t.Fatalf("second approval envelope = %+v, want idempotent approved result", secondEnvelope)
	}
}

func parseADKStreamFrames(t *testing.T, body string) []adkChatStreamEvent {
	t.Helper()
	segments := strings.Split(body, "\n\n")
	events := make([]adkChatStreamEvent, 0, len(segments))
	for _, segment := range segments {
		if !strings.Contains(segment, "data:") {
			continue
		}
		var dataLines []string
		for line := range strings.SplitSeq(segment, "\n") {
			if after, ok := strings.CutPrefix(line, "data:"); ok {
				dataLines = append(dataLines, strings.TrimSpace(after))
			}
		}
		payload := strings.Join(dataLines, "\n")
		if strings.TrimSpace(payload) == "" {
			continue
		}
		var event adkChatStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			t.Fatalf("unmarshal SSE frame: %v; payload=%q", err, payload)
		}
		events = append(events, event)
	}
	return events
}

func containsString(values []string, target string) bool {
	return slices.Contains(values, target)
}

func decodeAPIErrorEnvelope(t *testing.T, resp *http.Response) (int, string, string) {
	t.Helper()
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	return resp.StatusCode, envelope.Error.Code, envelope.Error.Message
}

func decodeAPIErrorRecorder(t *testing.T, rec *httptest.ResponseRecorder) (int, string, string) {
	t.Helper()
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode recorder error envelope: %v body=%q", err, rec.Body.String())
	}
	return rec.Code, envelope.Error.Code, envelope.Error.Message
}

type failingAppendSessionService struct {
	base     adksession.Service
	failText string
}

func (s *failingAppendSessionService) Create(ctx context.Context, req *adksession.CreateRequest) (*adksession.CreateResponse, error) {
	return s.base.Create(ctx, req)
}

func (s *failingAppendSessionService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	return s.base.Get(ctx, req)
}

func (s *failingAppendSessionService) List(ctx context.Context, req *adksession.ListRequest) (*adksession.ListResponse, error) {
	return s.base.List(ctx, req)
}

func (s *failingAppendSessionService) Delete(ctx context.Context, req *adksession.DeleteRequest) error {
	return s.base.Delete(ctx, req)
}

func (s *failingAppendSessionService) AppendEvent(ctx context.Context, session adksession.Session, event *adksession.Event) error {
	if strings.Contains(sessionEventText(event), s.failText) {
		return errors.New("simulated append failure")
	}
	return s.base.AppendEvent(ctx, session, event)
}

func sessionEventText(event *adksession.Event) string {
	if event == nil {
		return ""
	}
	var parts []string
	if event.Content != nil {
		for _, part := range event.Content.Parts {
			if part != nil && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	if event.Content != nil {
		for _, part := range event.Content.Parts {
			if part != nil && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}
