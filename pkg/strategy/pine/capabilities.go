package pine

import (
	"math"
	"strings"
)

const CompatibilityScoreModelVersion = "closed-bar-strategy-v1.5"

type CapabilityStatus string

const (
	CapabilitySupported   CapabilityStatus = "supported"
	CapabilityPartial     CapabilityStatus = "partial"
	CapabilityWarning     CapabilityStatus = "warning"
	CapabilityUnsupported CapabilityStatus = "unsupported"
)

type CapabilityLayers struct {
	Parser   bool `json:"parser"`
	Planner  bool `json:"planner"`
	Runtime  bool `json:"runtime"`
	Backtest bool `json:"backtest"`
	Frontend bool `json:"frontend"`
	Spec     bool `json:"spec"`
}

type Capability struct {
	ID        string           `json:"id"`
	Dimension string           `json:"dimension"`
	Status    CapabilityStatus `json:"status"`
	Weight    float64          `json:"weight"`
	Layers    CapabilityLayers `json:"layers"`
	TestIDs   []string         `json:"testIds"`
	Notes     string           `json:"notes,omitempty"`
}

type CompatibilityDimension struct {
	ID              string   `json:"id"`
	Weight          float64  `json:"weight"`
	Score           float64  `json:"score"`
	SupportedWeight float64  `json:"supportedWeight"`
	TotalWeight     float64  `json:"totalWeight"`
	UnsupportedIDs  []string `json:"unsupportedIds,omitempty"`
}

type CompatibilityAssessment struct {
	ScoreModelVersion string                   `json:"scoreModelVersion"`
	Score             float64                  `json:"score"`
	Dimensions        []CompatibilityDimension `json:"dimensions"`
	UnsupportedIDs    []string                 `json:"unsupportedIds"`
}

type capabilityDefinition struct {
	id     string
	status CapabilityStatus
	weight float64
	notes  string
}

var compatibilityDimensionWeights = map[string]float64{
	"language":   0.23,
	"indicators": 0.32,
	"mtf":        0.25,
	"orders":     0.10,
	"tooling":    0.10,
}

