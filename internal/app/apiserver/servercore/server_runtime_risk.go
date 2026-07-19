package servercore

import (
	"context"

	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func (s *Server) systemRiskOptions() []system.Option {
	return []system.Option{
		system.WithRealTradeRuntimeRiskControls(s.updateRuntimeRiskConfig, s.disableRuntimeRiskConfig),
		system.WithRealTradeKillSwitchControls(s.activateKillSwitch, s.releaseKillSwitch),
		system.WithRealTradeHardStopControls(s.activateHardStop, s.releaseHardStop),
	}
}

func (s *Server) updateRuntimeRiskConfig(ctx context.Context, command system.RealTradeRuntimeRiskCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.realTradeControlPlane.UpdateRuntimeRiskConfig(ctx, trdsrv.RealTradeRuntimeRiskCommand{
		TradingEnvironment: command.TradingEnvironment,
		RealTradingEnabled: command.RealTradingEnabled,
		MaxOrderQuantity:   command.MaxOrderQuantity,
		MaxOrderNotional:   command.MaxOrderNotional,
		OperatorID:         command.OperatorID,
		Reason:             command.Reason,
	})
}

func (s *Server) disableRuntimeRiskConfig(ctx context.Context, command system.RealTradeRuntimeRiskCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.realTradeControlPlane.DisableRuntimeRiskConfig(ctx, trdsrv.RealTradeRuntimeRiskCommand{
		TradingEnvironment: command.TradingEnvironment,
		OperatorID:         command.OperatorID,
		Reason:             command.Reason,
	})
}

func (s *Server) activateKillSwitch(ctx context.Context, command system.RealTradeKillSwitchCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.realTradeControlPlane.ActivateKillSwitch(ctx, trdsrv.RealTradeKillSwitchCommand{
		TradingEnvironment: command.TradingEnvironment,
		OperatorID:         command.OperatorID,
		Reason:             command.Reason,
	})
}

func (s *Server) releaseKillSwitch(ctx context.Context, command system.RealTradeKillSwitchCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.realTradeControlPlane.ReleaseKillSwitch(ctx, trdsrv.RealTradeKillSwitchCommand{
		TradingEnvironment: command.TradingEnvironment,
		OperatorID:         command.OperatorID,
		Reason:             command.Reason,
	})
}

func (s *Server) activateHardStop(ctx context.Context, command system.RealTradeHardStopCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.realTradeControlPlane.ActivateHardStop(ctx, trdsrv.RealTradeHardStopCommand{
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

func (s *Server) releaseHardStop(ctx context.Context, id string, command system.RealTradeHardStopCommand) (trdsrv.RealTradeRiskSnapshot, error) {
	return s.realTradeControlPlane.ReleaseHardStop(ctx, id, trdsrv.RealTradeHardStopCommand{
		OperatorID: command.OperatorID,
		Reason:     command.Reason,
	})
}
