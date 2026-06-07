package jftradeapi

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type managedBrokerAccountWriteRequest struct {
	BrokerID           string `json:"brokerId"`
	AccountID          string `json:"accountId"`
	DisplayName        string `json:"displayName"`
	TradingEnvironment string `json:"tradingEnvironment"`
	Market             string `json:"market"`
	SecurityFirm       string `json:"securityFirm"`
	Enabled            bool   `json:"enabled"`
}

type brokerIntegrationSaveRequest struct {
	Enabled bool                  `json:"enabled"`
	Config  FutuIntegrationConfig `json:"config"`
}

type uiAppearanceSettingsWriteRequest struct {
	Appearance UIAppearanceSettings `json:"appearance"`
}

type onboardingWriteRequest struct {
	Completed    bool   `json:"completed"`
	Dismissed    bool   `json:"dismissed"`
	LastBrokerID string `json:"lastBrokerId"`
}

func (payload managedBrokerAccountWriteRequest) toManagedBrokerAccount() ManagedBrokerAccount {
	return ManagedBrokerAccount{
		BrokerID:           payload.BrokerID,
		AccountID:          payload.AccountID,
		DisplayName:        payload.DisplayName,
		TradingEnvironment: payload.TradingEnvironment,
		Market:             payload.Market,
		SecurityFirm:       stringPointerOrNil(payload.SecurityFirm),
		Enabled:            payload.Enabled,
	}
}

// handleSaveBrokerIntegration godoc
// @Summary 保存 broker 集成
// @Tags settings
// @Accept json
// @Produce json
// @Param brokerId path string true "Broker 标识"
// @Param request body brokerIntegrationSaveRequest true "Broker 集成配置"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/settings/brokers/{brokerId}/integration [put]
func (s *Server) handleSaveBrokerIntegration(c *gin.Context) {
	var payload brokerIntegrationSaveRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	integration, err := s.store.saveIntegration(BrokerIntegration{BrokerID: "futu", Enabled: payload.Enabled, Config: payload.Config})
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(c, integration)
}

// handleSaveUIAppearance godoc
// @Summary 保存 UI 颜色配置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body uiAppearanceSettingsWriteRequest true "UI 配置"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/settings/ui [put]
func (s *Server) handleSaveUIAppearance(c *gin.Context) {
	var payload uiAppearanceSettingsWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	appearance, err := s.store.saveAppearance(payload.Appearance)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}

	s.writeOK(c, map[string]any{"appearance": appearance})
}

// handleSaveOnboarding godoc
// @Summary 保存新手引导状态
// @Tags settings
// @Accept json
// @Produce json
// @Param request body onboardingWriteRequest true "引导状态"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/settings/onboarding [put]
func (s *Server) handleSaveOnboarding(c *gin.Context) {
	var payload onboardingWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	existing := s.store.onboarding()
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

	onboarding, err := s.store.saveOnboarding(next)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(c, s.onboardingStateFromSettings(c.Request.Context(), onboarding))
}

// handleSaveExecutionSettings godoc
// @Summary 保存执行设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body ExecutionSettings true "执行设置"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/settings/execution [put]
func (s *Server) handleSaveExecutionSettings(c *gin.Context) {
	var payload ExecutionSettings
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	settings, err := s.store.saveExecutionSettings(payload)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	if s.executionOrders != nil {
		s.executionOrders.configureSeenFillRetention(settings.SeenFillRetentionDays)
	}
	s.writeOK(c, settings)
}

// handleSaveSecuritySettings godoc
// @Summary 保存安全设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body SecuritySettings true "安全设置"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/settings/security [put]
func (s *Server) handleSaveSecuritySettings(c *gin.Context) {
	var payload SecuritySettings
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	settings, err := s.store.saveSecuritySettings(payload)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.applySecuritySettings(settings)
	s.writeOK(c, settings)
}

// handleCreateManagedBrokerAccount godoc
// @Summary 创建托管账户
// @Tags settings
// @Accept json
// @Produce json
// @Param request body managedBrokerAccountWriteRequest true "托管账户"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/settings/broker-accounts [post]
func (s *Server) handleCreateManagedBrokerAccount(c *gin.Context) {
	var payload managedBrokerAccountWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if strings.TrimSpace(payload.AccountID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "accountId is required")
		return
	}
	account, err := s.store.createManagedAccount(payload.toManagedBrokerAccount())
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(c, account)
}

// handleUpdateManagedBrokerAccount godoc
// @Summary 更新托管账户
// @Tags settings
// @Accept json
// @Produce json
// @Param accountRecordId path string true "托管账户记录 ID"
// @Param request body managedBrokerAccountWriteRequest true "托管账户"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/settings/broker-accounts/{accountRecordId} [put]
func (s *Server) handleUpdateManagedBrokerAccount(c *gin.Context) {
	var uri accountRecordURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "accountRecordId is invalid")
		return
	}
	accountID := strings.TrimSpace(uri.AccountRecordID)
	if accountID == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "accountRecordId is required")
		return
	}
	var payload managedBrokerAccountWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	account, err := s.store.updateManagedAccount(accountID, payload.toManagedBrokerAccount())
	if errors.Is(err, os.ErrNotExist) {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "managed broker account not found")
		return
	}
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(c, account)
}

// handleDeleteManagedBrokerAccount godoc
// @Summary 删除托管账户
// @Tags settings
// @Produce json
// @Param accountRecordId path string true "托管账户记录 ID"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/settings/broker-accounts/{accountRecordId} [delete]
func (s *Server) handleDeleteManagedBrokerAccount(c *gin.Context) {
	var uri accountRecordURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "accountRecordId is invalid")
		return
	}
	accountID := strings.TrimSpace(uri.AccountRecordID)
	if accountID == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "accountRecordId is required")
		return
	}
	if err := s.store.deleteManagedAccount(accountID); errors.Is(err, os.ErrNotExist) {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "managed broker account not found")
		return
	} else if err != nil {
		s.writeError(c, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(c, map[string]any{"deleted": true, "id": accountID})
}
