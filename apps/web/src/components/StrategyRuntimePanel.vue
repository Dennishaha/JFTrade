<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import type {
    StrategyBrokerAccountBinding,
    StrategyDefinitionDocument,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
    StrategyRuntimeObservation,
    StrategySourceFormat,
} from "@jftrade/ui-contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import {
    buildBrokerAccountSelectionKey,
    type BrokerAccountSelectionOption,
} from "../composables/consoleDataBrokerAccountSelection";
import { useConsoleData } from "../composables/useConsoleData";

interface StrategyLogsResponse {
    instanceId: string;
    logs: string[];
}

interface StrategyAuditEntry {
    instanceId: string;
    kind: string;
    detail?: string;
    at: string;
}

interface StrategyAuditResponse {
    instanceId: string;
    entries: StrategyAuditEntry[];
}

type StrategyAction = "start" | "pause" | "stop";
type StrategySymbolEditorMode = "create" | "edit";
type StrategyActivityTab = "logs" | "audit";
type StrategyActivityLevel = "all" | "error" | "warning" | "info";

interface StrategyLogViewEntry {
    raw: string;
    message: string;
    at: string;
    level: Exclude<StrategyActivityLevel, "all">;
}

interface StrategyAuditViewEntry extends StrategyAuditEntry {
    detailText: string;
    label: string;
    level: Exclude<StrategyActivityLevel, "all">;
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
const isDeletingStrategy = ref(false);
const definitionsError = ref("");
const listError = ref("");
const detailsError = ref("");
const instanceMutationNotice = ref("");
const instanceMutationError = ref("");
const isCreateMenuOpen = ref(false);
const instanceEditorMode = ref<StrategySymbolEditorMode | null>(null);
const strategyActivityTab = ref<StrategyActivityTab>("logs");
const strategyActivityLevelFilter = ref<StrategyActivityLevel>("all");
const strategyParamsDialogOpen = ref(false);

const createDefinitionId = ref("");
const createSymbolsText = ref("");
const createSymbolDraft = ref("");
const createSymbolValidationMessage = ref("");
const createInterval = ref("5m");
const createExecutionMode = ref<StrategyExecutionMode>("live");
const createBrokerAccountKey = ref("");
const createBrokerAccountQuery = ref("");

const editSymbolsText = ref("");
const editSymbolDraft = ref("");
const editSymbolValidationMessage = ref("");
const editInterval = ref("5m");
const editExecutionMode = ref<StrategyExecutionMode>("live");
const editBrokerAccountKey = ref("");
const editBrokerAccountQuery = ref("");
const activeBrokerAccountPicker = ref<StrategySymbolEditorMode | null>(null);

const selectedStrategy = computed(
    () => strategies.value.find((item) => item.id === selectedStrategyId.value) ?? null,
);

const selectedStrategyBinding = computed<StrategyInstanceBindingDocument | null>(() => {
    if (selectedStrategy.value === null) {
        return null;
    }
    return readStrategyBinding(selectedStrategy.value);
});

const selectedStrategyRuntimeObservation = computed<StrategyRuntimeObservation | null>(
    () => selectedStrategy.value?.runtimeObservation ?? null,
);

const createDefinition = computed(
    () => strategyDefinitions.value.find((item) => item.id === createDefinitionId.value) ?? null,
);

const createSymbolTags = computed(() => parseSymbolsText(createSymbolsText.value));
const editSymbolTags = computed(() => parseSymbolsText(editSymbolsText.value));
const createSelectedBrokerAccountOption = computed(
    () => resolveBrokerAccountOption(createBrokerAccountKey.value),
);
const editSelectedBrokerAccountOption = computed(
    () => resolveBrokerAccountOption(editBrokerAccountKey.value),
);

const activeStrategyCount = computed(
    () => strategies.value.filter((item) => item.runtimeObservation?.actualStatus === "RUNNING").length,
);

const brokerAccountOptions = computed(() => availableBrokerAccounts.value);

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
    filterBrokerAccountOptions(createBrokerAccountQuery.value),
);
const editFilteredBrokerAccountOptions = computed(() =>
    filterBrokerAccountOptions(editBrokerAccountQuery.value),
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
    if (selectedStrategy.value.runtime === "dsl-go-plan") {
        return "当前实例已完成 DSL 编译与 requirements 规划，但暂不可启动。";
    }
    return "当前实例暂不可启动。";
});

const selectedStrategyCompiledSummary = computed(() => {
    if (selectedStrategy.value === null || selectedStrategy.value.runtime !== "dsl-go-plan") {
        return "";
    }
    const hookCount = readCompiledHookCount(selectedStrategy.value);
    const indicatorCount = readCompiledIndicatorCount(selectedStrategy.value);
    const parts: string[] = [];
    if (hookCount !== null) parts.push(`${hookCount} 个 hook`);
    if (indicatorCount !== null) parts.push(`${indicatorCount} 项依赖`);
    if (parts.length === 0) return "已完成 DSL 编译计划。";
    return `已完成 DSL 编译计划，包含 ${parts.join(" / ")}。`;
});

const strategyLogViewEntries = computed<StrategyLogViewEntry[]>(() =>
    strategyLogs.value.map((entry) => parseStrategyLogEntry(entry)),
);

const strategyAuditViewEntries = computed<StrategyAuditViewEntry[]>(() =>
    strategyAuditEntries.value.map((entry) => ({
        ...entry,
        detailText: entry.detail ?? "生命周期变更",
        label: formatAuditKind(entry.kind),
        level: classifyStrategyAuditLevel(entry),
    })),
);

const strategyActivityTabs = computed(() => [
    {
        value: "logs" as const,
        label: "运行日志",
        count: strategyLogViewEntries.value.length,
    },
    {
        value: "audit" as const,
        label: "运行审计",
        count: strategyAuditViewEntries.value.length,
    },
]);

const strategyActivityLevelOptions = computed(() => {
    const items = strategyActivityTab.value === "logs"
        ? strategyLogViewEntries.value
        : strategyAuditViewEntries.value;
    const counts = new Map<Exclude<StrategyActivityLevel, "all">, number>([
        ["error", 0],
        ["warning", 0],
        ["info", 0],
    ]);
    for (const item of items) {
        counts.set(item.level, (counts.get(item.level) ?? 0) + 1);
    }
    const options: Array<{
        value: StrategyActivityLevel;
        label: string;
        count: number;
    }> = [{
        value: "all" as const,
        label: formatStrategyActivityLevel("all"),
        count: items.length,
    }];
    for (const level of ["error", "warning", "info"] as const) {
        const count = counts.get(level) ?? 0;
        if (count === 0) {
            continue;
        }
        options.push({
            value: level,
            label: formatStrategyActivityLevel(level),
            count,
        });
    }
    return options;
});

const filteredStrategyLogViewEntries = computed(() => {
    if (strategyActivityLevelFilter.value === "all") {
        return strategyLogViewEntries.value;
    }
    return strategyLogViewEntries.value.filter(
        (entry) => entry.level === strategyActivityLevelFilter.value,
    );
});

const filteredStrategyAuditViewEntries = computed(() => {
    if (strategyActivityLevelFilter.value === "all") {
        return strategyAuditViewEntries.value;
    }
    return strategyAuditViewEntries.value.filter(
        (entry) => entry.level === strategyActivityLevelFilter.value,
    );
});

const strategyActivityEmptyMessage = computed(() => {
    if (strategyActivityTab.value === "logs") {
        return strategyActivityLevelFilter.value === "all"
            ? "暂无日志。"
            : "当前筛选下暂无日志。";
    }
    return strategyActivityLevelFilter.value === "all"
        ? "暂无审计记录。"
        : "当前筛选下暂无审计记录。";
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
    void Promise.all([loadStrategyDefinitions(), loadStrategies()]);
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
    (strategy, previousStrategy) => {
        if (strategy?.id !== previousStrategy?.id) {
            strategyActivityTab.value = "logs";
            strategyActivityLevelFilter.value = "all";
            strategyParamsDialogOpen.value = false;
        }
        if (strategy === null && instanceEditorMode.value === "edit") {
            closeInstanceEditorDialog();
        }
    },
);

watch(
    strategyActivityLevelOptions,
    (options) => {
        if (!options.some((option) => option.value === strategyActivityLevelFilter.value)) {
            strategyActivityLevelFilter.value = "all";
        }
    },
    { immediate: true },
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
    selectedStrategyBinding,
    (binding) => {
        if (binding === null) {
            editSymbolsText.value = "";
            editSymbolDraft.value = "";
            editSymbolValidationMessage.value = "";
            editInterval.value = "5m";
            editExecutionMode.value = "live";
            editBrokerAccountKey.value = defaultBrokerAccountSelectionKey.value;
            return;
        }

        editSymbolsText.value = binding.symbols.join("\n");
        editSymbolDraft.value = "";
        editSymbolValidationMessage.value = "";
        editInterval.value = binding.interval;
        editExecutionMode.value = binding.executionMode;
        editBrokerAccountKey.value =
            resolveBrokerAccountSelectionKey(binding.brokerAccount)
            || defaultBrokerAccountSelectionKey.value;
    },
    { immediate: true },
);

function normalizeText(value: unknown): string {
    return typeof value === "string" ? value.trim() : "";
}

function normalizeInstrumentId(value: string): string {
    const normalized = normalizeText(value).toUpperCase();
    if (normalized === "") {
        return "";
    }
    if (normalized.includes(":")) {
        const [market, symbol] = normalized.split(":", 2);
        if ((market ?? "") !== "" && (symbol ?? "") !== "") {
            return `${market}.${symbol}`;
        }
    }
    return normalized;
}

function normalizeSymbols(values: string[]): string[] {
    const seen = new Set<string>();
    const result: string[] = [];
    for (const value of values) {
        const normalized = normalizeInstrumentId(value);
        if (normalized === "" || seen.has(normalized)) {
            continue;
        }
        seen.add(normalized);
        result.push(normalized);
    }
    return result;
}

function splitSymbolsText(value: string): string[] {
    return value
    .split(/[\s,，;；]+/)
        .map((segment) => segment.trim())
        .filter((segment) => segment !== "");
}

function parseSymbolsText(value: string): string[] {
    return normalizeSymbols(splitSymbolsText(value));
}

function isValidNormalizedInstrumentId(value: string): boolean {
    return /^[A-Z0-9_-]+\.[A-Z0-9._-]+$/.test(value);
}

function parseValidatedSymbolsText(value: string): string[] {
    return normalizeSymbols(
        splitSymbolsText(value)
            .map((segment) => normalizeInstrumentId(segment))
            .filter((segment) => isValidNormalizedInstrumentId(segment)),
    );
}

function invalidSymbolsFromText(value: string): string[] {
    const seen = new Set<string>();
    const invalidSymbols: string[] = [];
    for (const segment of splitSymbolsText(value)) {
        const normalized = normalizeInstrumentId(segment);
        if (normalized === "" || isValidNormalizedInstrumentId(normalized) || seen.has(normalized)) {
            continue;
        }
        seen.add(normalized);
        invalidSymbols.push(normalized);
    }
    return invalidSymbols;
}

function formatTimestamp(value: unknown): string {
    const normalized = normalizeText(value);
    if (normalized === "") return "暂无";
    return normalized.replace("T", " ").replace(".000Z", "Z");
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
        case "dsl-go-plan":
            return "DSL 编译计划";
        default:
            return "未知 / 受限";
    }
}

