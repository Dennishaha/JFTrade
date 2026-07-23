package futu

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

func translatePredictionComboQuoteParams(params map[string]any) error {
	rawLegs, ok := params["legs"].([]any)
	if !ok || len(rawLegs) < 2 {
		return fmt.Errorf("futu: prediction combo quote requires at least two legs")
	}
	comboLegs := make([]any, 0, len(rawLegs))
	for index, raw := range rawLegs {
		leg, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("futu: prediction combo leg %d is invalid", index)
		}
		instrumentID := strings.ToUpper(strings.TrimSpace(stringValue(leg["instrumentId"])))
		if !strings.HasPrefix(instrumentID, "US.") {
			return fmt.Errorf("futu: prediction combo leg %d must be a US event contract", index)
		}
		code := strings.TrimPrefix(instrumentID, "US.")
		if code == "" {
			return fmt.Errorf("futu: prediction combo leg %d must be a US event contract", index)
		}
		predictionSide := strings.ToUpper(strings.TrimSpace(stringValue(leg["predictionSide"])))
		predSide := 0
		switch predictionSide {
		case "YES":
			predSide = 1
		case "NO":
			predSide = 2
		default:
			return fmt.Errorf("futu: prediction combo leg %d has invalid predictionSide", index)
		}
		side := 1
		if strings.EqualFold(stringValue(leg["side"]), "SELL") {
			side = 2
		}
		ratio := numberValue(leg["ratio"], 1)
		if ratio <= 0 {
			return fmt.Errorf("futu: prediction combo leg %d ratio must be positive", index)
		}
		comboLegs = append(comboLegs, map[string]any{
			"security": map[string]any{"market": 101, "code": code},
			"side":     side, "qtyRatio": ratio, "predSide": predSide,
		})
	}
	delete(params, "legs")
	params["comboLegList"] = comboLegs
	return nil
}

func featureResult(query broker.FeatureQuery, entries []map[string]any, metadata map[string]any) *broker.FeatureResult {
	now := time.Now().UTC()
	return &broker.FeatureResult{
		Provider: broker.ProviderAttribution{
			BrokerID:        string(Name),
			SecurityFirm:    "Futu/Moomoo via OpenD",
			FeatureID:       query.FeatureID,
			Capability:      broker.CapabilityAvailable,
			SelectionReason: "adapter_request",
			ResolvedAt:      now,
			AsOf:            now,
		},
		AsOf:     now,
		Entries:  entries,
		Metadata: metadata,
	}
}

func injectFeatureInstrument(params map[string]any, protocol, instrumentID string) error {
	instrumentID = strings.TrimSpace(instrumentID)
	if instrumentID == "" {
		return nil
	}
	field := protocolInstrumentField[protocol]
	if field == "" {
		if !opend.AdvancedC2SHasField(protocol, "security") {
			return nil
		}
		field = "security"
	}
	if _, exists := params[field]; exists {
		return nil
	}
	if strings.Contains(protocol, "EventContract") || protocol == "Qot_FilterCompetition" {
		code := strings.TrimPrefix(strings.ToUpper(instrumentID), "US.")
		value := map[string]any{"market": 101, "code": code}
		assignAdvancedInstrumentValue(params, field, value)
		return nil
	}
	security, _, err := futuSecurityFromSymbol(instrumentID)
	if err != nil {
		return err
	}
	value := map[string]any{"market": security.GetMarket(), "code": security.GetCode()}
	assignAdvancedInstrumentValue(params, field, value)
	return nil
}

func assignAdvancedInstrumentValue(params map[string]any, field string, value map[string]any) {
	switch field {
	case "securityList", "ownerList":
		params[field] = []any{value}
	case "multi_legs":
		params[field] = []any{map[string]any{"security": value, "side": 1, "qtyRatio": 1}}
	default:
		params[field] = value
	}
}

func defaultOperation(protocols map[string]string) string {
	if len(protocols) == 1 {
		for operation := range protocols {
			return operation
		}
	}
	operations := make([]string, 0, len(protocols))
	for operation := range protocols {
		operations = append(operations, operation)
	}
	sort.Strings(operations)
	return operations[0]
}

func payloadEntries(payload map[string]any) ([]map[string]any, map[string]any) {
	metadata := cloneMap(payload)
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values, ok := payload[key].([]any)
		if !ok {
			continue
		}
		entries := make([]map[string]any, 0, len(values))
		for _, value := range values {
			if entry, valid := value.(map[string]any); valid {
				entries = append(entries, entry)
			}
		}
		delete(metadata, key)
		return entries, metadata
	}
	if payloadContainsOnlyPaginationMetadata(payload) {
		return []map[string]any{}, metadata
	}
	if len(payload) == 0 {
		return []map[string]any{}, nil
	}
	return []map[string]any{payload}, nil
}

func payloadContainsOnlyPaginationMetadata(payload map[string]any) bool {
	if len(payload) == 0 {
		return false
	}
	for key := range payload {
		switch key {
		case "nextPage", "nextKey", "hasMore", "total", "totalCount", "allCount", "currency":
		default:
			return false
		}
	}
	return true
}
