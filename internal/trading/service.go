// Package trading 提供交易业务服务，封装 broker 资源查询、execution 订单管理和
// portfolio 组合查询。HTTP 层只负责参数绑定与响应写入。
//
// 设计约束：
//   - 零 protobuf 依赖
//   - 零 gin/HTTP 框架依赖
package trading

import (
	"context"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

// Service trading 业务门面。
type Service struct {
	activeBroker              func() broker.Broker
	defaultMarket             func() string
	defaultTradingEnvironment func() string
	brokerRuntime             func(context.Context) map[string]any
	orderUpdates              *OrderUpdatesWorker
	listOrders                func(context.Context, ExecutionOrderFilter) (ExecutionOrders, error)
	placeOrder                func(context.Context, ExecutionOrderCommand) (ExecutionOrder, error)
	cancelOrder               func(context.Context, string) (ExecutionOrder, error)
	getOrderEvents            func(context.Context, string) (ExecutionOrderEvents, error)
}

// NewService 创建交易服务。
func NewService(opts ...Option) *Service {
	s := &Service{}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Option 函数式选项。
type Option func(*Service)

// WithActiveBroker 注入当前券商解析函数。
func WithActiveBroker(fn func() broker.Broker) Option {
	return func(s *Service) { s.activeBroker = fn }
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
	return func(s *Service) { s.brokerRuntime = fn }
}

func WithOrderUpdates(worker *OrderUpdatesWorker) Option {
	return func(s *Service) { s.orderUpdates = worker }
}

// WithListOrders 注入执行订单列表查询。
func WithListOrders(fn func(context.Context, ExecutionOrderFilter) (ExecutionOrders, error)) Option {
	return func(s *Service) { s.listOrders = fn }
}

// WithPlaceOrder 注入执行下单。
func WithPlaceOrder(fn func(context.Context, ExecutionOrderCommand) (ExecutionOrder, error)) Option {
	return func(s *Service) { s.placeOrder = fn }
}

// WithCancelOrder 注入执行撤单。
func WithCancelOrder(fn func(context.Context, string) (ExecutionOrder, error)) Option {
	return func(s *Service) { s.cancelOrder = fn }
}

// WithGetOrderEvents 注入订单事件查询。
func WithGetOrderEvents(fn func(context.Context, string) (ExecutionOrderEvents, error)) Option {
	return func(s *Service) { s.getOrderEvents = fn }
}

func (s *Service) SyncOrderUpdates(ctx context.Context, force bool, activeOnly bool) {
	if s != nil && s.orderUpdates != nil {
		s.orderUpdates.Sync(ctx, force, activeOnly)
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
