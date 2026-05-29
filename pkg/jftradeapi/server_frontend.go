package jftradeapi

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/frontendassets"
)

type frontendServer struct {
	files             fs.FS
	runtimeAPIBaseURL string
}

func loadFrontendFS() fs.FS {
	frontendFS, available, err := frontendassets.FileSystem()
	if err != nil {
		log.Printf("JFTrade embedded frontend assets unavailable: %v", err)
		return nil
	}
	if !available {
		return nil
	}
	return frontendFS
}

func newFrontendServer(frontendFS fs.FS) *frontendServer {
	return newFrontendServerWithRuntimeConfig(frontendFS, "")
}

func newFrontendServerWithRuntimeConfig(frontendFS fs.FS, runtimeAPIBaseURL string) *frontendServer {
	if frontendFS == nil {
		return nil
	}
	return &frontendServer{
		files:             frontendFS,
		runtimeAPIBaseURL: strings.TrimRight(strings.TrimSpace(runtimeAPIBaseURL), "/"),
	}
}

func (f *frontendServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.serveRequest(w, r) {
		return
	}
	http.NotFound(w, r)
}

func (f *frontendServer) serveRequest(w http.ResponseWriter, r *http.Request) bool {
	if f == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return false
	}

	cleanPath := normalizeFrontendPath(r.URL.Path)
	assetPath := strings.TrimPrefix(cleanPath, "/")
	if assetPath == "" {
		assetPath = "index.html"
	}
	if cleanPath == "/runtime-config.js" {
		f.serveRuntimeConfig(w, r)
		return true
	}

	if f.hasFile(assetPath) {
		f.serveFile(w, r, "/"+assetPath)
		return true
	}

	if !shouldServeFrontendIndex(r, cleanPath) || !f.hasFile("index.html") {
		return false
	}

	f.serveFile(w, r, "/index.html")
	return true
}

func (f *frontendServer) serveRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	payload, err := json.Marshal(map[string]string{"apiBaseUrl": f.runtimeAPIBaseURL})
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
	_, _ = w.Write([]byte("window.__JFTRADE_RUNTIME_CONFIG__ = Object.assign({}, window.__JFTRADE_RUNTIME_CONFIG__, "))
	_, _ = w.Write(payload)
	_, _ = w.Write([]byte(");\n"))
}

func (f *frontendServer) hasFile(assetPath string) bool {
	if f == nil {
		return false
	}
	info, err := fs.Stat(f.files, assetPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func (f *frontendServer) serveFile(w http.ResponseWriter, r *http.Request, requestPath string) {
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

func normalizeFrontendPath(requestPath string) string {
	cleanPath := path.Clean("/" + strings.TrimSpace(requestPath))
	if cleanPath == "." {
		return "/"
	}
	return cleanPath
}

func shouldServeFrontendIndex(r *http.Request, cleanPath string) bool {
	if cleanPath == "" {
		return false
	}
	if cleanPath == "/openapi.json" || strings.HasPrefix(cleanPath, "/api/") || strings.HasPrefix(cleanPath, "/swagger") {
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
