<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import type {
  PineV6WorkflowDocument,
  StrategyDefinitionDocument,
  StrategyInstanceItem,
} from "@/contracts";
import type { components } from "@/generated/openapi";

import { apiGet, apiPost, apiPutPath, fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import { queryClient, queryKeys } from "../composables/serverState";
import { buildPineStrategyDefinitionPayload } from "./strategy-runtime/strategyDefinitionPayload";
import {
  formatStrategyInterval,
  formatStrategyRuntimeRiskSummary,
  formatStrategySymbols,
  readStrategyBinding,
} from "./strategy-runtime/strategyRuntimeInstanceBinding";
import PineSourceCodePane from "./PineSourceCodePane.vue";
import PineSourceStructureBlockList from "./PineSourceStructureBlockList.vue";
import SplitPane from "./shared/SplitPane.vue";
import SplitPaneItem from "./shared/SplitPaneItem.vue";
import {
  buildWorkflowSnapshotFromSource,
  buildPineSourceStructureIndex,
  deleteSourceBlock,
  duplicateSourceBlock,
  insertSourceBlock,
  isPineV6WorkflowBlockKind,
  moveSourceBlock,
  renderBlockToSource,
  replaceSourceRange,
  replaceSourceBlockKind,
  sourceBlockEditableFields,
  updateInstructionBlockParam,
  type PineSourceEditResult,
  type PineSourceBlock,
} from "../features/pineSourceStructureIndex";
import {
  assessPineV6Workflow,
  buildPineV6WorkflowScript,
  createDefaultPineV6Workflow,
  normalizePineV6Workflow,
  type PineV6WorkflowDiagnostic,
} from "../features/pineV6Workflow";
const props = withDefaults(defineProps<{
  entryMode?: "existing" | "new";
  initialDefinitionsCollapsed?: boolean;
}>(), {
  entryMode: "existing",
  initialDefinitionsCollapsed: true,
});

const emit = defineEmits<{
  "definitions-count-change": [count: number];
}>();

interface StrategyPineAnalyzeDiagnostic {
  severity: "error" | "warning" | "info";
  code?: string;
  message: string;
  line: number;
  column: number;
  endLine: number;
  endColumn: number;
}

interface StrategyPineAnalyzeResponse {
  ok: boolean;
  diagnostics?: StrategyPineAnalyzeDiagnostic[];
  features?: string[];
}

type StrategyDefinitionRequest = components["schemas"]["strategy.StrategyDesignDefinition"];
const strategyDefinitionsQueryKey = queryKeys.strategyDefinitions();

function fetchStrategyDefinitions(): Promise<StrategyDefinitionDocument[]> {
  return apiGet<StrategyDefinitionDocument[], "/api/v1/strategy-definitions">(
    "/api/v1/strategy-definitions",
  );
}

const strategyDefinitions = ref<StrategyDefinitionDocument[]>([]);
const strategies = ref<StrategyInstanceItem[]>([]);
const selectedDefinitionId = ref("");
const isLoadingDefinitions = ref(false);
const isLoadingStrategies = ref(false);
const isSavingDefinition = ref(false);
const isAnalyzing = ref(false);
const error = ref("");
const analyzeResult = ref<StrategyPineAnalyzeResponse | null>(null);
const selectedSourceNodeId = ref("");
const expandedSourceNodeId = ref<string | null>(null);
const sourceEditorRef = ref<InstanceType<typeof PineSourceCodePane> | null>(null);
const runtimeRefreshTimer = ref<ReturnType<typeof setInterval> | null>(null);
const actionFeedback = ref<"analyze" | "save" | "">("");
const actionFeedbackTimer = ref<ReturnType<typeof setTimeout> | null>(null);

const workflow = ref<PineV6WorkflowDocument>(createDefaultPineV6Workflow());
const definitionName = ref(workflow.value.declaration.title);
const definitionVersion = ref("0.1.0");
const definitionDescription = ref("Pine v6 原生快捷指令工作台生成的策略。");
const sourceOverride = ref(buildPineV6WorkflowScript(workflow.value));
const useSourceOverride = ref(false);
const strategyDisplayMode = ref<"instruction" | "split" | "code">("split");
const sourceUndoStack = ref<string[]>([]);
const sourceRedoStack = ref<string[]>([]);
const strategySidePanelIds = [
  "definition",
  "declaration",
  "diagnostics",
  "instances",
] as const;
const expandedStrategySidePanels = ref<string[]>([...strategySidePanelIds]);

const generatedScript = computed(() => buildPineV6WorkflowScript(workflow.value));
const activeScript = computed(() => sourceOverride.value);
const sourceStructureNodes = computed(() => buildPineSourceStructureIndex(activeScript.value));
const workflowDiagnostics = computed(() => assessPineV6Workflow(compatibleWorkflowSnapshot()));
const analyzerDiagnostics = computed(() => analyzeResult.value?.diagnostics ?? []);
const pineDiagnosticMarkers = computed(() =>
  analyzerDiagnostics.value.map((diagnostic) => ({
    severity: diagnostic.severity,
    message: diagnostic.message,
    line: diagnostic.line,
    column: diagnostic.column,
    endLine: diagnostic.endLine,
    endColumn: diagnostic.endColumn,
  })),
);
const analyzerErrorCount = computed(() =>
  analyzerDiagnostics.value.filter((diagnostic) => diagnostic.severity === "error").length,
);
const workflowErrorCount = computed(() =>
  workflowDiagnostics.value.filter((diagnostic) => diagnostic.severity === "error").length,
);
const selectedDefinition = computed(() =>
  strategyDefinitions.value.find((definition) => definition.id === selectedDefinitionId.value) ?? null,
);
const readonlyStrategies = computed(() =>
  selectedDefinitionId.value === ""
    ? []
    : strategies.value.filter((strategy) => strategy.definition.strategyId === selectedDefinitionId.value),
);
const sourceStructureSummary = computed(() => {
  const rawCount = sourceStructureNodes.value.filter((node) => node.match.type === "raw").length;
  return `${sourceStructureNodes.value.length} 个源码节点 / ${rawCount} 个 raw 锚点`;
});
const selectedSourceNodeSummary = computed(() => {
  const node = sourceStructureNodes.value.find((item) => item.id === selectedSourceNodeId.value);
  return node === undefined ? (sourceOverride.value === generatedScript.value ? "图块生成" : "源码覆盖") : `L${node.lineRange.start} ${node.label}`;
});
const canUndoSourceChange = computed(() => sourceUndoStack.value.length > 0);
const canRedoSourceChange = computed(() => sourceRedoStack.value.length > 0);

function statusLabel(status: string): string {
  switch (status) {
    case "RUNNING":
      return "运行中";
    case "PAUSED":
      return "已暂停";
    case "STOPPED":
      return "已停止";
    default:
      return status;
  }
}

function setStrategyDisplayMode(mode: "instruction" | "split" | "code"): void {
  strategyDisplayMode.value = mode;
}

function sourceBlockIsEditable(block: PineSourceBlock): boolean {
  return sourceBlockEditableFields(block).length > 0;
}

function addSourceBlock(kind: string): void {
  if (!isPineV6WorkflowBlockKind(kind)) {
    return;
  }
  applySourceEdit(insertSourceBlock(activeScript.value, selectedSourceNodeId.value || null, kind));
}

function changeSourceBlockKind(block: PineSourceBlock, kind: string): void {
  if (!isPineV6WorkflowBlockKind(kind)) {
    return;
  }
  applySourceEdit(replaceSourceBlockKind(activeScript.value, block.id, kind));
}

function deleteSourceStructureBlock(block: PineSourceBlock): void {
  applySourceEdit(deleteSourceBlock(activeScript.value, block.id));
}

function duplicateSourceStructureBlock(block: PineSourceBlock): void {
  applySourceEdit(duplicateSourceBlock(activeScript.value, block.id));
}

function moveSourceStructureBlock(block: PineSourceBlock, direction: -1 | 1): void {
  applySourceEdit(moveSourceBlock(activeScript.value, block.id, direction));
}

function rememberSourceSnapshot(snapshot: string): void {
  const lastSnapshot = sourceUndoStack.value[sourceUndoStack.value.length - 1];
  if (lastSnapshot === snapshot) {
    return;
  }
  sourceUndoStack.value = [...sourceUndoStack.value.slice(-79), snapshot];
  sourceRedoStack.value = [];
}

function commitSourceChange(nextSource: string): void {
  if (nextSource === activeScript.value) {
    return;
  }
  rememberSourceSnapshot(activeScript.value);
  useSourceOverride.value = true;
  sourceOverride.value = nextSource;
}

function resetSourceHistory(): void {
  sourceUndoStack.value = [];
  sourceRedoStack.value = [];
}

function restoreSourceSnapshot(nextSource: string): void {
  useSourceOverride.value = true;
  sourceOverride.value = nextSource;
  selectedSourceNodeId.value = "";
  expandedSourceNodeId.value = null;
}

function undoSourceChange(): void {
  const previousSource = sourceUndoStack.value[sourceUndoStack.value.length - 1];
  if (previousSource === undefined) {
    return;
  }
  sourceUndoStack.value = sourceUndoStack.value.slice(0, -1);
  sourceRedoStack.value = [...sourceRedoStack.value, activeScript.value];
  restoreSourceSnapshot(previousSource);
}

function redoSourceChange(): void {
  const nextSource = sourceRedoStack.value[sourceRedoStack.value.length - 1];
  if (nextSource === undefined) {
    return;
  }
  sourceRedoStack.value = sourceRedoStack.value.slice(0, -1);
  sourceUndoStack.value = [...sourceUndoStack.value, activeScript.value];
  restoreSourceSnapshot(nextSource);
}

function applySourceEdit(result: PineSourceEditResult): void {
  if (!result.changed) {
    return;
  }
  commitSourceChange(result.source);
  selectedSourceNodeId.value = "";
  expandedSourceNodeId.value = null;
}

function toggleSourceBlockExpansion(block: PineSourceBlock): void {
  selectedSourceNodeId.value = block.id;
  expandedSourceNodeId.value = expandedSourceNodeId.value === block.id ? null : block.id;
  sourceEditorRef.value?.revealOffsetRange({
    start: block.sourceRange.start,
    end: Math.max(block.sourceRange.start + 1, block.sourceRange.end),
  });
}

function updateSourceBlockField(block: PineSourceBlock, key: string, value: unknown): void {
  if (!sourceBlockIsEditable(block)) {
    return;
  }
  const nextBlock = updateInstructionBlockParam(block, key, value);
  const nextSource = replaceSourceRange(activeScript.value, block.sourceRange, renderBlockToSource(nextBlock));
  commitSourceChange(nextSource);
  selectedSourceNodeId.value = block.id;
  expandedSourceNodeId.value = block.id;
}

function compatibleWorkflowSnapshot(): PineV6WorkflowDocument {
  return buildWorkflowSnapshotFromSource(activeScript.value, workflow.value);
}

onMounted(() => {
  void loadStrategyDefinitions(selectedDefinitionId.value, { applyDefinition: props.entryMode !== "new" });
  void loadStrategies();
  runtimeRefreshTimer.value = setInterval(() => {
    void loadStrategies();
  }, 3000);
});

onBeforeUnmount(() => {
  if (runtimeRefreshTimer.value !== null) {
    clearInterval(runtimeRefreshTimer.value);
  }
  if (actionFeedbackTimer.value !== null) {
    clearTimeout(actionFeedbackTimer.value);
  }
});

function showActionFeedback(kind: "analyze" | "save"): void {
  actionFeedback.value = kind;
  if (actionFeedbackTimer.value !== null) {
    clearTimeout(actionFeedbackTimer.value);
  }
  actionFeedbackTimer.value = setTimeout(() => {
    actionFeedback.value = "";
    actionFeedbackTimer.value = null;
  }, 1600);
}

watch(sourceOverride, () => {
  syncWorkflowDeclarationFromSource();
});

async function loadStrategyDefinitions(
  preferredId = selectedDefinitionId.value,
  options: { applyDefinition?: boolean } = {},
): Promise<void> {
  isLoadingDefinitions.value = true;
  error.value = "";
  try {
    const definitions = await queryClient.ensureQueryData({
      queryKey: strategyDefinitionsQueryKey,
      queryFn: fetchStrategyDefinitions,
    });
    strategyDefinitions.value = definitions;
    emit("definitions-count-change", definitions.length);
    const next = definitions.find((definition) => definition.id === preferredId) ?? definitions[0] ?? null;
    if (options.applyDefinition !== false && next !== null) {
      applyDefinition(next);
    }
  } catch (cause) {
    error.value = `加载策略定义失败: ${cause instanceof Error ? cause.message : String(cause)}`;
  } finally {
    isLoadingDefinitions.value = false;
  }
}

async function loadStrategies(): Promise<void> {
  isLoadingStrategies.value = true;
  try {
    const items = await fetchEnvelope<StrategyInstanceItem[]>("/api/v1/strategies");
    strategies.value = items;
  } catch {
    strategies.value = [];
  } finally {
    isLoadingStrategies.value = false;
  }
}

function applyDefinition(definition: StrategyDefinitionDocument): void {
  selectedDefinitionId.value = definition.id;
  definitionName.value = definition.name;
  definitionVersion.value = definition.version;
  definitionDescription.value = definition.description;
  workflow.value = normalizePineV6Workflow(definition.visualModel);
  sourceOverride.value = definition.script || generatedScript.value;
  useSourceOverride.value = false;
  resetSourceHistory();
  analyzeResult.value = null;
}

function createNewWorkflow(): void {
  selectedDefinitionId.value = "";
  workflow.value = createDefaultPineV6Workflow();
  definitionName.value = workflow.value.declaration.title;
  definitionVersion.value = "0.1.0";
  definitionDescription.value = "Pine v6 原生快捷指令工作台生成的策略。";
  useSourceOverride.value = false;
  sourceOverride.value = generatedScript.value;
  resetSourceHistory();
  analyzeResult.value = null;
}

function updateDeclaration<K extends keyof PineV6WorkflowDocument["declaration"]>(
  key: K,
  value: PineV6WorkflowDocument["declaration"][K],
): void {
  const nextWorkflow = {
    ...workflow.value,
    declaration: {
      ...workflow.value.declaration,
      [key]: value,
    },
  };
  const strategyBlock = sourceStructureNodes.value.find((block) => block.match.type === "strategy");
  workflow.value = nextWorkflow;
  if (strategyBlock !== undefined) {
    const nextBlock = updateInstructionBlockParam(strategyBlock, String(key), value);
    commitSourceChange(replaceSourceRange(activeScript.value, strategyBlock.sourceRange, renderBlockToSource(nextBlock)));
  } else {
    commitSourceChange(buildPineV6WorkflowScript(nextWorkflow));
  }
  if (key === "title" && definitionName.value.trim() === "") {
    definitionName.value = String(value);
  }
}

function syncWorkflowDeclarationFromSource(): void {
  workflow.value = {
    ...workflow.value,
    declaration: compatibleWorkflowSnapshot().declaration,
  };
}

async function analyzeCurrentScript(): Promise<boolean> {
  isAnalyzing.value = true;
  actionFeedback.value = "";
  error.value = "";
  try {
    const result = await fetchEnvelopeWithInit<StrategyPineAnalyzeResponse>(
      "/api/v1/strategy-pine/analyze",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          script: activeScript.value,
          sourceFormat: "pine-v6",
          includeAst: false,
        }),
      },
    );
    analyzeResult.value = result;
    if (!result.ok || (result.diagnostics ?? []).some((diagnostic) => diagnostic.severity === "error")) {
      error.value = "Pine v6 分析未通过，请先处理错误诊断。";
      return false;
    }
    showActionFeedback("analyze");
    return true;
  } catch (cause) {
    analyzeResult.value = {
      ok: false,
      diagnostics: [{
        severity: "error",
        message: cause instanceof Error ? cause.message : String(cause),
        line: 1,
        column: 1,
        endLine: 1,
        endColumn: 2,
      }],
      features: [],
    };
    error.value = `Pine v6 分析失败: ${cause instanceof Error ? cause.message : String(cause)}`;
    return false;
  } finally {
    isAnalyzing.value = false;
  }
}

