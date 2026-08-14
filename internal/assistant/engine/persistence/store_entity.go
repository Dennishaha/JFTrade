package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

func (s *StoreCore) ListProviders(ctx context.Context) ([]jfadkmodel.Provider, error) {
	var items []jfadkmodel.Provider
	if err := s.ListJSON(ctx, tableProviders, "created_at ASC, id ASC", &items); err != nil {
		return nil, err
	}
	for index := range items {
		items[index] = jfadkmodel.NormalizeProvider(items[index])
		items[index].HasAPIKey = s.SecretHas(items[index].ID)
	}
	if changed := NormalizeDefaultProviderSelection(items); changed {
		if err := s.saveProviderDefaultSelection(ctx, items); err != nil {
			return nil, err
		}
		for index := range items {
			items[index].HasAPIKey = s.SecretHas(items[index].ID)
		}
	}
	SortProvidersDefaultFirst(items)
	return items, nil
}

func (s *StoreCore) SaveProvider(ctx context.Context, req jfadkmodel.ProviderWriteRequest) (jfadkmodel.Provider, error) {
	id := providerWriteID(req)
	existing, ok, err := s.Provider(ctx, id)
	if err != nil {
		return jfadkmodel.Provider{}, err
	}
	if err := validateProviderWriteRequest(req); err != nil {
		return jfadkmodel.Provider{}, err
	}
	now := jfadkmodel.NowString()
	createdAt := now
	if ok {
		createdAt = existing.CreatedAt
	}
	provider := jfadkmodel.Provider{
		ID:                  id,
		DisplayName:         jfadkmodel.DefaultString(req.DisplayName, id),
		BaseURL:             jfadkmodel.NormalizeBaseURL(req.BaseURL),
		Model:               jfadkmodel.DefaultString(req.Model, "gpt-4o-mini"),
		ReasoningConfig:     jfadkmodel.DefaultProviderReasoningConfig(),
		ContextWindowTokens: jfadkmodel.NormalizeContextWindowTokens(req.ContextWindowTokens),
		RequestTimeoutMs:    jfadkmodel.NormalizeProviderRequestTimeoutMs(req.RequestTimeoutMs),
		DefaultHeaders:      jfadkmodel.NormalizeHeaders(req.DefaultHeaders),
		Enabled:             req.Enabled,
		Default:             existing.Default,
		CreatedAt:           createdAt,
		UpdatedAt:           now,
	}
	if ok {
		provider.Capabilities = existing.Capabilities
		provider.ReasoningConfig = existing.ReasoningConfig
		if req.RequestTimeoutMs == 0 {
			provider.RequestTimeoutMs = existing.RequestTimeoutMs
		}
		if req.ContextWindowTokens == 0 {
			provider.ContextWindowTokens = existing.ContextWindowTokens
		}
	}
	if req.ReasoningConfig != nil {
		provider.ReasoningConfig = jfadkmodel.NormalizeProviderReasoningConfig(*req.ReasoningConfig)
	}
	provider = jfadkmodel.NormalizeProvider(provider)
	if strings.TrimSpace(provider.BaseURL) == "" {
		provider.BaseURL = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(req.APIKey) != "" {
		if err := s.SecretSet(id, strings.TrimSpace(req.APIKey)); err != nil {
			return jfadkmodel.Provider{}, err
		}
	}
	provider.HasAPIKey = s.SecretHas(id)
	if !ok {
		providers, err := s.ListProviders(ctx)
		if err != nil {
			return jfadkmodel.Provider{}, err
		}
		provider.Default = len(providers) == 0
	}
	if err := s.SaveJSON(ctx, tableProviders, provider.ID, provider.CreatedAt, provider.UpdatedAt, provider); err != nil {
		return jfadkmodel.Provider{}, err
	}
	if _, err := s.EnsureDefaultProvider(ctx); err != nil {
		return jfadkmodel.Provider{}, err
	}
	saved, ok, err := s.Provider(ctx, provider.ID)
	if err != nil {
		return jfadkmodel.Provider{}, err
	}
	if ok {
		return saved, nil
	}
	return provider, nil
}

func providerWriteID(req jfadkmodel.ProviderWriteRequest) string {
	id := jfadkmodel.NormalizeID(req.ID)
	if id == "" {
		id = jfadkmodel.NormalizeID(req.DisplayName)
	}
	if id == "" {
		return "provider-" + uuid.NewString()
	}
	return id
}

func validateProviderWriteRequest(req jfadkmodel.ProviderWriteRequest) error {
	if strings.TrimSpace(req.BaseURL) != "" {
		if err := ValidateProviderBaseURL(req.BaseURL); err != nil {
			return err
		}
	}
	if req.ReasoningConfig != nil {
		config := jfadkmodel.NormalizeProviderReasoningConfig(*req.ReasoningConfig)
		if err := jfadkmodel.ValidateProviderReasoningConfig(config); err != nil {
			return fmt.Errorf("%w: %w", jfadkmodel.ErrInvalidProviderReasoning, err)
		}
	}
	return validateProviderHeaders(req.DefaultHeaders)
}

func (s *StoreCore) UpdateProviderCapabilities(ctx context.Context, id string, capabilities map[string]bool) (jfadkmodel.Provider, error) {
	provider, ok, err := s.Provider(ctx, id)
	if err != nil {
		return jfadkmodel.Provider{}, err
	}
	if !ok {
		return jfadkmodel.Provider{}, os.ErrNotExist
	}
	provider.Capabilities = capabilities
	provider.UpdatedAt = jfadkmodel.NowString()
	return provider, s.SaveJSON(ctx, tableProviders, provider.ID, provider.CreatedAt, provider.UpdatedAt, provider)
}

func (s *StoreCore) Provider(ctx context.Context, id string) (jfadkmodel.Provider, bool, error) {
	var provider jfadkmodel.Provider
	ok, err := s.GetJSON(ctx, tableProviders, id, &provider)
	if err != nil || !ok {
		return jfadkmodel.Provider{}, ok, err
	}
	provider = jfadkmodel.NormalizeProvider(provider)
	provider.HasAPIKey = s.SecretHas(provider.ID)
	return provider, true, nil
}

func (s *StoreCore) DefaultProvider(ctx context.Context) (jfadkmodel.Provider, bool, error) {
	providers, err := s.ListProviders(ctx)
	if err != nil {
		return jfadkmodel.Provider{}, false, err
	}
	if len(providers) == 0 {
		return jfadkmodel.Provider{}, false, nil
	}
	return providers[0], true, nil
}

func (s *StoreCore) SetDefaultProvider(ctx context.Context, id string) (jfadkmodel.Provider, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return jfadkmodel.Provider{}, os.ErrNotExist
	}
	providers, err := s.loadProvidersCreatedFirst(ctx)
	if err != nil {
		return jfadkmodel.Provider{}, err
	}
	found := -1
	for index := range providers {
		if providers[index].ID == id {
			found = index
			break
		}
	}
	if found < 0 {
		return jfadkmodel.Provider{}, os.ErrNotExist
	}
	for index := range providers {
		providers[index].Default = providers[index].ID == id
	}
	if err := s.saveProviderDefaultSelection(ctx, providers); err != nil {
		return jfadkmodel.Provider{}, err
	}
	provider, ok, err := s.Provider(ctx, id)
	if err != nil {
		return jfadkmodel.Provider{}, err
	}
	if !ok {
		return jfadkmodel.Provider{}, os.ErrNotExist
	}
	return provider, nil
}

