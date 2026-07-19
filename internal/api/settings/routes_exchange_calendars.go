package settings

import (
	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/settings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

// ── Exchange Calendars ──

// handleExchangeCalendarSettings godoc
// @Summary 读取交易日历设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/exchange-calendars [get]
func handleExchangeCalendarSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, map[string]any{"exchangeCalendars": svc.GetExchangeCalendarSettings()})
	}
}

// handleSaveExchangeCalendarSettings godoc
// @Summary 保存交易日历设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body ExchangeCalendarSettingsWriteRequest true "交易日历设置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/exchange-calendars [put]
func handleSaveExchangeCalendarSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			ExchangeCalendars jfsettings.ExchangeCalendarSettings `json:"exchangeCalendars"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid exchange calendar payload")
			return
		}
		result, err := svc.SaveExchangeCalendarSettings(payload.ExchangeCalendars)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, map[string]any{"exchangeCalendars": result})
	}
}