async function saveDefinition(options: { requireAnalysis?: boolean } = {}): Promise<StrategyDefinitionDocument | null> {
  if (options.requireAnalysis === true && !await analyzeCurrentScript()) {
    return null;
  }
  isSavingDefinition.value = true;
  actionFeedback.value = "";
  error.value = "";
  try {
    const payload = buildPineStrategyDefinitionPayload({
      id: selectedDefinitionId.value,
      name: definitionName.value.trim() || workflow.value.declaration.title || "Pine v6 策略",
      version: definitionVersion.value.trim() || "0.1.0",
      description: definitionDescription.value.trim(),
      script: activeScript.value,
      visualModel: compatibleWorkflowSnapshot(),
      createdAt: selectedDefinition.value?.createdAt ?? "",
      updatedAt: selectedDefinition.value?.updatedAt ?? "",
    }) as StrategyDefinitionRequest;
    const existing = strategyDefinitions.value.some((definition) => definition.id === selectedDefinitionId.value);
    const saved = existing
      ? await apiPutPath<StrategyDefinitionDocument, "/api/v1/strategy-definitions/{definitionId}">(
        "/api/v1/strategy-definitions/{definitionId}",
        `/api/v1/strategy-definitions/${encodeURIComponent(selectedDefinitionId.value)}`,
        payload,
      )
      : await apiPost<StrategyDefinitionDocument, "/api/v1/strategy-definitions">(
        "/api/v1/strategy-definitions",
        payload,
      );
    selectedDefinitionId.value = saved.id;
    queryClient.setQueryData<StrategyDefinitionDocument[]>(
      strategyDefinitionsQueryKey,
      (current) => {
        const next = current?.filter((definition) => definition.id !== saved.id) ?? [];
        return [...next, saved];
      },
    );
    await queryClient.invalidateQueries({ queryKey: strategyDefinitionsQueryKey, refetchType: "none" });
    await loadStrategyDefinitions(saved.id);
    showActionFeedback("save");
    return saved;
  } catch (cause) {
    error.value = `保存策略定义失败: ${cause instanceof Error ? cause.message : String(cause)}`;
    return null;
  } finally {
    isSavingDefinition.value = false;
  }
}

