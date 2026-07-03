package bbgo

import (
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type Config struct {
	Backtest *Backtest `json:"backtest,omitempty" yaml:"backtest,omitempty"`
}

type Backtest struct {
	StartTime types.LooseFormatTime  `json:"startTime" yaml:"startTime"`
	EndTime   *types.LooseFormatTime `json:"endTime,omitempty" yaml:"endTime,omitempty"`
	Symbols   []string               `json:"symbols" yaml:"symbols"`
	Sessions  []string               `json:"sessions,omitempty" yaml:"sessions,omitempty"`
	Accounts  map[string]BacktestAccount
	FeeMode   BacktestFeeMode `json:"feeMode,omitempty" yaml:"feeMode,omitempty"`
}

type BacktestFeeMode int

const (
	BacktestFeeModeQuote BacktestFeeMode = iota
	BacktestFeeModeBase
)

type BacktestAccount struct {
	MakerFeeRate fixedpoint.Value
	TakerFeeRate fixedpoint.Value
	Balances     BacktestAccountBalanceMap
}

type BacktestAccountBalanceMap map[string]fixedpoint.Value

func (m BacktestAccountBalanceMap) BalanceMap() types.BalanceMap {
	balances := types.BalanceMap{}
	for currency, value := range m {
		balances[currency] = types.Balance{
			Currency:  currency,
			Available: value,
		}
	}
	return balances
}

func (b *Backtest) GetAccount(name string) BacktestAccount {
	if b == nil || b.Accounts == nil {
		return BacktestAccount{}
	}
	if account, ok := b.Accounts[name]; ok {
		return account
	}
	return BacktestAccount{}
}
