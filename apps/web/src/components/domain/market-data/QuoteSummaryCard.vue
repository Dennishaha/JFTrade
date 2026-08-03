<script setup lang="ts">
import { computed } from "vue";

import type { MarketSnapshotDisplayCard } from "@/composables/market-data/marketSessionDisplay";
import { formatLocalDateTime } from "@/utils/dateTime";
import {
  formatMarketPrice,
  formatNumber,
  formatPercent,
} from "../../../utils/numberFormat";
import WatchlistFavoriteButton from "../watchlist/WatchlistFavoriteButton.vue";
import InstrumentIdentity from "./InstrumentIdentity.vue";

const props = withDefaults(
  defineProps<{
    market?: string | null;
    code?: string | null;
    instrumentId?: string | null;
    name?: string | null;
    compactIdentity?: boolean;
    priceLabel?: string;
    price?: number | null;
    changeAmount?: number | null;
    changeRate?: number | null;
    showChangeAmount?: boolean;
    sessionLabel?: string;
    sessionActive?: boolean;
    statusText?: string;
    sourceText?: string;
    loading?: boolean;
    favoriteVisible?: boolean;
    favoriteActive?: boolean;
    favoriteDisabled?: boolean;
    favoriteTestId?: string;
    extendedCards?: MarketSnapshotDisplayCard[];
  }>(),
  {
    market: null,
    code: null,
    instrumentId: null,
    name: null,
    compactIdentity: true,
    priceLabel: "最新价",
    price: null,
    changeAmount: null,
    changeRate: null,
    showChangeAmount: false,
    sessionLabel: "",
    sessionActive: false,
    statusText: "",
    sourceText: "",
    loading: false,
    favoriteVisible: false,
    favoriteActive: false,
    favoriteDisabled: false,
    favoriteTestId: "",
    extendedCards: () => [],
  },
);

defineEmits<{
  favorite: [];
}>();

const directionClass = computed(() => marketDirectionClass(props.changeRate));

function marketDirectionClass(value: number | null | undefined): string {
  if (value == null || value === 0) return "";
  return value > 0 ? "tv-up" : "tv-down";
}

function formattedPrice(value: number | null | undefined): string {
  return formatMarketPrice(value, {
    market: props.market,
    fallback: "--",
  });
}

function formattedChangeAmount(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return "--";
  const text = formatNumber(Math.abs(value), {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
    useGrouping: false,
  });
  if (value > 0) return `+${text}`;
  if (value < 0) return `-${text}`;
  return text;
}

function formattedChangeRate(value: number | null | undefined): string {
  return formatPercent(value, {
    fallback: "--",
    showPositiveSign: true,
  });
}

function formattedQuoteTime(value: string): string {
  return formatLocalDateTime(value, value);
}

function formattedExchangeTime(
  value: string,
  timezone: string | null | undefined,
): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime()) || timezone == null) {
    return formattedQuoteTime(value);
  }
  try {
    return new Intl.DateTimeFormat("en-US", {
      timeZone: timezone,
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hour12: false,
      timeZoneName: "short",
    }).format(date);
  } catch {
    return formattedQuoteTime(value);
  }
}

function exchangeTimeTitle(
  value: string,
  timezone: string | null | undefined,
): string | undefined {
  if (timezone == null || timezone.trim() === "") return undefined;
  return `交易所时间 ${formattedExchangeTime(value, timezone)}`;
}

function cutoffLabel(card: MarketSnapshotDisplayCard): string {
  if (card.key === "pre") return "盘前截止";
  if (card.key === "overnight") return "夜盘截止";
  return "盘后截止";
}
</script>