function diagnosticClass(diagnostic: Pick<PineV6WorkflowDiagnostic | StrategyPineAnalyzeDiagnostic, "severity">): string {
  return `strategy-native-diagnostic--${diagnostic.severity}`;
}

function statusClass(status: string): string {
  switch (status) {
    case "RUNNING":
      return "strategy-native-status--running";
    case "PAUSED":
      return "strategy-native-status--paused";
    default:
      return "strategy-native-status--stopped";
  }
}
</script>

<template>
  <div class="strategy-native-page" data-testid="strategy-design-stage">
    <header class="strategy-native-header">
      <div>
        <div class="strategy-native-eyebrow">Pine v6 原生</div>
        <h1>策略快捷指令工作台</h1>
      </div>
      <div class="strategy-native-header__actions">
        <div class="strategy-native-history-actions" aria-label="源码历史">
          <button
            type="button"
            class="strategy-native-history-button"
            :disabled="!canUndoSourceChange"
            data-testid="strategy-source-undo"
            title="撤回"
            aria-label="撤回"
            @click="undoSourceChange"
          >
            <v-icon size="13">fa-solid fa-arrow-rotate-left</v-icon>
          </button>
          <button
            type="button"
            class="strategy-native-history-button"
            :disabled="!canRedoSourceChange"
            data-testid="strategy-source-redo"
            title="重做"
            aria-label="重做"
            @click="redoSourceChange"
          >
            <v-icon size="13">fa-solid fa-arrow-rotate-right</v-icon>
          </button>
        </div>
        <div class="strategy-native-view-switch" aria-label="策略工作区视图">
          <button
            class="strategy-native-view-switch__button"
            :class="{ 'is-active': strategyDisplayMode === 'instruction' }"
            data-testid="strategy-display-mode-instruction"
            type="button"
            @click="setStrategyDisplayMode('instruction')"
          >
            指令
          </button>
          <button
            class="strategy-native-view-switch__button"
            :class="{ 'is-active': strategyDisplayMode === 'split' }"
            data-testid="strategy-display-mode-split"
            type="button"
            @click="setStrategyDisplayMode('split')"
          >
            双栏
          </button>
          <button
            class="strategy-native-view-switch__button"
            :class="{ 'is-active': strategyDisplayMode === 'code' }"
            data-testid="strategy-display-mode-code"
            type="button"
            @click="setStrategyDisplayMode('code')"
          >
            代码
          </button>
        </div>
        <button type="button" @click="createNewWorkflow">新建 Pine v6</button>
        <button type="button" :disabled="isAnalyzing" @click="void analyzeCurrentScript()">
          {{ isAnalyzing ? "分析中" : actionFeedback === "analyze" ? "已分析" : "分析" }}
        </button>
        <button type="button" :disabled="isSavingDefinition" @click="void saveDefinition()">
          {{ isSavingDefinition ? "保存中" : actionFeedback === "save" ? "已保存" : "保存" }}
        </button>
      </div>
    </header>

    <div v-if="error" class="strategy-native-banner strategy-native-banner--error">{{ error }}</div>

    <SplitPane class="strategy-native-shell" :pane-min-size="20">
      <SplitPaneItem
        :size="strategyDisplayMode === 'instruction' ? 100 : strategyDisplayMode === 'split' ? 66 : 30"
        :min-size="strategyDisplayMode === 'instruction' ? 100 : 22"
        :max-size="strategyDisplayMode === 'instruction' ? 100 : strategyDisplayMode === 'code' ? 48 : 78"
      >
        <SplitPane class="strategy-native-instruction" :pane-min-size="18">
          <SplitPaneItem
            :size="strategyDisplayMode === 'code' ? 100 : 32"
            :min-size="strategyDisplayMode === 'code' ? 100 : 22"
            :max-size="strategyDisplayMode === 'code' ? 100 : 52"
          >
            <aside class="strategy-native-side">
              <v-expansion-panels
                v-model="expandedStrategySidePanels"
                multiple
                class="strategy-native-side-panels"
                variant="default"
              >
                <v-expansion-panel value="definition" class="strategy-native-panel strategy-native-side-panel">
                  <v-expansion-panel-title>
                    <div class="strategy-native-panel__title">策略定义</div>
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="strategy-native-panel__content">
                      <select v-model="selectedDefinitionId" :disabled="isLoadingDefinitions" @change="selectedDefinition && applyDefinition(selectedDefinition)">
                        <option value="">新建草稿</option>
                        <option v-for="definition in strategyDefinitions" :key="definition.id" :value="definition.id">
                          {{ definition.name }} / v{{ definition.version }}
                        </option>
                      </select>
                      <label>
                        <span>名称</span>
                        <input v-model="definitionName">
                      </label>
                      <label>
                        <span>版本</span>
                        <input v-model="definitionVersion">
                      </label>
                      <label>
                        <span>说明</span>
                        <textarea v-model="definitionDescription" rows="3" />
                      </label>
                    </div>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel value="declaration" class="strategy-native-panel strategy-native-side-panel">
                  <v-expansion-panel-title>
                    <div class="strategy-native-panel__title">策略声明</div>
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="strategy-native-panel__content">
                      <label>
                        <span>标题</span>
                        <input
                          data-testid="strategy-declaration-title"
                          :value="workflow.declaration.title"
                          @input="updateDeclaration('title', ($event.target as HTMLInputElement).value)"
                        >
                      </label>
                      <label class="strategy-native-toggle">
                        <input :checked="workflow.declaration.overlay" type="checkbox" @change="updateDeclaration('overlay', ($event.target as HTMLInputElement).checked)">
                        <span>叠加到主图</span>
                      </label>
                      <label>
                        <span>初始资金</span>
                        <input :value="workflow.declaration.initialCapital ?? ''" type="number" @input="updateDeclaration('initialCapital', Number(($event.target as HTMLInputElement).value) || null)">
                      </label>
                      <label>
                        <span>币种</span>
                        <input :value="workflow.declaration.currency ?? ''" @input="updateDeclaration('currency', ($event.target as HTMLInputElement).value)">
                      </label>
                      <label>
                        <span>允许加仓次数</span>
                        <input :value="workflow.declaration.pyramiding ?? 0" type="number" @input="updateDeclaration('pyramiding', Number(($event.target as HTMLInputElement).value) || 0)">
                      </label>
                    </div>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel value="diagnostics" class="strategy-native-panel strategy-native-side-panel">
                  <v-expansion-panel-title>
                    <div class="strategy-native-panel__title">诊断</div>
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="strategy-native-panel__content">
                      <div v-if="workflowDiagnostics.length === 0 && analyzerDiagnostics.length === 0" class="strategy-native-meta">
                        暂无诊断。
                      </div>
                      <div
                        v-for="diagnostic in workflowDiagnostics"
                        :key="`${diagnostic.code}-${diagnostic.blockId ?? ''}`"
                        class="strategy-native-diagnostic"
                        :class="diagnosticClass(diagnostic)"
                      >
                        <strong>{{ diagnostic.code }}</strong>
                        <span>{{ diagnostic.message }}</span>
                      </div>
                      <div
                        v-for="diagnostic in analyzerDiagnostics"
                        :key="`${diagnostic.line}-${diagnostic.column}-${diagnostic.message}`"
                        class="strategy-native-diagnostic"
                        :class="diagnosticClass(diagnostic)"
                      >
                        <strong>{{ diagnostic.code ?? diagnostic.severity }}</strong>
                        <span>第 {{ diagnostic.line }} 行：{{ diagnostic.message }}</span>
                      </div>
                      <div class="strategy-native-meta">
                        工作流错误 {{ workflowErrorCount }} 个 / Pine 分析错误 {{ analyzerErrorCount }} 个
                      </div>
                    </div>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel value="instances" class="strategy-native-panel strategy-native-side-panel">
                  <v-expansion-panel-title>
                    <div class="strategy-native-workspace-bar strategy-native-side-panel__titlebar">
                      <div class="strategy-native-panel__title">策略实例</div>
                      <button
                        type="button"
                        class="strategy-native-icon-button"
                        :disabled="isLoadingStrategies"
                        title="刷新"
                        aria-label="刷新策略实例"
                        @click.stop="void loadStrategies()"
                      >
                        <v-icon size="13">fa-solid fa-arrow-rotate-right</v-icon>
                      </button>
                    </div>
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="strategy-native-panel__content">
                      <div v-if="readonlyStrategies.length === 0" class="strategy-native-meta">暂无实例。</div>
                      <section
                        v-for="strategy in readonlyStrategies"
                        :key="strategy.id"
                        class="strategy-native-instance"
                      >
                        <div>
                          <strong>{{ strategy.definition.name }}</strong>
                          <span :class="['strategy-native-status', statusClass(strategy.status)]">{{ statusLabel(strategy.status) }}</span>
                        </div>
                        <div>{{ formatStrategySymbols(strategy) }} / {{ formatStrategyInterval(strategy) }}</div>
                        <div>{{ formatStrategyRuntimeRiskSummary(readStrategyBinding(strategy).runtimeRisk) }}</div>
                      </section>
                    </div>
                  </v-expansion-panel-text>
                </v-expansion-panel>
              </v-expansion-panels>
            </aside>
          </SplitPaneItem>

          <SplitPaneItem
            v-if="strategyDisplayMode !== 'code'"
            :size="68"
            :min-size="38"
            :max-size="78"
          >
            <main class="strategy-native-main">
              <section class="strategy-native-panel strategy-native-panel--workspace">
                <div class="strategy-native-workspace-bar">
                  <div>
                    <div class="strategy-native-panel__title">结构指令</div>
                    <div class="strategy-native-meta">
                      收盘确认执行 / 下一根 K 线成交 / {{ sourceStructureSummary }}
                    </div>
                  </div>
                  <div class="strategy-native-selected-block">{{ selectedSourceNodeSummary }}</div>
                </div>
                <div class="strategy-native-block-scroll" data-testid="strategy-instruction-scroll">
                  <PineSourceStructureBlockList
                    :nodes="sourceStructureNodes"
                    :selected-id="selectedSourceNodeId"
                    :expanded-id="expandedSourceNodeId"
                    @toggle-block="toggleSourceBlockExpansion"
                    @add-block="addSourceBlock"
                    @change-kind="changeSourceBlockKind"
                    @delete-block="deleteSourceStructureBlock"
                    @duplicate-block="duplicateSourceStructureBlock"
                    @move-block="moveSourceStructureBlock"
                    @update-field="updateSourceBlockField"
                  />
                </div>
              </section>
            </main>
          </SplitPaneItem>
        </SplitPane>
      </SplitPaneItem>

      <SplitPaneItem
        v-if="strategyDisplayMode !== 'instruction'"
        :size="strategyDisplayMode === 'split' ? 34 : 70"
        :min-size="strategyDisplayMode === 'split' ? 22 : 52"
        :max-size="100"
      >
        <PineSourceCodePane
          ref="sourceEditorRef"
          :model-value="activeScript"
          :source-editing-enabled="useSourceOverride"
          :diagnostic-markers="pineDiagnosticMarkers"
          @update:model-value="commitSourceChange"
          @update:source-editing-enabled="useSourceOverride = $event"
        />
      </SplitPaneItem>
    </SplitPane>
  </div>
