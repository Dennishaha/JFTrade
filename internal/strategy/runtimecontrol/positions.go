package runtimecontrol

import "strings"

type Position struct {
	Market           string
	Symbol           string
	Quantity         float64
	SellableQuantity float64
}

func SellableQuantity(positions []Position, symbol string) float64 {
	total := 0.0
	for _, position := range positions {
		if PositionMatchesSymbol(position, symbol) {
			total += position.SellableQuantity
		}
	}
	return total
}

func PositionMatchesSymbol(position Position, symbol string) bool {
	strategySymbol := strings.TrimSpace(strings.ToUpper(symbol))
	if strategySymbol == "" {
		return false
	}
	positionSymbol := strings.TrimSpace(strings.ToUpper(position.Symbol))
	if positionSymbol == "" {
		return false
	}
	if strings.EqualFold(positionSymbol, strategySymbol) {
		return true
	}
	market := strings.TrimSpace(strings.ToUpper(position.Market))
	if market != "" && !strings.Contains(positionSymbol, ".") && !strings.Contains(positionSymbol, ":") {
		if strings.EqualFold(market+"."+positionSymbol, strategySymbol) {
			return true
		}
		if strings.EqualFold(market+":"+positionSymbol, strategySymbol) {
			return true
		}
	}
	if market == "" {
		if parts := strings.SplitN(strategySymbol, ".", 2); len(parts) == 2 && strings.EqualFold(parts[1], positionSymbol) {
			return true
		}
		if parts := strings.SplitN(strategySymbol, ":", 2); len(parts) == 2 && strings.EqualFold(parts[1], positionSymbol) {
			return true
		}
	}
	return false
}
