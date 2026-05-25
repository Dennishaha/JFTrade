<script setup lang="ts">
import type {
    StrategyDefinitionDocument,
    StrategyVisualModelDocument,
    StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { onBeforeRouteLeave } from "vue-router";

import StrategyLogicFlowDesigner from "./StrategyLogicFlowDesigner.vue";
import StrategyStageCodeWorkbenchPanel from "./strategy-stage/StrategyStageCodeWorkbenchPanel.vue";
import StrategyStageDefinitionsPanel from "./strategy-stage/StrategyStageDefinitionsPanel.vue";
import StrategyStageOverlayDeck from "./strategy-stage/StrategyStageOverlayDeck.vue";
import "./strategy-stage/strategyStageShared.css";
import {
    fetchEnvelope,
    fetchEnvelopeWithInit,
} from "../composables/apiClient";
import { useDraggable } from "../composables/useDraggable";
import { useStrategyUndoRedo } from "../composables/useStrategyUndoRedo";
import { useStrategyVisualNodeInspector } from "../composables/useStrategyVisualNodeInspector";
import {
    strategyEditorCompletions,
    strategyEditorExtraLibs,
    strategyEditorHoverItems,
} from "../features/strategyEditorIntelliSense";
import {
    buildStrategyVisualModelFromScript,
    buildStrategyScriptFromVisualModel,
    cloneStrategyVisualModel,
    createDefaultStrategyVisualModel,
    getStrategyAuthoringTemplates,
    getStrategyBlockKind,
    type StrategyAuthoringTemplate,
} from "../features/strategyVisualBuilder";

const props = withDefaults(defineProps<{
    entryMode?: "existing" | "new";
    initialDefinitionsCollapsed?: boolean;
}>(), {
    entryMode: "existing",
    initialDefinitionsCollapsed: true,
});

const emit = defineEmits<{
    "switch-to-runtime": [payload?: { notice?: string }];
    "definitions-count-change": [count: number];
}>();

const fallbackTemplate: StrategyAuthoringTemplate = {
    id: "logic-flow-starter",
    label: "逻辑流起步骨架",
    description: "给一份可视化起步图，适合从拖拽块开始搭策略。",
    mode: "visual",
    defaultId: "js-logic-flow-starter",
    defaultName: "逻辑流起步骨架",
    defaultVersion: "0.1.0",
    defaultDescription: "用流程图拖拽块，快速搭一个可保存的 QuickJS 策略骨架。",
    defaultSymbol: "00700",
    defaultInterval: "1m",
    visualModel: createDefaultStrategyVisualModel(),
    syncVisualToCode: true,
    buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createDefaultStrategyVisualModel(), context),
};

const strategyTemplates = getStrategyAuthoringTemplates();
const defaultStrategyTemplate = strategyTemplates[0] ?? fallbackTemplate;
const defaultStrategyTemplateId = defaultStrategyTemplate.id;

const strategyDefinitions = ref<StrategyDefinitionDocument[]>([]);
const selectedDefinitionId = ref("");
const selectedStrategyTemplateId = ref(defaultStrategyTemplateId);
const isLoadingDefinitions = ref(false);
const isSavingDefinition = ref(false);
const isInstantiatingDefinition = ref(false);
const definitionError = ref("");
const definitionNotice = ref("");
const visualSyncStatus = ref<"ready" | "syncing" | "synced" | "partial" | "error">("ready");
const visualSyncMessage = ref("图形与代码会自动异步同步。\n修改代码后会尝试反解回流程图。\n无法标准化的语句会保留为代码块。");
const visualSyncCodeBlockCount = ref(0);

const isDefinitionsPanelCollapsed = ref(props.initialDefinitionsCollapsed);
const isTemplatesSectionCollapsed = ref(true);
const isBasicInfoSectionCollapsed = ref(true);
const isVisualBuilderSectionCollapsed = ref(false);
const isCodeEditorSectionCollapsed = ref(true);
const isMetadataSectionCollapsed = ref(true);
const isBlockInspectorCollapsed = ref(true);
const strategyDisplayMode = ref<"canvas" | "split" | "code">("canvas");

const definitionForm = ref<StrategyDefinitionDocument>(
    createDefinitionFromTemplate(defaultStrategyTemplateId),
);
const selectedVisualNodeId = ref("");
const codeContextNodeId = ref("");
const lastCommittedDefinitionSignature = ref("");
const isTemplatePickerEntry = ref(false);

const definitionsPanelDrag = useDraggable();
const codePanelDrag = useDraggable();

// ── Undo / Redo ──
const undoRedo = useStrategyUndoRedo({ maxDepth: 80 });
let skipNextHistorySnapshot = false;

function pushDefinitionHistory(): void {
    if (skipNextHistorySnapshot) {
        skipNextHistorySnapshot = false;
        return;
    }

    const snapshot = serializeDefinitionSnapshot(definitionForm.value);
    if (snapshot === "") {
        return;
    }

    undoRedo.pushSnapshot(snapshot);
}

function restoreFromSnapshot(snapshot: string): void {
    try {
        const parsed = JSON.parse(snapshot) as StrategyDefinitionDocument;
        const model = cloneStrategyVisualModel(parsed.visualModel);

        skipNextHistorySnapshot = true;
        markNextScriptSyncAsInternal();

        definitionForm.value = {
            ...parsed,
            visualModel: model,
        };

        selectedVisualNodeId.value = pickInitialVisualNodeId(
            model ?? createDefaultStrategyVisualModel(),
        );
    } catch {
        // Ignore corrupt snapshots.
    }
}

function handleUndo(): void {
    const snapshot = undoRedo.undo();
    if (snapshot === null) {
        return;
    }

    restoreFromSnapshot(snapshot);
    definitionNotice.value = "已撤销至上一步改动。";
}

function handleRedo(): void {
    const snapshot = undoRedo.redo();
    if (snapshot === null) {
        return;
    }

    restoreFromSnapshot(snapshot);
    definitionNotice.value = "已重做至下一步改动。";
}

function handleDesignKeydown(event: KeyboardEvent): void {
    const isMod = event.metaKey || event.ctrlKey;
    if (!isMod) {
        return;
    }

    if (event.key === "z" && !event.shiftKey) {
        event.preventDefault();
        handleUndo();
        return;
    }

    if (event.key === "z" && event.shiftKey) {
        event.preventDefault();
        handleRedo();
        return;
    }

    // Also support Ctrl+Y (Windows-style redo)
    if (event.key === "y") {
        event.preventDefault();
        handleRedo();
        return;
    }
}

const SCRIPT_TO_VISUAL_SYNC_DELAY = 650;

let scriptToVisualSyncTimer: ReturnType<typeof setTimeout> | null = null;
let skipScriptToVisualSyncCount = 0;
let isCodeOriginatedSelection = false;

const selectedDefinition = computed(
    () =>
        strategyDefinitions.value.find(
            (item) => item.id === selectedDefinitionId.value,
        ) ?? null,
);

const activeStrategyTemplate = computed(
    () =>
        strategyTemplates.find(
            (item) => item.id === selectedStrategyTemplateId.value,
        ) ?? null,
);

const resolvedVisualModel = computed(
    () => definitionForm.value.visualModel ?? createDefaultStrategyVisualModel(),
);

const selectedVisualNode = computed<StrategyVisualNodeDocument | null>(() => {
    const nodeId = selectedVisualNodeId.value;
    if (nodeId === "") {
        return null;
    }
    return (
        resolvedVisualModel.value.nodes.find((node) => node.id === nodeId) ?? null
    );
});

const {
    selectedVisualKind,
    selectedVisualBlock,
    selectedVisualNodeText,
    selectedVisualNodeMessage,
    showsCodeInput,
    selectedVisualNodeCode,
    showsThresholdInput,
    selectedVisualNodeThreshold,
    showsPeriodInput,
    selectedVisualNodePeriod,
    showsMacdInputs,
    showsTechnicalIndicatorMacdInputs,
    selectedMacdFastPeriod,
    selectedMacdSlowPeriod,
    selectedMacdSignalPeriod,
    showsMultiplierInput,
    selectedBollingerMultiplier,
    showsConditionModeInput,
    showsIndicatorTypeInput,
    showsPatternTypeInput,
    showsLookbackInput,
    selectedIndicatorType,
    selectedIndicatorConditionMode,
    selectedIndicatorOperator,
    selectedIndicatorPatternType,
    selectedIndicatorLookback,
    showsPlaceOrderInputs,
    selectedPlaceOrderSide,
    selectedPlaceOrderType,
    selectedPlaceOrderQuantityMode,
    selectedPlaceOrderQuantityValue,
    selectedPlaceOrderLimitPrice,
    showsPlaceOrderLimitPriceInput,
} = useStrategyVisualNodeInspector({
    selectedVisualNode,
    mutateSelectedVisualNode,
});

const overlayDeckBindings = {
    definitionForm,
    selectedVisualNodeText,
    selectedVisualNodeMessage,
    selectedVisualNodeCode,
    selectedVisualNodePeriod,
    selectedIndicatorType,
    selectedIndicatorConditionMode,
    selectedIndicatorOperator,
    selectedIndicatorPatternType,
    selectedIndicatorLookback,
    selectedMacdFastPeriod,
    selectedMacdSlowPeriod,
    selectedMacdSignalPeriod,
    selectedBollingerMultiplier,
    selectedVisualNodeThreshold,
    selectedPlaceOrderSide,
    selectedPlaceOrderType,
    selectedPlaceOrderQuantityMode,
    selectedPlaceOrderQuantityValue,
    selectedPlaceOrderLimitPrice,
} as const;

const codeWorkbenchBindings = {
    definitionForm,
} as const;

const codeWorkbenchRef = ref<InstanceType<typeof StrategyStageCodeWorkbenchPanel> | null>(null);
const logicFlowDesignerRef = ref<InstanceType<typeof StrategyLogicFlowDesigner> | null>(null);

const hasDesignOverlayDeck = computed(
    () =>
        !isTemplatesSectionCollapsed.value ||
        !isBasicInfoSectionCollapsed.value ||
        !isMetadataSectionCollapsed.value ||
        (!isBlockInspectorCollapsed.value && selectedVisualNode.value !== null),
);

const showsCanvas = computed(() => strategyDisplayMode.value !== "code");

const showsCodeWorkbench = computed(
    () => !isCodeEditorSectionCollapsed.value && strategyDisplayMode.value !== "canvas",
);

const logicFlowFitViewPadding = computed(() => ({
    top: hasDesignOverlayDeck.value ? 320 : 156,
    right: showsCodeWorkbench.value ? 540 : 36,
    bottom: !isVisualBuilderSectionCollapsed.value && showsCanvas.value ? 132 : 36,
    left: !isDefinitionsPanelCollapsed.value ? 320 : 36,
}));

const strategyLogicFlowZoomRightOffset = computed(() => "1rem");

const definitionsPanelStyle = computed(() => ({
    transform: `translate(${definitionsPanelDrag.drag.x}px, ${definitionsPanelDrag.drag.y}px)`,
    zIndex: definitionsPanelDrag.drag.dragging ? "55" : "25",
}));

const codePanelStyle = computed(() => ({
    transform: `translate(${codePanelDrag.drag.x}px, ${codePanelDrag.drag.y}px)`,
    zIndex:
        codePanelDrag.drag.dragging
            ? "56"
            : strategyDisplayMode.value === "code"
                ? "22"
                : "26",
}));

const hasUnsavedDefinitionChanges = computed(() => {
    if (isTemplatePickerEntry.value) {
        return false;
    }

    return (
        serializeDefinitionSnapshot(definitionForm.value) !==
        lastCommittedDefinitionSignature.value
    );
});

const visualSyncToneClass = computed(() => ({
    "strategy-stage__toolbar-status--syncing": visualSyncStatus.value === "syncing",
    "strategy-stage__toolbar-status--synced": visualSyncStatus.value === "synced",
    "strategy-stage__toolbar-status--partial": visualSyncStatus.value === "partial",
    "strategy-stage__toolbar-status--error": visualSyncStatus.value === "error",
}));

const visualSyncLabel = computed(() => {
    switch (visualSyncStatus.value) {
        case "syncing":
            return "同步中";
        case "synced":
            return "已同步";
        case "partial":
            return "代码块兜底";
        case "error":
            return "同步提示";
        default:
            return "自动同步";
    }
});

const visualNodeMappingQuality = computed(() => {
    const nodes = resolvedVisualModel.value.nodes;
    const total = nodes.length;
    if (total === 0) {
        return null;
    }

    let mappableCount = 0;
    for (const node of nodes) {
        const sourceRange = node.properties.sourceRange;
        if (
            sourceRange !== undefined &&
            sourceRange !== null &&
            typeof sourceRange === "object" &&
            typeof Reflect.get(sourceRange, "start") === "number" &&
            typeof Reflect.get(sourceRange, "end") === "number"
        ) {
            mappableCount += 1;
        }
    }

    return { mappableCount, total };
});

const codeContextNodeLabel = computed(() => {
    const nodeId = codeContextNodeId.value;
    if (nodeId === "") {
        return "";
    }
    const node = resolvedVisualModel.value.nodes.find((n) => n.id === nodeId);
    return node?.text ?? "";
});

watch(
    () => strategyDefinitions.value.length,
    (count) => {
        emit("definitions-count-change", count);
    },
    { immediate: true },
);

watch(
    () => definitionForm.value.script,
    (nextScript, previousScript) => {
        if (nextScript === previousScript) {
            return;
        }

        if (skipScriptToVisualSyncCount > 0) {
            skipScriptToVisualSyncCount -= 1;
            return;
        }

        scheduleScriptToVisualModelSync();
    },
);

// Push undo snapshots whenever the definition changes meaningfully.
watch(
    () => serializeDefinitionSnapshot(definitionForm.value),
    (nextSnapshot, previousSnapshot) => {
        if (nextSnapshot === previousSnapshot) {
            return;
        }

        if (nextSnapshot === lastCommittedDefinitionSignature.value) {
            return;
        }

        pushDefinitionHistory();
    },
    { flush: "post" },
);

onMounted(() => {
    if (typeof window !== "undefined") {
        window.addEventListener("beforeunload", handleBeforeUnload);
        window.addEventListener("keydown", handleDesignKeydown);
    }

    void loadStrategyDefinitions(selectedDefinitionId.value, {
        openTemplatePicker: props.entryMode === "new",
    });
});

onBeforeUnmount(() => {
    if (typeof window !== "undefined") {
        window.removeEventListener("beforeunload", handleBeforeUnload);
        window.removeEventListener("keydown", handleDesignKeydown);
    }

    clearPendingScriptToVisualSync();
});

onBeforeRouteLeave(async () => await confirmStrategyDesignExit());

function startDefinitionsPanelDrag(event: MouseEvent): void {
    definitionsPanelDrag.startDrag(event);
}

function startCodePanelDrag(event: MouseEvent): void {
    codePanelDrag.startDrag(event);
}

function setStrategyDisplayMode(mode: "canvas" | "split" | "code"): void {
    strategyDisplayMode.value = mode;
    if (mode !== "canvas") {
        isCodeEditorSectionCollapsed.value = false;
    }
}

async function handleSwitchToRuntime(
    payload?: { notice?: string },
): Promise<void> {
    if (!(await confirmStrategyDesignExit())) {
        return;
    }

    emit("switch-to-runtime", payload);
}

function getStrategyTemplate(templateId: string): StrategyAuthoringTemplate {
    return (
        strategyTemplates.find((item) => item.id === templateId) ??
        defaultStrategyTemplate
    );
}

function openNewDefinitionTemplatePicker(
    templateId = defaultStrategyTemplateId,
): void {
    clearDefinitionMessages();
    undoRedo.clear();
    isTemplatePickerEntry.value = true;
    lastCommittedDefinitionSignature.value = "";
    selectedDefinitionId.value = "";
    selectedStrategyTemplateId.value = templateId;
    replaceDefinitionForm(createDefinitionFromTemplate(templateId), {
        skipScriptParse: true,
    });
    selectedVisualNodeId.value = pickInitialVisualNodeId(resolvedVisualModel.value);
    isDefinitionsPanelCollapsed.value = true;
    isTemplatesSectionCollapsed.value = false;
    isBasicInfoSectionCollapsed.value = true;
    isMetadataSectionCollapsed.value = true;
    isBlockInspectorCollapsed.value = true;
    isCodeEditorSectionCollapsed.value = true;
    isVisualBuilderSectionCollapsed.value = false;
    strategyDisplayMode.value = "canvas";
    resetVisualSyncStatus();
}

function createDefinitionFromTemplate(
    templateId: string,
): StrategyDefinitionDocument {
    const template = getStrategyTemplate(templateId);
    const symbol = normalizeSymbol(template.defaultSymbol);
    const interval = normalizeInterval(template.defaultInterval);
    const visualModel =
        cloneStrategyVisualModel(template.visualModel) ??
        createDefaultStrategyVisualModel();

    return {
        id: template.defaultId,
        name: template.defaultName,
        version: template.defaultVersion,
        description: template.defaultDescription,
        runtime: "quickjs-js",
        symbol,
        interval,
        script: template.buildScript({
            name: template.defaultName,
            symbol,
            interval,
        }),
        visualModel,
        createdAt: "",
        updatedAt: "",
    };
}

function normalizeSymbol(value: string): string {
    return value.trim().toUpperCase();
}

function normalizeInterval(value: string): string {
    return value.trim() === "" ? "1m" : value.trim();
}

function normalizeDefinition(
    definition: StrategyDefinitionDocument,
): StrategyDefinitionDocument {
    const visualModel =
        cloneStrategyVisualModel(definition.visualModel) ??
        createDefaultStrategyVisualModel();

    return {
        ...definition,
        runtime: definition.runtime.trim() === "" ? "quickjs-js" : definition.runtime,
        symbol: normalizeSymbol(definition.symbol),
        interval: normalizeInterval(definition.interval),
        visualModel,
    };
}

function serializeDefinitionSnapshot(
    definition: StrategyDefinitionDocument,
): string {
    return JSON.stringify({
        id: definition.id.trim(),
        name: definition.name.trim(),
        version: definition.version.trim(),
        description: definition.description.trim(),
        runtime: definition.runtime.trim() || "quickjs-js",
        symbol: normalizeSymbol(definition.symbol),
        interval: normalizeInterval(definition.interval),
        script: definition.script,
        visualModel:
            cloneStrategyVisualModel(definition.visualModel) ??
            createDefaultStrategyVisualModel(),
    });
}

function pickInitialVisualNodeId(model: StrategyVisualModelDocument): string {
    const preferred = model.nodes.find((node) => {
        const kind = getStrategyBlockKind(node);
        return kind !== "onInit" && kind !== "onKLineClosed";
    });

    return preferred?.id ?? model.nodes[0]?.id ?? "";
}

function formatTimestamp(value: string | undefined | null): string {
    if ((value ?? "").trim() === "") {
        return "暂无";
    }
    return value!.replace("T", " ").replace(".000Z", "Z");
}

function updateCommittedDefinitionSignature(
    definition: StrategyDefinitionDocument,
): void {
    lastCommittedDefinitionSignature.value = serializeDefinitionSnapshot(definition);
}

function clearDefinitionMessages(): void {
    definitionError.value = "";
    definitionNotice.value = "";
}

function setVisualSyncState(
    status: "ready" | "syncing" | "synced" | "partial" | "error",
    message: string,
    codeBlockCount = 0,
): void {
    visualSyncStatus.value = status;
    visualSyncMessage.value = message;
    visualSyncCodeBlockCount.value = codeBlockCount;
}

function resetVisualSyncStatus(): void {
    setVisualSyncState(
        "ready",
        "图形与代码会自动同步。修改代码后会尝试反解回流程图，无法标准化的语句会保留为代码块。",
        0,
    );
}

function clearPendingScriptToVisualSync(): void {
    if (scriptToVisualSyncTimer !== null) {
        clearTimeout(scriptToVisualSyncTimer);
        scriptToVisualSyncTimer = null;
    }
}

function markNextScriptSyncAsInternal(): void {
    skipScriptToVisualSyncCount += 1;
    clearPendingScriptToVisualSync();
}

function replaceDefinitionForm(
    nextDefinition: StrategyDefinitionDocument,
    options?: {
        skipScriptParse?: boolean;
    },
): void {
    if (
        options?.skipScriptParse === true &&
        nextDefinition.script !== definitionForm.value.script
    ) {
        markNextScriptSyncAsInternal();
    }

    definitionForm.value = nextDefinition;
}

function scheduleScriptToVisualModelSync(
    delay = SCRIPT_TO_VISUAL_SYNC_DELAY,
): void {
    clearPendingScriptToVisualSync();
    setVisualSyncState("syncing", "正在把代码异步转换回流程图…", visualSyncCodeBlockCount.value);

    scriptToVisualSyncTimer = setTimeout(() => {
        scriptToVisualSyncTimer = null;
        syncScriptToVisualModelNow();
    }, delay);
}

function syncScriptToVisualModelNow(options?: { updateCommittedSignature?: boolean }): void {
    clearPendingScriptToVisualSync();

    const script = definitionForm.value.script;
    if (script.trim() === "") {
        setVisualSyncState("error", "代码为空，无法转换回流程图。已保留当前流程图。", 0);
        return;
    }

    setVisualSyncState("syncing", "正在把代码异步转换回流程图…", visualSyncCodeBlockCount.value);

    const parseResult = buildStrategyVisualModelFromScript(
        script,
        definitionForm.value.visualModel,
    );
    if (!parseResult.ok) {
        setVisualSyncState("error", `${parseResult.error} 已保留当前流程图。`, 0);
        return;
    }

    const nextVisualModel =
        cloneStrategyVisualModel(parseResult.model) ??
        createDefaultStrategyVisualModel();

    replaceDefinitionForm(
        {
            ...definitionForm.value,
            visualModel: nextVisualModel,
        },
        {
            skipScriptParse: false,
        },
    );
    selectedVisualNodeId.value = pickInitialVisualNodeId(nextVisualModel);

    if (options?.updateCommittedSignature === true) {
        updateCommittedDefinitionSignature(definitionForm.value);
    }

    if (parseResult.codeBlockCount > 0) {
        setVisualSyncState(
            "partial",
            `代码已同步回流程图，其中 ${parseResult.codeBlockCount} 段保留为代码块，后续仍可继续混编。`,
            parseResult.codeBlockCount,
        );
        return;
    }

    setVisualSyncState("synced", "代码已同步回流程图。", 0);
}

function dismissDefinitionError(): void {
    definitionError.value = "";
}

function dismissDefinitionNotice(): void {
    definitionNotice.value = "";
}

function buildScriptForModel(
    model: StrategyVisualModelDocument | null | undefined,
): string {
    return buildStrategyScriptFromVisualModel(model, {
        name: definitionForm.value.name.trim(),
        symbol: normalizeSymbol(definitionForm.value.symbol),
        interval: normalizeInterval(definitionForm.value.interval),
    });
}

function applyDefinition(definition: StrategyDefinitionDocument | null): void {
    clearDefinitionMessages();
    undoRedo.clear();
    isTemplatePickerEntry.value = false;

    if (definition === null) {
        selectedDefinitionId.value = "";
        selectedStrategyTemplateId.value = defaultStrategyTemplateId;
        replaceDefinitionForm(createDefinitionFromTemplate(defaultStrategyTemplateId), {
            skipScriptParse: true,
        });
        selectedVisualNodeId.value = pickInitialVisualNodeId(resolvedVisualModel.value);
        updateCommittedDefinitionSignature(definitionForm.value);
        resetVisualSyncStatus();
        return;
    }

    selectedDefinitionId.value = definition.id;
    selectedStrategyTemplateId.value = "";
    replaceDefinitionForm(normalizeDefinition(definition), {
        skipScriptParse: true,
    });
    selectedVisualNodeId.value = pickInitialVisualNodeId(resolvedVisualModel.value);
    updateCommittedDefinitionSignature(definitionForm.value);

    if (definition.visualModel === null || definition.visualModel === undefined) {
        syncScriptToVisualModelNow({ updateCommittedSignature: true });
        return;
    }

    attachSourceRangesFromScript(definition.script, definitionForm.value.visualModel);
    resetVisualSyncStatus();
}

function attachSourceRangesFromScript(
    script: string,
    visualModel: StrategyVisualModelDocument | null | undefined,
): void {
    if (visualModel === null || visualModel === undefined || script.trim() === "") {
        return;
    }

    const parseResult = buildStrategyVisualModelFromScript(script);
    if (!parseResult.ok) {
        return;
    }

    const parsedNodeById = new Map(
        parseResult.model.nodes.map((node) => [node.id, node] as const),
    );

    for (const node of visualModel.nodes) {
        const parsedNode = parsedNodeById.get(node.id);
        if (parsedNode === undefined) {
            continue;
        }

        const sourceRange = parsedNode.properties.sourceRange;
        if (
            sourceRange !== undefined &&
            sourceRange !== null &&
            typeof sourceRange === "object"
        ) {
            node.properties = {
                ...node.properties,
                sourceRange,
            };
        }
    }
}

function createNewDefinitionDraft(templateId = defaultStrategyTemplateId): void {
    const template = getStrategyTemplate(templateId);

    clearDefinitionMessages();
    undoRedo.clear();
    isTemplatePickerEntry.value = false;
    lastCommittedDefinitionSignature.value = "";
    selectedDefinitionId.value = "";
    selectedStrategyTemplateId.value = template.id;
    replaceDefinitionForm(createDefinitionFromTemplate(template.id), {
        skipScriptParse: true,
    });
    selectedVisualNodeId.value = pickInitialVisualNodeId(resolvedVisualModel.value);
    isDefinitionsPanelCollapsed.value = true;
    isTemplatesSectionCollapsed.value = true;
    isCodeEditorSectionCollapsed.value = true;
    isVisualBuilderSectionCollapsed.value = false;
    strategyDisplayMode.value = "canvas";
    definitionNotice.value = `已基于「${template.label}」创建新草稿。`;
    resetVisualSyncStatus();
}

async function loadStrategyDefinitions(
    preferredDefinitionId = selectedDefinitionId.value,
    options?: {
        openTemplatePicker?: boolean;
    },
): Promise<void> {
    isLoadingDefinitions.value = true;
    clearDefinitionMessages();

    try {
        const items = await fetchEnvelope<StrategyDefinitionDocument[]>(
            "/api/v1/strategy-definitions",
        );

        strategyDefinitions.value = items;

        if (items.length === 0 || options?.openTemplatePicker === true) {
            openNewDefinitionTemplatePicker();
            return;
        }

        const nextDefinition =
            items.find((item) => item.id === preferredDefinitionId) ?? items[0] ?? null;
        applyDefinition(nextDefinition);
    } catch (error) {
        strategyDefinitions.value = [];
        definitionError.value =
            error instanceof Error
                ? error.message
                : "加载策略定义失败。";
    } finally {
        isLoadingDefinitions.value = false;
    }
}

async function saveStrategyDefinition(): Promise<boolean> {
    clearDefinitionMessages();

    const payload = {
        id: definitionForm.value.id.trim(),
        name: definitionForm.value.name.trim(),
        version: definitionForm.value.version.trim(),
        description: definitionForm.value.description.trim(),
        runtime: definitionForm.value.runtime.trim() || "quickjs-js",
        symbol: normalizeSymbol(definitionForm.value.symbol),
        interval: normalizeInterval(definitionForm.value.interval),
        script: definitionForm.value.script,
        visualModel: cloneStrategyVisualModel(definitionForm.value.visualModel),
    };

    if (payload.name === "") {
        definitionError.value = "策略名称不能为空。";
        return false;
    }

    if (payload.symbol === "") {
        definitionError.value = "标的不能为空。";
        return false;
    }

    if (payload.script.trim() === "") {
        definitionError.value = "策略脚本不能为空。";
        return false;
    }

    const isExisting =
        selectedDefinition.value !== null &&
        selectedDefinition.value.id === selectedDefinitionId.value;
    const path = isExisting
        ? `/api/v1/strategy-definitions/${encodeURIComponent(selectedDefinitionId.value)}`
        : "/api/v1/strategy-definitions";

    isSavingDefinition.value = true;

    try {
        const response = await fetchEnvelopeWithInit<
            StrategyDefinitionDocument | StrategyDefinitionDocument[]
        >(path, {
            method: isExisting ? "PUT" : "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        });

        const savedId = Array.isArray(response)
            ? payload.id || selectedDefinitionId.value
            : response.id;

        definitionNotice.value = isExisting ? "策略已保存。" : "策略已创建。";
        await loadStrategyDefinitions(savedId);
        return true;
    } catch (error) {
        definitionError.value =
            error instanceof Error ? error.message : "保存策略失败。";
        return false;
    } finally {
        isSavingDefinition.value = false;
    }
}

async function instantiateStrategyDefinition(): Promise<void> {
    clearDefinitionMessages();

    if (selectedDefinition.value === null) {
        definitionError.value = "请先选择已保存的策略定义，再创建运行实例。";
        return;
    }

    if (!(await confirmStrategyDesignExit())) {
        return;
    }

    isInstantiatingDefinition.value = true;

    try {
        await fetchEnvelopeWithInit(
            `/api/v1/strategy-definitions/${encodeURIComponent(selectedDefinition.value.id)}/instantiate`,
            {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                },
                body: JSON.stringify({}),
            },
        );

        definitionNotice.value = "已创建运行实例。";
        emit("switch-to-runtime", {
            notice: definitionNotice.value,
        });
    } catch (error) {
        definitionError.value =
            error instanceof Error
                ? error.message
                : "创建运行实例失败。";
    } finally {
        isInstantiatingDefinition.value = false;
    }
}

async function confirmStrategyDesignExit(): Promise<boolean> {
    if (!hasUnsavedDefinitionChanges.value) {
        return true;
    }

    if (typeof window === "undefined" || typeof window.confirm !== "function") {
        return false;
    }

    const shouldSave = window.confirm(
        "当前策略有未保存更改。\n选择“确定”将先保存再离开当前编辑界面。",
    );

    if (shouldSave) {
        return await saveStrategyDefinition();
    }

    return window.confirm("要放弃未保存更改并离开当前编辑界面吗？");
}

function handleBeforeUnload(event: BeforeUnloadEvent): void {
    if (!hasUnsavedDefinitionChanges.value) {
        return;
    }

    event.preventDefault();
    event.returnValue = "";
}

function syncVisualModelToScript(showNotice = false): void {
    if (definitionForm.value.visualModel === null || definitionForm.value.visualModel === undefined) {
        definitionError.value = "请先创建流程图画布。";
        return;
    }

    replaceDefinitionForm(
        {
            ...definitionForm.value,
            script: buildScriptForModel(definitionForm.value.visualModel),
        },
        {
            skipScriptParse: true,
        },
    );

    setVisualSyncState("synced", "流程图已同步到代码。", visualSyncCodeBlockCount.value);

    if (showNotice) {
        definitionNotice.value = "已用流程图更新下方代码。";
    }
}

function initializeVisualModel(): void {
    clearDefinitionMessages();

    const visualModel = createDefaultStrategyVisualModel();
    replaceDefinitionForm(
        {
            ...definitionForm.value,
            visualModel,
            script: buildScriptForModel(visualModel),
        },
        {
            skipScriptParse: true,
        },
    );
    selectedVisualNodeId.value = pickInitialVisualNodeId(visualModel);
    definitionNotice.value = "已创建新的默认流程骨架。";
    setVisualSyncState("synced", "已创建新的默认流程骨架，并同步到代码。", 0);
}

function applyVisualModel(
    model: StrategyVisualModelDocument,
    options?: {
        preserveSelection?: boolean;
        notice?: string;
    },
): void {
    const nextModel =
        cloneStrategyVisualModel(model) ?? createDefaultStrategyVisualModel();
    const nextSelectedNodeId =
        options?.preserveSelection === true &&
            nextModel.nodes.some((node) => node.id === selectedVisualNodeId.value)
            ? selectedVisualNodeId.value
            : pickInitialVisualNodeId(nextModel);
    const nextScript = buildScriptForModel(nextModel);

    replaceDefinitionForm(
        {
            ...definitionForm.value,
            visualModel: nextModel,
            script: nextScript,
        },
        {
            skipScriptParse: true,
        },
    );
    selectedVisualNodeId.value = nextSelectedNodeId;
    setVisualSyncState("synced", "流程图已异步同步到代码。", visualSyncCodeBlockCount.value);

    if (options?.notice) {
        definitionNotice.value = options.notice;
    }
}

function handleVisualModelUpdated(model: StrategyVisualModelDocument): void {
    clearDefinitionMessages();
    applyVisualModel(model, { preserveSelection: true });
}

function handleScriptWorkbenchBlur(): void {
    syncScriptToVisualModelNow();
}

function handleCodeCursorOffset(offset: number): void {
    const nodes = resolvedVisualModel.value.nodes;

    for (const node of nodes) {
        const sourceRange = node.properties.sourceRange;
        if (
            sourceRange === undefined ||
            sourceRange === null ||
            typeof sourceRange !== "object"
        ) {
            continue;
        }

        const start = Reflect.get(sourceRange, "start");
        const end = Reflect.get(sourceRange, "end");
        if (typeof start !== "number" || typeof end !== "number") {
            continue;
        }

        if (offset >= start && offset < end) {
            isCodeOriginatedSelection = true;
            codeContextNodeId.value = node.id;
            selectedVisualNodeId.value = node.id;
            return;
        }
    }

    codeContextNodeId.value = "";
    logicFlowDesignerRef.value?.selectNodeById(null);
}

function handleCodeContextOpenInspector(): void {
    const nodeId = codeContextNodeId.value;
    if (nodeId === "") {
        return;
    }

    codeContextNodeId.value = "";
    isCodeOriginatedSelection = true;
    logicFlowDesignerRef.value?.selectNodeById(nodeId);
    isCodeOriginatedSelection = false;
    selectedVisualNodeId.value = nodeId;
    isBlockInspectorCollapsed.value = false;
}

function handleVisualNodeSelected(nodeId: string | null): void {
    selectedVisualNodeId.value = nodeId ?? "";

    if (isCodeOriginatedSelection) {
        isCodeOriginatedSelection = false;
        return;
    }

    if (nodeId !== null) {
        isBlockInspectorCollapsed.value = false;
    }
}

function readSourceRangeFromNode(
    node: StrategyVisualNodeDocument | null,
): { start: number; end: number } | null {
    if (node === null) {
        return null;
    }

    const sourceRange = node.properties.sourceRange;
    if (
        sourceRange === undefined ||
        sourceRange === null ||
        typeof sourceRange !== "object"
    ) {
        return null;
    }

    const start = Reflect.get(sourceRange, "start");
    const end = Reflect.get(sourceRange, "end");
    if (typeof start !== "number" || typeof end !== "number") {
        return null;
    }

    return { start, end };
}

watch(
    () => selectedVisualNode.value,
    (node) => {
        if (isCodeOriginatedSelection) {
            return;
        }

        const codeRange = readSourceRangeFromNode(node);
        if (codeRange === null) {
            return;
        }

        codeWorkbenchRef.value?.revealCodeRange(codeRange);
    },
);

function mutateSelectedVisualNode(
    mutator: (node: StrategyVisualNodeDocument) => StrategyVisualNodeDocument,
): void {
    const currentModel =
        cloneStrategyVisualModel(definitionForm.value.visualModel) ??
        createDefaultStrategyVisualModel();
    const targetNodeId = selectedVisualNodeId.value;

    if (targetNodeId === "") {
        return;
    }

    let changed = false;

    const nextNodes = currentModel.nodes.map((node) => {
        if (node.id !== targetNodeId) {
            return {
                ...node,
                properties: { ...node.properties },
            };
        }

        changed = true;
        return mutator({
            ...node,
            properties: { ...node.properties },
        });
    });

    if (!changed) {
        return;
    }

    applyVisualModel(
        {
            ...currentModel,
            nodes: nextNodes,
            edges: currentModel.edges.map((edge) => ({
                ...edge,
                properties:
                    edge.properties === undefined ? undefined : { ...edge.properties },
            })),
        },
        { preserveSelection: true },
    );
}

function deleteSelectedVisualNode(): void {
    if (selectedVisualNode.value === null) {
        return;
    }

    const currentModel =
        cloneStrategyVisualModel(definitionForm.value.visualModel) ??
        createDefaultStrategyVisualModel();
    const removedNodeId = selectedVisualNode.value.id;

    applyVisualModel(
        {
            ...currentModel,
            nodes: currentModel.nodes.filter((node) => node.id !== removedNodeId),
            edges: currentModel.edges.filter(
                (edge) =>
                    edge.sourceNodeId !== removedNodeId &&
                    edge.targetNodeId !== removedNodeId,
            ),
        },
        {
            preserveSelection: false,
            notice: "已删除当前图块。",
        },
    );
}

</script>

<template>
    <div class="strategy-stage">
        <div class="strategy-stage__toast-stack">
            <div v-if="definitionError" class="strategy-banner strategy-banner--error">
                <div class="strategy-banner__message">{{ definitionError }}</div>
                <button class="strategy-banner__close" data-testid="dismiss-strategy-error-banner" aria-label="关闭错误提示"
                    type="button" @click="dismissDefinitionError">
                    &times;
                </button>
            </div>
            <div v-if="definitionNotice" class="strategy-banner strategy-banner--success">
                <div class="strategy-banner__message">{{ definitionNotice }}</div>
                <button class="strategy-banner__close" data-testid="dismiss-strategy-notice-banner" aria-label="关闭提示"
                    type="button" @click="dismissDefinitionNotice">
                    &times;
                </button>
            </div>
        </div>

        <div class="strategy-stage__toolbar">
            <div class="strategy-stage__toolbar-card strategy-stage__toolbar-card--primary">
                <div class="strategy-stage__toolbar-top">
                    <div class="strategy-stage__toolbar-main">
                        <div class="strategy-stage__toolbar-title-row">
                            <div class="strategy-stage__toolbar-title">
                                {{ definitionForm.name || "未命名 QuickJS 策略" }}
                            </div>
                            <span class="strategy-page__pill">
                                {{ activeStrategyTemplate?.mode === "visual" ? "图优先" : "代码优先" }}
                            </span>
                            <div class="strategy-stage__toolbar-meta">
                                <span>{{ definitionForm.runtime || "quickjs-js" }}</span>
                                <span>{{ definitionForm.symbol || "未设置标的" }}</span>
                                <span>{{ definitionForm.interval || "1m" }}</span>
                            </div>
                        </div>
                    </div>

                    <div class="strategy-stage__toolbar-controls">
                        <div class="strategy-stage__mode-switch" aria-label="策略工作区模式">
                            <button class="strategy-stage__mode-button is-active"
                                data-testid="strategy-workspace-tab-design" type="button">
                                设计
                            </button>
                            <button class="strategy-stage__mode-button" data-testid="strategy-workspace-tab-runtime"
                                type="button" @click="void handleSwitchToRuntime()">
                                运行
                            </button>
                        </div>

                        <div class="strategy-stage__display-switch" aria-label="策略显示模式">
                            <span class="strategy-stage__display-switch-label">显示</span>
                            <div class="strategy-stage__display-switch-group">
                                <button class="strategy-stage__display-button"
                                    :class="{ 'is-active': strategyDisplayMode === 'canvas' }"
                                    data-testid="strategy-display-mode-canvas" type="button"
                                    @click="setStrategyDisplayMode('canvas')">
                                    画布
                                </button>
                                <button class="strategy-stage__display-button"
                                    :class="{ 'is-active': strategyDisplayMode === 'split' }"
                                    data-testid="strategy-display-mode-split" type="button"
                                    @click="setStrategyDisplayMode('split')">
                                    双栏
                                </button>
                                <button class="strategy-stage__display-button"
                                    :class="{ 'is-active': strategyDisplayMode === 'code' }"
                                    data-testid="strategy-display-mode-code" type="button"
                                    @click="setStrategyDisplayMode('code')">
                                    代码
                                </button>
                            </div>
                        </div>

                        <div class="strategy-stage__toolbar-actions">
                            <button class="strategy-btn strategy-btn--ghost" data-testid="undo-strategy-change"
                                :disabled="!undoRedo.canUndo" type="button" title="撤销 (Ctrl+Z)"
                                @click="handleUndo">
                                ↩
                            </button>
                            <button class="strategy-btn strategy-btn--ghost" data-testid="redo-strategy-change"
                                :disabled="!undoRedo.canRedo" type="button" title="重做 (Ctrl+Shift+Z)"
                                @click="handleRedo">
                                ↪
                            </button>
                            <button class="strategy-btn strategy-btn--primary" data-testid="save-strategy-definition"
                                :disabled="isSavingDefinition" type="button" @click="saveStrategyDefinition">
                                {{ isSavingDefinition ? "保存中…" : "保存策略" }}
                            </button>
                            <button class="strategy-btn" data-testid="instantiate-strategy-definition"
                                :disabled="selectedDefinition === null || isInstantiatingDefinition" type="button"
                                @click="instantiateStrategyDefinition">
                                {{ isInstantiatingDefinition ? "创建中…" : "创建运行实例" }}
                            </button>
                        </div>
                    </div>
                </div>

                <div class="strategy-stage__toolbar-bottom">
                    <div class="strategy-stage__toolbar-tools">
                        <button class="strategy-tool-switch" :class="{ 'is-active': !isDefinitionsPanelCollapsed }"
                            data-testid="toggle-strategy-definitions-floating" type="button"
                            @click="isDefinitionsPanelCollapsed = !isDefinitionsPanelCollapsed">
                            策略定义
                        </button>
                        <button class="strategy-tool-switch" :class="{ 'is-active': !isTemplatesSectionCollapsed }"
                            data-testid="toggle-strategy-templates-section" type="button"
                            @click="isTemplatesSectionCollapsed = !isTemplatesSectionCollapsed">
                            样板策略
                        </button>
                        <button class="strategy-tool-switch" :class="{ 'is-active': !isBasicInfoSectionCollapsed }"
                            data-testid="toggle-strategy-basic-info-section" type="button"
                            @click="isBasicInfoSectionCollapsed = !isBasicInfoSectionCollapsed">
                            基本信息
                        </button>
                        <button class="strategy-tool-switch" :class="{ 'is-active': !isMetadataSectionCollapsed }"
                            data-testid="toggle-strategy-metadata-section" type="button"
                            @click="isMetadataSectionCollapsed = !isMetadataSectionCollapsed">
                            元信息
                        </button>
                        <button class="strategy-tool-switch" :class="{ 'is-active': !isVisualBuilderSectionCollapsed }"
                            data-testid="toggle-strategy-visual-builder-section" type="button"
                            @click="isVisualBuilderSectionCollapsed = !isVisualBuilderSectionCollapsed">
                            画布工具
                        </button>
                        <button class="strategy-tool-switch" :class="{ 'is-active': !isBlockInspectorCollapsed }"
                            data-testid="toggle-strategy-block-inspector-section" type="button"
                            @click="isBlockInspectorCollapsed = !isBlockInspectorCollapsed">
                            图块详情
                        </button>
                    </div>
                    
                    <div class="strategy-stage__toolbar-status" :class="visualSyncToneClass"
                        data-testid="strategy-visual-sync-status">
                        <span class="strategy-stage__toolbar-status-label">{{ visualSyncLabel }}</span>
                        <span
                            v-if="visualNodeMappingQuality !== null"
                            class="strategy-stage__toolbar-status-quality"
                            data-testid="strategy-visual-mapping-quality"
                        >
                            映射 {{ visualNodeMappingQuality.mappableCount }}/{{ visualNodeMappingQuality.total }}
                        </span>
                        <span class="strategy-stage__toolbar-status-message">{{ visualSyncMessage }}</span>
                    </div>
                </div>
            </div>
        </div>

        <div v-if="showsCanvas" class="strategy-stage__canvas">
            <StrategyLogicFlowDesigner ref="logicFlowDesignerRef" :chrome="false" :fit-view-padding="logicFlowFitViewPadding" :height="'100%'"
                :min-height="'100%'" :model-value="resolvedVisualModel" :resizable="false"
                :show-builder-actions="!isVisualBuilderSectionCollapsed"
                :show-zoom-slider="!isVisualBuilderSectionCollapsed"
                :zoom-right-offset="strategyLogicFlowZoomRightOffset" class="h-full min-h-0"
                @select-node="handleVisualNodeSelected" @update:model-value="handleVisualModelUpdated" />
        </div>

        <div v-if="!isDefinitionsPanelCollapsed" data-testid="strategy-definitions-panel"
            class="strategy-stage__panel strategy-stage__panel--definitions" :style="definitionsPanelStyle">
            <StrategyStageDefinitionsPanel :is-loading-definitions="isLoadingDefinitions"
                :selected-definition-id="selectedDefinitionId" :strategy-definitions="strategyDefinitions"
                @close="isDefinitionsPanelCollapsed = true" @create-new="openNewDefinitionTemplatePicker()"
                @drag-start="startDefinitionsPanelDrag" @select-definition="applyDefinition" />
        </div>

        <div v-if="hasDesignOverlayDeck" class="strategy-stage__panel strategy-stage__panel--deck" :class="{
            'strategy-stage__panel--deck-code': strategyDisplayMode === 'code',
            'strategy-stage__panel--deck-no-definitions': isDefinitionsPanelCollapsed,
            'strategy-stage__panel--deck-no-code': !showsCodeWorkbench || strategyDisplayMode === 'code',
        }">
            <StrategyStageOverlayDeck :active-strategy-template-mode="activeStrategyTemplate?.mode ?? null"
                :bindings="overlayDeckBindings" :created-at-text="formatTimestamp(definitionForm.createdAt)"
                :selected-strategy-template-id="selectedStrategyTemplateId"
                :selected-visual-block-description="selectedVisualBlock?.description ?? '调整图块参数并同步 QuickJS。'"
                :selected-visual-block-label="selectedVisualBlock?.label ?? (selectedVisualNode?.text ?? '')"
                :selected-visual-kind="selectedVisualKind" :selected-visual-node="selectedVisualNode"
                :show-basic-info-section="!isBasicInfoSectionCollapsed"
                :show-block-details-section="selectedVisualNode !== null && !isBlockInspectorCollapsed"
                :show-metadata-section="!isMetadataSectionCollapsed"
                :show-templates-section="!isTemplatesSectionCollapsed" :shows-code-input="showsCodeInput"
                :shows-macd-inputs="showsMacdInputs" :shows-technical-indicator-macd-inputs="showsTechnicalIndicatorMacdInputs" :shows-multiplier-input="showsMultiplierInput"
                :shows-period-input="showsPeriodInput" :shows-threshold-input="showsThresholdInput"
                :shows-condition-mode-input="showsConditionModeInput" :shows-indicator-type-input="showsIndicatorTypeInput"
                :shows-pattern-type-input="showsPatternTypeInput" :shows-lookback-input="showsLookbackInput"
                :shows-place-order-inputs="showsPlaceOrderInputs"
                :shows-place-order-limit-price-input="showsPlaceOrderLimitPriceInput"
                :strategy-templates="strategyTemplates" :updated-at-text="formatTimestamp(definitionForm.updatedAt)"
                @delete-selected-node="deleteSelectedVisualNode" @select-template="createNewDefinitionDraft"
                @close-block-details="isBlockInspectorCollapsed = true" />
        </div>

        <div v-if="showsCodeWorkbench" data-testid="strategy-code-editor-section"
            class="strategy-stage__panel strategy-stage__panel--code"
            :class="{ 'strategy-stage__panel--code-pure': strategyDisplayMode === 'code' }" :style="codePanelStyle">
            <StrategyStageCodeWorkbenchPanel ref="codeWorkbenchRef" :bindings="codeWorkbenchBindings"
                :completion-items="strategyEditorCompletions" :extra-libs="strategyEditorExtraLibs"
                :hover-items="strategyEditorHoverItems" :strategy-display-mode="strategyDisplayMode"
                @drag-start="startCodePanelDrag" @script-blur="handleScriptWorkbenchBlur"
                @cursor-offset="handleCodeCursorOffset" />
        </div>

        <div
            v-if="showsCodeWorkbench && codeContextNodeLabel !== ''"
            class="strategy-stage__code-context"
            data-testid="strategy-code-context-hint"
        >
            <button
                class="strategy-stage__code-context-link"
                type="button"
                @click="handleCodeContextOpenInspector"
            >
                关联查看「{{ codeContextNodeLabel }}」的图块详情
            </button>
        </div>

    </div>
</template>

<style scoped>
.strategy-stage {
    --strategy-stage-code-panel-width: min(36vw, 35rem);
    --strategy-stage-code-panel-max-width: calc(100dvw - 4em);
    --strategy-stage-code-panel-max-height: calc(100dvh - 8em);
    --strategy-stage-floating-top: 9.4rem;
    --strategy-stage-toast-top: 9.4rem;
    --strategy-stage-canvas-top-inset: 9.4rem;
    --strategy-stage-panel-bottom: 4.8rem;
    --strategy-stage-footer-bottom: 1rem;
    position: relative;
    height: 100%;
    min-height: 0;
    overflow: hidden;
    background:
        radial-gradient(circle at top left, color-mix(in srgb, var(--tv-accent) 20%, transparent), transparent 34%),
        radial-gradient(circle at bottom right, color-mix(in srgb, var(--tv-up) 14%, transparent), transparent 28%),
        linear-gradient(180deg,
            color-mix(in srgb, var(--tv-bg-surface) 72%, var(--tv-bg-app)) 0%,
            color-mix(in srgb, var(--tv-bg-surface-2) 68%, var(--tv-bg-app)) 100%);
}

.strategy-page__pill {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    min-height: 2.2rem;
    padding: 0.45rem 0.85rem;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: color-mix(in srgb, var(--tv-bg-surface) 58%, transparent);
    color: var(--tv-text-muted);
    font-size: 0.78rem;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    backdrop-filter: blur(24px) saturate(160%);
}

.strategy-banner {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.85rem;
    flex-shrink: 0;
    padding: 0.85rem 1rem;
    border-radius: 1.5rem;
    font-size: 0.92rem;
    line-height: 1.5;
    border: 1px solid rgba(255, 255, 255, 0.08);
    backdrop-filter: blur(24px) saturate(160%);
}

.strategy-banner__message {
    flex: 1;
    min-width: 0;
}

