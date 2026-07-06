package instancebinding

import (
	"fmt"
	"strings"

	strategy "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/runtimecontrol"
	"github.com/jftrade/jftrade-main/pkg/market"
)

const (
	ExecutionModeLive       = "live"
	ExecutionModeNotifyOnly = "notify_only"
)

func NormalizeInstrumentID(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return ""
	}
	if strings.Contains(normalized, ":") {
		parts := strings.SplitN(normalized, ":", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
			return strings.TrimSpace(parts[0]) + "." + strings.TrimSpace(parts[1])
		}
	}
	return normalized
}

func NormalizeExecutionMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ExecutionModeNotifyOnly:
		return ExecutionModeNotifyOnly
	default:
		return ExecutionModeLive
	}
}

func NormalizeBrokerAccount(input *strategy.BrokerAccountBinding) *strategy.BrokerAccountBinding {
	if input == nil {
		return nil
	}
	copyValue := *input
	copyValue.BrokerID = strings.ToLower(strings.TrimSpace(copyValue.BrokerID))
	copyValue.AccountID = strings.TrimSpace(copyValue.AccountID)
	copyValue.TradingEnvironment = strings.ToUpper(strings.TrimSpace(copyValue.TradingEnvironment))
	copyValue.Market = strings.ToUpper(strings.TrimSpace(copyValue.Market))
	if copyValue.BrokerID == "" && copyValue.AccountID == "" && copyValue.TradingEnvironment == "" && copyValue.Market == "" {
		return nil
	}
	return &copyValue
}

func NormalizeRiskSettings(input strategy.RuntimeRiskSettings) strategy.RuntimeRiskSettings {
	normalized := runtimecontrol.NormalizeRiskSettings(runtimecontrol.RiskSettings{
		Mode:             input.Mode,
		CloseOnly:        input.CloseOnly,
		MaxOrderQuantity: input.MaxOrderQuantity,
		MaxOrderNotional: input.MaxOrderNotional,
		DailyMaxOrders:   input.DailyMaxOrders,
		PauseOnReject:    input.PauseOnReject,
	})
	return strategy.RuntimeRiskSettings{
		Mode:             normalized.Mode,
		CloseOnly:        normalized.CloseOnly,
		MaxOrderQuantity: normalized.MaxOrderQuantity,
		MaxOrderNotional: normalized.MaxOrderNotional,
		DailyMaxOrders:   normalized.DailyMaxOrders,
		PauseOnReject:    normalized.PauseOnReject,
	}
}

func NormalizeBinding(input strategy.InstanceBinding, params map[string]any) strategy.InstanceBinding {
	if params == nil {
		params = map[string]any{}
	}
	hydrateBindingTargets(&input, params)
	input.Symbols, input.Instruments = normalizeBindingTargets(input)
	applyBindingDefaults(&input, params)

	return input
}

func hydrateBindingTargets(input *strategy.InstanceBinding, params map[string]any) {
	if len(input.Instruments) == 0 {
		input.Instruments = readBindingInstruments(params["instruments"])
	}
	if len(input.Symbols) > 0 {
		return
	}
	input.Symbols = readStringSlice(params["symbols"])
	if len(input.Symbols) == 0 {
		if symbol, ok := params["symbol"].(string); ok && strings.TrimSpace(symbol) != "" {
			input.Symbols = []string{symbol}
		}
	}
}

func normalizeBindingTargets(input strategy.InstanceBinding) ([]string, []strategy.BindingInstrument) {
	seenSymbols := map[string]struct{}{}
	normalizedSymbols, normalizedInstruments := normalizeBindingInstruments(input.Instruments, seenSymbols)
	if len(normalizedSymbols) > 0 {
		return normalizedSymbols, normalizedInstruments
	}
	return normalizeBindingSymbols(input.Symbols, seenSymbols)
}

func normalizeBindingInstruments(
	instruments []strategy.BindingInstrument,
	seenSymbols map[string]struct{},
) ([]string, []strategy.BindingInstrument) {
	normalizedSymbols := make([]string, 0, len(instruments))
	normalizedInstruments := make([]strategy.BindingInstrument, 0, len(instruments))
	for _, instrument := range instruments {
		normalizedInstrument, symbol, ok := NormalizeBindingInstrument(instrument)
		if !ok || seenBindingSymbol(symbol, seenSymbols) {
			continue
		}
		normalizedSymbols = append(normalizedSymbols, symbol)
		normalizedInstruments = append(normalizedInstruments, normalizedInstrument)
	}
	return normalizedSymbols, normalizedInstruments
}

