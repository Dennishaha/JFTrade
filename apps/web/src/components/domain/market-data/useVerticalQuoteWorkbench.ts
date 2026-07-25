import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";

import { fetchEnvelope } from "../../../composables/apiClient";
import { withBrokerProvider } from "../../../composables/brokerProviderSelection";
import {
  normalizeMarketDataSnapshotQueryResult,
  type MarketDataSnapshotQueryResult,
} from "../../../composables/marketDataRealtime";
import { normalizeMarketSecurityDetailsQueryResult } from "../../../composables/marketSecurityNormalization";
import { resolveMarketSnapshotDisplay } from "../../../composables/marketSessionDisplay";
import { getWatchlistMembership } from "../../../composables/watchlistApi";
import type {
  MarketSecurityDetailsQueryResult,
  WatchlistMembership,
} from "../../../contracts";
import {
  normalizeResearchQuoteTarget,
  parseResearchInstrumentId,
  researchQuoteTargetFromEntry,
  type QuoteSeed,
  type ResearchQuoteTarget,
} from "../../research/researchQuote";
import { fetchResearchSnapshots } from "../../research/researchSnapshots";

interface FeatureResult {
  entries?: Record<string, unknown>[];
  total?: number;
}

interface MetricItem {
  label: string;
  value: string;
}

interface PlateMemberStats {
  raiseCount: number; fallCount: number; equalCount: number; sampleSize: number; total: number;
}

const PLATE_MEMBER_REQUEST_LIMIT = 200;
const PLATE_MEMBER_DISPLAY_LIMIT = 50;

export interface VerticalQuoteWorkbenchInput {
  target?: ResearchQuoteTarget | null;
  seed?: QuoteSeed | null;
  brokerId?: string;
  visible?: boolean;
}

export type ResearchQuoteRailInput = VerticalQuoteWorkbenchInput;

const numberFormatter = new Intl.NumberFormat("zh-CN", {
  maximumFractionDigits: 4,
});

