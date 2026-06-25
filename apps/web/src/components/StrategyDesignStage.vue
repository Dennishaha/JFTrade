<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import type {
  BacktestSyncRequestPayload,
  PineV6WorkflowBlock,
  PineV6WorkflowDocument,
  PineV6WorkflowInput,
  StrategyBindingInstrumentDocument,
  StrategyDefinitionDocument,
  StrategyExecutionMode,
  StrategyInstanceItem,
  StrategyRuntimeRiskMode,
  StrategyRuntimeRiskSettings,
} from "@/contracts";

import { KLINE_PERIODS } from "../charting/kline";
import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import { useConsoleData } from "../composables/useConsoleData";
import { useKlineSyncTask } from "../composables/useKlineSyncTask";
import { useMarketProfiles } from "../composables/marketProfiles";
import {
  buildStrategyBindingPayload,
  defaultStrategyRuntimeRiskSettings,
  formatBrokerAccountSummary,
  formatStrategyInterval,
  formatStrategyRuntimeRiskSummary,
  formatStrategySymbols,
  normalizeStrategyRuntimeRiskSettings,
  readStrategyBinding,
  resolveBrokerAccountOption,
  resolveBrokerAccountSelectionKey,
} from "./strategy-runtime/strategyRuntimeInstanceBinding";
import MonacoCodeEditor from "./MonacoCodeEditor.vue";
import PineV6WorkflowBlockList from "./PineV6WorkflowBlockList.vue";
import SplitPane from "./shared/SplitPane.vue";
import SplitPaneItem from "./shared/SplitPaneItem.vue";
import {
  PINE_V6_WORKFLOW_ENGINE,
  assessPineV6Workflow,
  buildPineV6WorkflowScript,
  createDefaultPineV6Workflow,
  createWorkflowId,
  normalizePineV6Workflow,
  type PineV6WorkflowDiagnostic,
} from "../features/pineV6Workflow";
import {
  strategyPineEditorCompletions,
  strategyPineEditorExtraLibs,
  strategyPineEditorHoverItems,
} from "../features/strategyPineEditorIntelliSense";

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

const { availableBrokerAccounts, selectedBrokerAccount, loadBrokerSettings } = useConsoleData();
const {
  marketOptions,
  loadMarketProfiles,
  normalizeInstrumentRefWithMarketApi,
} = useMarketProfiles();
const {
  syncProgress,
  syncError,
  startSync,
  stopSyncPolling,
} = useKlineSyncTask();

const strategyDefinitions = ref<StrategyDefinitionDocument[]>([]);
const strategies = ref<StrategyInstanceItem[]>([]);
const selectedDefinitionId = ref("");
const selectedStrategyId = ref("");
const isLoadingDefinitions = ref(false);
const isLoadingStrategies = ref(false);
const isSavingDefinition = ref(false);
const isAnalyzing = ref(false);
const isStarting = ref(false);
const notice = ref("");
const error = ref("");
const analyzeResult = ref<StrategyPineAnalyzeResponse | null>(null);
const selectedBlockId = ref("");
const runtimeRefreshTimer = ref<ReturnType<typeof setInterval> | null>(null);

const workflow = ref<PineV6WorkflowDocument>(createDefaultPineV6Workflow());
const definitionName = ref(workflow.value.declaration.title);
const definitionVersion = ref("0.1.0");
const definitionDescription = ref("Pine v6 原生快捷指令工作台生成的策略。");
const sourceOverride = ref("");
const useSourceOverride = ref(false);
const advancedSourceOpen = ref(false);
const strategyDisplayMode = ref<"instruction" | "split" | "code">("split");
const variablesPanelOpen = ref(false);
const strategySidePanelIds = [
  "definition",
  "declaration",
  "runtime",
  "risk",
  "diagnostics",
  "instances",
] as const;
const expandedStrategySidePanels = ref<string[]>([...strategySidePanelIds]);

const bindingMarket = ref(workflow.value.runtimeBindingDraft.market);
const bindingCode = ref(workflow.value.runtimeBindingDraft.code);
const bindingInterval = ref(workflow.value.runtimeBindingDraft.interval);
const executionMode = ref<StrategyExecutionMode>(workflow.value.runtimeBindingDraft.executionMode);
const useExtendedHours = ref(workflow.value.runtimeBindingDraft.useExtendedHours);
const brokerAccountKey = ref("");
const runtimeRisk = ref<StrategyRuntimeRiskSettings>(defaultStrategyRuntimeRiskSettings());

