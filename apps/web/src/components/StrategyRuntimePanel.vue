<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import StrategyRuntimeActivityPanel from "./strategy-runtime/StrategyRuntimeActivityPanel.vue";
import StrategyRuntimeInstanceEditorDialog from "./strategy-runtime/StrategyRuntimeInstanceEditorDialog.vue";
import StrategyRuntimeInstanceListPanel from "./strategy-runtime/StrategyRuntimeInstanceListPanel.vue";
import StrategyRuntimeOverviewSection from "./strategy-runtime/StrategyRuntimeOverviewSection.vue";
import StrategyRuntimeSelectedStrategyPanel from "./strategy-runtime/StrategyRuntimeSelectedStrategyPanel.vue";
import {
    bindingInstrumentsToSymbols,
    brokerAccountOptionSubtitle,
    buildStrategyBindingPayload,
    defaultStrategyRuntimeRiskSettings,
    filterBrokerAccountOptions,
    formatBrokerAccountSummary,
    formatRuntimeObservationSymbols,
    formatStrategyRuntimeRiskSummary,
    formatStrategyInterval,
    formatStrategySymbols,
    normalizeStrategyRuntimeRiskSettings,
    normalizeText,
    normalizeBindingInstruments,
    readStrategyBinding,
    resolveBrokerAccountOption,
    resolveBrokerAccountSelectionKey,
    splitSymbolsText,
} from "./strategy-runtime/strategyRuntimeInstanceBinding";
import type {
    StrategyAuditEntryDocument,
    StrategyAuditListResponse,
    StrategyBindingInstrumentDocument,
    StrategyBrokerAccountBinding,
    StrategyDefinitionDocument,
    StrategyDefinitionSyncStatus,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
    StrategyRuntimeRiskSettings,
    StrategyLogListResponse,
    StrategyRuntimeObservation,
    StrategySourceFormat,
} from "@/contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import {
    type BrokerAccountSelectionOption,
} from "../composables/consoleDataBrokerAccountSelection";
import { useMarketProfiles } from "../composables/marketProfiles";
import { useConsoleData } from "../composables/useConsoleData";
import { formatLocalDateTime } from "../utils/dateTime";

type StrategyLogsResponse = StrategyLogListResponse;
type StrategyAuditEntry = StrategyAuditEntryDocument;
type StrategyAuditResponse = StrategyAuditListResponse;

const STRATEGY_RUNTIME_ACTIVE_REFRESH_MS = 1_000;
const STRATEGY_RUNTIME_IDLE_REFRESH_MS = 3_000;

type StrategyAction = "start" | "pause" | "stop";
type StrategySymbolEditorMode = "create" | "edit";

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
const instanceEditorMode = ref<StrategySymbolEditorMode | null>(null);

const createDefinitionId = ref("");
const createBindingInstruments = ref<StrategyBindingInstrumentDocument[]>([]);
const createSymbolDraftMarket = ref("HK");
const createSymbolDraft = ref("");
const createSymbolValidationMessage = ref("");
const createInterval = ref("5m");
const createExecutionMode = ref<StrategyExecutionMode>("live");
const createRuntimeRisk = ref<StrategyRuntimeRiskSettings>(defaultStrategyRuntimeRiskSettings());
const createBrokerAccountKey = ref("");
const createBrokerAccountQuery = ref("");

const editBindingInstruments = ref<StrategyBindingInstrumentDocument[]>([]);
const editSymbolDraftMarket = ref("HK");
const editSymbolDraft = ref("");
const editSymbolValidationMessage = ref("");
const editInterval = ref("5m");
const editExecutionMode = ref<StrategyExecutionMode>("live");
const editRuntimeRisk = ref<StrategyRuntimeRiskSettings>(defaultStrategyRuntimeRiskSettings());
const editBrokerAccountKey = ref("");
const editBrokerAccountQuery = ref("");
const activeBrokerAccountPicker = ref<StrategySymbolEditorMode | null>(null);
let strategyRuntimeRefreshTimer: number | null = null;
const pendingSymbolDraftCommits = new Map<StrategySymbolEditorMode, Promise<boolean>>();

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

