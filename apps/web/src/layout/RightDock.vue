<script setup lang="ts">
import AppTabs from "@/components/shared/AppTabs.vue";
import { useWorkspaceViewState } from "@/composables/workspace/useWorkspaceLayout";
import AiAssistantPanel from "./AiAssistantPanel.vue";
import NotificationCenter from "./NotificationCenter.vue";

const { prefs, update } = useWorkspaceViewState();

const tabs = [
  { value: "notifications", label: "通知", testId: "rightdock-tab-notifications" },
  { value: "ai", label: "助手", testId: "rightdock-tab-ai" },
] as const;

function select(value: string): void {
  if (value !== "notifications" && value !== "ai") return;
  update({ rightDockTab: value, rightDockOpen: true });
}

function toggle(): void {
  update({ rightDockOpen: !prefs.value.rightDockOpen });
}
</script>

<template>
  <aside
    class="tv-rightdock"
    :class="{
      'is-ai': prefs.rightDockOpen && prefs.rightDockTab === 'ai',
    }"
  >
    <div class="flex h-full min-h-0 flex-col">
      <div class="tv-dock-navigation">
        <AppTabs
          class="tv-dock-tabs"
          variant="compact"
          fill
          :model-value="prefs.rightDockTab"
          :items="tabs"
          label="右侧停靠栏视图"
          @update:model-value="select"
        />
        <button class="tv-icon-btn jf-icon-btn-wide" title="收起" @click="toggle">⟩</button>
      </div>

      <NotificationCenter v-if="prefs.rightDockTab === 'notifications'" />
      <AiAssistantPanel v-else />
    </div>
  </aside>
</template>
