<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

import type { ADKAgent, ADKProvider } from "@/contracts";

import ADKWorkspaceShell from "../components/adk-page/ADKWorkspaceShell.vue";
import { fetchADKPageSessionData } from "../composables/adkPageSessionApi";
import { formatLocalDateTime } from "../utils/dateTime";

type ADKPageView = "agents" | "workflows";

const ADKWorkflowStudio = defineAsyncComponent(
  () => import("../components/adk-page/ADKWorkflowStudio.vue"),
);

const route = useRoute();
const router = useRouter();
const agents = ref<ADKAgent[]>([]);
const providers = ref<ADKProvider[]>([]);
const workflowResourcesLoaded = ref(false);
const workflowResourcesLoading = ref(false);
const workflowResourcesError = ref("");

const viewOptions = [
  { title: "智能体", value: "agents" },
  { title: "工作流", value: "workflows" },
];
const activeView = computed<ADKPageView>(() =>
  route.path === "/adk/workflows" ? "workflows" : "agents",
);
const activePageTab = computed<ADKPageView>({
  get: () => activeView.value,
  set(value) {
    if (value !== "agents" && value !== "workflows") return;
    if (value === activeView.value) return;
    void router.push({ path: `/adk/${value}`, query: { ...route.query } });
  },
});

function formatDateTime(value: string): string {
  return formatLocalDateTime(value, "-");
}

async function ensureWorkflowResources(): Promise<void> {
  if (workflowResourcesLoaded.value || workflowResourcesLoading.value) return;
  workflowResourcesLoading.value = true;
  workflowResourcesError.value = "";
  try {
    const data = await fetchADKPageSessionData();
    agents.value = data.agents;
    providers.value = data.providers;
    workflowResourcesLoaded.value = true;
  } catch (error) {
    workflowResourcesError.value =
      error instanceof Error ? error.message : "加载工作流资源失败";
  } finally {
    workflowResourcesLoading.value = false;
  }
}

watch(
  activeView,
  (view) => {
    if (view === "workflows") {
      void ensureWorkflowResources();
    }
  },
  { immediate: true },
);

onMounted(() => {
  if (activeView.value === "workflows") {
    void ensureWorkflowResources();
  }
});
</script>

<template>
  <div class="adk-page">
    <div class="adk-page__tabs">
      <v-tabs v-model="activePageTab" density="compact">
        <v-tab
          v-for="item in viewOptions"
          :key="item.value"
          :value="item.value"
        >
          {{ item.title }}
        </v-tab>
      </v-tabs>
    </div>

    <ADKWorkspaceShell v-if="activeView === 'agents'" layout="desktop" />

    <section v-else class="adk-page__workflow">
      <v-alert
        v-if="workflowResourcesError"
        type="warning"
        variant="tonal"
        density="compact"
      >
        {{ workflowResourcesError }}
      </v-alert>
      <v-progress-linear
        v-if="workflowResourcesLoading && !workflowResourcesLoaded"
        indeterminate
        color="primary"
      />
      <ADKWorkflowStudio
        v-else
        :agents="agents"
        :providers="providers"
        :format-date-time="formatDateTime"
        view-mode="workflows"
      />
    </section>
  </div>
</template>

<style scoped>
.adk-page {
  display: flex;
  min-height: 0;
  height: 100%;
  flex-direction: column;
  background: var(--tv-bg-app);
}

.adk-page__tabs {
  flex: 0 0 auto;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
  padding: 0 12px;
}

.adk-page__workflow {
  flex: 1 1 auto;
  min-height: 0;
  overflow: hidden;
}
</style>
