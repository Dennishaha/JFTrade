<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import RuntimeWorkbenchAlert from "@/components/strategy-runtime/RuntimeWorkbenchAlert.vue";
import StrategyRuntimeEmptyWorkbench from "@/components/strategy-runtime/StrategyRuntimeEmptyWorkbench.vue";
import StrategyRuntimeInstanceEditorDialog from "@/components/strategy-runtime/StrategyRuntimeInstanceEditorDialog.vue";
import StrategyRuntimeInstanceListPanel from "@/components/strategy-runtime/StrategyRuntimeInstanceListPanel.vue";
import StrategyRuntimePanelHeader from "@/components/strategy-runtime/StrategyRuntimePanelHeader.vue";
import StrategyRuntimeSelectedStrategyPanel from "@/components/strategy-runtime/StrategyRuntimeSelectedStrategyPanel.vue";
import StrategyRuntimeWorkbenchShell from "@/components/strategy-runtime/StrategyRuntimeWorkbenchShell.vue";
import "@/components/strategy-runtime/strategyRuntimePanel.css";
import {
    buildStrategyBindingPayload,
    formatBrokerAccountSummary,
    formatRuntimeObservationSymbols,
    formatStrategyRuntimeRiskSummary,
    formatStrategyInterval,
    formatStrategySymbols,
    normalizeStrategyRuntimeRiskSettings,
    normalizeText,
    readStrategyBinding,
    resolveBrokerAccountSelectionKey,
} from "@/components/strategy-runtime/strategyRuntimeInstanceBinding";
import type {
    StrategyDefinitionDocument,
    StrategyInstanceItem,
    StrategyRuntimeRiskSettings,
} from "@/types";
import type {
    StrategyAuditEntryDocument,
    StrategyBrokerAccountBinding,
} from "@/contracts";

import {
    apiDeletePath,
    apiGet,
    apiGetPath,
    apiPostPath,
    apiPostPathAction,
    apiPutPath,
} from "@/composables/shared/apiClient";
import { useMarketProfiles } from "@/composables/market-data/marketProfiles";
import { queryClient, queryKeys } from "@/composables/settings/serverState";
import { mapStrategyInstance, mapStrategyInstances } from "@/composables/strategy/strategyContract";
import {
    mapStrategyBindingRequest,
    mapStrategyRuntimeRiskRequest,
} from "@/composables/strategy/strategyApiRequests";
import { useConsoleData } from "@/composables/workspace/useConsoleData";
import {
    formatSourceFormat,
    formatStrategyEligibility,
    formatStrategyRuntime,
} from "@/components/strategy-runtime/strategyRuntimeIdentity";
import { useStrategyRuntimeInstanceEditor } from "@/components/strategy-runtime/useStrategyRuntimeInstanceEditor";
import { useStrategyRuntimeLayout } from "@/components/strategy-runtime/useStrategyRuntimeLayout";
import { useStrategyRuntimeRefresh } from "@/components/strategy-runtime/useStrategyRuntimeRefresh";
import {
    useStrategyRuntimeDetailPresentation,
    useStrategyRuntimeSelection,
} from "@/components/strategy-runtime/useStrategyRuntimePresentation";
import {
    formatStrategyActionError,
    formatStrategyDefinitionSyncSummary,
    formatStrategyExecutionMode,
    formatStrategyStatus,
    formatTimestamp,
    formatTimestampTooltip,
    type StrategyAction,
} from "@/components/strategy-runtime/strategyRuntimePresentation";

type StrategyAuditEntry = StrategyAuditEntryDocument;
const strategyActionTemplates = {
    start: "/api/v1/strategies/{instanceId}/start",
    pause: "/api/v1/strategies/{instanceId}/pause",
    stop: "/api/v1/strategies/{instanceId}/stop",
} as const;

const props = defineProps<{
    /** 设计阶段当前选中的定义数量，供头部统计展示 */
    definitionsCount?: number;
    pendingDefinitionId?: string;
}>();

const emit = defineEmits<{
    "switch-to-design": [payload?: { mode?: "existing" | "new" }];
}>();

const { systemStatus, availableBrokerAccounts, selectedBrokerAccount } = useConsoleData();
const {
    loadMarketProfiles,
    normalizeInstrumentRefWithMarketApi,
} = useMarketProfiles();

