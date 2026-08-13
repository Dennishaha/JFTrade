package productfeatures

import "github.com/jftrade/jftrade-main/pkg/broker"

// ToolSchemaKind identifies the stable schema shape shared by the HTTP
// capability and its Assistant tool projection.
type ToolSchemaKind string

const (
	ToolSchemaInstrument          ToolSchemaKind = "instrument"
	ToolSchemaCollection          ToolSchemaKind = "collection"
	ToolSchemaPredictionDiscovery ToolSchemaKind = "prediction-discovery"
	ToolSchemaPredictionQuote     ToolSchemaKind = "prediction-quote"
)

// TypedCapabilityDescription is the single internal description for the
// high-frequency product capabilities that have typed service ports.
type TypedCapabilityDescription struct {
	ToolName   string
	FeatureID  broker.FeatureID
	SchemaKind ToolSchemaKind
	Operations []string
}

var typedCapabilityDescriptions = []TypedCapabilityDescription{
	{
		ToolName: "research.instrument", FeatureID: broker.FeatureResearchInstrument,
		SchemaKind: ToolSchemaInstrument,
		Operations: []string{"profile", "executives", "executive_background", "operational_efficiency", "top_brokers"},
	},
	{
		ToolName: "research.financials", FeatureID: broker.FeatureResearchFinancials,
		SchemaKind: ToolSchemaInstrument,
		Operations: []string{"statements", "revenue_breakdown", "earnings_price_move", "earnings_price_history"},
	},
	{
		ToolName: "research.valuation", FeatureID: broker.FeatureResearchValuation,
		SchemaKind: ToolSchemaInstrument, Operations: []string{"detail", "constituents"},
	},
	{
		ToolName: "research.analyst", FeatureID: broker.FeatureResearchAnalyst,
		SchemaKind: ToolSchemaInstrument, Operations: []string{"consensus", "ratings", "morningstar", "changes"},
	},
	{
		ToolName: "research.ownership", FeatureID: broker.FeatureResearchOwnership,
		SchemaKind: ToolSchemaInstrument,
		Operations: []string{"overview", "changes", "holders", "institutional", "insider_holders", "insider_transactions", "management_changes"},
	},
	{
		ToolName: "research.corporate_actions", FeatureID: broker.FeatureResearchCorporateAction,
		SchemaKind: ToolSchemaInstrument, Operations: []string{"dividends", "buybacks", "splits", "code_changes"},
	},
	{
		ToolName: "research.short_interest", FeatureID: broker.FeatureResearchShortInterest,
		SchemaKind: ToolSchemaInstrument, Operations: []string{"daily_volume", "short_interest"},
	},
	{
		ToolName: "research.screen", FeatureID: broker.FeatureResearchScreen,
		SchemaKind: ToolSchemaCollection, Operations: []string{"stock_v1", "stock_v2"},
	},
	{
		ToolName: "research.calendar", FeatureID: broker.FeatureResearchCalendar,
		SchemaKind: ToolSchemaCollection,
		Operations: []string{"earnings", "dividends", "economic", "ipos", "trade_dates"},
	},
	{
		ToolName: "research.rankings", FeatureID: broker.FeatureResearchRankings,
		SchemaKind: ToolSchemaCollection,
		Operations: []string{"earnings_beat", "dividend", "pre_market", "after_hours", "overnight", "top_movers", "hot", "short_selling", "period_change", "high_dividend_state", "heatmap", "rise_fall_distribution", "market_state", "fund_catalog"},
	},
	{
		ToolName: "prediction.discover", FeatureID: broker.FeaturePredictionDiscover,
		SchemaKind: ToolSchemaPredictionDiscovery,
		Operations: []string{"categories", "competitions", "series", "events", "contracts", "milestones"},
	},
	{
		ToolName: "prediction.snapshot", FeatureID: broker.FeaturePredictionSnapshot,
		SchemaKind: ToolSchemaInstrument,
	},
	{
		ToolName: "prediction.depth", FeatureID: broker.FeaturePredictionDepth,
		SchemaKind: ToolSchemaInstrument,
	},
	{
		ToolName: "prediction.history", FeatureID: broker.FeaturePredictionHistory,
		SchemaKind: ToolSchemaInstrument, Operations: []string{"candles", "historical", "ticks"},
	},
	{
		ToolName: "prediction.combo_eligible", FeatureID: broker.FeaturePredictionComboEligible,
		SchemaKind: ToolSchemaCollection,
	},
	{
		ToolName: "prediction.combo_quote", FeatureID: broker.FeaturePredictionComboQuote,
		SchemaKind: ToolSchemaPredictionQuote,
	},
}

// TypedCapabilityDescriptions returns defensive copies so consumers cannot
// mutate the shared capability or operation contracts.
func TypedCapabilityDescriptions() []TypedCapabilityDescription {
	result := make([]TypedCapabilityDescription, 0, len(typedCapabilityDescriptions))
	for _, description := range typedCapabilityDescriptions {
		description.Operations = append([]string(nil), description.Operations...)
		result = append(result, description)
	}
	return result
}

// TypedCapabilityForTool resolves one typed capability by Assistant tool name.
func TypedCapabilityForTool(name string) (TypedCapabilityDescription, bool) {
	for _, description := range typedCapabilityDescriptions {
		if description.ToolName == name {
			description.Operations = append([]string(nil), description.Operations...)
			return description, true
		}
	}
	return TypedCapabilityDescription{}, false
}

func typedFeatureID(toolName string) broker.FeatureID {
	description, ok := TypedCapabilityForTool(toolName)
	if !ok {
		panic("missing typed product capability: " + toolName)
	}
	return description.FeatureID
}