const generatedScript = computed(() => buildPineV6WorkflowScript(workflow.value));
const activeScript = computed(() => useSourceOverride.value ? sourceOverride.value : generatedScript.value);
const workflowDiagnostics = computed(() => assessPineV6Workflow(workflow.value));
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
const canStart = computed(() =>
  !isStarting.value
  && definitionName.value.trim() !== ""
  && bindingMarket.value.trim() !== ""
  && bindingCode.value.trim() !== ""
  && bindingInterval.value.trim() !== ""
  && workflowErrorCount.value === 0,
);
const selectedStrategy = computed(() =>
  strategies.value.find((strategy) => strategy.id === selectedStrategyId.value) ?? null,
);
const selectedDefinition = computed(() =>
  strategyDefinitions.value.find((definition) => definition.id === selectedDefinitionId.value) ?? null,
);
const normalizedBrokerAccount = computed(() =>
  resolveBrokerAccountOption(availableBrokerAccounts.value, brokerAccountKey.value),
);
const warmupBars = computed(() => selectedDefinition.value?.derivedWarmupBars ?? 120);
const klineSyncWindow = computed(() => deriveKlineSyncWindow(warmupBars.value, bindingInterval.value));
const activeWorkflowFeatureSummary = computed(() => {
  const featureCount = analyzeResult.value?.features?.length ?? 0;
  const pieces = [
    `${workflow.value.blocks.length} 个顶层块`,
    `${countWorkflowBlocks(workflow.value.blocks)} 个总块`,
    `${featureCount} 个 Pine 分析特征`,
  ];
  return pieces.join(" / ");
});

function executionModeLabel(mode: StrategyExecutionMode): string {
  return mode === "notify_only" ? "仅通知" : "实盘执行";
}

function riskModeLabel(mode: StrategyRuntimeRiskMode): string {
  switch (mode) {
    case "monitor":
      return "监控";
    case "enforce":
      return "强制";
    default:
      return "关闭";
  }
}

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

function toggleVariablesPanel(): void {
  variablesPanelOpen.value = !variablesPanelOpen.value;
}

onMounted(() => {
  void loadMarketProfiles();
  void loadBrokerSettings().catch(() => undefined);
  void loadStrategyDefinitions(selectedDefinitionId.value, { applyDefinition: props.entryMode !== "new" });
  void loadStrategies();
  runtimeRefreshTimer.value = setInterval(() => {
    void loadStrategies(selectedStrategyId.value);
  }, 3000);
});

onBeforeUnmount(() => {
  stopSyncPolling();
  if (runtimeRefreshTimer.value !== null) {
    clearInterval(runtimeRefreshTimer.value);
  }
});

watch(
  () => availableBrokerAccounts.value,
  () => {
    if (brokerAccountKey.value === "" && selectedBrokerAccount.value !== null) {
      brokerAccountKey.value = selectedBrokerAccount.value.selectionKey;
    }
  },
  { immediate: true },
);

watch(
  [bindingMarket, bindingCode, bindingInterval, executionMode, useExtendedHours, brokerAccountKey, runtimeRisk],
  () => {
    workflow.value.runtimeBindingDraft = {
      market: bindingMarket.value.trim().toUpperCase(),
      code: bindingCode.value.trim().toUpperCase(),
      interval: bindingInterval.value.trim() || "5m",
      executionMode: executionMode.value,
      useExtendedHours: useExtendedHours.value,
      brokerAccountKey: brokerAccountKey.value,
      runtimeRisk: runtimeRisk.value,
    };
  },
  { deep: true },
);

watch(generatedScript, (script) => {
  if (!useSourceOverride.value) {
    sourceOverride.value = script;
  }
}, { immediate: true });

