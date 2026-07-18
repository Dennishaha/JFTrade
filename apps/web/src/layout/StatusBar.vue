<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";

import {
  formatGenericStatusLabel,
  formatTradingEnvironment,
} from "../composables/consoleDataFormatting";
import MarketStatusBadge from "../components/domain/market-data/MarketStatusBadge.vue";
import { useConsoleData } from "../composables/useConsoleData";
import { useSharedLiveStream } from "../composables/useSharedLiveStream";
import { formatLocalDateTime } from "../utils/dateTime";

const {
  selectedBrokerAccount,
  systemStatus,
  liveStreamCheckedAt,
  consoleRefreshError,
  realTradeKillSwitchState,
} = useConsoleData();
const { connectionState, lastHeartbeat } = useSharedLiveStream();

const now = ref(new Date());
let timer: ReturnType<typeof setInterval> | null = null;
const localClockFormatter = new Intl.DateTimeFormat(undefined, {
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

onMounted(() => {
  timer = setInterval(() => {
    now.value = new Date();
  }, 1000);
});

onUnmounted(() => {
  if (timer) clearInterval(timer);
});

const clock = computed(() => localClockFormatter.format(now.value));
const killActive = computed(
  () => realTradeKillSwitchState.value.killSwitchActive,
);
const consoleRefreshIssue = computed(
  () =>
    connectionState.value === "connected" &&
    consoleRefreshError.value.trim() !== "",
);
const consoleRefreshIssueTitle = computed(() =>
  [
    "实时通道已连接，但控制台后台刷新失败",
    `错误：${consoleRefreshError.value.trim()}`,
    `最近控制台刷新：${formatLocalDateTime(liveStreamCheckedAt.value, "—")}`,
    `最近实时心跳：${formatLocalDateTime(lastHeartbeat.value, "—")}`,
  ].join("\n"),
);
</script>

<template>
  <footer class="tv-statusbar">
    <MarketStatusBadge
      v-if="consoleRefreshIssue"
      data-testid="console-refresh-issue"
      state="stale"
      label="控制台刷新异常"
      :title="consoleRefreshIssueTitle"
    />
    <span>
      <span class="tv-status-dot" :class="killActive ? 'tv-dot-err' : 'tv-dot-ok'"></span>
      风控状态 {{ killActive ? "已激活" : "正常" }}
    </span>
    <span>存储：{{ systemStatus.persistence.engine }} / {{ formatGenericStatusLabel(systemStatus.persistence.status) }}</span>
    <span style="flex: 1"></span>
    <span>
      选定账户：
      {{
        selectedBrokerAccount
          ? `${selectedBrokerAccount.brokerId}/${selectedBrokerAccount.accountId}/${formatTradingEnvironment(selectedBrokerAccount.tradingEnvironment)}`
          : `${systemStatus.broker.displayName}/${formatTradingEnvironment(systemStatus.defaultTradingEnvironment)}`
      }}
    </span>
    <span style="font-variant-numeric: tabular-nums">{{ clock }} 本地时间</span>
    <span
      id="workspace-provider-statusbar"
      class="tv-statusbar__provider"
      aria-label="行情提供者"
    />
  </footer>
</template>
