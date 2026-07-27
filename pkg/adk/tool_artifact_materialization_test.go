package adk

import (
	"context"
	"errors"
	"strings"
	"testing"

	adkartifact "google.golang.org/adk/v2/artifact"
)

type recordingArtifactService struct {
	adkartifact.Service
	request *adkartifact.SaveRequest
	result  *adkartifact.SaveResponse
	err     error
}

func (s *recordingArtifactService) Save(_ context.Context, request *adkartifact.SaveRequest) (*adkartifact.SaveResponse, error) {
	s.request = request
	return s.result, s.err
}

func TestMaterializeToolOutputPersistsLargeResearchResultsAsArtifacts(t *testing.T) {
	payload := strings.Repeat("x", MaxToolOutputBytes+256)
	service := &recordingArtifactService{result: &adkartifact.SaveResponse{Version: 7}}
	execution := &googleADKExecution{
		artifactService: service,
		appName:         "jftrade-agent",
		sessionID:       "session-1",
		runID:           "run 1",
	}

	result := execution.materializeToolOutput(" strategy.research_backtest ", " call / 1 ", map[string]any{"payload": payload}).(map[string]any)
	if service.request == nil || service.request.FileName != "strategy.research_backtest-run-1-call-1.json" || service.request.AppName != execution.appName || service.request.SessionID != execution.sessionID {
		t.Fatalf("artifact save request = %#v", service.request)
	}
	if result["truncated"] != true || !strings.HasPrefix(result["preview"].(string), `{"payload":"`) {
		t.Fatalf("materialized map result = %#v", result)
	}
	ref, ok := result["artifactRef"].(toolArtifactRef)
	if !ok || ref.Version != 7 || ref.URI != "adk://artifacts/strategy.research_backtest-run-1-call-1.json?version=7" || !ref.Truncated {
		t.Fatalf("artifact ref = %#v", result["artifactRef"])
	}

	nonMap := execution.materializeToolOutput("backtest.result_view", "", []string{payload}).(map[string]any)
	if nonMap["truncated"] != true || nonMap["artifactRef"] == nil {
		t.Fatalf("materialized non-map result = %#v", nonMap)
	}
}

func TestMaterializeToolOutputFallsBackWithoutAnArtifactOrOnSaveFailure(t *testing.T) {
	payload := strings.Repeat("x", MaxToolOutputBytes+256)
	for _, execution := range []*googleADKExecution{
		nil,
		{runID: "run"},
		{runID: "run", artifactService: &recordingArtifactService{err: errors.New("storage unavailable")}},
		{runID: "run", artifactService: &recordingArtifactService{}},
	} {
		result := execution.materializeToolOutput("strategy.research_backtest", "call", map[string]any{"payload": payload})
		if _, hasRef := result.(map[string]any)["artifactRef"]; hasRef {
			t.Fatalf("fallback result unexpectedly has artifact ref: %#v", result)
		}
	}
	if result := (&googleADKExecution{}).materializeToolOutput("unknown.tool", "call", map[string]any{"payload": payload}); result.(map[string]any)["truncated"] != true {
		t.Fatalf("unknown tool result = %#v", result)
	}
	if result := (&googleADKExecution{}).materializeToolOutput("strategy.research_backtest", "call", make(chan int)); result == nil {
		t.Fatal("unmarshalable small result should remain available when no artifact can be written")
	}
}

func TestArtifactToolSelectionAndSafeNames(t *testing.T) {
	if !toolOutputShouldUseArtifact(" backtest.result_view ") || toolOutputShouldUseArtifact("backtest.unknown") {
		t.Fatal("artifact tool selection did not honor the explicit artifact-capable tool list")
	}
	for _, tc := range []struct {
		tool, run, call string
		want            string
	}{
		{tool: "strategy.optimize", run: "run/1", call: "call 1", want: "strategy.optimize-run-1-call-1.json"},
		{tool: " /// ", run: "", call: "", want: "tool-output.json"},
		{tool: "view", run: "", call: " id ", want: "view-id.json"},
	} {
		if got := artifactFileName(tc.tool, tc.run, tc.call); got != tc.want {
			t.Fatalf("artifactFileName(%q, %q, %q) = %q, want %q", tc.tool, tc.run, tc.call, got, tc.want)
		}
	}
}
