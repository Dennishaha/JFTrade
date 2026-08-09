package servercore

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	"github.com/jftrade/jftrade-main/internal/api/middleware"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/webaccess"
)

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.router == nil {
		http.NotFound(w, r)
		return
	}
	s.router.ServeHTTP(w, r)
}

// WebAccessHandler marks requests that arrived through the explicitly enabled
// browser listener. The desktop sidecar listener remains capability-only.
func (s *Server) WebAccessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.ServeHTTP(w, webaccess.WithAccessSurface(r))
	})
}

func isWebAccessSurfaceRequest(r *http.Request) bool {
	return webaccess.IsAccessSurfaceRequest(r)
}

var _ middleware.WriteMethodDetector = (*Server)(nil)

func (s *Server) IsWriteMethod(r *http.Request) bool {
	if r == nil {
		return false
	}
	return r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete
}

func writeError(s *Server, c *gin.Context, status int, code string, message string) {
	httpserver.WriteError(c, status, code, message)
}
