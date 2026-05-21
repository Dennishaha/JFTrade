<script setup lang="ts">
import { computed } from "vue";

import { useConsoleData } from "../../composables/useConsoleData";
import { useWorkspaceLayout } from "../../composables/useWorkspaceLayout";

const { marketDataSnapshot, marketInstrumentSearchOptions, formatDateTime } =
  useConsoleData();
const { prefs } = useWorkspaceLayout();

const snapshot = computed(() => marketDataSnapshot.value?.snapshot ?? null);
const instrumentId = computed(() => `${prefs.value.market}.${prefs.value.symbol}`);
const instrumentTitle = computed(() => {
  const option = marketInstrumentSearchOptions.value.find(
    (candidate) => candidate.instrumentId === instrumentId.value,
  );
  return option?.name == null || option.name === ""
    ? instrumentId.value
    : `${instrumentId.value} · ${option.name}`;
});
const isUSMarket = computed(() => prefs.value.market.trim().toUpperCase() === "US");
const snapshotSession = computed(() => {
  const session = snapshot.value?.session;
  return typeof session === "string" && session !== "" ? session : null;
});
const mainPriceLabel = computed(() => {
  if (!isUSMarket.value) return "最新价";
  return snapshotSession.value === "regular" ? "最新价" : "最近盘中收盘";
});
// 大字展示逻辑：盘中展示实时价，非盘中（盘前/盘后/夜盘）展示最近盘中收盘价
const mainDisplayPrice = computed(() => {
  const snap = snapshot.value;
  if (!snap) return null;
  if (!isUSMarket.value) return snap.price;
  if (snapshotSession.value === "regular") return snap.price;
  // 非盘中时段：previousClosePrice = 最近盘中收盘（盘前→昨日收盘，盘后/夜盘→今日4PM收盘）
  return snap.previousClosePrice ?? snap.price;
});
// mainChangePercent: 语义随时段变化
// ▸ 盘中（regular）：实时价 vs 昨收 → 当日涨跌
// ▸ 盘外（最近盘中收盘展示）：盘中收盘 vs 上个交易日收盘（lastClosePrice）
const mainChangePercent = computed(() => {
  const snap = snapshot.value;
  if (!snap) return null;
  if (!isUSMarket.value || snapshotSession.value === "regular") {
    const livePrice = snap.price;
    const previousClosePrice = snap.previousClosePrice;
    if (livePrice == null || previousClosePrice == null || previousClosePrice === 0) return null;
    return ((livePrice - previousClosePrice) / previousClosePrice) * 100;
  }
  // 扩展时段：最近盘中收盘 vs 上个交易日收盘
  const close = snap.previousClosePrice;
  const lastClose = snap.lastClosePrice;
  if (close == null || lastClose == null || lastClose === 0 || close === lastClose) return null;
  return ((close - lastClose) / lastClose) * 100;
});

// extendedCardChangeRate: 延伸时段卡片专用——实时延伸价格 vs 最近盘中收盘
const extendedCardChangeRate = computed(() => {
  const snap = snapshot.value;
  if (!snap) return null;
  const livePrice = snap.price;
  const previousClosePrice = snap.previousClosePrice;
  if (livePrice == null || previousClosePrice == null || previousClosePrice === 0) return null;
  return ((livePrice - previousClosePrice) / previousClosePrice) * 100;
});
const mainChangeClass = computed(() => {
  const percent = mainChangePercent.value;
  if (percent == null || percent === 0) return "";
  return percent > 0 ? "tv-up" : "tv-down";
});
const sessionLabel = computed(() => {
  if (snapshotSession.value === "regular") return "盘中";
  if (snapshotSession.value === "pre") return "盘前";
  if (snapshotSession.value === "after") return "盘后";
  if (snapshotSession.value === "overnight") return "夜盘";
  if (snapshotSession.value === "closed") return "休市";
  if (snapshotSession.value === "unknown") return "未知";
  return snapshotSession.value ?? "";
});
const extendedCards = computed(() => {
  if (!isUSMarket.value || snapshot.value?.extended == null) {
    return [] as Array<{
      key: string;
      label: string;
      price: number;
      changeRate: number | null;
      border: string;
      surface: string;
      accent: string;
    }>;
  }

  const snap = snapshot.value;
  const extended = snap.extended;
  // snap.price is refreshed on every live ticker event; extended.*.price only
  // updates on HTTP snapshot fetches (~60 s). For the currently-active extended
  // session, prefer snap.price so the card reflects live market data.
  const livePrice = snap.price > 0 ? snap.price : null;
  const liveChangeRate = extendedCardChangeRate.value;

  const cards: Array<{
    key: string;
    label: string;
    price: number;
    changeRate: number | null;
    border: string;
    surface: string;
    accent: string;
  }> = [];

  if (
    snapshotSession.value === "pre" &&
    extended?.preMarket?.price != null
  ) {
    cards.push({
      key: "pre",
      label: "盘前价格",
      price: livePrice ?? extended.preMarket.price,
      changeRate: liveChangeRate ?? extended.preMarket.changeRate ?? null,
      border: "var(--card-blue-border)",
      surface: "var(--card-blue-surface)",
      accent: "var(--card-blue-text)",
    });
  }

  if (
    (snapshotSession.value === "after" || snapshotSession.value === "overnight") &&
    extended?.afterMarket?.price != null
  ) {
    // after-market trading is only active when session === "after"; during
    // "overnight" that window is already closed so keep the snapshot price.
    const isActiveAfter = snapshotSession.value === "after";
    cards.push({
      key: "after",
      label: "盘后价格",
      price: isActiveAfter ? (livePrice ?? extended.afterMarket.price) : extended.afterMarket.price,
      changeRate: isActiveAfter
        ? (liveChangeRate ?? extended.afterMarket.changeRate ?? null)
        : (extended.afterMarket.changeRate ?? null),
      border: "var(--card-amber-border)",
      surface: "var(--card-amber-surface)",
      accent: "var(--card-amber-text)",
    });
  }

  if (
    snapshotSession.value === "overnight" &&
    extended?.overnight?.price != null
  ) {
    cards.push({
      key: "overnight",
      label: "夜盘价格",
      price: livePrice ?? extended.overnight.price,
      changeRate: liveChangeRate ?? extended.overnight.changeRate ?? null,
      border: "var(--card-purple-border)",
      surface: "var(--card-purple-surface)",
      accent: "var(--card-purple-text)",
    });
  }

  return cards;
});

