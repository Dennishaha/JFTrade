<script setup lang="ts">
import type {
    StrategyDefinitionDocument,
    StrategySourceFormat,
    StrategyVisualModelDocument,
} from "@jftrade/ui-contracts";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import StrategyLogicFlowDesigner from "./StrategyLogicFlowDesigner.vue";
import StrategyStageCodeWorkbenchPanel from "./strategy-stage/StrategyStageCodeWorkbenchPanel.vue";
import StrategyStageDefinitionsPanel from "./strategy-stage/StrategyStageDefinitionsPanel.vue";
import StrategyStageOverlayDeck from "./strategy-stage/StrategyStageOverlayDeck.vue";
import { useStrategyStageDefinitionPersistence } from "./strategy-stage/useStrategyStageDefinitionPersistence";
import { useStrategyStageVisualSync } from "./strategy-stage/useStrategyStageVisualSync";
import "./strategy-stage/strategyStageShared.css";
import {
    fetchEnvelope,
    fetchEnvelopeWithInit,
} from "../composables/apiClient";
import { useDraggable } from "../composables/useDraggable";
import { useStrategyIndicatorVariables } from "../composables/useStrategyIndicatorVariables";
import { useStrategyUndoRedo } from "../composables/useStrategyUndoRedo";
import { useStrategyVisualNodeInspector } from "../composables/useStrategyVisualNodeInspector";
import {
    strategyDslEditorCompletions,
    strategyDslEditorExtraLibs,
    strategyDslEditorHoverItems,
} from "../features/strategyDslEditorIntelliSense";
import {
    buildStrategyScriptFromVisualModel,
    cloneStrategyVisualModel,
    createDefaultStrategyVisualModel,
    getStrategyAuthoringTemplates,
    getStrategyBlockKind,
    type StrategyAuthoringTemplate,
} from "../features/strategyVisualBuilder";
import { migrateLegacyMovingAverageDefinition } from "../features/strategyVisualBuilderMigration";

const props = withDefaults(defineProps<{
    entryMode?: "existing" | "new";
    initialDefinitionsCollapsed?: boolean;
}>(), {
    entryMode: "existing",
    initialDefinitionsCollapsed: true,
});

const emit = defineEmits<{
    "switch-to-runtime": [payload?: { notice?: string; definitionId?: string }];
    "definitions-count-change": [count: number];
}>();

const fallbackTemplate: StrategyAuthoringTemplate = {
    id: "logic-flow-starter",
    label: "逻辑流起步骨架",
    description: "给一份可视化起步图，适合从拖拽块开始搭策略。",
    mode: "visual",
    defaultId: "dsl-logic-flow-starter",
    defaultName: "逻辑流起步骨架",
    defaultVersion: "0.1.0",
    defaultDescription: "用流程图拖拽块，快速搭一个可保存的 DSL 策略骨架。",
    defaultSymbol: "00700",
    defaultInterval: "5m",
    visualModel: createDefaultStrategyVisualModel(),
    syncVisualToCode: true,
    buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createDefaultStrategyVisualModel(), context),
};

const strategyTemplates = getStrategyAuthoringTemplates();
const defaultStrategyTemplate = strategyTemplates[0] ?? fallbackTemplate;
const defaultStrategyTemplateId = defaultStrategyTemplate.id;

function generateStrategyDefinitionId(): string {
    const cryptoObject = globalThis.crypto;
    if (typeof cryptoObject?.randomUUID === "function") {
        return cryptoObject.randomUUID();
    }

    const bytes = new Uint8Array(16);
    if (typeof cryptoObject?.getRandomValues === "function") {
        cryptoObject.getRandomValues(bytes);
    } else {
        for (let index = 0; index < bytes.length; index += 1) {
            bytes[index] = Math.floor(Math.random() * 256);
        }
    }

    bytes[6] = ((bytes[6] ?? 0) & 0x0f) | 0x40;
    bytes[8] = ((bytes[8] ?? 0) & 0x3f) | 0x80;

    const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, "0"));
    return `${hex.slice(0, 4).join("")}-${hex.slice(4, 6).join("")}-${hex.slice(6, 8).join("")}-${hex.slice(8, 10).join("")}-${hex.slice(10, 16).join("")}`;
}

