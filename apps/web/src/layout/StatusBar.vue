<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";

import {
  formatConnectivityLabel,
  formatGenericStatusLabel,
  formatTradingEnvironment,
} from "../composables/consoleDataFormatting";
import { useConsoleData } from "../composables/useConsoleData";
import { useSharedLiveSocket } from "../composables/useSharedLiveSocket";

const {
  selectedBrokerAccount,
  systemStatus,
  liveStreamStatus,
  liveStreamCheckedAt,
  realTradeKillSwitchState,
} = useConsoleData();
const { connectionState, lastHeartbeat } = useSharedLiveSocket();

const now = ref(new Date());
let timer: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  timer = setInterval(() => {
    now.value = new Date();
  }, 1000);
});

onUnmounted(() => {
  if (timer) clearInterval(timer);
});

const clock = computed(() => now.value.toISOString().substring(11, 19));
const killActive = computed(
  () => realTradeKillSwitchState.value.killSwitchActive,
);
</script>

<template>
  <footer class="tv-statusbar">
    <span>
      <span class="tv-status-dot" :class="liveStreamStatus === 'connected' ? 'tv-dot-ok' : 'tv-dot-idle'"></span>
      事件流 {{ formatConnectivityLabel(liveStreamStatus) }}
    </span>
    <span style="color: var(--tv-text-dim)">{{ liveStreamCheckedAt || "—" }}</span>
    <span>
      <span class="tv-status-dot" :class="connectionState === 'connected' ? 'tv-dot-ok' : connectionState === 'error' ? 'tv-dot-err' : 'tv-dot-idle'"></span>
      实时通道 {{ formatConnectivityLabel(connectionState) }}
    </span>
    <span style="color: var(--tv-text-dim)">{{ lastHeartbeat || "—" }}</span>
    <span>
      <span class="tv-status-dot" :class="killActive ? 'tv-dot-err' : 'tv-dot-ok'"></span>
      交易总闸 {{ killActive ? "已激活" : "正常" }}
    </span>
    <span>存储：{{ systemStatus.persistence.engine }} / {{ formatGenericStatusLabel(systemStatus.persistence.status) }}</span>
    <span style="flex: 1"></span>
    <span>
      账户范围：
      {{
        selectedBrokerAccount
          ? `${selectedBrokerAccount.brokerId}/${selectedBrokerAccount.accountId}/${formatTradingEnvironment(selectedBrokerAccount.tradingEnvironment)}`
          : `${systemStatus.broker.displayName}/${formatTradingEnvironment(systemStatus.defaultTradingEnvironment)}`
      }}
    </span>
    <span style="font-variant-numeric: tabular-nums">{{ clock }} 协调时</span>
  </footer>
</template>