async function loadStrategyDefinitions(
  preferredId = selectedDefinitionId.value,
  options: { applyDefinition?: boolean } = {},
): Promise<void> {
  isLoadingDefinitions.value = true;
  error.value = "";
  try {
    const definitions = await fetchEnvelope<StrategyDefinitionDocument[]>("/api/v1/strategy-definitions");
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

async function loadStrategies(preferredId = selectedStrategyId.value): Promise<void> {
  isLoadingStrategies.value = true;
  try {
    const items = await fetchEnvelope<StrategyInstanceItem[]>("/api/v1/strategies");
    strategies.value = items;
    selectedStrategyId.value = items.some((item) => item.id === preferredId)
      ? preferredId
      : items[0]?.id ?? "";
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
  hydrateBindingDraft(workflow.value);
  notice.value = `已加载 ${definition.name} / v${definition.version}`;
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
  hydrateBindingDraft(workflow.value);
  analyzeResult.value = null;
  notice.value = "已创建 Pine v6 工作流草稿。";
}

function hydrateBindingDraft(nextWorkflow: PineV6WorkflowDocument): void {
  bindingMarket.value = nextWorkflow.runtimeBindingDraft.market || "HK";
  bindingCode.value = nextWorkflow.runtimeBindingDraft.code || "00700";
  bindingInterval.value = nextWorkflow.runtimeBindingDraft.interval || "5m";
  executionMode.value = nextWorkflow.runtimeBindingDraft.executionMode === "notify_only" ? "notify_only" : "live";
  useExtendedHours.value = nextWorkflow.runtimeBindingDraft.useExtendedHours === true;
  brokerAccountKey.value = nextWorkflow.runtimeBindingDraft.brokerAccountKey ?? brokerAccountKey.value;
  runtimeRisk.value = normalizeStrategyRuntimeRiskSettings(nextWorkflow.runtimeBindingDraft.runtimeRisk);
}

function updateDeclaration<K extends keyof PineV6WorkflowDocument["declaration"]>(
  key: K,
  value: PineV6WorkflowDocument["declaration"][K],
): void {
  workflow.value.declaration = {
    ...workflow.value.declaration,
    [key]: value,
  };
  if (key === "title" && definitionName.value.trim() === "") {
    definitionName.value = String(value);
  }
}

function addInput(): void {
  workflow.value.inputs = [
    ...workflow.value.inputs,
    {
      id: createWorkflowId("input"),
      name: `input${workflow.value.inputs.length + 1}`,
      title: "输入参数",
      type: "int",
      defaultValue: "1",
    },
  ];
}

function updateInput(index: number, patch: Partial<PineV6WorkflowInput>): void {
  workflow.value.inputs = workflow.value.inputs.map((input, inputIndex) =>
    inputIndex === index ? { ...input, ...patch } : input,
  );
}

function deleteInput(index: number): void {
  workflow.value.inputs = workflow.value.inputs.filter((_, inputIndex) => inputIndex !== index);
}

function updateBlocks(blocks: PineV6WorkflowBlock[]): void {
  workflow.value.blocks = blocks;
}

async function analyzeCurrentScript(): Promise<boolean> {
  isAnalyzing.value = true;
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
    notice.value = `Pine v6 分析通过：${result.features?.length ?? 0} 个特征。`;
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
  if (workflowErrorCount.value > 0) {
    error.value = "工作流存在错误，不能保存。";
    return null;
  }
  if (options.requireAnalysis === true && !await analyzeCurrentScript()) {
    return null;
  }
  isSavingDefinition.value = true;
  error.value = "";
  try {
    const payload: StrategyDefinitionDocument = {
      id: selectedDefinitionId.value,
      name: definitionName.value.trim() || workflow.value.declaration.title || "Pine v6 策略",
      version: definitionVersion.value.trim() || "0.1.0",
      description: definitionDescription.value.trim(),
      runtime: "pine-go-plan",
      sourceFormat: "pine-v6",
      script: activeScript.value,
      visualModel: workflow.value,
      createdAt: selectedDefinition.value?.createdAt ?? "",
      updatedAt: selectedDefinition.value?.updatedAt ?? "",
    };
    const existing = strategyDefinitions.value.some((definition) => definition.id === selectedDefinitionId.value);
    const saved = await fetchEnvelopeWithInit<StrategyDefinitionDocument>(
      existing
        ? `/api/v1/strategy-definitions/${encodeURIComponent(selectedDefinitionId.value)}`
        : "/api/v1/strategy-definitions",
      {
        method: existing ? "PUT" : "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      },
    );
    selectedDefinitionId.value = saved.id;
    notice.value = `已保存策略定义：${saved.name} / v${saved.version}`;
    await loadStrategyDefinitions(saved.id);
    return saved;
  } catch (cause) {
    error.value = `保存策略定义失败: ${cause instanceof Error ? cause.message : String(cause)}`;
    return null;
  } finally {
    isSavingDefinition.value = false;
  }
}

async function startUnifiedRuntime(): Promise<void> {
  if (!canStart.value) {
    error.value = "启动条件不足，请检查策略、标的、周期和工作流诊断。";
    return;
  }
  isStarting.value = true;
  error.value = "";
  notice.value = "";
  try {
    const saved = await saveDefinition({ requireAnalysis: true });
    if (saved === null) {
      return;
    }
    const normalized = await normalizeInstrumentRefWithMarketApi({
      market: bindingMarket.value,
      code: bindingCode.value,
    });
    const syncPayload: BacktestSyncRequestPayload = {
      market: normalized.prefix,
      code: normalized.code,
      symbol: normalized.instrumentId,
      intervals: [bindingInterval.value],
      startDate: klineSyncWindow.value.startDate,
      endDate: klineSyncWindow.value.endDate,
      rehabType: "forward",
      sessionScope: useExtendedHours.value ? "extended" : "regular",
    };
    const progress = await startSync(syncPayload);
    if (progress === null || progress.status !== "completed") {
      error.value = syncError.value || `启动前数据同步未完成：${progress?.status ?? "unknown"}`;
      return;
    }
    const instance = await createOrUpdateStoppedInstance(saved, {
      market: normalized.prefix,
      code: normalized.code,
    });
    if (instance.status !== "STOPPED") {
      error.value = "实例不是 STOPPED，不能启动。";
      await loadStrategies(instance.id);
      return;
    }
    await fetchEnvelopeWithInit<StrategyInstanceItem>(
      `/api/v1/strategies/${encodeURIComponent(instance.id)}/start`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      },
    );
    selectedStrategyId.value = instance.id;
    await loadStrategies(instance.id);
    notice.value = `已启动策略实例：${saved.name}`;
    emit("switch-to-runtime", { notice: notice.value, definitionId: saved.id });
  } catch (cause) {
    error.value = `启动失败: ${cause instanceof Error ? cause.message : String(cause)}`;
  } finally {
    isStarting.value = false;
  }
}

async function createOrUpdateStoppedInstance(
  definition: StrategyDefinitionDocument,
  instrument: StrategyBindingInstrumentDocument,
): Promise<StrategyInstanceItem> {
  const bindingPayload = buildStrategyBindingPayload({
    brokerAccountOptions: availableBrokerAccounts.value,
    instruments: [instrument],
    interval: bindingInterval.value,
    executionMode: executionMode.value,
    brokerAccountKey: brokerAccountKey.value,
    runtimeRisk: runtimeRisk.value,
  });
  const selected = selectedStrategy.value;
  if (selected !== null && selected.definition.strategyId === definition.id) {
    if (selected.status !== "STOPPED") {
      throw new Error("当前选中实例不是 STOPPED，请先停止后再更新绑定。");
    }
    return fetchEnvelopeWithInit<StrategyInstanceItem>(
      `/api/v1/strategies/${encodeURIComponent(selected.id)}`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(bindingPayload),
      },
    );
  }
  return fetchEnvelopeWithInit<StrategyInstanceItem>(
    `/api/v1/strategy-definitions/${encodeURIComponent(definition.id)}/instantiate`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(bindingPayload),
    },
  );
}

async function changeStrategyStatus(instance: StrategyInstanceItem, action: "start" | "pause" | "stop"): Promise<void> {
  error.value = "";
  try {
    await fetchEnvelopeWithInit<StrategyInstanceItem>(
      `/api/v1/strategies/${encodeURIComponent(instance.id)}/${action}`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      },
    );
    await loadStrategies(instance.id);
  } catch (cause) {
    error.value = `策略${action}失败: ${cause instanceof Error ? cause.message : String(cause)}`;
  }
}

