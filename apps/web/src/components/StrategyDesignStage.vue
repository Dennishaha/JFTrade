<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";

import type {
  PineV6WorkflowDocument,
  StrategyDefinitionDocument,
  StrategyInstanceItem,
} from "@/contracts";
import { apiGet, apiPost, apiPutPath, fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import {
  fetchStrategyDefinitionVersion,
  fetchStrategyDefinitionVersions,
  sortStrategyDefinitionVersions,
  strategyDefinitionVersionQueryKey,
  strategyDefinitionVersionsQueryKey,
  type StrategyDefinitionVersionDocument,
  type StrategyDefinitionVersionSummary,
} from "../composables/strategyDefinitionVersions";
import { usePolling } from "../composables/usePolling";
import { queryClient, queryKeys } from "../composables/serverState";
import { formatLocalDateTime } from "../utils/dateTime";
import InstrumentIdentity from "./domain/market-data/InstrumentIdentity.vue";
import { buildPineStrategyDefinitionPayload } from "./strategy-runtime/strategyDefinitionPayload";
import {
  formatStrategyInterval,
  formatStrategyRuntimeRiskSummary,
  formatStrategySymbols,
  parseStrategyInstrumentIdsText,
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

type StrategyMobileSection = "definition" | "instruction" | "code";
type StrategyDisplayMode = "instruction" | "split" | "code";
type StrategySidePanelId = "definition" | "history" | "declaration" | "diagnostics" | "instances";
type StrategySidePanelDropEdge = "before" | "after";
const strategyDefinitionsQueryKey = queryKeys.strategyDefinitions();
const router = useRouter();
const STRATEGY_MEDIUM_WORKBENCH_QUERY = "(min-width: 769px) and (max-width: 1180px)";
const DEFAULT_STRATEGY_SIDE_PANEL_ORDER: StrategySidePanelId[] = [
  "definition",
  "history",
  "declaration",
  "diagnostics",
  "instances",
];
let strategyMediumWorkbenchMediaQuery: MediaQueryList | null = null;

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
const analyzeResult = ref<StrategyPineAnalyzeResponse | null>(null);
const selectedSourceNodeId = ref("");
const expandedSourceNodeId = ref<string | null>(null);
const sourceEditorRef = ref<InstanceType<typeof PineSourceCodePane> | null>(null);
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
const isMediumWorkbench = ref(false);
const isWideWorkbench = ref(true);
const metadataPaneOpen = ref(true);
const errorExpanded = ref(false);
const sourceUndoStack = ref<string[]>([]);
const sourceRedoStack = ref<string[]>([]);
const definitionVersions = ref<StrategyDefinitionVersionSummary[]>([]);
const isLoadingDefinitionVersions = ref(false);
const definitionVersionsError = ref("");
const selectedVersionSnapshot = ref<StrategyDefinitionVersionDocument | null>(null);
const isLoadingVersionSnapshot = ref(false);
const versionSnapshotError = ref("");
const comparisonVersionSelection = ref<string[]>([]);
let definitionVersionsRequestId = 0;
let versionSnapshotRequestId = 0;
const expandedStrategySidePanels = ref<string[]>(["definition"]);
const strategySidePanelOrder = ref<StrategySidePanelId[]>([...DEFAULT_STRATEGY_SIDE_PANEL_ORDER]);
const draggedStrategySidePanelId = ref<StrategySidePanelId | null>(null);
const strategySidePanelDropTarget = ref<{
  id: StrategySidePanelId;
  edge: StrategySidePanelDropEdge;
} | null>(null);

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
const canUndoSourceChange = computed(() => sourceUndoStack.value.length > 0);
const canRedoSourceChange = computed(() => sourceRedoStack.value.length > 0);
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
const fillStrategySidePanelId = computed<StrategySidePanelId | null>(() =>
  [...strategySidePanelOrder.value]
    .reverse()
    .find((panelId) => expandedStrategySidePanels.value.includes(panelId)) ?? null,
);
const expandedStrategySidePanelCount = computed(() =>
  strategySidePanelOrder.value.filter((panelId) =>
    expandedStrategySidePanels.value.includes(panelId),
  ).length,
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

function setStrategyDisplayMode(mode: StrategyDisplayMode): void {
  strategyDisplayMode.value = mode;
}

function toggleMetadataPane(): void {
  metadataPaneOpen.value = !metadataPaneOpen.value;
}

function closeMetadataPane(): void {
  metadataPaneOpen.value = false;
}

function strategySidePanelPosition(panelId: StrategySidePanelId): number {
  return strategySidePanelOrder.value.indexOf(panelId);
}

function strategySidePanelClasses(panelId: StrategySidePanelId): Record<string, boolean> {
  return {
    "is-fill-panel": fillStrategySidePanelId.value === panelId,
    "is-first-panel": strategySidePanelPosition(panelId) === 0,
    "is-dragging": draggedStrategySidePanelId.value === panelId,
    "is-drop-before": strategySidePanelDropTarget.value?.id === panelId
      && strategySidePanelDropTarget.value.edge === "before",
    "is-drop-after": strategySidePanelDropTarget.value?.id === panelId
      && strategySidePanelDropTarget.value.edge === "after",
  };
}

function moveStrategySidePanel(panelId: StrategySidePanelId, targetIndex: number): void {
  const currentIndex = strategySidePanelPosition(panelId);
  if (currentIndex < 0) {
    return;
  }
  const nextOrder = [...strategySidePanelOrder.value];
  nextOrder.splice(currentIndex, 1);
  const boundedTargetIndex = Math.max(0, Math.min(targetIndex, nextOrder.length));
  nextOrder.splice(boundedTargetIndex, 0, panelId);
  strategySidePanelOrder.value = nextOrder;
}

function handleStrategySidePanelDragStart(event: DragEvent, panelId: StrategySidePanelId): void {
  if (!isWideWorkbench.value) {
    event.preventDefault();
    return;
  }
  draggedStrategySidePanelId.value = panelId;
  strategySidePanelDropTarget.value = null;
  if (event.dataTransfer !== null) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", panelId);
  }
}

function handleStrategySidePanelDragOver(event: DragEvent, panelId: StrategySidePanelId): void {
  if (draggedStrategySidePanelId.value === null || draggedStrategySidePanelId.value === panelId) {
    strategySidePanelDropTarget.value = null;
    return;
  }
  const target = event.currentTarget as HTMLElement;
  const bounds = target.getBoundingClientRect();
  strategySidePanelDropTarget.value = {
    id: panelId,
    edge: event.clientY < bounds.top + bounds.height / 2 ? "before" : "after",
  };
  if (event.dataTransfer !== null) {
    event.dataTransfer.dropEffect = "move";
  }
}

function finishStrategySidePanelDrag(): void {
  draggedStrategySidePanelId.value = null;
  strategySidePanelDropTarget.value = null;
}

function handleStrategySidePanelDrop(event: DragEvent): void {
  const panelId = draggedStrategySidePanelId.value;
  const dropTarget = strategySidePanelDropTarget.value;
  if (panelId === null || dropTarget === null || panelId === dropTarget.id) {
    finishStrategySidePanelDrag();
    return;
  }
  const orderWithoutDraggedPanel = strategySidePanelOrder.value.filter((item) => item !== panelId);
  const targetIndex = orderWithoutDraggedPanel.indexOf(dropTarget.id);
  moveStrategySidePanel(panelId, targetIndex + (dropTarget.edge === "after" ? 1 : 0));
  finishStrategySidePanelDrag();
  event.preventDefault();
}

function syncMediumWorkbench(event: MediaQueryListEvent | MediaQueryList): void {
  const wasMediumWorkbench = isMediumWorkbench.value;
  isMediumWorkbench.value = event.matches;
  isWideWorkbench.value = typeof window === "undefined" || window.innerWidth > 1180;
  if (event.matches) {
    metadataPaneOpen.value = false;
    return;
  }
  if (wasMediumWorkbench) {
    metadataPaneOpen.value = true;
  }
}

function handleWorkbenchKeydown(event: KeyboardEvent): void {
  if (event.key === "Escape" && isMediumWorkbench.value && metadataPaneOpen.value) {
    closeMetadataPane();
  }
}

function installStrategyWorkbenchMediaQuery(): void {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return;
  }
  strategyMediumWorkbenchMediaQuery = window.matchMedia(STRATEGY_MEDIUM_WORKBENCH_QUERY);
  syncMediumWorkbench(strategyMediumWorkbenchMediaQuery);
  if (typeof strategyMediumWorkbenchMediaQuery.addEventListener === "function") {
    strategyMediumWorkbenchMediaQuery.addEventListener("change", syncMediumWorkbench);
  } else {
    strategyMediumWorkbenchMediaQuery.addListener(syncMediumWorkbench);
  }
}

function disposeStrategyWorkbenchMediaQuery(): void {
  if (typeof strategyMediumWorkbenchMediaQuery?.removeEventListener === "function") {
    strategyMediumWorkbenchMediaQuery.removeEventListener("change", syncMediumWorkbench);
  } else {
    strategyMediumWorkbenchMediaQuery?.removeListener(syncMediumWorkbench);
  }
  strategyMediumWorkbenchMediaQuery = null;
}

function setStrategyMobileSection(section: StrategyMobileSection): void {
  strategyMobileSection.value = section;
  if (section === "code") {
    setStrategyDisplayMode("code");
    return;
  }
  setStrategyDisplayMode("instruction");
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
    const items = await fetchEnvelope<StrategyInstanceItem[]>("/api/v1/strategies");
    strategies.value = items;
  } catch {
    strategies.value = [];
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
</script>

<template>
  <div
    class="strategy-native-page"
    :class="[
      `strategy-native-page--mobile-${strategyMobileSection}`,
      `strategy-native-page--mode-${strategyDisplayMode}`,
      metadataPaneOpen ? 'strategy-native-page--metadata-open' : 'strategy-native-page--metadata-closed',
      { 'strategy-native-page--medium': isMediumWorkbench },
    ]"
    data-testid="strategy-design-stage"
  >
    <header class="strategy-native-header">
      <div class="strategy-native-header__identity">
        <button
          type="button"
          class="strategy-native-metadata-toggle"
          :class="{ 'is-active': metadataPaneOpen }"
          :aria-expanded="metadataPaneOpen"
          aria-controls="strategy-design-metadata-pane"
          data-testid="strategy-metadata-toggle"
          title="显示或隐藏策略信息"
          @click="toggleMetadataPane"
        >
          <v-icon size="14">fa-solid fa-table-columns</v-icon>
          <span>策略信息</span>
        </button>
        <div class="strategy-native-title-block">
          <h1>策略快捷指令工作台</h1>
          <span class="strategy-native-active-definition" :title="definitionName || '新建草稿'">
            {{ definitionName || "新建草稿" }}
          </span>
        </div>
        <span class="strategy-native-chip">v{{ definitionVersion || "0.1.0" }}</span>
        <span
          class="strategy-native-chip strategy-native-chip--diagnostic"
          :class="{ 'has-error': totalErrorCount > 0 }"
          :title="`诊断 ${totalDiagnosticCount} 项，其中错误 ${totalErrorCount} 项`"
        >
          <v-icon size="11">fa-solid fa-stethoscope</v-icon>
          {{ totalDiagnosticCount }}
        </span>
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
        <button type="button" class="strategy-native-action-button" @click="createNewWorkflow">新建 Pine v6</button>
        <button type="button" class="strategy-native-action-button" :disabled="isAnalyzing" @click="void analyzeCurrentScript()">
          {{ isAnalyzing ? "分析中" : actionFeedback === "analyze" ? "已分析" : "分析" }}
        </button>
        <button type="button" class="strategy-native-action-button strategy-native-action-button--primary" :disabled="isSavingDefinition" @click="void saveDefinition()">
          {{ isSavingDefinition ? "保存中" : actionFeedback === "save" ? "已保存" : "保存" }}
        </button>
      </div>
    </header>

    <nav class="strategy-native-mobile-switch" aria-label="策略移动端工作区">
      <button
        type="button"
        class="strategy-native-mobile-switch__button"
        :class="{ 'is-active': strategyMobileSection === 'definition' }"
        data-testid="strategy-mobile-section-definition"
        @click="setStrategyMobileSection('definition')"
      >
        策略定义
      </button>
      <button
        type="button"
        class="strategy-native-mobile-switch__button"
        :class="{ 'is-active': strategyMobileSection === 'instruction' }"
        data-testid="strategy-mobile-section-instruction"
        @click="setStrategyMobileSection('instruction')"
      >
        结构指令
      </button>
      <button
        type="button"
        class="strategy-native-mobile-switch__button"
        :class="{ 'is-active': strategyMobileSection === 'code' }"
        data-testid="strategy-mobile-section-code"
        @click="setStrategyMobileSection('code')"
      >
        Pine 代码
      </button>
    </nav>

    <button
      v-if="error"
      type="button"
      class="strategy-native-banner strategy-native-banner--error"
      :class="{ 'is-expanded': errorExpanded }"
      :aria-expanded="errorExpanded"
      :title="error"
      @click="errorExpanded = !errorExpanded"
    >
      <v-icon size="13">fa-solid fa-triangle-exclamation</v-icon>
      <span>{{ error }}</span>
    </button>

    <button
      v-if="isMediumWorkbench && metadataPaneOpen"
      type="button"
      class="strategy-native-drawer-backdrop"
      aria-label="关闭策略信息"
      data-testid="strategy-metadata-backdrop"
      @click="closeMetadataPane"
    />

    <SplitPane class="strategy-native-shell" :pane-min-size="18">
      <SplitPaneItem
        :size="strategyDisplayMode === 'instruction' ? 100 : strategyDisplayMode === 'split' ? (isMediumWorkbench ? 58 : 65) : 22"
        :min-size="strategyDisplayMode === 'instruction' ? 100 : strategyDisplayMode === 'code' ? 18 : 32"
        :max-size="strategyDisplayMode === 'instruction' ? 100 : strategyDisplayMode === 'code' ? 36 : 78"
      >
        <SplitPane class="strategy-native-instruction" :pane-min-size="16">
          <SplitPaneItem
            :size="strategyDisplayMode === 'code' ? 100 : strategyDisplayMode === 'instruction' ? 22 : 34"
            :min-size="strategyDisplayMode === 'code' ? 100 : 18"
            :max-size="strategyDisplayMode === 'code' ? 100 : 42"
          >
            <aside id="strategy-design-metadata-pane" class="strategy-native-side">
              <div class="strategy-native-drawer-head">
                <div>
                  <strong>策略信息</strong>
                  <span>{{ definitionName || "新建草稿" }}</span>
                </div>
                <button type="button" aria-label="关闭策略信息" @click="closeMetadataPane">
                  <v-icon size="14">fa-solid fa-xmark</v-icon>
                </button>
              </div>
              <v-expansion-panels
                v-model="expandedStrategySidePanels"
                multiple
                class="strategy-native-side-panels"
                :class="{
                  'is-reorderable': isWideWorkbench,
                  'is-space-constrained': expandedStrategySidePanelCount >= 3,
                }"
                variant="default"
                @dragend="finishStrategySidePanelDrag"
              >
                <v-expansion-panel
                  value="definition"
                  class="strategy-native-panel strategy-native-side-panel"
                  :class="strategySidePanelClasses('definition')"
                  :style="{ order: strategySidePanelPosition('definition') }"
                  data-testid="strategy-side-panel-definition"
                >
                  <v-expansion-panel-title
                    collapse-icon="fa-solid fa-chevron-right"
                    data-testid="strategy-side-panel-definition-title"
                    :draggable="isWideWorkbench"
                    expand-icon="fa-solid fa-chevron-right"
                    title="拖动调整位置"
                    @dragover.prevent="handleStrategySidePanelDragOver($event, 'definition')"
                    @dragstart="handleStrategySidePanelDragStart($event, 'definition')"
                    @drop.prevent="handleStrategySidePanelDrop"
                  >
                    <div class="strategy-native-side-panel__heading">
                      <div class="strategy-native-panel__title">策略定义</div>
                    </div>
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="strategy-native-panel__content">
                      <select v-model="selectedDefinitionId" :disabled="isLoadingDefinitions" @change="selectedDefinition ? applyDefinition(selectedDefinition) : createNewWorkflow()">
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
                        <span>版本（保存后自动生成）</span>
                        <input v-model="definitionVersion" readonly aria-readonly="true">
                      </label>
                      <label>
                        <span>说明</span>
                        <textarea v-model="definitionDescription" rows="3" />
                      </label>
                    </div>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel
                  value="history"
                  class="strategy-native-panel strategy-native-side-panel"
                  :class="strategySidePanelClasses('history')"
                  :style="{ order: strategySidePanelPosition('history') }"
                  data-testid="strategy-side-panel-history"
                >
                  <v-expansion-panel-title
                    collapse-icon="fa-solid fa-chevron-right"
                    data-testid="strategy-side-panel-history-title"
                    :draggable="isWideWorkbench"
                    expand-icon="fa-solid fa-chevron-right"
                    title="拖动调整位置"
                    @dragover.prevent="handleStrategySidePanelDragOver($event, 'history')"
                    @dragstart="handleStrategySidePanelDragStart($event, 'history')"
                    @drop.prevent="handleStrategySidePanelDrop"
                  >
                    <div class="strategy-native-side-panel__heading">
                      <div class="strategy-native-panel__title">版本历史</div>
                      <span class="strategy-native-panel-count">{{ definitionVersions.length }}</span>
                    </div>
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="strategy-native-panel__content">
                      <div class="strategy-native-panel-tools">
                        <span>不可变版本</span>
                        <button
                          type="button"
                          class="strategy-native-icon-button"
                          :disabled="selectedDefinitionId === '' || isLoadingDefinitionVersions"
                          title="刷新版本历史"
                          aria-label="刷新版本历史"
                          @click.stop="void loadDefinitionVersions()"
                        >
                          <v-icon size="13">fa-solid fa-arrow-rotate-right</v-icon>
                        </button>
                      </div>
                      <div v-if="selectedDefinitionId === ''" class="strategy-native-meta">
                        保存策略后会生成首个不可变版本。
                      </div>
                      <div v-else-if="isLoadingDefinitionVersions" class="strategy-native-meta">
                        正在加载版本历史…
                      </div>
                      <div v-else-if="definitionVersionsError" class="strategy-native-version-notice">
                        版本历史暂不可用：{{ definitionVersionsError }}
                      </div>
                      <div v-else-if="definitionVersions.length === 0" class="strategy-native-meta">
                        暂无已保存版本。
                      </div>
                      <template v-else>
                        <div class="strategy-native-version-compare-note">
                          选择两个版本后可在回测页比较其已完成回测；左侧为较早基线，右侧为较新候选。
                        </div>
                        <section
                          v-for="version in definitionVersions"
                          :key="version.version"
                          class="strategy-native-version-entry"
                          :class="{ 'is-selected': isVersionSelectedForComparison(version.version) }"
                          :data-testid="`strategy-version-entry-${version.version}`"
                        >
                          <label class="strategy-native-version-entry__select">
                            <input
                              :checked="isVersionSelectedForComparison(version.version)"
                              :disabled="versionSelectionDisabled(version.version)"
                              type="checkbox"
                              @change="toggleVersionForComparison(version.version)"
                            >
                            <span>
                              <strong>v{{ version.version }}</strong>
                              <em v-if="version.isCurrent">当前</em>
                            </span>
                          </label>
                          <div class="strategy-native-version-entry__meta">
                            <span>{{ version.name || definitionName || "未命名策略" }}</span>
                            <span>{{ formatVersionSavedAt(version.savedAt) }}</span>
                          </div>
                          <button
                            type="button"
                            class="strategy-native-version-entry__view"
                            :disabled="isLoadingVersionSnapshot"
                            @click="void showVersionSnapshot(version.version)"
                          >
                            查看源码
                          </button>
                        </section>
                        <button
                          type="button"
                          class="strategy-native-version-compare-button"
                          data-testid="strategy-open-version-comparison"
                          :disabled="!canOpenVersionComparison"
                          @click="openVersionComparison"
                        >
                          比较选中版本（{{ selectedComparisonVersions.length }}/2）
                        </button>
                      </template>
                      <div v-if="isLoadingVersionSnapshot" class="strategy-native-meta">
                        正在加载历史源码…
                      </div>
                      <div v-else-if="versionSnapshotError" class="strategy-native-version-notice">
                        历史源码不可用：{{ versionSnapshotError }}
                      </div>
                      <details v-else-if="selectedVersionSnapshot" class="strategy-native-version-source" open>
                        <summary>v{{ selectedVersionSnapshot.version }} 只读源码</summary>
                        <pre>{{ selectedVersionSnapshot.script || "该版本没有可用源码快照。" }}</pre>
                      </details>
                    </div>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel
                  value="declaration"
                  class="strategy-native-panel strategy-native-side-panel"
                  :class="strategySidePanelClasses('declaration')"
                  :style="{ order: strategySidePanelPosition('declaration') }"
                  data-testid="strategy-side-panel-declaration"
                >
                  <v-expansion-panel-title
                    collapse-icon="fa-solid fa-chevron-right"
                    data-testid="strategy-side-panel-declaration-title"
                    :draggable="isWideWorkbench"
                    expand-icon="fa-solid fa-chevron-right"
                    title="拖动调整位置"
                    @dragover.prevent="handleStrategySidePanelDragOver($event, 'declaration')"
                    @dragstart="handleStrategySidePanelDragStart($event, 'declaration')"
                    @drop.prevent="handleStrategySidePanelDrop"
                  >
                    <div class="strategy-native-side-panel__heading">
                      <div class="strategy-native-panel__title">策略声明</div>
                    </div>
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

                <v-expansion-panel
                  value="diagnostics"
                  class="strategy-native-panel strategy-native-side-panel"
                  :class="strategySidePanelClasses('diagnostics')"
                  :style="{ order: strategySidePanelPosition('diagnostics') }"
                  data-testid="strategy-side-panel-diagnostics"
                >
                  <v-expansion-panel-title
                    collapse-icon="fa-solid fa-chevron-right"
                    data-testid="strategy-side-panel-diagnostics-title"
                    :draggable="isWideWorkbench"
                    expand-icon="fa-solid fa-chevron-right"
                    title="拖动调整位置"
                    @dragover.prevent="handleStrategySidePanelDragOver($event, 'diagnostics')"
                    @dragstart="handleStrategySidePanelDragStart($event, 'diagnostics')"
                    @drop.prevent="handleStrategySidePanelDrop"
                  >
                    <div class="strategy-native-side-panel__heading">
                      <div class="strategy-native-panel__title">诊断</div>
                      <span class="strategy-native-panel-count" :class="{ 'has-error': totalErrorCount > 0 }">
                        {{ totalDiagnosticCount }}
                      </span>
                    </div>
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

                <v-expansion-panel
                  value="instances"
                  class="strategy-native-panel strategy-native-side-panel"
                  :class="strategySidePanelClasses('instances')"
                  :style="{ order: strategySidePanelPosition('instances') }"
                  data-testid="strategy-side-panel-instances"
                >
                  <v-expansion-panel-title
                    collapse-icon="fa-solid fa-chevron-right"
                    data-testid="strategy-side-panel-instances-title"
                    :draggable="isWideWorkbench"
                    expand-icon="fa-solid fa-chevron-right"
                    title="拖动调整位置"
                    @dragover.prevent="handleStrategySidePanelDragOver($event, 'instances')"
                    @dragstart="handleStrategySidePanelDragStart($event, 'instances')"
                    @drop.prevent="handleStrategySidePanelDrop"
                  >
                    <div class="strategy-native-side-panel__heading">
                      <div class="strategy-native-panel__title">策略实例</div>
                      <span class="strategy-native-panel-count">{{ readonlyStrategies.length }}</span>
                    </div>
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="strategy-native-panel__content">
                      <div class="strategy-native-panel-tools">
                        <span>关联实例</span>
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
                        <div
                          class="flex flex-wrap items-center gap-1.5"
                          :data-testid="`strategy-design-instance-symbols-${strategy.id}`"
                        >
                          <template v-if="strategyInstrumentIds(strategy).length > 0">
                            <InstrumentIdentity
                              v-for="symbol in strategyInstrumentIds(strategy)"
                              :key="symbol"
                              :instrument-id="symbol"
                              compact
                            />
                          </template>
                          <span v-else>{{ formatStrategySymbols(strategy) }}</span>
                          <span>/ {{ formatStrategyInterval(strategy) }}</span>
                        </div>
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
            :size="strategyDisplayMode === 'instruction' ? 78 : 66"
            :min-size="38"
            :max-size="78"
          >
            <main class="strategy-native-main">
              <section class="strategy-native-panel strategy-native-panel--workspace">
                <div class="strategy-native-workspace-bar">
                  <div class="strategy-native-workspace-bar__identity">
                    <div class="strategy-native-panel__title">结构指令</div>
                    <span class="strategy-native-chip">{{ sourceStructureNodes.length }} 节点</span>
                    <span class="strategy-native-chip">{{ rawSourceNodeCount }} raw</span>
                    <span class="strategy-native-execution-hint">收盘确认 / 下一根 K 线成交</span>
                  </div>
                  <div class="strategy-native-selected-block" :title="selectedSourceNodeSummary">
                    {{ selectedSourceNodeSummary }}
                  </div>
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
        :size="strategyDisplayMode === 'split' ? (isMediumWorkbench ? 42 : 35) : 78"
        :min-size="strategyDisplayMode === 'split' ? 30 : 64"
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
/* Compact strategy workbench overrides. Keep the workspace full-bleed and let
   dividers, rather than nested cards, describe the three editing surfaces. */
.strategy-native-page {
  position: relative;
  display: flex;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  gap: 0;
  overflow: hidden;
  padding: 0;
  background: var(--tv-bg-app);
  color: var(--tv-text);
}

.strategy-native-header {
  display: flex;
  min-width: 0;
  min-height: 44px;
  flex: 0 0 44px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.strategy-native-header__identity,
.strategy-native-title-block,
.strategy-native-workspace-bar__identity,
.strategy-native-side-panel__heading {
  display: flex;
  min-width: 0;
  align-items: center;
}

.strategy-native-header__identity {
  flex: 1 1 auto;
  gap: 6px;
  overflow: hidden;
}

.strategy-native-title-block {
  flex: 0 1 auto;
  gap: 7px;
  overflow: hidden;
}

.strategy-native-header h1 {
  flex: 0 0 auto;
  margin: 0;
  font-size: 0.92rem;
  line-height: 1;
  white-space: nowrap;
}

.strategy-native-active-definition {
  min-width: 0;
  max-width: 15rem;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.8rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-native-chip,
.strategy-native-panel-count {
  display: inline-flex;
  min-height: 20px;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  padding: 1px 6px;
  color: var(--tv-text-muted);
  font-size: 0.7rem;
  font-weight: 700;
  line-height: 1;
  white-space: nowrap;
}

.strategy-native-chip--diagnostic.has-error,
.strategy-native-panel-count.has-error {
  border-color: color-mix(in srgb, #ef4444 52%, var(--tv-border));
  background: color-mix(in srgb, #ef4444 10%, transparent);
  color: color-mix(in srgb, #fca5a5 76%, var(--tv-text));
}

.strategy-native-metadata-toggle,
.strategy-native-action-button,
.strategy-native-history-button,
.strategy-native-icon-button {
  min-height: 30px;
  height: 30px;
  border-radius: 5px;
}

.strategy-native-metadata-toggle {
  display: inline-flex;
  min-width: 30px;
  align-items: center;
  justify-content: center;
  gap: 5px;
  border-color: transparent;
  background: transparent;
  padding: 0 6px;
  color: var(--tv-text-muted);
  font-size: 0.77rem;
}

.strategy-native-metadata-toggle:is(:hover, .is-active) {
  border-color: var(--tv-border);
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.strategy-native-header__actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: nowrap;
  gap: 4px;
}

.strategy-native-history-actions {
  display: inline-flex;
  align-items: center;
  gap: 0;
}

.strategy-native-history-button {
  display: inline-grid;
  width: 28px;
  place-items: center;
  border-color: transparent;
  background: transparent;
  color: var(--tv-text-muted);
  padding: 0;
}

.strategy-native-history-button:hover:not(:disabled) {
  color: var(--tv-text);
}

.strategy-native-view-switch {
  display: inline-grid;
  grid-template-columns: repeat(3, minmax(2.55rem, 1fr));
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 72%, transparent);
  padding: 2px;
}

.strategy-native-view-switch__button {
  display: inline-grid;
  min-width: 2.55rem;
  min-height: 26px;
  place-items: center;
  border: 0;
  border-radius: 4px;
  background: transparent;
  padding: 0 7px;
  color: var(--tv-text-muted);
  font-size: 0.77rem;
  font-weight: 800;
  line-height: 1;
  white-space: nowrap;
}

.strategy-native-view-switch__button.is-active {
  background: color-mix(in srgb, var(--tv-accent) 22%, var(--tv-bg-surface));
  color: var(--tv-text);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tv-accent) 36%, transparent);
}

.strategy-native-mobile-switch {
  display: none;
}

button {
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-weight: 700;
}

button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.strategy-native-action-button {
  padding: 0 9px;
  font-size: 0.8rem;
}

.strategy-native-action-button--primary {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 14%, var(--tv-bg-surface));
  color: var(--tv-accent);
}

.strategy-native-header button:focus-visible,
.strategy-native-side button:focus-visible,
.strategy-native-main button:focus-visible,
.strategy-native-banner:focus-visible {
  outline: 2px solid var(--tv-accent);
  outline-offset: -2px;
}

.strategy-native-shell {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  border: 0;
  border-radius: 0;
  background: var(--tv-bg-app);
}

.strategy-native-shell :deep(.splitpanes__pane),
.strategy-native-instruction :deep(.splitpanes__pane) {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.strategy-native-instruction {
  position: relative;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.strategy-native-side,
.strategy-native-main {
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  gap: 0;
  overflow: hidden;
  padding: 0;
}

.strategy-native-side {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--tv-bg-surface);
  scrollbar-gutter: auto;
}

.strategy-native-main {
  display: grid;
  grid-template-rows: minmax(0, 1fr);
  background: var(--tv-bg-app);
}

.strategy-native-drawer-head {
  display: none;
  min-height: 40px;
  flex: 0 0 40px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
}

.strategy-native-drawer-head > div {
  display: grid;
  min-width: 0;
  gap: 1px;
}

.strategy-native-drawer-head strong {
  font-size: 0.83rem;
}

.strategy-native-drawer-head span {
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.73rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-native-drawer-head button {
  display: inline-grid;
  width: 30px;
  height: 30px;
  place-items: center;
  padding: 0;
}

.strategy-native-side-panels {
  display: flex !important;
  height: 100%;
  width: 100% !important;
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
  flex-wrap: nowrap !important;
  justify-content: flex-start !important;
  gap: 0;
  overflow-x: hidden;
  overflow-y: auto;
  background: transparent;
}

.strategy-native-side-panels :deep(.v-expansion-panel-title__overlay),
.strategy-native-side-panels :deep(.v-expansion-panel__overlay) {
  display: none;
}

.strategy-native-side-panels :deep(.v-expansion-panel::after) {
  border: 0 !important;
  content: none !important;
}

.strategy-native-side-panels :deep(.v-expansion-panel-title) {
  width: 100% !important;
  min-height: 34px;
  flex: 0 0 34px;
  margin-inline: 0;
  border: 0 !important;
  border-radius: 0 !important;
  padding: 0 8px;
  background: var(--tv-bg-surface);
}

.strategy-native-side-panels :deep(.v-expansion-panel-title:hover) {
  background: var(--tv-bg-elevated);
}

.strategy-native-side-panels :deep(.v-expansion-panel) {
  width: 100% !important;
  min-width: 0;
  flex: 0 0 auto !important;
  margin: 0 !important;
  border-radius: 0 !important;
}

.strategy-native-side-panels :deep(.v-expansion-panel--active) {
  display: flex;
  min-height: 34px;
  flex: 0 1 auto !important;
  flex-direction: column;
}

.strategy-native-side-panels :deep(.v-expansion-panel--active.is-fill-panel) {
  flex: 1 1 0 !important;
}

.strategy-native-side-panels.is-space-constrained :deep(.v-expansion-panel--active) {
  min-height: 96px;
  flex: 1 1 0 !important;
}

.strategy-native-side-panels :deep(.v-expansion-panel-text) {
  min-height: 0;
  flex: 1 1 auto;
  overflow: hidden;
}

.strategy-native-side-panels :deep(.v-expansion-panel-text__wrapper) {
  width: 100%;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 0;
  scrollbar-gutter: auto;
}

.strategy-native-side-panels :deep(.v-expansion-panel-title__icon) {
  order: -1;
  margin-inline: 0 7px;
  font-size: 0.72rem;
  transform: rotate(0deg) !important;
}

.strategy-native-side-panels :deep(.v-expansion-panel-title--active > .v-expansion-panel-title__icon) {
  transform: rotate(90deg) !important;
}

.strategy-native-side-panels.is-reorderable :deep(.v-expansion-panel-title) {
  cursor: grab;
}

.strategy-native-side-panels.is-reorderable :deep(.v-expansion-panel-title:active) {
  cursor: grabbing;
}

.strategy-native-side-panel {
  position: relative;
  display: block;
  width: 100% !important;
  min-width: 0;
  overflow: hidden;
  border: 0;
  border-radius: 0;
  background: var(--tv-bg-surface);
}

.strategy-native-side-panel:not(.is-first-panel) :deep(.v-expansion-panel-title) {
  box-shadow: inset 0 1px 0 var(--tv-border);
}

.strategy-native-side-panel.is-dragging {
  opacity: 0.48;
}

.strategy-native-side-panel:is(.is-drop-before, .is-drop-after)::before {
  position: absolute;
  z-index: 3;
  right: 0;
  left: 0;
  height: 1px;
  background: var(--tv-accent);
  content: "";
  pointer-events: none;
}

.strategy-native-side-panel.is-drop-before::before {
  top: 0;
}

.strategy-native-side-panel.is-drop-after::before {
  bottom: 0;
}

.strategy-native-side-panel__heading {
  flex: 1 1 auto;
  justify-content: space-between;
  gap: 6px;
}

.strategy-native-panel__title {
  color: var(--tv-text-muted);
  font-size: 0.78rem;
  font-weight: 750;
  letter-spacing: 0.04em;
}

.strategy-native-panel-count {
  min-height: 18px;
  padding-inline: 5px;
  border-color: transparent;
  background: var(--tv-bg-elevated);
  font-size: 0.68rem;
}

.strategy-native-icon-button {
  display: inline-grid;
  width: 28px;
  place-items: center;
  border: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 0;
}

.strategy-native-panel-tools {
  display: flex;
  min-width: 0;
  min-height: 30px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  color: var(--tv-text-muted);
  font-size: 0.71rem;
  font-weight: 700;
}

.strategy-native-panel__content {
  display: grid;
  gap: 8px;
  padding: 8px;
  background: color-mix(in srgb, var(--tv-bg-app) 35%, var(--tv-bg-surface));
}

.strategy-native-panel label {
  display: grid;
  gap: 3px;
  color: var(--tv-text-muted);
  font-size: 0.7rem;
  letter-spacing: 0.06em;
  font-weight: 700;
  text-transform: uppercase;
}

.strategy-native-panel input,
.strategy-native-panel select,
.strategy-native-panel textarea {
  min-height: 32px;
  border-radius: 5px;
  padding: 5px 8px;
  font-size: 0.83rem;
  line-height: 1.25;
}

.strategy-native-panel textarea {
  min-height: 56px;
  resize: vertical;
}

.strategy-native-panel input,
.strategy-native-panel select,
.strategy-native-panel textarea {
  width: 100%;
  min-width: 0;
  border: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  outline: none;
}

.strategy-native-toggle {
  display: inline-flex !important;
  min-height: 28px;
  grid-template-columns: auto 1fr;
  align-items: center;
  gap: 5px;
  letter-spacing: 0 !important;
  text-transform: none !important;
}

.strategy-native-toggle input {
  width: auto;
}

.strategy-native-version-compare-note,
.strategy-native-version-notice {
  border: 1px solid var(--tv-border);
  padding: 5px 7px;
  border-radius: 5px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 72%, transparent);
  color: var(--tv-text-muted);
  font-size: 0.73rem;
  line-height: 1.35;
}

.strategy-native-version-notice {
  border-color: color-mix(in srgb, #f59e0b 44%, var(--tv-border));
  background: color-mix(in srgb, #f59e0b 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fbbf24 72%, var(--tv-text));
  overflow-wrap: anywhere;
}

.strategy-native-version-entry {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 3px 6px;
  align-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  padding: 5px 6px;
}

.strategy-native-version-entry.is-selected {
  border-color: color-mix(in srgb, var(--tv-accent) 58%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 9%, var(--tv-bg-surface));
}

.strategy-native-version-entry__select {
  display: inline-flex !important;
  min-width: 0;
  align-items: center;
  gap: 5px;
  color: var(--tv-text) !important;
  font-size: 0.8rem !important;
  letter-spacing: 0 !important;
  text-transform: none !important;
}

.strategy-native-version-entry__select input {
  width: auto !important;
}

.strategy-native-version-entry__select span {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.strategy-native-version-entry__select em {
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-accent) 18%, var(--tv-bg-surface));
  color: var(--tv-accent);
  padding: 1px 4px;
  font-size: 0.65rem;
  font-style: normal;
  font-weight: 800;
}

.strategy-native-version-entry__meta {
  grid-column: 1;
  display: flex;
  gap: 5px;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.69rem;
  white-space: nowrap;
}

.strategy-native-version-entry__meta span {
  overflow: hidden;
  text-overflow: ellipsis;
}

.strategy-native-version-entry__view {
  grid-column: 2;
  grid-row: 1 / span 2;
  align-self: center;
  min-height: 28px;
  padding: 0 6px;
  font-size: 0.71rem;
}

.strategy-native-version-compare-button {
  width: 100%;
  min-height: 32px;
  padding: 0 8px;
  font-size: 0.77rem;
}

.strategy-native-version-source {
  display: grid;
  gap: 5px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-elevated);
  padding: 6px;
}

.strategy-native-version-source summary {
  cursor: pointer;
  color: var(--tv-text);
  font-size: 0.77rem;
  font-weight: 800;
}

.strategy-native-version-source pre {
  max-height: 14rem;
  margin: 0;
  overflow: auto;
  color: var(--tv-text);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.73rem;
  line-height: 1.35;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.strategy-native-diagnostic {
  display: grid;
  gap: 2px;
  border: 1px solid var(--tv-border);
  padding: 5px 6px;
  border-radius: 5px;
  font-size: 0.77rem;
  overflow-wrap: anywhere;
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
  gap: 3px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--tv-border);
  font-size: 0.77rem;
  text-align: left;
}

.strategy-native-instance > div:first-child {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 5px;
}

.strategy-native-status {
  border-radius: 999px;
  padding: 2px 5px;
  font-size: 0.68rem;
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

.strategy-native-workspace-bar {
  display: flex;
  min-width: 0;
  min-height: 36px;
  flex: 0 0 36px;
  align-items: center;
  justify-content: space-between;
  flex-wrap: nowrap;
  gap: 6px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.strategy-native-workspace-bar__identity {
  flex: 1 1 auto;
  gap: 5px;
  overflow: hidden;
}

.strategy-native-execution-hint {
  min-width: 0;
  overflow: hidden;
  border-left: 1px solid var(--tv-border);
  padding-left: 6px;
  color: var(--tv-text-muted);
  font-size: 0.7rem;
  font-weight: 450;
  line-height: 1;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-native-selected-block {
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.73rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-native-selected-block {
  max-width: 42%;
  line-height: 1.2;
}

.strategy-native-panel--workspace {
  display: grid;
  min-width: 0;
  min-height: 0;
  height: 100%;
  grid-template-rows: auto minmax(0, 1fr);
  gap: 0;
  overflow: hidden;
  border: 0;
  background: transparent;
  padding: 0;
}

.strategy-native-block-scroll {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  width: 100%;
  min-width: 0;
  min-height: 0;
  align-content: start;
  overflow: auto;
  padding: 6px;
}

.strategy-native-meta {
  color: var(--tv-text-muted);
  font-size: 0.77rem;
  line-height: 1.35;
}

.strategy-native-banner {
  display: flex;
  width: 100%;
  min-height: 30px;
  flex: 0 0 30px;
  align-items: center;
  gap: 6px;
  overflow: hidden;
  border-width: 0 0 1px;
  border-radius: 0;
  padding: 0 8px;
  text-align: left;
}

.strategy-native-banner--error {
  border-color: color-mix(in srgb, #ef4444 52%, var(--tv-border));
  background: color-mix(in srgb, #ef4444 12%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fca5a5 72%, var(--tv-text));
}

.strategy-native-banner span {
  min-width: 0;
  overflow: hidden;
  font-size: 0.8rem;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-native-banner.is-expanded {
  height: auto;
  min-height: 30px;
  flex-basis: auto;
  padding-block: 6px;
}

.strategy-native-banner.is-expanded span {
  overflow: visible;
  white-space: normal;
}

.strategy-native-drawer-backdrop {
  position: absolute;
  z-index: 20;
  inset: 44px 0 0;
  border: 0;
  border-radius: 0;
  background: rgba(2, 6, 23, 0.38);
  padding: 0;
}

@media (min-width: 1181px) {
  .strategy-native-page--metadata-closed .strategy-native-instruction > :deep(.splitpanes__pane:first-of-type),
  .strategy-native-page--metadata-closed .strategy-native-instruction > :deep(.splitpanes__splitter:first-of-type) {
    display: none;
  }

  .strategy-native-page--metadata-closed .strategy-native-instruction > :deep(.splitpanes__pane:last-of-type) {
    width: 100% !important;
    max-width: 100% !important;
    flex: 0 0 100% !important;
  }

  .strategy-native-page--mode-code.strategy-native-page--metadata-closed .strategy-native-shell > :deep(.splitpanes__pane:first-of-type),
  .strategy-native-page--mode-code.strategy-native-page--metadata-closed .strategy-native-shell > :deep(.splitpanes__splitter:first-of-type) {
    display: none;
  }

  .strategy-native-page--mode-code.strategy-native-page--metadata-closed .strategy-native-shell > :deep(.splitpanes__pane:last-of-type) {
    width: 100% !important;
    max-width: 100% !important;
    flex: 0 0 100% !important;
  }
}

@media (min-width: 769px) and (max-width: 1180px) {
  .strategy-native-header {
    gap: 4px;
    padding-inline: 6px;
  }

  .strategy-native-active-definition {
    max-width: 9rem;
  }

  .strategy-native-page--mode-instruction .strategy-native-instruction > :deep(.splitpanes__pane:first-of-type),
  .strategy-native-page--mode-split .strategy-native-instruction > :deep(.splitpanes__pane:first-of-type) {
    position: absolute !important;
    z-index: 30;
    inset: 0 auto 0 0;
    width: min(360px, calc(100% - 48px)) !important;
    max-width: min(360px, calc(100% - 48px)) !important;
    min-width: min(280px, calc(100% - 48px)) !important;
    flex: 0 0 min(360px, calc(100% - 48px)) !important;
    transform: translateX(0);
    transition: transform 160ms ease;
    box-shadow: 16px 0 36px rgba(2, 6, 23, 0.3);
  }

  .strategy-native-page--mode-instruction .strategy-native-instruction > :deep(.splitpanes__splitter),
  .strategy-native-page--mode-split .strategy-native-instruction > :deep(.splitpanes__splitter) {
    display: none;
  }

  .strategy-native-page--mode-instruction .strategy-native-instruction > :deep(.splitpanes__pane:last-of-type),
  .strategy-native-page--mode-split .strategy-native-instruction > :deep(.splitpanes__pane:last-of-type) {
    width: 100% !important;
    max-width: 100% !important;
    flex: 0 0 100% !important;
  }

  .strategy-native-page--mode-code .strategy-native-shell > :deep(.splitpanes__pane:first-of-type) {
    position: absolute !important;
    z-index: 30;
    inset: 0 auto 0 0;
    width: min(360px, calc(100% - 48px)) !important;
    max-width: min(360px, calc(100% - 48px)) !important;
    min-width: min(280px, calc(100% - 48px)) !important;
    flex: 0 0 min(360px, calc(100% - 48px)) !important;
    transform: translateX(0);
    transition: transform 160ms ease;
    box-shadow: 16px 0 36px rgba(2, 6, 23, 0.3);
  }

  .strategy-native-page--mode-code .strategy-native-shell > :deep(.splitpanes__splitter) {
    display: none;
  }

  .strategy-native-page--mode-code .strategy-native-shell > :deep(.splitpanes__pane:last-of-type) {
    width: 100% !important;
    max-width: 100% !important;
    flex: 0 0 100% !important;
  }

  .strategy-native-page--metadata-closed.strategy-native-page--mode-instruction .strategy-native-instruction > :deep(.splitpanes__pane:first-of-type),
  .strategy-native-page--metadata-closed.strategy-native-page--mode-split .strategy-native-instruction > :deep(.splitpanes__pane:first-of-type),
  .strategy-native-page--metadata-closed.strategy-native-page--mode-code .strategy-native-shell > :deep(.splitpanes__pane:first-of-type) {
    pointer-events: none;
    transform: translateX(-105%);
    box-shadow: none;
  }

  .strategy-native-drawer-head {
    display: flex;
  }
}

@media (max-width: 920px) and (min-width: 769px) {
  .strategy-native-metadata-toggle span,
  .strategy-native-active-definition,
  .strategy-native-chip--diagnostic {
    display: none;
  }

  .strategy-native-title-block h1 {
    font-size: 0.86rem;
  }
}

@media (max-width: 768px) {
  .strategy-native-page {
    gap: 0;
    padding: 0;
  }

  .strategy-native-header {
    min-height: 44px;
    height: auto;
    flex: 0 0 auto;
    flex-flow: row wrap;
    gap: 4px 8px;
    padding: 5px 6px;
  }

  .strategy-native-header__identity {
    flex-basis: 100%;
  }

  .strategy-native-metadata-toggle,
  .strategy-native-active-definition {
    display: none;
  }

  .strategy-native-title-block {
    flex: 1 1 auto;
  }

  .strategy-native-header__actions {
    width: 100%;
    justify-content: flex-start;
  }

  .strategy-native-history-button,
  .strategy-native-action-button {
    min-height: 40px;
    height: 40px;
  }

  .strategy-native-history-button {
    width: 36px;
  }

  .strategy-native-view-switch {
    display: none;
  }

  .strategy-native-mobile-switch {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    min-height: 40px;
    flex: 0 0 40px;
    gap: 1px;
    border-width: 0 0 1px;
    border-radius: 0;
    border-color: var(--tv-border);
    background: var(--tv-bg-surface);
    padding: 3px 6px;
  }

  .strategy-native-mobile-switch__button {
    min-width: 0;
    min-height: 34px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: var(--tv-text-muted);
    padding: 0 6px;
    font-size: 0.77rem;
    font-weight: 800;
    white-space: nowrap;
  }

  .strategy-native-mobile-switch__button.is-active {
    background: color-mix(in srgb, var(--tv-accent) 18%, var(--tv-bg-surface));
    color: var(--tv-text);
  }

  .strategy-native-shell,
  .strategy-native-instruction {
    display: block !important;
    width: 100%;
    height: 100%;
    min-height: 0;
  }

  .strategy-native-shell :deep(.splitpanes__splitter),
  .strategy-native-instruction :deep(.splitpanes__splitter) {
    display: none !important;
  }

  .strategy-native-shell :deep(.splitpanes__pane),
  .strategy-native-instruction :deep(.splitpanes__pane) {
    width: 100% !important;
    max-width: 100% !important;
    min-width: 0 !important;
    height: 100% !important;
    max-height: 100% !important;
    min-height: 0 !important;
    flex: none !important;
    transform: none !important;
  }

  .strategy-native-page--mobile-definition .strategy-native-shell :deep(.splitpanes__pane:has(.strategy-native-code-pane)),
  .strategy-native-page--mobile-definition .strategy-native-instruction :deep(.splitpanes__pane:has(.strategy-native-main)),
  .strategy-native-page--mobile-instruction .strategy-native-shell :deep(.splitpanes__pane:has(.strategy-native-code-pane)),
  .strategy-native-page--mobile-instruction .strategy-native-instruction :deep(.splitpanes__pane:has(.strategy-native-side)),
  .strategy-native-page--mobile-code .strategy-native-shell :deep(.splitpanes__pane:has(.strategy-native-instruction)) {
    display: none !important;
  }

  .strategy-native-side,
  .strategy-native-main,
  .strategy-native-code-pane {
    padding: 0;
  }

  .strategy-native-workspace-bar {
    min-height: 42px;
    height: auto;
    flex-basis: auto;
    align-items: center;
    flex-direction: row;
  }

  .strategy-native-selected-block,
  .strategy-native-execution-hint {
    display: none;
  }

  .strategy-native-panel__content {
    padding: 8px;
  }

  .strategy-native-panel input,
  .strategy-native-panel select,
  .strategy-native-panel textarea {
    min-height: 40px;
  }

  .strategy-native-panel-tools,
  .strategy-native-icon-button {
    min-height: 40px;
  }

  .strategy-native-icon-button {
    width: 40px;
    height: 40px;
  }

  .strategy-native-block-scroll {
    padding: 6px;
  }
}
</style>
