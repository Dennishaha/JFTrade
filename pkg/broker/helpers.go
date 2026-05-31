package broker

// ConvertFutuReadQuery converts a Futu BrokerReadQuery to a broker ReadQuery.
// This is a transitional helper during the refactoring; once all consumers
// use broker.ReadQuery directly, this can be removed.
func ConvertFutuReadQuery(accountID, tradingEnvironment, market string) ReadQuery {
	return ReadQuery{
		BrokerID:           "futu",
		AccountID:          accountID,
		TradingEnvironment: tradingEnvironment,
		Market:             market,
	}
}

// Float64Ptr is a helper to create a *float64 from a float64 value.
func Float64Ptr(v float64) *float64 { return &v }

// StringPtr is a helper to create a *string from a string value.
func StringPtr(v string) *string { return &v }

// BoolPtr is a helper to create a *bool from a bool value.
func BoolPtr(v bool) *bool { return &v }

// Uint64Ptr is a helper to create a *uint64 from a uint64 value.
func Uint64Ptr(v uint64) *uint64 { return &v }