const strategyDefinitions = ref<StrategyDefinitionDocument[]>([]);
const strategies = ref<StrategyInstanceItem[]>([]);
const selectedStrategyId = ref("");
const strategyLogs = ref<string[]>([]);
const strategyAuditEntries = ref<StrategyAuditEntry[]>([]);
const isLoadingDefinitions = ref(false);
const isLoadingStrategies = ref(false);
const isLoadingDetails = ref(false);
const isCreatingStrategyInstance = ref(false);
const isUpdatingStrategyBinding = ref(false);
const isUpdatingStrategyRuntimeRisk = ref(false);
const isDeletingStrategy = ref(false);
const isRefreshingStrategyDefinition = ref(false);
const definitionsError = ref("");
const listError = ref("");
const detailsError = ref("");
const instanceMutationNotice = ref("");
const instanceMutationError = ref("");
const isCreateMenuOpen = ref(false);

const {
    selectedStrategy, selectedStrategyBinding, selectedStrategyDefinitionSync,
    selectedStrategyDefinitionDocument, selectedStrategyRuntimeObservation,
    brokerAccountOptions, activeStrategyCount, runtimeRealTradingLabel,
    runtimeKillSwitchLabel, isRefreshingStrategyContent,
    defaultBrokerAccountSelectionKey,
    effectiveCurrentBrokerAccountSelectionKey,
} = useStrategyRuntimeSelection({
    strategies,
    selectedStrategyId,
    strategyDefinitions,
    availableBrokerAccounts,
    selectedBrokerAccount,
    systemStatus,
    isLoadingStrategies,
    isLoadingDetails,
});
const {
    runtimePaneSizes, isCompactStrategyRuntime, isMobileStrategyRuntime,
    strategyRuntimeMobileSection, strategyRuntimeWorkbenchLayout,
    setupStrategyRuntimeMediaQueries, teardownStrategyRuntimeMediaQueries,
    selectStrategyRuntimeMobileSection, handleRuntimePaneResized,
} = useStrategyRuntimeLayout(selectedStrategy);
const {
    clearStrategyRuntimeRefreshTimer, shouldDeferStrategyRuntimeRefresh,
    scheduleStrategyRuntimeRefresh,
    handleStrategyRuntimeVisibilityChange,
} = useStrategyRuntimeRefresh({
    selectedStrategy,
    activeStrategyCount,
    busyStates: [
        isLoadingStrategies,
        isLoadingDetails,
        isCreatingStrategyInstance,
        isUpdatingStrategyBinding,
        isUpdatingStrategyRuntimeRisk,
        isDeletingStrategy,
        isRefreshingStrategyDefinition,
    ],
    refresh: () => refreshStrategyRuntimeContent(),
});
const {
    createDefinitionId, createDefinition, createBindingInstruments,
    createSymbolValidationMessage, createInterval, createChartType,
    createExecutionMode, createRuntimeRisk, createBrokerAccountKey,
    editBindingInstruments, editSymbolValidationMessage, editInterval, editChartType,
    editExecutionMode, editRuntimeRisk, editBrokerAccountKey,
    activeInstanceEditorMode, instanceEditorOpen, activeSymbolTags, activeSymbolDraft,
    activeSymbolValidationMessage, activeIntervalValue, activeChartType,
    activeExecutionMode, activeRuntimeRisk, activeSelectedBrokerAccountOption,
    activeSelectedBrokerAccountKey, activeBrokerAccountQuery,
    activeIsBrokerAccountPickerOpen, activeFilteredBrokerAccountOptions,
    activeInstanceEditorSymbolsSummary, activeInstanceEditorBrokerAccountSummary,
    instanceEditorPreviewDefinitionLabel, instanceEditorTitle, instanceEditorHint,
    acceptActiveResolvedInstrument, removeActiveSymbol, updateActiveSymbolDraft,
    handleActiveSymbolDraftKeydown, handleActiveSymbolDraftPaste,
    updateActiveIntervalValue, updateActiveChartType, updateActiveExecutionMode,
    updateActiveRuntimeRiskMode, updateActiveRuntimeRiskCloseOnly,
    updateActiveRuntimeRiskPauseOnReject, updateActiveRuntimeRiskNumber,
    toggleActiveBrokerAccountPicker, updateActiveBrokerAccountQuery,
    clearActiveBrokerAccountSelection, selectActiveBrokerAccount,
    openCreateInstanceForm: openCreateInstanceEditorForm,
    openEditInstanceForm: openEditInstanceEditorForm,
    closeInstanceEditorDialog: closeInstanceEditorState,
} = useStrategyRuntimeInstanceEditor({
    strategyDefinitions,
    selectedStrategy,
    selectedStrategyBinding,
    brokerAccountOptions,
    selectedBrokerAccount,
    defaultBrokerAccountSelectionKey,
    pendingDefinitionId: () => props.pendingDefinitionId,
    onPendingDefinitionSelected: () => {
        isCreateMenuOpen.value = false;
    },
    normalizeInstrumentRefWithMarketApi,
});

