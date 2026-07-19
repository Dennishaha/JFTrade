package settings

import (
	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/settings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

// ── Pine Worker ──

// handlePineWorkerSettings godoc
// @Summary 读取 PineTS worker 设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/pine-worker [get]
func handlePineWorkerSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.GetPineWorkerSettings())
	}
}

// handleSavePineWorkerSettings godoc
// @Summary 保存 PineTS worker 设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body jfsettings.PineWorkerSettings true "PineTS worker 设置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/pine-worker [put]
func handleSavePineWorkerSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input jfsettings.PineWorkerSettings
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid pine worker payload")
			return
		}
		result, err := svc.SavePineWorkerSettings(input)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}