const strategyDefinitions = ref<StrategyDefinitionDocument[]>([]);
const selectedDefinitionId = ref("");
const selectedStrategyTemplateId = ref(defaultStrategyTemplateId);
const isLoadingDefinitions = ref(false);
const definitionError = ref("");
const definitionNotice = ref("");

const isDefinitionsPanelCollapsed = ref(props.initialDefinitionsCollapsed);
const isTemplatesSectionCollapsed = ref(true);
const isBasicInfoSectionCollapsed = ref(true);
const isCodeEditorSectionCollapsed = ref(true);
const isBlockInspectorCollapsed = ref(true);
const strategyDisplayMode = ref<"canvas" | "split" | "code">("canvas");

const definitionForm = ref<StrategyDefinitionDocument>(
    createDefinitionFromTemplate(defaultStrategyTemplateId),
);
const lastCommittedDefinitionSignature = ref("");
const isTemplatePickerEntry = ref(false);

const definitionsPanelDrag = useDraggable();
const codePanelDrag = useDraggable();
const codeWorkbenchRef = ref<InstanceType<typeof StrategyStageCodeWorkbenchPanel> | null>(null);
const logicFlowDesignerRef = ref<InstanceType<typeof StrategyLogicFlowDesigner> | null>(null);

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

const {
    applyVisualModel,
    attachSourceRangesFromScript,
    codeContextNodeId,
    deleteSelectedVisualNode,
    handleCodeContextOpenInspector,
    handleCodeCursorOffset,
    handleScriptWorkbenchBlur,
    handleVisualModelUpdated,
    handleVisualNodeSelected,
    markNextScriptSyncAsInternal,
    mutateSelectedVisualNode,
    replaceDefinitionForm,
    resetVisualSyncStatus,
    resolvedVisualModel,
    selectedVisualNode,
    selectedVisualNodeId,
    syncScriptToVisualModelNow,
    updateCommittedDefinitionSignature,
    visualSyncCodeBlockCount,
    visualSyncMessage,
    visualSyncStatus,
} = useStrategyStageVisualSync({
    definitionForm,
    definitionError,
    definitionNotice,
    lastCommittedDefinitionSignature,
    clearDefinitionMessages,
    pickInitialVisualNodeId,
    buildScriptForModel,
    serializeDefinitionSnapshot,
    codeWorkbenchRef,
    logicFlowDesignerRef,
    showBlockInspector: () => {
        isBlockInspectorCollapsed.value = false;
    },
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
    showsMovingAverageTypeInput,
    showsIndicatorVariableNameInput,
    indicatorVariableNamePlaceholder,
    selectedIndicatorVariableName,
    showsIndicatorPrimaryInputSelect,
    showsIndicatorFastInputSelect,
    showsIndicatorSlowInputSelect,
    indicatorGetterOptions,
    selectedIndicatorPrimaryInputNodeId,
    selectedIndicatorFastInputNodeId,
    selectedIndicatorSlowInputNodeId,
    selectedStopLossMode,
    selectedStopLossDirection,
    selectedStopLossTimeUnit,
    selectedStopLossWindowPolicy,
    selectedMacdFastPeriod,
    selectedMacdSlowPeriod,
    selectedMacdSignalPeriod,
    selectedMovingAverageType,
    selectedIndicatorPeriodUnit,
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
    selectedPlaceOrderEntryPositionPolicy,
    selectedPlaceOrderQuantityMode,
    selectedPlaceOrderQuantityValue,
    selectedPlaceOrderLimitPrice,
    showsPlaceOrderEntryPositionPolicyInput,
    showsPlaceOrderLimitPriceInput,
} = useStrategyVisualNodeInspector({
    visualModel: resolvedVisualModel,
    selectedVisualNode,
    mutateSelectedVisualNode,
});

const overlayDeckBindings = {
    definitionForm,
    selectedVisualNodeText,
    selectedVisualNodeMessage,
    selectedVisualNodeCode,
    selectedVisualNodePeriod,
    selectedIndicatorVariableName,
    selectedIndicatorType,
    selectedMovingAverageType,
    selectedIndicatorPeriodUnit,
    selectedIndicatorConditionMode,
    selectedIndicatorOperator,
    selectedIndicatorPatternType,
    selectedIndicatorLookback,
    selectedIndicatorPrimaryInputNodeId,
    selectedIndicatorFastInputNodeId,
    selectedIndicatorSlowInputNodeId,
    selectedStopLossMode,
    selectedStopLossDirection,
    selectedStopLossTimeUnit,
    selectedStopLossWindowPolicy,
    selectedMacdFastPeriod,
    selectedMacdSlowPeriod,
    selectedMacdSignalPeriod,
    selectedBollingerMultiplier,
    selectedVisualNodeThreshold,
    selectedPlaceOrderSide,
    selectedPlaceOrderType,
    selectedPlaceOrderEntryPositionPolicy,
    selectedPlaceOrderQuantityMode,
    selectedPlaceOrderQuantityValue,
    selectedPlaceOrderLimitPrice,
} as const;

const codeWorkbenchBindings = {
    definitionForm,
} as const;

const hasDesignOverlayDeck = computed(
    () =>
        !isTemplatesSectionCollapsed.value ||
        !isBasicInfoSectionCollapsed.value ||
        (!isBlockInspectorCollapsed.value && selectedVisualNode.value !== null),
);

const showsCanvas = computed(() => strategyDisplayMode.value !== "code");

const showsCodeWorkbench = computed(
    () => !isCodeEditorSectionCollapsed.value && strategyDisplayMode.value !== "canvas",
);

const logicFlowFitViewPadding = computed(() => ({
    top: hasDesignOverlayDeck.value ? 320 : 156,
    right: showsCodeWorkbench.value ? 540 : 36,
    bottom: showsCanvas.value ? 132 : 36,
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
            return "DSL 提示";
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
        window.addEventListener("keydown", handleDesignKeydown);
    }

    void loadStrategyDefinitions(selectedDefinitionId.value, {
        openTemplatePicker: props.entryMode === "new",
    });
});

onBeforeUnmount(() => {
    if (typeof window !== "undefined") {
        window.removeEventListener("keydown", handleDesignKeydown);
    }
});

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
    isBlockInspectorCollapsed.value = true;
    isCodeEditorSectionCollapsed.value = true;
    strategyDisplayMode.value = "canvas";
    resetVisualSyncStatus();
}

