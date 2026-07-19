<script setup lang="ts">
import { computed, ref, watch } from "vue";

import {
  type BrokerMarginRatiosResponse,
  emptyBrokerMarginRatios,
} from "@/contracts";

import { formatDateTime } from "../../composables/consoleDataFormatting";
import { formatInstrumentIdentityText } from "../../composables/instrumentPresentation";
import type { MarketSecurityDetails } from "../../composables/marketDataRealtime";
import { fetchEnvelope } from "../../composables/apiClient";
import { resolveBrokerQuery } from "../../composables/consoleDataBrokerAccountSelection";
import { useMarketProfiles } from "../../composables/marketProfiles";
import { resolveMarketSnapshotDisplay } from "../../composables/marketSessionDisplay";
import { getSharedLiveSocketHub } from "../../composables/sharedLiveSocket";
import { useConsoleData } from "../../composables/useConsoleData";
import { useWatchlistMembership } from "../../composables/useWatchlist";
import { useWorkspaceTradingPrefs } from "../../composables/useWorkspaceLayout";
import {
  formatCompactNumber as formatSharedCompactNumber,
  formatMarketPrice,
  formatNumber,
  formatPercent as formatSharedPercent,
} from "../../utils/numberFormat";
import InstrumentIdentity from "../domain/market-data/InstrumentIdentity.vue";
import MarketFeedStatus from "../domain/market-data/MarketFeedStatus.vue";
import DenseMetricStrip from "../domain/shared/DenseMetricStrip.vue";
import WatchlistMembershipDialog from "../domain/watchlist/WatchlistMembershipDialog.vue";

const {
  currentMarketDataSnapshot: marketDataSnapshot,
  currentMarketSecurityDetails: marketSecurityDetails,
  isLoadingMarketDataQuery,
  marketInstrumentSearchOptions,
  marketDataQueryError,
  selectedBrokerAccount,
  brokerRuntime,
  systemStatus,
  supportsBrokerReadFeature,
} = useConsoleData();
const { prefs } = useWorkspaceTradingPrefs();
const { pricePrecisionForMarket, supportsExtendedHoursForMarket } = useMarketProfiles();
const liveHub = getSharedLiveSocketHub();
const membershipDialogOpen = ref(false);

