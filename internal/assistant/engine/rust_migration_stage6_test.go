package adk

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

type rustMigrationStage6Fixture struct {
	Statuses        []string `json:"statuses"`
	CompletionInput struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	} `json:"completionInput"`
	Approval struct {
		RunID    string   `json:"runId"`
		Approved bool     `json:"approved"`
		Approval Approval `json:"approval"`
		ToolCall ToolCall `json:"toolCall"`
	} `json:"approval"`
	Input struct {
		RunID          string          `json:"runId"`
		RequestID      string          `json:"requestId"`
		FunctionCallID string          `json:"functionCallId"`
		Draft          json.RawMessage `json:"draft"`
		Answers        []InputAnswer   `json:"answers"`
	} `json:"input"`
	InvalidInputs []json.RawMessage `json:"invalidInputs"`
	WorkflowTasks []Task            `json:"workflowTasks"`
	Claims        struct {
		RunID       string                       `json:"runId"`
		FirstOwner  string                       `json:"firstOwner"`
		SecondOwner string                       `json:"secondOwner"`
		ThirdOwner  string                       `json:"thirdOwner"`
		FourthOwner string                       `json:"fourthOwner"`
		StartUnixMs int64                        `json:"startUnixMs"`
		TTLMillis   int64                        `json:"ttlMs"`
		ReplaySafe  rustMigrationStage6ClaimSeed `json:"replaySafe"`
		FailClosed  rustMigrationStage6ClaimSeed `json:"failClosed"`
	} `json:"claims"`
}

type rustMigrationStage6ClaimSeed struct {
	IdempotencyKey string         `json:"idempotencyKey"`
	ToolName       string         `json:"toolName"`
	Input          map[string]any `json:"input"`
}

func TestRustMigrationStage6AssistantContractMatchesCorpus(t *testing.T) {
	fixture := loadRustMigrationStage6Fixture(t)
	wantStatuses := []string{
		RunStatusRunning, RunStatusCompleted, RunStatusPending, RunStatusPendingInput,
		RunStatusFailed, RunStatusDenied, RunStatusCancelled, RunStatusTimedOut, RunStatusPaused,
	}
	if !reflect.DeepEqual(fixture.Statuses, wantStatuses) {
		t.Fatalf("run statuses = %#v, want %#v", fixture.Statuses, wantStatuses)
	}
	if len(fixture.CompletionInput.Tools) != 1 || fixture.CompletionInput.Tools[0].Name != interactionRequestUserTool {
		t.Fatalf("Rig tool fixture = %#v", fixture.CompletionInput.Tools)
	}
	assertCanonicalJSONEqual(t, fixture.CompletionInput.Tools[0].InputSchema, inputRequestToolInputSchema())

	var valid requestUserToolArgs
	if err := json.Unmarshal(fixture.Input.Draft, &valid); err != nil {
		t.Fatalf("decode valid input request: %v", err)
	}
	request, err := buildInputRequest(fixture.Input.RunID, "agent-stage6", fixture.Input.FunctionCallID, valid)
	if err != nil || request.Status != InputRequestStatusPending || len(request.Questions) != 1 {
		t.Fatalf("build valid input request = %+v, err=%v", request, err)
	}
	for index, raw := range fixture.InvalidInputs {
		var invalid requestUserToolArgs
		if err := json.Unmarshal(raw, &invalid); err != nil {
			t.Fatalf("decode invalid input request %d: %v", index, err)
		}
		if _, err := buildInputRequest("invalid", "agent", "call", invalid); !errors.Is(err, errInputRequestInvalid) {
			t.Fatalf("invalid input request %d error = %v", index, err)
		}
	}

	verifyRustMigrationStage6ApprovalPersistence(t, fixture)
	verifyRustMigrationStage6InputPersistence(t, fixture, request)
	verifyRustMigrationStage6Workflow(t, fixture.WorkflowTasks)
	verifyRustMigrationStage6ExecutionClaims(t, fixture)
}

