package adk

import (
	"context"
	"fmt"
	"iter"
	"log"
	"strings"
	"sync"
	"time"

	adksession "google.golang.org/adk/v2/session"
)

type compactingSessionService struct {
	base            adksession.Service
	manager         *SessionContextManager
	beginCompaction func(string) (func(), bool)
}

func (s *compactingSessionService) Create(ctx context.Context, req *adksession.CreateRequest) (*adksession.CreateResponse, error) {
	return s.base.Create(ctx, req)
}

func (s *compactingSessionService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	response, err := s.base.Get(ctx, req)
	if err != nil || s.manager == nil || response == nil {
		return response, err
	}
	session, ok, storeErr := s.manager.store.Session(ctx, req.SessionID)
	if storeErr != nil || !ok {
		return response, storeErr
	}
	agent, ok, storeErr := s.manager.store.Agent(ctx, session.AgentID)
	if storeErr != nil || !ok {
		return response, storeErr
	}
	compacted, compactErr := s.autoCompactForModelContext(ctx, session, agent)
	if compactErr != nil {
		log.Printf("[adk] auto context compaction before model session read failed for session %s: %v", req.SessionID, compactErr)
	} else if compacted {
		response, err = s.base.Get(ctx, req)
		if err != nil || response == nil {
			return response, err
		}
	}
	state, stateOK, stateErr := s.manager.store.SessionContext(ctx, req.SessionID)
	if stateErr != nil {
		return response, stateErr
	}
	if !stateOK {
		state = SessionContextState{SessionID: req.SessionID}
	}
	state = ensureSessionContextRevision(state, req.SessionID)
	segments, stateErr := s.manager.store.HandoffSegmentsForRevision(ctx, req.SessionID, state.ContextRevisionID, true)
	if stateErr != nil {
		return response, stateErr
	}
	events := eventSlice(response.Session.Events())
	cutoff := minInt(maxActiveSegmentEnd(segments), len(events))
	recentStart := max(recentUserEventStart(events, normalizeRecentUserWindow(agent.RecentUserWindow)), cutoff)
	protectedStart := max(protectedTailStart(events), cutoff)
	projected := projectVisibleSessionEvents(events, len(segments) > 0, cutoff, recentStart, protectedStart)
	if len(segments) == 0 && projected.trimmedToolResponseCount == 0 {
		return response, nil
	}
	filtered := filterEvents(projected.events, req.After, req.NumRecentEvents)
	response.Session = &wrappedSession{
		base:   response.Session,
		events: &wrappedEvents{items: filtered},
	}
	return response, nil
}

func (s *compactingSessionService) autoCompactForModelContext(ctx context.Context, session Session, agent Agent) (bool, error) {
	if s == nil || s.manager == nil {
		return false, nil
	}
	if s.beginCompaction != nil {
		release, acquired := s.beginCompaction(session.ID)
		if !acquired {
			return false, nil
		}
		defer release()
	}
	_, compacted, err := s.manager.AutoCompactForModelContext(ctx, session, agent, "")
	return compacted, err
}

func (s *compactingSessionService) List(ctx context.Context, req *adksession.ListRequest) (*adksession.ListResponse, error) {
	return s.base.List(ctx, req)
}

func (s *compactingSessionService) Delete(ctx context.Context, req *adksession.DeleteRequest) error {
	return s.base.Delete(ctx, req)
}

func (s *compactingSessionService) AppendEvent(ctx context.Context, session adksession.Session, event *adksession.Event) error {
	var projected *wrappedEvents
	if wrapped, ok := session.(*wrappedSession); ok && wrapped != nil {
		session = wrapped.base
		projected = wrapped.events
	}
	if err := appendADKEventWithStaleRetry(ctx, serviceAppendLocks(s), s.base, session, event); err != nil {
		return err
	}
	if projected != nil && event != nil && !event.Partial {
		projected.Append(event)
	}
	return nil
}

func appendADKEventWithStaleRetry(ctx context.Context, locks *adkSessionAppendLockMap, service adksession.Service, session adksession.Session, event *adksession.Event) error {
	if service == nil {
		return fmt.Errorf("adk session service is unavailable")
	}
	if session == nil {
		return fmt.Errorf("adk session is unavailable")
	}

	lock, release := locks.acquire(session)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	current := session
	if isSyntheticADKSession(current) {
		latest, err := service.Get(ctx, &adksession.GetRequest{
			AppName:   current.AppName(),
			UserID:    current.UserID(),
			SessionID: current.ID(),
		})
		if err != nil {
			return err
		}
		if latest == nil || latest.Session == nil {
			return fmt.Errorf("adk session %q is unavailable", current.ID())
		}
		current = latest.Session
	}
	var lastErr error
	for range adkSessionAppendMaxAttempts {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := service.AppendEvent(ctx, current, event)
		if err == nil {
			return nil
		}
		if !isRefreshableADKSessionError(err) {
			return err
		}
		lastErr = err
		latest, getErr := service.Get(ctx, &adksession.GetRequest{
			AppName:   current.AppName(),
			UserID:    current.UserID(),
			SessionID: current.ID(),
		})
		if getErr != nil {
			return err
		}
		if latest == nil || latest.Session == nil {
			return err
		}
		current = latest.Session
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("failed to append ADK event after %d attempts", adkSessionAppendMaxAttempts)
}

func isSyntheticADKSession(session adksession.Session) bool {
	switch session.(type) {
	case *wrappedSession, *emptySession:
		return true
	default:
		return false
	}
}

func isStaleADKSessionError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "stale session error")
}

