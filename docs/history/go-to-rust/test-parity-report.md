# Go 与 Rust 测试用例全景对齐审计报告

本报告由 `scripts/compatibility/audit_test_parity.py` 自动扫描生成。

## 1. 总体概况

- **Go 分支（`go:452dea11`）测试总数**：4451
- **Go 高风险测试用例数**（涉及分页、缓存、时区、对账、断连重连等）：952
- **Rust 当前测试总数**：1647
- **总体测试覆盖比率**：37.0%

## 2. 分领域对齐矩阵

| 业务领域 | Go 测试数 | Go 高风险数 | Rust 测试数 | 迁移比例 |
| :--- | :--- | :--- | :--- | :--- |
| Futu / OpenD Protocol & Integration | 524 | 152 | 216 | 41.2% |
| MarketData / Quotes & Providers | 214 | 86 | 189 | 88.3% |
| Trading & Broker Execution | 138 | 36 | 252 | 182.6% |
| Strategy & Pine Runtime | 545 | 83 | 137 | 25.1% |
| Backtest & Exchange Calendar | 304 | 80 | 123 | 40.5% |
| Assistant & Workflow ADK | 810 | 213 | 171 | 21.1% |
| Storage & SQLite Persistence | 228 | 39 | 129 | 56.6% |
| Settings & Watchlist | 63 | 18 | 69 | 109.5% |
| API Server & Transport Wire | 951 | 163 | 31 | 3.3% |
| Other / Tooling / Core | 674 | 82 | 330 | 49.0% |

## 3. 高风险待对齐用例采样（各领域 Top 10）

### Futu / OpenD Protocol & Integration

- [x] `internal/integration/futu/candle_sessions_test.go:11 TestMarketSessionsForCandleSessions`
- [x] `internal/integration/futu/marketdata_runtime_opend_test.go:226 TestMarketDataRuntimePreservesRealtimeTicksWhenDelayedFallbackFails`
- [x] `internal/integration/futu/marketdata_runtime_test.go:267 TestTickConversionRejectsUnusablePricesAndUsesQuoteFallbacks`
- [x] `internal/integration/futu/marketdata_runtime_test.go:364 TestTickFromTradeInheritsLatestQuoteFieldsThroughCache`
- [x] `internal/integration/futu/marketdata_runtime_test.go:452 TestMarketDataRuntimeExchangeResetAndStreamLifecycle`
- [x] `internal/integration/futu/marketdata_runtime_test.go:613 TestMarketDataRuntimeFiltersFallbackInstrumentsFromPushStream`
- [x] `internal/integration/futu/marketdata_runtime_test.go:824 TestFallbackTickerMapProjectsOnlyRequestedUsableSnapshots`
- [x] `internal/integration/futu/marketdata_runtime_test.go:869 TestFallbackSnapshotConversionRejectsInvalidValuesAndUsesClassification`
- [x] `internal/integration/futu/notifications_test.go:13 TestLiveNotificationFromResponseRoutesProtocolPayloadsToNeutralCategories`
- [x] `internal/integration/futu/notifications_test.go:98 TestNeutralNotificationBuildersHandleNilAndStatusTransitions`

### MarketData / Quotes & Providers

- [x] `internal/marketdata/broker_candles_test.go:12 TestBrokerKLineCandlesResponseProjectsStrictPage`
- [x] `internal/marketdata/broker_candles_test.go:51 TestBrokerKLineCandlesResponseHandlesTerminalAndBoundedPages`
- [x] `internal/marketdata/broker_candles_test.go:109 TestBrokerKLineHelpersClassifySessionsAndNumbers`
- [x] `internal/marketdata/broker_candles_test.go:137 TestBrokerKLinePaginationRejectsInvalidBoundedAndPagedMetadata`
- [x] `internal/marketdata/cache_test.go:12 TestCacheDeduplicatesPromotesAndInherits`
- [x] `internal/marketdata/cache_test.go:76 TestCacheFreshnessRetentionAndMaximum`
- [x] `internal/marketdata/cache_test.go:103 TestCacheDoesNotInheritExtendedSessionsAcrossTradingDays`
- [x] `internal/marketdata/cache_test.go:138 TestCachePromotesUSRegularCloseWhenAfterHoursTradeArrives`
- [x] `internal/marketdata/cache_test.go:185 TestCacheRetainsNewExtendedQuoteWhenPriceIsUnchanged`
- [x] `internal/marketdata/cache_test.go:210 TestTickCandlesVolumeWindowAndLimit`

