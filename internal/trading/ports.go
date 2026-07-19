package trading

import (
	"context"
	"errors"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

var (
	ErrOrderStoreUnavailable   = errors.New("execution order store is unavailable")
	ErrOrderGatewayUnavailable = errors.New("execution order gateway is unavailable")
)

// OrderStore owns the durable execution ledger read boundary.
type OrderStore interface {
	ListOrders(context.Context, ExecutionOrderFilter) (ExecutionOrders, error)
	OrderEvents(context.Context, string) (ExecutionOrderEvents, error)
}

// OrderGateway owns broker-facing execution commands.
type OrderGateway interface {
	PlaceOrder(context.Context, ExecutionOrderCommand) (ExecutionOrder, error)
	CancelOrder(context.Context, string) (ExecutionOrder, error)
}

type ComboOrderGateway interface {
	PlaceCombo(context.Context, broker.ComboOrderIntent) (ExecutionOrder, error)
	CancelCombo(context.Context, string) (ExecutionOrder, error)
}

type ExecutionPreviewRecord struct {
	PreviewID         string
	RequestHash       string
	BrokerID          string
	CapabilityVersion string
	AccountID         string
	ExpiresAt         string
	QuoteExpiresAt    string
	RFQID             string
	NormalizedRequest string
	CreatedAt         string
}

type ExecutionPreviewStore interface {
	SavePreview(ExecutionPreviewRecord) error
	ConsumePreview(previewID, brokerID, accountID, requestHash, clientOrderID string) error
}

// BrokerRuntimeProvider resolves the active broker and its runtime state.
type BrokerRuntimeProvider interface {
	ActiveBroker() broker.Broker
	Runtime(context.Context) *BrokerRuntimeResponse
}

type orderStoreFunctions struct {
	list   func(context.Context, ExecutionOrderFilter) (ExecutionOrders, error)
	events func(context.Context, string) (ExecutionOrderEvents, error)
}

func (f *orderStoreFunctions) ListOrders(ctx context.Context, filter ExecutionOrderFilter) (ExecutionOrders, error) {
	if f == nil || f.list == nil {
		return ExecutionOrders{}, ErrOrderStoreUnavailable
	}
	return f.list(ctx, filter)
}

func (f *orderStoreFunctions) OrderEvents(ctx context.Context, id string) (ExecutionOrderEvents, error) {
	if f == nil || f.events == nil {
		return ExecutionOrderEvents{}, ErrOrderStoreUnavailable
	}
	return f.events(ctx, id)
}

type orderGatewayFunctions struct {
	place  func(context.Context, ExecutionOrderCommand) (ExecutionOrder, error)
	cancel func(context.Context, string) (ExecutionOrder, error)
}

type comboOrderGatewayFunctions struct {
	place  func(context.Context, broker.ComboOrderIntent) (ExecutionOrder, error)
	cancel func(context.Context, string) (ExecutionOrder, error)
}

func (f *orderGatewayFunctions) PlaceOrder(ctx context.Context, command ExecutionOrderCommand) (ExecutionOrder, error) {
	if f == nil || f.place == nil {
		return ExecutionOrder{}, ErrOrderGatewayUnavailable
	}
	return f.place(ctx, command)
}

func (f *orderGatewayFunctions) CancelOrder(ctx context.Context, id string) (ExecutionOrder, error) {
	if f == nil || f.cancel == nil {
		return ExecutionOrder{}, ErrOrderGatewayUnavailable
	}
	return f.cancel(ctx, id)
}

func (f *comboOrderGatewayFunctions) PlaceCombo(ctx context.Context, intent broker.ComboOrderIntent) (ExecutionOrder, error) {
	if f == nil || f.place == nil {
		return ExecutionOrder{}, ErrOrderGatewayUnavailable
	}
	return f.place(ctx, intent)
}

func (f *comboOrderGatewayFunctions) CancelCombo(ctx context.Context, id string) (ExecutionOrder, error) {
	if f == nil || f.cancel == nil {
		return ExecutionOrder{}, ErrOrderGatewayUnavailable
	}
	return f.cancel(ctx, id)
}

type brokerRuntimeFunctions struct {
	active  func() broker.Broker
	runtime func(context.Context) *BrokerRuntimeResponse
}

func (f *brokerRuntimeFunctions) ActiveBroker() broker.Broker {
	if f == nil || f.active == nil {
		return nil
	}
	return f.active()
}

func (f *brokerRuntimeFunctions) Runtime(ctx context.Context) *BrokerRuntimeResponse {
	if f == nil || f.runtime == nil {
		return emptyBrokerRuntimeResponse()
	}
	return f.runtime(ctx)
}
