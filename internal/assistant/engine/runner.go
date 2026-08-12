package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	adkartifact "google.golang.org/adk/v2/artifact"
	adkmemory "google.golang.org/adk/v2/memory"
	adksession "google.golang.org/adk/v2/session"
)

type Runtime struct {
	store              *Store
	tools              *ToolRegistry
	skills             *SkillRegistry
	executor           WorkflowExecution
	sessionService     adksession.Service
	rawSessionService  adksession.Service
	artifactService    adkartifact.Service
	memoryService      adkmemory.Service
	contextManager     *SessionContextManager
	responses          responsesClient
	limitsProvider     jfadkmodel.RuntimeLimitsProvider
	activeMu           sync.Mutex
	activeRuns         map[string]context.CancelFunc
	adkMu              sync.Mutex
	adkRuns            map[string]*googleADKExecution
	startupReconcile   bool
	workflowChildMu    sync.Mutex
	approvalMu         sync.Mutex
	approvalRuns       map[string]struct{}
	inputRuns          map[string]struct{}
	approvalWG         sync.WaitGroup
	closing            bool
	backgroundCtx      context.Context
	backgroundCancel   context.CancelFunc
	compactionMu       sync.Mutex
	compactionSessions map[string]struct{}
	runSem             chan struct{} // Concurrency limiter for active runs
	executorID         string
	runLeaseTTL        time.Duration
	runLeaseHeartbeat  time.Duration
	runLeases          map[string]enginepersistence.RunLease
	runLeaseWG         sync.WaitGroup
}

func NewRuntime(store *Store, tools *ToolRegistry) *Runtime {
	return NewRuntimeWithSessionService(store, tools, nil)
}

func NewRuntimeWithSessionService(store *Store, tools *ToolRegistry, sessionService adksession.Service) *Runtime {
	if tools == nil {
		tools = NewToolRegistry()
	}
	if sessionService == nil {
		sessionService = adksession.InMemoryService()
	}
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	skillsPath := ""
	if store != nil {
		skillsPath = store.SkillsPath()
	}
	artifactService, err := enginepersistence.NewGoogleADKArtifactService(enginepersistence.DeriveGoogleADKArtifactPathFromSessionService(sessionService))
	if err != nil {
		besteffort.LogError(err)
		artifactService = adkartifact.InMemoryService()
	}
	r := &Runtime{
		store: store, tools: tools, skills: NewSkillRegistry(skillsPath), sessionService: sessionService, rawSessionService: sessionService, artifactService: artifactService, memoryService: newGoogleADKMemoryService(store), responses: newResponsesClient(),
		activeRuns: map[string]context.CancelFunc{}, adkRuns: map[string]*googleADKExecution{}, approvalRuns: map[string]struct{}{}, inputRuns: map[string]struct{}{}, compactionSessions: map[string]struct{}{},
		backgroundCtx: backgroundCtx, backgroundCancel: backgroundCancel, runSem: make(chan struct{}, MaxConcurrentRuns),
		executorID: "executor-" + uuid.NewString(), runLeaseTTL: defaultADKRunLeaseTTL,
		runLeaseHeartbeat: defaultADKRunLeaseHeartbeat, runLeases: map[string]enginepersistence.RunLease{},
	}
	if store != nil {
		store.SetSessionService(sessionService)
	}
	if store != nil {
		r.contextManager = NewSessionContextManager(store, sessionService, r.responses, tools)
		r.sessionService = r.contextManager.WrapService(sessionService, r.beginSessionCompaction)
		store.SetSessionService(sessionService)
	}
	r.registerModelCatalogTool()
	besteffort.LogError(r.reconcileStaleRuns(context.Background()))
	return r
}

func (r *Runtime) beginSessionCompaction(sessionID string) (func(), bool) {
	sessionID = strings.TrimSpace(sessionID)
	if r == nil || sessionID == "" {
		return func() {}, true
	}
	r.compactionMu.Lock()
	if r.compactionSessions == nil {
		r.compactionSessions = make(map[string]struct{})
	}
	if _, exists := r.compactionSessions[sessionID]; exists {
		r.compactionMu.Unlock()
		return func() {}, false
	}
	r.compactionSessions[sessionID] = struct{}{}
	r.compactionMu.Unlock()
	release := func() {
		r.compactionMu.Lock()
		delete(r.compactionSessions, sessionID)
		r.compactionMu.Unlock()
	}
	return release, true
}