### Trading & Broker Execution

- [x] `internal/trading/broker_boundaries_test.go:11 TestServiceBrokerReadOperationsReturnFallbackWhenMarketDataUnavailable`
- [x] `internal/trading/broker_conformance_test.go:58 TestFakeBrokerConformanceCancelAcceptedAndCancelRejected`
- [x] `internal/trading/broker_test.go:453 TestServicePortfolioAndFallbackResponses`
- [x] `internal/trading/broker_test.go:533 TestServiceBrokerWriteAndTimeoutBehaviors`
- [x] `internal/trading/control_plane_idempotency_test.go:65 TestRealTradeControlPlaneHardStopReleaseIsSingleShot`
- [x] `internal/trading/control_plane_idempotency_test.go:99 TestRealTradeControlPlaneHardStopsBlockUntilEveryEntryReleased`
- [x] `internal/trading/control_plane_state_audit_test.go:99 TestControlPlaneTreatsEmptyStateAsFreshAndRejectsUnavailableMutations`
- [x] `internal/trading/control_plane_state_audit_test.go:250 TestControlPlaneSurfacesHardStopRejectionAuditPersistenceFailure`
- [x] `internal/trading/execution_combo_lifecycle_test.go:15 TestExecutionComboCompletePreviewPlaceCancelAndBuyingPower`
- [x] `internal/trading/execution_combo_lifecycle_test.go:635 TestExecutionDetailsResolverAndOrderUpdateCacheFailureBranches`

### Strategy & Pine Runtime

- [x] `internal/pineworkerassets/asset_selection_boundaries_test.go:30 TestSelectFromFSTreatsMissingAndEmptyBundlesAsUnavailable`
- [x] `internal/strategy/catalog/activity_degraded_test.go:65 TestCatalogActivityReturnsEmptyPagesWhenActivityStoreIsUnavailable`
- [x] `internal/strategy/catalog/catalog_boundary_behavior_test.go:34 TestCatalogActivityQueryFailureReturnsKnownEmptyPage`
- [x] `internal/strategy/catalog/catalog_boundary_behavior_test.go:192 TestCatalogPrivateBusinessHelpersHandleEmptyAndUnknownInputs`
- [x] `internal/strategy/catalog/runtime_reconciliation_business_test.go:12 TestCatalogRuntimeTransitionsPersistStateAndActivity`
- [x] `internal/strategy/catalog/runtime_reconciliation_business_test.go:52 TestCatalogRuntimeFailureReconcilesOnlyRunningInstance`
- [x] `internal/strategy/catalog/runtime_reconciliation_business_test.go:80 TestCatalogStartupReconcileResetsStaleRunningAndPausedState`
- [x] `internal/strategy/catalog/runtime_reconciliation_business_test.go:113 TestCatalogActivitySupportsPagingFilteringAndRuntimeObservationEnrichment`
- [x] `internal/strategy/errors_test.go:8 TestClassifiedStrategyErrorsMatchSentinelKinds`
- [x] `internal/strategy/instancebinding/binding_test.go:137 TestNormalizeBrokerAccountDropsEmptyInput`

### Backtest & Exchange Calendar