func verifyRustMigrationStage6ApprovalPersistence(t *testing.T, fixture rustMigrationStage6Fixture) {
	t.Helper()
	ctx := context.Background()
	store := newTestRuntime(t).Store()
	approval := fixture.Approval.Approval
	toolCall := fixture.Approval.ToolCall
	run := Run{
		ID: fixture.Approval.RunID, SessionID: "session-stage6", AgentID: approval.AgentID,
		Status: RunStatusPending, ResumeState: "waiting_approval", PendingApprovals: []Approval{approval},
		ToolCalls: []ToolCall{toolCall}, CreatedAt: approval.CreatedAt, StartedAt: approval.CreatedAt,
		UpdatedAt: approval.UpdatedAt, Usage: &RunUsage{},
	}
	if err := store.SaveRun(ctx, run); err != nil {
		t.Fatalf("save approval run: %v", err)
	}
	if err := store.SaveApproval(ctx, approval); err != nil {
		t.Fatalf("save approval: %v", err)
	}
	status := ApprovalStatusDenied
	if fixture.Approval.Approved {
		status = ApprovalStatusApproved
	}
	resolved, changed, staged, shouldContinue, err := store.ResolveAndStageApproval(ctx, approval.ID, status)
	if err != nil || !changed || !shouldContinue || staged == nil {
		t.Fatalf("first approval resolution = %+v/%v/%+v/%v/%v", resolved, changed, staged, shouldContinue, err)
	}
	if staged.Status != RunStatusRunning || staged.PendingApprovals[0].Status != status || staged.ToolCalls[0].RequiresUser {
		t.Fatalf("staged approval run = %+v", staged)
	}
	_, changed, staged, shouldContinue, err = store.ResolveAndStageApproval(ctx, approval.ID, status)
	if err != nil || changed || staged != nil || shouldContinue {
		t.Fatalf("replayed approval resolution = changed=%v staged=%+v continue=%v err=%v", changed, staged, shouldContinue, err)
	}
}

func verifyRustMigrationStage6InputPersistence(t *testing.T, fixture rustMigrationStage6Fixture, request *InputRequest) {
	t.Helper()
	ctx := context.Background()
	store := newTestRuntime(t).Store()
	request.ID = fixture.Input.RequestID
	run := Run{
		ID: fixture.Input.RunID, SessionID: "session-stage6", AgentID: request.AgentID,
		Status: RunStatusPendingInput, ResumeState: "waiting_input", InputRequest: request,
		InputRequests: []InputRequest{*request}, CreatedAt: request.CreatedAt, StartedAt: request.CreatedAt,
		UpdatedAt: request.UpdatedAt, Usage: &RunUsage{},
	}
	if err := store.SaveRun(ctx, run); err != nil {
		t.Fatalf("save input run: %v", err)
	}
	payload := InputResponseRequest{RequestID: request.ID, Answers: fixture.Input.Answers}
	resolved, changed, err := store.ResolveRunInput(ctx, run.ID, payload)
	if err != nil || !changed || resolved.Status != RunStatusRunning || resolved.InputRequest == nil || resolved.InputRequest.Status != InputRequestStatusAnswered {
		t.Fatalf("first input resolution = %+v changed=%v err=%v", resolved, changed, err)
	}
	resolved, changed, err = store.ResolveRunInput(ctx, run.ID, payload)
	if err != nil || changed || resolved.InputRequest == nil || resolved.InputRequest.Status != InputRequestStatusAnswered {
		t.Fatalf("replayed input resolution = %+v changed=%v err=%v", resolved, changed, err)
	}
}

func verifyRustMigrationStage6Workflow(t *testing.T, tasks []Task) {
	t.Helper()
	ready := assistantmodel.ExecutableWorkflowTasks(tasks, "")
	if len(ready) != 1 || ready[0].ID != "inspect" {
		t.Fatalf("first ready tasks = %+v", ready)
	}
	for index := range tasks {
		if tasks[index].ID == "inspect" {
			tasks[index].Status = "DONE"
		}
	}
	ready = assistantmodel.ExecutableWorkflowTasks(tasks, "")
	if len(ready) != 1 || ready[0].ID != "analyze" {
		t.Fatalf("second ready tasks = %+v", ready)
	}
	for _, status := range []string{"TODO", "IN_PROGRESS", "BLOCKED", "DONE", "CANCELLED"} {
		if normalized, err := assistantmodel.NormalizeTaskStatus(status); err != nil || normalized != status {
			t.Fatalf("NormalizeTaskStatus(%q) = %q, %v", status, normalized, err)
		}
	}
}

