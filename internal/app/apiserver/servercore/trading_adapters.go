package servercore

import (
	"context"
	"strings"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/tradingapp"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// newTradingService 创建交易服务并注入运行期依赖。
func newTradingService(s *Server) *trdsrv.Service {
	fallbackMarket := strings.ToUpper(strings.TrimSpace(s.store.Integration().Config.TradeMarket))
	if fallbackMarket == "" {
		fallbackMarket = "HK"
	}
	orderUpdates := tradingapp.NewOrderUpdatesWorker(
		newTradingOrderUpdateSource(s),
		newTradingExecutionOrderUpdates(s),
		trdsrv.OrderUpdatesConfig{
			FallbackMarket: fallbackMarket,
			HistoryLookback: func() int {
				return s.store.ExecutionSettings().BrokerOrderHistoryLookbackDays
			},
		},
	)
	return trdsrv.NewService(
		trdsrv.WithBrokerRuntimeProvider(&serverTradingBrokerRuntimeProvider{server: s}),
		trdsrv.WithDefaultMarket(func() string {
			return s.store.Integration().Config.TradeMarket
		}),
		trdsrv.WithDefaultTradingEnvironment(func() string { return defaultTradingEnvironment(&s.serverApplication) }),
		trdsrv.WithOrderUpdates(orderUpdates),
		trdsrv.WithPreTradeRiskGateway(s.runtimes.PreTradeRisk()),
		trdsrv.WithOrderStore(s.stores.ExecutionOrders),
		trdsrv.WithExecutionPreviewStore(s.stores.ExecutionOrders),
		trdsrv.WithPredictionQuoteStore(s.stores.ExecutionOrders),
		trdsrv.WithOrderGateway(newServerExecutionGateway(&s.serverApplication)),
		trdsrv.WithComboOrderGateway(newServerExecutionGateway(&s.serverApplication)),
	)
}

func newTradingOrderUpdateSource(s *Server) *tradingapp.OrderUpdateSource {
	return tradingapp.NewOrderUpdateSource(tradingapp.OrderUpdateSourceOptions{
		Brokers: func() []broker.Broker {
			if registry := s.runtimes.Brokers(); registry != nil {
				return registry.All()
			}
			return nil
		},
		ActivateBroker: func() { _ = s.futuCoordinator().ActiveBroker() },
		ResolveBroker:  s.futuCoordinator().ResolveBroker,
		SubscribeOrders: func(
			ctx context.Context,
			accounts []trdsrv.Account,
			handler trdsrv.OrderUpdateHandler,
		) (trdsrv.OrderUpdateSubscription, error) {
			if runtime := s.runtimes.MarketData(); runtime != nil {
				return runtime.SubscribeOrderUpdates(ctx, accounts, handler)
			}
			return nil, nil
		},
	})
}

func newTradingExecutionOrderUpdates(s *Server) *tradingapp.ExecutionOrderUpdates {
	if s == nil {
		return tradingapp.NewExecutionOrderUpdates(nil, nil)
	}
	return tradingapp.NewExecutionOrderUpdates(s.stores.ExecutionOrders, s.notifyExecutionOrderLifecycle)
}

type serverTradingBrokerRuntimeProvider struct {
	server *Server
}

func (p *serverTradingBrokerRuntimeProvider) ActiveBroker() broker.Broker {
	return p.server.futuCoordinator().ActiveBroker()
}

func (p *serverTradingBrokerRuntimeProvider) ResolveBroker(id string) broker.Broker {
	return p.server.futuCoordinator().ResolveBroker(id)
}

func (p *serverTradingBrokerRuntimeProvider) Runtime(ctx context.Context) *trdsrv.BrokerRuntimeResponse {
	return p.server.futuCoordinator().BrokerRuntime(ctx)
}

var (
	_ trdsrv.BrokerRuntimeProvider = (*serverTradingBrokerRuntimeProvider)(nil)
)

func defaultTradingEnvironment(s *serverApplication) string {
	if s == nil || s.store == nil {
		return "SIMULATE"
	}
	return s.store.ExecutionSettings().DefaultTradingEnvironment
}

func placeExecutionOrder(s *serverApplication, ctx context.Context, request trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
	return newServerExecutionGateway(s).PlaceOrder(ctx, request)
}

func cancelExecutionOrder(s *serverApplication, ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
	return newServerExecutionGateway(s).CancelOrder(ctx, internalOrderID)
}

func newServerExecutionGateway(s *serverApplication) *tradingapp.ExecutionGateway {
	return tradingapp.NewExecutionGateway(tradingapp.ExecutionGatewayDependencies{
		ResolveBroker: s.futuCoordinator().ResolveBroker,
		Orders: func() tradingapp.ExecutionOrderStore {
			return s.stores.ExecutionOrders
		},
		NotifyPlaced: s.notifyExecutionOrderPlaced,
	})
}
