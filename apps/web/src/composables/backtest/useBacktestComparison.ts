import { computed, ref, watch, type Ref } from "vue";
import type { RouteLocationNormalizedLoaded, Router } from "vue-router";

import {
  formatBacktestTimestamp,
} from "@/components/backtest/backtestRunPresentation";
import { queryClient } from "@/composables/settings/serverState";
import {
  fetchStrategyDefinitionVersion,
  fetchStrategyDefinitionVersions,
  strategyDefinitionVersionQueryKey,
  strategyDefinitionVersionsQueryKey,
  type StrategyDefinitionVersionDocument,
  type StrategyDefinitionVersionSummary,
} from "@/composables/strategy/strategyDefinitionVersions";
import type { BacktestRun } from "./backtestRunModels";
import {
  buildComparisonConfigRows,
  buildComparisonMetrics,
  compareConfigValue,
  comparisonChartType,
  comparisonFeeConfig,
  formatComparisonCurrency,
  formatComparisonMetric,
  type ComparisonConfigRow,
  type ComparisonMetric,
} from "./backtestComparisonModels";
import type {
  BacktestMobileSection,
  BacktestReportMode,
} from "./useBacktestPageLayout";

export interface BacktestStrategyDefinition {
  id: string;
  name: string;
  version: string;
  symbol?: string;
  derivedWarmupBars?: number;
  derivedWarmupInterval?: string;
}

type ComparisonSide = "left" | "right";

interface BacktestComparisonInput {
  backtestMobileSection: Ref<BacktestMobileSection>;
  definitions: Ref<BacktestStrategyDefinition[]>;
  getFocusedRun: () => BacktestRun | undefined;
  resolveRunQuoteCurrency: (run: BacktestRun) => string;
  resolveRunSessionMode: (run: BacktestRun) => string;
  route: RouteLocationNormalizedLoaded;
  router: Router;
  runs: Ref<BacktestRun[]>;
  selectedDefinitionId: Ref<string>;
  toggleRun: (runId: string) => Promise<unknown>;
}

export function firstQueryValue(value: unknown): string {
  if (Array.isArray(value)) {
    return typeof value[0] === "string" ? value[0].trim() : "";
  }
  return typeof value === "string" ? value.trim() : "";
}

export function reportModeFromQuery(value: unknown): BacktestReportMode {
  return firstQueryValue(value) === "compare" ? "compare" : "single";
}

export function formatStrategyVersion(version: string | undefined): string {
  const normalized = (version ?? "").trim();
  if (normalized === "") return "版本未知";
  return `v${normalized}`;
}

