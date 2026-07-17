package adk

import (
	"errors"
	"strings"
	"testing"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	"google.golang.org/genai"
)

func TestCoverage98GoogleExecutionEventReplayKeepsApprovalStateIdempotent(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	service := adksession.InMemoryService()
	created, err := service.Create(ctx, &adksession.CreateRequest{
		AppName: "coverage-replay", UserID: googleADKUserID, SessionID: "coverage-replay-session",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	original := &genai.FunctionCall{
		ID: "coverage-replay-tool", Name: "strategy.research_backtest", Args: map[string]any{"symbol": "TME"},
	}
	event := adksession.NewEvent(ctx, "coverage-replay-invocation")
	event.Content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
		ID: "coverage-replay-confirmation", Name: toolconfirmation.FunctionCallName,
		Args: map[string]any{
			"originalFunctionCall": original,
			"toolConfirmation":     toolconfirmation.ToolConfirmation{Hint: "approve"},
		},
	}}}, genai.RoleModel)
	if err := service.AppendEvent(ctx, created.Session, event); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	execution := newBareGoogleADKExecution("coverage-replay-run")
	execution.sessionService = service
	execution.appName = "coverage-replay"
	execution.sessionID = "coverage-replay-session"
	execution.agent = Agent{ID: "coverage-replay-agent"}
	execution.ensureCall(original.ID, ToolDescriptor{Name: original.Name}, original.Args)

	previous, inserted, err := runtime.Store().SaveApprovalIfConfirmationAbsent(ctx, Approval{
		ID: "coverage-replay-existing", RunID: execution.runID, AgentID: execution.agent.ID,
		ToolName: original.Name, Input: original.Args, Status: ApprovalStatusPending,
		FunctionCallID: original.ID, ConfirmationCallID: "coverage-replay-confirmation",
	})
	if err != nil || !inserted || previous.ID == "" {
		t.Fatalf("SaveApprovalIfConfirmationAbsent = %#v/%v/%v", previous, inserted, err)
	}

	approvals, err := execution.pendingApprovals(ctx, runtime.Store())
	if err != nil {
		t.Fatalf("pendingApprovals: %v", err)
	}
	if len(approvals) != 0 {
		t.Fatalf("replayed confirmation created approvals: %#v", approvals)
	}
	if call := execution.calls[0]; call.Status != "RUNNING" || call.RequiresUser {
		t.Fatalf("replayed confirmation mutated tracked call: %#v", call)
	}
}

func TestCoverage98GoogleExecutionEventGuardsPreserveProjectionState(t *testing.T) {
	t.Run("untracked confirmations and malformed identifiers are ignored", func(t *testing.T) {
		ctx := t.Context()
		service := adksession.InMemoryService()
		created, err := service.Create(ctx, &adksession.CreateRequest{
			AppName: "coverage-untracked", UserID: googleADKUserID, SessionID: "coverage-untracked-session",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		original := &genai.FunctionCall{ID: "untracked-tool", Name: "market.snapshot", Args: map[string]any{"symbol": "AAPL"}}
		event := adksession.NewEvent(ctx, "coverage-untracked-invocation")
		event.Content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
			ID: "untracked-confirmation", Name: toolconfirmation.FunctionCallName,
			Args: map[string]any{
				"originalFunctionCall": original,
				"toolConfirmation":     toolconfirmation.ToolConfirmation{Hint: "approve"},
			},
		}}}, genai.RoleModel)
		if err := service.AppendEvent(ctx, created.Session, event); err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}

		execution := newBareGoogleADKExecution("coverage-untracked-run")
		execution.sessionService = service
		execution.appName = "coverage-untracked"
		execution.sessionID = "coverage-untracked-session"
		approvals, err := execution.pendingApprovals(ctx, newTestRuntime(t).Store())
		if err != nil {
			t.Fatalf("pendingApprovals: %v", err)
		}
		if len(approvals) != 0 {
			t.Fatalf("untracked confirmation created approvals: %#v", approvals)
		}
		if !execution.hasApprovalForConfirmation("") {
			t.Fatal("blank confirmation id must be treated as non-actionable")
		}
		execution.markConfirmationProcessed(" \t ")
		if execution.processedConfirmationIDs != nil {
			t.Fatalf("blank confirmation id created replay state: %#v", execution.processedConfirmationIDs)
		}
	})

	t.Run("repeated tool events fill safe metadata without replacing input", func(t *testing.T) {
		execution := newBareGoogleADKExecution("coverage-duplicate-call")
		initial := execution.ensureCall("coverage-call", ToolDescriptor{}, map[string]any{"symbol": "AAPL"})
		replayed := execution.ensureCall("coverage-call", ToolDescriptor{Name: "market.snapshot", Permission: "read"}, map[string]any{"symbol": "MSFT"})
		if replayed.ID != initial.ID || replayed.ToolName != "market.snapshot" || replayed.Permission != "read" {
			t.Fatalf("replayed call metadata = %#v", replayed)
		}
		if got := replayed.Input["symbol"]; got != "AAPL" {
			t.Fatalf("replayed call replaced original input: %#v", replayed.Input)
		}
		execution.consumeFunctionResponse(nil)
		if err := execution.appendVisibleTextForRun(execution.runID, "", ""); err != nil {
			t.Fatalf("empty projected text: %v", err)
		}
		if builder := execution.builderForRun(nil, execution.runID); builder.Len() != 0 {
			t.Fatalf("nil builder store returned content: %q", builder.String())
		}
	})

	t.Run("terminal event returns a buffered projection delivery failure", func(t *testing.T) {
		execution := newBareGoogleADKExecution("coverage-buffered-delivery")
		execution.onDelta = func(ChatDelta) error { return errors.New("subscriber disconnected") }
		execution.builderForRun(execution.bufferedReplyByRunID, execution.runID).WriteString("final reply")
		event := adksession.NewEvent(t.Context(), "coverage-buffered-delivery")
		event.Content = genai.NewContentFromParts([]*genai.Part{}, genai.RoleModel)
		if err := execution.consumeEvent(event); err == nil || !strings.Contains(err.Error(), "subscriber disconnected") {
			t.Fatalf("consume terminal event error = %v, want projection delivery failure", err)
		}
	})
}
