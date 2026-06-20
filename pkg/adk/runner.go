package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	adksession "google.golang.org/adk/session"
)

type Runtime struct {
	store             *Store
	tools             *ToolRegistry
	skills            *SkillRegistry
	sessionService    adksession.Service
	rawSessionService adksession.Service
	contextManager    *SessionContextManager
	openai            openAIClient
	limitsProvider    RuntimeLimitsProvider
	activeMu          sync.Mutex
	activeRuns        map[string]context.CancelFunc
	adkMu             sync.Mutex
	adkRuns           map[string]*googleADKExecution
	workflowChildMu   sync.Mutex
	approvalMu        sync.Mutex
	approvalRuns      map[string]struct{}
	runSem            chan struct{} // Concurrency limiter for active runs
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
	skillsPath := ""
	if store != nil {
		skillsPath = store.SkillsPath()
	}
	r := &Runtime{
		store: store, tools: tools, skills: NewSkillRegistry(skillsPath), sessionService: sessionService, rawSessionService: sessionService, openai: newOpenAIClient(),
		activeRuns: map[string]context.CancelFunc{}, adkRuns: map[string]*googleADKExecution{}, approvalRuns: map[string]struct{}{},
		runSem: make(chan struct{}, MaxConcurrentRuns),
	}
	if store != nil {
		store.SetSessionService(sessionService)
	}
	if store != nil {
		r.contextManager = NewSessionContextManager(store, sessionService, r.openai, tools)
		r.sessionService = r.contextManager.WrapService(sessionService)
		store.SetSessionService(sessionService)
	}
	r.reconcileStaleRuns(context.Background())
	return r
}

func (r *Runtime) SetRuntimeLimitsProvider(provider RuntimeLimitsProvider) {
	if r == nil {
		return
	}
	r.limitsProvider = provider
}

func (r *Runtime) runtimeLimits() RuntimeLimits {
	limits := RuntimeLimits{RunTimeout: DefaultRunTimeout}
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
	agent, err := r.resolveAgent(ctx, session.AgentID)
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
		return fail(fmt.Errorf("session has an active or pending run"))
	}
	agent, err := r.resolveAgent(ctx, session.AgentID)
	if err != nil {
		return fail(err)
	}
	agent, err = r.prepareAgent(ctx, agent)
	if err != nil {
		return fail(err)
	}
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
	sessionErr := r.CloseSessionServices()
	return errors.Join(sessionErr, r.store.Close())
}

func (r *Runtime) CloseSessionServices() error {
	if r == nil {
		return nil
	}
	sessionErr := CloseSessionService(r.sessionService)
	if r.rawSessionService != nil && r.rawSessionService != r.sessionService {
		sessionErr = errors.Join(sessionErr, CloseSessionService(r.rawSessionService))
	}
	return sessionErr
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

func (r *Runtime) TestProvider(ctx context.Context, providerID string) (map[string]any, error) {
	provider, ok, err := r.store.Provider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("provider not found")
	}
	apiKey, _, err := r.store.ProviderAPIKey(provider.ID)
	if err != nil {
		return nil, err
	}
	reply, err := r.openai.chat(ctx, provider, apiKey, provider.Model, []openAIChatMessage{
		{Role: "system", Content: "Reply with a short health check sentence."},
		{Role: "user", Content: "JFTrade ADK provider connectivity test."},
	})
	if err != nil {
		return nil, err
	}
	_, toolErr := r.openai.selectTools(ctx, provider, apiKey, provider.Model, []openAIChatMessage{
		{Role: "user", Content: "Do not call a tool."},
	}, []ToolDescriptor{{
		Name: "system.health_probe", DisplayName: "健康探测", Description: "用于探测 provider 工具能力的内部工具。", Permission: "read_internal",
	}})
	capabilities := map[string]bool{
		"streaming": true,
		"tools":     toolErr == nil,
		"reasoning": false,
	}
	updated, updateErr := r.store.UpdateProviderCapabilities(ctx, provider.ID, capabilities)
	if updateErr != nil {
		return nil, updateErr
	}
	r.audit(ctx, "provider.tested", provider.ID, "Provider capability test completed.", map[string]any{"capabilities": capabilities})
	return map[string]any{"ok": true, "reply": reply, "capabilities": updated.Capabilities, "checkedAt": nowString()}, nil
}

func (r *Runtime) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return r.runChat(ctx, req, nil, false)
}

func (r *Runtime) ChatStream(ctx context.Context, req ChatRequest, onDelta func(ChatDelta) error) (ChatResponse, error) {
	return r.runChat(ctx, req, onDelta, true)
}

func (r *Runtime) resolveAgent(ctx context.Context, agentID string) (Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return r.store.DefaultAgent(ctx)
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
	if strings.TrimSpace(agent.ProviderID) == "" {
		return Agent{}, fmt.Errorf("agent provider is required")
	}
	provider, providerOK, providerErr := r.store.Provider(ctx, agent.ProviderID)
	if providerErr != nil {
		return Agent{}, providerErr
	}
	if !providerOK || !provider.Enabled {
		return Agent{}, fmt.Errorf("agent provider is unavailable")
	}
	if _, hasKey, keyErr := r.store.ProviderAPIKey(provider.ID); keyErr != nil {
		return Agent{}, keyErr
	} else if !hasKey {
		return Agent{}, fmt.Errorf("agent provider API key is not configured")
	}
	return agent, nil
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
			AppName:   googleADKAppName(session.AgentID),
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
	jftradeLogError(jftradeErr1)
}

func (r *Runtime) RecordAudit(ctx context.Context, kind string, subjectID string, detail string, metadata map[string]any) {
	r.audit(ctx, kind, subjectID, detail, metadata)
}

func approvalResolutionSummary(run Run, approval Approval, approved bool) string {
	if !approved {
		return fmt.Sprintf("已拒绝工具调用 `%s`。本次 run 已结束，未执行该操作。", approval.ToolName)
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("已批准并执行工具调用 `%s`。", approval.ToolName))
	for _, call := range run.ToolCalls {
		if call.ToolName != approval.ToolName {
			continue
		}
		if call.Status == "SUCCEEDED" {
			lines = append(lines, "执行结果：")
			lines = append(lines, summarizeToolOutput(call.ToolName, call.Output))
		}
		if call.Status == "FAILED" && call.Error != nil {
			lines = append(lines, "执行失败："+*call.Error)
		}
	}
	return strings.Join(lines, "\n")
}

func userFacingADKError(err error) string {
	if err == nil {
		return ""
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "wrote more than the declared content-length"):
		return "模型服务响应异常，请检查模型服务配置或稍后重试。"
	case strings.Contains(lower, "database is locked") || strings.Contains(lower, "sqlite_busy"):
		return "数据库繁忙，请稍后重试。"
	default:
		return err.Error()
	}
}