const {
    selectedStrategyParamsJson,
    selectedStrategyRuntimeLabel,
    selectedStrategySourceFormatLabel,
    selectedStrategyStartHint,
    selectedStrategyCompiledSummary,
    canRefreshSelectedStrategyDefinition,
    selectedStrategyDefinitionRefreshHint,
    canStartSelectedStrategy,
    canPauseSelectedStrategy,
    canStopSelectedStrategy,
    canCreateStrategyInstance,
    canUpdateSelectedStrategyBinding,
    canDeleteSelectedStrategy,
} = useStrategyRuntimeDetailPresentation({
    selectedStrategy,
    selectedStrategyBinding,
    selectedStrategyDefinitionSync,
    createDefinitionId,
    createInterval,
    isLoadingDefinitions,
    isCreatingStrategyInstance,
    isLoadingDetails,
    isRefreshingStrategyDefinition,
    isUpdatingStrategyBinding,
    isDeletingStrategy,
});

const instanceEditorDialogProps = computed(() => ({
    mode: activeInstanceEditorMode.value,
    title: instanceEditorTitle.value,
    hint: instanceEditorHint.value,
    isLoadingDefinitions: isLoadingDefinitions.value,
    definitionsError: definitionsError.value,
    strategyDefinitions: strategyDefinitions.value,
    createDefinitionId: createDefinitionId.value,
    createDefinition: createDefinition.value,
    selectedStrategy: selectedStrategy.value,
    symbolTags: activeSymbolTags.value,
    symbolDraft: activeSymbolDraft.value,
    symbolValidationMessage: activeSymbolValidationMessage.value,
    intervalValue: activeIntervalValue.value,
    chartType: activeChartType.value,
    executionMode: activeExecutionMode.value,
    runtimeRisk: activeRuntimeRisk.value,
    selectedBrokerAccountOption: activeSelectedBrokerAccountOption.value,
    selectedBrokerAccountKey: activeSelectedBrokerAccountKey.value,
    currentBrokerAccountSelectionKey: effectiveCurrentBrokerAccountSelectionKey.value,
    isBrokerAccountPickerOpen: activeIsBrokerAccountPickerOpen.value,
    brokerAccountQuery: activeBrokerAccountQuery.value,
    filteredBrokerAccountOptions: activeFilteredBrokerAccountOptions.value,
    previewDefinitionLabel: instanceEditorPreviewDefinitionLabel.value,
    symbolsSummary: activeInstanceEditorSymbolsSummary.value,
    brokerAccountSummary: activeInstanceEditorBrokerAccountSummary.value,
    canCreateStrategyInstance: canCreateStrategyInstance.value,
    canUpdateSelectedStrategyBinding: canUpdateSelectedStrategyBinding.value,
    canDeleteSelectedStrategy: canDeleteSelectedStrategy.value,
    isCreatingStrategyInstance: isCreatingStrategyInstance.value,
    isUpdatingStrategyBinding: isUpdatingStrategyBinding.value,
    isDeletingStrategy: isDeletingStrategy.value,
}));

