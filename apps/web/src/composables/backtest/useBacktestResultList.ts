import { computed, ref, watch, type Ref } from "vue";

import { isTerminalBacktestStatus } from "@/components/backtest/backtestRunPresentation";
import type { BacktestRun } from "./backtestRunModels";

export const BACKTEST_RESULTS_PAGE_SIZE = 5;

export function useBacktestResultList(input: {
  deleteRun: (runId: string) => Promise<unknown>;
  resolveStrategyName: (definitionId: string | undefined) => string;
  selectedRunId: Ref<string>;
  sortedRuns: Ref<BacktestRun[]>;
}) {
  const resultsPage = ref(1);
  const resultsSearchQuery = ref("");
  const resultsStatusFilter = ref("all");
  const resultsStrategyFilter = ref("all");
  const pendingDeleteRunId = ref("");
  const deletingRunId = ref("");

  const resultStrategyOptions = computed(() => {
    const options = [{ value: "all", title: "全部策略" }];
    const seenDefinitionIDs = new Set<string>();
    for (const run of input.sortedRuns.value) {
      const definitionID = run.request.definitionId.trim();
      if (definitionID === "" || seenDefinitionIDs.has(definitionID)) continue;
      seenDefinitionIDs.add(definitionID);
      options.push({
        value: definitionID,
        title: input.resolveStrategyName(definitionID),
      });
    }
    return options;
  });
  const hasResultsFilters = computed(
    () =>
      resultsSearchQuery.value.trim() !== "" ||
      resultsStatusFilter.value !== "all" ||
      resultsStrategyFilter.value !== "all",
  );
  const filteredRuns = computed(() => {
    const normalizedQuery = resultsSearchQuery.value.trim().toLowerCase();
    return input.sortedRuns.value.filter((run) => {
      if (
        resultsStatusFilter.value !== "all" &&
        run.status !== resultsStatusFilter.value
      ) {
        return false;
      }
      if (
        resultsStrategyFilter.value !== "all" &&
        run.request.definitionId !== resultsStrategyFilter.value
      ) {
        return false;
      }
      if (normalizedQuery === "") return true;
      return [
        run.id,
        run.request.symbol,
        run.request.market ?? "",
        run.request.code ?? "",
        run.request.interval,
        run.request.definitionId,
        run.request.definitionVersion ?? "",
        input.resolveStrategyName(run.request.definitionId),
        run.status,
      ]
        .join(" ")
        .toLowerCase()
        .includes(normalizedQuery);
    });
  });
  const emptyResultsMessage = computed(() =>
    input.sortedRuns.value.length === 0
      ? "暂无回测记录。请在左侧配置参数并启动回测。"
      : "没有匹配当前搜索或筛选条件的回测结果。",
  );
  const resultsPageCount = computed(() =>
    Math.max(
      1,
      Math.ceil(filteredRuns.value.length / BACKTEST_RESULTS_PAGE_SIZE),
    ),
  );
  const pagedRuns = computed(() => {
    const startIndex = (resultsPage.value - 1) * BACKTEST_RESULTS_PAGE_SIZE;
    return filteredRuns.value.slice(
      startIndex,
      startIndex + BACKTEST_RESULTS_PAGE_SIZE,
    );
  });
  const resultsPageSummary = computed(() => {
    if (filteredRuns.value.length === 0) return "";
    const startIndex = (resultsPage.value - 1) * BACKTEST_RESULTS_PAGE_SIZE;
    const visibleStart = startIndex + 1;
    const visibleEnd = Math.min(
      filteredRuns.value.length,
      startIndex + BACKTEST_RESULTS_PAGE_SIZE,
    );
    return hasResultsFilters.value
      ? `筛选后第 ${visibleStart}-${visibleEnd} 条，共 ${filteredRuns.value.length} 条；全部结果 ${input.sortedRuns.value.length} 条`
      : `第 ${visibleStart}-${visibleEnd} 条，共 ${filteredRuns.value.length} 条`;
  });
  const pendingDeleteRun = computed(() =>
    input.sortedRuns.value.find((run) => run.id === pendingDeleteRunId.value),
  );
  const pendingDeleteMessage = computed(() => {
    const run = pendingDeleteRun.value;
    if (run == null) return "";
    return `确认永久删除回测记录 ${run.id}（${input.resolveStrategyName(run.request.definitionId)} / ${run.request.symbol}）？此操作无法撤销。`;
  });

  function requestDeleteRun(runId: string): void {
    const run = input.sortedRuns.value.find((candidate) => candidate.id === runId);
    if (run == null || !isTerminalBacktestStatus(run.status)) return;
    pendingDeleteRunId.value = run.id;
  }

  async function confirmDeleteRun(): Promise<void> {
    const runId = pendingDeleteRunId.value;
    if (runId === "" || deletingRunId.value !== "") return;
    deletingRunId.value = runId;
    try {
      await input.deleteRun(runId);
    } finally {
      pendingDeleteRunId.value = "";
      deletingRunId.value = "";
    }
  }

  function resetResultsFilters(): void {
    resultsSearchQuery.value = "";
    resultsStatusFilter.value = "all";
    resultsStrategyFilter.value = "all";
    resultsPage.value = 1;
  }

  watch(
    () => [filteredRuns.value.length, resultsPageCount.value] as const,
    () => {
      resultsPage.value = Math.max(
        1,
        Math.min(resultsPage.value, resultsPageCount.value),
      );
    },
    { immediate: true },
  );
  watch(
    [resultsSearchQuery, resultsStatusFilter, resultsStrategyFilter],
    () => {
      resultsPage.value = 1;
    },
  );
  watch(
    filteredRuns,
    (nextRuns) => {
      if (nextRuns.length === 0) {
        input.selectedRunId.value = "";
      } else if (!nextRuns.some((run) => run.id === input.selectedRunId.value)) {
        input.selectedRunId.value = nextRuns[0]?.id ?? "";
      }
    },
    { immediate: true },
  );

  return {
    confirmDeleteRun,
    deletingRunId,
    emptyResultsMessage,
    filteredRuns,
    hasResultsFilters,
    pagedRuns,
    pendingDeleteMessage,
    pendingDeleteRun,
    pendingDeleteRunId,
    requestDeleteRun,
    resetResultsFilters,
    resultStrategyOptions,
    resultsPage,
    resultsPageCount,
    resultsPageSummary,
    resultsSearchQuery,
    resultsStatusFilter,
    resultsStrategyFilter,
  };
}
