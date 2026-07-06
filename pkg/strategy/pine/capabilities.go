package pine

import (
	"math"
	"strings"
)

const CompatibilityScoreModelVersion = "closed-bar-strategy-v4.0"

type CapabilityStatus string

const (
	CapabilitySupported   CapabilityStatus = "supported"
	CapabilityPartial     CapabilityStatus = "partial"
	CapabilityAnalyzed    CapabilityStatus = "analyzed"
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
	"language":   0.12,
	"indicators": 0.30,
	"mtf":        0.48,
	"orders":     0.00,
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
	supportedWeightedCapability("syntax.v16_security_tuple_destructure", 12),
	supportedWeightedCapability("syntax.v17_ast_semantic_transition", 20),
	supportedWeightedCapability("syntax.v21_collection_runtime_core", 35),
	supportedWeightedCapability("syntax.v22_structured_loop_runtime", 22),
	supportedWeightedCapability("syntax.v22_pure_udt_method_runtime", 18),
	supportedWeightedCapability("syntax.v23_collection_api_expansion", 16),
	supportedWeightedCapability("syntax.v23_pure_method_body_named_args", 12),
	supportedWeightedCapability("syntax.v24_collection_api_expansion", 14),
	supportedWeightedCapability("syntax.v24_runtime_loop_fallback", 8),
	supportedWeightedCapability("syntax.v24_persistent_object_field_set", 8),
	supportedWeightedCapability("syntax.v25_array_stat_api", 12),
	supportedWeightedCapability("syntax.v26_collection_iteration", 14),
	supportedWeightedCapability("syntax.v26_collection_history_snapshot", 12),
	supportedWeightedCapability("syntax.v26_object_collection_fields", 14),
	supportedWeightedCapability("syntax.v26_library_export_metadata", 4),
	supportedWeightedCapability("syntax.v27_collection_history_aggregates", 8),
	supportedWeightedCapability("syntax.v27_map_matrix_iteration", 8),
	supportedWeightedCapability("syntax.v28_object_history_read", 8),
	supportedWeightedCapability("syntax.v28_method_chain", 8),
	supportedWeightedCapability("syntax.v28_export_metadata", 6),
	supportedWeightedCapability("syntax.v29_object_history_method_receiver", 8),
	supportedWeightedCapability("syntax.v29_method_chain_named_defaults", 6),
	supportedWeightedCapability("syntax.v29_request_security_diagnostics", 6),
	supportedWeightedCapability("syntax.v30_stable_semantic_declarations", 10),
	supportedWeightedCapability("syntax.v30_varip_closed_bar_policy", 4),
	supportedWeightedCapability("syntax.v30_parser_whitespace_comments", 4),
	supportedWeightedCapability("syntax.v31_public_surface_lock", 6),
	supportedWeightedCapability("syntax.v33_advanced_language_boundary", 6),
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
	supportedWeightedCapability("expression.v16_tuple_alias_member_lowering", 8),
	supportedWeightedCapability("expression.v17_semantic_scope_types", 15),
	supportedWeightedCapability("expression.v17_signature_diagnostics", 10),
	supportedWeightedCapability("expression.v22_general_tuple", 10),
	supportedWeightedCapability("expression.v23_object_field_set", 8),
	supportedWeightedCapability("expression.v25_string_helpers", 8),
	supportedWeightedCapability("expression.v25_timeframe_change", 8),
	supportedWeightedCapability("expression.v27_timeframe_helpers", 8),
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
	supportedWeightedCapability("indicator.v16_mtf_tuple_bindings", 12),
	supportedWeightedCapability("indicator.v17_source_aware_semantic_requirements", 15),
	supportedWeightedCapability("indicator.v21_bbw_cog_anchored_vwap", 35),
	supportedWeightedCapability("indicator.v24_mtf_stoch", 30),
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
	supportedWeightedCapability("request.security.v16_tuple_whitelist", 18),
	supportedWeightedCapability("request.security.v17_semantic_tuple_corpus", 20),
	supportedWeightedCapability("request.security.v21_ast_pure_expression", 62),
	supportedWeightedCapability("request.security.v22_general_tuple", 16),
	supportedWeightedCapability("request.security.v23_pure_collection_object_expression", 22),
	supportedWeightedCapability("request.security.v24_mtf_stoch", 20),
	supportedWeightedCapability("request.security.v27_pure_helper_expression", 10),
	supportedWeightedCapability("request.security.v28_object_method_expression", 10),
	supportedWeightedCapability("request.security.v29_object_history_expression", 8),
	supportedWeightedCapability("request.security.v32_diagnostic_matrix", 6),
	supportedWeightedCapability("request.security.v32_lower_timeframe_preflight", 6),
	supportedCapability("expression.barmerge_constants"),
	warningCapability("visual.noop_calls", "plot/drawing/table 等视觉 API 由 PineTS worker 归入 visual output；Go 交易链路不消费这些输出。"),
	warningCapability("alert.alertcondition_noop", "alertcondition 由 PineTS worker 归入 alerts；交易告警仍建议使用 alert() 或 order alert metadata 明确表达。"),
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
	supportedWeightedCapability("strategy.v40_broker_boundary_decision", 6),
	partialCapability("order.short_broker_accounting", 1.8, "Pine runtime 计算反手数量；当前 JFTrade 现货回测执行器仍不模拟保证金裸空。"),
	partialCapability("syntax.arrays_maps_matrices", 2.2, "array/map/matrix 常用 constructor、读取、变更、copy/slice/fill/aggregate、排序、统计、array.from/concat/join、map.copy/keys/values、array for-in、map keys/values iteration、matrix rows/columns/get/set 与 collection history aggregate snapshot 已可执行并跨 K 线持久化；深层泛型、嵌套 collection 全表面仍未覆盖。"),
	partialCapability("syntax.methods_types_libraries", 2.0, "type、命名 constructor 参数、多语句纯 method、局部/持久 object 字段重赋值、object collection fields、object history read、纯 method chain 与 export kind metadata 子集可执行/可分析；library/import 和完整 Pine method/type 系统仍只进入 semantic metadata 与诊断。"),
	partialCapability("syntax.dynamic_loops_while", 2.0, "动态 for、while、break/continue 由 PineTS worker 执行，限制嵌套深度和单 bar 最大迭代数以避免无限循环。"),
	unsupportedCapability("syntax.recursive_nested_udf", 1.3, "递归和嵌套 UDF 暂不支持。"),
	partialCapability("expression.tuple_general", 1.0, "通用 tuple literal/destructure 支持 2 到 8 个元素和 _ discard；完整 Pine tuple/array 互操作仍未覆盖。"),
	unsupportedCapability("indicator.full_ta_surface", 3.2, "未覆盖 TradingView Pine v6 全部 ta.* 表面。"),
	partialCapability("indicator.v13_mtf_intraday_only", 1.4, "v1.3 新增 MTF 指标限制为同标的静态 intraday timeframe。"),
	unsupportedCapability("indicator.visual_only_plot_stack", 1.0, "plot/hline/bgcolor/barcolor/fill 等视觉 API 由 PineTS worker 归入 visual output；Go 交易链路不消费这些输出。"),
	unsupportedCapability("request.security.dynamic_symbol_timeframe", 1.2, "动态 symbol/timeframe 暂不支持。"),
	unsupportedCapability("request.security.lookahead_gaps_on", 0.8, "lookahead_on/gaps_on 暂不支持。"),
	partialCapability("request.security.tuple_general", 0.8, "同标的静态 timeframe 下支持 2 到 8 元纯表达式 tuple；动态 symbol/timeframe、side effect、nested request 仍不支持。"),
	unsupportedCapability("order.oca_partial_fill", 2.2, "OCA、partial fill 和完整 broker emulator 暂不支持。"),
	unsupportedCapability("order.intrabar_tick_recalc", 1.7, "tick 级重算和 intrabar 路径推断暂不支持。"),
	unsupportedCapability("order.full_tv_broker_emulator", 1.4, "完整 TradingView broker emulator 不属于当前目标。"),
	partialCapability("tooling.visual_builder_roundtrip", 0.6, "流程图反解只覆盖可标准化 Pine v6 子集；无法映射的新语法返回行号诊断，请继续在 Pine 工作台编辑。"),
	supportedWeightedCapability("tooling.migration_corpus_v14", 4),
	supportedWeightedCapability("tooling.migration_corpus_v15", 6),
	supportedWeightedCapability("tooling.migration_corpus_v16", 8),
	supportedWeightedCapability("tooling.migration_corpus_v17", 10),
	supportedWeightedCapability("tooling.migration_corpus_v21", 25),
	supportedWeightedCapability("tooling.migration_corpus_v22", 30),
	supportedWeightedCapability("tooling.migration_corpus_v23", 35),
	supportedWeightedCapability("tooling.migration_corpus_v24", 40),
	supportedWeightedCapability("tooling.migration_corpus_v25", 45),
	supportedWeightedCapability("tooling.migration_corpus_v26", 50),
	supportedWeightedCapability("tooling.migration_corpus_v27", 55),
	supportedWeightedCapability("tooling.migration_corpus_v28", 60),
	supportedWeightedCapability("tooling.migration_corpus_v29", 65),
	supportedWeightedCapability("tooling.migration_corpus_v30", 70),
	supportedWeightedCapability("tooling.semantic_analyze_payload", 10),
	supportedWeightedCapability("tooling.visual_metadata_output", 4),
	supportedWeightedCapability("tooling.v20_language_foundation", 10),
	supportedWeightedCapability("tooling.v31_structured_helper_diagnostics", 8),
	supportedWeightedCapability("tooling.v33_structured_language_diagnostics", 6),
	supportedWeightedCapability("tooling.v34_generated_support_snapshot", 6),
	supportedWeightedCapability("tooling.v40_broker_boundary_snapshot", 6),
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
	case CapabilityAnalyzed:
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
	case CapabilityAnalyzed:
		layers.Parser = true
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
		"indicator.v16_mtf_tuple_bindings", "indicator.v17_source_aware_semantic_requirements",
		"indicator.v21_bbw_cog_anchored_vwap", "indicator.v24_mtf_stoch",
		"expression.v14_direct_indicator_calls", "expression.v14_security_source_calls",
		"expression.v16_tuple_alias_member_lowering", "expression.v17_semantic_scope_types",
		"expression.v17_signature_diagnostics", "expression.v22_general_tuple", "expression.v25_string_helpers", "expression.v25_timeframe_change", "expression.v27_timeframe_helpers",
		"syntax.udf_multistatement", "syntax.v15_loop_control_subset", "syntax.v16_security_tuple_destructure",
		"syntax.v17_ast_semantic_transition", "syntax.v21_collection_runtime_core", "syntax.v22_structured_loop_runtime",
		"syntax.v22_pure_udt_method_runtime", "syntax.v24_collection_api_expansion", "syntax.v24_runtime_loop_fallback", "syntax.v24_persistent_object_field_set", "syntax.arrays_maps_matrices", "syntax.methods_types_libraries", "syntax.dynamic_loops_while",
		"syntax.v25_array_stat_api", "syntax.v26_collection_iteration", "syntax.v26_collection_history_snapshot", "syntax.v26_object_collection_fields", "syntax.v26_library_export_metadata",
		"syntax.v27_collection_history_aggregates", "syntax.v27_map_matrix_iteration", "syntax.v28_object_history_read", "syntax.v28_method_chain", "syntax.v28_export_metadata",
		"syntax.v29_object_history_method_receiver", "syntax.v29_method_chain_named_defaults", "syntax.v29_request_security_diagnostics",
		"syntax.v30_stable_semantic_declarations", "syntax.v30_varip_closed_bar_policy", "syntax.v30_parser_whitespace_comments",
		"syntax.v31_public_surface_lock", "syntax.v33_advanced_language_boundary", "tooling.v31_structured_helper_diagnostics", "tooling.v33_structured_language_diagnostics",
		"syntax.switch_static_lowering", "request.security.mtf_ma_subset",
		"request.security.mtf_v12_advanced", "request.security.mtf_v13_advanced",
		"request.security.pure_expression", "request.security.pure_expression_diagnostics",
		"request.security.v15_common_ta_expression", "request.security.v16_tuple_whitelist",
		"request.security.v17_semantic_tuple_corpus", "request.security.v21_ast_pure_expression", "request.security.v22_general_tuple",
		"request.security.v23_pure_collection_object_expression", "request.security.v24_mtf_stoch", "request.security.v27_pure_helper_expression", "request.security.v28_object_method_expression", "request.security.v29_object_history_expression", "request.security.v32_diagnostic_matrix", "request.security.v32_lower_timeframe_preflight",
		"visual.noop_calls", "alert.alertcondition_noop",
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
	return existingCapabilityTestIDs(capabilityTestIDsRaw(id))
}

