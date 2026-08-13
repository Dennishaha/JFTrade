import { useQuery } from "@tanstack/vue-query";
import { computed, onScopeDispose, reactive, ref, type ComputedRef } from "vue";

import type {
  BacktestCandleDto,
  BacktestFeeRuleDto,
  BacktestFeeScheduleDto,
  BacktestOrderBookEntryDto,
  BacktestRunResultDto,
  BacktestRunStateDto,
  BacktestStartRequestDto,
  BacktestTradeEventDto,
  BacktestTradingCostsDto,
  RunModelTradingCostsDto,
} from "@/contracts";
import type {
  BacktestFeeRulePayload,
  BacktestFeeSchedulePayload,
  BacktestStartRequestPayload,
  BacktestSyncRequestPayload,
  BacktestTradingCostsPayload,
} from "@/types";

import {
  normalizeChartType,
  type ChartType,
  type HeikinAshiSeed,
} from "@/charting/kline";
import type { BacktestTrade, BacktestPnlPoint, BacktestDrawdownPoint, BacktestCandle } from "@/components/backtest/BacktestChart.vue";
import { apiDeletePath, apiGet, apiGetPath, apiPost } from "@/composables/shared/apiClient";
import { queryClient, queryKeys } from "@/composables/settings/serverState";
import { useKlineSyncTask } from "@/composables/market-data/useKlineSyncTask";
import { normalizeRun } from "@/composables/backtest/backtestRunNormalization";
import type { BacktestRun } from "@/composables/backtest/backtestRunModels";

export type {
  BacktestCandleView,
  BacktestFeeBreakdownEntry,
  BacktestOrderBookEntry,
  BacktestRun,
  BacktestRunResult,
  BacktestTradeView,
} from "@/composables/backtest/backtestRunModels";

type BacktestStartRequestWire = BacktestStartRequestDto;
type BacktestFeeScheduleWire = BacktestFeeScheduleDto;
type BacktestFeeMode = NonNullable<BacktestFeeSchedulePayload["mode"]>;

export interface BacktestFormState {
  definitionId: string;
  definitionVersion: string;
  market: string;
  code: string;
  instrumentId: string;
  instrumentType: string;
  interval: string;
  chartType: ChartType;
  startDate: string;
  endDate: string;
  initialBalance: number;
  rehabType: string;
  useExtendedHours: boolean;
  brokerFeeMode: "market_preset" | "custom" | "script" | "none";
  marketFeeMode: "market_preset" | "custom" | "none";
  brokerFeeRules: BacktestFeeRulePayload[];
  marketFeeRules: BacktestFeeRulePayload[];
}

interface UseBacktestRunsOptions {
  formState: ComputedRef<BacktestFormState>;
  validateRange?: () => string;
  normalizeInstrument: (
    input: Pick<BacktestFormState, "market" | "code" | "instrumentId">,
  ) => Promise<{ market: string; prefix: string; code: string; instrumentId: string }>;
}

async function resolveBacktestInstrumentPayload(
  formState: Pick<BacktestFormState, "market" | "code" | "instrumentId">,
  resolver: UseBacktestRunsOptions["normalizeInstrument"],
): Promise<{ market: string; code: string; symbol: string } | null> {
  const normalized = await resolver(formState);
  const market = normalized.prefix.trim().toUpperCase();
  const code = normalized.code.trim().toUpperCase();
  const symbol = normalized.instrumentId.trim().toUpperCase();
  if (market === "" || code === "" || symbol === "") {
    return null;
  }
  return { market, code, symbol };
}

function resolveSyncSessionScope(formState: Pick<BacktestFormState, "useExtendedHours">): "regular" | "extended" {
  return formState.useExtendedHours ? "extended" : "regular";
}

export function buildBacktestStartRequestPayload(
  formState: BacktestFormState,
  instrument: { market: string; code: string; symbol: string },
): BacktestStartRequestPayload {
  return {
    definitionId: formState.definitionId,
    definitionVersion: formState.definitionVersion,
    market: instrument.market,
    code: instrument.code,
    symbol: instrument.symbol,
    instrumentType: normalizeBacktestInstrumentType(formState.instrumentType),
    interval: formState.interval,
    chartType: normalizeChartType(formState.chartType),
    startDate: formState.startDate,
    endDate: formState.endDate,
    initialBalance: formState.initialBalance,
    rehabType: formState.rehabType,
    useExtendedHours: formState.useExtendedHours,
    tradingCosts: buildBacktestTradingCostsPayload(formState, instrument.market),
    executionModel: "conservative-bar-v1",
  };
}

function normalizeBacktestInstrumentType(value: string): "stock" | "etf" {
  return value.trim().toLowerCase() === "etf" ? "etf" : "stock";
}

