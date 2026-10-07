# Go 与 Rust 测试用例全景对齐审计报告

本报告由 `scripts/compatibility/audit_test_parity.py` 自动扫描生成。

## 1. 总体概况

- **Go 分支（`go:452dea11`）测试总数**：4451
- **Go 高风险测试用例数**（涉及分页、缓存、时区、对账、断连重连等）：952
- **Rust 当前测试总数**：3609
- **总体测试数量比（非覆盖率）**：81.1%
- **Rust 基线（`8846e40d`）**：当前工作树

- **同名 Go 测试组**：32（映射必须使用文件路径与行号，不能仅按测试名）
## 2. 分领域对齐矩阵

| 业务领域 | Go 测试数 | Go 高风险数 | Rust 测试数 | 测试数量比（非覆盖率） |
| :--- | :--- | :--- | :--- | :--- |
| Futu / OpenD Protocol & Integration | 524 | 152 | 570 | 108.8% |
| MarketData / Quotes & Providers | 256 | 74 | 403 | 157.4% |
| Trading & Broker Execution | 138 | 36 | 482 | 349.3% |
| Strategy & Pine Runtime | 538 | 82 | 302 | 56.1% |
| Backtest & Exchange Calendar | 376 | 101 | 269 | 71.5% |
| Assistant & Workflow ADK | 810 | 213 | 460 | 56.8% |
| Storage & SQLite Persistence | 228 | 39 | 226 | 99.1% |
| Settings & Watchlist | 63 | 18 | 109 | 173.0% |
| API Server & Transport Wire | 951 | 163 | 136 | 14.3% |
| Other / Tooling / Core | 567 | 74 | 652 | 115.0% |

> 注意：测试数量比只表示数量关系，不证明行为等价；行为证据以逐项清单中的 `evidence_type` 为准。

## 3. 高风险待对齐用例采样（各领域 Top 10）

### Futu / OpenD Protocol & Integration

- `internal/integration/futu/candle_sessions_test.go:11 TestMarketSessionsForCandleSessions`
- `internal/integration/futu/marketdata_runtime_opend_test.go:226 TestMarketDataRuntimePreservesRealtimeTicksWhenDelayedFallbackFails`
- `internal/integration/futu/marketdata_runtime_test.go:267 TestTickConversionRejectsUnusablePricesAndUsesQuoteFallbacks`
- `internal/integration/futu/marketdata_runtime_test.go:364 TestTickFromTradeInheritsLatestQuoteFieldsThroughCache`
- `internal/integration/futu/marketdata_runtime_test.go:452 TestMarketDataRuntimeExchangeResetAndStreamLifecycle`
- `internal/integration/futu/marketdata_runtime_test.go:613 TestMarketDataRuntimeFiltersFallbackInstrumentsFromPushStream`
- `internal/integration/futu/marketdata_runtime_test.go:824 TestFallbackTickerMapProjectsOnlyRequestedUsableSnapshots`
- `internal/integration/futu/marketdata_runtime_test.go:869 TestFallbackSnapshotConversionRejectsInvalidValuesAndUsesClassification`
- `internal/integration/futu/notifications_test.go:13 TestLiveNotificationFromResponseRoutesProtocolPayloadsToNeutralCategories`
- `internal/integration/futu/notifications_test.go:98 TestNeutralNotificationBuildersHandleNilAndStatusTransitions`

### MarketData / Quotes & Providers

