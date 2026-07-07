package adk

import (
	"errors"
	"strings"
	"testing"
	"time"

	adksession "google.golang.org/adk/v2/session"
)

func TestCompactingSessionServiceAdditionalCoverageBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("auto compact helper and event filter cover nil and slice branches", func(t *testing.T) {
		compacted, err := (*compactingSessionService)(nil).autoCompactForModelContext(ctx, Session{ID: "session"}, Agent{})
		if err != nil || compacted {
			t.Fatalf("nil autoCompactForModelContext = %v/%v, want false/nil", compacted, err)
		}
		compacted, err = (&compactingSessionService{}).autoCompactForModelContext(ctx, Session{ID: "session"}, Agent{})
		if err != nil || compacted {
			t.Fatalf("empty autoCompactForModelContext = %v/%v, want false/nil", compacted, err)
		}

		now := time.Now().UTC()
		filtered := filterEvents([]*adksession.Event{
			{ID: "old", Timestamp: now.Add(-2 * time.Minute)},
			nil,
			{ID: "mid", Timestamp: now.Add(-time.Minute)},
			{ID: "new", Timestamp: now},
		}, now.Add(-90*time.Second), 1)
		if len(filtered) != 1 || filtered[0].ID != "new" {
			t.Fatalf("filterEvents = %#v, want newest single event", filtered)
		}
	})

	t.Run("Get logs compaction errors and surfaces store lookup failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "session-wrap-agent", Name: "Session Wrap Agent", Status: AgentStatusEnabled,
			ProviderID: testProviderID, RecentUserWindow: 1,
		})
		session, _ := mustCreateSessionWithRaw(t, runtime, agent.ID, "session wrap auto compact")
		service := &compactingSessionService{base: runtime.rawSessionService, manager: runtime.contextManager}
		runtime.contextManager.rawService = &sessionContextGetErrorService{Service: runtime.rawSessionService, err: errors.New("raw unavailable")}
		response, err := service.Get(ctx, &adksession.GetRequest{
			AppName: googleADKAppName(agent.ID), UserID: googleADKUserID, SessionID: session.ID,
		})
		if err != nil || response == nil || response.Session == nil {
			t.Fatalf("Get with compact error response=%#v err=%v, want delegated response", response, err)
		}

		agentRuntime := newTestRuntime(t)
		agentDef := mustSaveAgent(t, agentRuntime, AgentWriteRequest{
			ID: "session-wrap-agent-lookup", Name: "Session Wrap Agent Lookup", Status: AgentStatusEnabled, ProviderID: testProviderID,
		})
		session, _ = mustCreateSessionWithRaw(t, agentRuntime, agentDef.ID, "session wrap agent lookup")
		if _, err := agentRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableAgents); err != nil {
			t.Fatalf("drop agents: %v", err)
		}
		_, err = (&compactingSessionService{base: agentRuntime.rawSessionService, manager: agentRuntime.contextManager}).Get(ctx, &adksession.GetRequest{
			AppName: googleADKAppName(agentDef.ID), UserID: googleADKUserID, SessionID: session.ID,
		})
		if err == nil || !strings.Contains(err.Error(), tableAgents) {
			t.Fatalf("Get agent lookup err = %v, want %s failure", err, tableAgents)
		}

		stateRuntime := newTestRuntime(t)
		agentDef = mustSaveAgent(t, stateRuntime, AgentWriteRequest{
			ID: "session-wrap-state", Name: "Session Wrap State", Status: AgentStatusEnabled, ProviderID: testProviderID,
		})
		session, _ = mustCreateSessionWithRaw(t, stateRuntime, agentDef.ID, "session wrap state")
		if _, err := stateRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableSessionContextLive); err != nil {
			t.Fatalf("drop session context: %v", err)
		}
		_, err = (&compactingSessionService{base: stateRuntime.rawSessionService, manager: stateRuntime.contextManager}).Get(ctx, &adksession.GetRequest{
			AppName: googleADKAppName(agentDef.ID), UserID: googleADKUserID, SessionID: session.ID,
		})
		if err == nil || !strings.Contains(err.Error(), tableSessionContextLive) {
			t.Fatalf("Get session context err = %v, want %s failure", err, tableSessionContextLive)
		}

		handoffRuntime := newTestRuntime(t)
		agentDef = mustSaveAgent(t, handoffRuntime, AgentWriteRequest{
			ID: "session-wrap-handoff", Name: "Session Wrap Handoff", Status: AgentStatusEnabled, ProviderID: testProviderID,
		})
		session, _ = mustCreateSessionWithRaw(t, handoffRuntime, agentDef.ID, "session wrap handoff")
		if _, err := handoffRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableHandoffSegments); err != nil {
			t.Fatalf("drop handoff segments: %v", err)
		}
		_, err = (&compactingSessionService{base: handoffRuntime.rawSessionService, manager: handoffRuntime.contextManager}).Get(ctx, &adksession.GetRequest{
			AppName: googleADKAppName(agentDef.ID), UserID: googleADKUserID, SessionID: session.ID,
		})
		if err == nil || !strings.Contains(err.Error(), tableHandoffSegments) {
			t.Fatalf("Get handoff err = %v, want %s failure", err, tableHandoffSegments)
		}
	})
}
