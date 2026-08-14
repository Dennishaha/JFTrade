package assembly

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/jftrade/jftrade-main/internal/productfeatures"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
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
	"research.news":                 broker.FeatureResearchNews,
	"research.macro":                broker.FeatureResearchMacro,
	"research.institutions":         broker.FeatureResearchInstitutions,
	"research.industry":             broker.FeatureResearchIndustry,
	"research.technical_indicators": broker.FeatureTechnicalIndicator,
	"alerts.price.list":             broker.FeaturePriceAlertList,
	"alerts.option_event.list":      broker.FeatureOptionEventAlertList,
	"watchlist.remote.list":         broker.FeatureRemoteWatchlistList,
}

func init() {
	for _, description := range productfeatures.TypedCapabilityDescriptions() {
		productToolFeatureIDs[description.ToolName] = description.FeatureID
	}
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

// ProductFeatureService is the broker-neutral product surface consumed by
// assistant tool assembly.
type ProductFeatureService interface {
	CapabilitiesContext(context.Context, productfeatures.CapabilityQuery) map[string]any
	Query(context.Context, broker.FeatureQuery) (*broker.FeatureResult, error)
	BatchSnapshots(context.Context, broker.FeatureQuery, []string) (*broker.FeatureResult, error)
	ApplyCustomization(context.Context, broker.CustomizationAction) (*broker.CustomizationResult, error)
}

type typedProductFeatureService interface {
	QueryScreen(context.Context, broker.ScreenQueryV2) (broker.ResearchScreenResult, error)
	QueryCalendar(context.Context, productfeatures.CalendarRequest) (*productfeatures.DocumentResult, error)
}

// ExecutionService is the trading surface consumed by assistant tool
// assembly. The adapter never reaches into stores or broker implementations.
type ExecutionService interface {
	PreviewExecutionOrderContext(context.Context, trdsrv.ExecutionPlaceRequest) (trdsrv.ExecutionPreview, error)
	CreateExecutionOrder(context.Context, trdsrv.ExecutionPlaceRequest) (trdsrv.ExecutionCommandResponse, error)
	CancelExecutionOrder(context.Context, string) (trdsrv.ExecutionCommandResponse, error)
	PreviewExecutionCombo(context.Context, trdsrv.ExecutionComboRequest) (trdsrv.ExecutionComboPreview, error)
	CreateExecutionCombo(context.Context, trdsrv.ExecutionComboRequest) (trdsrv.ExecutionCommandResponse, error)
	CancelExecutionCombo(context.Context, string) (trdsrv.ExecutionCommandResponse, error)
	PreviewExecutionBuyingPower(context.Context, broker.ProductRuleQuery) (*broker.ProductRuleResult, error)
}

// ProductExecutionAdapter converts assistant tool inputs into product and
// trading service requests.
type ProductExecutionAdapter struct {
	productFeatures ProductFeatureService
	execution       ExecutionService
}

// NewProductExecutionAdapter creates the cross-domain assistant tool adapter.
func NewProductExecutionAdapter(
	productFeatures ProductFeatureService,
	execution ExecutionService,
) *ProductExecutionAdapter {
	return &ProductExecutionAdapter{productFeatures: productFeatures, execution: execution}
}

// InvokeProductTool dispatches a product tool without exposing service details
// to the assistant runtime.
func (a *ProductExecutionAdapter) InvokeProductTool(
	ctx context.Context,
	name string,
	input map[string]any,
) (any, error) {
	if result, handled, err := a.invokeSpecialProductTool(ctx, name, input); handled {
		return result, err
	}
	return a.invokeGenericProductTool(ctx, name, input)
}

func (a *ProductExecutionAdapter) invokeSpecialProductTool(
	ctx context.Context,
	name string,
	input map[string]any,
) (any, bool, error) {
	switch name {
	case "market.capabilities":
		service, err := a.productService()
		if err != nil {
			return nil, true, err
		}
		result := service.CapabilitiesContext(ctx, productfeatures.CapabilityQuery{
			BrokerID: toolMapString(input, "brokerId"), AccountID: toolMapString(input, "accountId"),
			TradingEnvironment: toolMapString(input, "tradingEnvironment"),
			Market:             strings.ToUpper(toolMapString(input, "market")),
			FeatureID:          broker.FeatureID(toolMapString(input, "featureId")),
		})
		return result, true, nil
	case "market.search":
		result, err := a.productSearch(ctx, input)
		return result, true, err
	case "market.snapshot":
		result, err := a.productSnapshots(ctx, input, broker.FeatureMarketSnapshot)
		return result, true, err
	case "market.snapshots":
		result, err := a.productSnapshots(ctx, input, broker.FeatureMarketSnapshots)
		return result, true, err
	case "execution.buying_power":
		result, err := a.productBuyingPower(ctx, input)
		return result, true, err
	case "research.screen":
		typed, err := a.typedProductService()
		if err != nil {
			return nil, true, err
		}
		result, err := a.productScreen(ctx, input, typed)
		return result, true, err
	case "research.calendar":
		typed, err := a.typedProductService()
		if err != nil {
			return nil, true, err
		}
		result, err := a.productCalendar(ctx, input, typed)
		return result, true, err
	}
	return nil, false, nil
}

func (a *ProductExecutionAdapter) invokeGenericProductTool(
	ctx context.Context,
	name string,
	input map[string]any,
) (any, error) {
	service, err := a.productService()
	if err != nil {
		return nil, err
	}
	if featureID, ok := customizationToolFeatureIDs[name]; ok {
		payload := cloneToolInput(input)
		if nested, ok := input["payload"].(map[string]any); ok {
			payload = nested
		}
		return service.ApplyCustomization(ctx, broker.CustomizationAction{
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
		TradingEnvironment: strings.ToUpper(toolMapString(input, "tradingEnvironment")),
		Market:             strings.ToUpper(toolMapString(input, "market")), InstrumentID: toolInstrumentID(input),
		FeatureID: featureID, Cursor: toolMapString(input, "cursor"),
		PageSize: min(max(toolMapInt(input, "pageSize", 50), 1), 100), Params: cloneToolInput(input),
	}
	delete(query.Params, "brokerId")
	delete(query.Params, "accountId")
	delete(query.Params, "market")
	delete(query.Params, "instrumentId")
	delete(query.Params, "cursor")
	delete(query.Params, "pageSize")
	delete(query.Params, "tradingEnvironment")
	delete(query.Params, "featureId")
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
	return service.Query(ctx, query)
}

// InvokeExecutionTool dispatches an execution tool through the trading service.
func (a *ProductExecutionAdapter) InvokeExecutionTool(
	ctx context.Context,
	name string,
	input map[string]any,
) (any, error) {
	service, err := a.executionService()
	if err != nil {
		return nil, err
	}
	switch name {
	case "execution.order_preview":
		var request trdsrv.ExecutionPlaceRequest
		if err := decodeToolInput(input, &request); err != nil {
			return nil, err
		}
		return service.PreviewExecutionOrderContext(ctx, request)
	case "execution.order_place":
		var request trdsrv.ExecutionPlaceRequest
		if err := decodeToolInput(input, &request); err != nil {
			return nil, err
		}
		return service.CreateExecutionOrder(ctx, request)
	case "execution.order_cancel":
		return service.CancelExecutionOrder(ctx, toolMapString(input, "internalOrderId"))
	case "execution.combo_preview":
		var request trdsrv.ExecutionComboRequest
		if err := decodeToolInput(input, &request); err != nil {
			return nil, err
		}
		return service.PreviewExecutionCombo(ctx, request)
	case "execution.combo_place":
		var request trdsrv.ExecutionComboRequest
		if err := decodeToolInput(input, &request); err != nil {
			return nil, err
		}
		return service.CreateExecutionCombo(ctx, request)
	case "execution.combo_cancel":
		return service.CancelExecutionCombo(ctx, toolMapString(input, "internalOrderId"))
	default:
		return nil, fmt.Errorf("unknown execution tool %q", name)
	}
}

func (a *ProductExecutionAdapter) productSearch(ctx context.Context, input map[string]any) (any, error) {
	service, err := a.productService()
	if err != nil {
		return nil, err
	}
	return service.Query(ctx, broker.FeatureQuery{
		BrokerID:  toolMapString(input, "brokerId"),
		AccountID: toolMapString(input, "accountId"),
		Market:    strings.ToUpper(toolMapString(input, "market")),
		FeatureID: broker.FeatureMarketSearch,
		PageSize:  min(max(toolMapInt(input, "pageSize", 20), 1), 100),
		Params:    map[string]any{"keyword": toolMapString(input, "query")},
	})
}

func (a *ProductExecutionAdapter) productSnapshots(
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
	service, err := a.productService()
	if err != nil {
		return nil, err
	}
	return service.BatchSnapshots(ctx, broker.FeatureQuery{
		BrokerID:  toolMapString(input, "brokerId"),
		AccountID: toolMapString(input, "accountId"),
		Market:    strings.ToUpper(toolMapString(input, "market")),
		FeatureID: featureID,
	}, symbols)
}

func (a *ProductExecutionAdapter) productBuyingPower(ctx context.Context, input map[string]any) (any, error) {
	var query broker.ProductRuleQuery
	if err := decodeToolInput(input, &query); err != nil {
		return nil, err
	}
	query.BrokerID = toolMapString(input, "brokerId")
	query.FeatureID = broker.FeatureExecutionBuyingPower
	service, err := a.executionService()
	if err != nil {
		return nil, err
	}
	return service.PreviewExecutionBuyingPower(ctx, query)
}

func (a *ProductExecutionAdapter) productService() (ProductFeatureService, error) {
	if a == nil || a.productFeatures == nil {
		return nil, fmt.Errorf("product feature service is unavailable")
	}
	return a.productFeatures, nil
}

func (a *ProductExecutionAdapter) typedProductService() (typedProductFeatureService, error) {
	service, err := a.productService()
	if err != nil {
		return nil, err
	}
	typed, ok := service.(typedProductFeatureService)
	if !ok {
		return nil, fmt.Errorf("typed product feature service is unavailable")
	}
	return typed, nil
}

func (a *ProductExecutionAdapter) executionService() (ExecutionService, error) {
	if a == nil || a.execution == nil {
		return nil, fmt.Errorf("trading service is unavailable")
	}
	return a.execution, nil
}

func (a *ProductExecutionAdapter) productScreen(ctx context.Context, input map[string]any, service typedProductFeatureService) (any, error) {
	var query broker.ScreenQueryV2
	if err := decodeToolInput(input, &query); err != nil {
		return nil, fmt.Errorf("invalid research.screen definition: %w", err)
	}
	query.Market = strings.ToUpper(strings.TrimSpace(query.Market))
	if query.Page.Limit == 0 {
		query.Page.Limit = 50
	}
	if query.Page.Offset < 0 || query.Page.Limit < 1 || query.Page.Limit > 100 {
		return nil, fmt.Errorf("page must use offset >= 0 and limit between 1 and 100")
	}
	normalized, err := researchscreen.NormalizeDefinitionV2(query.ScreenDefinitionV2)
	if err != nil {
		return nil, err
	}
	query.ScreenDefinitionV2 = normalized
	result, err := service.QueryScreen(ctx, query)
	if err != nil {
		return nil, err
	}
	result.CatalogVersion = normalized.CatalogVersion
	result.Columns = screenResultColumns(normalized)
	return result, nil
}

func (a *ProductExecutionAdapter) productCalendar(ctx context.Context, input map[string]any, service typedProductFeatureService) (any, error) {
	request := productfeatures.CalendarRequest{
		ReadContext: productfeatures.ReadContext{
			BrokerID: toolMapString(input, "brokerId"), AccountID: toolMapString(input, "accountId"),
			Market: strings.ToUpper(toolMapString(input, "market")), Cursor: toolMapString(input, "cursor"),
			PageSize: toolMapInt(input, "pageSize", 50), TradingEnvironment: toolMapString(input, "tradingEnvironment"),
		},
		Operation: toolMapString(input, "operation"), Date: toolMapString(input, "date"),
		BeginDate: toolMapString(input, "beginDate"), EndDate: toolMapString(input, "endDate"),
		Sort: toolMapString(input, "sort"), StockScope: toolMapString(input, "stockScope"),
		MarketCapMin: toolMapString(input, "marketCapMin"), MarketCapMax: toolMapString(input, "marketCapMax"),
		OptionVolumeMin: toolMapString(input, "optionVolumeMin"), OptionVolumeMax: toolMapString(input, "optionVolumeMax"),
		IVMin: toolMapString(input, "ivMin"), IVMax: toolMapString(input, "ivMax"),
		IVRankMin: toolMapString(input, "ivRankMin"), IVRankMax: toolMapString(input, "ivRankMax"),
		IVPercentileMin: toolMapString(input, "ivPercentileMin"), IVPercentileMax: toolMapString(input, "ivPercentileMax"),
		Refresh: boolInputValue(input, "refresh"),
	}
	result, err := service.QueryCalendar(ctx, request)
	if err != nil {
		return nil, err
	}
	return result.FeatureResult()
}

func screenResultColumns(definition broker.ScreenDefinitionV2) []broker.ScreenResultColumn {
	columns := make([]broker.ScreenResultColumn, 0, len(definition.Columns))
	for _, column := range definition.Columns {
		factor, _ := researchscreen.Lookup(column.Factor.FactorKey)
		columns = append(columns, broker.ScreenResultColumn{ColumnID: column.ID, InstanceID: column.Factor.InstanceID, FactorKey: column.Factor.FactorKey, Label: column.Label, Unit: factor.Unit})
	}
	return columns
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

func decodeToolInputValue(input any, output any) error {
	content, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode tool value: %w", err)
	}
	if err := json.Unmarshal(content, output); err != nil {
		return fmt.Errorf("decode tool value: %w", err)
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
