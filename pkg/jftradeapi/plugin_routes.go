package jftradeapi

import (
	"net/http"
	"os"
	"strings"
)

func (s *Server) servePluginRoutes(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.URL.Path == "/api/v1/plugins" && r.Method == http.MethodGet:
		s.writeOK(w, s.strategyStore.pluginCatalog())
	case strings.HasPrefix(r.URL.Path, "/api/v1/plugins/operations/") && r.Method == http.MethodGet:
		operationID := strings.TrimPrefix(r.URL.Path, "/api/v1/plugins/operations/")
		if operationID == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "operationId is required")
			return true
		}
		operation, ok := s.strategyStore.pluginOperation(operationID)
		if !ok {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "plugin operation not found")
			return true
		}
		s.writeOK(w, operation)
	case strings.HasPrefix(r.URL.Path, "/api/v1/plugins/") && strings.HasSuffix(r.URL.Path, "/install") && r.Method == http.MethodPost:
		pluginID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/plugins/", "/install"))
		if err != nil || strings.TrimSpace(pluginID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "pluginId is invalid")
			return true
		}
		operation, err := s.strategyStore.installPlugin(pluginID)
		if err != nil {
			if errorsIsNotFound(err) {
				s.writeError(w, http.StatusNotFound, "NOT_FOUND", "plugin not found")
				return true
			}
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "plugin install failed")
			return true
		}
		s.writeOK(w, map[string]any{"operation": operation})
	case strings.HasPrefix(r.URL.Path, "/api/v1/plugins/") && strings.HasSuffix(r.URL.Path, "/uninstall") && r.Method == http.MethodPost:
		pluginID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/plugins/", "/uninstall"))
		if err != nil || strings.TrimSpace(pluginID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "pluginId is invalid")
			return true
		}
		operation, err := s.strategyStore.uninstallPlugin(pluginID)
		if err != nil {
			if errorsIsNotFound(err) {
				s.writeError(w, http.StatusNotFound, "NOT_FOUND", "plugin not found")
				return true
			}
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "plugin uninstall failed")
			return true
		}
		s.writeOK(w, map[string]any{"operation": operation})
	case strings.HasPrefix(r.URL.Path, "/api/v1/plugins/") && strings.HasSuffix(r.URL.Path, "/uninstall-guidance") && r.Method == http.MethodGet:
		pluginID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/plugins/", "/uninstall-guidance"))
		if err != nil || strings.TrimSpace(pluginID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "pluginId is invalid")
			return true
		}
		guidance, ok := s.strategyStore.pluginUninstallGuidance(pluginID)
		if !ok {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "plugin not found")
			return true
		}
		s.writeOK(w, guidance)
	default:
		return false
	}
	return true
}

func errorsIsNotFound(err error) bool {
	return os.IsNotExist(err)
}
