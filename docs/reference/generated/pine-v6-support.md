# JFTrade Pine v6 Support Snapshot

> 自动生成，请勿手改。来源：`pkg/strategy/pinespec` 与 `pkg/strategy/pine` capability registry。

## Baseline

| Field | Value |
| --- | --- |
| Pine version | `v6` |
| Product version | `v4.0` |
| Source format | `pine-v6` |
| Runtime | `pine-pinets` |
| External engine | `pinets-shadow` (`off by default`) |
| Score model | `closed-bar-strategy-v4.0` |
| Compatibility score | `98.30` |

## Score Dimensions

| Dimension | Weight | Score | Supported Weight | Total Weight | Unsupported IDs |
| --- | ---: | ---: | ---: | ---: | --- |
| `language` | 0.12 | 98.86 | 424.60 | 429.50 | `syntax.recursive_nested_udf` |
| `indicators` | 0.30 | 96.44 | 132.70 | 137.60 | `indicator.full_ta_surface`<br>`indicator.visual_only_plot_stack` |
| `mtf` | 0.48 | 99.02 | 243.40 | 245.80 | `request.security.dynamic_symbol_timeframe`<br>`request.security.lookahead_gaps_on` |
| `orders` | 0.00 | 77.94 | 21.90 | 28.10 | `order.oca_partial_fill`<br>`order.intrabar_tick_recalc`<br>`order.full_tv_broker_emulator` |
| `tooling` | 0.10 | 99.71 | 558.00 | 559.60 |  |

## Capability Registry

