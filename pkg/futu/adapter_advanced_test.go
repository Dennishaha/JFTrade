package futu

import (
	"maps"
	"sort"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

func TestAdvancedFeatureDefaultsBuildStrictOpenDRequests(t *testing.T) {
	tests := []struct {
		name       string
		protocol   string
		query      broker.FeatureQuery
		wantFields []string
		absent     []string
	}{
		{
			name: "option chain", protocol: "Qot_GetOptionChain",
			query:      broker.FeatureQuery{Market: "US", InstrumentID: "US.AAPL", PageSize: 50},
			wantFields: []string{"owner", "beginTime", "endTime"}, absent: []string{"count"},
		},
		{
			name: "option screen", protocol: "Qot_OptionScreen",
			query:      broker.FeatureQuery{Market: "US", PageSize: 50},
			wantFields: []string{"marketCategoryList", "pageCount", "pageFrom"}, absent: []string{"count"},
		},
		{
			name: "warrant list", protocol: "Qot_GetWarrant",
			query:      broker.FeatureQuery{Market: "HK", InstrumentID: "HK.00700", PageSize: 50},
			wantFields: []string{"owner", "begin", "num", "sortField", "ascend"}, absent: []string{"count"},
		},
		{
			name: "top movers", protocol: "Qot_GetTopMoversRank",
			query:      broker.FeatureQuery{Market: "US", PageSize: 50},
			wantFields: []string{"market", "count", "offset"},
		},
		{
			name: "macro", protocol: "Qot_GetMacroIndicatorList",
			query:      broker.FeatureQuery{Market: "US", PageSize: 50},
			wantFields: []string{"region"}, absent: []string{"count"},
		},
		{
			name: "news", protocol: "Qot_GetSearchNews",
			query:      broker.FeatureQuery{Market: "US", InstrumentID: "US.AAPL", PageSize: 30},
			wantFields: []string{"keyword"}, absent: []string{"count"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := map[string]any{}
			injectAdvancedPageSize(params, test.protocol, test.query.PageSize)
			if err := injectFeatureInstrument(params, test.protocol, test.query.InstrumentID); err != nil {
				t.Fatalf("injectFeatureInstrument: %v", err)
			}
			if err := injectAdvancedDefaults(params, test.protocol, test.query); err != nil {
				t.Fatalf("injectAdvancedDefaults: %v", err)
			}
			for _, field := range test.wantFields {
				if _, ok := params[field]; !ok {
					t.Errorf("params = %#v, missing %q", params, field)
				}
			}
			for _, field := range test.absent {
				if _, ok := params[field]; ok {
					t.Errorf("params = %#v, unexpectedly contains %q", params, field)
				}
			}
			if err := opend.ValidateAdvancedC2S(test.protocol, params); err != nil {
				t.Fatalf("strict request validation failed: %v; params=%#v", err, params)
			}
		})
	}
}

func TestEveryAllowlistedAdvancedProtocolMapsToCatalogFeature(t *testing.T) {
	mapped := make(map[string]broker.FeatureID)
	for featureID, operations := range featureProtocols {
		if _, ok := broker.BuiltinCapabilityCatalog.Definition(featureID); !ok {
			t.Errorf("feature protocol mapping references unknown capability %s", featureID)
		}
		for _, protocol := range operations {
			if previous, exists := mapped[protocol]; exists && previous != featureID {
				t.Errorf("%s maps to both %s and %s", protocol, previous, featureID)
			}
			mapped[protocol] = featureID
		}
	}
	maps.Copy(mapped, directBrokerProtocols)

	var missing []string
	for _, protocol := range opend.AdvancedProtocols {
		featureID, ok := mapped[protocol.Key]
		if !ok {
			missing = append(missing, protocol.Key)
			continue
		}
		if _, exists := broker.BuiltinCapabilityCatalog.Definition(featureID); !exists {
			t.Errorf("%s maps to unknown catalog feature %s", protocol.Key, featureID)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("allowlisted OpenD protocols missing CapabilityCatalog mapping: %v", missing)
	}
}

func TestAdvancedProtocolReplaySafetyDefaultsToNoReplay(t *testing.T) {
	for _, protocol := range []string{
		"Qot_SetPriceReminder",
		"Qot_SetOptionEventAlert",
		"Qot_ModifyUserSecurity",
		"Qot_GetEventContractComboRfq",
		"Qot_UnknownMutation",
	} {
		if advancedProtocolReplaySafe(protocol) {
			t.Errorf("advancedProtocolReplaySafe(%q) = true, want no automatic replay", protocol)
		}
	}

	for _, protocol := range []string{
		"Qot_GetOptionChain",
		"Qot_RequestTradeDate",
		"Qot_FilterCompetition",
		"Qot_OptionScreen",
		"Qot_SubEventContract",
	} {
		if !advancedProtocolReplaySafe(protocol) {
			t.Errorf("advancedProtocolReplaySafe(%q) = false, want replay-safe", protocol)
		}
	}
}

func TestEveryDefaultFeatureOperationBuildsGeneratedRequest(t *testing.T) {
	for featureID, operation := range defaultFeatureOperations {
		protocol := featureProtocols[featureID][operation]
		if protocol == "" {
			t.Errorf("%s default operation %q has no protocol", featureID, operation)
			continue
		}
		market := "US"
		instrumentID := "US.AAPL"
		switch featureID {
		case broker.FeatureWarrants:
			market, instrumentID = "HK", "HK.00700"
		case broker.FeaturePredictionDiscover, broker.FeaturePredictionComboEligible,
			broker.FeaturePredictionComboQuote:
			instrumentID = ""
		case broker.FeaturePredictionSnapshot, broker.FeaturePredictionDepth,
			broker.FeaturePredictionHistory:
			instrumentID = "US.EC.TEST"
		case broker.FeatureOptionScreen, broker.FeatureOptionEvents,
			broker.FeatureResearchScreen, broker.FeatureResearchCalendar,
			broker.FeatureResearchMacro, broker.FeatureResearchRankings,
			broker.FeatureResearchInstitutions, broker.FeatureResearchIndustry,
			broker.FeatureOptionEventAlertList:
			instrumentID = ""
		}
		query := broker.FeatureQuery{
			FeatureID: featureID, Market: market, InstrumentID: instrumentID, PageSize: 20,
		}
		params := map[string]any{}
		injectAdvancedPageSize(params, protocol, query.PageSize)
		if err := injectFeatureInstrument(params, protocol, query.InstrumentID); err != nil {
			t.Errorf("%s inject instrument: %v", featureID, err)
			continue
		}
		if err := injectAdvancedDefaults(params, protocol, query); err != nil {
			t.Errorf("%s inject defaults: %v", featureID, err)
			continue
		}
		if featureID == broker.FeaturePredictionComboQuote {
			// RFQ content is caller-owned and cannot have a safe fabricated
			// default. Its generated request shape is covered separately.
			continue
		}
		if err := opend.ValidateAdvancedC2S(protocol, params); err != nil {
			t.Errorf("%s (%s/%s) request invalid: %v; params=%#v",
				featureID, operation, protocol, err, params)
		}
	}
}
