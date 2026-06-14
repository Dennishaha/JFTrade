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
	api.GET("/plugins", func(c *gin.Context) {
		httpserver.WriteOK(c, service.PluginCatalog())
	})
	api.GET("/plugins/operations/:operationId", handlePluginOperation(service))
	api.POST("/plugins/:pluginId/install", handlePluginInstall(service))
	api.POST("/plugins/:pluginId/uninstall", handlePluginUninstall(service))
	api.GET("/plugins/:pluginId/uninstall-guidance", handlePluginUninstallGuidance(service))
}

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
