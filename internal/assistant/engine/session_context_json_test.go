package adk

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionContextSnapshotJSONOmitsZeroRawBreakdown(t *testing.T) {
	raw, err := json.Marshal(SessionContextSnapshot{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("Marshal zero snapshot: %v", err)
	}
	if strings.Contains(string(raw), `"rawBreakdown"`) {
		t.Fatalf("zero snapshot JSON = %s, unexpectedly contains rawBreakdown", raw)
	}

	raw, err = json.Marshal(SessionContextSnapshot{
		SessionID: "session-1",
		RawBreakdown: SessionContextBreakdown{
			OtherVisibleTokens: 12,
		},
	})
	if err != nil {
		t.Fatalf("Marshal nonzero snapshot: %v", err)
	}
	if !strings.Contains(string(raw), `"rawBreakdown"`) {
		t.Fatalf("nonzero snapshot JSON = %s, missing rawBreakdown", raw)
	}
}