<template>
  <div class="quote-summary">
    <div class="quote-summary__card" data-testid="quote-summary-card">
      <div class="quote-summary__identity-row">
        <InstrumentIdentity
          class="quote-summary__identity"
          :market="market"
          :code="code"
          :instrument-id="instrumentId"
          :name="name"
          :compact="compactIdentity"
        />
        <WatchlistFavoriteButton
          v-if="favoriteVisible"
          :active="favoriteActive"
          :disabled="favoriteDisabled"
          :test-id="favoriteTestId"
          :title="favoriteActive ? '管理自选分组' : '加入自选'"
          @click="$emit('favorite')"
        />
      </div>

      <div class="quote-summary__price-label">
        {{ priceLabel }}
      </div>
      <div class="quote-summary__price-row">
        <strong class="quote-summary__price tv-num">
          {{ formattedPrice(price) }}
        </strong>
        <span
          v-if="showChangeAmount && changeAmount != null"
          class="quote-summary__change tv-num"
          :class="directionClass"
        >
          {{ formattedChangeAmount(changeAmount) }}
        </span>
        <span
          v-if="changeRate != null"
          class="quote-summary__change tv-num"
          :class="directionClass"
        >
          {{ formattedChangeRate(changeRate) }}
        </span>
      </div>

      <div
        v-if="
          sessionLabel ||
          statusText ||
          sourceText ||
          loading ||
          $slots.badges
        "
        class="quote-summary__status-row"
      >
        <span
          v-if="sessionLabel"
          class="quote-summary__session"
          :class="{ 'is-active': sessionActive }"
        >
          {{ sessionLabel }}
        </span>
        <slot name="badges" />
        <span v-if="statusText || loading" class="quote-summary__status">
          {{ statusText || "行情加载中…" }}
        </span>
        <span v-if="sourceText" class="quote-summary__source">
          {{ sourceText }}
        </span>
      </div>
    </div>

    <div v-if="extendedCards.length" class="quote-summary__extended">
      <div
        v-for="card in extendedCards"
        :key="card.key"
        class="quote-summary__extended-card"
        :class="`quote-summary__extended-card--${card.key}`"
      >
        <div class="quote-summary__extended-label">{{ card.label }}</div>
        <div class="quote-summary__extended-value-row">
          <strong class="quote-summary__extended-price tv-num">
            {{ formattedPrice(card.price) }}
          </strong>
          <span
            v-if="card.changeRate != null"
            class="quote-summary__extended-change tv-num"
            :class="marketDirectionClass(card.changeRate)"
          >
            {{ formattedChangeRate(card.changeRate) }}
          </span>
        </div>
        <div
          v-if="card.quoteTime || card.sessionEndAt"
          class="quote-summary__extended-time jf-note mt-1.5 leading-[1.3]"
        >
          <div
            v-if="card.quoteTime"
            :title="exchangeTimeTitle(card.quoteTime, card.exchangeTimezone)"
          >
            报价时间
            {{ formattedQuoteTime(card.quoteTime) }}
          </div>
          <div
            v-if="card.sessionEndAt"
            :title="exchangeTimeTitle(card.sessionEndAt, card.exchangeTimezone)"
          >
            {{ cutoffLabel(card) }}
            {{ formattedQuoteTime(card.sessionEndAt) }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.quote-summary {
  display: grid;
  min-width: 0;
  gap: 8px;
  container: quote-summary / inline-size;
}

.quote-summary__card {
  min-width: 0;
  padding: 6px 4px 2px;
}

.quote-summary__identity-row {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 10px;
}

.quote-summary__identity {
  min-width: 0;
  overflow: hidden;
}

.quote-summary__identity :deep(.instrument-identity__name) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quote-summary__price-label,
.quote-summary__extended-label {
  color: var(--tv-text-dim);
  font-size: var(--jf-text-5);
  letter-spacing: var(--jf-tracking-4);
  text-transform: uppercase;
}

.quote-summary__price-row {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  flex-wrap: wrap;
  margin-top: 8px;
}

.quote-summary__price {
  color: var(--tv-text);
  font-size: var(--jf-text-18);
  font-weight: 650;
  line-height: 1;
}

.quote-summary__change {
  font-size: var(--jf-text-10);
  font-weight: 600;
  line-height: 1.2;
}

.quote-summary__status-row {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 10px;
  color: var(--tv-text-dim);
  font-size: var(--jf-text-5);
}

.quote-summary__session {
  padding: 3px 8px;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  white-space: nowrap;
}

.quote-summary__session.is-active {
  background: color-mix(
    in srgb,
    var(--tv-accent) 14%,
    var(--tv-bg-surface-2)
  );
  color: var(--tv-accent);
}

.quote-summary__status {
  min-width: 0;
}

.quote-summary__source {
  min-width: 0;
  overflow: hidden;
  margin-inline-start: auto;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quote-summary__extended {
  display: grid;
  gap: 8px;
}

.quote-summary__extended-card {
  padding: 10px 12px;
  border: 1px solid var(--card-amber-border);
  border-radius: 6px;
  background: var(--card-amber-surface);
}

.quote-summary__extended-card--pre {
  border-color: var(--card-sky-border);
  background: var(--card-sky-surface);
}

.quote-summary__extended-card--overnight {
  border-color: var(--card-violet-border);
  background: var(--card-violet-surface);
}

.quote-summary__extended-card--pre .quote-summary__extended-label {
  color: var(--card-sky-text);
}

.quote-summary__extended-card--after .quote-summary__extended-label {
  color: var(--card-amber-text);
}

.quote-summary__extended-card--overnight .quote-summary__extended-label {
  color: var(--card-violet-text);
}

.quote-summary__extended-value-row {
  display: flex;
  align-items: flex-end;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 8px;
}

.quote-summary__extended-price {
  color: var(--tv-text);
  font-size: var(--jf-text-14);
  font-weight: 600;
  line-height: 1;
}

.quote-summary__extended-change {
  font-size: var(--jf-text-7);
  font-weight: 600;
}

@container quote-summary (max-width: 400px) {
  .quote-summary__price {
    font-size: var(--jf-text-17);
  }

  .quote-summary__change {
    font-size: var(--jf-text-8);
  }

  .quote-summary__source {
    width: 100%;
    margin-inline-start: 0;
  }
}
</style>