func (r *Runtime) SetRuntimeLimitsProvider(provider jfadkmodel.RuntimeLimitsProvider) {
	if r == nil {
		return
	}
	r.limitsProvider = provider
}

func (r *Runtime) runtimeLimits() jfadkmodel.RuntimeLimits {
	limits := jfadkmodel.RuntimeLimits{RunTimeout: DefaultRunTimeout}
	if r == nil || r.limitsProvider == nil {
		return limits
	}
	updated := r.limitsProvider()
	if updated.RunTimeout > 0 {
		limits.RunTimeout = updated.RunTimeout
	}
	return limits
}

func (r *Runtime) Store() *Store {
	if r == nil {
		return nil
	}
	return r.store
}

func (r *Runtime) WorkflowStore() jfadkmodel.WorkflowStore {
	if r == nil {
		return nil
	}
	return r.store
}

func (r *Runtime) HasDatabaseActivity(ctx context.Context) (bool, error) {
	if r == nil || r.store == nil {
		return false, nil
	}
	r.activeMu.Lock()
	active := len(r.activeRuns) > 0
	r.activeMu.Unlock()
	if active {
		return true, nil
	}
	return r.store.HasDatabaseActivity(ctx)
}

// PurgeDeletedConfigs delegates configuration maintenance through the runtime
// boundary so application assembly never needs the concrete Store handle.
func (r *Runtime) PurgeDeletedConfigs(ctx context.Context, ids DeletedConfigIDs) (int, error) {
	if r == nil || r.store == nil {
		return 0, fmt.Errorf("adk runtime is unavailable")
	}
	return r.store.PurgeDeletedConfigs(ctx, ids)
}

// CompactDatabase compacts the ADK configuration database through the runtime
// boundary so callers do not reach into Runtime.Store().
func (r *Runtime) CompactDatabase(ctx context.Context) error {
	if r == nil || r.store == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	return r.store.CompactDatabase(ctx)
}

func (r *Runtime) CompactSessionDatabase(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	return enginepersistence.CompactSQLiteSessionService(ctx, r.rawSessionService)
}

func (r *Runtime) CompactArtifactDatabase(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	service, ok := r.artifactService.(interface{ Compact(context.Context) error })
	if !ok || service == nil {
		return fmt.Errorf("ADK artifact database is unavailable")
	}
	return service.Compact(ctx)
}

