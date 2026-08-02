<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";

import type {
  PineV6WorkflowDocument,
  StrategyDefinitionDocument,
  StrategyInstanceItem,
} from "@/types";
import type { StrategyAnalyzePineData } from "@/contracts";
import { apiGet, apiPost, apiPutPath } from "@/composables/shared/apiClient";
import { mapStrategyInstances } from "@/composables/strategy/strategyContract";
import {
  fetchStrategyDefinitionVersion,
  fetchStrategyDefinitionVersions,
  sortStrategyDefinitionVersions,
  strategyDefinitionVersionQueryKey,
  strategyDefinitionVersionsQueryKey,
  type StrategyDefinitionVersionDocument,
  type StrategyDefinitionVersionSummary,
} from "@/composables/strategy/strategyDefinitionVersions";
import { usePolling } from "@/composables/shared/usePolling";
import { queryClient, queryKeys } from "@/composables/settings/serverState";
import { formatLocalDateTime } from "@/utils/dateTime";
import { buildPineStrategyDefinitionPayload } from "@/components/strategy-runtime/strategyDefinitionPayload";
import {
  formatStrategySymbols,
  parseStrategyInstrumentIdsText,
} from "@/components/strategy-runtime/strategyRuntimeInstanceBinding";
import {
  buildWorkflowSnapshotFromSource,
  buildPineSourceStructureIndex,
  renderBlockToSource,
  replaceSourceRange,
  updateInstructionBlockParam,
} from "@/features/pine-structure";
import {
  assessPineV6Workflow,
  buildPineV6WorkflowScript,
  createDefaultPineV6Workflow,
  normalizePineV6Workflow,
  type PineV6WorkflowDiagnostic,
} from "@/features/pineV6Workflow";
import StrategyDesignWorkbench from "@/components/strategy-design/StrategyDesignWorkbench.vue";
import { provideStrategyDesignContext } from "@/components/strategy-design/strategyDesignContext";
import {
  useStrategySidePanelLayout,
  type StrategyDisplayMode,
  type StrategyMobileSection,
} from "@/components/strategy-design/useStrategySidePanelLayout";
import { useStrategySourceEditing } from "@/components/strategy-design/useStrategySourceEditing";
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

type StrategyPineAnalyzeWire =
  StrategyAnalyzePineData;