export function useVerticalQuoteWorkbench(
  props: Readonly<VerticalQuoteWorkbenchInput>,
) {
  const snapshotResult = ref<MarketDataSnapshotQueryResult | null>(null);
  const securityResult = ref<MarketSecurityDetailsQueryResult | null>(null);
  const plateMembers = ref<ResearchQuoteTarget[]>([]);
  const plateMemberStats = ref<PlateMemberStats | null>(null);
  const snapshotLoading = ref(false);
  const securityLoading = ref(false);
  const plateMembersLoading = ref(false);
  const snapshotError = ref("");
  const securityError = ref("");
  const plateMembersError = ref("");
  const watchlistDialogOpen = ref(false);
  const favorite = ref(false);
  const documentVisible = ref(
    typeof document === "undefined" || document.visibilityState !== "hidden",
  );

  let requestToken = 0;
  let snapshotToken = 0;
  let mounted = false;
  let snapshotPollTimer: number | null = null;

  const resolvedTarget = computed(() =>
    normalizeResearchQuoteTarget(props.target),
  );
  const instrumentParts = computed(() =>
    parseResearchInstrumentId(resolvedTarget.value?.instrumentId),
  );
  const normalizedBrokerId = computed(() =>
    (props.brokerId ?? "").trim().toLowerCase(),
  );
  const security = computed(() => securityResult.value?.security ?? null);
  const snapshot = computed(() => snapshotResult.value?.snapshot ?? null);
  const targetKey = computed(() => {
    const target = resolvedTarget.value;
    return target == null
      ? ""
      : `${target.kind}:${target.instrumentId}:${target.productClass}`;
  });
  const extendedCards = computed(() => {
    if (instrumentParts.value?.market !== "US") return [];
    return resolveMarketSnapshotDisplay(snapshot.value, true).extendedCards;
  });

  function finiteNumber(value: unknown): number | null {
    if (typeof value === "number" && Number.isFinite(value)) return value;
    if (typeof value === "string" && value.trim() !== "") {
      const parsed = Number(value);
      if (Number.isFinite(parsed)) return parsed;
    }
    return null;
  }

  function firstNumber(...values: unknown[]): number | null {
    for (const value of values) {
      const normalized = finiteNumber(value);
      if (normalized != null) return normalized;
    }
    return null;
  }

  const name = computed(
    () =>
      security.value?.name ||
      resolvedTarget.value?.name ||
      props.seed?.name ||
      "",
  );
  const lastPrice = computed(() =>
    firstNumber(
      snapshot.value?.price,
      security.value?.currentPrice,
      props.seed?.lastPrice,
    ),
  );
  const previousClose = computed(() =>
    firstNumber(
      snapshot.value?.previousClosePrice,
      snapshot.value?.lastClosePrice,
      security.value?.lastClosePrice,
      props.seed?.previousClosePrice,
    ),
  );
  const changeAmount = computed(() => {
    if (lastPrice.value != null && previousClose.value != null) {
      return lastPrice.value - previousClose.value;
    }
    return finiteNumber(props.seed?.changeAmount);
  });
  const changeRate = computed(() => {
    if (
      lastPrice.value != null &&
      previousClose.value != null &&
      previousClose.value !== 0
    ) {
      return ((lastPrice.value - previousClose.value) / previousClose.value) * 100;
    }
    return finiteNumber(props.seed?.changeRate);
  });

  function directionClass(value: number | null): string {
    if (value == null || value === 0) return "";
    return value > 0 ? "tv-up" : "tv-down";
  }

  function formatPrice(value: number | null): string {
    return value == null ? "--" : numberFormatter.format(value);
  }

  function formatSigned(value: number | null, suffix = ""): string {
    if (value == null) return "--";
    const formatted = `${Math.abs(value).toFixed(2)}${suffix}`;
    if (value > 0) return `+${formatted}`;
    if (value < 0) return `-${formatted}`;
    return formatted;
  }

  function formatVolume(value: number | null): string {
    if (value == null) return "--";
    if (Math.abs(value) >= 1e8) return `${(value / 1e8).toFixed(2)}亿`;
    if (Math.abs(value) >= 1e4) return `${(value / 1e4).toFixed(2)}万`;
    return numberFormatter.format(value);
  }

  function formatRatio(value: number | null): string {
    return value == null ? "--" : `${numberFormatter.format(value)}%`;
  }

  function formatText(value: unknown): string {
    return typeof value === "string" && value.trim() !== ""
      ? value.trim()
      : "--";
  }

  function formatQuoteTime(value: string): string {
    const timestamp = Date.parse(value);
    if (value === "" || !Number.isFinite(timestamp)) return value;
    const date = new Date(timestamp);
    const month = String(date.getMonth() + 1).padStart(2, "0");
    const day = String(date.getDate()).padStart(2, "0");
    const hours = String(date.getHours()).padStart(2, "0");
    const minutes = String(date.getMinutes()).padStart(2, "0");
    const seconds = String(date.getSeconds()).padStart(2, "0");
    return `${month}/${day} ${hours}:${minutes}:${seconds}`;
  }

  const statusLine = computed(() => {
    const session =
      snapshot.value?.session ||
      security.value?.sessionStatus ||
      "";
    const time =
      snapshot.value?.at ||
      snapshot.value?.observedAt ||
      security.value?.updateTime ||
      "";
    const labels: Record<string, string> = {
      regular: "交易中",
      pre: "盘前",
      after: "盘后",
      overnight: "夜盘",
      closed: "已收盘",
    };
    const normalized = typeof session === "string" ? session.toLowerCase() : "";
    const status = labels[normalized] ?? (typeof session === "string" ? session : "");
    const timeLabel = formatQuoteTime(typeof time === "string" ? time : "");
    return [status, timeLabel].filter(Boolean).join(" ");
  });

  const metric = (label: string, value: string): MetricItem => ({ label, value });
  const metrics = computed<MetricItem[]>(() => {
    const details = security.value;
    const quote = snapshot.value;
    const base = [
      metric("最高", formatPrice(firstNumber(quote?.highPrice, details?.highPrice))),
      metric("最低", formatPrice(firstNumber(quote?.lowPrice, details?.lowPrice))),
      metric("今开", formatPrice(firstNumber(quote?.openPrice, details?.openPrice))),
      metric("昨收", formatPrice(previousClose.value)),
      metric("成交量", formatVolume(firstNumber(quote?.volume, details?.volume))),
      metric("成交额", formatVolume(firstNumber(quote?.turnover, details?.turnover))),
      metric("买一", formatPrice(firstNumber(quote?.bid, details?.bidPrice))),
      metric("卖一", formatPrice(firstNumber(quote?.ask, details?.askPrice))),
    ];
    if (details?.plate != null || resolvedTarget.value?.kind === "plate") {
      const exact = details?.plate ?? details?.index;
      const stats = exact ?? plateMemberStats.value;
      return [
        ...base,
        metric("上涨", formatPrice(firstNumber(stats?.raiseCount))),
        metric("下跌", formatPrice(firstNumber(stats?.fallCount))),
        metric("平盘", formatPrice(firstNumber(stats?.equalCount))),
        ...(exact == null && plateMemberStats.value != null
          ? [metric("统计范围", `${plateMemberStats.value.sampleSize} / ${plateMemberStats.value.total} 只`)]
          : []),
      ];
    }
    if (details?.index != null || resolvedTarget.value?.productClass === "index") {
      return [
        ...base,
        metric("上涨", formatPrice(firstNumber(details?.index?.raiseCount))),
        metric("下跌", formatPrice(firstNumber(details?.index?.fallCount))),
        metric("平盘", formatPrice(firstNumber(details?.index?.equalCount))),
      ];
    }
    if (
      details?.trust != null ||
      ["fund", "etf", "trust"].includes(
        resolvedTarget.value?.productClass ?? "",
      )
    ) {
      return [
        ...base,
        metric("资产类别", formatText(details?.trust?.assetClass)),
        metric("资产规模", formatVolume(firstNumber(details?.trust?.aum))),
        metric("净值", formatPrice(firstNumber(details?.trust?.netAssetValue))),
        metric("股息率", formatRatio(firstNumber(details?.trust?.dividendYield))),
      ];
    }
    if (
      details?.warrant != null ||
      ["warrant", "cbbc"].includes(
        resolvedTarget.value?.productClass ?? "",
      )
    ) {
      return [
        ...base,
        metric("行权价", formatPrice(firstNumber(details?.warrant?.strikePrice))),
        metric("到期日", formatText(details?.warrant?.maturityTime)),
        metric("杠杆", formatPrice(firstNumber(details?.warrant?.leverage))),
        metric("溢价", formatRatio(firstNumber(details?.warrant?.premium))),
        metric("引伸波幅", formatRatio(firstNumber(details?.warrant?.impliedVolatility))),
      ];
    }
    if (details?.equity != null) {
      return [
        ...base,
        metric("总市值", formatVolume(firstNumber(details.equity.issuedMarketValue))),
        metric("流通值", formatVolume(firstNumber(details.equity.outstandingMarketVal))),
        metric("换手率", formatRatio(firstNumber(details.turnoverRate))),
        metric("平均价", formatPrice(firstNumber(details.averagePrice))),
        metric("市盈率TTM", formatPrice(firstNumber(details.equity.peTTMRate, details.equity.peRate))),
        metric("市净率", formatPrice(firstNumber(details.equity.pbRate))),
        metric("总股本", formatVolume(firstNumber(details.equity.issuedShares))),
        metric("流通股", formatVolume(firstNumber(details.equity.outstandingShares))),
        metric("每股净资", formatPrice(firstNumber(details.equity.netAssetPerShare))),
        metric("委比", formatRatio(firstNumber(details.bidAskRatio))),
        metric("量比", formatPrice(firstNumber(details.volumeRatio))),
        metric("股息率TTM", formatRatio(firstNumber(details.equity.dividendRatioTTM))),
        metric("52周最高", formatPrice(firstNumber(details.highest52WeeksPrice))),
        metric("52周最低", formatPrice(firstNumber(details.lowest52WeeksPrice))),
      ];
    }
    return base;
  });

  const sourceLabel = computed(() => {
    const source =
      snapshotResult.value?.meta.source ?? securityResult.value?.meta.source ?? "";
    const broker =
      snapshotResult.value?.meta.brokerId ??
      securityResult.value?.meta.brokerId ??
      normalizedBrokerId.value;
    return [broker?.toUpperCase(), source].filter(Boolean).join(" · ");
  });

  function errorMessage(cause: unknown, fallback: string): string {
    return cause instanceof Error && cause.message.trim() !== ""
      ? cause.message
      : fallback;
  }

  function marketDataPath(
    resource: "snapshots" | "securities",
    suffix = "",
  ): string {
    const parts = instrumentParts.value;
    if (parts == null) return "";
    const base = `/api/v1/market-data/${resource}/${encodeURIComponent(parts.market)}/${encodeURIComponent(parts.symbol)}${suffix}`;
    return withBrokerProvider(base, normalizedBrokerId.value);
  }

  async function loadSnapshot(token: number, polling = false): Promise<void> {
    const localToken = ++snapshotToken;
    const path = marketDataPath("snapshots", "?refresh=true");
    if (path === "") return;
    if (!polling) snapshotLoading.value = true;
    try {
      const response = await fetchEnvelope<MarketDataSnapshotQueryResult>(path);
      if (token !== requestToken || localToken !== snapshotToken) return;
      snapshotResult.value = normalizeMarketDataSnapshotQueryResult(response);
      snapshotError.value = "";
    } catch (cause) {
      if (token === requestToken && localToken === snapshotToken) {
        snapshotError.value = errorMessage(cause, "行情快照加载失败");
      }
    } finally {
      if (!polling && token === requestToken) snapshotLoading.value = false;
    }
  }

  async function loadSecurity(token: number): Promise<void> {
    const path = marketDataPath("securities");
    if (path === "") return;
    securityLoading.value = true;
    try {
      const response = await fetchEnvelope<MarketSecurityDetailsQueryResult>(path);
      if (token !== requestToken) return;
      securityResult.value = normalizeMarketSecurityDetailsQueryResult(response);
      securityError.value = "";
    } catch (cause) {
      if (token === requestToken) {
        securityError.value = errorMessage(cause, "证券详情加载失败");
      }
    } finally {
      if (token === requestToken) securityLoading.value = false;
    }
  }

  async function loadPlateMembers(token: number): Promise<void> {
    const target = resolvedTarget.value;
    const parts = instrumentParts.value;
    if (target?.kind !== "plate" || parts == null) return;
    plateMembersLoading.value = true;
    const params = new URLSearchParams({
      operation: "plate_members",
      market: parts.market,
      instrumentId: target.instrumentId,
      pageSize: String(PLATE_MEMBER_REQUEST_LIMIT),
    });
    const path = withBrokerProvider(
      `/api/v1/research/industries?${params.toString()}`,
      normalizedBrokerId.value,
    );
    try {
      const response = await fetchEnvelope<FeatureResult>(path);
      if (token !== requestToken) return;
      const seen = new Set<string>();
      const members = (response.entries ?? [])
        .map((entry) => researchQuoteTargetFromEntry(entry, parts.market))
        .filter((member): member is ResearchQuoteTarget => {
          if (member == null || seen.has(member.instrumentId)) return false;
          seen.add(member.instrumentId);
          return true;
        })
        .slice(0, PLATE_MEMBER_REQUEST_LIMIT);
      plateMembers.value = members.slice(0, PLATE_MEMBER_DISPLAY_LIMIT);
      plateMembersError.value = "";
      try {
        const memberSnapshots = await fetchResearchSnapshots(
          members.map((member) => member.instrumentId),
          normalizedBrokerId.value,
        );
        if (token !== requestToken) return;
        const stats: PlateMemberStats = {
          raiseCount: 0, fallCount: 0, equalCount: 0,
          sampleSize: members.length, total: response.total ?? members.length,
        };
        for (const item of memberSnapshots) {
          const price = finiteNumber(item.lastPrice ?? item.price);
          const previous = finiteNumber(item.previousClose ?? item.previousClosePrice);
          if (price == null || previous == null) continue;
          if (price > previous) stats.raiseCount += 1;
          else if (price < previous) stats.fallCount += 1;
          else stats.equalCount += 1;
        }
        plateMemberStats.value = stats;
      } catch {
        // Snapshot enrichment is auxiliary; keep the successfully loaded members.
        if (token === requestToken) plateMemberStats.value = null;
      }
    } catch (cause) {
      if (token !== requestToken) return;
      plateMembersError.value = errorMessage(cause, "板块成分股加载失败");
    } finally {
      if (token === requestToken) plateMembersLoading.value = false;
    }
  }

  async function loadFavorite(token: number): Promise<void> {
    const parts = instrumentParts.value;
    if (parts == null || resolvedTarget.value?.kind === "plate") return;
    try {
      const membership = await getWatchlistMembership(parts.market, parts.symbol);
      if (token === requestToken) favorite.value = membership.groupIds.length > 0;
    } catch {
      // Favorite state is auxiliary. A watchlist outage must not degrade quotes.
      if (token === requestToken) favorite.value = false;
    }
  }

  function resetTargetState(): void {
    snapshotResult.value = null;
    securityResult.value = null;
    plateMembers.value = [];
    plateMemberStats.value = null;
    snapshotError.value = "";
    securityError.value = "";
    plateMembersError.value = "";
    snapshotLoading.value = false;
    securityLoading.value = false;
    plateMembersLoading.value = false;
    favorite.value = false;
    watchlistDialogOpen.value = false;
  }

  async function loadTarget(): Promise<void> {
    const token = ++requestToken;
    snapshotToken++;
    resetTargetState();
    if (resolvedTarget.value == null || instrumentParts.value == null) return;
    await Promise.allSettled([
      loadSnapshot(token),
      loadSecurity(token),
      loadPlateMembers(token),
      loadFavorite(token),
    ]);
  }

  async function refresh(): Promise<void> {
    if (resolvedTarget.value == null || instrumentParts.value == null) return;
    const token = requestToken;
    await Promise.allSettled([
      loadSnapshot(token),
      loadSecurity(token),
      loadPlateMembers(token),
      loadFavorite(token),
    ]);
  }

  function clearSnapshotPolling(): void {
    if (snapshotPollTimer == null || typeof window === "undefined") return;
    window.clearInterval(snapshotPollTimer);
    snapshotPollTimer = null;
  }

  function restartSnapshotPolling(): void {
    clearSnapshotPolling();
    if (
      !mounted ||
      typeof window === "undefined" ||
      props.visible === false ||
      !documentVisible.value ||
      instrumentParts.value == null
    ) {
      return;
    }
    snapshotPollTimer = window.setInterval(() => {
      if (props.visible === false || !documentVisible.value) return;
      void loadSnapshot(requestToken, true);
    }, 3_000);
  }

  function syncDocumentVisibility(): void {
    documentVisible.value =
      typeof document === "undefined" || document.visibilityState !== "hidden";
    restartSnapshotPolling();
    if (
      documentVisible.value &&
      props.visible !== false &&
      instrumentParts.value != null
    ) {
      void loadSnapshot(requestToken, true);
    }
  }

  function handleWatchlistSaved(membership: WatchlistMembership): void {
    favorite.value = membership.groupIds.length > 0;
  }

  watch(
    [targetKey, normalizedBrokerId],
    () => {
      void loadTarget();
      restartSnapshotPolling();
    },
    { immediate: true },
  );
  watch(
    () => props.visible,
    () => restartSnapshotPolling(),
  );

  onMounted(() => {
    mounted = true;
    if (typeof document !== "undefined") {
      document.addEventListener("visibilitychange", syncDocumentVisibility);
    }
    restartSnapshotPolling();
  });

  onBeforeUnmount(() => {
    requestToken++;
    snapshotToken++;
    mounted = false;
    clearSnapshotPolling();
    if (typeof document !== "undefined") {
      document.removeEventListener("visibilitychange", syncDocumentVisibility);
    }
  });

  return {
    resolvedTarget,
    instrumentParts,
    name,
    lastPrice,
    changeAmount,
    changeRate,
    statusLine,
    sourceLabel,
    metrics,
    security,
    snapshot,
    extendedCards,
    plateMembers,
    snapshotLoading,
    securityLoading,
    plateMembersLoading,
    snapshotError,
    securityError,
    plateMembersError,
    watchlistDialogOpen,
    favorite,
    directionClass,
    formatPrice,
    formatSigned,
    handleWatchlistSaved,
    refresh,
  };
}