.strategy-banner__close {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    flex: 0 0 auto;
    border: 0;
    border-radius: 999px;
    background: color-mix(in srgb, var(--tv-bg-elevated) 42%, transparent);
    color: inherit;
    font-size: 1.1rem;
    line-height: 1;
    cursor: pointer;
    transition:
        background-color 140ms ease,
        opacity 140ms ease;
}

.strategy-banner__close:hover {
    background: color-mix(in srgb, var(--tv-bg-elevated) 65%, transparent);
}

.strategy-banner__close:focus-visible {
    outline: 2px solid color-mix(in srgb, currentColor 72%, white);
    outline-offset: 2px;
}

.strategy-banner--error {
    background: color-mix(in srgb, var(--card-red-surface) 70%, transparent);
    color: var(--card-red-text);
}

.strategy-banner--success {
    background: color-mix(in srgb, var(--tv-up) 16%, transparent);
    color: color-mix(in srgb, var(--tv-up) 74%, var(--tv-text));
}

.strategy-stage__section-title {
    font-size: 1.05rem;
    line-height: 1.25;
    font-weight: 700;
    color: var(--tv-text);
}

.strategy-stage__toolbar-title-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.2rem;
}

.strategy-stage__toolbar-title {
    font-size: 1.35rem;
    line-height: 1.2;
    font-weight: 700;
    color: var(--tv-text);
}

.strategy-stage__toolbar-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.45rem;
    color: var(--tv-text-muted);
    font-size: 0.78rem;
    font-weight: 600;
}

.strategy-stage__toolbar-meta span {
    display: inline-flex;
    min-height: 1.65rem;
    align-items: center;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: color-mix(in srgb, var(--tv-bg-elevated) 50%, transparent);
    padding: 0.25rem 0.6rem;
    backdrop-filter: blur(20px) saturate(140%);
}

.strategy-stage__toast-stack {
    position: absolute;
    top: var(--strategy-stage-toast-top);
    left: 50%;
    z-index: 45;
    display: grid;
    width: min(40rem, calc(100% - 2rem));
    transform: translateX(-50%);
    gap: 0.5rem;
    pointer-events: none;
}

.strategy-stage__toast-stack>* {
    pointer-events: auto;
}

.strategy-stage__toolbar {
    position: absolute;
    top: 1rem;
    left: 1rem;
    right: 1rem;
    z-index: 30;
    display: block;
    pointer-events: none;
}

.strategy-stage__toolbar-card {
    pointer-events: auto;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-start;
    gap: 0.7rem;
    padding: 0.85rem 1rem;
    border-radius: 1.45rem;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: color-mix(in srgb, var(--tv-bg-surface) 55%, transparent);
    box-shadow:
        0 8px 32px rgba(0, 0, 0, 0.4),
        inset 0 1px 0 rgba(255, 255, 255, 0.06);
    backdrop-filter: blur(28px) saturate(180%);
}

.strategy-stage__toolbar-top,
.strategy-stage__toolbar-bottom {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
    width: 100%;
}

.strategy-stage__toolbar-bottom {
    padding-top: 0.25rem;
    border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.strategy-stage__toolbar-controls,
.strategy-stage__toolbar-tools {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.65rem;
}

.strategy-stage__toolbar-main {
    min-width: 0;
    flex: 1 1 auto;
}

.strategy-stage__toolbar-status {
    display: inline-flex;
    flex: 1 1 20rem;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.55rem;
    min-height: 2.4rem;
    padding: 0.45rem 0.7rem;
    border-radius: 1rem;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: color-mix(in srgb, var(--tv-bg-elevated) 46%, transparent);
    color: var(--tv-text-muted);
}

.strategy-stage__toolbar-status-label {
    display: inline-flex;
    align-items: center;
    min-height: 1.9rem;
    padding: 0.25rem 0.65rem;
    border-radius: 999px;
    background: color-mix(in srgb, var(--tv-bg-surface) 72%, transparent);
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
}

.strategy-stage__toolbar-status-message {
    flex: 1 1 15rem;
    min-width: 0;
    font-size: 0.84rem;
    line-height: 1.5;
}

.strategy-stage__toolbar-status-quality {
    flex: 0 0 auto;
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    padding: 0.12em 0.48em;
    border-radius: 999px;
    background: color-mix(in srgb, var(--tv-accent) 14%, transparent);
    color: color-mix(in srgb, var(--tv-accent) 70%, var(--tv-text));
    white-space: nowrap;
}

.strategy-stage__toolbar-status--syncing {
    border-color: color-mix(in srgb, var(--tv-accent) 55%, transparent);
    color: color-mix(in srgb, var(--tv-accent) 72%, var(--tv-text));
}

.strategy-stage__toolbar-status--synced {
    border-color: color-mix(in srgb, var(--tv-up) 45%, transparent);
    color: color-mix(in srgb, var(--tv-up) 70%, var(--tv-text));
}

.strategy-stage__toolbar-status--partial {
    border-color: color-mix(in srgb, var(--tv-warning, var(--card-amber-text)) 52%, transparent);
    color: color-mix(in srgb, var(--tv-warning, var(--card-amber-text)) 78%, var(--tv-text));
}

.strategy-stage__toolbar-status--error {
    border-color: color-mix(in srgb, var(--tv-down) 55%, transparent);
    color: color-mix(in srgb, var(--tv-down) 74%, var(--tv-text));
}

.strategy-stage__toolbar-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.65rem;
}

.strategy-stage__display-switch {
    display: inline-flex;
    align-items: center;
    gap: 0.55rem;
    padding: 0.25rem 0.35rem 0.25rem 0.7rem;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: color-mix(in srgb, var(--tv-bg-elevated) 42%, transparent);
    backdrop-filter: blur(20px) saturate(160%);
}

.strategy-stage__display-switch-label {
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--tv-text-muted);
}

.strategy-stage__display-switch-group {
    display: inline-flex;
    gap: 0.2rem;
}

.strategy-stage__display-button {
    min-height: 2.15rem;
    padding: 0.4rem 0.85rem;
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--tv-text-muted);
    font-size: 0.84rem;
    font-weight: 700;
    transition:
        background-color 140ms ease,
        color 140ms ease;
}

.strategy-stage__display-button.is-active,
.strategy-stage__display-button:hover {
    background: var(--card-active-surface);
    color: var(--card-active-text);
}

.strategy-stage__mode-switch {
    display: inline-flex;
    flex: 0 0 auto;
    gap: 0.25rem;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: color-mix(in srgb, var(--tv-bg-elevated) 42%, transparent);
    padding: 0.25rem;
    backdrop-filter: blur(20px) saturate(160%);
}

.strategy-stage__mode-button {
    min-height: 2.35rem;
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--tv-text-muted);
    cursor: pointer;
    font-size: 0.9rem;
    font-weight: 700;
    padding: 0.45rem 0.95rem;
    transition:
        background-color 140ms ease,
        color 140ms ease;
}

.strategy-stage__mode-button:hover,
.strategy-stage__mode-button.is-active {
    background: var(--card-active-surface);
    color: var(--card-active-text);
}

.strategy-stage__canvas {
    position: absolute;
    inset: 0;
    padding: 0;
}

.strategy-stage__panel {
    position: absolute;
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
    border-radius: 1.5rem;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: color-mix(in srgb, var(--tv-bg-surface) 55%, transparent);
    box-shadow:
        0 8px 32px rgba(0, 0, 0, 0.4),
        inset 0 1px 0 rgba(255, 255, 255, 0.06);
    backdrop-filter: blur(28px) saturate(180%);
}

.strategy-stage__panel--definitions {
    top: var(--strategy-stage-floating-top);
    left: 1rem;
    bottom: var(--strategy-stage-panel-bottom);
    width: 17.5rem;
}

.strategy-stage__panel--deck {
    top: var(--strategy-stage-floating-top);
    bottom: var(--strategy-stage-panel-bottom);
    left: 19.2rem;
    right: calc(min(36vw, 35rem) + 2rem);
    gap: 0.75rem;
    border: 0;
    background: transparent;
    box-shadow: none;
    backdrop-filter: none;
    pointer-events: none;
    z-index: 24;
}

.strategy-stage__panel--deck-code {
    right: 1rem;
}

.strategy-stage__panel--deck-no-definitions {
    left: 1rem;
}

.strategy-stage__panel--deck-no-code {
    right: 1rem;
}

.strategy-stage__panel--code {
    top: var(--strategy-stage-floating-top);
    right: 1rem;
    width: var(--strategy-stage-code-panel-width);
    min-width: 24rem;
    min-height: 20rem;
    height: min(40rem, var(--strategy-stage-code-panel-max-height), calc(100% - var(--strategy-stage-floating-top) - var(--strategy-stage-panel-bottom)));
    max-width: min(var(--strategy-stage-code-panel-max-width), calc(100% - 2rem));
    max-height: min(var(--strategy-stage-code-panel-max-height), calc(100% - var(--strategy-stage-floating-top) - var(--strategy-stage-footer-bottom)));
    resize: both;
}

