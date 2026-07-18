package broker

import "strings"

// ImplementsAdapterInterface verifies that a broker capability declaration has
// an executable implementation behind it. Required read/write services are
// obtained from the Broker accessors; optional product interfaces are composed
// directly on the broker adapter.
func ImplementsAdapterInterface(selected Broker, interfaceName string) bool {
	if selected == nil {
		return false
	}
	switch strings.TrimSpace(interfaceName) {
	case "MarketDataReader":
		return selected.MarketData() != nil
	case "TradingService":
		return selected.Trading() != nil
	case "BatchSnapshotSource":
		_, ok := selected.(BatchSnapshotSource)
		return ok
	case "MarketMicrostructureReader":
		_, ok := selected.(MarketMicrostructureReader)
		return ok
	case "InstrumentProfileReader":
		_, ok := selected.(InstrumentProfileReader)
		return ok
	case "DerivativeCatalogReader":
		_, ok := selected.(DerivativeCatalogReader)
		return ok
	case "OptionAnalyticsReader":
		_, ok := selected.(OptionAnalyticsReader)
		return ok
	case "InstrumentResearchReader":
		_, ok := selected.(InstrumentResearchReader)
		return ok
	case "MarketResearchReader":
		_, ok := selected.(MarketResearchReader)
		return ok
	case "PredictionMarketReader":
		_, ok := selected.(PredictionMarketReader)
		return ok
	case "TechnicalIndicatorReader":
		_, ok := selected.(TechnicalIndicatorReader)
		return ok
	case "CustomizationService":
		_, ok := selected.(CustomizationService)
		return ok
	case "ProductRuleProvider":
		_, ok := selected.(ProductRuleProvider)
		return ok
	case "ComboTradingService":
		_, ok := selected.(ComboTradingService)
		return ok
	case "EventContractTradingService":
		_, ok := selected.(EventContractTradingService)
		return ok
	default:
		return false
	}
}
