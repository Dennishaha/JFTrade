package backtest

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	bbgo2 "github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

const (
	tradingCostModeMarketPreset = "market_preset"
	tradingCostModeCustom       = "custom"
	tradingCostModeScript       = "script"
	tradingCostModeNone         = "none"

	feeGroupBroker = "broker"
	feeGroupMarket = "market"

	feeCategoryBroker     = "broker"
	feeCategoryExchange   = "exchange"
	feeCategoryClearing   = "clearing"
	feeCategoryRegulatory = "regulatory"
	feeCategoryTax        = "tax"

	feeBasisNotional = "notional"
	feeBasisShare    = "share"
	feeBasisOrder    = "order"

	feeSideBoth = "both"
	feeSideBuy  = "buy"
	feeSideSell = "sell"

	instrumentTypeStock = "stock"
	instrumentTypeETF   = "etf"
)

const (
	futuHKFeesSourceURL           = "https://www.futuhk.com/en/support/topic2_335"
	futuUSFeesSourceURL           = "https://www.futuhk.com/en/support/topic2_283"
	hkexHKFeesSourceURL           = "https://www.hkex.com.hk/Services/Rules-and-Forms-and-Fees/Fees/Securities-%28Hong-Kong%29/Trading/Transaction?sc_lang=en"
	hkexStockConnectFeesSourceURL = "https://www.hkex.com.hk/Services/Rules-and-Forms-and-Fees/Fees/Securities-%28Stock-Connect%29/Trading/Transactions?sc_lang=en"
	secFeeRateSourceURL           = "https://www.sec.gov/rules-regulations/fee-rate-advisories/2026-2"
	finraTAFSourceURL             = "https://www.finra.org/rules-guidance/guidance/faqs/trading-activity-fee"
	cnStampDutySourceURL          = "https://english.www.gov.cn/policies/policywatch/202308/28/content_WS64ec5513c6d0868f4e8dee23.html"
)

func defaultTradingCostsForRun(market, symbol, quoteCurrency, instrumentType string) TradingCosts {
	market = normalizeCostMarket(market, symbol)
	quoteCurrency = strings.ToUpper(strings.TrimSpace(quoteCurrency))
	instrumentType = normalizeInstrumentType(instrumentType)
	return TradingCosts{
		BrokerFees: defaultBrokerFeeSchedule(market, quoteCurrency),
		MarketFees: defaultMarketFeeSchedule(market, quoteCurrency, instrumentType),
	}
}

func resolveBacktestTradingCosts(cfg RunConfig, quoteCurrency string, metadata strategyir.StrategyMetadata) TradingCosts {
	defaults := defaultTradingCostsForRun(cfg.Market, cfg.Symbol, quoteCurrency, cfg.InstrumentType)
	return TradingCosts{
		BrokerFees: resolveFeeSchedule(feeGroupBroker, cfg.TradingCosts.BrokerFees, defaults.BrokerFees, pineBrokerFeeSchedule(metadata, quoteCurrency)),
		MarketFees: resolveFeeSchedule(feeGroupMarket, cfg.TradingCosts.MarketFees, defaults.MarketFees, FeeSchedule{Mode: tradingCostModeNone}),
	}
}

func resolveFeeSchedule(group string, requested FeeSchedule, preset FeeSchedule, script FeeSchedule) FeeSchedule {
	mode := normalizeTradingCostMode(requested.Mode)
	if mode == "" {
		if len(requested.Rules) > 0 {
			mode = tradingCostModeCustom
		} else if strings.TrimSpace(requested.PresetID) != "" {
			mode = tradingCostModeMarketPreset
		} else {
			mode = tradingCostModeMarketPreset
		}
	}

	var resolved FeeSchedule
	switch mode {
	case tradingCostModeNone:
		resolved = FeeSchedule{Mode: tradingCostModeNone}
	case tradingCostModeCustom:
		resolved = FeeSchedule{Mode: tradingCostModeCustom, PresetID: strings.TrimSpace(requested.PresetID), Rules: cloneFeeRules(requested.Rules)}
	case tradingCostModeScript:
		if group == feeGroupBroker && len(script.Rules) > 0 {
			resolved = script
		} else {
			resolved = FeeSchedule{Mode: tradingCostModeNone}
		}
	case tradingCostModeMarketPreset:
		resolved = preset
	default:
		resolved = preset
	}

	resolved.Mode = normalizeTradingCostMode(resolved.Mode)
	if resolved.Mode == "" {
		resolved.Mode = mode
	}
	resolved.Rules = normalizeFeeRules(group, resolved.Rules)
	if len(resolved.Rules) == 0 && resolved.Mode != tradingCostModeScript && resolved.Mode != tradingCostModeCustom {
		resolved.Mode = tradingCostModeNone
	}
	return resolved
}

func normalizeTradingCostMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case tradingCostModeMarketPreset:
		return tradingCostModeMarketPreset
	case tradingCostModeCustom:
		return tradingCostModeCustom
	case tradingCostModeScript:
		return tradingCostModeScript
	case tradingCostModeNone:
		return tradingCostModeNone
	default:
		return ""
	}
}

func normalizeFeeRules(group string, rules []FeeRule) []FeeRule {
	normalized := make([]FeeRule, 0, len(rules))
	for _, rule := range rules {
		rule.ID = strings.TrimSpace(rule.ID)
		if rule.ID == "" {
			continue
		}
		rule.Label = strings.TrimSpace(rule.Label)
		if rule.Label == "" {
			rule.Label = rule.ID
		}
		rule.Category = normalizeFeeCategory(rule.Category, group)
		rule.Side = normalizeFeeSide(rule.Side)
		rule.Basis = normalizeFeeBasis(rule.Basis)
		rule.Currency = strings.ToUpper(strings.TrimSpace(rule.Currency))
		rule.Rounding = strings.ToLower(strings.TrimSpace(rule.Rounding))
		rule.EffectiveFrom = strings.TrimSpace(rule.EffectiveFrom)
		rule.EffectiveTo = strings.TrimSpace(rule.EffectiveTo)
		rule.AppliesTo = normalizeAppliesTo(rule.AppliesTo)
		normalized = append(normalized, rule)
	}
	return normalized
}

func normalizeFeeCategory(value string, group string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case feeCategoryBroker, feeCategoryExchange, feeCategoryClearing, feeCategoryRegulatory, feeCategoryTax:
		return normalized
	}
	if group == feeGroupBroker {
		return feeCategoryBroker
	}
	return feeCategoryExchange
}

func normalizeFeeSide(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case feeSideBuy:
		return feeSideBuy
	case feeSideSell:
		return feeSideSell
	default:
		return feeSideBoth
	}
}

func normalizeFeeBasis(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case feeBasisShare, "contract", "quantity":
		return feeBasisShare
	case feeBasisOrder:
		return feeBasisOrder
	default:
		return feeBasisNotional
	}
}

func normalizeInstrumentType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case instrumentTypeETF, "fund":
		return instrumentTypeETF
	default:
		return instrumentTypeStock
	}
}

func normalizeAppliesTo(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeInstrumentType(value)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func cloneFeeRules(rules []FeeRule) []FeeRule {
	if len(rules) == 0 {
		return nil
	}
	cloned := make([]FeeRule, len(rules))
	copy(cloned, rules)
	for index := range cloned {
		cloned[index].AppliesTo = append([]string(nil), rules[index].AppliesTo...)
	}
	return cloned
}

func normalizeCostMarket(market, symbol string) string {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" {
		if dot := strings.Index(symbol, "."); dot > 0 {
			market = strings.ToUpper(strings.TrimSpace(symbol[:dot]))
		}
	}
	switch market {
	case "SH", "SZ", "CNSH", "CNSZ":
		return "CN"
	default:
		return market
	}
}