func (s *StoreCore) ProviderAPIKey(id string) (string, bool, error) {
	return s.SecretGet(id)
}

func (s *StoreCore) DeleteProvider(ctx context.Context, id string) error {
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
			return fmt.Errorf("%w %q", jfadkmodel.ErrProviderInUse, agent.Name)
		}
	}
	deletedProvider, deletedOK, err := s.Provider(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableProviders+` WHERE id = ?`, id); err != nil {
		return err
	}
	jftradeErr3 := s.SecretDelete(id)
	besteffort.LogError(jftradeErr3)
	if deletedOK && deletedProvider.Default {
		if _, err := s.EnsureDefaultProvider(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *StoreCore) loadProvidersCreatedFirst(ctx context.Context) ([]jfadkmodel.Provider, error) {
	var items []jfadkmodel.Provider
	if err := s.ListJSON(ctx, tableProviders, "created_at ASC, id ASC", &items); err != nil {
		return nil, err
	}
	for index := range items {
		items[index] = jfadkmodel.NormalizeProvider(items[index])
		items[index].HasAPIKey = s.SecretHas(items[index].ID)
	}
	return items, nil
}

// EnsureDefaultProvider repairs the provider default selection.
func (s *StoreCore) EnsureDefaultProvider(ctx context.Context) (bool, error) {
	providers, err := s.loadProvidersCreatedFirst(ctx)
	if err != nil {
		return false, err
	}
	if NormalizeDefaultProviderSelection(providers) {
		return true, s.saveProviderDefaultSelection(ctx, providers)
	}
	return false, nil
}

// NormalizeDefaultProviderSelection ensures exactly one provider is marked as
// default, repairing duplicate or missing selections.
func NormalizeDefaultProviderSelection(providers []jfadkmodel.Provider) bool {
	if len(providers) == 0 {
		return false
	}
	firstDefault := -1
	changed := false
	for index := range providers {
		providers[index] = jfadkmodel.NormalizeProvider(providers[index])
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

func (s *StoreCore) saveProviderDefaultSelection(ctx context.Context, providers []jfadkmodel.Provider) error {
	now := jfadkmodel.NowString()
	for index := range providers {
		provider := jfadkmodel.NormalizeProvider(providers[index])
		provider.HasAPIKey = s.SecretHas(provider.ID)
		provider.UpdatedAt = now
		if err := s.SaveJSON(ctx, tableProviders, provider.ID, provider.CreatedAt, provider.UpdatedAt, provider); err != nil {
			return err
		}
		providers[index] = provider
	}
	return nil
}

// SortProvidersDefaultFirst orders providers by default flag and stable
// created-at/id ties.
func SortProvidersDefaultFirst(providers []jfadkmodel.Provider) {
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

func (s *StoreCore) ListAgents(ctx context.Context) ([]jfadkmodel.Agent, error) {
	var items []jfadkmodel.Agent
	if err := s.ListJSON(ctx, tableAgents, "updated_at DESC, id ASC", &items); err != nil {
		return nil, err
	}
	active := make([]jfadkmodel.Agent, 0, len(items))
	for _, item := range items {
		if item.DeletedAt == nil {
			active = append(active, s.normalizeAgent(item))
		}
	}
	s.sortAgentsPrimaryDefaultFirst(active)
	return active, nil
}

func (s *StoreCore) ListAllAgents(ctx context.Context) ([]jfadkmodel.Agent, error) {
	var items []jfadkmodel.Agent
	if err := s.ListJSON(ctx, tableAgents, "updated_at DESC, id ASC", &items); err != nil {
		return nil, err
	}
	for index := range items {
		items[index] = s.normalizeAgent(items[index])
	}
	s.sortAgentsPrimaryDefaultFirst(items)
	return items, nil
}

func (s *StoreCore) SaveAgent(ctx context.Context, req jfadkmodel.AgentWriteRequest) (jfadkmodel.Agent, error) {
	if err := jfadkmodel.ValidateOptionalReasoningEffort(req.ReasoningEffort); err != nil {
		return jfadkmodel.Agent{}, err
	}
	id := jfadkmodel.NormalizeID(req.ID)
	if id == "" {
		id = jfadkmodel.NormalizeID(req.Name)
	}
	if id == "" {
		id = "agent-" + uuid.NewString()
	}
	now := jfadkmodel.NowString()
	existing, ok, err := s.Agent(ctx, id)
	if err != nil {
		return jfadkmodel.Agent{}, err
	}
	createdAt := now
	if ok {
		createdAt = existing.CreatedAt
	}
	status := strings.ToUpper(strings.TrimSpace(req.Status))
	if status == "" {
		status = jfadkmodel.AgentStatusEnabled
	}
	if status != jfadkmodel.AgentStatusEnabled && status != jfadkmodel.AgentStatusDisabled {
		return jfadkmodel.Agent{}, fmt.Errorf("invalid agent status %q", req.Status)
	}
	if s.isPrimaryBuiltinAgentID(id) && status == jfadkmodel.AgentStatusDisabled {
		return jfadkmodel.Agent{}, fmt.Errorf("%w: primary builtin agent cannot be disabled", jfadkmodel.ErrBuiltinAgentProtected)
	}
	agent := jfadkmodel.Agent{
		ID:                id,
		Name:              jfadkmodel.DefaultString(req.Name, id),
		Instruction:       strings.TrimSpace(req.Instruction),
		ProviderID:        strings.TrimSpace(req.ProviderID),
		Model:             strings.TrimSpace(req.Model),
		ReasoningEffort:   jfadkmodel.NormalizeReasoningEffort(req.ReasoningEffort),
		Tools:             jfadkmodel.NormalizeStringSlice(req.Tools),
		ToolAccessMode:    jfadkmodel.NormalizeToolAccessMode(req.ToolAccessMode, req.Tools),
		Skills:            jfadkmodel.NormalizeStringSlice(req.Skills),
		PermissionMode:    jfadkmodel.NormalizePermissionMode(req.PermissionMode),
		MemoryEnabled:     req.MemoryEnabled,
		RecentUserWindow:  jfadkmodel.NormalizeRecentUserWindow(req.RecentUserWindow),
		WorkMode:          jfadkmodel.NormalizeAgentDefaultWorkMode(req.WorkMode),
		LoopMaxIterations: jfadkmodel.NormalizeLoopMaxIterations(req.LoopMaxIterations),
		Status:            status,
		Builtin:           s.isBuiltinAgentID(id),
		CreatedAt:         createdAt,
		UpdatedAt:         now,
	}
	if ok {
		agent.Builtin = existing.Builtin || agent.Builtin
	}
	provider, providerOK, providerErr := s.Provider(ctx, agent.ProviderID)
	if strings.TrimSpace(agent.ProviderID) == "" && providerErr == nil {
		provider, providerOK, providerErr = s.DefaultProvider(ctx)
	}
	if providerErr != nil {
		return jfadkmodel.Agent{}, providerErr
	}
	if providerOK && agent.ReasoningEffort != "" {
		if _, supported := jfadkmodel.ProviderReasoningMappingValue(provider.ReasoningConfig, agent.ReasoningEffort); !supported {
			return jfadkmodel.Agent{}, fmt.Errorf("%w: %s", jfadkmodel.ErrProviderReasoningUnsupported, agent.ReasoningEffort)
		}
	}
	if agent.Instruction == "" {
		agent.Instruction = jfadkmodel.DefaultAgentInstruction()
	}
	if ok && req.RecentUserWindow == 0 {
		agent.RecentUserWindow = jfadkmodel.NormalizeRecentUserWindow(existing.RecentUserWindow)
	}
	if ok && strings.TrimSpace(req.WorkMode) == "" {
		agent.WorkMode = jfadkmodel.NormalizeAgentDefaultWorkMode(existing.WorkMode)
	}
	if ok && req.LoopMaxIterations == 0 {
		agent.LoopMaxIterations = jfadkmodel.NormalizeLoopMaxIterations(existing.LoopMaxIterations)
	}
	agent = s.normalizeAgent(agent)
	return agent, s.SaveJSON(ctx, tableAgents, agent.ID, agent.CreatedAt, agent.UpdatedAt, agent)
}

func (s *StoreCore) sortAgentsPrimaryDefaultFirst(agents []jfadkmodel.Agent) {
	sort.SliceStable(agents, func(i, j int) bool {
		leftDefault := s.isPrimaryBuiltinAgentID(agents[i].ID)
		rightDefault := s.isPrimaryBuiltinAgentID(agents[j].ID)
		if leftDefault != rightDefault {
			return leftDefault
		}
		return false
	})
}

func (s *StoreCore) EnsureAgent(ctx context.Context, req jfadkmodel.AgentWriteRequest) (jfadkmodel.Agent, error) {
	id := jfadkmodel.NormalizeID(req.ID)
	if id == "" {
		id = jfadkmodel.NormalizeID(req.Name)
	}
	if id != "" {
		if existing, ok, err := s.Agent(ctx, id); err != nil || ok {
			return existing, err
		}
	}
	return s.SaveAgent(ctx, req)
}

func (s *StoreCore) Agent(ctx context.Context, id string) (jfadkmodel.Agent, bool, error) {
	var agent jfadkmodel.Agent
	ok, err := s.GetJSON(ctx, tableAgents, id, &agent)
	if err != nil || !ok {
		return jfadkmodel.Agent{}, ok, err
	}
	return s.normalizeAgent(agent), true, nil
}

func (s *StoreCore) DefaultAgent(ctx context.Context) (jfadkmodel.Agent, error) {
	agents, err := s.ListAgents(ctx)
	if err != nil {
		return jfadkmodel.Agent{}, err
	}
	for _, agent := range agents {
		if agent.ID == s.defaultAgentID() && agent.Status == jfadkmodel.AgentStatusEnabled {
			return agent, nil
		}
	}
	for _, agent := range agents {
		if agent.Status == jfadkmodel.AgentStatusEnabled {
			return agent, nil
		}
	}
	if template, ok := s.builtinAgentTemplate(s.defaultAgentID()); ok {
		return s.SaveAgent(ctx, template)
	}
	return s.SaveAgent(ctx, jfadkmodel.AgentWriteRequest{ID: s.defaultAgentID(), Name: "默认助手", Instruction: jfadkmodel.DefaultAgentInstruction(), PermissionMode: jfadkmodel.PermissionModeApproval, Status: jfadkmodel.AgentStatusEnabled})
}

func (s *StoreCore) DeleteAgent(ctx context.Context, id string) error {
	agent, ok, err := s.Agent(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return os.ErrNotExist
	}
	if agent.Builtin || s.isBuiltinAgentID(agent.ID) {
		return fmt.Errorf("%w: builtin agent cannot be deleted", jfadkmodel.ErrBuiltinAgentProtected)
	}
	now := jfadkmodel.NowString()
	agent.Status = jfadkmodel.AgentStatusDisabled
	agent.DeletedAt = &now
	agent.UpdatedAt = now
	return s.SaveJSON(ctx, tableAgents, agent.ID, agent.CreatedAt, agent.UpdatedAt, agent)
}

func (s *StoreCore) CreateSession(ctx context.Context, agentID string, title string) (jfadkmodel.Session, error) {
	return s.CreateSessionWithSource(ctx, agentID, title, "", "")
}

func (s *StoreCore) CreateSessionWithSource(ctx context.Context, agentID string, title string, workflowID string, workflowName string) (jfadkmodel.Session, error) {
	now := jfadkmodel.NowString()
	session := jfadkmodel.Session{
		ID:           "session-" + uuid.NewString(),
		AgentID:      strings.TrimSpace(agentID),
		Title:        jfadkmodel.DefaultString(title, "新的 ADK 会话"),
		WorkflowID:   strings.TrimSpace(workflowID),
		WorkflowName: strings.TrimSpace(workflowName),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return jfadkmodel.Session{}, err
	}
	_, err = s.DB().ExecContext(ctx, `INSERT INTO `+tableSessions+` (id, agent_id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET agent_id = excluded.agent_id, payload_json = excluded.payload_json, updated_at = excluded.updated_at`, session.ID, session.AgentID, string(payload), session.CreatedAt, session.UpdatedAt)
	return session, err
}

func (s *StoreCore) RenameSession(ctx context.Context, id string, title string) (jfadkmodel.Session, error) {
	session, ok, err := s.Session(ctx, id)
	if err != nil {
		return jfadkmodel.Session{}, err
	}
	if !ok {
		return jfadkmodel.Session{}, os.ErrNotExist
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return jfadkmodel.Session{}, fmt.Errorf("session title is required")
	}
	if len([]rune(title)) > 80 {
		title = string([]rune(title)[:80])
	}
	session.Title = title
	session.UpdatedAt = jfadkmodel.NowString()
	payload, err := json.Marshal(session)
	if err != nil {
		return jfadkmodel.Session{}, err
	}
	_, err = s.DB().ExecContext(ctx, `UPDATE `+tableSessions+` SET payload_json = ?, updated_at = ? WHERE id = ?`, string(payload), session.UpdatedAt, session.ID)
	return session, err
}

func (s *StoreCore) Session(ctx context.Context, id string) (jfadkmodel.Session, bool, error) {
	var session jfadkmodel.Session
	ok, err := s.GetJSON(ctx, tableSessions, id, &session)
	return session, ok, err
}

func (s *StoreCore) ListSessions(ctx context.Context) ([]jfadkmodel.Session, error) {
	var sessions []jfadkmodel.Session
	return sessions, s.ListJSON(ctx, tableSessions, "updated_at DESC, id ASC", &sessions)
}

func (s *StoreCore) ListSessionsPage(ctx context.Context, agentID string, query string, limit int, offset int) ([]jfadkmodel.Session, int, error) {
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
	var sessions []jfadkmodel.Session
	total, err := s.ListJSONPage(ctx, tableSessions, clauses, args, "updated_at DESC, id ASC", limit, offset, &sessions)
	return sessions, total, err
}

func (s *StoreCore) DeleteSession(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return os.ErrNotExist
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableToolInvocations+` WHERE run_id IN (SELECT id FROM `+tableRuns+` WHERE session_id = ?)`, id); err != nil {
		return err
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableRunLeases+` WHERE run_id IN (SELECT id FROM `+tableRuns+` WHERE session_id = ?)`, id); err != nil {
		return err
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableApprovals+` WHERE run_id IN (SELECT id FROM `+tableRuns+` WHERE session_id = ?)`, id); err != nil {
		return err
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableTasks+` WHERE run_id IN (SELECT id FROM `+tableRuns+` WHERE session_id = ?)`, id); err != nil {
		return err
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableRuns+` WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableSessionContexts+` WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableSessionContextLive+` WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableHandoffSegments+` WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableSessionNotices+` WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableSessionComposer+` WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableSessions+` WHERE id = ?`, id); err != nil {
		return err
	}
	return nil
}