- [x] `internal/backtest/historical_source_test.go:111 TestHistoricalKLineSyncerCancelsInFlightProviderPage`
- [x] `internal/backtest/historical_source_test.go:147 TestHistoricalKLineSyncerRetriesTransientPageAndRejectsCapabilitiesDuringPreflight`
- [x] `internal/backtest/historical_source_test.go:181 TestHistoricalKLineSyncerRejectsEmptyProviderResult`
- [x] `internal/backtest/historical_source_test.go:268 TestHistoricalKLineSyncerRejectsBrokenPagination`
- [x] `internal/backtest/historical_source_test.go:341 TestHistoricalProviderRetryExhaustionAndTimerCancellation`
- [x] `internal/backtest/recovery_test.go:11 TestBacktestExecutionPersistsFailureWhenRunnerReturnsNil`
- [x] `internal/backtest/recovery_test.go:27 TestBacktestExecutionRecoversRunnerPanicIntoFailedRun`
- [x] `internal/backtest/recovery_test.go:45 TestStartScriptRejectsBlankResearchScript`
- [x] `internal/backtest/result_view_test.go:223 TestResultViewRejectsBadRequestsAndPreservesEmptyRunShape`
- [x] `internal/backtest/run_failure_recovery_test.go:26 TestBacktestStartDoesNotLeakLifecycleTaskWhenQueuePersistenceFails`

### Assistant & Workflow ADK

- [x] `internal/assistant/assembly/adk_strategy_test.go:194 TestADKStrategyToolsHandleNegativeAndFallbackScenarios`
- [x] `internal/assistant/assembly/adk_strategy_test.go:605 TestADKStrategyOptimizePersistsTasksAndCancelsQueuedRunsOnFailure`
- [x] `internal/assistant/assembly/application_adapter_test.go:226 TestApplicationAdapterProvidesScreenCatalogAndCancelResult`
- [x] `internal/assistant/assembly/mcp_server_test.go:76 TestMCPServerManagerStartsAndStopsOnLoopback`
- [x] `internal/assistant/assembly/mcp_server_test.go:106 TestMCPServerManagerServesAuthenticatedStreamableMCP`
- [x] `internal/assistant/assembly/portfolio_tools_test.go:15 TestPortfolioSummaryScansAllRealAccountsAndRanksNonEmptyFirst`
- [x] `internal/assistant/assembly/portfolio_tools_test.go:155 TestPortfolioLayeredToolsReportValidationDiscoveryAndPartialReadStates`
- [x] `internal/assistant/assembly/product_adapters_test.go:186 TestProductExecutionAdapterRejectsInvalidScreenPageAndValue`
- [x] `internal/assistant/assembly/runtime_test.go:14 TestOpenBuildsToolsServiceAndIdempotentLifecycle`
- [x] `internal/assistant/assembly/workflow_tools_error_boundaries_test.go:103 TestWorkflowToolsRemainingSessionAndPayloadErrors`

### Storage & SQLite Persistence

- [x] `internal/store/backtest/store_failure_test.go:121 TestStoreCanceledMaintenanceDoesNotMutateRuns`
- [x] `internal/store/backtest/store_test.go:168 TestInMemoryStoreImplementsRunLifecycleAndCancellation`
- [x] `internal/store/backtest/sync_tasks_test.go:12 TestSyncTaskStoreReturnsSnapshotsAndCancelsProgress`
- [x] `internal/store/exchangecalendar/store_boundaries_test.go:45 TestCalendarStoreEmptyLoadAndDeleteAreIdempotent`
- [x] `internal/store/exchangecalendar/store_snapshot_failures_test.go:95 TestSaveSnapshotValidatesInputsAndResolvesYearFallbacks`
- [x] `internal/store/settingsfile/normalization_and_persistence_test.go:13 TestSettingsNormalizationHandlesFallbacksAndBoundaries`
- [x] `internal/store/settingsfile/rollback_test.go:13 TestFailedSettingSavesRollbackAllRuntimeState`
- [x] `internal/store/settingsfile/rollback_test.go:185 TestFailedBootstrapAndMigrationRollbackRuntimeState`
- [x] `internal/store/settingsfile/rollback_test.go:232 TestFailedManagedAccountCRUDRollsBackBackingArray`
- [x] `internal/store/settingsfile/store_recovery_test.go:13 TestSettingsStoreRejectsMalformedOrUnreadableInput`

### Settings & Watchlist

