package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestResolveBacktestTradingCostsDefaultsByMarketAndInstrument(t *testing.T) {
	hkCosts := resolveBacktestTradingCosts(RunConfig{Symbol: "HK.00700", Market: "HK", InstrumentType: "stock"}, "HKD", strategyir.StrategyMetadata{})
	if hkCosts.BrokerFees.PresetID != "futu_hk_hk_stock_2026_06_30" {
		t.Fatalf("HK broker preset = %q", hkCosts.BrokerFees.PresetID)
	}
	hkStamp, ok := feeRuleByID(hkCosts.MarketFees.Rules, "hk_stamp_duty")
	if !ok {
		t.Fatal("HK market preset missing stamp duty")
	}
	if hkStamp.Rounding != "ceil_currency_unit" || hkStamp.Rate != 0.001 {
		t.Fatalf("HK stamp rule = %#v", hkStamp)
	}

	usCosts := resolveBacktestTradingCosts(RunConfig{Symbol: "US.AAPL", Market: "US", InstrumentType: "stock"}, "USD", strategyir.StrategyMetadata{})
	usCommission, ok := feeRuleByID(usCosts.BrokerFees.Rules, "futu_hk_us_commission")
	if !ok {
		t.Fatal("US broker preset missing commission")
	}
	if usCommission.FixedAmount != 0.0049 || usCommission.MinAmount != 0.99 || usCommission.MaxRate != 0.005 {
		t.Fatalf("US broker commission = %#v", usCommission)
	}
	finraTAF, ok := feeRuleByID(usCosts.MarketFees.Rules, "finra_taf")
	if !ok {
		t.Fatal("US market preset missing FINRA TAF")
	}
	if finraTAF.Side != feeSideSell || finraTAF.MaxAmount != 9.79 {
		t.Fatalf("FINRA TAF rule = %#v", finraTAF)
	}

	cnStockCosts := resolveBacktestTradingCosts(RunConfig{Symbol: "SH.600519", Market: "SH", InstrumentType: "stock"}, "CNY", strategyir.StrategyMetadata{})
	if cnStockCosts.BrokerFees.Mode != tradingCostModeNone {
		t.Fatalf("CN broker mode = %q, want none", cnStockCosts.BrokerFees.Mode)
	}
	if _, ok := feeRuleByID(cnStockCosts.MarketFees.Rules, "cn_stamp_duty"); !ok {
		t.Fatal("CN stock preset missing sell-side stamp duty")
	}

	cnETFCosts := resolveBacktestTradingCosts(RunConfig{Symbol: "SH.510300", Market: "SH", InstrumentType: "etf"}, "CNY", strategyir.StrategyMetadata{})
	if _, ok := feeRuleByID(cnETFCosts.MarketFees.Rules, "cn_stamp_duty"); ok {
		t.Fatalf("CN ETF preset should exempt stamp duty: %#v", cnETFCosts.MarketFees.Rules)
	}
	if _, ok := feeRuleByID(cnETFCosts.MarketFees.Rules, "stock_connect_securities_management_fee"); ok {
		t.Fatalf("CN ETF preset should exempt securities management fee: %#v", cnETFCosts.MarketFees.Rules)
	}
}

func TestResolveBacktestQuoteCurrencySupportsCNSymbols(t *testing.T) {
	if got := resolveBacktestQuoteCurrency("SH.600519", ""); got != "CNY" {
		t.Fatalf("SH quote currency = %q, want CNY", got)
	}
	if got := resolveBacktestQuoteCurrency("SZ.000001", ""); got != "CNY" {
		t.Fatalf("SZ quote currency = %q, want CNY", got)
	}
	if got := resolveBacktestQuoteCurrency("CN.600519", ""); got != "CNY" {
		t.Fatalf("CN quote currency = %q, want CNY", got)
	}
	if got := resolveBacktestQuoteCurrency("HK.00700", "USD"); got != "USD" {
		t.Fatalf("requested quote currency = %q, want USD", got)
	}
}

