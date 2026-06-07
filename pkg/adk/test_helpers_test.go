package adk

import (
	"context"
	"testing"
)

func mustCreateSession(t *testing.T, runtime *Runtime, agentID string, title string) Session {
	t.Helper()
	session, err := runtime.Store().CreateSession(context.Background(), agentID, title)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return session
}

func mustSaveRun(t *testing.T, runtime *Runtime, run Run) Run {
	t.Helper()
	if err := runtime.Store().SaveRun(context.Background(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	return run
}

func mustMessages(t *testing.T, runtime *Runtime, sessionID string) []Message {
	t.Helper()
	messages, err := runtime.Store().Messages(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	return messages
}

func mustAuditEvents(t *testing.T, runtime *Runtime) []AuditEvent {
	t.Helper()
	events, err := runtime.Store().ListAuditEvents(context.Background())
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	return events
}

func mustSaveProvider(t *testing.T, runtime *Runtime, req ProviderWriteRequest) Provider {
	t.Helper()
	provider, err := runtime.Store().SaveProvider(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	return provider
}

func mustSaveAgent(t *testing.T, runtime *Runtime, req AgentWriteRequest) Agent {
	t.Helper()
	agent, err := runtime.Store().SaveAgent(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	return agent
}
