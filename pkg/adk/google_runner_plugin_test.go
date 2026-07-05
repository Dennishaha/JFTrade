package adk

import (
	"context"
	"errors"
	"strings"
	"testing"

	adkmodel "google.golang.org/adk/v2/model"
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
	if plugin.BeforeRunCallback() == nil ||
		plugin.AfterRunCallback() == nil ||
		plugin.OnEventCallback() == nil ||
		plugin.AfterModelCallback() == nil ||
		plugin.OnModelErrorCallback() == nil ||
		plugin.BeforeToolCallback() == nil ||
		plugin.AfterToolCallback() == nil ||
		plugin.OnToolErrorCallback() == nil {
		t.Fatal("plugin did not register the full v2 projection callback surface")
	}

	content, err := plugin.BeforeRunCallback()(nil)
	if err != nil || content != nil {
		t.Fatalf("BeforeRunCallback = content %#v err %v, want nil passthrough", content, err)
	}
	plugin.AfterRunCallback()(nil)

	event := adksession.NewEvent(context.Background(), "invocation")
	gotEvent, err := plugin.OnEventCallback()(nil, event)
	if err != nil || gotEvent != event {
		t.Fatalf("OnEventCallback = event %#v err %v, want original event", gotEvent, err)
	}

	modelResp, err := plugin.AfterModelCallback()(nil, &adkmodel.LLMResponse{}, nil)
	if err != nil || modelResp != nil {
		t.Fatalf("AfterModelCallback = response %#v err %v, want nil passthrough", modelResp, err)
	}
	modelResp, err = plugin.OnModelErrorCallback()(nil, &adkmodel.LLMRequest{}, errors.New("model failed"))
	if err != nil || modelResp != nil {
		t.Fatalf("OnModelErrorCallback = response %#v err %v, want nil passthrough", modelResp, err)
	}

	toolResult, err := plugin.OnToolErrorCallback()(nil, nil, map[string]any{"x": "y"}, errors.New("tool failed"))
	if err != nil || toolResult != nil {
		t.Fatalf("OnToolErrorCallback = result %#v err %v, want nil passthrough", toolResult, err)
	}
}

func TestGoogleADKExecutionPluginRejectsNilExecution(t *testing.T) {
	var execution *googleADKExecution
	_, err := execution.plugin()
	if err == nil || !strings.Contains(err.Error(), "GO-ADK execution is unavailable") {
		t.Fatalf("plugin err = %v, want unavailable error", err)
	}
}
