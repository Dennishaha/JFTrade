package productfeatures

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (s *Service) SetPredictionQuoteStore(store broker.PredictionQuoteStore) {
	s.predictionQuotes = store
}

func (s *Service) QuotePredictionCombo(
	ctx context.Context,
	request PredictionComboQuoteRequest,
) (*broker.FeatureResult, error) {
	request.BrokerID = strings.TrimSpace(request.BrokerID)
	request.AccountID = strings.TrimSpace(request.AccountID)
	request.TradingEnvironment = strings.ToUpper(strings.TrimSpace(request.TradingEnvironment))
	request.MVC = strings.TrimSpace(request.MVC)
	if request.BrokerID == "" || request.AccountID == "" ||
		request.TradingEnvironment == "" || request.MVC == "" {
		return nil, fmt.Errorf("%w: brokerId, accountId, tradingEnvironment, and mvc are required", ErrInvalidQuery)
	}
	if len(request.Legs) < 2 {
		return nil, fmt.Errorf("%w: prediction combo quote requires at least two legs", ErrInvalidQuery)
	}
	for index := range request.Legs {
		leg := &request.Legs[index]
		leg.InstrumentID = strings.ToUpper(strings.TrimSpace(leg.InstrumentID))
		leg.Side = strings.ToUpper(strings.TrimSpace(leg.Side))
		leg.PredictionSide = strings.ToUpper(strings.TrimSpace(leg.PredictionSide))
		if leg.ProductClass == "" {
			leg.ProductClass = broker.ProductClassEventContract
		}
		if leg.ProductClass != broker.ProductClassEventContract ||
			!strings.HasPrefix(leg.InstrumentID, "US.") ||
			(leg.Side != "BUY" && leg.Side != "SELL") ||
			(leg.PredictionSide != "YES" && leg.PredictionSide != "NO") ||
			leg.Ratio <= 0 {
			return nil, fmt.Errorf("%w: prediction combo leg %d is invalid", ErrInvalidQuery, index)
		}
	}
	result, err := s.Query(ctx, broker.FeatureQuery{
		BrokerID: request.BrokerID, AccountID: request.AccountID,
		TradingEnvironment: request.TradingEnvironment,
		Market:             "US", MarketSegment: broker.MarketSegmentPrediction,
		ProductClass: broker.ProductClassEventContract,
		FeatureID:    broker.FeaturePredictionComboQuote,
		PageSize:     len(request.Legs),
		Params:       map[string]any{"operation": "quote", "mvc": request.MVC, "legs": orderLegMaps(request.Legs)},
	})
	if err != nil {
		return nil, err
	}
	quoteID := stringParam(result.Metadata, "quoteId")
	if quoteID == "" {
		return nil, fmt.Errorf("prediction combo quote did not return quoteId")
	}
	receivedAt := s.now().UTC()
	expiresAt := receivedAt.Add(30 * time.Second)
	record := broker.PredictionQuoteRecord{
		QuoteID: quoteID, BrokerID: result.Provider.BrokerID,
		AccountID: request.AccountID, TradingEnvironment: request.TradingEnvironment,
		MVC: request.MVC, LegsHash: broker.PredictionQuoteLegsHash(request.MVC, request.Legs),
		BidPrice:    optionalNumber(result.Metadata["bidPrice"]),
		AskPrice:    optionalNumber(result.Metadata["askPrice"]),
		ShouldRetry: boolValue(result.Metadata["shouldRetry"]),
		ReceivedAt:  receivedAt, ExpiresAt: expiresAt,
		ExpirySource: "jftrade_policy", Status: "active",
	}
	if s.predictionQuotes == nil {
		return nil, fmt.Errorf("prediction quote persistence is unavailable")
	}
	if err := s.predictionQuotes.SavePredictionQuote(ctx, record); err != nil {
		return nil, fmt.Errorf("persist prediction quote: %w", err)
	}
	result.Metadata["quoteId"] = quoteID
	result.Metadata["mvc"] = request.MVC
	result.Metadata["receivedAt"] = receivedAt.Format(time.RFC3339Nano)
	result.Metadata["quoteExpiresAt"] = expiresAt.Format(time.RFC3339Nano)
	result.Metadata["expirySource"] = record.ExpirySource
	result.Warnings = append(result.Warnings, "RFQ 有效期由 JFTrade 服务端接收时间起固定为 30 秒。")
	return result, nil
}

func orderLegMaps(legs []broker.OrderLegIntent) []any {
	result := make([]any, 0, len(legs))
	for _, leg := range legs {
		content, _ := json.Marshal(leg)
		var item map[string]any
		_ = json.Unmarshal(content, &item)
		result = append(result, item)
	}
	return result
}

func optionalNumber(value any) *float64 {
	number, ok := value.(float64)
	if !ok {
		return nil
	}
	return &number
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}
