package broker

import (
	"strings"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

// ApplyMarketRule returns a copy of market enriched with broker-provided
// quantity constraints.
func ApplyMarketRule(market types.Market, rule MarketRuleItem) types.Market {
	if rule.LotSize != nil && *rule.LotSize > 0 {
		lotSize := fixedpoint.NewFromFloat(float64(*rule.LotSize))
		market.MinQuantity = lotSize
		market.StepSize = lotSize
	}
	if rule.MinQuantity != nil && *rule.MinQuantity > 0 {
		market.MinQuantity = fixedpoint.NewFromFloat(*rule.MinQuantity)
	}
	if rule.StepSize != nil && *rule.StepSize > 0 {
		market.StepSize = fixedpoint.NewFromFloat(*rule.StepSize)
	}
	return market
}

// ApplyMarketRules applies the first matching rule to market. Symbols are
// matched case-insensitively so broker adapters can preserve their native case.
func ApplyMarketRules(market types.Market, rules []MarketRuleItem) types.Market {
	symbol := strings.ToUpper(strings.TrimSpace(market.Symbol))
	for _, rule := range rules {
		if strings.ToUpper(strings.TrimSpace(rule.Symbol)) != symbol {
			continue
		}
		return ApplyMarketRule(market, rule)
	}
	return market
}