func defaultBrokerFeeSchedule(market, quoteCurrency string) FeeSchedule {
	switch market {
	case "HK":
		return FeeSchedule{
			Mode:     tradingCostModeMarketPreset,
			PresetID: "futu_hk_hk_stock_2026_06_30",
			Rules: []FeeRule{
				{
					ID: "futu_hk_hk_commission", Label: "Futu HK commission", Category: feeCategoryBroker,
					Side: feeSideBoth, Basis: feeBasisNotional, Rate: 0.0003, MinAmount: 3,
					Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF},
					EffectiveFrom: "2026-06-30", SourceURL: futuHKFeesSourceURL,
				},
				{
					ID: "futu_hk_hk_platform_fee", Label: "Futu HK platform fee", Category: feeCategoryBroker,
					Side: feeSideBoth, Basis: feeBasisOrder, FixedAmount: 15,
					Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF},
					EffectiveFrom: "2026-06-30", SourceURL: futuHKFeesSourceURL,
				},
			},
		}
	case "US":
		return FeeSchedule{
			Mode:     tradingCostModeMarketPreset,
			PresetID: "futu_hk_us_stock_2026_06_30",
			Rules: []FeeRule{
				{
					ID: "futu_hk_us_commission", Label: "Futu HK US commission", Category: feeCategoryBroker,
					Side: feeSideBoth, Basis: feeBasisShare, FixedAmount: 0.0049, MinAmount: 0.99, MaxRate: 0.005,
					Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF},
					EffectiveFrom: "2026-06-30", SourceURL: futuUSFeesSourceURL,
				},
				{
					ID: "futu_hk_us_platform_fee", Label: "Futu HK US platform fee", Category: feeCategoryBroker,
					Side: feeSideBoth, Basis: feeBasisShare, FixedAmount: 0.005, MinAmount: 1, MaxRate: 0.005,
					Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF},
					EffectiveFrom: "2026-06-30", SourceURL: futuUSFeesSourceURL,
				},
			},
		}
	default:
		return FeeSchedule{Mode: tradingCostModeNone}
	}
}

func defaultMarketFeeSchedule(market, quoteCurrency, instrumentType string) FeeSchedule {
	switch market {
	case "HK":
		return FeeSchedule{
			Mode:     tradingCostModeMarketPreset,
			PresetID: "hkex_hk_stock_2026_06_30",
			Rules: []FeeRule{
				{ID: "hkex_hk_settlement_fee", Label: "HKEX settlement fee", Category: feeCategoryClearing, Side: feeSideBoth, Basis: feeBasisNotional, Rate: 0.000042, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF}, EffectiveFrom: "2026-06-30", SourceURL: hkexHKFeesSourceURL},
				{ID: "hkex_hk_trading_fee", Label: "HKEX trading fee", Category: feeCategoryExchange, Side: feeSideBoth, Basis: feeBasisNotional, Rate: 0.0000565, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF}, EffectiveFrom: "2026-06-30", SourceURL: hkexHKFeesSourceURL},
				{ID: "sfc_hk_transaction_levy", Label: "SFC transaction levy", Category: feeCategoryRegulatory, Side: feeSideBoth, Basis: feeBasisNotional, Rate: 0.000027, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF}, EffectiveFrom: "2026-06-30", SourceURL: hkexHKFeesSourceURL},
				{ID: "afrc_hk_transaction_levy", Label: "AFRC transaction levy", Category: feeCategoryRegulatory, Side: feeSideBoth, Basis: feeBasisNotional, Rate: 0.0000015, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF}, EffectiveFrom: "2026-06-30", SourceURL: hkexHKFeesSourceURL},
				{ID: "hk_stamp_duty", Label: "Hong Kong stamp duty", Category: feeCategoryTax, Side: feeSideBoth, Basis: feeBasisNotional, Rate: 0.001, Rounding: "ceil_currency_unit", Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock}, EffectiveFrom: "2026-06-30", SourceURL: hkexHKFeesSourceURL},
			},
		}
	case "US":
		return FeeSchedule{
			Mode:     tradingCostModeMarketPreset,
			PresetID: "us_stock_market_fees_2026_06_30",
			Rules: []FeeRule{
				{ID: "us_clearing_fee", Label: "US clearing fee", Category: feeCategoryClearing, Side: feeSideBoth, Basis: feeBasisShare, FixedAmount: 0.003, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF}, EffectiveFrom: "2026-06-30", SourceURL: futuUSFeesSourceURL},
				{ID: "sec_section_31_fee", Label: "SEC Section 31 fee", Category: feeCategoryRegulatory, Side: feeSideSell, Basis: feeBasisNotional, Rate: 20.60 / 1000000, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF}, EffectiveFrom: "2026-06-30", SourceURL: secFeeRateSourceURL},
				{ID: "finra_taf", Label: "FINRA TAF", Category: feeCategoryRegulatory, Side: feeSideSell, Basis: feeBasisShare, FixedAmount: 0.000195, MinAmount: 0.01, MaxAmount: 9.79, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF}, EffectiveFrom: "2026-06-30", SourceURL: finraTAFSourceURL},
				{ID: "cat_fee", Label: "CAT fee", Category: feeCategoryRegulatory, Side: feeSideBoth, Basis: feeBasisShare, FixedAmount: 0.000003, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock, instrumentTypeETF}, EffectiveFrom: "2026-06-30", SourceURL: finraTAFSourceURL},
			},
		}
	case "CN":
		if instrumentType == instrumentTypeETF {
			return FeeSchedule{
				Mode:     tradingCostModeMarketPreset,
				PresetID: "stock_connect_etf_market_fees_2026_06_30",
				Rules: []FeeRule{
					{ID: "stock_connect_handling_fee_etf", Label: "Stock Connect handling fee", Category: feeCategoryExchange, Side: feeSideBoth, Basis: feeBasisNotional, Rate: 0.0000341, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeETF}, EffectiveFrom: "2026-06-30", SourceURL: hkexStockConnectFeesSourceURL},
					{ID: "stock_connect_transfer_fee_etf", Label: "Stock Connect transfer fee", Category: feeCategoryClearing, Side: feeSideBoth, Basis: feeBasisNotional, Rate: 0.00001, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeETF}, EffectiveFrom: "2026-06-30", SourceURL: hkexStockConnectFeesSourceURL},
				},
			}
		}
		return FeeSchedule{
			Mode:     tradingCostModeMarketPreset,
			PresetID: "stock_connect_a_share_market_fees_2026_06_30",
			Rules: []FeeRule{
				{ID: "stock_connect_handling_fee", Label: "Stock Connect handling fee", Category: feeCategoryExchange, Side: feeSideBoth, Basis: feeBasisNotional, Rate: 0.0000341, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock}, EffectiveFrom: "2026-06-30", SourceURL: hkexStockConnectFeesSourceURL},
				{ID: "stock_connect_securities_management_fee", Label: "Securities management fee", Category: feeCategoryRegulatory, Side: feeSideBoth, Basis: feeBasisNotional, Rate: 0.00002, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock}, EffectiveFrom: "2026-06-30", SourceURL: hkexStockConnectFeesSourceURL},
				{ID: "stock_connect_transfer_fee", Label: "Stock Connect transfer fee", Category: feeCategoryClearing, Side: feeSideBoth, Basis: feeBasisNotional, Rate: 0.00001, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock}, EffectiveFrom: "2026-06-30", SourceURL: hkexStockConnectFeesSourceURL},
				{ID: "cn_stamp_duty", Label: "China stamp duty", Category: feeCategoryTax, Side: feeSideSell, Basis: feeBasisNotional, Rate: 0.0005, Currency: quoteCurrency, AppliesTo: []string{instrumentTypeStock}, EffectiveFrom: "2026-06-30", SourceURL: cnStampDutySourceURL},
			},
		}
	default:
		return FeeSchedule{Mode: tradingCostModeNone}
	}
}

