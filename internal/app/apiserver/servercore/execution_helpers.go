package servercore

import (
	"encoding/json"
	"strings"
)

func executionBrokerLookupKey(brokerID string, tradingEnvironment string, accountID string, market string, brokerOrderID string) string {
	brokerOrderID = strings.TrimSpace(brokerOrderID)
	if brokerOrderID == "" {
		return ""
	}
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(brokerID)),
		strings.ToUpper(strings.TrimSpace(tradingEnvironment)),
		strings.TrimSpace(accountID),
		strings.ToUpper(strings.TrimSpace(market)),
		brokerOrderID,
	}, "|")
}

func executionFillLookupKey(brokerID string, accountID string, tradingEnvironment string, market string, brokerFillID string, brokerFillIDEx *string) string {
	fillID := strings.TrimSpace(brokerFillID)
	if fillID == "" {
		fillID = strings.TrimSpace(derefString(brokerFillIDEx))
	}
	if fillID == "" {
		return ""
	}
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(brokerID)),
		strings.ToUpper(strings.TrimSpace(tradingEnvironment)),
		strings.TrimSpace(accountID),
		strings.ToUpper(strings.TrimSpace(market)),
		fillID,
	}, "|")
}

func marshalExecutionPayload(payload any) string {
	if payload == nil {
		return "{}"
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func cloneExecutionOrderSummary(in executionOrderSummaryResponse) executionOrderSummaryResponse {
	return executionOrderSummaryResponse{
		InternalOrderID:    in.InternalOrderID,
		BrokerID:           in.BrokerID,
		BrokerOrderID:      cloneStringPointer(in.BrokerOrderID),
		BrokerOrderIDEx:    cloneStringPointer(in.BrokerOrderIDEx),
		Source:             in.Source,
		SourceDetail:       in.SourceDetail,
		TradingEnvironment: in.TradingEnvironment,
		AccountID:          in.AccountID,
		Market:             in.Market,
		Symbol:             cloneStringPointer(in.Symbol),
		Side:               cloneStringPointer(in.Side),
		OrderType:          cloneStringPointer(in.OrderType),
		Status:             in.Status,
		RawBrokerStatus:    cloneStringPointer(in.RawBrokerStatus),
		RequestedQuantity:  cloneFloat64Pointer(in.RequestedQuantity),
		RequestedPrice:     cloneFloat64Pointer(in.RequestedPrice),
		FilledQuantity:     cloneFloat64Pointer(in.FilledQuantity),
		FilledAveragePrice: cloneFloat64Pointer(in.FilledAveragePrice),
		Remark:             cloneStringPointer(in.Remark),
		LastError:          cloneStringPointer(in.LastError),
		LastErrorCode:      cloneStringPointer(in.LastErrorCode),
		LastErrorSource:    cloneStringPointer(in.LastErrorSource),
		SubmittedAt:        cloneStringPointer(in.SubmittedAt),
		UpdatedAt:          in.UpdatedAt,
		CreatedAt:          in.CreatedAt,
	}
}

func cloneExecutionOrderEvent(in executionOrderEventResponse) executionOrderEventResponse {
	return executionOrderEventResponse{
		ID:              in.ID,
		InternalOrderID: in.InternalOrderID,
		EventType:       in.EventType,
		PreviousStatus:  cloneStringPointer(in.PreviousStatus),
		NextStatus:      in.NextStatus,
		PayloadJSON:     in.PayloadJSON,
		CreatedAt:       in.CreatedAt,
	}
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneFloat64Pointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return new(*value)
}

func stringPointersDiffer(left *string, right *string) bool {
	return derefString(left) != derefString(right)
}

func float64PointersDiffer(left *float64, right *float64) bool {
	return optionalFloat64(left) != optionalFloat64(right)
}

func optionalFloat64(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func executionStringPointerOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
