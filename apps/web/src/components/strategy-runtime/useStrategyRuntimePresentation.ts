import { computed, type Ref } from "vue";

import type { BrokerAccountSelectionOption } from "@/composables/trading/consoleDataBrokerAccountSelection";
import type {
  StrategyDefinitionDocument,
  StrategyDefinitionSyncStatus,
  StrategyInstanceBindingDocument,
  StrategyInstanceItem,
  StrategyRuntimeObservation,
  SystemStatusResponse,
} from "@/types";
import { formatSourceFormat, formatStrategyRuntime, isPineWorkerRuntime } from "./strategyRuntimeIdentity";
import { readStrategyBinding } from "./strategyRuntimeInstanceBinding";
import { readCompiledHookCount, readCompiledIndicatorCount } from "./strategyRuntimePresentation";

interface SelectionOptions {
  strategies: Ref<StrategyInstanceItem[]>;
  selectedStrategyId: Ref<string>;
  strategyDefinitions: Ref<StrategyDefinitionDocument[]>;
  availableBrokerAccounts: Readonly<Ref<BrokerAccountSelectionOption[]>>;
  selectedBrokerAccount: Readonly<Ref<BrokerAccountSelectionOption | null>>;
  systemStatus: Readonly<Ref<SystemStatusResponse>>;
  isLoadingStrategies: Ref<boolean>;
  isLoadingDetails: Ref<boolean>;
}

export function useStrategyRuntimeSelection(options: SelectionOptions) {
  const selectedStrategy = computed(
    () => options.strategies.value.find((item) => item.id === options.selectedStrategyId.value) ?? null,
  );
  const selectedStrategyBinding = computed<StrategyInstanceBindingDocument | null>(() =>
    selectedStrategy.value === null ? null : readStrategyBinding(selectedStrategy.value),
  );
  const selectedStrategyDefinitionSync = computed<StrategyDefinitionSyncStatus | null>(
    () => selectedStrategy.value?.definitionSync ?? null,
  );
  const selectedStrategyDefinitionDocument = computed<StrategyDefinitionDocument | null>(() => {
    if (selectedStrategy.value === null) return null;
    const definitionId = selectedStrategy.value.definition.strategyId;
    return options.strategyDefinitions.value.find((item) => item.id === definitionId) ?? null;
  });
  const selectedStrategyRuntimeObservation = computed<StrategyRuntimeObservation | null>(
    () => selectedStrategy.value?.runtimeObservation ?? null,
  );
  const brokerAccountOptions = computed(() => options.availableBrokerAccounts.value);
  const activeStrategyCount = computed(
    () => options.strategies.value.filter(
      (item) => item.runtimeObservation?.actualStatus === "RUNNING",
    ).length,
  );
  const runtimeRealTradingLabel = computed(() =>
    options.systemStatus.value.realTradingEnabled ? "已开启" : "已关闭",
  );
  const runtimeKillSwitchLabel = computed(() =>
    options.systemStatus.value.realTradingKillSwitch.active ? "已启用" : "未启用",
  );
  const isRefreshingStrategyContent = computed(
    () => options.isLoadingStrategies.value || options.isLoadingDetails.value,
  );
  const defaultBrokerAccountSelectionKey = computed(
    () => options.selectedBrokerAccount.value?.selectionKey
      ?? brokerAccountOptions.value[0]?.selectionKey
      ?? "",
  );
  const currentBrokerAccountSelectionKey = computed(
    () => options.selectedBrokerAccount.value?.selectionKey ?? "",
  );
  const effectiveCurrentBrokerAccountSelectionKey = computed(
    () => currentBrokerAccountSelectionKey.value || defaultBrokerAccountSelectionKey.value,
  );

  return {
    selectedStrategy,
    selectedStrategyBinding,
    selectedStrategyDefinitionSync,
    selectedStrategyDefinitionDocument,
    selectedStrategyRuntimeObservation,
    brokerAccountOptions,
    activeStrategyCount,
    runtimeRealTradingLabel,
    runtimeKillSwitchLabel,
    isRefreshingStrategyContent,
    defaultBrokerAccountSelectionKey,
    currentBrokerAccountSelectionKey,
    effectiveCurrentBrokerAccountSelectionKey,
  };
}

interface DetailOptions {
  selectedStrategy: Readonly<Ref<StrategyInstanceItem | null>>;
  selectedStrategyBinding: Readonly<Ref<StrategyInstanceBindingDocument | null>>;
  selectedStrategyDefinitionSync: Readonly<Ref<StrategyDefinitionSyncStatus | null>>;
  createDefinitionId: Ref<string>;
  createInterval: Ref<string>;
  isLoadingDefinitions: Ref<boolean>;
  isCreatingStrategyInstance: Ref<boolean>;
  isLoadingDetails: Ref<boolean>;
  isRefreshingStrategyDefinition: Ref<boolean>;
  isUpdatingStrategyBinding: Ref<boolean>;
  isDeletingStrategy: Ref<boolean>;
}

export function useStrategyRuntimeDetailPresentation(options: DetailOptions) {
  const selectedStrategyParamsJson = computed(() =>
    options.selectedStrategy.value === null
      ? ""
      : JSON.stringify(options.selectedStrategy.value.params, null, 2),
  );
  const selectedStrategyRuntimeLabel = computed(() =>
    options.selectedStrategy.value === null
      ? "暂无"
      : formatStrategyRuntime(options.selectedStrategy.value.runtime),
  );
  const selectedStrategySourceFormatLabel = computed(() =>
    options.selectedStrategy.value === null
      ? "暂无"
      : formatSourceFormat(options.selectedStrategy.value.sourceFormat),
  );
  const selectedStrategyStartHint = computed(() => {
    const strategy = options.selectedStrategy.value;
    if (strategy === null) return "请选择策略实例。";
    if (options.selectedStrategyBinding.value?.executionMode === "notify_only") {
      return "当前实例为仅通知模式：触发信号只发送准备下单通知，不自动下单。";
    }
    if (strategy.startable) return "当前实例已接入策略控制面生命周期，可启动、暂停、停止。";
    if (isPineWorkerRuntime(strategy.runtime)) {
      return "当前实例已完成 Pine 编译与 requirements 规划，但暂不可启动。";
    }
    return "当前实例暂不可启动。";
  });
  const selectedStrategyCompiledSummary = computed(() => {
    const strategy = options.selectedStrategy.value;
    if (strategy === null || !isPineWorkerRuntime(strategy.runtime)) return "";
    const parts: string[] = [];
    const hookCount = readCompiledHookCount(strategy);
    const indicatorCount = readCompiledIndicatorCount(strategy);
    if (hookCount !== null) parts.push(`${hookCount} 个 hook`);
    if (indicatorCount !== null) parts.push(`${indicatorCount} 项依赖`);
    return parts.length === 0
      ? "已完成 Pine v6 主路径编译规划。"
      : `已完成 Pine v6 主路径编译规划，包含 ${parts.join(" / ")}。`;
  });
  const canRefreshSelectedStrategyDefinition = computed(() => {
    const sync = options.selectedStrategyDefinitionSync.value;
    return options.selectedStrategy.value !== null
      && sync !== null
      && !sync.isLatest
      && sync.canApplyLatest
      && !options.isLoadingDetails.value
      && !options.isRefreshingStrategyDefinition.value;
  });
  const selectedStrategyDefinitionRefreshHint = computed(() => {
    const sync = options.selectedStrategyDefinitionSync.value;
    if (sync === null) return "";
    if (sync.isLatest) return "当前实例已采用最新保存版本。";
    if (sync.canApplyLatest) {
      return `当前实例版本为 v${sync.appliedVersion}，可刷新到最新设计 v${sync.latestVersion}。`;
    }
    return sync.blockedReason ?? "当前实例需要先停止后再刷新。";
  });
  const canStartSelectedStrategy = computed(() => {
    const strategy = options.selectedStrategy.value;
    return strategy !== null
      && !options.isLoadingDetails.value
      && strategy.startable
      && strategy.status !== "RUNNING";
  });
  const canPauseSelectedStrategy = computed(() => {
    const strategy = options.selectedStrategy.value;
    return strategy !== null
      && !options.isLoadingDetails.value
      && strategy.startable
      && strategy.status === "RUNNING";
  });
  const canStopSelectedStrategy = computed(() => {
    const strategy = options.selectedStrategy.value;
    return strategy !== null
      && !options.isLoadingDetails.value
      && strategy.startable
      && strategy.status !== "STOPPED";
  });
  const canCreateStrategyInstance = computed(
    () => !options.isLoadingDefinitions.value
      && !options.isCreatingStrategyInstance.value
      && options.createDefinitionId.value.trim() !== ""
      && options.createInterval.value.trim() !== "",
  );
  const canUpdateSelectedStrategyBinding = computed(() =>
    options.selectedStrategy.value?.status === "STOPPED"
      && !options.isLoadingDetails.value
      && !options.isUpdatingStrategyBinding.value,
  );
  const canDeleteSelectedStrategy = computed(() =>
    options.selectedStrategy.value?.status === "STOPPED"
      && !options.isLoadingDetails.value
      && !options.isDeletingStrategy.value,
  );

  return {
    selectedStrategyParamsJson,
    selectedStrategyRuntimeLabel,
    selectedStrategySourceFormatLabel,
    selectedStrategyStartHint,
    selectedStrategyCompiledSummary,
    canRefreshSelectedStrategyDefinition,
    selectedStrategyDefinitionRefreshHint,
    canStartSelectedStrategy,
    canPauseSelectedStrategy,
    canStopSelectedStrategy,
    canCreateStrategyInstance,
    canUpdateSelectedStrategyBinding,
    canDeleteSelectedStrategy,
  };
}