- `internal/marketdata/broker_candles_test.go:12 TestBrokerKLineCandlesResponseProjectsStrictPage`
- `internal/marketdata/broker_candles_test.go:51 TestBrokerKLineCandlesResponseHandlesTerminalAndBoundedPages`
- `internal/marketdata/broker_candles_test.go:109 TestBrokerKLineHelpersClassifySessionsAndNumbers`
- `internal/marketdata/broker_candles_test.go:137 TestBrokerKLinePaginationRejectsInvalidBoundedAndPagedMetadata`
- `internal/marketdata/cache_test.go:12 TestCacheDeduplicatesPromotesAndInherits`
- `internal/marketdata/cache_test.go:76 TestCacheFreshnessRetentionAndMaximum`
- `internal/marketdata/cache_test.go:103 TestCacheDoesNotInheritExtendedSessionsAcrossTradingDays`
- `internal/marketdata/cache_test.go:138 TestCachePromotesUSRegularCloseWhenAfterHoursTradeArrives`
- `internal/marketdata/cache_test.go:185 TestCacheRetainsNewExtendedQuoteWhenPriceIsUnchanged`
- `internal/marketdata/cache_test.go:210 TestTickCandlesVolumeWindowAndLimit`

### Trading & Broker Execution

- `internal/trading/broker_boundaries_test.go:11 TestServiceBrokerReadOperationsReturnFallbackWhenMarketDataUnavailable`
- `internal/trading/broker_conformance_test.go:58 TestFakeBrokerConformanceCancelAcceptedAndCancelRejected`
- `internal/trading/broker_test.go:453 TestServicePortfolioAndFallbackResponses`
- `internal/trading/broker_test.go:533 TestServiceBrokerWriteAndTimeoutBehaviors`
- `internal/trading/control_plane_idempotency_test.go:65 TestRealTradeControlPlaneHardStopReleaseIsSingleShot`
- `internal/trading/control_plane_idempotency_test.go:99 TestRealTradeControlPlaneHardStopsBlockUntilEveryEntryReleased`
- `internal/trading/control_plane_state_audit_test.go:99 TestControlPlaneTreatsEmptyStateAsFreshAndRejectsUnavailableMutations`
- `internal/trading/control_plane_state_audit_test.go:250 TestControlPlaneSurfacesHardStopRejectionAuditPersistenceFailure`
- `internal/trading/execution_combo_lifecycle_test.go:15 TestExecutionComboCompletePreviewPlaceCancelAndBuyingPower`
- `internal/trading/execution_combo_lifecycle_test.go:635 TestExecutionDetailsResolverAndOrderUpdateCacheFailureBranches`

### Strategy & Pine Runtime

- `internal/strategy/catalog/activity_degraded_test.go:65 TestCatalogActivityReturnsEmptyPagesWhenActivityStoreIsUnavailable`
- `internal/strategy/catalog/catalog_boundary_behavior_test.go:34 TestCatalogActivityQueryFailureReturnsKnownEmptyPage`
- `internal/strategy/catalog/catalog_boundary_behavior_test.go:192 TestCatalogPrivateBusinessHelpersHandleEmptyAndUnknownInputs`
- `internal/strategy/catalog/runtime_reconciliation_business_test.go:12 TestCatalogRuntimeTransitionsPersistStateAndActivity`
- `internal/strategy/catalog/runtime_reconciliation_business_test.go:52 TestCatalogRuntimeFailureReconcilesOnlyRunningInstance`
- `internal/strategy/catalog/runtime_reconciliation_business_test.go:80 TestCatalogStartupReconcileResetsStaleRunningAndPausedState`
- `internal/strategy/catalog/runtime_reconciliation_business_test.go:113 TestCatalogActivitySupportsPagingFilteringAndRuntimeObservationEnrichment`
- `internal/strategy/errors_test.go:8 TestClassifiedStrategyErrorsMatchSentinelKinds`
- `internal/strategy/instancebinding/binding_test.go:137 TestNormalizeBrokerAccountDropsEmptyInput`
- `internal/strategy/live_command_business_boundaries_test.go:468 TestIgnoredOrderWarningsRetainFallbackIdentityAndSymbol`

### Backtest & Exchange Calendar