func (r *Runtime) SessionContext(ctx context.Context, sessionID string) (SessionContextSnapshot, error) {
	if r == nil || r.store == nil || r.contextManager == nil {
		return SessionContextSnapshot{}, fmt.Errorf("adk runtime is unavailable")
	}
	session, ok, err := r.store.Session(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	if !ok {
		return SessionContextSnapshot{}, fmt.Errorf("session not found")
	}
	agent, err := r.resolveSessionContextAgent(ctx, session)
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	agent, err = r.prepareAgent(ctx, agent)
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	return r.contextManager.Snapshot(ctx, session, agent)
}

func (r *Runtime) CompactSessionContext(ctx context.Context, sessionID string, mode string, trigger string, reason string) (SessionContextSnapshot, error) {
	if r == nil || r.store == nil || r.contextManager == nil {
		return SessionContextSnapshot{}, fmt.Errorf("adk runtime is unavailable")
	}
	session, ok, err := r.store.Session(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return SessionContextSnapshot{}, err
	}
	if !ok {
		return SessionContextSnapshot{}, fmt.Errorf("session not found")
	}
	release, acquired := r.beginSessionCompaction(session.ID)
	if !acquired {
		return SessionContextSnapshot{}, fmt.Errorf("session context compaction already running")
	}
	defer release()
	notice := r.createContextCompactionNotice(ctx, session.ID)
	fail := func(compactErr error) (SessionContextSnapshot, error) {
		r.updateContextCompactionNotice(ctx, notice, TimelineStatusError, contextCompactionFailedText)
		return SessionContextSnapshot{}, compactErr
	}
	active, err := r.contextManager.HasActiveRun(ctx, session.ID)
	if err != nil {
		return fail(err)
	}
	if active {
		return fail(fmt.Errorf("session has an active run"))
	}
	agent, err := r.resolveSessionContextAgent(ctx, session)
	if err != nil {
		return fail(err)
	}
	agent, err = r.prepareAgent(ctx, agent)
	if err != nil {
		return fail(err)
	}
	agent.ReasoningEffort = ""
	snapshot, err := r.contextManager.Compact(ctx, session, agent, SessionCompactRequest{
		Mode:    normalizeCompactMode(mode),
		Trigger: defaultString(strings.TrimSpace(trigger), "manual"),
		Reason:  reason,
	})
	if err != nil {
		return fail(err)
	}
	r.updateContextCompactionNotice(ctx, notice, TimelineStatusFinal, contextCompactionDoneText)
	return snapshot, nil
}

func (r *Runtime) resolveSessionContextAgent(ctx context.Context, session Session) (Agent, error) {
	agent, err := r.resolveAgentDefinition(ctx, session.AgentID)
	if err != nil {
		return Agent{}, err
	}
	base := agent
	agent, overridden := r.applySessionComposerModelOverride(ctx, session.ID, agent)
	resolved, err := r.resolveAgentProvider(ctx, agent)
	if err != nil {
		if overridden && strings.TrimSpace(base.ProviderID) != "" {
			if fallbackResolved, fallbackErr := r.resolveAgentProvider(ctx, base); fallbackErr == nil {
				return fallbackResolved, nil
			}
		}
		return Agent{}, err
	}
	return resolved, nil
}

func (r *Runtime) applySessionComposerModelOverride(ctx context.Context, sessionID string, agent Agent) (Agent, bool) {
	if r == nil || r.store == nil || strings.TrimSpace(sessionID) == "" {
		return agent, false
	}
	state, _, err := r.store.SessionComposerState(ctx, sessionID)
	if err != nil {
		return agent, false
	}
	overridden := false
	if providerID := strings.TrimSpace(state.ProviderIDOverride); providerID != "" {
		agent.ProviderID = providerID
		overridden = true
	}
	if model := strings.TrimSpace(state.ModelOverride); model != "" {
		agent.Model = model
		overridden = true
	}
	return agent, overridden
}

// goBackground 启动一个被 Close 等待的后台 goroutine，并把 runtime 的
// backgroundCtx 传给它。关闭中（closing）时直接丢弃任务，避免在 store 关闭后
// 仍有写入落到已关闭的 SQLite 连接上。返回值表示任务是否真的被启动。
func (r *Runtime) goBackground(fn func(ctx context.Context)) bool {
	if r == nil || fn == nil {
		return false
	}
	r.approvalMu.Lock()
	if r.closing {
		r.approvalMu.Unlock()
		return false
	}
	r.approvalWG.Add(1)
	ctx := r.backgroundCtx
	if ctx == nil {
		ctx = context.Background()
	}
	r.approvalMu.Unlock()
	go func() {
		defer r.approvalWG.Done()
		fn(ctx)
	}()
	return true
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.activeMu.Lock()
	for id, cancel := range r.activeRuns {
		cancel()
		delete(r.activeRuns, id)
	}
	r.activeMu.Unlock()
	r.approvalMu.Lock()
	r.closing = true
	if r.backgroundCancel != nil {
		r.backgroundCancel()
	}
	r.approvalMu.Unlock()
	r.approvalWG.Wait()
	r.runLeaseWG.Wait()
	sessionErr := r.CloseSessionServices()
	return errors.Join(sessionErr, r.store.Close())
}

func (r *Runtime) CloseSessionServices() error {
	if r == nil {
		return nil
	}
	sessionErr := enginepersistence.CloseSessionService(r.sessionService)
	if r.rawSessionService != nil && r.rawSessionService != r.sessionService {
		sessionErr = errors.Join(sessionErr, enginepersistence.CloseSessionService(r.rawSessionService))
	}
	return errors.Join(sessionErr, enginepersistence.CloseArtifactService(r.artifactService))
}

func (r *Runtime) Tools() *ToolRegistry {
	if r == nil {
		return nil
	}
	return r.tools
}

func (r *Runtime) Skills() *SkillRegistry {
	if r == nil {
		return nil
	}
	return r.skills
}

func (r *Runtime) Snapshot(ctx context.Context) (Snapshot, error) {
	providers, err := r.store.ListProviders(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	agents, err := r.store.ListAgents(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	skills, err := r.skills.List(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Providers: providers, Agents: agents, Skills: skills, Tools: r.tools.List()}, nil
}

func (r *Runtime) TestProvider(ctx context.Context, providerID string, modes ...ProviderTestMode) (map[string]any, error) {
	provider, ok, err := r.store.Provider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("provider not found")
	}
	apiKey, hasKey, err := r.store.ProviderAPIKey(provider.ID)
	if err != nil {
		return nil, err
	}
	if !hasKey || strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("provider API key is not configured")
	}
	mode := ProviderTestMode("")
	if len(modes) > 0 {
		mode = modes[0]
	}
	result, err := providers.ProbeProvider(ctx, provider, apiKey, mode)
	if err != nil {
		return nil, err
	}
	updated, updateErr := r.store.UpdateProviderCapabilities(ctx, provider.ID, result.Capabilities)
	if updateErr != nil {
		return nil, updateErr
	}
	result.Capabilities = updated.Capabilities
	result.CheckedAt = nowString()
	r.audit(ctx, "provider.tested", provider.ID, "Provider capability test completed.", map[string]any{
		"capabilities": result.Capabilities, "mode": result.Reasoning.Mode,
		"reasoningOK": result.Reasoning.OK, "reasoningResults": len(result.Reasoning.Results),
	})
	return map[string]any{
		"ok": result.OK, "reply": result.Reply, "capabilities": result.Capabilities,
		"reasoning": result.Reasoning, "checkedAt": result.CheckedAt,
	}, nil
}

func (r *Runtime) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return r.runChat(ctx, req, nil, false)
}