func isRefreshableADKSessionError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return isStaleADKSessionError(err) || strings.Contains(lower, "unexpected session type")
}

func serviceAppendLocks(service *compactingSessionService) *adkSessionAppendLockMap {
	if service != nil && service.manager != nil && service.manager.appendLocks != nil {
		return service.manager.appendLocks
	}
	return newADKSessionAppendLockMap()
}

func runtimeAppendLocks(runtime *Runtime) *adkSessionAppendLockMap {
	if runtime != nil && runtime.contextManager != nil && runtime.contextManager.appendLocks != nil {
		return runtime.contextManager.appendLocks
	}
	return newADKSessionAppendLockMap()
}

func newADKSessionAppendLockMap() *adkSessionAppendLockMap {
	return &adkSessionAppendLockMap{locks: map[string]*adkSessionAppendLock{}}
}

func (m *adkSessionAppendLockMap) acquire(session adksession.Session) (*sync.Mutex, func()) {
	if m == nil {
		m = newADKSessionAppendLockMap()
	}
	key := strings.Join([]string{session.AppName(), session.UserID(), session.ID()}, "\x00")
	m.mu.Lock()
	lock := m.locks[key]
	if lock == nil {
		lock = &adkSessionAppendLock{}
		m.locks[key] = lock
	}
	lock.refs++
	m.mu.Unlock()
	return &lock.mu, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		lock.refs--
		if lock.refs <= 0 && m.locks[key] == lock {
			delete(m.locks, key)
		}
	}
}

func (m *adkSessionAppendLockMap) len() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.locks)
}

func filterEvents(events []*adksession.Event, after time.Time, numRecent int) []*adksession.Event {
	filtered := events[:0]
	for _, event := range events {
		if event == nil {
			continue
		}
		if !after.IsZero() && event.Timestamp.Before(after) {
			continue
		}
		filtered = append(filtered, event)
	}
	if numRecent > 0 && len(filtered) > numRecent {
		filtered = filtered[len(filtered)-numRecent:]
	}
	return filtered
}

type wrappedSession struct {
	base   adksession.Session
	events *wrappedEvents
}

func (s *wrappedSession) ID() string                { return s.base.ID() }
func (s *wrappedSession) AppName() string           { return s.base.AppName() }
func (s *wrappedSession) UserID() string            { return s.base.UserID() }
func (s *wrappedSession) State() adksession.State   { return s.base.State() }
func (s *wrappedSession) Events() adksession.Events { return s.events }
func (s *wrappedSession) LastUpdateTime() time.Time { return s.base.LastUpdateTime() }

type wrappedEvents struct {
	mu    sync.RWMutex
	items []*adksession.Event
}

func (e *wrappedEvents) All() iter.Seq[*adksession.Event] {
	e.mu.RLock()
	items := append([]*adksession.Event(nil), e.items...)
	e.mu.RUnlock()
	return func(yield func(*adksession.Event) bool) {
		for _, item := range items {
			if !yield(item) {
				return
			}
		}
	}
}

func (e *wrappedEvents) Len() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.items)
}

func (e *wrappedEvents) At(i int) *adksession.Event {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if i < 0 || i >= len(e.items) {
		return nil
	}
	return e.items[i]
}

func (e *wrappedEvents) Append(event *adksession.Event) {
	if e == nil || event == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, existing := range e.items {
		if existing != nil && existing.ID == event.ID {
			return
		}
	}
	e.items = append(e.items, event)
}

type emptySession struct {
	id             string
	appName        string
	userID         string
	state          adksession.State
	events         adksession.Events
	lastUpdateTime time.Time
}

func (s *emptySession) ID() string                { return s.id }
func (s *emptySession) AppName() string           { return s.appName }
func (s *emptySession) UserID() string            { return s.userID }
func (s *emptySession) State() adksession.State   { return s.state }
func (s *emptySession) Events() adksession.Events { return s.events }
func (s *emptySession) LastUpdateTime() time.Time { return s.lastUpdateTime }

type emptyState struct {
	values map[string]any
}

func (s *emptyState) Get(key string) (any, error) {
	value, ok := s.values[key]
	if !ok {
		return nil, adksession.ErrStateKeyNotExist
	}
	return value, nil
}

func (s *emptyState) Set(key string, value any) error {
	s.values[key] = value
	return nil
}

func (s *emptyState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for key, value := range s.values {
			if !yield(key, value) {
				return
			}
		}
	}
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
