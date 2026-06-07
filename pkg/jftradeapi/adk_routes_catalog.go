package jftradeapi

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

type adkInstallSkillRequest struct {
	URL string `json:"url"`
}

// handleADKSnapshot godoc
// @Summary 读取 ADK 快照
// @Tags adk
// @Produce json
// @Success 200 {object} envelope
// @Failure 500 {object} envelope
// @Router /api/v1/adk [get]
func (s *Server) handleADKSnapshot(c *gin.Context) {
	snapshot, err := s.adkRuntime.Snapshot(c.Request.Context())
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "ADK_SNAPSHOT_FAILED", err.Error())
		return
	}
	s.writeOK(c, snapshot)
}

func (s *Server) handleADKTools(c *gin.Context) {
	s.writeOK(c, map[string]any{"tools": s.adkRuntime.Tools().List()})
}

// handleADKProviders godoc
// @Summary 读取 ADK Provider 列表
// @Tags adk
// @Produce json
// @Success 200 {object} envelope
// @Failure 500 {object} envelope
// @Router /api/v1/adk/providers [get]
func (s *Server) handleADKProviders(c *gin.Context) {
	items, err := s.adkRuntime.Store().ListProviders(c.Request.Context())
	writeADKListOrError(s, c, "ADK_PROVIDER_LIST_FAILED", "providers", items, err)
}

func (s *Server) handleADKTestProvider(c *gin.Context) {
	var uri providerURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.ProviderID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "providerId is invalid")
		return
	}
	id := uri.ProviderID
	result, err := s.adkRuntime.TestProvider(c.Request.Context(), id)
	if err != nil {
		s.writeError(c, http.StatusBadGateway, "ADK_PROVIDER_TEST_FAILED", err.Error())
		return
	}
	s.writeOK(c, result)
}

func (s *Server) handleADKDeleteProvider(c *gin.Context) {
	var uri providerURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.ProviderID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "providerId is invalid")
		return
	}
	id := uri.ProviderID
	if err := s.adkRuntime.Store().DeleteProvider(c.Request.Context(), id); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "used by agent") {
			status = http.StatusConflict
		}
		s.writeError(c, status, "ADK_PROVIDER_DELETE_FAILED", err.Error())
		return
	}
	s.writeOK(c, map[string]any{"deleted": true, "id": id})
}

// handleADKAgents godoc
// @Summary 读取 ADK Agent 列表
// @Tags adk
// @Produce json
// @Param status query string false "Agent 状态过滤"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 500 {object} envelope
// @Router /api/v1/adk/agents [get]
func (s *Server) handleADKAgents(c *gin.Context) {
	var query adkAgentsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid agents query")
		return
	}
	items, err := s.adkRuntime.Store().ListAgents(c.Request.Context())
	if err == nil {
		items = filterADKAgents(items, query.Status)
	}
	writeADKPagedListOrError(s, c, "ADK_AGENT_LIST_FAILED", "agents", items, err)
}

func (s *Server) handleADKDeleteAgent(c *gin.Context) {
	var uri agentURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.AgentID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "agentId is invalid")
		return
	}
	id := uri.AgentID
	if err := s.adkRuntime.Store().DeleteAgent(c.Request.Context(), id); err != nil {
		s.writeError(c, http.StatusInternalServerError, "ADK_AGENT_DELETE_FAILED", err.Error())
		return
	}
	s.writeOK(c, map[string]any{"deleted": true, "id": id})
}

func (s *Server) handleADKSkills(c *gin.Context) {
	items, err := s.adkRuntime.Skills().List(c.Request.Context())
	writeADKListOrError(s, c, "ADK_SKILL_LIST_FAILED", "skills", items, err)
}

func (s *Server) handleADKInstallSkill(c *gin.Context) {
	var payload adkInstallSkillRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid skill install payload")
		return
	}
	skill, err := s.adkRuntime.Skills().InstallURL(c.Request.Context(), payload.URL)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "ADK_SKILL_INSTALL_FAILED", err.Error())
		return
	}
	s.adkRuntime.RecordAudit(c.Request.Context(), "skill.installed", skill.ID, "ADK skill installed.", map[string]any{"source": skill.Source})
	s.writeOK(c, skill)
}

func (s *Server) handleADKDeleteSkill(c *gin.Context) {
	var uri skillURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.SkillID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "skillId is invalid")
		return
	}
	id := uri.SkillID
	if err := s.adkRuntime.Skills().Uninstall(c.Request.Context(), id); err != nil {
		s.writeError(c, http.StatusInternalServerError, "ADK_SKILL_UNINSTALL_FAILED", err.Error())
		return
	}
	s.adkRuntime.RecordAudit(c.Request.Context(), "skill.uninstalled", id, "ADK skill uninstalled.", nil)
	s.writeOK(c, map[string]any{"deleted": true, "id": id})
}

func (s *Server) handleADKSaveProvider(c *gin.Context) {
	var payload jfadk.ProviderWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil && err != io.EOF {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid provider payload")
		return
	}
	if c.Request.Method == http.MethodPut {
		var uri providerURI
		if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.ProviderID) == "" {
			s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "providerId is invalid")
			return
		}
		payload.ID = uri.ProviderID
	}
	provider, err := s.adkRuntime.Store().SaveProvider(c.Request.Context(), payload)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "ADK_PROVIDER_SAVE_FAILED", err.Error())
		return
	}
	s.adkRuntime.RecordAudit(c.Request.Context(), "provider.saved", provider.ID, "ADK provider saved.", map[string]any{"enabled": provider.Enabled})
	s.writeOK(c, provider)
}

func (s *Server) handleADKSaveAgent(c *gin.Context) {
	var payload jfadk.AgentWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil && err != io.EOF {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid agent payload")
		return
	}
	if c.Request.Method == http.MethodPut {
		var uri agentURI
		if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.AgentID) == "" {
			s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "agentId is invalid")
			return
		}
		payload.ID = uri.AgentID
	}
	if err := s.validateADKAgentPayload(c.Request.Context(), payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	agent, err := s.adkRuntime.Store().SaveAgent(c.Request.Context(), payload)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "ADK_AGENT_SAVE_FAILED", err.Error())
		return
	}
	s.adkRuntime.RecordAudit(c.Request.Context(), "agent.saved", agent.ID, "ADK agent saved.", map[string]any{"status": agent.Status, "permissionMode": agent.PermissionMode})
	s.writeOK(c, agent)
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