</template>

<style scoped>
.strategy-native-page {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  overflow: hidden;
  padding: 0.75rem;
  color: var(--tv-text);
}

.strategy-native-header,
.strategy-native-shell,
.strategy-native-panel,
.strategy-native-workspace-bar {
  min-width: 0;
}

.strategy-native-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.strategy-native-header h1 {
  margin: 0.1rem 0 0;
  font-size: 1.35rem;
  line-height: 1.2;
}

.strategy-native-eyebrow,
.strategy-native-panel__title {
  color: var(--tv-text-muted);
  font-size: 0.72rem;
  font-weight: 800;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.strategy-native-header__actions,
.strategy-native-workspace-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem;
  justify-content: space-between;
}

.strategy-native-header__actions {
  justify-content: flex-end;
}

.strategy-native-history-actions {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
}

.strategy-native-history-button {
  display: inline-grid;
  place-items: center;
  width: 1.85rem;
  height: 1.85rem;
  border-color: transparent;
  background: transparent;
  color: var(--tv-text-muted);
  padding: 0;
}

.strategy-native-history-button:hover:not(:disabled) {
  border-color: transparent;
  background: transparent;
  color: var(--tv-text);
}

.strategy-native-history-button:disabled {
  border-color: transparent;
  background: transparent;
  color: var(--tv-text-dim);
  cursor: not-allowed;
  opacity: 0.5;
}

