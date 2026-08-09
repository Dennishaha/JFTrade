package servercoretest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/instancebinding"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestStrategiesExposeDefinitionSyncAndRefreshDefinitionRoute(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	definition := saveSeededStrategyDefinition(t, store, stratsrv.Definition{
		ID:           "dsl-versioned",
		Name:         "Versioned Strategy",
		Description:  "first save",
		Runtime:      stratsrv.RuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Versioned Strategy\", overlay=true)\nlog.info(\"old\")",
	})
	catalog, err := openSeededStrategyCatalog(t, store)
	if err != nil {
		t.Fatalf("open strategy catalog: %v", err)
	}
	instance, err := catalog.CreateInstance(definition, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: instancebinding.ExecutionModeNotifyOnly,
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	if err := catalog.Close(); err != nil {
		t.Fatalf("close strategy catalog: %v", err)
	}
	definition = saveSeededStrategyDefinition(t, store, stratsrv.Definition{
		ID:           definition.ID,
		Name:         definition.Name,
		Description:  "second save",
		Runtime:      definition.Runtime,
		SourceFormat: definition.SourceFormat,
		Symbol:       definition.Symbol,
		Interval:     definition.Interval,
		Script:       "//@version=6\nstrategy(\"Versioned Strategy\", overlay=true)\nfast = ta.sma(close, 10)\nlog.info(\"new\")",
		VisualModel:  definition.VisualModel,
	})
	if definition.Version != "0.1.1" {
		t.Fatalf("definition version = %q, want 0.1.1", definition.Version)
	}

	srv := newHTTPTestServer(t, store)

	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/strategies")
	if err != nil {
		t.Fatalf("GET strategies: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listResp.Body.Close()) }()
	var listEnvelope struct {
		OK   bool                    `json:"ok"`
		Data []stratsrv.InstanceView `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listEnvelope); err != nil {
		t.Fatalf("decode strategies: %v", err)
	}
	if len(listEnvelope.Data) != 1 {
		t.Fatalf("expected 1 strategy, got %+v", listEnvelope.Data)
	}
	if listEnvelope.Data[0].DefinitionSync == nil {
		t.Fatal("expected definition sync status")
	}
	if listEnvelope.Data[0].DefinitionSync.IsLatest {
		t.Fatalf("expected strategy to be stale, got %+v", listEnvelope.Data[0].DefinitionSync)
	}
	if !listEnvelope.Data[0].DefinitionSync.CanApplyLatest {
		t.Fatalf("expected stopped strategy to allow refresh, got %+v", listEnvelope.Data[0].DefinitionSync)
	}
	if listEnvelope.Data[0].DefinitionSync.LatestVersion != "0.1.1" {
		t.Fatalf("latestVersion = %q, want 0.1.1", listEnvelope.Data[0].DefinitionSync.LatestVersion)
	}

	refreshResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/strategies/"+instance.ID+"/refresh-definition", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST refresh-definition: %v", err)
	}
	defer func() { jftradeCheckTestError(t, refreshResp.Body.Close()) }()
	if refreshResp.StatusCode != http.StatusOK {
		t.Fatalf("POST refresh-definition status = %d", refreshResp.StatusCode)
	}
	var refreshEnvelope struct {
		OK   bool                  `json:"ok"`
		Data stratsrv.InstanceView `json:"data"`
	}
	if err := json.NewDecoder(refreshResp.Body).Decode(&refreshEnvelope); err != nil {
		t.Fatalf("decode refresh-definition: %v", err)
	}
	if refreshEnvelope.Data.Definition.Version != "0.1.1" {
		t.Fatalf("refreshed strategy version = %q, want 0.1.1", refreshEnvelope.Data.Definition.Version)
	}
	if refreshEnvelope.Data.DefinitionSync == nil || !refreshEnvelope.Data.DefinitionSync.IsLatest {
		t.Fatalf("expected refreshed strategy to be latest, got %+v", refreshEnvelope.Data.DefinitionSync)
	}
	if script := jftradeCheckedTypeAssertion[string](refreshEnvelope.Data.Params["script"]); !strings.Contains(script, "fast = ta.sma(close, 10)") {
		t.Fatalf("expected refreshed script snapshot, got %q", script)
	}
}
