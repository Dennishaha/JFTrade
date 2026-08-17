package productfeatures

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

// projectProviderScreen converts a provider-neutral screen page into the
// broker feature envelope. Row keys and cell semantics mirror the Futu
// normalization (pkg/futu/stock_screen_normalization.go): basic.* columns
// project from the entry identity, numeric factors read the values map, and a
// factor absent from the response still emits a cell with a missing value so
// the console table keeps every requested column.
func projectProviderScreen(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	definition broker.ScreenDefinitionV2,
	response marketdata.ScreenResponse,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0, len(response.Entries))
	for _, entry := range response.Entries {
		entries = append(entries, projectProviderScreenRow(definition, entry))
	}
	asOf := now
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(response.AsOf)); err == nil {
		asOf = parsed.UTC()
	}
	total := response.Total
	hasMore := response.HasMore
	result := &broker.FeatureResult{
		Provider: broker.ProviderAttribution{
			BrokerID:        strings.TrimSpace(descriptor.BrokerID),
			FeatureID:       query.FeatureID,
			Capability:      broker.CapabilityAvailable,
			SelectionReason: embeddedProviderSelectionReason,
			ResolvedAt:      now,
			AsOf:            asOf,
		},
		AsOf:     asOf,
		Entries:  entries,
		HasMore:  &hasMore,
		Total:    &total,
		Metadata: map[string]any{"source": response.Source},
	}
	if hasMore {
		currentOffset := embeddedScreenOffset(query)
		nextOffset := currentOffset + len(entries)
		if response.NextOffset != nil && *response.NextOffset > currentOffset {
			nextOffset = *response.NextOffset
		}
		result.NextCursor = strconv.Itoa(nextOffset)
	}
	return result
}

func projectProviderScreenRow(
	definition broker.ScreenDefinitionV2,
	entry marketdata.ScreenEntry,
) map[string]any {
	instrumentID := strings.ToUpper(strings.TrimSpace(entry.InstrumentID))
	market := definition.Market
	symbol := strings.ToUpper(strings.TrimSpace(entry.Symbol))
	if prefix, code, found := strings.Cut(instrumentID, "."); found {
		market = prefix
		if symbol == "" {
			symbol = code
		}
	}
	row := map[string]any{
		"stockId":      instrumentID,
		"instrumentId": instrumentID,
		"market":       market,
		"symbol":       symbol,
		"productClass": broker.ProductClassEquity,
		"cells":        projectProviderScreenCells(definition, entry, symbol),
	}
	if name := strings.TrimSpace(entry.Name); name != "" {
		row["name"] = name
	}
	if entry.Industry != nil {
		if industry := strings.TrimSpace(*entry.Industry); industry != "" {
			row["industry"] = industry
		}
	}
	if currency := strings.ToUpper(strings.TrimSpace(entry.QuoteCurrency)); currency != "" {
		row["quoteCurrency"] = currency
	}
	return row
}

func projectProviderScreenCells(
	definition broker.ScreenDefinitionV2,
	entry marketdata.ScreenEntry,
	symbol string,
) map[string]broker.ScreenResultCell {
	cells := make(map[string]broker.ScreenResultCell, len(definition.Columns))
	for _, column := range definition.Columns {
		factor, _ := researchscreen.LookupEmbedded(column.Factor.FactorKey)
		cell := broker.ScreenResultCell{
			ColumnID:   column.ID,
			InstanceID: column.Factor.InstanceID,
			FactorKey:  factor.Key,
			Value:      broker.ResearchScreenValue{Type: "missing", Unit: factor.Unit},
		}
		switch factor.Key {
		case "basic.code":
			cell.Value = screenStringValue(symbol, factor.Unit)
		case "basic.name":
			cell.Value = screenStringValue(strings.TrimSpace(entry.Name), factor.Unit)
		case "basic.industry":
			if entry.Industry != nil {
				cell.Value = screenStringValue(strings.TrimSpace(*entry.Industry), factor.Unit)
			}
		default:
			if raw, ok := entry.Values[factor.Key]; ok {
				cell.Value = screenNumericValue(factor, raw)
			}
		}
		cells[column.ID] = cell
	}
	return cells
}

func screenStringValue(value, unit string) broker.ResearchScreenValue {
	if value == "" {
		return broker.ResearchScreenValue{Type: "missing", Unit: unit}
	}
	return broker.ResearchScreenValue{Type: "string", String: &value, Unit: unit}
}

// screenNumericValue renders integers through the integer cell type (the
// Futu ival mapping) and every other factor through number (dval).
func screenNumericValue(factor researchscreen.FactorDescriptor, raw json.Number) broker.ResearchScreenValue {
	if factor.ValueType == "integer" {
		if parsed, err := raw.Int64(); err == nil {
			return broker.ResearchScreenValue{Type: "integer", Integer: &parsed, Unit: factor.Unit}
		}
	}
	if parsed, err := raw.Float64(); err == nil {
		return broker.ResearchScreenValue{Type: "number", Number: &parsed, Unit: factor.Unit}
	}
	return broker.ResearchScreenValue{Type: "missing", Unit: factor.Unit}
}