- [x] `internal/settings/market_data_test.go:203 TestMarketDataProviderRuntimeRollback`
- [x] `internal/settings/market_data_test.go:224 TestMarketDataProviderReportsPersistenceAndRollbackFailures`
- [x] `internal/settings/market_data_test.go:252 TestMarketDataProviderReadsWaitForRuntimeRollback`
- [x] `internal/settings/persistence_and_mcp_failures_test.go:125 TestServicePreservesSecurityAndMCPFallbacks`
- [x] `internal/settings/service_test.go:435 TestDefaultCallbacksReturnEmptyMaps`
- [x] `internal/watchlist/futu/source_test.go:44 TestFutuWatchlistReaderMarksDuplicateNamesAmbiguousAndCachesReads`
- [x] `internal/watchlist/futu/source_test.go:174 TestFutuWatchlistSnapshotDoesNotSplitGlobalOrCanceledFailures`
- [x] `internal/watchlist/futu/source_test.go:221 TestFutuWatchlistSnapshotUsesDelayedFallbackWhenSubscriptionQuotaIsFull`
- [x] `internal/watchlist/futu/source_test.go:396 TestWatchlistQuotePreservesSnapshotDisplayMetadataAndAvoidsUnknownTimezoneGuess`
- [x] `internal/watchlist/futu/source_test.go:418 TestWatchlistQuoteSelectsExtendedSessionPriceAndChange`

### API Server & Transport Wire

- [x] `cmd/jftrade-api/main_test.go:86 TestRunAPICommandStartsAndStopsAPI`
- [x] `cmd/jftrade-api/main_test.go:121 TestRunAPICommandPreservesConfiguredCacheAndWrapsStartupErrors`
- [x] `internal/api/assistant/adk_approval_test.go:335 TestADKRunCancelAndFilteredList`
- [x] `internal/api/assistant/adk_normalize_test.go:15 TestADKRoutesSerializeEmptySlicesAsArrays`
- [x] `internal/api/assistant/adk_ops_test.go:247 TestADKOptimizationTaskCanBeQueriedAndCancelled`
- [x] `internal/api/assistant/adk_routes_test.go:26 TestADKSessionDetailOmitsResolvedApprovalGroups`
- [x] `internal/api/assistant/adk_routes_test.go:214 TestADKAuditRouteRejectsInvalidPagination`
- [x] `internal/api/assistant/adk_routes_test.go:245 TestADKChatStreamEmitsSessionRunAndFinalEvents`
- [x] `internal/api/assistant/adk_routes_test.go:597 TestADKProviderSaveReturnsRequestTimeoutMs`
- [x] `internal/api/assistant/adk_routes_test.go:787 TestADKSessionNegativeRoutes`

### Other / Tooling / Core

- [x] `cmd/check-go-coverage/changed_lines_analysis_test.go:71 TestParseChangedGoLinesReportsPureRenameWithoutInventingChangedStatements`
- [x] `cmd/check-go-coverage/profile_analysis_test.go:109 TestAnalyzeProfilesRejectsEmptyBusinessCoverage`
- [x] `cmd/jftrade-desktop/desktop_startup_test.go:93 TestDesktopShutdownCancelsStartupAndReclaimsLateResources`
- [x] `cmd/jftrade-desktop/main_test.go:131 TestDesktopAssetHandlerDoesNotFallbackForMissingStaticAsset`
- [x] `cmd/jftrade-desktop/main_test.go:395 TestListDesktopLogDaysAndReadsFilteredPage`
- [x] `cmd/jftrade-desktop/main_test.go:426 TestDesktopLogPageCapsLimitAndPaginatesAllLines`
- [x] `cmd/jftrade-desktop/main_test.go:464 TestDesktopLogPageTailOffsetReturnsLastPageInFileOrder`
- [x] `cmd/jftrade-desktop/main_test.go:506 TestDesktopLogPageTailOffsetAppliesFiltersBeforePaging`
- [x] `cmd/jftrade-desktop/main_test.go:523 TestListDesktopLogDaysMissingDirReturnsEmpty`
- [x] `internal/datamanagement/service_test.go:44 TestServiceFallbacks`

