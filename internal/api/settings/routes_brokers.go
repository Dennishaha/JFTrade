package settings

import (
	"errors"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/settings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

// ── Broker ──

// handleBrokerSettings godoc
// @Summary 读取 broker 设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/brokers [get]
func handleBrokerSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.BrokerSettings())
	}
}

// handleSaveBrokerIntegration godoc
// @Summary 保存 broker 集成
// @Tags settings
// @Accept json
// @Produce json
// @Param brokerId path string true "Broker 标识"
// @Param request body BrokerIntegrationSaveRequest true "Broker 集成配置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/brokers/{brokerId}/integration [put]
func handleSaveBrokerIntegration(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			BrokerID string `uri:"brokerId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid broker id")
			return
		}
		var input jfsettings.BrokerIntegration
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid integration payload")
			return
		}
		input.BrokerID = uri.BrokerID
		result, err := svc.SaveIntegration(input)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// ── Managed Accounts ──

// handleCreateManagedBrokerAccount godoc
// @Summary 创建托管账户
// @Tags settings
// @Accept json
// @Produce json
// @Param request body ManagedBrokerAccountWriteRequest true "托管账户"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/broker-accounts [post]
func handleCreateManagedBrokerAccount(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input jfsettings.ManagedBrokerAccount
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid account payload")
			return
		}
		result, err := svc.CreateManagedAccount(input)
		if err != nil {
			if errors.Is(err, srv.ErrBadRequest) {
				httpserver.WriteError(c, 400, "BAD_REQUEST", err.Error())
				return
			}
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleUpdateManagedBrokerAccount godoc
// @Summary 更新托管账户
// @Tags settings
// @Accept json
// @Produce json
// @Param accountRecordId path string true "托管账户记录 ID"
// @Param request body ManagedBrokerAccountWriteRequest true "托管账户"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/settings/broker-accounts/{accountRecordId} [put]
func handleUpdateManagedBrokerAccount(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			AccountRecordID string `uri:"accountRecordId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid account id")
			return
		}
		var input jfsettings.ManagedBrokerAccount
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid account payload")
			return
		}
		result, err := svc.UpdateManagedAccount(uri.AccountRecordID, input)
		if errors.Is(err, os.ErrNotExist) {
			httpserver.WriteError(c, 404, "NOT_FOUND", "managed broker account not found")
			return
		}
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleDeleteManagedBrokerAccount godoc
// @Summary 删除托管账户
// @Tags settings
// @Produce json
// @Param accountRecordId path string true "托管账户记录 ID"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/settings/broker-accounts/{accountRecordId} [delete]
func handleDeleteManagedBrokerAccount(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			AccountRecordID string `uri:"accountRecordId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid account id")
			return
		}
		if err := svc.DeleteManagedAccount(uri.AccountRecordID); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				httpserver.WriteError(c, 404, "NOT_FOUND", "managed broker account not found")
				return
			}
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, map[string]any{"deleted": true, "id": uri.AccountRecordID})
	}
}
