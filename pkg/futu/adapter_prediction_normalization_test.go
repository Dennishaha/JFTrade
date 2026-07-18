package futu

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestPredictionPayloadNormalizationDoesNotLeakOpenDMarkets(t *testing.T) {
	result := featureResultFromProtocolPayload(broker.FeatureQuery{
		Market: "US", InstrumentID: "US.EC.ONE",
		MarketSegment: broker.MarketSegmentPrediction,
		ProductClass:  broker.ProductClassEventContract,
		FeatureID:     broker.FeaturePredictionSnapshot,
	}, "Qot_GetEventContractSnapshot", map[string]any{
		"snapshotList": []any{map[string]any{
			"code":      map[string]any{"market": "QotMarket_Event", "code": "EC.ONE"},
			"eventCode": map[string]any{"market": "QotMarket_Event", "code": "EVENT.ONE"},
			"name":      "Will it happen?",
			"status":    "EC_Status_Active",
			"price":     0.42,
		}},
	})
	if len(result.Entries) != 1 || result.ResolvedInstrument == nil {
		t.Fatalf("result = %#v", result)
	}
	if result.ResolvedInstrument.InstrumentID != "US.EC.ONE" ||
		result.ResolvedInstrument.MarketSegment != broker.MarketSegmentPrediction ||
		result.ResolvedInstrument.Event == nil ||
		result.ResolvedInstrument.Event.EventID != "US.EVENT.ONE" {
		t.Fatalf("resolved instrument = %#v", result.ResolvedInstrument)
	}
	rawContent, _ := json.Marshal(result)
	content := string(rawContent)
	if strings.Contains(content, "QotMarket") || strings.Contains(content, `"market":101`) {
		t.Fatalf("OpenD market leaked from normalized payload: %s", content)
	}
}

func TestPredictionComboQuoteTranslationUsesBrokerNeutralLegs(t *testing.T) {
	params := map[string]any{
		"mvc": "mvc-1",
		"legs": []any{
			map[string]any{"instrumentId": "US.EC.ONE", "side": "BUY", "ratio": float64(1), "predictionSide": "YES"},
			map[string]any{"instrumentId": "US.EC.TWO", "side": "SELL", "ratio": float64(2), "predictionSide": "NO"},
		},
	}
	if err := translatePredictionComboQuoteParams(params); err != nil {
		t.Fatalf("translatePredictionComboQuoteParams: %v", err)
	}
	if params["legs"] != nil {
		t.Fatal("broker-neutral legs were forwarded to OpenD")
	}
	legs, ok := params["comboLegList"].([]any)
	if !ok || len(legs) != 2 {
		t.Fatalf("comboLegList = %#v", params["comboLegList"])
	}
	first := legs[0].(map[string]any)
	security := first["security"].(map[string]any)
	if security["market"] != 101 || first["predSide"] != 1 {
		t.Fatalf("first OpenD leg = %#v", first)
	}

	result := featureResultFromProtocolPayload(broker.FeatureQuery{
		FeatureID: broker.FeaturePredictionComboQuote,
		Params:    map[string]any{"mvc": "mvc-1"},
	}, "Qot_GetEventContractComboRfq", map[string]any{
		"quoteId": "rfq-1", "bidPrice": 0.3, "askPrice": 0.4,
		"comboLegList": []any{},
	})
	if result.Metadata["quoteId"] != "rfq-1" || result.Metadata["quoteExpiresAt"] != nil {
		t.Fatalf("adapter RFQ metadata = %#v", result.Metadata)
	}
}

func TestInternalFutureMarketNormalizesToPublicProductIdentity(t *testing.T) {
	value := normalizeOpenDValue(map[string]any{
		"market": "QotMarket_HK_Future", "code": "HSI2607",
	}).(map[string]any)
	if value["market"] != "HK" || value["productClass"] != "future" ||
		value["instrumentId"] != "HK.HSI2607" {
		t.Fatalf("normalized future = %#v", value)
	}
}
