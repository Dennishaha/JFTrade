<script setup lang="ts">
import { onMounted, ref } from "vue";

import StrategyDesignStage from "../components/StrategyDesignStage.vue";
import StrategyRuntimePanel from "../components/StrategyRuntimePanel.vue";
import { fetchEnvelope } from "../composables/apiClient";

type StrategyWorkspaceTab = "design" | "runtime";
type StrategyDesignEntryMode = "existing" | "new";

const strategyWorkspaceTab = ref<StrategyWorkspaceTab>("runtime");
const strategyDesignEntryMode = ref<StrategyDesignEntryMode>("existing");
const strategyDesignSessionKey = ref(0);
const strategyDefinitionsCount = ref(0);
const runtimeNotice = ref("");

onMounted(() => {
  void loadStrategyDefinitionsCount();
});

async function loadStrategyDefinitionsCount(): Promise<void> {
  try {
    const items = await fetchEnvelope<Array<{ id: string }>>("/api/v1/strategy-definitions");
    strategyDefinitionsCount.value = items.length;
  } catch {
    strategyDefinitionsCount.value = 0;
  }
}

function handleSwitchToRuntime(payload?: { notice?: string }): void {
  runtimeNotice.value = payload?.notice ?? "";
  strategyWorkspaceTab.value = "runtime";
}

function handleSwitchToDesign(payload?: { mode?: StrategyDesignEntryMode }): void {
  runtimeNotice.value = "";
  strategyDesignEntryMode.value = payload?.mode ?? "existing";
  strategyDesignSessionKey.value += 1;
  strategyWorkspaceTab.value = "design";
}

function handleDefinitionsCountChange(count: number): void {
  strategyDefinitionsCount.value = count;
}
</script>

<template>
  <div class="strategy-page">
    <StrategyDesignStage
      v-if="strategyWorkspaceTab === 'design'"
      :key="strategyDesignSessionKey"
      :entry-mode="strategyDesignEntryMode"
      :initial-definitions-collapsed="true"
      @definitions-count-change="handleDefinitionsCountChange"
      @switch-to-runtime="handleSwitchToRuntime"
    />

    <div v-else class="strategy-page__shell">
      <div v-if="runtimeNotice" class="strategy-banner strategy-banner--success">
        {{ runtimeNotice }}
      </div>

      <StrategyRuntimePanel
        :definitions-count="strategyDefinitionsCount"
        @switch-to-design="handleSwitchToDesign"
      />
    </div>
  </div>
</template>

<style scoped>
.strategy-page {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.strategy-page__shell {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  overflow: hidden;
  padding: 0.75rem;
  border-radius: 2rem;
  border: 1px solid var(--tv-border);
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--tv-bg-surface) 96%, transparent) 0%,
    color-mix(in srgb, var(--tv-bg-surface-2) 92%, transparent) 100%
  );
  box-shadow: 0 24px 90px rgba(2, 6, 23, 0.24);
}

.strategy-banner {
  flex-shrink: 0;
  padding: 0.85rem 1rem;
  border-radius: 1.5rem;
  font-size: 0.92rem;
  line-height: 1.5;
}

.strategy-banner--success {
  border: 1px solid color-mix(in srgb, var(--tv-up) 34%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-up) 14%, transparent);
  color: color-mix(in srgb, var(--tv-up) 74%, var(--tv-text));
}
</style>