const instanceEditorDialogListeners = {
    "refresh-definitions": () => {
        void loadStrategyDefinitions();
    },
    "switch-to-design": openCreateDefinition,
    "update:create-definition-id": (value: string) => {
        createDefinitionId.value = value;
    },
    "remove-symbol": removeActiveSymbol,
    "resolve-symbol": acceptActiveResolvedInstrument,
    "update:symbol-draft": updateActiveSymbolDraft,
    "symbol-draft-keydown": handleActiveSymbolDraftKeydown,
    "symbol-draft-paste": handleActiveSymbolDraftPaste,
    "update:interval": updateActiveIntervalValue,
    "update:chart-type": updateActiveChartType,
    "update:execution-mode": updateActiveExecutionMode,
    "update:runtime-risk-mode": updateActiveRuntimeRiskMode,
    "update:runtime-risk-close-only": updateActiveRuntimeRiskCloseOnly,
    "update:runtime-risk-pause-on-reject": updateActiveRuntimeRiskPauseOnReject,
    "update:runtime-risk-number": updateActiveRuntimeRiskNumber,
    "toggle-broker-picker": toggleActiveBrokerAccountPicker,
    "update:broker-query": updateActiveBrokerAccountQuery,
    "clear-broker-selection": clearActiveBrokerAccountSelection,
    "select-broker-selection": selectActiveBrokerAccount,
    "submit-create": createStrategyInstance,
    "submit-update": updateSelectedStrategyBinding,
    "submit-delete": deleteSelectedStrategy,
};

onMounted(() => {
    if (typeof document !== "undefined") {
        document.addEventListener("visibilitychange", handleStrategyRuntimeVisibilityChange);
    }
    setupStrategyRuntimeMediaQueries();
    void Promise.all([loadMarketProfiles(), loadStrategyDefinitions(), loadStrategies()]);
});

onUnmounted(() => {
    clearStrategyRuntimeRefreshTimer();
    teardownStrategyRuntimeMediaQueries();
    if (typeof document !== "undefined") {
        document.removeEventListener("visibilitychange", handleStrategyRuntimeVisibilityChange);
    }
});

function isCurrentBrokerAccountSelectionKey(selectionKey: string | null | undefined): boolean {
    return selectionKey != null && selectionKey !== "" && selectionKey === effectiveCurrentBrokerAccountSelectionKey.value;
}

function isCurrentBrokerAccountBinding(
    brokerAccount: StrategyBrokerAccountBinding | null | undefined,
): boolean {
    return isCurrentBrokerAccountSelectionKey(
        resolveBrokerAccountSelectionKey(brokerAccountOptions.value, brokerAccount),
    );
}

function clearRuntimeDetails(): void {
    strategyLogs.value = [];
    strategyAuditEntries.value = [];
}

function clearInstanceMutationMessages(): void {
    instanceMutationNotice.value = "";
    instanceMutationError.value = "";
}

function closeInstanceMutationNotice(): void {
    instanceMutationNotice.value = "";
}

function closeInstanceMutationError(): void {
    instanceMutationError.value = "";
}

async function loadStrategyDefinitions(): Promise<void> {
    isLoadingDefinitions.value = true;
    definitionsError.value = "";

    try {
        strategyDefinitions.value = await queryClient.ensureQueryData({
            queryKey: queryKeys.strategyDefinitions(),
            queryFn: () => apiGet("/api/v1/strategy-definitions"),
        });
    } catch (error) {
        definitionsError.value =
            error instanceof Error ? error.message : "加载策略定义失败。";
    } finally {
        isLoadingDefinitions.value = false;
    }
}

async function loadStrategies(preferredId = selectedStrategyId.value): Promise<void> {
    isLoadingStrategies.value = true;
    listError.value = "";

    try {
        const items = mapStrategyInstances(await apiGet("/api/v1/strategies"));
        strategies.value = items;

        if (items.length === 0) {
            selectedStrategyId.value = "";
            clearRuntimeDetails();
            return;
        }

        const nextId =
            items.find((item) => item.id === preferredId)?.id ?? items[0]?.id ?? "";

        if (nextId !== "") {
            await loadStrategyDetails(nextId);
        }
    } catch (error) {
        listError.value =
            error instanceof Error ? error.message : "加载策略实例失败。";
    } finally {
        isLoadingStrategies.value = false;
        scheduleStrategyRuntimeRefresh();
    }
}

