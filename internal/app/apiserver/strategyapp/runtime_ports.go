package strategyapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	"github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type accountSource struct {
	reader broker.MarketDataReader
}

type tradeService interface {
	PlaceExecutionOrder(context.Context, trading.ExecutionOrderCommand) (trading.ExecutionOrder, error)
	CancelExecutionOrder(context.Context, string) (trading.ExecutionCommandResponse, error)
}

func (source accountSource) QueryBrokerFunds(ctx context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error) {
	return source.reader.QueryFunds(ctx, query)
}

func (source accountSource) QueryBrokerPositions(ctx context.Context, query broker.ReadQuery) ([]broker.PositionSnapshot, error) {
	return source.reader.QueryPositions(ctx, query)
}

func AccountResolver(resolve func(string) broker.Broker) func(strategy.InstanceBinding) liveruntime.AccountSource {
	return func(binding strategy.InstanceBinding) liveruntime.AccountSource {
		if binding.BrokerAccount == nil || resolve == nil {
			return nil
		}
		selected := resolve(strings.TrimSpace(binding.BrokerAccount.BrokerID))
		if selected == nil || selected.Trading() == nil || selected.MarketData() == nil {
			return nil
		}
		return accountSource{reader: selected.MarketData()}
	}
}

func MarketDataCapabilities(service *marketdata.Service) func(context.Context) (marketdata.ProviderCapabilities, error) {
	return func(ctx context.Context) (marketdata.ProviderCapabilities, error) {
		descriptor, err := marketdataapp.RuntimeFromService(service).Descriptor(ctx)
		if err != nil {
			return marketdata.ProviderCapabilities{}, err
		}
		return descriptor.Capabilities, nil
	}
}

func TradeCommands(service *trading.Service) liveruntime.TradeCommandPort {
	if service == nil {
		return tradeCommands(nil)
	}
	return tradeCommands(service)
}

func tradeCommands(service tradeService) liveruntime.TradeCommandPort {
	return liveruntime.TradeCommandFuncs{
		Place: func(ctx context.Context, command trading.ExecutionOrderCommand) (trading.ExecutionOrder, error) {
			if service == nil {
				return trading.ExecutionOrder{}, fmt.Errorf("trading service is unavailable")
			}
			return service.PlaceExecutionOrder(ctx, command)
		},
		Cancel: func(ctx context.Context, internalOrderID string) (trading.ExecutionOrder, error) {
			if service == nil {
				return trading.ExecutionOrder{}, fmt.Errorf("trading service is unavailable")
			}
			response, err := service.CancelExecutionOrder(ctx, internalOrderID)
			if err != nil {
				return trading.ExecutionOrder{}, err
			}
			if response.InternalOrderID == nil {
				return trading.ExecutionOrder{}, fmt.Errorf("cancel execution order response missing internal order id")
			}
			return trading.ExecutionOrder{InternalOrderID: *response.InternalOrderID}, nil
		},
	}
}