func verifyRustMigrationStage6ExecutionClaims(t *testing.T, fixture rustMigrationStage6Fixture) {
	t.Helper()
	ctx := context.Background()
	store := newTestRuntime(t).Store()
	claims := fixture.Claims
	ttl := time.Duration(claims.TTLMillis) * time.Millisecond
	firstAt := time.UnixMilli(claims.StartUnixMs).UTC()
	first, err := store.ClaimRunLease(ctx, claims.RunID, claims.FirstOwner, firstAt, ttl)
	if err != nil || first.FencingToken != 1 {
		t.Fatalf("first run lease = %+v, %v", first, err)
	}
	if _, err := store.ClaimRunLease(ctx, claims.RunID, claims.SecondOwner, firstAt.Add(time.Millisecond), ttl); !errors.Is(err, enginepersistence.ErrRunLeaseHeld) {
		t.Fatalf("held run lease error = %v", err)
	}
	takeoverAt := firstAt.Add(ttl + time.Millisecond)
	second, err := store.ClaimRunLease(ctx, claims.RunID, claims.SecondOwner, takeoverAt, ttl)
	if err != nil || second.FencingToken != 2 {
		t.Fatalf("second run lease = %+v, %v", second, err)
	}
	firstTicket, err := store.ClaimToolInvocation(ctx, rustMigrationStage6ToolClaim(claims.ReplaySafe, second, enginepersistence.ToolIdempotencyReplaySafe, takeoverAt, ttl))
	if err != nil || !firstTicket.Execute || firstTicket.FencingToken != 1 {
		t.Fatalf("first tool claim = %+v, %v", firstTicket, err)
	}
	if _, err := store.ClaimToolInvocation(ctx, rustMigrationStage6ToolClaim(claims.ReplaySafe, second, enginepersistence.ToolIdempotencyReplaySafe, takeoverAt.Add(time.Millisecond), ttl)); !errors.Is(err, enginepersistence.ErrToolInvocationInFlight) {
		t.Fatalf("in-flight tool error = %v", err)
	}
	replayTakeoverAt := takeoverAt.Add(ttl + time.Millisecond)
	third, err := store.ClaimRunLease(ctx, claims.RunID, claims.ThirdOwner, replayTakeoverAt, ttl)
	if err != nil || third.FencingToken != 3 {
		t.Fatalf("third run lease = %+v, %v", third, err)
	}
	replayTicket, err := store.ClaimToolInvocation(ctx, rustMigrationStage6ToolClaim(claims.ReplaySafe, third, enginepersistence.ToolIdempotencyReplaySafe, replayTakeoverAt, ttl))
	if err != nil || replayTicket.FencingToken != 2 {
		t.Fatalf("replay-safe takeover = %+v, %v", replayTicket, err)
	}
	if err := store.CompleteToolInvocation(ctx, replayTicket, map[string]any{"price": "100.00"}, replayTakeoverAt.Add(time.Millisecond)); err != nil {
		t.Fatalf("complete replay-safe claim: %v", err)
	}
	replayed, err := store.ClaimToolInvocation(ctx, rustMigrationStage6ToolClaim(claims.ReplaySafe, third, enginepersistence.ToolIdempotencyReplaySafe, replayTakeoverAt.Add(2*time.Millisecond), ttl))
	if err != nil || !replayed.Replayed || replayed.Execute {
		t.Fatalf("completed claim replay = %+v, %v", replayed, err)
	}
	if _, err := store.ClaimToolInvocation(ctx, rustMigrationStage6ToolClaim(claims.FailClosed, third, enginepersistence.ToolIdempotencyFailClosed, replayTakeoverAt.Add(2*time.Millisecond), ttl)); err != nil {
		t.Fatalf("first fail-closed claim: %v", err)
	}
	failClosedAt := replayTakeoverAt.Add(ttl + 3*time.Millisecond)
	fourth, err := store.ClaimRunLease(ctx, claims.RunID, claims.FourthOwner, failClosedAt, ttl)
	if err != nil || fourth.FencingToken != 4 {
		t.Fatalf("fourth run lease = %+v, %v", fourth, err)
	}
	if _, err := store.ClaimToolInvocation(ctx, rustMigrationStage6ToolClaim(claims.FailClosed, fourth, enginepersistence.ToolIdempotencyFailClosed, failClosedAt, ttl)); !errors.Is(err, enginepersistence.ErrToolOutcomeUnknown) {
		t.Fatalf("fail-closed stale claim error = %v", err)
	}
}

func rustMigrationStage6ToolClaim(seed rustMigrationStage6ClaimSeed, lease enginepersistence.RunLease, mode string, now time.Time, ttl time.Duration) enginepersistence.ToolInvocationClaim {
	return enginepersistence.ToolInvocationClaim{
		RunID: lease.RunID, IdempotencyKey: seed.IdempotencyKey, ToolName: seed.ToolName,
		OwnerID: lease.OwnerID, RunLeaseToken: lease.FencingToken, Input: seed.Input,
		Mode: mode, Now: now, TTL: ttl,
	}
}

func loadRustMigrationStage6Fixture(t *testing.T) rustMigrationStage6Fixture {
	t.Helper()
	fixtureRoot := os.Getenv("JFTRADE_STAGE6_FIXTURE_ROOT")
	if fixtureRoot == "" {
		_, currentFile, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("resolve current test file")
		}
		fixtureRoot = filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "tests", "fixtures", "rust-migration", "stage6")
	}
	path := filepath.Join(fixtureRoot, "assistant-rig-corpus.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Stage 6 fixture: %v", err)
	}
	var fixture rustMigrationStage6Fixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode Stage 6 fixture: %v", err)
	}
	return fixture
}

func assertCanonicalJSONEqual(t *testing.T, left, right any) {
	t.Helper()
	canonical := func(value any) any {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal canonical JSON: %v", err)
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("decode canonical JSON: %v", err)
		}
		return decoded
	}
	if !reflect.DeepEqual(canonical(left), canonical(right)) {
		t.Fatalf("JSON mismatch:\nleft=%#v\nright=%#v", left, right)
	}
}
