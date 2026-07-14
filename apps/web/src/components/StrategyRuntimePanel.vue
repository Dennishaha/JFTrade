<script setup lang="ts">
import type { SplitpanesResizedPayload } from "splitpanes";
import { computed, onMounted, onUnmounted, ref } from "vue";
import RuntimeWorkbenchAlert from "./strategy-runtime/RuntimeWorkbenchAlert.vue";
import StrategyRuntimeEmptyWorkbench from "./strategy-runtime/StrategyRuntimeEmptyWorkbench.vue";
import StrategyRuntimeInstanceEditorDialog from "./strategy-runtime/StrategyRuntimeInstanceEditorDialog.vue";
import StrategyRuntimeInstanceListPanel from "./strategy-runtime/StrategyRuntimeInstanceListPanel.vue";
import StrategyRuntimePanelHeader from "./strategy-runtime/StrategyRuntimePanelHeader.vue";
import StrategyRuntimeSelectedStrategyPanel from "./strategy-runtime/StrategyRuntimeSelectedStrategyPanel.vue";
import StrategyRuntimeWorkbenchShell from "./strategy-runtime/StrategyRuntimeWorkbenchShell.vue";
import "./strategy-runtime/strategyRuntimePanel.css";
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
} from "./strategy-runtime/strategyRuntimeInstanceBinding";
import type {
    StrategyAuditEntryDocument,
    StrategyAuditListResponse,
    StrategyBrokerAccountBinding,
    StrategyDefinitionDocument,
    StrategyDefinitionSyncStatus,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
    StrategyRuntimeRiskSettings,
    StrategyLogListResponse,
    StrategyRuntimeObservation,
} from "@/contracts";