function setRuntimeRiskMode(value: string): void {
  const mode: StrategyRuntimeRiskMode =
    value === "monitor" || value === "enforce" ? value : "off";
  runtimeRisk.value = normalizeStrategyRuntimeRiskSettings({
    ...runtimeRisk.value,
    mode,
  });
}

function updateRuntimeRiskNumber(
  field: "maxOrderQuantity" | "maxOrderNotional" | "dailyMaxOrders",
  value: string,
): void {
  const numeric = value.trim() === "" ? null : Number(value);
  runtimeRisk.value = normalizeStrategyRuntimeRiskSettings({
    ...runtimeRisk.value,
    [field]: Number.isFinite(numeric) ? numeric : null,
  });
}

function deriveKlineSyncWindow(warmup: number, interval: string): { startDate: string; endDate: string } {
  const today = new Date();
  const intervalMinutes = intervalToMinutes(interval);
  const tradingDays = Math.ceil(Math.max(120, warmup) * intervalMinutes / 240) + 8;
  const start = new Date(today);
  start.setDate(today.getDate() - Math.max(14, tradingDays * 2));
  return {
    startDate: formatDateInput(start),
    endDate: formatDateInput(today),
  };
}

function intervalToMinutes(interval: string): number {
  switch (interval) {
    case "1m": return 1;
    case "5m": return 5;
    case "15m": return 15;
    case "30m": return 30;
    case "1h": return 60;
    case "1d": return 240;
    case "1w": return 1200;
    default: return 5;
  }
}

