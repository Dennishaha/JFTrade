package settings_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestRuntimeDependencyRoutesReadAndSavePythonPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &routeStore{runtimeDependencies: jfsettings.RuntimeDependencySettings{
		PythonBinaryPath: "/usr/bin/python3",
	}}
	router := settingsRouter(store)

	getResponse := performSettingsRequest(t, router, http.MethodGet, "/api/v1/settings/runtime-dependencies", "")
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), `"pythonBinaryPath":"/usr/bin/python3"`) {
		t.Fatalf("get response = %d %s", getResponse.Code, getResponse.Body.String())
	}

	putResponse := performSettingsRequest(
		t,
		router,
		http.MethodPut,
		"/api/v1/settings/runtime-dependencies",
		`{"pythonBinaryPath":"/opt/python/bin/python3"}`,
	)
	if putResponse.Code != http.StatusOK || store.runtimeDependencies.PythonBinaryPath != "/opt/python/bin/python3" {
		t.Fatalf("put response/store = %d %s / %#v", putResponse.Code, putResponse.Body.String(), store.runtimeDependencies)
	}
}