var capabilityTestIDEvidence = map[string][]string{
	"order.trailing_exit":                                    {"TestCompileSupportsStrategyExitSubset"},
	"order.pending_stop_limit":                               {"TestCompileSupportsPendingStopAndCancelOrders"},
	"order.entry_reversal":                                   {"TestCompileSupportsAllowEntryInRiskDeclaration"},
	"order.allow_entry_in":                                   {"TestCompileSupportsAllowEntryInRiskDeclaration"},
	"strategy.v40_broker_boundary_decision":                  {"TestAnalyzeScriptReportsV40BrokerBoundaryDiagnostics", "TestBuildToolPayloadIncludesBrokerBoundary", "TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.v40_broker_boundary_snapshot":                   {"TestAnalyzeScriptReportsV40BrokerBoundaryDiagnostics", "TestBuildToolPayloadIncludesBrokerBoundary", "TestGeneratedPineSupportSnapshotIsCurrent"},
	"indicator.linreg_obv_pivots":                            {"TestCompileSupportsV12AdvancedIndicators", "TestAdvancedIndicatorCalculationsUseAuditedVectors"},
	"indicator.keltner_alma":                                 {"TestCompileSupportsV12AdvancedIndicators", "TestAdvancedIndicatorCalculationsUseAuditedVectors"},
	"indicator.v13_migration_set":                            {"TestCompileSupportsV13MigrationIndicators", "TestAdvancedIndicatorCalculationsUseAuditedVectors", "TestIndicatorRuntimeSnapshotIncludesV13MigrationIndicators"},
	"expression.v14_direct_indicator_calls":                  {"TestCompileSupportsV14RequestSecurityPureExpression"},
	"expression.v14_security_source_calls":                   {"TestCompileSupportsV14RequestSecurityPureExpression"},
	"indicator.v14_window_momentum_set":                      {"TestCompileSupportsV14WindowMomentumAndStatefulIndicators"},
	"indicator.v14_stateful_events":                          {"TestCompileSupportsV14WindowMomentumAndStatefulIndicators"},
	"indicator.true_range":                                   {"TestCompileSupportsV14WindowMomentumAndStatefulIndicators"},
	"indicator.v15_common_ta_set":                            {"TestCompileSupportsV15RequestSecurityCommonTAExpression"},
	"indicator.v16_mtf_tuple_bindings":                       {"TestCompileSupportsV16RequestSecurityTupleWhitelist"},
	"expression.v16_tuple_alias_member_lowering":             {"TestCompileSupportsV16RequestSecurityTupleWhitelist"},
	"syntax.v16_security_tuple_destructure":                  {"TestCompileSupportsV16RequestSecurityTupleWhitelist"},
	"syntax.v17_ast_semantic_transition":                     {"TestAnalyzeScriptIncludesV17SemanticSummary", "TestAnalyzeScriptReportsSemanticSignatureDiagnostics"},
	"expression.v17_semantic_scope_types":                    {"TestAnalyzeScriptIncludesV17SemanticSummary", "TestAnalyzeScriptReportsSemanticSignatureDiagnostics"},
	"expression.v17_signature_diagnostics":                   {"TestAnalyzeScriptIncludesV17SemanticSummary", "TestAnalyzeScriptReportsSemanticSignatureDiagnostics"},
	"indicator.v17_source_aware_semantic_requirements":       {"TestAnalyzeScriptIncludesV17SemanticSummary", "TestAnalyzeScriptReportsSemanticSignatureDiagnostics"},
	"request.security.v17_semantic_tuple_corpus":             {"TestAnalyzeScriptIncludesV17SemanticSummary", "TestAnalyzeScriptReportsSemanticSignatureDiagnostics"},
	"syntax.v21_collection_runtime_core":                     {"TestCompileSupportsV21ExecutableCollectionCore"},
	"syntax.arrays_maps_matrices":                            {"TestCompileSupportsV21ExecutableCollectionCore"},
	"syntax.v23_collection_api_expansion":                    {"TestCompileSupportsV23RequestSecurityPureObjectAndCollectionExpressions"},
	"syntax.v24_collection_api_expansion":                    {"TestCompileSupportsV24CollectionExpansionAndMTFStoch"},
	"syntax.v24_runtime_loop_fallback":                       {"TestCompileSupportsV24NamedObjectMethodExpressionAndRuntimeLoopFallback"},
	"syntax.v24_persistent_object_field_set":                 {"TestCompileSupportsV23LocalObjectFieldReassignment"},
	"syntax.v25_array_stat_api":                              {"TestCompileSupportsV25ArrayStringAndTimeframeHelpers"},
	"expression.v25_string_helpers":                          {"TestCompileSupportsV25ArrayStringAndTimeframeHelpers"},
	"expression.v25_timeframe_change":                        {"TestCompileSupportsV25ArrayStringAndTimeframeHelpers"},
	"syntax.v26_collection_iteration":                        {"TestCompileSupportsV26CollectionIterationHistoryAndObjectCollectionFields"},
	"syntax.v26_collection_history_snapshot":                 {"TestCompileSupportsV26CollectionIterationHistoryAndObjectCollectionFields"},
	"syntax.v26_object_collection_fields":                    {"TestCompileSupportsV26CollectionIterationHistoryAndObjectCollectionFields"},
	"syntax.v26_library_export_metadata":                     {"TestPineV20LanguageFoundationGate"},
	"syntax.v27_collection_history_aggregates":               {"TestCompileSupportsV27CollectionTimeframeAndMTFHelpers"},
	"syntax.v27_map_matrix_iteration":                        {"TestCompileSupportsV27CollectionTimeframeAndMTFHelpers"},
	"expression.v27_timeframe_helpers":                       {"TestCompileSupportsV27CollectionTimeframeAndMTFHelpers"},
	"request.security.v27_pure_helper_expression":            {"TestCompileSupportsV27CollectionTimeframeAndMTFHelpers"},
	"syntax.v28_object_history_read":                         {"TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata"},
	"syntax.v28_method_chain":                                {"TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata"},
	"syntax.v28_export_metadata":                             {"TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata"},
	"request.security.v28_object_method_expression":          {"TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata"},
	"syntax.v29_object_history_method_receiver":              {"TestCompileSupportsV29ObjectHistoryMethodReceiverAndMTFHistoryExpression"},
	"syntax.v29_method_chain_named_defaults":                 {"TestCompileSupportsV29ObjectHistoryMethodReceiverAndMTFHistoryExpression"},
	"request.security.v29_object_history_expression":         {"TestCompileSupportsV29ObjectHistoryMethodReceiverAndMTFHistoryExpression"},
	"syntax.v29_request_security_diagnostics":                {"TestAnalyzeScriptReportsV29RequestSecurityDiagnostics"},
	"request.security.v32_diagnostic_matrix":                 {"TestAnalyzeScriptReportsV32RequestSecurityDiagnosticMatrix"},
	"request.security.v32_lower_timeframe_preflight":         {"TestRequestSecurityTimeframeRequirementsValidateAgainstStrategyInterval"},
	"syntax.v30_stable_semantic_declarations":                {"TestCompileSupportsV30SemanticDeclarationModelAndVaripPolicy"},
	"syntax.v30_varip_closed_bar_policy":                     {"TestCompileSupportsV30SemanticDeclarationModelAndVaripPolicy"},
	"syntax.v30_parser_whitespace_comments":                  {"TestCompileSupportsV30SemanticDeclarationModelAndVaripPolicy"},
	"syntax.v31_public_surface_lock":                         {"TestCompileRejectsPublicInternalHelperCalls", "TestAnalyzeScriptReportsPublicInternalHelperDiagnostics"},
	"tooling.v31_structured_helper_diagnostics":              {"TestCompileRejectsPublicInternalHelperCalls", "TestAnalyzeScriptReportsPublicInternalHelperDiagnostics"},
	"syntax.v33_advanced_language_boundary":                  {"TestAnalyzeScriptReportsV33AdvancedLanguageBoundaryDiagnostics", "TestValidateScriptReportsUnsupportedUDFAndStaticForCases"},
	"tooling.v33_structured_language_diagnostics":            {"TestAnalyzeScriptReportsV33AdvancedLanguageBoundaryDiagnostics", "TestValidateScriptReportsUnsupportedUDFAndStaticForCases"},
	"tooling.v34_generated_support_snapshot":                 {"TestGeneratedPineSupportSnapshotIsCurrent", "TestBuildToolPayloadIncludesSupportMatrix"},
	"indicator.v21_bbw_cog_anchored_vwap":                    {"TestCompileSupportsV21BBWAndCOG", "TestAdvancedIndicatorCalculationsUseAuditedVectors"},
	"request.security.v21_ast_pure_expression":               {"TestCompileSupportsV21BBWAndCOG", "TestAdvancedIndicatorCalculationsUseAuditedVectors"},
	"syntax.v22_structured_loop_runtime":                     {"TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops"},
	"syntax.dynamic_loops_while":                             {"TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops"},
	"expression.v22_general_tuple":                           {"TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops"},
	"request.security.v22_general_tuple":                     {"TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops"},
	"request.security.tuple_general":                         {"TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops"},
	"expression.tuple_general":                               {"TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops"},
	"syntax.v22_pure_udt_method_runtime":                     {"TestCompileSupportsV22PureUDTAndMethodSubset"},
	"syntax.v23_pure_method_body_named_args":                 {"TestCompileSupportsV23NamedObjectArgsAndPureMethodBody", "TestCompileSupportsV23LocalObjectFieldReassignment"},
	"expression.v23_object_field_set":                        {"TestCompileSupportsV23NamedObjectArgsAndPureMethodBody", "TestCompileSupportsV23LocalObjectFieldReassignment"},
	"syntax.v15_loop_control_subset":                         {"TestCompileSupportsV15StaticForLoopControl"},
	"syntax.udf_multistatement":                              {"TestCompileSupportsSwitchAndMultiStatementUDF"},
	"syntax.switch_static_lowering":                          {"TestCompileSupportsSwitchAndMultiStatementUDF"},
	"request.security.mtf_ma_subset":                         {"TestCompileSupportsV12AdvancedIndicatorsInStaticIntradaySecurity", "TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes"},
	"request.security.mtf_v12_advanced":                      {"TestCompileSupportsV12AdvancedIndicatorsInStaticIntradaySecurity", "TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes"},
	"request.security.mtf_v13_advanced":                      {"TestCompileSupportsV13IndicatorsInStaticIntradaySecurity", "TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes"},
	"request.security.pure_expression":                       {"TestCompileSupportsV14RequestSecurityPureExpression"},
	"request.security.pure_expression_diagnostics":           {"TestValidateScriptRejectsUnsupportedPineRuntimeFeature"},
	"request.security.v15_common_ta_expression":              {"TestCompileSupportsV15RequestSecurityCommonTAExpression"},
	"request.security.v16_tuple_whitelist":                   {"TestCompileSupportsV16RequestSecurityTupleWhitelist"},
	"request.security.v23_pure_collection_object_expression": {"TestCompileSupportsV23RequestSecurityPureObjectAndCollectionExpressions"},
	"indicator.v24_mtf_stoch":                                {"TestCompileSupportsV24CollectionExpansionAndMTFStoch", "TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes"},
	"request.security.v24_mtf_stoch":                         {"TestCompileSupportsV24CollectionExpansionAndMTFStoch", "TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes"},
	"expression.math_avg_mintick":                            {"TestCompileSupportsV13MigrationIndicators"},
	"tooling.migration_corpus_v14":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v15":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v16":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v17":                           {"TestGeneratedPineSupportSnapshotIsCurrent", "TestAnalyzeStrategyPineRouteReturnsDiagnosticsAndRequirements"},
	"tooling.semantic_analyze_payload":                       {"TestGeneratedPineSupportSnapshotIsCurrent", "TestAnalyzeStrategyPineRouteReturnsDiagnosticsAndRequirements"},
	"tooling.migration_corpus_v21":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v22":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v23":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v24":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v25":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v26":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v27":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v28":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v29":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.migration_corpus_v30":                           {"TestGeneratedPineSupportSnapshotIsCurrent"},
	"tooling.v20_language_foundation":                        {"TestPineV20LanguageFoundationGate", "TestAnalyzeStrategyPineRouteReturnsV20ParseOnlyMetadata"},
	"tooling.visual_metadata_output":                         {"TestPineV20LanguageFoundationGate", "TestAnalyzeStrategyPineRouteReturnsV20ParseOnlyMetadata"},
	"visual.noop_calls":                                      {"TestPineV20LanguageFoundationGate", "TestAnalyzeStrategyPineRouteReturnsV20ParseOnlyMetadata"},
	"syntax.methods_types_libraries":                         {"TestPineV20LanguageFoundationGate", "TestCompileSupportsV22PureUDTAndMethodSubset"},
}

func capabilityTestIDsRaw(id string) []string {
	if tests, ok := capabilityTestIDEvidence[id]; ok {
		return tests
	}
	if strings.Contains(id, "unsupported") || strings.HasPrefix(id, "syntax.arrays") {
		return nil
	}
	return []string{"TestGoldenExamplesAnalyzeAndPlan"}
}

func existingCapabilityTestIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if currentCapabilityEvidenceTests[id] {
			result = append(result, id)
		}
	}
	return result
}

