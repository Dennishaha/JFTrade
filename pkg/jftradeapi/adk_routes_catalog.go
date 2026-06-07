package jftradeapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func (s *Server) handleADKSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.adkRuntime.Snapshot(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_SNAPSHOT_FAILED", err.Error())
		return
	}
	s.writeOK(w, snapshot)
}

func (s *Server) handleADKTools(w http.ResponseWriter, _ *http.Request) {
	s.writeOK(w, map[string]any{"tools": s.adkRuntime.Tools().List()})
}

func (s *Server) handleADKProviders(w http.ResponseWriter, r *http.Request) {
	items, err := s.adkRuntime.Store().ListProviders(r.Context())
	writeADKListOrError(s, w, "ADK_PROVIDER_LIST_FAILED", "providers", items, err)
}

func (s *Server) handleADKTestProvider(w http.ResponseWriter, r *http.Request) {
	id, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/adk/providers/", "/test"))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "providerId is invalid")
		return
	}
	result, err := s.adkRuntime.TestProvider(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "ADK_PROVIDER_TEST_FAILED", err.Error())
		return
	}
	s.writeOK(w, result)
}

func (s *Server) handleADKDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/adk/providers/"))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "providerId is invalid")
		return
	}
	if err := s.adkRuntime.Store().DeleteProvider(r.Context(), id); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "used by agent") {
			status = http.StatusConflict
		}
		s.writeError(w, status, "ADK_PROVIDER_DELETE_FAILED", err.Error())
		return
	}
	s.writeOK(w, map[string]any{"deleted": true, "id": id})
}

func (s *Server) handleADKAgents(w http.ResponseWriter, r *http.Request) {
	items, err := s.adkRuntime.Store().ListAgents(r.Context())
	if err == nil {
		items = filterADKAgents(items, r.URL.Query().Get("status"))
	}
	writeADKPagedListOrError(s, w, "ADK_AGENT_LIST_FAILED", "agents", items, err, r)
}

func (s *Server) handleADKDeleteAgent(w http.ResponseWriter, r *http.Request) {
	id, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/adk/agents/"))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "agentId is invalid")
		return
	}
	if err := s.adkRuntime.Store().DeleteAgent(r.Context(), id); err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_AGENT_DELETE_FAILED", err.Error())
		return
	}
	s.writeOK(w, map[string]any{"deleted": true, "id": id})
}

func (s *Server) handleADKSkills(w http.ResponseWriter, r *http.Request) {
	items, err := s.adkRuntime.Skills().List(r.Context())
	writeADKListOrError(s, w, "ADK_SKILL_LIST_FAILED", "skills", items, err)
}

func (s *Server) handleADKInstallSkill(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid skill install payload")
		return
	}
	skill, err := s.adkRuntime.Skills().InstallURL(r.Context(), payload.URL)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "ADK_SKILL_INSTALL_FAILED", err.Error())
		return
	}
	s.adkRuntime.RecordAudit(r.Context(), "skill.installed", skill.ID, "ADK skill installed.", map[string]any{"source": skill.Source})
	s.writeOK(w, skill)
}

func (s *Server) handleADKDeleteSkill(w http.ResponseWriter, r *http.Request) {
	id, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/adk/skills/"))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "skillId is invalid")
		return
	}
	if err := s.adkRuntime.Skills().Uninstall(r.Context(), id); err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_SKILL_UNINSTALL_FAILED", err.Error())
		return
	}
	s.adkRuntime.RecordAudit(r.Context(), "skill.uninstalled", id, "ADK skill uninstalled.", nil)
	s.writeOK(w, map[string]any{"deleted": true, "id": id})
}

func (s *Server) handleADKSaveProvider(w http.ResponseWriter, r *http.Request) {
	var payload jfadk.ProviderWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err != io.EOF {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid provider payload")
		return
	}
	if r.Method == http.MethodPut {
		id, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/adk/providers/"))
		if err != nil || strings.TrimSpace(id) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "providerId is invalid")
			return
		}
		payload.ID = id
	}
	provider, err := s.adkRuntime.Store().SaveProvider(r.Context(), payload)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_PROVIDER_SAVE_FAILED", err.Error())
		return
	}
	s.adkRuntime.RecordAudit(r.Context(), "provider.saved", provider.ID, "ADK provider saved.", map[string]any{"enabled": provider.Enabled})
	s.writeOK(w, provider)
}

func (s *Server) handleADKSaveAgent(w http.ResponseWriter, r *http.Request) {
	var payload jfadk.AgentWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err != io.EOF {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid agent payload")
		return
	}
	if r.Method == http.MethodPut {
		id, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/adk/agents/"))
		if err != nil || strings.TrimSpace(id) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "agentId is invalid")
			return
		}
		payload.ID = id
	}
	if err := s.validateADKAgentPayload(r.Context(), payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	agent, err := s.adkRuntime.Store().SaveAgent(r.Context(), payload)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_AGENT_SAVE_FAILED", err.Error())
		return
	}
	s.adkRuntime.RecordAudit(r.Context(), "agent.saved", agent.ID, "ADK agent saved.", map[string]any{"status": agent.Status, "permissionMode": agent.PermissionMode})
	s.writeOK(w, agent)
}

func (s *Server) validateADKAgentPayload(ctx context.Context, payload jfadk.AgentWriteRequest) error {
	status := strings.ToUpper(strings.TrimSpace(payload.Status))
	if status != "" && status != jfadk.AgentStatusEnabled && status != jfadk.AgentStatusDisabled {
		return errString("invalid agent status")
	}
	if strings.TrimSpace(payload.ProviderID) != "" {
		provider, ok, err := s.adkRuntime.Store().Provider(ctx, payload.ProviderID)
		if err != nil {
			return err
		} else if !ok {
			return errString("provider not found")
		} else if strings.ToUpper(strings.TrimSpace(payload.Status)) != jfadk.AgentStatusDisabled {
			if !provider.Enabled {
				return errString("provider is disabled")
			}
			if _, hasKey, keyErr := s.adkRuntime.Store().ProviderAPIKey(provider.ID); keyErr != nil {
				return keyErr
			} else if !hasKey {
				return errString("provider API key is not configured")
			}
		}
	}
	for _, name := range payload.Tools {
		if _, ok := s.adkRuntime.Tools().CanonicalName(name); !ok {
			return errString("unknown ADK tool: " + strings.TrimSpace(name))
		}
	}
	for _, id := range payload.Skills {
		if _, ok, err := s.adkRuntime.Skills().Get(ctx, id); err != nil {
			return err
		} else if !ok {
			return errString("unknown ADK skill: " + strings.TrimSpace(id))
		}
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }
