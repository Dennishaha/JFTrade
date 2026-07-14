import { computed, ref, watch, type Ref } from "vue";

import type {
    InstrumentResolutionCandidate,
    NormalizeInstrumentRequest,
    NormalizeInstrumentResponse,
    StrategyBindingInstrumentDocument,
    StrategyDefinitionDocument,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
    StrategyRuntimeRiskSettings,
} from "@/contracts";

import type { BrokerAccountSelectionOption } from "../../composables/consoleDataBrokerAccountSelection";
import {
    bindingInstrumentsToSymbols,
    brokerAccountOptionSubtitle,
    defaultStrategyRuntimeRiskSettings,
    filterBrokerAccountOptions,
    normalizeBindingInstruments,
    normalizeStrategyRuntimeRiskSettings,
    normalizeText,
    resolveBrokerAccountOption,
    resolveBrokerAccountSelectionKey,
    splitSymbolsText,
} from "./strategyRuntimeInstanceBinding";

export type StrategySymbolEditorMode = "create" | "edit";

const QUALIFIED_INSTRUMENT_MARKETS = new Set([
    "HK",
    "US",
    "SH",
    "SZ",
    "CNSH",
    "CNSZ",
    "SG",
    "JP",
    "AU",
    "MY",
    "CA",
    "FX",
    "CRYPTO",
    "HK_FUTURE",
]);
const SELECTABLE_INSTRUMENT_MARKETS = new Set(["HK", "US", "SH", "SZ"]);

function hasQualifiedInstrumentMarketPrefix(value: string): boolean {
    const normalized = value.trim().toUpperCase().replace(":", ".");
    const separator = normalized.indexOf(".");
    return separator > 0 && QUALIFIED_INSTRUMENT_MARKETS.has(normalized.slice(0, separator));
}

interface StrategyRuntimeInstanceEditorOptions {
    strategyDefinitions: Ref<StrategyDefinitionDocument[]>;
    selectedStrategy: Ref<StrategyInstanceItem | null>;
    selectedStrategyBinding: Ref<StrategyInstanceBindingDocument | null>;
    brokerAccountOptions: Ref<BrokerAccountSelectionOption[]>;
    selectedBrokerAccount: Ref<BrokerAccountSelectionOption | null>;
    defaultBrokerAccountSelectionKey: Ref<string>;
    pendingDefinitionId: () => string | undefined;
    onPendingDefinitionSelected?: () => void;
    normalizeInstrumentRefWithMarketApi: (
        input: NormalizeInstrumentRequest,
    ) => Promise<NormalizeInstrumentResponse>;
}

