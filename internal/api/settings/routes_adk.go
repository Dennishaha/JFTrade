package settings

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/settings"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

// ── ADK ──

// handleADKRuntimeSettings godoc
// @Summary 读取 ADK 运行时设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=jfsettings.ADKRuntimeSettings}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/settings/adk [get]
func handleADKRuntimeSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.GetADKRuntimeSettings())
	}
}

// handleSaveADKRuntimeSettings godoc
// @Summary 保存 ADK 运行时设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body ADKRuntimeSettingsWriteRequest true "ADK 运行时设置"
// @Success 200 {object} httpserver.Envelope{data=jfsettings.ADKRuntimeSettings}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/settings/adk [put]
func handleSaveADKRuntimeSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ADKRuntimeSettingsWriteRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid adk payload")
			return
		}
		result, err := svc.SaveADKRuntimeSettings(input.settings())
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// ── Local MCP Server ──

// handleMCPServerSettings godoc
// @Summary 读取本机 MCP Server 设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=jfsettings.MCPServerSettingsSnapshot}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/settings/adk/mcp [get]
func handleMCPServerSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := svc.GetMCPServerSettingsSnapshot()
		if !snapshot.Status.Running && snapshot.Status.LastError != "" {
			httpserver.WriteError(c, http.StatusServiceUnavailable, "MCP_SERVER_UNAVAILABLE", snapshot.Status.LastError)
			return
		}
		httpserver.WriteOK(c, snapshot)
	}
}

// handleSaveMCPServerSettings godoc
// @Summary 保存本机 MCP Server 设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body jfsettings.MCPServerSettingsUpdate true "本机 MCP Server 设置"
// @Success 200 {object} httpserver.Envelope{data=jfsettings.MCPServerSettingsSnapshot}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/settings/adk/mcp [put]
func handleSaveMCPServerSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input jfsettings.MCPServerSettingsUpdate
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid MCP server payload")
			return
		}
		result, err := svc.SaveMCPServerSettings(input)
		if err != nil {
			status := http.StatusInternalServerError
			code := "MCP_SERVER_SETTINGS_FAILED"
			if errors.Is(err, srv.ErrMCPServerPortInvalid) ||
				errors.Is(err, srv.ErrMCPServerAuthModeInvalid) ||
				errors.Is(err, srv.ErrMCPServerTokenRequired) {
				status = http.StatusBadRequest
				code = "MCP_SERVER_SETTINGS_REJECTED"
			} else if errors.Is(err, srv.ErrMCPServerRuntimeUpdate) ||
				errors.Is(err, srv.ErrMCPServerRuntimeUnavailable) {
				status = http.StatusBadGateway
				code = "MCP_SERVER_RUNTIME_UNAVAILABLE"
			}
			httpserver.WriteError(c, status, code, err.Error())
			return
		}
		snapshot := svc.GetMCPServerSettingsSnapshot()
		if snapshot.Settings.Enabled && !snapshot.Status.Running && snapshot.Status.LastError != "" {
			httpserver.WriteError(c, http.StatusServiceUnavailable, "MCP_SERVER_UNAVAILABLE", snapshot.Status.LastError)
			return
		}
		httpserver.WriteOK(c, jfsettings.MCPServerSettingsSnapshot{
			Settings: result,
			Status:   snapshot.Status,
		})
	}
}

// handleResetMCPServerToken godoc
// @Summary 重置本机 MCP Server Bearer Token
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=jfsettings.MCPServerTokenResetResult}
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/settings/adk/mcp/token/reset [post]
func handleResetMCPServerToken(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, token, err := svc.ResetMCPServerToken()
		if err != nil {
			status := http.StatusInternalServerError
			code := "MCP_SERVER_TOKEN_RESET_FAILED"
			if errors.Is(err, srv.ErrMCPServerRuntimeUpdate) ||
				errors.Is(err, srv.ErrMCPServerRuntimeUnavailable) {
				status = http.StatusBadGateway
				code = "MCP_SERVER_RUNTIME_UNAVAILABLE"
			}
			httpserver.WriteError(c, status, code, err.Error())
			return
		}
		snapshot := svc.GetMCPServerSettingsSnapshot()
		if !snapshot.Status.Running && snapshot.Status.LastError != "" {
			httpserver.WriteError(c, http.StatusServiceUnavailable, "MCP_SERVER_UNAVAILABLE", snapshot.Status.LastError)
			return
		}
		httpserver.WriteOK(c, jfsettings.MCPServerTokenResetResult{
			Settings: settings,
			Status:   snapshot.Status,
			Token:    token,
		})
	}
}
