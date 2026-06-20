package servercore

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestADKRoutesSerializeEmptySlicesAsArrays(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	session, err := server.adkRuntime.Store().CreateSession(t.Context(), "agent-default", "normalize")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run := jfadk.Run{
		ID:        "run-json-normalize",
		SessionID: session.ID,
		AgentID:   "agent-default",
		Status:    jfadk.RunStatusCompleted,
		Message:   "completed",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := server.adkRuntime.Store().SaveRun(t.Context(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	runResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/runs/"+run.ID)
	if err != nil {
		t.Fatalf("GET run: %v", err)
	}
	defer func() { jftradeCheckTestError(t, runResp.Body.Close()) }()
	var runEnvelope map[string]any
	if err := json.NewDecoder(runResp.Body).Decode(&runEnvelope); err != nil {
		t.Fatalf("decode run envelope: %v", err)
	}
	runData := jftradeCheckedTypeAssertion[map[string]any](runEnvelope["data"])
	if _, ok := runData["toolCalls"].([]any); !ok {
		t.Fatalf("toolCalls = %#v, want JSON array", runData["toolCalls"])
	}
	if _, ok := runData["pendingApprovals"].([]any); !ok {
		t.Fatalf("pendingApprovals = %#v, want JSON array", runData["pendingApprovals"])
	}

	sessionResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions/"+session.ID)
	if err != nil {
		t.Fatalf("GET session: %v", err)
	}
	defer func() { jftradeCheckTestError(t, sessionResp.Body.Close()) }()
	var sessionEnvelope map[string]any
	if err := json.NewDecoder(sessionResp.Body).Decode(&sessionEnvelope); err != nil {
		t.Fatalf("decode session envelope: %v", err)
	}
	sessionData := jftradeCheckedTypeAssertion[map[string]any](sessionEnvelope["data"])
	if _, ok := sessionData["timeline"].([]any); !ok {
		t.Fatalf("timeline = %#v, want JSON array", sessionData["timeline"])
	}

	approval := jfadk.Approval{
		ID:        "approval-json-normalize",
		RunID:     "run-approval-normalize",
		AgentID:   "agent-default",
		ToolName:  "strategy.save_draft",
		Status:    jfadk.ApprovalStatusPending,
		Reason:    "review",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := server.adkRuntime.Store().SaveRun(t.Context(), jfadk.Run{
		ID:               approval.RunID,
		SessionID:        session.ID,
		AgentID:          approval.AgentID,
		Status:           jfadk.RunStatusPending,
		Message:          "waiting approval",
		PendingApprovals: []jfadk.Approval{approval},
		CreatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("SaveRun approval: %v", err)
	}
	if err := server.adkRuntime.Store().SaveApproval(t.Context(), approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	approvalResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/approvals/"+approval.ID+"/deny", "application/json", nil)
	if err != nil {
		t.Fatalf("POST approval deny: %v", err)
	}
	defer func() { jftradeCheckTestError(t, approvalResp.Body.Close()) }()
	var approvalEnvelope map[string]any
	if err := json.NewDecoder(approvalResp.Body).Decode(&approvalEnvelope); err != nil {
		t.Fatalf("decode approval envelope: %v", err)
	}
	approvalData := jftradeCheckedTypeAssertion[map[string]any](approvalEnvelope["data"])
	resolutionRun := jftradeCheckedTypeAssertion[map[string]any](approvalData["run"])
	if _, ok := resolutionRun["toolCalls"].([]any); !ok {
		t.Fatalf("resolution run toolCalls = %#v, want JSON array", resolutionRun["toolCalls"])
	}
	if _, ok := resolutionRun["pendingApprovals"].([]any); !ok {
		t.Fatalf("resolution run pendingApprovals = %#v, want JSON array", resolutionRun["pendingApprovals"])
	}
}
