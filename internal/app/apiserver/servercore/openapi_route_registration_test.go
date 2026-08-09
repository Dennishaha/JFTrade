package servercore

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestOpenAPICoversRegisteredAPIRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	resp, err := jftradeTestHTTPGet(t, srv.URL+"/swagger/doc.json")
	if err != nil {
		t.Fatalf("GET /swagger/doc.json: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/swagger/doc.json status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /swagger/doc.json body: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("parse /swagger/doc.json: %v", err)
	}

	undocumented := make([]string, 0)
	registered := make(map[string]struct{}, len(server.router.Routes()))
	for _, route := range server.router.Routes() {
		if !strings.HasPrefix(route.Path, "/api/v1/") {
			continue
		}
		path := openAPIPathFromGinPath(route.Path)
		registered[route.Method+" "+path] = struct{}{}
		methods, ok := spec.Paths[path]
		if !ok {
			undocumented = append(undocumented, route.Method+" "+path)
			continue
		}
		if _, ok := methods[strings.ToLower(route.Method)]; !ok {
			undocumented = append(undocumented, route.Method+" "+path)
		}
	}
	sort.Strings(undocumented)
	if len(undocumented) > 0 {
		t.Fatalf("registered API routes missing from OpenAPI:\n%s", strings.Join(undocumented, "\n"))
	}

	unregistered := make([]string, 0)
	for path, methods := range spec.Paths {
		if !strings.HasPrefix(path, "/api/v1/") {
			continue
		}
		for method := range methods {
			operation := strings.ToUpper(method) + " " + path
			if _, ok := registered[operation]; !ok {
				unregistered = append(unregistered, operation)
			}
		}
	}
	sort.Strings(unregistered)
	if len(unregistered) > 0 {
		t.Fatalf("OpenAPI operations missing from registered API routes:\n%s", strings.Join(unregistered, "\n"))
	}
}

func TestCapabilityCatalogAPISurfacesAreRegistered(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	registered := make(map[string]struct{}, len(server.router.Routes()))
	for _, route := range server.router.Routes() {
		registered[route.Path] = struct{}{}
	}

	placeholder := regexp.MustCompile(`\{([^{}]+)\}`)
	missing := make([]string, 0)
	for _, capability := range broker.BuiltinCapabilityCatalog.Features {
		path := strings.SplitN(capability.Surface.API, "?", 2)[0]
		path = placeholder.ReplaceAllString(path, `:$1`)
		if _, ok := registered[path]; !ok {
			missing = append(missing, string(capability.ID)+" -> "+capability.Surface.API)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("CapabilityCatalog API surfaces are not registered:\n%s", strings.Join(missing, "\n"))
	}
}

func openAPIPathFromGinPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if after, ok := strings.CutPrefix(part, ":"); ok {
			parts[i] = "{" + after + "}"
		}
	}
	return strings.Join(parts, "/")
}