function presetIdForMarket(market: string, group: "broker" | "market"): string {
  const normalized = market.trim().toUpperCase();
  if (group === "broker") {
    if (normalized === "HK") return "futu_hk_hk_stock_2026_06_30";
    if (normalized === "US") return "futu_hk_us_stock_2026_06_30";
    return "";
  }
  if (normalized === "HK") return "hkex_hk_stock_2026_06_30";
  if (normalized === "US") return "us_stock_market_fees_2026_06_30";
  if (normalized === "CN" || normalized === "SH" || normalized === "SZ") {
    return "stock_connect_a_share_market_fees_2026_06_30";
  }
  return "";
}

function buildBacktestTradingCostsPayload(
  formState: BacktestFormState,
  market: string,
): BacktestTradingCostsPayload {
  return {
    brokerFees: buildFeeSchedulePayload(formState.brokerFeeMode, presetIdForMarket(market, "broker"), formState.brokerFeeRules),
    marketFees: buildFeeSchedulePayload(formState.marketFeeMode, presetIdForMarket(market, "market"), formState.marketFeeRules),
  };
}

function buildFeeSchedulePayload(
  mode: BacktestFeeMode,
  presetId: string,
  rules: BacktestFeeRulePayload[],
): BacktestFeeSchedulePayload {
  const schedule: BacktestFeeSchedulePayload = { mode };
  if (mode === "market_preset" && presetId !== "") {
    schedule.presetId = presetId;
  }
  if (mode === "custom") {
    schedule.rules = rules;
  }
  return schedule;
}

export function buildBacktestSyncRequestPayload(
  formState: BacktestFormState,
  instrument: { market: string; code: string; symbol: string },
): BacktestSyncRequestPayload {
  return {
    market: instrument.market,
    code: instrument.code,
    symbol: instrument.symbol,
    intervals: [formState.interval],
    startDate: formState.startDate,
    endDate: formState.endDate,
    rehabType: formState.rehabType,
    sessionScope: resolveSyncSessionScope(formState),
  };
}

function mapBacktestFeeScheduleRequest(
  value: BacktestFeeSchedulePayload | undefined,
): BacktestFeeScheduleWire {
  return {
    ...(value?.mode == null ? {} : { mode: value.mode }),
    ...(value?.presetId == null ? {} : { presetId: value.presetId }),
    ...(value?.rules == null
      ? {}
      : {
          rules: value.rules.map((rule) => ({
            ...rule,
            label: rule.label ?? "",
          })),
        }),
  };
}

export function toBacktestStartRequestWire(
  value: BacktestStartRequestPayload,
): BacktestStartRequestWire {
  return {
    definitionId: value.definitionId,
    market: value.market ?? "",
    code: value.code ?? "",
    symbol: value.symbol ?? "",
    interval: value.interval,
    chartType: value.chartType ?? "standard",
    initialBalance: value.initialBalance,
    rehabType: value.rehabType ?? "",
    tradingCosts: {
      brokerFees: mapBacktestFeeScheduleRequest(value.tradingCosts?.brokerFees),
      marketFees: mapBacktestFeeScheduleRequest(value.tradingCosts?.marketFees),
    },
    ...(value.definitionVersion == null
      ? {}
      : { definitionVersion: value.definitionVersion }),
    ...(value.instrumentType == null ? {} : { instrumentType: value.instrumentType }),
    ...(value.startDate === "" ? {} : { startDate: value.startDate }),
    ...(value.endDate === "" ? {} : { endDate: value.endDate }),
    ...(value.startTime == null ? {} : { startTime: value.startTime }),
    ...(value.endTime == null ? {} : { endTime: value.endTime }),
    ...(value.useExtendedHours == null
      ? {}
      : { useExtendedHours: value.useExtendedHours }),
    ...(value.executionModel == null ? {} : { executionModel: value.executionModel }),
  };
}