async function loadStrategyDetails(instanceId: string): Promise<void> {
    const previousStrategyId = selectedStrategyId.value;
    if (previousStrategyId === "") {
        selectedStrategyId.value = instanceId;
    }
    detailsError.value = "";
    isLoadingDetails.value = true;

    const logsUrl = new URL(`/api/v1/strategies/${encodeURIComponent(instanceId)}/logs`, window.location.origin);
    logsUrl.searchParams.set("limit", "500");
    const auditUrl = new URL(`/api/v1/strategies/${encodeURIComponent(instanceId)}/audit`, window.location.origin);
    auditUrl.searchParams.set("limit", "500");

    try {
        const [logs, audit] = await Promise.all([
            apiGetPath(
                "/api/v1/strategies/{instanceId}/logs",
                `${logsUrl.pathname}${logsUrl.search}`,
            ),
            apiGetPath(
                "/api/v1/strategies/{instanceId}/audit",
                `${auditUrl.pathname}${auditUrl.search}`,
            ),
        ]);

        strategyLogs.value = logs.logs;
        strategyAuditEntries.value = audit.entries;
        selectedStrategyId.value = instanceId;
    } catch (error) {
        if (previousStrategyId !== "") {
            selectedStrategyId.value = previousStrategyId;
        }
        detailsError.value =
            error instanceof Error ? error.message : "加载策略明细失败。";
    } finally {
        isLoadingDetails.value = false;
        scheduleStrategyRuntimeRefresh();
    }
}

async function refreshStrategyRuntimeContent(): Promise<void> {
    clearStrategyRuntimeRefreshTimer();
    if (typeof document !== "undefined" && document.visibilityState === "hidden") {
        return;
    }
    if (shouldDeferStrategyRuntimeRefresh()) {
        scheduleStrategyRuntimeRefresh();
        return;
    }
    await loadStrategies(selectedStrategyId.value);
}

async function selectStrategy(instanceId: string): Promise<void> {
    clearStrategyRuntimeRefreshTimer();
    await loadStrategyDetails(instanceId);
    strategyRuntimeMobileSection.value = "workbench";
}

async function createStrategyInstance(): Promise<void> {
    clearInstanceMutationMessages();
    if (createSymbolValidationMessage.value !== "") {
        instanceMutationError.value = createSymbolValidationMessage.value;
        return;
    }
    if (normalizeText(activeSymbolDraft.value) !== "") {
        instanceMutationError.value = "请先解析并确认待添加的交易代码。";
        return;
    }

    if (createDefinitionId.value.trim() === "") {
        instanceMutationError.value = "请先选择已保存的策略定义。";
        return;
    }

    isCreatingStrategyInstance.value = true;

    try {
        const instance = mapStrategyInstance(await apiPostPath(
            "/api/v1/strategy-definitions/{definitionId}/instantiate",
            `/api/v1/strategy-definitions/${encodeURIComponent(createDefinitionId.value)}/instantiate`,
            mapStrategyBindingRequest(buildStrategyBindingPayload({
                    brokerAccountOptions: brokerAccountOptions.value,
                    instruments: createBindingInstruments.value,
                    interval: createInterval.value,
                    chartType: createChartType.value,
                    executionMode: createExecutionMode.value,
                    runtimeRisk: createRuntimeRisk.value,
                    brokerAccountKey: createBrokerAccountKey.value,
            })),
        ));

        instanceMutationNotice.value = `已创建实例：${instance.definition.name}。`;
        await loadStrategies(instance.id);
        closeInstanceEditorDialog();
    } catch (error) {
        instanceMutationError.value =
            error instanceof Error ? error.message : "创建策略实例失败。";
    } finally {
        isCreatingStrategyInstance.value = false;
    }
}

function toggleCreateMenu(): void {
    isCreateMenuOpen.value = !isCreateMenuOpen.value;
}

function openCreateDefinition(): void {
    isCreateMenuOpen.value = false;
    closeInstanceEditorDialog();
    emit("switch-to-design", { mode: "new" });
}

function openCreateInstanceForm(): void {
    isCreateMenuOpen.value = false;
    openCreateInstanceEditorForm();
}

function openEditInstanceForm(): void {
    if (selectedStrategy.value === null) {
        return;
    }
    isCreateMenuOpen.value = false;
    openEditInstanceEditorForm();
}