function formatPrice(value: number | null | undefined): string {
  return value == null ? "—" : value.toFixed(3);
}

function formatPercent(value: number | null | undefined): string {
  if (value == null) return "—";
  const prefix = value > 0 ? "+" : "";
  return `${prefix}${value.toFixed(2)}%`;
}
</script>

<template>
  <section class="tv-panel tv-grid-area-watchlist">
    <div class="tv-panel-head">
      <span class="tv-panel-title">Price</span>
      <span style="font-weight: 600">{{ instrumentTitle }}</span>
      <div style="flex: 1"></div>
      <span
        style="font-size: 11px; padding: 3px 8px; border-radius: 999px; border: 1px solid var(--tv-border); white-space: nowrap"
        :style="snapshot ? 'color: var(--tv-up); background: var(--card-green-surface)' : 'color: var(--tv-text-dim)'"
      >
        {{ snapshot ? "LIVE" : "NO DATA" }}
      </span>
    </div>
    <div class="tv-panel-body">
      <div
        v-if="snapshot"
        style="display: flex; flex-direction: column; gap: 12px; min-height: 100%"
      >
        <div style="padding: 6px 4px 2px">
          <div style="font-size: 11px; color: var(--tv-text-dim); text-transform: uppercase; letter-spacing: 0.08em">
            {{ mainPriceLabel }}
          </div>
          <div style="display: flex; align-items: flex-end; gap: 10px; flex-wrap: wrap; margin-top: 8px">
            <div style="font-size: 42px; line-height: 1; font-weight: 700; color: var(--tv-text)">
              {{ formatPrice(mainDisplayPrice) }}
            </div>
            <div
              v-if="mainChangePercent != null"
              :class="mainChangeClass"
              style="font-size: 18px; line-height: 1.2; font-weight: 600"
            >
              {{ formatPercent(mainChangePercent) }}
            </div>
          </div>
          <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-top: 10px">
            <span
              v-if="isUSMarket && sessionLabel"
              style="font-size: 11px; padding: 3px 8px; border-radius: 999px; border: 1px solid var(--tv-border); white-space: nowrap"
              :style="snapshotSession === 'regular' ? 'color: var(--tv-up); background: var(--card-green-surface)' : 'color: var(--tv-text); background: var(--tv-bg-surface-2)'"
            >
              {{ sessionLabel }}
            </span>
            <span style="font-size: 11px; color: var(--tv-text-dim)">
              {{ formatDateTime(snapshot.observedAt ?? snapshot.at) }}
            </span>
          </div>
        </div>

        <div v-if="extendedCards.length" style="display: grid; gap: 8px">
          <div
            v-for="card in extendedCards"
            :key="card.key"
            style="border-radius: 6px; padding: 10px 12px; border: 1px solid"
            :style="`border-color: ${card.border}; background: ${card.surface};`"
          >
            <div
              style="font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em"
              :style="`color: ${card.accent};`"
            >
              {{ card.label }}
            </div>
            <div style="display: flex; align-items: flex-end; gap: 8px; margin-top: 8px; flex-wrap: wrap">
              <div style="font-size: 24px; line-height: 1; font-weight: 600; color: var(--tv-text)">
                {{ formatPrice(card.price) }}
              </div>
              <div
                v-if="card.changeRate != null"
                :class="card.changeRate > 0 ? 'tv-up' : card.changeRate < 0 ? 'tv-down' : ''"
                style="font-size: 13px; font-weight: 600"
              >
                {{ formatPercent(card.changeRate) }}
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="!isUSMarket"
          style="margin-top: auto; font-size: 11px; color: var(--tv-text-dim); line-height: 1.5"
        >
          非美股当前先按底层快照展示主价格；扩展时段数据是否可用，后续再按实际行情源补齐。
        </div>
      </div>
      <div
        v-else
        style="display: flex; align-items: center; justify-content: center; min-height: 180px; color: var(--tv-text-dim); text-align: center; padding: 24px"
      >
        当前标的暂无快照，行情加载后会在这里显示价格信息。
      </div>
    </div>
  </section>
</template>