var capabilityDefinitions = []capabilityDefinition{
	supportedCapability("metadata.version6"),
	supportedCapability("metadata.strategy"),
	supportedCapability("metadata.backtest_costs"),
	supportedCapability("metadata.process_orders_on_close"),
	supportedCapability("syntax.if_else"),
	supportedCapability("syntax.assignment"),
	supportedCapability("syntax.var"),
	supportedCapability("syntax.const_varip"),
	supportedCapability("syntax.reassign"),
	supportedCapability("syntax.udf_expression"),
	supportedCapability("syntax.udf_multistatement"),
	supportedCapability("syntax.switch_static_lowering"),
	supportedCapability("syntax.for_static_unroll"),
	supportedWeightedCapability("syntax.v15_loop_control_subset", 4),
	supportedCapability("expression.history_ref_1"),
	supportedCapability("expression.history_ref_n"),
	supportedCapability("expression.ternary"),
	supportedCapability("expression.na_nz"),
	supportedCapability("expression.strict_bool"),
	supportedCapability("expression.input_defaults"),
	supportedCapability("expression.input_symbol_session"),
	supportedCapability("expression.input_timeframe"),
	supportedCapability("expression.math_namespace"),
	supportedCapability("expression.math_avg_mintick"),
	supportedCapability("expression.string_namespace"),
	supportedCapability("expression.format_constants"),
	supportedCapability("expression.account_equity"),
	supportedCapability("expression.time_variables"),
	supportedCapability("expression.derived_sources"),
	supportedCapability("expression.timestamp"),
	supportedCapability("expression.barstate_session"),
	supportedCapability("expression.pine_constants"),
	supportedWeightedCapability("expression.v14_direct_indicator_calls", 3),
	supportedWeightedCapability("expression.v14_security_source_calls", 2),
	supportedCapability("indicator.ma"),
	supportedCapability("indicator.ma_source_aware"),
	supportedCapability("indicator.source_aware_core"),
	supportedCapability("indicator.rsi"),
	supportedCapability("indicator.macd"),
	supportedCapability("indicator.atr"),
	supportedCapability("indicator.cci"),
	supportedCapability("indicator.bollinger"),
	supportedCapability("indicator.williams_r"),
	supportedCapability("indicator.rolling_window"),
	supportedCapability("indicator.sum"),
	supportedCapability("indicator.cross"),
	supportedCapability("indicator.cum_stoch_extrema_bars"),
	supportedCapability("indicator.vwap_mfi_dmi_supertrend"),
	supportedCapability("indicator.stateful_events"),
	supportedCapability("indicator.sar"),
	supportedCapability("indicator.linreg_obv_pivots"),
	supportedCapability("indicator.keltner_alma"),
	supportedCapability("indicator.v13_migration_set"),
	supportedWeightedCapability("indicator.v14_window_momentum_set", 4),
	supportedWeightedCapability("indicator.v14_stateful_events", 3),
	supportedWeightedCapability("indicator.true_range", 2),
	supportedWeightedCapability("indicator.v15_common_ta_set", 12),
	supportedCapability("request.security.mtf_ma_subset"),
	supportedCapability("request.security.mtf_sources"),
	supportedCapability("request.security.mtf_ma_source_aware"),
	supportedCapability("request.security.timeframe_multipliers"),
	supportedCapability("request.security.htf_history"),
	supportedCapability("request.security.mtf_v12_advanced"),
	supportedCapability("request.security.mtf_v13_advanced"),
	supportedWeightedCapability("request.security.pure_expression", 8),
	supportedWeightedCapability("request.security.pure_expression_diagnostics", 2),
	supportedWeightedCapability("request.security.v15_common_ta_expression", 28),
	supportedCapability("expression.barmerge_constants"),
	warningCapability("visual.noop_calls", "plot/drawing/table 等视觉 API 解析为 warning/no-op。"),
	warningCapability("alert.alertcondition_noop", "alertcondition 解析为 warning/no-op，交易告警使用 order alert metadata。"),
	supportedCapability("order.strategy_order_net"),
	supportedCapability("order.qty_percent"),
	supportedCapability("order.close_all"),
	supportedCapability("order.close_immediately"),
	supportedCapability("order.comment_alert_metadata"),
	supportedCapability("order.exit_quantity"),
	supportedCapability("order.exit_bracket"),
	supportedCapability("order.exit_price_expressions"),
	supportedCapability("order.trailing_exit"),
	supportedCapability("order.pending_stop"),
	supportedCapability("order.pending_stop_limit"),
	supportedCapability("order.cancel_pending"),
	supportedCapability("order.entry_reversal"),
	supportedCapability("order.allow_entry_in"),
	supportedCapability("strategy.entry_close_exit_subset"),
	partialCapability("order.short_broker_accounting", 1.8, "Pine runtime 计算反手数量；当前 JFTrade 现货回测执行器仍不模拟保证金裸空。"),
	unsupportedCapability("syntax.arrays_maps_matrices", 2.2, "array/map/matrix 暂不支持。"),
	unsupportedCapability("syntax.methods_types_libraries", 2.0, "method/type/library/import 暂不支持。"),
	unsupportedCapability("syntax.dynamic_loops_while", 2.0, "while、动态 for、break/continue 暂不支持。"),
	unsupportedCapability("syntax.recursive_nested_udf", 1.3, "递归和嵌套 UDF 暂不支持。"),
	unsupportedCapability("expression.tuple_general", 1.0, "仅特定内建三元组支持；通用 tuple/array 表达式暂不支持。"),
	unsupportedCapability("indicator.full_ta_surface", 3.2, "未覆盖 TradingView Pine v6 全部 ta.* 表面。"),
	partialCapability("indicator.v13_mtf_intraday_only", 1.4, "v1.3 新增 MTF 指标限制为同标的静态 intraday timeframe。"),
	unsupportedCapability("indicator.visual_only_plot_stack", 1.0, "视觉指标 API 继续 no-op。"),
	unsupportedCapability("request.security.dynamic_symbol_timeframe", 1.2, "动态 symbol/timeframe 暂不支持。"),
	unsupportedCapability("request.security.lookahead_gaps_on", 0.8, "lookahead_on/gaps_on 暂不支持。"),
	unsupportedCapability("request.security.tuple_general", 0.8, "通用 tuple request.security 暂不支持。"),
	unsupportedCapability("order.oca_partial_fill", 2.2, "OCA、partial fill 和完整 broker emulator 暂不支持。"),
	unsupportedCapability("order.intrabar_tick_recalc", 1.7, "tick 级重算和 intrabar 路径推断暂不支持。"),
	unsupportedCapability("order.full_tv_broker_emulator", 1.4, "完整 TradingView broker emulator 不属于当前目标。"),
	partialCapability("tooling.visual_builder_roundtrip", 0.6, "无法映射的新语法保留为 pineSnippet，不扩张为完整 Pine IDE。"),
	supportedWeightedCapability("tooling.migration_corpus_v14", 4),
	supportedWeightedCapability("tooling.migration_corpus_v15", 6),
}