function formatSourceFormat(sourceFormat: StrategySourceFormat | string | null | undefined): string {
    switch (normalizeText(sourceFormat)) {
        case "dsl-v1":
            return "DSL v1";
        default:
            return "未知 / 受限";
    }
}

function formatStrategyEligibility(strategy: StrategyInstanceItem): string {
    if (strategy.startable) return "可启动";
    if (strategy.runtime === "dsl-go-plan") return "待启用";
    return "受限";
}

function formatStrategyExecutionMode(mode: StrategyExecutionMode | string | null | undefined): string {
    return normalizeText(mode) === "notify_only" ? "仅通知" : "确认执行";
}

function formatTradingEnvironment(value: unknown): string {
    switch (normalizeText(value).toUpperCase()) {
        case "SIMULATE":
            return "模拟盘";
        case "REAL":
            return "实盘";
        default:
            return normalizeText(value) || "未设置";
    }
}

function normalizeBrokerAccountBinding(
    value: StrategyBrokerAccountBinding | null | undefined,
): StrategyBrokerAccountBinding | null {
    if (value == null) {
        return null;
    }

    const brokerId = normalizeText(value.brokerId).toLowerCase();
    const accountId = normalizeText(value.accountId);
    const tradingEnvironment = normalizeText(value.tradingEnvironment).toUpperCase();
    const market = normalizeText(value.market).toUpperCase();

    if (brokerId === "" && accountId === "" && tradingEnvironment === "" && market === "") {
        return null;
    }

    return {
        brokerId,
        accountId,
        tradingEnvironment,
        market,
    };
}

function readStrategySymbolsFromParams(
    params: Record<string, unknown> | null,
): string[] {
    if (params === null) {
        return [];
    }
    if (Array.isArray(params.symbols)) {
        return normalizeSymbols(
            params.symbols.filter((entry): entry is string => typeof entry === "string"),
        );
    }
    const symbol = normalizeInstrumentId(normalizeText(params.symbol));
    return symbol === "" ? [] : [symbol];
}

function readStrategyBrokerAccount(
    params: Record<string, unknown> | null,
): StrategyBrokerAccountBinding | null {
    if (params === null) {
        return null;
    }
    const brokerAccount = asRecord(params.brokerAccount);
    if (brokerAccount === null) {
        return null;
    }
    return normalizeBrokerAccountBinding({
        brokerId: normalizeText(brokerAccount.brokerId),
        accountId: normalizeText(brokerAccount.accountId),
        tradingEnvironment: normalizeText(brokerAccount.tradingEnvironment),
        market: normalizeText(brokerAccount.market),
    });
}

function readStrategyBinding(strategy: StrategyInstanceItem): StrategyInstanceBindingDocument {
    const params = asRecord(strategy.params);
    const bindingSymbols = Array.isArray(strategy.binding?.symbols)
        ? normalizeSymbols(strategy.binding.symbols)
        : readStrategySymbolsFromParams(params);
    const executionModeSource =
        normalizeText(strategy.binding?.executionMode)
        || normalizeText(params?.executionMode);
    return {
        symbols: bindingSymbols,
        interval: normalizeText(strategy.binding?.interval) || normalizeText(params?.interval) || "5m",
        executionMode: executionModeSource === "notify_only" ? "notify_only" : "live",
        brokerAccount: normalizeBrokerAccountBinding(strategy.binding?.brokerAccount)
            ?? readStrategyBrokerAccount(params),
    };
}

function formatBrokerAccountSummary(
    brokerAccount: StrategyBrokerAccountBinding | null | undefined,
): string {
    const normalized = normalizeBrokerAccountBinding(brokerAccount);
    if (normalized == null) {
        return "未绑定账号";
    }
    return `${normalized.brokerId.toUpperCase()} / ${formatTradingEnvironment(normalized.tradingEnvironment)} / ${normalized.accountId} / ${normalized.market}`;
}

function formatStrategySymbols(strategy: StrategyInstanceItem): string {
    const symbols = readStrategyBinding(strategy).symbols;
    return symbols.length > 0 ? symbols.join(", ") : "未绑定交易代码";
}

function formatStrategyInterval(strategy: StrategyInstanceItem): string {
    return readStrategyBinding(strategy).interval || "5m";
}

function formatRuntimeObservationSymbols(symbols: string[] | null | undefined): string {
    if (!Array.isArray(symbols)) {
        return "暂无";
    }
    const normalized = normalizeSymbols(symbols);
    return normalized.length > 0 ? normalized.join(", ") : "暂无";
}

