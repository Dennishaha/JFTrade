package broker

import "fmt"

// BrokerError represents an error from a broker adapter with structured metadata.
type BrokerError struct {
	BrokerID string
	Code     string
	Message  string
}

func (e *BrokerError) Error() string {
	return fmt.Sprintf("broker %s: [%s] %s", e.BrokerID, e.Code, e.Message)
}

// NewBrokerError creates a new BrokerError.
func NewBrokerError(brokerID, code, message string) *BrokerError {
	return &BrokerError{BrokerID: brokerID, Code: code, Message: message}
}

// Common error codes.
const (
	ErrCodeNotConnected       = "NOT_CONNECTED"
	ErrCodeAccountNotFound    = "ACCOUNT_NOT_FOUND"
	ErrCodeMarketNotSupported = "MARKET_NOT_SUPPORTED"
	ErrCodeOrderNotFound      = "ORDER_NOT_FOUND"
	ErrCodeInsufficientFunds  = "INSUFFICIENT_FUNDS"
	ErrCodeRateLimited        = "RATE_LIMITED"
	ErrCodeTimeout            = "TIMEOUT"
	ErrCodeInternal           = "INTERNAL"
)