func supportedCapability(id string) capabilityDefinition {
	return capabilityDefinition{id: id, status: CapabilitySupported, weight: 1}
}

func supportedWeightedCapability(id string, weight float64) capabilityDefinition {
	return capabilityDefinition{id: id, status: CapabilitySupported, weight: weight}
}

func warningCapability(id string, notes string) capabilityDefinition {
	return capabilityDefinition{id: id, status: CapabilityWarning, weight: 1, notes: notes}
}

func partialCapability(id string, weight float64, notes string) capabilityDefinition {
	return capabilityDefinition{id: id, status: CapabilityPartial, weight: weight, notes: notes}
}

func unsupportedCapability(id string, weight float64, notes string) capabilityDefinition {
	return capabilityDefinition{id: id, status: CapabilityUnsupported, weight: weight, notes: notes}
}

func CapabilityRegistry() []Capability {
	out := make([]Capability, 0, len(capabilityDefinitions))
	for _, definition := range capabilityDefinitions {
		out = append(out, Capability{
			ID:        definition.id,
			Dimension: capabilityDimension(definition.id),
			Status:    definition.status,
			Weight:    definition.weight,
			Layers:    capabilityLayers(definition),
			TestIDs:   capabilityTestIDs(definition.id),
			Notes:     definition.notes,
		})
	}
	return out
}

func CompatibilityScore() CompatibilityAssessment {
	registry := CapabilityRegistry()
	dimensionOrder := []string{"language", "indicators", "mtf", "orders", "tooling"}
	totals := map[string]float64{}
	supported := map[string]float64{}
	unsupportedByDimension := map[string][]string{}
	unsupportedIDs := make([]string, 0)
	for _, capability := range registry {
		dimension := capability.Dimension
		weight := capability.Weight
		if weight <= 0 {
			weight = 1
		}
		totals[dimension] += weight
		supported[dimension] += weight * capabilityStatusValue(capability.Status)
		if capability.Status == CapabilityUnsupported {
			unsupportedByDimension[dimension] = append(unsupportedByDimension[dimension], capability.ID)
			unsupportedIDs = append(unsupportedIDs, capability.ID)
		}
	}

	dimensions := make([]CompatibilityDimension, 0, len(dimensionOrder))
	score := 0.0
	for _, id := range dimensionOrder {
		total := totals[id]
		dimensionScore := 0.0
		if total > 0 {
			dimensionScore = roundScore(supported[id] / total * 100)
		}
		weight := compatibilityDimensionWeights[id]
		score += weight * dimensionScore
		dimensions = append(dimensions, CompatibilityDimension{
			ID:              id,
			Weight:          weight,
			Score:           dimensionScore,
			SupportedWeight: roundScore(supported[id]),
			TotalWeight:     roundScore(total),
			UnsupportedIDs:  unsupportedByDimension[id],
		})
	}
	return CompatibilityAssessment{
		ScoreModelVersion: CompatibilityScoreModelVersion,
		Score:             roundScore(score),
		Dimensions:        dimensions,
		UnsupportedIDs:    unsupportedIDs,
	}
}