export function useBacktestRuns(options: UseBacktestRunsOptions) {
  const running = ref(false);
  const polling = ref<ReturnType<typeof setTimeout> | null>(null);
  const error = ref("");
  let pollingGeneration = 0;
  let disposed = false;
  const {
    syncing,
    syncProgress,
    syncError,
    startSync,
    cancelSync: cancelKlineSync,
  } = useKlineSyncTask();

  const expandedRuns = reactive<Record<string, boolean>>({});
  const detailLoading = reactive<Record<string, boolean>>({});
  const detailErrors = reactive<Record<string, string>>({});
  const backtestRunsQueryKey = queryKeys.backtestRuns();
  const runsQuery = useQuery({
    queryKey: backtestRunsQueryKey,
    queryFn: fetchBacktestRuns,
    enabled: false,
  }, queryClient);

  const runs = computed(() => runsQuery.data.value ?? []);

  onScopeDispose(() => {
    disposed = true;
    stopPolling();
  }, true);

  const filteredRuns = computed(() =>
    [...runs.value].sort(
      (a, b) =>
        new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
    ),
  );

  async function toggleRun(runId: string) {
    expandedRuns[runId] = true;
    await loadRunDetail(runId);
  }

  async function loadRunDetail(runId: string): Promise<void> {
    const current = runs.value.find((run) => run.id === runId);
    if (current?.result) {
      return;
    }
    if (detailLoading[runId]) {
      return;
    }
    detailLoading[runId] = true;
    detailErrors[runId] = "";
    try {
      const detail = await apiGetPath(
        "/api/v1/backtests/{runId}",
        `/api/v1/backtests/${encodeURIComponent(runId)}`,
      );
      const normalized = normalizeRun(detail);
      queryClient.setQueryData(queryKeys.backtestRun(runId), normalized);
      patchBacktestRuns([normalized]);
    } catch (cause) {
      detailErrors[runId] = `加载回测详情失败: ${cause instanceof Error ? cause.message : String(cause)}`;
    } finally {
      detailLoading[runId] = false;
    }
  }

  function pickPreferredRun(existingRun: BacktestRun, candidateRun: BacktestRun): BacktestRun {
    const existingUpdatedAt = new Date(existingRun.updatedAt).getTime();
    const candidateUpdatedAt = new Date(candidateRun.updatedAt).getTime();

    if (Number.isFinite(candidateUpdatedAt) && Number.isFinite(existingUpdatedAt)) {
      if (candidateUpdatedAt > existingUpdatedAt) {
        return candidateRun;
      }
      if (candidateUpdatedAt < existingUpdatedAt) {
        return existingRun;
      }
    }

    if (candidateRun.result && !existingRun.result) {
      return candidateRun;
    }
    if (existingRun.result && !candidateRun.result) {
      return existingRun;
    }
    if (candidateRun.status === "completed" && existingRun.status !== "completed") {
      return candidateRun;
    }
    return candidateRun;
  }

  function mergeRunsById(nextRuns: BacktestRun[]): BacktestRun[] {
    const merged = new Map<string, BacktestRun>();

    for (const run of nextRuns) {
      const existing = merged.get(run.id);
      merged.set(run.id, existing ? pickPreferredRun(existing, run) : run);
    }

    return Array.from(merged.values());
  }

  function patchBacktestRuns(nextRuns: BacktestRun[]): void {
    queryClient.setQueryData<BacktestRun[]>(
      backtestRunsQueryKey,
      (current) => mergeRunsById([...(current ?? []), ...nextRuns]),
    );
  }

  function patchBacktestRunStatus(runId: string, status: string): void {
    queryClient.setQueryData<BacktestRun[]>(
      backtestRunsQueryKey,
      (current) =>
        (current ?? []).map((run) =>
          run.id === runId ? { ...run, status } : run,
        ),
    );
    queryClient.setQueryData<BacktestRun | undefined>(
      queryKeys.backtestRun(runId),
      (current) => current == null ? current : { ...current, status },
    );
  }

  async function fetchBacktestRuns(): Promise<BacktestRun[]> {
    const data = await apiGet("/api/v1/backtests");
    return (data.runs ?? []).map(normalizeRun);
  }

  async function refreshRuns(): Promise<void> {
    const data = await fetchBacktestRuns();
    patchBacktestRuns(data);
  }

  async function loadRuns() {
    try {
      const data = await queryClient.ensureQueryData({
        queryKey: backtestRunsQueryKey,
        queryFn: fetchBacktestRuns,
      });
      patchBacktestRuns(data);
    } catch {
      // backtests may not be available yet
    }
  }

  async function deleteRun(runId: string): Promise<boolean> {
    const normalizedRunID = runId.trim();
    if (normalizedRunID === "") {
      return false;
    }

    const targetRun = runs.value.find((run) => run.id === normalizedRunID);
    const isTerminal = targetRun != null && isTerminalBacktestStatus(targetRun.status);
    if (!isTerminal) {
      return false;
    }

    error.value = "";
    try {
      const result = await apiDeletePath(
        "/api/v1/backtests/{runId}",
        `/api/v1/backtests/${encodeURIComponent(normalizedRunID)}`,
      );
      if (!result.deleted) {
        throw new Error("服务端未确认删除");
      }
    } catch (cause) {
      error.value = `删除回测记录失败: ${cause instanceof Error ? cause.message : String(cause)}`;
      return false;
    }

    delete expandedRuns[normalizedRunID];
    queryClient.setQueryData<BacktestRun[]>(
      backtestRunsQueryKey,
      (current) => (current ?? []).filter((run) => run.id !== normalizedRunID),
    );
    return true;
  }

  async function syncKlines() {
    const formState = options.formState.value;
    error.value = "";
    const rangeError = options.validateRange?.() ?? "";
    if (rangeError !== "") {
      error.value = `同步启动失败: ${rangeError}`;
      return;
    }

    try {
      const instrument = await resolveBacktestInstrumentPayload(
        formState,
        options.normalizeInstrument,
      );
      if (instrument == null) {
        error.value = "同步启动失败: 请先输入有效的市场与代码";
        return;
      }
      const payload = buildBacktestSyncRequestPayload(formState, instrument);
      await startSync(payload);
      if (syncError.value !== "") {
        error.value = syncError.value;
      }
    } catch (cause) {
      error.value = `同步启动失败: ${cause instanceof Error ? cause.message : String(cause)}`;
    }
  }

  async function cancelSync() {
    await cancelKlineSync();
    if (syncProgress.value) {
      syncProgress.value.status = "cancelled";
    }
  }

  async function startBacktest() {
    const formState = options.formState.value;
    if (!formState.definitionId) return;

    running.value = true;
    error.value = "";
    const rangeError = options.validateRange?.() ?? "";
    if (rangeError !== "") {
      error.value = `启动回测失败: ${rangeError}`;
      running.value = false;
      return;
    }
    try {
      const instrument = await resolveBacktestInstrumentPayload(
        formState,
        options.normalizeInstrument,
      );
      if (instrument == null) {
        error.value = "启动回测失败: 请先输入有效的市场与代码";
        return;
      }
      const payload = buildBacktestStartRequestPayload(formState, instrument);
      const wire = toBacktestStartRequestWire(payload);
      const submitBacktest = () => apiPost("/api/v1/backtests", wire);
      let data: Awaited<ReturnType<typeof submitBacktest>>;
      try {
        data = await submitBacktest();
      } catch (cause) {
        if (!isMissingBacktestDataError(cause)) throw cause;
        const progress = await startSync(
          buildBacktestSyncRequestPayload(formState, instrument),
        );
        if (progress?.status !== "completed") {
          throw new Error(syncError.value || "历史 K 线同步未完成");
        }
        data = await submitBacktest();
      }
      startPolling(data.id);
      await queryClient.invalidateQueries({ queryKey: backtestRunsQueryKey });
      await refreshRuns();
    } catch (cause) {
      error.value = `启动回测失败: ${cause instanceof Error ? cause.message : String(cause)}`;
    } finally {
      running.value = false;
    }
  }

  function startPolling(runId: string) {
    stopPolling();
    if (disposed) {
      return;
    }

    const generation = pollingGeneration;
    let consecutiveFailures = 0;
    const poll = async () => {
      if (disposed || generation !== pollingGeneration) {
        return;
      }
      try {
        const data = await apiGetPath(
          "/api/v1/backtests/{runId}/status",
          `/api/v1/backtests/${runId}/status`,
        );
        if (disposed || generation !== pollingGeneration) {
          return;
        }
        consecutiveFailures = 0;
        patchBacktestRunStatus(runId, data.status);
        if (isTerminalBacktestStatus(data.status)) {
          stopPolling();
          await queryClient.invalidateQueries({ queryKey: backtestRunsQueryKey });
          if (disposed) {
            return;
          }
          await refreshRuns();
          if (disposed) {
            return;
          }
          await loadRunDetail(runId);
          return;
        }
      } catch (cause) {
        if (disposed || generation !== pollingGeneration) {
          return;
        }
        consecutiveFailures += 1;
        if (consecutiveFailures >= 3) {
          stopPolling();
          error.value = `回测状态轮询失败: ${cause instanceof Error ? cause.message : String(cause)}`;
          return;
        }
      }

      if (!disposed && generation === pollingGeneration) {
        polling.value = setTimeout(() => void poll(), 2000);
      }
    };
    polling.value = setTimeout(() => void poll(), 2000);
  }

  function stopPolling() {
    pollingGeneration += 1;
    if (polling.value !== null) {
      clearTimeout(polling.value);
      polling.value = null;
    }
  }

  return {
    runs,
    running,
    syncing,
    syncProgress,
    error,
    expandedRuns,
    detailLoading,
    detailErrors,
    filteredRuns,
    toggleRun,
    deleteRun,
    loadRuns,
    syncKlines,
    cancelSync,
    startBacktest,
  };
}

function isMissingBacktestDataError(cause: unknown): boolean {
  const candidate = cause as { status?: unknown; message?: unknown } | null;
  if (candidate?.status !== 400 || typeof candidate.message !== "string") {
    return false;
  }
  const message = candidate.message.toLowerCase();
  return (
    message.includes("k-line data is not ready") ||
    message.includes("missing k-line coverage")
  );
}

function isTerminalBacktestStatus(status: string | undefined): boolean {
  return status === "completed" || status === "failed" || status === "cancelled";
}
