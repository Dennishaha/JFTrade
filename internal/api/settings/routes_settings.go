package settings

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	"github.com/jftrade/jftrade-main/internal/api/middleware"
	srv "github.com/jftrade/jftrade-main/internal/settings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

// ── UI Appearance ──

// handleUIAppearance godoc
// @Summary 读取 UI 颜色配置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/ui [get]
func handleUIAppearance(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, map[string]any{"appearance": svc.GetAppearance()})
	}
}

// handleSaveUIAppearance godoc
// @Summary 保存 UI 颜色配置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body UIAppearanceSettingsWriteRequest true "UI 配置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/ui [put]
func handleSaveUIAppearance(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Appearance jfsettings.UIAppearanceSettings `json:"appearance"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid appearance payload")
			return
		}
		result, err := svc.SaveAppearance(payload.Appearance)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, map[string]any{"appearance": result})
	}
}

// ── Onboarding ──

// handleOnboardingState godoc
// @Summary 读取新手引导状态
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/onboarding [get]
func handleOnboardingState(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.OnboardingState(c.Request.Context()))
	}
}

// handleSaveOnboarding godoc
// @Summary 保存新手引导状态
// @Tags settings
// @Accept json
// @Produce json
// @Param request body OnboardingWriteRequest true "引导状态"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/onboarding [put]
func handleSaveOnboarding(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Completed    bool   `json:"completed"`
			Dismissed    bool   `json:"dismissed"`
			LastBrokerID string `json:"lastBrokerId"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid onboarding payload")
			return
		}

		existing := svc.GetOnboarding()
		now := time.Now().UTC().Format(time.RFC3339Nano)
		next := existing
		next.LastBrokerID = payload.LastBrokerID
		if strings.TrimSpace(next.LastBrokerID) == "" {
			next.LastBrokerID = existing.LastBrokerID
		}
		if payload.Completed || payload.Dismissed {
			next.Completed = true
			if payload.Dismissed {
				next.DismissedAt = now
			}
			if next.CompletedAt == "" {
				next.CompletedAt = now
			}
		} else {
			next.Completed = false
			next.CompletedAt = ""
			next.DismissedAt = ""
		}

		_, err := svc.SaveOnboarding(next)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, svc.OnboardingState(c.Request.Context()))
	}
}

// ── Execution ──

// handleExecutionSettings godoc
// @Summary 读取执行设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/execution [get]
func handleExecutionSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.GetExecutionSettings())
	}
}

// handleSaveExecutionSettings godoc
// @Summary 保存执行设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body jfsettings.ExecutionSettings true "执行设置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/execution [put]
func handleSaveExecutionSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input jfsettings.ExecutionSettings
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid execution payload")
			return
		}
		result, err := svc.SaveExecutionSettings(input)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// ── Security ──

// handleSecuritySettings godoc
// @Summary 读取安全设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=jfsettings.SecuritySettings}
// @Router /api/v1/settings/security [get]
func handleSecuritySettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.GetSecuritySettings())
	}
}

// handleSaveSecuritySettings godoc
// @Summary 保存安全设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body jfsettings.SecuritySettingsUpdate true "Web 访问设置（新密码仅写入）"
// @Success 200 {object} httpserver.Envelope{data=jfsettings.SecuritySettings}
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/security [put]
func handleSaveSecuritySettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !middleware.IsRequestTrustedHost(c.Request) {
			httpserver.WriteError(c, 403, "WEB_ACCESS_SETTINGS_DESKTOP_ONLY", "Web access settings can only be changed from the JFTrade desktop app")
			return
		}
		var input jfsettings.SecuritySettingsUpdate
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid security payload")
			return
		}
		result, err := svc.SaveSecuritySettings(input)
		if err != nil {
			if errors.Is(err, srv.ErrWebAccessRuntimeUpdate) {
				httpserver.WriteError(c, http.StatusConflict, "WEB_ACCESS_LISTENER_UPDATE_FAILED", err.Error())
				return
			}
			if errors.Is(err, srv.ErrWebAccessPortInvalid) {
				httpserver.WriteError(c, 400, "INVALID_WEB_ACCESS_PORT", err.Error())
				return
			}
			if errors.Is(err, srv.ErrWebAccessPasswordRequired) ||
				errors.Is(err, srv.ErrWebAccessPasswordTooShort) ||
				errors.Is(err, srv.ErrWebAccessPasswordTooLong) {
				httpserver.WriteError(c, 400, "INVALID_WEB_ACCESS_PASSWORD", err.Error())
				return
			}
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// ── System Notifications ──

// handleSystemNotificationSettings godoc
// @Summary 读取系统通知设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/system-notifications [get]
func handleSystemNotificationSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.GetSystemNotificationSettings())
	}
}

// handleSaveSystemNotificationSettings godoc
// @Summary 保存系统通知设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body jfsettings.SystemNotificationSettings true "系统通知设置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/system-notifications [put]
func handleSaveSystemNotificationSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input jfsettings.SystemNotificationSettings
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid system notification payload")
			return
		}
		result, err := svc.SaveSystemNotificationSettings(input)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}
