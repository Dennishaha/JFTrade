package servercore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	httpserver "github.com/jftrade/jftrade-main/internal/api/httpserver"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestDeleteStrategyDefinitionRequiresDeletingLinkedInstancesFirst(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	definition, err := server.stores.Design.SaveDefinition(stratsrv.Definition{
		ID:           "pine-delete-guard",
		Name:         "Delete Guard",
		Description:  "delete guard",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Delete Guard\", overlay=true)\nlog.info(\"close\")",
	})
	if err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	instance, err := server.stores.StrategyCatalog.CreateInstance(definition, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "5m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	deleteReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/strategy-definitions/"+definition.ID, nil)
	if err != nil {
		t.Fatalf("build delete definition request: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete definition with linked instance: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteResp.Body.Close()) }()
	if deleteResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("delete definition with linked instance status = %d, want %d", deleteResp.StatusCode, http.StatusBadRequest)
	}
	var blockedEnvelope httpserver.Envelope
	if err := json.NewDecoder(deleteResp.Body).Decode(&blockedEnvelope); err != nil {
		t.Fatalf("decode blocked delete response: %v", err)
	}
	if blockedEnvelope.Error == nil || !strings.Contains(blockedEnvelope.Error.Message, "请先删除对应实例再删除") {
		t.Fatalf("unexpected blocked delete response: %+v", blockedEnvelope)
	}
	if _, ok, err := server.stores.Design.GetDefinition(definition.ID); err != nil || !ok {
		t.Fatal("definition should still exist after blocked delete")
	}

	instanceDeleteReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/strategies/"+instance.ID, nil)
	if err != nil {
		t.Fatalf("build delete instance request: %v", err)
	}
	instanceDeleteResp, err := http.DefaultClient.Do(instanceDeleteReq)
	if err != nil {
		t.Fatalf("delete linked instance: %v", err)
	}
	defer func() { jftradeCheckTestError(t, instanceDeleteResp.Body.Close()) }()
	if instanceDeleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete linked instance status = %d, want %d", instanceDeleteResp.StatusCode, http.StatusOK)
	}

	deleteReq, err = http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/strategy-definitions/"+definition.ID, nil)
	if err != nil {
		t.Fatalf("build second delete definition request: %v", err)
	}
	deleteResp, err = http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete definition after removing instances: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteResp.Body.Close()) }()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete definition after removing instances status = %d, want %d", deleteResp.StatusCode, http.StatusOK)
	}
	if _, ok, err := server.stores.Design.GetDefinition(definition.ID); err != nil || ok {
		t.Fatal("definition should be hidden after soft delete")
	}
	definitions, err := server.stores.Design.ListDefinitions()
	if err != nil || len(definitions) != 0 {
		t.Fatalf("expected no active definitions after delete, got %+v, err %v", definitions, err)
	}
	historyResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/strategy-definitions/"+definition.ID+"/versions")
	if err != nil {
		t.Fatalf("GET soft-deleted strategy definition versions: %v", err)
	}
	defer func() { jftradeCheckTestError(t, historyResp.Body.Close()) }()
	if historyResp.StatusCode != http.StatusOK {
		t.Fatalf("GET soft-deleted strategy definition versions status = %d", historyResp.StatusCode)
	}
	var historyEnvelope struct {
		Data []stratsrv.DefinitionVersionSummary `json:"data"`
	}
	if err := json.NewDecoder(historyResp.Body).Decode(&historyEnvelope); err != nil {
		t.Fatalf("decode soft-deleted strategy definition versions: %v", err)
	}
	if len(historyEnvelope.Data) != 1 || historyEnvelope.Data[0].IsCurrent {
		t.Fatalf("soft-deleted strategy version history = %+v", historyEnvelope.Data)
	}
}