const createDefinition = computed(
    () => strategyDefinitions.value.find((item) => item.id === createDefinitionId.value) ?? null,
);

const brokerAccountOptions = computed(() => availableBrokerAccounts.value);
const createSelectedBrokerAccountOption = computed(
    () => resolveBrokerAccountOption(brokerAccountOptions.value, createBrokerAccountKey.value),
);
const editSelectedBrokerAccountOption = computed(
    () => resolveBrokerAccountOption(brokerAccountOptions.value, editBrokerAccountKey.value),
);

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

const createFilteredBrokerAccountOptions = computed(() =>
    filterBrokerAccountOptions(brokerAccountOptions.value, createBrokerAccountQuery.value),
);
const editFilteredBrokerAccountOptions = computed(() =>
    filterBrokerAccountOptions(brokerAccountOptions.value, editBrokerAccountQuery.value),
);
const activeInstanceEditorMode = computed<StrategySymbolEditorMode>(() => instanceEditorMode.value ?? "create");
const instanceEditorOpen = computed({
    get: () => instanceEditorMode.value !== null,
    set: (value: boolean) => {
        if (!value) {
            closeInstanceEditorDialog();
        }
    },
});
const isCreateInstanceEditor = computed(() => instanceEditorMode.value === "create");
const isEditInstanceEditor = computed(() => instanceEditorMode.value === "edit");
const activeSymbolTags = computed(() => symbolTagsFor(activeInstanceEditorMode.value));
const activeSymbolDraftMarket = computed(() => symbolDraftMarketFor(activeInstanceEditorMode.value));
const activeSymbolDraft = computed(() => symbolDraftFor(activeInstanceEditorMode.value));
const activeSymbolValidationMessage = computed(() => symbolValidationMessageFor(activeInstanceEditorMode.value));
const activeIntervalValue = computed(() => intervalValueFor(activeInstanceEditorMode.value));
const activeExecutionMode = computed(() => executionModeFor(activeInstanceEditorMode.value));
const activeRuntimeRisk = computed(() => runtimeRiskFor(activeInstanceEditorMode.value));
const activeSelectedBrokerAccountOption = computed(() => selectedBrokerAccountOptionFor(activeInstanceEditorMode.value));
const activeSelectedBrokerAccountKey = computed(() => selectedBrokerAccountKeyFor(activeInstanceEditorMode.value));
const activeBrokerAccountQuery = computed(() => brokerAccountQueryFor(activeInstanceEditorMode.value));
const activeIsBrokerAccountPickerOpen = computed(() => isBrokerAccountPickerOpen(activeInstanceEditorMode.value));
const activeFilteredBrokerAccountOptions = computed(() => filteredBrokerAccountOptionsFor(activeInstanceEditorMode.value));
const activeInstanceEditorSymbolsSummary = computed(() => instanceEditorSymbolsSummary(activeInstanceEditorMode.value));
const activeInstanceEditorBrokerAccountSummary = computed(() => instanceEditorBrokerAccountSummary(activeInstanceEditorMode.value));
const instanceEditorPreviewDefinitionLabel = computed(() => {
    if (isCreateInstanceEditor.value) {
        return createDefinition.value == null
            ? "未选择策略定义"
            : `${createDefinition.value.name} / v${createDefinition.value.version}`;
    }
    return `${selectedStrategy.value?.definition.name ?? "未选择"} / v${selectedStrategy.value?.definition.version ?? ""}`;
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

const createInstanceHint = computed(() => {
    if (strategyDefinitions.value.length === 0) {
        return "先回到设计区保存策略定义，再在这里绑定交易代码、周期和账号。";
    }
    return "实例负责绑定交易代码、周期、账号与执行模式；同一个策略定义可以生成多个实例。";
});

const instanceEditorTitle = computed(() =>
    instanceEditorMode.value === "edit" ? "实例绑定" : "新增实例",
);

const instanceEditorHint = computed(() => {
    if (instanceEditorMode.value === "edit") {
        if (selectedStrategy.value === null) {
            return "请选择策略实例后再调整绑定。";
        }
        return "当前选中的实例可以独立绑定多个交易代码、账号和执行模式。";
    }
    return createInstanceHint.value;
});

const selectedStrategyStartHint = computed(() => {
    if (selectedStrategy.value === null) return "请选择策略实例。";
    if (selectedStrategyBinding.value?.executionMode === "notify_only") {
        return "当前实例为仅通知模式：触发信号只发送准备下单通知，不自动下单。";
    }
    if (selectedStrategy.value.startable) {
        return "当前实例已接入策略控制面生命周期，可启动、暂停、停止。";
    }
    if (selectedStrategy.value.runtime === "pine-go-plan") {
        return "当前实例已完成 Pine 编译与 requirements 规划，但暂不可启动。";
    }
    return "当前实例暂不可启动。";
});

const selectedStrategyCompiledSummary = computed(() => {
    if (selectedStrategy.value === null || selectedStrategy.value.runtime !== "pine-go-plan") {
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

watch(
    () => props.pendingDefinitionId,
    (definitionId) => {
        const normalizedDefinitionId = normalizeText(definitionId);
        if (normalizedDefinitionId !== "") {
            createDefinitionId.value = normalizedDefinitionId;
            instanceEditorMode.value = "create";
            isCreateMenuOpen.value = false;
        }
    },
    { immediate: true },
);

watch(
    selectedStrategy,
    (strategy) => {
        if (strategy === null && instanceEditorMode.value === "edit") {
            closeInstanceEditorDialog();
        }
    },
);

watch(
    strategyDefinitions,
    (definitions) => {
        if (definitions.length === 0) {
            createDefinitionId.value = "";
            return;
        }

        if (definitions.some((item) => item.id === createDefinitionId.value)) {
            return;
        }

        const pendingDefinition = definitions.find(
            (item) => item.id === normalizeText(props.pendingDefinitionId),
        );
        createDefinitionId.value = pendingDefinition?.id ?? definitions[0]?.id ?? "";
    },
    { immediate: true },
);

watch(
    [brokerAccountOptions, defaultBrokerAccountSelectionKey],
    () => {
        const defaultSelectionKey = defaultBrokerAccountSelectionKey.value;
        if (
            (createBrokerAccountKey.value === ""
                || !brokerAccountOptions.value.some(
                    (option) => option.selectionKey === createBrokerAccountKey.value,
                ))
            && defaultSelectionKey !== ""
        ) {
            createBrokerAccountKey.value = defaultSelectionKey;
        }

        if (
            selectedStrategyBinding.value?.brokerAccount == null
            && (
                editBrokerAccountKey.value === ""
                || !brokerAccountOptions.value.some(
                    (option) => option.selectionKey === editBrokerAccountKey.value,
                )
            )
            && defaultSelectionKey !== ""
        ) {
            editBrokerAccountKey.value = defaultSelectionKey;
        }
    },
    { immediate: true },
);

watch(
    [selectedBrokerAccount, createSelectedBrokerAccountOption],
    () => {
        if (normalizeText(createSymbolDraftMarket.value) !== "") {
            return;
        }
        createSymbolDraftMarket.value = defaultSymbolDraftMarket("create");
    },
    { immediate: true },
);

watch(
    selectedStrategyBinding,
    (binding) => {
        if (binding === null) {
            editBindingInstruments.value = [];
            editSymbolDraftMarket.value = defaultSymbolDraftMarket("edit");
            editSymbolDraft.value = "";
            editSymbolValidationMessage.value = "";
            editInterval.value = "5m";
            editExecutionMode.value = "live";
            editRuntimeRisk.value = defaultStrategyRuntimeRiskSettings();
            editBrokerAccountKey.value = defaultBrokerAccountSelectionKey.value;
            return;
        }

        editBindingInstruments.value = normalizeBindingInstruments(binding.instruments ?? []);
        editSymbolDraftMarket.value = binding.instruments?.[0]?.market ?? defaultSymbolDraftMarket("edit");
        editSymbolDraft.value = "";
        editSymbolValidationMessage.value = "";
        editInterval.value = binding.interval;
        editExecutionMode.value = binding.executionMode;
        editRuntimeRisk.value = normalizeStrategyRuntimeRiskSettings(binding.runtimeRisk);
        editBrokerAccountKey.value =
            resolveBrokerAccountSelectionKey(brokerAccountOptions.value, binding.brokerAccount)
            || defaultBrokerAccountSelectionKey.value;
    },
    { immediate: true },
);

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

function formatStrategyRuntime(runtime: unknown): string {
    switch (normalizeText(runtime)) {
        case "pine-go-plan":
            return "Pine v6 主路径";
        default:
            return "未知 / 受限";
    }
}

function formatSourceFormat(sourceFormat: StrategySourceFormat | string | null | undefined): string {
    switch (normalizeText(sourceFormat)) {
        case "pine-v6":
            return "Pine v6";
        default:
            return "未知 / 受限";
    }
}

function formatStrategyEligibility(strategy: StrategyInstanceItem): string {
    if (strategy.startable) return "可启动";
    if (strategy.runtime === "pine-go-plan") return "待启用";
    return "受限";
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

function formatStrategyExecutionMode(mode: StrategyExecutionMode | string | null | undefined): string {
    return normalizeText(mode) === "notify_only" ? "仅通知" : "确认执行";
}

function bindingInstrumentsFor(mode: StrategySymbolEditorMode): StrategyBindingInstrumentDocument[] {
    return mode === "create" ? createBindingInstruments.value : editBindingInstruments.value;
}

function symbolTagsFor(mode: StrategySymbolEditorMode): string[] {
    return bindingInstrumentsToSymbols(bindingInstrumentsFor(mode));
}

function defaultSymbolDraftMarket(mode: StrategySymbolEditorMode): string {
    if (mode === "edit") {
        const bindingMarket = selectedStrategyBinding.value?.instruments?.[0]?.market;
        if (normalizeText(bindingMarket) !== "") {
            return normalizeText(bindingMarket).toUpperCase();
        }
    }
    const optionMarket = selectedBrokerAccountOptionFor(mode)?.market;
    if (normalizeText(optionMarket) !== "") {
        return normalizeText(optionMarket).toUpperCase();
    }
    const selectedMarket = selectedBrokerAccount.value?.market;
    if (normalizeText(selectedMarket) !== "") {
        return normalizeText(selectedMarket).toUpperCase();
    }
    return "HK";
}

function symbolDraftMarketFor(mode: StrategySymbolEditorMode): string {
    return mode === "create" ? createSymbolDraftMarket.value : editSymbolDraftMarket.value;
}

function setSymbolDraftMarket(mode: StrategySymbolEditorMode, value: string): void {
    const normalized = normalizeText(value).toUpperCase();
    if (mode === "create") {
        createSymbolDraftMarket.value = normalized || defaultSymbolDraftMarket(mode);
        return;
    }
    editSymbolDraftMarket.value = normalized || defaultSymbolDraftMarket(mode);
}

function symbolDraftFor(mode: StrategySymbolEditorMode): string {
    return mode === "create" ? createSymbolDraft.value : editSymbolDraft.value;
}

function setSymbolDraft(mode: StrategySymbolEditorMode, value: string): void {
    setSymbolValidationMessage(mode, "");
    if (mode === "create") {
        createSymbolDraft.value = value;
        return;
    }
    editSymbolDraft.value = value;
}

function symbolValidationMessageFor(mode: StrategySymbolEditorMode): string {
    return mode === "create" ? createSymbolValidationMessage.value : editSymbolValidationMessage.value;
}

function setSymbolValidationMessage(mode: StrategySymbolEditorMode, value: string): void {
    if (mode === "create") {
        createSymbolValidationMessage.value = value;
        return;
    }
    editSymbolValidationMessage.value = value;
}

function setBindingInstruments(mode: StrategySymbolEditorMode, values: StrategyBindingInstrumentDocument[]): void {
    const nextValue = normalizeBindingInstruments(values);
    if (mode === "create") {
        createBindingInstruments.value = nextValue;
        createSymbolDraft.value = "";
        return;
    }
    editBindingInstruments.value = nextValue;
    editSymbolDraft.value = "";
}

function bindingInstrumentId(value: StrategyBindingInstrumentDocument): string {
    return bindingInstrumentsToSymbols([value])[0] ?? "";
}

async function commitSymbolDraft(mode: StrategySymbolEditorMode, draft = symbolDraftFor(mode)): Promise<boolean> {
    const pending = pendingSymbolDraftCommits.get(mode);
    if (pending != null) {
        return pending;
    }
    const commit = (async () => {
        const draftSegments = splitSymbolsText(draft);
        const parsed: StrategyBindingInstrumentDocument[] = [];
        const invalidSymbols: string[] = [];
        const fallbackMarket = symbolDraftMarketFor(mode).trim().toUpperCase();
        for (const segment of draftSegments) {
            const raw = normalizeText(segment);
            if (raw === "") {
                continue;
            }
            const request =
                raw.includes(".") || raw.includes(":")
                    ? { instrumentId: raw }
                    : { market: fallbackMarket, code: raw };
            try {
                const normalized = await normalizeInstrumentRefWithMarketApi(request);
                const market = normalized.prefix.trim().toUpperCase();
                const code = normalized.code.trim().toUpperCase();
                if (market === "" || code === "") {
                    invalidSymbols.push(raw.toUpperCase());
                    continue;
                }
                parsed.push({ market, code });
            } catch {
                invalidSymbols.push(raw.toUpperCase());
            }
        }
        if (parsed.length === 0) {
            setSymbolDraft(mode, "");
        } else {
            setBindingInstruments(mode, [...bindingInstrumentsFor(mode), ...parsed]);
            const last = parsed[parsed.length - 1];
            if (last != null) {
                setSymbolDraftMarket(mode, last.market);
            }
        }
        if (invalidSymbols.length > 0) {
            setSymbolValidationMessage(
                mode,
                `已忽略无效交易代码：${invalidSymbols.join("、")}。请选择市场后输入代码，或直接使用 US.TME、HK.00700 这类完整格式。`,
            );
            return false;
        }
        setSymbolValidationMessage(mode, "");
        return true;
    })();
    pendingSymbolDraftCommits.set(mode, commit);
    try {
        return await commit;
    } finally {
        pendingSymbolDraftCommits.delete(mode);
    }
}

function removeSymbolTag(mode: StrategySymbolEditorMode, symbol: string): void {
    setBindingInstruments(
        mode,
        bindingInstrumentsFor(mode).filter((item) => bindingInstrumentId(item) !== symbol),
    );
}

function handleSymbolDraftKeydown(event: KeyboardEvent, mode: StrategySymbolEditorMode): void {
    if (event.isComposing) {
        return;
    }
    if (event.key === "Enter" || event.key === "," || event.key === "Tab") {
        event.preventDefault();
        void commitSymbolDraft(mode);
        return;
    }
    if (event.key === "Backspace" && normalizeText(symbolDraftFor(mode)) === "") {
        const instruments = bindingInstrumentsFor(mode);
        if (instruments.length === 0) {
            return;
        }
        event.preventDefault();
        setBindingInstruments(mode, instruments.slice(0, -1));
    }
}

function handleSymbolDraftPaste(event: ClipboardEvent, mode: StrategySymbolEditorMode): void {
    const pastedText = event.clipboardData?.getData("text") ?? "";
    if (splitSymbolsText(pastedText).length <= 1) {
        return;
    }
    event.preventDefault();
    void commitSymbolDraft(mode, pastedText);
}

function brokerAccountQueryFor(mode: StrategySymbolEditorMode): string {
    return mode === "create" ? createBrokerAccountQuery.value : editBrokerAccountQuery.value;
}

function setBrokerAccountQuery(mode: StrategySymbolEditorMode, value: string): void {
    if (mode === "create") {
        createBrokerAccountQuery.value = value;
        return;
    }
    editBrokerAccountQuery.value = value;
}

function intervalValueFor(mode: StrategySymbolEditorMode): string {
    return mode === "create" ? createInterval.value : editInterval.value;
}

function setIntervalValue(mode: StrategySymbolEditorMode, value: string): void {
    if (mode === "create") {
        createInterval.value = value;
        return;
    }
    editInterval.value = value;
}

function executionModeFor(mode: StrategySymbolEditorMode): StrategyExecutionMode {
    return mode === "create" ? createExecutionMode.value : editExecutionMode.value;
}

function runtimeRiskFor(mode: StrategySymbolEditorMode): StrategyRuntimeRiskSettings {
    return mode === "create" ? createRuntimeRisk.value : editRuntimeRisk.value;
}

function setExecutionMode(mode: StrategySymbolEditorMode, value: string): void {
    const normalized = value === "notify_only" ? "notify_only" : "live";
    if (mode === "create") {
        createExecutionMode.value = normalized;
        return;
    }
    editExecutionMode.value = normalized;
}

function setRuntimeRisk(mode: StrategySymbolEditorMode, patch: Partial<StrategyRuntimeRiskSettings>): void {
    const current = runtimeRiskFor(mode);
    const next = normalizeStrategyRuntimeRiskSettings({ ...current, ...patch });
    if (mode === "create") {
        createRuntimeRisk.value = next;
        return;
    }
    editRuntimeRisk.value = next;
}

function selectedBrokerAccountOptionFor(mode: StrategySymbolEditorMode): BrokerAccountSelectionOption | null {
    return mode === "create" ? createSelectedBrokerAccountOption.value : editSelectedBrokerAccountOption.value;
}

function selectedBrokerAccountKeyFor(mode: StrategySymbolEditorMode): string {
    return mode === "create" ? createBrokerAccountKey.value : editBrokerAccountKey.value;
}

function setSelectedBrokerAccountKey(mode: StrategySymbolEditorMode, value: string): void {
    if (mode === "create") {
        createBrokerAccountKey.value = value;
        return;
    }
    editBrokerAccountKey.value = value;
}

function isBrokerAccountPickerOpen(mode: StrategySymbolEditorMode): boolean {
    return activeBrokerAccountPicker.value === mode;
}

function toggleBrokerAccountPicker(mode: StrategySymbolEditorMode): void {
    if (activeBrokerAccountPicker.value === mode) {
        activeBrokerAccountPicker.value = null;
        setBrokerAccountQuery(mode, "");
        return;
    }
    activeBrokerAccountPicker.value = mode;
    setBrokerAccountQuery(mode, "");
}

function closeBrokerAccountPicker(mode?: StrategySymbolEditorMode): void {
    if (mode == null || activeBrokerAccountPicker.value === mode) {
        activeBrokerAccountPicker.value = null;
    }
    if (mode == null || mode === "create") {
        createBrokerAccountQuery.value = "";
    }
    if (mode == null || mode === "edit") {
        editBrokerAccountQuery.value = "";
    }
}

function filteredBrokerAccountOptionsFor(mode: StrategySymbolEditorMode): BrokerAccountSelectionOption[] {
    return mode === "create"
        ? createFilteredBrokerAccountOptions.value
        : editFilteredBrokerAccountOptions.value;
}

function selectBrokerAccountOption(mode: StrategySymbolEditorMode, selectionKey: string): void {
    setSelectedBrokerAccountKey(mode, selectionKey);
    closeBrokerAccountPicker(mode);
}

function clearBrokerAccountSelection(mode: StrategySymbolEditorMode): void {
    setSelectedBrokerAccountKey(mode, "");
    closeBrokerAccountPicker(mode);
}

function instanceEditorSymbolsSummary(mode: StrategySymbolEditorMode): string {
    const symbols = symbolTagsFor(mode);
    return symbols.length > 0 ? symbols.join(", ") : "暂未绑定交易代码";
}

function instanceEditorBrokerAccountSummary(mode: StrategySymbolEditorMode): string {
    const option = selectedBrokerAccountOptionFor(mode);
    return option == null ? "暂不绑定账号" : brokerAccountOptionSubtitle(option);
}

function removeActiveSymbol(symbol: string): void {
    removeSymbolTag(activeInstanceEditorMode.value, symbol);
}

function updateActiveSymbolDraft(value: string): void {
    setSymbolDraft(activeInstanceEditorMode.value, value);
}

function updateActiveSymbolDraftMarket(value: string): void {
    setSymbolDraftMarket(activeInstanceEditorMode.value, value);
}

function commitActiveSymbolDraft(): void {
    void commitSymbolDraft(activeInstanceEditorMode.value);
}

function handleActiveSymbolDraftKeydown(event: KeyboardEvent): void {
    handleSymbolDraftKeydown(event, activeInstanceEditorMode.value);
}

function handleActiveSymbolDraftPaste(event: ClipboardEvent): void {
    handleSymbolDraftPaste(event, activeInstanceEditorMode.value);
}

function updateActiveIntervalValue(value: string): void {
    setIntervalValue(activeInstanceEditorMode.value, value);
}

function updateActiveExecutionMode(value: string): void {
    setExecutionMode(activeInstanceEditorMode.value, value);
}

function updateActiveRuntimeRiskMode(value: string): void {
    const mode = value === "monitor" || value === "enforce" ? value : "off";
    setRuntimeRisk(activeInstanceEditorMode.value, { mode });
}

function updateActiveRuntimeRiskCloseOnly(value: boolean): void {
    setRuntimeRisk(activeInstanceEditorMode.value, { closeOnly: value });
}

function updateActiveRuntimeRiskPauseOnReject(value: boolean): void {
    setRuntimeRisk(activeInstanceEditorMode.value, { pauseOnReject: value });
}

function updateActiveRuntimeRiskNumber(field: "maxOrderQuantity" | "maxOrderNotional" | "dailyMaxOrders", value: string): void {
    const trimmed = normalizeText(value);
    const numeric = trimmed === "" ? null : Number(trimmed);
    setRuntimeRisk(activeInstanceEditorMode.value, {
        [field]: Number.isFinite(numeric) && numeric !== null && numeric > 0 ? numeric : null,
    });
}

function toggleActiveBrokerAccountPicker(): void {
    toggleBrokerAccountPicker(activeInstanceEditorMode.value);
}

function updateActiveBrokerAccountQuery(value: string): void {
    setBrokerAccountQuery(activeInstanceEditorMode.value, value);
}

function clearActiveBrokerAccountSelection(): void {
    clearBrokerAccountSelection(activeInstanceEditorMode.value);
}

function selectActiveBrokerAccount(selectionKey: string): void {
    selectBrokerAccountOption(activeInstanceEditorMode.value, selectionKey);
}

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
    instanceEditorMode.value = "create";
    createSymbolDraftMarket.value = defaultSymbolDraftMarket("create");
    createSymbolDraft.value = "";
    createSymbolValidationMessage.value = "";
    createRuntimeRisk.value = defaultStrategyRuntimeRiskSettings();
    closeBrokerAccountPicker();
}

function openEditInstanceForm(): void {
    if (selectedStrategy.value === null) {
        return;
    }
    isCreateMenuOpen.value = false;
    instanceEditorMode.value = "edit";
    closeBrokerAccountPicker();
}

function closeInstanceEditorDialog(): void {
    const mode = instanceEditorMode.value;
    instanceEditorMode.value = null;
    isCreateMenuOpen.value = false;
    if (mode === "create") {
        createSymbolValidationMessage.value = "";
    }
    if (mode === "edit") {
        editSymbolValidationMessage.value = "";
    }
    closeBrokerAccountPicker();
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
        detailsError.value =
            error instanceof Error ? error.message : `执行${formatActionLabel(action)}失败。`;
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

<style scoped>
.runtime-panel {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    overflow: hidden;
}

.runtime-panel__bar {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    flex-shrink: 0;
}

.runtime-panel__eyebrow {
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.22em;
    text-transform: uppercase;
    color: var(--tv-text-muted);
}

.runtime-panel__title-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.2rem;
}

.runtime-panel__title {
    font-size: 1.35rem;
    line-height: 1.2;
    font-weight: 700;
    color: var(--tv-text);
}

.runtime-panel__bar-actions {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.75rem;
}

.runtime-panel__metrics {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.5rem;
}

.runtime-panel__metric-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    min-height: 2.2rem;
    padding: 0.45rem 0.85rem;
    border-radius: 999px;
    border: 1px solid var(--tv-border);
    background: color-mix(in srgb, var(--tv-bg-surface) 84%, transparent);
    color: var(--tv-text-muted);
    font-size: 0.78rem;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
}

.runtime-panel__scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    overscroll-behavior: contain;
    display: grid;
    gap: 1rem;
    align-content: start;
    padding-bottom: 1rem;
}

</style>
