package assistant

import (
	"encoding/json"
	"testing"

	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestADKChatStreamTransportPreservesEventIdentityAndPayload(t *testing.T) {
	event := adkChatStreamEvent{
		Type: "run", StreamID: "stream-1", Sequence: 7, RunID: "run-1",
		Run: &assistantmodel.Run{ID: "run-1", Status: assistantmodel.RunStatusRunning},
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal stream event: %v", err)
	}
	var decoded adkChatStreamEvent
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal stream event: %v", err)
	}
	if decoded.Type != "run" || decoded.StreamID != "stream-1" || decoded.Sequence != 7 || decoded.RunID != "run-1" {
		t.Fatalf("decoded event identity = %#v", decoded)
	}
	if decoded.Run == nil || decoded.Run.ID != "run-1" || decoded.Run.Status != assistantmodel.RunStatusRunning {
		t.Fatalf("decoded run payload = %#v", decoded.Run)
	}
}
