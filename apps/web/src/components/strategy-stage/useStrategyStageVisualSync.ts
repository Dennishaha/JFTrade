import type {
    StrategyDefinitionDocument,
    StrategyVisualModelDocument,
    StrategyVisualNodeDocument,
} from "@/contracts";
import { computed, onBeforeUnmount, ref, watch, type Ref } from "vue";

import { reconcileStrategyVisualModelIndicatorBindings } from "../../features/strategyVisualBuilderIndicatorReferences";
import {
    buildStrategyVisualModelFromScript,
    cloneStrategyVisualModel,
    createDefaultStrategyVisualModel,
} from "../../features/strategyVisualBuilder";

type StrategyVisualSyncStatus = "ready" | "syncing" | "synced" | "partial" | "error";

interface CodeOffsetRange {
    start: number;
    end: number;
}

interface StrategyStageCodeWorkbenchHandle {
    revealCodeRange(range: CodeOffsetRange): void;
}

interface StrategyStageLogicFlowHandle {
    selectNodeById(nodeId: string | null): void;
}

interface UseStrategyStageVisualSyncOptions {
    definitionForm: Ref<StrategyDefinitionDocument>;
    definitionError: Ref<string>;
    definitionNotice: Ref<string>;
    lastCommittedDefinitionSignature: Ref<string>;
    clearDefinitionMessages: () => void;
    pickInitialVisualNodeId: (model: StrategyVisualModelDocument) => string;
    buildScriptForModel: (model: StrategyVisualModelDocument | null | undefined) => string;
    serializeDefinitionSnapshot: (definition: StrategyDefinitionDocument) => string;
    codeWorkbenchRef: Ref<StrategyStageCodeWorkbenchHandle | null>;
    logicFlowDesignerRef: Ref<StrategyStageLogicFlowHandle | null>;
    showBlockInspector: () => void;
}

const SCRIPT_TO_VISUAL_SYNC_DELAY = 650;

