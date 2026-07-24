<script setup lang="ts">
import { computed } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import {
  dayKeyOf,
  directionClass,
  formatPrice,
  formatSigned,
  pickNumber,
  pickString,
} from "./researchEntry";
import {
  mergeResearchSnapshot,
  useResearchSnapshots,
} from "./researchSnapshots";
import { isResearchQuoteEntry } from "./researchQuote";

const props = withDefaults(
  defineProps<{ market?: string; brokerId?: string }>(),
  { market: "US", brokerId: "" },
);

const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
  more: [group: string];
}>();

const feature = useResearchFeature(
  () =>
    `/api/v1/research/calendars?market=${encodeURIComponent(props.market)}&operation=ipos&pageSize=50`,
  { brokerId: () => props.brokerId },
);

function isListed(entry: Record<string, unknown>): boolean {
  const status = pickString(entry, ["status"]);
  if (/listed|已上市/i.test(status)) return true;
  if (/pending|upcoming|待上市|申购/i.test(status)) return false;
  const rawDate = pickString(entry, [
    "listingDate",
    "date",
    "listTime",
    "eventDate",
  ]);
  const match = rawDate.match(/^(\d{4}-\d{2}-\d{2})/);
  if (match) return match[1]! <= dayKeyOf(new Date());
  return false;
}

const pendingEntries = computed(() =>
  feature.entries.value.filter((entry) => !isListed(entry)),
);
const listedRawEntries = computed(() =>
  feature.entries.value.filter((entry) => isListed(entry)),
);
const listedSnapshots = useResearchSnapshots(
  () =>
    listedRawEntries.value
      .map((entry) => pickString(entry, ["instrumentId"]))
      .filter(Boolean),
  () => props.brokerId,
);
const listedEntries = computed(() =>
  listedRawEntries.value.map((entry) =>
    mergeResearchSnapshot(
      entry,
      listedSnapshots.byInstrumentId.value[
        pickString(entry, ["instrumentId"]).toUpperCase()
      ],
    ),
  ),
);

function issueVolume(entry: Record<string, unknown>): string {
  const value = pickNumber(entry, ["issueVolume"]);
  if (value == null) return "--";
  if (Math.abs(value) >= 1e8) return `${(value / 1e8).toFixed(2)}亿股`;
  if (Math.abs(value) >= 1e4) return `${(value / 1e4).toFixed(2)}万股`;
  return String(value);
}

function issuePriceLabel(entry: Record<string, unknown>): string {
  const issuePrice = pickNumber(entry, ["issuePrice"]);
  const minimum = pickNumber(entry, ["issuePriceMin"]);
  const maximum = pickNumber(entry, ["issuePriceMax"]);
  if (minimum != null || maximum != null) {
    const lower = minimum ?? maximum;
    const upper = maximum ?? minimum;
    if (lower === upper) return formatPrice(lower);
    return `${formatPrice(lower)} ~ ${formatPrice(upper)}`;
  }
  return formatPrice(issuePrice);
}

function listedIsQuoteable(entry: Record<string, unknown>): boolean {
  return isResearchQuoteEntry(entry, props.market);
}
</script>