func normalizeBindingSymbols(symbols []string, seenSymbols map[string]struct{}) ([]string, []strategy.BindingInstrument) {
	normalizedSymbols := make([]string, 0, len(symbols))
	normalizedInstruments := make([]strategy.BindingInstrument, 0, len(symbols))
	for _, symbol := range symbols {
		normalized := NormalizeInstrumentID(symbol)
		if normalized == "" || seenBindingSymbol(normalized, seenSymbols) {
			continue
		}
		normalizedSymbols = append(normalizedSymbols, normalized)
		if instrument, ok := BindingInstrumentFromSymbol(normalized); ok {
			normalizedInstruments = append(normalizedInstruments, instrument)
		}
	}
	return normalizedSymbols, normalizedInstruments
}

func seenBindingSymbol(symbol string, seenSymbols map[string]struct{}) bool {
	if _, exists := seenSymbols[symbol]; exists {
		return true
	}
	seenSymbols[symbol] = struct{}{}
	return false
}

func applyBindingDefaults(input *strategy.InstanceBinding, params map[string]any) {
	input.Interval = bindingInterval(input.Interval, params["interval"])
	input.BrokerAccount = bindingBrokerAccount(input.BrokerAccount, params["brokerAccount"])
	input.ExecutionMode = bindingExecutionMode(input.ExecutionMode, params["executionMode"])
	input.RuntimeRisk = bindingRuntimeRisk(input.RuntimeRisk, params["runtimeRisk"])
}

func bindingInterval(interval string, value any) string {
	normalized := strings.TrimSpace(interval)
	if normalized == "" {
		if paramInterval, ok := value.(string); ok {
			normalized = strings.TrimSpace(paramInterval)
		}
	}
	if normalized == "" {
		return "5m"
	}
	return normalized
}

func bindingBrokerAccount(current *strategy.BrokerAccountBinding, value any) *strategy.BrokerAccountBinding {
	if current == nil {
		current = BrokerAccountFromAny(value)
	}
	return NormalizeBrokerAccount(current)
}

func bindingExecutionMode(current string, value any) string {
	if strings.TrimSpace(current) == "" {
		if executionMode, ok := value.(string); ok {
			current = executionMode
		}
	}
	return NormalizeExecutionMode(current)
}

func bindingRuntimeRisk(current strategy.RuntimeRiskSettings, value any) strategy.RuntimeRiskSettings {
	if strings.TrimSpace(current.Mode) == "" {
		current = RiskSettingsFromAny(value)
	}
	return NormalizeRiskSettings(current)
}

func ApplyParams(input *strategy.ManagedInstance) {
	if input == nil {
		return
	}
	if input.Params == nil {
		input.Params = map[string]any{}
	}
	input.Binding = NormalizeBinding(input.Binding, input.Params)
	input.Params["instruments"] = BindingInstrumentsToParams(input.Binding.Instruments)
	input.Params["symbols"] = append([]string(nil), input.Binding.Symbols...)
	if len(input.Binding.Symbols) > 0 {
		input.Params["symbol"] = input.Binding.Symbols[0]
	} else {
		delete(input.Params, "symbol")
	}
	input.Params["interval"] = input.Binding.Interval
	input.Params["executionMode"] = input.Binding.ExecutionMode
	input.Params["runtimeRisk"] = RiskSettingsToParams(input.Binding.RuntimeRisk)
	if input.Binding.BrokerAccount != nil {
		input.Params["brokerAccount"] = map[string]any{
			"brokerId":           input.Binding.BrokerAccount.BrokerID,
			"accountId":          input.Binding.BrokerAccount.AccountID,
			"tradingEnvironment": input.Binding.BrokerAccount.TradingEnvironment,
			"market":             input.Binding.BrokerAccount.Market,
		}
	} else {
		delete(input.Params, "brokerAccount")
	}
}

func NormalizeBindingInstrument(input strategy.BindingInstrument) (strategy.BindingInstrument, string, bool) {
	instrument, err := market.ParseInstrument(market.InstrumentInput{
		Market: input.Market,
		Code:   input.Code,
	})
	if err != nil {
		return strategy.BindingInstrument{}, "", false
	}
	return BindingInstrumentFromMarket(instrument), instrument.Symbol, true
}

func BindingInstrumentFromSymbol(symbol string) (strategy.BindingInstrument, bool) {
	normalized := NormalizeInstrumentID(symbol)
	if normalized == "" {
		return strategy.BindingInstrument{}, false
	}
	instrument, err := market.ParseQualifiedInstrumentSymbol(normalized)
	if err != nil {
		return strategy.BindingInstrument{}, false
	}
	return BindingInstrumentFromMarket(instrument), true
}

func BindingInstrumentFromMarket(instrument market.Instrument) strategy.BindingInstrument {
	return strategy.BindingInstrument{
		Market: instrument.Prefix,
		Code:   instrument.Code,
	}
}

