package webaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/frontendassets"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

type AccessSurfaceContextKey struct{}

// WithAccessSurface marks requests forwarded by the explicitly enabled browser listener.
func WithAccessSurface(r *http.Request) *http.Request {
	if r == nil {
		return nil
	}
	return r.WithContext(context.WithValue(r.Context(), AccessSurfaceContextKey{}, true))
}

// IsAccessSurfaceRequest reports whether a request came through the browser listener.
func IsAccessSurfaceRequest(r *http.Request) bool {
	return r != nil && r.Context().Value(AccessSurfaceContextKey{}) == true
}

type FrontendServer struct {
	files             fs.FS
	devProxy          *httputil.ReverseProxy
	runtimeAPIBaseURL string
	authRequired      bool
	desktopMode       bool
}

func LoadFrontendFS() fs.FS {
	return frontendassets.Load()
}

func NewFrontendServer(frontendFS fs.FS) *FrontendServer {
	return NewFrontendServerWithRuntimeConfig(frontendFS, "")
}

func NewFrontendServerWithRuntimeConfig(frontendFS fs.FS, runtimeAPIBaseURL string) *FrontendServer {
	return NewFrontendServerWithOptions(frontendFS, runtimeAPIBaseURL, "")
}

func NewFrontendServerWithOptions(frontendFS fs.FS, runtimeAPIBaseURL string, frontendDevURL string) *FrontendServer {
	devProxy := newFrontendDevProxy(frontendDevURL)
	if frontendFS == nil && devProxy == nil {
		return nil
	}
	return &FrontendServer{
		files:             frontendFS,
		devProxy:          devProxy,
		runtimeAPIBaseURL: strings.TrimRight(strings.TrimSpace(runtimeAPIBaseURL), "/"),
	}
}

func newFrontendDevProxy(rawURL string) *httputil.ReverseProxy {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || target.Hostname() == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil
	}
	host := strings.Trim(strings.TrimSpace(target.Hostname()), "[]")
	if !strings.EqualFold(host, "localhost") {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return nil
		}
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, proxyErr error) {
		log.Printf("JFTrade frontend development proxy unavailable: %v", proxyErr)
		http.Error(w, "JFTrade development UI is not available; start the Vite development server", http.StatusBadGateway)
	}
	return proxy
}

func (f *FrontendServer) SetAuthRequired(required bool) {
	if f != nil {
		f.authRequired = required
	}
}

func (f *FrontendServer) SetDesktopMode(enabled bool) {
	if f != nil {
		f.desktopMode = enabled
	}
}

func (f *FrontendServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.ServeRequest(w, r) {
		return
	}
	http.NotFound(w, r)
}

func (f *FrontendServer) ServeRequest(w http.ResponseWriter, r *http.Request) bool {
	if f == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return false
	}

	cleanPath := NormalizeFrontendPath(r.URL.Path)
	assetPath := strings.TrimPrefix(cleanPath, "/")
	if assetPath == "" {
		assetPath = "index.html"
	}
	if cleanPath == "/runtime-config.js" {
		f.serveRuntimeConfig(w, r)
		return true
	}
	if f.files == nil && f.devProxy != nil {
		f.devProxy.ServeHTTP(w, r)
		return true
	}

	if f.HasFile(assetPath) {
		f.ServeFile(w, r, "/"+assetPath)
		return true
	}

	if !ShouldServeFrontendIndex(r, cleanPath) || !f.HasFile("index.html") {
		return false
	}

	f.ServeFile(w, r, "/index.html")
	return true
}

func (f *FrontendServer) serveRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	desktopMode := f.desktopMode
	apiBaseURL := f.runtimeAPIBaseURL
	if IsAccessSurfaceRequest(r) {
		desktopMode = false
		if strings.TrimSpace(r.Host) != "" {
			apiBaseURL = requestScheme(r) + "://" + r.Host
		}
	}
	config := map[string]any{
		"apiBaseUrl":   apiBaseURL,
		"authRequired": f.authRequired,
		"desktopMode":  desktopMode,
	}
	payload, err := json.Marshal(config)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, jftradeErr3 := w.Write([]byte("window.__JFTRADE_RUNTIME_CONFIG__ = Object.assign({}, window.__JFTRADE_RUNTIME_CONFIG__, "))
	besteffort.LogError(jftradeErr3)
	_, jftradeErr1 := w.Write(payload)
	besteffort.LogError(jftradeErr1)
	_, jftradeErr2 := w.Write([]byte(");\n"))
	besteffort.LogError(jftradeErr2)
}

func (f *FrontendServer) HasFile(assetPath string) bool {
	if f == nil {
		return false
	}
	info, err := fs.Stat(f.files, assetPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func (f *FrontendServer) ServeFile(w http.ResponseWriter, r *http.Request, requestPath string) {
	assetPath := strings.TrimPrefix(requestPath, "/")
	data, err := fs.ReadFile(f.files, assetPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(assetPath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, path.Base(assetPath), time.Time{}, bytes.NewReader(data))
}

func NormalizeFrontendPath(requestPath string) string {
	cleanPath := path.Clean("/" + strings.TrimSpace(requestPath))
	if cleanPath == "." {
		return "/"
	}
	return cleanPath
}

func ShouldServeFrontendIndex(r *http.Request, cleanPath string) bool {
	if cleanPath == "" {
		return false
	}
	if strings.HasPrefix(cleanPath, "/api/") || strings.HasPrefix(cleanPath, "/swagger") {
		return false
	}
	if cleanPath == "/assets" || strings.HasPrefix(cleanPath, "/assets/") {
		return false
	}
	if strings.Contains(path.Base(cleanPath), ".") {
		return false
	}

	accept := strings.ToLower(r.Header.Get("Accept"))
	return cleanPath == "/" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func requestScheme(r *http.Request) string {
	if r != nil && r.TLS != nil {
		return "https"
	}
	if r != nil {
		host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
		if err != nil {
			host = strings.TrimSpace(r.RemoteAddr)
		}
		if remote := net.ParseIP(strings.Trim(host, "[]")); remote != nil && remote.IsLoopback() {
			values := strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")
			for index := len(values) - 1; index >= 0; index-- {
				if strings.EqualFold(strings.TrimSpace(values[index]), "https") {
					return "https"
				}
				if strings.TrimSpace(values[index]) != "" {
					break
				}
			}
		}
	}
	return "http"
}