.strategy-native-view-switch {
  display: inline-grid;
  grid-template-columns: repeat(3, minmax(2.9rem, 1fr));
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 72%, transparent);
  padding: 0.18rem;
}

.strategy-native-view-switch__button {
  display: inline-grid;
  place-items: center;
  min-width: 2.9rem;
  min-height: 2rem;
  border: 0;
  border-radius: 999px;
  background: transparent;
  padding: 0.35rem 0.65rem;
  color: var(--tv-text-muted);
  font-size: 0.8rem;
  font-weight: 800;
  line-height: 1;
  white-space: nowrap;
}

.strategy-native-view-switch__button.is-active {
  background: color-mix(in srgb, var(--tv-accent) 22%, var(--tv-bg-surface));
  color: var(--tv-text);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tv-accent) 36%, transparent);
}

.strategy-native-shell {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 0.5rem;
  background: color-mix(in srgb, var(--tv-bg-elevated) 42%, transparent);
}

.strategy-native-instruction {
  min-height: 0;
  height: 100%;
  overflow: hidden;
}

.strategy-native-side,
.strategy-native-main {
  width: 100%;
  min-width: 0;
  min-height: 0;
  height: 100%;
  overflow: auto;
  display: grid;
  align-content: start;
  gap: 0.75rem;
  padding: 0.75rem;
}

