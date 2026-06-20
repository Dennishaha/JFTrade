package pineruntime

import (
	"github.com/c9s/bbgo/pkg/fixedpoint"
)

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
