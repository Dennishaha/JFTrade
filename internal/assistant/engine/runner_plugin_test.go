package adk

import (
	"context"
	"strings"
	"testing"

	adksession "google.golang.org/adk/v2/session"
)

func TestGoogleADKExecutionPluginRegistersV2ProjectionCallbacks(t *testing.T) {
	execution := &googleADKExecution{}
	plugin, err := execution.plugin()
	if err != nil {
		t.Fatalf("plugin: %v", err)
	}
	if plugin.Name() != "jftrade_execution_projection" {
		t.Fatalf("plugin name = %q", plugin.Name())
	}
	if plugin.OnEventCallback() == nil || plugin.BeforeToolCallback() == nil || plugin.AfterToolCallback() == nil {
		t.Fatal("plugin did not register the projection callbacks")
	}
	if plugin.BeforeRunCallback() != nil ||
		plugin.AfterRunCallback() != nil ||
		plugin.AfterModelCallback() != nil ||
		plugin.OnModelErrorCallback() != nil ||
		plugin.OnToolErrorCallback() != nil {
		t.Fatal("plugin registered callbacks with no product behavior")
	}

	event := adksession.NewEvent(context.Background(), "invocation")
	gotEvent, err := plugin.OnEventCallback()(nil, event)
	if err != nil || gotEvent != event {
		t.Fatalf("OnEventCallback = event %#v err %v, want original event", gotEvent, err)
	}
}

func TestGoogleADKExecutionPluginRejectsNilExecution(t *testing.T) {
	var execution *googleADKExecution
	_, err := execution.plugin()
	if err == nil || !strings.Contains(err.Error(), "GO-ADK execution is unavailable") {
		t.Fatalf("plugin err = %v, want unavailable error", err)
	}
}
