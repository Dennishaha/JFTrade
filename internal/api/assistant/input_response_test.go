package assistant

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestRunInputResponseErrorAndRetryContracts(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	run := jfadk.Run{
		ID: "run-input-errors", SessionID: "session-input-errors", AgentID: "agent-input-errors",
		Status: jfadk.RunStatusPendingInput, ResumeState: "waiting_input", CreatedAt: now, UpdatedAt: now,
		ToolCalls: []jfadk.ToolCall{}, PendingApprovals: []jfadk.Approval{},
		InputRequest: &jfadk.InputRequest{
			ID: "input-errors", RunID: "run-input-errors", AgentID: "agent-input-errors", FunctionCallID: "call-input-errors",
			Status: jfadk.InputRequestStatusPending, CreatedAt: now, UpdatedAt: now,
			Questions: []jfadk.InputQuestion{{
				ID: "q1", Question: "Choose", AllowOther: false,
				Options: []jfadk.InputOption{{ID: "q1-o1", Label: "A"}, {ID: "q1-o2", Label: "B"}},
			}},
		},
	}
	if err := runtime.Store().SaveRun(t.Context(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	cases := []struct {
		name   string
		path   string
		body   string
		status int
		code   string
	}{
		{name: "malformed payload", path: run.ID, body: `{`, status: http.StatusBadRequest, code: "BAD_REQUEST"},
		{name: "missing run", path: "missing-run", body: `{"requestId":"input-errors","answers":[]}`, status: http.StatusNotFound, code: "NOT_FOUND"},
		{name: "mismatched request", path: run.ID, body: `{"requestId":"other-input","answers":[{"questionId":"q1","optionId":"q1-o1"}]}`, status: http.StatusConflict, code: "ADK_INPUT_RESPONSE_CONFLICT"},
		{name: "invalid answer", path: run.ID, body: `{"requestId":"input-errors","answers":[{"questionId":"q1","otherText":"custom"}]}`, status: http.StatusBadRequest, code: "ADK_INPUT_RESPONSE_INVALID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/runs/"+tc.path+"/input-response", []byte(tc.body))
			if response.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tc.status, response.Body.String())
			}
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if envelope.Error.Code != tc.code {
				t.Fatalf("error code=%q want=%q body=%s", envelope.Error.Code, tc.code, response.Body.String())
			}
		})
	}

	body := []byte(`{"requestId":"input-errors","answers":[{"questionId":"q1","optionId":"q1-o2"}]}`)
	first := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/runs/"+run.ID+"/input-response", body)
	retry := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/runs/"+run.ID+"/input-response", body)
	if first.Code != http.StatusOK || retry.Code != http.StatusOK {
		t.Fatalf("idempotent response statuses first=%d retry=%d firstBody=%s retryBody=%s", first.Code, retry.Code, first.Body.String(), retry.Body.String())
	}
}
