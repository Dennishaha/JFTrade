<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";

import StrategyDesignStage from "../components/StrategyDesignStage.vue";

type StrategyDesignEntryMode = "existing" | "new";

const route = useRoute();
const router = useRouter();

const entryMode = computed<StrategyDesignEntryMode>(() =>
  route.query.mode === "new" ? "new" : "existing",
);

function handleSwitchToRuntime(payload?: { notice?: string; definitionId?: string }): void {
  void router.push({
    path: "/strategy/runtime",
    query: {
      ...(payload?.notice ? { notice: payload.notice } : {}),
      ...(payload?.definitionId ? { definitionId: payload.definitionId } : {}),
    },
  });
}
</script>

<template>
  <StrategyDesignStage
    :key="entryMode"
    :entry-mode="entryMode"
    :initial-definitions-collapsed="true"
    @switch-to-runtime="handleSwitchToRuntime"
  />
</template>
