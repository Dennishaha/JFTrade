# Go → Rust 全量测试索引

本文件由 `scripts/compatibility/audit_test_parity.py` 生成；状态仅表示自动映射候选，行为等价性以高价值清单和回归证据为准。

| 状态 | Go 测试 | 业务域 | 风险 | Rust crate 候选 |
|---|---|---|---|---|
| [ ] | `go:452dea11:cmd/jftrade-api/main_test.go:13`<br>`TestValidateArgsAllowsNoArgs` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:cmd/jftrade-api/main_test.go:19`<br>`TestValidateArgsRejectsLegacySubcommands` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:cmd/jftrade-api/main_test.go:31`<br>`TestIsHelpArgs` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:cmd/jftrade-api/main_test.go:46`<br>`TestRunAPICommandPrintsUsageWithoutStartingTheServer` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:cmd/jftrade-api/main_test.go:69`<br>`TestRunAPICommandRejectsUnsupportedArgsBeforeStartingTheServer` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:cmd/jftrade-api/main_test.go:86`<br>`TestRunAPICommandStartsAndStopsAPI` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:cmd/jftrade-api/main_test.go:121`<br>`TestRunAPICommandPreservesConfiguredCacheAndWrapsStartupErrors` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:cmd/jftrade-api/main_test.go:142`<br>`TestRunAPICommandContinuesAfterBestEffortEnvironmentFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:cmd/jftrade-api/main_test.go:160`<br>`TestMainDelegatesToCommandRunner` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:cmd/jftrade-api/main_test.go:191`<br>`TestReportFatalForwardsErrorsAndIgnoresNil` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_approval_test.go:16`<br>`TestADKApprovalApproveRouteReturnsRunningResolutionEnvelope` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_approval_test.go:183`<br>`TestADKApprovalRouteReturnsResolutionEnvelope` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_approval_test.go:282`<br>`TestADKProviderDeleteRejectsReferencedProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_approval_test.go:335`<br>`TestADKRunCancelAndFilteredList` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_approval_test.go:382`<br>`TestADKRunPauseAndResumeRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_approval_test.go:450`<br>`TestADKRunPauseResumeRoutesRejectInvalidRuns` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_integration_test.go:20`<br>`TestRealADKChatStreamWithSavedProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_normalize_test.go:15`<br>`TestADKRoutesSerializeEmptySlicesAsArrays` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_ops_test.go:18`<br>`TestADKMetricsExposeLifecycleAndApprovalLatency` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_ops_test.go:216`<br>`TestADKMetricsIgnoresUnexpectedQueryParams` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_ops_test.go:247`<br>`TestADKOptimizationTaskCanBeQueriedAndCancelled` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_ops_test.go:286`<br>`TestADKTaskAndMemoryWorkflowRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_ops_test.go:394`<br>`TestADKOptimizationTaskNegativeRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_ops_test.go:448`<br>`TestAssistantChatCompatibilityRouteIsGone` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_ops_test.go:468`<br>`TestADKSnapshotAndToolsRoutesReturnCatalogData` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:26`<br>`TestADKSessionDetailOmitsResolvedApprovalGroups` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:165`<br>`TestADKAuditRouteFiltersByKindAndSubjectID` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:214`<br>`TestADKAuditRouteRejectsInvalidPagination` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:245`<br>`TestADKChatStreamEmitsSessionRunAndFinalEvents` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:359`<br>`TestADKChatReturnsCompletedEnvelopeWithVisibleToolFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:420`<br>`TestADKChatStreamReturnsFinalEventForCompletedRunWithToolFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:481`<br>`TestADKChatStreamRecoversCompletedRunAsFinalEventWhenFinalMessageAppendFails` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:552`<br>`TestADKChatStreamReturnsErrorEventForInvalidPayload` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:578`<br>`TestADKProviderSaveRejectsInvalidPayload` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:597`<br>`TestADKProviderSaveReturnsRequestTimeoutMs` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:634`<br>`TestADKAgentSaveValidationFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:707`<br>`TestADKSkillInstallAndUninstallFailureRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:741`<br>`TestADKBindAgentWithPreinstalledNeodataFinancialSearch` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:787`<br>`TestADKSessionNegativeRoutes` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:858`<br>`TestADKRunNegativeRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_routes_test.go:911`<br>`TestADKApprovalNegativeAndIdempotentRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_sessions_test.go:15`<br>`TestADKSessionsCRUDAndFilteringRoutes` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_transport_contracts_test.go:10`<br>`TestADKChatStreamTransportPreservesEventIdentityAndPayload` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_workflow_routes_test.go:15`<br>`TestADKWorkflowDefinitionTriggerAndRunRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/adk_workflow_routes_test.go:262`<br>`TestADKWorkflowRoutesRejectInvalidInputs` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/catalog_failure_contracts_test.go:17`<br>`TestCatalogReadFaultsExposeStableAPIContracts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_helpers_test.go:13`<br>`TestTimelineStreamStateTracksSessionRunAndToolTiming` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_helpers_test.go:49`<br>`TestChatStreamRecordCurrentRunID` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_helpers_test.go:59`<br>`TestTimelineStreamStateEmptyAndCloneBoundaries` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_helpers_test.go:87`<br>`TestChatStreamHubRetentionRunLookupAndCloneBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_helpers_test.go:151`<br>`TestStreamHelpersRunIDAndBestEffortLogging` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_helpers_test.go:167`<br>`TestChatStreamExecutionPublishesDeltaAndFinalVariants` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_helpers_test.go:216`<br>`TestExecuteADKChatStreamPublishesTerminalErrorForInvalidRequest` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_helpers_test.go:260`<br>`TestAssistantRequestHelpersCoverInvalidAndBoundaryInputs` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_stream_lifecycle_test.go:9`<br>`TestHandlerCloseCancelsAndJoinsBackgroundExecutions` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_stream_recovery_contracts_test.go:12`<br>`TestChatStreamHubKeepsEventAndTimelineContracts` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_stream_recovery_contracts_test.go:41`<br>`TestChatStreamExecutionReusesKnownContextAndRecoversTerminalRun` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_transport_disconnect_test.go:55`<br>`TestChatStreamTransportHandlesDisconnectedClients` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/chat_transport_disconnect_test.go:110`<br>`TestChatStreamReconnectAndReplayRespectClientDisconnect` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/input_response_test.go:12`<br>`TestRunInputResponseErrorAndRetryContracts` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/query_encoding_contracts_test.go:13`<br>`TestAssistantQueryRoutesRejectMalformedEncoding` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_boundary_contracts_test.go:15`<br>`TestAssistantRoutesReturnUnavailableWhenRuntimeMissing` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_boundary_contracts_test.go:92`<br>`TestAssistantRoutesSurfaceStoreFailuresAfterRuntimeClose` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_boundary_contracts_test.go:178`<br>`TestAssistantCatalogSessionAndObservabilitySuccessContracts` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_boundary_contracts_test.go:278`<br>`TestAssistantCatalogBoundaryStatusCodes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_boundary_contracts_test.go:329`<br>`TestAssistantSessionRunBoundaryStatusCodes` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_boundary_contracts_test.go:369`<br>`TestAssistantMutationRoutesRejectMalformedJSON` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_error_contracts_test.go:14`<br>`TestAssistantRoutesRejectInvalidQueriesPayloadsAndMissingResources` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_error_contracts_test.go:98`<br>`TestAssistantRoutesEnforceBusinessValidationOnUpdates` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_identifier_validation_test.go:12`<br>`TestAssistantRoutesRejectBlankDecodedIdentifiers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_payload_pagination_test.go:12`<br>`TestAssistantChatRoutesRejectMalformedOrUnresolvableRequests` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_payload_pagination_test.go:27`<br>`TestAssistantRoutesRejectMalformedMutationPayloads` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_payload_pagination_test.go:65`<br>`TestAssistantRoutesClampPaginationBeyondAvailableItems` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_payload_pagination_test.go:80`<br>`TestAssistantRunMutationRoutesEnforceGoalLifecycleRules` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_payload_pagination_test.go:134`<br>`TestAssistantRoutesClassifyMissingMutationTargets` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:15`<br>`TestTaskAndMemoryCRUDContracts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:115`<br>`TestCatalogSnapshotToolsTemplatesAndDeleteAgentContracts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:159`<br>`TestProviderAndAgentValidationContracts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:261`<br>`TestProviderDefaultContract` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:295`<br>`TestSessionRunAndOptimizationRouteContracts` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:410`<br>`TestStreamReconnectAndSkillContracts` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_test.go:29`<br>`TestCatalogSessionRunAndObservabilityContracts` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_test.go:92`<br>`TestAgentSaveErrorClassification` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_test.go:101`<br>`TestRunInputResponseContract` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_test.go:136`<br>`TestChatStreamHubReplayAndCleanupBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_test.go:180`<br>`TestSessionTimelineFailureKeepsLegacyErrorCode` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_test.go:265`<br>`TestChatAndSSEContracts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_test.go:301`<br>`TestChatRequestIdempotencyContracts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_test.go:348`<br>`TestChatRequestUsesDeclaredMessageFieldOnly` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/routes_test.go:366`<br>`TestApprovalContract` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/workflow_routes_test.go:14`<br>`TestWorkflowRoutesCoverDefinitionTriggerRunAndWebhookContracts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/assistant/workflow_routes_test.go:185`<br>`TestWorkflowRoutesClassifyInvalidPayloadsAndUnavailableRuns` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_boundaries_test.go:19`<br>`TestBacktestListAndMissingResultRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_boundaries_test.go:32`<br>`TestBacktestStartRouteRejectsMalformedAndMissingStrategy` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_boundaries_test.go:53`<br>`TestBacktestSyncRouteReturnsTaskForValidRequest` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_boundaries_test.go:78`<br>`TestBacktestSyncRouteRejectsObsoleteSessionScope` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_boundaries_test.go:98`<br>`TestBacktestHandlersRejectMissingAndBlankURIParameters` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_progress_test.go:71`<br>`TestSyncProgressAndCancelRoutesHandleSuccessAndNotFound` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_progress_test.go:97`<br>`TestStatusResultAndDeleteRoutesCoverTerminalAndStoreFailures` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_progress_test.go:143`<br>`TestResultAndDeleteRoutesMapRunStoreErrorsToInternalServerError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_progress_test.go:160`<br>`TestDeleteRouteReturnsNotFoundWhenTerminalRunDisappearsBeforeDelete` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_test.go:21`<br>`TestSyncRouteClassifiesRequestErrorsAsBadRequest` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_test.go:35`<br>`TestSyncRouteClassifiesAdapterFailureAsInternalServerError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_test.go:52`<br>`TestSyncRouteRejectsMalformedJSON` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_test.go:58`<br>`TestStartRouteClassifiesRequestAndInternalErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/backtest/routes_test.go:85`<br>`TestStartRoutePreservesQueuedResponseShape` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:13`<br>`TestOptionalBoolRecognizesFalseAliases` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:27`<br>`TestCandlePeriodValueHandlesEmptyAndUnsupportedInputs` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:37`<br>`TestNormalizeCandlePeriodSupportsEveryDocumentedFamily` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:61`<br>`TestParseQueryTimeReturnsCallerFallback` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:70`<br>`TestBindURIHandlesBindingAndFallbackEscapeValidation` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:109`<br>`TestRequestEscapedPathFallsBackToURLRawPath` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_test.go:14`<br>`TestParseQueryTimeNormalizesToUTC` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_test.go:56`<br>`TestBindURIRejectsMalformedEscapeInRequestURI` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_test.go:75`<br>`TestBindURIAllowsEscapedLiteralPercent` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_test.go:100`<br>`TestOptionalQueryValueParsingSemantics` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_test.go:186`<br>`TestCandlePeriodAndPaginationNormalization` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/bindings_test.go:224`<br>`TestResponseEnvelopeWriters` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_boundaries_test.go:30`<br>`TestSSEWriterPropagatesSerializationAndWriteFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_boundaries_test.go:46`<br>`TestRunSSEStreamLoopHandlesTriggerWithoutCallback` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_boundaries_test.go:60`<br>`TestRunSSEStreamLoopPropagatesTriggerAndTickerFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_boundaries_test.go:86`<br>`TestFailingSSEWriterIncludesReadableError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_concurrent_test.go:42`<br>`TestSSEWriterSerializesConcurrentWrites` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_test.go:41`<br>`TestSSEWriterReturnsFlushPanicAsError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_test.go:55`<br>`TestSSEWriterReturnsWritePanicAsError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_test.go:87`<br>`TestPrepareSSEWriterAndFrameFormatting` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_test.go:129`<br>`TestPrepareSSEWriterRejectsWriterWithoutFlusher` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_test.go:136`<br>`TestRunSSEStreamLoopPropagatesInitialError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_test.go:146`<br>`TestRunSSEStreamLoopRunsTriggerAndTickerTicks` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/httpserver/sse_test.go:196`<br>`TestTickerCHandlesNilTicker` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/dispatcher_boundaries_test.go:20`<br>`TestDispatcherInitialAndLiveDataFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/dispatcher_boundaries_test.go:59`<br>`TestDispatcherAuxiliarySubscriptionBranches` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/dispatcher_boundaries_test.go:120`<br>`TestDispatcherRunPropagatesTriggerAndTickerFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/dispatcher_boundaries_test.go:205`<br>`TestDispatcherEnvelopeDefaultsAndMapFallback` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/dispatcher_boundaries_test.go:219`<br>`TestBackendReceivesExplicitBrokerSelection` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/dispatcher_boundaries_test.go:257`<br>`TestHandlerNilAndClosedLifecycleBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/dispatcher_boundaries_test.go:295`<br>`TestDepthUpdateSubscriptionFiltersAndCoalesces` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/handler_test.go:113`<br>`TestHandlerHeartbeatSubscribeNormalizationAndPayloads` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/handler_test.go:207`<br>`TestHandlerRejectsSubscriptionWithoutProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/handler_test.go:244`<br>`TestHandlerDepthUpdatePublishesFreshPayload` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/handler_test.go:316`<br>`TestHandlerNotificationSequenceZeroReplay` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/handler_test.go:342`<br>`TestHandlerConnectionLimitAndCloseLifecycle` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/handler_test.go:397`<br>`TestHandlerRejectsUntrustedWebSocketOrigin` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/handler_test.go:421`<br>`TestHandlerAcceptsSameOriginWebSocket` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/handler_test.go:441`<br>`TestDispatcherDeduplicatesTickObservedAt` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/live/handler_test.go:480`<br>`TestDispatcherProviderSwitchTagsAndDoesNotDeduplicateNewProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:35`<br>`TestInstrumentHandlersRejectMissingURIParameters` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:63`<br>`TestCandlesAndDepthRoutesMapProviderFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:97`<br>`TestMarketsRouteFailsWhenActiveProviderDescriptorIsUnavailable` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:119`<br>`TestProviderFailureCodesPreserveFutuCompatibilityOnlyForFutu` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:167`<br>`TestActiveNonBrokerProviderMatchingHandlesMissingAndFutuDescriptors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:208`<br>`TestSnapshotRejectsMalformedRefreshQuery` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:229`<br>`TestNormalizeOptionalQueryTimeAcceptsEmptyAndRejectsMalformedValues` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:239`<br>`TestExplicitYFinanceReadsUseTheActiveMarketDataProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:273`<br>`TestMarketDataReadErrorsExposeProviderSwitchRetrySignal` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:286`<br>`TestMarketDataReadErrorsExposeProviderWarmupRetrySignal` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:300`<br>`TestMarketDataReadErrorsExposeProviderBusyRetrySignal` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:314`<br>`TestMarketDataReadErrorsRejectInvalidCandleSessions` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:327`<br>`TestBrokerMarketDataReadErrorsPreserveClientActionability` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:387`<br>`TestLiveReadRoutesReturnConflictForMissingSubscriptionLease` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:421`<br>`TestPollOnlyReadRoutesPrioritizeCapabilitiesAndPreserveLogicalLeases` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:462`<br>`TestSubscriptionRoutesRejectMalformedAndIncompleteRequests` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:502`<br>`TestSubscriptionRequestHelpersPreserveOnlyValidTargets` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:523`<br>`TestSubscriptionHandlersMapCanceledServiceOperations` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_boundaries_test.go:555`<br>`TestReleaseAndClearMapSnapshotCancellationAfterLogicalCleanup` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_news_actions_test.go:55`<br>`TestNewsAndCorporateActionsRoutesRequireInstrumentURI` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_news_actions_test.go:76`<br>`TestNewsRouteValidatesLimitAndForwardsToService` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_news_actions_test.go:115`<br>`TestCorporateActionsRouteValidatesRangeAndForwardsToService` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_news_actions_test.go:160`<br>`TestNewsAndCorporateActionsRoutesMapCapabilityAndProviderFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:22`<br>`TestSubscriptionRoutesUseInstrumentRequestContract` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:96`<br>`TestSubscriptionRoutesUseBrokerNeutralPollingWithoutFutuLease` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:147`<br>`TestExplicitBrokerRoutesUseBrokerReaderAndNeverLegacyFallback` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:205`<br>`TestSubscriptionReleaseConsumerOnlyClearsConsumer` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:239`<br>`TestClearSubscriptionRoutePreservesRunningStrategyLease` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:267`<br>`TestCandlesRoutePreservesLegacyQueryParsing` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:302`<br>`TestCandlesRouteNormalizesRepeatedSessions` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:321`<br>`TestCandlesRouteRejectsInvalidSessions` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:337`<br>`TestCandlesRouteRejectsUnsupportedPeriod` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:357`<br>`TestCandlesRouteForwardsExclusiveBeforeAndRejectsInvalidCombinations` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:387`<br>`TestCandlesRouteRejectsInvalidLimit` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:407`<br>`TestCandlesRouteTickAndStrictBeforePagination` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:439`<br>`TestDepthRouteRejectsInvalidNum` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:459`<br>`TestReadRoutesCoverMarketsSecuritySnapshotSearchHeartbeatAndNormalize` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:563`<br>`TestReadRoutesMapProviderAndRequestFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:625`<br>`TestInstrumentSearchRouteReturnsSubsetResolutionContract` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/marketdata/routes_test.go:725`<br>`TestInstrumentSearchRouteValidatesInputAndMapsProviderFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/adk_test.go:12`<br>`TestADKAvailableGuardsUnavailableRuntime` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/adk_test.go:30`<br>`TestADKAvailablePassesThroughWhenRuntimeExists` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/auth_test.go:13`<br>`TestAuthSkipsPublicPaths` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/auth_test.go:28`<br>`TestTrustedAndValidatedRequestContextHelpers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/auth_test.go:51`<br>`TestAuthProtectsLogout` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/auth_test.go:71`<br>`TestAuthProtectsSystemStatus` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/auth_test.go:78`<br>`TestAuthTrustedHostWithoutBrowserOriginBypassesCSRFChecks` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/auth_test.go:86`<br>`TestAuthTrustedHostStillRequiresTrustedBrowserOrigin` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/auth_test.go:112`<br>`TestAuthRejectsNilAuthenticator` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/auth_test.go:119`<br>`TestAuthRejectsUntrustedOrigin` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/auth_test.go:128`<br>`TestAuthRequiresOriginAndCSRFForSessionWrites` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/auth_test.go:153`<br>`TestAuthTreatsPatchAsSessionWrite` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/auth_test.go:173`<br>`TestCORSReflectsAllowedOriginsAndRejectsUnknownPreflight` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/security_boundaries_test.go:17`<br>`TestAuthenticationBoundaryDecisions` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/security_boundaries_test.go:46`<br>`TestWriteMethodDetectionSupportsOverridesAndNilRequests` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/security_boundaries_test.go:62`<br>`TestCORSAllowsTrustedPreflightAndSameOriginOptions` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/security_boundaries_test.go:90`<br>`TestRequestOriginUsesRefererAndHandlesNil` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/middleware/security_boundaries_test.go:108`<br>`TestCanonicalOriginRejectsMalformedAndUnsupportedValues` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/origin/origin_test.go:8`<br>`TestCanonical` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/origin/origin_test.go:31`<br>`TestFromRequest` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/prediction_combo_routes_test.go:15`<br>`TestPredictionComboQuoteAcceptsContextFromQueryAndMapsFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/prediction_combo_routes_test.go:64`<br>`TestPostQueryMapsUpstreamFailureAndRouteHelpersCoverAcceptedEnums` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:179`<br>`TestEmbeddedProviderNewsAndCorporateActionRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:229`<br>`TestEmbeddedProviderRouteErrorsKeepHTTPContract` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:264`<br>`TestEmbeddedProviderRankingsAndIndustryRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:335`<br>`TestEmbeddedProviderRankingsRouteMapsUnsupportedOperations` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:370`<br>`TestEmbeddedProviderCompanyResearchRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:458`<br>`TestEmbeddedProviderCompanyResearchRejectsUnsupportedOperations` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:498`<br>`TestEmbeddedProviderCalendarAndMacroRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:606`<br>`TestEmbeddedProviderCalendarMacroRoutesRejectUnsupportedOperations` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/research_screen_test.go:19`<br>`TestResearchScreenCatalogRouteAndValidation` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/research_screen_test.go:47`<br>`TestNormalizeResearchScreenQueryDefaultsAndRejectsNonV2Input` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/research_screen_test.go:70`<br>`TestWriteResearchScreenErrorReturnsStructured429` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/research_screen_test.go:80`<br>`TestResearchScreenPostUsesTypedDefinitionAndOffset` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/research_screen_test.go:130`<br>`TestResearchScreenPostPreservesExecutableV2Definition` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/research_screen_test.go:185`<br>`TestResearchScreenPostRejectsV1Payload` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/research_screen_test.go:213`<br>`TestTypedResearchScreenResultOmitsUnknownTotal` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/research_screen_test.go:272`<br>`TestResearchScreenCatalogServesEmbeddedProviders` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/research_screen_test.go:355`<br>`TestResearchScreenPostServesEmbeddedProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/research_screen_test.go:387`<br>`TestResearchScreenPostEmbeddedProviderConflictMatrix` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/routes_test.go:19`<br>`TestProductFeatureRoutesCoverReadWritePredictionAndSnapshots` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/routes_test.go:93`<br>`TestTypedResearchRoutesPreserveQueriesAndWire` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/routes_test.go:142`<br>`TestProductFeatureRoutesMapValidationCapabilityEligibilityAndBrokerErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/productfeatures/routes_test.go:265`<br>`TestProductFeatureRouteHelpers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/research/routes_test.go:18`<br>`TestScreenPresetRoutesCRUDAndConflict` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/research/routes_test.go:74`<br>`TestScreenPresetRoutesValidatePayloadAndUnavailableStore` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/adk_routes_contracts_test.go:18`<br>`TestADKRuntimeSettingsDefaultAndSave` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_accounts_validation_test.go:18`<br>`TestManagedAccountUpdateUsesPathIDAndSurfacesServerErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_accounts_validation_test.go:73`<br>`TestDataManagementRebuildRejectsMalformedAndRejectedRequests` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_failure_boundaries_test.go:20`<br>`TestSettingWriteRoutesRejectMalformedJSON` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_failure_boundaries_test.go:48`<br>`TestSettingWriteRoutesMapPersistenceFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_failure_boundaries_test.go:84`<br>`TestSystemNotificationRoutesReadAndSaveSettings` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_failure_boundaries_test.go:125`<br>`TestOnboardingCanBeResetWithoutLosingLastBroker` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_failure_boundaries_test.go:147`<br>`TestDataManagementStatusMapsCallbackFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_failure_boundaries_test.go:164`<br>`TestSettingsAndDataManagementRoutesExposeOperationalFailureContracts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_market_data_test.go:18`<br>`TestMarketDataSettingsRoutesReadSaveAndApplyProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_market_data_test.go:63`<br>`TestBacktestMarketDataSettingsRoutesExposeCatalogAndRollbackPreparationFailure` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_market_data_test.go:115`<br>`TestLegacyYFinanceConnectionRoutesAreRemoved` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_market_data_test.go:128`<br>`TestMarketDataSettingsRoutesMapValidationPersistenceAndRuntimeErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:247`<br>`TestSettingsRoutesPreserveLegacyResponseShapes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:304`<br>`TestMCPServerSettingsRoutesReturnTokenOnlyOnce` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:350`<br>`TestManagedAccountRoutesMapMissingRecordsToNotFound` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:376`<br>`TestCreateManagedAccountRejectsMissingAccountID` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:393`<br>`TestCreateManagedAccountDropsServerManagedFields` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:419`<br>`TestExecutionSettingsRouteUsesInjectedService` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:468`<br>`TestAppearanceOnboardingSecurityAndADKRoutesCoverSaveFlows` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:589`<br>`TestExecutionExchangeCalendarAndBrokerRoutesCoverReadAndValidation` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:709`<br>`TestExchangeCalendarSettingsRouteUsesInjectedService` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:754`<br>`TestDataManagementRoutesUseInjectedCallbacks` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:802`<br>`TestDataManagementRoutesUseTypedCallbacks` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_test.go:935`<br>`TestSystemNotificationTestRouteUsesSettingsServicePort` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/settings/routes_uri_boundaries_test.go:12`<br>`TestResourceHandlersRejectMissingURIParameters` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/pine_routes_contracts_test.go:72`<br>`TestAnalyzeStrategyPineRouteReturnsDiagnosticsAndRequirements` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/pine_routes_contracts_test.go:148`<br>`TestAnalyzeStrategyPineRouteOmitsASTByDefault` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/pine_routes_contracts_test.go:182`<br>`TestAnalyzeStrategyPineRouteRejectsUnsupportedSourceFormat` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/pine_routes_contracts_test.go:217`<br>`TestAnalyzeStrategyPineRouteReportsUnsupportedSyntax` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/pine_routes_contracts_test.go:253`<br>`TestAnalyzeStrategyPineRouteReturnsV20ParseOnlyMetadata` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/pine_routes_contracts_test.go:398`<br>`TestAnalyzeStrategyPineRouteReturnsObjectSignatureDiagnostics` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/pine_routes_contracts_test.go:453`<br>`TestAnalyzeStrategyPineRouteReturnsImportAliasDiagnostics` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/pine_routes_contracts_test.go:502`<br>`TestAnalyzeStrategyPineRouteReturnsTypeMethodRegistryDiagnostics` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_boundary_contracts_test.go:16`<br>`TestStrategyDefinitionPreviewQueryRejectsInvalidBooleans` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_boundary_contracts_test.go:31`<br>`TestStrategyRoutesRejectMissingBoundURIParamsAtHandlerBoundary` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_boundary_contracts_test.go:75`<br>`TestStrategyStartRouteMapsPreflightRuntimeAndTransitionBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_boundary_contracts_test.go:142`<br>`TestStrategyActivityRoutesRejectMalformedPagination` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_failure_boundaries_test.go:16`<br>`TestAnalyzePineRouteCoversSuccessMalformedAndAnalysisFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_failure_boundaries_test.go:71`<br>`TestDefinitionRoutesMapReadWriteAndDeleteFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_failure_boundaries_test.go:138`<br>`TestDefinitionOrchestrationRoutesMapBusinessFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_failure_boundaries_test.go:184`<br>`TestInstanceMutationRoutesMapCatalogFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_failure_boundaries_test.go:230`<br>`TestPluginRoutesRejectWhitespaceIdentifiers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_failure_boundaries_test.go:252`<br>`TestWriteStrategyErrorIgnoresNil` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_lifecycle_test.go:203`<br>`TestDefinitionRoutesNormalizeCreateUpdateAndDeleteGuards` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_lifecycle_test.go:252`<br>`TestDefinitionVersionRoutesExposeHistoryAndSnapshots` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_lifecycle_test.go:319`<br>`TestInstantiateApplyAndLifecycleRoutesFollowBusinessStateTransitions` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_lifecycle_test.go:401`<br>`TestDeleteInstanceRouteMapsBusinessOutcomes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_lifecycle_test.go:462`<br>`TestStrategyRoutesMapValidationAndBusinessErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_lifecycle_test.go:611`<br>`TestActivityRoutesNormalizePaginationAndTimeFilters` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_lifecycle_test.go:642`<br>`TestPluginRoutesCoverCatalogOperationMutationAndGuidance` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_lifecycle_test.go:689`<br>`TestPluginMutationRoutesMapNotFoundAndInternalFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_test.go:41`<br>`TestWriteStrategyErrorMapsBusinessErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_test.go:114`<br>`TestEnrichDefinitionResponseDefaultsAndQueryOverride` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_test.go:146`<br>`TestHandleGetDefinitionReturnsNotFoundAndBadQuery` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/strategy/routes_test.go:181`<br>`TestHandleAnalyzePineMapsValidationErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/system/routes_test.go:19`<br>`TestSystemRoutesReturnEnvelopes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/system/routes_test.go:61`<br>`TestSystemManualRetryRouteCallsReset` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/system/routes_test.go:77`<br>`TestExchangeCalendarRefreshRouteCallsRefresh` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/system/routes_test.go:102`<br>`TestSystemRouteBoundaryValidatorsRejectMissingHardStopAndNonPositiveLimits` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/system/routes_test.go:121`<br>`TestRealTradeReleaseRoutesRejectMalformedOptionalPayloadBeforeStateChange` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/system/routes_test.go:170`<br>`TestExchangeCalendarProbeRouteCallsProbe` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/system/routes_test.go:191`<br>`TestRealTradeControlRoutesDelegateStateChanges` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/system/routes_test.go:316`<br>`TestRealTradeControlRoutesMapValidationAndControlFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/system/status_mapper_test.go:11`<br>`TestSystemStatusTransportMapperPreservesDomainJSON` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_products_test.go:18`<br>`TestExecutionProductRoutesBuyingPowerComboLifecycle` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_products_test.go:73`<br>`TestExecutionProductRoutesValidationAndServiceErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_test.go:19`<br>`TestExecutionCommandErrorMapsRequestAndBrokerFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_test.go:47`<br>`TestHandleExecutionPlaceReturnsRiskRejectionEnvelope` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_test.go:79`<br>`TestHandleExecutionPlaceRejectsEquityAmountModeSpoof` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_test.go:110`<br>`TestHandleExecutionOrdersNormalizesScopeAndFilter` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_test.go:148`<br>`TestHandleExecutionCancelReturnsMappedEnvelope` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_test.go:175`<br>`TestTradingQueryHelpersNormalizeAndValidate` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_test.go:205`<br>`TestExecutionPlacePreviewAndEventsRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_test.go:249`<br>`TestHandleExecutionOrderDetailsReturnsCanonicalReceiptAndNotFound` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_validation_contracts_test.go:52`<br>`TestExecutionRoutesValidatePayloadsAndMapHandlerErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_validation_contracts_test.go:161`<br>`TestExecutionOrdersRouteSwitchesBetweenActiveAndHistorySync` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_validation_contracts_test.go:205`<br>`TestExecutionHandlersRejectMissingInternalOrderIDAndTrimWhitespace` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/execution_validation_contracts_test.go:238`<br>`TestExecutionOrderDetailsRouteMapsMissingAndStoreFailures` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/openapi_route_alignment_test.go:14`<br>`TestTradingOpenAPIDocumentationRoutesMatchGinRegistration` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_broker_contracts_test.go:143`<br>`TestPortfolioRoutesUseBrokerSnapshotsAndMissingBrokerSemantics` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_broker_contracts_test.go:214`<br>`TestBrokerWriteRoutesMapBodyBrokerAndOperationOutcomes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_failure_boundaries_test.go:17`<br>`TestBrokerReadRoutesPreserveDegradedBackendFailureSemantics` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_failure_boundaries_test.go:58`<br>`TestBrokerReadRoutesRejectMissingAndInvalidOptionalInputs` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_helper_boundaries_test.go:17`<br>`TestTradingRouteHelpersWriteHTTPBoundaryErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_helper_boundaries_test.go:119`<br>`TestPortfolioReadUnknownResourceIsNotFound` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_helper_boundaries_test.go:131`<br>`TestTradingRoutesRejectMalformedQueryEncodingBeforeDispatch` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_read_handlers_test.go:18`<br>`TestBrokerReadHandlersSerializeEmptyCollectionsAsArrays` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_read_handlers_test.go:71`<br>`TestTradingReadHandlersValidateAndNormalizeBusinessQueries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_test.go:32`<br>`TestBrokerRoutesPreserveFallbackSemanticsAndRequestValidation` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_test.go:95`<br>`TestBrokerWriteRoutesClassifyUnsupportedTradingAndUnlock` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_test.go:121`<br>`TestBrokerReadRoutesCoverOrdersFillsQuotesKLinesAndSecurities` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/trading/routes_test.go:145`<br>`TestBrokerReadRoutesRejectInvalidScopeAndNumericQueryValues` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/route_error_handling_test.go:16`<br>`TestWriteErrorMapsAllDomainErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/route_error_handling_test.go:50`<br>`TestRouteHelpersCoverBindingAndDeleteErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/route_error_handling_test.go:77`<br>`TestBindQueryRejectsMalformedAndInvalidValues` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/routes_business_test.go:51`<br>`TestWatchlistRoutesMembershipIdempotencyConflictAndPagination` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/routes_business_test.go:126`<br>`TestWatchlistRoutesMapValidationNotFoundAndProtectedConflicts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/routes_business_test.go:147`<br>`TestWatchlistRoutesCompleteImportAndGroupLifecycle` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/routes_business_test.go:281`<br>`TestWatchlistRoutesRejectMalformedBodiesAndPageLimits` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/routes_test.go:15`<br>`TestUnavailableServiceReturns503Envelope` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/routes_test.go:34`<br>`TestUnavailableServiceExercisesAllRouteErrorBranches` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/routes_test.go:72`<br>`TestInvalidListLimitReturns400` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/routes_test.go:84`<br>`TestWatchlistListAndBindingRoutesRejectMalformedQueryEncoding` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/api/watchlist/routes_test.go:107`<br>`TestWatchlistRoutesRejectMissingURIValuesBeforeExecutingBusinessOperations` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/assistant_test.go:45`<br>`TestAssistantPortsProjectSettingsAndHealth` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/assistant_test.go:75`<br>`TestAssistantPortsAndPathsAreNilSafe` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/assistant_test.go:98`<br>`TestAssistantCompositionOpensRuntimeAndProjectsServices` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/installers_test.go:16`<br>`TestInstallersExpressTypedDependencyOrder` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/installers_test.go:49`<br>`TestInstallersRollbackPartialInitialization` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/lifecycle_test.go:11`<br>`TestLifecycleClosesAdoptedResourcesInReverseOrderAndPreservesSetupError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/lifecycle_test.go:41`<br>`TestLifecycleEnsuresOnlyMissingOwnershipGroupsOnce` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/lifecycle_test.go:68`<br>`TestLifecycleRecordsLateRegistrationFailureAndClosesResource` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/resources_test.go:12`<br>`TestResourcesClosesApplicationDependenciesInReverseStartupOrder` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/resources_test.go:34`<br>`TestOpenRollsBackEarlierResourcesWhenLaterStartupFails` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/resources_test.go:78`<br>`TestResourcesCloseIsIdempotentAndConcurrentSafe` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/resources_test.go:112`<br>`TestResourcesCloseAggregatesEveryNamedFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/resources_test.go:137`<br>`TestRegisterAfterShutdownClosesLateResourceImmediately` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/application/runtime_dependencies_test.go:11`<br>`TestRuntimeDependenciesDelegatesAndReportsUnavailableService` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:62`<br>`TestProviderHistoricalSourceMapsExtendedSessionsToProviderCapabilities` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:87`<br>`TestProviderHistoricalSourceRejectsExtendedSessionsOutsideUSIntraday` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:108`<br>`TestProviderHistoricalSourceValidatesAdjustmentAndLookback` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:141`<br>`TestProviderHistoricalSourceAppliesMarketScopedLookback` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:162`<br>`TestProviderHistoricalSourceEnforcesProviderAdjustmentMatrix` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:206`<br>`TestKLineSyncPreflightRejectsStaticCapabilityMismatchAndUnknownProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:226`<br>`TestProviderHistoricalSourceFetchesAndParsesProviderPage` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:266`<br>`TestHistoricalPageParsingRejectsMalformedProviderValues` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:298`<br>`TestDecimalStringAcceptsProviderNumericRepresentations` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:310`<br>`TestBacktestProviderSyncerPinsFutuAndClosesOnFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:354`<br>`TestInstrumentSpecUsesProviderRulesAndConservativeFallbacks` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:385`<br>`TestInstrumentSpecRequiresReadyPythonProviders` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:401`<br>`TestProviderOptionsRequireMarketDataRuntime` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:414`<br>`TestPositiveFloatRecognizesSupportedRuleTypes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/databaseguard/groups_test.go:14`<br>`TestGroupsDeclareDatabaseAvailabilityPerRouteFamily` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/databaseguard/groups_test.go:35`<br>`TestGroupsKeepRoutesAvailableWhenDatabasesAreHealthy` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:16`<br>`TestMaintenanceExecutionRejectsConcurrentStaleAndPartialCleanup` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:70`<br>`TestMaintenanceCompactAndBackupProtectUnavailableAndBusyDatabases` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:109`<br>`TestMaintenanceDatabaseInspectionAndBackupRetentionFailuresStayLocal` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:142`<br>`TestMaintenanceCandidateQueriesFailSafelyWhenSchemaDoesNotMatch` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:167`<br>`TestMaintenancePreviewDefaultsAndConcurrentStateChangesFailClosed` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:225`<br>`TestMaintenanceBackupAndRebuildSurfacePersistentStateErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:245`<br>`TestBackupRetentionReportsRemovalPermissionFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:270`<br>`TestMaintenanceStorageAndCandidateBoundaryErrorsAreContained` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:341`<br>`TestBackupSnapshotRejectsBlockedDirectoryAndEmptySource` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:358`<br>`TestBackupRetentionEvictsQuotaPressureAcrossDatabaseFiles` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:394`<br>`TestBackupRetentionNeverEvictsRebuildMarkerSnapshots` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:455`<br>`TestBackupSnapshotDoesNotMutateIncompatibleSource` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:502`<br>`TestMaintenanceCompactionAndBackupFailClosedWhenPersistentStateBreaks` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:529`<br>`TestNewPreviewIDSurfacesSecureRandomnessFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:538`<br>`TestMaintenancePreviewRejectsAConfiguredCleanupTargetWithoutDescriptor` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:552`<br>`TestMaintenanceCleanupAndCompactionReportActualReclaimedStorage` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:17`<br>`TestOverviewCountsMainWALSHMAndKeepsDatabaseErrorsLocal` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:49`<br>`TestOverviewSupportsSummaryOnlyAndSingleDatabase` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:82`<br>`TestDatabaseBackupCreatesVerifiedPrivateSnapshot` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:147`<br>`TestBackupCapacityPrunesManagedFilesOnlyAndEnforcesQuota` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:185`<br>`TestBacktestCleanupPreviewUsesAgeAndLatestProtection` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:227`<br>`TestCleanupPreviewExpiresAndMaintenanceConflictIsTyped` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:255`<br>`TestOverviewCleanableCategoriesADKPreviewAndCompact` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:315`<br>`TestCleanupAndCompactRejectInvalidOrUnavailableOperations` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/managed_backup_retention_test.go:12`<br>`TestManagedBackupRetentionKeepsCurrentSnapshotAndPrunesOldFiles` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/managed_backup_retention_test.go:61`<br>`TestManagedBackupFileDiscoveryAndFilenameBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/managed_backup_retention_test.go:105`<br>`TestBackupSnapshotFailureCleansUpPartialFilesAndVerificationRejectsInvalidSQLite` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:14`<br>`TestManagerStatusesReflectRuntimeFailuresAndScheduledRebuilds` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:50`<br>`TestManagerScheduleRebuildValidatesModesAndSelection` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:74`<br>`TestManagerPendingLifecycleHandlesMissingCorruptAndUnknownMarkers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:108`<br>`TestInspectDatabaseClassifiesFilesystemAndSchemaStates` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:163`<br>`TestInspectDatabaseRejectsManifestDrift` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:187`<br>`TestManagerMarkerPersistenceNormalizesAndSurfacesFilesystemErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:280`<br>`TestManagerPropagatesUnreadableMarkerAndDatabaseStatErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_test.go:14`<br>`TestManagerSchedulesSingleAndBatchRebuilds` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_test.go:49`<br>`TestManagerScheduleRebuildSharesMaintenanceLocks` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_test.go:75`<br>`TestManagerDescriptorsMatchSchemaCatalog` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_test.go:100`<br>`TestManagerApplyPendingDeletesOnlySelectedDatabaseFiles` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_test.go:145`<br>`TestManagerApplyPendingRejectsTamperedBackupBeforeDeletingAnySource` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/manager_test.go:190`<br>`TestManagerKeepsMarkerWhenDeleteFails` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:12`<br>`TestVerifyMarkerBackupRejectsEveryUntrustedMarkerField` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:77`<br>`TestApplyPendingRejectsDuplicateAndMissingBackups` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:106`<br>`TestScheduleRebuildLockedRemovesSnapshotsAfterBatchFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:128`<br>`TestScheduleRebuildLockedRequiresBackupForExistingMarkerIDs` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:150`<br>`TestScheduleRebuildIsIdempotentForAnExistingVerifiedBackup` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:177`<br>`TestRebuildSelectionAndLockRollbackBoundaries` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:211`<br>`TestScheduleRebuildLockedPropagatesUnreadableMarker` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:221`<br>`TestProtectedBackupAndDigestFailureBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/datamigration/research_lifecycle_test.go:13`<br>`TestResearchDatabaseParticipatesInStatusBackupAndRebuild` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/desktop_api_startup_test.go:40`<br>`TestAPIServerHelperBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/desktop_api_startup_test.go:87`<br>`TestLoadFrontendFSPreservesUnavailableAndEmbeddedAssetSemantics` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/desktop_api_startup_test.go:106`<br>`TestWaitDesktopAPIReadyCoversAuthorizationAndTimeout` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/desktop_api_startup_test.go:148`<br>`TestStartDesktopWithConfigClosesSidecarWhenReadinessTargetFails` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/futuapp/coordinator_test.go:15`<br>`TestCoordinatorResetPreservesApplicationOrderAndInvalidatesFutu` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/futuapp/coordinator_test.go:39`<br>`TestCoordinatorDisabledProjectionsAndRetryDiagnostics` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/futuapp/coordinator_test.go:69`<br>`TestMarketDataHealthRequiresHealthyOpenDQuoteSession` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/futuapp/coordinator_test.go:109`<br>`TestCoordinatorOnboardingUsesRuntimeAndAccountReadiness` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/futuapp/runtime_contracts_test.go:14`<br>`TestCoordinatorProjectsConnectedRuntimeAndDiscoveredAccounts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/futuapp/runtime_contracts_test.go:44`<br>`TestCoordinatorConnectedProbeWithoutBrokerFailsClosed` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/futuapp/runtime_contracts_test.go:58`<br>`TestCoordinatorEnabledClosedPortReportsManualRetryDiagnosis` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/futuapp/runtime_probe_contracts_test.go:10`<br>`TestCoordinatorDisabledProbeAndSettingsBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/futuapp/runtime_state_boundaries_test.go:26`<br>`TestFutuRuntimeRemainingDisconnectedAndResetPaths` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/futuapp/runtime_state_boundaries_test.go:44`<br>`TestFutuRuntimeRemainingDisabledProbeAndBoolValue` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/futuapp/runtime_state_boundaries_test.go:51`<br>`TestFutuRuntimeHealthyProbeAndGlobalStateBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:90`<br>`TestStartForRunArgsReturnsNoopWhenDisabled` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:110`<br>`TestStartForRunArgsConfiguresRuntimeAndFrontend` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:196`<br>`TestWebAccessBindDefaultsToLoopbackAndRequiresCompletePublicConfiguration` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:222`<br>`TestWebAccessListenerBindUsesIndependentConfiguredPort` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:243`<br>`TestSeparateWebListenerStartsAlongsideLoopbackDesktopSidecar` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:275`<br>`TestSeparateWebListenerRebindsImmediatelyAndKeepsOldPortOnConflict` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:331`<br>`TestWebAccessServerManagerCoversLiveReconfigurationLifecycle` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:378`<br>`TestWebAccessServerManagerRestoresOldBindAfterHostSwitchFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:424`<br>`TestStartForRunArgsStartsAPIOnlyAndShutsDown` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:459`<br>`TestStartForRunArgsClosesHandlerWhenDatabaseRebuildFinalizeFails` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:490`<br>`TestStartForRunArgsReportsAPIPortConflictAndClosesHandler` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:516`<br>`TestStartForRunArgsRollsBackWebListenerBeforeHandlerOnAPIPortConflict` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:555`<br>`TestStartForRunArgsWithEmbeddedFrontendDoesNotBindAPIPort` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:587`<br>`TestStartForRunArgsReportsIntegratedHTTPPortConflictAndClosesHandler` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:616`<br>`TestStartForRunArgsStopsAtFailingStartupStage` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:691`<br>`TestRunAPIOnlyReturnsStartupErrorAndWaitsForCancellation` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:711`<br>`TestOnceShutdownReturnsStableHandlerError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/liveapp/bbgo_notifications_test.go:12`<br>`TestBBGONotificationMappingPreservesAlertSemantics` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/liveapp/bbgo_notifications_test.go:43`<br>`TestBBGONotificationSourceStartsStopsAndMapsUploads` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/liveapp/bbgo_notifications_test.go:84`<br>`TestDeliverNotificationConvertsSinkPanicsToDeliveryFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/liveapp/handler_test.go:7`<br>`TestNewHandlerKeepsLiveTransportOptions` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/assistant_provider_test.go:95`<br>`TestAssistantMarketProvidersReportsSelectionAndActiveHealth` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/assistant_provider_test.go:114`<br>`TestSelectAssistantMarketProviderPersistsScopeAndReturnsBeforeAfter` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/assistant_provider_test.go:143`<br>`TestAssistantProviderPortsAndUnavailableServices` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/assistant_provider_test.go:159`<br>`TestAssistantMarketProviderReportsAndPropagatesFailureBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/data_plane_switch_test.go:16`<br>`TestApplyProviderSettingsUsesAtomicQuoteProviderSwitch` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/data_plane_switch_test.go:40`<br>`TestApplyProviderSettingsPreservesAtomicQuoteCacheOnFailure` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/data_plane_switch_test.go:66`<br>`TestApplyProviderSettingsAllowsUnavailableWatchlist` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/data_plane_switch_test.go:83`<br>`TestApplyProviderSettingsRestoresExistingFutuDemandBeforeReturning` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/data_plane_switch_test.go:140`<br>`TestApplyProviderSettingsRollsBackFailedFutuDemandRestore` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/data_plane_switch_test.go:189`<br>`TestNewDataPlaneKeepsConfiguredPythonProviderWhenHelperIsUnavailable` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/data_plane_switch_test.go:237`<br>`TestProviderNeedsActivationReflectsUnavailableRuntime` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/data_plane_switch_test.go:265`<br>`TestRestoreConfiguredProviderSkipsUnavailableRuntime` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/heartbeat_test.go:14`<br>`TestLiveHeartbeatProjectsSamplesRetriesAndClientState` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/heartbeat_test.go:108`<br>`TestLiveHeartbeatUsesPollOnlyFreshnessAndIgnoresStreamStaleReasons` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/heartbeat_test.go:155`<br>`TestLiveHeartbeatPolicyBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_depth_test.go:35`<br>`TestMarketDepthResponseWithMockOpenD` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_depth_test.go:143`<br>`TestMarketDepthNumClamping` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_depth_test.go:198`<br>`TestMarketDepthSymbolCasing` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_depth_test.go:244`<br>`TestMarketDepthHKMarket` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_depth_test.go:306`<br>`TestMarketDepthEmptyOrderBook` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:18`<br>`TestMarketCandlesResponseUsesExchangeResolvedSessionsForUSIntraday` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:91`<br>`TestMarketCandlesResponseOmitsSessionMetadataForDailyCandles` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:137`<br>`TestMarketCandlesResponseRejectsInvalidSessionsBeforeFutuAccess` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:150`<br>`TestMarketCandlesResponseClassifiesUnknownUSSessionAsDataError` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:245`<br>`TestMarketSnapshotResponseUsesFreshCache` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:306`<br>`TestMarketSnapshotResponseQueriesQuoteSnapshotOnCacheMiss` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:330`<br>`TestMarketSnapshotResponseForceRefreshBypassesCache` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:368`<br>`TestMarketSnapshotResponseRejectsInvalidRefreshQuery` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:382`<br>`TestMarketCandlesTickResponseUsesFreshCache` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:418`<br>`TestMarketCandlesTickResponseQueriesTickerOnCacheMiss` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:442`<br>`TestMarketCandlesTickResponseFallsBackToCachedCandlesOnTickerError` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:478`<br>`TestMarketSecurityDetailsResponseQueriesSecuritySnapshot` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:533`<br>`TestMarketSecurityDetailsResponseIncludesWarrantBlock` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:555`<br>`TestMarketSecurityDetailsResponseIncludesOptionBlock` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:577`<br>`TestMarketSecurityDetailsResponseIncludesFutureBlock` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:592`<br>`TestMarketSecurityDetailsResponseIncludesTrustBlock` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:607`<br>`TestMarketSecurityDetailsResponseIncludesIndexBlock` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:622`<br>`TestMarketSecurityDetailsResponseIncludesPlateBlock` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/provider_boundaries_test.go:145`<br>`TestMarketDataProviderClosureAndOptionalCapabilityBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/provider_boundaries_test.go:163`<br>`TestMarketDataProviderLookupFailureAndFilteringBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/provider_boundaries_test.go:197`<br>`TestMarketDataProviderSearchFailureAndNormalizationBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/provider_boundaries_test.go:220`<br>`TestMarketDataProviderCandleParsingRemainingBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/provider_boundaries_test.go:271`<br>`TestBrokerSearchInstrumentPartsPreservesDottedCodes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/provider_test.go:12`<br>`TestProviderDelegatesApplicationCallbacks` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/provider_test.go:86`<br>`TestBrokerSearchInstrumentPartsNormalizesKnownPrefixes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/provider_test.go:107`<br>`TestBrokerSearchInstrumentPartsRejectsUnknownPrefixInference` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/python_runtime_test.go:13`<br>`TestResolveSourcePythonRuntimeHonorsEnvironmentAndFallbacks` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/python_runtime_test.go:55`<br>`TestResolvePythonRuntimeReportsExternalAndEmbeddedHelpers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/python_runtime_test.go:79`<br>`TestMarketDataEnvironmentOverridesTakePriorityOverYFinanceAliases` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/python_runtime_test.go:94`<br>`TestProbePythonRuntimeValidatesVersionAndModulesWithoutImportingThem` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/query_test.go:11`<br>`TestNormalizeCandlePeriodMapsAliases` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/query_test.go:21`<br>`TestParseQueryTimeFallsBackOnInvalidInput` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/query_test.go:28`<br>`TestKLineQueryWindowUsesExplicitBounds` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/query_test.go:45`<br>`TestDecodeMarketCandlesQueryParsesRepeatedSessions` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/query_test.go:60`<br>`TestKLineQueryWindowResetsInvalidBeginToDefaultLookback` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_akshare_test.go:11`<br>`TestRuntimeReusesSharedSidecarAcrossPythonProviders` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_akshare_test.go:48`<br>`TestRuntimeKeepsSharedSidecarOnCrossPythonActivationFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_akshare_test.go:78`<br>`TestRuntimeStopsNewSidecarWhenInitialAKShareActivationFails` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_akshare_test.go:97`<br>`TestRuntimeRetriesAProviderMarkedUnavailable` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_akshare_test.go:126`<br>`TestRuntimeUsesGenericCacheDirectoryWithLegacyFallback` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_calendar_forwarding_test.go:74`<br>`TestRuntimeCalendarMacroForwarding` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_calendar_forwarding_test.go:119`<br>`TestRuntimeCalendarMacroPropagatesError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_calendar_forwarding_test.go:131`<br>`TestRuntimeCalendarMacroCapabilityUnsupported` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_company_forwarding_test.go:57`<br>`TestRuntimeCompanyResearchForwarding` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_company_forwarding_test.go:95`<br>`TestRuntimeCompanyResearchPropagatesError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_company_forwarding_test.go:108`<br>`TestRuntimeCompanyResearchCapabilityUnsupported` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_forwarding_test.go:12`<br>`TestRuntimeForwardsEveryDataPlaneOperationWithoutRewritingProviderResults` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_forwarding_test.go:98`<br>`TestRuntimePreservesErrorsFromEveryActiveCapability` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_forwarding_test.go:166`<br>`TestRuntimeSameProviderActivationDoesNotReleasePhysicalSubscriptions` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_forwarding_test.go:191`<br>`TestRuntimeSidecarFailureAndCloseKeepSelectionStable` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_health_test.go:13`<br>`TestRuntimeExplicitYFinanceActivationRequiresHealthBeforePublishing` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_health_test.go:64`<br>`TestRuntimeFailedHealthCheckRestoresSidecarWithoutChangingProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_health_test.go:94`<br>`TestRuntimeReportsBothHealthAndSidecarRestoreFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_health_test.go:114`<br>`TestRuntimeDefersSubscriptionReleaseFailureAfterHealthyActivation` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_health_test.go:142`<br>`TestRuntimeChecksEmbeddedYFinanceOnStartupButNotFutu` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_health_test.go:167`<br>`TestWaitForProviderHealthRetriesUntilConnected` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_health_test.go:183`<br>`TestWaitForProviderHealthAllowsWarmingOnlyDuringStartupRestore` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_health_test.go:205`<br>`TestWaitForProviderHealthStopsOnFailedWarmup` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_health_test.go:222`<br>`TestProviderHealthRetryDelayBacksOffAndCaps` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_health_test.go:240`<br>`TestWaitForProviderHealthPreservesLastFailureOnCancellation` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_health_test.go:261`<br>`TestWaitForProviderHealthAndRuntimeDefaultCheckerBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_index_constituents_forwarding_test.go:35`<br>`TestRuntimeForwardsIndexConstituentsToCapableActiveProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_index_constituents_forwarding_test.go:61`<br>`TestRuntimeIndexConstituentsRejectsProvidersWithoutCapability` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_news_forwarding_test.go:52`<br>`TestRuntimeForwardsNewsAndCorporateActionsToCapableActiveProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_news_forwarding_test.go:89`<br>`TestRuntimeNewsAndCorporateActionsRejectProvidersWithoutCapability` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_rankings_industry_forwarding_test.go:66`<br>`TestRuntimeForwardsRankingsToCapableActiveProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_rankings_industry_forwarding_test.go:92`<br>`TestRuntimeForwardsIndustryReadsToCapableActiveProvider` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_rankings_industry_forwarding_test.go:125`<br>`TestRuntimeRankingsAndIndustriesRejectProvidersWithoutCapability` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_screen_forwarding_test.go:39`<br>`TestRuntimeScreenForwarding` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_screen_forwarding_test.go:63`<br>`TestRuntimeScreenPropagatesError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_screen_forwarding_test.go:77`<br>`TestRuntimeScreenCapabilityUnsupported` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:14`<br>`TestRuntimeSwitchesStableDataPlaneBetweenFutuAndYFinance` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:85`<br>`TestRuntimeCommitsFutuWhenSidecarCleanupNeedsRetry` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:113`<br>`TestRuntimeRejectsInvalidActivationAndDefersFutuCleanupFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:143`<br>`TestRuntimeRollsBackYFinanceWhenNonFutuProviderRetirementFails` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:168`<br>`TestRuntimeRetiresPreviousSubscriptionsThroughBrokerReconciliation` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:191`<br>`TestRuntimeReportsFutuActivationAndRollbackReleaseFailures` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:230`<br>`TestRuntimeContinuesInactiveFutuCleanupWhileYFinanceIsActive` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:276`<br>`TestRuntimeSerializesInactiveCleanupWithFutuReactivation` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:337`<br>`TestRuntimeRestoresPreviousSubscriptionsAfterActivationFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:370`<br>`TestRuntimeCommitsSwitchWhenFutuRetirementContextExpires` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:394`<br>`TestRuntimeConstructorAndUnavailablePollingBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:414`<br>`TestRuntimeCannotRestartManagedSidecarAfterClose` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:437`<br>`TestRuntimeCloseWinsConcurrentActivationWithoutRestartingSidecar` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:476`<br>`TestRuntimeDoesNotCommitActivationCanceledWhileQueued` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:504`<br>`TestProviderChangeCannotRestoreSubscriptionsAfterRuntimeClose` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:547`<br>`TestRuntimeRetriesSidecarCleanupWithinClose` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:570`<br>`TestRuntimeReportsBothBoundedSidecarCleanupFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:591`<br>`TestProviderLeasesKeepSharedPythonSidecarUntilLastRelease` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:633`<br>`TestProviderSwitchKeepsAcceptedLeaseOnOldInstance` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:667`<br>`TestBacktestProviderHelpersListAndPrepareProviders` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:704`<br>`TestBacktestProviderPreparerReturnsPreparationFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:724`<br>`TestProviderLeaseNilBoundariesAndRuntimeInitialization` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_os_process_test.go:14`<br>`TestOSSidecarProcessValidatesLaunchesAndStopsRealChild` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_os_process_test.go:78`<br>`TestOSSidecarProcessTreatsNaturalExitAsStoppedAndCloseable` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_os_process_test.go:115`<br>`TestOSSidecarProcessBoundsGracefulStopBeforeKillingChild` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_os_process_test.go:146`<br>`TestWaitForSidecarDoneHasExplicitTimeout` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_os_process_test.go:161`<br>`TestProcessAlreadyFinishedIsASuccessfulStop` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:15`<br>`TestSidecarManagerStartsReusesAndStopsManagedExecutable` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:61`<br>`TestSidecarManagerRestartsExitedProcessAndCleansPreviousMaterialization` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:92`<br>`TestSidecarManagerCleansMaterializationAfterPreparationFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:128`<br>`TestSidecarManagerRetainsProcessUntilStopSucceeds` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:147`<br>`TestSidecarExecutableDevelopmentOverrideAndManagerBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:190`<br>`TestDevelopmentOverrideRejectsMissingAndNonFilePaths` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:208`<br>`TestDevelopmentPythonSourceCommandAndExplicitHelperPrecedence` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:261`<br>`TestDevelopmentPythonSourceCommandRejectsInvalidSourcePath` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:297`<br>`TestDevelopmentPythonSourceCommandRejectsInvalidRuntimeBeforeStart` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_signal_test.go:9`<br>`TestStopSidecarProcessAppliesSignalPlan` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/sidecar_signal_test.go:24`<br>`TestStopSidecarProcessAppliesImmediateKillPlan` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/unavailable_provider_test.go:12`<br>`TestUnavailableProviderRejectsAllMarketDataOperations` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/unavailable_provider_test.go:72`<br>`TestRuntimeUnavailableProviderGuardsAndRecordsRetry` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/watchlist_source_test.go:15`<br>`TestWatchlistSnapshotSourceRoutesFutuAndYFinance` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/watchlist_source_test.go:74`<br>`TestWatchlistSnapshotSourceRoutesAKShareAndPreservesMissingValues` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/watchlist_source_test.go:100`<br>`TestWatchlistSnapshotSourcePreservesFutuPermissionErrorsWithoutPythonQuery` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/watchlist_source_test.go:121`<br>`TestWatchlistSnapshotSourceReportsUnavailableBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/marketdataapp/watchlist_source_test.go:175`<br>`TestWatchlistQuoteUsesPriorCloseForClosedRegularYahooQuote` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/dependencies_test.go:13`<br>`TestCheckNodeRuntimeDependencyOK` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/dependencies_test.go:39`<br>`TestCheckNodeRuntimeDependencyReportsMissing` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/dependencies_test.go:60`<br>`TestCheckNodeRuntimeDependencyUsesMacOSCommonPathFallback` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/dependencies_test.go:94`<br>`TestCheckNodeRuntimeDependencyMissingMessageListsMacOSAttempts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/dependencies_test.go:115`<br>`TestCheckNodeRuntimeDependencyReportsOutdatedInvalidAndCommandError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/dependencies_test.go:141`<br>`TestRuntimeDependenciesAggregatesRequiredStatus` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/dependencies_test.go:156`<br>`TestNodeRuntimeDependencyMessagesHandleInvalidAndTruncatedOutput` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/environment_fallbacks_test.go:14`<br>`TestRuntimeFallbacksAndEnvironmentOverridesRemainUsable` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/research_runtime_test.go:9`<br>`TestResearchDatabasePathAndRuntimeResource` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/resources_test.go:9`<br>`TestRuntimeResourcesDeclareOwnersAndDerivedPaths` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/resources_test.go:58`<br>`TestRuntimeResourcesHonorRealTradeControlPathOverride` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/resources_test.go:78`<br>`TestRuntimeResourceSummaryIncludesCountAndItems` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/runtime_test.go:12`<br>`TestLaunchDefaultsForDevelopmentMode` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/runtime_test.go:28`<br>`TestLaunchDefaultsForEmbeddedFrontendMode` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/runtime_test.go:44`<br>`TestBindHelpers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/runtime_test.go:59`<br>`TestEnsureRuntimeLayout` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/runtime_test.go:82`<br>`TestRuntimePathEnvOverrides` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/runtime_test.go:116`<br>`TestRuntimePathDerivationFallsBackForRelativeSettings` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/runtime_test.go:155`<br>`TestDeriveDesktopLogPaths` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/runtime_test.go:170`<br>`TestBindHelpersRejectMalformedAndIPv6Binds` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtime/runtime_test.go:185`<br>`TestIntegrationWithEnvDefaults` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:110`<br>`TestNilHandleSupportsOptionalRuntimeAssembly` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:161`<br>`TestHandlePublishesRuntimeGroupsBeforeShutdown` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:250`<br>`TestHandleRejectsConcreteResourcesAfterShutdown` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:331`<br>`TestHandleRestoresCalendarResolverOnShutdown` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:370`<br>`TestHandleRefusesPublicationWhenResourceGroupAlreadyClosed` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:417`<br>`TestHandleConcurrentCloseIsIdempotentAndAggregatesErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:475`<br>`TestHandleClosesConsumersBeforeProvidersAndProvidersInReverseOrder` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:513`<br>`TestHandleSerializesStrategyRuntimeAndPineRunnerUpdates` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:552`<br>`TestHandleClosesPineRunnersInjectedAfterShutdown` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:573`<br>`TestHandleRejectsAssistantRuntimeInjectedAfterShutdown` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:592`<br>`TestHandleCloseAndAssistantPublicationAreAtomic` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:622`<br>`TestHandleCreatesAndRegistersOnePineManagerConcurrently` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:20`<br>`TestStartForRunArgsInitializesRuntimeLayout` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:73`<br>`TestStartForRunArgsDisabledReturnsNoop` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:85`<br>`TestStartDesktopDoesNotMutatePersistedWebAccessSettings` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:150`<br>`TestResolveDesktopRuntimeConfigUsesProfileBindInsteadOfPersistedInterfaceBind` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:176`<br>`TestResolveDesktopRuntimeConfigRejectsEphemeralAPIBind` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:184`<br>`TestStartDesktopWithConfigKeepsResolvedProfileBind` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:224`<br>`TestResolvePackagedDesktopRuntimeRequiresLoopback` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:250`<br>`TestStartDesktopStartsAPIButNotLegacyGUIServer` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:280`<br>`TestStartDesktopAllowsWailsDevOrigin` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:310`<br>`TestDesktopTrustedOriginsDeriveDevelopmentPort` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:331`<br>`TestStartDesktopAllowsPackagedWailsOrigins` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:359`<br>`TestStartDesktopReportsAPIBindFailure` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:472`<br>`TestDependenciesApplyScheduledDatabaseRebuildBeforeStartup` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/server_test.go:518`<br>`TestRunAPIOnlyReturnsAfterContextCancellation` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/adk_data_management_test.go:15`<br>`TestDataManagementADKCleanupAndCompactionPaths` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/adk_data_management_test.go:60`<br>`TestDatabaseMaintenanceADKBusyAndPurgeBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/assistant_transport_lifecycle_test.go:11`<br>`TestServerCloseClosesAssistantHTTPTransport` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/broker_read_query_default_test.go:11`<br>`TestBrokerReadQueryDefaultMarket` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/data_management_failure_boundaries_test.go:19`<br>`TestDataManagementBackendRemainingOperations` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/data_management_failure_boundaries_test.go:61`<br>`TestDatabaseMaintenanceRemainingBusyReasons` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/data_management_failure_boundaries_test.go:93`<br>`TestDataManagementRemainingPurgeAndCompactBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/data_management_failure_boundaries_test.go:141`<br>`TestDataManagementRemainingStorePurgeFailures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/data_management_failure_boundaries_test.go:178`<br>`TestCompactBacktestRejectsInvalidDatabasePath` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/data_management_failure_boundaries_test.go:191`<br>`TestDataManagementStatusErrorIsIgnoredByPathLookup` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/data_management_test.go:17`<br>`TestBacktestRunMaintenanceKeepsMemoryAndDatabaseInSync` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/data_management_test.go:59`<br>`TestDataManagementServerCleanupAndCompactionPaths` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/data_management_test.go:124`<br>`TestDataManagementAdaptersRejectBusyRuntimeAndMapStalePreview` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/data_management_test.go:191`<br>`TestTranslateDataManagementErrors` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/desktop_token_test.go:14`<br>`TestDesktopTokenMiddlewareProtectsHTTPAndWebSocket` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/desktop_token_test.go:46`<br>`TestDesktopSidecarDoesNotDoubleAsBrowserListener` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/desktop_token_test.go:77`<br>`TestDesktopDevelopmentWithoutInjectedTokenRemainsTrusted` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/desktop_token_test.go:101`<br>`TestDesktopDevelopmentWebListenerStillRequiresPasswordSession` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/desktop_token_test.go:127`<br>`TestStandaloneServerWithoutDesktopTokenIsNotTrusted` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/exec_writeback_test.go:11`<br>`TestExecutionPushHandlersWriteBackAndNotify` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/exec_writeback_test.go:127`<br>`TestRecordPlacedOrderReusesExistingBrokerDiscoveredOrder` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/instrument_ref_test.go:8`<br>`TestNormalizeInstrumentInput` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/live_adapter_volume_test.go:11`<br>`TestMarketTradeFromTickUsesExplicitVolumeDelta` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/live_adapter_volume_test.go:29`<br>`TestMarketTradeFromTickKeepsDecimalVolumeWhenLegacyQuantityOverflows` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/live_adapter_volume_test.go:49`<br>`TestMarketTradeFromTickRejectsAmbiguousOrInvalidDelta` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/live_heartbeat_boundaries_test.go:27`<br>`TestLiveHeartbeatActiveInstrumentDeduplicationBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/live_runtime_test.go:13`<br>`TestLiveStreamDiagnosticsUseConfiguredLimit` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/live_ws_boundaries_test.go:36`<br>`TestLiveWebSocketUsesActivePollOnlyProviderBehindLegacyFutuSelection` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/live_ws_boundaries_test.go:114`<br>`TestLiveWebSocketBackendProviderAndNilBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/live_ws_boundaries_test.go:172`<br>`TestLiveWebSocketBackendPollsExplicitBrokerSnapshots` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/market_depth_test.go:33`<br>`TestMarketDepthWebSocketSendsInitialPayload` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/market_depth_test.go:117`<br>`TestMarketDepthOpenDError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/market_details_ws_test.go:11`<br>`TestMarketSecurityDetailsWebSocketSendsInitialPayload` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/market_instrument_resolver_test.go:13`<br>`TestMarketInstrumentResolverEndpointUsesSearchWithoutQuoteSubscription` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/market_instrument_resolver_test.go:65`<br>`TestMarketInstrumentResolverQualifiedInputOnlyQueriesSelectedLeaf` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/market_realtime_test.go:18`<br>`TestMarketCandlesEndpointIncludesCurrentRealtimeBucket` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/notification_market_workflow_contracts_test.go:18`<br>`TestBusinessNotificationsAndOptionalSecurityFieldsPreserveWireSemantics` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/notification_market_workflow_contracts_test.go:84`<br>`TestMarketQueryAndExecutionPayloadFallbacksRemainDeterministic` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/notification_market_workflow_contracts_test.go:140`<br>`TestRealTradeControlPathPrefersExplicitOverride` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/notification_market_workflow_contracts_test.go:154`<br>`TestWorkflowMarketAdaptersRejectInvalidInputsAndPreserveMetadata` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/notification_sources_test.go:13`<br>`TestExchangeCalendarAlertRecordingHonorsNotificationSetting` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/notification_sources_test.go:46`<br>`TestLiveNotificationFromExchangeCalendarAlertMapsSourceAndCategory` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/notifications_lifecycle_test.go:17`<br>`TestServerCloseUnregistersOnlyItsBBGONotificationSink` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/notifications_lifecycle_test.go:58`<br>`TestLiveNotificationEventMapContract` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/notifications_lifecycle_test.go:92`<br>`TestRecordLiveNotificationCallsSink` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/notifications_lifecycle_test.go:117`<br>`TestSystemNotificationTestRouteReturnsDeliveryStatus` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/notifications_lifecycle_test.go:144`<br>`TestRecordLiveNotificationSinkPanicDoesNotDropEvent` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/openapi_boundary_test.go:11`<br>`TestSwaggerRoutesRedirectToBrowsableDocumentation` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/openapi_route_registration_test.go:17`<br>`TestOpenAPICoversRegisteredAPIRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/openapi_route_registration_test.go:85`<br>`TestCapabilityCatalogAPISurfacesAreRegistered` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/product_infrastructure_boundaries_test.go:13`<br>`TestServerTradingStoreAndGatewaySmallBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/product_infrastructure_boundaries_test.go:33`<br>`TestProductInfrastructureRemainingNilAndFallbackBoundaries` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/product_lifecycle_closure_test.go:14`<br>`TestProductLifecycleFeeUpdatesPersistOnlyOnLiveOrderLedger` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/product_lifecycle_closure_test.go:39`<br>`TestPredictionAndPreviewPersistenceRejectsStaleOrChangedBindings` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/product_lifecycle_closure_test.go:199`<br>`TestProductLifecycleSnapshotIdentityAndRuntimeHelpers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/product_lifecycle_closure_test.go:206`<br>`TestProductLifecycleExecutionGatewayGuardsAndSubscriptionFallback` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/product_lifecycle_closure_test.go:255`<br>`TestProductLifecycleStartupBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/request_observability_test.go:14`<br>`TestRequestObservabilityInjectsStableContextAndRecordsSummary` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/request_observability_test.go:46`<br>`TestRequestObservabilityReplacesUnsafeRequestID` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_integration_boundaries_test.go:17`<br>`TestStartupIntegrationRemainsEffectiveWithoutPersistedBrokerSettings` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_integration_boundaries_test.go:65`<br>`TestPersistenceStoreUnwrapsStartupIntegrationForMCPSettings` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_integration_boundaries_test.go:106`<br>`TestFutuBrokerRefreshesAfterRuntimeResetAndStaysHiddenWhenDisabled` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_integration_boundaries_test.go:136`<br>`TestExplicitFutuResolutionRestoresRuntimeBrokerAlongsideOtherBrokers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_integration_boundaries_test.go:151`<br>`TestActiveBrokerWaitsForFutuRuntimeResetInvalidation` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_observation_test.go:13`<br>`TestStrategyRuntimeObservationAppearsInStrategiesAndSystemStatus` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_observation_test.go:114`<br>`TestStrategyRuntimeObservationPersistsAcrossServerRestart` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_observation_test.go:176`<br>`TestStrategyRuntimePanicAutoReconcilesToStopped` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_polling_test.go:14`<br>`TestStrategyRuntimePollsClosedKLinesWhenTradePushStalls` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_test.go:16`<br>`TestStrategyRuntimeNotifyOnlyEmitsSignalNotification` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_test.go:82`<br>`TestStrategyRuntimeStartEnsuresMissingMarketMetadata` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_test.go:119`<br>`TestStrategyRuntimeStartRejectsWhenInstanceWorkerLimitReached` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_test.go:166`<br>`TestStrategyRuntimeLiveWorkerRequestIncludesModeCandlesAndParams` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_test.go:212`<br>`TestStrategyRuntimeLiveWorkerErrorRecordsRuntimeError` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:18`<br>`TestStrategyRuntimeOrderUsesSharedPreTradeRiskGateway` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:49`<br>`TestStrategyRuntimeLiveModeRecordsExecutionOrder` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:123`<br>`TestStrategyRuntimeRiskCloseOnlyRejectsBuyOrder` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:181`<br>`TestStrategyRuntimeLiveSizesEntryQuantityPctFromEquity` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:239`<br>`TestStrategyRuntimeLiveUsesExplicitQuantityBeforeQuantityPct` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:286`<br>`TestStrategyRuntimeLiveSizesCloseQuantityPctFromPosition` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:334`<br>`TestStrategyRuntimeLiveDefaultsCloseToFullPosition` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:382`<br>`TestStrategyRuntimeLiveIgnoredOrderRecordsRuntimeEvidence` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:443`<br>`TestStrategyRuntimeLiveCancelsTrackedOrderFromWorkerCommand` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:506`<br>`TestStrategyRuntimeExecutesOnlyCurrentBarWorkerIntent` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:575`<br>`TestStrategyRuntimeSkipsWhenWorkerReturnsNoCurrentBarIntent` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:635`<br>`TestStrategyRuntimeRefreshesBrokerPositionsBeforeSellOnKLineClose` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:723`<br>`TestStrategyRuntimeDisconnectedBrokerRefreshKeepsCachedState` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/security_test.go:43`<br>`TestSecurityChangeCancelsExistingWebStream` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_application_lifecycle_test.go:37`<br>`TestServerCloseUsesApplicationResourceOrderAndStableAggregation` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_application_lifecycle_test.go:75`<br>`TestServerCloseStopsBacktestBeforeMarketDataRuntime` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_application_lifecycle_test.go:118`<br>`TestPersistentAssemblyStopsAndRollsBackInReverseOpenOrder` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_backtest_test.go:19`<br>`TestBacktestRouteAcceptsExplicitMarketAndCode` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_backtest_test.go:97`<br>`TestEnqueueBacktestUsesPineInitialCapitalWhenRequestOmitsBalance` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_bootstrap_boundaries_test.go:15`<br>`TestServerBootstrapRemainingFailureAndFallbackPaths` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_bootstrap_boundaries_test.go:64`<br>`TestServerRemainingPublicSettersAndRuntimeBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_bootstrap_boundaries_test.go:107`<br>`TestServerRemainingBrokerAndSystemOptionBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_bootstrap_degraded_runtime_test.go:15`<br>`TestServerBootstrapPersistsUnavailableDatabaseReasons` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_bootstrap_degraded_runtime_test.go:67`<br>`TestServerOptionCallbacksExposeNilRuntimeStatesSafely` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_bootstrap_degraded_runtime_test.go:84`<br>`TestServerBootstrapBuildsBrokerBridgeForEnabledIntegration` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_business_test.go:25`<br>`TestWorkflowAndMarketRuntimeBoundaryHelpers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_business_test.go:42`<br>`TestStrategyRuntimeBrokerBridgeDelegates` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_business_test.go:77`<br>`TestTimeStatusAndDefaultScriptBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_business_test.go:92`<br>`TestStrategyBindingHelpersNormalizeLooseAPIParams` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_business_test.go:263`<br>`TestServerCloseAggregatesPineWorkerRunnerErrorsOnce` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_business_test.go:292`<br>`TestServerSidecarFrontendRuntimeConfigFollowsSecuritySettings` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_business_test.go:329`<br>`TestBrokerExecutionExchangeDoesNotReuseStrategyMarketDataOverride` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_calendar_boundaries_test.go:12`<br>`TestServerCalendarOptionsAndOperationsRemainingBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_lifecycle_test.go:16`<br>`TestInstantiatePineStrategyDefinitionBuildsCompiledPlan` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_lifecycle_test.go:222`<br>`TestInstantiateStrategyDefinitionRejectsMalformedJSON` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_market_test.go:11`<br>`TestMarketDataSubscriptionHeartbeat` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_option_callbacks_test.go:13`<br>`TestServerSystemAndSettingsOptionCallbacks` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_option_callbacks_test.go:53`<br>`TestServerStrategyDemandWithRuntimeManager` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_runtime_side_effects_test.go:47`<br>`TestServerRuntimeRiskControlsDelegateToControlPlane` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_runtime_side_effects_test.go:143`<br>`TestServerSettingsSideEffectsPropagateRuntimeChanges` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_runtime_side_effects_test.go:228`<br>`TestDataManagementBackendNilManagerBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_test.go:14`<br>`TestShouldStartForAPIOnlyArgs` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_test.go:26`<br>`TestPersistenceOnlySettingsStoreKeepsConcreteStore` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_test.go:36`<br>`TestExchangeCalendarOperationContextIgnoresRequestCancellation` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_test.go:50`<br>`TestNewServerUsesStrategyRuntimeDBEnvOverride` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_test.go:70`<br>`TestServerCloseStopsMarketdataAndPreventsExchangeRevival` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/server_warmup_test.go:20`<br>`TestBacktestRouteUsesDerivedStrategyWarmup` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/settings_broker_futu_health_test.go:16`<br>`TestFutuRuntimeAndHealthDiagnoseEnabledButUnreachableOpenD` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/settings_broker_futu_health_test.go:66`<br>`TestFutuOpenDHealthRejectsOldBuildAndGuidesUpgrade` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/settings_market_data_test.go:11`<br>`TestServerSettingsStoreDefaultsAndPersistsMarketDataSelection` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/settings_market_data_test.go:32`<br>`TestStartupKeepsConfiguredYFinanceWhenHelperCannotActivate` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/settings_security_test.go:14`<br>`TestWebAccessSettingsDefaultToDesktopOnly` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/settings_security_test.go:37`<br>`TestDesktopCanEnablePasswordProtectedWebWithoutExposingPassword` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/settings_security_test.go:86`<br>`TestWebAccessCannotBeEnabledWithoutPassword` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/settings_security_test.go:95`<br>`TestBrowserSessionCannotChangeWebExposure` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/settings_security_test.go:116`<br>`TestDisablingWebImmediatelyInvalidatesBrowserButNotDesktop` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/strategy_definition_delete_guard_test.go:16`<br>`TestDeleteStrategyDefinitionRequiresDeletingLinkedInstancesFirst` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/strategy_runtime_dependency_boundaries_test.go:14`<br>`TestStrategyRuntimeRemainingDependencyBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/strategy_runtime_dependency_boundaries_test.go:47`<br>`TestNewStrategyRuntimeManagerDisabledExchange` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/strategy_runtime_dependency_boundaries_test.go:59`<br>`TestStrategyRuntimeRejectsPollOnlyMarketDataProvider` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/strategy_runtime_live_semantics_test.go:12`<br>`TestStrategyRuntimeAdapterAllowsBrokerExecutedLiveSemantics` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/strategy_runtime_nil_boundaries_test.go:11`<br>`TestStrategyRuntimeNilBoundariesIgnoreMarketTicks` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/strategy_runtime_workflow_replay_test.go:13`<br>`TestWorkflowSnapshotAndLiveTradeReplayReachBusinessConsumers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/strategy_runtime_workflow_replay_test.go:55`<br>`TestWorkflowSnapshotReturnsProviderFailureAfterStaleCache` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/strategy_subscription_lifecycle_test.go:12`<br>`TestStrategyRuntimeHoldsExactKLineLeasesUntilStopAndClose` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/system_reconcile_strategy_states_test.go:11`<br>`TestNewServerReconcilesPersistedActiveStrategyStates` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/trading_order_cancellation_contracts_test.go:98`<br>`TestTradingOrderCancellationRejectsInvalidPersistedOrders` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/trading_order_cancellation_contracts_test.go:134`<br>`TestTradingOrderCancellationPropagatesBrokerFailuresAndPersistsAcceptedCancel` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/watchlist_runtime_boundaries_test.go:52`<br>`TestFutuWatchlistProbeErrorRemainingStates` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/watchlist_runtime_boundaries_test.go:72`<br>`TestFutuWatchlistBrokerRemainingAvailabilityAndCapabilities` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/watchlist_runtime_boundaries_test.go:127`<br>`TestInitializeWatchlistServiceNilBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/ws_events_test.go:20`<br>`TestLiveWebSocketSendsHeartbeat` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/ws_events_test.go:45`<br>`TestLiveWebSocketSendsSystemNotification` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/ws_events_test.go:80`<br>`TestLiveWebSocketSendsBBGONotification` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/ws_events_test.go:112`<br>`TestLiveWebSocketHeartbeatReportsStaleMarketData` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/ws_events_test.go:152`<br>`TestLiveWebSocketInitialMarketTickRefreshesObservedAt` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercore/ws_events_test.go:215`<br>`TestLiveWebSocketSendsConsoleRefresh` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/backtest_provider_runtime_test.go:27`<br>`TestBacktestSyncUsesAssembledMarketDataRuntime` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/backtest_provider_runtime_test.go:55`<br>`TestBacktestSyncRejectsActualAKShareOneYearUSFiveMinuteRange` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/backtest_provider_runtime_test.go:119`<br>`TestLiveBacktestHistoricalProviders` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/backtest_provider_runtime_test.go:162`<br>`TestLiveBABAOneYearBacktestProviderSwitchMatrix` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/backtest_runs_test.go:34`<br>`TestNewServerReloadsPersistedBacktestRuns` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/backtest_runs_test.go:127`<br>`TestBacktestRouteDeletesTerminalRuns` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/backtest_runs_test.go:206`<br>`TestBacktestListReturnsLightweightRunsAndResultReturnsDetail` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/backtest_runs_test.go:279`<br>`TestBacktestRoutesCreateRuntimeLayoutForMissingBacktestDir` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:100`<br>`TestBrokerFundsIncludesMarginFields` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:126`<br>`TestBrokerQuoteWithoutLeaseIsDegraded` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:141`<br>`TestBrokerQuoteMissingSymbol` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:155`<br>`TestBrokerKLinesDisconnected` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:167`<br>`TestBrokerKLinesMissingSymbol` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:181`<br>`TestBrokerSecuritiesDisconnected` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:193`<br>`TestBrokerSecuritiesMissingSymbol` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:205`<br>`TestBrokerReadRoutesRejectInvalidQueryShape` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:237`<br>`TestBrokerReadRoutesKeepValidDisconnectedShape` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:264`<br>`TestBrokerUnlockDisconnectedOpenD` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:286`<br>`TestBrokerUnlockInvalidPayload` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:302`<br>`TestBrokerPlaceOrderNoBroker` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:319`<br>`TestBrokerPlaceOrderInvalidPayload` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:336`<br>`TestBrokerCancelOrdersNoBroker` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:359`<br>`TestBrokerCancelOrdersInvalidPayload` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:388`<br>`TestBrokerFundsSummaryHasAllFields` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:422`<br>`TestNewBrokerRoutesReturnJSON` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_new_test.go:462`<br>`TestBrokerGinRoutesRejectIncompletePaths` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_read_test.go:72`<br>`TestBrokerReadEndpointsReturnExchangeBackedData` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_routes_test.go:14`<br>`TestBrokerFundsEndpointReturnsDisconnectedSummary` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/broker_routes_test.go:61`<br>`TestBrokerRuntimeDescriptorIncludesReadFeatures` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/contract_test.go:13`<br>`TestContractSystemStatus` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/contract_test.go:90`<br>`TestContractSettings` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/contract_test.go:123`<br>`TestContractMarketDataMarkets` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/contract_test.go:165`<br>`TestContractBrokerRuntime` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/contract_test.go:205`<br>`TestContractStrategyDefinitions` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/contract_test.go:240`<br>`TestContractBacktests` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/exec_routes_test.go:18`<br>`TestExecutionOrderRoutesPlaceListEventsAndCancel` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/exec_validate_test.go:18`<br>`TestExecutionOrderRoutesNormalizeUSPricePrecision` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/exec_validate_test.go:88`<br>`TestExecutionOrderRoutesPropagateUSSessionSelection` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/exec_validate_test.go:153`<br>`TestExecutionOrderRoutesAcceptExplicitCodeWithMarket` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/exec_validate_test.go:211`<br>`TestExecutionOrderRoutesRejectBareSymbolWithoutMarket` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/execution_routes_test.go:53`<br>`TestExecutionOrdersEndpointFiltersByTradingEnvironmentAndScope` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/execution_routes_test.go:110`<br>`TestExecutionOrdersEndpointDefaultTradingEnvironmentFromSettings` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/execution_routes_test.go:155`<br>`TestExecutionOrdersSyncBrokerOrdersAndTracksWorkerState` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/frontend_test.go:25`<br>`TestServerServesFrontendAssetsAndSPAFallback` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/frontend_test.go:96`<br>`TestDesktopWebAccessHandlerServesProxiedDevelopmentUIAtRoot` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/frontend_test.go:137`<br>`TestFrontendServerBoundaryHelpers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/frontend_test.go:157`<br>`TestStartForRunArgsInitializesRuntimeLayout` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/frontend_test.go:198`<br>`TestRunAPIOnlyStopsAfterCallerCancellation` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/frontend_test.go:246`<br>`TestStartForRunArgsUsesInterfaceSettingsForAPIBindWhileWebIsDisabled` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/installers_degraded_test.go:13`<br>`TestInstallersPreserveDegradedStartupWhenAssistantDatabasesAreUnavailable` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/market_depth_routes_test.go:31`<br>`TestMarketDepthEndpointRouting` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/market_depth_routes_test.go:54`<br>`TestMarketDepthEndpointMethodNotAllowed` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/market_depth_routes_test.go:72`<br>`TestMarketDepthEndpointPutNotAllowed` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/market_depth_routes_test.go:92`<br>`TestMarketDepthRouteDoesNotCollide` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/market_profiles_test.go:15`<br>`TestMarketProfilesEndpoint` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/market_profiles_test.go:79`<br>`TestNormalizeMarketInstrumentEndpoint` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/openapi_schema_compatibility_test.go:14`<br>`TestOpenAPIPreservesLegacySchemaNames` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/openapi_snapshot_test.go:20`<br>`TestOpenAPISpecStable` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/openapi_snapshot_test.go:89`<br>`TestOpenAPIDocumentsExplicitErrorResponses` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/openapi_snapshot_test.go:136`<br>`TestOpenAPIDocumentsWritableRequestBodies` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/openapi_snapshot_test.go:204`<br>`TestOpenAPIDocumentsTypedBrokerRuntimeResponse` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/plugin_lifecycle_test.go:16`<br>`TestPluginCatalogLifecycleEndpoints` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/portfolio_routes_test.go:12`<br>`TestPortfolioCashBalancesEndpointReturnsEmptyBalances` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/portfolio_routes_test.go:43`<br>`TestPortfolioReconciliationEndpointsAreRemoved` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/portfolio_routes_test.go:71`<br>`TestPortfolioRoutesReturnDegradedEmptyStateWithoutConfiguredBroker` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/research_runtime_test.go:14`<br>`TestServerInitializesResearchDatabaseAndPresetRoutes` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/research_runtime_test.go:36`<br>`TestResearchPresetRoutesReturn503WhenDatabaseCannotOpen` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/runtime_defaults_test.go:11`<br>`TestLaunchDefaultsForDevelopmentMode` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/runtime_defaults_test.go:28`<br>`TestLaunchDefaultsForEmbeddedFrontendMode` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/runtime_defaults_test.go:48`<br>`TestAPIBaseURLForBindNormalizesWildcardHost` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/server_business_public_test.go:15`<br>`TestRuntimeDefaultsAndLayoutBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/server_business_public_test.go:48`<br>`TestServerSidecarBoundaryMethodsAreNilSafe` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/server_definitions_test.go:17`<br>`TestStrategyDefinitionEndpoints` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/server_definitions_test.go:236`<br>`TestStrategyDefinitionCreateGeneratesUUIDWhenIDMissing` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/server_definitions_test.go:291`<br>`TestStrategyDefinitionRejectsInvalidScriptPayloads` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/server_definitions_test.go:316`<br>`TestDeleteMissingStrategyDefinitionReturnsNotFound` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_broker_test.go:18`<br>`TestBrokerIntegrationSavePersistsWithoutMutatingRuntimeEnv` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_broker_test.go:95`<br>`TestSettingsStoreDirectSaveDoesNotMutateRuntimeEnv` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_broker_test.go:120`<br>`TestBrokerSettingsExposeNullIntegrationUntilFirstSave` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_broker_test.go:162`<br>`TestFutuRuntimeAndHealthStayNeutralWithoutSavedEnabledIntegration` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_broker_test.go:281`<br>`TestManagedBrokerAccountCRUDReflectsInBrokerSettings` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_broker_test.go:403`<br>`TestUIAppearanceSavePersistsToSettings` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_interfaces_test.go:14`<br>`TestEnsureBootstrapFilePersistsInterfaceDefaults` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_interfaces_test.go:58`<br>`TestInterfaceSettingsUsesStoredOverride` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_normalization_test.go:10`<br>`TestNormalizeManagedBrokerAccountAppliesDefaults` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_normalization_test.go:40`<br>`TestNormalizeFutuConfigAppliesDefaults` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_normalization_test.go:69`<br>`TestNormalizeExecutionSettingsAppliesDefaultsAndBounds` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_onboarding_test.go:18`<br>`TestOnboardingDefaultsAndSave` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_onboarding_test.go:61`<br>`TestOnboardingRoutesSuggestOobeUntilCompleted` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/settings_onboarding_test.go:149`<br>`TestOnboardingReopensWhenRuntimeDependencyFailsAfterCompletion` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/strategy_logs_test.go:15`<br>`TestStrategiesEndpointReturnsList` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/strategy_logs_test.go:120`<br>`TestStrategiesEndpointIncludesPersistedRuntimeLogTail` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/strategy_logs_test.go:169`<br>`TestStrategyLogsAndAuditEndpointsSupportPaginationAndFilters` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/strategy_preview_test.go:28`<br>`TestInstantiateStoredDefinitionRejectsLegacySourceFormat` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/strategy_preview_test.go:99`<br>`TestStrategyDefinitionPreviewUsesRequestedSymbolAndExtendedHours` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/strategy_sync_test.go:17`<br>`TestStrategiesExposeDefinitionSyncAndRefreshDefinitionRoute` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/swagger_openapi_test.go:14`<br>`TestSwaggerUIAvailable` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/swagger_openapi_test.go:72`<br>`TestOpenAPISpecExposesCorePaths` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/system_routes_test.go:12`<br>`TestSystemStatusEndpointReturnsStatus` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/system_routes_test.go:89`<br>`TestRequestObservabilityMiddlewarePropagatesRequestID` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/system_routes_test.go:112`<br>`TestSystemStatusReflectsUpdatedAPIPort` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/watchlist_runtime_test.go:14`<br>`TestServerInitializesWatchlistDatabaseAndDefaultGroup` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/servercoretest/watchlist_runtime_test.go:51`<br>`TestWatchlistRoutesReturn503WhenDatabaseCannotOpen` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/status/status_test.go:12`<br>`TestLiveStatsSortsActiveInstruments` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/status/status_test.go:26`<br>`TestMarketDataRuntimeSummaryStates` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/status/status_test.go:76`<br>`TestStrategyRuntimeSummaryDelegatesAndDefaults` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/status/status_test.go:88`<br>`TestTimeAndStringPointers` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/stores/handle_test.go:10`<br>`TestHandleClosesStoresInReverseOpenOrder` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/stores/handle_test.go:42`<br>`TestHandleRollsBackAndStopsAfterOpenFailure` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/strategyapp/runtime_ports_test.go:62`<br>`TestAccountResolverRequiresExactTradableBrokerAndDelegatesReads` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/strategyapp/runtime_ports_test.go:110`<br>`TestMarketDataCapabilitiesReadsRuntimeDescriptor` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/strategyapp/runtime_ports_test.go:137`<br>`TestMarketDataHealthReturnsActiveProviderHealth` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/strategyapp/runtime_ports_test.go:145`<br>`TestTradeCommandsMapPlaceCancelAndDefensiveFailures` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/execution_gateway_boundaries_test.go:10`<br>`TestComboOrderQuantityModeMapsEventParlaysToAmount` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/execution_gateway_boundaries_test.go:19`<br>`TestNormalizedBrokerComboIntentKeepsClientOrderIdentity` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/execution_gateway_lifecycle_test.go:144`<br>`TestExecutionGatewayPlaceOrderBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/execution_gateway_lifecycle_test.go:202`<br>`TestExecutionGatewayCancelOrderBoundaries` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/execution_gateway_lifecycle_test.go:250`<br>`TestExecutionGatewayPlaceComboBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/execution_gateway_lifecycle_test.go:298`<br>`TestExecutionGatewayCancelComboBoundaries` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/execution_gateway_lifecycle_test.go:331`<br>`TestExecutionGatewayHelpers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/notifications_lifecycle_test.go:10`<br>`TestOrderPlacedNotificationMapsBrokerLabelAndMessage` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/notifications_lifecycle_test.go:29`<br>`TestOrderLifecycleNotificationMapsSubmittedCancelledAndFilled` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/notifications_lifecycle_test.go:62`<br>`TestExecutionOrderNotificationMessageOmitsBlankParts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/notifications_test.go:10`<br>`TestOrderLifecycleNotificationHandlesUnrelatedAndPartialFillEvents` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/notifications_test.go:32`<br>`TestExecutionOrderNotificationMessageIncludesAvailableIdentifiers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/order_update_source_broker_test.go:12`<br>`TestOrderUpdateSourceActivatesThenDiscoversAccounts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/order_update_source_broker_test.go:33`<br>`TestOrderUpdateSourceSubscribeFiltersFutuAccounts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/order_update_source_test.go:88`<br>`TestProductLifecycleOrderUpdateSourceAggregatesBrokersAndFees` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/order_update_source_test.go:172`<br>`TestProductLifecycleOrderUpdateSourceSkipsFundOnlyAccounts` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/order_updates_test.go:13`<br>`TestNewOrderUpdatesWorkerConstructsWorker` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/order_updates_test.go:20`<br>`TestBrokerOrderMappingsPreserveLifecycleFields` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/order_updates_test.go:70`<br>`TestBrokerOrderQueryTrimsRuntimeIdentifiers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/order_updates_test.go:80`<br>`TestExecutionOrderUpdatesIgnoreMissingLedger` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/order_updates_test.go:92`<br>`TestOrderUpdateSourceDegradesCleanlyWithoutActiveFutuRuntime` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/tradingapp/order_updates_test.go:114`<br>`TestExecutionOrderUpdatesPersistBrokerLifecycleFields` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:43`<br>`TestWebAuthRemainingNilAndRequestHelpers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:101`<br>`TestWebAuthRemainingAuthenticationStates` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:138`<br>`TestWebAuthRemainingLoginResponses` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:195`<br>`TestPasswordChangeDuringLoginCannotCreateOldPasswordSession` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:235`<br>`TestWebAuthRemainingSessionAndPruningErrors` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:282`<br>`TestForwardedClientUsesProxyAppendedAddress` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:295`<br>`TestWebAuthStateMapsStayBounded` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:319`<br>`TestWebAuthRemainingCanceledPasswordSlotAndStatus` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/frontend_test.go:28`<br>`TestFrontendServesAssetsAndSPAFallback` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/frontend_test.go:50`<br>`TestFrontendRuntimeConfigAndAccessSurface` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/frontend_test.go:67`<br>`TestFrontendDevelopmentProxyOnlyAllowsLoopback` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/frontend_test.go:81`<br>`TestFrontendBoundaryHelpers` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/frontend_test.go:106`<br>`TestShouldServeFrontendIndexRequestBoundaries` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/frontend_test.go:139`<br>`TestFrontendRequestSchemeTrustsTLSAndLoopbackProxyOnly` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:175`<br>`TestWebPasswordIsRequiredForProtectedAPI` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:188`<br>`TestBrowserNavigationGetsFriendlyDisabledWebPage` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:210`<br>`TestSameHostHTTPSProxyUsesSecureSessionCookie` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:237`<br>`TestNetworkClientCannotSpoofHTTPSProxyScheme` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:256`<br>`TestSameHostProxyCannotBypassPublicAccessSetting` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:278`<br>`TestAdminBearerMechanismNoLongerAuthenticates` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:293`<br>`TestWebPasswordSessionSupportsReadAndCSRFProtectedWrite` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:335`<br>`TestWebLoginRejectsWrongPasswordAndRateLimits` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:352`<br>`TestPasswordChangesInvalidateWebSessions` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:362`<br>`TestProductionWebDoesNotTrustDevelopmentOrigin` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:379`<br>`TestWebLogoutClearsSessionCookie` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:400`<br>`TestWebLoginCookieIsHttpOnlyAndSameSiteStrict` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:422`<br>`TestUntrustedOriginIsRejectedButSameOriginLANHostWorks` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:437`<br>`TestLoopbackPolicyBlocksRemoteBrowserUntilExplicitlyEnabled` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:461`<br>`TestDesktopCapabilityStaysPasswordlessWhenWebAccessIsDisabled` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:490`<br>`TestWebSocketUsesCookieSessionWithoutDesktopToken` | api_transport | 高风险 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:510`<br>`TestRemovedAuthTokenRouteReturnsNotFound` | api_transport | 普通边界 | `crates/jftrade-api` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_backtest_adapter_test.go:8`<br>`TestADKStrategyValidationAndVisualModelBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_capability_contracts_test.go:12`<br>`TestADKNewCapabilityToolsForwardInputsAndApprovalBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_capability_contracts_test.go:94`<br>`TestADKNewCapabilityHandlersRejectUnavailableAndMalformedInputs` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_closure_contracts_test.go:12`<br>`TestADKToolDependencyClosuresForwardNormalizedOwnerPorts` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_closure_contracts_test.go:73`<br>`TestADKToolDependencyClosuresFailClosedWhenPortsAreMissing` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_product_catalog_test.go:13`<br>`TestDefaultBuiltinAgentToolsExistInAssembledRegistry` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_product_catalog_test.go:29`<br>`TestCapabilityCatalogSurfacesAreRegisteredAndMCPBounded` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_product_catalog_test.go:77`<br>`TestProductToolRegistryAndOperationSchemasAreCatalogBacked` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_product_catalog_test.go:138`<br>`TestProductReadSchemasRejectInvalidRoutingAndFreeTextFields` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_runtime_contracts_test.go:13`<br>`TestADKRuntimeStrategyToolsPreserveOwnerContracts` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_runtime_contracts_test.go:113`<br>`TestADKRuntimeOptimizationPersistsQueuedRunReferences` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_runtime_contracts_test.go:156`<br>`TestADKRuntimeResearchToolStopsBeforeRunWhenDataSyncIsPending` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_strategy_input_validation_test.go:10`<br>`TestStrategyADKInputsAndSummariesEnforceBusinessBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:19`<br>`TestADKCoreToolHandlersSurfaceSubscriptionErrors` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:32`<br>`TestADKWorkflowAuditAndAdapterHelpers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:77`<br>`TestADKSystemAndWorkflowToolHandlersReflectBusinessState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:194`<br>`TestADKStrategyToolsHandleNegativeAndFallbackScenarios` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:308`<br>`TestADKStrategyDefinitionVersionToolsExposeImmutableSnapshotsAndFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:397`<br>`TestADKBacktestRunsFiltersByDefinitionVersionStatusAndLimit` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:449`<br>`TestADKStrategyToolContractsCoverUnavailableAndSuccessfulViewScenarios` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:605`<br>`TestADKStrategyOptimizePersistsTasksAndCancelsQueuedRunsOnFailure` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:699`<br>`TestADKBacktestProviderFreezesDefaultAcrossPreparationAndQueue` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:774`<br>`TestADKConcurrentResearchBacktestOverridesStayIsolated` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_summary_contracts_test.go:9`<br>`TestADKStrategySummariesHideSourceDetailsAndCountLinkedInstances` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_summary_contracts_test.go:33`<br>`TestADKBacktestSummariesRetainCountsWithoutEmbeddingRawSeries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/adk_tool_failure_contracts_test.go:19`<br>`TestADKToolFailuresPreserveBusinessErrorContracts` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_boundaries_test.go:20`<br>`TestApplicationAdapterKeepsNilPortsCallable` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_boundaries_test.go:71`<br>`TestApplicationAdapterUsesConfiguredRuntimeAndSettings` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_boundaries_test.go:128`<br>`TestApplicationAdapterValidatesDomainInputsBeforeDelegation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:16`<br>`TestApplicationAdapterReportsUnavailableDomainServices` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:58`<br>`TestApplicationAdapterPropagatesExecutionProjectionFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:81`<br>`TestApplicationAdapterNormalizesCrossDomainInputs` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:123`<br>`TestApplicationAdapterForwardsMarketCandles` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:138`<br>`TestApplicationAdapterForwardsAdvancedMarketCandles` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:164`<br>`TestApplicationAdapterRejectsAdvancedCandleInputsBeforeProviderCall` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:185`<br>`TestApplicationAdapterExposesProviderAndRuntimePorts` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:226`<br>`TestApplicationAdapterProvidesScreenCatalogAndCancelResult` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:253`<br>`TestApplicationAdapterNormalizesStrategyVisualModels` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:278`<br>`TestApplicationAdapterProjectsBacktestState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:308`<br>`TestApplicationWorkflowSnapshotRejectsInvalidInstrument` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_strategy_lifecycle_test.go:84`<br>`TestApplicationAdapterStrategyInstanceLifecyclePorts` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_strategy_lifecycle_test.go:122`<br>`TestApplicationAdapterStrategyInstanceLifecycleRejectsInvalidInputs` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/application_strategy_lifecycle_test.go:141`<br>`TestApplicationAdapterStrategyInstanceLifecycleRejectsDefinitionAndActivityBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/maintenance_test.go:12`<br>`TestDatabaseMaintenanceOwnsADKBusyPurgeAndCompactPaths` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/maintenance_test.go:77`<br>`TestDatabaseMaintenanceFailsClosedWithoutOwnedRuntime` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/market_index_constituents_tools_test.go:14`<br>`TestADKMarketIndexConstituentsToolForwardsNormalizedInputs` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/market_index_constituents_tools_test.go:60`<br>`TestADKMarketIndexConstituentsToolFailsClosedWithoutPort` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/market_index_constituents_tools_test.go:69`<br>`TestADKMarketIndexConstituentsToolSurfacesProviderCapabilityAsClearMessage` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/market_news_tools_test.go:15`<br>`TestADKMarketNewsAndCorporateActionsToolsForwardNormalizedInputs` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/market_news_tools_test.go:93`<br>`TestADKMarketNewsAndCorporateActionsToolsFailClosedWithoutPorts` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/market_news_tools_test.go:104`<br>`TestADKMarketNewsToolSurfacesProviderCapabilityAsClearMessage` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/mcp_server_lifecycle_authorization_test.go:27`<br>`TestMCPServerManagerRemainingLifecycleBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/mcp_server_lifecycle_authorization_test.go:74`<br>`TestMCPServerManagerRemainingServeFailureStates` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/mcp_server_lifecycle_authorization_test.go:95`<br>`TestMCPAuthorizedHandlerRemainingRequestBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/mcp_server_test.go:22`<br>`TestMCPServerManagerEnforcesBearerAndSupportsTokenRotation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/mcp_server_test.go:76`<br>`TestMCPServerManagerStartsAndStopsOnLoopback` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/mcp_server_test.go:106`<br>`TestMCPServerManagerServesAuthenticatedStreamableMCP` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/mcp_server_test.go:181`<br>`TestMCPServerManagerListenerFailurePreservesRunningState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/mcp_server_test.go:203`<br>`TestMCPServerManagerReleasesHandlersOnReplacementDisableAndClose` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/mcp_server_test.go:257`<br>`TestMCPServerManagerReleasesHandlerOnUnexpectedServeExit` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/mcp_server_test.go:280`<br>`TestMCPServerManagerUsesLoopbackOnly` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:15`<br>`TestPortfolioSummaryScansAllRealAccountsAndRanksNonEmptyFirst` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:85`<br>`TestPortfolioLayeredToolsKeepDiscoveryOverviewAndPositionsSeparate` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:155`<br>`TestPortfolioLayeredToolsReportValidationDiscoveryAndPartialReadStates` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:255`<br>`TestPortfolioAccountResolutionSupportsExactSuffixAndIsolation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:288`<br>`TestPortfolioSummaryKeepsPartialAccountResultsAndDiscoveryFailuresVisible` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:338`<br>`TestAccountOrdersFiltersAccountEnvironmentMarketAndActiveStatus` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:376`<br>`TestApplicationPortfolioReadsRuntimeFundsAndPositionsFromOneBrokerAccount` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:444`<br>`TestApplicationPortfolioMarksIncompleteBrokerResponsesPartial` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/product_adapters_test.go:15`<br>`TestCustomizationToolsMapToOpenDOperations` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/product_adapters_test.go:26`<br>`TestProductToolInputHelpersCompleteBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/product_adapters_test.go:83`<br>`TestProductAndExecutionDispatchFailureBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/product_adapters_test.go:155`<br>`TestProductExecutionAdapterNormalizesScreenAndCalendarV2Inputs` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/product_adapters_test.go:186`<br>`TestProductExecutionAdapterRejectsInvalidScreenPageAndValue` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/product_adapters_test.go:217`<br>`TestProductExecutionAdapterCoversSpecialDispatchFailuresAndSnapshots` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/product_execution_contracts_test.go:84`<br>`TestProductExecutionAdapterPreservesProductAndExecutionBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/runtime_test.go:14`<br>`TestOpenBuildsToolsServiceAndIdempotentLifecycle` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/runtime_test.go:45`<br>`TestRuntimeDatabaseProbesUseProvidedLayout` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/runtime_test.go:55`<br>`TestOpenOwnsApplicationToolRegistration` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/runtime_test.go:74`<br>`TestHandleExposesNarrowAuditAndToolOperations` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/runtime_test.go:131`<br>`TestNilHandleLifecycleIsSafe` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/tool_catalog_test.go:16`<br>`TestADKRuntimeHelperInputNormalization` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/tool_catalog_test.go:159`<br>`TestADKRuntimePollingAndPayloadHelpers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/tool_catalog_test.go:272`<br>`TestADKReadToolsNormalizeInputsAndExposeBusinessHandlers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/tool_catalog_test.go:415`<br>`TestADKRuntimeMiscHelpersAndMetadata` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/tool_catalog_test.go:550`<br>`TestADKCoreToolHandlersNormalizeMarketAndPortfolioFlows` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/tool_catalog_test.go:672`<br>`TestWatchlistListToolDefaultsToReadOnlyMetadataAndNormalizesPaging` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/tool_catalog_test.go:707`<br>`TestExecutionReadToolsPropagateProjectionFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/typed_product_capabilities_test.go:11`<br>`TestTypedProductCapabilitiesDriveFeatureAndAssistantSchemas` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/typed_product_capabilities_test.go:45`<br>`TestAssistantSchemasCoverProviderAndResearchExtensions` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/watchlist_adapter_test.go:24`<br>`TestADKWatchlistListReturnsRealDataWithoutImplicitQuoteCalls` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/watchlist_adapter_test.go:72`<br>`TestWatchlistToolAdapterUnavailableAndMissingGroupBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_bridge_contracts_test.go:14`<br>`TestWorkflowManagerProjectsServiceCRUDAndRuns` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_bridge_contracts_test.go:103`<br>`TestWorkflowManagerRejectsUnavailableServicesAcrossOperations` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_execution_injection_test.go:60`<br>`TestRuntimeUsesInjectedWorkflowExecutionForLoopChat` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_tools_error_boundaries_test.go:52`<br>`TestWorkflowToolsRemainingManagerErrorPropagation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_tools_error_boundaries_test.go:103`<br>`TestWorkflowToolsRemainingSessionAndPayloadErrors` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_tools_test.go:15`<br>`TestWorkflowManagementToolCatalogAndApprovalMatrix` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_tools_test.go:68`<br>`TestWorkflowRunWaitReturnsBoundedStatusEnvelope` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_tools_test.go:86`<br>`TestWorkflowRunWaitHonorsDeadlineAndCancellation` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_tools_test.go:132`<br>`TestWorkflowManagementToolUpdatesUsePatchSemantics` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_tools_test.go:183`<br>`TestWorkflowManagementToolListsCreatesAndDeletes` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_tools_test.go:240`<br>`TestWorkflowRunToolsRequireInteractiveSession` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/assembly/workflow_tools_test.go:285`<br>`TestUnavailableWorkflowToolManagerFailsClosed` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk22regression/native_runtime_test.go:24`<br>`TestConfirmedToolsResumeInRequestOrder` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk22regression/native_runtime_test.go:86`<br>`TestWorkflowGraphResumesByInterruptIDAndPreservesEventOrder` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk22regression/native_runtime_test.go:155`<br>`TestWorkflowPropagatesExternalCancellationWithoutSuccessorOrRetry` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:25`<br>`TestGoogleADKMemoryServiceBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:61`<br>`TestModelCatalogToolBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:105`<br>`TestContextCompactionNoticeBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:153`<br>`TestPauseGuardBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:203`<br>`TestSchemaConversionReportsMarshalError` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:209`<br>`TestSQLiteGormPoolBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:237`<br>`TestStoreMaintenanceBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:294`<br>`TestRuntimeFacadeBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:351`<br>`TestRuntimeConstructionCompactionAndCloseBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:416`<br>`TestRuntimeSessionContextBoundaryBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:532`<br>`TestRuntimeAgentProviderSessionAndMemoryBoundaryBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_edges_test.go:684`<br>`TestRuntimeSnapshotAndProviderTestBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_runner_edges_test.go:14`<br>`TestWorkflowApprovalParentChildBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_runner_edges_test.go:142`<br>`TestRunnerChatAndStoreAdditionalBoundaryBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_runner_edges_test.go:234`<br>`TestResumeGoogleADKFakeExecutionBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_runner_edges_test.go:352`<br>`TestRunnerApprovalStateMachineAdditionalBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_schema_test.go:8`<br>`TestGoogleADKJSONSchemaFromMapPreservesObjectFields` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_schema_test.go:42`<br>`TestGoogleADKJSONSchemaFromMapAllowsNil` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_skill_edges_test.go:13`<br>`TestSkillRegistryArchiveAndFilesystemBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_store_edges_test.go:11`<br>`TestStoreSessionContextAndNoticeBoundaryBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_store_edges_test.go:82`<br>`TestNewStoreAndDeleteSessionBoundaryBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_store_edges_test.go:148`<br>`TestStoreProviderSecretAndDefaultErrorBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_store_edges_test.go:193`<br>`TestStoreRunApprovalAndMemoryErrorBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_store_edges_test.go:272`<br>`TestStoreBuiltinAndLowLevelJSONErrorBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_store_edges_test.go:303`<br>`TestWorkflowStoreFullCRUDAndLogBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_store_edges_test.go:404`<br>`TestStoreSessionComposerBoundaryBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_store_edges_test.go:437`<br>`TestStoreNormalizationEdgeBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_tool_edges_test.go:21`<br>`TestSmallADKBoundaryTailBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_tool_edges_test.go:77`<br>`TestProviderHTTPBoundaryTailBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_tool_edges_test.go:131`<br>`TestProjectionAndReasoningHelperBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_tool_edges_test.go:165`<br>`TestNormalizeAndApprovalResolutionBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_tool_edges_test.go:177`<br>`TestWorkflowTaskLocalHelperBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_tool_edges_test.go:235`<br>`TestWorkflowPlannerAdditionalBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/adk_tool_edges_test.go:340`<br>`TestTimelineAdditionalBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/approval_persistence_failures_test.go:9`<br>`TestApprovalPersistenceFailuresRemainObservable` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/approval_persistence_failures_test.go:75`<br>`TestWorkflowApprovalReconcilerRestagesPersistedResolution` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/approval_reconciliation_lifecycle_test.go:9`<br>`TestWorkflowApprovalReconcilerProtectsParentAndChildLifecycle` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/approval_reconciliation_lifecycle_test.go:65`<br>`TestApprovalContinuationEligibilityDistinguishesRecoverableState` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/approval_retry_sibling_cancellation_test.go:14`<br>`TestSynchronousApprovalDenialCancelsSiblingActions` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/approval_retry_sibling_cancellation_test.go:68`<br>`TestApprovalBusyRetryStopsWhenRequestIsCancelled` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/approval_stage_boundaries_test.go:8`<br>`TestResolveAndStageApprovalBoundaryStates` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/approval_stage_boundaries_test.go:75`<br>`TestResolveAndStageApprovalRejectsCorruptDurablePayloads` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/approval_state_guard_test.go:9`<br>`TestSaveRunProtectsClaimedApprovalContinuationFromStaleSnapshot` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/approval_state_guard_test.go:114`<br>`TestApprovalContinuationGetsFreshTimeoutWindow` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/canvas_provider_model_overrides_test.go:8`<br>`TestCanvasWorkflowAppliesExplicitProviderAndModelOverrides` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/chat_request_idempotency_test.go:17`<br>`TestChatRequestIdentityValidationAndFingerprintConflict` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/chat_request_idempotency_test.go:48`<br>`TestConcurrentResponsesRequestReusesOneRunAndNativeAssistantEvent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/completion_review_test.go:17`<br>`TestCompletionReviewCompleteLeavesReplyUnchanged` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/completion_review_test.go:35`<br>`TestCompletionReviewHighConfidenceAppendUsesSameRunAndSyntheticFinalMessage` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/completion_review_test.go:79`<br>`TestDefaultChatCompletionReviewAppendsAfterToolReplyInSSEOrder` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/completion_review_test.go:121`<br>`TestCompletionReviewFailsOpen` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/completion_review_test.go:153`<br>`TestCompletionReviewAnonymousPortfolioScenarioFinishesInOneRun` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/completion_review_test.go:185`<br>`TestCompletionReviewResponsesRequestIsBoundedStructuredAndToolFree` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/completion_review_test.go:227`<br>`TestCompletionReviewEligibilityAndMemoBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/completionreview/policy_test.go:10`<br>`TestParseRejectsInconsistentCompletionReviewResponses` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/completionreview/policy_test.go:30`<br>`TestPrepareIncludesLatestAnsweredInputWithoutToolOutputs` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/completionreview/policy_test.go:68`<br>`TestIneligibleReasonClassifiesControlAndToolStates` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/completionreview/policy_test.go:100`<br>`TestCoordinatorRejectsDuplicateAndMissingApplications` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/context_cache_test.go:61`<br>`TestProviderPayloadKeepsStablePrefixAcrossTurns` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/context_cache_test.go:116`<br>`TestProviderPayloadUsesOnlyCurrentContextRevisionHandoff` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/context_cache_test.go:187`<br>`TestProviderPayloadSortsToolsByNameIndependentOfAgentInputOrder` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/error_identity_test.go:20`<br>`TestSerializedADKErrorRestoresKnownSentinelIdentity` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/error_identity_test.go:49`<br>`TestGoogleADKRunnerErrorClassificationPreservesCause` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/error_identity_test.go:68`<br>`TestADKProductionCodeUsesSentinelIdentityChecks` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/event_projection_boundaries_test.go:14`<br>`TestEventProjectionHandlesEmptyStorageOrderingAndFallbackEntries` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/event_projection_boundaries_test.go:72`<br>`TestSessionProjectionPropagatesADKSessionReadFailures` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/event_projection_boundaries_test.go:87`<br>`TestSessionProjectionTreatsMissingADKSessionAsEmpty` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/event_projection_reply_ordering_test.go:15`<br>`TestProjectedAssistantEntryAnchorsAtLatestTextEvent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/exec_bounds_test.go:14`<br>`TestGoogleADKExecutionDescriptorRunMappingAndContentBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/exec_bounds_test.go:57`<br>`TestGoogleADKExecutionToolCallReuseAndCompletionBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/exec_bounds_test.go:93`<br>`TestGoogleADKExecutionRunAndEventErrorBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/exec_bounds_test.go:148`<br>`TestExecuteRegisteredToolCancellationJoinsHandler` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/exec_bounds_test.go:175`<br>`TestGoogleADKExecutionPauseAndBufferedErrorBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/exec_bounds_test.go:238`<br>`TestGoogleADKFinalReplySynthesisBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/exec_bounds_test.go:275`<br>`TestGoogleADKExecutionApprovalResolutionAndPendingBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/exec_bounds_test.go:364`<br>`TestGoogleADKExecutionRehydrateBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/exec_state_bounds_test.go:9`<br>`TestGoogleADKExecutionRunScopedStateProjectionBoundaries` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/exec_state_bounds_test.go:101`<br>`TestGoogleADKExecutionBufferedTextAndDeltaBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/exec_state_bounds_test.go:171`<br>`TestGoogleADKExecutionRunSnapshotPersistenceBoundary` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_claim_failure_boundaries_test.go:39`<br>`TestExecutionClaimValidationAndReleaseLifecycle` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_claim_failure_boundaries_test.go:83`<br>`TestToolInvocationAbandonAndCorruptionBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_claim_failure_boundaries_test.go:173`<br>`TestExecutionClaimUpdateResultFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_claim_failure_boundaries_test.go:203`<br>`TestExecutionClaimsSurfaceClosedDatabaseFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_claims_test.go:54`<br>`TestGoogleADKToolUsesDurableInvocationKeyAndReplay` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_claims_test.go:108`<br>`TestFailedReadIsDurablyCompletedButProjectedAsFailedToolCall` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_claims_test.go:167`<br>`TestGoogleADKToolRejectsStaleContextAfterLeaseTurnover` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_claims_test.go:214`<br>`TestGoogleADKKeyedToolFailsClosedWhenHandlerIgnoresKey` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_claims_test.go:265`<br>`TestRuntimeReconciliationDoesNotStealFreshForeignLease` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_claims_test.go:294`<br>`TestRuntimeRunLeaseHeartbeatPreventsPrematureTakeover` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_claims_test.go:343`<br>`TestRunSaveIsFencedWithExecutionLeaseContext` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_state_projection_contracts_test.go:44`<br>`TestGoogleADKExecutionProjectsToolResponseLifecycle` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_state_projection_contracts_test.go:103`<br>`TestGoogleADKExecutionStateModelsVisibleAndPersistedRunStatus` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_state_projection_contracts_test.go:175`<br>`TestGoogleADKRunnerHelpersPreserveRecoveryRules` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/execution_state_projection_contracts_test.go:234`<br>`TestDirectApprovalCompletionPersistsAssistantTimelineAndTerminalState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/goal_state_boundaries_test.go:9`<br>`TestWorkflowApprovalStateTransitionsPersistTheObservableOutcome` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/google_exec_concurrency_test.go:12`<br>`TestGoogleADKExecutionSerializesConcurrentToolCallbacks` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/google_execution_replay_guards_test.go:13`<br>`TestGoogleExecutionEventReplayKeepsApprovalStateIdempotent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/google_execution_replay_guards_test.go:67`<br>`TestGoogleExecutionEventGuardsPreserveProjectionState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/google_memory_test.go:10`<br>`TestGoogleADKMemoryServiceSearchesJFTradeMemory` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/google_memory_test.go:77`<br>`TestGoogleADKAgentIDFromAppName` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/google_runner_failure_diagnostics_test.go:8`<br>`TestGoogleRunnerPreservesResumedFailureDiagnostics` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/google_runner_failure_diagnostics_test.go:35`<br>`TestGoogleRunnerSurfacesModelAndChildConstructionFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/handoff_notice_test.go:10`<br>`TestHandoffSegmentsReplaceActiveChainAndFilterByRevision` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/handoff_notice_test.go:100`<br>`TestSessionNoticesPersistNormalizedEntriesAndHandleMissingState` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_continuation_failure_recovery_test.go:13`<br>`TestAnsweredInputFailuresRemainObservable` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_continuation_failure_recovery_test.go:110`<br>`TestInputResolutionExposesParentProjectionWriteFailure` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_continuation_failure_recovery_test.go:140`<br>`TestAnsweredInputCrashRecoveryHonorsDurableRunLease` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_continuation_idempotency_test.go:14`<br>`TestResolveInputAsyncDoesNotRestartAnInFlightContinuation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_continuation_idempotency_test.go:87`<br>`TestAnsweredInputIsRequeuedAfterInFlightContinuationReleasesClaim` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:16`<br>`TestBuildInputRequestAndValidateAnswers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:72`<br>`TestInputRequestToolRunReturnsCorrectableFeedbackForInvalidArgs` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:114`<br>`TestInputRequestValidationAndErrorEdges` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:225`<br>`TestResolveRunInputStoreErrors` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:305`<br>`TestPendingInputRequestConflictEdges` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:413`<br>`TestInputContinuationFailureIsPersisted` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:449`<br>`TestResolveRunInputIsValidatedAndIdempotent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:490`<br>`TestCancelPendingInputRunCancelsRequestAndRejectsLateAnswer` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:530`<br>`TestInputRequestTimelinePersistsAnsweredCard` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:556`<br>`TestRequestUserToolIsLongRunning` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:584`<br>`TestRequestUserToolPausesAndResumesChatRun` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:647`<br>`TestRequestUserToolSupportsSequentialQuestionsInOneRun` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:694`<br>`TestRequestUserToolCanTransitionToApproval` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:738`<br>`TestRequestUserToolResumesAfterRuntimeRestart` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:773`<br>`TestInputResponsePayloadAnchorsResumedRunToOriginalRequest` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_request_test.go:802`<br>`TestResumedInputRunInjectsOriginalRequestAnchor` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/input_workflow_test.go:9`<br>`TestInputRequestProjectsThroughWorkflowBlockingState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/lifecycle_reconciliation_failures_test.go:9`<br>`TestStaleRunReconciliationCoversTerminalPlanAndSelfReferenceRecovery` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/lifecycle_reconciliation_failures_test.go:102`<br>`TestLifecycleStoreFailuresAreReturnedToTheCaller` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:17`<br>`TestLocalMCPHandlerExposesOnlyReviewedReadTools` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:119`<br>`TestLocalMCPHandlerRejectsWriteCapableReplacementOfReviewedName` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:129`<br>`TestLocalMCPHandlerRequiresAtLeastOneReviewedTool` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:136`<br>`TestLocalMCPHandlerCloseUnsubscribesRegistryListener` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:162`<br>`TestLocalMCPHandlerReturnsToolFailuresAsMCPToolErrors` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:194`<br>`TestLocalMCPHandlerServesStatelessPostOnlyRequests` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:266`<br>`TestLocalMCPHandlerPreservesMCPHostProtection` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:293`<br>`TestLocalMCPHandlerReadsSanitizedRuntimeStatusResource` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:333`<br>`TestSanitizedMCPRuntimeStatusIncludesConfiguredDataAndErrors` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:358`<br>`TestSanitizedMCPRuntimeStatusSerializesDescriptors` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:367`<br>`TestLocalMCPRuntimeStatusSubscriptionValidation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:382`<br>`TestLocalMCPHandlerSynchronizesReviewedToolsAndRuntimeSubscriptions` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/mcp_server_test.go:440`<br>`TestLocalMCPHandlerRefreshesReplacedToolHandler` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/normalize_test.go:5`<br>`TestNormalizeRunAndResponsesReplaceNilSlices` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/normalize_test.go:62`<br>`TestNormalizeWorkflowAndSessionResponsesPreserveBusinessContracts` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/observability_test.go:10`<br>`TestADKRunContextCarriesCanonicalCorrelationFields` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/approval_query_plan_test.go:9`<br>`TestApprovalByConfirmationCallIDQueryUsesPartialIndex` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/composer_normalize_test.go:8`<br>`TestNormalizeSessionComposerStateClearsInvalidModes` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/execution_claims_test.go:58`<br>`TestRunLeaseUsesExpiryAndFencingTokens` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/execution_claims_test.go:93`<br>`TestRunAndToolClaimsSerializeAcrossStoreConnections` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/execution_claims_test.go:186`<br>`TestToolInvocationClaimReplaysCompletedOutput` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/execution_claims_test.go:229`<br>`TestToolInvocationCrashPolicyFailsClosedOrFencedTakeover` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/google_artifact_test.go:19`<br>`TestGoogleADKArtifactServiceStoresVersionedArtifacts` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/google_artifact_test.go:120`<br>`TestGoogleADKArtifactServiceAllocatesAutoVersionsAtomically` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/google_artifact_test.go:186`<br>`TestGoogleADKArtifactServicePersistsAcrossRestartAndUserScope` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/google_artifact_test.go:247`<br>`TestGoogleADKArtifactServiceBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/google_artifact_test.go:420`<br>`TestGoogleADKArtifactPathDerivesFromSQLiteSessionService` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/provider_reasoning_test.go:10`<br>`TestProviderReasoningPersistenceAllowsMappingChanges` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/provider_reasoning_test.go:47`<br>`TestProviderReasoningPersistenceDefaultsToEmptyMappings` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/provider_selection_test.go:8`<br>`TestNormalizeDefaultProviderSelection` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/provider_selection_test.go:29`<br>`TestSortProvidersDefaultFirst` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/secret_store_test.go:9`<br>`TestSecretStoreFileBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/session_sqlite_boundaries_test.go:14`<br>`TestSQLiteSessionDirectBoundaryBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/session_sqlite_schema_test.go:15`<br>`TestSQLiteSessionServiceRejectsUnavailableAndPreservesIncompatibleDatabases` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/session_sqlite_schema_test.go:49`<br>`TestSQLiteSessionSchemaHealthReportsMissingAndClosedConnections` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/session_sqlite_schema_test.go:82`<br>`TestSQLiteSessionServiceRejectsV1SchemaWithoutMutatingEvents` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/session_sqlite_schema_test.go:127`<br>`TestSQLiteSessionServiceCloseNilBoundaries` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/session_sqlite_test.go:30`<br>`TestValidateSQLiteSessionServiceAcceptsCurrentSchema` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/session_sqlite_test.go:58`<br>`TestSQLiteSessionServiceReopenPreservesADKEvents` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/session_sqlite_test.go:108`<br>`TestSQLiteSessionServiceBoundaries` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/session_sqlite_test.go:138`<br>`TestSQLiteSessionServiceClosedAndBrokenMetadataBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/store_run_test.go:11`<br>`TestSavePreparedRunWithExecutorRejectsUnsupportedPayload` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/store_run_test.go:31`<br>`TestRunReasoningSnapshotIsPrivateAndRestored` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence/task_patch_test.go:8`<br>`TestApplyTaskPatchAcceptsNilTask` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence_failure_boundaries_test.go:12`<br>`TestCanvasWorkflowPersistenceFailuresDoNotReportSuccess` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence_failure_boundaries_test.go:111`<br>`TestNativeTaskGraphPersistsCompletedAndPendingInputOutcomes` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence_failure_boundaries_test.go:162`<br>`TestStaleRunRecoveryAggregatesPersistenceFailures` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence_failure_boundaries_test.go:216`<br>`TestApprovalContinuationPersistenceFailuresRemainRetryable` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence_failure_boundaries_test.go:266`<br>`TestWorkflowChildContinuationPersistsRecoveryDecisions` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence_failure_boundaries_test.go:311`<br>`TestStaleRunRecoveryHandlesReadRepairAndParentTermination` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence_failure_boundaries_test.go:381`<br>`TestCanvasWorkflowSetupFailuresRemainPreRunErrors` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence_propagation_closeout_test.go:13`<br>`TestWorkflowTaskToolContractAndLifecycleReadFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence_propagation_closeout_test.go:34`<br>`TestWorkflowResumePersistenceFailuresRemainObservable` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/persistence_propagation_closeout_test.go:92`<br>`TestWorkflowBlockerAndRuntimeInitializationFailureSemantics` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/planner_identity_test.go:5`<br>`TestWorkflowPlannerToolsetNameIsStableForADKRegistration` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/planner_toolset_test.go:21`<br>`TestWorkflowPlannerToolsetDraftLifecycleAndRequestInjection` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/planner_toolset_test.go:167`<br>`TestWorkflowPlannerArgumentAndDependencyHelpers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/projection_canvas_memory_contracts_test.go:18`<br>`TestSessionProjectionRetainsUserReplyReasoningAndToolLifecycle` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/projection_canvas_memory_contracts_test.go:106`<br>`TestProjectionApprovalMemoryAndCanvasBoundarySemantics` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/projection_canvas_memory_contracts_test.go:225`<br>`TestDirectApprovalResumeErrorRequiresTerminalToolStates` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/provider_base_url_test.go:5`<br>`TestValidateProviderBaseURLRejectsMetadataTargets` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/provider_headers_test.go:9`<br>`TestSaveProviderValidatesBaseURLAndDefaultHeaders` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/http_test.go:15`<br>`TestHTTPClientAllowsPrivateNetworkProvider` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/http_test.go:38`<br>`TestHTTPClientRevalidatesRedirectDNSResolution` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/probe_test.go:13`<br>`TestProbeProviderQuickAndFullRequestCounts` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/probe_test.go:81`<br>`TestProbeProviderWithoutMappingsSendsNoReasoningField` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/probe_test.go:105`<br>`TestProviderProbeTimeoutCapsConfiguredRequestTimeout` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/reasoning_effort_transport_test.go:14`<br>`TestResponsesReasoningEffortRequestField` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/reasoning_effort_transport_test.go:64`<br>`TestResponsesCustomReasoningMappingInjectsNestedField` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/responses_model_test.go:18`<br>`TestResponsesModelSendsSanitizedToolsAndRestoresCalls` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/responses_model_test.go:48`<br>`TestResponsesModelRetainsStreamingUsageMetadata` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/responses_model_test.go:87`<br>`TestResponsesToolNamesRejectSanitizationCollision` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/responses_model_test.go:96`<br>`TestResponsesToolNameModelRestoresStreamFunctionCalls` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/responses_model_test.go:112`<br>`TestResponsesModelBoundaryErrorsAndNameFallbacks` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/responses_model_test.go:144`<br>`TestProbeResponsesProviderReportsMalformedResponse` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/safe_http_test.go:14`<br>`TestSafeHTTPClientValidatesResolvedAddressAtDialTime` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/providers/safe_http_test.go:41`<br>`TestSafeHTTPClientDialBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/reasoning_effort_lifecycle_test.go:10`<br>`TestReasoningEffortOverridePriority` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/reasoning_effort_lifecycle_test.go:23`<br>`TestReasoningEffortResumeUsesRunSnapshot` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/responses_model_runtime_test.go:9`<br>`TestProviderAlwaysSelectsResponsesModel` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/responses_stream_projection_test.go:12`<br>`TestResponsesExecutionSkipsDuplicateFinalTextAfterPartial` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/resumed_execution_recovery_boundaries_test.go:14`<br>`TestResumedExecutionFailurePersistenceIsObservable` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/resumed_execution_recovery_boundaries_test.go:92`<br>`TestDirectResumeSurfacesRehydrationFailure` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/resumed_execution_recovery_boundaries_test.go:111`<br>`TestChildApprovalResumeFallsBackToDirectRecovery` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/run_timeline_test.go:8`<br>`TestSessionTimelineHandlesEmptyAndMissingSessions` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_approval_concurrency_test.go:12`<br>`TestConcurrentResolveApprovalExecutesApprovedToolOnce` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_approval_concurrency_test.go:92`<br>`TestConcurrentSiblingApprovalsAreMergedBeforeContinuation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_approval_concurrency_test.go:119`<br>`TestConcurrentSiblingAsyncApprovalsEnqueueOneContinuation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_approval_concurrency_test.go:154`<br>`TestAsyncApprovalWaitsForLocalInputContinuationLease` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_approval_concurrency_test.go:191`<br>`TestApprovalLeaseWaitStopsWhenRuntimeContextIsCancelled` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_callbacks_test.go:49`<br>`TestRunnerChatCallbacksAndEventProjection` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_continuation_signal_test.go:9`<br>`TestContinuationOnlyMessageRecognition` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_continuation_signal_test.go:22`<br>`TestChatAuditsContinuationOnlyMessageAgainstRecentCompletedRun` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_continuation_signal_test.go:52`<br>`TestRecentContinuationSignalRequiresFreshCompletedRunInSameSession` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_runtime_branches_test.go:55`<br>`TestGoogleADKExecuteRuntimeBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_runtime_branches_test.go:190`<br>`TestGoogleADKResumeRuntimeBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_runtime_branches_test.go:337`<br>`TestGoogleADKRunnerConstructionAndSynthesisBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_runtime_branches_test.go:487`<br>`TestRunChatRuntimeBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:18`<br>`TestPrepareChatRequestValidationAndConcurrency` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:56`<br>`TestRequestedInputEventFailsWithUnsupportedInputCode` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:69`<br>`TestHydrateRunExecutionResultPopulatesRunFields` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:102`<br>`TestCompleteChatRunDoesNotPromoteTopLevelFollowUpToPendingInput` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:127`<br>`TestChatToolOnlyADKRunSynthesizesFinalReply` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:156`<br>`TestMarkFailedChatRunMapsContextToTerminalState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:181`<br>`TestPersistRunTerminalStateWritesRunAndAudit` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:225`<br>`TestAttachFinalAssistantMessagePersistsMessageAndRunLink` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:267`<br>`TestFinishPendingApprovalRunPersistsPendingStateAndAssistantPrompt` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:325`<br>`TestCompleteChatRunFailurePersistsUserFacingErrorReply` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:376`<br>`TestCompleteChatRunSuccessPersistsCompletedRunAndAssistantReply` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:423`<br>`TestCompleteChatRunKeepsFailedToolCallsVisibleWithoutFailingRun` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:490`<br>`TestProjectedChatResponseAppliesProjectionToRunFields` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:541`<br>`TestRunChatRejectsInvalidPermissionModeOverride` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:552`<br>`TestRunStoresResolvedModelSnapshot` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:609`<br>`TestChatRequestProviderOverrideRunsWithoutEditingAgent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:670`<br>`TestAgentWithoutProviderDynamicallyUsesDefaultProvider` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:737`<br>`TestRunnerChatProjectionPersistenceAndAssistantBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:846`<br>`TestProjectedChatResponseDoesNotExposeResolvedApprovals` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:891`<br>`TestResolveApprovalAsyncDetachesClosedStreamBeforeBackgroundResume` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:978`<br>`TestResolveAgentCoversDefaultAndProviderValidation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:1043`<br>`TestResolveSessionReusesExistingRejectsMismatchAndCreatesTrimmedSession` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:1079`<br>`TestStartRunPersistsRunAndFinishRemovesActiveHandle` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_chat_test.go:1124`<br>`TestCancelRunOnTerminalStateIsNoop` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:11`<br>`TestContinuationClaimGuardsAndClosingRuntime` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:63`<br>`TestContinuationQueuesInitializeWithoutRuntimeBackgroundContext` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:87`<br>`TestResolvedContinuationsHonorForeignLeasesAndEmptyState` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:136`<br>`TestResolvedApprovalContinuationKeepsSiblingStateAtomic` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:188`<br>`TestStartRunPersistsExecutionLeaseClaimFailure` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:207`<br>`TestResolvedApprovalDoesNotStealForeignExecutionLease` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:242`<br>`TestLifecycleReportsLeaseStorageFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:275`<br>`TestGoalResumeFailsClosedWhenExecutionLeaseCannotBeClaimed` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:314`<br>`TestGoBackgroundNilGuardsAndClosingState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:347`<br>`TestRuntimeCloseWaitsForInFlightBackgroundWorkAndRejectsNewWork` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:406`<br>`TestGoBackgroundUsesBackgroundCtxOrFallback` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:427`<br>`TestGoalResumeExecutionErrorPaths` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_goal_test.go:11`<br>`TestPauseGoalRunEnforcesRootActiveGoalBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_goal_test.go:72`<br>`TestResumeGoalRunRejectsNonResumableStates` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_goal_test.go:111`<br>`TestReconcileExpiredRunsCancelsTimedOutRuns` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_goal_test.go:174`<br>`TestCancelRunTreeAndRunLifecycleHelpers` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_goal_test.go:264`<br>`TestUpdateRunObjectiveAndRecentMessageBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_lifecycle_boundaries_test.go:24`<br>`TestRunnerLifecycleBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_lifecycle_reconciliation_test.go:9`<br>`TestReconcileStaleRunsPreservesRecoverableWorkflowStateAndFailsOrphans` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_lifecycle_reconciliation_test.go:70`<br>`TestReconcileTerminalWorkflowCancelsChildrenAndClearsStaleApprovals` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_lifecycle_reconciliation_test.go:106`<br>`TestRepairWorkflowSelfReferenceResetsTheTaskAndPausesTheParent` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_lifecycle_reconciliation_test.go:146`<br>`TestWorkflowParentReferenceAndReconcileHelpersBoundaries` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_lifecycle_shutdown_failures_test.go:8`<br>`TestLifecycleFailsClosedWhenStorageStopsDuringReconciliation` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_plugin_test.go:11`<br>`TestGoogleADKExecutionPluginRegistersV2ProjectionCallbacks` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runner_plugin_test.go:38`<br>`TestGoogleADKExecutionPluginRejectsNilExecution` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:14`<br>`TestRunExecutionLeaseContextAndReuseBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:108`<br>`TestRunExecutionLeaseHeartbeatFailureCancelsOwner` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:133`<br>`TestRefreshRunExecutionLeaseRejectsExpiredLeaseBeforeStoreWrite` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:144`<br>`TestRefreshRunExecutionLeaseUsesRemainingTTLForNearExpiryLease` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:161`<br>`TestRunExecutionLeaseUsesSafeDefaults` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:176`<br>`TestRuntimeCloseCancelsAndWaitsForInFlightRunLease` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:234`<br>`TestRuntimeCloseRejectsRunLeaseWorkAfterClosingStarts` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_store_test.go:36`<br>`TestStoreDefaultAgentEnsureAgentAndSessionOrdering` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_store_test.go:117`<br>`TestStoreStartupRefreshesBuiltinPolicyAndPreservesModelSelection` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_store_test.go:174`<br>`TestStoreBuiltinAgentsAreProtected` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_store_test.go:193`<br>`TestModelsListToolReturnsCallableModelsWithoutKeys` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_store_test.go:286`<br>`TestRuntimeSnapshotProviderProbeAndDeleteSession` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_store_test.go:362`<br>`TestRuntimeTestProviderMarksToolsUnsupportedWhenSelectionFails` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_store_test.go:409`<br>`TestRuntimeDeleteSessionIgnoresMissingRemoteSessionAndUnavailableRuntime` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_store_test.go:434`<br>`TestNewRuntimeInitializesRegistriesAndClosesCleanly` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_store_test.go:454`<br>`TestNewRuntimeDirectCtorAndNilSafeHelpers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/runtime_store_test.go:485`<br>`TestApprovalResolutionSummaryAndUserFacingErrors` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_compaction_boundaries_test.go:12`<br>`TestCompactingSessionServiceBoundaryBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_conflict_test.go:11`<br>`TestSessionContextAndCompactionRejectMissingResourcesAndConflicts` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_json_test.go:9`<br>`TestSessionContextSnapshotJSONOmitsZeroRawBreakdown` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_projection_test.go:15`<br>`TestSessionContextProjectionBucketsAndTrimsProtectedToolOutput` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_projection_test.go:60`<br>`TestSessionContextSummaryAndBoundaryHelpersUseBusinessSemantics` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_projection_test.go:118`<br>`TestSessionContextManagerAndWrappedEventsBoundaryHelpers` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_projection_test.go:181`<br>`TestSessionContextApprovalResolutionAndEventIndexBoundaries` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_projection_test.go:208`<br>`TestSessionContextAdditionalPureHelperBoundaries` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_recovery_edges_test.go:9`<br>`TestSessionContextManagerRecoveryEdgeBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_retry_boundaries_test.go:89`<br>`TestSessionContextAppendRetryBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_retry_boundaries_test.go:143`<br>`TestSessionContextManagerBoundaryBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_stale_test.go:106`<br>`TestAppendADKEventWithStaleRetrySerializesConcurrentStaleSession` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_stale_test.go:160`<br>`TestAppendADKEventWithStaleRetryReturnsNonStaleError` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_stale_test.go:188`<br>`TestAppendADKEventWithStaleRetryRefreshesUnexpectedSessionType` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_stale_test.go:209`<br>`TestAppendADKEventWithStaleRetryRefreshesSyntheticSessionBeforeAppend` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_stale_test.go:231`<br>`TestSyncHandoffStateSkipsMissingRawADKSession` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_stale_test.go:315`<br>`TestSessionContextProjectionTrimsOversizedToolResponses` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_stale_test.go:420`<br>`TestSessionContextProjectionKeepsSmallToolResponsesUntouched` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:15`<br>`TestSessionContextCompactionShrinksSessionView` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:120`<br>`TestSessionContextUsesSessionProviderOverrideWindow` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:183`<br>`TestSessionContextCompactionCreatesCurrentRevision` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:287`<br>`TestCompactSessionContextWritesContextNotice` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:341`<br>`TestMaybeAutoCompactSessionEmitsContextNoticeDeltas` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:446`<br>`TestMaybeAutoCompactSessionSkipsWhenSessionCompactionAlreadyRunning` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:506`<br>`TestSessionServiceAutoCompactionUsesSessionGate` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:569`<br>`TestMaybeAutoCompactSessionDuringWorkflowAllowsActiveParent` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:651`<br>`TestSessionContextViewDoesNotAutoCompact` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:703`<br>`TestModelContextReadAutoCompactsBeforeProviderPayload` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:773`<br>`TestProtectedTailStartsAtEarliestUnresolvedApprovalEvent` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:787`<br>`TestProtectedTailIncludesOriginalFunctionCallForPendingApproval` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:800`<br>`TestProtectedTailIgnoresResolvedApprovalEvent` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:813`<br>`TestProtectedTailKeepsOnlyUnresolvedApprovalWhenOlderApprovalResolved` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:827`<br>`TestSessionContextIgnoresHandoffSegmentsWithoutRevision` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:868`<br>`TestAppendADKEventWithStaleRetryRefreshesSession` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:922`<br>`TestCompactedSessionViewTracksEventsAppendedDuringInvocation` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:977`<br>`TestHasActiveRunDoesNotTreatPendingApprovalAsExecuting` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_context_test.go:1009`<br>`TestCompactedSessionPreservesOriginalCallForPendingApproval` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_skill_test.go:15`<br>`TestWrappedAndEmptySessionAccessorsPreserveProjectedState` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_skill_test.go:68`<br>`TestStoreListSkillsSortsBuiltinFirstAndDeleteProtectsBuiltins` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/session_wrap_test.go:9`<br>`TestCompactingSessionServiceListDelegatesToBaseService` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_recover_test.go:11`<br>`TestSkillRegistrySourceAndFrontmatterFailureBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_recover_test.go:39`<br>`TestSkillRegistryBuiltinSyncBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_recover_test.go:114`<br>`TestSkillRegistryCopyAndReplaceDirectoryBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_fs_test.go:19`<br>`TestSkillRegistryListSortsBySourceAndDefaultsFilesystemMetadata` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_fs_test.go:48`<br>`TestSkillRegistryArchiveInstallsBundlesWithDirectoryEntries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_fs_test.go:72`<br>`TestSkillRegistryWarnsWhenExternalSkillReferencesUnknownTools` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_fs_test.go:89`<br>`TestSkillRegistryFilesystemFailureBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_fs_test.go:139`<br>`TestSkillRegistryAdditionalBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_fs_test.go:209`<br>`TestSkillRegistryMalformedBuiltinSyncFailsWithoutReplacingExternalState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_test.go:21`<br>`TestSkillRegistryFilteredSourceExposesOnlyAllowedResources` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_test.go:84`<br>`TestBuiltinSkillMetadataRejectsInvalidBundles` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_test.go:115`<br>`TestSkillRegistryArchiveRejectsUnsafeOrAmbiguousBundles` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_test.go:162`<br>`TestSkillRegistryInstallURLAndDirectoryBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_test.go:235`<br>`TestSkillRegistryInstallURLPlainDocumentAndRedirectSafety` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_test.go:286`<br>`TestSkillRegistryInstallURLSupportsArchivesAndUninstallProtections` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_reg_test.go:361`<br>`TestSkillRegistryFileHelpersDetectArchiveAndBundleBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_registry_archives_test.go:45`<br>`TestSkillRegistryFilesystemAndArchiveBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_registry_http_sources_test.go:19`<br>`TestSkillRegistryHTTPAndSourceBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skill_registry_http_sources_test.go:198`<br>`TestSkillInstallDeterministicErrorBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skillsruntime/install_boundary_test.go:14`<br>`TestSkillInstallAdditionalBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skillsruntime/schema_market_index_constituents_test.go:8`<br>`TestMarketIndexConstituentsSchemaStaysStrict` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skillsruntime/schema_market_index_constituents_test.go:27`<br>`TestMarketSkillDocumentsIndexConstituentsTool` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skillsruntime/schema_market_news_test.go:8`<br>`TestMarketNewsAndCorporateActionsSchemasStayStrict` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skillsruntime/schema_market_news_test.go:49`<br>`TestMarketSkillDocumentsNewsAndCorporateActionsTools` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skillsruntime/schema_test.go:9`<br>`TestWorkflowSchemasStayStrict` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skillsruntime/schema_test.go:29`<br>`TestToolMetadataNormalization` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/skillsruntime/schema_test.go:48`<br>`TestBacktestAndStrategyLifecycleSchemasCloseNestedObjects` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/sqlite_dialector_boundaries_test.go:14`<br>`TestSQLiteDialectorBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/sqlite_tools_test.go:35`<br>`TestSQLiteDialectorMigratesAndPersistsWithGORM` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/sqlite_tools_test.go:79`<br>`TestSQLiteDialectorDataTypesDefaultsAndQuoting` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/sqlite_tools_test.go:145`<br>`TestSQLiteDialectorClauseBuildersAndVersionCompare` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/sqlite_tools_test.go:194`<br>`TestDefaultToolSchemasCoverBusinessCriticalToolPayloads` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/sqlite_tools_test.go:333`<br>`TestToolRegistryAliasesModesAndNumericInputs` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_approve_test.go:10`<br>`TestResolveApprovalAsyncIsIdempotentForMissingAndResolvedApprovals` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_approve_test.go:41`<br>`TestSaveRunAndDenyPendingApprovalsDeniesOnlyPendingRecords` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_approve_test.go:98`<br>`TestStoreDeleteSessionContextRemovesLiveState` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_async_test.go:10`<br>`TestResolveApprovalAsyncDenialRejectsSiblingApprovalsWithoutExecutingTools` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_async_test.go:89`<br>`TestResolveApprovalAsyncDoesNotResumeCompletedRunOnRetry` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_audit_query_test.go:7`<br>`TestStoreListAuditEventsPageFiltersCountsAndOrdersInSQL` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_business_test.go:22`<br>`TestStoreProviderLifecycleMaintainsDefaultAndSecrets` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_business_test.go:108`<br>`TestStoreAgentSessionCascadeAndTaskMemoryBoundaries` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_business_test.go:275`<br>`TestStoreRunApprovalSkillAndOptimizationBusinessQueries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_business_test.go:412`<br>`TestStoreOperationalBoundaryErrorsAndDefaults` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_business_test.go:563`<br>`TestStoreDefaultSelectionSecretsAndListBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_entity_lifecycle_edges_test.go:10`<br>`TestStoreEntityAndCoreLifecycleEdges` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_failure_normalization_boundaries_test.go:10`<br>`TestStoreDataFailureAndNormalizationBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_failure_normalization_boundaries_test.go:170`<br>`TestStoreEntityDefaultsAndStorageFailures` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_identity_test.go:10`<br>`TestStoreGeneratedIdentityAndDefaultBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_identity_test.go:47`<br>`TestStoreTaskAndOptimizationUpdatesPreserveCreationTime` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_identity_test.go:85`<br>`TestStoreDeleteSessionCascadesRuntimeState` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:17`<br>`TestDeleteProviderFailsWhenReferencedByAgent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:45`<br>`TestProvidersMaintainDefaultSelectionAndCreatedOrder` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:110`<br>`TestDeleteSessionRemovesApprovals` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:146`<br>`TestSaveRunDoesNotRegressTerminalLifecycle` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:250`<br>`TestSaveRunReopensCompletedRunForFreshPendingApproval` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:281`<br>`TestSaveRunAllowsPausedWorkflowLifecycleUpdates` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:328`<br>`TestSaveRunPreservesUserGoalPauseLifecycle` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:466`<br>`TestListSessionsPageFiltersQueryAndPaginates` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:501`<br>`TestSessionComposerStatePersistsAndDeletesWithSession` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:553`<br>`TestDeleteSessionMissingAndBlankAreNotFound` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:565`<br>`TestListApprovalsPageFiltersAndSortsNewestFirst` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:609`<br>`TestListOptimizationTasksSortsByUpdatedAtDesc` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:641`<br>`TestExecuteToolTagInvokesCanonicalToolWithParameters` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:701`<br>`TestRejectUnsafeHost` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:711`<br>`TestInternalSkillCannotBeUninstalled` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:735`<br>`TestExternalSkillUninstallRemovesInstallDir` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:765`<br>`TestPreparedAgentLoadsOnlyEnabledBoundSkillsAndTools` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_lifecycle_test.go:792`<br>`TestSkillRegistryReportsMetadataAndAllowedTools` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_maintenance_handoff_test.go:10`<br>`TestStoreMaintenanceBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_maintenance_handoff_test.go:93`<br>`TestStoreHandoffBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_maintenance_test.go:8`<br>`TestPurgeDeletedConfigsCascadesConfigurationButKeepsHistory` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_maintenance_test.go:54`<br>`TestCompactDatabaseReclaimsFreePages` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:21`<br>`TestStoreBuiltinSkillsSplitStrategySkill` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:36`<br>`TestBuiltinSkillStoreMetadataComesFromBundleRegistry` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:88`<br>`TestBuiltinStrategySkillRefreshesOutdatedBundle` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:128`<br>`TestBuiltinAgentTemplatesOnlyExposeDefaultAgent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:147`<br>`TestBuiltinRefreshDoesNotOverrideNonBuiltinSkill` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:184`<br>`TestInstallSkillArchivePreservesResources` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:224`<br>`TestInstallSkillURLInstallsNeodataFinancialSearch` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:281`<br>`TestResolveSessionRejectsDifferentAgent` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:294`<br>`TestDeleteAgentSoftDeletesHistoricalRecord` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:320`<br>`TestListAgentsExcludesSoftDeletedWhileListAllIncludesThem` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:363`<br>`TestSaveAgentRestoresDeletedAgentRecord` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:402`<br>`TestCancelPendingRunDeniesApprovals` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:426`<br>`TestCancelRunMissingReturnsNotFound` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:436`<br>`TestResolveApprovalMissingReturnsIdempotentEmptyResult` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:452`<br>`TestStoreResolvePendingApprovalMissingAndIdempotent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:486`<br>`TestListRunsPageFiltersAndSortsNewestFirst` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:527`<br>`TestDuplicateApprovalResolutionDoesNotExecuteTwice` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:565`<br>`TestPendingApprovalResumesThroughGoogleADKAfterRuntimeRestart` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:621`<br>`TestApprovalResumingRunIsRecoveredAfterRuntimeRestart` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:745`<br>`TestUnrecoverablePendingApprovalRunIsMarkedOrphanedOnRestart` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:788`<br>`TestMultipleApprovalsExecuteOnlyAfterAllApproved` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:844`<br>`TestADKTaskUpdateDeleteAndValidation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:899`<br>`TestADKMemoryFiltersDeleteAndAgentValidation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:939`<br>`TestPrepareAgentInjectsMemoryOnlyWhenEnabled` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:961`<br>`TestToolsSearchReturnsOnlyCurrentAgentTools` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_ops_test.go:998`<br>`TestWorkflowWriteToolsRequireApprovalExceptLowRiskTaskWrites` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_recover_test.go:9`<br>`TestStoreListProvidersRepairsPersistedDefaultSelection` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_recover_test.go:66`<br>`TestStoreDefaultAgentSkipsDisabledPrimaryAndRestoresTemplate` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_recover_test.go:111`<br>`TestStoreListAuditEventsReturnsCorruptPayloadError` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_recover_test.go:127`<br>`TestStorePersistenceMethodsSurfaceClosedDatabaseErrors` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:59`<br>`TestNewStoreUsesSeparatedConcurrentReadAndSingleWritePools` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:75`<br>`TestStoreMigrationNormalizesHiddenAgentWorkflowDefaults` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:127`<br>`TestStoreMigrationRepairsOrphanTasksAndDuplicateConfirmations` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:198`<br>`TestStoreMigrationReopensCompletedWorkflowWithRecoverablePendingApproval` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:254`<br>`TestNewStoreRejectsLegacyDatabaseWithoutMutatingIt` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:295`<br>`TestSaveApprovalIfConfirmationAbsentIsConcurrentIdempotent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:373`<br>`TestNewStoreDropsLegacyMessageTables` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:385`<br>`TestProviderSecretIsNotEchoed` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:437`<br>`TestProviderRequestTimeoutDefaultsAndClamp` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:471`<br>`TestApprovalModeCreatesPendingApprovalForWriteTool` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:538`<br>`TestIdempotentApprovalRecoversPendingRunWithStaleEmbeddedApproval` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:607`<br>`TestReconcileResolvedApprovalsRecoversPendingRun` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:670`<br>`TestApprovalDenialCreatesAssistantSummary` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:720`<br>`TestApprovalDenialRecordsResumedAndDeniedAuditEvents` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:792`<br>`TestApprovedPendingRunMarksFailureWhenToolExecutionFails` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:862`<br>`TestGoogleADKExecutionRunHonorsContextDeadline` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:881`<br>`TestStartRunUsesConfiguredRuntimeTimeout` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:898`<br>`TestResumeGoalRunAllowsTimedOutGoalWithFreshTimeoutWindow` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:947`<br>`TestReconcileExpiredRunsMarksHungRunTimedOut` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/store_test.go:1005`<br>`TestReconcileExpiredRunsUsesRunSpecificTimeout` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/task_runner_test.go:13`<br>`TestGoogleADKTaskRunnerBoundsFanOutAndRunsEveryTask` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/task_runner_test.go:79`<br>`TestGoogleADKTaskRunnerRunsCancelledTasksWithOriginalContext` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/taskset_biz_test.go:9`<br>`TestWorkflowPlanningHelpersPreserveBusinessOrdering` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/timeline_projection_helpers_test.go:11`<br>`TestTimelineProjectionHelperBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tool_artifact_materialization_test.go:24`<br>`TestMaterializeToolOutputPersistsLargeResearchResultsAsArtifacts` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tool_artifact_materialization_test.go:52`<br>`TestMaterializeToolOutputFallsBackWithoutAnArtifactOrOnSaveFailure` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tool_artifact_materialization_test.go:73`<br>`TestArtifactToolSelectionAndSafeNames` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tool_registry_change_test.go:8`<br>`TestToolRegistryOnChangeNotifiesAndUnsubscribesIdempotently` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tool_schema_workflow_test.go:10`<br>`TestWorkflowManagementToolSchemasAreStrictAndConvertible` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tool_schema_workflow_test.go:35`<br>`TestExecuteRegisteredToolPreservesInvocationSessionID` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_net_transport_boundaries_test.go:12`<br>`TestHTTPFetchToolNetworkBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_security_test.go:11`<br>`TestRejectUnsafeHostCoversDNSAndLocalhostBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_security_test.go:33`<br>`TestUnsafeAddrCoversIPv6MetadataAndDocumentationRanges` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:25`<br>`TestToolRegistrySerializesEmptyApprovalModesAsArray` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:44`<br>`TestDefaultTaskToolSchemaIncludesPlannerProjectionFields` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:61`<br>`TestModelsListToolRegisteredWithSafeSchema` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:87`<br>`TestTaskWriteToolsMarkedLowRiskCanSkipApproval` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:106`<br>`TestLowRiskWriteToolsCanSkipApproval` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:129`<br>`TestApprovalModeRequiresMediumAndHigherRiskApproval` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:145`<br>`TestResearchBacktestExplicitlySkipsApproval` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:163`<br>`TestWorkflowWaitToolWaitsAndDoesNotRequireApproval` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:195`<br>`TestWorkflowWaitToolRejectsTooLongDuration` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:206`<br>`TestWorkflowWaitToolReturnsContextCancellation` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:219`<br>`TestWorkflowWaitDurationParsesMultipleInputForms` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:275`<br>`TestHTTPFetchToolRejectsInvalidAndUnsafeTargets` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:315`<br>`TestRejectUnsafeHostAndUnsafeAddrClassification` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:342`<br>`TestHTTPFetchToolHandlesResponsesWithoutRealNetwork` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:468`<br>`TestAccountOrdersCompletesWithoutHanging` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:605`<br>`TestAccountOrdersWithSlowPortfolioSummary` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:700`<br>`TestLiveTradingToolsAreAvailableInAllModesWithApproval` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:726`<br>`TestBacktestToolsIncludeRequiredKLineSyncStatusCompanion` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:743`<br>`TestToolDescriptorsRespectExplicitAccessModes` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:775`<br>`TestChatContinuesAfterToolFailure` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/tools_test.go:837`<br>`TestAccountOrdersStreamCompletes` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/usageprojection/projection_test.go:10`<br>`TestTrackerAccumulatesFinalUsageOnceAndPreservesHistory` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/usageprojection/projection_test.go:27`<br>`TestTrackerIgnoresPartialMissingAndUnidentifiedEvents` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/usageprojection/projection_test.go:41`<br>`TestTrackerContinuesFromPersistedUsage` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_native_integration_test.go:22`<br>`TestGoogleADKWorkflowNativeAgentNodeForwardsPartialAndFinalOutput` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_native_integration_test.go:75`<br>`TestGoogleADKWorkflowNativeAgentNodeStopsWithConsumer` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_native_integration_test.go:122`<br>`TestGoogleADKWorkflowNativeAgentNodePreservesBranchAndIsolation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_native_integration_test.go:162`<br>`TestGoogleADKWorkflowNativeAgentNodeResumesConfirmationAfterRecreation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_runtime_branches_test.go:53`<br>`TestGoogleADKWorkflowRootAdapterRuntimeBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_test.go:21`<br>`TestGoogleADKWorkflowResumeResponsesMatchToolConfirmationByInterruptID` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_test.go:47`<br>`TestGoogleADKWorkflowResumeResponsesMatchOpenLongRunningCall` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_test.go:81`<br>`TestGoogleADKWorkflowResumeResponsesIgnoreUnmatchedFunctionResponse` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_test.go:99`<br>`TestGoogleADKWorkflowResumeResponsesIgnoreAlreadyConsumedInterrupt` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_test.go:118`<br>`TestGoogleADKWorkflowResumeBoundaryHelpersFailClosed` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_test.go:136`<br>`TestGoogleADKWorkflowAgentDoesNotFreshRunUnmatchedFunctionResponse` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_test.go:176`<br>`TestNewGoogleADKWorkflowAgentUsesNativeWorkflowAgentWithoutConcurrencyCap` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_test.go:253`<br>`TestGoogleADKWorkflowChildNodeUsesNativeAgentNodeConfiguration` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_agent_test.go:274`<br>`TestGoogleADKWorkflowNativeAgentNodeResumesToolConfirmation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_approval_recovery_boundaries_test.go:8`<br>`TestWorkflowApprovalRecoveryPreservesUserPauseAndContextFailures` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_approval_test.go:9`<br>`TestWorkflowApprovalAdditionalBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_canvas_test.go:11`<br>`TestWorkflowCanvasCompilerSequentialFanOutAndJoin` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_canvas_test.go:60`<br>`TestWorkflowCanvasCompilerRejectsInvalidGraphs` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_canvas_test.go:133`<br>`TestRunCanvasWorkflowExecutesAReachableAgentGraph` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_canvas_test.go:167`<br>`TestRunCanvasWorkflowPausesForAChildInputRequest` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_canvas_test.go:223`<br>`TestRunCanvasWorkflowFailsClosedWhenTheChildProviderIsUnavailable` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_child_test.go:8`<br>`TestUserPausedGoalParentPreservesPauseWhileChildStateChanges` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_child_test.go:79`<br>`TestCompletedChildReopensPendingParentWorkflowToRunning` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_child_test.go:137`<br>`TestNonWorkflowParentIgnoresChildWorkflowCallbacks` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_compiler_test.go:25`<br>`TestWorkflowCompilerBuildsJoinForFanIn` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_compiler_test.go:51`<br>`TestWorkflowCompilerKeepsDefaultSequentialDependencies` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_compiler_test.go:77`<br>`TestWorkflowCompilerDeduplicatesAndIgnoresBlankDependencies` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_compiler_test.go:97`<br>`TestWorkflowCompilerRejectsUnknownDependencies` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_execution_persistence_test.go:7`<br>`TestSanitizeWorkflowPlanStepRewritesEchoedMessages` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_finalization_contracts_test.go:7`<br>`TestRegisterWorkflowExecutionTracksParentAndChildRuns` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_goal_test.go:13`<br>`TestLoopWorkflowCanBeSelectedPerRun` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_goal_test.go:52`<br>`TestGoalWorkflowMissingDecisionSafelyContinuesUntilPaused` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_goal_test.go:81`<br>`TestGoalWorkflowContinueRespectsMaxIterations` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_goal_test.go:119`<br>`TestGoalWorkflowPauseAfterContinueAndResume` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_goal_test.go:222`<br>`TestGoalWorkflowPauseRequestedBeforeCompleteDecisionPausesInsteadOfCompleting` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_goal_test.go:282`<br>`TestGoalWorkflowPauseRequestBlocksChildCompletionContinuation` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_goal_test.go:342`<br>`TestGoalWorkflowActivitySnapshotDoesNotDowngradeUserPausedParent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_goal_test.go:389`<br>`TestGoalWorkflowDecisionPromptUsesUpdatedObjective` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_goal_test.go:457`<br>`TestUpdateRunObjectiveOnlyAllowsActiveGoalParent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_helpers_provider_failures_test.go:10`<br>`TestWorkflowHelpersAndProviderFailureBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_observation_projection_test.go:11`<br>`TestWorkflowObservationProjectsNodeLifecycleOntoParentAndChildRuns` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_observation_projection_test.go:62`<br>`TestWorkflowObservationHelperBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_persistence_test.go:9`<br>`TestExpiredRunReconciliationReturnsTerminalPersistenceFailure` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_plan_boundaries_test.go:9`<br>`TestWorkflowPlanAgentResolutionFailure` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_planner_runtime_test.go:12`<br>`TestWorkflowPlannerRuntimeBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_reconcile_test.go:8`<br>`TestTaskWorkflowApprovalContinuesParentWorkflow` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_reconcile_test.go:49`<br>`TestTaskWorkflowApprovalDeniedTerminatesParentWorkflow` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_reconcile_test.go:73`<br>`TestPendingChildCanReopenCompletedRunningParentWorkflow` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_reconcile_test.go:128`<br>`TestWorkflowParentReconcilesResolvedChildApproval` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_resume_test.go:9`<br>`TestResumeLoopWorkflowHonorsUserPauseAndCompletedChild` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_resume_test.go:65`<br>`TestRunChildAndWorkflowResumeEdgeCases` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_store_boundaries_test.go:24`<br>`TestWorkflowStoreBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_store_test.go:9`<br>`TestWorkflowStoreCRUDSoftDeleteAndLogs` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_tools_test.go:28`<br>`TestGoogleADKToolsetRunsRegisteredToolsAndNormalizesResponses` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_tools_test.go:140`<br>`TestGoogleADKProductToolsetFunctionToolBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_tools_test.go:206`<br>`TestGoogleADKProductToolsetRejectsInvalidFunctionSchema` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_tools_test.go:222`<br>`TestGoogleADKProductToolsetEmptySelectionReturnsNil` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_tools_test.go:230`<br>`TestGoogleADKToolResponseErrorHelpers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_tools_test.go:263`<br>`TestGoogleADKToolErrorEnvelopeClassifiesRetryability` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_tools_test.go:285`<br>`TestGoogleADKSkillFilteringAndToolsetsRespectAgentPermissions` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_tools_test.go:438`<br>`TestGoogleADKToolsetsBoundaryErrorsAndArtifactToolsets` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_tools_test.go:474`<br>`TestGoogleADKLLMAgentDirectToolsIncludeMemoryWhenEnabled` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_tools_test.go:488`<br>`TestGoogleADKToolsetsIncludeADKArtifactTools` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflow_tools_test.go:559`<br>`TestWorkflowStoreTriggerDeletionAndLogLookupBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/executor_integration_test.go:56`<br>`TestWorkflowExecutorRunsLoopChatEndToEnd` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/goal_resume_failure_boundaries_test.go:26`<br>`TestGoalResumeSurfacesReconcileAndPersistenceFaults` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/goal_resume_failure_boundaries_test.go:86`<br>`TestGoalDecisionErrorsAndTerminalFallbacksStayObservable` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/goal_state_boundaries_test.go:16`<br>`TestGoalWorkflowStateBoundariesFailClosedAndRemainResumable` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/goal_state_boundaries_test.go:158`<br>`TestWorkflowResumeReconcilerPausesOnRequestAndExposesStoreFailures` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/goal_turn_failure_boundaries_test.go:11`<br>`TestGoalTurnPersistsUserPauseAndTerminatesFailedChildren` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/goal_turn_failure_boundaries_test.go:84`<br>`TestGoalTurnFailsClosedWhenTaskStateCannotBeRead` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/goal_turn_failure_boundaries_test.go:115`<br>`TestGoalWorkflowSaveFailureReturnsFailedResponseWithoutRunningModel` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/persistence_failure_boundaries_test.go:14`<br>`TestGoalTurnPersistenceFailuresBubbleThroughTheOrchestrator` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/persistence_failure_boundaries_test.go:111`<br>`TestWorkflowTerminalProjectionPersistenceFailuresDoNotDisappear` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/persistence_failure_boundaries_test.go:217`<br>`TestGoalProjectionPersistenceFailuresStopAtTheirBoundary` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/persistence_propagation_closeout_test.go:14`<br>`TestGoalPausePersistenceErrorsPropagateAcrossDecisionBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/persistence_propagation_closeout_test.go:51`<br>`TestGoalDecisionAndChildTerminationWritesFailClosed` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/persistence_propagation_closeout_test.go:124`<br>`TestNativeTaskGraphProviderFailurePersistsTheParent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/persistence_propagation_closeout_test.go:165`<br>`TestWorkflowTaskToolsetNameAndModelsListContract` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/persistence_propagation_closeout_test.go:179`<br>`TestWorkflowResumePausedPersistenceFailureRemainsObservable` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/persistence_propagation_closeout_test.go:193`<br>`TestWorkflowCompletionBlockerMissingChildSemantics` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/taskset_biz_test.go:9`<br>`TestWorkflowTaskToolsetBusinessLifecycle` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/taskset_done_test.go:9`<br>`TestWorkflowTaskToolsetCompleteHonorsTaskAndChildRunBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/taskset_done_test.go:107`<br>`TestWorkflowGoalCompleteBlocksUnfinishedChildrenAndApprovals` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_approval_persistence_boundaries_test.go:12`<br>`TestWorkflowResumeAndCompletePersistenceFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_approval_recovery_boundaries_test.go:11`<br>`TestWorkflowApprovalRecoveryReturnsPausedSaveFailure` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_approval_recovery_boundaries_test.go:28`<br>`TestWorkflowApprovalReconcileFailsClosedOnTaskAndRunPersistence` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_child_failure_persistence_test.go:10`<br>`TestWorkflowChildFailurePersistsTerminalStateAndFallbackAgent` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_child_finalization_boundaries_test.go:10`<br>`TestWorkflowChildrenFailClosedForMissingAgentsAndFinalReplies` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_child_finalization_boundaries_test.go:66`<br>`TestWorkflowChildrenSkipIdleOrApprovalBlockedFinalization` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_child_lifecycle_test.go:13`<br>`TestWorkflowChildLifecycleBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_execution_failure_boundaries_test.go:13`<br>`TestWorkflowTaskStorageFailuresFailClosed` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_execution_failure_boundaries_test.go:73`<br>`TestWorkflowResponseIndexProtectsTaskState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_execution_failure_boundaries_test.go:108`<br>`TestWorkflowExecutionSetupSurfacesRecoverableFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_execution_persistence_test.go:14`<br>`TestWorkflowExecutorRunAndFinalizePersistence` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_execution_persistence_test.go:123`<br>`TestWorkflowExecutorSaveAndCancelBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_executor_boundary_branches_test.go:11`<br>`TestWorkflowExecutorAdditionalBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_executor_boundary_branches_test.go:85`<br>`TestWorkflowTaskModelsListUsesRuntimeProviderCatalog` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_executor_boundary_branches_test.go:121`<br>`TestWorkflowModelsListToolNameAndClosedRuntime` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_finalization_contracts_test.go:10`<br>`TestWorkflowExecutorPersistsFinalizedAndIncompletePlans` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_finalization_contracts_test.go:65`<br>`TestWorkflowExecutorPreparesParentPlanAndEmitsAuthoritativeSnapshot` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_goal_pause_boundaries_test.go:11`<br>`TestGoalWorkflowPauseRequestBeforeNextTurnDoesNotCallModel` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_goal_pause_boundaries_test.go:55`<br>`TestWorkflowResponseUsesAuthoritativePauseRequestedParent` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_goal_terminal_helpers_test.go:12`<br>`TestGoalWorkflowTerminalHelpersPersistCompletionContinuationAndStablePause` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_helpers_test.go:11`<br>`TestWorkflowHelperBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_pending_input_contract_test.go:10`<br>`TestWorkflowExecutorProjectsPendingInputAndPersistsChildState` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_persistence_test.go:12`<br>`TestGoalWorkflowFailsWhenInitialStateCannotBePersisted` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_persistence_test.go:35`<br>`TestCompletedWorkflowFailsWhenTerminalStateCannotBePersisted` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_persistence_test.go:67`<br>`TestUserPauseFailsWhenPausedStateCannotBePersisted` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_persistence_test.go:96`<br>`TestGoalWorkflowFailsWhenIterationLimitPauseCannotBePersisted` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_reconcile_executor_boundaries_test.go:9`<br>`TestTaskResumeUsesStoredPendingChildBeforeCompletingParent` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_reconcile_executor_boundaries_test.go:69`<br>`TestTaskResumeUsesStoredRunningChildBeforeCompletingParent` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_reconcile_executor_boundaries_test.go:110`<br>`TestTaskResumeTerminatesParentForStoredTerminalChild` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_reconcile_executor_boundaries_test.go:164`<br>`TestCompleteResumedWorkflowClearsTerminalPendingApprovals` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_reconcile_ignore_boundaries_test.go:9`<br>`TestReconcileWorkflowChildrenIgnoresMissingAndForeignRuns` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_resume_approval_boundaries_test.go:13`<br>`TestRunChildBlocksDelegatedApprovalTask` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_resume_executor_boundaries_test.go:10`<br>`TestResumeLoopWorkflowKeepsUserPausedParentPaused` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_resume_executor_boundaries_test.go:35`<br>`TestRunChildSurfacesDeltaSinkErrorsAfterChildLaunchSnapshot` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_resume_executor_boundaries_test.go:106`<br>`TestRunChildBlocksTaskWhenChildExecutionFailsImmediately` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_resume_executor_boundaries_test.go:173`<br>`TestResumeLoopWorkflowHonorsPauseRequestAfterChildCompletion` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_task_limit_boundaries_test.go:11`<br>`TestWorkflowExecutorRejectsRuntimeTaskOverflow` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_task_state_contracts_test.go:11`<br>`TestPauseGoalWorkflowPrunesInterruptedInternalToolCalls` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_task_state_contracts_test.go:49`<br>`TestPrepareGoalWorkflowTurnHandlesPendingChildrenBlockedTasksAndErrors` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_task_tools_boundaries_test.go:13`<br>`TestWorkflowTaskToolsetBusinessBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_task_tools_boundaries_test.go:69`<br>`TestWorkflowTaskToolsetErrorBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_task_tools_goal_test.go:7`<br>`TestWorkflowTaskToolsetSwitchesToGoalDecisionTools` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_task_tools_lookup_test.go:10`<br>`TestWorkflowTaskToolsetLookupBoundaryBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_task_tools_lookup_test.go:135`<br>`TestWorkflowTaskToolsetMethodErrorAndFallbackBranches` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_task_tools_persistence_test.go:13`<br>`TestWorkflowTaskToolsReturnParentPlanPersistenceFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowexec/workflow_task_tools_persistence_test.go:112`<br>`TestWorkflowTaskToolsReturnParentPlanRefreshFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowruntime/runtime_test.go:11`<br>`TestFacadeConstructorsAndSessionService` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/engine/workflowruntime/runtime_test.go:48`<br>`TestFacadeKeepsRuntimeAssemblyHelpers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:8`<br>`TestProviderReasoningPresetsAndExplicitEmptyMappings` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:27`<br>`TestProviderReasoningValidationAndCustomMapping` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:65`<br>`TestOptionalReasoningEffortRejectsDefault` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/model/timeline_helper_test.go:5`<br>`TestTimelineHelperBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/model/timeline_reply_ordering_test.go:11`<br>`TestBuildSessionTimelinePlacesFinalReplyAfterToolActivity` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/model/workflow_graph_resume_identity_test.go:5`<br>`TestWorkflowGraphFingerprintNormalizesSetsAndDetectsExecutionDrift` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/model/workflow_plan_test.go:8`<br>`TestWorkflowPlanPresentationKeepsDeterministicHumanState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/model/workflow_plan_test.go:31`<br>`TestWorkflowPlanBoundaryFormattingAndGraphSemantics` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/model/workflow_plan_test.go:100`<br>`TestApprovalsForRunScoping` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/model/workflow_task_tools_test.go:8`<br>`TestWorkflowGoalDecisionAndUtilityContracts` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_audit_pagination_test.go:9`<br>`TestServiceGetAuditPagePreservesFilteredPagination` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_builtin_agent_edit_test.go:10`<br>`TestPrimaryBuiltinAgentAllowsOnlyProviderReasoningSettings` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_business_helpers_test.go:22`<br>`TestServiceProviderChatAndSkillWrappers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_business_helpers_test.go:105`<br>`TestServiceSessionContextCompactionWrapper` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_business_helpers_test.go:145`<br>`TestServiceRuntimeUnavailableErrorBranches` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_business_helpers_test.go:189`<br>`TestServiceOptimizationTaskLifecycleAndMetrics` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_business_test.go:12`<br>`TestServiceSaveAgentValidationScenarios` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_business_test.go:117`<br>`TestServicePreviewSessionScenarios` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_business_test.go:169`<br>`TestServiceRecoverTerminalChatResponseFromProjection` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_business_test.go:230`<br>`TestServiceCRUDQueriesAndSnapshots` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_business_test.go:433`<br>`TestServiceRunLifecycleAndApprovalWrappers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_contract_boundaries_test.go:12`<br>`TestServiceCatalogCRUDRecordsBusinessAudit` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_contract_boundaries_test.go:133`<br>`TestServiceSessionAndRunReadBoundaries` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_contract_boundaries_test.go:224`<br>`TestServiceRuntimeUnavailableCatalogAndReadWriteBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_contract_boundaries_test.go:312`<br>`TestServiceCloseClosesRuntimeOwnedResources` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_lifecycle_boundaries_test.go:10`<br>`TestServiceLifecycleAndTimeoutBoundaryHelpers` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_lifecycle_boundaries_test.go:25`<br>`TestApprovalWaitDurationMsHandlesBoundaryTimestamps` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_lifecycle_boundaries_test.go:71`<br>`TestServicePreviewSessionFallsBackWhenRequestedSessionIsMissing` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_lifecycle_boundaries_test.go:95`<br>`TestServiceRecoverTerminalChatResponseHandlesBlankRunIDAndMissingProjection` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_persistence_runtime_boundaries_test.go:12`<br>`TestServicePropagatesCancelledPersistenceContextAcrossAssistantResources` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_persistence_runtime_boundaries_test.go:119`<br>`TestCanvasWorkflowNodeProjectionPreservesNodeSpecificBusinessState` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_persistence_runtime_boundaries_test.go:164`<br>`TestWorkflowUtilityAndUnavailableInputResolutionBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_persistence_runtime_boundaries_test.go:180`<br>`TestWorkflowEntryPointsRejectUnavailableRuntimeBeforePersistingAnything` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_persistence_runtime_boundaries_test.go:228`<br>`TestAgentValidationAndSchedulerBoundariesProtectRuntimeResources` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_recovery_test.go:10`<br>`TestServiceRecoverTerminalChatResponseFallsBackToLatestAssistant` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_skill_state_recovery_test.go:19`<br>`TestServiceSkillRecoveryContracts` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_skill_state_recovery_test.go:61`<br>`TestServiceAuditAndOptimizationStateRecovery` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_skill_state_recovery_test.go:119`<br>`TestCancelOptimizationTaskPropagatesPersistenceFailure` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_test.go:9`<br>`TestWrapSessionTimelineErrorPreservesErrorChain` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_test.go:24`<br>`TestServiceUnavailableWithoutRuntime` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_test.go:38`<br>`TestServiceOptionsExposeRuntimeSettings` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/service_test.go:49`<br>`TestAgentTemplatesAvailableWithoutRuntime` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow/rules_test.go:12`<br>`TestNextScheduleRunUsesFiveFieldCronAndTimezone` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow/rules_test.go:40`<br>`TestEvaluateMarketThresholdTriggerEdgesAndCooldown` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow/rules_test.go:89`<br>`TestEventRulesNormalizeAndValidate` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow/rules_test.go:135`<br>`TestRuleHelpersAndValidationEdges` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow/rules_test.go:207`<br>`TestRuleFallbacksMismatchesAndValidVariants` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_async_tools_test.go:12`<br>`TestStartWorkflowQueuesAndCompletesInBackground` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_async_tools_test.go:49`<br>`TestStartWorkflowTriggerSkipsWhenPreviousRunIsActive` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_async_tools_test.go:86`<br>`TestStartWorkflowBackgroundFailureTerminatesLog` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_crud_test.go:14`<br>`TestWorkflowResourceCrudPaginationAndLogs` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_crud_test.go:116`<br>`TestWorkflowTriggerRunWebhookAndValidationFailures` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_crud_test.go:228`<br>`TestWorkflowDefinitionValidationAndMissingResourceErrors` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_crud_test.go:314`<br>`TestWorkflowInvocationFailureAndBackgroundPaths` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_crud_test.go:400`<br>`TestWorkflowInvocationPreflightAndStaleResourcePaths` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_crud_test.go:455`<br>`TestWorkflowInvocationPersistsOrReturnsEachLogWriteFailure` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_crud_test.go:497`<br>`TestWorkflowActiveRunGuardHandlesStoredRunStates` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_crud_test.go:555`<br>`TestWorkflowEventMatchingCooldownAndUtilityBoundaries` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_crud_test.go:616`<br>`TestWorkflowMarketThresholdAndConfigHelpersCoverEdges` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_crud_test.go:671`<br>`TestWorkflowCoreHelpersAndUnavailableService` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_lifecycle_test.go:13`<br>`TestServiceCloseCancelsAndJoinsAdmittedWorkflowBackground` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_lifecycle_test.go:58`<br>`TestServiceCloseKeepsStoreOpenUntilWorkflowCleanupFinishes` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_lifecycle_test.go:98`<br>`TestWorkflowSchedulerStopCancelsAndJoinsInFlightTick` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflow_store_failures_test.go:13`<br>`TestWorkflowOperationsPropagateClosedStoreFailures` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_extended_test.go:14`<br>`TestWorkflowTriggerValidationAndBoundaryHelpers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_extended_test.go:135`<br>`TestWorkflowBuiltinTemplatesWatchedInstrumentsAndScheduleHelpers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_extended_test.go:208`<br>`TestWorkflowSchedulerTickAndMarketPollingStablePaths` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_extended_test.go:278`<br>`TestWorkflowEventAndSchedulerTriggerBackgroundRuns` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_extended_test.go:528`<br>`TestWorkflowActiveRunSkipAndReconciliation` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_extended_test.go:631`<br>`TestWorkflowResultAndRunStatusHelpers` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_resource_recovery_test.go:11`<br>`TestWorkflowResourcesRejectCrossWorkflowAndInvalidRequests` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_resource_recovery_test.go:52`<br>`TestWorkflowAsyncTriggerAndBackgroundRecovery` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_resource_recovery_test.go:105`<br>`TestWorkflowBackgroundPersistsFailureAfterRunningTransitionWriteFails` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_test.go:11`<br>`TestNextWorkflowScheduleRunUsesFiveFieldCronAndTimezone` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_test.go:39`<br>`TestEvaluateMarketThresholdTriggerEdgesAndCooldown` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_test.go:88`<br>`TestWorkflowWebhookTriggerSecretLifecycle` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_test.go:157`<br>`TestSaveWorkflowRoundTripsCanvasGraph` | assistant_workflow | 普通边界 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_test.go:198`<br>`TestRunWorkflowStoresResultAndNodeTrace` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_test.go:248`<br>`TestRunWorkflowWithoutCanvasGraphFailsInsteadOfChatFallback` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/assistant/workflows_test.go:276`<br>`TestRunWorkflowCanvasCompilesAndStoresNodeOutputs` | assistant_workflow | 高风险 | `crates/jftrade-assistant` |
| [ ] | `go:452dea11:internal/backtest/business_test.go:12`<br>`TestServiceQueryMethodsHandleNilStoresAndFullList` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/business_test.go:59`<br>`TestServiceCoverageAndNormalizationBoundaries` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/historical_source_test.go:59`<br>`TestHistoricalKLineSyncerPaginatesBackwardAndIsolatesProvider` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/historical_source_test.go:111`<br>`TestHistoricalKLineSyncerCancelsInFlightProviderPage` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/historical_source_test.go:147`<br>`TestHistoricalKLineSyncerRetriesTransientPageAndRejectsCapabilitiesDuringPreflight` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/historical_source_test.go:181`<br>`TestHistoricalKLineSyncerRejectsEmptyProviderResult` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/historical_source_test.go:198`<br>`TestValidateHistoricalKLineSyncAllowsMissingCapabilityValidator` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/historical_source_test.go:204`<br>`TestHistoricalKLineSyncerValidatesLifecycleAndTerminalProviderFailures` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/historical_source_test.go:268`<br>`TestHistoricalKLineSyncerRejectsBrokenPagination` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/historical_source_test.go:309`<br>`TestHistoricalCandleConversionRejectsInvalidFieldsAndDefaultsVolume` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/historical_source_test.go:341`<br>`TestHistoricalProviderRetryExhaustionAndTimerCancellation` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/input_and_readiness_validation_test.go:14`<br>`TestBacktestDateRangeRejectsIncompleteAndInvalidMarketInputs` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/input_and_readiness_validation_test.go:53`<br>`TestBacktestDataPreparationRejectsInvalidCandidatesBeforeStartingSync` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/input_and_readiness_validation_test.go:87`<br>`TestBacktestProviderOverrideValidationAndResolution` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/input_and_readiness_validation_test.go:122`<br>`TestBacktestStartRejectsUnsupportedProviderBeforeQueueing` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/input_and_readiness_validation_test.go:137`<br>`TestBacktestStartRejectsMissingCoverageBeforePersistingRun` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/input_and_readiness_validation_test.go:161`<br>`TestBacktestSyncHelpersHandleDefaultsAndAdapterSetupFailures` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/input_and_readiness_validation_test.go:181`<br>`TestBacktestDiagnosticHelpersRejectUnexpectedDynamicValues` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/input_and_readiness_validation_test.go:191`<br>`TestResultViewValidationRejectsMalformedWindowsAndResolutions` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/input_and_readiness_validation_test.go:218`<br>`TestDataReadinessPropagatesCoverageFailuresAndExistingSyncTerminalStates` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/input_and_readiness_validation_test.go:307`<br>`TestDataReadinessPinsProviderAcrossCoverageAndSyncAcceptance` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/recovery_test.go:11`<br>`TestBacktestExecutionPersistsFailureWhenRunnerReturnsNil` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/recovery_test.go:27`<br>`TestBacktestExecutionRecoversRunnerPanicIntoFailedRun` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/recovery_test.go:45`<br>`TestStartScriptRejectsBlankResearchScript` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/result_view_aggregation_test.go:10`<br>`TestResultViewExposesWarningsAndFiltersChartSeries` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/result_view_aggregation_test.go:84`<br>`TestResultViewAggregationDropsDamagedCandlesWithoutInventingVolume` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/result_view_test.go:12`<br>`TestResultViewRunPayloadPreservesProviderAndExecutionMetadata` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/result_view_test.go:34`<br>`TestResultViewOrdersLogsAndErrorsUseWindowAndCursor` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/result_view_test.go:122`<br>`TestResultViewParsingAndResolutionBoundaries` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/result_view_test.go:199`<br>`TestResultViewCandlesFiltersInvalidTimesAndAggregatesVolumeBoundaries` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/result_view_test.go:223`<br>`TestResultViewRejectsBadRequestsAndPreservesEmptyRunShape` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/result_view_test.go:310`<br>`TestResultViewSummaryPayloadIncludesRunMetadataAndLatestDiagnostics` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/run_failure_recovery_test.go:26`<br>`TestBacktestStartDoesNotLeakLifecycleTaskWhenQueuePersistenceFails` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/run_failure_recovery_test.go:53`<br>`TestBacktestPreparationRejectsInvalidInstrumentAndWarmupPlan` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/run_failure_recovery_test.go:81`<br>`TestBacktestKeepsTerminalStateWhenRunningTransitionCannotPersist` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_pineworker_test.go:12`<br>`TestServiceDefaultBacktestRequiresPineWorkerRunner` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_pineworker_test.go:27`<br>`TestServiceDefaultBacktestUsesConfiguredPineWorkerRunner` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:16`<br>`TestStartQueuesRunAndExecutesWithInjectedRunner` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:138`<br>`TestStartScriptQueuesResearchRunWithoutStrategyProvider` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:196`<br>`TestPrepareResolvedBacktestNormalizesChartType` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:221`<br>`TestEnsureScriptDataReturnsReadyAndIncludesDerivedWarmup` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:253`<br>`TestEnsureScriptDataStartsAndDeduplicatesKLineSync` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:295`<br>`TestEnsureScriptDataSurfacesFailedSyncWithoutRestarting` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:332`<br>`TestEnsureScriptDataStopsAfterCompletedSyncStillLacksCoverage` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:365`<br>`TestEnsureScriptDataBecomesReadyAfterCompletedSyncFillsCoverage` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:406`<br>`TestEnsureDefinitionsDataUsesMaximumCandidateWarmup` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:439`<br>`TestResultViewReturnsWindowedAggregatedChartData` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:512`<br>`TestStartValidationErrors` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:628`<br>`TestStartMarksFailedWhenRunnerReturnsError` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:652`<br>`TestCloseCancelsAndWaitsForActiveBacktest` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:680`<br>`TestRunStoreDelegation` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/service_test.go:732`<br>`TestFinishRunFallsBackToMemoryOnlyWhenPersistentUpdateFails` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:17`<br>`TestSyncProgressAndCancelDelegateToSyncTaskStore` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:38`<br>`TestSyncConvertsParamsAndClosesAdapter` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:106`<br>`TestSyncFailureMarksProgressFailedAndClosesAdapter` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:147`<br>`TestCloseCancelsAndWaitsForActiveSync` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:191`<br>`TestSyncTaskIDsAreUnique` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:221`<br>`TestSyncClosesAdapterWhenTaskStoreMissing` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:244`<br>`TestSyncRejectsUnsupportedProviderCombinationBeforeAcceptingTask` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:275`<br>`TestSyncRetainsConstructedAdapterValidation` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:298`<br>`TestSyncRequestErrors` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:332`<br>`TestSyncUnknownRehabTypeFallsBackToForward` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:367`<br>`TestPlanSyncIntervals` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/sync_test.go:412`<br>`TestParseSessionScope` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/time_test.go:8`<br>`TestResolveBacktestTimeRangeUsesMarketDateAndDST` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/time_test.go:33`<br>`TestResolveBacktestTimeRangeUsesHongKongCalendarDay` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/backtest/time_test.go:55`<br>`TestResolveBacktestTimeRangeNormalizesLegacyTimestamps` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:21`<br>`TestNormalizeExecutionModelName` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:53`<br>`TestConservativeBarExecutorValidationErrors` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:89`<br>`TestConservativeBarExecutorFillsMarketOrderOnNextOpenWithLiquidityCap` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:147`<br>`TestConservativeBarExecutorRunsParentBracketAtomicallyAndStopFirst` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:196`<br>`TestConservativeBarExecutorRejectsAtomicChildWithoutParent` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:214`<br>`TestConservativeBarExecutorFillsAtomicBracketOnSignalClose` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:245`<br>`TestConservativeBarExecutorCancelParentCancelsProtectiveChildren` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:276`<br>`TestConservativeBarExecutorCancelsReduceOnlyOrderWithoutPosition` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:302`<br>`TestConservativeBarExecutorLimitsReduceOnlyFillToOpenPosition` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:336`<br>`TestConservativeBarExecutorCancelOrders` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:383`<br>`TestConservativeBarExecutorProcessOrdersOnCloseUsesSignalClose` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:412`<br>`TestConservativeBarExecutorSellMarketAndSlippage` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:447`<br>`TestConservativeBarExecutorLimitOrderGetsGapImprovement` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:477`<br>`TestConservativeBarExecutorLimitSellAndClosePointBranches` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:515`<br>`TestConservativeBarExecutorStopOrders` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:575`<br>`TestConservativeBarExecutorWarningsAndUnmatchedOrders` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:602`<br>`TestConservativeBarExecutorLiquidityWarnings` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:652`<br>`TestConservativeBarExecutorHelperBranches` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/conservative_bar_executor_test.go:771`<br>`TestConservativeBarExecutorCancelSkipsUnmatchedPendingOrders` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/cost_account_failure_boundaries_test.go:13`<br>`TestFeeSchedulePreservesExplicitPresetIntentAndSafeEmptyFallback` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/cost_account_failure_boundaries_test.go:26`<br>`TestFeeEngineRejectsNonBillableTradesAndDoesNotDoubleChargeOrderFee` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/cost_account_failure_boundaries_test.go:56`<br>`TestAccountReadFailureDoesNotCreateSpeculativeEquityPoint` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/cost_account_failure_boundaries_test.go:68`<br>`TestPrepareKLineErrorClassifierRequiresBothStableFragments` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/filter_store_session_queries_test.go:137`<br>`TestSessionFilteredStoreDelegatesVerifyAndSync` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/filter_store_session_queries_test.go:160`<br>`TestSessionFilteredStoreQueryHelpersFilterUSRegularHours` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/filter_store_session_queries_test.go:246`<br>`TestSessionFilteredStoreStreamKLinesFallsBackToChannels` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/filter_store_session_queries_test.go:276`<br>`TestSessionFilteredStoreUsesCustomExtendedHoursRangeQueries` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/filter_store_session_queries_test.go:310`<br>`TestSessionFilteredStoreCustomBackwardQueriesTrimLatestWindow` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/filter_store_session_queries_test.go:346`<br>`TestSessionFilteredStoreQueryKLinesChIncludesCustomExtendedHoursRows` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/filter_store_session_queries_test.go:387`<br>`TestSessionFilteredStoreQueryKLinesChFiltersRegularHoursWithoutExtendedHours` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/filter_store_session_queries_test.go:413`<br>`TestSessionFilteredStoreQueryKLinesChPropagatesBaseAndCustomErrors` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/filter_store_session_queries_test.go:459`<br>`TestSessionFilteredStoreCustomRangeFallbackAndCursorBoundaries` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/filter_store_session_queries_test.go:540`<br>`TestSessionFilteredStoreStreamKLinesUsesStreamerAndCustomExtendedHoursRows` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/filter_store_session_queries_test.go:627`<br>`TestSessionFilteredStoreHelperFunctions` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/runmodel/result_test.go:8`<br>`TestRunResultSnapshotHandlesNilAndReturnsIndependentCopy` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/runmodel/result_test.go:126`<br>`TestRunResultSnapshotOmitsEmptyRuntimeErrorCounts` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/runmodel/result_test.go:137`<br>`TestAddRuntimeErrorReusesExistingSamplesAndCapsUniqueList` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/runmodel/result_test.go:172`<br>`TestRunResultWarningsTrackIgnoredOrdersAndCapSamples` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/runmodel/result_test.go:198`<br>`TestRunResultGroupsRepeatedIgnoredOrderWarnings` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/runmodel/result_test.go:224`<br>`TestRunResultWarningAndRuntimeErrorBoundaryAccounting` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/aggregate_corruption_errors_test.go:15`<br>`TestDailyAggregationPropagatesDamagedStorageErrors` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/codec_progress_boundaries_test.go:15`<br>`TestSyncProgressLifecycleAndNilReceiverBoundaries` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/codec_progress_boundaries_test.go:55`<br>`TestStoredFixedDecimalBoundaryMatrix` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/codec_progress_boundaries_test.go:90`<br>`TestStoredKLineScanningClassifiesMissingMalformedAndValidRows` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/codec_progress_boundaries_test.go:130`<br>`TestStorageSchemaFallbackBoundaries` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/codec_progress_boundaries_test.go:150`<br>`TestNewFutuKLineStoreRejectsBlankAndLegacyDatabase` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/codec_progress_boundaries_test.go:172`<br>`TestNewFutuKLineStoreClosesConnectionWhenCurrentSchemaInitializationFails` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/codec_test.go:11`<br>`TestParseStoredFixedRoundTripsFixedpointString` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/codec_test.go:51`<br>`TestParseStoredFixedFallsBackToUpstreamParser` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/query_failure_empty_recovery_test.go:11`<br>`TestAggregateQueryAPIsPropagateCoverageAndSourceFailures` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/query_failure_empty_recovery_test.go:120`<br>`TestStoredReadersSkipEmptyScopedTables` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_aggregation_boundaries_test.go:11`<br>`TestIntervalStorageValueCoversSupportedAndCustomIntervals` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_aggregation_boundaries_test.go:38`<br>`TestIntervalFromStorageValueCoversAllPersistedIntervals` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_aggregation_boundaries_test.go:71`<br>`TestReadSessionScopeNormalizationAndStorageTags` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_aggregation_boundaries_test.go:98`<br>`TestTradingPeriodIntervalHelpers` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_aggregation_boundaries_test.go:145`<br>`TestAggregationBaseIntervalsAndExtendedDailyPriority` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_aggregation_boundaries_test.go:173`<br>`TestAggregationBaseRangesRespectUSExtendedHours` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_aggregation_boundaries_test.go:194`<br>`TestPureAggregationHelpersHandleEmptyUnknownAndOutOfRangeInputs` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_aggregation_boundaries_test.go:234`<br>`TestSessionAwareIntradayAggregationMergesBarsInsideMarketBucket` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_business_aggregation_test.go:12`<br>`TestInsertKLineReplacesExistingBarAndQueryDefaultsToLatest` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_business_aggregation_test.go:37`<br>`TestFindMissingRangesUsesLowerIntervalCoverageAndVerifyAllowsOpenWindow` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_business_aggregation_test.go:84`<br>`TestQueryKLinesForwardAndBackwardAggregateWeeklyTradingPeriods` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_business_aggregation_test.go:144`<br>`TestQueryDailyKLinesInRangeAggregatesUSExtendedHoursFromHourlyBars` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_business_aggregation_test.go:168`<br>`TestAggregationMissingCoverageMessagesAndDailyFallbacks` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_business_aggregation_test.go:209`<br>`TestAggregationPureHelpersSkipUnsupportedOrUnlabelledRows` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_connection_test.go:14`<br>`TestNewFutuKLineStoreAllowsConcurrentReadConnections` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_connection_test.go:30`<br>`TestFutuKLineUnifiedControllerSerializesConcurrentWriters` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_connection_test.go:96`<br>`TestFutuKLineReadersRunInParallelWithLaterWALWrite` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_connection_test.go:156`<br>`TestFutuKLineReadWaitsForPreviouslyQueuedWrite` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_failure_boundaries_test.go:13`<br>`TestStoreAPIsPropagateClosedDatabaseFailures` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_failure_boundaries_test.go:68`<br>`TestClosedDatabaseFailuresPropagateThroughStorageHelpers` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_failure_boundaries_test.go:163`<br>`TestCompactSchemaRejectsLegacyKLineTableShapes` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_failure_boundaries_test.go:188`<br>`TestReadTableNamesHonorExplicitSessionScopePriority` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_failure_boundaries_test.go:207`<br>`TestStreamAndQueryShortCircuitEmptyInputs` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_query_aggregation_contracts_test.go:12`<br>`TestStoreRoundTripsScopedSeriesQueries` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_query_aggregation_contracts_test.go:81`<br>`TestStoreAggregatesFiveMinuteBarsFromOneMinuteCoverage` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_query_aggregation_contracts_test.go:158`<br>`TestStoreAggregatesDailyAndWeeklyBarsFromLowerIntervals` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:15`<br>`TestCompactDatabasePreservesUsableStoreAndReportsUnavailableStore` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:47`<br>`TestTableExistenceCacheRejectsUnexpectedDynamicTypes` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:59`<br>`TestSelectReadTableNameUsesFirstExistingTableForUnboundedReads` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:81`<br>`TestAggregateQueryHelpersHandleDailyTradingAndIntradayReadSources` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:132`<br>`TestAggregateForwardBackwardQueriesNormalizeLimitsAndUseSynthesizedBars` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:179`<br>`TestScopedReadUsesOnlyTheRequestedSeries` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:208`<br>`TestBatchInsertFailureRollsBackPriorBars` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:236`<br>`TestCoverageSelectionDistinguishesMissingBoundariesAndSynthesis` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:306`<br>`TestAggregationBoundaryHelpersKeepIncompleteAndOutOfWindowBarsOut` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:354`<br>`TestCoverageResolversReportUnsupportedAggregationAndClosedStoreErrors` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:380`<br>`TestAggregatedReadersDistinguishDirectSeriesCorruptBaseDataAndUnavailableStorage` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:431`<br>`TestStorageInvariantHelpersReportUnexpectedValues` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:441`<br>`TestCoverageQueriesHandleEmptyWindowsMissingTablesAndBrokenTables` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_runtime_invariants_test.go:500`<br>`TestSessionAwarePagingReportsCoverageGapAfterAPartialBatch` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_session_aggregation_contracts_test.go:11`<br>`TestSchemaHelpersExposeStableStorageContracts` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_session_aggregation_contracts_test.go:58`<br>`TestSessionAwareIntradayAggregationAcrossHKTradingSessions` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_session_aggregation_contracts_test.go:105`<br>`TestQuerySessionAwareIntradayKLinesInRangeSupportsUSExtendedHours` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/store_session_aggregation_contracts_test.go:132`<br>`TestQueryAPIsSortMultiSymbolAndMixedIntervalResults` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/stream_query_failure_sorting_test.go:11`<br>`TestStoredStreamFailuresRemainVisibleToAllQueryShapes` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/internal/storage/stream_query_failure_sorting_test.go:47`<br>`TestMultiSymbolStreamUsesSymbolAsFinalStableSortKey` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pine_costs_test.go:14`<br>`TestPineCommissionRateConvertsPercentToRate` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pine_costs_test.go:27`<br>`TestResolvePineInitialBalancePrecedence` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pine_costs_test.go:40`<br>`TestBacktestSlippagePriceUsesMarketTickSize` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pine_costs_test.go:64`<br>`TestBacktestSlippageExecutorSubmitsAdjustedMarketOrdersAndPassesCancels` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pine_ts_corpus_test.go:31`<br>`TestPinetsShadowCorpusReport` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pine_ts_shadow_reference_test.go:8`<br>`TestPinetsShadowEMAUsesSMAInitialization` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pine_ts_shadow_reference_test.go:21`<br>`TestPinetsShadowMACDSkipsNaNValuesForSignalInitialization` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pine_ts_smoke_test.go:20`<br>`TestRealPineTSBacktestSmoke` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:17`<br>`TestCommandsFromOrderIntents` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:41`<br>`TestCommandFromOrderIntentPreservesShortDirection` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:57`<br>`TestCommandFromOrderIntentMapsShortExitToBuy` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:74`<br>`TestScopedExitQuantitySurvivesGoExecution` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:111`<br>`TestCommandFromOrderIntentMapsConditionalOrderTypes` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:161`<br>`TestCommandFromOrderIntentRejectsUnsupportedExitBracket` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:175`<br>`TestCommandsFromOrderIntentsExpandsAtomicOCOExit` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:208`<br>`TestCommandFromOrderIntentCanonicalizesSellEntryAsShort` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:238`<br>`TestCommandFromOrderIntentDefaultsEntryQuantity` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:248`<br>`TestCommandFromOrderIntentRejectsUnsupportedIntent` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:293`<br>`TestPineWorkerBacktestAdapterRun` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_adapter_test.go:323`<br>`TestPineWorkerBacktestAdapterMapsErrors` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_atomic_boundaries_test.go:26`<br>`TestPineWorkerAtomicGroupRejectsEveryUnsafeShape` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_atomic_boundaries_test.go:67`<br>`TestPineWorkerAtomicExecutionFailureBoundaries` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:14`<br>`TestPineWorkerCommandExecutorSubmitsOrders` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:44`<br>`TestPineWorkerCommandExecutorRejectsQuantityPctWithoutSizing` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:58`<br>`TestPineWorkerCommandExecutorRejectsMissingQuantity` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:71`<br>`TestPineWorkerCommandExecutorSizesEntryQuantityPctFromEquity` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:97`<br>`TestPineWorkerCommandExecutorSizesCloseQuantityPctFromPosition` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:128`<br>`TestPineWorkerCommandExecutorDefaultsCloseToFullPosition` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:158`<br>`TestPineWorkerCommandExecutorTagsShortReplayOrders` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:176`<br>`TestPineWorkerCommandExecutorIgnoresCloseWithoutPosition` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:204`<br>`TestPineWorkerCommandExecutorIgnoresQuantityBelowMarketStep` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:233`<br>`TestPineWorkerCommandExecutorIgnoresHKOddLotBelowBoardLot` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:262`<br>`TestPineWorkerCommandExecutorIgnoresOrdersWhenMarketRulesUnavailable` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:292`<br>`TestPineWorkerCommandExecutorGroupsRepeatedIgnoredOrderWarnings` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:327`<br>`TestPineWorkerCommandExecutorAutoCloseCoversShortPosition` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:361`<br>`TestPineWorkerCommandExecutorHonorsExplicitShortCloseAndOptionalWarnings` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:406`<br>`TestPineWorkerCommandExecutorCancelsTrackedOrders` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:421`<br>`TestPineWorkerCommandExecutorCancelAll` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:440`<br>`TestPineWorkerCommandExecutorRejectsAtomicBracketBeforeAnySubmission` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:452`<br>`TestPineWorkerCommandExecutorSubmitsParentOCOBracketAtomically` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:486`<br>`TestPineWorkerCommandExecutorRejectsMalformedAtomicBracket` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:510`<br>`TestPineWorkerCommandExecutorPropagatesExecutorErrors` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:534`<br>`TestPineWorkerCommandExecutorBusinessBoundaryErrors` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:579`<br>`TestPineWorkerCommandExecutorGeneratedOrderIDStopsAndTrackingFallbacks` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_command_executor_test.go:626`<br>`TestPineWorkerCommandExecutorCancelBoundaries` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_pump_test.go:15`<br>`TestPineWorkerReplayPumpConsumesThenExecutesCommands` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_pump_test.go:59`<br>`TestPineWorkerReplayPumpValidatesReplayShape` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_pump_test.go:89`<br>`TestPineWorkerReplayPumpFinishDetectsMissingBars` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_pump_test.go:118`<br>`TestPineWorkerReplayPumpPropagatesCommandErrors` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_source_test.go:12`<br>`TestCollectPineWorkerReplayKLines` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_source_test.go:36`<br>`TestCollectPineWorkerReplayKLinesMapsErrors` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_source_test.go:51`<br>`TestPineWorkerReplayKLineBatchUsesFixedChunksAndExactResultCapacity` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_test.go:18`<br>`TestBuildPineWorkerBacktestRequestFromKLines` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_test.go:47`<br>`TestBuildPineWorkerBacktestRequestValidatesInputs` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_test.go:66`<br>`TestPineWorkerReplayPlannerPlanGroupsCommands` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_test.go:111`<br>`TestPineWorkerReplayPlannerRejectsInvalidCommandBarIndex` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_test.go:129`<br>`TestNormalizeReplayCommandsPreservesSameBarEmissionOrder` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_replay_test.go:144`<br>`TestPineWorkerReplayPlannerPropagatesWorkerError` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_boundaries_test.go:14`<br>`TestRunWithPineWorkerRejectsUnavailableRuntimeAndData` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_boundaries_test.go:37`<br>`TestRunWithPineWorkerValidatesSourceAndCoverageBeforeReplay` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_boundaries_test.go:75`<br>`TestResolveBacktestQuoteCurrencyDefaultsByMarket` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_boundaries_test.go:93`<br>`TestRemoveFutuMarketCacheToleratesMissingFile` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_failure_boundaries_test.go:16`<br>`TestPineWorkerRunnerRejectsInvalidConfigurationBeforeReplay` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_failure_boundaries_test.go:57`<br>`TestPineWorkerRunnerReplayStopsOnIntegrityFailures` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_failure_boundaries_test.go:94`<br>`TestPineWorkerRunnerFailsClosedForInvalidWorkerCommandsDuringReplay` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_test.go:19`<br>`TestRunWithPineWorkerExecutesReplayThroughGoMatching` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_test.go:91`<br>`TestRunWithPineWorkerReportsWarmupFillsThatAffectEquity` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_test.go:163`<br>`TestRunWithPineWorkerExecutesQuantityPctReplay` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_test.go:234`<br>`TestRunWithPineWorkerWarnsAndIgnoresInitialCloseSignal` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_test.go:293`<br>`TestRunWithPineWorkerWarnsWhenHKLotSizeUnavailable` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_test.go:358`<br>`TestEnsureBacktestSourceMarketReportsFallbackWarningWithoutRejectingOrders` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/pineworker_runner_test.go:381`<br>`TestRunWithPineWorkerMapsWorkerErrors` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/replay_sizer_bounds_test.go:13`<br>`TestPineWorkerReplaySizerRejectsInvalidQuantityPctInputs` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/replay_sizer_bounds_test.go:35`<br>`TestPineWorkerReplaySizerMaintainsPositionFromIncrementalFills` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/replay_sizer_bounds_test.go:88`<br>`TestPineWorkerReplaySizerEntryEquityBoundaries` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/replay_sizer_bounds_test.go:135`<br>`TestPineWorkerReplaySizerPriceAndQuantityPrecision` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_test.go:19`<br>`TestResultCollectorPersistsHeikinAshiWarmupSeed` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_test.go:66`<br>`TestResultCollectorBuildsTradesAndFinalStats` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_test.go:198`<br>`TestResultCollectorTracksPartialFillIncrementally` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_test.go:256`<br>`TestResultCollectorFeeBoundaryBranches` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_test.go:270`<br>`TestResultCollectorTracksDrawdownMetrics` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_test.go:303`<br>`TestResultCollectorWarnsOnNonPositiveCloseOnce` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_test.go:344`<br>`TestOrderBookIdentityHelpersPreferExchangeThenClientThenPending` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_test.go:392`<br>`TestResultCollectorFinalizeValuesOpenPositionAndMarksWarmupOrders` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_trade_stats_test.go:13`<br>`TestResultCollectorClosedTradeStatsUseWeightedPositionCost` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_trade_stats_test.go:28`<br>`TestResultCollectorClosedTradeStatsHandleLongShortAndReversal` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_trade_stats_test.go:63`<br>`TestResultCollectorClosedTradeStatsAggregatePartialClosingOrder` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_trade_stats_test.go:81`<br>`TestResultCollectorClosedTradeStatsFinalizePartiallyFilledCancellation` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_trade_stats_test.go:96`<br>`TestResultCollectorClosedTradeStatsIncludeWarmupClosuresThatAffectEquity` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/result_collector_trade_stats_test.go:116`<br>`TestResultCollectorTradeStatsDefensiveBoundaries` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/run_result_test.go:8`<br>`TestRunResultSnapshotReturnsIndependentCopy` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/run_result_test.go:83`<br>`TestRunResultAddRuntimeErrorAggregatesCountsAndCapsSamples` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/runner_hardcut_test.go:10`<br>`TestRunDirectGoPineRunnerIsDisabled` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/runner_helpers_test.go:10`<br>`TestBacktestRunnerHelpers` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/session_filter_store_boundaries_test.go:11`<br>`TestSessionFilteredReplayStorePassThroughAndCursorBoundaries` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/session_filter_store_boundaries_test.go:78`<br>`TestSessionFilteredReplayStoreStreamingContracts` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/session_filter_store_boundaries_test.go:141`<br>`TestSessionFilteredReplayStoreCustomAggregationFailuresAndOrdering` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/session_filter_store_test.go:9`<br>`TestSessionFilteredStoreCustomAggregationOnlyAppliesToUS` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/short_replay_bounds_test.go:14`<br>`TestPineWorkerShortReplayExecutorDelegationBoundaries` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/short_replay_bounds_test.go:48`<br>`TestPineWorkerShortReplayExecutorSyntheticValidation` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/short_replay_bounds_test.go:88`<br>`TestPineWorkerShortReplayExecutorSyntheticPriceFallbacks` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/short_replay_test.go:12`<br>`TestPineWorkerShortReplayExecutorFillsShortEntryAndCover` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/short_replay_test.go:74`<br>`TestPineWorkerShortReplayExecutorCancelOrdersFiltersSyntheticOrders` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/source_exchange_business_test.go:9`<br>`TestBacktestSourceExchangeExposesPinnedRulesWithoutLiveOperations` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/source_exchange_business_test.go:49`<br>`TestInstrumentSpecConservativeDefaultsRespectMarketProfiles` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_session_synth_test.go:11`<br>`TestFutuKLineStoreSynthesizesTwoHourFromUSSessionAwareBuckets` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_session_synth_test.go:75`<br>`TestFutuKLineStoreSynthesizesTwoHourAcrossHKLunchBreak` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_session_synth_test.go:138`<br>`TestFutuKLineStoreSynthesizesTwoHourForwardFromUSSessionAwareBuckets` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_session_synth_test.go:184`<br>`TestFutuKLineStoreQueryBackwardSessionAwarePaginationMatchesRangeSeries` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_session_synth_test.go:234`<br>`TestFutuKLineStoreSynthesizesDailyFromUSRegularTradingHours` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_session_synth_test.go:302`<br>`TestFutuKLineStoreSynthesizesDailyAcrossHKLunchBreak` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_session_synth_test.go:376`<br>`TestFutuKLineStoreSynthesizesWeeklyFromDailyTradingDays` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_session_synth_test.go:468`<br>`TestFutuKLineStoreSynthesizesMonthlyFromDailyTradingDays` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:16`<br>`TestNewFutuKLineStoreCreatesCompactSchema` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:85`<br>`TestNewFutuKLineStoreUsesSeparateTablesPerDimension` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:120`<br>`TestFutuKLineStoreSeparatesScopedSyncVersions` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:183`<br>`TestFutuKLineStoreRegularScopeDoesNotReadExtendedRowsWhenCoverageIsIncomplete` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:250`<br>`TestFutuKLineStoreQueryKLinesChPrefersRegularScopedRangeWhenCoverageIsComplete` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:292`<br>`TestFutuKLineStoreQueryKLinesChDoesNotFallbackWhenRegularRangeIsIncomplete` | backtest_calendar | 高风险 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:330`<br>`TestFutuKLineStoreRoundTripsCompactRows` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:409`<br>`TestFutuKLineStoreFiltersByRehabType` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:495`<br>`TestFutuKLineStoreVerifyAcceptsOverlappingCoverage` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:510`<br>`TestFutuKLineStoreVerifyReportsMissingCoverage` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:532`<br>`TestFutuKLineStoreSynthesizesFiveMinuteFromOneMinute` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:584`<br>`TestFutuKLineStoreSynthesizesFifteenMinuteFromFiveMinute` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:652`<br>`TestFutuKLineStorePrefersFiveMinuteSourceForFifteenMinuteQuery` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/store_test.go:738`<br>`TestIntervalStorageValueCoversSupportedIntervals` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/sync_progress_test.go:10`<br>`TestSyncProgressSnapshotReturnsIndependentCopy` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_cost_replay_boundaries_test.go:15`<br>`TestTradingCostHelperBoundaryRules` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_cost_replay_boundaries_test.go:86`<br>`TestFeeAndReplayPrimitiveBoundaryHelpers` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_cost_replay_boundaries_test.go:150`<br>`TestPineWorkerCommandAndReplayValidationEdges` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_cost_replay_boundaries_test.go:255`<br>`TestPineWorkerAdapterAndPumpFailureContracts` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_costs_test.go:14`<br>`TestResolveBacktestTradingCostsDefaultsByMarketAndInstrument` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_costs_test.go:60`<br>`TestResolveBacktestQuoteCurrencySupportsCNSymbols` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_costs_test.go:75`<br>`TestResolveFeeScheduleClonesAndNormalizesCustomRules` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_costs_test.go:133`<br>`TestFeeRuleEffectiveRangeUsesInclusiveTradeDates` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_costs_test.go:195`<br>`TestBacktestFeeEngineSeparatesBrokerMarketAndAppliesHKRounding` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_costs_test.go:248`<br>`TestBacktestFeeEngineAppliesUSBrokerCapAndSellSideMarketFees` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_costs_test.go:275`<br>`TestBacktestFeeEngineAppliesPerOrderMinimumIncrementally` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:pkg/backtest/trading_costs_test.go:301`<br>`TestScriptCommissionMapsToBrokerFeesOnly` | backtest_calendar | 普通边界 | `crates/jftrade-backtest, crates/jftrade-calendar` |
| [ ] | `go:452dea11:internal/integration/futu/candle_sessions_test.go:11`<br>`TestMarketSessionsForCandleSessions` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_opend_test.go:154`<br>`TestMarketDataRuntimeQueryAndSubscriptionWrappers` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_opend_test.go:226`<br>`TestMarketDataRuntimePreservesRealtimeTicksWhenDelayedFallbackFails` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_opend_test.go:275`<br>`TestTranslateSubscriptionRequiredErrorPreservesBrokerNeutralLeaseDetails` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:27`<br>`TestMarketDataRuntimeCloseWaitsForEnsureAndDoesNotRevive` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:85`<br>`TestMarketDataRuntimeCloseReturnsActiveExchangeFailureIdempotently` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:116`<br>`TestMarketDataRuntimeCloseReturnsInflightExchangeFailure` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:163`<br>`TestMarketDataRuntimeDoesNotPublishExchangeWhenConfigChangesDuringCreate` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:196`<br>`TestMarketDataRuntimeNilAndClosedLifecycleBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:220`<br>`TestTickFromTradeProducesBrokerNeutralPushTick` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:267`<br>`TestTickConversionRejectsUnusablePricesAndUsesQuoteFallbacks` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:318`<br>`TestTickFromTickerPreservesHKPreviousCloseDuringLunchBreak` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:364`<br>`TestTickFromTradeInheritsLatestQuoteFieldsThroughCache` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:432`<br>`TestTickFromTickerReclassifiesUSRegularBoundary` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:452`<br>`TestMarketDataRuntimeExchangeResetAndStreamLifecycle` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:591`<br>`TestMarketDataRuntimeReplacesAnExchangeWhenItsConfigKeyChanges` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:613`<br>`TestMarketDataRuntimeFiltersFallbackInstrumentsFromPushStream` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:655`<br>`TestMarketDataRuntimeUnavailableQueryHelpers` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:712`<br>`TestTickFromSnapshotMapsExtendedQuoteFields` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:824`<br>`TestFallbackTickerMapProjectsOnlyRequestedUsableSnapshots` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:869`<br>`TestFallbackSnapshotConversionRejectsInvalidValuesAndUsesClassification` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:937`<br>`TestMarketDataRuntimeQueriesDelayedSnapshotsAlongsideRealtimeQuotes` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/notifications_test.go:13`<br>`TestLiveNotificationFromResponseRoutesProtocolPayloadsToNeutralCategories` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/notifications_test.go:98`<br>`TestNeutralNotificationBuildersHandleNilAndStatusTransitions` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/notifications_test.go:135`<br>`TestNotificationLabelsCoverEverySupportedProgramAndGatewayState` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/notifications_test.go:207`<br>`TestNotificationAndQuoteRightLabelsRemainStable` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/order_updates_test.go:59`<br>`TestOrderUpdatesAdapterConvertsPushesAndStopsOnce` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/order_updates_test.go:105`<br>`TestOrderUpdatesAdapterRefreshRepeatsAccountPushWithoutReregisteringHandlers` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/order_updates_test.go:137`<br>`TestOrderUpdateSubscriptionNilAndNoAccountPaths` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/order_updates_test.go:159`<br>`TestOrderUpdatesAdapterSubscribeReturnsNoOpOrErrorsCleanly` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/probe_test.go:22`<br>`TestProbeOpenDReportsProtocolOutcomesWithoutARealOpenD` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/probe_test.go:96`<br>`TestProbeOpenDMapsHealthyProtocolFixture` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/probe_test.go:114`<br>`TestProbeOpenDReportsClosedPortAsDisconnected` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/probe_test.go:137`<br>`TestProbeFromGlobalStateEnforcesMinimumVersionAndMapsNeutralState` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/probe_test.go:173`<br>`TestProgramStatusStringHandlesMissingPlainAndDescribedStatus` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/security_details_test.go:12`<br>`TestSecurityDetailsMapPreservesCompleteBrokerNeutralWireShape` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/security_details_test.go:126`<br>`TestSecurityDetailsMapKeepsMissingOptionalAndProductBlocksNull` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/security_details_test.go:160`<br>`TestSecurityRefMapUsesCanonicalIdentity` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:88`<br>`TestSubscriptionReconcilerSharesExactPhysicalSubscriptionsAndDefersFinalRelease` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:160`<br>`TestSubscriptionReconcilerConcurrentReconcileIsIdempotent` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:191`<br>`TestSubscriptionReconcilerReplaysAllDesiredSubscriptionsWhenConnectionGenerationChanges` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:236`<br>`TestSubscriptionReconcilerFailsWhenConnectionKeepsChanging` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:287`<br>`TestSubscriptionReconcilerRetriesFailuresAndCancelsRetryOnReacquire` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:346`<br>`TestSubscriptionReconcilerUsesDelayedFallbackForBasicQuoteAvailabilityFailures` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:394`<br>`TestSubscriptionReconcilerProviderSwitchDefersPhysicalReleaseUntilOpenDEligible` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:459`<br>`TestSubscriptionReconcilerMeasuresRetentionFromOpenDAcknowledgement` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:499`<br>`TestSubscriptionReconcilerMeasuresRetryFromOpenDFailureAcknowledgement` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:548`<br>`TestSubscriptionReconcilerMeasuresQuotaRefreshFromOpenDAcknowledgement` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:587`<br>`TestSubscriptionReconcilerPendingProviderCleanupDropsClosedConnectionOwnership` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:623`<br>`TestSubscriptionReconcilerHandlesQuotaExchangeReplacementResetAndNilBoundaries` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:676`<br>`TestSubscriptionReconcilerDropsFailedRecordsReleasedBeforeRetry` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:697`<br>`TestDesiredPhysicalSubscriptionsRejectsIncompleteRefsAndNormalizesSymbols` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/integration/futu/subscription_reconciler_test.go:713`<br>`TestSubscriptionReconcilerKeepsThreeViewerCapabilitiesAndReleasesOnlyOldOrderBook` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/account_fill_test.go:14`<br>`TestQueryAccountReturnsBBGOAccountSnapshot` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/account_fill_test.go:71`<br>`TestQueryBrokerOrderFillsFiltersAndSortsCurrentSessionFills` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/account_fill_test.go:153`<br>`TestQueryBrokerMaxTradeQuantityRejectsInvalidBusinessInputs` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_protocol_test.go:15`<br>`TestFutuAdvancedAdapterReaderSurfaceAndPredictionSubscriptions` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_protocol_test.go:128`<br>`TestBrokerKLineAdjustmentMapping` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_protocol_test.go:149`<br>`TestFutuAdvancedAdapterProtocolValidationDefaultsAndPayloadHelpers` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_protocol_test.go:296`<br>`TestFutuComboAdapterOptionAndEventLifecycle` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_protocol_test.go:365`<br>`TestFutuComboAdapterProductRulesAndValidationFailures` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_protocol_test.go:471`<br>`TestFutuAdvancedProtocolTransportFailureIsReturned` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_test.go:14`<br>`TestAdvancedFeatureDefaultsBuildStrictOpenDRequests` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_test.go:80`<br>`TestIndustrialChainListPageSizeRespectsOpenDLimit` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_test.go:92`<br>`TestEveryAllowlistedAdvancedProtocolMapsToCatalogFeature` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_test.go:124`<br>`TestAdvancedProtocolReplaySafetyDefaultsToNoReplay` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_test.go:150`<br>`TestEveryDefaultFeatureOperationBuildsGeneratedRequest` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_advanced_test.go:212`<br>`TestHighDividendStateRejectsMisleadingMainlandScope` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_bridge_test.go:19`<br>`TestBrokerAdapterDiscoverAccountsAndTradingBridge` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_bridge_test.go:124`<br>`TestBrokerAdapterQueryMarketRulesUsesSecurityInfoLotSize` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_bridge_test.go:155`<br>`TestBrokerAdapterQueryMarketRulesFallsBackToSecuritySnapshotLotSize` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_bridge_test.go:191`<br>`TestBrokerAdapterMarketDataReaderTradingSnapshots` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_bridge_test.go:465`<br>`TestBrokerAdapterMarketDataReaderAccountAnalytics` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_bridge_test.go:608`<br>`TestBrokerAdapterQuoteKLinesSubscriptionsAndValidation` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_bridge_test.go:709`<br>`TestBrokerAdapterUnlockTradeBridge` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_capabilities_service_test.go:13`<br>`TestFutuCapabilitiesContextLoadsInitialQuoteRightsOnce` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_capability_runtime_test.go:18`<br>`TestFutuCapabilityNotificationsAndRuntimeAggregation` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_capability_runtime_test.go:85`<br>`TestFutuCapabilityConnectionAccountAndEntitlementDecisions` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_capability_runtime_test.go:202`<br>`TestFutuCapabilityQuoteRightProductsAndStates` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_capability_runtime_test.go:265`<br>`TestFutuCapabilityLoadsQuoteRightsOncePerConnection` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_capability_runtime_test.go:313`<br>`TestFutuCapabilityCachesQuoteRightFailuresAndRefreshesAfterReconnect` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_capability_runtime_test.go:366`<br>`TestQuoteRightsFromUserInfoDoesNotInferDetailedRights` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_capability_runtime_test.go:405`<br>`TestFutuCapabilityRejectsStaleNotificationsAndFailureWrites` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_combo_rules_test.go:12`<br>`TestFutuOptionComboHelperBusinessBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_combo_rules_test.go:134`<br>`TestFutuComboIntentValidOptionCalendarAndEventForms` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_combo_transport_test.go:16`<br>`TestFutuOptionComboPreviewLegalityAndTransportFailures` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_combo_transport_test.go:80`<br>`TestFutuOptionComboNonSpreadOpenDLegalityBranches` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_combo_transport_test.go:149`<br>`TestFutuOptionComboPreviewNormalizesAllAccountImpacts` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_combo_transport_test.go:199`<br>`TestFutuComboPlaceValidatedLegAccountAndTransportFailures` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_earnings_calendar_test.go:12`<br>`TestTranslateEarningsCalendarParamsMapsBusinessSemantics` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_earnings_calendar_test.go:51`<br>`TestTranslateEarningsCalendarParamsRejectsUnsupportedMarketConditions` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_earnings_calendar_test.go:61`<br>`TestEarningsCalendarDateChunksLimitEveryOpenDCallToSevenDays` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_earnings_calendar_test.go:79`<br>`TestEarningsCalendarDateChunksSupportsThirtyFiveDayGridAndRejectsLongerThanFortyTwo` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_earnings_calendar_test.go:96`<br>`TestDeduplicateEarningsCalendarEntriesUsesDateAndSecurity` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_earnings_calendar_test.go:110`<br>`TestCollectEarningsCalendarChunksFailsTheWholeRangeWhenOneChunkFails` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_earnings_calendar_test.go:137`<br>`TestCollectEarningsCalendarChunksUsesEveryExactSegmentInOrder` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_earnings_calendar_test.go:164`<br>`TestEarningsCalendarParameterValidationEdges` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_earnings_calendar_test.go:190`<br>`TestEarningsCalendarDateValidationEdges` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_earnings_calendar_test.go:217`<br>`TestDeduplicateEarningsCalendarEntriesFallsBackForAnonymousRows` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_failure_boundaries_test.go:19`<br>`TestBrokerAdapterForwardsUnavailableOpenDErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_failure_boundaries_test.go:64`<br>`TestAdapterSubscriptionParsingErrorsAreReturned` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_failure_boundaries_test.go:77`<br>`TestAdapterAndDecimalConversionBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_failure_boundaries_test.go:115`<br>`TestKLineSessionAndPriceHelperBoundaries` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_failure_boundaries_test.go:165`<br>`TestCanceledContextIsPreservedByUnavailableAdapter` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_kline_pagination_test.go:15`<br>`TestBrokerKLinesReturnLatestPageAndUseExclusiveBeforeCursor` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_kline_pagination_test.go:66`<br>`TestFutuDeclaredCandlePeriodsMapToHistoricalAndRealtimeTypes` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_kline_pagination_test.go:92`<br>`TestBrokerKLineQueryFormatsOpenDWindowInMarketTimeAndReturnsUTC` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_kline_pagination_test.go:121`<br>`TestBrokerKLineCursorPreservesExactSecondWindowAndExcludesBoundaryLocally` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_kline_pagination_test.go:152`<br>`TestNormalizeBrokerKLinePageDeduplicatesSortsAndKeepsLatest` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_kline_pagination_test.go:198`<br>`TestNormalizeBrokerKLineRangeKeepsInclusiveBoundaries` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_kline_pagination_test.go:223`<br>`TestBrokerKLineQueryRejectsCursorAndTimeBoundaryErrors` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_kline_pagination_test.go:284`<br>`TestBrokerKLinePaginationHelpersCoverSessionsBoundsAndListingDates` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_kline_pagination_test.go:342`<br>`TestFutuCandlePeriodCatalogSkipsMissingAndUnmappableIntervals` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_marketdata_search_test.go:12`<br>`TestFutuSearchMarketCodePreservesEveryStableDisplayMarket` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_marketdata_search_test.go:38`<br>`TestCanonicalSearchQuoteSymbolHandlesOpenDPrefixedCodes` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_marketdata_search_test.go:58`<br>`TestBrokerAdapterSecuritySearchMapsCrossMarketOpenDResults` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_marketdata_search_test.go:113`<br>`TestBrokerAdapterSecuritySearchRejectsInvalidQueriesBeforeConnecting` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:14`<br>`TestConvertFundsSnapshotFullMarginFields` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:78`<br>`TestConvertFundsSnapshotNilMarginFields` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:103`<br>`TestConvertFundsSnapshotNilInput` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:112`<br>`TestConvertFundsSnapshotCurrencyBalances` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:140`<br>`TestSecuritiesFromSymbols` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:156`<br>`TestSecuritiesFromSymbolsInvalid` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:163`<br>`TestSecuritiesFromSymbolsEmpty` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:175`<br>`TestSecuritySymbol` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:182`<br>`TestSecuritySymbolNil` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:191`<br>`TestFutuKLTypeFromIntervalStringAll` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:228`<br>`TestFutuKLTypeFromIntervalStringInvalid` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:237`<br>`TestInt64AsFloat64Ptr` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:248`<br>`TestInt64AsFloat64PtrNil` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:257`<br>`TestBrokerFundsSnapshotFromProtoFullMargin` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:344`<br>`TestBrokerFundsSnapshotFromProtoNilFunds` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:358`<br>`TestBrokerFundsSnapshotRoundTripNoMargin` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:431`<br>`TestOrderBookLevelFromPb` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:465`<br>`TestOrderBookLevelFromPbNil` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:475`<br>`TestOrderBookLevelFromPbEmptyDetails` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:492`<br>`TestOrderBookSnapshotFromOpendResult` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:552`<br>`TestOrderBookSnapshotFromOpendResultNil` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_new_methods_test.go:559`<br>`TestOrderBookSnapshotFromOpendResultEmptyResult` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_option_fix_test.go:12`<br>`TestFutuRootAdapterForwardsBatchSnapshotsThroughProtocol3203` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_option_fix_test.go:31`<br>`TestFutuDeclaredCapabilitiesHaveExecutableAdapterInterfaces` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_option_fix_test.go:56`<br>`TestFutuOptionEventRequestsPassStrictOpenDValidation` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_option_fix_test.go:120`<br>`TestFutuZeroDteContractRebuildsBrokerNeutralChainContext` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_option_fix_test.go:168`<br>`TestFutuOptionEventValidationRejectsUnsupportedInputs` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_option_fix_test.go:206`<br>`TestFutuOptionRequestTranslationBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_option_fix_test.go:261`<br>`TestFutuZeroDteContractTranslationRejectsEachInvalidBoundary` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_option_fix_test.go:331`<br>`TestFutuOptionNumericParameterBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_option_fix_test.go:362`<br>`TestFutuZeroDteNormalizationHandlesAbsentAndNestedUnderlying` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_prediction_boundaries_test.go:15`<br>`TestFutuPredictionMilestonesResolveOwningEventBeforeQuery` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_prediction_boundaries_test.go:78`<br>`TestFutuAdvancedPredictionSnapshotSearchFallbackAndQueryValidation` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_prediction_boundaries_test.go:144`<br>`TestFutuPredictionComboQuoteTranslationRejectsEveryInvalidLegShape` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_prediction_normalization_test.go:11`<br>`TestPredictionPayloadNormalizationDoesNotLeakOpenDMarkets` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_prediction_normalization_test.go:42`<br>`TestPredictionComboQuoteTranslationUsesBrokerNeutralLegs` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_prediction_normalization_test.go:78`<br>`TestInternalFutureMarketNormalizesToPublicProductIdentity` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_prediction_stream_test.go:13`<br>`TestFutuPredictionStreamListenersSequencesAndMalformedPushes` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_prediction_stream_test.go:87`<br>`TestFutuPredictionPushHandlerInstallationReplayAndFailure` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_prediction_stream_test.go:139`<br>`TestFutuPredictionNormalizationCatalogPaginationAndIdentity` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_prediction_stream_test.go:239`<br>`TestFutuAdvancedSecurityNormalizationAllPublicMarkets` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_contract_test.go:12`<br>`TestResearchCatalogOperationsBuildStrictOpenDRequests` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_contract_test.go:84`<br>`TestResearchCatalogOperationsRejectMissingOrInvalidParameters` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_contract_test.go:115`<br>`TestResearchProtocolPayloadAddsCanonicalFieldsWithoutDroppingOpenDFields` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_contract_test.go:237`<br>`TestResearchCatalogLocalPaginationKeepsOpenDRequestUnchanged` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_contract_test.go:314`<br>`TestEconomicCalendarPaginationHonorsExplicitHasMoreAndEmptyRows` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_normalization_boundaries_test.go:16`<br>`TestAdvancedResearchDefaultsRejectIncompleteQueries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_normalization_boundaries_test.go:54`<br>`TestAdvancedResearchDefaultsTranslatePublicInputs` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_normalization_boundaries_test.go:96`<br>`TestAdvancedResearchEnumTranslations` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_normalization_boundaries_test.go:178`<br>`TestResearchNormalizationCoversAlternateWireShapes` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_normalization_boundaries_test.go:208`<br>`TestResearchNormalizationCoversProductAndCalendarVariants` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_normalization_boundaries_test.go:284`<br>`TestResearchNumberAcceptsSupportedScalarTypes` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_normalization_boundaries_test.go:306`<br>`TestQuoteRightsRefreshStateEdges` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_research_normalization_boundaries_test.go:368`<br>`TestQuoteRightsFailureAndExchangeGenerationEdges` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_stock_screen_test.go:15`<br>`TestTranslateResearchScreenParamsBuildsStrictStockScreenRequest` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_stock_screen_test.go:105`<br>`TestTranslateResearchScreenParamsValidatesStableKeysAndMarket` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_stock_screen_test.go:142`<br>`TestStockScreenFeatureResultNormalizesIdentityCellsAndOffset` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_stock_screen_test.go:188`<br>`TestNormalizeStockScreenRowPreservesParameterizedInstanceIdentity` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_stock_screen_test.go:228`<br>`TestStockScreenFeatureResultUsesPerRowMainlandIdentityAndFiltersExactMarkets` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_stock_screen_test.go:316`<br>`TestStockScreenMainlandIdentityMustBeAuthoritative` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_stock_screen_test.go:327`<br>`TestResearchScreenQuoteCurrencyUsesSecurityCounterIdentity` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_stock_screen_test.go:367`<br>`TestResearchScreenLimiterAllowsTenPerThirtySeconds` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_stock_screen_test.go:385`<br>`TestResearchScreenRateLimitErrorRoundTrip` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_stock_screen_test.go:395`<br>`TestResearchScreenTranslationEdges` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_stock_screen_test.go:607`<br>`TestStockScreenNormalizationEdges` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_test.go:12`<br>`TestFutuAdapterCompileTimeChecks` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/adapter_test.go:56`<br>`TestBrokerRegistryWithFutu` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:22`<br>`TestFutuAdvancedSpecializedReadersAndCustomizationSuccess` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:150`<br>`TestFutuComboAdapterErrorPropagationBranches` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:216`<br>`TestFutuEventContractStatusBranches` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:235`<br>`TestFutuWarrantsStayHKOnlyAndFuturesRemainDiscoverable` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:262`<br>`TestSecurityDetailsProductIdentityFallbacks` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:336`<br>`TestFutuSnapshotProductExtensionsAndSecurityTypeMapping` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:403`<br>`TestFutuTradeProductRequestAndReadLifecycleBranches` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:475`<br>`TestFutuComboProtocolTransportErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/basicqot_subscription_errors_test.go:12`<br>`TestClassifyBasicQotSubscriptionErrorOnlyMarksAvailabilityFailures` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:19`<br>`TestWithClientReplayPolicyForRecoverableErrors` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:56`<br>`TestExchangeReconnectsClosedReadyClientAndCoversHandlerBoundaries` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:107`<br>`TestReconnectDoesNotDeadlockWithInFlightNotification` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:198`<br>`TestTradeAccountPushNormalizationSubscriptionAndFactoryEnvironment` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:249`<br>`TestOldOpenDVersionFailsSessionInitialization` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:260`<br>`TestInitResponseAndSessionTransportFailures` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:282`<br>`TestReconnectTradePushFailureAndTradeHandlerBinding` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/codec/frame_size_guards_test.go:9`<br>`TestFrameSizeGuardsCoverEncodeAndDecode` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/codec/frame_test.go:8`<br>`TestEncodeDecodeRoundTrip` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/codec/frame_test.go:29`<br>`TestDecodeRejectsCorruptedBody` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/codec/frame_test.go:39`<br>`TestDecodeRejectsBadMagic` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/codec/frame_test.go:49`<br>`TestDecodeRejectsShortFrame` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/codec/frame_test.go:56`<br>`TestDecodeRejectsLengthMismatch` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:18`<br>`TestBalanceMapFromBrokerFundsUsesCurrencyRowsBeforeAccountFallback` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:46`<br>`TestBalanceMapFromBrokerFundsFallsBackToMarketCurrencyAndLockedCash` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:92`<br>`TestBalanceMapFromFundsAndBrokerOrderSortBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:128`<br>`TestBrokerOrderMappingCoversOrderLifecycleEnums` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:179`<br>`TestBrokerOrderTypeAndTimeInForceMappingsCoverTradingVariants` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:216`<br>`TestBrokerOrderMarketAndCurrencyBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:277`<br>`TestExchangeLocalMarketAndOrderBookHandlerBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:326`<br>`TestExchangeInvalidateClientClearsReadyStateAndSubscriptions` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:349`<br>`TestTradeSecurityInfoAndRuntimeMarketAuthorityBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:433`<br>`TestKLineSessionRegistryResolvesExactRecordAndQuoteSamples` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:461`<br>`TestKLineSessionSamplePruningAndWindowFallback` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_business_boundary_test.go:487`<br>`TestMergeStaticInfoIntoSecurityDetailsFillsMissingFieldsWithoutClobberingSnapshot` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:17`<br>`TestQueryTickersBatchesBasicQotRequests` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:50`<br>`TestQueryKLinesSplitsUSHistoricalRequestsBySessionAndMergesResults` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:106`<br>`TestQueryKLinesForSessionsFiltersUSHistoricalRoutes` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:131`<br>`TestHistoricalKLineSessionHelpersFilterAndPlanExplicitSelections` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:150`<br>`TestQueryKLinesForSessionsFiltersCurrentUSBucket` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:178`<br>`TestResolveHistoricalRequestSessionUsesRouteForRTHAndOvernight` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:192`<br>`TestQueryKLinesFallsBackToSessionAllWhenHistoricalRouteUnsupported` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:228`<br>`TestShouldFallbackHistoricalKLineSplitRecognizesChineseSupportedSessionsMessage` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:241`<br>`TestQueryKLinesNormalizesIntradayHistoryLabelToBucketStart` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:269`<br>`TestQueryKLinesKeepsDailyHistoryLabelAsBucketStart` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:291`<br>`TestQueryKLinesFollowsHistoryPaginationAndKeepsLatestLimit` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:326`<br>`TestQueryKLinesAllowsMoreThanEightHistoryPages` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:356`<br>`TestQueryKLinesUsesLargerHistoryPageSizeThanRequestedLimit` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:385`<br>`TestQueryKLinesIncludesCurrentRealtimeBucketFromGetKL` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:428`<br>`TestStreamConnectEmitsBasicQotPushAsBBGOEvents` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_kline_test.go:479`<br>`TestStreamConnectRebuildsClosedCachedOpenDClient` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_mapping_boundaries_test.go:19`<br>`TestFutuKLineIntervalMappingsCoverSupportedAndUnsupportedValues` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_mapping_boundaries_test.go:85`<br>`TestFutuKLineQueryWindowAndPreflightValidation` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_mapping_boundaries_test.go:127`<br>`TestFutuHistoricalKLineSessionFallbacksAndETHClassification` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_mapping_boundaries_test.go:158`<br>`TestFutuTradeEnumMappingsCoverBrokerAndBBGOBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_mapping_boundaries_test.go:237`<br>`TestFutuReadHelperMappingsAndPushSnapshots` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_mapping_boundaries_test.go:316`<br>`TestFutuMarketAndSecuritySymbolBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_mapping_boundaries_test.go:353`<br>`TestFutuOrderBookSubscriptionRequestExtraction` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_orderbook_test.go:20`<br>`TestSubscribeOrderBookRequestConstruction` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_orderbook_test.go:47`<br>`TestIsHKMarket` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_orderbook_test.go:99`<br>`TestSubscriptionRegistryOrderBook` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_orderbook_test.go:129`<br>`TestSubscriptionRegistryOrderBookReset` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_orderbook_test.go:144`<br>`TestSubscriptionRegistryOrderBookEnsure` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_orderbook_test.go:156`<br>`TestSubscriptionRegistryQuoteAndKLineFamiliesAreIndependent` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_orderbook_test.go:178`<br>`TestGroupOrderBookRequestsForPushSplitsHKAndNonHK` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_orderbook_test.go:214`<br>`TestGroupOrderBookRequestsForPushSingleHKBatchNeedsDetail` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_orderbook_test.go:237`<br>`TestEnsureOrderBookPushSubscriptionsSplitsDetailsAndDeduplicates` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_orderbook_test.go:281`<br>`TestOrderBookSubscriptionLifecycleRequiresLeaseAndUnsubscribes` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_orderbook_test.go:363`<br>`TestHandleOrderBookPushEmitsSingleCompleteBookTicker` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_quote_request_boundaries_test.go:16`<br>`TestExchangeAccountPushMarketWarningAndEmptySymbolBoundaries` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_quote_request_boundaries_test.go:42`<br>`TestMaxTradeQuantityInvalidSecurityAfterAccountResolution` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_quote_request_boundaries_test.go:50`<br>`TestBasicQuoteMissingInvalidAndSubscriptionCacheBoundaries` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_quote_request_boundaries_test.go:77`<br>`TestCurrentKLineExtendedErrorBlankAndInvalidRequestBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_quote_request_boundaries_test.go:110`<br>`TestSecuritySnapshotInvalidRowsErrorsAndEmptyOptionMerge` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_read_boundaries_test.go:15`<br>`TestQueryAllKLinesReturnsCompleteSortedHistoricalSeries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_read_boundaries_test.go:53`<br>`TestExchangeReadAPIsPropagateOpenDQuoteAndSnapshotFailures` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_read_boundaries_test.go:76`<br>`TestTradeAccountSelectionAndWriteBoundaryFailures` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_session_test.go:12`<br>`TestSessionFromExtendedBlocksClockGuardsStaleExtendedData` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:23`<br>`TestRegistration` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:36`<br>`TestConstructorFallsBackToDefaultAddress` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:47`<br>`TestQueryMarketsReturnsBootstrapMarket` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:62`<br>`TestEnsureMarketWithContextAppliesBrokerLotSize` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:89`<br>`TestEnsureMarketWithContextFallsBackToSecuritySnapshotLotSize` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:114`<br>`TestEnsureMarketWithContextReturnsInferredMarketWhenStaticInfoUnavailable` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:125`<br>`TestInferMarketUsesMarketProfiles` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:152`<br>`TestFutuSecurityFromSymbolUsesMarketParser` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:198`<br>`TestQueryTickerReusesSingleOpenDConnection` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:237`<br>`TestConnectRejectsOpenDBelowMinimumVersion` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:251`<br>`TestConnectRejectsOpenDBelowMinimumBuild` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:265`<br>`TestDiscoverAccountsReusesSingleOpenDConnection` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:322`<br>`TestQueryAccountBalancesUsesOpenDFundsSnapshot` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:367`<br>`TestQueryOpenOrdersReturnsActiveOrders` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:434`<br>`TestTradeProtocolConstantsMatchOfficialIDs` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:452`<br>`TestQueryBrokerHistoryOrdersReturnsHistoricalOrders` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:514`<br>`TestQueryBrokerHistoryOrderFillsReturnsHistoricalFills` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:561`<br>`TestQueryBrokerOrderFeesReturnsFeeBreakdown` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:609`<br>`TestQueryBrokerMarginRatiosReturnsMarginData` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:654`<br>`TestQueryBrokerMarginRatiosSkipsUnknownStock` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:691`<br>`TestQueryBrokerMarginRatiosUsesCacheWithinTTL` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:729`<br>`TestQueryBrokerCashFlowsReturnsFlowSummary` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:774`<br>`TestQueryBrokerMaxTradeQuantityReturnsSnapshot` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:834`<br>`TestSubmitOrderPlacesViaOpenD` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:885`<br>`TestCancelOrdersUsesModifyOrderCancel` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:927`<br>`TestEnsureSystemNotificationsBindsSystemPushHandler` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_test.go:974`<br>`TestSubscribeTradeAccountPushReplaysOnReconnectedClient` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_trade_price_test.go:13`<br>`TestFutuRequestLocationUsesMainlandMarketFallback` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_trade_price_test.go:24`<br>`TestNormalizeSubmitOrderPriceForUSMarkets` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_trade_price_test.go:39`<br>`TestPriceStepHelpersCoverEdgeCases` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_trade_price_test.go:75`<br>`TestPlaceOrderRequestFromSubmitOrderNormalizesUSPriceAndFlags` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/exchange_trade_push_test.go:9`<br>`TestTradeUpdateHandlersCanUnsubscribe` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/kline_history_subscription_boundaries_test.go:13`<br>`TestKLineHistoryAndSubscriptionBoundaryPaths` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/kline_session_registry_boundaries_test.go:12`<br>`TestKLineSessionRegistryHandlesInvalidStaleAndBoundedRecords` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/kline_session_registry_boundaries_test.go:56`<br>`TestKLineSessionRegistryPruningAndSampleResolutionBoundaries` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/kline_session_registry_boundaries_test.go:114`<br>`TestResolveKLineSessionByClockUsesProvidedSymbolAndKLineFallback` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/live_opend_test.go:18`<br>`TestLiveOpenDProto108Contract` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/live_opend_test.go:128`<br>`TestLiveOpenDHKDualCounterCurrencyResolution` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/live_opend_test.go:168`<br>`TestLiveOpenDQuoteRightDiscoveryDoesNotSubscribe` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/live_opend_test.go:251`<br>`TestLiveOpenDDelayedStockScreenSnapshotsDoNotSubscribe` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/live_opend_test.go:297`<br>`TestLiveOpenDResearchCatalogReadsDoNotSubscribe` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/live_opend_test.go:533`<br>`TestLiveOpenDEarningsCalendarDayWeekMonthSortAndFilter` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/live_opend_test.go:610`<br>`TestLiveOpenDSZ000858DeclaredCandlePeriods` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/live_opend_test.go:684`<br>`TestLiveOpenDOptionBABAReadClosure` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/market_mapping_order_quantity_test.go:14`<br>`TestFutuMarketMappingsCoverEverySupportedQuotePrefix` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/market_mapping_order_quantity_test.go:58`<br>`TestBrokerOrderQuantityValidationAndEmptyCancellationAreSafe` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/marketdata_bridge_success_test.go:14`<br>`TestExchangeSecuritySnapshotMergesStaticInfo` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/marketdata_bridge_success_test.go:82`<br>`TestSecuritySnapshotItemKeepsRawUSPreviousCloseWhenMarketIsClosed` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/marketdata_bridge_success_test.go:111`<br>`TestBrokerAdapterSecurityInfoSnapshotAndOrderBookBridge` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:17`<br>`TestMarketDataReaderSurfacesTransportAndPayloadBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:108`<br>`TestBrokerKLineSessionHelpersNormalizeAndRejectSelections` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:153`<br>`TestMarketRuleFallbacksExplainTheirSourceAndFailures` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:188`<br>`TestMarketDataRuleHelpersRejectIncompleteBrokerPayloads` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/advanced_combo_protocol_test.go:17`<br>`TestAdvancedDispatcherKeysValidationSuccessAndFailures` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/advanced_combo_protocol_test.go:52`<br>`TestCallAdvancedSuccessEnvelopeAndAllErrorBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/advanced_combo_protocol_test.go:105`<br>`TestAdvancedResponseValidationAndPayloadHelpers` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/advanced_combo_protocol_test.go:132`<br>`TestComboTradingClientResponseShapes` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/advanced_test.go:5`<br>`TestAdvancedProtocolsAreUniqueAndRegistered` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/advanced_test.go:24`<br>`TestAdvancedProtocolPredictionIDs` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/advanced_test.go:66`<br>`TestAdvancedC2SFieldInspectionAndStrictValidation` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_test.go:77`<br>`TestCallFailureRecordsRequestCorrelation` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_test.go:104`<br>`TestStartKeepAliveIgnoresNonPositiveIntervalsWithoutConsumingStart` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_test.go:181`<br>`TestCallRoundTrip` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_test.go:206`<br>`TestRequestTimeout` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_test.go:243`<br>`TestKeepAliveFailureClosesClient` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_test.go:280`<br>`TestSubscribeNotifyReceivesSystemPush` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_test.go:405`<br>`TestCallIgnoresMismatchedProtoOnSameSerial` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:69`<br>`TestConnectSurfacesInvalidAddressAndClosedClient` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:107`<br>`TestClientCloseWaitsForReadWorkerAfterClosingTransport` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:143`<br>`TestClientCloseDrainsPendingRequestsAfterBestEffortCloseFailure` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:166`<br>`TestKeepAliveLoopHalvesLongIntervalsAndStopsForClosedClient` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:182`<br>`TestStartKeepAliveRejectsClosedClientWithoutOwningWorker` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:202`<br>`TestSubscribeNotifySkipsNilAndMalformedPush` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:217`<br>`TestCallFrameRejectsMarshalAndOversizedPayloads` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:230`<br>`TestCallFrameReturnsWriteAndCloseFailures` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:263`<br>`TestReadLoopClosesConnectionOnOversizedFrame` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:290`<br>`TestReadLoopDiscardsInvalidFramesAndClosesOnTruncation` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:315`<br>`TestDispatchDropsDuplicateResponseWhenPendingBufferIsFull` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:327`<br>`TestSubscribeQuotesReportsTransportFailure` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:334`<br>`TestProgramStatusLabelIncludesServerDescription` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/market_read_boundaries_test.go:21`<br>`TestSubscribeQuotesEncodesAdvancedMarketDataOptions` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/market_read_boundaries_test.go:77`<br>`TestRequestHistoryKLEncodesOptionalFieldsAndHandlesEmptyResult` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/market_read_boundaries_test.go:129`<br>`TestMarketReadMethodsPropagateOpenDBusinessErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/market_read_boundaries_test.go:192`<br>`TestSecurityInfoMethodsReturnEmptyCollectionsForEmptyOpenDResults` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/market_read_boundaries_test.go:212`<br>`TestGetKLReturnsEmptyResultWhenOpenDOmitsS2C` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/market_read_boundaries_test.go:232`<br>`TestMarketReadMethodsRejectDisconnectedSession` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/market_read_boundaries_test.go:284`<br>`TestMarketPushSubscribersIgnoreMalformedAndUnsuccessfulUpdates` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:272`<br>`TestGetGlobalState` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:313`<br>`TestSubscribeQuotes` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:352`<br>`TestUnsubscribeQuotes` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:391`<br>`TestGetBasicQot` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:419`<br>`TestSubscribeBasicQot` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:466`<br>`TestGetKL` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:499`<br>`TestRequestHistoryKL` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:534`<br>`TestGetStaticInfo` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:578`<br>`TestGetSecuritySnapshot` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:627`<br>`TestUnlockTrade` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:654`<br>`TestLockTrade` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:683`<br>`TestGetBasicQotError` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:700`<br>`TestGetGlobalStateError` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:716`<br>`TestSubscribeQuotesError` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:734`<br>`TestUnlockTradeError` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:752`<br>`TestGetBasicQotEmptyS2C` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:768`<br>`TestGetKLNullS2C` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:791`<br>`TestGetGlobalStateAdvancedFields` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:848`<br>`TestRequestHistoryKLPagination` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:919`<br>`TestGetBasicQotMultipleSecurities` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:954`<br>`TestUnlockTradeWithSecurityFirm` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:986`<br>`TestGetSecuritySnapshotIndex` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:1023`<br>`TestSubscribeQuotesAllOptions` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/new_methods_test.go:1082`<br>`TestSubscribeQuotesUnsubAll` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/orderbook_boundaries_test.go:14`<br>`TestGetOrderBookRejectsDisconnectedSession` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/orderbook_boundaries_test.go:22`<br>`TestParseOrderBookResponseRejectsMalformedTopLevelFields` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/orderbook_boundaries_test.go:50`<br>`TestParseOrderBookResponseRejectsMalformedS2CFields` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/orderbook_boundaries_test.go:85`<br>`TestParseOrderBookResponseSkipsValidUnknownFields` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/orderbook_boundaries_test.go:101`<br>`TestParseOrderBookLevelAcceptsValidRequiredFields` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/orderbook_test.go:13`<br>`TestParseOrderBookResponseCurrentOpenDWireLayout` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/orderbook_test.go:94`<br>`TestParseOrderBookResponseError` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/prediction_push_test.go:14`<br>`TestPredictionPushSubscribersDispatchOnlySuccessfulTypedUpdates` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/prediction_push_test.go:69`<br>`TestPredictionPushSubscribersIgnoreNilHandlers` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/proto_v108_contract_test.go:13`<br>`TestProto108OrderBookFieldNumbers` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/proto_v108_contract_test.go:29`<br>`TestProto108NotifyFieldNumbers` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/protocol_ids_test.go:5`<br>`TestStaticInfoAndKLineUpdateProtocolIDsDoNotOverlap` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/read_boundary_test.go:15`<br>`TestReadHelpersPropagateClosedClientErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/read_boundary_test.go:40`<br>`TestGlobalStateNormalizesMissingPayloadAndProgramStatusLabels` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/search_quote_test.go:14`<br>`TestGetSearchQuoteSendsKeywordAndReturnsCandidates` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/search_quote_test.go:48`<br>`TestGetSearchQuoteValidatesInputAndPreservesOpenDErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/search_quote_test.go:77`<br>`TestGetSearchQuoteNormalizesMissingPayload` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/subscription_info_test.go:13`<br>`TestGetSubInfoUsesReadOnlySubscriptionProtocol` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/subscription_info_test.go:42`<br>`TestGetSubInfoSurfacesOpenDErrorsAndNormalizesMissingPayload` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/system_user_info_test.go:14`<br>`TestGetQuoteRightsRequestsOnlyEntitlementField` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/system_user_info_test.go:49`<br>`TestGetQuoteRightsNormalizesMissingPayloadAndErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_error_boundaries_test.go:26`<br>`TestTradingReadMethodsPropagateOpenDBusinessErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_error_boundaries_test.go:102`<br>`TestTradingReadMethodsRejectDisconnectedSession` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_error_boundaries_test.go:138`<br>`TestHistoryTradingReadsReturnStableEmptyCollections` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_methods_test.go:64`<br>`TestPlaceOrderRequiresRequestAndConnID` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_methods_test.go:83`<br>`TestPlaceOrderAndModifyOrderEncodeTradeWrites` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_methods_test.go:161`<br>`TestModifyOrderReturnsTradeRetTypeErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_methods_test.go:182`<br>`TestHistoryOrderReadersPreserveFiltersAndEmptyResponses` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_methods_test.go:244`<br>`TestSubscribeAccountPushAndTradePushDecoding` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_methods_test.go:334`<br>`TestSubscribeAccountPushPropagatesTradeErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_methods_test.go:351`<br>`TestPlaceOrderUsesPresetPacketIDWhenProvided` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_methods_test.go:381`<br>`TestTradeWriteWrappersSurfaceCallErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_reads_contracts_test.go:25`<br>`TestTradingReadWrappersDecodeBusinessPayloads` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_reads_contracts_test.go:268`<br>`TestTradingReadWrappersReturnStableEmptyValues` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_reads_contracts_test.go:343`<br>`TestSubscribeOrderBookDispatchesSuccessfulPushes` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_write_boundaries_test.go:17`<br>`TestTradeWriteMethodsEnforcePrerequisitesAndDisconnectedState` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_write_boundaries_test.go:58`<br>`TestPlaceOrderPropagatesOpenDBusinessRejection` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_write_boundaries_test.go:82`<br>`TestModifyOrderReturnsStableEmptyResult` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/trading_write_boundaries_test.go:102`<br>`TestTradePushSubscribersIgnoreMalformedAndUnsuccessfulUpdates` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/user_security_test.go:15`<br>`TestGetUserSecurityGroupsEncodesAllAndReturnsCustomAndSystemGroups` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/user_security_test.go:51`<br>`TestGetUserSecuritiesEncodesTrimmedGroupName` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/user_security_test.go:93`<br>`TestUserSecurityMethodsRejectInvalidInputBeforeEncoding` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/user_security_test.go:103`<br>`TestUserSecurityMethodsPropagateBusinessErrorsAndEmptyResults` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/user_security_test.go:125`<br>`TestUserSecurityMethodsNormalizeMissingGroupsAndReportSecurityErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/user_security_test.go:148`<br>`TestUserSecurityProtocolIDs` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/version_test.go:8`<br>`TestFormatVersion` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/opend/version_test.go:17`<br>`TestValidateMinimumVersion` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_adapter_boundaries_test.go:15`<br>`TestQuoteSnapshotUsesCompleteActiveExtendedBlock` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_adapter_boundaries_test.go:46`<br>`TestMarketDataAdapterSkipsNilRowsAndReturnsProtocolFailures` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_query_test.go:12`<br>`TestExchangeQueryQuoteSnapshotUsesBasicQotPayload` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_query_test.go:89`<br>`TestExchangeQueryQuoteSnapshotsPreservesVolumeBeyondFixedpointRange` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_test.go:14`<br>`TestQuoteSnapshotResolvesHighPrecisionVolumeWithoutLosingInt64Precision` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_test.go:49`<br>`TestQuoteSnapshotPreviousClosePriceInClosedSession` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_test.go:93`<br>`TestQuoteSnapshotHolidayRemainsClosedWithStaleExtendedBlocks` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_test.go:133`<br>`TestQuoteSnapshotPreviousClosePriceInAfterHours` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_test.go:164`<br>`TestQuoteSnapshotPreviousClosePriceZeroCurPrice` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_test.go:183`<br>`TestQuoteSnapshotPreviousClosePriceForHKLunchBreak` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_test.go:217`<br>`TestPreviousClosePriceConditionBySessionType` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/quote_snapshot_test.go:258`<br>`TestPreviousClosePriceConditionDoesNotRewriteNonUSUnknownSession` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/read_account_test.go:12`<br>`TestRecoverableOpenDErrClassifiesConnectionFailures` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/read_account_test.go:37`<br>`TestResolveTradeMarketHonorsRequestedAuthorityAndFallbacks` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/read_account_test.go:85`<br>`TestCandidateTradeAccountFromProtoFiltersAndBuildsHeader` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/read_account_test.go:140`<br>`TestBrokerReadQueryNormalizationAndAccountPriority` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/read_account_test.go:167`<br>`TestTrdMarketFromNormalizedCoversSupportedMarkets` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_query_test.go:13`<br>`TestSecuritySnapshotQueriesValidateInputsBeforeOpenDCall` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_query_test.go:39`<br>`TestMergeStaticInfoFillsMissingSecurityMetadataWithoutOverwritingSnapshot` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_query_test.go:109`<br>`TestSecurityReferenceHelpersPreservePartialCanonicalData` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_boundaries_test.go:14`<br>`TestSecurityDetailsFromSnapshotMapsDerivativeAndMarketExtensionData` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_boundaries_test.go:144`<br>`TestSecurityDetailsFromSnapshotPreservesOrdinaryVolumeWhenHighPrecisionIsInvalid` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_boundaries_test.go:172`<br>`TestSecurityDetailsFromSnapshotHandlesMissingBasicAndUnknownEnums` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:17`<br>`TestSecuritySnapshotCoordinatorCachesClonesAndUsesMarketBatches` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:48`<br>`TestSecuritySnapshotCoordinatorCoalescesConcurrentRequests` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:83`<br>`TestSecuritySnapshotCoordinatorEnforcesSlidingBudgetAndDoesNotCacheFailures` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:120`<br>`TestSecuritySnapshotCoordinatorClassifiesRemoteRateLimitAndHonorsCancellation` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:154`<br>`TestSecuritySnapshotCoordinatorRejectsUnavailableAndInvalidInputs` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:189`<br>`TestSecuritySnapshotCoordinatorHandlesEmptyAndUnexpectedResults` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_error_classification_test.go:14`<br>`TestMarketDataReaderClassifiesSymbolScopedSecuritySnapshotProtocolErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_error_classification_test.go:71`<br>`TestSecuritySnapshotTransportAndCanceledErrorsAreNotSymbolScoped` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_products_test.go:12`<br>`TestSecuritySnapshotItemNormalizesOptionMetrics` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_reader_boundaries_test.go:13`<br>`TestSecuritySnapshotReadersHandleDuplicateMissingAndTransportBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/security_snapshot_reader_boundaries_test.go:54`<br>`TestMergeStaticInfoFillsAbsentSnapshotFieldsWithoutOverwritingExistingValues` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/snapshot_fallback_parsing_test.go:14`<br>`TestStockScreenFallbackWireValueHelpers` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/snapshot_fallback_parsing_test.go:93`<br>`TestStockScreenFallbackParsesRowsAndMarketGroups` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/snapshot_fallback_parsing_test.go:146`<br>`TestStockScreenFallbackCoordinatesCopiesAndErrors` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/snapshot_fallback_test.go:18`<br>`TestStockScreenSnapshotParamsUseStrictDelayedQuoteFields` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/snapshot_fallback_test.go:58`<br>`TestFutuStockScreenSnapshotFallbackUsesStaticIDsWithoutSubscription` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/snapshot_fallback_test.go:98`<br>`TestStockScreenSnapshotCoordinatorCachesRowsAndNegativeResults` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/snapshot_fallback_test.go:153`<br>`TestFutuStockScreenSnapshotFallbackReportsScreenErrors` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:22`<br>`TestStreamCloseCancelsAndJoinsOwnedWorkers` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:78`<br>`TestStreamConnectionAndSubscriptionBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:115`<br>`TestStreamPushHandlersRejectInactiveMalformedAndEmptyQuotes` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:143`<br>`TestStreamConvertsCumulativeQuoteVolumeToIncrementalTradeQuantity` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:174`<br>`TestStreamPreservesFractionalCumulativeVolumeDelta` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:188`<br>`TestStreamMarketTradeCarriesDeltaAndCumulativeVolume` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:215`<br>`TestStreamMarketTradePreservesVolumeBeyondLegacyFixedpointRange` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:239`<br>`TestStreamRejectsNegativeSnapshotVolume` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:255`<br>`TestBasicQuotePushSubscriptionErrorsAndIdempotency` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:292`<br>`TestOrderBookStreamConnectionBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:317`<br>`TestStreamReconnectAndClientWatcherExitPaths` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:369`<br>`TestStreamConnectReportsPhysicalSubscriptionFailures` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/subscription_lifecycle_test.go:19`<br>`TestExchangeSubscriptionMethodsArePairedExactAndIdempotent` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/subscription_lifecycle_test.go:80`<br>`TestExchangeSubscriptionCacheUpdatesOnlyAfterOpenDConfirmation` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/subscription_lifecycle_test.go:123`<br>`TestQueryKLinesWithoutLeaseReturnsExplicitErrorBeforeRealtimeRead` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/subscription_lifecycle_test.go:142`<br>`TestBasicQuoteReadRequiresExplicitLeaseAndNeverLeaksRawOpenDError` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/subscription_lifecycle_test.go:162`<br>`TestConnectionGenerationInvalidatesClosedSessionAndItsSubscriptions` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/subscription_lifecycle_test.go:194`<br>`TestExchangeCloseIsTerminalAndPreventsOrphanedReconnect` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/subscription_lifecycle_test.go:215`<br>`TestSubscriptionRequiredErrorFormattingAndNilConnectionGeneration` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/subscription_lifecycle_test.go:236`<br>`TestFailedConnectionDoesNotAdvanceEstablishedSessionGeneration` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/subscription_lifecycle_test.go:246`<br>`TestExchangeQuerySubscriptionQuotaSeparatesOwnAndOtherConnections` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/subscription_lifecycle_test.go:270`<br>`TestSubscriptionMethodsValidateSymbolsAndIntervals` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/time_normalization_test.go:10`<br>`TestFutuQuoteTimeUsesMarketTimezoneForFallback` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/time_normalization_test.go:60`<br>`TestFormatBrokerOrderTimeUsesMarketTimezoneForFallback` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/time_normalization_test.go:79`<br>`TestBrokerOrderSnapshotUsesResolvedMarketForBareCodeFallbackTime` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_account_sorting_boundaries_test.go:10`<br>`TestTradeAccountResolverUsesStableTieBreakersAndSurfacesClientClose` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_account_test.go:15`<br>`TestDiscoverAccountsDeduplicatesAndFallsBackToCardIdentifier` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_account_test.go:72`<br>`TestResolveTradeMarketCoversRequestedAndFallbackBranches` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_account_test.go:118`<br>`TestQueryBrokerOrdersFiltersAndSortsWorkingOrders` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_account_test.go:199`<br>`TestPlaceOrderRequestFromSubmitOrderCoversValidationAndRemarkSemantics` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_bounds_test.go:12`<br>`TestBrokerReadQueriesSurfaceAccountResolutionErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_bounds_test.go:92`<br>`TestQueryBrokerMaxTradeQuantityOptionalRequestFieldsAndSessionValidation` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_helpers_boundary_test.go:14`<br>`TestTradeReadConversionBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_helpers_boundary_test.go:46`<br>`TestTradeReadHelperBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_helpers_boundary_test.go:68`<br>`TestTradeProtoConversionSkipsNilFeeAndInvalidMarginSecurity` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_helpers_boundary_test.go:81`<br>`TestAccountAndPushConversionBoundaries` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_helpers_test.go:10`<br>`TestNormalizeTradeFilterTimeInput` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_helpers_test.go:48`<br>`TestBrokerTradeFilterConditionsNormalizesTimes` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_helpers_test.go:65`<br>`TestFutuRequestTimesUseMarketWallClock` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_margin_ratio_boundaries_test.go:13`<br>`TestMarginRatioRecoveryAndErrorClassificationBoundaries` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_margin_ratio_boundaries_test.go:62`<br>`TestMarginRatioCacheReturnsDefensiveFreshSnapshots` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_margin_ratio_boundaries_test.go:86`<br>`TestBrokerMarginRatioFallsBackToRecentCacheAndSurfacesInputFailures` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_margin_ratio_boundaries_test.go:124`<br>`TestBasicQuoteQueriesHandleEmptyDuplicateAndInvalidRequests` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_margin_ratio_boundaries_test.go:150`<br>`TestMarginRatioUncachedRecoveryAndConversionBoundaries` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_proto_business_test.go:14`<br>`TestBrokerFundsSnapshotFromProtoPreservesCurrencyAndMarketBreakdowns` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_proto_business_test.go:77`<br>`TestBalanceMapUsesFuturesAvailableFundsOverWithdrawalCash` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_proto_business_test.go:103`<br>`TestBrokerReadProtoSnapshotsNormalizePositionMarginAndQuantityDetails` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_proto_business_test.go:182`<br>`TestBrokerReadProtoMapsEventAndOptionComboLifecycle` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/trade_proto_sorting_contracts_test.go:10`<br>`TestBrokerTradeProtoListsFilterAndSortEveryTieBreaker` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/transport_error_propagation_test.go:13`<br>`TestTradeReadMethodsPropagateTargetProtocolDisconnects` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/transport_error_propagation_test.go:69`<br>`TestQuoteKLineAndOrderBookPropagateTargetDisconnects` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/transport_error_propagation_test.go:125`<br>`TestTradeWriteMethodsPropagateAccountAndWriteDisconnects` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/transport_error_propagation_test.go:166`<br>`TestTradeWritesAreNotReplayedWhenResponseIsLost` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/transport_error_propagation_test.go:207`<br>`TestDirectSubscriptionCallsPropagateClosedClientErrors` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/watchlist_reader_boundaries_test.go:14`<br>`TestWatchlistGroupReaderCoversSecondCacheAndFailures` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/watchlist_reader_boundaries_test.go:49`<br>`TestWatchlistMemberReaderCoversSecondCacheAndFailures` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/watchlist_reader_boundaries_test.go:112`<br>`TestWatchlistConversionAndGateRejectInvalidInputs` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/watchlist_reader_integration_test.go:11`<br>`TestBrokerAdapterWatchlistReaderLoadsCachesAndRefreshesOpenDData` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/watchlist_reader_test.go:14`<br>`TestFutuWatchlistReadGateEnforcesTenCallsPerRollingThirtySeconds` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/watchlist_reader_test.go:30`<br>`TestFutuWatchlistReaderCacheUsesTTLAndReturnsCopies` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/watchlist_reader_test.go:72`<br>`TestConvertFutuWatchlistGroupsMarksEveryNormalizedDuplicateAmbiguous` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/watchlist_reader_test.go:89`<br>`TestConvertFutuWatchlistSecuritiesPreservesCanonicalIDAndBrokerAlias` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/watchlist_reader_test.go:113`<br>`TestFutuWatchlistFreshReadBypassesAndReplacesGroupAndMemberCaches` | futu_opend | 高风险 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:pkg/futu/watchlist_reader_test.go:164`<br>`TestFutuWatchlistFreshMemberReadRechecksRemoteAmbiguity` | futu_opend | 普通边界 | `crates/jftrade-integration-futu` |
| [ ] | `go:452dea11:internal/marketdata/broker_candles_test.go:12`<br>`TestBrokerKLineCandlesResponseProjectsStrictPage` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/broker_candles_test.go:51`<br>`TestBrokerKLineCandlesResponseHandlesTerminalAndBoundedPages` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/broker_candles_test.go:77`<br>`TestBrokerKLineCandlesResponseRejectsInvalidProviderRows` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/broker_candles_test.go:109`<br>`TestBrokerKLineHelpersClassifySessionsAndNumbers` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/broker_candles_test.go:137`<br>`TestBrokerKLinePaginationRejectsInvalidBoundedAndPagedMetadata` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/cache_test.go:12`<br>`TestCacheDeduplicatesPromotesAndInherits` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/cache_test.go:76`<br>`TestCacheFreshnessRetentionAndMaximum` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/cache_test.go:103`<br>`TestCacheDoesNotInheritExtendedSessionsAcrossTradingDays` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/cache_test.go:138`<br>`TestCachePromotesUSRegularCloseWhenAfterHoursTradeArrives` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/cache_test.go:185`<br>`TestCacheRetainsNewExtendedQuoteWhenPriceIsUnchanged` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/cache_test.go:210`<br>`TestTickCandlesVolumeWindowAndLimit` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/cache_test.go:241`<br>`TestTickCandlesUsesExplicitVolumeDeltaAcrossTradingDays` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/cache_test.go:261`<br>`TestSerializationPreservesNullExtendedAndStringPrices` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/cache_test.go:301`<br>`TestServiceUsesSingleCacheForSnapshotCandlesAndLatest` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/cache_test.go:337`<br>`TestServiceTickCandleFallsBackToRetainedCache` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/calendar_macro_facade_test.go:66`<br>`TestServiceCalendarMacroRejectsProvidersWithoutCapability` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/calendar_macro_facade_test.go:109`<br>`TestServiceCalendarValidatesDateFormats` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/calendar_macro_facade_test.go:144`<br>`TestServiceMacroIndicatorHistoryValidatesIDAndLimit` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/calendar_macro_facade_test.go:163`<br>`TestServiceCalendarMacroPassesProviderErrorsThrough` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/candle_sessions_test.go:8`<br>`TestParseCandleSessionsNormalizesCSVAndRepeatedValues` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/candle_sessions_test.go:18`<br>`TestParseCandleSessionsRejectsEmptyAndUnknownValues` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/candle_sessions_test.go:26`<br>`TestResolveCandleSessionsDefaultsAndRejectsUnsupportedValues` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/candle_sessions_test.go:44`<br>`TestFilterCandlesBySessionsPreservesUnknownAsRegular` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/candle_sessions_test.go:71`<br>`TestNormalizeInstrumentFallsBackForUnqualifiedInput` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:15`<br>`TestCollectorCloseCancelsBlockingConnectAndPreventsRevival` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:47`<br>`TestCollectorOldGenerationCannotCommitPushOrConnect` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:79`<br>`TestCollectorResetBoundsStreamCloseAndRejectsLateTick` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:114`<br>`TestCollectorCloseBoundsUncooperativeConnectAndKeepsError` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:142`<br>`TestCollectorPollingFallbackDoesNotCallPushHandler` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:161`<br>`TestCollectorSkipsDynamicallyUnavailablePushSource` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:180`<br>`TestCollectorPollsFallbackInstrumentsWithoutAddingThemToPushStream` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:223`<br>`TestCollectorUsesDynamicPollingPolicyAndPreventsOverlap` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:249`<br>`TestCollectorDemandChangeCancelsPreviousProviderPoll` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:279`<br>`TestCollectorResetInvalidatesBlockingQueryResult` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:300`<br>`TestCollectorStreamFailureBacksOff` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:325`<br>`TestCollectorQuoteFailureBacksOff` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/collector_test.go:355`<br>`TestRetryDelaySequence` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/company_research_facade_test.go:59`<br>`TestServiceCompanyResearchRejectsProvidersWithoutCapability` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/company_research_facade_test.go:94`<br>`TestServiceCompanyResearchRequiresMarketAndSymbol` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/company_research_facade_test.go:109`<br>`TestServiceFinancialStatementsValidatesStatementAndDefaultsToIncome` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/company_research_facade_test.go:143`<br>`TestServiceCompanyResearchResolvesChinaAggregateToExchangeLeaf` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/company_research_facade_test.go:156`<br>`TestServiceCompanyResearchPassesProviderErrorsThrough` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/index_constituents_facade_test.go:31`<br>`TestServiceIndexConstituentsRejectsProvidersWithoutCapability` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/index_constituents_facade_test.go:41`<br>`TestServiceIndexConstituentsValidatesLimitAndForwardsArguments` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/index_constituents_facade_test.go:73`<br>`TestServiceIndexConstituentsResolvesChinaAggregateToExchangeLeaf` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:45`<br>`TestMarketSubsetInstrumentResolverKeepsQualifiedExactLookup` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:83`<br>`TestMarketSubsetInstrumentResolverMarksUnsupportedQualifiedMarketUnavailable` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:101`<br>`TestMarketSubsetInstrumentResolverSearchesNamesAndPreservesRelevance` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:136`<br>`TestMarketSubsetInstrumentResolverExactCodeWinsAndCrossMarketCodeStaysAmbiguous` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:158`<br>`TestMarketSubsetInstrumentResolverFiltersCNAndDeduplicates` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:183`<br>`TestMarketSubsetInstrumentResolverNormalizesProviderPrefixedCodes` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:211`<br>`TestMarketSubsetInstrumentResolverUnavailableWhenAllMatchesAreUnsupported` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:229`<br>`TestMarketSubsetInstrumentResolverLimitsAfterRankingWithoutAutoResolvingHiddenMatches` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:248`<br>`TestMarketSubsetInstrumentResolverCachesAndCoalescesNormalizedKeyword` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:314`<br>`TestMarketSubsetInstrumentResolverResetSeparatesProviderGenerations` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:365`<br>`TestMarketSubsetInstrumentResolverRechecksCacheInsideSingleflightWork` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:397`<br>`TestMarketSubsetInstrumentResolverDoesNotCacheSearchErrors` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:417`<br>`TestMarketSubsetInstrumentResolverValidatesInput` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:437`<br>`TestMarketSubsetInstrumentResolverPropagatesContextCancellation` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/instrument_resolver_test.go:466`<br>`TestClassifyInstrumentResolutionKeepsMultiplePartialCandidatesAmbiguous` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/lifecycle_boundaries_test.go:13`<br>`TestCacheRemainingLifecycleBoundaries` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/lifecycle_boundaries_test.go:60`<br>`TestSubscriptionRegistryRemainingLifecycleBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/lifecycle_boundaries_test.go:95`<br>`TestNormalizeInstrumentIDRejectsIncompleteValues` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/lifecycle_boundaries_test.go:101`<br>`TestServiceRemainingLifecycleBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/lifecycle_boundaries_test.go:233`<br>`TestServiceFinalSubscriptionCleanupAlwaysHasDeadline` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/lifecycle_boundaries_test.go:271`<br>`TestCollectorAdvancesInactiveSubscriptionCleanupAfterActiveDemand` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/lifecycle_boundaries_test.go:302`<br>`TestCollectorRemainingLifecycleBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/lifecycle_boundaries_test.go:416`<br>`TestInstrumentResolverRemainingLifecycleBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/news_facade_test.go:42`<br>`TestServiceNewsAndCorporateActionsRejectProvidersWithoutCapability` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/news_facade_test.go:57`<br>`TestServiceNewsValidatesLimitAndForwardsNormalizedArguments` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/news_facade_test.go:84`<br>`TestServiceNewsResolvesChinaAggregateToExchangeLeaf` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/news_facade_test.go:97`<br>`TestServiceCorporateActionsValidatesRangeAndForwardsArguments` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_boundaries_test.go:37`<br>`TestProviderSwitchHelpersHandleNilAndResetBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_boundaries_test.go:55`<br>`TestProviderSwitchRetainsOnlyCurrentGenerationTickCandles` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_boundaries_test.go:81`<br>`TestPollOnlyProviderHealthUsesPollingModes` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_boundaries_test.go:106`<br>`TestPollOnlyProviderReadsDoNotRequireLogicalLease` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_boundaries_test.go:123`<br>`TestProviderSwitchCacheUtilitiesCoverConcreteValuesAndCNRejection` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_boundaries_test.go:141`<br>`TestCollectorCloseHelpersRejectElapsedDeadlines` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_lifecycle_test.go:63`<br>`TestProviderChangeAndManagedLeaseAreMutuallyExclusive` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_lifecycle_test.go:98`<br>`TestConcurrentManagedLeaseWinsBeforeProviderChange` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_lifecycle_test.go:143`<br>`TestConcurrentProviderChangeWinsBeforeManagedLease` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_lifecycle_test.go:184`<br>`TestProviderChangeInvalidatesCacheBeforeUnblockingReaders` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_lifecycle_test.go:224`<br>`TestProviderChangeBlocksReadsDuringActivation` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_lifecycle_test.go:268`<br>`TestProviderChangeWaitsForInflightReadThenClearsItsCache` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/provider_switch_lifecycle_test.go:312`<br>`TestFailedProviderChangeKeepsOldCollectorAndCache` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/quote_availability_test.go:9`<br>`TestSnapshotSerializationPreservesAuthoritativeMissingQuoteFields` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/quote_availability_test.go:35`<br>`TestSnapshotSerializationKeepsLegacyZeroValuesAvailable` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/rankings_facade_test.go:54`<br>`TestServiceRankingsRejectsProvidersWithoutCapability` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/rankings_facade_test.go:72`<br>`TestServiceRankingsValidatesKindAndLimit` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/rankings_facade_test.go:91`<br>`TestServiceRankingsForwardsNormalizedKindAndDefaultLimit` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/rankings_facade_test.go:121`<br>`TestServiceIndustriesRejectsNonCNMarkets` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/rankings_facade_test.go:139`<br>`TestServiceIndustriesForwardsKindAndMembersArguments` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/rankings_facade_test.go:184`<br>`TestServiceIndustriesDefaultsEmptyKindToIndustry` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/screen_facade_test.go:23`<br>`TestServiceScreenRejectsProvidersWithoutCapability` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/screen_facade_test.go:31`<br>`TestServiceScreenValidatesRequestAndForwards` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/screen_facade_test.go:78`<br>`TestServiceScreenPassesProviderErrorsThrough` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/service_facade_test.go:11`<br>`TestServiceDelegatesProviderFacadeAndRefreshBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/service_facade_test.go:69`<br>`TestServiceSnapshotErrorsAreBusinessVisible` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/service_facade_test.go:83`<br>`TestServiceSnapshotResolvesChinaAggregateToExchangeLeaf` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/service_facade_test.go:105`<br>`TestServiceProviderReadsResolveChinaAggregateToExchangeLeaf` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/service_facade_test.go:136`<br>`TestServiceRejectsSnapshotCompletedAfterProviderChange` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/service_facade_test.go:162`<br>`TestServiceTickCandlesProviderAndFallbackBoundaries` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/service_facade_test.go:213`<br>`TestLimitCandleMapsKeepsLatestEntriesOnly` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/service_facade_test.go:237`<br>`TestServiceSubscriptionFacadeCacheHelpersAndLifecycle` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/service_facade_test.go:320`<br>`TestServiceHealthAndSerializationNilBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/service_facade_test.go:335`<br>`TestServiceProviderStatusCombinesDescriptorHealthAndDemand` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:51`<br>`TestSubscriptionRegistryExpiresOnlyWebConsumersAndPreservesManagedLeases` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:83`<br>`TestSubscriptionRegistryConcurrentAcquireHeartbeatAndRelease` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:128`<br>`TestManagedSubscriptionConcurrentReleaseIsIdempotent` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:149`<br>`TestServiceReconcilesWebAndManagedSubscriptionLifecycles` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:199`<br>`TestServiceRollsBackFailedAcquireAndHandlesDeferredReleaseFailures` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:237`<br>`TestSubscriptionValidationAndSnapshotDecorationBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:303`<br>`TestFailedAcquireRestoresOnlyTheAttemptedConsumerState` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:338`<br>`TestManagedLeaseReleaseRestoresPreexistingConsumerOwnership` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:357`<br>`TestSubscriptionRequiredErrorsAndManagedReadDemandBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:446`<br>`TestPollOnlyProviderPreservesLogicalLeaseAndRejectsUnsupportedReadsFirst` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:469`<br>`TestServiceMergesAdditionalDemandIntoExactReconciliation` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscription_lifecycle_test.go:498`<br>`TestServiceCloseLeavesPhysicalSubscriptionsEmpty` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscriptions_test.go:10`<br>`TestSubscriptionRegistryContract` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscriptions_test.go:51`<br>`TestSubscriptionRegistrySeparatesChannelAndInterval` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscriptions_test.go:82`<br>`TestSubscriptionRegistryClearAllAndActiveInstruments` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdata/subscriptions_test.go:100`<br>`TestServiceOwnsSubscriptionsAndHealthMode` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:15`<br>`TestBinaryNameForUsesGoPlatformNames` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:30`<br>`TestSelectFromFSReturnsPlatformOnedirAssetMetadata` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:58`<br>`TestSelectFromFSTreatsMissingAndEmptyAssetsAsUnavailable` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:80`<br>`TestSelectFromFSReturnsUnexpectedReadError` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:91`<br>`TestSelectFromFSRejectsInvalidPlatformBundleShapes` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:126`<br>`TestReadAssetFilesRejectsUnsafeOrUnreadableEntries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:151`<br>`TestMaterializeAssetUsesPrivateDirectoryAndCleansUp` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:192`<br>`TestMaterializeAssetRejectsEmptyAsset` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:204`<br>`TestMaterializeAssetRejectsDigestChanges` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:218`<br>`TestMaterializeAssetRejectsInvalidBundlePath` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:231`<br>`TestMaterializeAssetRequiresUsableExecutableAndWritableTempRoot` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:271`<br>`TestAssetPathAndDigestHelpersRejectDuplicateOrEscapingPaths` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:287`<br>`TestCleanupHandlesNilAndAlreadyCleanedAssets` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:298`<br>`TestDigestMaterializedFilesReportsOpenError` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/asset_selection_boundaries_test.go:315`<br>`TestIsMissingAssetRecognizesOnlyNotFoundErrors` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/assets_dev_test.go:14`<br>`TestSelectReturnsUnavailableWithoutReleaseAssets` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/assets_dev_test.go:34`<br>`TestReleaseReturnsUnavailableWithoutReleaseAssets` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/assets_dev_test.go:44`<br>`TestDevelopmentCacheWrappersRemainSafeWithoutEmbeddedAssets` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/assets_release_test.go:14`<br>`TestSelectReturnsStagedPlatformAssetWhenPresent` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/assets_release_test.go:49`<br>`TestStagedDarwinBundleEmbedsPythonLoaderAsRegularFile` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/assets_release_test.go:72`<br>`TestMaterializeCachedReleaseAssetReusesBundleAndFallsBack` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/assets_test.go:8`<br>`TestBinaryNameUsesCurrentRuntime` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:15`<br>`TestMaterializeCachedAssetReusesVerifiedContent` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:45`<br>`TestMaterializeCachedAssetRepairsTamperAndSymlink` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:83`<br>`TestMaterializeCachedAssetPublishesConcurrently` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:118`<br>`TestPublishCachedAssetAcceptsOnlyAValidConcurrentWinner` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:159`<br>`TestMaterializeCachedAssetPrunesOnlyExpiredDigests` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:185`<br>`TestPruneCachedAssetsIgnoresMissingCacheRoot` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:193`<br>`TestMaterializeCachedAssetRejectsUnsafeCacheRoot` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:207`<br>`TestEnsurePrivateCacheRootReportsUninspectablePath` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:218`<br>`TestRemoveInvalidCacheTargetReportsInspectionFailure` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:229`<br>`TestRemoveInvalidCacheTargetReportsRemovalFailure` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:249`<br>`TestMaterializeCachedAssetRejectsInvalidAssetAndRootInputs` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:310`<br>`TestWriteAssetFilesReportsInvalidPathsAndFilesystemConflicts` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:336`<br>`TestValidateCachedAssetRejectsUnsafeShapes` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:internal/marketdataassets/cache_test.go:407`<br>`TestValidateCachedAssetRejectsSocket` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/builtin_test.go:8`<br>`TestBuiltinResolverUSHolidayAndEarlyClose` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/builtin_test.go:38`<br>`TestBuiltinResolverHKWeekdayFallbackUsesTemplateSessions` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/builtin_test.go:54`<br>`TestBuiltinResolverMainlandHolidayFallbackClosesKnownHoliday` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/builtin_test.go:73`<br>`TestBuiltinResolverMainlandWeekdayFallbackStillOpensRegularDay` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/builtin_test.go:92`<br>`TestBuiltinResolverMainlandAliasesShareHolidayFallback` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/calendar_boundaries_test.go:8`<br>`TestBuiltinCalendarBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/calendar_boundaries_test.go:34`<br>`TestCalendarHelperBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/helpers_boundaries_test.go:8`<br>`TestSnapshotHelpersRespectMarketDateAndCoverageWindow` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/helpers_boundaries_test.go:52`<br>`TestTradingDaySessionHelpersReflectBusinessState` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/helpers_boundaries_test.go:87`<br>`TestBuiltinUSChristmasEveEarlyCloseAndTemplateCopies` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/helpers_boundaries_test.go:136`<br>`TestBuiltinResolverCalendarBoundaryFallbacks` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/helpers_boundaries_test.go:167`<br>`TestCalendarSessionNormalizationAndLocationFallbacks` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/calendar/types_json_test.go:10`<br>`TestTradingDayScheduleJSONOmitsZeroUpdatedAt` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/hk/hk_test.go:8`<br>`TestHKProfileUsesHongKongTimezoneAndSplitSessions` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/hk/hk_test.go:20`<br>`TestLoadLocationFallsBackToUTC` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/instrument_session_validation_test.go:8`<br>`TestInstrumentAndSessionValidationContracts` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/instrument_session_validation_test.go:31`<br>`TestTimeBoundaryValidationRules` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_normalization_test.go:32`<br>`TestMarketInputMatchingAndCalendarResolverLifecycle` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_normalization_test.go:83`<br>`TestTradingMinuteHelpersAndExtendedSessionFlags` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_normalization_test.go:108`<br>`TestCalendarAndTradingPeriodLabels` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_normalization_test.go:140`<br>`TestParseInstrumentValidationAndProfileFormatting` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_normalization_test.go:161`<br>`TestMarketResolverAndSessionBoundaryFallbacks` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_normalization_test.go:191`<br>`TestUSAfterHoursWindowsAndPeriodLabelBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_normalization_test.go:248`<br>`TestNormalizeMarketInputAliases` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_normalization_test.go:264`<br>`TestCustomCalendarSessionWindowsAndClosedDays` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_normalization_test.go:302`<br>`TestTradingDayBoundaryUsesCalendarOvernightCarryStart` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_normalization_test.go:330`<br>`TestSessionWindowBoundsRejectsMissingSessionWindows` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_normalization_test.go:382`<br>`TestNonUSExtendedRequestsUseRegularWindowsAndLabelValidation` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_test.go:11`<br>`TestClassifySessionForUS` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_test.go:38`<br>`TestIsRegularTradingTimeUsesHolidayCalendarForUS` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_test.go:51`<br>`TestRegularTradingTimeForHKAndChinaLunchBreaks` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_test.go:78`<br>`TestNormalizeMarketInputAndParseInstrument` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_test.go:115`<br>`TestMarketDescriptorsExposeFrontendMetadata` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_test.go:142`<br>`TestUserMarketDescriptorsKeepChinaExchangesAsCNSubset` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_test.go:174`<br>`TestShouldUseRegularCloseAsPreviousClose` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_test.go:193`<br>`TestTradingPeriodKeys` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_test.go:225`<br>`TestTradingPeriodLabelStart` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_test.go:239`<br>`TestTradingDayBoundaryStart` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/market_test.go:260`<br>`TestSessionAwareIntradayBucketBounds` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/session_boundaries_test.go:10`<br>`TestMarketSessionBoundaryGuardClauses` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/session_boundaries_test.go:51`<br>`TestMarketSessionBucketClampsAtSessionEnd` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/session_boundaries_test.go:67`<br>`TestMarketSundayOvernightCarryWindow` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/session_boundaries_test.go:102`<br>`TestMarketLabelAndResolverFailureBoundaries` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/session_calendar_refresh_contract_test.go:36`<br>`TestCoverage98ExtendedWindowFailsClosedWhenCalendarRefreshRemovesClassifiedWindow` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/session_window_test.go:8`<br>`TestResolveSessionWindowUsesTradingCalendarBoundaries` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/session_window_test.go:50`<br>`TestResolveSessionWindowRejectsClosedAndResolvesOvernightTradingDate` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/session_window_test.go:80`<br>`TestResolveTradingDaySessionWindowCoversNamedSessionsAndInvalidInputs` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/session_window_test.go:126`<br>`TestResolveSessionWindowDoesNotInventMissingCalendarData` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/session_window_test.go:142`<br>`TestResolveTradingDaySessionWindowTracksDaylightSavingTime` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/sh/sh_test.go:8`<br>`TestSHProfileUsesChinaMarketAndTimezone` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/sh/sh_test.go:20`<br>`TestLoadLocationFallsBackToUTC` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/sz/sz_test.go:8`<br>`TestSZProfileUsesChinaMarketAndTimezone` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/sz/sz_test.go:20`<br>`TestLoadLocationFallsBackToUTC` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/us/us_test.go:8`<br>`TestUSProfileUsesNewYorkTimezoneAndRegularSession` | marketdata_quotes | 高风险 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/us/us_test.go:17`<br>`TestUSTradingDayAndEarlyCloseRules` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:pkg/market/us/us_test.go:56`<br>`TestUSTradingCalendarEdgeBoundaries` | marketdata_quotes | 普通边界 | `crates/jftrade-marketdata, crates/jftrade-integration-marketdata-helper` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:42`<br>`TestParseChangedGoLinesCollectsAddedAndModifiedHunks` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:64`<br>`TestRepoRelativeProfilePathKeepsNestedInternalUnderPackageRoot` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:71`<br>`TestParseChangedGoLinesReportsPureRenameWithoutInventingChangedStatements` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:82`<br>`TestParseChangedGoLinesIgnoresDeletedFiles` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:96`<br>`TestParseChangedGoLinesRejectsMalformedHunk` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:102`<br>`TestParseGitDiffPathSupportsQuotedPathsAndRejectsTraversal` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:112`<br>`TestParseGitListedPathPreservesLeadingDiffPrefixDirectories` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:122`<br>`TestChangedGoLinesForRefBuildsWorkingTreeDiffCommand` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:142`<br>`TestChangedGoLinesForRefIncludesGitDiagnostics` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:150`<br>`TestAnalyzeDiffCoverageSeparatesOrdinaryAndCriticalPackages` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:203`<br>`TestAnalyzeDiffCoverageExcludesOnlyFutuTestSupportPackage` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:242`<br>`TestAnalyzeDiffCoverageExcludesFutuTestSupportWithoutProfile` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:275`<br>`TestAnalyzeDiffCoverageFailsExecutableSourceWithoutProfile` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:313`<br>`TestAnalyzeDiffCoverageAllowsTypeCommentImportAndTestOnlyChangesWithoutProfile` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:326`<br>`TestRun` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:355`<br>`TestAnalyzeDiffCoverageIncludesUntrackedExecutableGoSource` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:372`<br>`TestUntrackedGoFilesIncludesGitDiagnostics` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:380`<br>`TestEvaluateDiffCoverageGatesEachCriticalPackageSeparately` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:393`<br>`TestPrintDiffCoverageReportHandlesNoExecutableStatementsAndCriticalScopes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/main_test.go:17`<br>`TestParseConfigDefaults` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/main_test.go:37`<br>`TestParseConfigExplicitValues` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/main_test.go:61`<br>`TestParseConfigRejectsInvalidValues` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/main_test.go:87`<br>`TestParseConfigHelp` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/main_test.go:95`<br>`TestRunCLIUsesDefaultDependenciesForHelp` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/main_test.go:101`<br>`TestRunCLIWithStopsBeforeExecutionForHelpAndInvalidConfiguration` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/main_test.go:116`<br>`TestRunCLIWithReportsWorkingDirectoryAndExecutionFailures` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/main_test.go:151`<br>`TestRunCLIWithReportsViolationsAndSuccess` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/main_test.go:188`<br>`TestMainDelegatesToCLIAndProcessExit` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:14`<br>`TestAnalyzeProfilesAppliesScopesAndExclusions` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:46`<br>`TestAnalyzeProfilesNormalizesWindowsSeparators` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:59`<br>`TestAnalyzeProfilesExcludesOnlyFutuTestSupportPackage` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:85`<br>`TestCoverageStatsPercentageAndPackageScopeBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:109`<br>`TestAnalyzeProfilesRejectsEmptyBusinessCoverage` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:120`<br>`TestAnalyzeProfilesRetainsRequiredCriticalScopesWithoutProfileData` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:137`<br>`TestExclusionRulesAreExplicitAndDoNotHideBackendEntrypoints` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:173`<br>`TestPackageScopeIncludesAPICommand` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:179`<br>`TestEvaluateCoverageAggregatesViolations` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:202`<br>`TestEvaluateCoverageAllowsExactThresholds` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:219`<br>`TestEvaluateCoverageUsesSingleOrdinaryThreshold` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:238`<br>`TestCriticalDomainForScopeUsesRiskPrefixesAndPackageBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:274`<br>`TestEvaluateCoverageGatesEachCriticalPackageSeparately` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:291`<br>`TestPrintCoverageReportIncludesMissingAndSortedScopes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:313`<br>`TestPrintCoverageReportReturnsWriterErrorsForEachSection` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_merge_test.go:13`<br>`TestMergeCoverageProfileKeepsOneBlockWithAnyObservedExecution` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/profile_merge_test.go:30`<br>`TestMergeCoverageProfileRejectsMalformedInput` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/runner_test.go:46`<br>`TestExecGoRunnerRunsGoCommandInRequestedDirectory` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/runner_test.go:53`<br>`TestExecuteCoverageCheckBuildsCommandAndCleansProfile` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/runner_test.go:82`<br>`TestExecuteCoverageCheckReturnsGoTestErrorAndCleansProfile` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/runner_test.go:97`<br>`TestExecuteCoverageCheckReportsProfileAndOutputFailures` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/runner_test.go:131`<br>`TestExecuteCoverageCheckReportsTemporaryProfileCreationFailure` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/runner_test.go:142`<br>`TestExecuteCoverageCheckWithGitReportsAndEnforcesChangedCodeCoverage` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/runner_test.go:174`<br>`TestExecuteCoverageCheckWithGitReturnsDiffFailures` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/check-go-coverage/runner_test.go:188`<br>`TestFindRepoRootRejectsDirectoryWithoutGoMod` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/generator_test.go:18`<br>`TestGenerateFutuProtoReplacesOutputsAfterSuccess` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/generator_test.go:46`<br>`TestGenerateFutuProtoPreservesOutputsOnProtocFailure` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/generator_test.go:60`<br>`TestGenerateFutuProtoRejectsChecksumBeforeCommands` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/main_test.go:14`<br>`TestParseCLIConfig` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/main_test.go:29`<br>`TestParseCLIConfigRejectsInvalidArguments` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/main_test.go:37`<br>`TestParseCLIConfigHelp` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/main_test.go:44`<br>`TestRunCLIReturnsParameterExitCode` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/manifest_test.go:15`<br>`TestParseChecksumManifest` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/manifest_test.go:25`<br>`TestManifestFileNamesReturnsSortedInputs` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/manifest_test.go:38`<br>`TestParseChecksumManifestRejectsInvalidEntries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/manifest_test.go:60`<br>`TestValidateManifestFilesReportsAllDifferences` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/manifest_test.go:70`<br>`TestVerifyFutuInputsRejectsMissingAndMismatchedFiles` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/repository_verify_test.go:12`<br>`TestRepositoryDigestDetectsGeneratedOutputDrift` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/repository_verify_test.go:41`<br>`TestRepositoryDigestRejectsUnexpectedFilesAndProtoNames` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/repository_verify_test.go:59`<br>`TestParseRepositoryDigestValidation` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/rewrite_test.go:12`<br>`TestRewriteGoPackageReplacesOrInsertsOption` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/rewrite_test.go:35`<br>`TestRewriteGoPackageRejectsMissingPackage` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-futu-proto/rewrite_test.go:42`<br>`TestOrganizeGeneratedFilesUsesPackageDirectories` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/generator_test.go:17`<br>`TestGeneratePineworkerProtoReplacesOutputAfterSuccess` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/generator_test.go:40`<br>`TestGeneratePineworkerProtoPropagatesExitAndPreservesOutput` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/generator_test.go:54`<br>`TestGeneratePineworkerProtoPreservesOutputOnLineLimit` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/generator_test.go:70`<br>`TestGeneratePineworkerProtoPreservesOutputOnCollision` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/generator_test.go:87`<br>`TestGeneratePineworkerProtoChecksInputsBeforeCommands` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/main_test.go:14`<br>`TestParseCLIConfig` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/main_test.go:25`<br>`TestParseCLIConfigRejectsInvalidArguments` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/main_test.go:33`<br>`TestParseCLIConfigHelp` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/main_test.go:40`<br>`TestRunCLIReturnsParameterExitCode` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/output_test.go:12`<br>`TestFlattenGeneratedFiles` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/output_test.go:26`<br>`TestFlattenGeneratedFilesRejectsNameCollisions` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/generate-pineworker-proto/output_test.go:41`<br>`TestEnforceGeneratedLineLimitMatchesWcSemantics` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/internal/protogen/files_test.go:13`<br>`TestCopyFileCopiesContent` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/internal/protogen/files_test.go:24`<br>`TestReplaceDirectoriesReplacesAllTargets` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/internal/protogen/files_test.go:39`<br>`TestReplaceDirectoriesRollsBackAllTargets` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/internal/protogen/files_test.go:55`<br>`TestExitCodeUsesWrappedExitCoder` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/internal/protogen/repository_test.go:12`<br>`TestFindRepoRoot` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/internal/protogen/repository_test.go:23`<br>`TestFindRepoRootRejectsMissingModule` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/internal/protogen/tools_test.go:15`<br>`TestPrepareToolchainInstallsMissingPlugins` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/internal/protogen/tools_test.go:65`<br>`TestPrepareToolchainRejectsWrongProtoc` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/internal/protogen/tools_test.go:74`<br>`TestEnvironmentHelpersAreCaseInsensitive` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_profile_dev_test.go:10`<br>`TestDevelopmentDesktopBuildProfile` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_profile_release_test.go:11`<br>`TestReleaseDesktopBuildProfile` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_startup_test.go:15`<br>`TestDesktopStartupWindowStatePrecedesAPIStartup` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_startup_test.go:29`<br>`TestDesktopStartupPublishesReadyAndClosesResources` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_startup_test.go:67`<br>`TestDesktopStartupFailureUsesSafeMessage` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_startup_test.go:93`<br>`TestDesktopShutdownCancelsStartupAndReclaimsLateResources` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_startup_test.go:124`<br>`TestDesktopShutdownIsIdempotent` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_updates_test.go:9`<br>`TestDesktopUpdateServiceSelectsLatestStableDesktopRelease` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_updates_test.go:33`<br>`TestDesktopUpdateServiceDisabledForDevelopment` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_window_state_test.go:12`<br>`TestDesktopWindowStatePathIsReleaseOnly` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_window_state_test.go:22`<br>`TestDesktopWindowStateRoundTrip` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_window_state_test.go:46`<br>`TestApplyDesktopWindowState` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/desktop_window_state_test.go:60`<br>`TestEnsureDesktopWindowVisibleMovesOffscreenWindow` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:19`<br>`TestMainWindowOptionsUseWebZoom` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:39`<br>`TestDesktopSingleInstanceOptionsAreChannelScoped` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:52`<br>`TestDesktopBuildChannelsCanCoexist` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:66`<br>`TestDesktopRuntimeConfigDisablesAuth` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:93`<br>`TestDesktopAssetHandlerOverridesRuntimeConfig` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:110`<br>`TestDesktopAssetHandlerServesIndexForSPARoute` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:131`<br>`TestDesktopAssetHandlerDoesNotFallbackForMissingStaticAsset` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:149`<br>`TestDesktopTrayMenuLabels` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:166`<br>`TestDesktopLogWindowOptionsUseVueRoute` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:179`<br>`TestDesktopAssetHandlerServesLogViewer` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:195`<br>`TestDesktopAssetHandlerFallsBackForUnknownClientRoute` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:217`<br>`TestShouldUseExplicitTrayMenuClick` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:236`<br>`TestNormalizeDesktopDocsURL` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:261`<br>`TestNormalizeDesktopDocsURLRejectsUnsafePaths` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:271`<br>`TestShouldQuitDesktopAppOnlyAllowsExplicitTrayQuit` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:317`<br>`TestSanitizeDesktopExternalURL` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:335`<br>`TestDesktopLogManagerWritesOriginalAndRotatesByDay` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:375`<br>`TestDesktopLogLevelParsing` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:395`<br>`TestListDesktopLogDaysAndReadsFilteredPage` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:426`<br>`TestDesktopLogPageCapsLimitAndPaginatesAllLines` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:464`<br>`TestDesktopLogPageTailOffsetReturnsLastPageInFileOrder` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:506`<br>`TestDesktopLogPageTailOffsetAppliesFiltersBeforePaging` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:523`<br>`TestListDesktopLogDaysMissingDirReturnsEmpty` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:cmd/jftrade-desktop/main_test.go:533`<br>`TestDesktopOpenFolderCommand` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/buildinfo/buildinfo_test.go:8`<br>`TestSnapshotTrimsBuildMetadataAndDefaultsBuildTime` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/datamanagement/maintenance_test.go:9`<br>`TestMaintenanceRegistryDispatchesOnlyDeclaredCapabilities` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/datamanagement/maintenance_test.go:33`<br>`TestMaintenanceRegistryFailsClosedForMissingCapabilities` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/datamanagement/maintenance_test.go:46`<br>`TestBusyCheckersReturnTheFirstOwnedActivityReason` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/datamanagement/service_test.go:44`<br>`TestServiceFallbacks` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/datamanagement/service_test.go:71`<br>`TestServiceDelegatesTypedRequests` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/datamanagement/service_test.go:101`<br>`TestServicePreservesBackendErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/desktop/notification_policy_test.go:10`<br>`TestShouldForwardSystemNotification` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/desktop/notification_policy_test.go:35`<br>`TestNotificationMetadata` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/desktop/runtime_path_matching_test.go:5`<br>`TestProductDataDirReportsMissingHome` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/desktop/runtime_path_matching_test.go:13`<br>`TestMatchesAnyCoverageForBlankAndNormalizedValues` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/desktop/runtime_path_test.go:8`<br>`TestProductDataDirByPlatform` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/desktop/runtime_path_test.go:41`<br>`TestProductDataDirUsesCurrentPlatform` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:30`<br>`TestHTTPCalendarSourceFetchPreservesDistinctTransportAndParsingFailures` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:89`<br>`TestCalendarParserHelpersHandleMalformedAndPartialAuthorityDocuments` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:156`<br>`TestCalendarAuthorityValidatorHandlesMissingAnchorsAndSparseYears` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:179`<br>`TestCalendarParsersDiscardIncompleteOrOutOfRangeAuthorityRows` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:229`<br>`TestCalendarSourceAlertLifecycleRecordsFailuresDeduplicatesAndRecovers` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:278`<br>`TestCalendarParsersHonorNarrowFetchWindowsAndDiscardImpossibleDates` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:298`<br>`TestNilCalendarManagerOperationsRemainSafeDuringStartupAndShutdown` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:21`<br>`TestDefaultRegistryRegistersExpectedSources` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:40`<br>`TestDefaultRegistryUsesCalendarFetchTimeout` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:55`<br>`TestHTTPCalendarSourceFetchBuildsSnapshotMetadata` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:88`<br>`TestHTTPCalendarSourceFetchReturnsStatusErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:109`<br>`TestHTTPCalendarSourceFetchRejectsSparseAnnualSchedules` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:133`<br>`TestAnchorYearSchedulesValidatorAllowsMissingFutureYearCoverage` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:201`<br>`TestDefaultHolidayOverrideParserUSParsesTableRows` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:224`<br>`TestNYSEHolidayScheduleParserParsesMultiYearTableAndFootnotes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:276`<br>`TestDefaultHolidayOverrideParserHKParsesEnglishList` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:288`<br>`TestHongKongHolidayICalParserParsesClosedDays` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:320`<br>`TestSSETradingScheduleParserExpandsRangesAndSkipsMakeupDays` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:351`<br>`TestSSETradingScheduleParserInfersCrossYearRange` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:378`<br>`TestDefaultHolidayOverrideParserCNParsesChineseDateLine` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/http_source_test.go:390`<br>`TestDefaultHolidayOverrideParserRejectsOutOfRangeDates` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:14`<br>`TestManagerLifecycleAndTemplateBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:50`<br>`TestManualOverrideStatusAndSessionBoundaries` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:119`<br>`TestHTTPCalendarSourceValidateSnapshotBoundary` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:146`<br>`TestCalendarSourceAvailabilityNotesAndRefreshTargets` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:172`<br>`TestManagerValidateCachedSnapshotRejectsCorruptSnapshots` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:216`<br>`TestManagerValidateCachedSnapshotUsesRegisteredSourceValidator` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:248`<br>`TestManagerCachedSnapshotMainlandFallbackAndFreshnessBoundaries` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:287`<br>`TestSourceRegistryNilDuplicateAndMarketNormalizationBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:333`<br>`TestExtractNYSEHeaderYearsSkipsMalformedRowsBeforeValidHeader` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_probe_test.go:14`<br>`TestManagerProbeMarketWarmupAndSnapshotOrdering` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_runtime_test.go:17`<br>`TestManagerBackgroundRefreshFollowsSettingsReload` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_runtime_test.go:86`<br>`TestManagerRefreshKeepsValidSnapshotWhenPersistenceFails` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_runtime_test.go:125`<br>`TestManagerRestoreReportsMalformedCachedSnapshot` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_runtime_test.go:142`<br>`TestManagerStatusReportsManualAndRemoteOverrideModes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:33`<br>`TestManagerFailureBackoffUsesHoursAndCapsAtTwentyFour` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:50`<br>`TestDefaultWarmupRefreshTimeoutCoversSequentialRemoteSources` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:64`<br>`TestManagerFallsBackToBuiltinWhenOfficialRefreshFails` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:119`<br>`TestManagerStatusIncludesSnapshotSummariesAndSampleSchedules` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:207`<br>`TestManagerManualOverridesBeatRemoteAndBuiltin` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:235`<br>`TestManagerSharedMainlandSourceAppliesToSHAndSZ` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:291`<br>`TestManagerIgnoresStaleRemoteSnapshots` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:346`<br>`TestManagerDiscardInvalidCachedSnapshotOnRestore` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:393`<br>`TestManagerProbeMarksHealthySources` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:444`<br>`TestManagerProbeMarksEmptyParsesUnhealthy` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:488`<br>`TestManagerRefreshTreatsEmptyParsesAsFailureAndAlerts` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:543`<br>`TestManagerSourceAlertsDeduplicateAndRecover` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:613`<br>`TestManagerSourceAlertsDeduplicateNetworkTimeoutVariants` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:670`<br>`TestManagerProbeRecoveryClearsCurrentFetchError` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:736`<br>`TestSourceRegistryHonorsPreferredSourceOrder` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:751`<br>`TestManagerSourcesExposeAvailabilityNotes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:779`<br>`TestManagerStatusExplainsBuiltinEffectiveReason` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:811`<br>`TestManagerStatusUsesRemoteCoverageSourceForRegularDay` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:876`<br>`TestSnapshotCacheKeyUsesMarketLocalYear` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:890`<br>`TestSnapshotCacheIndexesEveryCoveredMarketYear` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/manager_test.go:917`<br>`TestManagerCurrentTimeNormalizesInjectedClockToUTC` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/source_health_status_test.go:17`<br>`TestRefreshAndProbeKeepPerSourceHealthTruthful` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/source_health_status_test.go:98`<br>`TestStatusDistinguishesRemoteCoverageFromRemoteOverride` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/exchangecalendar/source_json_test.go:10`<br>`TestSourceStatusJSONOmitsZeroTimes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/frontendassets/dev_test.go:7`<br>`TestFileSystemReportsExternalAssetsForDevelopmentBuild` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/frontendassets/release_test.go:14`<br>`TestFileSystemEmbedsUnderscorePrefixedAssets` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/frontendassets/release_test.go:49`<br>`TestFileSystemEmbedsDocumentationAndLegalNotices` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/frontendassets/release_test.go:96`<br>`TestFileSystemDoesNotEmbedRemovedGoPineRuntimeReferences` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:19`<br>`TestClientResponseAndErrorBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:62`<br>`TestClientRetryAndTransportBoundaries` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:131`<br>`TestClientDoesNotRetryExplicitPoolBackpressure` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:151`<br>`TestClientHealthContractBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:188`<br>`TestClientCandlesEncodeAdjustmentOnlyWhenRequested` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:219`<br>`TestConversionRejectsInvalidMarketAndInstrumentContracts` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:268`<br>`TestConversionRejectsInvalidSecurityAndSnapshotContracts` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:321`<br>`TestConversionProjectsFundamentalsOnlyWhenSidecarReportsThem` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:356`<br>`TestConversionRejectsInvalidCandleContracts` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:392`<br>`TestConversionRejectsInvalidCandlePaginationMetadata` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:487`<br>`TestProviderInputAndUnavailableBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:517`<br>`TestHelperBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:580`<br>`TestClientRequestOnceReadFailure` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/boundaries_test.go:604`<br>`TestRemainingProviderAndConversionBranches` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/client_index_constituents_test.go:14`<br>`TestClientIndexConstituentsEncodesLimitAndDecodesNullableWeights` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/client_index_constituents_test.go:45`<br>`TestClientIndexConstituentsMapsUnsupportedInstrumentToCapabilityError` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/client_news_actions_test.go:41`<br>`TestClientNewsEncodesLimitAndDecodesNullableEntries` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/client_news_actions_test.go:70`<br>`TestClientCorporateActionsEncodesRangeAndDecodesEvents` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/client_news_actions_test.go:101`<br>`TestClientCorporateActionsOmitsUnsetRange` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/client_news_actions_test.go:121`<br>`TestClientNewsMapsUnsupportedMarketToCapabilityError` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/client_news_actions_test.go:139`<br>`TestClientCorporateActionsSurfacesPoolBusyWithoutRetryStorm` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/client_news_actions_test.go:159`<br>`TestClientNewsSurfacesColdCacheWarming` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_calendar_macro_test.go:12`<br>`TestClientCalendarMacroEndpointsEncodePathsAndQuery` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_calendar_macro_test.go:81`<br>`TestProviderCalendarConvertsEntriesAndKeepsNulls` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_calendar_macro_test.go:157`<br>`TestProviderCalendarRejectsMalformedEntries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_calendar_macro_test.go:183`<br>`TestProviderMacroConvertsCatalogAndHistory` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_calendar_macro_test.go:241`<br>`TestProviderMacroHistoryRejectsMismatchedEcho` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_calendar_macro_test.go:256`<br>`TestProviderCalendarMacroPassesSidecarErrorsThrough` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:14`<br>`TestClientCompanyResearchEndpointsEncodePathAndStatement` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:72`<br>`TestProviderCompanyProfileConvertsCNInstrument` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:106`<br>`TestProviderFinancialStatementsConvertsPeriodsAndValidatesEcho` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:142`<br>`TestProviderOwnershipConvertsMajorHoldersAndHolderTypes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:172`<br>`TestProviderCompanyResearchRejectsUnsupportedMarketsAndStatement` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:193`<br>`TestProviderCompanyProfileSupportsHK` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:220`<br>`TestProviderCompanyResearchMapsSidecarUnsupportedMarket` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:238`<br>`TestProviderOwnershipRejectsUnknownGroupKind` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:254`<br>`TestProviderAnalystConsensusConvertsEastmoneyAggregate` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:298`<br>`TestProviderAnalystConsensusMapsSidecarUnsupportedMarket` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:316`<br>`TestProviderAnalystConsensusPassesThroughNotFound` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_company_research_test.go:335`<br>`TestProviderAnalystConsensusRejectsUnsupportedMarketsGoSide` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_index_constituents_test.go:23`<br>`TestProviderIndexConstituentsConvertsEntriesAndAppliesDefaultLimit` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_index_constituents_test.go:48`<br>`TestProviderIndexConstituentsRejectsMalformedPayloads` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_index_constituents_test.go:65`<br>`TestProviderIndexConstituentsRejectsIdentityMismatch` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_index_constituents_test.go:80`<br>`TestProviderIndexConstituentsSurfacesUnsupportedMarkets` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_index_constituents_test.go:97`<br>`TestProviderIndexConstituentsMeetsOptionalCapabilityContract` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_news_actions_test.go:14`<br>`TestProviderNewsConvertsEntriesAndAppliesDefaultLimit` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_news_actions_test.go:41`<br>`TestProviderCorporateActionsSortsAndValidatesEvents` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_news_actions_test.go:71`<br>`TestProviderNewsAndCorporateActionsRejectMalformedPayloads` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_news_actions_test.go:100`<br>`TestProviderNewsRejectsIdentityMismatch` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_news_actions_test.go:114`<br>`TestProviderNewsAndCorporateActionsSurfaceUnsupportedUSAndHK` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_news_actions_test.go:134`<br>`TestProviderNewsAndCorporateActionsMeetOptionalCapabilityContracts` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_rankings_industries_test.go:29`<br>`TestClientRankingsEncodesMarketKindAndLimit` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_rankings_industries_test.go:53`<br>`TestClientIndustriesEncodesKindAndDecodesBoards` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_rankings_industries_test.go:82`<br>`TestClientIndustryMembersEscapesBoardNameAndOmitsEmptyKind` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_rankings_industries_test.go:108`<br>`TestProviderRankingsConvertsEntriesAndNormalizesIdentity` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_rankings_industries_test.go:141`<br>`TestProviderRankingsRejectsUnsupportedMarketAndKind` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_rankings_industries_test.go:159`<br>`TestProviderRankingsRejectsKindMismatch` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_rankings_industries_test.go:172`<br>`TestProviderIndustriesConvertsBoardsAndMembers` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_rankings_industries_test.go:231`<br>`TestProviderIndustriesMapsSidecarUnsupportedToCapabilityError` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_screen_test.go:16`<br>`TestClientScreenPostsConditionSortAndPagingBody` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_screen_test.go:83`<br>`TestProviderScreenConvertsCNEntriesAndDerivesSymbol` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_screen_test.go:133`<br>`TestProviderScreenAcceptsCNSHSZHKUSAndRejectsOthers` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_screen_test.go:160`<br>`TestProviderScreenClassifiesSidecarErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_screen_test.go:179`<br>`TestProviderScreenClassifiesMultipleSortKeysAsCapabilityError` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_screen_test.go:202`<br>`TestProviderScreenRejectsEntriesWithoutInstrumentID` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_test.go:20`<br>`TestProviderExposesPollingOnlyAKShareBoundary` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_test.go:51`<br>`TestProviderAdvertisesDailyAdjustmentAndRejectsIntradayAdjustment` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_test.go:115`<br>`TestProviderConvertsNamespacedSidecarContract` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_test.go:186`<br>`TestProviderPreservesErrorsAndPartialBatchResults` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_test.go:236`<br>`TestProviderNormalizesIndexAndExchangeIdentities` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_test.go:272`<br>`TestProviderRejectsNonRegularCandleSessions` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_test.go:286`<br>`TestProviderChunksBatchSnapshotsAtContractLimit` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/akshare/provider_test.go:404`<br>`TestClientRejectsInvalidEndpoint` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_news_actions_test.go:14`<br>`TestClientNewsEncodesLimitAndDecodesEntries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_news_actions_test.go:39`<br>`TestClientCorporateActionsEncodesInclusiveRange` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_news_actions_test.go:66`<br>`TestClientCorporateActionsOmitsUnsetRange` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_news_actions_test.go:83`<br>`TestClientNewsSurfacesWarmingAndStructuredFailures` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:17`<br>`TestNewClientValidatesURLAndDefaultTransport` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:30`<br>`TestClientRetriesSafeServerFailuresThenReturnsDecodedResponse` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:56`<br>`TestClientHealthRequiresYFinanceVersion` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:75`<br>`TestClientHealthRequiresKnownRuntimeState` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:86`<br>`TestClientPreservesStructuredHTTPErrorWithoutRetryingCallerFailures` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:110`<br>`TestClientClassifiesExhaustedServerAndNetworkFailuresAsUnavailable` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:136`<br>`TestClientRejectsMalformedEmptyTrailingAndOversizedResponses` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:156`<br>`TestClientRetryWaitHonorsContextCancellation` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:172`<br>`TestClientClassifiesRuntimeWarmingAfterRetryBudget` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:192`<br>`TestClientTimeoutCoversAllRetryAttempts` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:214`<br>`TestClientEndpointMethodsEncodePathAndOptionalCandleQuery` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/client_test.go:265`<br>`TestHTTPErrorFormattingAndClassification` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:45`<br>`TestSnapshotConversionPreservesRegularChangeWhenPreMarketQuoteIsUnavailable` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:77`<br>`TestSnapshotConversionUsesRegularCloseForValidPreMarketQuote` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:105`<br>`TestSnapshotConversionRejectsInvalidPreMarketQuotesWithoutChangingTheBaseline` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:147`<br>`TestSnapshotConversionUsesPriceFallbacksAndCanonicalTimes` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:179`<br>`TestVolumeConversionPreservesLargeFractionalDecimalValues` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:214`<br>`TestSnapshotConversionKeepsOriginalCloseForNonUSClosedMarkets` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:237`<br>`TestSnapshotConversionUsesCalendarForLatestClosedSessionAfterMarket` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:283`<br>`TestSnapshotConversionRejectsContractViolations` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:306`<br>`TestCandleConversionBuildsNeutralResponseAndRejectsDrift` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:354`<br>`TestCandleConversionRejectsInvalidPaginationMetadata` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:446`<br>`TestHistoricalCandleResponseValidationPreservesStrictAndBoundedPages` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:481`<br>`TestCandleConversionUsesEarlyCloseCalendarAndDropsClosedBars` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:504`<br>`TestCandleConversionMarksYahooExtendedVolumeUnavailable` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:528`<br>`TestCandleConversionFiltersSessionsAndAppliesFinalLimit` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:548`<br>`TestConvertedCandleSessionGroupRejectsUnknownLabels` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:554`<br>`TestMarketAndCandidateConversionRejectsMalformedProviderData` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:616`<br>`TestSecurityConversionValidatesIdentityAndDefaultsSource` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/conversion_test.go:641`<br>`TestNormalizationAndNumericHelpersCoverAliasesAndBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_company_research_test.go:14`<br>`TestClientCompanyResearchEndpointsEncodePathAndStatement` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_company_research_test.go:69`<br>`TestProviderCompanyProfileConvertsGroupsAndSkipsEmptyFields` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_company_research_test.go:100`<br>`TestProviderFinancialStatementsConvertsFieldsPeriodsAndNullableRatios` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_company_research_test.go:134`<br>`TestProviderAnalystConsensusConvertsRatingTargetAndDistribution` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_company_research_test.go:161`<br>`TestProviderOwnershipConvertsGroupsAndValidatesKind` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_company_research_test.go:192`<br>`TestProviderCompanyResearchRejectsUnsupportedMarketsWithoutSidecarCall` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_company_research_test.go:220`<br>`TestProviderCompanyResearchMapsSidecarUnsupportedMarket` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_company_research_test.go:236`<br>`TestProviderCompanyResearchRejectsIdentityMismatch` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_news_actions_test.go:14`<br>`TestProviderNewsNormalizesEntriesAndDefaults` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_news_actions_test.go:46`<br>`TestProviderNewsRejectsMismatchedIdentityAndInvalidTimestamps` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_news_actions_test.go:67`<br>`TestProviderCorporateActionsSortsEventsByExDateAndKind` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_news_actions_test.go:102`<br>`TestProviderCorporateActionsRejectsUnknownKindsAndBadExDates` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_news_actions_test.go:124`<br>`TestProviderNewsAndCorporateActionsSupportEveryLeafMarket` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_news_actions_test.go:140`<br>`TestProviderNewsAndCorporateActionsMeetOptionalCapabilityContracts` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_rankings_test.go:14`<br>`TestClientRankingsEncodesMarketKindAndLimit` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_rankings_test.go:42`<br>`TestProviderRankingsConvertsEntriesAndAppliesDefaultLimit` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_rankings_test.go:71`<br>`TestProviderRankingsRejectsNonUSMarketsWithoutSidecarCall` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_rankings_test.go:92`<br>`TestProviderRankingsRejectsKindMismatch` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_screen_test.go:16`<br>`TestClientScreenPostsConditionSortAndPagingBody` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_screen_test.go:76`<br>`TestProviderScreenConvertsEntriesAndDerivesSymbol` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_screen_test.go:120`<br>`TestProviderScreenRejectsNonUSMarketsWithoutSidecarCall` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_screen_test.go:138`<br>`TestProviderScreenClassifiesSidecarErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_screen_test.go:163`<br>`TestProviderScreenRejectsEntriesWithoutInstrumentID` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_test.go:33`<br>`TestProviderAdvertisesForwardAdjustmentAndRejectsBackward` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_test.go:66`<br>`TestProviderDescriptorReflectsActualYahooPollingBoundary` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_test.go:98`<br>`TestProviderReadsMarketsSearchLookupAndSecurityThroughSidecar` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_test.go:163`<br>`TestProviderConvertsSnapshotsCandlesHealthAndUnsupportedDepth` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_test.go:229`<br>`TestProviderMarksYahooExtendedMinuteVolumeUnavailable` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_test.go:254`<br>`TestProviderRejectsUnsupportedYahooCandleSessions` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_test.go:265`<br>`TestProviderQueriesHongKongAndChinaLeafMarkets` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_test.go:287`<br>`TestProviderNormalizesUSAliasesAndRejectsInvalidInputs` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_test.go:340`<br>`TestGetHistoricalCandlesRejectsOneMinutePeriodBeyondSevenDayWindow` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_test.go:379`<br>`TestProviderQueryTickersReturnsPartialSuccessAndPreservesAllFailureErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/integration/yfinance/provider_test.go:421`<br>`TestProviderSearchClampsLimitAndLookupRejectsMismatchedIdentity` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/jftsettings/exchange_calendar_settings_validation_test.go:8`<br>`TestExchangeCalendarSettingsUnmarshalExplicitEnabledField` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/jftsettings/exchange_calendar_settings_validation_test.go:18`<br>`TestExchangeCalendarSettingsRejectsInvalidFieldValueInsideObject` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/jftsettings/types_test.go:8`<br>`TestExchangeCalendarSettingsDefaultsLegacyErrorNotifications` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/jftsettings/types_test.go:21`<br>`TestExchangeCalendarSettingsPreservesExplicitErrorNotifications` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/jftsettings/types_test.go:39`<br>`TestExchangeCalendarSettingsRejectsMalformedJSON` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/client_test.go:8`<br>`TestNormalizeSubscriptions` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/client_test.go:47`<br>`TestClientRegistryTracksActiveInstruments` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/client_test.go:63`<br>`TestClientSnapshotIsIsolatedAndUpdateIsCoalesced` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/lifecycle_boundaries_test.go:10`<br>`TestNilClientAndPublisherBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/lifecycle_boundaries_test.go:38`<br>`TestReplayPublisherStartErrorAndNilSourceAreNoops` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/lifecycle_boundaries_test.go:63`<br>`TestReplayPublisherStartRacingCloseStopsLateSource` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/notification_delivery_test.go:5`<br>`TestNotificationDeliveryKeepsHostNotificationOutcomeExplicit` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/publisher_test.go:11`<br>`TestReplayPublisherSequencesAndRetainsWindow` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/publisher_test.go:39`<br>`TestReplayPublisherNormalizesEventTimeToUTC` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/publisher_test.go:57`<br>`TestReplayPublisherExpiresOldEventsWithoutResettingSequence` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/publisher_test.go:73`<br>`TestReplayPublisherCloseStopsSourcesOnce` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/live/publisher_test.go:108`<br>`TestReplayPublisherConcurrentPublishAndAfter` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/candle_query_options_test.go:8`<br>`TestNormalizeCandleOptionsAcceptsSessionsAndAdjustments` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/candle_query_options_test.go:18`<br>`TestNormalizeCandleOptionsRejectsUnsupportedValues` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/capabilities_evaluation_test.go:11`<br>`TestCapabilitiesContextFiltersAndReportsRuntimeEvaluation` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/capabilities_evaluation_test.go:64`<br>`TestCapabilitiesContextFiltersProductsAndSegments` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/capabilities_evaluation_test.go:99`<br>`TestCapabilitiesContextMarksDeclaredButMissingInterfaceUnavailable` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/capabilities_evaluation_test.go:118`<br>`TestStaticRuntimeEvaluationDistinguishesRequiredDimensions` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/earnings_calendar_query_test.go:11`<br>`TestValidateResearchCalendarQueryAcceptsSupportedBusinessParameters` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/earnings_calendar_query_test.go:35`<br>`TestValidateResearchCalendarQueryRejectsInvalidParameters` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/earnings_calendar_query_test.go:72`<br>`TestValidateResearchCalendarQueryIgnoresOtherOperations` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/market_data_reads_test.go:12`<br>`TestWorkspaceMarketDataReadsPreserveExplicitProviderAndResponseShape` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/market_data_reads_test.go:116`<br>`TestWorkspaceMarketDataReadsRejectInvalidInstrument` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/market_data_reads_test.go:134`<br>`TestWorkspaceCandlePaginationRejectsInvalidMetadata` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/market_data_reads_test.go:226`<br>`TestNormalizeCoreCandleQueryAcceptsSessionParameterShapes` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/market_data_reads_test.go:245`<br>`TestWorkspaceMarketDataReadsResolveChinaAggregateToExchangeLeaf` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/market_data_reads_test.go:307`<br>`TestWorkspaceMarketDataReadsSurfaceProviderFailuresAndNormalizeFallbacks` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/market_data_reads_test.go:371`<br>`TestWorkspaceSnapshotRestoresRegularCloseComparisonSemantics` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/market_data_reads_test.go:418`<br>`TestWorkspaceSnapshotUsesActiveExtendedSessionFields` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/market_data_reads_test.go:470`<br>`TestWorkspaceSnapshotExtendedSessionFallbacksRemainStable` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/prediction_quote_candle_bridge_test.go:13`<br>`TestQuotePredictionComboValidatesPersistsAndPublishesServerExpiry` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/prediction_quote_candle_bridge_test.go:61`<br>`TestQuotePredictionComboRejectsInvalidAndUnpersistableQuotes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/prediction_quote_candle_bridge_test.go:137`<br>`TestPredictionPushSourceCachesFreshUniqueUpdates` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/prediction_quote_candle_bridge_test.go:202`<br>`TestCoreCandleBridgeValidatesBoundariesAndProductSemantics` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_capability_alignment_test.go:9`<br>`TestEmbeddedResearchFeatureAllowListIsExplicit` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_calendar_test.go:33`<br>`TestEmbeddedProviderServesCalendarOperations` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_calendar_test.go:147`<br>`TestEmbeddedProviderServesMacroOperations` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_calendar_test.go:205`<br>`TestEmbeddedProviderRejectsUnsupportedCalendarMacroOperations` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_calendar_test.go:237`<br>`TestEmbeddedProviderPropagatesCalendarMacroErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_calendar_test.go:262`<br>`TestEmbeddedProviderCalendarMacroStayOnBrokerPathForFutu` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_company_test.go:51`<br>`TestEmbeddedProviderServesCompanyResearchDefaultOperations` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_company_test.go:148`<br>`TestEmbeddedProviderCompanyResearchForwardsMarketSymbolAndStatement` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_company_test.go:170`<br>`TestEmbeddedProviderCompanyResearchAcceptsOmittedOperation` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_company_test.go:183`<br>`TestEmbeddedProviderRejectsNonDefaultCompanyOperations` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_company_test.go:207`<br>`TestEmbeddedProviderPropagatesCompanyResearchCapabilityErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_company_test.go:226`<br>`TestEmbeddedProviderCompanyResearchStaysOnBrokerPathForFutu` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_interception_test.go:180`<br>`TestEmbeddedProviderServesNewsForExplicitBrokerID` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_interception_test.go:213`<br>`TestEmbeddedProviderServesCorporateActionsForActiveProvider` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_interception_test.go:248`<br>`TestEmbeddedProviderLeavesFutuQueriesOnBrokerPath` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_interception_test.go:294`<br>`TestEmbeddedProviderPropagatesCapabilityAndLifecycleErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:53`<br>`TestEmbeddedProviderMapsRankingsOperationsToKinds` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:101`<br>`TestEmbeddedProviderRejectsUnmappedRankingsOperations` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:127`<br>`TestEmbeddedProviderMapsIndustryBoardOperations` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:173`<br>`TestEmbeddedProviderServesPlateMembersFromInstrumentID` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:199`<br>`TestEmbeddedProviderRejectsUnsupportedIndustryOperationsAndPlateTypes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:235`<br>`TestEmbeddedProviderPropagatesRankingsCapabilityErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:262`<br>`TestEmbeddedProviderRankingsStayOnBrokerPathForFutu` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:278`<br>`TestEmbeddedProviderDefaultsEmptyMarketToProviderDefault` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_screen_test.go:83`<br>`TestEmbeddedProviderServesScreenAndProjectsRows` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_screen_test.go:168`<br>`TestEmbeddedProviderRejectsFutuCatalogScreenWith409` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_screen_test.go:186`<br>`TestEmbeddedProviderRejectsNonExecutableScreenShapes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_screen_test.go:214`<br>`TestEmbeddedProviderMapsScreenCapabilityErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_screen_test.go:236`<br>`TestEmbeddedScreenDecodesMapDefinitionAndDefaultsPaging` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_facade_screen_test.go:267`<br>`TestEmbeddedProviderServesUSScreenViaAkshare` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:16`<br>`TestProviderEarningsCalendarProjectionMapsFrontendKeys` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:63`<br>`TestProviderDividendCalendarProjectionMapsFrontendKeys` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:98`<br>`TestProviderEconomicCalendarProjectionDerivesDateAndTime` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:154`<br>`TestProviderIpoCalendarProjectionMapsFrontendKeys` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:194`<br>`TestProviderMacroIndicatorsProjectionNestsIndicatorList` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:242`<br>`TestProviderMacroIndicatorHistoryProjectionMapsFrontendKeys` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:14`<br>`TestProviderNewsProjectionMapsNullableFieldsAndAsOf` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:79`<br>`TestProviderNewsProjectionFallsBackToNowWithoutTimestamps` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:97`<br>`TestProviderCorporateActionProjectionFormatsStatements` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:139`<br>`TestEmbeddedNewsLimitPrecedenceAndClamp` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:160`<br>`TestEmbeddedProviderServesMirrorsActiveProviderMatching` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:186`<br>`TestEmbeddedResearchInstrumentDerivesMarketAndSymbol` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:205`<br>`TestMapEmbeddedProviderErrorKeepsSentinels` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:226`<br>`TestProviderRankingsProjectionMapsFrontendKeys` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:290`<br>`TestProviderIndustryBoardsProjectionMapsFrontendKeys` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:326`<br>`TestProviderIndustryMembersProjectionUsesRankingKeys` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:351`<br>`TestEmbeddedRankingsLimitPrecedenceAndClamp` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:371`<br>`TestProviderCompanyProfileProjectionMapsFrontendKeys` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:421`<br>`TestProviderFinancialStatementsProjectionMapsFrontendKeys` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:486`<br>`TestProviderAnalystConsensusProjectionMapsFrontendKeys` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:530`<br>`TestProviderAnalystConsensusProjectionOmitsAbsentSections` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/provider_projection_test.go:547`<br>`TestProviderOwnershipProjectionMapsFrontendKeys` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_routing_and_validation_test.go:14`<br>`TestProductFeatureServiceRemainingRoutingAndDegradationBranches` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_routing_and_validation_test.go:187`<br>`TestOptionFeatureValidationRejectsMalformedAdvancedFilters` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_routing_and_validation_test.go:289`<br>`TestResearchInstitutionDetailQueriesRequireInstitutionID` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_routing_and_validation_test.go:346`<br>`TestQueryUsesFreshPredictionPushBeforePolling` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_test.go:12`<br>`TestPredictionEligibilityRejectsFutuSecuritiesAndAcceptsFutuInc` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_test.go:30`<br>`TestQueryDoesNotFallbackWhenBrokerIsExplicit` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_test.go:46`<br>`TestBatchSnapshotsUsesOptionalSourceWithoutSubscriptions` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_test.go:82`<br>`TestBatchSnapshotsRejectsUnsupportedRegionsAndOversizedRequests` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_test.go:99`<br>`TestPredictionSubscriptionLeasesReferenceCountVisibleContracts` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_test.go:143`<br>`TestProductFeatureServiceRoutesEveryOptionalInterfaceAndCaches` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_test.go:210`<br>`TestProductFeatureServiceFailureBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_test.go:243`<br>`TestProductFeatureServiceExhaustiveFailureAndNormalizationBranches` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_test.go:321`<br>`TestProductFeaturePredictionAndCustomizationFailureBranches` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_test.go:404`<br>`TestProductFeatureServiceNormalizesCoreMarketCandlesWithProvider` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/service_test.go:443`<br>`TestProductFeatureDirectAdapterCacheAndEligibilityBranches` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/typed_queries_test.go:12`<br>`TestDocumentResultPreservesFeatureWireShape` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/productfeatures/typed_queries_test.go:41`<br>`TestTypedCapabilityDescriptionsAreDefensive` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/research/presets_test.go:85`<br>`TestServiceCreateListGetAndDeletePreset` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/research/presets_test.go:111`<br>`TestServiceUpdatePresetMergesFieldsAndEnforcesRevision` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/research/presets_test.go:146`<br>`TestServiceRejectsUnavailableAndInvalidPresetOperations` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/research/presets_test.go:202`<br>`TestServicePropagatesRepositoryFailures` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/retry/do_attempts_test.go:8`<br>`TestDoUsesDefaultsAndReturnsEventualSuccess` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/retry/do_attempts_test.go:26`<br>`TestDoRetriesUntilSuccess` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/retry/retry_test.go:10`<br>`TestDoRetriesWithDeterministicBackoff` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/retry/retry_test.go:40`<br>`TestDoZeroBaseDelayDoesNotSleep` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/retry/retry_test.go:57`<br>`TestDoReturnsNonRetryableErrorImmediately` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/retry/retry_test.go:76`<br>`TestFutuRateLimitShouldRetry` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/security/passwordhash/passwordhash_test.go:9`<br>`TestHashAndVerify` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/security/passwordhash/passwordhash_test.go:25`<br>`TestVerifyRejectsUnsafeParametersBeforeHashing` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/security/passwordhash/passwordhash_test.go:32`<br>`TestValidRejectsMalformedHashes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_status_defaults_test.go:10`<br>`TestStatusIncludesInjectedObservabilitySummaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_status_defaults_test.go:41`<br>`TestStatusProvidesDefaultRequestObservabilitySummary` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_status_defaults_test.go:61`<br>`TestExchangeCalendarDelegatesAndFallbacks` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_status_defaults_test.go:109`<br>`TestStorageAndRealTradeDefaultsExposeFrontendShape` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_status_defaults_test.go:137`<br>`TestFutuDefaultsExposeEmptyGuideAndSnapshot` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_status_defaults_test.go:151`<br>`TestRuntimeDependenciesDelegates` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_test.go:13`<br>`TestStatusDefaultsAndInjectedSummaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_test.go:59`<br>`TestStatusUsesDynamicPortAndTradingEnvironmentProviders` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_test.go:83`<br>`TestRealTradeDefaultsMatchFrontendContract` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_test.go:142`<br>`TestRealTradeStateUsesInjectedRiskGatewaySnapshot` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_test.go:201`<br>`TestRealTradeStateNormalizesTypedNilSlices` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_test.go:212`<br>`TestRealTradeControlDelegatesAndUnavailableBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_test.go:309`<br>`TestFutuHealthAndResetDelegates` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/system/service_test.go:345`<br>`TestFutuHealthDefaultsUnavailable` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/datatype/floats/funcs_test.go:9`<br>`TestPercentile` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/datatype/floats/funcs_test.go:24`<br>`TestLower` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/datatype/floats/funcs_test.go:29`<br>`TestHigher` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/datatype/floats/funcs_test.go:34`<br>`TestLSM` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/datatype/floats/pivot_test.go:9`<br>`TestFindPivot` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/datatype/floats/slice_test.go:12`<br>`TestNewRandomNormal` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/datatype/floats/slice_test.go:24`<br>`TestNewRandomPoisson` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/datatype/floats/slice_test.go:35`<br>`TestNewRandomUniform` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/datatype/floats/slice_test.go:47`<br>`TestSub` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/datatype/floats/slice_test.go:56`<br>`TestTruncate` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/datatype/floats/slice_test.go:64`<br>`TestAdd` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/convert_test.go:10`<br>`Test_FormatString` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_dnum_test.go:11`<br>`TestDelta` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_dnum_test.go:17`<br>`TestFloor` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_dnum_test.go:23`<br>`TestInternal` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_legacy_test.go:9`<br>`TestNumFractionalDigitsLegacy` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:122`<br>`TestMulString` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:132`<br>`TestMulExp` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:140`<br>`TestNew` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:157`<br>`TestFormatString` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:189`<br>`TestRound` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:200`<br>`TestNewFromString` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:206`<br>`TestFromString` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:231`<br>`TestJson` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:274`<br>`TestYaml` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:317`<br>`TestNumFractionalDigits` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/expirable_test.go:12`<br>`TestExpirableValue_SetAndGet` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/expirable_test.go:24`<br>`TestExpirableValue_IsExpired` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/expirable_test.go:32`<br>`TestExpirableValue_GetExpired` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/expirable_test.go:43`<br>`TestExpirableValue_GetUnset` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/expirable_test.go:50`<br>`TestExpirableValue_String` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/reduce_test.go:9`<br>`TestReduce` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/fixedpoint/slice_test.go:10`<br>`TestSortInterface` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/account_test.go:11`<br>`TestAccountLockAndUnlock` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/account_test.go:37`<br>`TestAccountLockAndUse` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/account_test.go:64`<br>`TestUpdateFuturesPositions_MergeAndOverride` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/balance_test.go:13`<br>`TestBalanceMap_Add` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/balance_test.go:43`<br>`TestBalanceMap_Assets` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/connectivity_test.go:8`<br>`TestConnectivity` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/connectivitygroup_test.go:11`<br>`TestConnectivityGroupAuthC` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/connectivitygroup_test.go:38`<br>`TestConnectivityGroup` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/connectivitygroup_test.go:295`<br>`Test_sumStates` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/duration_test.go:13`<br>`TestParseSimpleDuration` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/duration_test.go:59`<br>`TestSerialization` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/error_test.go:10`<br>`TestRecoverOrderError` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/exchange_test.go:9`<br>`Test_exchangeName` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:16`<br>`TestQueue` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:35`<br>`TestFloat` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:41`<br>`TestNextCross` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:55`<br>`TestFloat64Slice` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:75`<br>`TestCorr` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:95`<br>`TestCov` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:109`<br>`TestSkew` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:115`<br>`TestEntropy` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:121`<br>`TestCrossEntropy` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:128`<br>`TestSoftmax` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:137`<br>`TestSigmoid` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:146`<br>`TestHighLowest` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:152`<br>`TestAdd` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:160`<br>`TestDiv` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:169`<br>`TestMul` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:178`<br>`TestArray` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:186`<br>`TestSwitchInterface` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:204`<br>`TestLogisticRegression` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:220`<br>`TestDot` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:235`<br>`TestClone` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:244`<br>`TestPlot` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:255`<br>`TestFilter` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/indicator_test.go:265`<br>`TestOLS` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/interval_test.go:10`<br>`TestTruncate` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/interval_test.go:23`<br>`TestParseInterval` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/interval_test.go:31`<br>`TestIntervalSort` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/kline_test.go:10`<br>`TestKLineWindow_Tail` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/kline_test.go:37`<br>`TestKLineWindow_Truncate` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/kline_test.go:61`<br>`TestShrinkSlice` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/market_store_test.go:9`<br>`TestMarketDataStore_AddKLineAndTruncateWindow` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/market_test.go:16`<br>`TestMarket_GreaterThanMinimalOrderQuantity` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/market_test.go:41`<br>`TestFormatQuantity` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/market_test.go:53`<br>`TestFormatPrice` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/market_test.go:63`<br>`TestDurationParse` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/market_test.go:106`<br>`Test_FormatPrice` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/market_test.go:163`<br>`Test_formatQuantity` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/market_test.go:220`<br>`TestMarket_TruncateQuantity` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/market_test.go:242`<br>`TestMarket_AdjustQuantityByMinNotional` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/market_test.go:270`<br>`TestMarket_AdjustQuantityToContractSize` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/omega_test.go:11`<br>`TestOmega` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/orderbook_test.go:99`<br>`TestOrderBook_IsValid` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/position_test.go:13`<br>`TestPosition_ROI` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/position_test.go:57`<br>`TestPosition_ExchangeFeeRate_Short` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/position_test.go:106`<br>`TestPosition_ExchangeFeeRate_Long` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/position_test.go:156`<br>`TestPosition` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/position_test.go:357`<br>`TestPosition_SetClosing` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/position_test.go:369`<br>`TestPosition_GetBaseAndAverageCost` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/price_volume_heartbeat_test.go:12`<br>`TestPriceHeartBeat_Update` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/price_volume_slice_test.go:11`<br>`TestPriceVolumeSlice_UnmarshalJSON` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/price_volume_slice_test.go:33`<br>`TestPriceVolumeSlice_Remove` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtorderbook_test.go:10`<br>`TestRBOrderBook_EmptyBook` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtorderbook_test.go:21`<br>`TestRBOrderBook_Load` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtorderbook_test.go:43`<br>`TestRBOrderBook_LoadAndDelete` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtree_test.go:15`<br>`TestRBTree_ConcurrentIndependence` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtree_test.go:40`<br>`TestRBTree_InsertAndDelete` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtree_test.go:59`<br>`TestRBTree_Rightmost` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtree_test.go:75`<br>`TestRBTree_RandomInsertSearchAndDelete` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtree_test.go:94`<br>`TestRBTree_CopyInorder` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtree_test.go:118`<br>`TestTree_Copy` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtree_test.go:143`<br>`TestRBTree_basic` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtree_test.go:175`<br>`TestRBTree_bulkInsert` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtree_test.go:188`<br>`TestRBTree_bulkInsertAndDelete` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/rbtree_test.go:245`<br>`TestRBTree_StressInsertDeleteAndValidate` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/series.go:83`<br>`TestUpdate` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/series_float64_test.go:9`<br>`TestSeriesBaseFuncWithPushData` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/sharpe_test.go:21`<br>`TestSharpe` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/sliceorderbook_test.go:9`<br>`TestSliceOrderBook_CopyDepth` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/sort_test.go:12`<br>`TestSortTradesAscending` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/sort_test.go:43`<br>`TestSortOrdersByPrice` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/sortino_test.go:21`<br>`TestSortino` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/standardstream_test.go:37`<br>`TestStandardStream_RawMessage_NoParser` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/standardstream_test.go:71`<br>`TestStandardStream_ParserAndDispatcher` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/standardstream_test.go:121`<br>`TestStandardStream_BeforeConnectCalled` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/standardstream_test.go:149`<br>`TestStandardStream_HeartbeatAndPing` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/standardstream_test.go:190`<br>`TestStandardStream_ResubscribeTriggersReconnect` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/standardstream_test.go:221`<br>`TestStandardStream_Close` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/syncgroup_test.go:9`<br>`Test_waitGroup_Run` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/time_test.go:10`<br>`TestParseLooseFormatTime_alias_now` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/time_test.go:18`<br>`TestParseLooseFormatTime_alias_yesterday` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/time_test.go:26`<br>`TestLooseFormatTime_UnmarshalJSON` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/time_test.go:56`<br>`TestMillisecondTimestamp_UnmarshalJSON` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/trade_ring_buffer_test.go:12`<br>`TestTradeRingBuffer_Add` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/trade_ring_buffer_test.go:46`<br>`TestTradeRingBuffer_Filter` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/trade_ring_buffer_test.go:96`<br>`TestTradeRingBuffer_Filter_WrapAround` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/trade_ring_buffer_test.go:127`<br>`TestTradeRingBuffer_TradeFrequency` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/trade_stat_test.go:12`<br>`TestCAGR` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/trade_stat_test.go:21`<br>`TestKellyCriterion` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/trade_stat_test.go:31`<br>`TestAnnualHistoricVolatility` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/trade_stat_test.go:40`<br>`TestOptimalF` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/trade_stat_test.go:46`<br>`TestDrawdown` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/trade_stats_test.go:29`<br>`TestTradeStats_consecutiveCounterAndAmount` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/trade_test.go:6`<br>`Test_trimTrailingZero` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/value_map_test.go:10`<br>`Test_ValueMap_Eq` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/value_map_test.go:37`<br>`Test_ValueMap_Add` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/value_map_test.go:59`<br>`Test_ValueMap_AddScalar` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/value_map_test.go:75`<br>`Test_ValueMap_DivScalar` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/value_map_test.go:91`<br>`Test_ValueMap_Sum` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/value_map_test.go:100`<br>`Test_ValueMap_Normalize` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/bbgo/types/value_map_test.go:118`<br>`Test_ValueMap_Normalize_zero_sum` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/besteffort/besteffort_test.go:11`<br>`TestLogError` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/besteffort/besteffort_test.go:33`<br>`TestLogErrorNoError` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/chart/chart_type_test.go:5`<br>`TestNormalizeChartType` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/observability/context_detach_and_importance_test.go:10`<br>`TestObservabilityBackgroundContextsAndImportanceRanks` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/observability/context_detach_and_importance_test.go:37`<br>`TestObservabilityContextDetachWithBackgroundBase` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/observability/observability_test.go:14`<br>`TestStructuredLogIncludesCanonicalCorrelationFields` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/observability/observability_test.go:45`<br>`TestRecorderSnapshotSerializesEmptyCollectionsAsArrays` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:pkg/observability/observability_test.go:62`<br>`TestImportanceThresholdSuppressesLowerImportanceLogs` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/observability/observability_test.go:94`<br>`TestGlobalImportanceThresholdAppliesWithoutRecorder` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/observability/observability_test.go:113`<br>`TestRecorderBoundsErrorsSlowRequestsAndOpenDHealth` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/observability/observability_test.go:148`<br>`TestDetachPreservesCorrelationWithoutParentCancellation` | other | 高风险 | `待人工归类` |
| [ ] | `go:452dea11:pkg/observability/observability_test.go:163`<br>`TestObservabilityBoundaryDefaultsAndGlobalOpenDLogging` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/observability/observability_test.go:215`<br>`TestObservabilityNilContextAndOpenDSuccessBoundaries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_edges_test.go:8`<br>`TestCatalogHelperContractsCoverEditorVariants` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_edges_test.go:82`<br>`TestCatalogParameterBoundsAndAvailability` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_edges_test.go:138`<br>`TestValidateCatalogRejectsIncompleteSemanticRows` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_embedded_test.go:31`<br>`TestEmbeddedCatalogShapeAndSemantics` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_embedded_test.go:80`<br>`TestEmbeddedCatalogValidationUseFlags` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_embedded_test.go:99`<br>`TestNormalizeDefinitionV2AcceptsEmbeddedCatalog` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_embedded_test.go:121`<br>`TestNormalizeDefinitionV2EmbeddedRejectsOutOfCatalogFactors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_embedded_test.go:149`<br>`TestNormalizeDefinitionV2FutuContractUnchanged` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_test.go:9`<br>`TestCatalogIsCompleteStableAndDoesNotExposeProviderEnums` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_test.go:45`<br>`TestCatalogParametersExposeEditorContract` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_test.go:77`<br>`TestValidateFactorUseRejectsUnsupportedPurposes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/catalog_test.go:97`<br>`TestFactorDisplaySemanticsAreExplicitAndCorrected` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_edges_test.go:13`<br>`TestDefinitionHelperContractsCoverSupportedValueShapes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_edges_test.go:56`<br>`TestParameterValidationRejectsTypeRangeStepAndEnumErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_edges_test.go:94`<br>`TestUnionValidationCoversEveryProviderShape` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_edges_test.go:132`<br>`TestConditionValueValidationCoversSetRangePositionAndPattern` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_edges_test.go:199`<br>`TestDefinitionNormalizationCoversPoolsSortsAndSecondFactors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_edges_test.go:249`<br>`TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_test.go:10`<br>`TestNormalizeDefinitionPreservesParameterizedInstances` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_test.go:35`<br>`TestNormalizeDefinitionRequiresExplicitV2Versions` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_test.go:71`<br>`TestNormalizeDefinitionRejectsDuplicateConfigurationWithFieldPath` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_test.go:87`<br>`TestNormalizeDefinitionRejectsMarketIncompatibleFactor` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_test.go:103`<br>`TestNormalizeDefinitionValidatesParameterTypesEnumsAndUnions` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:pkg/researchscreen/definition_test.go:139`<br>`TestNormalizeDefinitionRejectsWrongOperatorForFactorKind` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:scripts/archive_frontend_assets_test.go:11`<br>`TestArchiveFrontendAssetsPreservesRelativePathsAndTimestamp` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:scripts/go-test-quality/main_test.go:8`<br>`TestAnalyzeSourcesRejectsCallsWithoutAssertions` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:scripts/go-test-quality/main_test.go:14`<br>`TestPublishes` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:scripts/go-test-quality/main_test.go:30`<br>`TestAnalyzeSourcesRecognizesStandardAndTestifyAssertions` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:scripts/go-test-quality/main_test.go:39`<br>`TestStandard` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:scripts/go-test-quality/main_test.go:44`<br>`TestTestify` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:scripts/go-test-quality/main_test.go:59`<br>`TestAnalyzeSourcesFollowsAssertionHelpersAcrossFiles` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:scripts/go-test-quality/main_test.go:78`<br>`TestWorkflow` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:scripts/go-test-quality/main_test.go:94`<br>`TestAnalyzeSourcesRecognizesAssertionsInsideNestedSubtests` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:scripts/go-test-quality/main_test.go:100`<br>`TestNested` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:scripts/go-test-quality/main_test.go:119`<br>`TestValidateExemptionsRejectsStaleEntries` | other | 普通边界 | `待人工归类` |
| [ ] | `go:452dea11:internal/settings/market_data_test.go:56`<br>`TestMarketDataProviderSettingsNormalizeAndApply` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/market_data_test.go:99`<br>`TestMarketDataProviderRetriesDegradedCurrentSelection` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/market_data_test.go:143`<br>`TestMarketDataProviderSettingsAcceptAKShare` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/market_data_test.go:161`<br>`TestBacktestProviderIsPreparedBeforeAtomicPersistence` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/market_data_test.go:203`<br>`TestMarketDataProviderRuntimeRollback` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/market_data_test.go:224`<br>`TestMarketDataProviderReportsPersistenceAndRollbackFailures` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/market_data_test.go:252`<br>`TestMarketDataProviderReadsWaitForRuntimeRollback` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/persistence_and_mcp_failures_test.go:55`<br>`TestServiceReportsPersistenceAndMCPFailures` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/persistence_and_mcp_failures_test.go:90`<br>`TestServiceRollsBackMCPOnSaveFailure` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/persistence_and_mcp_failures_test.go:125`<br>`TestServicePreservesSecurityAndMCPFallbacks` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_managed_accounts_test.go:13`<br>`TestServiceCreateManagedAccountNormalizesClientFields` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_managed_accounts_test.go:35`<br>`TestServiceNotificationAndMCPStatusAccessors` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_managed_accounts_test.go:58`<br>`TestServiceSystemNotificationTestUsesNarrowPublisherAndFailsClosed` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_managed_accounts_test.go:84`<br>`TestServiceDefaultMCPStatusAndTokenGeneration` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_managed_accounts_test.go:99`<br>`TestValidateWebAccessPasswordBoundaries` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_managed_accounts_test.go:111`<br>`TestServiceCreateManagedAccountRejectsBlankAccountID` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_managed_accounts_test.go:120`<br>`TestServiceOptionsCaptureBrokerDescriptorAndDefaultTradingEnvironment` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_test.go:153`<br>`TestSaveSettingsTriggersSideEffects` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_test.go:232`<br>`TestSaveSecuritySettingsRejectsInvalidWebPort` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_test.go:242`<br>`TestSaveSecuritySettingsRollsBackWhenRuntimeListenerUpdateFails` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_test.go:268`<br>`TestMCPServerTokenResetDoesNotLeakAndInvalidatesPreviousToken` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_test.go:318`<br>`TestSaveMCPServerSettingsRollsBackWhenListenerUpdateFails` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_test.go:344`<br>`TestSaveMCPServerSettingsValidatesTokenAndPort` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_test.go:361`<br>`TestConcurrentSecuritySavesPreserveNewestPasswordAndCallbackOrder` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_test.go:435`<br>`TestDefaultCallbacksReturnEmptyMaps` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_test.go:446`<br>`TestSaveIntegrationPassesStructuredConfigWithoutChangingRuntimeEnv` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_test.go:480`<br>`TestServiceDelegatesGettersAndSimpleSavers` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/settings/service_test.go:565`<br>`TestServiceDelegatesProvidersAndLifecycle` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_boundaries_test.go:51`<br>`TestFutuWatchlistReaderRemainingSourceAndReadErrors` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_boundaries_test.go:99`<br>`TestFutuWatchlistFreshAndRemoteIDRemainingBoundaries` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_boundaries_test.go:137`<br>`TestFutuSnapshotRemainingProviderRateLimitAndSplitPaths` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_boundaries_test.go:181`<br>`TestWatchlistQuoteRemainingFormattingAndTimeBoundaries` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:44`<br>`TestFutuWatchlistReaderMarksDuplicateNamesAmbiguousAndCachesReads` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:83`<br>`TestRemoteMembersKeepBrokerCodeAndSecurityIDAsSeparateAliases` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:174`<br>`TestFutuWatchlistSnapshotDoesNotSplitGlobalOrCanceledFailures` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:221`<br>`TestFutuWatchlistSnapshotUsesDelayedFallbackWhenSubscriptionQuotaIsFull` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:267`<br>`TestFutuWatchlistSnapshotUsesMarketSpecificChunksWithPerItemErrors` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:316`<br>`TestFutuWatchlistSnapshotIsolatesMarketPermissionFailures` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:357`<br>`TestFutuWatchlistSnapshotIsolatesUnknownAndOTCSymbolErrors` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:396`<br>`TestWatchlistQuotePreservesSnapshotDisplayMetadataAndAvoidsUnknownTimezoneGuess` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:418`<br>`TestWatchlistQuoteSelectsExtendedSessionPriceAndChange` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:445`<br>`TestFutuWatchlistSourceIdentityDoesNotUseTradingAccount` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:459`<br>`TestFutuWatchlistSourceReportsUnavailableRuntimeBeforeDiscovery` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/futu/source_test.go:470`<br>`TestFutuWatchlistSourceReportsFailedOpenDProbe` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/quote_preview_boundaries_test.go:12`<br>`TestBatchQuotesRejectsUnsafeInputAndHonorsCanceledFlights` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/quote_preview_boundaries_test.go:37`<br>`TestQuoteCacheAndImportHelpersKeepAbsentDataExplicit` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/quote_preview_boundaries_test.go:74`<br>`TestPreviewImportRejectsInvalidDerivedGroupName` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_quotes_test.go:62`<br>`TestBatchQuotesCachesAndSingleflightsOverlappingRequests` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_quotes_test.go:100`<br>`TestBatchQuotesTurnsBatchFailureIntoPerItemErrors` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_quotes_test.go:117`<br>`TestBatchQuotesHonorsProviderCachePolicy` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_quotes_test.go:145`<br>`TestBatchQuotesPreservesPartialResultsAndUpdatesKnownMetadata` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_quotes_test.go:174`<br>`TestChangeQuoteProviderRejectsPreviousProviderInflightResults` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_quotes_test.go:210`<br>`TestChangeQuoteProviderFailurePreservesCurrentCache` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_quotes_test.go:230`<br>`TestResetQuoteCacheRejectsPreviousProviderInflightMetadata` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_quotes_test.go:270`<br>`TestQuoteResultHelpersReturnEmptySlicesForNilValues` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_test.go:220`<br>`TestServiceCRUDNormalizesAndDelegates` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_test.go:335`<br>`TestServiceRejectsInvalidDomainInputsBeforeRepositoryWrites` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_test.go:386`<br>`TestUnavailableServiceGuardsEveryRepositoryOperation` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_test.go:431`<br>`TestServiceSynchronizesSourceHealthAndRemoteGroups` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_test.go:508`<br>`TestServiceBuildsImportPreviewFromFreshConnectorState` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_test.go:562`<br>`TestServicePreviewImportRejectsUnsafeOrUnusableSnapshots` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_test.go:645`<br>`TestServiceCommitImportRevalidatesAndConstrainsDeletes` | settings_watchlist | 普通边界 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/watchlist/service_test.go:678`<br>`TestServiceCommitImportRejectsExpiredStaleAndInvalidRequests` | settings_watchlist | 高风险 | `crates/jftrade-settings, crates/jftrade-watchlist` |
| [ ] | `go:452dea11:internal/store/backtest/adapter_lifecycle_test.go:18`<br>`TestBacktestRunStoreDirectlyImplementsDomainLifecycle` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/adapter_lifecycle_test.go:114`<br>`TestBacktestSyncTaskStoreLifecycle` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/adapter_lifecycle_test.go:140`<br>`TestBacktestRunStoreMigration` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/kline_database_test.go:9`<br>`TestCheckKLineCoverageOwnsConcreteHistoryStore` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/maintenance_concurrency_test.go:15`<br>`TestMaintenancePurgesOnlyExactTerminalSetAndCompacts` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/maintenance_concurrency_test.go:57`<br>`TestStoreConcurrentReadersAndWritersKeepIndependentState` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/maintenance_concurrency_test.go:93`<br>`TestUnavailableStoreMaintenanceFailsClosed` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/maintenance_concurrency_test.go:106`<br>`TestKLineDatabaseMaintenanceBoundaryOpensAndCompacts` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/resource_test.go:10`<br>`TestConstructorsReturnRunResourcesWithIdempotentClose` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/store_failure_test.go:15`<br>`TestStoreRejectsUnavailableIncompatibleAndCorruptDatabases` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/store_failure_test.go:62`<br>`TestStoreRollsBackMemoryWhenClosedDatabaseRejectsWrites` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/store_failure_test.go:97`<br>`TestStoreFullReadHandlesMissingRowsAndInvalidResults` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/store_failure_test.go:121`<br>`TestStoreCanceledMaintenanceDoesNotMutateRuns` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/store_failure_test.go:140`<br>`TestPersistenceDecodersRejectInvalidPayloads` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/store_test.go:12`<br>`TestStoreSnapshotsDoNotMutateRuns` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/store_test.go:52`<br>`TestStorePersistsResultsAndRecoversTransientRuns` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/store_test.go:138`<br>`TestStoredRequestDoesNotInferMissingDateMetadata` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/store_test.go:154`<br>`TestDerivePathHonorsOverrideAndSettingsDirectory` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/store_test.go:168`<br>`TestInMemoryStoreImplementsRunLifecycleAndCancellation` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/sync_tasks_test.go:12`<br>`TestSyncTaskStoreReturnsSnapshotsAndCancelsProgress` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/backtest/sync_tasks_test.go:48`<br>`TestSyncTaskStoreFinishAndNilProgressBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/snapshot_load_failures_test.go:12`<br>`TestLoadSnapshotsReportsWalkAndReadFailures` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_boundaries_test.go:13`<br>`TestCalendarStoreRejectsInvalidSnapshotPersistence` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_boundaries_test.go:31`<br>`TestCalendarStoreReportsUnavailableSnapshotDirectory` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_boundaries_test.go:45`<br>`TestCalendarStoreEmptyLoadAndDeleteAreIdempotent` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:31`<br>`TestSaveSnapshotUsesAtomicReplacement` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:73`<br>`TestStoreRootAndNilSafety` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:95`<br>`TestSaveSnapshotValidatesInputsAndResolvesYearFallbacks` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:136`<br>`TestDeleteSnapshotIgnoresMissingFilesAndReturnsRealRemoveErrors` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:169`<br>`TestWriteSnapshotPropagatesTemporaryFileDurabilityFailures` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:199`<br>`TestWriteSnapshotDefaultHooksAndDirectorySyncErrors` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:214`<br>`TestSaveSnapshotReturnsDirectoryCreationError` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_test.go:12`<br>`TestStoreRoundTripsSnapshotsAndIsolatesCorruption` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/exchangecalendar/store_test.go:58`<br>`TestStoreUsesSnapshotLocalYearForPositiveOffsetMarkets` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/research/maintenance_test.go:8`<br>`TestResearchMaintenanceCompactsLiveStoreAndFailsClosed` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/research/store_test.go:16`<br>`TestStoreScreenPresetCRUDRevisionAndRestart` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/research/store_test.go:84`<br>`TestStoreRejectsNewerSchemaAndUnavailableReceiver` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/research/store_test.go:122`<br>`TestStoreRejectsV1PresetOnRead` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/research/store_test.go:142`<br>`TestStoreReportsPathMissingRowsAndInvalidPersistedDefinitions` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/research/store_test.go:190`<br>`TestStoreOpenAndWriteErrorBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/legacy_runtime_dependencies_test.go:10`<br>`TestLegacyRuntimeDependencySettingsAreIgnoredAndDroppedOnSave` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/market_data_test.go:14`<br>`TestMarketDataProviderDefaultsToAKShareAndPersistsSelection` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/market_data_test.go:82`<br>`TestMarketDataProviderSaveRollsBackOnAtomicReplaceFailure` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/market_data_test.go:113`<br>`TestNormalizeActiveMarketDataProviderFallsBackToAKShare` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/market_data_test.go:130`<br>`TestMarketDataProviderPersistsAKShareSelection` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/market_data_test.go:145`<br>`TestBacktestProviderUpgradeCopiesGlobalSelectionOnce` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/normalization_and_persistence_test.go:13`<br>`TestSettingsNormalizationHandlesFallbacksAndBoundaries` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/normalization_and_persistence_test.go:53`<br>`TestSettingsInterfaceAndAccountNormalization` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/normalization_and_persistence_test.go:74`<br>`TestSettingsPersistenceReportsAtomicReplaceFailure` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/normalization_and_persistence_test.go:92`<br>`TestSettingsPersistenceRejectsAFileInItsDirectoryPath` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/normalization_and_persistence_test.go:114`<br>`TestSettingsStorePersistsValuesAndManagesAccountLifecycle` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/normalization_and_persistence_test.go:178`<br>`TestSettingsFileReportsInterfaceAndLoadFailures` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/persist_failures_test.go:26`<br>`TestPersistLockedPropagatesTemporaryFileDurabilityFailures` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/rollback_test.go:13`<br>`TestFailedSettingSavesRollbackAllRuntimeState` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/rollback_test.go:185`<br>`TestFailedBootstrapAndMigrationRollbackRuntimeState` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/rollback_test.go:232`<br>`TestFailedManagedAccountCRUDRollsBackBackingArray` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_persistence_contracts_test.go:15`<br>`TestStoreDefaultsExposePathAndNormalizedDefaults` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_persistence_contracts_test.go:63`<br>`TestMCPServerSettingsPersistVerifierWithoutPublicLeak` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_persistence_contracts_test.go:105`<br>`TestSaveAppearanceAndADKSettingsPersistNormalizedValues` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_persistence_contracts_test.go:166`<br>`TestSaveSystemNotificationSettingsNormalizesAndPersists` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_persistence_contracts_test.go:203`<br>`TestSystemNotificationSettingsImportantUsesDefaults` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_persistence_contracts_test.go:217`<br>`TestSavePineWorkerSettingsPersistsNormalizedWorkerLimits` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_persistence_contracts_test.go:261`<br>`TestSaveExchangeCalendarSettingsNormalizesPoliciesAndOverrides` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_persistence_contracts_test.go:357`<br>`TestExchangeCalendarErrorNotificationSettingPreservesExplicitFalse` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_persistence_contracts_test.go:397`<br>`TestManagedAccountLifecyclePreservesScopeAndHandlesMissingIDs` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_recovery_test.go:13`<br>`TestSettingsStoreRejectsMalformedOrUnreadableInput` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_recovery_test.go:27`<br>`TestEnsureBootstrapFileRepairsExistingSettingsWithoutAppearance` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_recovery_test.go:55`<br>`TestSettingsStoreReadsPersistedConfigurationBranches` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_recovery_test.go:87`<br>`TestUnknownSecurityFieldsAreIgnoredWithoutRewritingSettings` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_recovery_test.go:111`<br>`TestFailedSecurityReplaceKeepsDiskAndRuntimeStateUnchanged` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_recovery_test.go:158`<br>`TestFailedMCPServerReplaceKeepsDiskAndRuntimeStateUnchanged` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_test.go:13`<br>`TestEnsureBootstrapFileInitializesDefaults` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_test.go:40`<br>`TestSettingsPersistenceAndNormalization` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_test.go:83`<br>`TestSaveIntegrationPersistsWithoutChangingRuntimeEnv` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_test.go:136`<br>`TestManagedAccountsDefaults` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_test.go:154`<br>`TestCreateManagedAccountRequiresAccountIDAndOwnsServerFields` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_test.go:184`<br>`TestNormalizeExchangeCalendarSettingsRewritesLegacySourceIDs` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/settingsfile/store_test.go:205`<br>`TestDefaultExchangeCalendarSettingsUseNYSEAsOnlyDefaultUSRemoteSource` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/conn_test.go:12`<br>`TestOpenXConfiguresBusyTimeoutAndConcurrentReadsByDefault` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/conn_test.go:47`<br>`TestOpenXCanEnableConcurrentReadConnections` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/conn_test.go:64`<br>`TestDSNAppendsPragmasToExistingQuery` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/conn_test.go:76`<br>`TestReadDSNEnforcesQueryOnlyConnections` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/conn_test.go:87`<br>`TestOpenCreatesUsableSQLiteDatabaseWithConfiguredPool` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/conn_test.go:125`<br>`TestOpenFunctionsRejectBlankPath` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/conn_test.go:134`<br>`TestOpenFunctionsPropagateDriverOpenErrors` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/conn_test.go:163`<br>`TestResolveOptionsNormalizesConnectionPoolBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/conn_test.go:186`<br>`TestReadOnlyDSNAddsModeWithoutWritePragmas` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/conn_test.go:203`<br>`TestForeignKeysAndCascadesAreEnforcedAcrossDatabaseConnections` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/coordinator_test.go:11`<br>`TestWriteCoordinatorOrdersWritersAndReadBarriers` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/coordinator_test.go:44`<br>`TestWriteCoordinatorAllowsAdmittedReadsToOverlapLaterWrites` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/coordinator_test.go:58`<br>`TestWriteCoordinatorCancellationDoesNotBlockFollowingWork` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/coordinator_test.go:83`<br>`TestWriteCoordinatorReadBarrierHonorsCancellation` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/coordinator_test.go:94`<br>`TestCoordinatorRegistryNormalizesDatabasePaths` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/coordinator_test.go:112`<br>`TestCoordinatorRegistryReleasesLastDatabaseReference` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/coordinator_test.go:156`<br>`TestCoordinatorRegistryDefersRemovalUntilOutstandingWriteFinishes` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/db_api_test.go:12`<br>`TestDatabaseReadAndWriteAPI` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/db_api_test.go:109`<br>`TestDatabaseTransactionsAndCancellationBoundaries` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/db_api_test.go:178`<br>`TestDatabaseOpenReadOnlyAndClosedPoolErrors` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/db_api_test.go:218`<br>`TestSQLStatementClassification` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/db_concurrency_test.go:10`<br>`TestDatabaseUsesSeparateConcurrentReadPoolAndSerialWritePool` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/db_concurrency_test.go:46`<br>`TestDatabaseReadWaitsForPreviouslyQueuedWrite` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/db_concurrency_test.go:80`<br>`TestDatabaseQueuedWritesPreserveSubmissionOrder` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/db_concurrency_test.go:113`<br>`TestDatabaseReadBarrierHonorsContextCancellation` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteconn/maintenance_test.go:8`<br>`TestCompactSerializesWithConcurrentWritesAndFailsAfterClose` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:14`<br>`TestCatalogLookupsReturnDefensiveCopies` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:60`<br>`TestCurrentCatalogInitializesAndValidatesEveryDatabase` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:79`<br>`TestCatalogRejectsUnknownIDsAndInvalidPreflightPaths` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:98`<br>`TestValidateCurrentDetectsManifestDrift` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:130`<br>`TestValidateDefinitionSupportsOnlyMatchingDynamicTables` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:155`<br>`TestDefinitionConstructionAndComparisonFailurePaths` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:198`<br>`TestValidateDefinitionReportsBuildAndInspectionFailures` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:232`<br>`TestCatalogInspectionPropagatesDatabaseFailures` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:253`<br>`TestValidateIntegrityDetectsForeignKeyViolations` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:266`<br>`TestValidateIntegrityFailureResults` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:289`<br>`TestValidateDefinitionWrapsIntegrityViolations` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:308`<br>`TestInspectTablePropagatesEachManifestQueryFailure` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:329`<br>`TestValidateMetadataRejectsAdditionalComponentRows` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:344`<br>`TestValidateCurrentPropagatesMetadataVersionDrift` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:359`<br>`TestBacktestV2MetadataRequiresRebuildForV3` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:375`<br>`TestInspectIndexesPropagatesIndexColumnScanFailure` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/catalog_test.go:383`<br>`TestInitializeMetadataPublicBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_boundaries_test.go:30`<br>`TestIncompatibleErrorIncludesRecoveryContext` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_boundaries_test.go:37`<br>`TestInitializeOrValidateRejectsUnavailableAndInvalidDatabasePaths` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_boundaries_test.go:58`<br>`TestInitializeOrValidateHandlesBlankStatementsAndValidationErrors` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_boundaries_test.go:85`<br>`TestInitializeOrValidateUsesManagedSQLiteControllerTransaction` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_boundaries_test.go:107`<br>`TestInitializeOrValidateRejectsDatabaseWithoutManagedWriteTransaction` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_boundaries_test.go:115`<br>`TestValidateMetadataClassifiesMissingAndUnreadableComponentRows` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_boundaries_test.go:147`<br>`TestDatabaseAndTableValidationFilesystemBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_boundaries_test.go:170`<br>`TestInitializeOrValidateRollsBackWhenDeferredConstraintFailsAtCommit` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_fault_driver_test.go:21`<br>`TestInitializeOrValidateReportsMetadataInsertFailure` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_fault_driver_test.go:33`<br>`TestValidateTablePropagatesRowsScanIterationAndCloseFailures` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_fault_driver_test.go:57`<br>`TestCloseRowsPreservesPrimaryErrorAndReportsCloseOnlyFailure` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_test.go:14`<br>`TestInitializeOrValidateStrictSchemaForAllDatabases` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/sqliteschema/schema_test.go:99`<br>`TestNewDatabaseFailureDoesNotLeaveSchemaMetadata` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/persistence_contracts_test.go:15`<br>`TestStrategyDesignPersistencePathsAndSerializationContracts` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/resource_test.go:10`<br>`TestConstructorsReturnDesignResourcesWithIdempotentClose` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/runtime_activity_test.go:39`<br>`TestNewStoreCreatesExpectedSchema` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/runtime_activity_test.go:53`<br>`TestStrategySchemaHelpersIncludeImmutableDefinitionVersions` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/runtime_activity_test.go:85`<br>`TestNewStoreRejectsLegacySchema` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/runtime_activity_test.go:107`<br>`TestStoreRoundTripsLogsAuditAndObservation` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/runtime_activity_test.go:218`<br>`TestStoreInputAndMissingObservationBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/runtime_activity_test.go:254`<br>`TestStorePathPaginationAndClosedDatabaseBoundaries` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:130`<br>`TestStrategyDesignStoreIgnoresLegacyJSONFile` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:175`<br>`TestStrategyDesignStoreSaveDefinitionManagesVersionAndScriptMetadata` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:251`<br>`TestStrategyDesignStorePersistsImmutableDefinitionVersionSnapshots` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:338`<br>`TestStrategyDesignStoreRollsBackDefinitionWhenSnapshotInsertFails` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:382`<br>`TestStrategyDesignStoreRejectsV1DatabaseWithoutMigration` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:410`<br>`TestStrategyDesignStoreRejectsLegacyRuntimeSourceAndVisualBlocks` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:477`<br>`TestStrategyDesignStoreGeneratesUUIDWhenIDMissing` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:504`<br>`TestStrategyDesignStoreDeleteDefinitionSoftDeletes` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:545`<br>`TestStrategyDesignStoreRejectsCorruptRowsAndClosedOperations` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:592`<br>`TestStrategyDesignNormalizationKeepsRunnableDefaultsAndRejectsUnpersistableModels` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:621`<br>`TestNormalizeStrategyRuntimeUsesPineTSAndMigratesLegacy` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:636`<br>`TestStrategyDesignStorePublicMethodsMapBusinessErrors` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/strategy/store_test.go:662`<br>`TestStrategyDesignStoreMaintenanceRejectsStaleCandidatesAndCompacts` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/broker_fill_reconciliation_test.go:11`<br>`TestExecutionOrderStoreReconcilesFillBeforeOrderSnapshot` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/broker_fill_reconciliation_test.go:97`<br>`TestExecutionOrderStoreDoesNotDoubleCountSnapshotCoveredFill` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/broker_fill_reconciliation_test.go:144`<br>`TestExecutionOrderStoreRejectsStaleBrokerSnapshotRegression` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/broker_ledger_test.go:10`<br>`TestBrokerSnapshotCoveredFillQuantityLedgerBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/broker_ledger_test.go:50`<br>`TestBrokerEventCoverageTimestampBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/broker_ledger_test.go:65`<br>`TestBrokerFeeNilAndMissingOrderBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/execution_composition_test.go:32`<br>`TestExecutionOrderStorePromotesBrokerSourceToSystemOnPlacedMerge` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/execution_composition_test.go:70`<br>`TestExecutionOrderStorePersistsOrdersEventsAndFillKeys` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/execution_composition_test.go:153`<br>`TestExecutionOrderStoreBrokerSyncUpdatesAndFillDeduplication` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/execution_composition_test.go:313`<br>`TestExecutionOrderStorePlacedMergeCancelAndFiltering` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/fill_retention_test.go:8`<br>`TestSeenFillRetentionPrunesExpiredKeysAndBoundsConfiguration` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/ledger_lifecycle_test.go:12`<br>`TestExecutionStoreRemainingMergeTimestampAndFillKeyBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/ledger_lifecycle_test.go:41`<br>`TestExecutionStoreRemainingSnapshotIdentityAndFillDefaults` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/ledger_lifecycle_test.go:73`<br>`TestExecutionStorePersistsParentBrokerFeesWithoutInventingLegAllocation` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/ledger_lifecycle_test.go:110`<br>`TestBrokerSnapshotDoesNotDowngradePreviewLockedProduct` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/ledger_lifecycle_test.go:131`<br>`TestExecutionStoreRemainingPersistenceLifecycleBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/ledger_test.go:12`<br>`TestExecutionOrderStoreSortingFilteringAndMissingOrderBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/ledger_test.go:65`<br>`TestExecutionOrderStorePlacedOrderPreservesMissingBrokerAndRepairsSparseSummary` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/ledger_test.go:150`<br>`TestExecutionOrderStoreBrokerSyncPreservesMissingBrokerAndRepairsIncompleteSummary` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/ledger_test.go:236`<br>`TestExecutionOrderStorePersistenceWorkerQueueAndFallbackPaths` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/ledger_test.go:330`<br>`TestExecutionOrderSQLiteStoreIgnoresBlankIdentifiersAndZeroCutoff` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/maintenance_concurrency_test.go:13`<br>`TestExecutionMaintenanceReportsBusyAndCompactsAvailableDatabase` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/maintenance_concurrency_test.go:43`<br>`TestExecutionStoreConcurrentReadsWritesAndDurableReload` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/order_leg_merge_test.go:10`<br>`TestExecutionLegSnapshotsMergeAppendAndNormalizeLifecycle` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/out_of_order_reconciliation_test.go:43`<br>`TestExecutionOrderStoreIgnoresOutOfOrderRegressionPushAfterTerminalState` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/out_of_order_reconciliation_test.go:96`<br>`TestExecutionOrderStoreAppliesFillProgressFromOlderSnapshot` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/out_of_order_reconciliation_test.go:140`<br>`TestExecutionOrderStoreKeepsUpdatedAtMonotonicForOlderFill` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/out_of_order_reconciliation_test.go:178`<br>`TestExecutionOrderStoreResolvesCancelRequestRaceAgainstBrokerPush` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/out_of_order_reconciliation_test.go:227`<br>`TestExecutionOrderStoreDuplicateTerminalPushIsNoOp` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/persistence_failures_test.go:14`<br>`TestExecutionPersistenceConstructorDependencyFailures` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/persistence_failures_test.go:58`<br>`TestExecutionPersistenceLoadsStoredSequenceHighWaterMarks` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/persistence_query_plan_test.go:12`<br>`TestExecutionEventLoadUsesOrderIndexAndPreservesPerOrderChronology` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/resource_test.go:10`<br>`TestConstructorsReturnExecutionResourcesWithIdempotentClose` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/snapshot_normalization_test.go:11`<br>`TestBrokerSnapshotNormalizesIdentityAndQuantities` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/snapshot_normalization_test.go:44`<br>`TestExecutionTimestampCutoffBoundaries` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/startup_compatibility_test.go:14`<br>`TestExecutionOrderDatabasePathResolution` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/startup_compatibility_test.go:31`<br>`TestExecutionOrderPersistenceRejectsV1WithoutMutatingFile` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/startup_compatibility_test.go:73`<br>`TestExecutionOrderPersistenceRejectsInvalidPaths` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/startup_compatibility_test.go:87`<br>`TestExecutionOrderPersistenceRejectsPartialLegacySchema` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/startup_compatibility_test.go:105`<br>`TestExecutionOrderPersistenceRejectsWrongColumnLayout` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/startup_compatibility_test.go:131`<br>`TestExecutionPersistenceNilLifecycleAndSequenceSuffix` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/startup_compatibility_test.go:156`<br>`TestExecutionOrderPersistenceLoadRejectsMissingRuntimeTables` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/submission_safety_test.go:13`<br>`TestExecutionSubmissionLedgerDeduplicatesAndNeverRetriesUnknown` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/submission_safety_test.go:63`<br>`TestExecutionPreviewConsumptionIsIdempotentOnlyForIdenticalClientRequest` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/trading/submission_safety_test.go:117`<br>`TestPredictionRFQPersistsBindingExpiryAndSingleConsumption` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/delete_transaction_rollback_test.go:9`<br>`TestDeleteGroupNeverPartiallyRemovesWatchlistState` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/delete_transaction_rollback_test.go:84`<br>`TestRemoteReplacementAndBindingRemovalKeepPriorStateOnStorageFailure` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/import_persistence_boundaries_test.go:13`<br>`TestCommitImportPersistsNewAndExistingBindings` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/import_persistence_boundaries_test.go:130`<br>`TestImportPreviewStorageRejectsCorruptionAndMissingRecords` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/import_persistence_boundaries_test.go:172`<br>`TestImportStorageRollsBackRemoteGroupConflictsAndInvalidOpenPath` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/import_storage_faults_test.go:10`<br>`TestCommitImportRollsBackEachPersistenceStage` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/import_test.go:47`<br>`TestImportPreviewCommitRepeatAndStaleGuards` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/import_test.go:150`<br>`TestImportCommitRejectsConnectorWithoutFreshRevalidation` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/import_test.go:174`<br>`TestImportSupportsMultipleSourcesExpiryAndLocalOnlyUnbind` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/import_test.go:239`<br>`TestDuplicateRemoteGroupNamesAreAmbiguous` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/items_query_plan_test.go:12`<br>`TestGroupedListItemsPreservesOrderingPaginationAndFilters` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/items_query_plan_test.go:70`<br>`TestGroupedListItemsQueryUsesMembershipPrimaryKeyRange` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/maintenance_test.go:8`<br>`TestWatchlistMaintenanceCompactsLiveStoreAndFailsClosed` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/storage_failure_boundaries_test.go:14`<br>`TestCommitImportRejectsStaleAndBrokenStorageStates` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/storage_failure_boundaries_test.go:126`<br>`TestImportStoreDetectsStaleCompletionAndCursorStorageFailure` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/storage_failure_boundaries_test.go:151`<br>`TestItemStorageBoundaryPaths` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/storage_failure_boundaries_test.go:213`<br>`TestMembershipStorageFaultsAndMigrationGuards` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/storage_failure_boundaries_test.go:292`<br>`TestMembershipDiffStorageFailuresRollback` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/store_availability_and_filters_test.go:12`<br>`TestNilStoreOperationsReturnUnavailable` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/store_availability_and_filters_test.go:44`<br>`TestClosedStoreOperationsSurfaceDatabaseErrors` | storage_sqlite | 高风险 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/store_availability_and_filters_test.go:89`<br>`TestStorePureHelpersCoverBoundaryInputs` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/store_availability_and_filters_test.go:164`<br>`TestListItemsCoversMarketAndCursorFilters` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/store_test.go:14`<br>`TestStoreDefaultGroupMembershipsAndRestart` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/store_test.go:100`<br>`TestReplaceMembershipsRollsBackNewGroupsAndInstrumentOnConflict` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/store_test.go:122`<br>`TestSnapshotMetadataEnrichesExistingInstrumentWithoutChangingMembershipRevision` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/store_test.go:150`<br>`TestListItemsBatchHydratesGroupsAndSources` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/store_test.go:196`<br>`TestStoreGroupUpdateAndDeletePreserveMembershipRevisionContract` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/store/watchlist/store_test.go:253`<br>`TestStoreRoundTripsSourceAndRemoteGroupSnapshots` | storage_sqlite | 普通边界 | `crates/jftrade-store-sqlite, crates/jftrade-store-settings-file, crates/jftrade-owner-lock` |
| [ ] | `go:452dea11:internal/pineworkerassets/asset_selection_boundaries_test.go:13`<br>`TestSelectFromFSReturnsEmbeddedBundleMetadata` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/pineworkerassets/asset_selection_boundaries_test.go:30`<br>`TestSelectFromFSTreatsMissingAndEmptyBundlesAsUnavailable` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/pineworkerassets/asset_selection_boundaries_test.go:52`<br>`TestSelectFromFSReturnsUnexpectedReadError` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/pineworkerassets/asset_selection_boundaries_test.go:63`<br>`TestIsMissingAssetRecognizesOnlyNotFoundErrors` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/pineworkerassets/assets_dev_test.go:7`<br>`TestSelectReturnsUnavailableWhenAssetMissing` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/pineworkerassets/assets_release_test.go:13`<br>`TestSelectReturnsEmbeddedBundleWhenStaged` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/pineworkerassets/assets_test.go:5`<br>`TestBundleNameIsPlatformIndependent` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/activity_degraded_test.go:65`<br>`TestCatalogActivityReturnsEmptyPagesWhenActivityStoreIsUnavailable` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/catalog_boundary_behavior_test.go:34`<br>`TestCatalogActivityQueryFailureReturnsKnownEmptyPage` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/catalog_boundary_behavior_test.go:58`<br>`TestCatalogActivityWriteFailureDoesNotBlockControlState` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/catalog_boundary_behavior_test.go:84`<br>`TestCatalogDefinitionSyncExplainsLatestRefreshableAndBusyStates` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/catalog_boundary_behavior_test.go:123`<br>`TestCatalogNormalizationAndClonePreserveCallerIsolation` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/catalog_boundary_behavior_test.go:192`<br>`TestCatalogPrivateBusinessHelpersHandleEmptyAndUnknownInputs` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/instance_lifecycle_business_test.go:11`<br>`TestCatalogInstanceCreateUpdateAndDeleteRespectStoppedBoundary` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/instance_lifecycle_business_test.go:89`<br>`TestCatalogInstanceOperationsClassifyInvalidAndMissingResources` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/instance_lifecycle_business_test.go:143`<br>`TestCatalogDefinitionRefreshPreservesPlacementAndClassifiesLinkedInstances` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/instance_lifecycle_business_test.go:189`<br>`TestCatalogRefreshInstanceDefinitionUsesConfiguredDefinitionStore` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/plugin_normalization_business_test.go:13`<br>`TestCatalogPluginLifecyclePersistsSortedMetadataAndOperations` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/plugin_normalization_business_test.go:89`<br>`TestCatalogPluginLifecycleClassifiesMissingResource` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/plugin_normalization_business_test.go:99`<br>`TestCatalogNormalizesLegacySnapshotAndDropsRuntimeOnlyFields` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/plugin_normalization_business_test.go:157`<br>`TestCatalogNormalizationPreservesExplicitUnsupportedRuntimeForValidation` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/plugin_normalization_business_test.go:186`<br>`TestCatalogPluginCompatibilityAndUninstallCommandsDescribeHostBoundary` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/repository_failure_business_test.go:11`<br>`TestCatalogConstructionPropagatesRepositoryLoadFailure` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/repository_failure_business_test.go:19`<br>`TestCatalogFailedSavesLeaveDurableRepositorySnapshotUnchanged` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/repository_failure_business_test.go:126`<br>`TestCatalogReturnsIndependentCopiesAcrossRepositoryAndCallers` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:12`<br>`TestCatalogRuntimeTransitionsPersistStateAndActivity` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:52`<br>`TestCatalogRuntimeFailureReconcilesOnlyRunningInstance` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:80`<br>`TestCatalogStartupReconcileResetsStaleRunningAndPausedState` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:113`<br>`TestCatalogActivitySupportsPagingFilteringAndRuntimeObservationEnrichment` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/errors_test.go:8`<br>`TestClassifiedStrategyErrorsMatchSentinelKinds` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instancebinding/binding_test.go:11`<br>`TestNormalizeBindingPrefersExplicitInstruments` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instancebinding/binding_test.go:28`<br>`TestNormalizeBindingBackfillsLegacyParams` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instancebinding/binding_test.go:65`<br>`TestApplyParamsWritesCanonicalBindingFields` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instancebinding/binding_test.go:96`<br>`TestNormalizeBindingPreservesSupportedChartType` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instancebinding/binding_test.go:106`<br>`TestRiskAndBindingAuditDetails` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instancebinding/binding_test.go:137`<br>`TestNormalizeBrokerAccountDropsEmptyInput` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instancebinding/binding_test.go:143`<br>`TestNormalizeBindingAcceptsLegacyArrayPayloadsAndDropsInvalidEntries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instancebinding/binding_test.go:161`<br>`TestBindingConversionBoundaryTypes` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instancebinding/binding_test.go:202`<br>`TestApplyParamsHandlesNilAndClearsStaleOptionalFields` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instanceview/runtime_projection_test.go:9`<br>`TestInstanceViewHandlesUntypedRuntimeAndNilParams` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instanceview/view_test.go:13`<br>`TestRuntimeAndSourceFormatFromParams` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instanceview/view_test.go:28`<br>`TestDefinitionIDFromParamsTrimsStringValues` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instanceview/view_test.go:46`<br>`TestStartableRequiresPineV6AndPineRuntime` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instanceview/view_test.go:62`<br>`TestToInstanceViewNormalizesBindingAndCopiesParams` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/instanceview/view_test.go:92`<br>`TestBuildInstanceIDUsesDefinitionOrDefaultPrefix` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/live_command_business_boundaries_test.go:14`<br>`TestDefaultPineProducesEscapedCanonicalStarterStrategy` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/live_command_business_boundaries_test.go:33`<br>`TestServiceDelegatesDefinitionVersionHistoryWithIdentityPreserved` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/live_command_business_boundaries_test.go:57`<br>`TestCommandsFromOrderIntentsRejectsInvalidAtomicOCOLegs` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/live_command_business_boundaries_test.go:85`<br>`TestWorkerIntentDirectionAliasesPreserveTradingSide` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/live_command_business_boundaries_test.go:127`<br>`TestExecuteBarCommandsPreflightsBeforeBrokerSideEffects` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/live_command_business_boundaries_test.go:204`<br>`TestAtomicPineOrderValidationRejectsUnsafeGroupShapes` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/live_command_business_boundaries_test.go:362`<br>`TestAtomicPineOrderSubmissionIsAllOrNothing` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/live_command_business_boundaries_test.go:399`<br>`TestPositionAwareCloseNeverCrossesTheWrongSide` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/live_command_business_boundaries_test.go:468`<br>`TestIgnoredOrderWarningsRetainFallbackIdentityAndSymbol` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/live_command_business_boundaries_test.go:490`<br>`TestLiveOrderQuantityRespectsMinimumAndPrecision` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/live_command_business_boundaries_test.go:545`<br>`TestCancelByIntentDeduplicatesAliasesAndToleratesStaleMappings` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_boundaries_test.go:16`<br>`TestManagerMaintenanceStateAndPollingConfiguration` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_boundaries_test.go:54`<br>`TestManagerCompatibilityResolversAndCommandBoundariesFailClosed` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_boundaries_test.go:94`<br>`TestStrategyRuntimeRequiresStreamingCandlesForLiveAndNotifyOnly` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_boundaries_test.go:116`<br>`TestStrategyRuntimeRejectsUnhealthyActiveProviderUnlessExchangeOverrideOwnsHealth` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_boundaries_test.go:155`<br>`TestStrategyRuntimeResolvesExactBoundBrokerWithoutLegacyFallback` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_boundaries_test.go:182`<br>`TestLiveStrategyRequiresExplicitBrokerAccountBinding` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_boundaries_test.go:216`<br>`TestNotifyOnlyLoadsRealtimeMarketDataWithoutBrokerAccount` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_boundaries_test.go:241`<br>`TestManagerInputLoadingReportsEachUnavailableDependency` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_boundaries_test.go:275`<br>`TestManagerActivationReservationAndUnknownTradeBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_boundaries_test.go:302`<br>`TestManagerBuildSymbolRequiresMarketAndPineWorker` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_boundaries_test.go:356`<br>`TestRuntimeBuildCallbacksRecordErrorsAndIgnoredOrders` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_close_test.go:86`<br>`TestManagerCloseAggregatesNamedSessionErrorsOnce` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_close_test.go:140`<br>`TestManagerCloseWaitsForInFlightStartAndCollectsItsCloseError` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/manager_close_test.go:188`<br>`TestManagerCloseJoinsBackgroundSyncBeforeClosingPineSession` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/nil_boundaries_test.go:10`<br>`TestNilRuntimeBoundariesReturnEmptyState` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/nil_boundaries_test.go:43`<br>`TestTradeCommandFuncsRequireCallbacksAndDelegateCommands` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/order_risk_business_test.go:18`<br>`TestLiveOrderPassesStopPriceToExecutionGateway` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/order_risk_business_test.go:67`<br>`TestRuntimeRiskEvaluatesOrderLimits` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/order_risk_business_test.go:135`<br>`TestLiveCancelOnlyRemovesSuccessfullyCancelledTrackedOrders` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/order_risk_business_test.go:216`<br>`TestMarketDayStartUsesOrderSymbolTimezone` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/order_risk_business_test.go:235`<br>`TestSubmittedOrderCountKeepsInstanceScopeWithinMarketDay` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/pineworker_live_business_test.go:226`<br>`TestPineWorkerLiveUsesStatefulSessionAfterWarmup` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/pineworker_live_business_test.go:270`<br>`TestPineWorkerLiveRemainingConstructorAndWarmupErrors` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/pineworker_live_business_test.go:305`<br>`TestPineWorkerLiveRemainingMarketAndSizerBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/pineworker_live_business_test.go:368`<br>`TestPineWorkerLiveRemainingEquityPriceAndParamBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/pineworker_live_business_test.go:404`<br>`TestPineWorkerLiveSessionFailureBoundaries` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/pineworker_live_business_test.go:527`<br>`TestLiveWarningSinkRecordsOnlyActionableOrderWarnings` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/product_lifecycle_business_test.go:18`<br>`TestRuntimeSnapshotIdentityAndAccountHelpers` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/product_lifecycle_business_test.go:56`<br>`TestRuntimeRejectsBeforeAnyBrokerSubmission` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/product_lifecycle_business_test.go:96`<br>`TestRuntimePropagatesGatewayFailureAndSortsObservations` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/product_lifecycle_business_test.go:151`<br>`TestRuntimeAccountAcceptsBlankCurrencyBalance` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/runtime_boundaries_test.go:21`<br>`TestStrategyRuntimeManagerUsesExplicitDependenciesInsteadOfServerOwnership` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/runtime_boundaries_test.go:48`<br>`TestStrategyRuntimeManagerStartValidationAndReservationBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/runtime_boundaries_test.go:116`<br>`TestStrategySymbolRuntimeTradeBucketsAndOrderSignals` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/runtime_boundaries_test.go:180`<br>`TestStrategyRuntimeAccountAndFormattingBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/runtime_boundaries_test.go:258`<br>`TestStrategyRuntimeSymbolMarketAndStartErrorBoundaries` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/runtime_boundaries_test.go:277`<br>`TestStrategyRuntimeRefreshAndSyncErrorBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/runtime_risk_evidence_test.go:18`<br>`TestRuntimeRiskRejectionRecordsAuditAndPauseTransition` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/subscription_lifecycle_test.go:14`<br>`TestSubscriptionLeaseAndWarmupFailuresRollBackRuntime` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/subscription_lifecycle_test.go:58`<br>`TestRuntimePanicReleasesSubscriptionLease` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/subscription_lifecycle_test.go:81`<br>`TestKLineSubscriptionRefsSkipMalformedSymbols` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/symbol_failure_business_test.go:14`<br>`TestStrategyRuntimeSymbolKeepsPollingAndLateTradeFailuresVisible` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/liveruntime/symbol_failure_business_test.go:57`<br>`TestStrategyRuntimeSymbolFallbacksAvoidPanicsDuringShutdown` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_command_test.go:16`<br>`TestCandleFromKLinePreservesClosedBarWireFields` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_command_test.go:36`<br>`TestCommandsFromOrderIntents` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_command_test.go:60`<br>`TestCommandFromOrderIntentPreservesShortDirection` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_command_test.go:76`<br>`TestCommandFromOrderIntentMapsShortExitToBuy` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_command_test.go:93`<br>`TestScopedExitQuantitySurvivesGoExecution` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_command_test.go:130`<br>`TestCommandFromOrderIntentMapsConditionalOrderTypes` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_command_test.go:180`<br>`TestCommandFromOrderIntentRejectsUnsupportedExitBracket` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_command_test.go:194`<br>`TestCommandsFromOrderIntentsExpandsAtomicOCOExit` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_command_test.go:227`<br>`TestCommandFromOrderIntentCanonicalizesSellEntryAsShort` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_command_test.go:257`<br>`TestCommandFromOrderIntentDefaultsEntryQuantity` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_command_test.go:267`<br>`TestCommandFromOrderIntentRejectsUnsupportedIntent` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:14`<br>`TestNewLiveCommandExecutorRetainsInjectedControlPorts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:36`<br>`TestLiveCommandExecutorSubmitsOrders` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:66`<br>`TestLiveCommandExecutorRejectsQuantityPctWithoutSizing` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:80`<br>`TestLiveCommandExecutorRejectsMissingQuantity` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:93`<br>`TestLiveCommandExecutorSizesEntryQuantityPctFromEquity` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:119`<br>`TestLiveCommandExecutorSizesCloseQuantityPctFromPosition` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:150`<br>`TestLiveCommandExecutorDefaultsCloseToFullPosition` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:180`<br>`TestLiveCommandExecutorTagsShortOrders` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:198`<br>`TestLiveCommandExecutorIgnoresCloseWithoutPosition` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:226`<br>`TestLiveCommandExecutorIgnoresQuantityBelowMarketStep` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:255`<br>`TestLiveCommandExecutorIgnoresHKOddLotBelowBoardLot` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:284`<br>`TestLiveCommandExecutorIgnoresOrdersWhenMarketRulesUnavailable` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:314`<br>`TestLiveCommandExecutorGroupsRepeatedIgnoredOrderWarnings` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:349`<br>`TestLiveCommandExecutorAutoCloseCoversShortPosition` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:383`<br>`TestLiveCommandExecutorHonorsExplicitShortCloseAndOptionalWarnings` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:428`<br>`TestLiveCommandExecutorCancelsTrackedOrders` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:443`<br>`TestLiveCommandExecutorCancelAll` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:462`<br>`TestLiveCommandExecutorRejectsAtomicBracketBeforeAnySubmission` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:474`<br>`TestLiveCommandExecutorSubmitsParentOCOBracketAtomically` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:508`<br>`TestLiveCommandExecutorRejectsMalformedAtomicBracket` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:532`<br>`TestLiveCommandExecutorPropagatesExecutorErrors` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:556`<br>`TestLiveCommandExecutorBusinessBoundaryErrors` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:601`<br>`TestLiveCommandExecutorGeneratedOrderIDStopsAndTrackingFallbacks` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pine_live_executor_test.go:648`<br>`TestLiveCommandExecutorCancelBoundaries` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/recovery_contracts_test.go:9`<br>`TestManagerReconfigureRejectsNilManager` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runner_lifecycle_test.go:41`<br>`TestLiveSessionContextWatcherExitsWhenSessionCloses` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runner_lifecycle_test.go:59`<br>`TestOpenLiveSessionCleansUpWhenRunnerClosesBeforeRegistration` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runner_lifecycle_test.go:110`<br>`TestLiveSessionContextWatcherCancellationBoundaries` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runner_lifecycle_test.go:157`<br>`TestLiveSessionCloseInitializesCompletionSignal` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_failure_contracts_test.go:16`<br>`TestResolveConfigReportsUnavailableEmbeddedBundle` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_failure_contracts_test.go:30`<br>`TestResolveConfigHonorsRuntimeFallbacksAndConfiguredWorkerLimits` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_failure_contracts_test.go:52`<br>`TestResolveWorkDirFindsRepositoryFromExternalBundleAfterGetwdFailure` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_failure_contracts_test.go:76`<br>`TestManagerDoesNotPublishWhenBacktestRunnerCannotBeBuilt` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_failure_contracts_test.go:99`<br>`TestBacktestRunnerWaitsForCapacityAndPropagatesStartupFailures` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_failure_contracts_test.go:146`<br>`TestRunnerAndSessionNilAndClosedLifecycleBoundaries` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_failure_contracts_test.go:171`<br>`TestManagerCloseDrainsActiveLiveSession` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:18`<br>`TestResolveConfigUsesEnvironmentAndSettings` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:60`<br>`TestResolveConfigSelectsEmbeddedAssetAndExternalOverride` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:79`<br>`TestResolveConfigDisabledAndInvalidLimits` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:105`<br>`TestResolveConfigUsesWorkerDefaultsAndRuntimePrecedence` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:118`<br>`TestResolveConfigFindsRepositoryAndHonorsProtoOverride` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:145`<br>`TestRuntimeDependencyOptionsAndDefaultFactories` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:165`<br>`TestRuntimePathAndWorkDirFallbacks` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:185`<br>`TestManagerBuildsPublishesAndRetiresRunnerPairs` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:228`<br>`TestManagerDoesNotPublishPartialPairAndRollsBack` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:250`<br>`TestManagerNilClosedAndNonClosableRunnerBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:278`<br>`TestEphemeralRunnerConcurrencyAndLiveSessionLifecycle` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:325`<br>`TestEphemeralRunnerFailureBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/pineruntime/runtime_test.go:344`<br>`TestEphemeralRunnerOpenSessionFailureBoundaries` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/runtimecontrol/optional_values_risk_off_test.go:8`<br>`TestOptionalRuntimeControlValuesCoverTimeAndPositionEdges` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/runtimecontrol/optional_values_risk_off_test.go:33`<br>`TestEvaluateRiskOffModeIgnoresConfiguredLimits` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/runtimecontrol/policy_test.go:11`<br>`TestEvaluateRiskAppliesRuntimeLimits` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/runtimecontrol/policy_test.go:53`<br>`TestEvaluateRiskMonitorModeRecordsButDoesNotReject` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/runtimecontrol/policy_test.go:68`<br>`TestNormalizeRiskSettingsClearsOffModeLimits` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/runtimecontrol/policy_test.go:83`<br>`TestMarketDayStartUTCUsesOrderSymbolTimezone` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/runtimecontrol/policy_test.go:102`<br>`TestObservationFromSnapshotFormatsTimesAndDefaultsStatus` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/runtimecontrol/policy_test.go:120`<br>`TestPositionMatchesMarketQualifiedSymbols` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/runtimecontrol/policy_test.go:146`<br>`TestFormatNumberNormalizesNegativeZero` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/runtimecontrol/semantics_test.go:5`<br>`TestLiveExecutionLimitationsAllowsBrokerExecutedPineSemantics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/service_test.go:138`<br>`TestServiceDelegatesStoresAndRuntime` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/service_test.go:165`<br>`TestServiceListDefinitionsPropagatesStoreError` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/service_test.go:174`<br>`TestServicePineAnalyzerOption` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/service_test.go:198`<br>`TestServiceAnalyzePineRejectsUnsupportedSourceFormat` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/service_test.go:210`<br>`TestServiceStartInstanceRejectsNotStartableBeforeRuntimeStart` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/service_test.go:223`<br>`TestServiceStartInstanceMapsPineWorkerCapacityToBusyError` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/service_test.go:236`<br>`TestServiceStartInstancePreservesLookupRuntimeAndTransitionFailures` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/service_test.go:255`<br>`TestServiceStartInstanceRefreshesLiveMarketStreamAfterSuccess` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/service_test.go:278`<br>`TestServicePauseAndStopInstancesStopRuntimeAfterStateTransition` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/service_test.go:304`<br>`TestServiceDelegatesCatalogRuntimeAndLifecycleEntryPoints` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/types_test.go:8`<br>`TestDefinitionViewJSONRemainsFlat` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/strategy/types_test.go:42`<br>`TestInstanceBindingJSONContract` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/definition/source_format_test.go:8`<br>`TestNormalizeSourceFormatDefaultsToPineV6` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/definition/source_format_test.go:17`<br>`TestValidateScriptAndSupportsInstantiation` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/definition/source_format_validation_test.go:8`<br>`TestValidateScriptAcceptsPineV6Source` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/definition/source_format_validation_test.go:16`<br>`TestValidateScriptRejectsUnsupportedFormatAndInvalidSource` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_semantics_test.go:8`<br>`TestMatchingFunctionCallParenHandlesQuotesAndEscapes` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_semantics_test.go:20`<br>`TestParseIndicatorTimeUnitValueSupportsQuotedAndMinuteCountInputs` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_semantics_test.go:46`<br>`TestParseMovingAverageOptionalArgsCoversSourceAndTimeUnitSemantics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_semantics_test.go:114`<br>`TestParsePriceSourceAndBuildMovingAverageKeyWithSource` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_semantics_test.go:155`<br>`TestNormalizeFallbackHelpersCoverDefaultBranches` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:10`<br>`TestParseFunctionCall` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:93`<br>`TestSplitArguments` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:152`<br>`TestNormalizeFunctionName` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:176`<br>`TestParseMovingAverageType` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:212`<br>`TestNormalizeMovingAverageType` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:235`<br>`TestParseIndicatorTimeUnitValue` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:283`<br>`TestNormalizeIndicatorTimeUnit` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:304`<br>`TestBuildMovingAverageKey` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:347`<br>`TestParseQuantityMode` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:384`<br>`TestNormalizeQuantityMode` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:395`<br>`TestParseProtectMode` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:425`<br>`TestNormalizeProtectMode` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:433`<br>`TestParseProtectDirection` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:467`<br>`TestNormalizeProtectDirection` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:475`<br>`TestParseProtectWindowPolicy` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:501`<br>`TestNormalizeProtectWindowPolicy` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:509`<br>`TestParsePositiveInt` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:542`<br>`TestParsePositiveFloat` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:574`<br>`TestParsePercentage` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:609`<br>`TestExpectOnePositiveIntArg` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:666`<br>`TestExpectPositiveIntArgs` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:720`<br>`TestIntsToStrings` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/parser_validation_test.go:5`<br>`TestAdvancedRequirementParserRejectsMalformedShapes` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/parser_validation_test.go:55`<br>`TestFixedTimeframeValidationRejectsEveryRequirementFamily` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/risk_specification_rejection_test.go:5`<br>`TestRiskSpecificationRejectsMalformedTimeAndPolicyContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/spec_parse_business_test.go:11`<br>`TestParseIndicatorRequirementKeysCoversSourceAwareAdvancedAndRiskKeys` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/spec_parse_business_test.go:135`<br>`TestParseIndicatorRequirementKeysCoversLegacyCloseBasedFamilies` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/spec_parse_business_test.go:198`<br>`TestIndicatorRequirementsFromPlanTrimsBlankKeysAndStaysStrict` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/spec_parse_business_test.go:215`<br>`TestParseIndicatorRequirementKeysStrictRejectsInvalidBusinessKeys` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/spec_parse_invalid_test.go:8`<br>`TestParseIndicatorRequirementKeysRejectsEveryKeyFamilyBoundary` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/spec_parse_invalid_test.go:60`<br>`TestParseIndicatorRequirementKeysRejectsMalformedVariants` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/spec_parse_invalid_test.go:137`<br>`TestMovingAverageAndRiskKeyParsingBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/spec_parse_invalid_test.go:203`<br>`TestIndicatorTimeUnitAndSourceNormalizationBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/spec_sort_business_test.go:8`<br>`TestSortedIndicatorConfigsRespectDeterministicBusinessPriority` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/spec_sort_business_test.go:66`<br>`TestSortedRiskAndDivergenceConfigsRespectTieBreakers` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_internal_test.go:10`<br>`TestCalculateIndicatorWarmupBarsCoversSourceAwareWindowsAndDivergences` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_internal_test.go:38`<br>`TestEstimateTradingPeriodBarsHandlesFallbackAndInvalidInputs` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_internal_test.go:50`<br>`TestAdvancedIndicatorLookbackReflectsWarmupSemantics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_internal_test.go:62`<br>`TestValidateFixedTimeframeRequirementsCoversAllConfigFamilies` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_internal_test.go:110`<br>`TestFormatFixedTimeframeLabels` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_internal_test.go:142`<br>`TestResolveIntervalMinutesSupportsBrokerIntervalsAndSafeFallbacks` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_plan_test.go:12`<br>`TestWarmupBarsFromPlanUsesLargestIndicatorRequirement` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_plan_test.go:40`<br>`TestWarmupBarsFromPlanForSymbolUsesMarketTradingProfiles` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_plan_test.go:74`<br>`TestWarmupBarsFromPlanForSymbolUsesExtendedTradingDayWhenEnabled` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_plan_test.go:98`<br>`TestWarmupBarsFromPlanDoesNotApplyRuntimeSeriesFloor` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_plan_test.go:121`<br>`TestWarmupBarsFromPlanHandlesDivergenceAndProtectLookback` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_script_test.go:14`<br>`TestWarmupBarsFromScriptMatchesPlanWithExtendedHours` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_script_test.go:51`<br>`TestWarmupBarsFromScriptFallsBackToGenericTradingCalendarForUnknownSymbol` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_script_test.go:67`<br>`TestRequestSecurityTimeframeRequirementsValidateAgainstStrategyInterval` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_script_test.go:96`<br>`TestWarmupBarsFromScriptRejectsInvalidScript` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/indicatorwarmup/warmup_script_test.go:103`<br>`TestWarmupBarsFromPlanRejectsInvalidIndicatorKeys` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_branch_test.go:9`<br>`TestPlanRequirementsCollectsLoopExitDivergenceAndLegacyProtectKeys` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_branch_test.go:100`<br>`TestIRStatementKindsAndSourceRangesStayStableForDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_business_boundary_test.go:10`<br>`TestPlanRequirementsKeepsBranchLocalIndicatorAliases` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_business_boundary_test.go:42`<br>`TestPlanRequirementsCollectsLoopObjectAndExitExpressions` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_business_boundary_test.go:100`<br>`TestPlanRequirementsRejectsBusinessInvalidIndicatorParameters` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_business_boundary_test.go:165`<br>`TestPlanRequirementsRejectsNilAndUnsupportedStatements` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_indicator_matrix_test.go:8`<br>`TestParseIndicatorBindingSupportedMatrix` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_indicator_matrix_test.go:55`<br>`TestParseIndicatorBindingValidationMatrix` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_indicator_matrix_test.go:147`<br>`TestCollectExpressionRequirementsHandlesInvalidAndBaseIndicatorForms` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_internal_boundaries_test.go:9`<br>`TestParseIndicatorBindingBusinessKeys` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_internal_boundaries_test.go:57`<br>`TestParseIndicatorBindingRejectsInvalidBusinessParameters` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_internal_boundaries_test.go:101`<br>`TestAdvancedIndicatorBindingsRejectTrailingTimeframeArguments` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_internal_boundaries_test.go:131`<br>`TestCollectExpressionRequirementsCoversCompoundIndicatorExpressions` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_internal_boundaries_test.go:188`<br>`TestCollectExpressionRequirementsRejectsInvalidCompoundIndicatorCalls` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_internal_boundaries_test.go:217`<br>`TestProtectAndDivergenceKeyBusinessBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_internal_test.go:5`<br>`TestParseIndicatorBindingNormalizesSourceAwareIndicators` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_internal_test.go:88`<br>`TestParseIndicatorBindingRejectsInvalidIndicatorParameters` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_internal_test.go:122`<br>`TestBuildDivergenceRequirementKeySupportsExpectedIndicators` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_test.go:10`<br>`TestPlanRequirementsCollectsPineIndicatorsAndRuntimeNeeds` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_test.go:51`<br>`TestPlanRequirementsRejectsInvalidIndicatorBinding` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_test.go:62`<br>`TestPlanRequirementsRejectsInvalidMovingAverageType` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_test.go:73`<br>`TestPlanRequirementsRejectsUnsupportedOrderQuantityMode` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_test.go:85`<br>`TestPlanRequirementsRejectsInvalidProtectTimeUnit` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_test.go:100`<br>`TestPlanRequirementsDetectsPositionVariablesInExpressions` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_test.go:122`<br>`TestPlanRequirementsIndicatorKeysMatchRuntimeBindingParity` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_test.go:204`<br>`TestPlanRequirementsPreservesLegacyCloseKeysAndSourceAwareKeys` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_test.go:254`<br>`TestPlanRequirementsRejectsUnsupportedWindowSource` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_test.go:265`<br>`TestPlanRequirementsCollectsAdvancedIndicatorBindings` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/ir/planner_test.go:310`<br>`TestPlanRequirementsCollectsExpressionIndicatorsAcrossStatementShapes` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/collection_object_bounds_test.go:11`<br>`TestCollectionHelperBusinessBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/collection_object_bounds_test.go:85`<br>`TestCollectionObjectFieldAndHistoryBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/collection_object_bounds_test.go:141`<br>`TestObjectDefinitionAndMethodArgumentBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/compiler_and_security_diagnostics_test.go:11`<br>`TestCompilerDiagnosticsPreserveActionablePlannerAndRemoteErrors` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/compiler_and_security_diagnostics_test.go:44`<br>`TestRequestSecurityPurityCoversOptionalAndBuiltinRecovery` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/compiler_and_security_diagnostics_test.go:70`<br>`TestPineEditorRecoveryAndCapabilityEvidenceContracts` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/compiler_rejection_contracts_test.go:9`<br>`TestUnsupportedSyntaxDiagnosticsDescribeUnsafeRequestSecurityContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/compiler_rejection_contracts_test.go:79`<br>`TestTupleHelpersRejectMalformedIndicatorArityWithoutInventingAliases` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/compiler_rejection_contracts_test.go:180`<br>`TestTALoweringLeavesMalformedCallsUntouched` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/control_flow_reject_test.go:8`<br>`TestCompileRejectsInvalidControlFlowAndUDFContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/control_flow_reject_test.go:43`<br>`TestCompilePreservesDescendingAndConditionalLoopSemantics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/controlflow_object_collection_contracts_test.go:11`<br>`TestControlFlowParserRetainsUserFunctionAndCollectionLoopSemantics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/controlflow_object_collection_contracts_test.go:85`<br>`TestObjectLifecycleParserCoversFieldsMultilineMethodsAndReceivers` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/controlflow_object_collection_contracts_test.go:157`<br>`TestCollectionParserKeepsHistoryAndExpressionContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/expression_test.go:5`<br>`TestParseExpressionRejectsBlankInput` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/expression_test.go:11`<br>`TestParseExpressionParsesTrimmedExpressions` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/expression_test.go:21`<br>`TestParseExpressionRejectsInvalidSyntax` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/extended_ticker_test.go:5`<br>`TestExtendedTickerRequestSecuritySupportsCurrentSymbolOnly` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/extended_ticker_test.go:49`<br>`TestCompileAcceptsExtendedTickerAndChartFlags` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/language_execution_boundaries_test.go:12`<br>`TestRequestSecuritySupportedTAFamiliesKeepTheirExecutionContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/language_execution_boundaries_test.go:99`<br>`TestCollectionExecutionParserKeepsReceiverAndErrorBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/language_execution_boundaries_test.go:179`<br>`TestTupleLoweringHelpersMaintainAliasAndArityContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/language_execution_boundaries_test.go:287`<br>`TestCompilerHeadersAndLexicalHelpersRetainPineV6Rules` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/language_execution_boundaries_test.go:364`<br>`TestObjectExecutionParserPreservesDeclaredTypeContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/language_execution_boundaries_test.go:454`<br>`TestCollectionLexicalParserRejectsMalformedHistoryAndNamespaceReferences` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/language_execution_boundaries_test.go:531`<br>`TestUDFAndDynamicLoopHelpersProtectRuntimeBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/language_execution_boundaries_test.go:600`<br>`TestTALoweringHelpersKeepNativePineArgumentSemantics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/language_failure_contracts_test.go:9`<br>`TestObjectAndCollectionParserFailureContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/language_failure_contracts_test.go:149`<br>`TestRequestSecurityAndTupleContractsRejectUnsafeExpressions` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/language_failure_contracts_test.go:239`<br>`TestOrderAndTupleHelperContractsKeepTradeInstructionsUnambiguous` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/object_collect_bounds_test.go:12`<br>`TestObjectNamedArgumentNormalizationBusinessBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/object_collect_bounds_test.go:57`<br>`TestObjectMethodLoweringHandlesHistoryNamedDefaultsAndExpressionReceivers` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/object_collect_bounds_test.go:83`<br>`TestCollectionStatementsAndReadLoweringBusinessBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/object_collect_reject_test.go:9`<br>`TestCompileRejectsInvalidObjectAndCollectionContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/object_declaration_contracts_test.go:10`<br>`TestObjectDeclarationContractsRejectMalformedDomainTypesAndMethods` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/object_declaration_contracts_test.go:120`<br>`TestObjectCallsPreserveNamedArgumentAndOverloadSafety` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/order_command_security_rejection_test.go:11`<br>`TestOrderCommandRejectionsPropagateToPineCallers` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/order_command_security_rejection_test.go:82`<br>`TestRequestSecurityTupleValidationKeepsParserBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/order_metadata_contracts_test.go:8`<br>`TestOrderMetadataRejectsAmbiguousInputsAndKeepsSupportedPositionals` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_benchmark_business_test.go:5`<br>`TestBenchmarkBusinessScriptsCompileAsRegressionCases` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:11`<br>`TestAnalyzeScriptIncludesV20CollectionAndDeclarationSemantics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:140`<br>`TestPineV20LanguageFoundationGate` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:210`<br>`TestAnalyzeScriptReportsCollectionOperationSignatureDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:233`<br>`TestAnalyzeScriptIncludesCollectionMethodStyleOperations` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:274`<br>`TestAnalyzeScriptIncludesTypedCollectionDeclarations` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:330`<br>`TestCompileSupportsV21ExecutableCollectionCore` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:375`<br>`TestCompileSupportsV21CollectionAliases` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:400`<br>`TestCompileSupportsV21BBWAndCOG` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:430`<br>`TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:477`<br>`TestCompileSupportsV22PureUDTAndMethodSubset` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:515`<br>`TestCompileSupportsV23NamedObjectArgsAndPureMethodBody` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:554`<br>`TestCompileSupportsV23LocalObjectFieldReassignment` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:587`<br>`TestCompileSupportsV23RequestSecurityPureObjectAndCollectionExpressions` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:628`<br>`TestCompileSupportsV24CollectionExpansionAndMTFStoch` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:678`<br>`TestCompileSupportsV24NamedObjectMethodExpressionAndRuntimeLoopFallback` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:724`<br>`TestCompileSupportsV25ArrayStringAndTimeframeHelpers` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_collection_test.go:768`<br>`TestCompileSupportsV26CollectionIterationHistoryAndObjectCollectionFields` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:10`<br>`TestCompileSupportsV27CollectionTimeframeAndMTFHelpers` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:65`<br>`TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:103`<br>`TestCompileSupportsV29ObjectHistoryMethodReceiverAndMTFHistoryExpression` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:136`<br>`TestAnalyzeScriptReportsV29RequestSecurityDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:173`<br>`TestAnalyzeScriptReportsV32RequestSecurityDiagnosticMatrix` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:206`<br>`TestCompileSupportsV30SemanticDeclarationModelAndVaripPolicy` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:249`<br>`TestAnalyzeScriptReportsCollectionTypeDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:289`<br>`TestAnalyzeScriptReportsCollectionMethodStyleSignatureDiagnostics` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:313`<br>`TestAnalyzeScriptReportsDeclarationSemanticDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:341`<br>`TestAnalyzeScriptReportsTypeAndMethodRegistryDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:410`<br>`TestAnalyzeScriptReportsImportAliasDeclarationDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_object_test.go:438`<br>`TestAnalyzeScriptReportsObjectOperationSignatureDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_request_test.go:10`<br>`TestCompileSupportsMovingAverageRequestSecuritySubset` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_request_test.go:82`<br>`TestCompileSupportsPineStdev` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_request_test.go:97`<br>`TestCompileSupportsCommonTradingViewTAFunctions` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_request_test.go:141`<br>`TestCompileSupportsV14WindowMomentumAndStatefulIndicators` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_request_test.go:183`<br>`TestCompileSupportsV14RequestSecurityPureExpression` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_request_test.go:217`<br>`TestCompileSupportsV15RequestSecurityCommonTAExpression` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_request_test.go:263`<br>`TestCompileSupportsV16RequestSecurityTupleWhitelist` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:10`<br>`TestAnalyzeScriptIncludesV17SemanticSummary` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:55`<br>`TestAnalyzeScriptReportsSupportedTASemanticSignatures` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:98`<br>`TestAnalyzeScriptReportsSupportedUtilitySemanticSignatures` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:180`<br>`TestAnalyzeScriptReportsSemanticSignatureDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:195`<br>`TestCompileSupportsV15StaticForLoopControl` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:230`<br>`TestCompileSupportsInputMathCrossAndSourceAwareMovingAverages` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:268`<br>`TestCompileSupportsPineStrategyPositionVariables` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:287`<br>`TestAnalyzeScriptSupportsTrendAndStatefulTAFunctions` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:301`<br>`TestAnalyzeScriptSupportsSarBarstateSessionAndPineConstants` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:340`<br>`TestCompileSupportsOrderQtyPercentStrategyOrderAndCloseAll` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:377`<br>`TestCompileIgnoresVisualCallsWithWarning` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_semantic_test.go:401`<br>`TestAnalyzeScriptReturnsVisualMetadata` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:10`<br>`TestParseScriptLowersPineStrategyToIR` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:54`<br>`TestCompileRejectsPublicInternalHelperCalls` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:84`<br>`TestAnalyzeScriptReportsPublicInternalHelperDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:113`<br>`TestCompileAcceptsNativePineIndicatorPublicEntry` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:126`<br>`TestCompileUsesStrategyDefaultQuantityForEntryWithoutQty` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:145`<br>`TestCompileParsesBacktestStrategyMetadata` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:167`<br>`TestCompilePreservesOrderNotificationMetadataAndImmediateClose` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:186`<br>`TestCompileExplicitEntryQtyOverridesStrategyDefaultQuantity` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:202`<br>`TestValidateScriptRejectsUnsupportedPineRuntimeFeature` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:232`<br>`TestCompileSupportsMultiBarHistoryReferences` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:253`<br>`TestValidateScriptReportsUnsupportedHistoryReferences` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:286`<br>`TestCompileSupportsStrategyExitSubset` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:317`<br>`TestCompileCapturesWhenExpressionsForWorkflowOrders` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:348`<br>`TestCompileSupportsStrategyExitProfitLossTicks` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:367`<br>`TestCompileCapturesStrategyExitSpecificMetadata` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:390`<br>`TestCompileSupportsPendingStopAndCancelOrders` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:432`<br>`TestCompileSupportsCloseAllPositionalMetadata` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:448`<br>`TestCompileSupportsClosePositionalQty` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:464`<br>`TestValidateScriptReportsUnsupportedAdvancedOrders` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:488`<br>`TestAnalyzeScriptReportsV40BrokerBoundaryDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:560`<br>`TestCompileSupportsFrameworkLanguageFeatures` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:587`<br>`TestCompileSupportsV12AdvancedIndicators` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:621`<br>`TestCompileSupportsV12AdvancedIndicatorsInStaticIntradaySecurity` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:651`<br>`TestCompileSupportsV13MigrationIndicators` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:690`<br>`TestCompileSupportsV13IndicatorsInStaticIntradaySecurity` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:718`<br>`TestCompileSupportsAllowEntryInRiskDeclaration` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:732`<br>`TestCompileSupportsRuntimeRiskDeclarations` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:763`<br>`TestCompatibilityScoreAndSupportedFeatureIDsAreRegistryDriven` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:790`<br>`TestCompileSupportsExpressionUDFAndStaticForUnroll` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:838`<br>`TestValidateScriptReportsUnsupportedUDFAndStaticForCases` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:903`<br>`TestAnalyzeScriptReportsV33AdvancedLanguageBoundaryDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:977`<br>`TestCompileSupportsSwitchAndMultiStatementUDF` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:1014`<br>`TestAnalyzeScriptReturnsStructuredUnsupportedDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:1029`<br>`TestAnalyzeScriptPreservesOriginalLineNumbers` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parse_test.go:1044`<br>`TestHistoryReferencesIgnoreStringLiterals` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_and_lowering_recovery_test.go:8`<br>`TestIncompleteColorAndRequestCallsRemainRecoverable` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_and_lowering_recovery_test.go:19`<br>`TestUDFExpansionRetainsWhitespaceAndUnwindsRejectedArguments` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_and_lowering_recovery_test.go:43`<br>`TestMethodAndControlFlowFailuresKeepSourceContracts` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_and_lowering_recovery_test.go:119`<br>`TestNestedLoopStateAndImportRecoveryRemainStable` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_and_lowering_recovery_test.go:162`<br>`TestOrderAndTupleErrorsPreserveExecutableBoundaries` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_and_lowering_recovery_test.go:206`<br>`TestOrderCallsRejectUnknownNamedArgumentsBeforePlanning` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_and_lowering_recovery_test.go:235`<br>`TestSourceAnnotationsAndCommentsKeepTheirSeparateRoles` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_helper_boundaries_test.go:11`<br>`TestParserHelperContractsCoverEmptyHeadersAndLexicalEdges` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_helper_boundaries_test.go:66`<br>`TestValidationAndDynamicWhileHelpersKeepDiagnosticsActionable` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_helper_boundaries_test.go:110`<br>`TestRuntimeLoopAndTupleParserErrorContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_helper_boundaries_test.go:196`<br>`TestCollectionParserHelperErrorsPreserveExecutableBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_loop_boundaries_test.go:10`<br>`TestMalformedNamespaceCallsRemainVisibleForValidation` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_loop_boundaries_test.go:38`<br>`TestStaticLoopBoundsRejectNonTerminatingUserRanges` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_loop_boundaries_test.go:63`<br>`TestTupleIndicatorsExposeUnsupportedCallHistory` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_recovery_boundaries_test.go:12`<br>`TestParserRejectsMalformedUDFAndStatementBoundaries` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_recovery_boundaries_test.go:104`<br>`TestObjectArgumentRecoveryRejectsOverrunAndPreservesDefaults` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/parser_recovery_boundaries_test.go:151`<br>`TestCollectionScannerRejectsDamagedAndUnknownCalls` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/public_lowering_test.go:10`<br>`TestParseScriptPublicEntryReturnsProgramAndPropagatesErrors` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/public_lowering_test.go:38`<br>`TestTALoweringBoundariesPreserveInvalidNativeCalls` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/public_lowering_test.go:64`<br>`TestPineMovingAverageTypeCoversNativeAliases` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/public_lowering_test.go:84`<br>`TestRequestSecurityArgsFromLineCoversAssignmentForms` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/public_lowering_test.go:130`<br>`TestStrategyQuantityAndMetadataBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/request_security_ast_contracts_test.go:5`<br>`TestRequestSecurityPurityRejectsUnsafeLoweredExpressions` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/request_security_ast_contracts_test.go:26`<br>`TestRequestSecurityRejectsMalformedAdvancedIndicatorArguments` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/request_security_diagnostics_test.go:8`<br>`TestRequestSecurityDiagnosticsRejectUnsafeOrAmbiguousInputs` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/request_security_diagnostics_test.go:52`<br>`TestRequestSecurityLoweringRetainsOnlyPureStaticExpressions` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/request_security_object_contracts_test.go:11`<br>`TestRequestSecurityLoweringRejectsUnrepresentableExecutionBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/request_security_object_contracts_test.go:86`<br>`TestObjectDefinitionParserProtectsTypeAndMethodContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/runtime_and_parser_boundaries_test.go:9`<br>`TestOrderAndParserBoundaryContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/runtime_and_parser_boundaries_test.go:46`<br>`TestDynamicForBoundsUseRuntimeFallback` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/runtime_and_parser_boundaries_test.go:62`<br>`TestNormalizationPreservesInvalidUserSyntaxAndErrors` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/runtime_and_parser_boundaries_test.go:94`<br>`TestMalformedTAExpressionsRemainVisibleForValidation` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/runtime_and_parser_boundaries_test.go:134`<br>`TestControlFlowErrorsRemainActionableBeforeRuntime` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/security_lowering_test.go:9`<br>`TestRequestSecurityLoweringHelperBusinessBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/security_lowering_test.go:92`<br>`TestRequestSecurityPurityAndMergeArgumentBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/security_lowering_test.go:168`<br>`TestRequestSecurityAdvancedTALoweringBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/semantic_helper_boundaries_test.go:8`<br>`TestSemanticHelpersHandleDeclarationAndParameterBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/semantic_helper_boundaries_test.go:58`<br>`TestSemanticHelpersReflectPineCollectionAndObjectContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/semantic_helper_boundaries_test.go:99`<br>`TestSemanticHelpersReportMalformedScriptBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/semantic_helper_boundaries_test.go:128`<br>`TestSemanticHelpersCoverObjectAndVisualFallbacks` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/shared_structure_corpus_test.go:35`<br>`TestSharedPineStructureCorpusMatchesBackendIR` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/strategy_business_test.go:11`<br>`TestStrategyRiskArgumentParsersCoverBusinessBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/strategy_business_test.go:95`<br>`TestCompileCoversTrailPriceExitAndShortCloseBusinessSemantics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/strategy_business_test.go:130`<br>`TestValidateScriptReportsRiskDeclarationBoundaryErrors` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/strategy_call_bounds_test.go:11`<br>`TestParseStrategyCallCoversOrderLifecycleBusinessBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/strategy_call_bounds_test.go:98`<br>`TestParseStrategyCallRejectsUnsupportedOrderBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/strategy_call_bounds_test.go:120`<br>`TestParseStrategyCallRejectsInvalidTradingExpressions` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/tuple_assignment_contracts_test.go:11`<br>`TestGeneralTupleAssignmentsKeepPineAliasContract` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/tuple_switch_reject_test.go:8`<br>`TestCompileRejectsMalformedSwitchAndTupleContracts` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/udf_expansion_contracts_test.go:8`<br>`TestUDFExpansionRejectsMalformedRecursiveAndDeepCalls` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/udf_expansion_contracts_test.go:37`<br>`TestUDFExpansionSkipsMembersAndExpandsStandaloneCalls` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:8`<br>`TestRequestSecurityValidationExplainsMalformedAndUnsafeExpressions` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:29`<br>`TestRequestSecurityExpressionTASubsetValidation` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:43`<br>`TestRequestSecurityTupleAliasExtractionPreservesAssignmentContract` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:57`<br>`TestRejectUnsupportedReturnsRuntimeAndCollectionBusinessErrors` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:70`<br>`TestStrategyDeclarationInvalidConstantsFallBackWithWarnings` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:86`<br>`TestStrategyDeclarationWithoutArgumentsUsesBusinessDefaults` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:96`<br>`TestExpressionAndHistoryValidationRejectsInvalidBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:108`<br>`TestPublicHelperGuardReturnsActionableMigrationErrors` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_client_test.go:10`<br>`TestDisabledPayloadDocumentsCommunityAGPL` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_client_test.go:20`<br>`TestPinetsWorkerClientEngineInfoAndRunIndicator` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_client_test.go:56`<br>`TestPinetsWorkerClientMapsRuntimeErrors` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:18`<br>`TestExternalModeFromEnvAndDisabledShadowPayload` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:39`<br>`TestShadowPayloadReportsWorkerStartupFailure` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:55`<br>`TestShadowPayloadRunsConfiguredWorkerAndReturnsExternalResult` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:72`<br>`TestCommunityAGPLModeBlocksExecutionWhenNoticeCannotBeFound` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:97`<br>`TestExternalEnginePayloadFromResultMapsSuccessAndFailure` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:131`<br>`TestPinetsWorkerClientCallHandlesProtocolFailures` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:190`<br>`TestWorkerErrorStringAndStderrSuffix` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:207`<br>`TestPinetsWorkerConfigurationAndStartupErrors` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:224`<br>`TestPinetsWorkerClientCapturesBoundedStderr` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:236`<br>`TestPinetsWorkerClientCloseWaitsForOwnedWorkers` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_runtime_test.go:12`<br>`TestShadowPayloadForScriptExecutesRepositoryWorker` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_runtime_test.go:31`<br>`TestRepositoryRelativePineTSAssetsAreDiscoverable` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineengine/pine_ts_runtime_test.go:58`<br>`TestPinetsWorkerClientProtocolBoundaryFailures` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pinespec/lint_helpers_test.go:5`<br>`TestOptionalTypeAssertionReturnsZeroForUnexpectedType` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pinespec/skill_metadata_test.go:8`<br>`TestPineSpecSkillMetadataAndResources` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pinespec/skill_metadata_test.go:80`<br>`TestPineSpecRejectsUnknownSectionsAndKeepsFallbackFormattingStable` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pinespec/spec_test.go:14`<br>`TestExamplesParseAndPlan` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pinespec/spec_test.go:28`<br>`TestGoldenExamplesAnalyzeAndPlan` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pinespec/spec_test.go:59`<br>`TestBuildToolPayloadSectionsAndExamples` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pinespec/spec_test.go:108`<br>`TestBuildToolPayloadIncludesSupportMatrix` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pinespec/spec_test.go:248`<br>`TestBuildToolPayloadIncludesBrokerBoundary` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pinespec/spec_test.go:274`<br>`TestGeneratedPineSupportSnapshotIsCurrent` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pinespec/spec_test.go:285`<br>`TestSkillResourcesContainSpecAndExamples` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/client_test.go:12`<br>`TestClientRunScriptSuccessAppliesMetadataDefaults` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/client_test.go:41`<br>`TestClientRunScriptRejectsInvalidRequestBeforeTransport` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/client_test.go:56`<br>`TestJSONSizeMatchesMarshalForRunScriptRequest` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/client_test.go:81`<br>`TestClientRunScriptMapsTransportError` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/client_test.go:92`<br>`TestClientRunScriptMapsTimeout` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/client_test.go:103`<br>`TestClientRunScriptMapsWorkerError` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/client_test.go:114`<br>`TestClientRunScriptRejectsMismatchedJobID` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/client_test.go:125`<br>`TestClientRunScriptRejectsPerformanceGateFailure` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/client_test.go:143`<br>`TestClientRunScriptDoesNotApplyPerformanceGateByDefault` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/client_test.go:161`<br>`TestNewClientRequiresTransport` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/grpc_dialer_test.go:9`<br>`TestGRPCDialerCreatesManagedTransport` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/grpc_dialer_test.go:23`<br>`TestGRPCDialerRejectsNilReceiver` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/grpc_transport_test.go:16`<br>`TestGRPCTransportRunScriptAndHealthCheck` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/grpc_transport_test.go:71`<br>`TestGRPCTransportRequiresClient` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/hardcut_audit_test.go:12`<br>`TestPineTSHardCutDoesNotExposeGoPineRuntime` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_readiness_recovery_test.go:15`<br>`TestWorkerManagerReadinessFailuresCloseTransportAndRespectCancellation` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:14`<br>`TestWorkerManagerStartStopAndSnapshot` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:46`<br>`TestWorkerManagerRunScriptRoundRobinsHealthyWorkers` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:74`<br>`TestWorkerManagerPinsLiveSessionAndClearsItOnClose` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:119`<br>`TestWorkerManagerReservesLiveSessionBeforeOpenCompletes` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:158`<br>`TestWorkerManagerRunScriptQueuesWhenAllWorkersBusy` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:200`<br>`TestWorkerManagerRunScriptRejectsWhenBusyIfConfigured` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:236`<br>`TestWorkerManagerCheckHealthRestartsFailedWorker` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:262`<br>`TestWorkerManagerCheckHealthReportsRestartFailure` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:281`<br>`TestWorkerManagerStartCleansUpAfterDialFailure` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:299`<br>`TestWorkerManagerStartDialFailureIncludesProcessDiagnostics` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:324`<br>`TestWorkerManagerStartRetriesDialUntilWorkerReady` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:341`<br>`TestWorkerManagerStopReturnsFirstCloseError` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/manager_test.go:359`<br>`TestWorkerManagerRequiresDependenciesAndStart` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/payload_size_test.go:9`<br>`TestJSONSizeMatchesMarshalAcrossPayloadShapes` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/payload_size_test.go:26`<br>`TestEstimateRunScriptRequestJSONSizeHandlesNilAndEmptyCollections` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/payload_size_test.go:49`<br>`TestEstimateCandleJSONSizeRejectsNonFiniteFields` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_boundaries_test.go:19`<br>`TestNodeWorkerLauncherDefaultsAndMaterializationBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_boundaries_test.go:70`<br>`TestNodeWorkerLauncherRemovesMaterializedBundleWhenContextAlreadyCanceled` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_boundaries_test.go:94`<br>`TestOSWorkerProcessDiagnosticsAndNilBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_boundaries_test.go:118`<br>`TestOSWorkerProcessForcesTerminationOnTimeoutAndCancellation` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_boundaries_test.go:165`<br>`TestPineworkerInterruptIgnoringHelperProcess` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_test.go:16`<br>`TestNodeWorkerLauncherMaterializesBundleWithArgs` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_test.go:72`<br>`TestNodeWorkerLauncherRejectsBadChecksum` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_test.go:88`<br>`TestNodeWorkerLauncherStopKillsLongRunningProcess` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_test.go:107`<br>`TestNewNodeWorkerLauncherRequiresBundle` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_test.go:114`<br>`TestNodeWorkerEnvironmentPreservesNodeOptionsWithoutOldSpaceOverride` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_test.go:133`<br>`TestNodeWorkerLauncherRejectsNilReceiver` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_launcher_test.go:141`<br>`TestNodeWorkerLauncherReturnsStartErrorAndRemovesFile` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_smoke_test.go:20`<br>`TestWorkerManagerProcessSmokeWithNodeWorker` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/process_smoke_test.go:34`<br>`TestWorkerManagerRealPineTSProcessSmoke` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/proto_contract_test.go:14`<br>`TestPineWorkerProtoCompilesAndExposesContract` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/proto_mapping_test.go:15`<br>`TestProtoMappingRoundTripRequestAndResponse` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/proto_mapping_test.go:154`<br>`TestCandleBatchEncodingGoldenVector` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/proto_mapping_test.go:182`<br>`TestProtoMappingHandlesNilResponses` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/proto_mapping_test.go:197`<br>`TestHealthFromProtoCopiesCapabilities` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/proto_mapping_test.go:212`<br>`TestRequestToProtoDoesNotAliasNilOrMutableParams` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/runtime_boundaries_test.go:14`<br>`TestTailBufferRetainsOnlyConfiguredProcessLogTail` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/runtime_boundaries_test.go:39`<br>`TestPerformanceGateRejectsLatencyAndThroughputBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/runtime_boundaries_test.go:66`<br>`TestWorkerConfigAndCandleTimeBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/runtime_boundaries_test.go:79`<br>`TestClientDefaultsAndResponseIdentityBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/runtime_boundaries_test.go:107`<br>`TestGRPCTransportPropagatesRPCFailures` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/runtime_boundaries_test.go:134`<br>`TestWorkerManagerSelectionCapacityAndErrorBoundaries` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/runtime_boundaries_test.go:189`<br>`TestWorkerManagerDiagnosticErrorFormatting` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/types_test.go:10`<br>`TestNormalizeRuntimeMigratesLegacyRuntime` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/types_test.go:21`<br>`TestDefaultWorkerConfigScalesByCPU` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/types_test.go:41`<br>`TestValidateRunScriptRequest` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/types_test.go:92`<br>`TestValidateRunScriptRequestAnalyzeModeAllowsNoCandles` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/types_test.go:105`<br>`TestValidateRunScriptRequestLiveSessionContract` | strategy_pine | 高风险 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/types_test.go:147`<br>`TestValidateRunScriptRequestRejectsTooManyCandles` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/types_test.go:164`<br>`TestRunScriptPayloadSizeRejectsNonFiniteCandle` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:pkg/strategy/pineworker/types_test.go:171`<br>`TestCheckPerformanceGate` | strategy_pine | 普通边界 | `crates/jftrade-strategy, crates/jftrade-integration-pine` |
| [ ] | `go:452dea11:internal/trading/broker_account_read_failures_test.go:11`<br>`TestBrokerReadFailuresRemainVisibleAcrossAccountDataViews` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_account_read_failures_test.go:87`<br>`TestFundsMapsMarketAssetsAlongsideCashBalances` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_boundaries_test.go:11`<br>`TestServiceBrokerReadOperationsReturnFallbackWhenMarketDataUnavailable` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_boundaries_test.go:62`<br>`TestServiceBrokerReadOperationsClassifyUpstreamFailures` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_boundaries_test.go:116`<br>`TestServiceBrokerWriteOperationsPropagateUpstreamFailures` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_conformance_test.go:15`<br>`TestFakeBrokerConformanceAcceptedPartialFullAndOutOfOrderUpdates` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_conformance_test.go:58`<br>`TestFakeBrokerConformanceCancelAcceptedAndCancelRejected` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_conformance_test.go:107`<br>`TestFakeBrokerConformancePlaceRejectedPushBeforeQueryAndUnsupportedCapability` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_test.go:186`<br>`TestServiceBrokerReadOperationsMapSnapshotsAndQueries` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_test.go:453`<br>`TestServicePortfolioAndFallbackResponses` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_test.go:533`<br>`TestServiceBrokerWriteAndTimeoutBehaviors` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_test.go:648`<br>`TestPlaceBrokerOrderRunsPreTradeRiskBeforeBrokerSubmission` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_test.go:680`<br>`TestPlaceBrokerOrderCannotBypassRiskWithImplicitRealEnvironment` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_test.go:711`<br>`TestPlaceBrokerOrderFailsClosedWhenRealRiskGatewayIsUnavailable` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/broker_test.go:735`<br>`TestNormalizeSymbolsAndRuntimeDefaults` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/control_plane_idempotency_test.go:12`<br>`TestRealTradeControlPlaneKillSwitchReleaseIsIdempotentAndAudited` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/control_plane_idempotency_test.go:65`<br>`TestRealTradeControlPlaneHardStopReleaseIsSingleShot` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/control_plane_idempotency_test.go:99`<br>`TestRealTradeControlPlaneHardStopsBlockUntilEveryEntryReleased` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/control_plane_state_audit_test.go:13`<br>`TestControlPlaneRetainsActivationAndBoundsRepeatedAuditEvents` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/control_plane_state_audit_test.go:75`<br>`TestControlPlaneExecutesSimulatedOrdersThroughRiskEvaluation` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/control_plane_state_audit_test.go:99`<br>`TestControlPlaneTreatsEmptyStateAsFreshAndRejectsUnavailableMutations` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/control_plane_state_audit_test.go:190`<br>`TestControlPlaneKeepsStateWhenAtomicPersistenceCannotComplete` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/control_plane_state_audit_test.go:250`<br>`TestControlPlaneSurfacesHardStopRejectionAuditPersistenceFailure` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:15`<br>`TestExecutionComboCompletePreviewPlaceCancelAndBuyingPower` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:83`<br>`TestExecutionOrderRechecksRiskImmediatelyBeforeBrokerSubmission` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:110`<br>`TestExecutionEventParlayCompletePreviewAndAmountRisk` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:150`<br>`TestEventParlayRejectsCallerControlledPrice` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:172`<br>`TestOptionComboValidationRejectsIncompleteRiskShape` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:205`<br>`TestExecutionComboPreviewKeepsLegacyBuyingPowerCompatible` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:237`<br>`TestExecutionComboRejectsEveryUnsafeBoundary` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:319`<br>`TestExecutionComboProviderStoreRiskAndGatewayFailures` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:412`<br>`TestExecutionComboHelperBranches` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:441`<br>`TestExecutionProductPreviewAndSubmissionFailureContractsComplete` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:571`<br>`TestExecutionProductRemainingLifecycleAndUpdateHelpers` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:635`<br>`TestExecutionDetailsResolverAndOrderUpdateCacheFailureBranches` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_products_test.go:12`<br>`TestDerivativeSingleLegRequiresBrokerPreviewAndStableClientID` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_products_test.go:48`<br>`TestPredictionSingleLegEligibilityUsesSecurityFirmAndUSAuthority` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_products_test.go:85`<br>`TestSingleNonEventOrderRejectsAmountAndPredictionFields` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_products_test.go:128`<br>`TestRealFuturesPreviewRequiresFuturesAuthority` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_products_test.go:149`<br>`TestComboPreviewRejectsMixedProductsAndExpiredParlayRFQ` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_products_test.go:193`<br>`TestPreviewHashesBindStableClientOrderID` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:16`<br>`TestNormalizeExecutionOrderDefaultsUSLimitOrder` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:47`<br>`TestNormalizeExecutionOrderSupportsExtendedUSLimitSessions` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:71`<br>`TestNormalizeExecutionOrderSupportsStopAndMarketOrders` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:96`<br>`TestNormalizeExecutionOrderRejectsBusinessRuleViolations` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:159`<br>`TestNormalizeExecutionOrderPreservesBrokerAbstraction` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:173`<br>`TestExecutionOrderServiceFacadeUsesInjectedStoresAndBrokerCommands` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:285`<br>`TestExecutionOrderServiceFacadeReturnsBusinessErrors` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:327`<br>`TestCreateExecutionOrderRejectsInvalidPayloadBeforeBrokerCall` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:342`<br>`TestCreateExecutionOrderRunsPreTradeRiskBeforeBrokerCall` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:367`<br>`TestCreateExecutionOrderAllowsRuntimeEnabledRealTradeBeforeBrokerCall` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:397`<br>`TestCreateExecutionOrderAllowsSimulateWhenRealTradingIsDisabled` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:426`<br>`TestPlaceExecutionOrderResolvesImplicitRealEnvironmentBeforeRisk` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:470`<br>`TestPlaceExecutionOrderFailsClosedWithoutRealRiskGateway` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:496`<br>`TestKillSwitchActivationWaitsForInFlightRealPlacement` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:573`<br>`TestPreTradeRiskRejectsKillSwitchAndLimits` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:613`<br>`TestPreTradeRiskEnforcesAmountModeQuantityAndNotionalLimits` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:687`<br>`TestPreTradeRiskSnapshotUsesNonNilEmptySlices` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:703`<br>`TestPreTradeRiskDoesNotLetUnknownNotionalBypassRealTradeControls` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:718`<br>`TestRealTradeEnvVariablesDoNotConfigurePreTradeRisk` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:738`<br>`TestRealTradeControlPlanePersistsKillSwitchAndHardStop` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:813`<br>`TestRealTradeControlPlaneRuntimeRiskConfigValidationAndDisableEvents` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:874`<br>`TestRealTradeControlPlaneRollsBackFailedPersistence` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:939`<br>`TestRealTradeControlPlaneFailsClosedWhenPersistedStateCannotLoad` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:965`<br>`TestNormalizeExecutionOrderUsesEnvFallbackAndSupportsNonLimitUSSessions` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:988`<br>`TestExecutionNormalizationHelpersRejectUnsupportedInputs` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:997`<br>`TestNormalizeExecutionOrderRejectsInvalidInstrumentAndUnsupportedOrderType` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:1014`<br>`TestExecutionOrderDetailsReturnsOrderAndBoundedRecentEvents` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/execution_test.go:1047`<br>`TestExecutionOrderDetailsRefreshesTargetHistoryBeforeReturning` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_status_test.go:5`<br>`TestCanonicalBrokerOrderStatusCoversFutuLifecycle` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_status_test.go:33`<br>`TestReconcileCanonicalOrderStatusPreventsBrokerRegressions` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_status_test.go:59`<br>`TestCanonicalTerminalOrderStatus` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_update_recovery_test.go:11`<br>`TestExecutionOrderHistoryFailsClosedForInvalidOrUnavailableBackfill` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_update_recovery_test.go:68`<br>`TestConcurrentSubscriptionWaitHonorsCallerCancellation` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_concurrency_test.go:9`<br>`TestOrderUpdatesWorkerBrokerIDConcurrentInitializationAndPush` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_reconnect_test.go:12`<br>`TestOrderUpdatesWorkerResubscribesAfterSubscribeFailureAndPushResumes` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_reconnect_test.go:68`<br>`TestOrderUpdatesWorkerKeepsPushSubscriptionWhenRefreshFails` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:158`<br>`TestOrderUpdatesWorkerThrottleForceAndSubscribeOnce` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:180`<br>`TestOrderUpdatesWorkerCacheTTLTerminalRemovalAndDefensiveCopy` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:228`<br>`TestOrderUpdatesWorkerCurrentHistoryCacheAndPushMetadata` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:263`<br>`TestOrderUpdatesWorkerForcedActiveSyncBypassesCache` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:280`<br>`TestOrderUpdatesWorkerSyncExecutionOrderHistoryUsesOrderScope` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:322`<br>`TestOrderUpdatesWorkerStopIsIdempotentAndCanResubscribe` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:343`<br>`TestOrderUpdatesWorkerResubscribesWhenAccountSetChanges` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:369`<br>`TestOrderUpdatesWorkerRefreshesExistingSubscriptionOnSync` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:397`<br>`TestOrderUpdatesWorkerConcurrentSyncSubscribesOnce` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:414`<br>`TestOrderUpdatesWorkerSnapshotCapsInvalidations` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:427`<br>`TestOrderUpdatesWorkerInactiveSourcePreservesDiagnosticState` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:442`<br>`TestOrderUpdatesWorkerSyncCoversSubscriptionAndHistoryFallbackPaths` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:469`<br>`TestOrderUpdatesWorkerMarksCurrentAndHistoryFailures` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:508`<br>`TestOrderUpdatesWorkerFeeSyncFiltersIdentifiersAndReportsFailure` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/order_updates_test.go:531`<br>`TestOrderUpdatesWorkerHelperBoundariesCoverNilAndReplacementPaths` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/ports_test.go:50`<br>`TestServiceUsesExplicitTradingPorts` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/ports_test.go:82`<br>`TestServiceDefaultTradingPortsFailExplicitly` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/responses_test.go:12`<br>`TestBrokerRuntimeResponseJSONShape` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/responses_test.go:55`<br>`TestBrokerFundsResponseJSONShape` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/responses_test.go:107`<br>`TestBrokerPositionsResponseJSONShape` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/responses_test.go:141`<br>`TestBrokerReadStatusSerializesNullLastError` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/risk_shape_boundaries_test.go:11`<br>`TestCommandRiskShapeRejectsSpoofedAndIncompatibleFields` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/risk_shape_boundaries_test.go:78`<br>`TestExecutionProductAndPriceValidationBoundaries` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/risk_status_broker_boundaries_test.go:13`<br>`TestRiskDecisionAndHardStopBoundarySemantics` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/risk_status_broker_boundaries_test.go:82`<br>`TestOrderStatusMapsEveryBrokerLifecycleFamily` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/risk_status_broker_boundaries_test.go:124`<br>`TestBrokerIdentityMismatchPropagatesAcrossReadAndWriteOperations` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/risk_status_broker_boundaries_test.go:187`<br>`TestLowLevelTradingFallbackHelpers` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/risk_status_broker_boundaries_test.go:228`<br>`TestServiceBrokerResolutionAndPredictionStoreBoundaries` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/service_test.go:8`<br>`TestServiceReadQueryAppliesDefaultMarket` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/service_test.go:24`<br>`TestServiceOrderUpdateDefaultsAreNoops` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:internal/trading/service_test.go:45`<br>`TestServiceOrderUpdatesDelegateToWorkerWhenPresent` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/broker_test.go:13`<br>`TestRegistryBasic` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/broker_test.go:27`<br>`TestRegistryRegisterAndLookup` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/broker_test.go:46`<br>`TestRegistryReplaceUpdatesActiveBroker` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/broker_test.go:67`<br>`TestRegistryRemoveDeletesOnlySelectedBroker` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/broker_test.go:85`<br>`TestRegistryDuplicatePanics` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/broker_test.go:97`<br>`TestConvertFutuReadQuery` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/broker_test.go:114`<br>`TestPointerHelpersReturnStableIndependentValues` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/broker_test.go:157`<br>`TestApplyMarketRuleUsesLotSizeAsQuantityConstraints` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/broker_test.go:175`<br>`TestApplyMarketRuleIgnoresMissingAndInvalidLotSize` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/broker_test.go:194`<br>`TestBrokerError` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/catalog_test.go:11`<br>`TestCapabilityCatalog` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/catalog_test.go:44`<br>`TestAdapterInterfaceSupportRejectsNilMissingAndUnknownImplementations` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/catalog_test.go:75`<br>`TestCapabilityCatalogRejectsUnsafeWriteMCP` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/catalog_test.go:98`<br>`TestBrokerFeatureRouterHonorsExplicitSelectionAndFallback` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/catalog_test.go:141`<br>`TestCapabilityCatalogValidationAndOrderingBranches` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/catalog_test.go:195`<br>`TestBrokerFeatureRouterFailureAndProductBranches` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/catalog_test.go:233`<br>`TestBrokerFeatureRouterCandidateOrderingDeduplicationAndReasonFallback` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/market_rules_snapshot_errors_test.go:14`<br>`TestApplyMarketRulesMatchesAndOverridesConstraints` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/market_rules_snapshot_errors_test.go:33`<br>`TestApplyMarketRuleIgnoresInvalidExplicitConstraints` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/market_rules_snapshot_errors_test.go:43`<br>`TestSymbolScopedSnapshotError` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/market_rules_snapshot_errors_test.go:63`<br>`TestSnapshotRateLimitErrorCarriesRetryDelay` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/market_rules_snapshot_errors_test.go:85`<br>`TestSnapshotAvailabilityErrorsExposeFallbackEligibility` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/product_capability_contracts_test.go:11`<br>`TestPredictionQuoteLegsHashNormalizesBrokerNeutralLegs` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/product_capability_contracts_test.go:26`<br>`TestCapabilityOperationSurfaceFallbacksAndOverrides` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/product_capability_contracts_test.go:89`<br>`TestCapabilityCatalogOperationValidationFailures` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/product_capability_contracts_test.go:159`<br>`TestBrokerFeatureRouterRejectsDeclaredFeatureWithoutAdapterInterface` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/product_capability_contracts_test.go:174`<br>`TestBrokerFeatureRouterRuntimeEvaluatorFailuresAndReasons` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/research_screen_test.go:11`<br>`TestFactorRefIdentityAndStableConstruction` | trading_broker | 高风险 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/research_screen_test.go:31`<br>`TestNewFactorRefRejectsUnrepresentableOrInvalidParameters` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
| [ ] | `go:452dea11:pkg/broker/research_screen_test.go:46`<br>`TestResearchScreenRateLimitErrorContract` | trading_broker | 普通边界 | `crates/jftrade-trading, crates/jftrade-broker` |