.strategy-native-main {
  overflow: hidden;
  grid-template-rows: minmax(0, 1fr) auto;
  align-content: stretch;
}

.strategy-native-side-panels {
  display: grid !important;
  grid-template-columns: minmax(0, 1fr);
  justify-items: stretch;
  gap: 0.75rem;
  width: 100% !important;
  inline-size: 100% !important;
  min-width: 0;
  max-width: none !important;
}

.strategy-native-side-panels :deep(.v-expansion-panel-title__overlay),
.strategy-native-side-panels :deep(.v-expansion-panel__overlay) {
  display: none;
}

.strategy-native-side-panels :deep(.v-expansion-panel-title) {
  width: 100% !important;
  inline-size: 100% !important;
  max-width: none !important;
  min-height: 3.25rem;
  padding: 0.85rem;
  color: var(--tv-text);
}

.strategy-native-side-panels :deep(.v-expansion-panel) {
  flex: 1 1 100% !important;
  justify-self: stretch;
  width: 100% !important;
  inline-size: 100% !important;
  min-width: 0;
  max-width: none !important;
}

.strategy-native-side-panels :deep(.v-expansion-panel-title__icon) {
  color: var(--tv-text-muted);
}

.strategy-native-side-panels :deep(.v-expansion-panel-text__wrapper) {
  width: 100%;
  max-width: none;
  padding: 0;
}

