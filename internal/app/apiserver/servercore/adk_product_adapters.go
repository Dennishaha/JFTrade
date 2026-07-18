package servercore

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/jftrade/jftrade-main/internal/productfeatures"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

var productToolFeatureIDs = map[string]broker.FeatureID{
	"market.instrument_profile":     broker.FeatureInstrumentProfile,
	"market.candles":                broker.FeatureMarketCandles,
	"market.intraday":               broker.FeatureMarketIntraday,
	"market.ticks":                  broker.FeatureMarketTicks,
	"market.depth":                  broker.FeatureMarketDepth,
	"market.broker_queue":           broker.FeatureMarketBrokerQueue,
	"market.capital_flow":           broker.FeatureMarketCapitalFlow,
	"derivatives.option_chain":      broker.FeatureOptionChain,
	"derivatives.option_screen":     broker.FeatureOptionScreen,
	"derivatives.option_analysis":   broker.FeatureOptionAnalysis,
	"derivatives.option_events":     broker.FeatureOptionEvents,
	"derivatives.warrants":          broker.FeatureWarrants,
	"derivatives.futures":           broker.FeatureFutures,
	"research.instrument":           broker.FeatureResearchInstrument,
	"research.financials":           broker.FeatureResearchFinancials,
	"research.valuation":            broker.FeatureResearchValuation,
	"research.analyst":              broker.FeatureResearchAnalyst,
	"research.ownership":            broker.FeatureResearchOwnership,
	"research.corporate_actions":    broker.FeatureResearchCorporateAction,
	"research.short_interest":       broker.FeatureResearchShortInterest,
	"research.news":                 broker.FeatureResearchNews,
	"research.screen":               broker.FeatureResearchScreen,
	"research.calendar":             broker.FeatureResearchCalendar,
	"research.macro":                broker.FeatureResearchMacro,
	"research.rankings":             broker.FeatureResearchRankings,
	"research.institutions":         broker.FeatureResearchInstitutions,
	"research.industry":             broker.FeatureResearchIndustry,
	"research.technical_indicators": broker.FeatureTechnicalIndicator,
	"prediction.discover":           broker.FeaturePredictionDiscover,
	"prediction.snapshot":           broker.FeaturePredictionSnapshot,
	"prediction.depth":              broker.FeaturePredictionDepth,
	"prediction.history":            broker.FeaturePredictionHistory,
	"prediction.combo_eligible":     broker.FeaturePredictionComboEligible,
	"prediction.combo_quote":        broker.FeaturePredictionComboQuote,
	"alerts.price.list":             broker.FeaturePriceAlertList,
	"alerts.option_event.list":      broker.FeatureOptionEventAlertList,
	"watchlist.remote.list":         broker.FeatureRemoteWatchlistList,
}

var customizationToolFeatureIDs = map[string]broker.FeatureID{
	"alerts.price.set":        broker.FeaturePriceAlertSet,
	"alerts.option_event.set": broker.FeatureOptionEventAlertSet,
	"watchlist.remote.modify": broker.FeatureRemoteWatchlistModify,
}

var customizationToolActions = map[string]string{
	"alerts.price.set":        "set",
	"alerts.option_event.set": "set",
	"watchlist.remote.modify": "modify",
}

func (s *Server) invokeADKProductTool(ctx context.Context, name string, input map[string]any) (any, error) {
	switch name {
	case "market.capabilities":
		return s.productFeaturesSvc.CapabilitiesContext(ctx, productfeatures.CapabilityQuery{
			BrokerID: toolMapString(input, "brokerId"), AccountID: toolMapString(input, "accountId"),
			TradingEnvironment: toolMapString(input, "tradingEnvironment"),
			Market:             strings.ToUpper(toolMapString(input, "market")),
			FeatureID:          broker.FeatureID(toolMapString(input, "featureId")),
		}), nil
	case "market.search":
		return s.adkProductSearch(ctx, input)
	case "market.snapshot":
		return s.adkProductSnapshots(ctx, input, broker.FeatureMarketSnapshot)
	case "market.snapshots":
		return s.adkProductSnapshots(ctx, input, broker.FeatureMarketSnapshots)
	case "execution.buying_power":
		return s.adkProductBuyingPower(ctx, input)
	}
	if featureID, ok := customizationToolFeatureIDs[name]; ok {
		payload := cloneToolInput(input)
		if nested, ok := input["payload"].(map[string]any); ok {
			payload = nested
		}
		return s.productFeaturesSvc.ApplyCustomization(ctx, broker.CustomizationAction{
			FeatureID: featureID, BrokerID: toolMapString(input, "brokerId"),
			AccountID: toolMapString(input, "accountId"), Action: customizationToolActions[name], Payload: payload,
		})
	}
	featureID, ok := productToolFeatureIDs[name]
	if !ok {
		return nil, fmt.Errorf("unknown product tool %q", name)
	}
	query := broker.FeatureQuery{
		BrokerID: toolMapString(input, "brokerId"), AccountID: toolMapString(input, "accountId"),
		Market: strings.ToUpper(toolMapString(input, "market")), InstrumentID: toolInstrumentID(input),
		FeatureID: featureID, Cursor: toolMapString(input, "cursor"),
		PageSize: min(max(toolMapInt(input, "pageSize", 50), 1), 100), Params: cloneToolInput(input),
	}
	delete(query.Params, "brokerId")
	delete(query.Params, "accountId")
	delete(query.Params, "market")
	delete(query.Params, "instrumentId")
	delete(query.Params, "cursor")
	delete(query.Params, "pageSize")
	if strings.HasPrefix(name, "derivatives.") {
		query.MarketSegment = broker.MarketSegmentDerivatives
	}
	if strings.HasPrefix(name, "prediction.") {
		query.Market = "US"
		query.MarketSegment = broker.MarketSegmentPrediction
		query.ProductClass = broker.ProductClassEventContract
	}
	if operation := toolMapString(input, "operation"); operation != "" {
		query.Params["operation"] = operation
	}
	return s.productFeaturesSvc.Query(ctx, query)
}

