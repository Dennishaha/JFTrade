package opend

import "testing"

func TestAdvancedProtocolsAreUniqueAndRegistered(t *testing.T) {
	if len(AdvancedProtocols) < 110 {
		t.Fatalf("advanced protocol count = %d, want complete 10.9 product surface", len(AdvancedProtocols))
	}
	seenIDs := make(map[uint32]string, len(AdvancedProtocols))
	for _, protocol := range AdvancedProtocols {
		if previous, ok := seenIDs[protocol.ID]; ok {
			t.Fatalf("protocol id %d used by %s and %s", protocol.ID, previous, protocol.Key)
		}
		seenIDs[protocol.ID] = protocol.Key
		if _, err := newRegisteredMessage(protocol.Package + ".Request"); err != nil {
			t.Errorf("%s request is not registered: %v", protocol.Key, err)
		}
		if _, err := newRegisteredMessage(protocol.Package + ".Response"); err != nil {
			t.Errorf("%s response is not registered: %v", protocol.Key, err)
		}
	}
}

func TestAdvancedProtocolPredictionIDs(t *testing.T) {
	want := map[string]uint32{
		"Qot_GetEventContractCategory":      3434,
		"Qot_FilterCompetition":             3435,
		"Qot_GetEventContractSeriesList":    3436,
		"Qot_GetEventContractEventList":     3437,
		"Qot_GetEventContract":              3438,
		"Qot_GetEventContractMilestoneList": 3439,
		"Qot_GetEventContractSnapshot":      3445,
		"Qot_GetEventContractOrderBook":     3446,
		"Qot_GetEventContractKline":         3447,
		"Qot_GetEventContractTicker":        3448,
		"Qot_GetEventContractComboList":     3453,
		"Qot_GetEventContractComboRfq":      3454,
		"Qot_SubEventContract":              3455,
		"Qot_RequestHistoryEventContractKL": 3456,
	}
	for key, id := range want {
		if got := advancedProtocolByKey[key].ID; got != id {
			t.Errorf("%s id = %d, want %d", key, got, id)
		}
	}
	pushIDs := map[string]uint32{
		"Qot_UpdateEventContractOrderBook": ProtoQotUpdateEventContractOrderBook,
		"Qot_UpdateEventContractKline":     ProtoQotUpdateEventContractKline,
		"Qot_UpdateEventContractTicker":    ProtoQotUpdateEventContractTicker,
	}
	for key, got := range pushIDs {
		want := map[string]uint32{
			"Qot_UpdateEventContractOrderBook": 3450,
			"Qot_UpdateEventContractKline":     3451,
			"Qot_UpdateEventContractTicker":    3452,
		}[key]
		if got != want {
			t.Errorf("%s id = %d, want %d", key, got, want)
		}
		if _, err := newRegisteredMessage(key + ".Response"); err != nil {
			t.Errorf("%s push response is not registered: %v", key, err)
		}
	}
}

func TestAdvancedC2SFieldInspectionAndStrictValidation(t *testing.T) {
	if !AdvancedC2SHasField("Qot_GetWarrant", "num") {
		t.Fatal("Qot_GetWarrant num field was not discovered")
	}
	if AdvancedC2SHasField("Qot_GetWarrant", "count") {
		t.Fatal("Qot_GetWarrant must not accept generic count")
	}
	if err := ValidateAdvancedC2S("Qot_GetWarrant", map[string]any{"count": 10}); err == nil {
		t.Fatal("strict validation accepted an unknown pagination field")
	}
}
