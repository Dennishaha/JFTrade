package futu

import (
	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/market"
)

var candleSessionMapping = map[marketdata.CandleSession][]market.Session{
	marketdata.CandleSessionRegular:   {market.SessionRegular},
	marketdata.CandleSessionExtended:  {market.SessionPre, market.SessionAfter},
	marketdata.CandleSessionOvernight: {market.SessionOvernight},
}

func MarketSessionsForCandleSessions(sessions []marketdata.CandleSession) []market.Session {
	result := make([]market.Session, 0, len(sessions)+1)
	for _, session := range sessions {
		result = append(result, candleSessionMapping[session]...)
	}
	return result
}