function formatDateInput(value: Date): string {
  return value.toISOString().slice(0, 10);
}

function countWorkflowBlocks(blocks: PineV6WorkflowBlock[]): number {
  return blocks.reduce(
    (total, block) => total + 1 + countWorkflowBlocks(block.thenBlocks ?? []) + countWorkflowBlocks(block.elseBlocks ?? []),
    0,
  );
}

function inputTypeValue(value: string): PineV6WorkflowInput["type"] {
  return value === "float" || value === "bool" || value === "string" || value === "source" || value === "time"
    ? value
    : "int";
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
          {{ isAnalyzing ? "分析中" : "分析" }}
        </button>
        <button type="button" :disabled="isSavingDefinition" @click="void saveDefinition()">
          {{ isSavingDefinition ? "保存中" : "保存" }}
        </button>
        <button class="strategy-native-primary" type="button" :disabled="!canStart" @click="void startUnifiedRuntime()">
          {{ isStarting ? "启动中" : "保存并启动" }}
        </button>
      </div>
    </header>

    <div v-if="notice" class="strategy-native-banner strategy-native-banner--ok">{{ notice }}</div>
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
                    <div class="strategy-native-panel__title">策略声明 strategy(...)</div>
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="strategy-native-panel__content">
                      <label>
                        <span>标题</span>
                        <input :value="workflow.declaration.title" @input="updateDeclaration('title', ($event.target as HTMLInputElement).value)">
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

                <v-expansion-panel value="runtime" class="strategy-native-panel strategy-native-side-panel">
                  <v-expansion-panel-title>
                    <div class="strategy-native-panel__title">运行向导</div>
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="strategy-native-panel__content">
                      <label>
                        <span>市场</span>
                        <select v-model="bindingMarket">
                          <option v-for="market in marketOptions" :key="market.value" :value="market.value">{{ market.title }}</option>
                          <option v-if="marketOptions.length === 0" value="HK">HK</option>
                          <option v-if="marketOptions.length === 0" value="US">US</option>
                        </select>
                      </label>
                      <label>
                        <span>代码</span>
                        <input v-model="bindingCode" placeholder="00700">
                      </label>
                      <label>
                        <span>K线周期</span>
                        <select v-model="bindingInterval">
                          <option v-for="period in KLINE_PERIODS.filter((item) => item.value !== 'tick')" :key="period.value" :value="period.value">
                            {{ period.label }}
                          </option>
                        </select>
                      </label>
                      <label>
                        <span>执行模式</span>
                        <select v-model="executionMode">
                          <option value="live">{{ executionModeLabel("live") }}</option>
                          <option value="notify_only">{{ executionModeLabel("notify_only") }}</option>
                        </select>
                      </label>
                      <label class="strategy-native-toggle">
                        <input v-model="useExtendedHours" type="checkbox">
                        <span>包含扩展交易时段</span>
                      </label>
                      <label>
                        <span>交易账号</span>
                        <select v-model="brokerAccountKey">
                          <option value="">未绑定账号</option>
                          <option v-for="account in availableBrokerAccounts" :key="account.selectionKey" :value="account.selectionKey">
                            {{ account.displayName }} / {{ account.market }}
                          </option>
                        </select>
                      </label>
                      <div class="strategy-native-meta">
                        {{ normalizedBrokerAccount ? formatBrokerAccountSummary(normalizedBrokerAccount) : "未绑定账号" }}
                      </div>
                    </div>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel value="risk" class="strategy-native-panel strategy-native-side-panel">
                  <v-expansion-panel-title>
                    <div class="strategy-native-panel__title">动态风控</div>
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="strategy-native-panel__content">
                      <label>
                        <span>模式</span>
                        <select :value="runtimeRisk.mode" @change="setRuntimeRiskMode(($event.target as HTMLSelectElement).value)">
                          <option value="off">{{ riskModeLabel("off") }}</option>
                          <option value="monitor">{{ riskModeLabel("monitor") }}</option>
                          <option value="enforce">{{ riskModeLabel("enforce") }}</option>
                        </select>
                      </label>
                      <label class="strategy-native-toggle">
                        <input :checked="runtimeRisk.closeOnly" type="checkbox" @change="runtimeRisk = normalizeStrategyRuntimeRiskSettings({ ...runtimeRisk, closeOnly: ($event.target as HTMLInputElement).checked })">
                        <span>仅允许平仓</span>
                      </label>
                      <label>
                        <span>最大下单数量</span>
                        <input :value="runtimeRisk.maxOrderQuantity ?? ''" type="number" @input="updateRuntimeRiskNumber('maxOrderQuantity', ($event.target as HTMLInputElement).value)">
                      </label>
                      <label>
                        <span>最大订单金额</span>
                        <input :value="runtimeRisk.maxOrderNotional ?? ''" type="number" @input="updateRuntimeRiskNumber('maxOrderNotional', ($event.target as HTMLInputElement).value)">
                      </label>
                      <div class="strategy-native-meta">{{ formatStrategyRuntimeRiskSummary(runtimeRisk) }}</div>
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
                      <button type="button" :disabled="isLoadingStrategies" @click.stop="void loadStrategies()">刷新</button>
                    </div>
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="strategy-native-panel__content">
                      <div v-if="strategies.length === 0" class="strategy-native-meta">暂无实例。</div>
                      <button
                        v-for="strategy in strategies"
                        :key="strategy.id"
                        class="strategy-native-instance"
                        :class="{ 'strategy-native-instance--active': strategy.id === selectedStrategyId }"
                        type="button"
                        @click="selectedStrategyId = strategy.id"
                      >
                        <div>
                          <strong>{{ strategy.definition.name }}</strong>
                          <span :class="['strategy-native-status', statusClass(strategy.status)]">{{ statusLabel(strategy.status) }}</span>
                        </div>
                        <div>{{ formatStrategySymbols(strategy) }} / {{ formatStrategyInterval(strategy) }}</div>
                        <div>{{ formatStrategyRuntimeRiskSummary(readStrategyBinding(strategy).runtimeRisk) }}</div>
                        <div class="strategy-native-instance__actions">
                          <button type="button" :disabled="strategy.status !== 'STOPPED'" @click.stop="void changeStrategyStatus(strategy, 'start')">启动</button>
                          <button type="button" :disabled="strategy.status !== 'RUNNING'" @click.stop="void changeStrategyStatus(strategy, 'pause')">暂停</button>
                          <button type="button" :disabled="strategy.status === 'STOPPED'" @click.stop="void changeStrategyStatus(strategy, 'stop')">停止</button>
                        </div>
                      </button>
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
                    <div class="strategy-native-panel__title">收盘确认指令</div>
                    <div class="strategy-native-meta">
                      收盘确认执行 / 下一根 K 线成交 / {{ activeWorkflowFeatureSummary }}
                    </div>
                  </div>
                  <div class="strategy-native-selected-block">
                    {{ selectedBlockId || "未选中块" }}
                  </div>
                </div>
                <div class="strategy-native-block-scroll" data-testid="strategy-instruction-scroll">
                  <PineV6WorkflowBlockList
                    class="strategy-native-block-list"
                    :blocks="workflow.blocks"
                    @select-block="selectedBlockId = $event"
                    @update:blocks="updateBlocks"
                  />
                </div>
              </section>

              <section
                class="strategy-native-panel strategy-native-variables"
                :class="{ 'strategy-native-variables--open': variablesPanelOpen }"
                data-testid="strategy-variables-panel"
              >
                <div
                  class="strategy-native-workspace-bar strategy-native-variables__bar"
                  role="button"
                  tabindex="0"
                  @click="toggleVariablesPanel"
                  @keydown.enter.prevent="toggleVariablesPanel"
                  @keydown.space.prevent="toggleVariablesPanel"
                >
                  <div>
                    <div class="strategy-native-panel__title">输入参数 input.*</div>
                    <div class="strategy-native-meta">{{ workflow.inputs.length }} 个变量</div>
                  </div>
                  <div class="strategy-native-variables__actions">
                    <button type="button" @click.stop="variablesPanelOpen = true; addInput()">新增输入参数</button>
                    <span class="strategy-native-variables__chevron">{{ variablesPanelOpen ? "收起" : "展开" }}</span>
                  </div>
                </div>
                <div class="strategy-native-inputs strategy-native-variables__body" data-testid="strategy-variables-body">
                  <div v-for="(input, index) in workflow.inputs" :key="input.id" class="strategy-native-input-row">
                    <input :value="input.name" placeholder="变量名" @input="updateInput(index, { name: ($event.target as HTMLInputElement).value })">
                    <input :value="input.title" placeholder="标题" @input="updateInput(index, { title: ($event.target as HTMLInputElement).value })">
                    <select :value="input.type" @change="updateInput(index, { type: inputTypeValue(($event.target as HTMLSelectElement).value) })">
                      <option value="int">整数 int</option>
                      <option value="float">小数 float</option>
                      <option value="bool">布尔 bool</option>
                      <option value="string">文本 string</option>
                      <option value="source">价格序列 source</option>
                      <option value="time">时间 time</option>
                    </select>
                    <input :value="input.defaultValue" placeholder="默认值" @input="updateInput(index, { defaultValue: ($event.target as HTMLInputElement).value })">
                    <button type="button" @click="deleteInput(index)">×</button>
                  </div>
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
        <aside class="strategy-native-code-pane">
        <section class="strategy-native-panel strategy-native-code">
          <div class="strategy-native-workspace-bar">
            <div>
              <div class="strategy-native-panel__title">Pine v6 源码</div>
              <div class="strategy-native-meta">后端 script 字段是运行权威输入</div>
            </div>
            <label class="strategy-native-toggle">
              <input v-model="useSourceOverride" data-testid="strategy-source-override-toggle" type="checkbox">
              <span>源码编辑</span>
            </label>
          </div>
          <MonacoCodeEditor
            :model-value="useSourceOverride ? sourceOverride : generatedScript"
            language="pine-v6"
            height="min(56vh, 34rem)"
            min-height="24rem"
            test-id="strategy-script-editor"
            :read-only="!useSourceOverride"
            :extra-libs="strategyPineEditorExtraLibs"
            :completion-items="strategyPineEditorCompletions"
            :hover-items="strategyPineEditorHoverItems"
            :diagnostic-markers="pineDiagnosticMarkers"
            @update:model-value="sourceOverride = $event"
          />
          <button type="button" @click="advancedSourceOpen = !advancedSourceOpen">
            {{ advancedSourceOpen ? "收起说明" : "Pine v6 支持边界" }}
          </button>
          <div v-if="advancedSourceOpen" class="strategy-native-meta">
            当前按闭合 K 线执行；订单按下一根 K 线成交；OCA、部分成交、tick 级重算是明确边界。
          </div>
        </section>

        </aside>
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
.strategy-native-workspace-bar,
.strategy-native-instance__actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem;
  justify-content: space-between;
}