function createDefinitionFromTemplate(
    templateId: string,
): StrategyDefinitionDocument {
    const template = getStrategyTemplate(templateId);
    const visualModel =
        cloneStrategyVisualModel(template.visualModel) ??
        createDefaultStrategyVisualModel();

    return {
        id: generateStrategyDefinitionId(),
        name: template.defaultName,
        version: template.defaultVersion,
        description: template.defaultDescription,
        runtime: "dsl-go-plan",
        sourceFormat: "dsl-v1",
        script: template.buildScript({
            name: template.defaultName,
            version: template.defaultVersion,
        }),
        visualModel,
        createdAt: "",
        updatedAt: "",
    };
}

function normalizeSourceFormat(
    value: StrategySourceFormat | string | null | undefined,
): StrategySourceFormat {
    void value;
    return "dsl-v1";
}

function normalizeDefinition(
    definition: StrategyDefinitionDocument,
): StrategyDefinitionDocument {
    const migrated = migrateLegacyMovingAverageDefinition(definition);
    const visualModel =
        cloneStrategyVisualModel(migrated.visualModel) ??
        createDefaultStrategyVisualModel();
    const name = migrated.name.trim();
    const version = migrated.version.trim() || "0.1.0";

    return {
        ...migrated,
        runtime: "dsl-go-plan",
        sourceFormat: "dsl-v1",
        version,
        script: buildStrategyScriptFromVisualModel(visualModel, {
            name,
            version,
        }),
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
        runtime: "dsl-go-plan",
        sourceFormat: normalizeSourceFormat(definition.sourceFormat),
        script: definition.script,
        visualModel:
            cloneStrategyVisualModel(definition.visualModel) ??
            createDefaultStrategyVisualModel(),
    });
}

function pickInitialVisualNodeId(model: StrategyVisualModelDocument): string {
    const preferred = model.nodes.find((node) => {
        const kind = getStrategyBlockKind(node);
        return kind !== "onInit" && kind !== "onKLineClosed" && kind !== "getTechnicalIndicator";
    });

    return preferred?.id ?? model.nodes[0]?.id ?? "";
}

function formatTimestamp(value: string | undefined | null): string {
    if ((value ?? "").trim() === "") {
        return "暂无";
    }
    return value!.replace("T", " ").replace(".000Z", "Z");
}