func pineBrokerFeeSchedule(metadata strategyir.StrategyMetadata, quoteCurrency string) FeeSchedule {
	if metadata.CommissionValue <= 0 {
		return FeeSchedule{Mode: tradingCostModeNone}
	}
	rule := FeeRule{
		ID: "pine_strategy_commission", Label: "Pine strategy commission", Category: feeCategoryBroker,
		Side: feeSideBoth, Currency: strings.ToUpper(strings.TrimSpace(quoteCurrency)),
		AppliesTo: []string{instrumentTypeStock, instrumentTypeETF},
	}
	switch metadata.CommissionType {
	case "percent":
		rule.Basis = feeBasisNotional
		rule.Rate = metadata.CommissionValue / 100
	case "cash_per_order":
		rule.Basis = feeBasisOrder
		rule.FixedAmount = metadata.CommissionValue
	case "cash_per_contract":
		rule.Basis = feeBasisShare
		rule.FixedAmount = metadata.CommissionValue
	default:
		return FeeSchedule{Mode: tradingCostModeNone}
	}
	return FeeSchedule{Mode: tradingCostModeScript, PresetID: "pine_strategy_commission", Rules: []FeeRule{rule}}
}

type appliedTradeFees struct {
	BrokerFee   float64
	MarketFee   float64
	TotalFee    float64
	FeeCurrency string
}

