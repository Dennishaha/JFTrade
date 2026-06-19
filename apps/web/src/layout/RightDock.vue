<script setup lang="ts">
import { computed } from "vue";

import { useConsoleData } from "../composables/useConsoleData";
import {
  useWorkspaceTradingPrefs,
  useWorkspaceViewState,
} from "../composables/useWorkspaceLayout";
import AiAssistantPanel from "./AiAssistantPanel.vue";
import NotificationCenter from "./NotificationCenter.vue";

const { prefs: tradingPrefs } = useWorkspaceTradingPrefs();
const { prefs, update } = useWorkspaceViewState();
const { currentMarketDataSnapshot: marketDataSnapshot, marketDataSubscriptions, systemStatus } =
  useConsoleData();

const symbolInfo = computed(
  () => `${tradingPrefs.value.market}:${tradingPrefs.value.symbol}`,
);
const snap = computed(() => marketDataSnapshot.value?.snapshot ?? null);
const sessionLabels: Record<string, string> = {
  regular: "盘中",
  pre: "盘前",
  after: "盘后",
  overnight: "夜盘",
  closed: "休市",
  unknown: "未知",
};
const snapSessionLabel = computed(() => {
  const session = snap.value?.session;
  if (typeof session !== "string" || session === "") {
    return "—";
  }
  return sessionLabels[session] ?? session;
});

const tabs = [
  { id: "notifications", label: "通知" },
  { id: "ai", label: "助手" },
  { id: "context", label: "上下文" },
] as const;

function select(id: (typeof tabs)[number]["id"]): void {
  update({ rightDockTab: id, rightDockOpen: true });
}

function toggle(): void {
  update({ rightDockOpen: !prefs.value.rightDockOpen });
}
</script>

<template>
  <aside
    class="tv-rightdock"
    :class="{
      'is-collapsed': !prefs.rightDockOpen,
      'is-ai': prefs.rightDockOpen && prefs.rightDockTab === 'ai',
    }"
  >
    <div v-if="prefs.rightDockOpen" style="display: flex; flex-direction: column; height: 100%; min-height: 0">
      <div class="tv-dock-tabs">
        <div
          v-for="tab in tabs"
          :key="tab.id"
          class="tv-dock-tab"
          :class="{ 'is-active': prefs.rightDockTab === tab.id }"
          :data-testid="`rightdock-tab-${tab.id}`"
          @click="select(tab.id)"
        >
          {{ tab.label }}
        </div>
        <button class="tv-icon-btn" style="width: 36px" title="收起" @click="toggle">⟩</button>
      </div>

      <NotificationCenter v-if="prefs.rightDockTab === 'notifications'" />
      <AiAssistantPanel v-else-if="prefs.rightDockTab === 'ai'" />
      <div v-else class="tv-dock-body">
        <div style="font-size: 11px; color: var(--tv-text-muted); text-transform: uppercase; letter-spacing: 0.08em; margin-bottom: 6px">
          标的
        </div>
        <div data-testid="rightdock-symbol-info" style="font-size: 18px; font-weight: 600; margin-bottom: 8px">{{ symbolInfo }}</div>
        <table class="tv-table">
          <tbody>
            <tr><td>最新价</td><td class="tv-num">{{ snap?.price ?? "—" }}</td></tr>
            <tr><td>买一</td><td class="tv-num">{{ snap?.bid ?? "—" }}</td></tr>
            <tr><td>卖一</td><td class="tv-num">{{ snap?.ask ?? "—" }}</td></tr>
            <tr><td>时段</td><td class="tv-num">{{ snapSessionLabel }}</td></tr>
            <tr><td>成交量</td><td class="tv-num">{{ snap?.volume ?? "—" }}</td></tr>
            <tr><td>成交额</td><td class="tv-num">{{ snap?.turnover ?? "—" }}</td></tr>
            <tr><td>时间</td><td class="tv-num">{{ snap?.at ?? "—" }}</td></tr>
          </tbody>
        </table>

        <div style="font-size: 11px; color: var(--tv-text-muted); text-transform: uppercase; letter-spacing: 0.08em; margin: 14px 0 6px">
          订阅
        </div>
        <div style="font-size: 12px; color: var(--tv-text-muted)">
          {{ marketDataSubscriptions.totalActiveSubscriptions }} 个活跃
          · 配额 {{ marketDataSubscriptions.quota.totalUsed }} / {{ marketDataSubscriptions.quota.totalLimit ?? "∞" }}
        </div>

        <div style="font-size: 11px; color: var(--tv-text-muted); text-transform: uppercase; letter-spacing: 0.08em; margin: 14px 0 6px">
          系统
        </div>
        <div style="font-size: 12px; color: var(--tv-text-muted)">
          {{ systemStatus.message }}
        </div>
      </div>
    </div>
    <button
      v-else
      class="tv-rightdock-toggle"
      title="打开侧栏"
      @click="toggle"
    >⟨</button>
  </aside>
</template>