function closeInstanceEditorDialog(): void {
    isCreateMenuOpen.value = false;
    closeInstanceEditorState();
}

async function updateSelectedStrategyBinding(): Promise<void> {
    clearInstanceMutationMessages();
    if (editSymbolValidationMessage.value !== "") {
        instanceMutationError.value = editSymbolValidationMessage.value;
        return;
    }
    if (normalizeText(activeSymbolDraft.value) !== "") {
        instanceMutationError.value = "请先解析并确认待添加的交易代码。";
        return;
    }

    if (selectedStrategy.value === null) {
        instanceMutationError.value = "请先选择策略实例。";
        return;
    }
    if (selectedStrategy.value.status !== "STOPPED") {
        instanceMutationError.value = "仅已停止的实例允许修改绑定。";
        return;
    }

    isUpdatingStrategyBinding.value = true;

    try {
        const updated = mapStrategyInstance(await apiPutPath(
            "/api/v1/strategies/{instanceId}",
            `/api/v1/strategies/${encodeURIComponent(selectedStrategy.value.id)}`,
            mapStrategyBindingRequest(buildStrategyBindingPayload({
                    brokerAccountOptions: brokerAccountOptions.value,
                    instruments: editBindingInstruments.value,
                    interval: editInterval.value,
                    chartType: editChartType.value,
                    executionMode: editExecutionMode.value,
                    runtimeRisk: editRuntimeRisk.value,
                    brokerAccountKey: editBrokerAccountKey.value,
                    fallbackBrokerAccount: selectedStrategyBinding.value?.brokerAccount ?? null,
            })),
        ));

        instanceMutationNotice.value = `已更新实例绑定：${updated.definition.name}。`;
        await loadStrategies(updated.id);
        closeInstanceEditorDialog();
    } catch (error) {
        instanceMutationError.value =
            error instanceof Error ? error.message : "更新实例绑定失败。";
    } finally {
        isUpdatingStrategyBinding.value = false;
    }
}

async function updateSelectedStrategyRuntimeRisk(patch: Partial<StrategyRuntimeRiskSettings>): Promise<void> {
    clearInstanceMutationMessages();
    if (selectedStrategy.value === null || selectedStrategyBinding.value === null) {
        instanceMutationError.value = "请先选择策略实例。";
        return;
    }
    const runtimeRisk = normalizeStrategyRuntimeRiskSettings({
        ...selectedStrategyBinding.value.runtimeRisk,
        ...patch,
    });
    isUpdatingStrategyRuntimeRisk.value = true;
    try {
        const updated = mapStrategyInstance(await apiPutPath(
            "/api/v1/strategies/{instanceId}/runtime-risk",
            `/api/v1/strategies/${encodeURIComponent(selectedStrategy.value.id)}/runtime-risk`,
            mapStrategyRuntimeRiskRequest(runtimeRisk),
        ));
        instanceMutationNotice.value = `已更新动态风控：${formatStrategyRuntimeRiskSummary(runtimeRisk)}。`;
        await loadStrategies(updated.id);
    } catch (error) {
        instanceMutationError.value =
            error instanceof Error ? error.message : "更新动态风控失败。";
    } finally {
        isUpdatingStrategyRuntimeRisk.value = false;
    }
}

async function deleteSelectedStrategy(): Promise<void> {
    clearInstanceMutationMessages();

    if (selectedStrategy.value === null) {
        instanceMutationError.value = "请先选择策略实例。";
        return;
    }
    if (selectedStrategy.value.status !== "STOPPED") {
        instanceMutationError.value = "仅已停止的实例允许删除。";
        return;
    }

    isDeletingStrategy.value = true;

    try {
        const removed = mapStrategyInstance(await apiDeletePath(
            "/api/v1/strategies/{instanceId}",
            `/api/v1/strategies/${encodeURIComponent(selectedStrategy.value.id)}`,
        ));

        instanceMutationNotice.value = `已删除实例：${removed.definition.name}。`;
        closeInstanceEditorDialog();
        await loadStrategies();
    } catch (error) {
        instanceMutationError.value =
            error instanceof Error ? error.message : "删除策略实例失败。";
    } finally {
        isDeletingStrategy.value = false;
    }
}

