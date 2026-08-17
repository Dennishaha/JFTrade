package productfeatures

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestEmbeddedResearchFeatureAllowListIsExplicit(t *testing.T) {
	want := map[broker.FeatureID]struct{}{
		broker.FeatureResearchNews:            {},
		broker.FeatureResearchCorporateAction: {},
		broker.FeatureResearchRankings:        {},
		broker.FeatureResearchIndustry:        {},
		broker.FeatureResearchInstrument:      {},
		broker.FeatureResearchFinancials:      {},
		broker.FeatureResearchAnalyst:         {},
		broker.FeatureResearchOwnership:       {},
		broker.FeatureResearchCalendar:        {},
		broker.FeatureResearchMacro:           {},
		broker.FeatureResearchScreen:          {},
	}

	if len(embeddedResearchFeatureIDs) != len(want) {
		t.Fatalf("embedded feature count = %d, want %d", len(embeddedResearchFeatureIDs), len(want))
	}
	for feature := range want {
		if _, ok := embeddedResearchFeatureIDs[feature]; !ok {
			t.Fatalf("embedded feature %q is missing from the facade allow-list", feature)
		}
	}
}