function clearDefinitionMessages(): void {
    definitionError.value = "";
    definitionNotice.value = "";
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
        version: definitionForm.value.version.trim() || "0.1.0",
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

    if (definition.visualModel === null || definition.visualModel === undefined) {
        syncScriptToVisualModelNow({ updateCommittedSignature: true });
        return;
    }

    attachSourceRangesFromScript(definition.script, definitionForm.value.visualModel);
    updateCommittedDefinitionSignature(definitionForm.value);
    resetVisualSyncStatus();
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

        strategyDefinitions.value = items.map((item) => normalizeDefinition(item));

        if (strategyDefinitions.value.length === 0 || options?.openTemplatePicker === true) {
            openNewDefinitionTemplatePicker();
            return;
        }

        const nextDefinition =
            strategyDefinitions.value.find((item) => item.id === preferredDefinitionId) ?? strategyDefinitions.value[0] ?? null;
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

function loadStrategyDefinitionsForPersistence(
    preferredDefinitionId = selectedDefinitionId.value,
): Promise<void> {
    return loadStrategyDefinitions(preferredDefinitionId);
}

function buildStrategyDefinitionSavePayload(): StrategyDefinitionDocument {
    const definitionId = definitionForm.value.id.trim() || generateStrategyDefinitionId();
    if (definitionForm.value.id.trim() === "") {
        definitionForm.value.id = definitionId;
    }

    return {
        id: definitionId,
        name: definitionForm.value.name.trim(),
        version: definitionForm.value.version.trim(),
        description: definitionForm.value.description.trim(),
        runtime: "dsl-go-plan",
        sourceFormat: normalizeSourceFormat(definitionForm.value.sourceFormat),
        script: definitionForm.value.script,
        visualModel: cloneStrategyVisualModel(definitionForm.value.visualModel),
        createdAt: definitionForm.value.createdAt,
        updatedAt: definitionForm.value.updatedAt,
    };
}

const {
    closeDeleteDefinitionDialog,
    closeSaveLinkedInstancesDialog,
    confirmDeleteStrategyDefinition,
    confirmStrategyDesignExit,
    deleteStrategyDefinition,
    deletingDefinitionId,
    instantiateStrategyDefinition,
    isDeleteDefinitionDialogOpen,
    isInstantiatingDefinition,
    isSaveLinkedInstancesDialogOpen,
    isSavingDefinition,
    jumpToRuntimeForDeletingLinkedInstances,
    pendingDeleteDefinition,
    pendingSaveLinkedInstancesSummary,
    saveStrategyDefinition,
    saveStrategyDefinitionWithLinkedInstanceChoice,
    summarizeLinkedStrategyForDeleteDialog,
} = useStrategyStageDefinitionPersistence({
    definitionForm,
    selectedDefinitionId,
    selectedDefinition,
    strategyDefinitions,
    hasUnsavedDefinitionChanges,
    definitionError,
    definitionNotice,
    clearDefinitionMessages,
    buildSavePayload: buildStrategyDefinitionSavePayload,
    loadStrategyDefinitions: loadStrategyDefinitionsForPersistence,
    emitSwitchToRuntime: (payload) => emit("switch-to-runtime", payload),
});

const {
    indicatorVariables,
    addIndicatorVariable,
    updateIndicatorVariable,
    deleteIndicatorVariable,
} = useStrategyIndicatorVariables({
    visualModel: resolvedVisualModel,
    selectedVisualNodeId,
    applyVisualModel,
});

</script>

<template>
    <div class="strategy-stage">
        <v-dialog v-model="isSaveLinkedInstancesDialogOpen" max-width="560">
            <div class="strategy-save-dialog" data-testid="strategy-save-linked-dialog">
                <div class="strategy-save-dialog__eyebrow">保存策略</div>
                <div class="strategy-save-dialog__title">检测到关联实例正在使用当前策略</div>
                <div v-if="pendingSaveLinkedInstancesSummary !== null"
                    class="strategy-save-dialog__body" data-testid="strategy-save-linked-summary">
                    当前共有 {{ pendingSaveLinkedInstancesSummary.linkedCount }} 个实例应用了这份策略，
                    其中 {{ pendingSaveLinkedInstancesSummary.stoppedCount }} 个实例为 STOPPED，保存后可立即刷新到最新版本；
                    {{ pendingSaveLinkedInstancesSummary.busyCount }} 个运行中或暂停中的实例会保留当前版本，后续停止后再刷新。
                </div>
                <div class="strategy-save-dialog__hint">策略版本会由系统在本次保存时自动编号。</div>
                <div class="strategy-save-dialog__actions">
                    <button class="strategy-btn strategy-btn--ghost" data-testid="strategy-save-definition-only"
                        :disabled="isSavingDefinition" type="button"
                        @click="void saveStrategyDefinitionWithLinkedInstanceChoice('save-only')">
                        仅保存策略
                    </button>
                    <button class="strategy-btn strategy-btn--primary" data-testid="strategy-save-definition-and-apply"
                        :disabled="isSavingDefinition" type="button"
                        @click="void saveStrategyDefinitionWithLinkedInstanceChoice('save-and-apply')">
                        {{ isSavingDefinition ? "保存中…" : "保存并更新关联实例" }}
                    </button>
                </div>
                <button class="strategy-save-dialog__close" type="button" @click="closeSaveLinkedInstancesDialog">
                    继续编辑
                </button>
            </div>
        </v-dialog>

        <v-dialog v-model="isDeleteDefinitionDialogOpen" max-width="620">
            <div class="strategy-save-dialog" data-testid="strategy-delete-definition-dialog">
                <div class="strategy-save-dialog__eyebrow">删除策略</div>
                <div class="strategy-save-dialog__title">
                    {{ pendingDeleteDefinition?.linkedStrategies.length ? "当前还有实例引用该策略" : "确认删除当前策略定义" }}
                </div>
                <div v-if="pendingDeleteDefinition !== null"
                    class="strategy-save-dialog__body" data-testid="strategy-delete-definition-summary">
                    <template v-if="pendingDeleteDefinition.linkedStrategies.length === 0">
                        策略定义「{{ pendingDeleteDefinition.definition.name }}」会被软删除，并从当前定义列表中移除。
                        <template v-if="selectedDefinitionId === pendingDeleteDefinition.definition.id && hasUnsavedDefinitionChanges">
                            当前未保存的编辑也会一并丢弃。
                        </template>
                    </template>
                    <template v-else>
                        策略定义「{{ pendingDeleteDefinition.definition.name }}」当前仍被
                        {{ pendingDeleteDefinition.linkedStrategies.length }} 个实例引用，请先删除这些实例，再回来删除该定义。
                    </template>
                </div>
                <div v-if="pendingDeleteDefinition !== null && pendingDeleteDefinition.linkedStrategies.length > 0"
                    class="strategy-save-dialog__linked-list" data-testid="strategy-delete-linked-instances">
                    <div v-for="strategy in pendingDeleteDefinition.linkedStrategies" :key="strategy.id"
                        class="strategy-save-dialog__linked-item"
                        :data-testid="`strategy-delete-linked-instance-${strategy.id}`">
                        <div class="strategy-save-dialog__linked-item-id">{{ strategy.id }}</div>
                        <div class="strategy-save-dialog__linked-item-meta">
                            {{ summarizeLinkedStrategyForDeleteDialog(strategy) }}
                        </div>
                    </div>
                </div>
                <div class="strategy-save-dialog__hint">
                    {{ pendingDeleteDefinition?.linkedStrategies.length
                        ? "删除策略定义不会自动删除运行实例。请先在运行面板删除这些实例，再回到设计页删除定义。"
                        : "删除后不会自动影响现有实例；只有未被实例引用的定义才允许删除。" }}
                </div>
                <div class="strategy-save-dialog__actions">
                    <button class="strategy-btn strategy-btn--ghost" data-testid="cancel-delete-strategy-definition"
                        :disabled="deletingDefinitionId !== ''" type="button" @click="closeDeleteDefinitionDialog()">
                        {{ pendingDeleteDefinition?.linkedStrategies.length ? "我知道了" : "取消" }}
                    </button>
                    <button v-if="pendingDeleteDefinition !== null && pendingDeleteDefinition.linkedStrategies.length > 0"
                        class="strategy-btn" data-testid="jump-to-runtime-for-delete-linked-instances"
                        :disabled="deletingDefinitionId !== ''" type="button"
                        @click="jumpToRuntimeForDeletingLinkedInstances">
                        去运行面板删除实例
                    </button>
                    <button v-if="pendingDeleteDefinition !== null && pendingDeleteDefinition.linkedStrategies.length === 0"
                        class="strategy-btn strategy-btn--primary" data-testid="confirm-delete-strategy-definition"
                        :disabled="deletingDefinitionId !== ''" type="button"
                        @click="void confirmDeleteStrategyDefinition()">
                        {{ deletingDefinitionId === pendingDeleteDefinition.definition.id ? "删除中…" : "确认删除" }}
                    </button>
                </div>
            </div>
        </v-dialog>

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
                                {{ definitionForm.name || "未命名 DSL 策略" }}
                            </div>
                            <span class="strategy-page__pill">
                                {{ activeStrategyTemplate?.mode === "visual" ? "图优先" : "代码优先" }}
                            </span>
                            <div class="strategy-stage__toolbar-meta">
                                <span>{{ definitionForm.runtime || "dsl-go-plan" }}</span>
                                <span>v{{ definitionForm.version || "0.1.0" }}</span>
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
                :indicator-variables="indicatorVariables" :min-height="'100%'" :model-value="resolvedVisualModel" :resizable="false"
                :show-builder-actions="showsCanvas"
                :show-zoom-slider="showsCanvas"
                :zoom-right-offset="strategyLogicFlowZoomRightOffset" class="h-full min-h-0"
                @add-indicator-variable="addIndicatorVariable"
                @delete-indicator-variable="deleteIndicatorVariable"
                @select-node="handleVisualNodeSelected"
                @update-indicator-variable="updateIndicatorVariable"
                @update:model-value="handleVisualModelUpdated" />
        </div>

        <div v-if="!isDefinitionsPanelCollapsed" data-testid="strategy-definitions-panel"
            class="strategy-stage__panel strategy-stage__panel--definitions" :style="definitionsPanelStyle">
            <StrategyStageDefinitionsPanel :is-loading-definitions="isLoadingDefinitions"
                :deleting-definition-id="deletingDefinitionId"
                :selected-definition-id="selectedDefinitionId" :strategy-definitions="strategyDefinitions"
                @close="isDefinitionsPanelCollapsed = true" @create-new="openNewDefinitionTemplatePicker()"
                @delete-definition="void deleteStrategyDefinition($event)"
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
                :selected-visual-block-description="selectedVisualBlock?.description ?? '调整图块参数并同步 DSL。'"
                :selected-visual-block-label="selectedVisualBlock?.label ?? (selectedVisualNode?.text ?? '')"
                :selected-visual-kind="selectedVisualKind" :selected-visual-node="selectedVisualNode"
                :show-basic-info-section="!isBasicInfoSectionCollapsed"
                :show-block-details-section="selectedVisualNode !== null && !isBlockInspectorCollapsed"
                :show-templates-section="!isTemplatesSectionCollapsed" :shows-code-input="showsCodeInput"
                :shows-macd-inputs="showsMacdInputs" :shows-technical-indicator-macd-inputs="showsTechnicalIndicatorMacdInputs" :shows-multiplier-input="showsMultiplierInput"
                :shows-moving-average-type-input="showsMovingAverageTypeInput"
                :shows-indicator-variable-name-input="showsIndicatorVariableNameInput"
                :indicator-variable-name-placeholder="indicatorVariableNamePlaceholder"
                :shows-indicator-primary-input-select="showsIndicatorPrimaryInputSelect"
                :shows-indicator-fast-input-select="showsIndicatorFastInputSelect"
                :shows-indicator-slow-input-select="showsIndicatorSlowInputSelect"
                :indicator-getter-options="indicatorGetterOptions"
                :shows-period-input="showsPeriodInput" :shows-threshold-input="showsThresholdInput"
                :shows-condition-mode-input="showsConditionModeInput" :shows-indicator-type-input="showsIndicatorTypeInput"
                :shows-pattern-type-input="showsPatternTypeInput" :shows-lookback-input="showsLookbackInput"
                :shows-place-order-inputs="showsPlaceOrderInputs"
                :shows-place-order-entry-position-policy-input="showsPlaceOrderEntryPositionPolicyInput"
                :shows-place-order-limit-price-input="showsPlaceOrderLimitPriceInput"
                :strategy-templates="strategyTemplates" :updated-at-text="formatTimestamp(definitionForm.updatedAt)"
                @delete-selected-node="deleteSelectedVisualNode" @select-template="createNewDefinitionDraft"
                @close-block-details="isBlockInspectorCollapsed = true" />
        </div>

        <div v-if="showsCodeWorkbench" data-testid="strategy-code-editor-section"
            class="strategy-stage__panel strategy-stage__panel--code"
            :class="{ 'strategy-stage__panel--code-pure': strategyDisplayMode === 'code' }" :style="codePanelStyle">
            <StrategyStageCodeWorkbenchPanel ref="codeWorkbenchRef" :bindings="codeWorkbenchBindings"
                :completion-items="strategyDslEditorCompletions" :extra-libs="strategyDslEditorExtraLibs"
                :hover-items="strategyDslEditorHoverItems" :strategy-display-mode="strategyDisplayMode"
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
        radial-gradient(circle at bottom right, color-mix(in srgb, var(--tv-accent-strong) 14%, transparent), transparent 28%),
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
    background: color-mix(in srgb, var(--tv-bg-surface) 34%, transparent);
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
    border: 1px solid rgba(255, 255, 255, 0.06);
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
    background: color-mix(in srgb, var(--tv-accent) 16%, transparent);
    color: color-mix(in srgb, var(--tv-accent) 74%, var(--tv-text));
}

