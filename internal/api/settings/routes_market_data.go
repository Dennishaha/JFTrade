package settings

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/settings"
)

// handleActiveMarketDataProvider godoc
// @Summary 读取当前行情数据源
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=MarketDataProviderSettingsResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/settings/market-data-provider [get]
func handleActiveMarketDataProvider(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, MarketDataProviderSettingsResponse{
			ActiveProvider: svc.GetActiveMarketDataProvider(),
		})
	}
}

// handleSaveActiveMarketDataProvider godoc
// @Summary 切换当前行情数据源
// @Tags settings
// @Accept json
// @Produce json
// @Param request body MarketDataProviderWriteRequest true "行情数据源选择"
// @Success 200 {object} httpserver.Envelope{data=MarketDataProviderSettingsResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/settings/market-data-provider [put]
func handleSaveActiveMarketDataProvider(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input MarketDataProviderWriteRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid market-data provider payload")
			return
		}
		result, err := svc.SaveActiveMarketDataProvider(input.ActiveProvider)
		if err != nil {
			writeMarketDataSettingsError(c, err, "MARKET_DATA_PROVIDER_INVALID")
			return
		}
		httpserver.WriteOK(c, MarketDataProviderSettingsResponse{ActiveProvider: result})
	}
}

func writeMarketDataSettingsError(c *gin.Context, err error, invalidCode string) {
	switch {
	case errors.Is(err, srv.ErrProviderRuntimeUpdate):
		httpserver.WriteError(
			c,
			http.StatusConflict,
			"MARKET_DATA_PROVIDER_UPDATE_FAILED",
			err.Error(),
		)
	case errors.Is(err, srv.ErrMarketDataProviderInvalid):
		httpserver.WriteError(c, http.StatusBadRequest, invalidCode, err.Error())
	default:
		httpserver.WriteError(c, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
	}
}