func capabilityStatusValue(status CapabilityStatus) float64 {
	switch status {
	case CapabilitySupported:
		return 1
	case CapabilityPartial:
		return 0.5
	case CapabilityWarning:
		return 0.35
	default:
		return 0
	}
}

func roundScore(value float64) float64 {
	return math.Round(value*100) / 100
}

func capabilityLayers(definition capabilityDefinition) CapabilityLayers {
	layers := CapabilityLayers{Spec: true}
	switch definition.status {
	case CapabilitySupported:
		layers.Parser = true
		layers.Planner = true
		layers.Runtime = true
		layers.Backtest = true
	case CapabilityPartial:
		layers.Parser = true
		layers.Planner = true
		layers.Runtime = true
	case CapabilityWarning:
		layers.Parser = true
		layers.Runtime = true
	default:
		return layers
	}
	if capabilityHasFrontend(definition.id) {
		layers.Frontend = true
	}
	return layers
}

func capabilityHasFrontend(id string) bool {
	switch id {
	case "indicator.ma", "indicator.rsi", "indicator.macd", "indicator.atr",
		"indicator.cci", "indicator.bollinger", "indicator.williams_r",
		"indicator.linreg_obv_pivots", "indicator.keltner_alma",
		"indicator.v13_migration_set", "indicator.v14_window_momentum_set",
		"indicator.v14_stateful_events", "indicator.true_range", "indicator.v15_common_ta_set",
		"expression.v14_direct_indicator_calls", "expression.v14_security_source_calls",
		"syntax.udf_multistatement", "syntax.v15_loop_control_subset",
		"syntax.switch_static_lowering", "request.security.mtf_ma_subset",
		"request.security.mtf_v12_advanced", "request.security.mtf_v13_advanced",
		"request.security.pure_expression", "request.security.pure_expression_diagnostics",
		"request.security.v15_common_ta_expression",
		"order.qty_percent", "order.exit_bracket", "order.pending_stop",
		"order.pending_stop_limit", "order.trailing_exit", "order.allow_entry_in",
		"order.entry_reversal":
		return true
	default:
		return strings.HasPrefix(id, "metadata.") || strings.HasPrefix(id, "tooling.")
	}
}

func capabilityDimension(id string) string {
	switch {
	case strings.HasPrefix(id, "indicator."):
		return "indicators"
	case strings.HasPrefix(id, "request.security."):
		return "mtf"
	case strings.HasPrefix(id, "order."), strings.HasPrefix(id, "strategy."):
		return "orders"
	case strings.HasPrefix(id, "visual."), strings.HasPrefix(id, "alert."), strings.HasPrefix(id, "metadata."), strings.HasPrefix(id, "tooling."):
		return "tooling"
	default:
		return "language"
	}
}

