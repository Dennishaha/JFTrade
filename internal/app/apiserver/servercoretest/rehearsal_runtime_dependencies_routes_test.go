package servercoretest

import (
	"os/exec"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestRuntimeDependenciesReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operation := "GET /api/v1/system/runtime-dependencies"
	path := "/api/v1/system/runtime-dependencies"
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:          "runtime-dependencies-read",
		operations:      []string{operation},
		paths:           []string{path},
		operationPaths:  map[string]string{operation: path},
		dynamicJSONKeys: map[string]struct{}{"timestamp": {}, "checkedAt": {}},
		prepareStore:    prepareRuntimeDependenciesReadRehearsal,
	})
}

func prepareRuntimeDependenciesReadRehearsal(t *testing.T, store *servercore.SettingsStore) {
	t.Helper()
	prepareDisabledFutuReadRehearsal(t, store)
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("resolve required Node runtime: %v", err)
	}
	_, err = store.SavePineWorkerSettings(jfsettings.PineWorkerSettings{
		BacktestWorkerLimit: 2,
		InstanceWorkerLimit: 10,
		NodeBinaryPath:      nodePath,
	})
	if err != nil {
		t.Fatalf("save runtime dependency settings: %v", err)
	}
}