const snapshot = computed(() => marketDataSnapshot.value?.snapshot ?? null);
const security = computed(() => marketSecurityDetails.value?.security ?? null);
const instrumentId = computed(() => `${prefs.value.market}.${prefs.value.symbol}`);
const instrumentName = computed(() => {
  const option = marketInstrumentSearchOptions.value.find(
    (candidate) => candidate.instrumentId === instrumentId.value,
  );
  return option?.name ?? security.value?.name ?? "";
});
const { query: watchlistMembershipQuery } = useWatchlistMembership(
  () => prefs.value.market,
  () => prefs.value.symbol,
);
const isWatchlisted = computed(
  () => (watchlistMembershipQuery.data.value?.groupIds.length ?? 0) > 0,
);
const supportsExtendedHoursMarket = computed(() => supportsExtendedHoursForMarket(prefs.value.market));
const displayModel = computed(() =>
  resolveMarketSnapshotDisplay(snapshot.value, supportsExtendedHoursMarket.value),
);
const quoteObservedAt = computed(() =>
  snapshot.value?.observedAt ?? snapshot.value?.at ?? marketDataSnapshot.value?.meta.resolvedAt ?? null,
);
const quoteConnectionState = computed(() => liveHub.connectionState?.value ?? "idle");
const quoteTransportMode = computed(() => liveHub.lastHeartbeatEvent?.value?.transport?.mode ?? null);
const mainPriceLabel = computed(() => displayModel.value.mainPriceLabel);
const mainDisplayPrice = computed(() => displayModel.value.mainDisplayPrice);
const mainChangePercent = computed(() => displayModel.value.mainChangePercent);
const mainChangeClass = computed(() => {
  const percent = mainChangePercent.value;
  if (percent == null || percent === 0) return "";
  return percent > 0 ? "tv-up" : "tv-down";
});
const extendedCards = computed(() => {
  if (!supportsExtendedHoursMarket.value) {
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
  return displayModel.value.extendedCards.map((card) => ({
    ...card,
    border:
      card.key === "pre"
        ? "var(--card-sky-border)"
        : card.key === "overnight"
          ? "var(--card-violet-border)"
          : "var(--card-amber-border)",
    surface:
      card.key === "pre"
        ? "var(--card-sky-surface)"
        : card.key === "overnight"
          ? "var(--card-violet-surface)"
          : "var(--card-amber-surface)",
    accent:
      card.key === "pre"
        ? "var(--card-sky-text)"
        : card.key === "overnight"
          ? "var(--card-violet-text)"
          : "var(--card-amber-text)",
  }));
});

// ---- 融资融券（Margin / Short Selling） ----
const currentMarginRatio = ref<BrokerMarginRatiosResponse>(emptyBrokerMarginRatios);
const isLoadingCurrentMarginRatio = ref(false);
const marginHovered = ref(false);
const marginTriggerRef = ref<HTMLElement | null>(null);
const marginPopoverTop = ref(0);
const marginPopoverRight = ref(0);
let marginRatioFetchToken = 0;

function updateMarginPopoverPosition(): void {
  if (marginTriggerRef.value == null || typeof window === "undefined") return;
  const rect = marginTriggerRef.value.getBoundingClientRect();
  marginPopoverTop.value = rect.bottom + 8;
  marginPopoverRight.value = window.innerWidth - rect.right;
}

watch(marginHovered, (hovered) => {
  if (hovered) updateMarginPopoverPosition();
});

const marginRatioSupported = computed(() =>
  supportsBrokerReadFeature("marginRatios"),
);

const marginRatioEntry = computed(() => {
  const symbol = prefs.value.symbol;
  if (!symbol) return null;
  // 后端返回的 symbol 是带市场前缀的完整格式（如 HK.00700），
  // 而 prefs.value.symbol 可能是裸代码（如 00700）或已含前缀。
  const qualified = prefs.value.market ? `${prefs.value.market}.${symbol}` : symbol;
  return currentMarginRatio.value.marginRatios.find(
    (entry) => entry.symbol === symbol || entry.symbol === qualified,
  ) ?? null;
});

const hasLongPermit = computed(() => marginRatioEntry.value?.isLongPermit === true);
const hasShortPermit = computed(() => marginRatioEntry.value?.isShortPermit === true);
const showMarginBadges = computed(
  () => marginRatioSupported.value && (hasLongPermit.value || hasShortPermit.value),
);

async function fetchCurrentMarginRatio(): Promise<void> {
  const token = ++marginRatioFetchToken;
  const account = selectedBrokerAccount.value;
  if (!account || !prefs.value.symbol) {
    currentMarginRatio.value = emptyBrokerMarginRatios;
    marginHovered.value = false;
    return;
  }

  const brokerQuery = resolveBrokerQuery({
    selection: account,
    runtime: brokerRuntime.value,
    status: systemStatus.value,
  });
  brokerQuery.set("symbol", prefs.value.symbol);

  // 切换标的后立即清空，避免残留上一条标的的数据
  currentMarginRatio.value = emptyBrokerMarginRatios;
  isLoadingCurrentMarginRatio.value = true;

  try {
    const result = await fetchEnvelope<BrokerMarginRatiosResponse>(
      `/api/v1/brokers/${encodeURIComponent(account.brokerId)}/margin-ratios?${brokerQuery.toString()}`,
    );
    if (token === marginRatioFetchToken) {
      currentMarginRatio.value = result;
    }
  } catch {
    if (token === marginRatioFetchToken) {
      currentMarginRatio.value = emptyBrokerMarginRatios;
    }
  } finally {
    if (token === marginRatioFetchToken) {
      isLoadingCurrentMarginRatio.value = false;
    }
  }
}

watch(
  () => [
    prefs.value.symbol,
    selectedBrokerAccount.value?.selectionKey,
    brokerRuntime.value?.descriptor?.id,
  ],
  () => {
    if (marginRatioSupported.value) {
      fetchCurrentMarginRatio();
    } else {
      currentMarginRatio.value = emptyBrokerMarginRatios;
    }
  },
  { immediate: true },
);

type DetailRow = {
  label: string;
  value: string;
};

type DetailSection = {
  title: string;
  rows: DetailRow[];
};

const securitySummaryRows = computed<DetailRow[]>(() => {
  const item = security.value;
  if (!item) return [];
  return [
    { label: "类型", value: item.securityType || "—" },
    { label: "交易所", value: item.exchangeType || item.market || "—" },
    { label: "状态", value: formatSecurityStatus(item) },
    { label: "每手", value: formatInteger(item.lotSize) },
    { label: "上市", value: item.listTime || "—" },
    { label: "昨收", value: formatPrice(item.lastClosePrice) },
    { label: "开盘", value: formatPrice(item.openPrice) },
    { label: "最高", value: formatPrice(item.highPrice) },
    { label: "最低", value: formatPrice(item.lowPrice) },
    { label: "52周高", value: formatMaybePrice(item.highest52WeeksPrice) },
    { label: "52周低", value: formatMaybePrice(item.lowest52WeeksPrice) },
    { label: "量比", value: formatPlainNumber(item.volumeRatio) },
  ];
});

const typedDetailSections = computed<DetailSection[]>(() => {
  const item = security.value;
  if (!item) return [];

  const sections: DetailSection[] = [];
  if (item.equity) {
    sections.push({
      title: "股票基本面",
      rows: [
        { label: "总市值", value: formatCompactNumber(item.equity.issuedMarketValue) },
        { label: "流通市值", value: formatCompactNumber(item.equity.outstandingMarketVal) },
        { label: "PE", value: formatPlainNumber(item.equity.peRate) },
        { label: "PB", value: formatPlainNumber(item.equity.pbRate) },
        { label: "PE TTM", value: formatPlainNumber(item.equity.peTTMRate) },
        { label: "股息率 TTM", value: formatPercentValue(item.equity.dividendRatioTTM) },
      ],
    });
  }
  if (item.option) {
    sections.push({
      title: "期权信息",
      rows: [
        { label: "方向", value: item.option.optionType || "—" },
        { label: "标的", value: formatOwner(item.option.owner) },
        { label: "行权日", value: item.option.strikeTime || "—" },
        { label: "行权价", value: formatPrice(item.option.strikePrice) },
        { label: "隐含波动率", value: formatPercentValue(item.option.impliedVolatility) },
        { label: "Delta", value: formatPlainNumber(item.option.delta) },
        { label: "Gamma", value: formatPlainNumber(item.option.gamma) },
        { label: "Theta", value: formatPlainNumber(item.option.theta) },
      ],
    });
  }
  if (item.warrant) {
    sections.push({
      title: "轮证信息",
      rows: [
        { label: "类型", value: item.warrant.warrantType || "—" },
        { label: "正股", value: formatOwner(item.warrant.owner) },
        { label: "发行人", value: item.warrant.issuerCode || "—" },
        { label: "行使价", value: formatPrice(item.warrant.strikePrice) },
        { label: "到期日", value: item.warrant.maturityTime || "—" },
        { label: "杠杆", value: formatPlainNumber(item.warrant.leverage) },
        { label: "溢价", value: formatPercentValue(item.warrant.premium) },
        { label: "对冲值", value: formatPlainNumber(item.warrant.delta) },
      ],
    });
  }
  if (item.future) {
    sections.push({
      title: "期货信息",
      rows: [
        { label: "昨结", value: formatPrice(item.future.lastSettlePrice) },
        { label: "持仓量", value: formatInteger(item.future.position) },
        { label: "日增仓", value: formatInteger(item.future.positionChange) },
        { label: "最后交易日", value: item.future.lastTradeTime || "—" },
        { label: "主连", value: item.future.isMainContract ? "是" : "否" },
      ],
    });
  }
  if (item.trust) {
    sections.push({
      title: "基金信息",
      rows: [
        { label: "资产类别", value: item.trust.assetClass || "—" },
        { label: "股息率", value: formatPercentValue(item.trust.dividendYield) },
        { label: "AUM", value: formatCompactNumber(item.trust.aum) },
        { label: "单位净值", value: formatPrice(item.trust.netAssetValue) },
        { label: "溢价", value: formatPercentValue(item.trust.premium) },
      ],
    });
  }
  if (item.index) {
    sections.push({
      title: "指数成分",
      rows: [
        { label: "上涨", value: formatInteger(item.index.raiseCount) },
        { label: "下跌", value: formatInteger(item.index.fallCount) },
        { label: "平盘", value: formatInteger(item.index.equalCount) },
      ],
    });
  }
  if (item.plate) {
    sections.push({
      title: "板块成分",
      rows: [
        { label: "上涨", value: formatInteger(item.plate.raiseCount) },
        { label: "下跌", value: formatInteger(item.plate.fallCount) },
        { label: "平盘", value: formatInteger(item.plate.equalCount) },
      ],
    });
  }

  return sections;
});

function formatPrice(value: number | null | undefined): string {
  const market = prefs.value.market;
  return formatMarketPrice(value, {
    market,
    precision: pricePrecisionForMarket(market),
  });
}

function formatMaybePrice(value: number | null | undefined): string {
  return value == null ? "—" : formatPrice(value);
}

function formatPlainNumber(value: number | null | undefined): string {
  return formatNumber(value, { maximumFractionDigits: 2 });
}

function formatCompactNumber(value: number | null | undefined): string {
  return formatSharedCompactNumber(value);
}

function formatInteger(value: number | null | undefined): string {
  return formatNumber(value, { maximumFractionDigits: 0 });
}

function formatPercentValue(value: number | null | undefined): string {
  return formatSharedPercent(value);
}

function formatOwner(owner: { instrumentId: string } | null | undefined): string {
  return owner == null
    ? "—"
    : formatInstrumentIdentityText({ instrumentId: owner.instrumentId });
}

function formatSecurityStatus(item: MarketSecurityDetails): string {
  if (item.isSuspend) return "停牌";
  if (item.sessionStatus && item.sessionStatus !== "") return item.sessionStatus;
  return "正常";
}

function formatPercent(value: number | null | undefined): string {
  return formatSharedPercent(value, { showPositiveSign: true });
}
</script>

<template>
  <section class="tv-panel">
    <div class="tv-panel-head">
      <span class="tv-panel-title">行情</span>
      <div style="flex: 1"></div>
      <MarketFeedStatus
        :connection-state="quoteConnectionState"
        :observed-at="quoteObservedAt"
        :transport-mode="quoteTransportMode"
        :source="marketDataSnapshot?.meta.source ?? null"
        :from-cache="marketDataSnapshot?.meta.fromCache ?? false"
        :loading="isLoadingMarketDataQuery"
        :error="marketDataQueryError"
      />
    </div>
    <div class="tv-panel-body">
      <div v-if="snapshot" style="display: flex; flex-direction: column; gap: 12px; min-height: 100%">
        <div class="instrument-overview__quote-card">
          <div class="instrument-overview__identity-row">
            <InstrumentIdentity
              :market="prefs.market"
              :code="prefs.symbol"
              :instrument-id="instrumentId"
              :name="instrumentName"
              compact
            />
            <button
              type="button"
              class="instrument-overview__favorite"
              :class="{ 'is-active': isWatchlisted }"
              :title="isWatchlisted ? '管理自选分组' : '加入自选'"
              :aria-label="isWatchlisted ? '管理自选分组' : '加入自选'"
              data-testid="instrument-overview-favorite"
              @click="membershipDialogOpen = true"
            >
              {{ isWatchlisted ? "★" : "☆" }}
            </button>
          </div>
          <div style="font-size: 11px; color: var(--tv-text-dim); text-transform: uppercase; letter-spacing: 0.08em">
            {{ mainPriceLabel }}
          </div>
          <div style="display: flex; align-items: flex-end; gap: 10px; flex-wrap: wrap; margin-top: 8px">
            <div style="font-size: 38px; line-height: 1; font-weight: 650; color: var(--tv-text)">
              {{ formatPrice(mainDisplayPrice) }}
            </div>
            <div v-if="mainChangePercent != null" :class="mainChangeClass"
              style="font-size: 18px; line-height: 1.2; font-weight: 600">
              {{ formatPercent(mainChangePercent) }}
            </div>
          </div>
          <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-top: 10px">
            <span v-if="supportsExtendedHoursMarket && displayModel.sessionLabel"
              style="font-size: 11px; padding: 3px 8px; border-radius: 999px; border: 1px solid var(--tv-border); white-space: nowrap"
              :style="displayModel.session === 'regular' ? 'color: var(--tv-accent); background: color-mix(in srgb, var(--tv-accent) 14%, var(--tv-bg-surface-2))' : 'color: var(--tv-text); background: var(--tv-bg-surface-2)'">
              {{ displayModel.sessionLabel }}
            </span>
            <!-- 融资融券标志 -->
            <span v-if="showMarginBadges" ref="marginTriggerRef"
              style="display: inline-flex; align-items: center; gap: 4px" @mouseenter="marginHovered = true"
              @mouseleave="marginHovered = false">
              <span v-if="hasLongPermit"
                style="font-size: 11px; padding: 3px 8px; border-radius: 999px; white-space: nowrap; cursor: default"
                :style="{ color: 'var(--tv-price-up)', border: '1px solid var(--tv-price-up)', background: 'color-mix(in srgb, var(--tv-price-up) 12%, var(--tv-bg-surface-2))' }">融</span>
              <span v-if="hasShortPermit"
                style="font-size: 11px; padding: 3px 8px; border-radius: 999px; white-space: nowrap; cursor: default"
                :style="{ color: 'var(--tv-price-down)', border: '1px solid var(--tv-price-down)', background: 'color-mix(in srgb, var(--tv-price-down) 12%, var(--tv-bg-surface-2))' }">沽</span>
              <!-- 悬浮面板 -->
              <Teleport to="body">
                <div v-if="marginHovered && marginRatioEntry" :style="{
                  position: 'fixed',
                  top: `${marginPopoverTop}px`,
                  right: `${marginPopoverRight}px`,
                  zIndex: 9999,
                  minWidth: '260px',
                  background: 'var(--tv-bg-surface)',
                  border: '1px solid var(--tv-border)',
                  borderRadius: '8px',
                  padding: '14px 16px',
                  boxShadow: '0 8px 24px rgba(0,0,0,0.18)',
                  fontSize: '12px',
                  lineHeight: 1.6,
                  color: 'var(--tv-text)',
                  whiteSpace: 'nowrap',
                }">
                  <div class="text-xs" 
                    style="color: var(--tv-text); text-transform: uppercase; letter-spacing: 0.08em; margin-bottom: 10px">
                    融资融券信息
                  </div>
                  <div v-if="hasLongPermit" style="margin-bottom: 8px">
                    <div style="font-weight: 600; color: var(--tv-price-up); margin-bottom: 4px">融资（做多）</div>
                    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 2px 12px">
                      <span style="color: var(--tv-text-dim)">初始保证金率</span>
                      <span style="text-align: right">{{ formatPercentValue(marginRatioEntry.initialMarginLongRatio)
                      }}</span>
                      <span style="color: var(--tv-text-dim)">维持保证金率</span>
                      <span style="text-align: right">{{ formatPercentValue(marginRatioEntry.maintenanceLongRatio)
                      }}</span>
                      <span style="color: var(--tv-text-dim)">预警比率</span>
                      <span style="text-align: right">{{ formatPercentValue(marginRatioEntry.alertLongRatio) }}</span>
                      <span style="color: var(--tv-text-dim)">Margin Call</span>
                      <span style="text-align: right">{{ formatPercentValue(marginRatioEntry.marginCallLongRatio)
                      }}</span>
                    </div>
                  </div>
                  <div v-if="hasShortPermit">
                    <div style="font-weight: 600; color: var(--tv-price-down); margin-bottom: 4px">融券（做空）</div>
                    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 2px 12px">
                      <span style="color: var(--tv-text-dim)">融券利率</span>
                      <span style="text-align: right">{{ formatPercentValue(marginRatioEntry.shortFeeRate) }}</span>
                      <span style="color: var(--tv-text-dim)">初始保证金率</span>
                      <span style="text-align: right">{{ formatPercentValue(marginRatioEntry.initialMarginShortRatio)
                      }}</span>
                      <span style="color: var(--tv-text-dim)">维持保证金率</span>
                      <span style="text-align: right">{{ formatPercentValue(marginRatioEntry.maintenanceShortRatio)
                      }}</span>
                      <span style="color: var(--tv-text-dim)">预警比率</span>
                      <span style="text-align: right">{{ formatPercentValue(marginRatioEntry.alertShortRatio) }}</span>
                      <span style="color: var(--tv-text-dim)">Margin Call</span>
                      <span style="text-align: right">{{ formatPercentValue(marginRatioEntry.marginCallShortRatio)
                      }}</span>
                      <span v-if="marginRatioEntry.shortPoolRemain != null"
                        style="color: var(--tv-text-dim)">卖空池剩余</span>
                      <span v-if="marginRatioEntry.shortPoolRemain != null" style="text-align: right">{{
                        marginRatioEntry.shortPoolRemain.toLocaleString("zh-CN") }}</span>
                    </div>
                  </div>
                </div>
              </Teleport>
            </span>
            <span style="font-size: 11px; color: var(--tv-text-dim)">
              {{ formatDateTime(snapshot.observedAt ?? snapshot.at) }}
            </span>
          </div>
        </div>

        <div v-if="extendedCards.length" style="display: grid; gap: 8px">
          <div v-for="card in extendedCards" :key="card.key"
            style="border-radius: 6px; padding: 10px 12px; border: 1px solid"
            :style="`border-color: ${card.border}; background: ${card.surface};`">
            <div style="font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em"
              :style="`color: ${card.accent};`">
              {{ card.label }}
            </div>
            <div style="display: flex; align-items: flex-end; gap: 8px; margin-top: 8px; flex-wrap: wrap">
              <div style="font-size: 24px; line-height: 1; font-weight: 600; color: var(--tv-text)">
                {{ formatPrice(card.price) }}
              </div>
              <div v-if="card.changeRate != null"
                :class="card.changeRate > 0 ? 'tv-up' : card.changeRate < 0 ? 'tv-down' : ''"
                style="font-size: 13px; font-weight: 600">
                {{ formatPercent(card.changeRate) }}
              </div>
            </div>
          </div>
        </div>

        <div v-if="typedDetailSections.length" style="display: grid; gap: 10px">
          <div v-for="section in typedDetailSections" :key="section.title" style="display: grid; gap: 8px">
            <div style="font-size: 11px; color: var(--tv-text-dim); text-transform: uppercase; letter-spacing: 0.08em">
              {{ section.title }}
            </div>
            <DenseMetricStrip :items="section.rows" />
          </div>
        </div>

        <div v-if="securitySummaryRows.length" style="display: grid; gap: 8px">
          <div style="font-size: 11px; color: var(--tv-text-dim); text-transform: uppercase; letter-spacing: 0.08em">
            Security
          </div>
          <DenseMetricStrip :items="securitySummaryRows" />
        </div>

        <div v-if="!supportsExtendedHoursMarket" style="margin-top: auto; font-size: 11px; color: var(--tv-text-dim); line-height: 1.5">
          非美股当前先按底层快照展示主价格；扩展时段数据是否可用，后续再按实际行情源补齐。
        </div>
      </div>
      <div v-else
        style="display: flex; align-items: center; justify-content: center; min-height: 180px; color: var(--tv-text-dim); text-align: center; padding: 24px">
        当前标的暂无快照，行情加载后会在这里显示价格信息。
      </div>
    </div>
    <WatchlistMembershipDialog
      v-model="membershipDialogOpen"
      :market="prefs.market"
      :symbol="prefs.symbol"
      :name="instrumentName"
    />
  </section>
</template>

<style scoped>
.instrument-overview__quote-card {
  position: relative;
  padding: 6px 4px 2px;
}

.instrument-overview__identity-row {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 10px;
}

.instrument-overview__identity-row :deep(.instrument-identity) {
  min-width: 0;
  overflow: hidden;
}

.instrument-overview__identity-row :deep(.instrument-identity__name) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.instrument-overview__favorite {
  display: grid;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  place-items: center;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--tv-text-dim);
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
}

.instrument-overview__favorite:hover,
.instrument-overview__favorite.is-active {
  background: color-mix(in srgb, #eab308 12%, transparent);
  color: #eab308;
}
</style>
