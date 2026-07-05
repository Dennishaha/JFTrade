package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) ListProviders(ctx context.Context) ([]Provider, error) {
	var items []Provider
	if err := s.listJSON(ctx, tableProviders, "created_at ASC, id ASC", &items); err != nil {
		return nil, err
	}
	for index := range items {
		items[index] = normalizeProvider(items[index])
		items[index].HasAPIKey = s.secrets.has(items[index].ID)
	}
	if changed := normalizeDefaultProviderSelection(items); changed {
		if err := s.saveProviderDefaultSelection(ctx, items); err != nil {
			return nil, err
		}
		for index := range items {
			items[index].HasAPIKey = s.secrets.has(items[index].ID)
		}
	}
	sortProvidersDefaultFirst(items)
	return items, nil
}

func (s *Store) SaveProvider(ctx context.Context, req ProviderWriteRequest) (Provider, error) {
	id := normalizeID(req.ID)
	if id == "" {
		id = normalizeID(req.DisplayName)
	}
	if id == "" {
		id = "provider-" + uuid.NewString()
	}
	if strings.TrimSpace(req.BaseURL) != "" {
		if err := validateProviderBaseURL(req.BaseURL); err != nil {
			return Provider{}, err
		}
	}
	if err := validateProviderHeaders(req.DefaultHeaders); err != nil {
		return Provider{}, err
	}
	now := nowString()
	existing, ok, err := s.Provider(ctx, id)
	if err != nil {
		return Provider{}, err
	}
	createdAt := now
	if ok {
		createdAt = existing.CreatedAt
	}
	provider := Provider{
		ID:                  id,
		DisplayName:         defaultString(req.DisplayName, id),
		BaseURL:             normalizeBaseURL(req.BaseURL),
		Model:               defaultString(req.Model, "gpt-4o-mini"),
		ContextWindowTokens: normalizeContextWindowTokens(req.ContextWindowTokens),
		RequestTimeoutMs:    normalizeProviderRequestTimeoutMs(req.RequestTimeoutMs),
		DefaultHeaders:      normalizeHeaders(req.DefaultHeaders),
		Enabled:             req.Enabled,
		Default:             existing.Default,
		CreatedAt:           createdAt,
		UpdatedAt:           now,
	}
	if ok {
		provider.Capabilities = existing.Capabilities
		if req.RequestTimeoutMs == 0 {
			provider.RequestTimeoutMs = existing.RequestTimeoutMs
		}
		if req.ContextWindowTokens == 0 {
			provider.ContextWindowTokens = existing.ContextWindowTokens
		}
	}
	provider = normalizeProvider(provider)
	if strings.TrimSpace(provider.BaseURL) == "" {
		provider.BaseURL = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(req.APIKey) != "" {
		if err := s.secrets.set(id, strings.TrimSpace(req.APIKey)); err != nil {
			return Provider{}, err
		}
	}
	provider.HasAPIKey = s.secrets.has(id)
	if !ok {
		providers, err := s.ListProviders(ctx)
		if err != nil {
			return Provider{}, err
		}
		provider.Default = len(providers) == 0
	}
	if err := s.saveJSON(ctx, tableProviders, provider.ID, provider.CreatedAt, provider.UpdatedAt, provider); err != nil {
		return Provider{}, err
	}
	if _, err := s.ensureDefaultProvider(ctx); err != nil {
		return Provider{}, err
	}
	saved, ok, err := s.Provider(ctx, provider.ID)
	if err != nil {
		return Provider{}, err
	}
	if ok {
		return saved, nil
	}
	return provider, nil
}

func (s *Store) UpdateProviderCapabilities(ctx context.Context, id string, capabilities map[string]bool) (Provider, error) {
	provider, ok, err := s.Provider(ctx, id)
	if err != nil {
		return Provider{}, err
	}
	if !ok {
		return Provider{}, os.ErrNotExist
	}
	provider.Capabilities = capabilities
	provider.UpdatedAt = nowString()
	return provider, s.saveJSON(ctx, tableProviders, provider.ID, provider.CreatedAt, provider.UpdatedAt, provider)
}

func (s *Store) Provider(ctx context.Context, id string) (Provider, bool, error) {
	var provider Provider
	ok, err := s.getJSON(ctx, tableProviders, id, &provider)
	if err != nil || !ok {
		return Provider{}, ok, err
	}
	provider = normalizeProvider(provider)
	provider.HasAPIKey = s.secrets.has(provider.ID)
	return provider, true, nil
}

func (s *Store) DefaultProvider(ctx context.Context) (Provider, bool, error) {
	providers, err := s.ListProviders(ctx)
	if err != nil {
		return Provider{}, false, err
	}
	if len(providers) == 0 {
		return Provider{}, false, nil
	}
	return providers[0], true, nil
}

func (s *Store) SetDefaultProvider(ctx context.Context, id string) (Provider, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Provider{}, os.ErrNotExist
	}
	providers, err := s.loadProvidersCreatedFirst(ctx)
	if err != nil {
		return Provider{}, err
	}
	found := -1
	for index := range providers {
		if providers[index].ID == id {
			found = index
			break
		}
	}
	if found < 0 {
		return Provider{}, os.ErrNotExist
	}
	for index := range providers {
		providers[index].Default = providers[index].ID == id
	}
	if err := s.saveProviderDefaultSelection(ctx, providers); err != nil {
		return Provider{}, err
	}
	provider, ok, err := s.Provider(ctx, id)
	if err != nil {
		return Provider{}, err
	}
	if !ok {
		return Provider{}, os.ErrNotExist
	}
	return provider, nil
}