type feeRuleAccumulator struct {
	raw       float64
	notional  float64
	charged   float64
	orderUsed bool
}

type feeBreakdownAccumulator struct {
	entry FeeBreakdownEntry
}

type backtestFeeEngine struct {
	account        *types.Account
	quoteCurrency  string
	instrumentType string
	costs          TradingCosts
	result         *RunResult
	onApplied      func(types.Trade, appliedTradeFees)

	mu        sync.Mutex
	orders    map[string]*feeRuleAccumulator
	breakdown map[string]*feeBreakdownAccumulator
}

func newBacktestFeeEngine(account *types.Account, quoteCurrency, instrumentType string, costs TradingCosts, result *RunResult, onApplied func(types.Trade, appliedTradeFees)) *backtestFeeEngine {
	return &backtestFeeEngine{
		account:        account,
		quoteCurrency:  strings.ToUpper(strings.TrimSpace(quoteCurrency)),
		instrumentType: normalizeInstrumentType(instrumentType),
		costs:          costs,
		result:         result,
		onApplied:      onApplied,
		orders:         map[string]*feeRuleAccumulator{},
		breakdown:      map[string]*feeBreakdownAccumulator{},
	}
}

func (engine *backtestFeeEngine) onTradeUpdate(trade types.Trade) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	fees := appliedTradeFees{FeeCurrency: engine.quoteCurrency}
	fees.BrokerFee = engine.applyScheduleLocked(feeGroupBroker, engine.costs.BrokerFees, trade)
	fees.MarketFee = engine.applyScheduleLocked(feeGroupMarket, engine.costs.MarketFees, trade)
	fees.TotalFee = fees.BrokerFee + fees.MarketFee
	if fees.TotalFee > 0 && engine.account != nil && engine.quoteCurrency != "" {
		engine.account.AddBalance(engine.quoteCurrency, fixedpoint.NewFromFloat(fees.TotalFee).Neg())
	}
	if engine.result != nil && fees.TotalFee > 0 {
		engine.result.TotalBrokerFees += fees.BrokerFee
		engine.result.TotalMarketFees += fees.MarketFee
		engine.result.TotalFees += fees.TotalFee
	}
	engine.mu.Unlock()

	if engine.onApplied != nil && fees.TotalFee > 0 {
		engine.onApplied(trade, fees)
	}
}

func (engine *backtestFeeEngine) applyScheduleLocked(group string, schedule FeeSchedule, trade types.Trade) float64 {
	total := 0.0
	for _, rule := range schedule.Rules {
		fee, ok := engine.applyRuleLocked(group, rule, trade)
		if !ok || fee <= 0 {
			continue
		}
		total += fee
		engine.addBreakdownLocked(group, rule, fee)
	}
	return total
}

func (engine *backtestFeeEngine) applyRuleLocked(group string, rule FeeRule, trade types.Trade) (float64, bool) {
	if !feeRuleAppliesAt(rule, trade.Time.Time()) || !feeRuleAppliesToSide(rule, trade) || !feeRuleAppliesToInstrument(rule, engine.instrumentType) {
		return 0, false
	}
	notional := trade.Price.Float64() * trade.Quantity.Float64()
	quantity := trade.Quantity.Float64()
	if notional <= 0 || quantity <= 0 {
		return 0, false
	}

	key := feeOrderRuleKey(group, rule, trade)
	accumulator := engine.orders[key]
	if accumulator == nil {
		accumulator = &feeRuleAccumulator{}
		engine.orders[key] = accumulator
	}

	raw := 0.0
	switch rule.Basis {
	case feeBasisShare:
		raw = quantity*rule.FixedAmount + quantity*rule.Rate
	case feeBasisOrder:
		if accumulator.orderUsed {
			raw = 0
		} else {
			raw = rule.FixedAmount
			accumulator.orderUsed = true
		}
	default:
		raw = notional*rule.Rate + rule.FixedAmount
	}
	if raw <= 0 {
		return 0, false
	}

	accumulator.raw += raw
	accumulator.notional += notional
	target := accumulator.raw
	if rule.MinAmount > 0 && target < rule.MinAmount {
		target = rule.MinAmount
	}
	if capAmount := ruleCapAmount(rule, accumulator.notional); capAmount > 0 && target > capAmount {
		target = capAmount
	}
	target = roundedFeeAmount(target, rule.Rounding)
	incremental := target - accumulator.charged
	if incremental <= 0.0000000001 {
		return 0, false
	}
	accumulator.charged = target
	return incremental, true
}

