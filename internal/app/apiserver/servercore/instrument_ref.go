package servercore

import "github.com/jftrade/jftrade-main/pkg/market"

type normalizedInstrument struct {
	Market string
	Prefix string
	Code   string
	Symbol string
}

func normalizeInstrumentInput(marketInput string, symbol string, code string) (normalizedInstrument, error) {
	instrument, err := market.ParseInstrument(market.InstrumentInput{
		Market: marketInput,
		Symbol: symbol,
		Code:   code,
	})
	if err != nil {
		return normalizedInstrument{}, err
	}
	return normalizedInstrumentFromMarket(instrument), nil
}

func parseQualifiedInstrumentSymbol(symbol string) (normalizedInstrument, error) {
	instrument, err := market.ParseQualifiedInstrumentSymbol(symbol)
	if err != nil {
		return normalizedInstrument{}, err
	}
	return normalizedInstrumentFromMarket(instrument), nil
}

func normalizeInstrumentMarketInput(marketInput string) (resolvedMarket string, preferredPrefix string, err error) {
	return market.NormalizeMarketInput(marketInput)
}

func instrumentMarketInputMatchesParsedSymbol(marketInput string, parsed normalizedInstrument) bool {
	instrument := market.Instrument{
		Market: parsed.Market,
		Prefix: parsed.Prefix,
		Code:   parsed.Code,
		Symbol: parsed.Symbol,
	}
	return market.MarketInputMatchesParsedSymbol(marketInput, instrument)
}

func normalizedInstrumentFromMarket(instrument market.Instrument) normalizedInstrument {
	return normalizedInstrument{
		Market: instrument.Market,
		Prefix: instrument.Prefix,
		Code:   instrument.Code,
		Symbol: instrument.Symbol,
	}
}
