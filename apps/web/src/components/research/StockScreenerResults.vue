<script setup lang="ts">
import {
  factorRefKey,
  formatStockScreenValue,
  resultColumnFor,
  stockScreenValueTitle,
} from "./stockScreenModel";
import { useStockScreenerControllerContext } from "./useStockScreenerController";

const {
  mobilePane,
  resultLabel,
  asOf,
  resultStale,
  catalog,
  loading,
  entries,
  selectedPreset,
  displayColumns,
  columnIdentity,
  factorFor,
  resultColumns,
  selectedInstrumentId,
  selectEntry,
  openEntry,
  hasMore,
  loadingMore,
  execute,
  nextOffset,
} = useStockScreenerControllerContext();
</script>

<template>
  <main
    class="stock-screener-view__results"
    :class="{ 'is-mobile-hidden': mobilePane !== 'results' }"
  >
    <div class="stock-screener-view__result-head">
      <strong>筛选结果</strong>
      <span>{{ resultLabel }}</span>
      <span v-if="asOf">数据时间 {{ asOf }}</span>
      <span v-if="resultStale" class="stock-screener-view__result-stale">
        条件已修改，结果待更新
      </span>
      <span v-if="catalog?.rateLimit">
        限流 {{ catalog.rateLimit.windowSeconds }} 秒 /
        {{ catalog.rateLimit.requests }} 次
      </span>
    </div>
    <div v-if="loading" class="stock-screener-view__empty">
      正在执行筛选…
    </div>
    <div v-else-if="entries.length === 0" class="stock-screener-view__empty">
      {{
        selectedPreset
          ? "已恢复预设，请手动执行筛选"
          : "配置条件和结果列后手动执行筛选"
      }}
    </div>
    <div v-else class="stock-screener-view__table-wrap">
      <table>
        <thead>
          <tr>
            <th>代码</th>
            <th>名称</th>
            <th
              v-for="(column, columnIndex) in displayColumns"
              :key="columnIdentity(column, columnIndex)"
            >
              {{
                factorFor(factorRefKey(column))?.label ?? factorRefKey(column)
              }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="entry in entries"
            :key="entry.instrumentId ?? entry.stockId"
            :class="{
              'is-selected':
                selectedInstrumentId ===
                (entry.instrumentId ?? entry.stockId),
            }"
            tabindex="0"
            @click="selectEntry(entry)"
            @dblclick="openEntry(entry)"
            @keydown.enter="selectEntry(entry)"
          >
            <td class="tv-num">{{ entry.symbol }}</td>
            <td>{{ entry.name }}</td>
            <td
              v-for="(column, columnIndex) in displayColumns"
              :key="columnIdentity(column, columnIndex)"
              class="tv-num"
              :title="
                stockScreenValueTitle(
                  resultColumnFor(entry, column, resultColumns),
                  factorFor(factorRefKey(column)),
                  entry,
                )
              "
            >
              {{
                formatStockScreenValue(
                  resultColumnFor(entry, column, resultColumns),
                  factorFor(factorRefKey(column)),
                  entry,
                )
              }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <button
      v-if="hasMore"
      class="stock-screener-view__more"
      type="button"
      :disabled="loadingMore"
      @click="execute(nextOffset ?? entries.length, true)"
    >
      {{ loadingMore ? "加载中…" : "加载更多" }}
    </button>
  </main>
</template>

<style scoped>
.stock-screener-view__results {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.stock-screener-view__result-head {
  display: flex;
  min-width: 0;
  min-height: 32px;
  flex: none;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
}

.stock-screener-view__result-head > span {
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
}

.stock-screener-view__result-head span:last-child {
  margin-left: auto;
}

.stock-screener-view__result-stale {
  color: #d99a2b !important;
  font-weight: 600;
}

.stock-screener-view__empty {
  display: grid;
  min-width: 0;
  min-height: 120px;
  place-items: center;
  color: var(--tv-text-dim);
}

.stock-screener-view__table-wrap {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  flex: 1;
  overflow: auto;
}

.stock-screener-view__results table {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.stock-screener-view__results th {
  position: sticky;
  z-index: 2;
  top: 0;
  height: 32px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
  text-align: left;
}

.stock-screener-view__results td {
  height: 32px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
}

.stock-screener-view__results tbody tr:hover {
  background: var(--tv-bg-elevated);
}

.stock-screener-view__results tbody tr.is-selected {
  background: color-mix(in srgb, var(--tv-accent) 10%, transparent);
}

.stock-screener-view__more {
  align-self: center;
  margin: 8px;
}
</style>
