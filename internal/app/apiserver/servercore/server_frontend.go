package servercore

import (
	"io/fs"
	"net/http"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/webaccess"
)

// frontendServer keeps the historical servercore test seam while the actual
// browser asset/proxy implementation lives in the webaccess adapter package.
type frontendServer struct {
	*webaccess.FrontendServer
}

func loadFrontendFS() fs.FS {
	return webaccess.LoadFrontendFS()
}

func newFrontendServer(frontendFS fs.FS) *frontendServer {
	return wrapFrontend(webaccess.NewFrontendServer(frontendFS))
}

func newFrontendServerWithRuntimeConfig(frontendFS fs.FS, runtimeAPIBaseURL string) *frontendServer {
	return wrapFrontend(webaccess.NewFrontendServerWithRuntimeConfig(frontendFS, runtimeAPIBaseURL))
}

func newFrontendServerWithOptions(frontendFS fs.FS, runtimeAPIBaseURL string, frontendDevURL string) *frontendServer {
	return wrapFrontend(webaccess.NewFrontendServerWithOptions(frontendFS, runtimeAPIBaseURL, frontendDevURL))
}

func wrapFrontend(frontend *webaccess.FrontendServer) *frontendServer {
	if frontend == nil {
		return nil
	}
	return &frontendServer{FrontendServer: frontend}
}

func (f *frontendServer) setAuthRequired(required bool) {
	if f != nil {
		f.SetAuthRequired(required)
	}
}

func (f *frontendServer) setDesktopMode(enabled bool) {
	if f != nil {
		f.SetDesktopMode(enabled)
	}
}

func (f *frontendServer) serveRequest(w http.ResponseWriter, r *http.Request) bool {
	return f != nil && f.ServeRequest(w, r)
}
