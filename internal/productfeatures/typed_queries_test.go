package productfeatures

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestDocumentResultPreservesFeatureWireShape(t *testing.T) {
	total := 1
	hasMore := false
	source := &broker.FeatureResult{
		Provider: broker.ProviderAttribution{
			BrokerID: "futu", FeatureID: broker.FeatureResearchCalendar,
			Capability: broker.CapabilityAvailable,
		},
		AsOf:       time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
		Entries:    []map[string]any{{"instrumentId": "US.AAPL", "value": 1.25, "active": true}},
		NextCursor: "cursor-2", HasMore: &hasMore, Total: &total,
		Warnings: []string{"partial"},
		Metadata: map[string]any{"source": "typed", "count": 1.0},
	}

	documents, err := documentResult(source)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := documents.FeatureResult()
	if err != nil {
		t.Fatal(err)
	}

	if got, want := normalizedJSON(t, projected), normalizedJSON(t, source); !reflect.DeepEqual(got, want) {
		t.Fatalf("projected wire = %#v, want %#v", got, want)
	}
}

func TestTypedCapabilityDescriptionsAreDefensive(t *testing.T) {
	description, ok := TypedCapabilityForTool("research.calendar")
	if !ok || description.FeatureID != broker.FeatureResearchCalendar {
		t.Fatalf("calendar description = %#v, %v", description, ok)
	}
	description.Operations[0] = "mutated"
	again, _ := TypedCapabilityForTool("research.calendar")
	if again.Operations[0] != "earnings" {
		t.Fatalf("shared operations were mutated: %#v", again.Operations)
	}
}

func normalizedJSON(t *testing.T, value any) any {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var normalized any
	if err := json.Unmarshal(content, &normalized); err != nil {
		t.Fatal(err)
	}
	return normalized
}