function symbolTagsFor(mode: StrategySymbolEditorMode): string[] {
    return mode === "create" ? createSymbolTags.value : editSymbolTags.value;
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

function setSymbolTags(mode: StrategySymbolEditorMode, tags: string[]): void {
    const nextValue = normalizeSymbols(tags).join("\n");
    if (mode === "create") {
        createSymbolsText.value = nextValue;
        createSymbolDraft.value = "";
        return;
    }
    editSymbolsText.value = nextValue;
    editSymbolDraft.value = "";
}

function commitSymbolDraft(mode: StrategySymbolEditorMode, draft = symbolDraftFor(mode)): boolean {
    const parsed = parseValidatedSymbolsText(draft);
    const invalidSymbols = invalidSymbolsFromText(draft);
    if (parsed.length === 0) {
        setSymbolDraft(mode, "");
    } else {
        setSymbolTags(mode, [...symbolTagsFor(mode), ...parsed]);
    }
    if (invalidSymbols.length > 0) {
        setSymbolValidationMessage(
            mode,
            `已忽略无效交易代码：${invalidSymbols.join("、")}。请使用带市场前缀的格式，例如 US.TME、HK.00700。`,
        );
        return false;
    }
    setSymbolValidationMessage(mode, "");
    return true;
}

function removeSymbolTag(mode: StrategySymbolEditorMode, symbol: string): void {
    setSymbolTags(
        mode,
        symbolTagsFor(mode).filter((item) => item !== symbol),
    );
}

function handleSymbolDraftKeydown(event: KeyboardEvent, mode: StrategySymbolEditorMode): void {
    if (event.isComposing) {
        return;
    }
    if (event.key === "Enter" || event.key === "," || event.key === "Tab") {
        event.preventDefault();
        commitSymbolDraft(mode);
        return;
    }
    if (event.key === "Backspace" && normalizeText(symbolDraftFor(mode)) === "") {
        const tags = symbolTagsFor(mode);
        if (tags.length === 0) {
            return;
        }
        event.preventDefault();
        setSymbolTags(mode, tags.slice(0, -1));
    }
}

function handleSymbolDraftPaste(event: ClipboardEvent, mode: StrategySymbolEditorMode): void {
    const pastedText = event.clipboardData?.getData("text") ?? "";
    if (splitSymbolsText(pastedText).length <= 1) {
        return;
    }
    event.preventDefault();
    commitSymbolDraft(mode, pastedText);
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

function setExecutionMode(mode: StrategySymbolEditorMode, value: string): void {
    const normalized = value === "notify_only" ? "notify_only" : "live";
    if (mode === "create") {
        createExecutionMode.value = normalized;
        return;
    }
    editExecutionMode.value = normalized;
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

function filterBrokerAccountOptions(query: string): BrokerAccountSelectionOption[] {
    const normalizedQuery = normalizeText(query).toLowerCase();
    if (normalizedQuery === "") {
        return brokerAccountOptions.value;
    }
    return brokerAccountOptions.value.filter((option) =>
        [
            option.displayName,
            option.accountId,
            option.market,
            option.brokerId,
            option.tradingEnvironment,
            formatBrokerAccountOption(option),
        ]
            .filter((value): value is string => typeof value === "string")
            .some((value) => value.toLowerCase().includes(normalizedQuery)),
    );
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

function resolveBrokerAccountSelectionKey(
    brokerAccount: StrategyBrokerAccountBinding | null | undefined,
): string {
    const normalized = normalizeBrokerAccountBinding(brokerAccount);
    if (normalized == null) {
        return "";
    }

    const selectionKey = buildBrokerAccountSelectionKey({
        brokerId: normalized.brokerId,
        tradingEnvironment: normalized.tradingEnvironment,
        accountId: normalized.accountId,
        market: normalized.market,
    });

    return brokerAccountOptions.value.find((option) => option.selectionKey === selectionKey)?.selectionKey ?? "";
}

function resolveBrokerAccountOption(selectionKey: string): BrokerAccountSelectionOption | null {
    return brokerAccountOptions.value.find((option) => option.selectionKey === selectionKey) ?? null;
}

function isCurrentBrokerAccountSelectionKey(selectionKey: string | null | undefined): boolean {
    return selectionKey != null && selectionKey !== "" && selectionKey === effectiveCurrentBrokerAccountSelectionKey.value;
}

function isCurrentBrokerAccountBinding(
    brokerAccount: StrategyBrokerAccountBinding | null | undefined,
): boolean {
    return isCurrentBrokerAccountSelectionKey(resolveBrokerAccountSelectionKey(brokerAccount));
}

function formatBrokerAccountOption(option: BrokerAccountSelectionOption): string {
    return `${option.brokerId.toUpperCase()} / ${formatTradingEnvironment(option.tradingEnvironment)} / ${option.accountId} / ${option.market}`;
}

function brokerAccountOptionSubtitle(option: BrokerAccountSelectionOption): string {
    return `${option.brokerId.toUpperCase()} / ${formatTradingEnvironment(option.tradingEnvironment)} / ${option.accountId} / ${option.market}`;
}

function buildStrategyBindingPayload(input: {
    symbolsText: string;
    interval: string;
    executionMode: StrategyExecutionMode;
    brokerAccountKey: string;
    fallbackBrokerAccount?: StrategyBrokerAccountBinding | null;
}): StrategyInstanceBindingDocument {
    const selectedAccount = resolveBrokerAccountOption(input.brokerAccountKey);
    return {
        symbols: parseValidatedSymbolsText(input.symbolsText),
        interval: normalizeText(input.interval) || "5m",
        executionMode: input.executionMode === "notify_only" ? "notify_only" : "live",
        brokerAccount: selectedAccount == null
            ? input.fallbackBrokerAccount ?? null
            : {
                brokerId: selectedAccount.brokerId,
                accountId: selectedAccount.accountId,
                tradingEnvironment: selectedAccount.tradingEnvironment,
                market: selectedAccount.market,
            },
    };
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

function formatAuditKind(kind: unknown): string {
    switch (normalizeText(kind).toLowerCase()) {
        case "instantiated":
            return "已实例化";
        case "binding.updated":
            return "已更新绑定";
        case "created":
            return "已创建";
        case "started":
            return "已启动";
        case "running":
            return "运行中";
        case "paused":
            return "已暂停";
        case "stopped":
            return "已停止";
        case "failed":
            return "执行失败";
        default:
            return normalizeText(kind) || "未知";
    }
}

function formatStrategyActivityLevel(level: StrategyActivityLevel): string {
    switch (level) {
        case "error":
            return "高优先";
        case "warning":
            return "需关注";
        case "info":
            return "常规";
        default:
            return "全部";
    }
}

function classifyStrategyLogLevel(message: string): Exclude<StrategyActivityLevel, "all"> {
    const normalized = normalizeText(message).toLowerCase();
    if (["panic", "fatal", "error", "failed", "exception", "reject", "denied", "timeout"].some(
        (keyword) => normalized.includes(keyword),
    )) {
        return "error";
    }
    if (["warn", "warning", "paused", "pause", "stopped", "stop", "retry", "skip", "throttle"].some(
        (keyword) => normalized.includes(keyword),
    )) {
        return "warning";
    }
    return "info";
}

function classifyStrategyAuditLevel(entry: StrategyAuditEntry): Exclude<StrategyActivityLevel, "all"> {
    const signal = `${normalizeText(entry.kind)} ${normalizeText(entry.detail)}`.toLowerCase();
    if (["failed", "panic", "error", "exception", "reject", "denied", "timeout"].some(
        (keyword) => signal.includes(keyword),
    )) {
        return "error";
    }
    if (["paused", "pause", "stopped", "stop", "retry", "warning", "warn"].some(
        (keyword) => signal.includes(keyword),
    )) {
        return "warning";
    }
    return "info";
}

function parseStrategyLogEntry(entry: string): StrategyLogViewEntry {
    const raw = normalizeText(entry);
    const matched = raw.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s*(.*)$/);
    const at = matched?.[1] ?? "";
    const message = normalizeText(matched?.[2]) || raw;
    return {
        raw: entry,
        message,
        at,
        level: classifyStrategyLogLevel(message || raw),
    };
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
    }
}

async function loadStrategyDetails(instanceId: string): Promise<void> {
    selectedStrategyId.value = instanceId;
    detailsError.value = "";
    isLoadingDetails.value = true;

    try {
        const [logs, audit] = await Promise.all([
            fetchEnvelope<StrategyLogsResponse>(
                `/api/v1/strategies/${encodeURIComponent(instanceId)}/logs`,
            ),
            fetchEnvelope<StrategyAuditResponse>(
                `/api/v1/strategies/${encodeURIComponent(instanceId)}/audit`,
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
    }
}

async function createStrategyInstance(): Promise<void> {
    clearInstanceMutationMessages();
    if (createSymbolValidationMessage.value !== "") {
        instanceMutationError.value = createSymbolValidationMessage.value;
        return;
    }
    if (!commitSymbolDraft("create")) {
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
                    symbolsText: createSymbolsText.value,
                    interval: createInterval.value,
                    executionMode: createExecutionMode.value,
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
    createSymbolValidationMessage.value = "";
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
    if (!commitSymbolDraft("edit")) {
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
                    symbolsText: editSymbolsText.value,
                    interval: editInterval.value,
                    executionMode: editExecutionMode.value,
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
                <div class="inline-flex rounded-full border border-slate-200 bg-slate-50 p-1">
                    <button
                        class="rounded-full px-4 py-2 text-sm font-medium text-slate-600 transition hover:text-slate-900"
                        data-testid="strategy-workspace-tab-design" type="button"
                        @click="emit('switch-to-design', { mode: 'existing' })">
                        策略设计
                    </button>
                    <button
                        class="rounded-full bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm transition"
                        data-testid="strategy-workspace-tab-runtime" type="button">
                        策略运行
                    </button>
                </div>

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

            <div class="grid gap-4 lg:grid-cols-[1fr_1fr]">
                <div class="rounded-[28px] border border-slate-200 bg-slate-50/70 p-4">
                    <div class="flex items-center justify-between gap-3">
                        <div class="text-xl font-semibold text-slate-900">运行总览</div>
                        <span
                            class="rounded-full border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.18em] text-slate-600">
                            {{ activeStrategyCount }} 个运行中
                        </span>
                    </div>
                    <div class="mt-4 grid gap-3 md:grid-cols-3">
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">交易环境</div>
                            <div class="mt-2 text-2xl font-semibold text-slate-900">
                                {{ systemStatus.defaultTradingEnvironment }}
                            </div>
                        </div>
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">当前策略</div>
                            <div class="mt-2 text-xl font-semibold text-slate-900">
                                {{ selectedStrategy?.definition.name ?? "暂无" }}
                            </div>
                        </div>
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">运行形态</div>
                            <div class="mt-2 text-xl font-semibold text-slate-900" data-testid="strategy-runtime-mode">
                                {{ selectedStrategyRuntimeLabel }}
                            </div>
                        </div>
                    </div>
                    <div class="mt-4 text-sm text-slate-600">
                        运行面板负责实例控制、日志和审计；新策略统一使用 DSL 编译计划生命周期。
                    </div>
                </div>

                <div class="rounded-[28px] border border-slate-200 bg-slate-50/70 p-4">
                    <div class="text-xl font-semibold text-slate-900">运行保护</div>
                    <div class="mt-4 grid gap-3 md:grid-cols-3">
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">实盘开关</div>
                            <div class="mt-2 text-xl font-semibold text-slate-900">
                                {{ systemStatus.realTradingEnabled ? "已开启" : "已关闭" }}
                            </div>
                        </div>
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">急停开关</div>
                            <div class="mt-2 text-xl font-semibold"
                                :class="systemStatus.realTradingKillSwitch.active ? 'text-red-600' : 'text-teal-700'">
                                {{ systemStatus.realTradingKillSwitch.active ? "已启用" : "未启用" }}
                            </div>
                        </div>
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最大下单数量</div>
                            <div class="mt-2 text-xl font-semibold text-slate-900">
                                {{ systemStatus.realTradingRisk.maxOrderQuantity ?? "暂无" }}
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="grid gap-4" :class="selectedStrategy === null ? 'grid-cols-1' : 'xl:grid-cols-[minmax(22rem,26rem)_minmax(0,1fr)]'">
                <div class="min-w-0 rounded-[28px] border border-slate-200 bg-white p-4">
                    <div class="mb-4 flex items-center justify-between gap-3">
                        <div class="text-xl font-semibold text-slate-900">策略实例</div>
                        <div class="flex flex-wrap items-center gap-2">
                            <div class="relative">
                                <button
                                    class="rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                                    data-testid="strategy-create-menu-toggle" type="button"
                                    :aria-expanded="isCreateMenuOpen ? 'true' : 'false'"
                                    @click="toggleCreateMenu">
                                    新增
                                </button>
                                <div v-if="isCreateMenuOpen" data-testid="strategy-create-menu"
                                    class="absolute right-0 z-10 mt-2 grid min-w-[12rem] gap-1 rounded-3xl border border-slate-200 bg-white p-2 shadow-lg">
                                    <button
                                        class="rounded-2xl px-3 py-2 text-left text-sm font-medium text-slate-700 transition hover:bg-slate-50 hover:text-slate-900"
                                        data-testid="strategy-new-definition" type="button"
                                        @click="openCreateDefinition">
                                        新增策略
                                    </button>
                                    <button
                                        class="rounded-2xl px-3 py-2 text-left text-sm font-medium text-slate-700 transition hover:bg-slate-50 hover:text-slate-900"
                                        data-testid="strategy-new-instance" type="button"
                                        @click="openCreateInstanceForm">
                                        新增实例
                                    </button>
                                </div>
                            </div>
                            <button
                                class="rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                                type="button" @click="loadStrategies()">
                                {{ isLoadingStrategies ? "等待" : "刷新" }}
                            </button>
                        </div>
                    </div>
                    <v-dialog v-model="instanceEditorOpen" max-width="980">
                        <div class="strategy-instance-dialog" data-testid="strategy-instance-dialog">
                            <div class="flex items-start justify-between gap-3">
                                <div>
                                    <div class="text-sm font-semibold uppercase tracking-[0.16em] text-slate-500">{{ instanceEditorTitle }}</div>
                                    <div class="mt-1 text-sm text-slate-500">
                                        {{ instanceEditorHint }}
                                    </div>
                                </div>
                                <div class="flex flex-wrap items-center gap-2">
                                    <button v-if="isCreateInstanceEditor"
                                        class="rounded-full border border-slate-300 px-3 py-1.5 text-xs font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                                        type="button" @click="loadStrategyDefinitions()">
                                        {{ isLoadingDefinitions ? "等待" : "刷新定义" }}
                                    </button>
                                    <button
                                        class="rounded-full border border-slate-300 px-3 py-1.5 text-xs font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                                        :data-testid="isCreateInstanceEditor ? 'strategy-create-instance-close' : 'strategy-edit-instance-close'"
                                        type="button" @click="closeInstanceEditorDialog">
                                        关闭
                                    </button>
                                </div>
                            </div>
                            <div v-if="definitionsError && isCreateInstanceEditor"
                                class="mt-3 rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
                                {{ definitionsError }}
                            </div>
                            <div v-else-if="isCreateInstanceEditor && strategyDefinitions.length === 0"
                                class="mt-3 rounded-3xl border border-dashed border-slate-300 bg-white px-4 py-5 text-sm text-slate-500">
                                <div>暂无已保存策略定义。</div>
                                <button
                                    class="mt-3 rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                                    type="button" @click="openCreateDefinition">
                                    去设计区创建
                                </button>
                            </div>
                            <div v-else-if="isEditInstanceEditor && selectedStrategy === null"
                                class="mt-3 rounded-3xl border border-dashed border-slate-300 bg-white px-4 py-5 text-sm text-slate-500">
                                请先选择策略实例。
                            </div>
                            <div v-else class="mt-4 grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.25fr)_minmax(18rem,22rem)]">
                                <div class="min-w-0 grid gap-3"
                                    :data-testid="isCreateInstanceEditor ? 'strategy-create-instance-panel' : 'strategy-edit-instance-panel'">
                                    <label v-if="isCreateInstanceEditor" class="grid gap-1.5 text-sm text-slate-600">
                                        <span class="font-medium text-slate-700">策略定义</span>
                                        <select v-model="createDefinitionId" data-testid="strategy-instance-definition"
                                            class="rounded-2xl border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition focus:border-slate-500">
                                            <option value="" disabled>请选择策略定义</option>
                                            <option v-for="definition in strategyDefinitions" :key="definition.id" :value="definition.id">
                                                {{ definition.name }} / v{{ definition.version }}
                                            </option>
                                        </select>
                                    </label>
                                    <div v-else class="rounded-3xl bg-slate-50 px-4 py-4 text-sm text-slate-600">
                                        <div class="text-xs uppercase tracking-[0.16em] text-slate-500">策略定义</div>
                                        <div class="mt-2 break-words font-medium text-slate-900">
                                            {{ selectedStrategy?.definition.name }} / v{{ selectedStrategy?.definition.version }}
                                        </div>
                                    </div>
                                    <label class="grid gap-1.5 text-sm text-slate-600">
                                        <span class="font-medium text-slate-700">交易代码</span>
                                        <div class="strategy-tag-input"
                                            :class="{ 'strategy-tag-input--invalid': symbolValidationMessageFor(activeInstanceEditorMode) !== '' }">
                                            <button
                                                v-for="symbol in symbolTagsFor(activeInstanceEditorMode)"
                                                :key="`${activeInstanceEditorMode}-${symbol}`"
                                                class="strategy-tag-chip"
                                                type="button"
                                                @click="removeSymbolTag(activeInstanceEditorMode, symbol)">
                                                <span>{{ symbol }}</span>
                                                <span class="strategy-tag-chip__remove">x</span>
                                            </button>
                                            <input
                                                :value="symbolDraftFor(activeInstanceEditorMode)"
                                                :data-testid="activeInstanceEditorMode === 'create' ? 'strategy-instance-symbols' : 'strategy-edit-symbols'"
                                                class="strategy-tag-input__field"
                                                placeholder="输入交易代码后按 Enter，例如 US.TME"
                                                type="text"
                                                @input="setSymbolDraft(activeInstanceEditorMode, ($event.target as HTMLInputElement).value)"
                                                @blur="commitSymbolDraft(activeInstanceEditorMode)"
                                                @keydown="handleSymbolDraftKeydown($event, activeInstanceEditorMode)"
                                                @paste="handleSymbolDraftPaste($event, activeInstanceEditorMode)">
                                        </div>
                                        <span v-if="symbolValidationMessageFor(activeInstanceEditorMode)"
                                            :data-testid="activeInstanceEditorMode === 'create' ? 'strategy-instance-symbols-validation' : 'strategy-edit-symbols-validation'"
                                            class="text-xs text-amber-700">
                                            {{ symbolValidationMessageFor(activeInstanceEditorMode) }}
                                        </span>
                                        <span v-else class="text-xs text-slate-500">
                                            {{ activeInstanceEditorMode === 'create'
                                                ? '支持多个交易代码，按 Enter、Tab、逗号或直接粘贴多个代码生成标签。'
                                                : '为空时表示暂未绑定交易代码；按 Backspace 可快速删除最后一个标签。' }}
                                        </span>
                                    </label>
                                    <div class="grid gap-3 md:grid-cols-2">
                                        <label class="grid gap-1.5 text-sm text-slate-600">
                                            <span class="font-medium text-slate-700">运行周期</span>
                                            <input
                                                :value="intervalValueFor(activeInstanceEditorMode)"
                                                :data-testid="activeInstanceEditorMode === 'create' ? 'strategy-instance-interval' : 'strategy-edit-interval'"
                                                class="rounded-2xl border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition focus:border-slate-500"
                                                placeholder="5m" type="text"
                                                @input="setIntervalValue(activeInstanceEditorMode, ($event.target as HTMLInputElement).value)">
                                        </label>
                                        <label class="grid gap-1.5 text-sm text-slate-600">
                                            <span class="font-medium text-slate-700">执行模式</span>
                                            <select
                                                :value="executionModeFor(activeInstanceEditorMode)"
                                                :data-testid="activeInstanceEditorMode === 'create' ? 'strategy-instance-execution-mode' : 'strategy-edit-execution-mode'"
                                                class="rounded-2xl border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition focus:border-slate-500"
                                                @change="setExecutionMode(activeInstanceEditorMode, ($event.target as HTMLSelectElement).value)">
                                                <option value="live">确认执行</option>
                                                <option value="notify_only">仅通知</option>
                                            </select>
                                        </label>
                                    </div>
                                    <label class="grid gap-1.5 text-sm text-slate-600">
                                        <span class="font-medium text-slate-700">券商账号</span>
                                        <div class="strategy-account-picker">
                                            <button
                                                class="strategy-account-picker__trigger"
                                                :data-testid="activeInstanceEditorMode === 'create' ? 'strategy-instance-account' : 'strategy-edit-account'"
                                                type="button"
                                                @click="toggleBrokerAccountPicker(activeInstanceEditorMode)">
                                                <span class="strategy-account-picker__copy">
                                                    <span class="strategy-account-picker__label">
                                                        {{ selectedBrokerAccountOptionFor(activeInstanceEditorMode)?.displayName ?? '暂不绑定账号' }}
                                                    </span>
                                                    <span v-if="selectedBrokerAccountOptionFor(activeInstanceEditorMode)"
                                                        class="strategy-account-picker__meta">
                                                        <span>{{ brokerAccountOptionSubtitle(selectedBrokerAccountOptionFor(activeInstanceEditorMode)!) }}</span>
                                                        <span v-if="isCurrentBrokerAccountSelectionKey(selectedBrokerAccountKeyFor(activeInstanceEditorMode))"
                                                            :data-testid="activeInstanceEditorMode === 'create' ? 'strategy-create-account-current-tag' : 'strategy-edit-account-current-tag'"
                                                            class="strategy-account-picker__tag strategy-account-picker__tag--current">
                                                            当前
                                                        </span>
                                                    </span>
                                                    <span v-else class="strategy-account-picker__meta">保留当前默认路由</span>
                                                </span>
                                                <span class="strategy-account-picker__action">
                                                    {{ isBrokerAccountPickerOpen(activeInstanceEditorMode) ? '收起' : '搜索选择' }}
                                                </span>
                                            </button>
                                            <div v-if="isBrokerAccountPickerOpen(activeInstanceEditorMode)" class="strategy-account-picker__menu">
                                                <input
                                                    :value="brokerAccountQueryFor(activeInstanceEditorMode)"
                                                    :data-testid="activeInstanceEditorMode === 'create' ? 'strategy-instance-account-search' : 'strategy-edit-account-search'"
                                                    class="strategy-account-picker__search"
                                                    placeholder="搜索账号 / 环境 / 市场"
                                                    type="text"
                                                    @input="setBrokerAccountQuery(activeInstanceEditorMode, ($event.target as HTMLInputElement).value)">
                                                <div class="strategy-account-picker__options">
                                                    <button
                                                        class="strategy-account-picker__option"
                                                        :class="{ 'is-active': selectedBrokerAccountKeyFor(activeInstanceEditorMode) === '' }"
                                                        :data-testid="activeInstanceEditorMode === 'create' ? 'strategy-instance-account-option-none' : 'strategy-edit-account-option-none'"
                                                        type="button"
                                                        @click="clearBrokerAccountSelection(activeInstanceEditorMode)">
                                                        <span class="strategy-account-picker__option-title">暂不绑定账号</span>
                                                        <span class="strategy-account-picker__option-meta">保留当前默认路由</span>
                                                    </button>
                                                    <button
                                                        v-for="option in filteredBrokerAccountOptionsFor(activeInstanceEditorMode)"
                                                        :key="option.selectionKey"
                                                        class="strategy-account-picker__option"
                                                        :class="{ 'is-active': selectedBrokerAccountKeyFor(activeInstanceEditorMode) === option.selectionKey }"
                                                        :data-testid="`${activeInstanceEditorMode === 'create' ? 'strategy-instance-account-option' : 'strategy-edit-account-option'}-${option.accountId}`"
                                                        type="button"
                                                        @click="selectBrokerAccountOption(activeInstanceEditorMode, option.selectionKey)">
                                                        <span class="strategy-account-picker__option-header">
                                                            <span class="strategy-account-picker__option-title">{{ option.displayName }}</span>
                                                            <span v-if="isCurrentBrokerAccountSelectionKey(option.selectionKey)"
                                                                class="strategy-account-picker__tag strategy-account-picker__tag--current">
                                                                当前
                                                            </span>
                                                        </span>
                                                        <span class="strategy-account-picker__option-meta">{{ brokerAccountOptionSubtitle(option) }}</span>
                                                    </button>
                                                    <div v-if="filteredBrokerAccountOptionsFor(activeInstanceEditorMode).length === 0"
                                                        class="strategy-account-picker__empty">
                                                        没有匹配的券商账号。
                                                    </div>
                                                </div>
                                            </div>
                                        </div>
                                    </label>
                                    <div v-if="executionModeFor(activeInstanceEditorMode) === 'notify_only'"
                                        class="rounded-3xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700">
                                        {{ activeInstanceEditorMode === 'create'
                                            ? '仅通知模式只发送准备下单提示，不自动下单。'
                                            : '仅通知模式会发送准备下单提示，不自动下单。实例卡片会同步显示“仅通知”。' }}
                                    </div>
                                </div>

                                <div class="min-w-0 rounded-3xl bg-slate-50 px-4 py-4">
                                    <div class="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">
                                        {{ isCreateInstanceEditor ? '创建预览' : '绑定预览' }}
                                    </div>
                                    <div class="mt-4 grid gap-3 text-sm text-slate-600">
                                        <div>
                                            <div class="text-xs uppercase tracking-[0.16em] text-slate-400">策略定义</div>
                                            <div class="mt-1 break-words font-medium text-slate-900">
                                                {{ isCreateInstanceEditor
                                                    ? (createDefinition == null ? '未选择策略定义' : `${createDefinition.name} / v${createDefinition.version}`)
                                                    : `${selectedStrategy?.definition.name ?? '未选择'} / v${selectedStrategy?.definition.version ?? ''}` }}
                                            </div>
                                        </div>
                                        <div>
                                            <div class="text-xs uppercase tracking-[0.16em] text-slate-400">交易代码</div>
                                            <div class="mt-1 break-words font-medium text-slate-900">
                                                {{ instanceEditorSymbolsSummary(activeInstanceEditorMode) }}
                                            </div>
                                        </div>
                                        <div>
                                            <div class="text-xs uppercase tracking-[0.16em] text-slate-400">周期</div>
                                            <div class="mt-1 font-medium text-slate-900">
                                                {{ intervalValueFor(activeInstanceEditorMode).trim() || '5m' }}
                                            </div>
                                        </div>
                                        <div>
                                            <div class="text-xs uppercase tracking-[0.16em] text-slate-400">执行模式</div>
                                            <div class="mt-1 font-medium text-slate-900">
                                                {{ formatStrategyExecutionMode(executionModeFor(activeInstanceEditorMode)) }}
                                            </div>
                                        </div>
                                        <div>
                                            <div class="text-xs uppercase tracking-[0.16em] text-slate-400">券商账号</div>
                                            <div class="mt-1 break-all font-medium text-slate-900">
                                                {{ instanceEditorBrokerAccountSummary(activeInstanceEditorMode) }}
                                            </div>
                                            <div v-if="isCurrentBrokerAccountSelectionKey(selectedBrokerAccountKeyFor(activeInstanceEditorMode))"
                                                class="mt-2 inline-flex rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.14em] text-emerald-700">
                                                当前
                                            </div>
                                        </div>
                                    </div>
                                    <div class="mt-4 flex flex-wrap gap-2">
                                        <button v-if="isCreateInstanceEditor"
                                            class="rounded-full border border-slate-900 bg-slate-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50"
                                            data-testid="strategy-create-instance" :disabled="!canCreateStrategyInstance"
                                            type="button" @click="createStrategyInstance">
                                            {{ isCreatingStrategyInstance ? "创建中" : `添加${createDefinition?.name ?? '策略'}到实例` }}
                                        </button>
                                        <template v-else>
                                            <button
                                                class="rounded-full border border-slate-900 bg-slate-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50"
                                                data-testid="strategy-update-binding"
                                                :disabled="!canUpdateSelectedStrategyBinding" type="button"
                                                @click="updateSelectedStrategyBinding">
                                                {{ isUpdatingStrategyBinding ? "保存中" : "保存绑定" }}
                                            </button>
                                            <button
                                                class="rounded-full border border-rose-300 px-4 py-2 text-sm font-medium text-rose-700 transition hover:border-rose-400 hover:text-rose-900 disabled:cursor-not-allowed disabled:opacity-50"
                                                data-testid="strategy-delete-instance"
                                                :disabled="!canDeleteSelectedStrategy" type="button"
                                                @click="deleteSelectedStrategy">
                                                {{ isDeletingStrategy ? "删除中" : "删除实例" }}
                                            </button>
                                        </template>
                                    </div>
                                    <div v-if="isEditInstanceEditor && selectedStrategy?.status !== 'STOPPED'" class="mt-3 text-xs text-amber-700">
                                        当前实例不是 STOPPED，先停止后才能修改绑定或删除。
                                    </div>
                                </div>
                            </div>
                        </div>
                    </v-dialog>
                    <div v-if="listError"
                        class="rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
                        {{ listError }}
                    </div>
                    <div v-else-if="strategies.length === 0"
                        class="rounded-3xl border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-500">
                        暂无策略实例。先从设计区保存定义并创建运行实例。
                    </div>
                    <div v-else class="grid gap-3">
                        <button v-for="strategy in strategies" :key="strategy.id"
                            :data-testid="`strategy-${strategy.id}`" class="strategy-list-card"
                            :class="[strategyStatusCardClass(strategy), { 'is-active': strategy.id === selectedStrategyId }]" type="button"
                            @click="loadStrategyDetails(strategy.id)">
                            <div class="flex items-center justify-between gap-3">
                                <div class="min-w-0 break-words text-base font-semibold">{{ strategy.definition.name }}</div>
                                <div :data-testid="`strategy-status-${strategy.id}`" :class="strategyStatusBadgeClass(strategy)">{{
                                    formatStrategyStatus(displayStrategyStatus(strategy)) }}</div>
                            </div>
                            <div class="mt-2 break-all text-sm text-slate-500">{{ strategy.id }}</div>
                            <div class="mt-2 text-sm text-slate-500">标的 {{ formatStrategySymbols(strategy) }}</div>
                            <div class="mt-1 text-sm text-slate-500">
                                周期 {{ formatStrategyInterval(strategy) }}
                            </div>
                            <div class="mt-1 break-all text-sm text-slate-500">
                                {{ formatBrokerAccountSummary(readStrategyBinding(strategy).brokerAccount) }}
                            </div>
                            <div v-if="isCurrentBrokerAccountBinding(readStrategyBinding(strategy).brokerAccount)"
                                class="mt-1 inline-flex rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.14em] text-emerald-700">
                                当前
                            </div>
                            <div class="mt-2 text-sm text-slate-500">创建于 {{ formatTimestamp(strategy.createdAt) }}</div>
                            <div class="mt-3 flex flex-wrap gap-2 text-[11px] font-semibold uppercase tracking-[0.16em]">
                                <span class="rounded-full border border-slate-200 bg-slate-100 px-2.5 py-1 text-slate-600">
                                    {{ formatStrategyRuntime(strategy.runtime) }}
                                </span>
                                <span class="rounded-full border border-slate-200 bg-slate-100 px-2.5 py-1 text-slate-600">
                                    {{ formatSourceFormat(strategy.sourceFormat) }}
                                </span>
                                <span
                                    class="rounded-full px-2.5 py-1"
                                    :class="strategy.startable
                                        ? 'border border-emerald-200 bg-emerald-50 text-emerald-700'
                                        : 'border border-amber-200 bg-amber-50 text-amber-700'">
                                    {{ formatStrategyEligibility(strategy) }}
                                </span>
                                <span
                                    class="rounded-full px-2.5 py-1"
                                    :class="readStrategyBinding(strategy).executionMode === 'notify_only'
                                        ? 'border border-sky-200 bg-sky-50 text-sky-700'
                                        : 'border border-slate-200 bg-slate-100 text-slate-600'">
                                    {{ formatStrategyExecutionMode(readStrategyBinding(strategy).executionMode) }}
                                </span>
                            </div>
                        </button>
                    </div>
                </div>

                <div v-if="selectedStrategy !== null" class="min-w-0 grid gap-4">
                    <div class="grid gap-4 2xl:grid-cols-[minmax(19rem,22rem)_minmax(0,1fr)]">
                        <button
                            class="strategy-binding-summary min-w-0 rounded-[28px] border border-slate-200 bg-white p-4 text-left"
                            data-testid="strategy-current-binding-summary"
                            type="button" @click="openEditInstanceForm">
                            <div class="flex flex-wrap items-center justify-between gap-3">
                                <div>
                                    <div class="text-xl font-semibold text-slate-900">当前绑定摘要</div>
                                    <div class="mt-1 text-sm text-slate-500">
                                        点击卡片即可编辑绑定、更新执行模式或删除实例。
                                    </div>
                                </div>
                                <span
                                    class="rounded-full border border-slate-200 bg-slate-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">
                                    仅 STOPPED 可编辑
                                </span>
                            </div>
                            <div class="mt-4 grid gap-3 text-sm text-slate-600">
                                <div>
                                    <div class="text-xs uppercase tracking-[0.16em] text-slate-400">策略定义</div>
                                    <div class="mt-1 break-words font-medium text-slate-900">
                                        {{ selectedStrategy.definition.name }} / v{{ selectedStrategy.definition.version }}
                                    </div>
                                </div>
                                <div>
                                    <div class="text-xs uppercase tracking-[0.16em] text-slate-400">交易代码</div>
                                    <div class="mt-1 break-words font-medium text-slate-900">
                                        {{ formatStrategySymbols(selectedStrategy) }}
                                    </div>
                                </div>
                                <div class="grid gap-3 sm:grid-cols-2">
                                    <div>
                                        <div class="text-xs uppercase tracking-[0.16em] text-slate-400">周期</div>
                                        <div class="mt-1 font-medium text-slate-900">
                                            {{ formatStrategyInterval(selectedStrategy) }}
                                        </div>
                                    </div>
                                    <div>
                                        <div class="text-xs uppercase tracking-[0.16em] text-slate-400">执行模式</div>
                                        <div class="mt-1 font-medium text-slate-900">
                                            {{ formatStrategyExecutionMode(selectedStrategyBinding?.executionMode) }}
                                        </div>
                                    </div>
                                </div>
                                <div>
                                    <div class="text-xs uppercase tracking-[0.16em] text-slate-400">券商账号</div>
                                    <div class="mt-1 break-all font-medium text-slate-900">
                                        {{ formatBrokerAccountSummary(selectedStrategyBinding?.brokerAccount) }}
                                    </div>
                                    <div v-if="isCurrentBrokerAccountBinding(selectedStrategyBinding?.brokerAccount)"
                                        class="mt-2 inline-flex rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.14em] text-emerald-700">
                                        当前
                                    </div>
                                </div>
                            </div>
                            <div v-if="selectedStrategy.status !== 'STOPPED'" class="mt-4 text-xs text-amber-700">
                                当前实例不是 STOPPED，先停止后才能修改绑定或删除。
                            </div>
                        </button>

                        <div class="rounded-[28px] border border-slate-200 bg-white p-4">
                            <div class="flex flex-wrap items-center justify-between gap-3">
                                <div>
                                    <div class="text-xl font-semibold text-slate-900">运行控制</div>
                                    <div class="mt-1 text-sm text-slate-500">
                                        启动、暂停、停止都会同步刷新日志与审计视图。
                                    </div>
                                </div>
                                <div class="rounded-3xl bg-slate-50 px-4 py-4">
                                    <div class="flex flex-wrap gap-2">
                                        <span class="rounded-full border border-slate-200 bg-white px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-slate-600">
                                            {{ selectedStrategyRuntimeLabel }}
                                        </span>
                                        <span class="rounded-full border border-slate-200 bg-white px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-slate-600">
                                            {{ selectedStrategySourceFormatLabel }}
                                        </span>
                                        <span
                                            class="rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em]"
                                            :class="selectedStrategy.startable
                                                ? 'border border-emerald-200 bg-emerald-50 text-emerald-700'
                                                : 'border border-amber-200 bg-amber-50 text-amber-700'">
                                            {{ formatStrategyEligibility(selectedStrategy) }}
                                        </span>
                                        <span v-if="selectedStrategyBinding !== null"
                                            class="rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em]"
                                            :class="selectedStrategyBinding.executionMode === 'notify_only'
                                                ? 'border border-sky-200 bg-sky-50 text-sky-700'
                                                : 'border border-slate-200 bg-white text-slate-600'">
                                            {{ formatStrategyExecutionMode(selectedStrategyBinding.executionMode) }}
                                        </span>
                                    </div>
                                    <div class="mt-3 text-sm text-slate-600" data-testid="strategy-runtime-start-hint">
                                        {{ selectedStrategyStartHint }}
                                    </div>
                                    <div v-if="selectedStrategyCompiledSummary" class="mt-2 text-xs text-slate-500">
                                        {{ selectedStrategyCompiledSummary }}
                                    </div>
                                </div>
                            </div>

                            <div v-if="selectedStrategyRuntimeObservation !== null"
                                class="mt-4 rounded-3xl border border-slate-200 bg-white/80 px-4 py-4"
                                data-testid="strategy-runtime-observation">
                                <div class="text-[11px] uppercase tracking-[0.18em] text-slate-500">实际运行态</div>
                                <div class="mt-3 grid gap-3 text-sm text-slate-600 sm:grid-cols-2 xl:grid-cols-3">
                                    <div>
                                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">运行状态</div>
                                        <div class="mt-1 font-medium text-slate-900">
                                            {{ formatStrategyStatus(selectedStrategyRuntimeObservation.actualStatus) }}
                                        </div>
                                    </div>
                                    <div>
                                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">活跃标的</div>
                                        <div class="mt-1 font-medium text-slate-900">
                                            {{ formatRuntimeObservationSymbols(selectedStrategyRuntimeObservation.activeSymbols) }}
                                        </div>
                                    </div>
                                    <div>
                                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">最近闭合 K 线</div>
                                        <div class="mt-1 font-medium text-slate-900">
                                            {{ formatTimestamp(selectedStrategyRuntimeObservation.lastClosedKlineAt) }}
                                        </div>
                                    </div>
                                    <div>
                                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">最近信号</div>
                                        <div class="mt-1 font-medium text-slate-900">
                                            {{ formatTimestamp(selectedStrategyRuntimeObservation.lastSignalAt) }}
                                        </div>
                                    </div>
                                    <div>
                                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">最近下单</div>
                                        <div class="mt-1 font-medium text-slate-900">
                                            {{ formatTimestamp(selectedStrategyRuntimeObservation.lastOrderAt) }}
                                        </div>
                                    </div>
                                    <div>
                                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">最近更新</div>
                                        <div class="mt-1 font-medium text-slate-900">
                                            {{ formatTimestamp(selectedStrategyRuntimeObservation.updatedAt) }}
                                        </div>
                                    </div>
                                </div>
                                <div v-if="selectedStrategyRuntimeObservation.lastError"
                                    class="mt-3 rounded-2xl border border-amber-200 bg-amber-50 px-3 py-3 text-xs text-amber-700">
                                    最近异常：{{ selectedStrategyRuntimeObservation.lastError }}
                                    <span class="text-amber-600">（{{ formatTimestamp(selectedStrategyRuntimeObservation.lastErrorAt) }}）</span>
                                </div>
                            </div>
                            <div v-else class="mt-4 text-xs text-slate-500">
                                实例未运行时暂无实时观测信息。
                            </div>
                            <div class="mt-4 flex flex-wrap gap-2">
                                <button
                                    class="strategy-runtime-start-button rounded-full border border-emerald-300 px-4 py-2 text-sm font-medium text-emerald-700 transition hover:border-emerald-400 hover:text-emerald-900 disabled:cursor-not-allowed disabled:opacity-50"
                                    data-testid="strategy-start"
                                    :disabled="!canStartSelectedStrategy" type="button"
                                    @click="changeStrategyStatus('start')">
                                    启动
                                </button>
                                <button
                                    class="rounded-full border border-amber-300 px-4 py-2 text-sm font-medium text-amber-700 transition hover:border-amber-400 hover:text-amber-900 disabled:cursor-not-allowed disabled:opacity-50"
                                    data-testid="strategy-pause"
                                    :disabled="!canPauseSelectedStrategy" type="button"
                                    @click="changeStrategyStatus('pause')">
                                    暂停
                                </button>
                                <button
                                    class="rounded-full border border-rose-300 px-4 py-2 text-sm font-medium text-rose-700 transition hover:border-rose-400 hover:text-rose-900 disabled:cursor-not-allowed disabled:opacity-50"
                                    data-testid="strategy-stop"
                                    :disabled="!canStopSelectedStrategy" type="button"
                                    @click="changeStrategyStatus('stop')">
                                    停止
                                </button>
                            </div>
                            <div v-if="detailsError"
                                class="mt-4 rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
                                {{ detailsError }}
                            </div>
                        </div>
                    </div>

                    <div class="strategy-activity-panel rounded-[28px] border border-slate-200 bg-white p-4"
                        data-testid="strategy-activity-panel">
                        <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
                            <div>
                                <div class="text-xl font-semibold text-slate-900">运行明细</div>
                                <div class="mt-1 text-sm text-slate-500">
                                    在运行日志与运行审计之间切换，并按重要性快速聚焦当前实例的问题与状态变化。
                                </div>
                            </div>
                            <div class="flex flex-wrap items-center gap-2 xl:justify-end">
                                <button v-for="tab in strategyActivityTabs" :key="tab.value" type="button"
                                    class="strategy-activity-tab"
                                    :class="{ 'is-active': strategyActivityTab === tab.value }"
                                    :data-testid="`strategy-activity-tab-${tab.value}`"
                                    @click="strategyActivityTab = tab.value">
                                    <span>{{ tab.label }}</span>
                                    <span class="strategy-activity-tab-count">{{ tab.count }}</span>
                                </button>
                                <button type="button" class="strategy-runtime-params-trigger"
                                    data-testid="strategy-open-params-dialog" aria-label="查看运行参数"
                                    @click="strategyParamsDialogOpen = true">
                                    <span aria-hidden="true">{}</span>
                                </button>
                            </div>
                        </div>

                        <div class="mt-4 flex flex-wrap gap-2">
                            <button v-for="option in strategyActivityLevelOptions" :key="option.value"
                                type="button" class="strategy-activity-filter"
                                :class="[
                                    `strategy-activity-filter--${option.value}`,
                                    { 'is-active': strategyActivityLevelFilter === option.value },
                                ]"
                                :data-testid="`strategy-activity-filter-${option.value}`"
                                @click="strategyActivityLevelFilter = option.value">
                                <span>{{ option.label }}</span>
                                <span class="strategy-activity-filter-count">{{ option.count }}</span>
                            </button>
                        </div>

                        <div class="mt-4">
                            <div v-if="isLoadingDetails" class="strategy-activity-empty">
                                正在加载运行明细…
                            </div>

                            <ul v-else-if="strategyActivityTab === 'logs' && filteredStrategyLogViewEntries.length > 0"
                                class="strategy-activity-viewport" data-testid="strategy-log-list">
                                <li v-for="entry in filteredStrategyLogViewEntries" :key="entry.raw"
                                    class="strategy-activity-entry" :class="`strategy-activity-entry--${entry.level}`"
                                    data-testid="strategy-log-entry">
                                    <div class="flex flex-wrap items-center justify-between gap-3">
                                        <div class="flex flex-wrap items-center gap-2">
                                            <span class="strategy-activity-level-badge"
                                                :class="`strategy-activity-level-badge--${entry.level}`">{{
                                                    formatStrategyActivityLevel(entry.level) }}</span>
                                            <span class="strategy-activity-kind-badge">运行日志</span>
                                        </div>
                                        <span class="strategy-activity-time">{{
                                            entry.at === '' ? '未标注时间' : formatTimestamp(entry.at) }}</span>
                                    </div>
                                    <div class="mt-3 break-words font-mono text-xs leading-6 text-slate-700">
                                        {{ entry.message }}
                                    </div>
                                </li>
                            </ul>

                            <ul v-else-if="strategyActivityTab === 'audit' && filteredStrategyAuditViewEntries.length > 0"
                                class="strategy-activity-viewport" data-testid="strategy-audit-list">
                                <li v-for="entry in filteredStrategyAuditViewEntries"
                                    :key="`${entry.at}-${entry.kind}-${entry.detail ?? ''}`"
                                    class="strategy-activity-entry" :class="`strategy-activity-entry--${entry.level}`"
                                    data-testid="strategy-audit-entry">
                                    <div class="flex flex-wrap items-center justify-between gap-3">
                                        <div class="flex flex-wrap items-center gap-2">
                                            <span class="strategy-activity-level-badge"
                                                :class="`strategy-activity-level-badge--${entry.level}`">{{
                                                    formatStrategyActivityLevel(entry.level) }}</span>
                                            <span class="strategy-activity-kind-badge">{{ entry.label }}</span>
                                        </div>
                                        <span class="strategy-activity-time">{{ formatTimestamp(entry.at) }}</span>
                                    </div>
                                    <div class="mt-2 text-sm font-medium text-slate-900">
                                        {{ entry.detailText }}
                                    </div>
                                </li>
                            </ul>

                            <div v-else class="strategy-activity-empty">
                                {{ strategyActivityEmptyMessage }}
                            </div>
                        </div>

                        <v-dialog v-model="strategyParamsDialogOpen" max-width="840">
                            <div class="strategy-instance-dialog strategy-params-dialog"
                                data-testid="strategy-params-dialog">
                                <div class="flex flex-wrap items-start justify-between gap-4">
                                    <div>
                                        <div class="text-xl font-semibold text-slate-900">运行参数</div>
                                        <div class="mt-1 text-sm text-slate-500">
                                            当前实例启动时使用的参数快照，通过对话框查看避免占用主面板高度。
                                        </div>
                                    </div>
                                    <button type="button" class="strategy-params-dialog-close"
                                        data-testid="strategy-close-params-dialog"
                                        @click="strategyParamsDialogOpen = false">
                                        关闭
                                    </button>
                                </div>
                                <div class="mt-4 rounded-3xl bg-slate-50 px-4 py-4">
                                    <pre class="strategy-params-json">{{ selectedStrategyParamsJson }}</pre>
                                </div>
                            </div>
                        </v-dialog>
                    </div>
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

/* 复用主题 strategy-list-card */
.strategy-list-card {
    display: block;
    width: 100%;
    text-align: left;
    border-radius: 1.5rem;
    border: 1px solid var(--tv-border);
    background: color-mix(in srgb, var(--tv-bg-surface) 90%, transparent);
    padding: 0.9rem 1rem;
    cursor: pointer;
    transition: border-color 140ms ease, background-color 140ms ease;
}

.strategy-list-card:hover {
    border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
}

.strategy-list-card.is-active {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 18%, transparent);
}

.strategy-list-card--running {
    border-color: color-mix(in srgb, rgb(52 211 153) 58%, var(--tv-border));
    background: color-mix(in srgb, rgb(236 253 245) 90%, var(--tv-bg-surface));
}

.strategy-list-card--paused {
    border-color: color-mix(in srgb, rgb(251 191 36) 55%, var(--tv-border));
    background: color-mix(in srgb, rgb(255 251 235) 90%, var(--tv-bg-surface));
}

.strategy-list-card--stopped {
    border-color: var(--tv-border);
}

.strategy-status-badge {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 0.2rem 0.65rem;
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
}

.strategy-status-badge--running {
    border: 1px solid rgb(167 243 208);
    background: rgb(236 253 245);
    color: rgb(4 120 87);
}

.strategy-status-badge--paused {
    border: 1px solid rgb(253 230 138);
    background: rgb(254 249 195);
    color: rgb(161 98 7);
}

.strategy-status-badge--stopped {
    border: 1px solid rgb(226 232 240);
    background: rgb(248 250 252);
    color: rgb(71 85 105);
}

.tv-main .strategy-binding-summary {
    cursor: pointer;
    border-color: var(--card-border);
    background: var(--card-surface);
    color: var(--card-text-1);
    transition: border-color 140ms ease, background-color 140ms ease, transform 140ms ease, box-shadow 140ms ease;
}

.tv-main .strategy-binding-summary:hover {
    border-color: var(--card-border);
    background: var(--card-surface-raised);
    box-shadow: 0 18px 40px rgb(15 23 42 / 0.08);
    transform: translateY(-1px);
}

.tv-main .strategy-binding-summary:focus-visible {
    outline: 2px solid color-mix(in srgb, var(--tv-accent) 70%, var(--card-surface));
    outline-offset: 3px;
}

.tv-main .strategy-binding-summary .text-slate-900,
.tv-main .strategy-binding-summary .text-slate-800,
.tv-main .strategy-binding-summary .text-slate-700 {
    color: var(--card-text-1);
}

.tv-main .strategy-binding-summary .text-slate-600,
.tv-main .strategy-binding-summary .text-slate-500 {
    color: var(--card-text-2);
}

.tv-main .strategy-binding-summary .text-slate-400 {
    color: var(--card-text-3);
}

.tv-main .strategy-binding-summary .border-slate-200,
.tv-main .strategy-binding-summary .border-slate-300 {
    border-color: var(--card-border);
}

.tv-main .strategy-binding-summary .bg-white {
    background: var(--card-surface);
}

.tv-main .strategy-binding-summary .bg-slate-50 {
    background: var(--card-surface-raised);
}

.tv-main .strategy-runtime-start-button {
    border-color: var(--card-teal-border);
    background: color-mix(in srgb, var(--card-teal-surface) 74%, var(--tv-bg-surface) 26%);
    color: var(--card-teal-text);
    box-shadow: 0 8px 20px rgb(15 23 42 / 0.04);
    transition: border-color 140ms ease, background-color 140ms ease, color 140ms ease, box-shadow 140ms ease, transform 140ms ease;
}

.tv-main .strategy-runtime-start-button:hover {
    border-color: color-mix(in srgb, var(--card-teal-border) 60%, var(--tv-accent));
    background: color-mix(in srgb, var(--card-teal-surface) 84%, var(--tv-accent) 8%);
    color: var(--tv-text);
    box-shadow: 0 12px 24px rgb(15 23 42 / 0.08);
    transform: translateY(-1px);
}

.tv-main .strategy-runtime-start-button:disabled {
    border-color: var(--tv-border);
    background: var(--tv-bg-surface-2);
    color: var(--tv-text-dim);
    box-shadow: none;
    transform: none;
}

.tv-main .strategy-activity-panel {
    border-color: var(--card-border);
    background: var(--card-surface);
}

.tv-main .strategy-activity-tab,
.tv-main .strategy-activity-filter,
.tv-main .strategy-runtime-params-trigger,
.tv-main .strategy-params-dialog-close {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    border-radius: 999px;
    border: 1px solid var(--card-border);
    background: color-mix(in srgb, var(--tv-bg-surface) 72%, transparent);
    color: var(--card-text-2);
    cursor: pointer;
    transition: border-color 140ms ease, background-color 140ms ease, color 140ms ease, transform 140ms ease;
}

.tv-main .strategy-activity-tab,
.tv-main .strategy-activity-filter {
    padding: 0.5rem 0.85rem;
    font-size: 0.76rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
}

.tv-main .strategy-runtime-params-trigger,
.tv-main .strategy-params-dialog-close {
    padding: 0.55rem 0.95rem;
    font-size: 0.8rem;
    font-weight: 700;
}

.tv-main .strategy-runtime-params-trigger {
    font-family: ui-monospace, SFMono-Regular, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}

.tv-main .strategy-activity-tab:hover,
.tv-main .strategy-activity-filter:hover,
.tv-main .strategy-runtime-params-trigger:hover,
.tv-main .strategy-params-dialog-close:hover {
    border-color: var(--card-border);
    background: var(--card-surface-raised);
    color: var(--card-text-1);
    transform: translateY(-1px);
}

.tv-main .strategy-activity-tab.is-active {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 82%, var(--card-surface));
    color: var(--card-active-text);
}

.tv-main .strategy-activity-filter.is-active {
    color: var(--card-text-1);
}

.tv-main .strategy-activity-filter--error.is-active {
    border-color: var(--card-red-border);
    background: var(--card-red-surface);
    color: var(--card-red-text);
}

.tv-main .strategy-activity-filter--warning.is-active {
    border-color: var(--card-amber-border);
    background: var(--card-amber-surface);
    color: var(--card-amber-text);
}

.tv-main .strategy-activity-filter--info.is-active {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 80%, var(--card-surface));
    color: var(--card-active-text);
}

.tv-main .strategy-activity-tab-count,
.tv-main .strategy-activity-filter-count {
    border-radius: 999px;
    padding: 0.15rem 0.45rem;
    background: color-mix(in srgb, var(--tv-text) 7%, transparent);
    font-size: 0.72rem;
    line-height: 1;
}

.tv-main .strategy-activity-viewport {
    display: grid;
    gap: 0.75rem;
    max-height: 24rem;
    overflow-y: auto;
    padding-right: 0.35rem;
}

.tv-main .strategy-activity-entry {
    border-radius: 1.5rem;
    border: 1px solid var(--card-border);
    background: var(--card-surface-raised);
    padding: 1rem;
}

.tv-main .strategy-activity-entry--error {
    border-color: var(--card-red-border);
    background: var(--card-red-surface);
}

.tv-main .strategy-activity-entry--warning {
    border-color: var(--card-amber-border);
    background: var(--card-amber-surface);
}

.tv-main .strategy-activity-level-badge,
.tv-main .strategy-activity-kind-badge {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 0.22rem 0.65rem;
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
}

.tv-main .strategy-activity-level-badge--error {
    border: 1px solid var(--card-red-border);
    background: color-mix(in srgb, var(--card-red-surface) 88%, transparent);
    color: var(--card-red-text);
}

.tv-main .strategy-activity-level-badge--warning {
    border: 1px solid var(--card-amber-border);
    background: color-mix(in srgb, var(--card-amber-surface) 88%, transparent);
    color: var(--card-amber-text);
}

.tv-main .strategy-activity-level-badge--info {
    border: 1px solid var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 85%, transparent);
    color: var(--card-active-text);
}

.tv-main .strategy-activity-kind-badge {
    border: 1px solid var(--card-border);
    background: color-mix(in srgb, var(--tv-text) 5%, transparent);
    color: var(--card-text-2);
}

.tv-main .strategy-activity-time {
    color: var(--card-text-3);
    font-size: 0.76rem;
}

.tv-main .strategy-activity-entry .text-slate-900,
.tv-main .strategy-activity-entry .text-slate-700 {
    color: var(--card-text-1);
}

.tv-main .strategy-activity-empty {
    border-radius: 1.5rem;
    border: 1px dashed var(--card-border);
    background: color-mix(in srgb, var(--card-surface-raised) 76%, transparent);
    padding: 1.25rem 1rem;
    color: var(--card-text-2);
    font-size: 0.92rem;
}

.tv-main .strategy-params-dialog {
    border-color: var(--card-border);
    background: var(--card-surface);
}

.tv-main .strategy-params-json {
    max-height: 28rem;
    overflow: auto;
    white-space: pre-wrap;
    word-break: break-word;
    color: var(--card-text-1);
    font-size: 0.78rem;
    line-height: 1.7;
}

.tv-main .strategy-instance-dialog {
    max-height: calc(100vh - 2rem);
    overflow-y: auto;
    overflow-x: hidden;
    border-color: var(--card-border);
    background: var(--card-surface);
    color: var(--card-text-1);
}

.tv-main .strategy-instance-dialog .text-slate-900,
.tv-main .strategy-instance-dialog .text-slate-800,
.tv-main .strategy-instance-dialog .text-slate-700 {
    color: var(--card-text-1);
}

.tv-main .strategy-instance-dialog .text-slate-600,
.tv-main .strategy-instance-dialog .text-slate-500 {
    color: var(--card-text-2);
}

.tv-main .strategy-instance-dialog .text-slate-400 {
    color: var(--card-text-3);
}

.tv-main .strategy-instance-dialog .bg-white,
.tv-main .strategy-instance-dialog .bg-slate-50 {
    background: var(--card-surface-raised);
}

.tv-main .strategy-instance-dialog .border-slate-200,
.tv-main .strategy-instance-dialog .border-slate-300 {
    border-color: var(--card-border);
}

.tv-main .strategy-instance-dialog .bg-amber-50 {
    background: var(--card-amber-surface);
}

.tv-main .strategy-instance-dialog .border-amber-200 {
    border-color: var(--card-amber-border);
}

.tv-main .strategy-instance-dialog .text-amber-700,
.tv-main .strategy-instance-dialog .text-amber-800 {
    color: var(--card-amber-text);
}

.tv-main .strategy-instance-dialog .bg-red-50 {
    background: var(--card-red-surface);
}

.tv-main .strategy-instance-dialog .border-red-200 {
    border-color: var(--card-red-border);
}

.tv-main .strategy-instance-dialog .text-red-700,
.tv-main .strategy-instance-dialog .text-red-800 {
    color: var(--card-red-text);
}

.tv-main .strategy-instance-dialog .bg-emerald-50 {
    background: var(--card-teal-surface);
}

.tv-main .strategy-instance-dialog .border-emerald-200 {
    border-color: var(--card-teal-border);
}

.tv-main .strategy-instance-dialog .text-emerald-700,
.tv-main .strategy-instance-dialog .text-emerald-800 {
    color: var(--card-teal-text);
}

.tv-main .strategy-instance-dialog .bg-sky-50 {
    background: color-mix(in srgb, var(--card-active-surface) 88%, transparent);
}

.tv-main .strategy-instance-dialog .border-sky-200 {
    border-color: var(--card-active-border);
}

.tv-main .strategy-instance-dialog .text-sky-700,
.tv-main .strategy-instance-dialog .text-sky-800 {
    color: var(--card-active-text);
}

.tv-main .strategy-account-picker__menu {
    position: static;
    top: auto;
    left: auto;
    right: auto;
    z-index: auto;
    margin-top: 0.45rem;
    border-color: var(--card-border);
    background: var(--card-surface);
    box-shadow: 0 18px 40px rgb(15 23 42 / 0.14);
}

.tv-main .strategy-account-picker__search {
    border-color: var(--card-border);
    background: var(--card-surface-raised);
    color: var(--card-text-1);
}

.tv-main .strategy-account-picker__search:focus {
    border-color: color-mix(in srgb, var(--tv-accent) 72%, var(--card-border));
    background: var(--card-surface);
}

.tv-main .strategy-account-picker__option {
    background: var(--card-surface-raised);
    border-color: transparent;
}

.tv-main .strategy-account-picker__option:hover {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 72%, var(--card-surface));
}

.tv-main .strategy-account-picker__option.is-active {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 84%, var(--card-surface));
}

.tv-main .strategy-account-picker__label,
.tv-main .strategy-account-picker__option-title,
.tv-main .strategy-account-picker__option-header {
    color: var(--card-text-1);
}

.tv-main .strategy-account-picker__meta,
.tv-main .strategy-account-picker__action,
.tv-main .strategy-account-picker__option-meta,
.tv-main .strategy-account-picker__empty {
    color: var(--card-text-2);
}

.tv-main .strategy-account-picker__tag--current {
    border-color: var(--card-teal-border);
    background: color-mix(in srgb, var(--card-teal-surface) 86%, transparent);
    color: var(--card-teal-text);
}

.tv-main .strategy-account-picker__empty {
    border-color: var(--card-border);
    background: color-mix(in srgb, var(--card-surface-raised) 88%, transparent);
}

.tv-main .strategy-tag-input {
    border-color: var(--card-border);
    background: var(--card-surface);
}

.tv-main .strategy-tag-input:focus-within {
    border-color: color-mix(in srgb, var(--tv-accent) 70%, var(--card-border));
}

.tv-main .strategy-tag-input--invalid {
    border-color: var(--card-amber-border);
    background: var(--card-amber-surface);
}

.tv-main .strategy-tag-input--invalid:focus-within {
    border-color: color-mix(in srgb, var(--card-amber-text) 70%, var(--card-amber-border));
}

.tv-main .strategy-tag-chip {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 88%, var(--card-surface));
    color: var(--card-active-text);
}

.tv-main .strategy-tag-chip__remove {
    color: var(--card-text-2);
}

.tv-main .strategy-tag-input__field {
    color: var(--card-text-1);
}

.tv-main .strategy-tag-input__field::placeholder {
    color: var(--card-text-3);
}

.strategy-instance-dialog {
    border-radius: 1.75rem;
    border: 1px solid rgb(226 232 240);
    background: white;
    padding: 1.25rem;
    box-shadow: 0 24px 90px rgb(15 23 42 / 0.2);
}

.strategy-tag-input {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    min-height: 3rem;
    padding: 0.6rem 0.75rem;
    border-radius: 1rem;
    border: 1px solid rgb(203 213 225);
    background: white;
    transition: border-color 140ms ease;
}

.strategy-tag-input:focus-within {
    border-color: rgb(100 116 139);
}

.strategy-tag-input--invalid {
    border-color: rgb(245 158 11);
    background: rgb(255 251 235);
}

.strategy-tag-input--invalid:focus-within {
    border-color: rgb(217 119 6);
}

.strategy-tag-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    max-width: 100%;
    padding: 0.35rem 0.7rem;
    border-radius: 999px;
    border: 1px solid rgb(191 219 254);
    background: rgb(239 246 255);
    color: rgb(30 64 175);
    font-size: 0.76rem;
    font-weight: 600;
    line-height: 1;
}

