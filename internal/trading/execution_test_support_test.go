package trading

import (
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func newExecutionTestService(options ...Option) *Service {
	defaultBroker := WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "test-broker"}
	})
	return NewService(append([]Option{defaultBroker}, options...)...)
}