export function useBacktestComparison(input: BacktestComparisonInput) {
  const reportMode = ref<BacktestReportMode>(
    reportModeFromQuery(input.route.query.mode),
  );
  const comparisonDefinitionId = ref(
    firstQueryValue(input.route.query.definitionId),
  );
  const leftComparisonVersion = ref(
    firstQueryValue(input.route.query.leftVersion),
  );
  const rightComparisonVersion = ref(
    firstQueryValue(input.route.query.rightVersion),
  );
  const leftComparisonRunId = ref(
    firstQueryValue(input.route.query.leftRunId),
  );
  const rightComparisonRunId = ref(
    firstQueryValue(input.route.query.rightRunId),
  );
  const comparisonVersions = ref<StrategyDefinitionVersionSummary[]>([]);
  const isLoadingComparisonVersions = ref(false);
  const comparisonVersionsError = ref("");
  const leftComparisonSnapshot = ref<StrategyDefinitionVersionDocument | null>(
    null,
  );
  const rightComparisonSnapshot = ref<StrategyDefinitionVersionDocument | null>(
    null,
  );
  const comparisonSnapshotErrors = ref({ left: "", right: "" });
  const comparisonSnapshotLoading = ref({ left: false, right: false });
  let comparisonVersionsRequestId = 0;
  let leftComparisonSnapshotRequestId = 0;
  let rightComparisonSnapshotRequestId = 0;
  let applyingComparisonRoute = false;

  const comparisonDefinitionOptions = computed(() => {
    const items = input.definitions.value.map((definition) => ({
      value: definition.id,
      title: `${definition.name || definition.id} / ${formatStrategyVersion(definition.version)}`,
    }));
    if (
      comparisonDefinitionId.value !== "" &&
      !items.some((item) => item.value === comparisonDefinitionId.value)
    ) {
      items.unshift({
        value: comparisonDefinitionId.value,
        title: comparisonDefinitionId.value,
      });
    }
    return items;
  });
  const leftComparisonVersionOptions = computed(() =>
    comparisonVersions.value.filter(
      (version) => version.version !== rightComparisonVersion.value,
    ),
  );
  const rightComparisonVersionOptions = computed(() =>
    comparisonVersions.value.filter(
      (version) => version.version !== leftComparisonVersion.value,
    ),
  );
  const leftComparisonVersionSelectOptions = computed(() =>
    leftComparisonVersionOptions.value.map((version) => ({
      value: version.version,
      title: versionOptionTitle(version),
    })),
  );
  const rightComparisonVersionSelectOptions = computed(() =>
    rightComparisonVersionOptions.value.map((version) => ({
      value: version.version,
      title: versionOptionTitle(version),
    })),
  );

  function comparisonRunTimestamp(run: BacktestRun): number {
    const updated = Date.parse(run.updatedAt);
    if (Number.isFinite(updated)) return updated;
    const created = Date.parse(run.createdAt);
    return Number.isFinite(created) ? created : 0;
  }

  function completedRunsForComparisonVersion(version: string): BacktestRun[] {
    const normalizedVersion = version.trim();
    const definitionId = comparisonDefinitionId.value.trim();
    if (definitionId === "" || normalizedVersion === "") return [];
    return input.runs.value
      .filter(
        (run) =>
          run.status === "completed" &&
          run.request.definitionId === definitionId &&
          (run.request.definitionVersion ?? "").trim() === normalizedVersion,
      )
      .sort(
        (left, right) =>
          comparisonRunTimestamp(right) - comparisonRunTimestamp(left),
      );
  }

  const leftComparisonRuns = computed(() =>
    completedRunsForComparisonVersion(leftComparisonVersion.value),
  );
  const rightComparisonRuns = computed(() =>
    completedRunsForComparisonVersion(rightComparisonVersion.value),
  );
  const leftComparisonRunOptions = computed(() =>
    leftComparisonRuns.value.map((run) => ({
      value: run.id,
      title: comparisonRunOptionTitle(run),
    })),
  );
  const rightComparisonRunOptions = computed(() =>
    rightComparisonRuns.value.map((run) => ({
      value: run.id,
      title: comparisonRunOptionTitle(run),
    })),
  );
  const leftComparisonRun = computed(() =>
    leftComparisonRuns.value.find((run) => run.id === leftComparisonRunId.value),
  );
  const rightComparisonRun = computed(() =>
    rightComparisonRuns.value.find(
      (run) => run.id === rightComparisonRunId.value,
    ),
  );
  const comparisonRunsReady = computed(
    () =>
      leftComparisonRun.value?.result != null &&
      rightComparisonRun.value?.result != null,
  );
  const comparisonSourcesReady = computed(
    () =>
      leftComparisonSnapshot.value != null &&
      rightComparisonSnapshot.value != null,
  );

  function versionOptionTitle(
    version: StrategyDefinitionVersionSummary,
  ): string {
    return `v${version.version}${version.isCurrent ? "（当前）" : ""}`;
  }

  function comparisonRunOptionTitle(run: BacktestRun): string {
    return `${run.id} · ${formatBacktestTimestamp(run.updatedAt)} · ${run.request.symbol}`;
  }

  function clearComparisonSnapshots(): void {
    leftComparisonSnapshotRequestId += 1;
    rightComparisonSnapshotRequestId += 1;
    leftComparisonSnapshot.value = null;
    rightComparisonSnapshot.value = null;
    comparisonSnapshotErrors.value = { left: "", right: "" };
    comparisonSnapshotLoading.value = { left: false, right: false };
  }

  function clearComparisonSelection(): void {
    comparisonVersionsRequestId += 1;
    comparisonVersions.value = [];
    comparisonVersionsError.value = "";
    isLoadingComparisonVersions.value = false;
    leftComparisonVersion.value = "";
    rightComparisonVersion.value = "";
    leftComparisonRunId.value = "";
    rightComparisonRunId.value = "";
    clearComparisonSnapshots();
  }

  function comparisonVersionExists(version: string): boolean {
    return comparisonVersions.value.some(
      (candidate) => candidate.version === version,
    );
  }

  function applyComparisonVersionDefaults(): void {
    const latest = comparisonVersions.value[0]?.version ?? "";
    const previous = comparisonVersions.value[1]?.version ?? "";
    let left = comparisonVersionExists(leftComparisonVersion.value)
      ? leftComparisonVersion.value
      : "";
    let right = comparisonVersionExists(rightComparisonVersion.value)
      ? rightComparisonVersion.value
      : "";
    if (left === "" && right === "" && previous !== "" && latest !== "") {
      left = previous;
      right = latest;
    } else if (left === "") {
      left =
        comparisonVersions.value.find((version) => version.version !== right)
          ?.version ?? "";
    } else if (right === "") {
      right =
        comparisonVersions.value.find((version) => version.version !== left)
          ?.version ?? "";
    }
    if (left === right) {
      right =
        comparisonVersions.value.find((version) => version.version !== left)
          ?.version ?? "";
    }
    leftComparisonVersion.value = left;
    rightComparisonVersion.value = right;
    ensureComparisonRunDefaults();
    void loadComparisonSnapshot("left", left);
    void loadComparisonSnapshot("right", right);
  }

  async function loadComparisonVersions(
    definitionId = comparisonDefinitionId.value,
  ): Promise<void> {
    const normalizedDefinitionId = definitionId.trim();
    const requestId = ++comparisonVersionsRequestId;
    if (normalizedDefinitionId === "") {
      clearComparisonSelection();
      return;
    }
    isLoadingComparisonVersions.value = true;
    comparisonVersionsError.value = "";
    clearComparisonSnapshots();
    try {
      const versions = await queryClient.fetchQuery({
        queryKey: strategyDefinitionVersionsQueryKey(normalizedDefinitionId),
        queryFn: () => fetchStrategyDefinitionVersions(normalizedDefinitionId),
        staleTime: 0,
      });
      if (
        requestId !== comparisonVersionsRequestId ||
        normalizedDefinitionId !== comparisonDefinitionId.value
      ) {
        return;
      }
      comparisonVersions.value = versions;
      applyComparisonVersionDefaults();
    } catch (cause) {
      if (
        requestId !== comparisonVersionsRequestId ||
        normalizedDefinitionId !== comparisonDefinitionId.value
      ) {
        return;
      }
      comparisonVersions.value = [];
      comparisonVersionsError.value =
        cause instanceof Error ? cause.message : String(cause);
      leftComparisonVersion.value = "";
      rightComparisonVersion.value = "";
      leftComparisonRunId.value = "";
      rightComparisonRunId.value = "";
    } finally {
      if (requestId === comparisonVersionsRequestId) {
        isLoadingComparisonVersions.value = false;
      }
    }
  }

  async function loadComparisonSnapshot(
    side: ComparisonSide,
    version: string,
  ): Promise<void> {
    const definitionId = comparisonDefinitionId.value.trim();
    const normalizedVersion = version.trim();
    const requestId =
      side === "left"
        ? ++leftComparisonSnapshotRequestId
        : ++rightComparisonSnapshotRequestId;
    const setSnapshot = (
      snapshot: StrategyDefinitionVersionDocument | null,
    ): void => {
      if (side === "left") leftComparisonSnapshot.value = snapshot;
      else rightComparisonSnapshot.value = snapshot;
    };
    const currentRequestId = (): number =>
      side === "left"
        ? leftComparisonSnapshotRequestId
        : rightComparisonSnapshotRequestId;
    if (definitionId === "" || normalizedVersion === "") {
      setSnapshot(null);
      comparisonSnapshotErrors.value = {
        ...comparisonSnapshotErrors.value,
        [side]: "",
      };
      comparisonSnapshotLoading.value = {
        ...comparisonSnapshotLoading.value,
        [side]: false,
      };
      return;
    }
    comparisonSnapshotLoading.value = {
      ...comparisonSnapshotLoading.value,
      [side]: true,
    };
    comparisonSnapshotErrors.value = {
      ...comparisonSnapshotErrors.value,
      [side]: "",
    };
    try {
      const snapshot = await queryClient.ensureQueryData({
        queryKey: strategyDefinitionVersionQueryKey(
          definitionId,
          normalizedVersion,
        ),
        queryFn: () =>
          fetchStrategyDefinitionVersion(definitionId, normalizedVersion),
      });
      const selectedVersion =
        side === "left"
          ? leftComparisonVersion.value
          : rightComparisonVersion.value;
      if (
        requestId !== currentRequestId() ||
        definitionId !== comparisonDefinitionId.value ||
        normalizedVersion !== selectedVersion
      ) {
        return;
      }
      setSnapshot(snapshot);
    } catch (cause) {
      const selectedVersion =
        side === "left"
          ? leftComparisonVersion.value
          : rightComparisonVersion.value;
      if (
        requestId !== currentRequestId() ||
        definitionId !== comparisonDefinitionId.value ||
        normalizedVersion !== selectedVersion
      ) {
        return;
      }
      setSnapshot(null);
      comparisonSnapshotErrors.value = {
        ...comparisonSnapshotErrors.value,
        [side]: cause instanceof Error ? cause.message : String(cause),
      };
    } finally {
      if (requestId === currentRequestId()) {
        comparisonSnapshotLoading.value = {
          ...comparisonSnapshotLoading.value,
          [side]: false,
        };
      }
    }
  }

  function nativeSelectValue(event: Event): string {
    return event.target instanceof HTMLSelectElement ? event.target.value : "";
  }

  function changeComparisonDefinition(value: unknown): void {
    const nextDefinitionId = typeof value === "string" ? value.trim() : "";
    if (nextDefinitionId === comparisonDefinitionId.value) return;
    comparisonDefinitionId.value = nextDefinitionId;
    clearComparisonSelection();
    void loadComparisonVersions(nextDefinitionId);
  }

  function changeComparisonVersion(side: ComparisonSide, value: unknown): void {
    const nextVersion = typeof value === "string" ? value.trim() : "";
    const otherVersion =
      side === "left"
        ? rightComparisonVersion.value
        : leftComparisonVersion.value;
    if (nextVersion === otherVersion) return;
    if (side === "left") {
      leftComparisonVersion.value = nextVersion;
      leftComparisonRunId.value = "";
    } else {
      rightComparisonVersion.value = nextVersion;
      rightComparisonRunId.value = "";
    }
    void loadComparisonSnapshot(side, nextVersion);
  }

  function changeComparisonRun(side: ComparisonSide, value: unknown): void {
    const runId = typeof value === "string" ? value : "";
    if (side === "left") leftComparisonRunId.value = runId;
    else rightComparisonRunId.value = runId;
  }

  function activateComparisonMode(): void {
    reportMode.value = "compare";
    const definitionId =
      comparisonDefinitionId.value ||
      input.selectedDefinitionId.value ||
      input.definitions.value[0]?.id ||
      "";
    if (definitionId !== comparisonDefinitionId.value) {
      comparisonDefinitionId.value = definitionId;
      clearComparisonSelection();
    }
    if (definitionId !== "") void loadComparisonVersions(definitionId);
    input.backtestMobileSection.value = "report";
  }

  function activateSingleReportMode(): void {
    reportMode.value = "single";
    if (input.getFocusedRun() != null) {
      input.backtestMobileSection.value = "report";
    }
  }

  function comparisonQueryMatchesRoute(): boolean {
    return (
      reportMode.value === reportModeFromQuery(input.route.query.mode) &&
      comparisonDefinitionId.value ===
        firstQueryValue(input.route.query.definitionId) &&
      leftComparisonVersion.value ===
        firstQueryValue(input.route.query.leftVersion) &&
      rightComparisonVersion.value ===
        firstQueryValue(input.route.query.rightVersion) &&
      leftComparisonRunId.value ===
        firstQueryValue(input.route.query.leftRunId) &&
      rightComparisonRunId.value ===
        firstQueryValue(input.route.query.rightRunId)
    );
  }

  function syncComparisonRoute(): void {
    if (applyingComparisonRoute || comparisonQueryMatchesRoute()) return;
    const query = { ...input.route.query } as Record<
      string,
      string | string[] | undefined
    >;
    for (const key of [
      "mode",
      "definitionId",
      "leftVersion",
      "rightVersion",
      "leftRunId",
      "rightRunId",
    ]) {
      delete query[key];
    }
    if (reportMode.value === "compare") {
      query.mode = "compare";
      if (comparisonDefinitionId.value) {
        query.definitionId = comparisonDefinitionId.value;
      }
      if (leftComparisonVersion.value) {
        query.leftVersion = leftComparisonVersion.value;
      }
      if (rightComparisonVersion.value) {
        query.rightVersion = rightComparisonVersion.value;
      }
      if (leftComparisonRunId.value) query.leftRunId = leftComparisonRunId.value;
      if (rightComparisonRunId.value) {
        query.rightRunId = rightComparisonRunId.value;
      }
    }
    void input.router.replace({ path: input.route.path, query });
  }

  function comparisonMetricDelta(metric: ComparisonMetric): string {
    if (
      metric.left == null ||
      metric.right == null ||
      !Number.isFinite(metric.left) ||
      !Number.isFinite(metric.right)
    ) {
      return "--";
    }
    const leftCurrency = leftComparisonRun.value
      ? input.resolveRunQuoteCurrency(leftComparisonRun.value)
      : "";
    const rightCurrency = rightComparisonRun.value
      ? input.resolveRunQuoteCurrency(rightComparisonRun.value)
      : "";
    if (metric.kind === "currency" && leftCurrency !== rightCurrency) {
      return "币种不同";
    }
    const delta = metric.right - metric.left;
    const prefix = delta > 0 ? "+" : "";
    return `${prefix}${formatComparisonMetric(delta, metric.kind, rightCurrency)}`;
  }

  const comparisonMetrics = computed<ComparisonMetric[]>(() =>
    buildComparisonMetrics(leftComparisonRun.value, rightComparisonRun.value),
  );
  const comparisonConfigRows = computed<ComparisonConfigRow[]>(() =>
    buildComparisonConfigRows({
      left: leftComparisonRun.value,
      right: rightComparisonRun.value,
      resolveQuoteCurrency: input.resolveRunQuoteCurrency,
      resolveSessionMode: input.resolveRunSessionMode,
    }),
  );
  const comparisonConditionsMatch = computed(
    () =>
      comparisonConfigRows.value.length > 0 &&
      comparisonConfigRows.value.every((row) => row.same),
  );

  function ensureComparisonRunDefaults(): void {
    if (
      !leftComparisonRuns.value.some(
        (run) => run.id === leftComparisonRunId.value,
      )
    ) {
      leftComparisonRunId.value = leftComparisonRuns.value[0]?.id ?? "";
    }
    if (
      !rightComparisonRuns.value.some(
        (run) => run.id === rightComparisonRunId.value,
      )
    ) {
      rightComparisonRunId.value = rightComparisonRuns.value[0]?.id ?? "";
    }
  }

  function applyComparisonRouteState(): void {
    const nextMode = reportModeFromQuery(input.route.query.mode);
    const nextDefinitionId = firstQueryValue(input.route.query.definitionId);
    const definitionChanged =
      nextDefinitionId !== comparisonDefinitionId.value;
    applyingComparisonRoute = true;
    reportMode.value = nextMode;
    comparisonDefinitionId.value = nextDefinitionId;
    leftComparisonVersion.value = firstQueryValue(
      input.route.query.leftVersion,
    );
    rightComparisonVersion.value = firstQueryValue(
      input.route.query.rightVersion,
    );
    leftComparisonRunId.value = firstQueryValue(input.route.query.leftRunId);
    rightComparisonRunId.value = firstQueryValue(input.route.query.rightRunId);
    applyingComparisonRoute = false;
    if (
      nextMode === "compare" &&
      nextDefinitionId !== "" &&
      (definitionChanged || comparisonVersions.value.length === 0)
    ) {
      void loadComparisonVersions(nextDefinitionId);
    }
  }

  watch(
    () => [
      input.route.query.mode,
      input.route.query.definitionId,
      input.route.query.leftVersion,
      input.route.query.rightVersion,
      input.route.query.leftRunId,
      input.route.query.rightRunId,
    ],
    applyComparisonRouteState,
  );
  watch(
    [
      reportMode,
      comparisonDefinitionId,
      leftComparisonVersion,
      rightComparisonVersion,
      leftComparisonRunId,
      rightComparisonRunId,
    ],
    syncComparisonRoute,
  );
  watch(
    () => [
      leftComparisonVersion.value,
      rightComparisonVersion.value,
      input.runs.value,
    ],
    ensureComparisonRunDefaults,
    { deep: true },
  );
  watch(
    () => [leftComparisonRunId.value, rightComparisonRunId.value] as const,
    ([leftRunId, rightRunId]) => {
      if (leftRunId) void input.toggleRun(leftRunId);
      if (rightRunId) void input.toggleRun(rightRunId);
    },
    { immediate: true },
  );

  return {
    activateComparisonMode,
    activateSingleReportMode,
    applyComparisonRouteState,
    applyComparisonVersionDefaults,
    changeComparisonDefinition,
    changeComparisonRun,
    changeComparisonVersion,
    clearComparisonSelection,
    clearComparisonSnapshots,
    comparisonConditionsMatch,
    comparisonConfigRows,
    comparisonDefinitionId,
    comparisonDefinitionOptions,
    comparisonChartType,
    comparisonFeeConfig,
    comparisonMetricDelta,
    comparisonMetrics,
    comparisonQueryMatchesRoute,
    comparisonRunsReady,
    comparisonSnapshotErrors,
    comparisonSnapshotLoading,
    comparisonSourcesReady,
    comparisonVersionExists,
    comparisonVersions,
    comparisonVersionsError,
    completedRunsForComparisonVersion,
    compareConfigValue,
    ensureComparisonRunDefaults,
    formatComparisonCurrency,
    formatComparisonMetric,
    isLoadingComparisonVersions,
    leftComparisonRun,
    leftComparisonRunId,
    leftComparisonRunOptions,
    leftComparisonRuns,
    leftComparisonSnapshot,
    leftComparisonVersion,
    leftComparisonVersionOptions,
    leftComparisonVersionSelectOptions,
    loadComparisonSnapshot,
    loadComparisonVersions,
    nativeSelectValue,
    reportMode,
    rightComparisonRun,
    rightComparisonRunId,
    rightComparisonRunOptions,
    rightComparisonRuns,
    rightComparisonSnapshot,
    rightComparisonVersion,
    rightComparisonVersionOptions,
    rightComparisonVersionSelectOptions,
    syncComparisonRoute,
    versionOptionTitle,
    comparisonRunOptionTitle,
    comparisonRunTimestamp,
  };
}