.strategy-tag-chip__remove {
    color: rgb(71 85 105);
    font-size: 0.72rem;
    text-transform: uppercase;
}

.strategy-tag-input__field {
    flex: 1 1 10rem;
    min-width: 10rem;
    border: 0;
    outline: none;
    background: transparent;
    color: rgb(15 23 42);
    font-size: 0.875rem;
    padding: 0.1rem 0;
}

.strategy-tag-input__field::placeholder {
    color: rgb(148 163 184);
}

.strategy-account-picker {
    position: relative;
}

.strategy-account-picker__trigger {
    display: flex;
    width: 100%;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    border-radius: 1rem;
    border: 1px solid rgb(203 213 225);
    background: white;
    padding: 0.75rem 0.85rem;
    text-align: left;
    transition: border-color 140ms ease, box-shadow 140ms ease;
}

.strategy-account-picker__trigger:hover {
    border-color: rgb(148 163 184);
}

.strategy-account-picker__trigger:focus-visible {
    outline: none;
    border-color: rgb(100 116 139);
    box-shadow: 0 0 0 3px rgb(226 232 240 / 0.9);
}

.strategy-account-picker__copy {
    display: grid;
    min-width: 0;
    gap: 0.2rem;
}

.strategy-account-picker__label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: rgb(15 23 42);
    font-size: 0.875rem;
    font-weight: 600;
}

