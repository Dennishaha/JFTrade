package adk

import (
	"strings"
	"testing"
	"time"
)

func TestContinuationOnlyMessageRecognition(t *testing.T) {
	for _, message := range []string{"继续", " 继续\n", "continue", " CONTINUE ", "go on", "go   on"} {
		if !isContinuationOnlyMessage(message) {
			t.Errorf("isContinuationOnlyMessage(%q)=false", message)
		}
	}
	for _, message := range []string{"继续分析", "continue please", "go on with the plan", "go"} {
		if isContinuationOnlyMessage(message) {
			t.Errorf("isContinuationOnlyMessage(%q)=true", message)
		}
	}
}

func TestChatAuditsContinuationOnlyMessageAgainstRecentCompletedRun(t *testing.T) {
	runtime := newTestRuntime(t)
	first, err := runtime.Chat(t.Context(), ChatRequest{Message: "Summarize the current state."})
	if err != nil || first.Run.Status != RunStatusCompleted {
		t.Fatalf("first Chat response=%+v err=%v", first, err)
	}
	second, err := runtime.Chat(t.Context(), ChatRequest{SessionID: first.Session.ID, Message: "  GO   ON  "})
	if err != nil || second.Run.Status != RunStatusCompleted {
		t.Fatalf("second Chat response=%+v err=%v", second, err)
	}
	found := false
	for _, event := range mustAuditEvents(t, runtime) {
		if event.Kind != "run.continuation_only" || event.SubjectID != second.Run.ID {
			continue
		}
		found = true
		if event.Metadata["previousRunId"] != first.Run.ID || event.Metadata["sessionId"] != first.Session.ID {
			t.Fatalf("continuation audit=%+v", event)
		}
		for _, value := range event.Metadata {
			if text, ok := value.(string); ok && strings.EqualFold(strings.TrimSpace(text), "go on") {
				t.Fatalf("continuation audit leaked the raw message: %+v", event.Metadata)
			}
		}
	}
	if !found {
		t.Fatal("run.continuation_only audit event not found")
	}
}

func TestRecentContinuationSignalRequiresFreshCompletedRunInSameSession(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		completed time.Time
		want      bool
	}{
		{name: "fresh completed", status: RunStatusCompleted, completed: time.Now().Add(-9 * time.Minute), want: true},
		{name: "old completed", status: RunStatusCompleted, completed: time.Now().Add(-11 * time.Minute)},
		{name: "not completed", status: RunStatusRunning, completed: time.Now().Add(-time.Minute)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := newTestRuntime(t)
			session := mustCreateSession(t, runtime, DefaultBuiltinAgentID, test.name)
			completedAt := test.completed.UTC().Format(time.RFC3339Nano)
			mustSaveRun(t, runtime, Run{
				ID: "previous-run", SessionID: session.ID, AgentID: DefaultBuiltinAgentID,
				Status: test.status, CompletedAt: &completedAt, CreatedAt: nowString(), UpdatedAt: nowString(),
			})
			previousRunID, _ := runtime.recentContinuationSignal(t.Context(), session.ID, "continue")
			if got := previousRunID != ""; got != test.want {
				t.Fatalf("previousRunID=%q, want present=%v", previousRunID, test.want)
			}
			if other, _ := runtime.recentContinuationSignal(t.Context(), "different-session", "continue"); other != "" {
				t.Fatalf("different session signal=%q", other)
			}
		})
	}
}