func BrokerAccountFromAny(value any) *strategy.BrokerAccountBinding {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return NormalizeBrokerAccount(&strategy.BrokerAccountBinding{
		BrokerID:           optionalString(raw["brokerId"]),
		AccountID:          optionalString(raw["accountId"]),
		TradingEnvironment: optionalString(raw["tradingEnvironment"]),
		Market:             optionalString(raw["market"]),
	})
}

func RiskSettingsFromAny(value any) strategy.RuntimeRiskSettings {
	raw, ok := value.(map[string]any)
	if !ok {
		return strategy.RuntimeRiskSettings{}
	}
	return NormalizeRiskSettings(strategy.RuntimeRiskSettings{
		Mode:             optionalString(raw["mode"]),
		CloseOnly:        optionalBool(raw["closeOnly"]),
		MaxOrderQuantity: numberPointerFromAny(raw["maxOrderQuantity"]),
		MaxOrderNotional: numberPointerFromAny(raw["maxOrderNotional"]),
		DailyMaxOrders:   intPointerFromAny(raw["dailyMaxOrders"]),
		PauseOnReject:    optionalBool(raw["pauseOnReject"]),
	})
}

func RiskSettingsToParams(input strategy.RuntimeRiskSettings) map[string]any {
	normalized := NormalizeRiskSettings(input)
	return map[string]any{
		"mode":             normalized.Mode,
		"closeOnly":        normalized.CloseOnly,
		"maxOrderQuantity": normalized.MaxOrderQuantity,
		"maxOrderNotional": normalized.MaxOrderNotional,
		"dailyMaxOrders":   normalized.DailyMaxOrders,
		"pauseOnReject":    normalized.PauseOnReject,
	}
}

func RiskAuditDetail(input strategy.RuntimeRiskSettings) string {
	normalized := NormalizeRiskSettings(input)
	parts := []string{"mode=" + normalized.Mode}
	if normalized.CloseOnly {
		parts = append(parts, "closeOnly=true")
	}
	if normalized.MaxOrderQuantity != nil {
		parts = append(parts, fmt.Sprintf("maxOrderQuantity=%v", *normalized.MaxOrderQuantity))
	}
	if normalized.MaxOrderNotional != nil {
		parts = append(parts, fmt.Sprintf("maxOrderNotional=%v", *normalized.MaxOrderNotional))
	}
	if normalized.DailyMaxOrders != nil {
		parts = append(parts, fmt.Sprintf("dailyMaxOrders=%d", *normalized.DailyMaxOrders))
	}
	if normalized.PauseOnReject {
		parts = append(parts, "pauseOnReject=true")
	}
	return strings.Join(parts, " | ")
}

func BindingInstrumentsToParams(input []strategy.BindingInstrument) []map[string]any {
	result := make([]map[string]any, 0, len(input))
	for _, instrument := range input {
		result = append(result, map[string]any{
			"market": instrument.Market,
			"code":   instrument.Code,
		})
	}
	return result
}

func BindingAuditDetail(definitionID string, binding strategy.InstanceBinding) string {
	parts := []string{strings.TrimSpace(definitionID)}
	if len(binding.Symbols) > 0 {
		parts = append(parts, "symbols="+strings.Join(binding.Symbols, ","))
	}
	parts = append(parts, "interval="+binding.Interval)
	parts = append(parts, "mode="+binding.ExecutionMode)
	if binding.BrokerAccount != nil {
		parts = append(parts, fmt.Sprintf("account=%s/%s/%s/%s", binding.BrokerAccount.BrokerID, binding.BrokerAccount.TradingEnvironment, binding.BrokerAccount.AccountID, binding.BrokerAccount.Market))
	}
	return strings.Join(parts, " | ")
}

func readStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, entry := range typed {
			if text, ok := entry.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func readBindingInstruments(value any) []strategy.BindingInstrument {
	switch typed := value.(type) {
	case []strategy.BindingInstrument:
		return append([]strategy.BindingInstrument(nil), typed...)
	case []any:
		result := make([]strategy.BindingInstrument, 0, len(typed))
		for _, entry := range typed {
			record, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			result = append(result, strategy.BindingInstrument{
				Market: optionalString(record["market"]),
				Code:   optionalString(record["code"]),
			})
		}
		return result
	default:
		return nil
	}
}

func numberPointerFromAny(value any) *float64 {
	switch typed := value.(type) {
	case float64:
		return &typed
	case float32:
		value := float64(typed)
		return &value
	case int:
		value := float64(typed)
		return &value
	case int64:
		value := float64(typed)
		return &value
	}
	return nil
}

func intPointerFromAny(value any) *int {
	switch typed := value.(type) {
	case int:
		return &typed
	case int64:
		value := int(typed)
		return &value
	case float64:
		value := int(typed)
		if float64(value) == typed {
			return &value
		}
	}
	return nil
}

func optionalString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func optionalBool(value any) bool {
	if flag, ok := value.(bool); ok {
		return flag
	}
	return false
}
