package adk

import (
	"context"
	"errors"
	"strings"
	"testing"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

type sessionContextGetErrorService struct {
	adksession.Service
	err error
}

func (s *sessionContextGetErrorService) Get(context.Context, *adksession.GetRequest) (*adksession.GetResponse, error) {
	return nil, s.err
}

type sessionContextRefreshEdgeService struct {
	adksession.Service
	getErr      error
	nilResponse bool
	appendCalls int
	getCalls    int
}

func (s *sessionContextRefreshEdgeService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	s.getCalls++
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.nilResponse {
		return &adksession.GetResponse{}, nil
	}
	return s.Service.Get(ctx, req)
}

func (s *sessionContextRefreshEdgeService) AppendEvent(context.Context, adksession.Session, *adksession.Event) error {
	s.appendCalls++
	return errors.New("stale session error")
}

type sessionContextNthGetService struct {
	adksession.Service
	failAt      int
	err         error
	nilResponse bool
	getCalls    int
}

func (s *sessionContextNthGetService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	s.getCalls++
	if s.failAt > 0 && s.getCalls == s.failAt {
		if s.err != nil {
			return nil, s.err
		}
		if s.nilResponse {
			return &adksession.GetResponse{}, nil
		}
	}
	return s.Service.Get(ctx, req)
}

func mustCreateSessionWithRaw(t *testing.T, runtime *Runtime, agentID string, title string) (Session, adksession.Session) {
	t.Helper()
	session := mustCreateSession(t, runtime, agentID, title)
	created, err := runtime.rawSessionService.Create(t.Context(), &adksession.CreateRequest{
		AppName:   googleADKAppName(agentID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	return session, created.Session
}

func installFailTrigger(t *testing.T, runtime *Runtime, name string, tableName string, op string, message string) {
	t.Helper()
	sql := `CREATE TRIGGER ` + name + ` BEFORE ` + op + ` ON ` + tableName + ` BEGIN SELECT RAISE(FAIL, '` + message + `'); END`
	if _, err := runtime.Store().db.ExecContext(t.Context(), sql); err != nil {
		t.Fatalf("create trigger %s: %v", name, err)
	}
}

func TestSessionContextAppendRetryAdditionalBranches(t *testing.T) {
	ctx := t.Context()
	base := adksession.InMemoryService()
	created, err := base.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "append-edges",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	event := newContextTextEvent("append-edge", "hello", genai.RoleUser)

	if err := appendADKEventWithStaleRetry(ctx, newADKSessionAppendLockMap(), nil, created.Session, event); err == nil || !strings.Contains(err.Error(), "service is unavailable") {
		t.Fatalf("nil service err = %v", err)
	}
	if err := appendADKEventWithStaleRetry(ctx, newADKSessionAppendLockMap(), base, nil, event); err == nil || !strings.Contains(err.Error(), "session is unavailable") {
		t.Fatalf("nil session err = %v", err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := appendADKEventWithStaleRetry(cancelled, newADKSessionAppendLockMap(), base, created.Session, event); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled append err = %v, want context.Canceled", err)
	}

	projected := &wrappedSession{base: created.Session, events: &wrappedEvents{}}
	refreshErrService := &sessionContextRefreshEdgeService{Service: base, getErr: errors.New("refresh unavailable")}
	if err := appendADKEventWithStaleRetry(ctx, newADKSessionAppendLockMap(), refreshErrService, projected, event); err == nil || !strings.Contains(err.Error(), "refresh unavailable") {
		t.Fatalf("synthetic refresh err = %v, want refresh failure", err)
	}

	nilSessionService := &sessionContextRefreshEdgeService{Service: base, nilResponse: true}
	if err := appendADKEventWithStaleRetry(ctx, newADKSessionAppendLockMap(), nilSessionService, projected, event); err == nil || !strings.Contains(err.Error(), "is unavailable") {
		t.Fatalf("synthetic nil session err = %v, want unavailable", err)
	}

	refreshFailService := &sessionContextRefreshEdgeService{Service: base, getErr: errors.New("second refresh failed")}
	if err := appendADKEventWithStaleRetry(ctx, newADKSessionAppendLockMap(), refreshFailService, created.Session, event); err == nil || !strings.Contains(err.Error(), "stale session error") {
		t.Fatalf("refresh after stale err = %v, want original stale append error", err)
	}

	refreshNilService := &sessionContextRefreshEdgeService{Service: base, nilResponse: true}
	if err := appendADKEventWithStaleRetry(ctx, newADKSessionAppendLockMap(), refreshNilService, created.Session, event); err == nil || !strings.Contains(err.Error(), "stale session error") {
		t.Fatalf("refresh nil session err = %v, want stale append error", err)
	}

	staleForever := &sessionContextRefreshEdgeService{Service: base}
	if err := appendADKEventWithStaleRetry(ctx, newADKSessionAppendLockMap(), staleForever, created.Session, event); err == nil || !strings.Contains(err.Error(), "stale session error") {
		t.Fatalf("stale forever err = %v, want stale append error", err)
	}
	if staleForever.appendCalls != adkSessionAppendMaxAttempts || staleForever.getCalls != adkSessionAppendMaxAttempts {
		t.Fatalf("retry counts append=%d get=%d, want %d each", staleForever.appendCalls, staleForever.getCalls, adkSessionAppendMaxAttempts)
	}
}

func TestSessionContextManagerAdditionalBoundaryBranches(t *testing.T) {
	ctx := t.Context()

	if mode, ok := (&SessionContextManager{}).ShouldAutoCompact(SessionContextSnapshot{ProjectedNextTurnTokens: 85, ContextWindowTokens: 100}); !ok || mode != "normal" {
		t.Fatalf("ShouldAutoCompact normal = %q/%v", mode, ok)
	}
	if got, degraded := (&SessionContextManager{}).mergeSummary(ctx, Agent{}, " ", " existing summary ", "normal"); got != "existing summary" || degraded {
		t.Fatalf("mergeSummary blank deterministic = %q/%v", got, degraded)
	}
	if err := (*SessionContextManager)(nil).syncHandoffStateForSession(ctx, Session{}); err != nil {
		t.Fatalf("nil syncHandoffStateForSession: %v", err)
	}
	if err := (&SessionContextManager{}).syncHandoffStateForSessionID(ctx, "missing", nil); err != nil {
		t.Fatalf("empty manager syncHandoffStateForSessionID: %v", err)
	}
	if err := (&SessionContextManager{}).syncHandoffState(ctx, Session{}, nil); err != nil {
		t.Fatalf("empty manager syncHandoffState: %v", err)
	}
	if got := maxInt(9, 3); got != 9 {
		t.Fatalf("maxInt high-first = %d, want 9", got)
	}
	wrapped := &wrappedEvents{items: []*adksession.Event{{ID: "first"}, {ID: "second"}}}
	seen := 0
	for range wrapped.All() {
		seen++
		break
	}
	if seen != 1 {
		t.Fatalf("wrappedEvents early stop seen = %d, want 1", seen)
	}

	t.Run("auto compact short-circuits when no new compaction boundary exists", func(t *testing.T) {
		runtime := newTestRuntime(t)
		provider := mustSaveProvider(t, runtime, ProviderWriteRequest{
			ID: "tiny-context-provider-no-boundary", BaseURL: "https://example.test/v1", Model: "model", Enabled: true, ContextWindowTokens: 1,
		})
		session, _ := mustCreateSessionWithRaw(t, runtime, "auto-compact-no-boundary", "auto compact no boundary")
		snapshot, compacted, err := runtime.contextManager.AutoCompactForModelContext(ctx, session, Agent{
			ID: session.AgentID, ProviderID: provider.ID, RecentUserWindow: 1,
		}, strings.Repeat("pending text ", 200))
		if err != nil {
			t.Fatalf("AutoCompactForModelContext no boundary: %v", err)
		}
		if compacted {
			t.Fatal("AutoCompactForModelContext compacted without new boundary")
		}
		if snapshot.ProjectedNextTurnTokens == 0 || snapshot.ContextWindowTokens == 0 {
			t.Fatalf("snapshot = %+v, want pending user projection", snapshot)
		}
	})

	t.Run("auto compact surfaces second raw session failure", func(t *testing.T) {
		runtime := newTestRuntime(t)
		provider := mustSaveProvider(t, runtime, ProviderWriteRequest{
			ID: "tiny-context-provider-second-get", BaseURL: "https://example.test/v1", Model: "model", Enabled: true, ContextWindowTokens: 1,
		})
		session, _ := mustCreateSessionWithRaw(t, runtime, "auto-compact-second-get", "auto compact second get")
		runtime.contextManager.rawService = &sessionContextNthGetService{
			Service: runtime.rawSessionService,
			failAt:  3,
			err:     errors.New("second raw get failed"),
		}
		if _, compacted, err := runtime.contextManager.AutoCompactForModelContext(ctx, session, Agent{
			ID: session.AgentID, ProviderID: provider.ID, RecentUserWindow: 1,
		}, strings.Repeat("pending text ", 200)); err == nil || compacted || !strings.Contains(err.Error(), "second raw get failed") {
			t.Fatalf("AutoCompactForModelContext second get compacted=%v err=%v", compacted, err)
		}
	})

	t.Run("auto compact propagates compaction failure", func(t *testing.T) {
		runtime := newTestRuntime(t)
		provider := mustSaveProvider(t, runtime, ProviderWriteRequest{
			ID: "tiny-context-provider-compact-fail", BaseURL: "https://example.test/v1", Model: "model", Enabled: true, ContextWindowTokens: 1,
		})
		session, raw := mustCreateSessionWithRaw(t, runtime, "auto-compact-compact-fail", "auto compact compact fail")
		appendContextEvents(t, runtime.rawSessionService, raw, 0, 5)
		installFailTrigger(t, runtime, "fail_handoff_insert_auto", tableHandoffSegments, "INSERT", "auto compact handoff failed")
		if _, compacted, err := runtime.contextManager.AutoCompactForModelContext(ctx, session, Agent{
			ID: session.AgentID, ProviderID: provider.ID, RecentUserWindow: 1,
		}, strings.Repeat("pending text ", 200)); err == nil || compacted || !strings.Contains(err.Error(), "auto compact handoff failed") {
			t.Fatalf("AutoCompactForModelContext compact failure compacted=%v err=%v", compacted, err)
		}
	})

	t.Run("snapshot compact and suffix surface raw and store failures", func(t *testing.T) {
		t.Run("raw session errors", func(t *testing.T) {
			runtime := newTestRuntime(t)
			session := mustCreateSession(t, runtime, "snapshot-raw-error", "snapshot raw error")
			agent := Agent{ID: session.AgentID}
			runtime.contextManager.rawService = &sessionContextGetErrorService{Service: runtime.rawSessionService, err: errors.New("raw unavailable")}
			if _, err := runtime.contextManager.Snapshot(ctx, session, agent); err == nil || !strings.Contains(err.Error(), "raw unavailable") {
				t.Fatalf("Snapshot raw err = %v", err)
			}
			if _, err := runtime.contextManager.Compact(ctx, session, agent, SessionCompactRequest{}); err == nil || !strings.Contains(err.Error(), "raw unavailable") {
				t.Fatalf("Compact raw err = %v", err)
			}
			if _, err := runtime.contextManager.canAdvanceAutoCompaction(ctx, session, agent); err == nil || !strings.Contains(err.Error(), "raw unavailable") {
				t.Fatalf("canAdvanceAutoCompaction err = %v", err)
			}
			if err := runtime.contextManager.syncHandoffState(ctx, session, nil); err == nil || !strings.Contains(err.Error(), "raw unavailable") {
				t.Fatalf("syncHandoffState raw err = %v", err)
			}
			if _, err := runtime.contextManager.rawSession(ctx, session.AgentID, session.ID); err == nil || !strings.Contains(err.Error(), "raw unavailable") {
				t.Fatalf("rawSession err = %v", err)
			}
		})

		t.Run("session context table failures", func(t *testing.T) {
			runtime := newTestRuntime(t)
			session := mustCreateSession(t, runtime, "snapshot-state-error", "snapshot state error")
			agent := Agent{ID: session.AgentID}
			if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableSessionContextLive); err != nil {
				t.Fatalf("drop session context table: %v", err)
			}
			if _, err := runtime.contextManager.Snapshot(ctx, session, agent); err == nil {
				t.Fatal("Snapshot with dropped session context table err = nil")
			}
			if _, err := runtime.contextManager.Compact(ctx, session, agent, SessionCompactRequest{}); err == nil {
				t.Fatal("Compact with dropped session context table err = nil")
			}
			if _, err := runtime.contextManager.InstructionSuffix(ctx, session.ID); err == nil {
				t.Fatal("InstructionSuffix with dropped session context table err = nil")
			}
			if err := runtime.contextManager.syncHandoffStateForSession(ctx, session); err == nil {
				t.Fatal("syncHandoffStateForSession with dropped session context table err = nil")
			}
		})

		t.Run("handoff table failures", func(t *testing.T) {
			runtime := newTestRuntime(t)
			session := mustCreateSession(t, runtime, "snapshot-handoff-error", "snapshot handoff error")
			agent := Agent{ID: session.AgentID}
			if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableHandoffSegments); err != nil {
				t.Fatalf("drop handoff table: %v", err)
			}
			if _, err := runtime.contextManager.Snapshot(ctx, session, agent); err == nil {
				t.Fatal("Snapshot with dropped handoff table err = nil")
			}
			if _, err := runtime.contextManager.Compact(ctx, session, agent, SessionCompactRequest{}); err == nil {
				t.Fatal("Compact with dropped handoff table err = nil")
			}
			if _, err := runtime.contextManager.InstructionSuffix(ctx, session.ID); err == nil {
				t.Fatal("InstructionSuffix with dropped handoff table err = nil")
			}
			if err := runtime.contextManager.syncHandoffStateForSession(ctx, session); err == nil {
				t.Fatal("syncHandoffStateForSession with dropped handoff table err = nil")
			}
		})

		t.Run("snapshot and compact write failures", func(t *testing.T) {
			t.Run("snapshot save context failure", func(t *testing.T) {
				runtime := newTestRuntime(t)
				session := mustCreateSession(t, runtime, "snapshot-save-error", "snapshot save error")
				installFailTrigger(t, runtime, "fail_ctx_state_insert_snapshot", tableSessionContextLive, "INSERT", "snapshot save failed")
				if _, err := runtime.contextManager.Snapshot(ctx, session, Agent{ID: session.AgentID}); err == nil || !strings.Contains(err.Error(), "snapshot save failed") {
					t.Fatalf("Snapshot save err = %v", err)
				}
			})

			t.Run("compact manual handoff failure", func(t *testing.T) {
				runtime := newTestRuntime(t)
				session, raw := mustCreateSessionWithRaw(t, runtime, "compact-manual-error", "compact manual error")
				appendContextEvents(t, runtime.rawSessionService, raw, 0, 5)
				installFailTrigger(t, runtime, "fail_handoff_insert_manual", tableHandoffSegments, "INSERT", "manual handoff failed")
				if _, err := runtime.contextManager.Compact(ctx, session, Agent{ID: session.AgentID, RecentUserWindow: 1}, SessionCompactRequest{}); err == nil || !strings.Contains(err.Error(), "manual handoff failed") {
					t.Fatalf("Compact manual err = %v", err)
				}
			})

			t.Run("compact aggressive handoff failure", func(t *testing.T) {
				runtime := newTestRuntime(t)
				session, raw := mustCreateSessionWithRaw(t, runtime, "compact-aggressive-error", "compact aggressive error")
				appendContextEvents(t, runtime.rawSessionService, raw, 0, 5)
				installFailTrigger(t, runtime, "fail_handoff_insert_aggressive", tableHandoffSegments, "INSERT", "aggressive handoff failed")
				if _, err := runtime.contextManager.Compact(ctx, session, Agent{ID: session.AgentID, RecentUserWindow: 1}, SessionCompactRequest{Mode: "aggressive"}); err == nil || !strings.Contains(err.Error(), "aggressive handoff failed") {
					t.Fatalf("Compact aggressive err = %v", err)
				}
			})

			t.Run("compact save context failure", func(t *testing.T) {
				runtime := newTestRuntime(t)
				session, raw := mustCreateSessionWithRaw(t, runtime, "compact-save-error", "compact save error")
				appendContextEvents(t, runtime.rawSessionService, raw, 0, 5)
				installFailTrigger(t, runtime, "fail_ctx_state_insert_compact", tableSessionContextLive, "INSERT", "compact state failed")
				if _, err := runtime.contextManager.Compact(ctx, session, Agent{ID: session.AgentID, RecentUserWindow: 1}, SessionCompactRequest{}); err == nil || !strings.Contains(err.Error(), "compact state failed") {
					t.Fatalf("Compact save err = %v", err)
				}
			})
		})

		t.Run("instruction suffix blank summary and missing session branches", func(t *testing.T) {
			runtime := newTestRuntime(t)
			session := mustCreateSession(t, runtime, "suffix-blank", "suffix blank")
			state := SessionContextState{SessionID: session.ID, ContextRevisionID: "ctxrev-blank", ContextRevisionCreatedAt: nowString(), CreatedAt: nowString(), UpdatedAt: nowString()}
			if _, err := runtime.Store().SaveSessionContext(ctx, state); err != nil {
				t.Fatalf("SaveSessionContext: %v", err)
			}
			if _, err := runtime.Store().SaveHandoffSegment(ctx, HandoffSegment{
				SessionID:         session.ID,
				ContextRevisionID: state.ContextRevisionID,
				Sequence:          1,
				StartEventIndex:   0,
				EndEventIndex:     1,
				Summary:           "   ",
				Active:            true,
			}); err != nil {
				t.Fatalf("SaveHandoffSegment blank: %v", err)
			}
			if suffix, err := runtime.contextManager.InstructionSuffix(ctx, session.ID); err != nil || suffix != "" {
				t.Fatalf("InstructionSuffix blank summary = %q/%v", suffix, err)
			}
			if err := runtime.contextManager.syncHandoffStateForSessionID(ctx, "missing-session", nil); err != nil {
				t.Fatalf("syncHandoffStateForSessionID missing session: %v", err)
			}
		})
	})
}
