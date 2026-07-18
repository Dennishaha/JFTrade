package trading

import (
	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func newExecutionRouteTestService(options ...srv.Option) *srv.Service {
	defaultBroker := srv.WithActiveBroker(func() broker.Broker {
		return &routeTestBroker{id: "test-broker"}
	})
	return srv.NewService(append([]srv.Option{defaultBroker}, options...)...)
}