func TestResolveFeeScheduleClonesAndNormalizesCustomRules(t *testing.T) {
	requested := FeeSchedule{
		Mode:     " custom ",
		PresetID: "custom-fees",
		Rules: []FeeRule{
			{},
			{
				ID:            " broker-share ",
				Label:         " ",
				Category:      "unknown",
				Side:          "SELL",
				Basis:         "quantity",
				Currency:      " usd ",
				Rounding:      "CEIL_CENT",
				EffectiveFrom: " 2026-01-02 ",
				EffectiveTo:   " 2026-12-31 ",
				AppliesTo:     []string{"fund", "ETF", "stock"},
			},
		},
	}
	resolved := resolveFeeSchedule(feeGroupBroker, requested, FeeSchedule{Mode: tradingCostModeNone}, FeeSchedule{})
	if resolved.Mode != tradingCostModeCustom || resolved.PresetID != "custom-fees" {
		t.Fatalf("resolved custom schedule = %#v", resolved)
	}
	if len(resolved.Rules) != 1 {
		t.Fatalf("normalized rules = %#v, want only valid id rule", resolved.Rules)
	}
	rule := resolved.Rules[0]
	if rule.ID != "broker-share" || rule.Label != "broker-share" || rule.Category != feeCategoryBroker || rule.Side != feeSideSell || rule.Basis != feeBasisShare || rule.Currency != "USD" || rule.Rounding != "ceil_cent" || rule.EffectiveFrom != "2026-01-02" || rule.EffectiveTo != "2026-12-31" {
		t.Fatalf("normalized custom rule = %#v", rule)
	}
	if len(rule.AppliesTo) != 2 || rule.AppliesTo[0] != instrumentTypeETF || rule.AppliesTo[1] != instrumentTypeStock {
		t.Fatalf("appliesTo = %#v, want ETF/stock dedupe", rule.AppliesTo)
	}
	requested.Rules[1].AppliesTo[0] = "mutated"
	if resolved.Rules[0].AppliesTo[0] != instrumentTypeETF {
		t.Fatalf("resolved custom rules share AppliesTo backing array: %#v", resolved.Rules[0].AppliesTo)
	}

	marketResolved := resolveFeeSchedule(feeGroupMarket, FeeSchedule{
		Rules: []FeeRule{{ID: "market-defaults", Category: "bad", Side: "bad", Basis: "bad"}},
	}, FeeSchedule{}, FeeSchedule{})
	if marketResolved.Mode != tradingCostModeCustom || len(marketResolved.Rules) != 1 {
		t.Fatalf("implicit custom market schedule = %#v", marketResolved)
	}
	if marketResolved.Rules[0].Category != feeCategoryExchange || marketResolved.Rules[0].Side != feeSideBoth || marketResolved.Rules[0].Basis != feeBasisNotional {
		t.Fatalf("market defaulted rule = %#v", marketResolved.Rules[0])
	}

	scriptWithoutBrokerRule := resolveFeeSchedule(feeGroupMarket, FeeSchedule{Mode: tradingCostModeScript}, FeeSchedule{}, FeeSchedule{Rules: []FeeRule{{ID: "script"}}})
	if scriptWithoutBrokerRule.Mode != tradingCostModeNone || len(scriptWithoutBrokerRule.Rules) != 0 {
		t.Fatalf("script market schedule = %#v, want none", scriptWithoutBrokerRule)
	}
	if cloned := cloneFeeRules(nil); cloned != nil {
		t.Fatalf("cloneFeeRules(nil) = %#v, want nil", cloned)
	}
}

