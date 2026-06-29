<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import StrategyRuntimeActivityPanel from "./strategy-runtime/StrategyRuntimeActivityPanel.vue";
import StrategyRuntimeInstanceEditorDialog from "./strategy-runtime/StrategyRuntimeInstanceEditorDialog.vue";
import StrategyRuntimeInstanceListPanel from "./strategy-runtime/StrategyRuntimeInstanceListPanel.vue";
import StrategyRuntimeOverviewSection from "./strategy-runtime/StrategyRuntimeOverviewSection.vue";
import StrategyRuntimeSelectedStrategyPanel from "./strategy-runtime/StrategyRuntimeSelectedStrategyPanel.vue";
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

import { ApiClientError, fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import { useMarketProfiles } from "../composables/marketProfiles";
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

type StrategyAction = "start" | "pause" | "stop";

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
    marketOptions: strategyInstrumentMarketOptions,
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
let strategyRuntimeRefreshTimer: number | null = null;

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

const selectedStrategyRuntimeObservation = computed<StrategyRuntimeObservation | null>(
    () => selectedStrategy.value?.runtimeObservation ?? null,
);

const brokerAccountOptions = computed(() => availableBrokerAccounts.value);

const activeStrategyCount = computed(
    () => strategies.value.filter((item) => item.runtimeObservation?.actualStatus === "RUNNING").length,
);

const isRefreshingStrategyContent = computed(
    () => isLoadingStrategies.value || isLoadingDetails.value,
);

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
    activeSymbolDraftMarket,
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
    commitSymbolDraft,
    removeActiveSymbol,
    updateActiveSymbolDraft,
    updateActiveSymbolDraftMarket,
    commitActiveSymbolDraft,
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

onMounted(() => {
    if (typeof document !== "undefined") {
        document.addEventListener("visibilitychange", handleStrategyRuntimeVisibilityChange);
    }
    void Promise.all([loadMarketProfiles(), loadStrategyDefinitions(), loadStrategies()]);
});

onUnmounted(() => {
    clearStrategyRuntimeRefreshTimer();
    if (typeof document !== "undefined") {
        document.removeEventListener("visibilitychange", handleStrategyRuntimeVisibilityChange);
    }
});

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

function strategyStatusBadgeClass(strategy: StrategyInstanceItem): string {
    switch (displayStrategyStatus(strategy)) {
        case "RUNNING":
            return "strategy-status-badge strategy-status-badge--running";
        case "PAUSED":
            return "strategy-status-badge strategy-status-badge--paused";
        default:
            return "strategy-status-badge strategy-status-badge--stopped";
    }
}

function strategyStatusCardClass(strategy: StrategyInstanceItem): string {
    switch (displayStrategyStatus(strategy)) {
        case "RUNNING":
            return "strategy-list-card--running";
        case "PAUSED":
            return "strategy-list-card--paused";
        default:
            return "strategy-list-card--stopped";
    }
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

async function loadStrategyDefinitions(): Promise<void> {
    isLoadingDefinitions.value = true;
    definitionsError.value = "";

    try {
        strategyDefinitions.value = await fetchEnvelope<StrategyDefinitionDocument[]>("/api/v1/strategy-definitions");
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
}

async function createStrategyInstance(): Promise<void> {
    clearInstanceMutationMessages();
    if (createSymbolValidationMessage.value !== "") {
        instanceMutationError.value = createSymbolValidationMessage.value;
        return;
    }
    if (!await commitSymbolDraft("create")) {
        instanceMutationError.value = createSymbolValidationMessage.value;
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
    if (!await commitSymbolDraft("edit")) {
        instanceMutationError.value = editSymbolValidationMessage.value;
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
        <!-- 头部 -->
        <div class="runtime-panel__bar">
            <div class="runtime-panel__intro">
                <div class="runtime-panel__eyebrow">策略运行时</div>
                <div class="runtime-panel__title-row">
                    <div class="runtime-panel__title">策略运行</div>
                </div>
            </div>

            <div class="runtime-panel__bar-actions">
                <div class="runtime-panel__metrics">
                    <div class="runtime-panel__metric-chip">{{ activeStrategyCount }} 个活跃实例</div>
                    <div class="runtime-panel__metric-chip">
                        {{ props.definitionsCount ?? 0 }} 个策略定义
                    </div>
                </div>
            </div>
        </div>

        <!-- 内容区 -->
        <div class="runtime-panel__scroll">
            <div v-if="instanceMutationNotice"
                class="rounded-3xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">
                {{ instanceMutationNotice }}
            </div>
            <div v-if="instanceMutationError"
                class="rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
                {{ instanceMutationError }}
            </div>

            <StrategyRuntimeOverviewSection
                :active-strategy-count="activeStrategyCount"
                :selected-strategy="selectedStrategy"
                :selected-strategy-runtime-label="selectedStrategyRuntimeLabel"
                :system-status="systemStatus"
                :format-strategy-runtime-risk-summary="formatStrategyRuntimeRiskSummary"
            />

            <div class="grid gap-4" :class="selectedStrategy === null ? 'grid-cols-1' : 'xl:grid-cols-[minmax(22rem,26rem)_minmax(0,1fr)]'">
                <StrategyRuntimeInstanceListPanel
                    :is-create-menu-open="isCreateMenuOpen"
                    :is-loading-strategies="isLoadingStrategies"
                    :list-error="listError"
                    :strategies="strategies"
                    :selected-strategy-id="selectedStrategyId"
                    :display-strategy-status="displayStrategyStatus"
                    :strategy-status-badge-class="strategyStatusBadgeClass"
                    :strategy-status-card-class="strategyStatusCardClass"
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
                        :mode="activeInstanceEditorMode"
                        :title="instanceEditorTitle"
                        :hint="instanceEditorHint"
                        :is-loading-definitions="isLoadingDefinitions"
                        :definitions-error="definitionsError"
                        :strategy-definitions="strategyDefinitions"
                        :create-definition-id="createDefinitionId"
                        :create-definition="createDefinition"
                        :selected-strategy="selectedStrategy"
                        :symbol-tags="activeSymbolTags"
                        :symbol-market="activeSymbolDraftMarket"
                        :symbol-draft="activeSymbolDraft"
                        :symbol-validation-message="activeSymbolValidationMessage"
                        :market-options="strategyInstrumentMarketOptions"
                        :interval-value="activeIntervalValue"
                        :execution-mode="activeExecutionMode"
                        :runtime-risk="activeRuntimeRisk"
                        :selected-broker-account-option="activeSelectedBrokerAccountOption"
                        :selected-broker-account-key="activeSelectedBrokerAccountKey"
                        :current-broker-account-selection-key="effectiveCurrentBrokerAccountSelectionKey"
                        :is-broker-account-picker-open="activeIsBrokerAccountPickerOpen"
                        :broker-account-query="activeBrokerAccountQuery"
                        :filtered-broker-account-options="activeFilteredBrokerAccountOptions"
                        :preview-definition-label="instanceEditorPreviewDefinitionLabel"
                        :symbols-summary="activeInstanceEditorSymbolsSummary"
                        :broker-account-summary="activeInstanceEditorBrokerAccountSummary"
                        :can-create-strategy-instance="canCreateStrategyInstance"
                        :can-update-selected-strategy-binding="canUpdateSelectedStrategyBinding"
                        :can-delete-selected-strategy="canDeleteSelectedStrategy"
                        :is-creating-strategy-instance="isCreatingStrategyInstance"
                        :is-updating-strategy-binding="isUpdatingStrategyBinding"
                        :is-deleting-strategy="isDeletingStrategy"
                        @refresh-definitions="void loadStrategyDefinitions()"
                        @switch-to-design="openCreateDefinition"
                        @update:create-definition-id="createDefinitionId = $event"
                        @remove-symbol="removeActiveSymbol"
                        @update:symbol-market="updateActiveSymbolDraftMarket"
                        @update:symbol-draft="updateActiveSymbolDraft"
                        @commit-symbol-draft="commitActiveSymbolDraft"
                        @symbol-draft-keydown="handleActiveSymbolDraftKeydown"
                        @symbol-draft-paste="handleActiveSymbolDraftPaste"
                        @update:interval="updateActiveIntervalValue"
                        @update:execution-mode="updateActiveExecutionMode"
                        @update:runtime-risk-mode="updateActiveRuntimeRiskMode"
                        @update:runtime-risk-close-only="updateActiveRuntimeRiskCloseOnly"
                        @update:runtime-risk-pause-on-reject="updateActiveRuntimeRiskPauseOnReject"
                        @update:runtime-risk-number="updateActiveRuntimeRiskNumber"
                        @toggle-broker-picker="toggleActiveBrokerAccountPicker"
                        @update:broker-query="updateActiveBrokerAccountQuery"
                        @clear-broker-selection="clearActiveBrokerAccountSelection"
                        @select-broker-selection="selectActiveBrokerAccount"
                        @submit-create="createStrategyInstance"
                        @submit-update="updateSelectedStrategyBinding"
                        @submit-delete="deleteSelectedStrategy"
                    />
                </StrategyRuntimeInstanceListPanel>

                <div v-if="selectedStrategy !== null" class="min-w-0 grid gap-4">
                    <StrategyRuntimeSelectedStrategyPanel
                        :selected-strategy="selectedStrategy"
                        :selected-strategy-binding="selectedStrategyBinding"
                        :selected-strategy-definition-sync="selectedStrategyDefinitionSync"
                        :selected-strategy-runtime-observation="selectedStrategyRuntimeObservation"
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

                    <StrategyRuntimeActivityPanel
                        :key="selectedStrategy?.id ?? 'strategy-runtime-activity-empty'"
                        :is-loading-details="isLoadingDetails"
                        :strategy-logs="strategyLogs"
                        :strategy-audit-entries="strategyAuditEntries"
                        :selected-strategy-params-json="selectedStrategyParamsJson"
                    />
                </div>
            </div>
        </div>
    </div>
</template>