.strategy-native-header__actions {
  justify-content: flex-end;
}

.strategy-native-view-switch {
  display: inline-flex;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 72%, transparent);
  padding: 0.18rem;
}

.strategy-native-view-switch__button {
  min-height: 2rem;
  border: 0;
  border-radius: 999px;
  background: transparent;
  padding: 0.35rem 0.7rem;
  color: var(--tv-text-muted);
  font-size: 0.8rem;
  font-weight: 800;
}

.strategy-native-view-switch__button.is-active {
  background: var(--tv-text);
  color: var(--tv-bg);
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
.strategy-native-main,
.strategy-native-code-pane {
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

.strategy-native-block-list {
  width: 100%;
  min-width: 0;
  justify-self: stretch;
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

.strategy-native-primary {
  border-color: color-mix(in srgb, var(--tv-accent) 70%, var(--tv-border));
  background: var(--tv-accent);
  color: white;
}

.strategy-native-banner {
  flex-shrink: 0;
  border-radius: 0.5rem;
  padding: 0.65rem 0.8rem;
  font-size: 0.9rem;
}

.strategy-native-banner--ok {
  border: 1px solid #bbf7d0;
  background: #f0fdf4;
  color: #166534;
}

.strategy-native-banner--error {
  border: 1px solid #fecaca;
  background: #fef2f2;
  color: #991b1b;
}

.strategy-native-meta,
.strategy-native-selected-block {
  color: var(--tv-text-muted);
  font-size: 0.82rem;
  line-height: 1.45;
}

.strategy-native-inputs {
  display: grid;
  gap: 0.5rem;
}

.strategy-native-variables {
  align-self: end;
  min-height: 0;
  max-height: 4.25rem;
  overflow: hidden;
  grid-template-rows: auto minmax(0, 1fr);
  gap: 0;
  transition: max-height 160ms ease, border-color 160ms ease, box-shadow 160ms ease;
}

.strategy-native-variables--open {
  max-height: min(42vh, 24rem);
  border-color: color-mix(in srgb, var(--tv-accent) 42%, var(--tv-border));
  box-shadow: 0 -12px 28px rgba(15, 23, 42, 0.08);
}

.strategy-native-variables__bar {
  min-height: 2.5rem;
  cursor: pointer;
}

.strategy-native-variables__bar:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--tv-accent) 72%, transparent);
  outline-offset: 3px;
  border-radius: 0.35rem;
}