- `internal/backtest/historical_source_test.go:111 TestHistoricalKLineSyncerCancelsInFlightProviderPage`
- `internal/backtest/historical_source_test.go:147 TestHistoricalKLineSyncerRetriesTransientPageAndRejectsCapabilitiesDuringPreflight`
- `internal/backtest/historical_source_test.go:181 TestHistoricalKLineSyncerRejectsEmptyProviderResult`
- `internal/backtest/historical_source_test.go:268 TestHistoricalKLineSyncerRejectsBrokenPagination`
- `internal/backtest/historical_source_test.go:341 TestHistoricalProviderRetryExhaustionAndTimerCancellation`
- `internal/backtest/recovery_test.go:11 TestBacktestExecutionPersistsFailureWhenRunnerReturnsNil`
- `internal/backtest/recovery_test.go:27 TestBacktestExecutionRecoversRunnerPanicIntoFailedRun`
- `internal/backtest/recovery_test.go:45 TestStartScriptRejectsBlankResearchScript`
- `internal/backtest/result_view_test.go:223 TestResultViewRejectsBadRequestsAndPreservesEmptyRunShape`
- `internal/backtest/run_failure_recovery_test.go:26 TestBacktestStartDoesNotLeakLifecycleTaskWhenQueuePersistenceFails`

### Assistant & Workflow ADK

- `internal/assistant/assembly/adk_strategy_test.go:194 TestADKStrategyToolsHandleNegativeAndFallbackScenarios`
- `internal/assistant/assembly/adk_strategy_test.go:605 TestADKStrategyOptimizePersistsTasksAndCancelsQueuedRunsOnFailure`
- `internal/assistant/assembly/application_adapter_test.go:226 TestApplicationAdapterProvidesScreenCatalogAndCancelResult`
- `internal/assistant/assembly/mcp_server_test.go:76 TestMCPServerManagerStartsAndStopsOnLoopback`
- `internal/assistant/assembly/mcp_server_test.go:106 TestMCPServerManagerServesAuthenticatedStreamableMCP`
- `internal/assistant/assembly/portfolio_tools_test.go:15 TestPortfolioSummaryScansAllRealAccountsAndRanksNonEmptyFirst`
- `internal/assistant/assembly/portfolio_tools_test.go:155 TestPortfolioLayeredToolsReportValidationDiscoveryAndPartialReadStates`
- `internal/assistant/assembly/product_adapters_test.go:186 TestProductExecutionAdapterRejectsInvalidScreenPageAndValue`
- `internal/assistant/assembly/runtime_test.go:14 TestOpenBuildsToolsServiceAndIdempotentLifecycle`
- `internal/assistant/assembly/workflow_tools_error_boundaries_test.go:103 TestWorkflowToolsRemainingSessionAndPayloadErrors`

### Storage & SQLite Persistence

- `internal/store/backtest/store_failure_test.go:121 TestStoreCanceledMaintenanceDoesNotMutateRuns`
- `internal/store/backtest/store_test.go:168 TestInMemoryStoreImplementsRunLifecycleAndCancellation`
- `internal/store/backtest/sync_tasks_test.go:12 TestSyncTaskStoreReturnsSnapshotsAndCancelsProgress`
- `internal/store/exchangecalendar/store_boundaries_test.go:45 TestCalendarStoreEmptyLoadAndDeleteAreIdempotent`
- `internal/store/exchangecalendar/store_snapshot_failures_test.go:95 TestSaveSnapshotValidatesInputsAndResolvesYearFallbacks`
- `internal/store/settingsfile/normalization_and_persistence_test.go:13 TestSettingsNormalizationHandlesFallbacksAndBoundaries`
- `internal/store/settingsfile/rollback_test.go:13 TestFailedSettingSavesRollbackAllRuntimeState`
- `internal/store/settingsfile/rollback_test.go:185 TestFailedBootstrapAndMigrationRollbackRuntimeState`
- `internal/store/settingsfile/rollback_test.go:232 TestFailedManagedAccountCRUDRollsBackBackingArray`
- `internal/store/settingsfile/store_recovery_test.go:13 TestSettingsStoreRejectsMalformedOrUnreadableInput`

### Settings & Watchlist