func (s *Server) invokeADKExecutionTool(ctx context.Context, name string, input map[string]any) (any, error) {
	switch name {
	case "execution.order_preview":
		var request trdsrv.ExecutionPlaceRequest
		if err := decodeToolInput(input, &request); err != nil {
			return nil, err
		}
		return s.tradingSvc.PreviewExecutionOrderContext(ctx, request)
	case "execution.order_place":
		var request trdsrv.ExecutionPlaceRequest
		if err := decodeToolInput(input, &request); err != nil {
			return nil, err
		}
		return s.tradingSvc.CreateExecutionOrder(ctx, request)
	case "execution.order_cancel":
		return s.tradingSvc.CancelExecutionOrder(ctx, toolMapString(input, "internalOrderId"))
	case "execution.combo_preview":
		var request trdsrv.ExecutionComboRequest
		if err := decodeToolInput(input, &request); err != nil {
			return nil, err
		}
		return s.tradingSvc.PreviewExecutionCombo(ctx, request)
	case "execution.combo_place":
		var request trdsrv.ExecutionComboRequest
		if err := decodeToolInput(input, &request); err != nil {
			return nil, err
		}
		return s.tradingSvc.CreateExecutionCombo(ctx, request)
	case "execution.combo_cancel":
		return s.tradingSvc.CancelExecutionCombo(ctx, toolMapString(input, "internalOrderId"))
	default:
		return nil, fmt.Errorf("unknown execution tool %q", name)
	}
}

func (s *Server) adkProductSearch(ctx context.Context, input map[string]any) (any, error) {
	return s.productFeaturesSvc.Query(ctx, broker.FeatureQuery{
		BrokerID:  toolMapString(input, "brokerId"),
		AccountID: toolMapString(input, "accountId"),
		Market:    strings.ToUpper(toolMapString(input, "market")),
		FeatureID: broker.FeatureMarketSearch,
		PageSize:  min(max(toolMapInt(input, "pageSize", 20), 1), 100),
		Params:    map[string]any{"keyword": toolMapString(input, "query")},
	})
}

func (s *Server) adkProductSnapshots(
	ctx context.Context,
	input map[string]any,
	featureID broker.FeatureID,
) (any, error) {
	symbols := toolMapStrings(input, "symbols")
	if instrumentID := toolInstrumentID(input); instrumentID != "" {
		symbols = append(symbols, instrumentID)
	}
	if len(symbols) == 0 {
		return nil, fmt.Errorf("instrumentId or symbols is required")
	}
	return s.productFeaturesSvc.BatchSnapshots(ctx, broker.FeatureQuery{
		BrokerID:  toolMapString(input, "brokerId"),
		AccountID: toolMapString(input, "accountId"),
		Market:    strings.ToUpper(toolMapString(input, "market")),
		FeatureID: featureID,
	}, symbols)
}

func (s *Server) adkProductBuyingPower(ctx context.Context, input map[string]any) (any, error) {
	var query broker.ProductRuleQuery
	if err := decodeToolInput(input, &query); err != nil {
		return nil, err
	}
	query.BrokerID = toolMapString(input, "brokerId")
	query.FeatureID = broker.FeatureExecutionBuyingPower
	return s.tradingSvc.PreviewExecutionBuyingPower(ctx, query)
}

func decodeToolInput(input map[string]any, output any) error {
	content, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode tool input: %w", err)
	}
	if err := json.Unmarshal(content, output); err != nil {
		return fmt.Errorf("decode tool input: %w", err)
	}
	return nil
}

func toolInstrumentID(input map[string]any) string {
	if value := strings.ToUpper(strings.TrimSpace(toolMapString(input, "instrumentId"))); value != "" {
		return value
	}
	market := strings.ToUpper(strings.TrimSpace(toolMapString(input, "market")))
	symbol := strings.ToUpper(strings.TrimSpace(toolMapString(input, "symbol")))
	if market != "" && symbol != "" {
		return market + "." + symbol
	}
	return symbol
}

func toolMapString(input map[string]any, key string) string {
	value, ok := input[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func toolMapInt(input map[string]any, key string, fallback int) int {
	var result int
	if _, err := fmt.Sscan(toolMapString(input, key), &result); err != nil {
		return fallback
	}
	return result
}

func toolMapStrings(input map[string]any, key string) []string {
	values, ok := input[key].([]any)
	if !ok {
		if direct, directOK := input[key].([]string); directOK {
			return append([]string(nil), direct...)
		}
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := strings.TrimSpace(fmt.Sprint(value)); normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func cloneToolInput(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	maps.Copy(result, input)
	return result
}