import { ApiClientError, apiGet, fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import { useMarketProfiles } from "../composables/marketProfiles";
import { queryClient, queryKeys } from "../composables/serverState";
import { useConsoleData } from "../composables/useConsoleData";
import { formatLocalDateTime } from "../utils/dateTime";
import {
    formatSourceFormat,
    formatStrategyEligibility,
    formatStrategyRuntime,
    isPineWorkerRuntime,
} from "./strategy-runtime/strategyRuntimeIdentity";
import { useStrategyRuntimeInstanceEditor } from "./strategy-runtime/useStrategyRuntimeInstanceEditor";

type StrategyLogsResponse = StrategyLogListResponse;
type StrategyAuditEntry = StrategyAuditEntryDocument;
type StrategyAuditResponse = StrategyAuditListResponse;

const STRATEGY_RUNTIME_ACTIVE_REFRESH_MS = 1_000;
const STRATEGY_RUNTIME_IDLE_REFRESH_MS = 3_000;
const STRATEGY_RUNTIME_COMPACT_MEDIA_QUERY = "(max-width: 1180px)";
const STRATEGY_RUNTIME_MOBILE_MEDIA_QUERY = "(max-width: 768px)";

type StrategyAction = "start" | "pause" | "stop";
type StrategyRuntimeWorkbenchLayout = "desktop" | "compact" | "mobile";
type StrategyRuntimeMobileSection = "instances" | "workbench";

interface StrategyTimestampParts {
    display: string;
    timestampMs: number | null;
}

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
const runtimePaneSizes = ref<[number, number]>([30, 70]);
const isCompactStrategyRuntime = ref(false);
const isMobileStrategyRuntime = ref(false);
const strategyRuntimeMobileSection = ref<StrategyRuntimeMobileSection>("instances");
let strategyRuntimeRefreshTimer: number | null = null;
let compactStrategyRuntimeMediaQuery: MediaQueryList | null = null;
let mobileStrategyRuntimeMediaQuery: MediaQueryList | null = null;

const selectedStrategy = computed(
    () => strategies.value.find((item) => item.id === selectedStrategyId.value) ?? null,
);

const selectedStrategyBinding = computed<StrategyInstanceBindingDocument | null>(() => {
    if (selectedStrategy.value === null) {
        return null;
    }
    return readStrategyBinding(selectedStrategy.value);
});

const selectedStrategyDefinitionSync = computed<StrategyDefinitionSyncStatus | null>(
    () => selectedStrategy.value?.definitionSync ?? null,
);

const selectedStrategyDefinitionDocument = computed<StrategyDefinitionDocument | null>(() => {
    if (selectedStrategy.value === null) {
        return null;
    }
    const definitionId = selectedStrategy.value.definition.strategyId;
    return strategyDefinitions.value.find((definition) => definition.id === definitionId) ?? null;
});

const selectedStrategyRuntimeObservation = computed<StrategyRuntimeObservation | null>(
    () => selectedStrategy.value?.runtimeObservation ?? null,
);

const brokerAccountOptions = computed(() => availableBrokerAccounts.value);

const activeStrategyCount = computed(
    () => strategies.value.filter((item) => item.runtimeObservation?.actualStatus === "RUNNING").length,
);

const runtimeRealTradingLabel = computed(() =>
    systemStatus.value.realTradingEnabled ? "已开启" : "已关闭",
);

const runtimeKillSwitchLabel = computed(() =>
    systemStatus.value.realTradingKillSwitch.active ? "已启用" : "未启用",
);

const isRefreshingStrategyContent = computed(
    () => isLoadingStrategies.value || isLoadingDetails.value,
);

const strategyRuntimeWorkbenchLayout = computed<StrategyRuntimeWorkbenchLayout>(() => {
    if (isMobileStrategyRuntime.value) {
        return "mobile";
    }
    return isCompactStrategyRuntime.value ? "compact" : "desktop";
});

const defaultBrokerAccountSelectionKey = computed(
    () => selectedBrokerAccount.value?.selectionKey ?? brokerAccountOptions.value[0]?.selectionKey ?? "",
);

const currentBrokerAccountSelectionKey = computed(
    () => selectedBrokerAccount.value?.selectionKey ?? "",
);

const effectiveCurrentBrokerAccountSelectionKey = computed(
    () => currentBrokerAccountSelectionKey.value || defaultBrokerAccountSelectionKey.value,
);

const {
    createDefinitionId,
    createDefinition,
    createBindingInstruments,
    createSymbolValidationMessage,
    createInterval,
    createExecutionMode,
    createRuntimeRisk,
    createBrokerAccountKey,
    editBindingInstruments,
    editSymbolValidationMessage,
    editInterval,
    editExecutionMode,
    editRuntimeRisk,
    editBrokerAccountKey,
    activeInstanceEditorMode,
    instanceEditorOpen,
    activeSymbolTags,
    activeSymbolDraft,
    activeSymbolValidationMessage,
    activeIntervalValue,
    activeExecutionMode,
    activeRuntimeRisk,
    activeSelectedBrokerAccountOption,
    activeSelectedBrokerAccountKey,
    activeBrokerAccountQuery,
    activeIsBrokerAccountPickerOpen,
    activeFilteredBrokerAccountOptions,
    activeInstanceEditorSymbolsSummary,
    activeInstanceEditorBrokerAccountSummary,
    instanceEditorPreviewDefinitionLabel,
    instanceEditorTitle,
    instanceEditorHint,
    acceptActiveResolvedInstrument,
    removeActiveSymbol,
    updateActiveSymbolDraft,
    handleActiveSymbolDraftKeydown,
    handleActiveSymbolDraftPaste,
    updateActiveIntervalValue,
    updateActiveExecutionMode,
    updateActiveRuntimeRiskMode,
    updateActiveRuntimeRiskCloseOnly,
    updateActiveRuntimeRiskPauseOnReject,
    updateActiveRuntimeRiskNumber,
    toggleActiveBrokerAccountPicker,
    updateActiveBrokerAccountQuery,
    clearActiveBrokerAccountSelection,
    selectActiveBrokerAccount,
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

const selectedStrategyParamsJson = computed(() => {
    if (selectedStrategy.value === null) return "";
    return JSON.stringify(selectedStrategy.value.params, null, 2);
});

const selectedStrategyRuntimeLabel = computed(() => {
    if (selectedStrategy.value === null) return "暂无";
    return formatStrategyRuntime(selectedStrategy.value.runtime);
});

const selectedStrategySourceFormatLabel = computed(() => {
    if (selectedStrategy.value === null) return "暂无";
    return formatSourceFormat(selectedStrategy.value.sourceFormat);
});

const selectedStrategyStartHint = computed(() => {
    if (selectedStrategy.value === null) return "请选择策略实例。";
    if (selectedStrategyBinding.value?.executionMode === "notify_only") {
        return "当前实例为仅通知模式：触发信号只发送准备下单通知，不自动下单。";
    }
    if (selectedStrategy.value.startable) {
        return "当前实例已接入策略控制面生命周期，可启动、暂停、停止。";
    }
    if (isPineWorkerRuntime(selectedStrategy.value.runtime)) {
        return "当前实例已完成 Pine 编译与 requirements 规划，但暂不可启动。";
    }
    return "当前实例暂不可启动。";
});

const selectedStrategyCompiledSummary = computed(() => {
    if (selectedStrategy.value === null || !isPineWorkerRuntime(selectedStrategy.value.runtime)) {
        return "";
    }
    const hookCount = readCompiledHookCount(selectedStrategy.value);
    const indicatorCount = readCompiledIndicatorCount(selectedStrategy.value);
    const parts: string[] = [];
    if (hookCount !== null) parts.push(`${hookCount} 个 hook`);
    if (indicatorCount !== null) parts.push(`${indicatorCount} 项依赖`);
    if (parts.length === 0) return "已完成 Pine v6 主路径编译规划。";
    return `已完成 Pine v6 主路径编译规划，包含 ${parts.join(" / ")}。`;
});

const canRefreshSelectedStrategyDefinition = computed(
    () =>
        selectedStrategy.value !== null
        && selectedStrategyDefinitionSync.value !== null
        && !selectedStrategyDefinitionSync.value.isLatest
        && selectedStrategyDefinitionSync.value.canApplyLatest
        && !isLoadingDetails.value
        && !isRefreshingStrategyDefinition.value,
);

const selectedStrategyDefinitionRefreshHint = computed(() => {
    if (selectedStrategyDefinitionSync.value === null) {
        return "";
    }
    if (selectedStrategyDefinitionSync.value.isLatest) {
        return "当前实例已采用最新保存版本。";
    }
    if (selectedStrategyDefinitionSync.value.canApplyLatest) {
        return `当前实例版本为 v${selectedStrategyDefinitionSync.value.appliedVersion}，可刷新到最新设计 v${selectedStrategyDefinitionSync.value.latestVersion}。`;
    }
    return selectedStrategyDefinitionSync.value.blockedReason ?? "当前实例需要先停止后再刷新。";
});

const canStartSelectedStrategy = computed(
    () =>
        selectedStrategy.value !== null
        && !isLoadingDetails.value
        && selectedStrategy.value.startable
        && selectedStrategy.value.status !== "RUNNING",
);

const canPauseSelectedStrategy = computed(
    () =>
        selectedStrategy.value !== null
        && !isLoadingDetails.value
        && selectedStrategy.value.startable
        && selectedStrategy.value.status === "RUNNING",
);

const canStopSelectedStrategy = computed(
    () =>
        selectedStrategy.value !== null
        && !isLoadingDetails.value
        && selectedStrategy.value.startable
        && selectedStrategy.value.status !== "STOPPED",
);

const canCreateStrategyInstance = computed(
    () =>
        !isLoadingDefinitions.value
        && !isCreatingStrategyInstance.value
        && createDefinitionId.value.trim() !== ""
        && createInterval.value.trim() !== "",
);

const canUpdateSelectedStrategyBinding = computed(
    () =>
        selectedStrategy.value !== null
        && selectedStrategy.value.status === "STOPPED"
        && !isLoadingDetails.value
        && !isUpdatingStrategyBinding.value,
);

const canDeleteSelectedStrategy = computed(
    () =>
        selectedStrategy.value !== null
        && selectedStrategy.value.status === "STOPPED"
        && !isLoadingDetails.value
        && !isDeletingStrategy.value,
);

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

function syncCompactStrategyRuntime(event: MediaQueryListEvent | MediaQueryList): void {
    isCompactStrategyRuntime.value = event.matches;
}

function syncMobileStrategyRuntime(event: MediaQueryListEvent | MediaQueryList): void {
    isMobileStrategyRuntime.value = event.matches;
    if (!event.matches && strategyRuntimeMobileSection.value !== "instances") {
        strategyRuntimeMobileSection.value = "instances";
    }
}

function setupStrategyRuntimeMediaQueries(): void {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
        return;
    }
    compactStrategyRuntimeMediaQuery = window.matchMedia(STRATEGY_RUNTIME_COMPACT_MEDIA_QUERY);
    mobileStrategyRuntimeMediaQuery = window.matchMedia(STRATEGY_RUNTIME_MOBILE_MEDIA_QUERY);
    isCompactStrategyRuntime.value = compactStrategyRuntimeMediaQuery.matches;
    isMobileStrategyRuntime.value = mobileStrategyRuntimeMediaQuery.matches;

    if (typeof compactStrategyRuntimeMediaQuery.addEventListener === "function") {
        compactStrategyRuntimeMediaQuery.addEventListener("change", syncCompactStrategyRuntime);
        mobileStrategyRuntimeMediaQuery.addEventListener("change", syncMobileStrategyRuntime);
    } else {
        compactStrategyRuntimeMediaQuery.addListener(syncCompactStrategyRuntime);
        mobileStrategyRuntimeMediaQuery.addListener(syncMobileStrategyRuntime);
    }
}

function teardownStrategyRuntimeMediaQueries(): void {
    if (compactStrategyRuntimeMediaQuery !== null) {
        if (typeof compactStrategyRuntimeMediaQuery.removeEventListener === "function") {
            compactStrategyRuntimeMediaQuery.removeEventListener("change", syncCompactStrategyRuntime);
        } else {
            compactStrategyRuntimeMediaQuery.removeListener(syncCompactStrategyRuntime);
        }
    }
    if (mobileStrategyRuntimeMediaQuery !== null) {
        if (typeof mobileStrategyRuntimeMediaQuery.removeEventListener === "function") {
            mobileStrategyRuntimeMediaQuery.removeEventListener("change", syncMobileStrategyRuntime);
        } else {
            mobileStrategyRuntimeMediaQuery.removeListener(syncMobileStrategyRuntime);
        }
    }
    compactStrategyRuntimeMediaQuery = null;
    mobileStrategyRuntimeMediaQuery = null;
}

function selectStrategyRuntimeMobileSection(section: StrategyRuntimeMobileSection): void {
    if (section === "workbench" && selectedStrategy.value === null) {
        strategyRuntimeMobileSection.value = "instances";
        return;
    }
    strategyRuntimeMobileSection.value = section;
}

function isCurrentBrokerAccountSelectionKey(selectionKey: string | null | undefined): boolean {
    return selectionKey != null && selectionKey !== "" && selectionKey === effectiveCurrentBrokerAccountSelectionKey.value;
}

function formatTimestampParts(value: unknown): StrategyTimestampParts {
    const normalized = normalizeText(value);
    if (normalized === "") {
        return {
            display: "暂无",
            timestampMs: null,
        };
    }

    const parsed = new Date(normalized);
    if (Number.isNaN(parsed.getTime())) {
        const fallback = normalized.replace("T", " ").replace(".000Z", "Z");
        return {
            display: fallback,
            timestampMs: null,
        };
    }

    return {
        display: formatLocalDateTime(parsed, normalized),
        timestampMs: parsed.getTime(),
    };
}

function formatTimestamp(value: unknown): string {
    return formatTimestampParts(value).display;
}

function formatTimestampTooltip(value: unknown): string {
    return formatTimestampParts(value).display;
}

function formatStrategyStatus(status: StrategyInstanceItem["status"] | string): string {
    switch (status) {
        case "RUNNING":
            return "运行中";
        case "PAUSED":
            return "已暂停";
        case "STOPPED":
            return "已停止";
        default:
            return normalizeText(status) || "未知";
    }
}

function displayStrategyStatus(strategy: StrategyInstanceItem): StrategyInstanceItem["status"] {
    return strategy.runtimeObservation?.actualStatus ?? strategy.status;
}

function formatStrategyDefinitionSyncSummary(
    sync: StrategyDefinitionSyncStatus | null | undefined,
): string {
    if (sync == null) {
        return "";
    }
    if (sync.isLatest) {
        return `已同步至 v${sync.latestVersion}`;
    }
    return `待刷新 v${sync.appliedVersion} -> v${sync.latestVersion}`;
}

function formatStrategyExecutionMode(
    mode: StrategyInstanceBindingDocument["executionMode"] | string | null | undefined,
): string {
    return normalizeText(mode) === "notify_only" ? "仅通知" : "确认执行";
}

function isCurrentBrokerAccountBinding(
    brokerAccount: StrategyBrokerAccountBinding | null | undefined,
): boolean {
    return isCurrentBrokerAccountSelectionKey(
        resolveBrokerAccountSelectionKey(brokerAccountOptions.value, brokerAccount),
    );
}

function readCompiledIndicatorCount(strategy: StrategyInstanceItem): number | null {
    const compiledRequirements = asRecord(strategy.params.compiledRequirements);
    if (compiledRequirements === null) return null;
    return Array.isArray(compiledRequirements.indicators)
        ? compiledRequirements.indicators.length
        : null;
}

function readCompiledHookCount(strategy: StrategyInstanceItem): number | null {
    return Array.isArray(strategy.params.compiledHooks)
        ? strategy.params.compiledHooks.length
        : null;
}

function asRecord(value: unknown): Record<string, unknown> | null {
    if (value === null || typeof value !== "object" || Array.isArray(value)) {
        return null;
    }
    return value as Record<string, unknown>;
}

function formatActionLabel(action: StrategyAction): string {
    switch (action) {
        case "start":
            return "启动";
        case "pause":
            return "暂停";
        case "stop":
            return "停止";
        default:
            return action;
    }
}

function formatStrategyActionError(action: StrategyAction, error: unknown): string {
    if (
        action === "start"
        && error instanceof ApiClientError
        && error.code === "BAD_REQUEST"
        && error.message.includes("运行实例 PineTS Worker 已达到上限")
    ) {
        return "运行实例 PineTS Worker 已达到上限。请停止其他运行实例，或打开“设置 > PineTS Worker”调高“运行实例 Worker 最大值”后再启动。";
    }
    return error instanceof Error ? error.message : `执行${formatActionLabel(action)}失败。`;
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

function clearStrategyRuntimeRefreshTimer(): void {
    if (strategyRuntimeRefreshTimer != null) {
        window.clearTimeout(strategyRuntimeRefreshTimer);
        strategyRuntimeRefreshTimer = null;
    }
}

function resolveStrategyRuntimeRefreshMs(): number {
    const selectedStatus = selectedStrategy.value == null ? "" : displayStrategyStatus(selectedStrategy.value);
    if (activeStrategyCount.value > 0 || selectedStatus === "RUNNING" || selectedStatus === "PAUSED") {
        return STRATEGY_RUNTIME_ACTIVE_REFRESH_MS;
    }
    return STRATEGY_RUNTIME_IDLE_REFRESH_MS;
}

function shouldDeferStrategyRuntimeRefresh(): boolean {
    return isLoadingStrategies.value
        || isLoadingDetails.value
        || isCreatingStrategyInstance.value
        || isUpdatingStrategyBinding.value
        || isUpdatingStrategyRuntimeRisk.value
        || isDeletingStrategy.value
        || isRefreshingStrategyDefinition.value;
}

function scheduleStrategyRuntimeRefresh(): void {
    if (typeof window === "undefined") {
        return;
    }
    clearStrategyRuntimeRefreshTimer();
    if (typeof document !== "undefined" && document.visibilityState === "hidden") {
        return;
    }
    strategyRuntimeRefreshTimer = window.setTimeout(() => {
        void refreshStrategyRuntimeContent();
    }, resolveStrategyRuntimeRefreshMs());
}

function handleStrategyRuntimeVisibilityChange(): void {
    if (typeof document !== "undefined" && document.visibilityState === "hidden") {
        clearStrategyRuntimeRefreshTimer();
        return;
    }
    void refreshStrategyRuntimeContent();
}

function handleRuntimePaneResized(payload: SplitpanesResizedPayload): void {
    const sizes = payload.panes?.map((pane) => pane.size);
    if (
        sizes == null
        || sizes.length !== 2
        || !sizes.every((size) => Number.isFinite(size) && size > 0 && size <= 100)
    ) {
        return;
    }

    runtimePaneSizes.value = [sizes[0]!, sizes[1]!];
}

async function loadStrategyDefinitions(): Promise<void> {
    isLoadingDefinitions.value = true;
    definitionsError.value = "";

    try {
        strategyDefinitions.value = await queryClient.ensureQueryData({
            queryKey: queryKeys.strategyDefinitions(),
            queryFn: () => apiGet<StrategyDefinitionDocument[], "/api/v1/strategy-definitions">(
                "/api/v1/strategy-definitions",
            ),
        });
    } catch (error) {
        strategyDefinitions.value = [];
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
        const items = await fetchEnvelope<StrategyInstanceItem[]>("/api/v1/strategies");
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
        strategies.value = [];
        selectedStrategyId.value = "";
        clearRuntimeDetails();
        listError.value =
            error instanceof Error ? error.message : "加载策略实例失败。";
    } finally {
        isLoadingStrategies.value = false;
        scheduleStrategyRuntimeRefresh();
    }
}

async function loadStrategyDetails(instanceId: string): Promise<void> {
    selectedStrategyId.value = instanceId;
    detailsError.value = "";
    isLoadingDetails.value = true;

    const logsUrl = new URL(`/api/v1/strategies/${encodeURIComponent(instanceId)}/logs`, window.location.origin);
    logsUrl.searchParams.set("limit", "500");
    const auditUrl = new URL(`/api/v1/strategies/${encodeURIComponent(instanceId)}/audit`, window.location.origin);
    auditUrl.searchParams.set("limit", "500");

    try {
        const [logs, audit] = await Promise.all([
            fetchEnvelope<StrategyLogsResponse>(
                `${logsUrl.pathname}${logsUrl.search}`,
            ),
            fetchEnvelope<StrategyAuditResponse>(
                `${auditUrl.pathname}${auditUrl.search}`,
            ),
        ]);

        strategyLogs.value = logs.logs;
        strategyAuditEntries.value = audit.entries;
    } catch (error) {
        clearRuntimeDetails();
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
        const instance = await fetchEnvelopeWithInit<StrategyInstanceItem>(
            `/api/v1/strategy-definitions/${encodeURIComponent(createDefinitionId.value)}/instantiate`,
            {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                },
                body: JSON.stringify(buildStrategyBindingPayload({
                    brokerAccountOptions: brokerAccountOptions.value,
                    instruments: createBindingInstruments.value,
                    interval: createInterval.value,
                    executionMode: createExecutionMode.value,
                    runtimeRisk: createRuntimeRisk.value,
                    brokerAccountKey: createBrokerAccountKey.value,
                })),
            },
        );

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
        const updated = await fetchEnvelopeWithInit<StrategyInstanceItem>(
            `/api/v1/strategies/${encodeURIComponent(selectedStrategy.value.id)}`,
            {
                method: "PUT",
                headers: {
                    "Content-Type": "application/json",
                },
                body: JSON.stringify(buildStrategyBindingPayload({
                    brokerAccountOptions: brokerAccountOptions.value,
                    instruments: editBindingInstruments.value,
                    interval: editInterval.value,
                    executionMode: editExecutionMode.value,
                    runtimeRisk: editRuntimeRisk.value,
                    brokerAccountKey: editBrokerAccountKey.value,
                    fallbackBrokerAccount: selectedStrategyBinding.value?.brokerAccount ?? null,
                })),
            },
        );

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
        const updated = await fetchEnvelopeWithInit<StrategyInstanceItem>(
            `/api/v1/strategies/${encodeURIComponent(selectedStrategy.value.id)}/runtime-risk`,
            {
                method: "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(runtimeRisk),
            },
        );
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
    if (
        typeof window !== "undefined"
        && typeof window.confirm === "function"
        && !window.confirm(`确认删除策略实例「${selectedStrategy.value.definition.name}」吗？`)
    ) {
        return;
    }

    isDeletingStrategy.value = true;

    try {
        const removed = await fetchEnvelopeWithInit<StrategyInstanceItem>(
            `/api/v1/strategies/${encodeURIComponent(selectedStrategy.value.id)}`,
            {
                method: "DELETE",
            },
        );

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
        await fetchEnvelopeWithInit<StrategyInstanceItem>(
            `/api/v1/strategies/${encodeURIComponent(selectedStrategy.value.id)}/${action}`,
            {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({}),
            },
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
        const updated = await fetchEnvelopeWithInit<StrategyInstanceItem>(
            `/api/v1/strategies/${encodeURIComponent(selectedStrategy.value.id)}/refresh-definition`,
            {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({}),
            },
        );
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