- `internal/settings/market_data_test.go:203 TestMarketDataProviderRuntimeRollback`
- `internal/settings/market_data_test.go:224 TestMarketDataProviderReportsPersistenceAndRollbackFailures`
- `internal/settings/market_data_test.go:252 TestMarketDataProviderReadsWaitForRuntimeRollback`
- `internal/settings/persistence_and_mcp_failures_test.go:125 TestServicePreservesSecurityAndMCPFallbacks`
- `internal/settings/service_test.go:435 TestDefaultCallbacksReturnEmptyMaps`
- `internal/watchlist/futu/source_test.go:44 TestFutuWatchlistReaderMarksDuplicateNamesAmbiguousAndCachesReads`
- `internal/watchlist/futu/source_test.go:174 TestFutuWatchlistSnapshotDoesNotSplitGlobalOrCanceledFailures`
- `internal/watchlist/futu/source_test.go:221 TestFutuWatchlistSnapshotUsesDelayedFallbackWhenSubscriptionQuotaIsFull`
- `internal/watchlist/futu/source_test.go:396 TestWatchlistQuotePreservesSnapshotDisplayMetadataAndAvoidsUnknownTimezoneGuess`
- `internal/watchlist/futu/source_test.go:418 TestWatchlistQuoteSelectsExtendedSessionPriceAndChange`

### API Server & Transport Wire

- `cmd/jftrade-api/main_test.go:86 TestRunAPICommandStartsAndStopsAPI`
- `cmd/jftrade-api/main_test.go:121 TestRunAPICommandPreservesConfiguredCacheAndWrapsStartupErrors`
- `internal/api/assistant/adk_approval_test.go:335 TestADKRunCancelAndFilteredList`
- `internal/api/assistant/adk_normalize_test.go:15 TestADKRoutesSerializeEmptySlicesAsArrays`
- `internal/api/assistant/adk_ops_test.go:247 TestADKOptimizationTaskCanBeQueriedAndCancelled`
- `internal/api/assistant/adk_routes_test.go:26 TestADKSessionDetailOmitsResolvedApprovalGroups`
- `internal/api/assistant/adk_routes_test.go:214 TestADKAuditRouteRejectsInvalidPagination`
- `internal/api/assistant/adk_routes_test.go:245 TestADKChatStreamEmitsSessionRunAndFinalEvents`
- `internal/api/assistant/adk_routes_test.go:597 TestADKProviderSaveReturnsRequestTimeoutMs`
- `internal/api/assistant/adk_routes_test.go:787 TestADKSessionNegativeRoutes`

### Other / Tooling / Core

- `cmd/check-go-coverage/changed_lines_analysis_test.go:71 TestParseChangedGoLinesReportsPureRenameWithoutInventingChangedStatements`
- `cmd/check-go-coverage/profile_analysis_test.go:109 TestAnalyzeProfilesRejectsEmptyBusinessCoverage`
- `cmd/jftrade-desktop/desktop_startup_test.go:93 TestDesktopShutdownCancelsStartupAndReclaimsLateResources`
- `cmd/jftrade-desktop/main_test.go:131 TestDesktopAssetHandlerDoesNotFallbackForMissingStaticAsset`
- `cmd/jftrade-desktop/main_test.go:395 TestListDesktopLogDaysAndReadsFilteredPage`
- `cmd/jftrade-desktop/main_test.go:426 TestDesktopLogPageCapsLimitAndPaginatesAllLines`
- `cmd/jftrade-desktop/main_test.go:464 TestDesktopLogPageTailOffsetReturnsLastPageInFileOrder`
- `cmd/jftrade-desktop/main_test.go:506 TestDesktopLogPageTailOffsetAppliesFiltersBeforePaging`
- `cmd/jftrade-desktop/main_test.go:523 TestListDesktopLogDaysMissingDirReturnsEmpty`
- `internal/datamanagement/service_test.go:44 TestServiceFallbacks`