export function useStrategyStageVisualSync(options: UseStrategyStageVisualSyncOptions) {
    const visualSyncStatus = ref<StrategyVisualSyncStatus>("ready");
    const visualSyncMessage = ref("图形与 Pine 会自动异步同步。\n修改 Pine 后会尝试反解回流程图。");
    const visualSyncCodeBlockCount = ref(0);
    const selectedVisualNodeId = ref("");
    const codeContextNodeId = ref("");

    let scriptToVisualSyncTimer: ReturnType<typeof setTimeout> | null = null;
    let skipScriptToVisualSyncCount = 0;
    let isCodeOriginatedSelection = false;

    const resolvedVisualModel = computed(
        () => options.definitionForm.value.visualModel ?? createDefaultStrategyVisualModel(),
    );

    const selectedVisualNode = computed<StrategyVisualNodeDocument | null>(() => {
        const nodeId = selectedVisualNodeId.value;
        if (nodeId === "") {
            return null;
        }
        return resolvedVisualModel.value.nodes.find((node) => node.id === nodeId) ?? null;
    });

    function updateCommittedDefinitionSignature(definition: StrategyDefinitionDocument): void {
        options.lastCommittedDefinitionSignature.value = options.serializeDefinitionSnapshot(definition);
    }

    function setVisualSyncState(
        status: StrategyVisualSyncStatus,
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
            "图形与 Pine 会自动同步。修改 Pine 后会尝试反解回流程图。",
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
        optionsWithSkip?: {
            skipScriptParse?: boolean;
        },
    ): void {
        if (
            optionsWithSkip?.skipScriptParse === true &&
            nextDefinition.script !== options.definitionForm.value.script
        ) {
            markNextScriptSyncAsInternal();
        }

        options.definitionForm.value = nextDefinition;
    }

    function scheduleScriptToVisualModelSync(delay = SCRIPT_TO_VISUAL_SYNC_DELAY): void {
        clearPendingScriptToVisualSync();
        setVisualSyncState("syncing", "正在把 Pine 异步转换回流程图…", visualSyncCodeBlockCount.value);

        scriptToVisualSyncTimer = setTimeout(() => {
            scriptToVisualSyncTimer = null;
            syncScriptToVisualModelNow();
        }, delay);
    }

    function syncScriptToVisualModelNow(optionsWithCommit?: { updateCommittedSignature?: boolean }): void {
        clearPendingScriptToVisualSync();

        const script = options.definitionForm.value.script;
        if (script.trim() === "") {
            setVisualSyncState("error", "Pine 为空，无法转换回流程图。已保留当前流程图。", 0);
            return;
        }

        setVisualSyncState("syncing", "正在把 Pine 异步转换回流程图…", visualSyncCodeBlockCount.value);

        const parseResult = buildStrategyVisualModelFromScript(
            script,
            options.definitionForm.value.visualModel,
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
                ...options.definitionForm.value,
                visualModel: nextVisualModel,
            },
            {
                skipScriptParse: false,
            },
        );
        selectedVisualNodeId.value = options.pickInitialVisualNodeId(nextVisualModel);

        if (optionsWithCommit?.updateCommittedSignature === true) {
            updateCommittedDefinitionSignature(options.definitionForm.value);
        }

        if (parseResult.codeBlockCount > 0) {
            setVisualSyncState(
                "partial",
                `Pine 已同步回流程图，其中 ${parseResult.codeBlockCount} 段无法标准化。`,
                parseResult.codeBlockCount,
            );
            return;
        }

        setVisualSyncState("synced", "Pine 已同步回流程图。", 0);
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

    function applyVisualModel(
        model: StrategyVisualModelDocument,
        applyOptions?: {
            preserveSelection?: boolean;
            notice?: string;
        },
    ): void {
        const nextModel = reconcileStrategyVisualModelIndicatorBindings(
            cloneStrategyVisualModel(model) ?? createDefaultStrategyVisualModel(),
        );
        const nextSelectedNodeId =
            applyOptions?.preserveSelection === true &&
                nextModel.nodes.some((node) => node.id === selectedVisualNodeId.value)
                ? selectedVisualNodeId.value
                : options.pickInitialVisualNodeId(nextModel);
        const nextScript = options.buildScriptForModel(nextModel);

        replaceDefinitionForm(
            {
                ...options.definitionForm.value,
                visualModel: nextModel,
                script: nextScript,
            },
            {
                skipScriptParse: true,
            },
        );
        selectedVisualNodeId.value = nextSelectedNodeId;
        setVisualSyncState("synced", "流程图已异步同步到 Pine。", visualSyncCodeBlockCount.value);

        if (applyOptions?.notice) {
            options.definitionNotice.value = applyOptions.notice;
        }
    }

    function handleVisualModelUpdated(model: StrategyVisualModelDocument): void {
        options.clearDefinitionMessages();
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
        options.logicFlowDesignerRef.value?.selectNodeById(null);
    }

    function handleCodeContextOpenInspector(): void {
        const nodeId = codeContextNodeId.value;
        if (nodeId === "") {
            return;
        }

        codeContextNodeId.value = "";
        isCodeOriginatedSelection = true;
        options.logicFlowDesignerRef.value?.selectNodeById(nodeId);
        isCodeOriginatedSelection = false;
        selectedVisualNodeId.value = nodeId;
        options.showBlockInspector();
    }

    function handleVisualNodeSelected(nodeId: string | null): void {
        selectedVisualNodeId.value = nodeId ?? "";

        if (isCodeOriginatedSelection) {
            isCodeOriginatedSelection = false;
            return;
        }

        if (nodeId !== null) {
            options.showBlockInspector();
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

    function mutateSelectedVisualNode(
        mutator: (node: StrategyVisualNodeDocument) => StrategyVisualNodeDocument,
    ): void {
        const currentModel =
            cloneStrategyVisualModel(options.definitionForm.value.visualModel) ??
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
            cloneStrategyVisualModel(options.definitionForm.value.visualModel) ??
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

    watch(
        () => options.definitionForm.value.script,
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

            options.codeWorkbenchRef.value?.revealCodeRange(codeRange);
        },
    );

    onBeforeUnmount(() => {
        clearPendingScriptToVisualSync();
    });

    return {
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
    };
}
