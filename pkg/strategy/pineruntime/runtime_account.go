package pineruntime

import (
	"strings"

	"github.com/c9s/bbgo/pkg/fixedpoint"

	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func (r *strategyRuntime) getAvailableCash() float64 {
	account := runtimeAccount(r.session)
	if account == nil {
		return 0
	}
	if quoteCurrency := r.strategyQuoteCurrency(); quoteCurrency != "" {
		if balance, ok := account.Balance(quoteCurrency); ok {
			if !balance.Available.IsZero() {
				return balance.Available.Float64()
			}
			if !balance.NetAsset.IsZero() {
				return balance.NetAsset.Float64()
			}
		}
	}
	if !account.TotalAccountValue.IsZero() {
		return account.TotalAccountValue.Float64()
	}
	total := fixedpoint.Zero
	for _, balance := range account.Balances() {
		total = total.Add(balance.Available)
	}
	if !total.IsZero() {
		return total.Float64()
	}
	for _, balance := range account.Balances() {
		total = total.Add(balance.NetAsset)
	}
	return total.Float64()
}

func (r *strategyRuntime) getTotalAccountValue() float64 {
	account := runtimeAccount(r.session)
	if account == nil {
		return 0
	}
	if !account.TotalAccountValue.IsZero() {
		return account.TotalAccountValue.Float64()
	}
	total := fixedpoint.Zero
	for _, balance := range account.Balances() {
		total = total.Add(balance.NetAsset)
	}
	if !total.IsZero() {
		return total.Float64()
	}
	for _, balance := range account.Balances() {
		total = total.Add(balance.Available)
	}
	return total.Float64()
}

func (r *strategyRuntime) getMarginBuyingPower() float64 {
	funds := r.brokerFunds()
	if funds == nil {
		return 0
	}
	return funds.GetPower()
}

func (r *strategyRuntime) getShortSellingPower() float64 {
	funds := r.brokerFunds()
	if funds == nil {
		return 0
	}
	return funds.GetMaxPowerShort()
}

func (r *strategyRuntime) brokerFunds() *trdcommonpb.Funds {
	account := runtimeAccount(r.session)
	if account == nil || account.RawAccount == nil {
		return nil
	}
	funds, _ := account.RawAccount.(*trdcommonpb.Funds)
	return funds
}

func (r *strategyRuntime) strategyQuoteCurrency() string {
	if r.session == nil || r.strategy == nil {
		return ""
	}
	market, ok := r.session.Market(r.symbol)
	if !ok {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(market.QuoteCurrency))
}