func TestFeeRuleEffectiveRangeUsesInclusiveTradeDates(t *testing.T) {
	rule := FeeRule{EffectiveFrom: "2026-06-30", EffectiveTo: "2026-07-31"}
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{name: "missing trade time fails closed", want: false},
		{name: "before", at: time.Date(2026, time.June, 29, 23, 59, 0, 0, time.UTC), want: false},
		{name: "first day", at: time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC), want: true},
		{name: "last day", at: time.Date(2026, time.July, 31, 23, 59, 0, 0, time.UTC), want: true},
		{name: "after", at: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := feeRuleAppliesAt(rule, test.at); got != test.want {
				t.Fatalf("feeRuleAppliesAt(%s) = %v, want %v", test.at, got, test.want)
			}
		})
	}
	if feeRuleAppliesAt(FeeRule{EffectiveFrom: "not-a-date"}, time.Now()) {
		t.Fatal("invalid effectiveFrom must not apply")
	}
	if feeRuleAppliesAt(FeeRule{EffectiveFrom: "2026-08-01", EffectiveTo: "2026-07-31"}, time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("inverted effective range must not apply")
	}
	if !feeRuleAppliesAt(FeeRule{}, time.Time{}) {
		t.Fatal("unbounded rule should apply without a trade timestamp")
	}

	result := &RunResult{}
	engine := newBacktestFeeEngine(nil, "USD", "stock", TradingCosts{BrokerFees: FeeSchedule{
		Mode: tradingCostModeCustom,
		Rules: []FeeRule{{
			ID:            "dated-fee",
			Category:      feeCategoryBroker,
			Side:          feeSideBoth,
			Basis:         feeBasisNotional,
			Rate:          0.01,
			EffectiveFrom: rule.EffectiveFrom,
			EffectiveTo:   rule.EffectiveTo,
		}},
	}}, result, nil)
	for index, at := range []time.Time{
		time.Date(2026, time.June, 29, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.June, 30, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC),
	} {
		engine.onTradeUpdate(types.Trade{
			ID:       uint64(index + 1),
			OrderID:  uint64(index + 1),
			Symbol:   "US.AAPL",
			Side:     types.SideTypeBuy,
			Price:    fixedpoint.NewFromFloat(100),
			Quantity: fixedpoint.One,
			Time:     types.Time(at),
		})
	}
	engine.finalize()
	assertFloatNear(t, result.TotalBrokerFees, 1)
}

func TestScriptCommissionMapsToBrokerFeesOnly(t *testing.T) {
	costs := resolveBacktestTradingCosts(
		RunConfig{
			Symbol: "US.AAPL",
			Market: "US",
			TradingCosts: TradingCosts{
				BrokerFees: FeeSchedule{Mode: tradingCostModeScript},
				MarketFees: FeeSchedule{Mode: tradingCostModeScript},
			},
		},
		"USD",
		strategyir.StrategyMetadata{CommissionType: "percent", CommissionValue: 0.15},
	)
	if costs.BrokerFees.Mode != tradingCostModeScript {
		t.Fatalf("broker fee mode = %q, want script", costs.BrokerFees.Mode)
	}
	if costs.MarketFees.Mode != tradingCostModeNone {
		t.Fatalf("market fee mode = %q, want none", costs.MarketFees.Mode)
	}

	result := &RunResult{}
	engine := newBacktestFeeEngine(nil, "USD", "stock", costs, result, nil)
	engine.onTradeUpdate(types.Trade{
		ID:       301,
		OrderID:  41,
		Symbol:   "US.AAPL",
		Side:     types.SideTypeBuy,
		Price:    fixedpoint.NewFromFloat(100),
		Quantity: fixedpoint.NewFromFloat(10),
	})
	engine.finalize()

	assertFloatNear(t, result.TotalBrokerFees, 1.5)
	assertFloatNear(t, result.TotalMarketFees, 0)
	assertFloatNear(t, result.TotalFees, 1.5)
	assertBreakdownAmount(t, result.FeeBreakdown, "broker", "pine_strategy_commission", 1.5)
}

func feeRuleByID(rules []FeeRule, id string) (FeeRule, bool) {
	for _, rule := range rules {
		if rule.ID == id {
			return rule, true
		}
	}
	return FeeRule{}, false
}

func assertBreakdownAmount(t *testing.T, breakdown []FeeBreakdownEntry, group, ruleID string, want float64) {
	t.Helper()
	for _, entry := range breakdown {
		if entry.Group == group && entry.RuleID == ruleID {
			assertFloatNear(t, entry.Amount, want)
			return
		}
	}
	t.Fatalf("missing breakdown %s/%s in %#v", group, ruleID, breakdown)
}

func assertFloatNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("got %.10f, want %.10f", got, want)
	}
}