.strategy-native-panel {
  display: grid;
  gap: 0.75rem;
  border: 1px solid var(--tv-border);
  border-radius: 0.5rem;
  background: color-mix(in srgb, var(--tv-bg-surface) 96%, transparent);
  padding: 0.85rem;
}

.strategy-native-side-panel {
  display: block;
  flex-basis: 100% !important;
  justify-self: stretch;
  width: 100% !important;
  inline-size: 100% !important;
  min-width: 0;
  max-width: none !important;
  gap: 0;
  overflow: hidden;
  padding: 0;
  color: var(--tv-text);
}

.strategy-native-side-panel + .strategy-native-side-panel {
  margin-top: 0;
}

.strategy-native-side-panel__titlebar {
  width: 100%;
}

.strategy-native-icon-button {
  display: inline-grid;
  place-items: center;
  width: 1.85rem;
  height: 1.85rem;
  border: 1px solid var(--tv-border);
  border-radius: 0.45rem;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 0;
}

.strategy-native-icon-button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.strategy-native-panel__content {
  display: grid;
  gap: 0.75rem;
  padding: 0.75rem;
}

.strategy-native-panel--workspace {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  grid-template-rows: auto minmax(0, 1fr);
}

.strategy-native-panel--workspace {
  border: 0;
  border-radius: 0;
  background: transparent;
  padding: 0;
}

