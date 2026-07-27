package strategy

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/strategy"
)

type pluginURI struct {
	PluginID string `uri:"pluginId" binding:"required"`
}

type pluginOperationURI struct {
	OperationID string `uri:"operationId" binding:"required"`
}

func RegisterPluginRoutes(api *gin.RouterGroup, service *srv.Service) {
	api.GET("/plugins", handlePluginCatalog(service))
	api.GET("/plugins/operations/:operationId", handlePluginOperation(service))
	api.POST("/plugins/:pluginId/install", handlePluginInstall(service))
	api.POST("/plugins/:pluginId/uninstall", handlePluginUninstall(service))
	api.GET("/plugins/:pluginId/uninstall-guidance", handlePluginUninstallGuidance(service))
}

// handlePluginCatalog godoc
// @Summary 读取策略插件目录
// @Tags plugins
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=srv.PluginCatalog}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/plugins [get]
func handlePluginCatalog(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, service.PluginCatalog())
	}
}

// handlePluginOperation godoc
// @Summary 读取策略插件操作状态
// @Tags plugins
// @Produce json
// @Param operationId path string true "插件操作 ID"
// @Success 200 {object} httpserver.Envelope{data=srv.PluginOperation}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/plugins/operations/{operationId} [get]
func handlePluginOperation(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri pluginOperationURI
		if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.OperationID) == "" {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "operationId is required")
			return
		}
		operation, ok := service.PluginOperation(strings.TrimSpace(uri.OperationID))
		if !ok {
			httpserver.WriteError(c, http.StatusNotFound, "NOT_FOUND", "plugin operation not found")
			return
		}
		httpserver.WriteOK(c, operation)
	}
}

func handlePluginInstall(service *srv.Service) gin.HandlerFunc {
	return handlePluginMutation(service, "install")
}

func handlePluginUninstall(service *srv.Service) gin.HandlerFunc {
	return handlePluginMutation(service, "uninstall")
}

// handlePluginMutation godoc
// @Summary 安装或卸载策略插件
// @Tags plugins
// @Produce json
// @Param pluginId path string true "插件 ID"
// @Success 200 {object} httpserver.Envelope{data=PluginMutationData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/plugins/{pluginId}/install [post]
// @Router /api/v1/plugins/{pluginId}/uninstall [post]
func handlePluginMutation(service *srv.Service, operationName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri pluginURI
		if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.PluginID) == "" {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "pluginId is invalid")
			return
		}
		pluginID := strings.TrimSpace(uri.PluginID)
		var (
			operation srv.PluginOperation
			err       error
		)
		if operationName == "install" {
			operation, err = service.InstallPlugin(pluginID)
		} else {
			operation, err = service.UninstallPlugin(pluginID)
		}
		if os.IsNotExist(err) {
			httpserver.WriteError(c, http.StatusNotFound, "NOT_FOUND", "plugin not found")
			return
		}
		if err != nil {
			httpserver.WriteError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "plugin "+operationName+" failed")
			return
		}
		httpserver.WriteOK(c, map[string]any{"operation": operation})
	}
}

// handlePluginUninstallGuidance godoc
// @Summary 读取策略插件卸载指引
// @Tags plugins
// @Produce json
// @Param pluginId path string true "插件 ID"
// @Success 200 {object} httpserver.Envelope{data=srv.PluginUninstallGuidance}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/plugins/{pluginId}/uninstall-guidance [get]
func handlePluginUninstallGuidance(service *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri pluginURI
		if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.PluginID) == "" {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "pluginId is invalid")
			return
		}
		guidance, ok := service.PluginUninstallGuidance(strings.TrimSpace(uri.PluginID))
		if !ok {
			httpserver.WriteError(c, http.StatusNotFound, "NOT_FOUND", "plugin not found")
			return
		}
		httpserver.WriteOK(c, guidance)
	}
}