.strategy-stage__panel--code-pure {
    left: 1rem;
    right: 1rem;
    width: auto;
    height: min(var(--strategy-stage-code-panel-max-height), calc(100% - var(--strategy-stage-floating-top) - var(--strategy-stage-footer-bottom)));
    max-width: min(calc(100dvw - 2rem), calc(100% - 2rem));
    max-height: var(--strategy-stage-code-panel-max-height);
    resize: none;
}

.strategy-stage__code-context {
    position: absolute;
    right: 1rem;
    bottom: calc(var(--strategy-stage-panel-bottom) - 0.5rem);
    z-index: 28;
    max-width: var(--strategy-stage-code-panel-width);
    pointer-events: auto;
}

.strategy-stage__code-context-link {
    display: inline-block;
    padding: 0.35em 0.85em;
    border-radius: 999px;
    border: 1px solid color-mix(in srgb, var(--tv-accent) 32%, transparent);
    background: color-mix(in srgb, var(--tv-bg-surface) 88%, var(--tv-accent));
    color: color-mix(in srgb, var(--tv-accent) 78%, var(--tv-text));
    font-size: 0.78rem;
    font-weight: 600;
    letter-spacing: 0.02em;
    cursor: pointer;
    white-space: nowrap;
    transition:
        background-color 140ms ease,
        border-color 140ms ease;
    box-shadow: 0 4px 18px rgba(0, 0, 0, 0.22);
    backdrop-filter: blur(14px);
}

.strategy-stage__code-context-link:hover {
    background: color-mix(in srgb, var(--tv-bg-surface) 76%, var(--tv-accent));
    border-color: color-mix(in srgb, var(--tv-accent) 56%, transparent);
}

.strategy-stage__panel-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.9rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid rgba(255, 255, 255, 0.08);
    flex-shrink: 0;
}

.strategy-stage__drag-handle {
    cursor: grab;
    user-select: none;
}

.strategy-stage__drag-handle:active {
    cursor: grabbing;
}

.strategy-stage__panel-body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 1rem;
    overscroll-behavior: contain;
    scrollbar-gutter: stable both-edges;
}

.strategy-stage__panel--deck>.strategy-stage__panel-body {
    display: flex;
    flex-direction: row;
    align-items: stretch;
    gap: 0.75rem;
    overflow-x: auto;
    overflow-y: hidden;
    padding: 0;
    pointer-events: none;
}

.strategy-stage__panel--deck .strategy-stack-card {
    flex: 0 0 min(25rem, calc(100vw - 4rem));
    min-width: min(22rem, calc(100vw - 4rem));
    max-width: 28rem;
    display: flex;
    flex-direction: column;
    height: 100%;
    max-height: none;
    overflow: auto;
    overscroll-behavior: contain;
    pointer-events: auto;
    scrollbar-gutter: stable;
}

.strategy-stage__panel--deck .strategy-stack-card:only-child {
    max-height: 100%;
}

.strategy-stage__panel-body--editor {
    display: flex;
    padding: 0.55rem;
}

.strategy-stack-card,
.strategy-list-card,
.strategy-template-card,
.strategy-meta-card,
.strategy-empty-state {
    border-radius: 1.35rem;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: color-mix(in srgb, var(--tv-bg-surface) 58%, transparent);
    box-shadow: 0 12px 28px rgba(2, 6, 23, 0.2);
    backdrop-filter: blur(24px) saturate(160%);
}

.strategy-stack-card {
    padding: 1rem;
}

.strategy-stack-card__head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
}

.strategy-list-card,
.strategy-template-card {
    width: 100%;
    padding: 1rem;
    text-align: left;
    transition:
        border-color 140ms ease,
        background-color 140ms ease,
        transform 140ms ease;
}

.strategy-list-card:hover,
.strategy-template-card:hover {
    border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
    transform: translateY(-1px);
}

.strategy-list-card.is-active,
.strategy-template-card.is-active {
    border-color: var(--card-active-border);
    background: var(--card-active-surface);
    color: var(--card-active-text);
}

.strategy-meta-card {
    padding: 0.9rem 1rem;
}

.strategy-empty-state {
    padding: 1rem;
    color: var(--tv-text-muted);
    line-height: 1.65;
}

.strategy-stage input,
.strategy-stage textarea {
    border-color: rgba(255, 255, 255, 0.1);
    background: color-mix(in srgb, var(--tv-bg-elevated) 58%, var(--tv-bg-surface));
    color: var(--tv-text);
}

.strategy-stage input::placeholder,
.strategy-stage textarea::placeholder {
    color: var(--tv-text-dim);
}

.strategy-btn,
.strategy-tool-switch,
.strategy-icon-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    min-height: 2.4rem;
    padding: 0.55rem 1rem;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: color-mix(in srgb, var(--tv-bg-elevated) 50%, transparent);
    color: var(--tv-text);
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    transition:
        border-color 140ms ease,
        background-color 140ms ease,
        color 140ms ease,
        opacity 140ms ease,
        transform 140ms ease;
    backdrop-filter: blur(20px) saturate(160%);
}

.strategy-btn:hover,
.strategy-tool-switch:hover,
.strategy-icon-btn:hover {
    border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
    transform: translateY(-1px);
}

.strategy-btn:disabled,
.strategy-tool-switch:disabled,
.strategy-icon-btn:disabled {
    cursor: not-allowed;
    opacity: 0.55;
    transform: none;
}

.strategy-btn--primary {
    background: var(--card-active-surface);
    border-color: var(--card-active-border);
    color: var(--card-active-text);
}

.strategy-btn--ghost {
    background: transparent;
}

.strategy-btn--danger {
    color: var(--tv-down);
}

.strategy-tool-switch.is-active {
    background: var(--card-active-surface);
    border-color: var(--card-active-border);
    color: var(--card-active-text);
}

.strategy-stage__toolbar-card--tools .strategy-tool-switch {
    min-height: 1.9rem;
    padding: 0.3rem 0.8rem;
    font-size: 0.82rem;
}

.strategy-stage__toolbar-tools .strategy-tool-switch {
    min-height: 1.95rem;
    padding: 0.32rem 0.82rem;
    font-size: 0.82rem;
}

.strategy-icon-btn {
    width: 2.5rem;
    min-width: 2.5rem;
    padding: 0.45rem;
    font-size: 0.72rem;
}

@media (max-width: 1440px) {
    .strategy-stage {
        --strategy-stage-code-panel-width: min(38vw, 31rem);
    }

    .strategy-stage__panel--deck {
        right: calc(min(38vw, 31rem) + 2rem);
    }

}

@media (max-width: 1180px) {
    .strategy-stage {
        --strategy-stage-code-panel-width: min(42vw, 27rem);
    }

    .strategy-stage__panel--definitions {
        width: 15rem;
    }

    .strategy-stage__panel--deck {
        left: 17rem;
        right: calc(min(42vw, 27rem) + 2rem);
    }

    .strategy-stage__toolbar-top,
    .strategy-stage__toolbar-bottom {
        align-items: flex-start;
    }

    .strategy-stage__toolbar-status {
        width: 100%;
    }

    .strategy-stage__panel--deck-code {
        right: 1rem;
    }

    .strategy-stage__panel--deck-no-definitions {
        left: 1rem;
    }

    .strategy-stage__panel--code {
        min-width: 21rem;
    }
}

@media (max-width: 960px) {
    .strategy-stage {
        --strategy-stage-toast-top: 11.6rem;
        --strategy-stage-canvas-top-inset: 11.6rem;
    }

    .strategy-stage__panel--definitions,
    .strategy-stage__panel--deck,
    .strategy-stage__panel--code,
    .strategy-stage__panel--code-pure {
        top: auto;
        left: 1rem;
        right: 1rem;
        bottom: 6rem;
        width: calc(100% - 2rem);
        max-width: none;
        height: auto;
        max-height: none;
        resize: none;
        transform: none !important;
    }

    .strategy-stage__panel--definitions {
        height: 15.5rem;
    }

    .strategy-stage__panel--deck,
    .strategy-stage__panel--deck-no-definitions {
        height: 18rem;
        bottom: 22.5rem;
    }

    .strategy-stage__panel--code,
    .strategy-stage__panel--code-pure {
        height: 19rem;
    }
}
</style>
