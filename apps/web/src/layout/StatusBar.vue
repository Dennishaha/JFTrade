<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";

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
      SSE {{ liveStreamStatus }}
    </span>
    <span style="color: var(--tv-text-dim)">{{ liveStreamCheckedAt || "—" }}</span>
    <span>
      <span class="tv-status-dot" :class="connectionState === 'connected' ? 'tv-dot-ok' : connectionState === 'error' ? 'tv-dot-err' : 'tv-dot-idle'"></span>
      WS {{ connectionState }}
    </span>
    <span style="color: var(--tv-text-dim)">{{ lastHeartbeat || "—" }}</span>
    <span>
      <span class="tv-status-dot" :class="killActive ? 'tv-dot-err' : 'tv-dot-ok'"></span>
      Kill switch {{ killActive ? "ACTIVE" : "clear" }}
    </span>
    <span>Persistence: {{ systemStatus.persistence.engine }}</span>
    <span style="flex: 1"></span>
    <span>
      Scope:
      {{
        selectedBrokerAccount
          ? `${selectedBrokerAccount.brokerId}/${selectedBrokerAccount.accountId}/${selectedBrokerAccount.tradingEnvironment}`
          : `${systemStatus.broker.displayName}/${systemStatus.defaultTradingEnvironment}`
      }}
    </span>
    <span style="font-variant-numeric: tabular-nums">{{ clock }} UTC</span>
  </footer>
</template>
