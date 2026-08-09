package marketdataapp

import (
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

type MarketTradingWindowDTO struct {
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
	Label       string `json:"label"`
}

type MarketPrecisionDTO struct {
	Price int `json:"price"`
	Quote int `json:"quote"`
}

type MarketProfileDTO struct {
	Code                   string                   `json:"code"`
	ResolvedMarket         string                   `json:"resolvedMarket"`
	PreferredPrefix        string                   `json:"preferredPrefix"`
	DisplayName            string                   `json:"displayName"`
	QuoteCurrency          string                   `json:"quoteCurrency"`
	Timezone               string                   `json:"timezone"`
	SupportsExtendedHours  bool                     `json:"supportsExtendedHours"`
	RequiresExchangePrefix bool                     `json:"requiresExchangePrefix"`
	Aliases                []string                 `json:"aliases"`
	RegularSessions        []MarketTradingWindowDTO `json:"regularSessions"`
	Precision              MarketPrecisionDTO       `json:"precision"`
	TickSize               float64                  `json:"tickSize"`
}

type NormalizeMarketInstrumentResponse struct {
	Market         string `json:"market"`
	Prefix         string `json:"prefix"`
	Code           string `json:"code"`
	Symbol         string `json:"symbol"`
	InstrumentID   string `json:"instrumentId"`
	ResolvedMarket string `json:"resolvedMarket"`
}

func MarketProfileDTOs() []MarketProfileDTO {
	return marketProfileDTOsFromDescriptors(market.MarketDescriptors())
}

func UserMarketProfileDTOs() []MarketProfileDTO {
	return marketProfileDTOsFromDescriptors(market.UserMarketDescriptors())
}

func marketProfileDTOsFromDescriptors(descriptors []market.MarketDescriptor) []MarketProfileDTO {
	result := make([]MarketProfileDTO, 0, len(descriptors))
	for _, descriptor := range descriptors {
		sessions := make([]MarketTradingWindowDTO, 0, len(descriptor.RegularSessions))
		for _, session := range descriptor.RegularSessions {
			sessions = append(sessions, MarketTradingWindowDTO{
				StartMinute: session.StartMinute, EndMinute: session.EndMinute,
				Label: tradingWindowLabel(session),
			})
		}
		result = append(result, MarketProfileDTO{
			Code: descriptor.Code, ResolvedMarket: descriptor.ResolvedMarket,
			PreferredPrefix: descriptor.PreferredPrefix, DisplayName: descriptor.DisplayName,
			QuoteCurrency: descriptor.QuoteCurrency, Timezone: descriptor.Timezone,
			SupportsExtendedHours:  descriptor.SupportsExtendedHours,
			RequiresExchangePrefix: descriptor.RequiresExchangePrefix,
			Aliases:                append([]string(nil), descriptor.Aliases...), RegularSessions: sessions,
			Precision: MarketPrecisionDTO{Price: descriptor.PricePrecision, Quote: descriptor.QuotePrecision},
			TickSize:  descriptor.TickSize,
		})
	}
	return result
}

func tradingWindowLabel(window market.TradingWindow) string {
	return minuteLabel(window.StartMinute) + "-" + minuteLabel(window.EndMinute)
}

func minuteLabel(minute int) string {
	hour := minute / 60
	min := minute % 60
	return time.Date(2000, time.January, 1, hour, min, 0, 0, time.UTC).Format("15:04")
}