.strategy-save-dialog {
    display: grid;
    gap: 0.9rem;
    padding: 1.35rem;
    border: 1px solid rgba(15, 23, 42, 0.1);
    border-radius: 1.5rem;
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.98));
    box-shadow: 0 24px 80px rgba(15, 23, 42, 0.18);
    color: var(--tv-text-main);
}

.strategy-save-dialog__eyebrow {
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.18em;
    text-transform: uppercase;
    color: var(--tv-accent-strong);
}

.strategy-save-dialog__title {
    font-size: 1.1rem;
    font-weight: 700;
    color: var(--tv-text-main);
}

.strategy-save-dialog__body {
    line-height: 1.7;
    color: var(--tv-text-subtle);
}

.strategy-save-dialog__hint {
    padding: 0.8rem 0.95rem;
    border-radius: 1rem;
    background: rgba(15, 23, 42, 0.05);
    font-size: 0.85rem;
    color: var(--tv-text-muted);
}

.strategy-save-dialog__actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.75rem;
}

.strategy-save-dialog__linked-list {
    display: grid;
    gap: 0.7rem;
}

.strategy-save-dialog__linked-item {
    display: grid;
    gap: 0.2rem;
    padding: 0.8rem 0.95rem;
    border-radius: 1rem;
    background: rgba(15, 23, 42, 0.05);
}