.strategy-native-variables__actions {
  display: inline-flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.strategy-native-variables__chevron {
  color: var(--tv-text-muted);
  font-size: 0.78rem;
  font-weight: 800;
}

.strategy-native-variables__body {
  min-height: 0;
  max-height: 0;
  overflow: auto;
  opacity: 0;
  padding-top: 0;
  pointer-events: none;
  scrollbar-gutter: stable;
  transition: max-height 160ms ease, opacity 120ms ease, padding-top 160ms ease;
}

.strategy-native-variables--open .strategy-native-variables__body {
  max-height: 18rem;
  opacity: 1;
  padding-top: 0.75rem;
  pointer-events: auto;
}

.strategy-native-input-row {
  display: grid;
  grid-template-columns: minmax(6rem, 1fr) minmax(6rem, 1fr) 6rem minmax(6rem, 1fr) auto;
  gap: 0.5rem;
}

.strategy-native-code :deep(.monaco-code-editor-shell) {
  border-radius: 0.5rem;
  border-color: var(--tv-border);
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
  border-color: #fecaca;
  background: #fef2f2;
  color: #991b1b;
}

.strategy-native-diagnostic--warning {
  border-color: #fde68a;
  background: #fffbeb;
  color: #92400e;
}

.strategy-native-diagnostic--info {
  border-color: #bfdbfe;
  background: #eff6ff;
  color: #1e40af;
}

.strategy-native-instance {
  display: grid;
  width: 100%;
  gap: 0.4rem;
  text-align: left;
}

.strategy-native-instance--active {
  border-color: var(--tv-accent);
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
  background: #dcfce7;
  color: #166534;
}

.strategy-native-status--paused {
  background: #fef3c7;
  color: #92400e;
}

.strategy-native-status--stopped {
  background: #e2e8f0;
  color: #334155;
}

@media (max-width: 860px) {
  .strategy-native-header,
  .strategy-native-input-row {
    grid-template-columns: 1fr;
  }

  .strategy-native-header {
    align-items: stretch;
  }
}
</style>