export function useStrategyRuntimeInstanceEditor(options: StrategyRuntimeInstanceEditorOptions) {
    const instanceEditorMode = ref<StrategySymbolEditorMode | null>(null);
    const createDefinitionId = ref("");
    const createBindingInstruments = ref<StrategyBindingInstrumentDocument[]>([]);
    const createSymbolDraft = ref("");
    const createSymbolValidationMessage = ref("");
    const createInterval = ref("5m");
    const createExecutionMode = ref<StrategyExecutionMode>("live");
    const createRuntimeRisk = ref<StrategyRuntimeRiskSettings>(defaultStrategyRuntimeRiskSettings());
    const createBrokerAccountKey = ref("");
    const createBrokerAccountQuery = ref("");
    const editBindingInstruments = ref<StrategyBindingInstrumentDocument[]>([]);
    const editSymbolDraft = ref("");
    const editSymbolValidationMessage = ref("");
    const editInterval = ref("5m");
    const editExecutionMode = ref<StrategyExecutionMode>("live");
    const editRuntimeRisk = ref<StrategyRuntimeRiskSettings>(defaultStrategyRuntimeRiskSettings());
    const editBrokerAccountKey = ref("");
    const editBrokerAccountQuery = ref("");
    const activeBrokerAccountPicker = ref<StrategySymbolEditorMode | null>(null);
    const pendingSymbolDraftCommits = new Map<StrategySymbolEditorMode, Promise<boolean>>();

    const createDefinition = computed(
        () => options.strategyDefinitions.value.find((item) => item.id === createDefinitionId.value) ?? null,
    );
    const createSelectedBrokerAccountOption = computed(
        () => resolveBrokerAccountOption(options.brokerAccountOptions.value, createBrokerAccountKey.value),
    );
    const editSelectedBrokerAccountOption = computed(
        () => resolveBrokerAccountOption(options.brokerAccountOptions.value, editBrokerAccountKey.value),
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
    const activeSymbolTags = computed(() => symbolTagsFor(activeInstanceEditorMode.value));
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
        return `${options.selectedStrategy.value?.definition.name ?? "未选择"} / v${options.selectedStrategy.value?.definition.version ?? ""}`;
    });
    const createInstanceHint = computed(() => {
        if (options.strategyDefinitions.value.length === 0) {
            return "先回到设计区保存策略定义，再在这里绑定交易代码、周期和账号。";
        }
        return "实例负责绑定交易代码、周期、账号与执行模式；同一个策略定义可以生成多个实例。";
    });
    const instanceEditorTitle = computed(() =>
        instanceEditorMode.value === "edit" ? "实例绑定" : "新增实例",
    );
    const instanceEditorHint = computed(() => {
        if (instanceEditorMode.value === "edit") {
            if (options.selectedStrategy.value === null) {
                return "请选择策略实例后再调整绑定。";
            }
            return "当前选中的实例可以独立绑定多个交易代码、账号和执行模式。";
        }
        return createInstanceHint.value;
    });

    watch(
        () => options.pendingDefinitionId(),
        (definitionId) => {
            const normalizedDefinitionId = normalizeText(definitionId);
            if (normalizedDefinitionId !== "") {
                createDefinitionId.value = normalizedDefinitionId;
                instanceEditorMode.value = "create";
                options.onPendingDefinitionSelected?.();
            }
        },
        { immediate: true },
    );

    watch(
        options.selectedStrategy,
        (strategy) => {
            if (strategy === null && instanceEditorMode.value === "edit") {
                closeInstanceEditorDialog();
            }
        },
    );

    watch(
        options.strategyDefinitions,
        (definitions) => {
            if (definitions.length === 0) {
                createDefinitionId.value = "";
                return;
            }
            if (definitions.some((item) => item.id === createDefinitionId.value)) {
                return;
            }
            const pendingDefinition = definitions.find(
                (item) => item.id === normalizeText(options.pendingDefinitionId()),
            );
            createDefinitionId.value = pendingDefinition?.id ?? definitions[0]?.id ?? "";
        },
        { immediate: true },
    );

    watch(
        [options.brokerAccountOptions, options.defaultBrokerAccountSelectionKey],
        () => {
            const defaultSelectionKey = options.defaultBrokerAccountSelectionKey.value;
            if (
                (createBrokerAccountKey.value === ""
                    || !options.brokerAccountOptions.value.some(
                        (option) => option.selectionKey === createBrokerAccountKey.value,
                    ))
                && defaultSelectionKey !== ""
            ) {
                createBrokerAccountKey.value = defaultSelectionKey;
            }
            if (
                options.selectedStrategyBinding.value?.brokerAccount == null
                && (
                    editBrokerAccountKey.value === ""
                    || !options.brokerAccountOptions.value.some(
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
        options.selectedStrategyBinding,
        (binding) => {
            if (binding === null) {
                editBindingInstruments.value = [];
                editSymbolDraft.value = "";
                editSymbolValidationMessage.value = "";
                editInterval.value = "5m";
                editExecutionMode.value = "live";
                editRuntimeRisk.value = defaultStrategyRuntimeRiskSettings();
                editBrokerAccountKey.value = options.defaultBrokerAccountSelectionKey.value;
                return;
            }
            editBindingInstruments.value = normalizeBindingInstruments(binding.instruments ?? []);
            editSymbolDraft.value = "";
            editSymbolValidationMessage.value = "";
            editInterval.value = binding.interval;
            editExecutionMode.value = binding.executionMode;
            editRuntimeRisk.value = normalizeStrategyRuntimeRiskSettings(binding.runtimeRisk);
            editBrokerAccountKey.value =
                resolveBrokerAccountSelectionKey(options.brokerAccountOptions.value, binding.brokerAccount)
                || options.defaultBrokerAccountSelectionKey.value;
        },
        { immediate: true },
    );

    function bindingInstrumentsFor(mode: StrategySymbolEditorMode): StrategyBindingInstrumentDocument[] {
        return mode === "create" ? createBindingInstruments.value : editBindingInstruments.value;
    }

    function symbolTagsFor(mode: StrategySymbolEditorMode): string[] {
        return bindingInstrumentsToSymbols(bindingInstrumentsFor(mode));
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
            for (const segment of draftSegments) {
                const raw = normalizeText(segment);
                if (raw === "") {
                    continue;
                }
                if (!hasQualifiedInstrumentMarketPrefix(raw)) {
                    invalidSymbols.push(raw.toUpperCase());
                    continue;
                }
                try {
                    const normalized = await options.normalizeInstrumentRefWithMarketApi({ instrumentId: raw });
                    const market = normalized.prefix.trim().toUpperCase();
                    const code = normalized.code.trim().toUpperCase();
                    if (market === "" || code === "" || !SELECTABLE_INSTRUMENT_MARKETS.has(market)) {
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
            }
            if (invalidSymbols.length > 0) {
                setSymbolValidationMessage(
                    mode,
                    `已忽略无效交易代码：${invalidSymbols.join("、")}。批量输入请使用 US.TME、HK.00700 这类完整格式。`,
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
            ? filterBrokerAccountOptions(options.brokerAccountOptions.value, createBrokerAccountQuery.value)
            : filterBrokerAccountOptions(options.brokerAccountOptions.value, editBrokerAccountQuery.value);
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

    function acceptResolvedInstrument(
        mode: StrategySymbolEditorMode,
        candidate: InstrumentResolutionCandidate,
    ): void {
        if (!candidate.selectable) {
            setSymbolValidationMessage(mode, candidate.unavailableReason ?? "当前市场暂不支持交易代码绑定。");
            return;
        }
        const market = normalizeText(candidate.market).toUpperCase();
        const code = normalizeText(candidate.code || candidate.symbol).toUpperCase();
        if (market === "" || code === "") {
            setSymbolValidationMessage(mode, "解析结果缺少市场或代码，请重新选择标的。");
            return;
        }
        setBindingInstruments(mode, [
            ...bindingInstrumentsFor(mode),
            { market, code },
        ]);
        setSymbolValidationMessage(mode, "");
    }

    function acceptActiveResolvedInstrument(
        candidate: InstrumentResolutionCandidate,
    ): void {
        acceptResolvedInstrument(activeInstanceEditorMode.value, candidate);
    }

    function updateActiveSymbolDraft(value: string): void {
        setSymbolDraft(activeInstanceEditorMode.value, value);
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

    function updateActiveRuntimeRiskNumber(
        field: "maxOrderQuantity" | "maxOrderNotional" | "dailyMaxOrders",
        value: string,
    ): void {
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

    function openCreateInstanceForm(): void {
        instanceEditorMode.value = "create";
        createSymbolDraft.value = "";
        createSymbolValidationMessage.value = "";
        createRuntimeRisk.value = defaultStrategyRuntimeRiskSettings();
        closeBrokerAccountPicker();
    }

    function openEditInstanceForm(): void {
        if (options.selectedStrategy.value === null) {
            return;
        }
        instanceEditorMode.value = "edit";
        closeBrokerAccountPicker();
    }

    function closeInstanceEditorDialog(): void {
        const mode = instanceEditorMode.value;
        instanceEditorMode.value = null;
        if (mode === "create") {
            createSymbolValidationMessage.value = "";
        }
        if (mode === "edit") {
            editSymbolValidationMessage.value = "";
        }
        closeBrokerAccountPicker();
    }

    return {
        instanceEditorMode,
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
        commitSymbolDraft,
        acceptResolvedInstrument,
        removeActiveSymbol,
        acceptActiveResolvedInstrument,
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
        openCreateInstanceForm,
        openEditInstanceForm,
        closeInstanceEditorDialog,
    };
}
