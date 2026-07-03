package exchange

import "github.com/jftrade/jftrade-main/pkg/bbgo/types"

func GetSessionAttributes(exchange types.ExchangeMinimal) (isMargin, isFutures, isIsolated bool, isolatedSymbol string) {
	if marginExchange, ok := exchange.(types.MarginExchange); ok {
		marginSettings := marginExchange.GetMarginSettings()
		isMargin = marginSettings.IsMargin
		if isMargin {
			isIsolated = marginSettings.IsIsolatedMargin
			if marginSettings.IsIsolatedMargin {
				isolatedSymbol = marginSettings.IsolatedMarginSymbol
			}
		}
	}

	if futuresExchange, ok := exchange.(types.FuturesExchange); ok {
		futuresSettings := futuresExchange.GetFuturesSettings()
		isFutures = futuresSettings.IsFutures
		if isFutures {
			isIsolated = futuresSettings.IsIsolatedFutures
			if futuresSettings.IsIsolatedFutures {
				isolatedSymbol = futuresSettings.IsolatedFuturesSymbol
			}
		}
	}

	return isMargin, isFutures, isIsolated, isolatedSymbol
}

func IsMaxExchange(exchange any) bool {
	return false
}