.strategy-save-dialog__linked-item-id {
    font-size: 0.92rem;
    font-weight: 700;
    color: var(--tv-text-main);
}

.strategy-save-dialog__linked-item-meta {
    font-size: 0.82rem;
    color: var(--tv-text-muted);
}

.strategy-save-dialog__close {
    justify-self: flex-start;
    padding: 0;
    border: 0;
    background: transparent;
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--tv-text-muted);
    cursor: pointer;
}

.strategy-save-dialog__close:hover,
.strategy-save-dialog__close:focus-visible {
    color: var(--tv-text-main);
    outline: none;
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
    background: color-mix(in srgb, var(--tv-bg-elevated) 32%, transparent);
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
    background: color-mix(in srgb, var(--tv-bg-surface) 38%, transparent);
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
    background: color-mix(in srgb, var(--tv-bg-elevated) 28%, transparent);
    color: var(--tv-text-muted);
}

.strategy-stage__toolbar-status-label {
    display: inline-flex;
    align-items: center;
    min-height: 1.9rem;
    padding: 0.25rem 0.65rem;
    border-radius: 999px;
    background: color-mix(in srgb, var(--tv-bg-surface) 48%, transparent);
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
    border-color: color-mix(in srgb, var(--tv-accent) 45%, transparent);
    color: color-mix(in srgb, var(--tv-accent) 70%, var(--tv-text));
}

.strategy-stage__toolbar-status--partial {
    border-color: color-mix(in srgb, var(--tv-warning, var(--card-amber-text)) 52%, transparent);
    color: color-mix(in srgb, var(--tv-warning, var(--card-amber-text)) 78%, var(--tv-text));
}

.strategy-stage__toolbar-status--error {
    border-color: color-mix(in srgb, var(--tv-accent-strong) 55%, transparent);
    color: color-mix(in srgb, var(--tv-accent-strong) 74%, var(--tv-text));
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
    color: var(--tv-accent);
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