const feeRuleDateLayout = "2006-01-02"

func feeRuleAppliesAt(rule FeeRule, at time.Time) bool {
	fromText := strings.TrimSpace(rule.EffectiveFrom)
	toText := strings.TrimSpace(rule.EffectiveTo)
	if fromText == "" && toText == "" {
		return true
	}
	if at.IsZero() {
		return false
	}
	tradeDate := at.Format(feeRuleDateLayout)
	if fromText != "" {
		if _, err := time.Parse(feeRuleDateLayout, fromText); err != nil || tradeDate < fromText {
			return false
		}
	}
	if toText != "" {
		if _, err := time.Parse(feeRuleDateLayout, toText); err != nil || tradeDate > toText {
			return false
		}
	}
	return fromText == "" || toText == "" || fromText <= toText
}

func feeRuleAppliesToSide(rule FeeRule, trade types.Trade) bool {
	side := rule.Side
	if side == "" || side == feeSideBoth {
		return true
	}
	if trade.Side == types.SideTypeBuy {
		return side == feeSideBuy
	}
	if trade.Side == types.SideTypeSell {
		return side == feeSideSell
	}
	return false
}

func feeRuleAppliesToInstrument(rule FeeRule, instrumentType string) bool {
	if len(rule.AppliesTo) == 0 {
		return true
	}
	instrumentType = normalizeInstrumentType(instrumentType)
	for _, appliesTo := range rule.AppliesTo {
		if normalizeInstrumentType(appliesTo) == instrumentType {
			return true
		}
	}
	return false
}

func feeOrderRuleKey(group string, rule FeeRule, trade types.Trade) string {
	orderID := trade.OrderID
	if orderID == 0 {
		orderID = trade.ID
	}
	return group + "|" + rule.ID + "|" + strconv.FormatUint(orderID, 10)
}

func ruleCapAmount(rule FeeRule, notional float64) float64 {
	capAmount := 0.0
	if rule.MaxAmount > 0 {
		capAmount = rule.MaxAmount
	}
	if rule.MaxRate > 0 && notional > 0 {
		rateCap := notional * rule.MaxRate
		if capAmount == 0 || rateCap < capAmount {
			capAmount = rateCap
		}
	}
	return capAmount
}

func roundedFeeAmount(amount float64, rounding string) float64 {
	switch strings.ToLower(strings.TrimSpace(rounding)) {
	case "ceil_currency_unit", "ceil_hkd":
		return math.Ceil(amount)
	case "ceil_cent":
		return math.Ceil(amount*100) / 100
	default:
		return amount
	}
}

func (engine *backtestFeeEngine) addBreakdownLocked(group string, rule FeeRule, amount float64) {
	if amount <= 0 || engine.result == nil {
		return
	}
	key := group + "|" + rule.ID
	entry := engine.breakdown[key]
	if entry == nil {
		entry = &feeBreakdownAccumulator{entry: FeeBreakdownEntry{
			RuleID:   rule.ID,
			Label:    rule.Label,
			Group:    group,
			Category: rule.Category,
			Currency: engine.quoteCurrency,
		}}
		engine.breakdown[key] = entry
	}
	entry.entry.Amount += amount
	entry.entry.Count++
}

func (engine *backtestFeeEngine) finalize() {
	if engine == nil || engine.result == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	breakdown := make([]FeeBreakdownEntry, 0, len(engine.breakdown))
	for _, item := range engine.breakdown {
		breakdown = append(breakdown, item.entry)
	}
	sort.Slice(breakdown, func(i, j int) bool {
		if breakdown[i].Group != breakdown[j].Group {
			return breakdown[i].Group < breakdown[j].Group
		}
		return breakdown[i].RuleID < breakdown[j].RuleID
	})
	engine.result.FeeBreakdown = breakdown
	engine.result.TradingCosts = engine.costs
}

func disableBacktestNativeFeeRates(config *bbgo2.Backtest) {
	if config == nil {
		return
	}
	for session, account := range config.Accounts {
		account.MakerFeeRate = fixedpoint.Zero
		account.TakerFeeRate = fixedpoint.Zero
		config.Accounts[session] = account
	}
}