async function changeStrategyStatus(action: StrategyAction): Promise<void> {
    detailsError.value = "";

    if (selectedStrategy.value === null) {
        detailsError.value = "请先选择策略实例。";
        return;
    }
    isLoadingDetails.value = true;

    try {
        await apiPostPathAction(
            strategyActionTemplates[action],
            `/api/v1/strategies/${encodeURIComponent(selectedStrategy.value.id)}/${action}`,
        );
        await loadStrategies(selectedStrategy.value.id);
    } catch (error) {
        detailsError.value = formatStrategyActionError(action, error);
    } finally {
        isLoadingDetails.value = false;
    }
}

async function refreshSelectedStrategyDefinition(): Promise<void> {
    clearInstanceMutationMessages();

    if (selectedStrategy.value === null) {
        instanceMutationError.value = "请先选择策略实例。";
        return;
    }
    if (selectedStrategyDefinitionSync.value === null || selectedStrategyDefinitionSync.value.isLatest) {
        instanceMutationNotice.value = "当前实例已经是最新策略版本。";
        return;
    }
    if (!selectedStrategyDefinitionSync.value.canApplyLatest) {
        instanceMutationError.value =
            selectedStrategyDefinitionSync.value.blockedReason ?? "当前实例需要先停止后再刷新。";
        return;
    }

    isRefreshingStrategyDefinition.value = true;

    try {
        const updated = mapStrategyInstance(await apiPostPathAction(
            "/api/v1/strategies/{instanceId}/refresh-definition",
            `/api/v1/strategies/${encodeURIComponent(selectedStrategy.value.id)}/refresh-definition`,
        ));
        instanceMutationNotice.value = `已刷新实例策略到最新版本：${updated.definition.name} / v${updated.definition.version}。`;
        await loadStrategies(updated.id);
    } catch (error) {
        instanceMutationError.value =
            error instanceof Error ? error.message : "刷新实例策略失败。";
    } finally {
        isRefreshingStrategyDefinition.value = false;
    }
}
</script>

