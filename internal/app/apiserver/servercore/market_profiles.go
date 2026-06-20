package servercore

import (
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

type marketTradingWindowDTO struct {
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
	Label       string `json:"label"`
}

type marketPrecisionDTO struct {
	Price int `json:"price"`
	Quote int `json:"quote"`
}

type marketProfileDTO struct {
	Code                   string                   `json:"code"`
	ResolvedMarket         string                   `json:"resolvedMarket"`
	PreferredPrefix        string                   `json:"preferredPrefix"`
	DisplayName            string                   `json:"displayName"`
	QuoteCurrency          string                   `json:"quoteCurrency"`
	Timezone               string                   `json:"timezone"`
	SupportsExtendedHours  bool                     `json:"supportsExtendedHours"`
	RequiresExchangePrefix bool                     `json:"requiresExchangePrefix"`
	Aliases                []string                 `json:"aliases"`
	RegularSessions        []marketTradingWindowDTO `json:"regularSessions"`
	Precision              marketPrecisionDTO       `json:"precision"`
	TickSize               float64                  `json:"tickSize"`
}

type normalizeMarketInstrumentResponse struct {
	Market         string `json:"market"`
	Prefix         string `json:"prefix"`
	Code           string `json:"code"`
	Symbol         string `json:"symbol"`
	InstrumentID   string `json:"instrumentId"`
	ResolvedMarket string `json:"resolvedMarket"`
}

func marketProfileDTOs() []marketProfileDTO {
	descriptors := market.MarketDescriptors()
	result := make([]marketProfileDTO, 0, len(descriptors))
	for _, descriptor := range descriptors {
		sessions := make([]marketTradingWindowDTO, 0, len(descriptor.RegularSessions))
		for _, session := range descriptor.RegularSessions {
			sessions = append(sessions, marketTradingWindowDTO{
				StartMinute: session.StartMinute,
				EndMinute:   session.EndMinute,
				Label:       tradingWindowLabel(session),
			})
		}
		result = append(result, marketProfileDTO{
			Code:                   descriptor.Code,
			ResolvedMarket:         descriptor.ResolvedMarket,
			PreferredPrefix:        descriptor.PreferredPrefix,
			DisplayName:            descriptor.DisplayName,
			QuoteCurrency:          descriptor.QuoteCurrency,
			Timezone:               descriptor.Timezone,
			SupportsExtendedHours:  descriptor.SupportsExtendedHours,
			RequiresExchangePrefix: descriptor.RequiresExchangePrefix,
			Aliases:                append([]string(nil), descriptor.Aliases...),
			RegularSessions:        sessions,
			Precision: marketPrecisionDTO{
				Price: descriptor.PricePrecision,
				Quote: descriptor.QuotePrecision,
			},
			TickSize: descriptor.TickSize,
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
