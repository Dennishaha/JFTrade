<script setup lang="ts">
import { ref } from "vue";

import type {
  RealTradeKillSwitchEventsResponse,
  RealTradeRiskEventsResponse,
} from "@/contracts";
import {
  formatDateTime,
  formatRealTradeEventTypeLabel,
  resolveRealTradeKillSwitchEventTagType,
  resolveRealTradeRiskEventTagType,
} from "@/composables/consoleDataFormatting";

defineProps<{
  killSwitchEvents: RealTradeKillSwitchEventsResponse["entries"];
  riskEvents: RealTradeRiskEventsResponse["entries"];
}>();

type EventSourceFilter = "all" | "risk" | "kill-switch";

const SOURCE_FILTERS: ReadonlyArray<{
  value: EventSourceFilter;
  label: string;
}> = [
  { value: "all", label: "全部" },
  { value: "risk", label: "配置与拒单" },
  { value: "kill-switch", label: "熔断" },
];

const sourceFilter = ref<EventSourceFilter>("all");

function tagClass(tag: "success" | "warning" | "danger" | "info"): string {
  return tag === "danger" ? "tv-status--error" : `tv-status--${tag}`;
}
</script>

<template>
  <section class="risk-events" aria-label="最近风控事件">
    <header class="risk-events__head">
      <span class="risk-events__title">最近风控事件</span>
      <span class="risk-events__desc">运行时配置、熔断和拒单事件会出现在这里。</span>
      <div class="risk-events__filter" role="group" aria-label="事件来源筛选">
        <button
          v-for="option in SOURCE_FILTERS"
          :key="option.value"
          type="button"
          class="risk-events__filter-btn"
          :class="{ 'is-active': sourceFilter === option.value }"
          :aria-pressed="sourceFilter === option.value"
          @click="sourceFilter = option.value"
        >
          {{ option.label }}
        </button>
      </div>
    </header>

    <div
      class="risk-events__columns"
      :class="{ 'risk-events__columns--single': sourceFilter !== 'all' }"
    >
      <div v-if="sourceFilter !== 'kill-switch'" class="risk-events__column">
        <div class="risk-events__column-title">配置与拒单</div>
        <template v-if="riskEvents.length">
          <div
            v-for="item in riskEvents.slice(0, 5)"
            :key="item.id"
            class="risk-events__event"
          >
            <div class="risk-events__event-head">
              <b>{{ item.action || formatRealTradeEventTypeLabel(item.eventType) }}</b>
              <span
                class="risk-events__tag tv-status-surface"
                :class="tagClass(resolveRealTradeRiskEventTagType(item.eventType))"
              >
                {{ formatRealTradeEventTypeLabel(item.eventType) }}
              </span>
            </div>
            <div class="risk-events__event-meta">
              {{ formatDateTime(item.createdAt) }} / {{ item.operatorId ?? "system" }}
            </div>
            <div class="risk-events__event-reason">
              {{ item.reason || item.errorCode || "暂无原因" }}
            </div>
          </div>
        </template>
        <div v-else class="risk-events__empty">暂无运行时风控事件。</div>
      </div>

      <div v-if="sourceFilter !== 'risk'" class="risk-events__column">
        <div class="risk-events__column-title">熔断</div>
        <template v-if="killSwitchEvents.length">
          <div
            v-for="item in killSwitchEvents.slice(0, 5)"
            :key="item.id"
            class="risk-events__event"
          >
            <div class="risk-events__event-head">
              <b>{{ item.action || formatRealTradeEventTypeLabel(item.eventType) }}</b>
              <span
                class="risk-events__tag tv-status-surface"
                :class="tagClass(resolveRealTradeKillSwitchEventTagType(item.eventType))"
              >
                {{ formatRealTradeEventTypeLabel(item.eventType) }}
              </span>
            </div>
            <div class="risk-events__event-meta">
              {{ formatDateTime(item.createdAt) }} / {{ item.operatorId ?? "system" }}
            </div>
            <div class="risk-events__event-reason">
              {{ item.reason || item.errorCode || "暂无原因" }}
            </div>
          </div>
        </template>
        <div v-else class="risk-events__empty">暂无熔断事件。</div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.risk-events {
  display: flex;
  min-width: 0;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: var(--tv-bg-surface);
}

.risk-events__head {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.risk-events__title {
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.risk-events__desc {
  overflow: hidden;
  flex: 1;
  color: var(--tv-text-dim);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.risk-events__filter {
  display: inline-flex;
  flex: 0 0 auto;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
}

.risk-events__filter-btn {
  padding: 3px 10px;
  border: 0;
  background: transparent;
  color: var(--tv-text-dim);
  cursor: pointer;
  font-size: 10px;
}

.risk-events__filter-btn.is-active {
  background: color-mix(in srgb, var(--tv-accent) 18%, var(--tv-bg-surface));
  color: var(--tv-accent);
}

.risk-events__columns {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  padding: 12px;
}

.risk-events__columns--single {
  grid-template-columns: minmax(0, 1fr);
}

.risk-events__column-title {
  margin-bottom: 8px;
  color: var(--tv-text-dim);
  font-size: 10px;
  font-weight: 650;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.risk-events__event {
  display: grid;
  gap: 3px;
  margin-bottom: 8px;
  padding: 8px 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}

.risk-events__event-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.risk-events__event-head b {
  color: var(--tv-text);
  font-size: 11px;
  font-weight: 600;
}

.risk-events__tag {
  flex: 0 0 auto;
  padding: 2px 8px;
  border: 1px solid;
  border-radius: 999px;
  font-size: 10px;
  white-space: nowrap;
}

.risk-events__event-meta {
  color: var(--tv-text-dim);
  font-size: 10px;
}

.risk-events__event-reason {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.risk-events__empty {
  padding: 8px 2px;
  color: var(--tv-text-dim);
  font-size: 11px;
}

@media (max-width: 1180px) {
  .risk-events__columns {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