<template>
    <div class="runtime-panel">
        <StrategyRuntimePanelHeader
            :active-strategy-count="activeStrategyCount"
            :definitions-count="props.definitionsCount ?? 0"
            :default-trading-environment="systemStatus.defaultTradingEnvironment"
            :runtime-real-trading-label="runtimeRealTradingLabel"
            :is-kill-switch-active="systemStatus.realTradingKillSwitch.active"
            :runtime-kill-switch-label="runtimeKillSwitchLabel"
            :runtime-risk-summary="formatStrategyRuntimeRiskSummary(selectedStrategyBinding?.runtimeRisk)"
        />

        <StrategyRuntimeWorkbenchShell
            :layout="strategyRuntimeWorkbenchLayout"
            :runtime-pane-sizes="runtimePaneSizes"
            :mobile-section="strategyRuntimeMobileSection"
            :has-selected-detail="selectedStrategy !== null"
            @resized="handleRuntimePaneResized"
            @update:mobile-section="selectStrategyRuntimeMobileSection"
        >
            <template #messages>
                <RuntimeWorkbenchAlert
                    v-if="instanceMutationNotice"
                    close-label="关闭提示"
                    close-test-id="strategy-instance-mutation-notice-close"
                    tone="success"
                    @close="closeInstanceMutationNotice"
                >
                    {{ instanceMutationNotice }}
                </RuntimeWorkbenchAlert>
                <RuntimeWorkbenchAlert
                    v-if="instanceMutationError"
                    close-label="关闭错误"
                    close-test-id="strategy-instance-mutation-error-close"
                    tone="error"
                    @close="closeInstanceMutationError"
                >
                    {{ instanceMutationError }}
                </RuntimeWorkbenchAlert>
            </template>

            <template #list>
                        <StrategyRuntimeInstanceListPanel
                            :is-create-menu-open="isCreateMenuOpen"
                            :is-loading-strategies="isLoadingStrategies"
                            :list-error="listError"
                            :strategies="strategies"
                            :selected-strategy-id="selectedStrategyId"
                            :format-strategy-status="formatStrategyStatus"
                            :format-strategy-definition-sync-summary="formatStrategyDefinitionSyncSummary"
                            :format-strategy-symbols="formatStrategySymbols"
                            :format-strategy-interval="formatStrategyInterval"
                            :format-broker-account-summary="formatBrokerAccountSummary"
                            :read-strategy-binding="readStrategyBinding"
                            :is-current-broker-account-binding="isCurrentBrokerAccountBinding"
                            :format-timestamp="formatTimestamp"
                            :format-timestamp-tooltip="formatTimestampTooltip"
                            :format-strategy-runtime="formatStrategyRuntime"
                            :format-source-format="formatSourceFormat"
                            :format-strategy-eligibility="formatStrategyEligibility"
                            :format-strategy-execution-mode="formatStrategyExecutionMode"
                            @toggle-create-menu="toggleCreateMenu"
                            @open-create-definition="openCreateDefinition"
                            @open-create-instance="openCreateInstanceForm"
                            @refresh-strategies="refreshStrategyRuntimeContent"
                            @select-strategy="selectStrategy($event)"
                        >
                            <StrategyRuntimeInstanceEditorDialog
                                v-model:open="instanceEditorOpen"
                                v-bind="instanceEditorDialogProps"
                                v-on="instanceEditorDialogListeners"
                            />
                        </StrategyRuntimeInstanceListPanel>
            </template>

            <template #detail>
                        <StrategyRuntimeEmptyWorkbench v-if="selectedStrategy === null" />
                        <StrategyRuntimeSelectedStrategyPanel
                            v-else
                            :key="selectedStrategy.id"
                            :selected-strategy="selectedStrategy"
                            :selected-strategy-binding="selectedStrategyBinding"
                            :selected-strategy-definition-sync="selectedStrategyDefinitionSync"
                            :selected-strategy-runtime-observation="selectedStrategyRuntimeObservation"
                            :is-loading-details="isLoadingDetails"
                            :strategy-logs="strategyLogs"
                            :strategy-audit-entries="strategyAuditEntries"
                            :selected-strategy-params-json="selectedStrategyParamsJson"
                            :is-refreshing-strategy-definition="isRefreshingStrategyDefinition"
                            :can-refresh-selected-strategy-definition="canRefreshSelectedStrategyDefinition"
                            :selected-strategy-definition-refresh-hint="selectedStrategyDefinitionRefreshHint"
                            :selected-strategy-runtime-label="selectedStrategyRuntimeLabel"
                            :selected-strategy-source-format-label="selectedStrategySourceFormatLabel"
                            :selected-strategy-start-hint="selectedStrategyStartHint"
                            :selected-strategy-compiled-summary="selectedStrategyCompiledSummary"
                            :is-refreshing-strategy-content="isRefreshingStrategyContent"
                            :is-updating-strategy-runtime-risk="isUpdatingStrategyRuntimeRisk"
                            :can-start-selected-strategy="canStartSelectedStrategy"
                            :can-pause-selected-strategy="canPauseSelectedStrategy"
                            :can-stop-selected-strategy="canStopSelectedStrategy"
                            :details-error="detailsError"
                            :format-strategy-definition-sync-summary="formatStrategyDefinitionSyncSummary"
                            :format-strategy-symbols="formatStrategySymbols"
                            :format-strategy-interval="formatStrategyInterval"
                            :format-strategy-execution-mode="formatStrategyExecutionMode"
                            :format-strategy-runtime-risk-summary="formatStrategyRuntimeRiskSummary"
                            :format-broker-account-summary="formatBrokerAccountSummary"
                            :is-current-broker-account-binding="isCurrentBrokerAccountBinding"
                            :format-strategy-eligibility="formatStrategyEligibility"
                            :format-strategy-status="formatStrategyStatus"
                            :format-runtime-observation-symbols="formatRuntimeObservationSymbols"
                            :format-timestamp="formatTimestamp"
                            :format-timestamp-tooltip="formatTimestampTooltip"
                            @open-edit="openEditInstanceForm"
                            @refresh-content="refreshStrategyRuntimeContent"
                            @refresh-definition="refreshSelectedStrategyDefinition"
                            @update-runtime-risk="updateSelectedStrategyRuntimeRisk"
                            @change-status="changeStrategyStatus"
                        />
            </template>
        </StrategyRuntimeWorkbenchShell>
    </div>
</template>