func (s *Store) ProviderAPIKey(id string) (string, bool, error) {
	return s.secrets.get(id)
}

func (s *Store) DeleteProvider(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return os.ErrNotExist
	}
	agents, err := s.ListAgents(ctx)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		if strings.TrimSpace(agent.ProviderID) == id {
			return fmt.Errorf("provider is used by agent %q", agent.Name)
		}
	}
	deletedProvider, deletedOK, err := s.Provider(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableProviders+` WHERE id = ?`, id); err != nil {
		return err
	}
	jftradeErr3 := s.secrets.delete(id)
	jftradeLogError(jftradeErr3)
	if deletedOK && deletedProvider.Default {
		if _, err := s.ensureDefaultProvider(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadProvidersCreatedFirst(ctx context.Context) ([]Provider, error) {
	var items []Provider
	if err := s.listJSON(ctx, tableProviders, "created_at ASC, id ASC", &items); err != nil {
		return nil, err
	}
	for index := range items {
		items[index] = normalizeProvider(items[index])
		items[index].HasAPIKey = s.secrets.has(items[index].ID)
	}
	return items, nil
}

func (s *Store) ensureDefaultProvider(ctx context.Context) (bool, error) {
	providers, err := s.loadProvidersCreatedFirst(ctx)
	if err != nil {
		return false, err
	}
	if normalizeDefaultProviderSelection(providers) {
		return true, s.saveProviderDefaultSelection(ctx, providers)
	}
	return false, nil
}

func normalizeDefaultProviderSelection(providers []Provider) bool {
	if len(providers) == 0 {
		return false
	}
	firstDefault := -1
	changed := false
	for index := range providers {
		providers[index] = normalizeProvider(providers[index])
		if providers[index].Default {
			if firstDefault < 0 {
				firstDefault = index
			} else {
				providers[index].Default = false
				changed = true
			}
		}
	}
	if firstDefault < 0 {
		providers[0].Default = true
		changed = true
	}
	return changed
}

func (s *Store) saveProviderDefaultSelection(ctx context.Context, providers []Provider) error {
	now := nowString()
	for index := range providers {
		provider := normalizeProvider(providers[index])
		provider.HasAPIKey = s.secrets.has(provider.ID)
		provider.UpdatedAt = now
		if err := s.saveJSON(ctx, tableProviders, provider.ID, provider.CreatedAt, provider.UpdatedAt, provider); err != nil {
			return err
		}
		providers[index] = provider
	}
	return nil
}

func sortProvidersDefaultFirst(providers []Provider) {
	sort.SliceStable(providers, func(i, j int) bool {
		if providers[i].Default != providers[j].Default {
			return providers[i].Default
		}
		if providers[i].CreatedAt != providers[j].CreatedAt {
			return providers[i].CreatedAt < providers[j].CreatedAt
		}
		return providers[i].ID < providers[j].ID
	})
}

func (s *Store) ListAgents(ctx context.Context) ([]Agent, error) {
	var items []Agent
	if err := s.listJSON(ctx, tableAgents, "updated_at DESC, id ASC", &items); err != nil {
		return nil, err
	}
	active := make([]Agent, 0, len(items))
	for _, item := range items {
		if item.DeletedAt == nil {
			active = append(active, NormalizeAgent(item))
		}
	}
	sortAgentsPrimaryDefaultFirst(active)
	return active, nil
}

func (s *Store) ListAllAgents(ctx context.Context) ([]Agent, error) {
	var items []Agent
	if err := s.listJSON(ctx, tableAgents, "updated_at DESC, id ASC", &items); err != nil {
		return nil, err
	}
	for index := range items {
		items[index] = NormalizeAgent(items[index])
	}
	sortAgentsPrimaryDefaultFirst(items)
	return items, nil
}

func (s *Store) SaveAgent(ctx context.Context, req AgentWriteRequest) (Agent, error) {
	id := normalizeID(req.ID)
	if id == "" {
		id = normalizeID(req.Name)
	}
	if id == "" {
		id = "agent-" + uuid.NewString()
	}
	now := nowString()
	existing, ok, err := s.Agent(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	createdAt := now
	if ok {
		createdAt = existing.CreatedAt
	}
	status := strings.ToUpper(strings.TrimSpace(req.Status))
	if status == "" {
		status = AgentStatusEnabled
	}
	if status != AgentStatusEnabled && status != AgentStatusDisabled {
		return Agent{}, fmt.Errorf("invalid agent status %q", req.Status)
	}
	if IsPrimaryBuiltinAgentID(id) && status == AgentStatusDisabled {
		return Agent{}, fmt.Errorf("%w: primary builtin agent cannot be disabled", ErrBuiltinAgentProtected)
	}
	agent := Agent{
		ID:                id,
		Name:              defaultString(req.Name, id),
		Instruction:       strings.TrimSpace(req.Instruction),
		ProviderID:        strings.TrimSpace(req.ProviderID),
		Model:             strings.TrimSpace(req.Model),
		Tools:             normalizeStringSlice(req.Tools),
		Skills:            normalizeStringSlice(req.Skills),
		PermissionMode:    normalizePermissionMode(req.PermissionMode),
		MemoryEnabled:     req.MemoryEnabled,
		RecentUserWindow:  normalizeRecentUserWindow(req.RecentUserWindow),
		WorkMode:          normalizeAgentDefaultWorkMode(req.WorkMode),
		LoopMaxIterations: normalizeLoopMaxIterations(req.LoopMaxIterations),
		Status:            status,
		Builtin:           IsBuiltinAgentID(id),
		CreatedAt:         createdAt,
		UpdatedAt:         now,
	}
	if ok {
		agent.Builtin = existing.Builtin || agent.Builtin
	}
	if agent.Instruction == "" {
		agent.Instruction = defaultAgentInstruction()
	}
	if ok && req.RecentUserWindow == 0 {
		agent.RecentUserWindow = normalizeRecentUserWindow(existing.RecentUserWindow)
	}
	if ok && strings.TrimSpace(req.WorkMode) == "" {
		agent.WorkMode = normalizeAgentDefaultWorkMode(existing.WorkMode)
	}
	if ok && req.LoopMaxIterations == 0 {
		agent.LoopMaxIterations = normalizeLoopMaxIterations(existing.LoopMaxIterations)
	}
	agent = NormalizeAgent(agent)
	return agent, s.saveJSON(ctx, tableAgents, agent.ID, agent.CreatedAt, agent.UpdatedAt, agent)
}

func sortAgentsPrimaryDefaultFirst(agents []Agent) {
	sort.SliceStable(agents, func(i, j int) bool {
		leftDefault := IsPrimaryBuiltinAgentID(agents[i].ID)
		rightDefault := IsPrimaryBuiltinAgentID(agents[j].ID)
		if leftDefault != rightDefault {
			return leftDefault
		}
		return false
	})
}

func (s *Store) EnsureAgent(ctx context.Context, req AgentWriteRequest) (Agent, error) {
	id := normalizeID(req.ID)
	if id == "" {
		id = normalizeID(req.Name)
	}
	if id != "" {
		if existing, ok, err := s.Agent(ctx, id); err != nil || ok {
			return existing, err
		}
	}
	return s.SaveAgent(ctx, req)
}

func (s *Store) Agent(ctx context.Context, id string) (Agent, bool, error) {
	var agent Agent
	ok, err := s.getJSON(ctx, tableAgents, id, &agent)
	if err != nil || !ok {
		return Agent{}, ok, err
	}
	return NormalizeAgent(agent), true, nil
}

func (s *Store) DefaultAgent(ctx context.Context) (Agent, error) {
	agents, err := s.ListAgents(ctx)
	if err != nil {
		return Agent{}, err
	}
	for _, agent := range agents {
		if agent.ID == DefaultBuiltinAgentID && agent.Status == AgentStatusEnabled {
			return agent, nil
		}
	}
	for _, agent := range agents {
		if agent.Status == AgentStatusEnabled {
			return agent, nil
		}
	}
	if template, ok := BuiltinAgentTemplate(DefaultBuiltinAgentID); ok {
		return s.SaveAgent(ctx, template)
	}
	return s.SaveAgent(ctx, AgentWriteRequest{ID: DefaultBuiltinAgentID, Name: "默认助手", Instruction: defaultAgentInstruction(), PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled})
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	agent, ok, err := s.Agent(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return os.ErrNotExist
	}
	if agent.Builtin || IsBuiltinAgentID(agent.ID) {
		return fmt.Errorf("%w: builtin agent cannot be deleted", ErrBuiltinAgentProtected)
	}
	now := nowString()
	agent.Status = AgentStatusDisabled
	agent.DeletedAt = &now
	agent.UpdatedAt = now
	return s.saveJSON(ctx, tableAgents, agent.ID, agent.CreatedAt, agent.UpdatedAt, agent)
}

func (s *Store) CreateSession(ctx context.Context, agentID string, title string) (Session, error) {
	now := nowString()
	session := Session{ID: "session-" + uuid.NewString(), AgentID: strings.TrimSpace(agentID), Title: defaultString(title, "新的 ADK 会话"), CreatedAt: now, UpdatedAt: now}
	payload, err := json.Marshal(session)
	if err != nil {
		return Session{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+tableSessions+` (id, agent_id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET agent_id = excluded.agent_id, payload_json = excluded.payload_json, updated_at = excluded.updated_at`, session.ID, session.AgentID, string(payload), session.CreatedAt, session.UpdatedAt)
	return session, err
}

func (s *Store) RenameSession(ctx context.Context, id string, title string) (Session, error) {
	session, ok, err := s.Session(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		return Session{}, os.ErrNotExist
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return Session{}, fmt.Errorf("session title is required")
	}
	if len([]rune(title)) > 80 {
		title = string([]rune(title)[:80])
	}
	session.Title = title
	session.UpdatedAt = nowString()
	payload, err := json.Marshal(session)
	if err != nil {
		return Session{}, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE `+tableSessions+` SET payload_json = ?, updated_at = ? WHERE id = ?`, string(payload), session.UpdatedAt, session.ID)
	return session, err
}

func (s *Store) Session(ctx context.Context, id string) (Session, bool, error) {
	var session Session
	ok, err := s.getJSON(ctx, tableSessions, id, &session)
	return session, ok, err
}

func (s *Store) ListSessions(ctx context.Context) ([]Session, error) {
	var sessions []Session
	return sessions, s.listJSON(ctx, tableSessions, "updated_at DESC, id ASC", &sessions)
}

func (s *Store) ListSessionsPage(ctx context.Context, agentID string, query string, limit int, offset int) ([]Session, int, error) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if agentID = strings.TrimSpace(agentID); agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if query = strings.ToLower(strings.TrimSpace(query)); query != "" {
		clauses = append(clauses, "LOWER(json_extract(payload_json, '$.title')) LIKE ?")
		args = append(args, "%"+query+"%")
	}
	var sessions []Session
	total, err := s.listJSONPage(ctx, tableSessions, clauses, args, "updated_at DESC, id ASC", limit, offset, &sessions)
	return sessions, total, err
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return os.ErrNotExist
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableApprovals+` WHERE run_id IN (SELECT id FROM `+tableRuns+` WHERE session_id = ?)`, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableTasks+` WHERE run_id IN (SELECT id FROM `+tableRuns+` WHERE session_id = ?)`, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableRuns+` WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableSessionContexts+` WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableSessionContextLive+` WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableHandoffSegments+` WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableSessionNotices+` WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableSessionComposer+` WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableSessions+` WHERE id = ?`, id); err != nil {
		return err
	}
	return nil
}