.strategy-account-picker__meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem;
    color: rgb(100 116 139);
    font-size: 0.74rem;
    line-height: 1.3;
}

.strategy-account-picker__action {
    flex-shrink: 0;
    color: rgb(71 85 105);
    font-size: 0.74rem;
    font-weight: 600;
}

.strategy-account-picker__menu {
    z-index: 20;
    display: grid;
    gap: 0.65rem;
    border-radius: 1.1rem;
    border: 1px solid rgb(226 232 240);
    background: white;
    padding: 0.8rem;
    box-shadow: 0 18px 40px rgb(15 23 42 / 0.14);
}

.strategy-account-picker__search {
    width: 100%;
    border-radius: 0.9rem;
    border: 1px solid rgb(203 213 225);
    background: rgb(248 250 252);
    padding: 0.7rem 0.8rem;
    color: rgb(15 23 42);
    font-size: 0.875rem;
    outline: none;
}

.strategy-account-picker__search:focus {
    border-color: rgb(100 116 139);
    background: white;
}

.strategy-account-picker__options {
    display: grid;
    gap: 0.45rem;
    max-height: 16rem;
    overflow-y: auto;
}

.strategy-account-picker__option {
    display: grid;
    gap: 0.25rem;
    width: 100%;
    border-radius: 0.95rem;
    border: 1px solid transparent;
    background: rgb(248 250 252);
    padding: 0.7rem 0.8rem;
    text-align: left;
    transition: border-color 140ms ease, background-color 140ms ease;
}

.strategy-account-picker__option:hover {
    border-color: rgb(191 219 254);
    background: rgb(239 246 255);
}

.strategy-account-picker__option.is-active {
    border-color: rgb(59 130 246);
    background: rgb(239 246 255);
}

.strategy-account-picker__option-header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
}

.strategy-account-picker__option-title {
    color: rgb(15 23 42);
    font-size: 0.84rem;
    font-weight: 600;
}

.strategy-account-picker__option-meta {
    color: rgb(100 116 139);
    font-size: 0.72rem;
    line-height: 1.35;
}

.strategy-account-picker__tag {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 0.15rem 0.5rem;
    font-size: 0.64rem;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
}

.strategy-account-picker__tag--current {
    border: 1px solid rgb(167 243 208);
    background: rgb(236 253 245);
    color: rgb(4 120 87);
}

.strategy-account-picker__empty {
    border-radius: 0.95rem;
    border: 1px dashed rgb(203 213 225);
    padding: 0.9rem 0.8rem;
    color: rgb(100 116 139);
    font-size: 0.78rem;
}
</style>
