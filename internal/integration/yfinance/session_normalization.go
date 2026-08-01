package yfinance

import (
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func yahooSessionAt(instrumentID string, observedAt time.Time) string {
	session := market.ClassifySession(instrumentID, observedAt)
	if session == market.SessionOvernight {
		// Yahoo does not provide a dependable overnight quote stream.
		return string(market.SessionClosed)
	}
	return string(session)
}

func normalizeYahooExtendedQuote(
	instrumentID string,
	quote *marketdata.ExtendedQuote,
	expected market.Session,
) *marketdata.ExtendedQuote {
	if quote == nil || quote.Price == nil || strings.TrimSpace(quote.QuoteTime) == "" {
		return nil
	}
	quoteAt, err := time.Parse(time.RFC3339Nano, quote.QuoteTime)
	if err != nil {
		return nil
	}
	window, ok := market.ResolveSessionWindow(instrumentID, quoteAt)
	if !ok || window.Session != expected {
		return nil
	}
	quote.TradingDate = window.TradingDate
	quote.ExchangeTimezone = window.Timezone
	quote.SessionStartAt = window.StartAt.Format(time.RFC3339Nano)
	quote.SessionEndAt = window.EndAt.Format(time.RFC3339Nano)
	return quote
}

func retainRelevantYahooExtendedQuotes(
	instrumentID string,
	values snapshotValues,
	observedAt time.Time,
) snapshotValues {
	if values.session == string(market.SessionUnknown) {
		values.preMarket = nil
		values.afterMarket = nil
		return values
	}
	currentDate := quoteTradingDate(instrumentID, observedAt)
	if values.preMarket != nil && values.preMarket.TradingDate != currentDate {
		values.preMarket = nil
	}
	regularDate := extendedQuoteTradingDate(instrumentID, values.regularQuote)
	if values.afterMarket != nil {
		if regularDate == "" || values.afterMarket.TradingDate != regularDate {
			values.afterMarket = nil
		} else if values.session == string(market.SessionAfter) && values.afterMarket.TradingDate != currentDate {
			values.afterMarket = nil
		}
	}
	return values
}

func extendedQuoteTradingDate(instrumentID string, quote *marketdata.ExtendedQuote) string {
	if quote == nil || strings.TrimSpace(quote.QuoteTime) == "" {
		return ""
	}
	quoteAt, err := time.Parse(time.RFC3339Nano, quote.QuoteTime)
	if err != nil {
		return ""
	}
	return quoteTradingDate(instrumentID, quoteAt)
}

func quoteTradingDate(instrumentID string, at time.Time) string {
	profile, ok := market.ProfileForSymbol(instrumentID)
	if !ok || profile.Location == nil || at.IsZero() {
		return ""
	}
	return at.In(profile.Location).Format("2006-01-02")
}

func selectYahooActiveQuote(values snapshotValues) snapshotValues {
	var active *marketdata.ExtendedQuote
	switch values.session {
	case string(market.SessionPre):
		active = values.preMarket
	case string(market.SessionAfter):
		active = values.afterMarket
	}
	if active == nil || active.Price == nil {
		return values
	}
	baselinePrice := values.price
	values.price = *active.Price
	if values.bid.Equal(baselinePrice) {
		values.bid = values.price
	}
	if values.ask.Equal(baselinePrice) {
		values.ask = values.price
	}
	values.quoteAt = active.QuoteTime
	if active.Volume != nil {
		values.volume = *active.Volume
	}
	if active.Turnover != nil {
		values.turnover = *active.Turnover
	}
	return values
}