var currentCapabilityEvidenceTests = map[string]bool{
	"TestAdvancedIndicatorCalculationsUseAuditedVectors":                        true,
	"TestAnalyzeScriptIncludesV17SemanticSummary":                               true,
	"TestAnalyzeScriptReportsPublicInternalHelperDiagnostics":                   true,
	"TestAnalyzeScriptReportsSemanticSignatureDiagnostics":                      true,
	"TestAnalyzeScriptReportsV29RequestSecurityDiagnostics":                     true,
	"TestAnalyzeScriptReportsV32RequestSecurityDiagnosticMatrix":                true,
	"TestAnalyzeScriptReportsV33AdvancedLanguageBoundaryDiagnostics":            true,
	"TestAnalyzeScriptReportsV40BrokerBoundaryDiagnostics":                      true,
	"TestAnalyzeStrategyPineRouteReturnsDiagnosticsAndRequirements":             true,
	"TestAnalyzeStrategyPineRouteReturnsV20ParseOnlyMetadata":                   true,
	"TestBuildToolPayloadIncludesBrokerBoundary":                                true,
	"TestBuildToolPayloadIncludesSupportMatrix":                                 true,
	"TestCompileRejectsPublicInternalHelperCalls":                               true,
	"TestCompileSupportsAllowEntryInRiskDeclaration":                            true,
	"TestCompileSupportsPendingStopAndCancelOrders":                             true,
	"TestCompileSupportsStrategyExitSubset":                                     true,
	"TestCompileSupportsSwitchAndMultiStatementUDF":                             true,
	"TestCompileSupportsV12AdvancedIndicators":                                  true,
	"TestCompileSupportsV12AdvancedIndicatorsInStaticIntradaySecurity":          true,
	"TestCompileSupportsV13IndicatorsInStaticIntradaySecurity":                  true,
	"TestCompileSupportsV13MigrationIndicators":                                 true,
	"TestCompileSupportsV14RequestSecurityPureExpression":                       true,
	"TestCompileSupportsV14WindowMomentumAndStatefulIndicators":                 true,
	"TestCompileSupportsV15RequestSecurityCommonTAExpression":                   true,
	"TestCompileSupportsV15StaticForLoopControl":                                true,
	"TestCompileSupportsV16RequestSecurityTupleWhitelist":                       true,
	"TestCompileSupportsV21BBWAndCOG":                                           true,
	"TestCompileSupportsV21ExecutableCollectionCore":                            true,
	"TestCompileSupportsV22PureUDTAndMethodSubset":                              true,
	"TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops":            true,
	"TestCompileSupportsV23LocalObjectFieldReassignment":                        true,
	"TestCompileSupportsV23NamedObjectArgsAndPureMethodBody":                    true,
	"TestCompileSupportsV23RequestSecurityPureObjectAndCollectionExpressions":   true,
	"TestCompileSupportsV24CollectionExpansionAndMTFStoch":                      true,
	"TestCompileSupportsV24NamedObjectMethodExpressionAndRuntimeLoopFallback":   true,
	"TestCompileSupportsV25ArrayStringAndTimeframeHelpers":                      true,
	"TestCompileSupportsV26CollectionIterationHistoryAndObjectCollectionFields": true,
	"TestCompileSupportsV27CollectionTimeframeAndMTFHelpers":                    true,
	"TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata":           true,
	"TestCompileSupportsV29ObjectHistoryMethodReceiverAndMTFHistoryExpression":  true,
	"TestCompileSupportsV30SemanticDeclarationModelAndVaripPolicy":              true,
	"TestGeneratedPineSupportSnapshotIsCurrent":                                 true,
	"TestGoldenExamplesAnalyzeAndPlan":                                          true,
	"TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes":            true,
	"TestIndicatorRuntimeSnapshotIncludesV13MigrationIndicators":                true,
	"TestPineV20LanguageFoundationGate":                                         true,
	"TestRequestSecurityTimeframeRequirementsValidateAgainstStrategyInterval":   true,
	"TestValidateScriptRejectsUnsupportedPineRuntimeFeature":                    true,
	"TestValidateScriptReportsUnsupportedUDFAndStaticForCases":                  true,
}
