package servercore

import (
	"fmt"
	"strings"
)

func normalizeStrategyInstrumentID(value string) string {
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

func normalizeStrategyExecutionMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case strategyExecutionModeNotifyOnly:
		return strategyExecutionModeNotifyOnly
	default:
		return strategyExecutionModeLive
	}
}

func normalizeStrategyBrokerAccountBinding(input *strategyBrokerAccountBinding) *strategyBrokerAccountBinding {
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

func readStrategyBindingInstruments(value any) []strategyBindingInstrument {
	switch typed := value.(type) {
	case []strategyBindingInstrument:
		return append([]strategyBindingInstrument(nil), typed...)
	case []any:
		result := make([]strategyBindingInstrument, 0, len(typed))
		for _, entry := range typed {
			record, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			market, _ := record["market"].(string)
			code, _ := record["code"].(string)
			result = append(result, strategyBindingInstrument{Market: market, Code: code})
		}
		return result
	default:
		return nil
	}
}

func strategyBrokerAccountBindingFromAny(value any) *strategyBrokerAccountBinding {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	brokerID, _ := raw["brokerId"].(string)
	accountID, _ := raw["accountId"].(string)
	tradingEnvironment, _ := raw["tradingEnvironment"].(string)
	market, _ := raw["market"].(string)
	return normalizeStrategyBrokerAccountBinding(&strategyBrokerAccountBinding{
		BrokerID:           brokerID,
		AccountID:          accountID,
		TradingEnvironment: tradingEnvironment,
		Market:             market,
	})
}

func strategyBindingInstrumentFromNormalized(value normalizedInstrument) strategyBindingInstrument {
	return strategyBindingInstrument{
		Market: value.Prefix,
		Code:   value.Code,
	}
}

func normalizeStrategyBindingInstrument(input strategyBindingInstrument) (strategyBindingInstrument, string, bool) {
	normalized, err := normalizeInstrumentInput(input.Market, "", input.Code)
	if err != nil {
		return strategyBindingInstrument{}, "", false
	}
	return strategyBindingInstrumentFromNormalized(normalized), normalized.Symbol, true
}

func strategyBindingInstrumentFromSymbol(symbol string) (strategyBindingInstrument, bool) {
	normalized := normalizeStrategyInstrumentID(symbol)
	if normalized == "" {
		return strategyBindingInstrument{}, false
	}
	parsed, err := parseQualifiedInstrumentSymbol(normalized)
	if err != nil {
		return strategyBindingInstrument{}, false
	}
	return strategyBindingInstrumentFromNormalized(parsed), true
}

func normalizeStrategyInstanceBinding(input strategyInstanceBinding, params map[string]any) strategyInstanceBinding {
	if len(input.Instruments) == 0 {
		input.Instruments = readStrategyBindingInstruments(params["instruments"])
	}
	if len(input.Symbols) == 0 {
		input.Symbols = readStringSlice(params["symbols"])
		if len(input.Symbols) == 0 {
			if symbol, ok := params["symbol"].(string); ok && strings.TrimSpace(symbol) != "" {
				input.Symbols = []string{symbol}
			}
		}
	}
	seenSymbols := map[string]struct{}{}
	normalizedSymbols := make([]string, 0, len(input.Symbols))
	normalizedInstruments := make([]strategyBindingInstrument, 0, len(input.Instruments))

	if len(input.Instruments) > 0 {
		for _, instrument := range input.Instruments {
			normalizedInstrument, symbol, ok := normalizeStrategyBindingInstrument(instrument)
			if !ok {
				continue
			}
			if _, exists := seenSymbols[symbol]; exists {
				continue
			}
			seenSymbols[symbol] = struct{}{}
			normalizedSymbols = append(normalizedSymbols, symbol)
			normalizedInstruments = append(normalizedInstruments, normalizedInstrument)
		}
	}

	if len(normalizedSymbols) == 0 {
		for _, symbol := range input.Symbols {
			normalized := normalizeStrategyInstrumentID(symbol)
			if normalized == "" {
				continue
			}
			if _, exists := seenSymbols[normalized]; exists {
				continue
			}
			seenSymbols[normalized] = struct{}{}
			normalizedSymbols = append(normalizedSymbols, normalized)
			if instrument, ok := strategyBindingInstrumentFromSymbol(normalized); ok {
				normalizedInstruments = append(normalizedInstruments, instrument)
			}
		}
	}

	input.Symbols = normalizedSymbols
	input.Instruments = normalizedInstruments

	input.Interval = strings.TrimSpace(input.Interval)
	if input.Interval == "" {
		if interval, ok := params["interval"].(string); ok {
			input.Interval = strings.TrimSpace(interval)
		}
	}
	if input.Interval == "" {
		input.Interval = "5m"
	}

	if input.BrokerAccount == nil {
		input.BrokerAccount = strategyBrokerAccountBindingFromAny(params["brokerAccount"])
	}
	input.BrokerAccount = normalizeStrategyBrokerAccountBinding(input.BrokerAccount)

	if strings.TrimSpace(input.ExecutionMode) == "" {
		if executionMode, ok := params["executionMode"].(string); ok {
			input.ExecutionMode = executionMode
		}
	}
	input.ExecutionMode = normalizeStrategyExecutionMode(input.ExecutionMode)

	return input
}

func applyStrategyBindingParams(input *managedStrategyInstance) {
	if input == nil {
		return
	}
	if input.Params == nil {
		input.Params = map[string]any{}
	}
	input.Binding = normalizeStrategyInstanceBinding(input.Binding, input.Params)
	input.Params["instruments"] = strategyBindingInstrumentsToParams(input.Binding.Instruments)
	input.Params["symbols"] = append([]string(nil), input.Binding.Symbols...)
	if len(input.Binding.Symbols) > 0 {
		input.Params["symbol"] = input.Binding.Symbols[0]
	} else {
		delete(input.Params, "symbol")
	}
	input.Params["interval"] = input.Binding.Interval
	input.Params["executionMode"] = input.Binding.ExecutionMode
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

func strategyBindingInstrumentsToParams(input []strategyBindingInstrument) []map[string]any {
	result := make([]map[string]any, 0, len(input))
	for _, instrument := range input {
		result = append(result, map[string]any{
			"market": instrument.Market,
			"code":   instrument.Code,
		})
	}
	return result
}

func strategyBindingAuditDetail(definitionID string, binding strategyInstanceBinding) string {
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
