// Package trading 提供交易业务服务，封装 broker 资源查询、execution 订单管理和
// portfolio 组合查询。HTTP 层只负责参数绑定与响应写入。
//
// 设计约束：
//   - 零 protobuf 依赖
//   - 零 gin/HTTP 框架依赖
package trading

import (
	"context"
	"errors"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

var ErrExecutionOrderNotFound = errors.New("execution order not found")

// Service trading 业务门面。
type Service struct {
	defaultMarket             func() string
	defaultTradingEnvironment func() string
	brokerRuntime             BrokerRuntimeProvider
	orderUpdates              *OrderUpdatesWorker
	preTradeRisk              PreTradeRiskGateway
	orderStore                OrderStore
	orderGateway              OrderGateway
}

// NewService 创建交易服务。
func NewService(opts ...Option) *Service {
	s := &Service{
		brokerRuntime: &brokerRuntimeFunctions{},
		orderStore:    &orderStoreFunctions{},
		orderGateway:  &orderGatewayFunctions{},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Option 函数式选项。
type Option func(*Service)

// WithActiveBroker 注入当前券商解析函数。
func WithActiveBroker(fn func() broker.Broker) Option {
	return func(s *Service) { ensureBrokerRuntimeFunctions(s).active = fn }
}

// WithDefaultMarket 注入默认交易市场。
func WithDefaultMarket(fn func() string) Option {
	return func(s *Service) { s.defaultMarket = fn }
}

func WithDefaultTradingEnvironment(fn func() string) Option {
	return func(s *Service) { s.defaultTradingEnvironment = fn }
}

// WithBrokerRuntime 注入券商运行态读取函数。
func WithBrokerRuntime(fn func(context.Context) map[string]any) Option {
	return func(s *Service) { ensureBrokerRuntimeFunctions(s).runtime = fn }
}

func WithBrokerRuntimeProvider(provider BrokerRuntimeProvider) Option {
	return func(s *Service) { s.brokerRuntime = provider }
}

func WithOrderUpdates(worker *OrderUpdatesWorker) Option {
	return func(s *Service) { s.orderUpdates = worker }
}

func WithPreTradeRiskGateway(gateway PreTradeRiskGateway) Option {
	return func(s *Service) { s.preTradeRisk = gateway }
}

// WithListOrders 注入执行订单列表查询。
func WithListOrders(fn func(context.Context, ExecutionOrderFilter) (ExecutionOrders, error)) Option {
	return func(s *Service) { ensureOrderStoreFunctions(s).list = fn }
}

// WithPlaceOrder 注入执行下单。
func WithPlaceOrder(fn func(context.Context, ExecutionOrderCommand) (ExecutionOrder, error)) Option {
	return func(s *Service) { ensureOrderGatewayFunctions(s).place = fn }
}

// WithCancelOrder 注入执行撤单。
func WithCancelOrder(fn func(context.Context, string) (ExecutionOrder, error)) Option {
	return func(s *Service) { ensureOrderGatewayFunctions(s).cancel = fn }
}

// WithGetOrderEvents 注入订单事件查询。
func WithGetOrderEvents(fn func(context.Context, string) (ExecutionOrderEvents, error)) Option {
	return func(s *Service) { ensureOrderStoreFunctions(s).events = fn }
}

func WithOrderStore(store OrderStore) Option {
	return func(s *Service) { s.orderStore = store }
}

func WithOrderGateway(gateway OrderGateway) Option {
	return func(s *Service) { s.orderGateway = gateway }
}

func ensureOrderStoreFunctions(s *Service) *orderStoreFunctions {
	if functions, ok := s.orderStore.(*orderStoreFunctions); ok {
		return functions
	}
	functions := &orderStoreFunctions{}
	s.orderStore = functions
	return functions
}

func ensureOrderGatewayFunctions(s *Service) *orderGatewayFunctions {
	if functions, ok := s.orderGateway.(*orderGatewayFunctions); ok {
		return functions
	}
	functions := &orderGatewayFunctions{}
	s.orderGateway = functions
	return functions
}

func ensureBrokerRuntimeFunctions(s *Service) *brokerRuntimeFunctions {
	if functions, ok := s.brokerRuntime.(*brokerRuntimeFunctions); ok {
		return functions
	}
	functions := &brokerRuntimeFunctions{}
	s.brokerRuntime = functions
	return functions
}

func (s *Service) SyncOrderUpdates(ctx context.Context, force bool, activeOnly bool) {
	if s != nil && s.orderUpdates != nil {
		s.orderUpdates.Sync(ctx, force, activeOnly)
	}
}

func (s *Service) SyncExecutionOrderHistory(ctx context.Context, order ExecutionOrder) {
	if s != nil && s.orderUpdates != nil {
		s.orderUpdates.SyncExecutionOrderHistory(ctx, order)
	}
}

func (s *Service) OrderUpdatesSnapshot() map[string]any {
	if s == nil || s.orderUpdates == nil {
		return map[string]any{}
	}
	return s.orderUpdates.SnapshotResponse()
}

func (s *Service) StopOrderUpdates() error {
	if s == nil || s.orderUpdates == nil {
		return nil
	}
	return s.orderUpdates.Stop()
}