<template>
  <div class="ipo-center-view">
    <div v-if="feature.loading.value" class="ipo-center-view__status">加载中…</div>
    <div v-else-if="feature.error.value" class="ipo-center-view__status">
      {{ feature.error.value }}
    </div>
    <template v-else>
      <div v-if="listedSnapshots.error.value" class="ipo-center-view__warning">
        上市后行情补充失败：{{ listedSnapshots.error.value }}
      </div>
      <div class="ipo-center-view__panels">
        <section class="ipo-center-view__panel">
          <header class="ipo-center-view__head">
            <span>待上市</span>
            <button
              type="button"
              class="ipo-center-view__more"
              @click="emit('more', 'pending')"
            >
              更多 &gt;
            </button>
          </header>
          <div v-if="pendingEntries.length === 0" class="ipo-center-view__status">
            暂无数据
          </div>
          <div
            v-else
            class="ipo-center-view__table-scroll"
            role="region"
            aria-label="待上市新股表格"
            tabindex="0"
          >
            <table class="ipo-center-view__table">
              <thead>
                <tr>
                  <th>代码</th>
                  <th>名称</th>
                  <th class="ipo-center-view__num">发行价</th>
                  <th class="ipo-center-view__num">预计发行量</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(entry, index) in pendingEntries"
                  :key="pickString(entry, ['instrumentId', 'name']) || index"
                >
                  <td>
                    {{ pickString(entry, ["symbol", "instrumentId"]) || "--" }}
                  </td>
                  <td class="ipo-center-view__name">
                    {{ pickString(entry, ["name"]) || "--" }}
                  </td>
                  <td class="ipo-center-view__num tv-num">
                    {{ issuePriceLabel(entry) }}
                  </td>
                  <td class="ipo-center-view__num tv-num">
                    {{ issueVolume(entry) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section class="ipo-center-view__panel">
          <header class="ipo-center-view__head">
            <span>已上市</span>
            <button
              type="button"
              class="ipo-center-view__more"
              @click="emit('more', 'listed')"
            >
              更多 &gt;
            </button>
          </header>
          <div v-if="listedEntries.length === 0" class="ipo-center-view__status">
            暂无数据
          </div>
          <div
            v-else
            class="ipo-center-view__table-scroll"
            role="region"
            aria-label="已上市新股表格"
            tabindex="0"
          >
            <table class="ipo-center-view__table">
              <thead>
                <tr>
                  <th class="ipo-center-view__index">#</th>
                  <th>代码</th>
                  <th>名称</th>
                  <th class="ipo-center-view__num">最新价</th>
                  <th class="ipo-center-view__num">最新涨跌幅</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(entry, index) in listedEntries"
                  :key="pickString(entry, ['instrumentId', 'name']) || index"
                  :class="{ 'is-quoteable': listedIsQuoteable(entry) }"
                  @click="listedIsQuoteable(entry) && emit('select', entry)"
                >
                  <td class="ipo-center-view__index">{{ index + 1 }}</td>
                  <td>
                    {{ pickString(entry, ["symbol", "instrumentId"]) || "--" }}
                  </td>
                  <td class="ipo-center-view__name">
                    {{ pickString(entry, ["name"]) || "--" }}
                  </td>
                  <td class="ipo-center-view__num tv-num">
                    {{ formatPrice(pickNumber(entry, ["price", "lastPrice"])) }}
                  </td>
                  <td
                    class="ipo-center-view__num tv-num"
                    :class="directionClass(pickNumber(entry, ['changeRate']))"
                  >
                    {{ formatSigned(pickNumber(entry, ["changeRate"]), "%") }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
      <button
        v-if="feature.hasMore.value"
        type="button"
        class="ipo-center-view__load-more"
        :disabled="feature.loadingMore.value"
        @click="feature.loadMore()"
      >{{ feature.loadingMore.value ? "加载中…" : "加载更多" }}</button>
    </template>
  </div>
</template>

<style scoped>
.ipo-center-view {
  display: flex;
  min-height: 0;
  flex-direction: column;
  color: var(--tv-text);
  font-size: 12px;
}

.ipo-center-view__panels {
  display: grid;
  min-width: 0;
  grid-template-columns: repeat(auto-fit, minmax(min(320px, 100%), 1fr));
  gap: 8px;
}

.ipo-center-view__panel {
  display: flex;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.ipo-center-view__head {
  display: flex;
  height: 32px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  padding: 0 10px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  font-weight: 600;
}

.ipo-center-view__more {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--tv-text-dim);
  cursor: pointer;
  font-size: 12px;
  font-weight: 400;
}

.ipo-center-view__more:hover {
  color: var(--tv-text);
}

.ipo-center-view__status {
  display: grid;
  min-height: 96px;
  place-items: center;
  color: var(--tv-text-dim);
}

.ipo-center-view__warning {
  padding: 8px 10px;
  border: 1px solid color-mix(in srgb, var(--tv-warn) 40%, var(--tv-border));
  border-radius: 4px;
  color: var(--tv-warn);
}

.ipo-center-view__table-scroll {
  min-width: 0;
  overflow-x: auto;
  overscroll-behavior-inline: contain;
}

.ipo-center-view__table {
  width: 100%;
  min-width: 480px;
  border-collapse: collapse;
}

.ipo-center-view__table th,
.ipo-center-view__table td {
  height: 32px;
  padding: 0 10px;
  border-bottom: 1px solid var(--tv-border);
  text-align: left;
  white-space: nowrap;
}

.ipo-center-view__table th {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 500;
}

.ipo-center-view__table tbody tr.is-quoteable {
  cursor: pointer;
}

.ipo-center-view__table tbody tr.is-quoteable:hover td {
  background: var(--tv-bg-elevated);
}

.ipo-center-view__index {
  width: 32px;
  color: var(--tv-text-dim);
  font-variant-numeric: tabular-nums;
}

.ipo-center-view__num {
  text-align: right;
}

.ipo-center-view__name {
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ipo-center-view__load-more {
  align-self: center;
  margin: 8px;
  padding: 5px 14px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  cursor: pointer;
}
</style>
