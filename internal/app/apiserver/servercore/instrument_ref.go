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

func normalizedInstrumentFromMarket(instrument market.Instrument) normalizedInstrument {
	return normalizedInstrument{
		Market: instrument.Market,
		Prefix: instrument.Prefix,
		Code:   instrument.Code,
		Symbol: instrument.Symbol,
	}
}