| ID | Dimension | Status | Weight | Layers | Tests | Notes |
| --- | --- | --- | ---: | --- | --- | --- |
| `metadata.version6` | `tooling` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `metadata.strategy` | `tooling` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `metadata.backtest_costs` | `tooling` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `metadata.process_orders_on_close` | `tooling` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `syntax.if_else` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `syntax.assignment` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `syntax.var` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `syntax.const_varip` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `syntax.reassign` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `syntax.udf_expression` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `syntax.udf_multistatement` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsSwitchAndMultiStatementUDF` |  |
| `syntax.switch_static_lowering` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsSwitchAndMultiStatementUDF` |  |
| `syntax.for_static_unroll` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `syntax.v15_loop_control_subset` | `language` | `supported` | 4.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV15StaticForLoopControl`<br>`TestPineV15MigrationCorpusGate` |  |
| `syntax.v16_security_tuple_destructure` | `language` | `supported` | 12.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV16RequestSecurityTupleWhitelist`<br>`TestPineV16MigrationCorpusGate` |  |
| `syntax.v17_ast_semantic_transition` | `language` | `supported` | 20.00 | parser, planner, runtime, backtest, frontend, spec | `TestAnalyzeScriptIncludesV17SemanticSummary`<br>`TestAnalyzeScriptReportsSemanticSignatureDiagnostics`<br>`TestPineV17MigrationCorpusGate` |  |
| `syntax.v21_collection_runtime_core` | `language` | `supported` | 35.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV21ExecutableCollectionCore`<br>`TestExecuteCollectionStatementsPersistAcrossBars`<br>`TestPineV21MigrationCorpusGate` |  |
| `syntax.v22_structured_loop_runtime` | `language` | `supported` | 22.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops`<br>`TestCompiledWhileLoopHonorsContinueBeforeConditionExit`<br>`TestPineV22MigrationCorpusGate` |  |
| `syntax.v22_pure_udt_method_runtime` | `language` | `supported` | 18.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV22PureUDTAndMethodSubset`<br>`TestExecutePureUDTConstructorAndMethod`<br>`TestPineV22MigrationCorpusGate` |  |
| `syntax.v23_collection_api_expansion` | `language` | `supported` | 16.00 | parser, planner, runtime, backtest, spec | `TestExecuteV23ArrayCollectionOperations`<br>`TestExecuteV23MatrixCollectionOperations`<br>`TestPineV23MigrationCorpusGate` |  |
| `syntax.v23_pure_method_body_named_args` | `language` | `supported` | 12.00 | parser, planner, runtime, backtest, spec | `TestCompileSupportsV23NamedObjectArgsAndPureMethodBody`<br>`TestCompileSupportsV23LocalObjectFieldReassignment`<br>`TestExecuteV23PureUDTNamedConstructorAndMethodBody`<br>`TestExecuteV23LocalObjectFieldReassignment`<br>`TestPineV23MigrationCorpusGate` |  |
| `syntax.v24_collection_api_expansion` | `language` | `supported` | 14.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV24CollectionExpansionAndMTFStoch`<br>`TestExecuteV24ArrayCollectionOperations`<br>`TestExecuteV24MapCollectionOperations`<br>`TestPineV24MigrationCorpusGate` |  |
| `syntax.v24_runtime_loop_fallback` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV24NamedObjectMethodExpressionAndRuntimeLoopFallback`<br>`TestPineV24MigrationCorpusGate` |  |
| `syntax.v24_persistent_object_field_set` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV23LocalObjectFieldReassignment`<br>`TestExecuteV24PersistentObjectFieldReassignment`<br>`TestPineV24MigrationCorpusGate` |  |
| `syntax.v25_array_stat_api` | `language` | `supported` | 12.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV25ArrayStringAndTimeframeHelpers`<br>`TestExecuteV25ArrayCollectionStatistics`<br>`TestPineV25MigrationCorpusGate` |  |
| `syntax.v26_collection_iteration` | `language` | `supported` | 14.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV26CollectionIterationHistoryAndObjectCollectionFields`<br>`TestExecuteV26CollectionForLoop`<br>`TestPineV26MigrationCorpusGate` |  |
| `syntax.v26_collection_history_snapshot` | `language` | `supported` | 12.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV26CollectionIterationHistoryAndObjectCollectionFields`<br>`TestExecuteV26CollectionHistorySnapshot`<br>`TestPineV26MigrationCorpusGate` |  |
| `syntax.v26_object_collection_fields` | `language` | `supported` | 14.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV26CollectionIterationHistoryAndObjectCollectionFields`<br>`TestExecuteV26ObjectCollectionFieldMutation`<br>`TestPineV26MigrationCorpusGate` |  |
| `syntax.v26_library_export_metadata` | `language` | `supported` | 4.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV20LanguageFoundationGate`<br>`TestPineV26MigrationCorpusGate` |  |
| `syntax.v27_collection_history_aggregates` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV27CollectionTimeframeAndMTFHelpers`<br>`TestPineV27MigrationCorpusGate` |  |
| `syntax.v27_map_matrix_iteration` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV27CollectionTimeframeAndMTFHelpers`<br>`TestPineV27MigrationCorpusGate` |  |
| `syntax.v28_object_history_read` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata`<br>`TestExecuteV28ObjectHistoryAndMethodChain`<br>`TestPineV28MigrationCorpusGate` |  |
| `syntax.v28_method_chain` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata`<br>`TestExecuteV28ObjectHistoryAndMethodChain`<br>`TestPineV28MigrationCorpusGate` |  |
| `syntax.v28_export_metadata` | `language` | `supported` | 6.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata`<br>`TestPineV28MigrationCorpusGate` |  |
| `syntax.v29_object_history_method_receiver` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV29ObjectHistoryMethodReceiverAndMTFHistoryExpression`<br>`TestExecuteV29ObjectHistoryMethodReceiverAndNamedChain`<br>`TestPineV29MigrationCorpusGate` |  |
| `syntax.v29_method_chain_named_defaults` | `language` | `supported` | 6.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV29ObjectHistoryMethodReceiverAndMTFHistoryExpression`<br>`TestExecuteV29ObjectHistoryMethodReceiverAndNamedChain`<br>`TestPineV29MigrationCorpusGate` |  |
| `syntax.v29_request_security_diagnostics` | `language` | `supported` | 6.00 | parser, planner, runtime, backtest, frontend, spec | `TestAnalyzeScriptReportsV29RequestSecurityDiagnostics`<br>`TestPineV29MigrationCorpusGate` |  |
| `syntax.v30_stable_semantic_declarations` | `language` | `supported` | 10.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV30SemanticDeclarationModelAndVaripPolicy`<br>`TestPineV30MigrationCorpusGate` |  |
| `syntax.v30_varip_closed_bar_policy` | `language` | `supported` | 4.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV30SemanticDeclarationModelAndVaripPolicy`<br>`TestPineV30MigrationCorpusGate` |  |
| `syntax.v30_parser_whitespace_comments` | `language` | `supported` | 4.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV30SemanticDeclarationModelAndVaripPolicy`<br>`TestPineV30MigrationCorpusGate` |  |
| `syntax.v31_public_surface_lock` | `language` | `supported` | 6.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileRejectsPublicInternalHelperCalls`<br>`TestAnalyzeScriptReportsPublicInternalHelperDiagnostics`<br>`TestStrategyPineEditorIntelliSense` |  |
| `syntax.v33_advanced_language_boundary` | `language` | `supported` | 6.00 | parser, planner, runtime, backtest, frontend, spec | `TestAnalyzeScriptReportsV33AdvancedLanguageBoundaryDiagnostics`<br>`TestValidateScriptReportsUnsupportedUDFAndStaticForCases`<br>`TestExecuteWhileLoopHonorsBreakAndLimit` |  |
| `expression.history_ref_1` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.history_ref_n` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.ternary` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.na_nz` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.strict_bool` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.input_defaults` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.input_symbol_session` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.input_timeframe` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.math_namespace` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.math_avg_mintick` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestCompileSupportsV13MigrationIndicators`<br>`TestEvaluateExpressionSupportsMathAndTimeVariables` |  |
| `expression.string_namespace` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.format_constants` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.account_equity` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.time_variables` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.derived_sources` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.timestamp` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.barstate_session` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.pine_constants` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `expression.v14_direct_indicator_calls` | `language` | `supported` | 3.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV14RequestSecurityPureExpression`<br>`TestEvaluateExpressionSupportsNewIndicatorLookups` |  |
| `expression.v14_security_source_calls` | `language` | `supported` | 2.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV14RequestSecurityPureExpression`<br>`TestEvaluateExpressionSupportsNewIndicatorLookups` |  |
| `expression.v16_tuple_alias_member_lowering` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV16RequestSecurityTupleWhitelist`<br>`TestPineV16MigrationCorpusGate` |  |
| `expression.v17_semantic_scope_types` | `language` | `supported` | 15.00 | parser, planner, runtime, backtest, frontend, spec | `TestAnalyzeScriptIncludesV17SemanticSummary`<br>`TestAnalyzeScriptReportsSemanticSignatureDiagnostics`<br>`TestPineV17MigrationCorpusGate` |  |
| `expression.v17_signature_diagnostics` | `language` | `supported` | 10.00 | parser, planner, runtime, backtest, frontend, spec | `TestAnalyzeScriptIncludesV17SemanticSummary`<br>`TestAnalyzeScriptReportsSemanticSignatureDiagnostics`<br>`TestPineV17MigrationCorpusGate` |  |
| `expression.v22_general_tuple` | `language` | `supported` | 10.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops`<br>`TestExecuteDynamicLoopsAndGeneralTuple`<br>`TestPineV22MigrationCorpusGate` |  |
| `expression.v23_object_field_set` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, spec | `TestCompileSupportsV23NamedObjectArgsAndPureMethodBody`<br>`TestCompileSupportsV23LocalObjectFieldReassignment`<br>`TestExecuteV23PureUDTNamedConstructorAndMethodBody`<br>`TestExecuteV23LocalObjectFieldReassignment`<br>`TestPineV23MigrationCorpusGate` |  |
| `expression.v25_string_helpers` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV25ArrayStringAndTimeframeHelpers`<br>`TestEvaluateExpressionSupportsDerivedSourcesEnvironmentTimestampAndTR`<br>`TestPineV25MigrationCorpusGate` |  |
| `expression.v25_timeframe_change` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV25ArrayStringAndTimeframeHelpers`<br>`TestEvaluateExpressionSupportsDerivedSourcesEnvironmentTimestampAndTR`<br>`TestPineV25MigrationCorpusGate` |  |
| `expression.v27_timeframe_helpers` | `language` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV27CollectionTimeframeAndMTFHelpers`<br>`TestEvaluateExpressionSupportsDerivedSourcesEnvironmentTimestampAndTR`<br>`TestPineV27MigrationCorpusGate` |  |
| `indicator.ma` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.ma_source_aware` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.source_aware_core` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.rsi` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.macd` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.atr` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.cci` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.bollinger` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.williams_r` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.rolling_window` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.sum` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.cross` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.cum_stoch_extrema_bars` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.vwap_mfi_dmi_supertrend` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.stateful_events` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.sar` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `indicator.linreg_obv_pivots` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV12AdvancedIndicators`<br>`TestAdvancedIndicatorCalculationsUseAuditedVectors` |  |
| `indicator.keltner_alma` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV12AdvancedIndicators`<br>`TestAdvancedIndicatorCalculationsUseAuditedVectors` |  |
| `indicator.v13_migration_set` | `indicators` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV13MigrationIndicators`<br>`TestAdvancedIndicatorCalculationsUseAuditedVectors`<br>`TestIndicatorRuntimeSnapshotIncludesV13MigrationIndicators` |  |
| `indicator.v14_window_momentum_set` | `indicators` | `supported` | 4.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV14WindowMomentumAndStatefulIndicators`<br>`TestIndicatorRuntimeSnapshotIncludesWindowAndSourceAwareIndicators` |  |
| `indicator.v14_stateful_events` | `indicators` | `supported` | 3.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV14WindowMomentumAndStatefulIndicators`<br>`TestEvaluateExpressionSupportsBarsSinceAndValueWhenState`<br>`TestPineV14MigrationCorpusGate` |  |
| `indicator.true_range` | `indicators` | `supported` | 2.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV14WindowMomentumAndStatefulIndicators`<br>`TestEvaluateExpressionSupportsDerivedSourcesEnvironmentTimestampAndTR` |  |
| `indicator.v15_common_ta_set` | `indicators` | `supported` | 12.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV15RequestSecurityCommonTAExpression`<br>`TestEvaluateExpressionSupportsNewIndicatorLookups`<br>`TestPineV15MigrationCorpusGate` |  |
| `indicator.v16_mtf_tuple_bindings` | `indicators` | `supported` | 12.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV16RequestSecurityTupleWhitelist`<br>`TestPineV16MigrationCorpusGate` |  |
| `indicator.v17_source_aware_semantic_requirements` | `indicators` | `supported` | 15.00 | parser, planner, runtime, backtest, frontend, spec | `TestAnalyzeScriptIncludesV17SemanticSummary`<br>`TestAnalyzeScriptReportsSemanticSignatureDiagnostics`<br>`TestPineV17MigrationCorpusGate` |  |
| `indicator.v21_bbw_cog_anchored_vwap` | `indicators` | `supported` | 35.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV21BBWAndCOG`<br>`TestAdvancedIndicatorCalculationsUseAuditedVectors`<br>`TestPineV21MigrationCorpusGate` |  |
| `indicator.v24_mtf_stoch` | `indicators` | `supported` | 30.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV24CollectionExpansionAndMTFStoch`<br>`TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes`<br>`TestPineV24MigrationCorpusGate` |  |
| `request.security.mtf_ma_subset` | `mtf` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV12AdvancedIndicatorsInStaticIntradaySecurity`<br>`TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes` |  |
| `request.security.mtf_sources` | `mtf` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `request.security.mtf_ma_source_aware` | `mtf` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `request.security.timeframe_multipliers` | `mtf` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `request.security.htf_history` | `mtf` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `request.security.mtf_v12_advanced` | `mtf` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV12AdvancedIndicatorsInStaticIntradaySecurity`<br>`TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes` |  |
| `request.security.mtf_v13_advanced` | `mtf` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV13IndicatorsInStaticIntradaySecurity`<br>`TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes` |  |
| `request.security.pure_expression` | `mtf` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV14RequestSecurityPureExpression`<br>`TestEvaluateExpressionSupportsNewIndicatorLookups`<br>`TestPineV14MigrationCorpusGate` |  |
| `request.security.pure_expression_diagnostics` | `mtf` | `supported` | 2.00 | parser, planner, runtime, backtest, frontend, spec | `TestValidateScriptRejectsUnsupportedPineRuntimeFeature`<br>`TestPineV14MigrationCorpusGate` |  |
| `request.security.v15_common_ta_expression` | `mtf` | `supported` | 28.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV15RequestSecurityCommonTAExpression`<br>`TestEvaluateExpressionSupportsNewIndicatorLookups`<br>`TestPineV15MigrationCorpusGate` |  |
| `request.security.v16_tuple_whitelist` | `mtf` | `supported` | 18.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV16RequestSecurityTupleWhitelist`<br>`TestPineV16MigrationCorpusGate` |  |
| `request.security.v17_semantic_tuple_corpus` | `mtf` | `supported` | 20.00 | parser, planner, runtime, backtest, frontend, spec | `TestAnalyzeScriptIncludesV17SemanticSummary`<br>`TestAnalyzeScriptReportsSemanticSignatureDiagnostics`<br>`TestPineV17MigrationCorpusGate` |  |
| `request.security.v21_ast_pure_expression` | `mtf` | `supported` | 62.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV21BBWAndCOG`<br>`TestAdvancedIndicatorCalculationsUseAuditedVectors`<br>`TestPineV21MigrationCorpusGate` |  |
| `request.security.v22_general_tuple` | `mtf` | `supported` | 16.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops`<br>`TestExecuteDynamicLoopsAndGeneralTuple`<br>`TestPineV22MigrationCorpusGate` |  |
| `request.security.v23_pure_collection_object_expression` | `mtf` | `supported` | 22.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV23RequestSecurityPureObjectAndCollectionExpressions`<br>`TestEvaluateV23ObjectMethodExpression`<br>`TestPineV23MigrationCorpusGate` |  |
| `request.security.v24_mtf_stoch` | `mtf` | `supported` | 20.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV24CollectionExpansionAndMTFStoch`<br>`TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes`<br>`TestPineV24MigrationCorpusGate` |  |
| `request.security.v27_pure_helper_expression` | `mtf` | `supported` | 10.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV27CollectionTimeframeAndMTFHelpers`<br>`TestPineV27MigrationCorpusGate` |  |
| `request.security.v28_object_method_expression` | `mtf` | `supported` | 10.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata`<br>`TestPineV28MigrationCorpusGate` |  |
| `request.security.v29_object_history_expression` | `mtf` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsV29ObjectHistoryMethodReceiverAndMTFHistoryExpression`<br>`TestExecuteV29ObjectHistoryMethodReceiverAndNamedChain`<br>`TestPineV29MigrationCorpusGate` |  |
| `request.security.v32_diagnostic_matrix` | `mtf` | `supported` | 6.00 | parser, planner, runtime, backtest, frontend, spec | `TestAnalyzeScriptReportsV32RequestSecurityDiagnosticMatrix` |  |
| `request.security.v32_lower_timeframe_preflight` | `mtf` | `supported` | 6.00 | parser, planner, runtime, backtest, frontend, spec | `TestRequestSecurityTimeframeRequirementsValidateAgainstStrategyInterval`<br>`TestRunRejectsLowerTimeframeRequestSecurityBeforeReplay` |  |
| `expression.barmerge_constants` | `language` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `visual.noop_calls` | `tooling` | `warning` | 1.00 | parser, runtime, frontend, spec | `TestPineV20LanguageFoundationGate`<br>`TestAnalyzeStrategyPineRouteReturnsV20ParseOnlyMetadata` | plot/drawing/table 等视觉 API 解析为 warning/no-op，并在 AnalyzeScript 中暴露分类 metadata，包括赋值形式的 drawing/table constructor。 |
| `alert.alertcondition_noop` | `tooling` | `warning` | 1.00 | parser, runtime, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | alertcondition 解析为 warning/no-op，交易告警使用 order alert metadata。 |
| `order.strategy_order_net` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `order.qty_percent` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `order.close_all` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `order.close_immediately` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `order.comment_alert_metadata` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `order.exit_quantity` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `order.exit_bracket` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `order.exit_price_expressions` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `order.trailing_exit` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsStrategyExitSubset`<br>`TestRunPinePendingStopCancelAndBracketExit/trailing_points_closes_position`<br>`TestRunPinePendingStopCancelAndBracketExit/trailing_price_closes_position` |  |
| `order.pending_stop` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `order.pending_stop_limit` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsPendingStopAndCancelOrders`<br>`TestRunPinePendingStopCancelAndBracketExit/stop-limit_activates_before_limit_fill` |  |
| `order.cancel_pending` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `order.entry_reversal` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsAllowEntryInRiskDeclaration`<br>`TestAdjustEntryOrderQuantitySupportsPineReversalAndAllowEntryIn`<br>`TestRunPineEntryReversalAndAllowedEntryDirection` |  |
| `order.allow_entry_in` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileSupportsAllowEntryInRiskDeclaration`<br>`TestAdjustEntryOrderQuantitySupportsPineReversalAndAllowEntryIn`<br>`TestRunPineEntryReversalAndAllowedEntryDirection` |  |
| `strategy.entry_close_exit_subset` | `orders` | `supported` | 1.00 | parser, planner, runtime, backtest, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` |  |
| `strategy.v40_broker_boundary_decision` | `orders` | `supported` | 6.00 | parser, planner, runtime, backtest, spec | `TestAnalyzeScriptReportsV40BrokerBoundaryDiagnostics`<br>`TestBuildToolPayloadIncludesBrokerBoundary`<br>`TestGeneratedPineSupportSnapshotIsCurrent` |  |
| `order.short_broker_accounting` | `orders` | `partial` | 1.80 | parser, planner, runtime, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | Pine runtime 计算反手数量；当前 JFTrade 现货回测执行器仍不模拟保证金裸空。 |
| `syntax.arrays_maps_matrices` | `language` | `partial` | 2.20 | parser, planner, runtime, frontend, spec | `TestCompileSupportsV21ExecutableCollectionCore`<br>`TestExecuteCollectionStatementsPersistAcrossBars`<br>`TestPineV21MigrationCorpusGate` | array/map/matrix 常用 constructor、读取、变更、copy/slice/fill/aggregate、排序、统计、array.from/concat/join、map.copy/keys/values、array for-in、map keys/values iteration、matrix rows/columns/get/set 与 collection history aggregate snapshot 已可执行并跨 K 线持久化；深层泛型、嵌套 collection 全表面仍未覆盖。 |
| `syntax.methods_types_libraries` | `language` | `partial` | 2.00 | parser, planner, runtime, frontend, spec | `TestPineV20LanguageFoundationGate`<br>`TestCompileSupportsV22PureUDTAndMethodSubset`<br>`TestPineV22MigrationCorpusGate` | type、命名 constructor 参数、多语句纯 method、局部/持久 object 字段重赋值、object collection fields、object history read、纯 method chain 与 export kind metadata 子集可执行/可分析；library/import 和完整 Pine method/type 系统仍只进入 semantic metadata 与诊断。 |
| `syntax.dynamic_loops_while` | `language` | `partial` | 2.00 | parser, planner, runtime, frontend, spec | `TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops`<br>`TestCompiledWhileLoopHonorsContinueBeforeConditionExit`<br>`TestPineV22MigrationCorpusGate` | 动态 for、while、break/continue 已在闭盘 runtime 中执行，限制嵌套深度和单 bar 最大迭代数以避免无限循环。 |
| `syntax.recursive_nested_udf` | `language` | `unsupported` | 1.30 | spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | 递归和嵌套 UDF 暂不支持。 |
| `expression.tuple_general` | `language` | `partial` | 1.00 | parser, planner, runtime, spec | `TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops`<br>`TestExecuteDynamicLoopsAndGeneralTuple`<br>`TestPineV22MigrationCorpusGate` | 通用 tuple literal/destructure 支持 2 到 8 个元素和 _ discard；完整 Pine tuple/array 互操作仍未覆盖。 |
| `indicator.full_ta_surface` | `indicators` | `unsupported` | 3.20 | spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | 未覆盖 TradingView Pine v6 全部 ta.* 表面。 |
| `indicator.v13_mtf_intraday_only` | `indicators` | `partial` | 1.40 | parser, planner, runtime, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | v1.3 新增 MTF 指标限制为同标的静态 intraday timeframe。 |
| `indicator.visual_only_plot_stack` | `indicators` | `unsupported` | 1.00 | spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | 视觉指标 API 继续 no-op；分析结果可返回分类视觉 metadata。 |
| `request.security.dynamic_symbol_timeframe` | `mtf` | `unsupported` | 1.20 | spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | 动态 symbol/timeframe 暂不支持。 |
| `request.security.lookahead_gaps_on` | `mtf` | `unsupported` | 0.80 | spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | lookahead_on/gaps_on 暂不支持。 |
| `request.security.tuple_general` | `mtf` | `partial` | 0.80 | parser, planner, runtime, spec | `TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops`<br>`TestExecuteDynamicLoopsAndGeneralTuple`<br>`TestPineV22MigrationCorpusGate` | 同标的静态 timeframe 下支持 2 到 8 元纯表达式 tuple；动态 symbol/timeframe、side effect、nested request 仍不支持。 |
| `order.oca_partial_fill` | `orders` | `unsupported` | 2.20 | spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | OCA、partial fill 和完整 broker emulator 暂不支持。 |
| `order.intrabar_tick_recalc` | `orders` | `unsupported` | 1.70 | spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | tick 级重算和 intrabar 路径推断暂不支持。 |
| `order.full_tv_broker_emulator` | `orders` | `unsupported` | 1.40 | spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | 完整 TradingView broker emulator 不属于当前目标。 |
| `tooling.visual_builder_roundtrip` | `tooling` | `partial` | 0.60 | parser, planner, runtime, frontend, spec | `TestGoldenExamplesAnalyzeAndPlan`<br>`TestPineGoldenBenchmarkCasesSmoke` | 流程图反解只覆盖可标准化 Pine v6 子集；无法映射的新语法返回行号诊断，请继续在 Pine 工作台编辑。 |
| `tooling.migration_corpus_v14` | `tooling` | `supported` | 4.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV14MigrationCorpusGate` |  |
| `tooling.migration_corpus_v15` | `tooling` | `supported` | 6.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV15MigrationCorpusGate` |  |
| `tooling.migration_corpus_v16` | `tooling` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV16MigrationCorpusGate` |  |
| `tooling.migration_corpus_v17` | `tooling` | `supported` | 10.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV17MigrationCorpusGate`<br>`TestAnalyzeStrategyPineRouteReturnsDiagnosticsAndRequirements` |  |
| `tooling.migration_corpus_v21` | `tooling` | `supported` | 25.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV21MigrationCorpusGate` |  |
| `tooling.migration_corpus_v22` | `tooling` | `supported` | 30.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV22MigrationCorpusGate` |  |
| `tooling.migration_corpus_v23` | `tooling` | `supported` | 35.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV23MigrationCorpusGate` |  |
| `tooling.migration_corpus_v24` | `tooling` | `supported` | 40.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV24MigrationCorpusGate` |  |
| `tooling.migration_corpus_v25` | `tooling` | `supported` | 45.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV25MigrationCorpusGate` |  |
| `tooling.migration_corpus_v26` | `tooling` | `supported` | 50.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV26MigrationCorpusGate` |  |
| `tooling.migration_corpus_v27` | `tooling` | `supported` | 55.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV27MigrationCorpusGate` |  |
| `tooling.migration_corpus_v28` | `tooling` | `supported` | 60.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV28MigrationCorpusGate` |  |
| `tooling.migration_corpus_v29` | `tooling` | `supported` | 65.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV29MigrationCorpusGate` |  |
| `tooling.migration_corpus_v30` | `tooling` | `supported` | 70.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV30MigrationCorpusGate` |  |
| `tooling.semantic_analyze_payload` | `tooling` | `supported` | 10.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV17MigrationCorpusGate`<br>`TestAnalyzeStrategyPineRouteReturnsDiagnosticsAndRequirements` |  |
| `tooling.visual_metadata_output` | `tooling` | `supported` | 4.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV20LanguageFoundationGate`<br>`TestAnalyzeStrategyPineRouteReturnsV20ParseOnlyMetadata` |  |
| `tooling.v20_language_foundation` | `tooling` | `supported` | 10.00 | parser, planner, runtime, backtest, frontend, spec | `TestPineV20LanguageFoundationGate`<br>`TestAnalyzeStrategyPineRouteReturnsV20ParseOnlyMetadata` |  |
| `tooling.v31_structured_helper_diagnostics` | `tooling` | `supported` | 8.00 | parser, planner, runtime, backtest, frontend, spec | `TestCompileRejectsPublicInternalHelperCalls`<br>`TestAnalyzeScriptReportsPublicInternalHelperDiagnostics`<br>`TestStrategyPineEditorIntelliSense` |  |
| `tooling.v33_structured_language_diagnostics` | `tooling` | `supported` | 6.00 | parser, planner, runtime, backtest, frontend, spec | `TestAnalyzeScriptReportsV33AdvancedLanguageBoundaryDiagnostics`<br>`TestValidateScriptReportsUnsupportedUDFAndStaticForCases`<br>`TestExecuteWhileLoopHonorsBreakAndLimit` |  |
| `tooling.v34_generated_support_snapshot` | `tooling` | `supported` | 6.00 | parser, planner, runtime, backtest, frontend, spec | `TestGeneratedPineSupportSnapshotIsCurrent`<br>`TestBuildToolPayloadIncludesSupportMatrix` |  |
| `tooling.v40_broker_boundary_snapshot` | `tooling` | `supported` | 6.00 | parser, planner, runtime, backtest, frontend, spec | `TestAnalyzeScriptReportsV40BrokerBoundaryDiagnostics`<br>`TestBuildToolPayloadIncludesBrokerBoundary`<br>`TestGeneratedPineSupportSnapshotIsCurrent` |  |

## Support Matrix

| Capability | Parser | Planner | Runtime | JFTrade | Frontend | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| JFTrade Pine v6 main path | yes | yes | yes | yes | yes | 新建、保存、预览、回测、实例化和启动统一使用 sourceFormat=pine-v6 + runtime=pine-pinets；旧 source/runtime 与旧 visual model 明确拒绝。 |
| Backtest capital and trading costs | yes | yes | yes | yes | yes | API initialBalance > Pine initial_capital > 系统默认；支持 percent/cash commission 与按最小价格单位计算的 slippage ticks，仅作用于回测。 |
| Pine metadata and diagnostics | yes | yes | yes | yes | yes | 统一通过 AnalyzeScript、strategy.pine_spec、编辑器提示、结构化 diagnostics、visuals/declarations/collectionOperations/objectOperations metadata 和 semantic summary 暴露。 |
| Source-aware indicators | yes | yes | yes | yes | yes | MA/RSI/stdev/variance/CCI/rolling/source-aware MTF 使用稳定 key；close 保留 legacy key。 |
| Rolling and stateful indicators | yes | yes | yes | yes | no | highest/lowest/change/mom/roc/rising/falling/sum、barssince、valuewhen 已可执行；前端只覆盖常用子集。 |
| MTF request.security subset | yes | yes | yes | yes | yes | 同标的 source/source[n]/source-aware MA、静态 intraday 高级指标、v1.4 纯表达式、v1.5 common TA pure-expression、v1.6 tuple 白名单、v2.2 2-8 元纯表达式 tuple、v2.3 纯 collection/object 表达式，以及 v2.4 MTF stoch；禁止 lookahead_on/gaps_on、动态 symbol/timeframe、side effect 和 nested request。 |
| Orders and exits | yes | yes | yes | yes | yes | entry/order/close/close_all/exit/cancel 的可执行子集已贯通；entry 反手与 allow_entry_in 已支持，完整 broker emulator 不属于当前目标。 |
| UDF, switch and static for | yes | yes | yes | yes | no | 表达式/受控多语句 UDF、switch 与静态整数 for 编译期展开；静态 for 内条件 break/continue 会回退到 bounded runtime loop；递归 UDF 诊断失败。 |
| v1.2 migration indicators and switch | yes | yes | yes | yes | yes | linreg/OBV/pivot/Keltner/ALMA、switch lowering 与受控多语句 UDF 已贯通。 |
| v1.3 migration indicators and entry risk | yes | yes | yes | yes | yes | CMO/TSI/correlation/dev/median/percentile/percentrank/SWMA、math.avg/round_to_mintick、entry 反手和 allow_entry_in 已贯通。 |
| v1.4 practical migration set | yes | yes | yes | yes | yes | 窗口/动量、barssince/valuewhen、ta.tr(true\|false)、request.security 纯表达式和 80+ 迁移语料门禁已纳入。 |
| v1.5 practical migration set | yes | yes | yes | yes | yes | request.security common TA pure-expression、交叉/状态组合、静态 for 无条件 break/continue 和 100+ 迁移语料门禁已纳入。 |
| v1.6 practical migration set | yes | yes | yes | yes | yes | request.security tuple 白名单、MTF 多返回指标 tuple assignment 和 130+ 迁移语料门禁已纳入。 |
| v1.7 semantic transition | yes | yes | yes | yes | yes | AST 驱动 semantic summary、函数签名诊断、tuple 解构摘要和 170+ 迁移语料门禁已纳入。 |
| v2.0 language foundation | yes | no | no | yes | yes | array/map/matrix typed declaration、constructor、namespace/method-style operation、type/method/import alias/library、UDT object operation 和视觉 API 已进入 parse/semantic/top-level metadata 模型；collection namespace/type argument compatibility、visual kind/variable/target/title、type fields、method receiver/parameters/defaults、duplicate declaration/receiver/overload diagnostics、object constructor/method signatures、object arity diagnostics 与 import version/alias 可分析，非执行表面返回明确诊断。 |
| v2.1 executable collection and TA set | yes | yes | yes | yes | yes | array/map/matrix 常用 constructor/read/mutation 支持跨 K 线引用状态；ta.bbw、ta.cog、日/周/月锚定 VWAP 与 AST 校验的静态同标的 request.security 纯表达式已进入 250+ 语料门禁。 |
| v2.2 structured loops, tuple and pure object subset | yes | yes | yes | yes | yes | 结构化 AST lowering 消费缩进树；2-8 元 tuple literal/destructure、静态同标的 request.security tuple、动态 for/while/break/continue、纯 UDT constructor 与单表达式 method 已进入 420+ 语料门禁。 |
| v2.3 collection, pure object and MTF expression expansion | yes | yes | yes | yes | yes | array copy/slice/reverse/fill/includes/indexof/min/max/avg/sum、matrix fill/copy/reshape/add/remove、命名 constructor/method 参数、多语句纯 method、局部 object 字段重赋值，以及 request.security 纯 collection/object 表达式已进入 850+ 语料门禁。 |
| v2.4 collection/map, MTF stoch and persistent object expansion | yes | yes | yes | yes | yes | array.from/concat/join/sort/sort_indices/binary_search/median/mode/range、map.copy/keys/values、order.ascending/descending、MTF ta.stoch、静态 for 条件 break/continue runtime fallback、持久 object 字段重赋值已进入 1250+ 语料门禁。 |
| v2.5 array stats, string and timeframe helpers | yes | yes | yes | yes | yes | array abs/binary_search_leftmost/rightmost/percentrank/percentile/stdev/variance/covariance、str.length/contains/pos/substring/replace/upper/lower/format/tostring、time_close 与 timeframe.change 已进入 1450+ 语料门禁。 |
| v2.6 collection iteration, history and object fields | yes | yes | yes | yes | yes | array for-in、只读 collection history snapshot、inline collection constructor expression、UDT collection fields 与 library/export metadata 诊断已进入 1650+ 语料门禁。 |
| v2.7 collection/timeframe and MTF helper expansion | yes | yes | yes | yes | yes | array history aggregate snapshot、map keys/values iteration、matrix rows/columns/get/set、timeframe.in_seconds/timeframe.multiplier/timeframe.isseconds 与 request.security 纯 helper 表达式已进入 1900+ 语料门禁。 |
| v2.8 object history, method chain and export metadata | yes | yes | yes | yes | yes | box[1].field object history read、无副作用 method chain、request.security object method expression 与 export function/type/method kind metadata 已进入 2200+ 语料门禁。 |
| v2.9 object history method receiver and MTF diagnostics | yes | yes | yes | yes | yes | box[1].score(...)、method chain named/default args、request.security object history field/method pure expression 与 dynamic symbol/timeframe、nested、side-effect、lookahead/gaps 分码诊断已进入 2500+ 语料门禁。 |
| v3.0 stable semantic declarations and varip policy | yes | yes | yes | yes | yes | SemanticDeclaration 增补 signature/unsupportedReason，type/method/export/import metadata 稳定；varip 在 closed-bar runtime 下按 var 执行并输出 warning，空白/注释解析韧性已进入 2850+ 语料门禁。 |
| v3.1 native public surface diagnostics | yes | yes | yes | yes | yes | 用户输入 ma/security_source/bollinger/history/ifelse/cross_over/cross_under/notify 等 JFTrade 内部 helper 或 ta.adx shortcut 时，AnalyzeScript 返回稳定分码诊断并提示 Pine v6 native 替代写法；Monaco 不暴露这些 internal helper 作为 public completion/hover。 |
| v3.2 MTF diagnostics and lower-timeframe preflight | yes | yes | yes | yes | no | request.security 固定 timeframe requirements 会在 warmup、indicator engine 和 backtest replay 前与策略原生 interval 比较；低于原生周期或不能整除的 intraday timeframe 返回明确错误，不进入 runtime 执行。AnalyzeScript 对 tuple assignment、tuple width、alias mismatch 和无法 lower 的纯表达式返回稳定分码诊断。 |
| v3.3 advanced language boundary diagnostics | yes | yes | yes | yes | yes | AnalyzeScript 对递归 UDF、嵌套 UDF、UDF 签名问题、循环嵌套/迭代上限和循环变量只读返回稳定分码诊断；动态 for/while、collection for、break/continue 和 loop runtime 上限继续作为闭盘可执行子集的受控边界。 |
| v3.4 generated support snapshot | yes | yes | yes | yes | yes | npm run generate:reference 生成 docs/reference/generated/pine-v6-support.md，将 ProductVersion、score model、compatibility dimensions、capability registry、support matrix 和 unsupported patterns 固化为可 diff 快照；pinespec 测试会拒绝过期快照。 |
| v4.0 broker emulator boundary decision | yes | yes | yes | yes | yes | 完整 TradingView broker emulator、OCA、partial fill、intrabar tick recalculation 和多标的组合撮合正式作为单独 trading-runtime parity track，排除在 JFTrade executable Pine v6 completion score 之外；brokerBoundary payload 与生成快照列出 scoreTreatment 和稳定诊断码。 |

## Broker Boundary

| Area | Status | Score Treatment | Diagnostics | Notes |
| --- | --- | --- | --- | --- |
| Closed-bar order model | `supported` | included in executable Pine v6 score |  | strategy.entry/order/close/close_all/exit/cancel 在 K 线收盘执行；stop-limit、bracket、trailing、reversal、allow_entry_in、commission、slippage 和 process_orders_on_close 有专门可执行测试。 |
| OCA and partial fill | `out_of_scope` | excluded from executable Pine v6 score and listed as unsupported order capability | `PINE_ORDER_OCA_UNSUPPORTED` | oca_name/oca_type、partial fill 和 OCA reduce/cancel 组合属于 TradingView broker-emulator parity track，不计入 JFTrade closed-bar Pine completion。 |
| Intrabar tick recalculation | `out_of_scope` | excluded from executable Pine v6 score and listed as unsupported order capability | `PINE_BROKER_EMULATOR_OUT_OF_SCOPE` | tick 级重算、intrabar path 推断、bar magnifier 和同一根 K 线内部成交路径不属于当前 runtime；当前策略只在闭盘 hook 执行。 |
| Advanced strategy.exit broker semantics | `diagnostic_only` | supported subset counted; unsupported combinations stay outside score | `PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED`<br>`PINE_ORDER_EXIT_ADVANCED_UNSUPPORTED` | 基础 stop、limit、stop+limit bracket、trail_points/trail_price + trail_offset 可执行；trail 与 bracket 混用、无触发器 exit 和高级 broker emulator 语义返回稳定诊断。 |
| Full TradingView broker emulator | `out_of_scope` | tracked separately as order.full_tv_broker_emulator, not used to inflate Pine language completion | `PINE_BROKER_EMULATOR_OUT_OF_SCOPE` | 完整 TradingView broker emulator、保证金清算、多标的组合撮合和 partial fill parity 需要单独 trading-runtime track；v4.0 正式将其排除在 JFTrade executable Pine v6 completion 之外。 |

## Unsupported Patterns

- indicator()、study()、library() 脚本不能作为 JFTrade 可执行策略。
- request.security() 仅支持 syminfo.tickerid + 静态 timeframe + source/source[n]、受支持 MA/高级指标、v1.4 纯表达式、v1.5 common TA pure-expression、v1.6 tuple 白名单、v2.2 2-8 元纯表达式 tuple、v2.3 纯 collection/object 表达式、v2.4 MTF stoch、v2.7 helper 表达式、v2.8 object method 表达式与 v2.9 object history field/method 表达式；动态参数、side effect、nested request、lookahead_on/gaps_on 会返回分码诊断。
- array/map/matrix 常用 constructor/read/mutation/copy/slice/fill/aggregate/sort/stats/map views、array for-in、map keys/values iteration、matrix rows/columns/get/set、只读 collection history aggregate 与 object collection fields 已执行；深层泛型与全部 Pine collection API 仍会返回诊断。
- type constructor、命名参数、多语句纯 method、局部/持久 object 字段重赋值、object collection fields、object history read 与纯 method chain 子集已执行；library/import、method 副作用、完整 overload/type system 与跨 library 解析仍只进入诊断或返回不支持；export 进入 function/type/method kind metadata。
- 静态 for 循环会在编译期展开；v1.5+ 支持无条件 break/continue 子集，v2.4 起条件 break/continue 回退到 bounded runtime loop；超过 100 次静态展开和超过 2 层嵌套会返回明确诊断。
- 表达式 UDF 与受控多语句函数支持编译期内联；递归函数、嵌套定义、method/type 会进入 parse-only 语义模型并返回明确诊断。
- 历史引用支持简单 identifier/member 的 `[n]`，最大 lookback 500；函数调用结果需先赋值再引用历史。
- strategy.exit() 支持基础 stop、limit、stop+limit bracket 与 trail_points|trail_price + trail_offset；trail 与 stop/limit 同用、OCA、partial fill、intrabar broker emulator 等高级语义暂不支持。
- strategy.entry/order 支持 stop-limit 激活后转限价；OCA、strategy.cancel 已成交订单等完整 broker emulator 语义暂不支持。
- plot/hline/bgcolor/barcolor/fill/alertcondition/label.new/line.new/box.new/table.* 等非交易调用会被解析为 warning 并忽略。
- 除文档列出的 ta.*、input.*、math.*、strategy.entry、strategy.close、alert/log 外的 built-ins 不应假定可执行。
