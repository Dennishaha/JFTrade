package settings

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	"github.com/jftrade/jftrade-main/internal/jftsettings"
	srv "github.com/jftrade/jftrade-main/internal/settings"
)

var _ jftsettings.RuntimeDependencySettings

// handleRuntimeDependencySettings godoc
// @Summary 读取运行时依赖路径设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=jftsettings.RuntimeDependencySettings}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/settings/runtime-dependencies [get]
func handleRuntimeDependencySettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.GetRuntimeDependencySettings())
	}
}

// handleSaveRuntimeDependencySettings godoc
// @Summary 保存运行时依赖路径设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body RuntimeDependencySettingsWriteRequest true "运行时依赖路径设置"
// @Success 200 {object} httpserver.Envelope{data=jftsettings.RuntimeDependencySettings}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/settings/runtime-dependencies [put]
func handleSaveRuntimeDependencySettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input RuntimeDependencySettingsWriteRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid runtime dependency payload")
			return
		}
		result, err := svc.SaveRuntimeDependencySettings(input.settings())
		if err != nil {
			code := "SETTINGS_SAVE_FAILED"
			if errors.Is(err, srv.ErrRuntimeDependencyStoreUnavailable) {
				code = "RUNTIME_DEPENDENCY_SETTINGS_UNAVAILABLE"
			}
			httpserver.WriteError(c, 500, code, err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}