.strategy-native-block-scroll {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  align-content: start;
  justify-items: stretch;
  width: 100%;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  scrollbar-gutter: auto;
}

.strategy-native-panel label {
  display: grid;
  gap: 0.25rem;
  color: var(--tv-text-muted);
  font-size: 0.72rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.strategy-native-panel input,
.strategy-native-panel select,
.strategy-native-panel textarea {
  width: 100%;
  min-width: 0;
  border: 1px solid var(--tv-border);
  border-radius: 0.45rem;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 0.5rem 0.6rem;
  font-size: 0.85rem;
  line-height: 1.35;
  outline: none;
}

.strategy-native-panel label > select {
  appearance: none;
  padding-right: 2rem;
  background: var(--tv-bg-surface)
    url("data:image/svg+xml,%3Csvg width='12' height='12' viewBox='0 0 12 12' fill='none' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M3 4.5L6 7.5L9 4.5' stroke='%23A9B7CC' stroke-width='1.7' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E")
    no-repeat right 0.75rem center / 0.75rem 0.75rem;
}

.strategy-native-toggle {
  display: inline-flex !important;
  grid-template-columns: auto 1fr;
  align-items: center;
  gap: 0.5rem;
  text-transform: none !important;
  letter-spacing: 0 !important;
}

.strategy-native-toggle input {
  width: auto;
}

button {
  border: 1px solid var(--tv-border);
  border-radius: 0.45rem;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 0.48rem 0.75rem;
  font-size: 0.85rem;
  font-weight: 700;
}

button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.strategy-native-banner {
  flex-shrink: 0;
  border-radius: 0.5rem;
  padding: 0.65rem 0.8rem;
  font-size: 0.9rem;
}

.strategy-native-banner--error {
  border: 1px solid color-mix(in srgb, #ef4444 52%, var(--tv-border));
  background: color-mix(in srgb, #ef4444 12%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fca5a5 72%, var(--tv-text));
}

.strategy-native-meta,
.strategy-native-selected-block {
  color: var(--tv-text-muted);
  font-size: 0.82rem;
  line-height: 1.45;
}

.strategy-native-diagnostic {
  display: grid;
  gap: 0.2rem;
  border-radius: 0.45rem;
  border: 1px solid var(--tv-border);
  padding: 0.55rem 0.65rem;
  font-size: 0.82rem;
}

.strategy-native-diagnostic--error {
  border-color: color-mix(in srgb, #ef4444 48%, var(--tv-border));
  background: color-mix(in srgb, #ef4444 12%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fca5a5 72%, var(--tv-text));
}

.strategy-native-diagnostic--warning {
  border-color: color-mix(in srgb, #f59e0b 48%, var(--tv-border));
  background: color-mix(in srgb, #f59e0b 12%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fbbf24 70%, var(--tv-text));
}

.strategy-native-diagnostic--info {
  border-color: color-mix(in srgb, var(--tv-accent) 44%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, var(--tv-accent) 74%, var(--tv-text));
}

.strategy-native-instance {
  display: grid;
  width: 100%;
  gap: 0.4rem;
  text-align: left;
}

.strategy-native-instance > div:first-child {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.strategy-native-status {
  border-radius: 999px;
  padding: 0.15rem 0.45rem;
  font-size: 0.7rem;
}

.strategy-native-status--running {
  background: color-mix(in srgb, #22c55e 18%, var(--tv-bg-surface));
  color: color-mix(in srgb, #86efac 72%, var(--tv-text));
}
.strategy-native-status--paused {
  background: color-mix(in srgb, #f59e0b 18%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fbbf24 72%, var(--tv-text));
}
.strategy-native-status--stopped {
  background: color-mix(in srgb, var(--tv-text-muted) 18%, var(--tv-bg-surface));
  color: var(--tv-text-muted);
}

@media (max-width: 860px) {
  .strategy-native-header {
    grid-template-columns: 1fr;
    align-items: stretch;
  }
}
</style>
