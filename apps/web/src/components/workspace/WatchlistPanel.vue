<script setup lang="ts">
import { computed } from "vue";

import { useConsoleData } from "../../composables/useConsoleData";
import { useWorkspaceLayout } from "../../composables/useWorkspaceLayout";

const {
  marketDataSubscriptions,
  marketInstrumentSearchOptions,
  formatDateTime,
} = useConsoleData();
const { prefs, update } = useWorkspaceLayout();

const rows = computed(() =>
  marketDataSubscriptions.value.entries.map((entry) => ({
    key: entry.key,
    market: entry.market,
    symbol: entry.symbol,
    name:
      marketInstrumentSearchOptions.value.find(
        (option) => option.instrumentId === entry.instrumentId,
      )?.name ?? null,
    refCount: entry.refCount,
    consumers: entry.consumers.length,
    updatedAt: entry.updatedAt,
    channel: entry.channel,
  })),
);

function pick(market: string, symbol: string): void {
  update({ market, symbol });
}
</script>

<template>
  <section class="tv-panel tv-grid-area-watchlist">
    <div class="tv-panel-head">
      <span class="tv-panel-title">Watchlist</span>
      <span style="color: var(--tv-text-dim); font-size: 11px">
        {{ marketDataSubscriptions.totalActiveSubscriptions }} active ·
        {{ marketInstrumentSearchOptions.length }} searchable ·
        quota {{ marketDataSubscriptions.quota.totalUsed }}/{{ marketDataSubscriptions.quota.totalLimit ?? "∞" }}
      </span>
    </div>
    <div class="tv-panel-body is-flush">
      <table class="tv-table">
        <thead>
          <tr>
            <th>Symbol</th>
            <th>Name</th>
            <th>Mkt</th>
            <th>Ch</th>
            <th class="tv-num">Refs</th>
            <th class="tv-num">Cons</th>
            <th>Updated</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in rows"
            :key="row.key"
            style="cursor: pointer"
            :class="{ 'is-active': prefs.symbol === row.symbol && prefs.market === row.market }"
            @click="pick(row.market, row.symbol)"
          >
            <td style="font-weight: 600">{{ row.symbol }}</td>
            <td style="color: var(--tv-text-muted)">{{ row.name ?? "N/A" }}</td>
            <td>{{ row.market }}</td>
            <td style="color: var(--tv-text-muted)">{{ row.channel }}</td>
            <td class="tv-num">{{ row.refCount }}</td>
            <td class="tv-num">{{ row.consumers }}</td>
            <td style="color: var(--tv-text-dim); font-size: 11px">{{ formatDateTime(row.updatedAt) }}</td>
          </tr>
          <tr v-if="rows.length === 0">
            <td colspan="7" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">
              暂无订阅。策略 / Broker 建立订阅后将出现在此处。
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
