package jftradeapi

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) handlePluginOperation(c *gin.Context) {
	var uri operationURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "operationId is required")
		return
	}
	operationID := strings.TrimSpace(uri.OperationID)
	if operationID == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "operationId is required")
		return
	}
	operation, ok := s.strategyStore.pluginOperation(operationID)
	if !ok {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "plugin operation not found")
		return
	}
	s.writeOK(c, operation)
}

func (s *Server) handlePluginInstall(c *gin.Context) {
	var uri pluginURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "pluginId is invalid")
		return
	}
	pluginID := strings.TrimSpace(uri.PluginID)
	if pluginID == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "pluginId is invalid")
		return
	}
	operation, err := s.strategyStore.installPlugin(pluginID)
	if err != nil {
		if errorsIsNotFound(err) {
			s.writeError(c, http.StatusNotFound, "NOT_FOUND", "plugin not found")
			return
		}
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "plugin install failed")
		return
	}
	s.writeOK(c, map[string]any{"operation": operation})
}

func (s *Server) handlePluginUninstall(c *gin.Context) {
	var uri pluginURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "pluginId is invalid")
		return
	}
	pluginID := strings.TrimSpace(uri.PluginID)
	if pluginID == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "pluginId is invalid")
		return
	}
	operation, err := s.strategyStore.uninstallPlugin(pluginID)
	if err != nil {
		if errorsIsNotFound(err) {
			s.writeError(c, http.StatusNotFound, "NOT_FOUND", "plugin not found")
			return
		}
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "plugin uninstall failed")
		return
	}
	s.writeOK(c, map[string]any{"operation": operation})
}

func (s *Server) handlePluginUninstallGuidance(c *gin.Context) {
	var uri pluginURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "pluginId is invalid")
		return
	}
	pluginID := strings.TrimSpace(uri.PluginID)
	if pluginID == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "pluginId is invalid")
		return
	}
	guidance, ok := s.strategyStore.pluginUninstallGuidance(pluginID)
	if !ok {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "plugin not found")
		return
	}
	s.writeOK(c, guidance)
}

func errorsIsNotFound(err error) bool {
	return os.IsNotExist(err)
}