func (r *Runtime) ChatStream(ctx context.Context, req ChatRequest, onDelta func(ChatDelta) error) (ChatResponse, error) {
	return r.runChat(ctx, req, onDelta, true)
}

func (r *Runtime) resolveAgent(ctx context.Context, agentID string) (Agent, error) {
	agent, err := r.resolveAgentDefinition(ctx, agentID)
	if err != nil {
		return Agent{}, err
	}
	if strings.TrimSpace(agentID) == "" && strings.TrimSpace(agent.ProviderID) == "" {
		return r.resolveAgentProvider(ctx, agent)
	}
	agent, err = r.resolveAgentProvider(ctx, agent)
	if err != nil {
		return Agent{}, err
	}
	return agent, nil
}

func (r *Runtime) resolveAgentDefinition(ctx context.Context, agentID string) (Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		agent, err := r.store.DefaultAgent(ctx)
		if err != nil {
			return Agent{}, err
		}
		if agent.Status == AgentStatusDisabled {
			return Agent{}, fmt.Errorf("agent is disabled")
		}
		if agent.DeletedAt != nil {
			return Agent{}, fmt.Errorf("agent is deleted")
		}
		return agent, nil
	}
	agent, ok, err := r.store.Agent(ctx, agentID)
	if err != nil {
		return Agent{}, err
	}
	if !ok {
		return Agent{}, fmt.Errorf("agent not found")
	}
	if agent.Status == AgentStatusDisabled {
		return Agent{}, fmt.Errorf("agent is disabled")
	}
	if agent.DeletedAt != nil {
		return Agent{}, fmt.Errorf("agent is deleted")
	}
	return agent, nil
}

func (r *Runtime) resolveAgentProvider(ctx context.Context, agent Agent) (Agent, error) {
	if r == nil || r.store == nil {
		return Agent{}, fmt.Errorf("adk runtime is unavailable")
	}
	provider, err := r.effectiveProvider(ctx, agent.ProviderID)
	if err != nil {
		return Agent{}, err
	}
	if !provider.Enabled {
		return Agent{}, fmt.Errorf("agent provider is unavailable")
	}
	if _, hasKey, keyErr := r.store.ProviderAPIKey(provider.ID); keyErr != nil {
		return Agent{}, keyErr
	} else if !hasKey {
		return Agent{}, fmt.Errorf("agent provider API key is not configured")
	}
	field, value, reasoningErr := jfadkmodel.ResolveProviderReasoning(provider, agent.ReasoningEffort)
	if reasoningErr != nil {
		return Agent{}, reasoningErr
	}
	agent.ProviderID = provider.ID
	agent.ReasoningEffort = jfadkmodel.NormalizeReasoningEffort(agent.ReasoningEffort)
	agent.ReasoningEffortField = field
	agent.ReasoningEffortValue = value
	return agent, nil
}