func capabilityTestIDs(id string) []string {
	switch id {
	case "order.trailing_exit":
		return []string{"TestCompileSupportsStrategyExitSubset", "TestRunPinePendingStopCancelAndBracketExit/trailing_points_closes_position", "TestRunPinePendingStopCancelAndBracketExit/trailing_price_closes_position"}
	case "order.pending_stop_limit":
		return []string{"TestCompileSupportsPendingStopAndCancelOrders", "TestRunPinePendingStopCancelAndBracketExit/stop-limit_activates_before_limit_fill"}
	case "order.entry_reversal", "order.allow_entry_in":
		return []string{"TestCompileSupportsAllowEntryInRiskDeclaration", "TestAdjustEntryOrderQuantitySupportsPineReversalAndAllowEntryIn", "TestRunPineEntryReversalAndAllowedEntryDirection"}
	case "indicator.linreg_obv_pivots", "indicator.keltner_alma":
		return []string{"TestCompileSupportsV12AdvancedIndicators", "TestAdvancedIndicatorCalculationsUseAuditedVectors"}
	case "indicator.v13_migration_set":
		return []string{"TestCompileSupportsV13MigrationIndicators", "TestAdvancedIndicatorCalculationsUseAuditedVectors", "TestIndicatorRuntimeSnapshotIncludesV13MigrationIndicators"}
	case "expression.v14_direct_indicator_calls", "expression.v14_security_source_calls":
		return []string{"TestCompileSupportsV14RequestSecurityPureExpression", "TestEvaluateExpressionSupportsNewIndicatorLookups"}
	case "indicator.v14_window_momentum_set":
		return []string{"TestCompileSupportsV14WindowMomentumAndStatefulIndicators", "TestIndicatorRuntimeSnapshotIncludesWindowAndSourceAwareIndicators"}
	case "indicator.v14_stateful_events":
		return []string{"TestCompileSupportsV14WindowMomentumAndStatefulIndicators", "TestEvaluateExpressionSupportsBarsSinceAndValueWhenState", "TestPineV14MigrationCorpusGate"}
	case "indicator.true_range":
		return []string{"TestCompileSupportsV14WindowMomentumAndStatefulIndicators", "TestEvaluateExpressionSupportsDerivedSourcesEnvironmentTimestampAndTR"}
	case "indicator.v15_common_ta_set":
		return []string{"TestCompileSupportsV15RequestSecurityCommonTAExpression", "TestEvaluateExpressionSupportsNewIndicatorLookups", "TestPineV15MigrationCorpusGate"}
	case "syntax.v15_loop_control_subset":
		return []string{"TestCompileSupportsV15StaticForLoopControl", "TestPineV15MigrationCorpusGate"}
	case "syntax.udf_multistatement", "syntax.switch_static_lowering":
		return []string{"TestCompileSupportsSwitchAndMultiStatementUDF"}
	case "request.security.mtf_ma_subset", "request.security.mtf_v12_advanced":
		return []string{"TestCompileSupportsV12AdvancedIndicatorsInStaticIntradaySecurity", "TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes"}
	case "request.security.mtf_v13_advanced":
		return []string{"TestCompileSupportsV13IndicatorsInStaticIntradaySecurity", "TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes"}
	case "request.security.pure_expression":
		return []string{"TestCompileSupportsV14RequestSecurityPureExpression", "TestEvaluateExpressionSupportsNewIndicatorLookups", "TestPineV14MigrationCorpusGate"}
	case "request.security.pure_expression_diagnostics":
		return []string{"TestValidateScriptRejectsUnsupportedPineRuntimeFeature", "TestPineV14MigrationCorpusGate"}
	case "request.security.v15_common_ta_expression":
		return []string{"TestCompileSupportsV15RequestSecurityCommonTAExpression", "TestEvaluateExpressionSupportsNewIndicatorLookups", "TestPineV15MigrationCorpusGate"}
	case "expression.math_avg_mintick":
		return []string{"TestCompileSupportsV13MigrationIndicators", "TestEvaluateExpressionSupportsMathAndTimeVariables"}
	case "tooling.migration_corpus_v14":
		return []string{"TestPineV14MigrationCorpusGate"}
	case "tooling.migration_corpus_v15":
		return []string{"TestPineV15MigrationCorpusGate"}
	default:
		if strings.Contains(id, "unsupported") || strings.HasPrefix(id, "syntax.arrays") {
			return nil
		}
		return []string{"TestGoldenExamplesAnalyzeAndPlan", "TestPineGoldenBenchmarkCasesSmoke"}
	}
}
