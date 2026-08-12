package assembly

import (
	"context"
	"fmt"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (a *ApplicationAdapter) brokerRuntime(ctx context.Context) (BrokerRuntimeView, error) {
	service, err := a.requireTrading()
	if err != nil {
		return BrokerRuntimeView{Accounts: []BrokerAccountView{}}, err
	}
	runtime, err := service.Runtime(ctx, "futu")
	if err != nil {
		return BrokerRuntimeView{Accounts: []BrokerAccountView{}}, err
	}
	if runtime == nil {
		return BrokerRuntimeView{Accounts: []BrokerAccountView{}}, fmt.Errorf("broker runtime returned an empty response")
	}
	accounts := make([]BrokerAccountView, 0, len(runtime.Accounts))
	for _, account := range runtime.Accounts {
		accounts = append(accounts, BrokerAccountView{
			AccountID: account.AccountID, TradingEnvironment: account.TradingEnvironment,
			AccountType: account.AccountType, AccountRole: account.AccountRole,
			SecurityFirm:         account.SecurityFirm,
			MarketAuthorities:    append([]string(nil), account.MarketAuthorities...),
			SimulatedAccountType: account.SimulatedAccountType,
		})
	}
	view := BrokerRuntimeView{Connectivity: runtime.Session.Connectivity, Accounts: accounts}
	if runtime.Session.LastError != nil {
		view.LastError = *runtime.Session.LastError
	}
	return view, nil
}

func (a *ApplicationAdapter) brokerAccountRead(
	ctx context.Context,
	query broker.ReadQuery,
	timeout time.Duration,
) BrokerAccountReadResult {
	service := a.trading()
	if service == nil {
		return BrokerAccountReadResult{Partial: true, Errors: []string{"trading service is unavailable"}}
	}
	fundsReady := make(chan *trdsrv.BrokerFundsResponse, 1)
	positionsReady := make(chan *trdsrv.BrokerPositionsResponse, 1)
	go func() {
		fundsReady <- service.FundsWithTimeout(ctx, query, timeout)
	}()
	go func() {
		positionsReady <- service.PositionsWithTimeout(ctx, query, timeout)
	}()
	return summarizeBrokerAccountResponses(<-fundsReady, <-positionsReady)
}

func summarizeBrokerAccountResponses(
	funds *trdsrv.BrokerFundsResponse,
	positions *trdsrv.BrokerPositionsResponse,
) BrokerAccountReadResult {
	result := BrokerAccountReadResult{Funds: funds, Positions: positions, Errors: []string{}}
	if funds == nil {
		result.Partial = true
		result.Errors = append(result.Errors, "funds: empty broker response")
	} else {
		result.HasAssetsOrPositions = brokerFundsHaveAssets(funds)
		if funds.LastError != nil {
			result.Partial = true
			result.Errors = append(result.Errors, "funds: "+*funds.LastError)
		}
	}
	if positions == nil {
		result.Partial = true
		result.Errors = append(result.Errors, "positions: empty broker response")
	} else {
		result.PositionCount = len(positions.Positions)
		result.HasAssetsOrPositions = result.HasAssetsOrPositions || result.PositionCount > 0
		if positions.LastError != nil {
			result.Partial = true
			result.Errors = append(result.Errors, "positions: "+*positions.LastError)
		}
	}
	return result
}

func brokerFundsHaveAssets(response *trdsrv.BrokerFundsResponse) bool {
	if response == nil {
		return false
	}
	if summary := response.Summary; summary != nil && anyNonZero(
		summary.TotalAssets, summary.SecuritiesAssets, summary.FundAssets,
		summary.BondAssets, summary.Cash, summary.MarketValue,
	) {
		return true
	}
	for _, balance := range response.CurrencyBalances {
		if anyNonZero(balance.Cash, balance.NetCashPower) {
			return true
		}
	}
	for _, asset := range response.MarketAssets {
		if anyNonZero(asset.Assets) {
			return true
		}
	}
	return false
}

func anyNonZero(values ...*float64) bool {
	for _, value := range values {
		if value != nil && *value != 0 {
			return true
		}
	}
	return false
}
