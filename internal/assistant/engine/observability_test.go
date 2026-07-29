package adk

import (
	"context"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/observability"
)

func TestADKRunContextCarriesCanonicalCorrelationFields(t *testing.T) {
	runtime := newTestRuntime(t)
	requestContext := observability.WithFields(context.Background(), observability.Fields{RequestID: "request-adk-1"})
	run, runContext, finish, err := runtime.startRun(requestContext, "session-adk-1", Agent{
		ID:         "agent-adk-1",
		ProviderID: testProviderID,
	}, "hello")
	if err != nil {
		t.Fatalf("startRun: %v", err)
	}
	defer finish()

	fields := observability.FieldsFromContext(runContext)
	if fields.RequestID != "request-adk-1" || fields.SessionID != "session-adk-1" || fields.RunID != run.ID || fields.ProviderID != testProviderID || fields.Source != "adk" {
		t.Fatalf("ADK observability fields = %#v", fields)
	}
}
