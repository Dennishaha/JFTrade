package assistant

import (
	"testing"
	"time"

	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestServiceRecoverTerminalChatResponseFallsBackToLatestAssistant(t *testing.T) {
	runtime, service, sessionService := newAssistantServiceHarness(t)
	ctx := t.Context()

	agent, err := runtime.Store().SaveAgent(ctx, assistantmodel.AgentWriteRequest{
		ID: "agent-recover-latest", Name: "Recover Latest Agent", Status: assistantmodel.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Recover Latest Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	appendAssistantSessionEvent(t, sessionService, agent.ID, session.ID, newUserSessionEvent("run-recover-latest", "继续恢复", time.Unix(20, 0)))
	appendAssistantSessionEvent(t, sessionService, agent.ID, session.ID, newAssistantSessionEvent("run-recover-latest", "assistant-latest", "最新终态答复", "最新推理", time.Unix(22, 0)))

	completed := assistantmodel.Run{
		ID:             "run-recover-latest",
		SessionID:      session.ID,
		AgentID:        agent.ID,
		Status:         assistantmodel.RunStatusCompleted,
		FinalMessageID: "assistant-missing-after-append-failure",
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, completed); err != nil {
		t.Fatalf("SaveRun completed: %v", err)
	}

	response, err := service.RecoverTerminalChatResponse(ctx, completed.ID)
	if err != nil {
		t.Fatalf("RecoverTerminalChatResponse: %v", err)
	}
	if response == nil {
		t.Fatal("RecoverTerminalChatResponse = nil")
		return
	}
	if response.Reply != "最新终态答复" || response.ReasoningContent != "最新推理" {
		t.Fatalf("recovered reply=%q reasoning=%q, want latest assistant fallback", response.Reply, response.ReasoningContent)
	}
}
