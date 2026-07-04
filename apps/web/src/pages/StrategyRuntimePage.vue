<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";

import StrategyRuntimePanel from "../components/StrategyRuntimePanel.vue";
import { apiGet } from "../composables/apiClient";
import { queryClient, queryKeys } from "../composables/serverState";

type StrategyDesignEntryMode = "existing" | "new";

const router = useRouter();
const route = useRoute();

const strategyDefinitionsCount = ref(0);

const runtimeNotice = computed(() => {
  const value = route.query.notice;
  return typeof value === "string" ? value : "";
});

const pendingStrategyDefinitionId = computed(() => {
  const value = route.query.definitionId;
  return typeof value === "string" ? value : "";
});

onMounted(() => {
  void loadStrategyDefinitionsCount();
});

async function loadStrategyDefinitionsCount(): Promise<void> {
  try {
    const items = await queryClient.ensureQueryData({
      queryKey: queryKeys.strategyDefinitions(),
      queryFn: () => apiGet<Array<{ id: string }>, "/api/v1/strategy-definitions">(
        "/api/v1/strategy-definitions",
      ),
    });
    strategyDefinitionsCount.value = items.length;
  } catch {
    strategyDefinitionsCount.value = 0;
  }
}

function handleSwitchToDesign(payload?: { mode?: StrategyDesignEntryMode }): void {
  if (payload?.mode === "new") {
    void router.push({ path: "/strategy/design", query: { mode: "new" } });
    return;
  }
  void router.push({ path: "/strategy/design" });
}
</script>

<template>
  <div class="strategy-runtime-page">
    <div class="strategy-runtime-page__shell">
      <div v-if="runtimeNotice" class="strategy-banner strategy-banner--success">
        {{ runtimeNotice }}
      </div>
      <StrategyRuntimePanel
        :definitions-count="strategyDefinitionsCount"
        :pending-definition-id="pendingStrategyDefinitionId"
        @switch-to-design="handleSwitchToDesign"
      />
    </div>
  </div>
</template>

<style scoped>
.strategy-runtime-page {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.strategy-runtime-page__shell {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  overflow: hidden;
  padding: 0.75rem;
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
  border: 1px solid color-mix(in srgb, var(--tv-accent) 34%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 14%, transparent);
  color: color-mix(in srgb, var(--tv-accent) 74%, var(--tv-text));
}
</style>