function mapStrategyPineAnalyzeDiagnostic(
  value: unknown,
): StrategyPineAnalyzeDiagnostic | null {
  if (value == null || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  const record = value as Record<string, unknown>;
  const severity =
    record.severity === "error" ||
    record.severity === "warning" ||
    record.severity === "info"
      ? record.severity
      : "info";
  const numberOrZero = (field: string): number =>
    typeof record[field] === "number" && Number.isFinite(record[field])
      ? record[field]
      : 0;
  return {
    severity,
    ...(typeof record.code === "string" ? { code: record.code } : {}),
    message: typeof record.message === "string" ? record.message : "",
    line: numberOrZero("line"),
    column: numberOrZero("column"),
    endLine: numberOrZero("endLine"),
    endColumn: numberOrZero("endColumn"),
  };
}

function mapStrategyPineAnalyzeResponse(
  value: StrategyPineAnalyzeWire,
): StrategyPineAnalyzeResponse {
  return {
    ok: value.ok,
    diagnostics: (value.diagnostics ?? [])
      .map(mapStrategyPineAnalyzeDiagnostic)
      .filter((entry): entry is StrategyPineAnalyzeDiagnostic => entry != null),
    features: value.features ?? [],
  };
}

const strategyDefinitionsQueryKey = queryKeys.strategyDefinitions();
const router = useRouter();

function fetchStrategyDefinitions(): Promise<StrategyDefinitionDocument[]> {
  return apiGet("/api/v1/strategy-definitions");
}

const strategyDefinitions = ref<StrategyDefinitionDocument[]>([]);
const strategies = ref<StrategyInstanceItem[]>([]);
const selectedDefinitionId = ref("");
const isLoadingDefinitions = ref(false);
const isLoadingStrategies = ref(false);
const isSavingDefinition = ref(false);
const isAnalyzing = ref(false);
const error = ref("");
const strategiesLoadError = ref("");
const displayError = computed(() =>
  [error.value, strategiesLoadError.value]
    .filter((message) => message !== "")
    .join("；"),
);
const analyzeResult = ref<StrategyPineAnalyzeResponse | null>(null);
const actionFeedback = ref<"analyze" | "save" | "">("");
const actionFeedbackTimer = ref<ReturnType<typeof setTimeout> | null>(null);
const runtimeRefreshPolling = usePolling(
  () => loadStrategies(),
  { intervalMs: 3_000 },
);

const workflow = ref<PineV6WorkflowDocument>(createDefaultPineV6Workflow());
const definitionName = ref(workflow.value.declaration.title);
const definitionVersion = ref("0.1.0");
const definitionDescription = ref("Pine v6 原生快捷指令工作台生成的策略。");
const sourceOverride = ref(buildPineV6WorkflowScript(workflow.value));
const useSourceOverride = ref(false);
const strategyDisplayMode = ref<StrategyDisplayMode>("split");
const strategyMobileSection = ref<StrategyMobileSection>("definition");
const metadataPaneOpen = ref(true);
const errorExpanded = ref(false);
const definitionVersions = ref<StrategyDefinitionVersionSummary[]>([]);
const isLoadingDefinitionVersions = ref(false);
const definitionVersionsError = ref("");
const selectedVersionSnapshot = ref<StrategyDefinitionVersionDocument | null>(null);
const isLoadingVersionSnapshot = ref(false);
const versionSnapshotError = ref("");
const comparisonVersionSelection = ref<string[]>([]);
let definitionVersionsRequestId = 0;
let versionSnapshotRequestId = 0;

const generatedScript = computed(() => buildPineV6WorkflowScript(workflow.value));
const activeScript = computed(() => sourceOverride.value);
const {
  selectedSourceNodeId, expandedSourceNodeId, sourceEditorRef, sourceUndoStack, sourceRedoStack,
  canUndoSourceChange, canRedoSourceChange, sourceBlockIsEditable, addSourceBlock,
  changeSourceBlockKind, deleteSourceStructureBlock, duplicateSourceStructureBlock,
  moveSourceStructureBlock, rememberSourceSnapshot, commitSourceChange, resetSourceHistory,
  restoreSourceSnapshot, undoSourceChange, redoSourceChange, applySourceEdit,
  toggleSourceBlockExpansion, updateSourceBlockField,
} = useStrategySourceEditing(activeScript, sourceOverride, useSourceOverride);
const {
  isMediumWorkbench, isWideWorkbench, expandedStrategySidePanels, strategySidePanelOrder,
  draggedStrategySidePanelId, strategySidePanelDropTarget, fillStrategySidePanelId,
  expandedStrategySidePanelCount, setStrategyDisplayMode, toggleMetadataPane, closeMetadataPane,
  strategySidePanelPosition, strategySidePanelClasses, moveStrategySidePanel,
  handleStrategySidePanelDragStart, handleStrategySidePanelDragOver,
  finishStrategySidePanelDrag, handleStrategySidePanelDrop, syncMediumWorkbench,
  handleWorkbenchKeydown, installStrategyWorkbenchMediaQuery,
  disposeStrategyWorkbenchMediaQuery, setStrategyMobileSection,
} = useStrategySidePanelLayout(strategyDisplayMode, strategyMobileSection, metadataPaneOpen);
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
const totalDiagnosticCount = computed(
  () => workflowDiagnostics.value.length + analyzerDiagnostics.value.length,
);
const totalErrorCount = computed(() => workflowErrorCount.value + analyzerErrorCount.value);
const selectedDefinition = computed(() =>
  strategyDefinitions.value.find((definition) => definition.id === selectedDefinitionId.value) ?? null,
);
const readonlyStrategies = computed(() =>
  selectedDefinitionId.value === ""
    ? []
    : strategies.value.filter((strategy) => strategy.definition.strategyId === selectedDefinitionId.value),
);
const rawSourceNodeCount = computed(
  () => sourceStructureNodes.value.filter((node) => node.match.type === "raw").length,
);
const selectedSourceNodeSummary = computed(() => {
  const node = sourceStructureNodes.value.find((item) => item.id === selectedSourceNodeId.value);
  return node === undefined ? (sourceOverride.value === generatedScript.value ? "图块生成" : "源码覆盖") : `L${node.lineRange.start} ${node.label}`;
});
const selectedComparisonVersions = computed(() =>
  sortStrategyDefinitionVersions(
    definitionVersions.value.filter((version) =>
      comparisonVersionSelection.value.includes(version.version),
    ),
  ).reverse(),
);
const canOpenVersionComparison = computed(
  () => selectedDefinitionId.value !== "" && selectedComparisonVersions.value.length === 2,
);

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

function compatibleWorkflowSnapshot(): PineV6WorkflowDocument {
  return buildWorkflowSnapshotFromSource(activeScript.value, workflow.value);
}

function formatVersionSavedAt(value: string): string {
  return formatLocalDateTime(value, "保存时间未知");
}

function resetDefinitionVersions(): void {
  definitionVersionsRequestId += 1;
  versionSnapshotRequestId += 1;
  definitionVersions.value = [];
  definitionVersionsError.value = "";
  isLoadingDefinitionVersions.value = false;
  selectedVersionSnapshot.value = null;
  isLoadingVersionSnapshot.value = false;
  versionSnapshotError.value = "";
  comparisonVersionSelection.value = [];
}

async function loadDefinitionVersions(definitionId = selectedDefinitionId.value): Promise<void> {
  const normalizedDefinitionId = definitionId.trim();
  const requestId = ++definitionVersionsRequestId;
  if (normalizedDefinitionId === "") {
    resetDefinitionVersions();
    return;
  }

  isLoadingDefinitionVersions.value = true;
  definitionVersionsError.value = "";
  selectedVersionSnapshot.value = null;
  versionSnapshotError.value = "";
  comparisonVersionSelection.value = [];
  try {
    // Version history changes when the definition is saved.  Fetch explicitly
    // (rather than returning a still-fresh query-cache entry) so the refresh
    // button and the post-save reload always show the newly created snapshot.
    const versions = await queryClient.fetchQuery({
      queryKey: strategyDefinitionVersionsQueryKey(normalizedDefinitionId),
      queryFn: () => fetchStrategyDefinitionVersions(normalizedDefinitionId),
      staleTime: 0,
    });
    if (requestId !== definitionVersionsRequestId || normalizedDefinitionId !== selectedDefinitionId.value) {
      return;
    }
    definitionVersions.value = versions;
  } catch (cause) {
    if (requestId !== definitionVersionsRequestId || normalizedDefinitionId !== selectedDefinitionId.value) {
      return;
    }
    definitionVersions.value = [];
    definitionVersionsError.value = cause instanceof Error ? cause.message : String(cause);
  } finally {
    if (requestId === definitionVersionsRequestId) {
      isLoadingDefinitionVersions.value = false;
    }
  }
}

function isVersionSelectedForComparison(version: string): boolean {
  return comparisonVersionSelection.value.includes(version);
}

function versionSelectionDisabled(version: string): boolean {
  return !isVersionSelectedForComparison(version) && comparisonVersionSelection.value.length >= 2;
}

function toggleVersionForComparison(version: string): void {
  const normalizedVersion = version.trim();
  if (normalizedVersion === "") {
    return;
  }
  if (comparisonVersionSelection.value.includes(normalizedVersion)) {
    comparisonVersionSelection.value = comparisonVersionSelection.value.filter(
      (candidate) => candidate !== normalizedVersion,
    );
    return;
  }
  if (comparisonVersionSelection.value.length >= 2) {
    return;
  }
  comparisonVersionSelection.value = [
    ...comparisonVersionSelection.value,
    normalizedVersion,
  ];
}

async function showVersionSnapshot(version: string): Promise<void> {
  const definitionId = selectedDefinitionId.value.trim();
  const normalizedVersion = version.trim();
  if (definitionId === "" || normalizedVersion === "") {
    return;
  }
  const requestId = ++versionSnapshotRequestId;
  isLoadingVersionSnapshot.value = true;
  versionSnapshotError.value = "";
  try {
    const snapshot = await queryClient.ensureQueryData({
      queryKey: strategyDefinitionVersionQueryKey(definitionId, normalizedVersion),
      queryFn: () => fetchStrategyDefinitionVersion(definitionId, normalizedVersion),
    });
    if (requestId !== versionSnapshotRequestId || definitionId !== selectedDefinitionId.value) {
      return;
    }
    selectedVersionSnapshot.value = snapshot;
  } catch (cause) {
    if (requestId !== versionSnapshotRequestId || definitionId !== selectedDefinitionId.value) {
      return;
    }
    selectedVersionSnapshot.value = null;
    versionSnapshotError.value = cause instanceof Error ? cause.message : String(cause);
  } finally {
    if (requestId === versionSnapshotRequestId) {
      isLoadingVersionSnapshot.value = false;
    }
  }
}

function openVersionComparison(): void {
  const definitionId = selectedDefinitionId.value.trim();
  const [baseline, candidate] = selectedComparisonVersions.value;
  if (definitionId === "" || baseline == null || candidate == null) {
    return;
  }
  void router.push({
    path: "/backtest",
    query: {
      mode: "compare",
      definitionId,
      leftVersion: baseline.version,
      rightVersion: candidate.version,
    },
  });
}

onMounted(() => {
  installStrategyWorkbenchMediaQuery();
  if (typeof window !== "undefined") {
    window.addEventListener("keydown", handleWorkbenchKeydown);
  }
  void loadStrategyDefinitions(selectedDefinitionId.value, { applyDefinition: props.entryMode !== "new" });
  void loadStrategies();
  runtimeRefreshPolling.start();
});

onBeforeUnmount(() => {
  disposeStrategyWorkbenchMediaQuery();
  if (typeof window !== "undefined") {
    window.removeEventListener("keydown", handleWorkbenchKeydown);
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
    strategies.value = mapStrategyInstances(await apiGet("/api/v1/strategies"));
    strategiesLoadError.value = "";
  } catch (cause) {
    const message = String(cause).replace(/^[A-Za-z]*Error:\s*/u, "");
    strategiesLoadError.value = `加载策略实例失败: ${message}`;
  } finally {
    isLoadingStrategies.value = false;
  }
}

function applyDefinition(definition: StrategyDefinitionDocument): void {
  selectedDefinitionId.value = definition.id ?? "";
  definitionName.value = definition.name ?? "";
  definitionVersion.value = definition.version ?? "";
  definitionDescription.value = definition.description ?? "";
  workflow.value = normalizePineV6Workflow(definition.visualModel);
  sourceOverride.value = definition.script || generatedScript.value;
  useSourceOverride.value = false;
  resetSourceHistory();
  analyzeResult.value = null;
  void loadDefinitionVersions(selectedDefinitionId.value);
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
  resetDefinitionVersions();
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
    const result = mapStrategyPineAnalyzeResponse(await apiPost(
      "/api/v1/strategy-pine/analyze",
      {
        script: activeScript.value,
        sourceFormat: "pine-v6",
        includeAst: false,
      },
    ));
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
    });
    const existing = strategyDefinitions.value.some((definition) => definition.id === selectedDefinitionId.value);
    const saved = existing
      ? await apiPutPath(
        "/api/v1/strategy-definitions/{definitionId}",
        `/api/v1/strategy-definitions/${encodeURIComponent(selectedDefinitionId.value)}`,
        payload,
      )
      : await apiPost(
        "/api/v1/strategy-definitions",
        payload,
      );
    const savedDefinitionId = saved.id ?? "";
    selectedDefinitionId.value = savedDefinitionId;
    queryClient.setQueryData<StrategyDefinitionDocument[]>(
      strategyDefinitionsQueryKey,
      (current) => {
        const next = current?.filter((definition) => definition.id !== savedDefinitionId) ?? [];
        return [...next, saved];
      },
    );
    await queryClient.invalidateQueries({ queryKey: strategyDefinitionsQueryKey, refetchType: "none" });
    await queryClient.invalidateQueries({
      queryKey: strategyDefinitionVersionsQueryKey(savedDefinitionId),
      refetchType: "none",
    });
    await loadStrategyDefinitions(savedDefinitionId);
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

function strategyInstrumentIds(strategy: StrategyInstanceItem): string[] {
  return parseStrategyInstrumentIdsText(formatStrategySymbols(strategy));
}

provideStrategyDesignContext({
  definitionName, definitionVersion, definitionDescription, selectedDefinitionId,
  strategyDefinitions, selectedDefinition, isLoadingDefinitions, workflow,
  workflowDiagnostics, analyzerDiagnostics, workflowErrorCount, analyzerErrorCount,
  totalDiagnosticCount, totalErrorCount, readonlyStrategies, isLoadingStrategies,
  definitionVersions, isLoadingDefinitionVersions, definitionVersionsError,
  selectedVersionSnapshot, isLoadingVersionSnapshot, versionSnapshotError,
  selectedComparisonVersions, canOpenVersionComparison, expandedStrategySidePanels,
  expandedStrategySidePanelCount, isWideWorkbench, isMediumWorkbench,
  strategyMobileSection, strategyDisplayMode, metadataPaneOpen, error: displayError, errorExpanded,
  sourceEditorRef, activeScript, useSourceOverride, pineDiagnosticMarkers,
  sourceStructureNodes, rawSourceNodeCount, selectedSourceNodeSummary,
  selectedSourceNodeId, expandedSourceNodeId, canUndoSourceChange, canRedoSourceChange,
  isAnalyzing, isSavingDefinition, actionFeedback,
  closeMetadataPane, toggleMetadataPane, finishStrategySidePanelDrag,
  strategySidePanelClasses, strategySidePanelPosition, moveStrategySidePanel,
  handleStrategySidePanelDragOver, handleStrategySidePanelDragStart,
  handleStrategySidePanelDrop, applyDefinition, createNewWorkflow, updateDeclaration,
  diagnosticClass, loadStrategies, statusLabel, statusClass, strategyInstrumentIds,
  loadDefinitionVersions, isVersionSelectedForComparison, versionSelectionDisabled,
  toggleVersionForComparison, formatVersionSavedAt, showVersionSnapshot,
  openVersionComparison, undoSourceChange, redoSourceChange, setStrategyDisplayMode,
  setStrategyMobileSection, analyzeCurrentScript, saveDefinition, commitSourceChange,
  toggleSourceBlockExpansion, addSourceBlock, changeSourceBlockKind,
  deleteSourceStructureBlock, duplicateSourceStructureBlock, moveSourceStructureBlock,
  updateSourceBlockField,
});
</script>

<template>
  <StrategyDesignWorkbench />
</template>
