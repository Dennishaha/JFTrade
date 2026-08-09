package futuapp

import (
	"errors"

	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// ErrFutuIntegrationNotEnabled is returned when a Futu-backed operation is
// requested while the Futu integration is disabled or unavailable.
var ErrFutuIntegrationNotEnabled = errors.New("futu integration is not enabled")

// ExchangeOrError returns the stable exchange boundary or the disabled-error.
func ExchangeOrError(c *Coordinator) (futuintegration.RuntimeExchange, error) {
	exchange := c.Exchange()
	if exchange == nil {
		return nil, ErrFutuIntegrationNotEnabled
	}
	return exchange, nil
}

// BrokerOrError returns the Futu broker adapter or the disabled-error.
func BrokerOrError(c *Coordinator) (broker.Broker, error) {
	b := c.Broker()
	if b == nil {
		return nil, ErrFutuIntegrationNotEnabled
	}
	return b, nil
}
