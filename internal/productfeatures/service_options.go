package productfeatures

import "github.com/jftrade/jftrade-main/pkg/broker"

type Option func(*Service)

func WithPredictionQuoteStore(store broker.PredictionQuoteStore) Option {
	return func(service *Service) { service.predictionQuotes = store }
}

type PredictionComboQuoteRequest struct {
	BrokerID           string                  `json:"brokerId"`
	AccountID          string                  `json:"accountId"`
	TradingEnvironment string                  `json:"tradingEnvironment"`
	MVC                string                  `json:"mvc"`
	Legs               []broker.OrderLegIntent `json:"legs"`
}
