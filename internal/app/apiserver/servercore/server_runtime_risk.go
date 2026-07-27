package servercore

import (
	"context"

	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func (s *serverApplication) systemRiskOptions() []system.Option {
	return []system.Option{
		system.WithRealTradeRuntimeRiskControls(s.updateRuntimeRiskConfig, s.disableRuntimeRiskConfig),
		system.WithRealTradeKillSwitchControls(s.activateKillSwitch, s.releaseKillSwitch),
		system.WithRealTradeHardStopControls(s.activateHardStop, s.releaseHardStop),
	}
}

func (s *serverApplication) updateRuntimeRiskConfig(ctx context.Context, command system.RealTradeRuntimeRiskCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.runtimes.RealTradeControl().UpdateRuntimeRiskConfig(ctx, trdsrv.RealTradeRuntimeRiskCommand{
		TradingEnvironment: command.TradingEnvironment,
		RealTradingEnabled: command.RealTradingEnabled,
		MaxOrderQuantity:   command.MaxOrderQuantity,
		MaxOrderNotional:   command.MaxOrderNotional,
		OperatorID:         command.OperatorID,
		Reason:             command.Reason,
	})
}

func (s *serverApplication) disableRuntimeRiskConfig(ctx context.Context, command system.RealTradeRuntimeRiskCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.runtimes.RealTradeControl().DisableRuntimeRiskConfig(ctx, trdsrv.RealTradeRuntimeRiskCommand{
		TradingEnvironment: command.TradingEnvironment,
		OperatorID:         command.OperatorID,
		Reason:             command.Reason,
	})
}

func (s *serverApplication) activateKillSwitch(ctx context.Context, command system.RealTradeKillSwitchCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.runtimes.RealTradeControl().ActivateKillSwitch(ctx, trdsrv.RealTradeKillSwitchCommand{
		TradingEnvironment: command.TradingEnvironment,
		OperatorID:         command.OperatorID,
		Reason:             command.Reason,
	})
}

func (s *serverApplication) releaseKillSwitch(ctx context.Context, command system.RealTradeKillSwitchCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.runtimes.RealTradeControl().ReleaseKillSwitch(ctx, trdsrv.RealTradeKillSwitchCommand{
		TradingEnvironment: command.TradingEnvironment,
		OperatorID:         command.OperatorID,
		Reason:             command.Reason,
	})
}

func (s *serverApplication) activateHardStop(ctx context.Context, command system.RealTradeHardStopCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.runtimes.RealTradeControl().ActivateHardStop(ctx, trdsrv.RealTradeHardStopCommand{
		BrokerID:           command.BrokerID,
		TradingEnvironment: command.TradingEnvironment,
		AccountID:          command.AccountID,
		Market:             command.Market,
		Symbol:             command.Symbol,
		HardStopScope:      command.HardStopScope,
		OperatorID:         command.OperatorID,
		Reason:             command.Reason,
	})
}

func (s *serverApplication) releaseHardStop(ctx context.Context, id string, command system.RealTradeHardStopCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.runtimes.RealTradeControl().ReleaseHardStop(ctx, id, trdsrv.RealTradeHardStopCommand{
		OperatorID: command.OperatorID,
		Reason:     command.Reason,
	})
}