func (r *Runtime) effectiveProvider(ctx context.Context, providerID string) (Provider, error) {
	if r == nil || r.store == nil {
		return Provider{}, fmt.Errorf("adk runtime is unavailable")
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		provider, ok, err := r.store.DefaultProvider(ctx)
		if err != nil {
			return Provider{}, err
		}
		if !ok {
			return Provider{}, fmt.Errorf("default agent provider is not configured")
		}
		return provider, nil
	}
	provider, providerOK, providerErr := r.store.Provider(ctx, providerID)
	if providerErr != nil {
		return Provider{}, providerErr
	}
	if !providerOK {
		return Provider{}, fmt.Errorf("agent provider is unavailable")
	}
	return provider, nil
}

func (r *Runtime) resolveSession(ctx context.Context, sessionID string, agent Agent, text string) (Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		session, ok, err := r.store.Session(ctx, sessionID)
		if err != nil {
			return Session{}, err
		}
		if ok {
			if session.AgentID != "" && session.AgentID != agent.ID {
				return Session{}, fmt.Errorf("session belongs to a different agent")
			}
			return session, nil
		}
		return Session{}, fmt.Errorf("session not found")
	}
	title := text
	if len([]rune(title)) > 28 {
		title = string([]rune(title)[:28])
	}
	return r.store.CreateSession(ctx, agent.ID, title)
}

func (r *Runtime) DeleteSession(ctx context.Context, sessionID string) error {
	if r == nil || r.store == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	session, ok, err := r.store.Session(ctx, sessionID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("session not found")
	}
	if r.sessionService != nil {
		if err := r.sessionService.Delete(ctx, &adksession.DeleteRequest{
			AppName:   GoogleADKAppName(session.AgentID),
			UserID:    googleADKUserID,
			SessionID: session.ID,
		}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
			return err
		}
	}
	return r.store.DeleteSession(ctx, sessionID)
}

func (r *Runtime) prepareAgent(ctx context.Context, agent Agent) (Agent, error) {
	for _, id := range agent.Skills {
		if _, ok, err := r.skills.Get(ctx, id); err != nil {
			return Agent{}, err
		} else if !ok {
			return Agent{}, fmt.Errorf("skill not found: %s", strings.TrimSpace(id))
		}
	}
	if agent.MemoryEnabled {
		memoryPrompt, err := r.agentMemoryPrompt(ctx, agent.ID)
		if err != nil {
			return Agent{}, err
		}
		if memoryPrompt != "" {
			agent.Instruction = strings.TrimSpace(agent.Instruction) + "\n\nJFTrade memory:\n" + memoryPrompt
		}
	}
	return agent, nil
}

func (r *Runtime) agentMemoryPrompt(ctx context.Context, agentID string) (string, error) {
	if r == nil || r.store == nil {
		return "", nil
	}
	entries, err := r.store.ListMemory(ctx, agentID)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	lines := make([]string, 0, len(entries))
	remaining := 4000
	for _, entry := range entries {
		line := fmt.Sprintf("- [%s] %s: %s", entry.Scope, entry.Key, strings.TrimSpace(entry.Value))
		if len([]rune(line)) > remaining {
			line = string([]rune(line)[:remaining])
		}
		lines = append(lines, line)
		remaining -= len([]rune(line))
		if remaining <= 0 {
			break
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (r *Runtime) audit(ctx context.Context, kind string, subjectID string, detail string, metadata map[string]any) {
	if r == nil || r.store == nil {
		return
	}
	jftradeErr1 := r.store.AddAuditEvent(ctx, AuditEvent{
		Kind: kind, SubjectID: subjectID, Detail: detail, Metadata: metadata,
	})
	besteffort.LogError(jftradeErr1)
}

func (r *Runtime) RecordAudit(ctx context.Context, kind string, subjectID string, detail string, metadata map[string]any) {
	r.audit(ctx, kind, subjectID, detail, metadata)
}

func approvalResolutionSummary(run Run, approval Approval, approved bool) string {
	return jfadkmodel.ApprovalResolutionSummary(run, approval, approved)
}

func userFacingADKError(err error) string {
	return jfadkmodel.UserFacingADKError(err)
}
