<script setup lang="ts">
import { useWorkspaceViewState } from "../composables/useWorkspaceLayout";
import AiAssistantPanel from "./AiAssistantPanel.vue";
import NotificationCenter from "./NotificationCenter.vue";

const { prefs, update } = useWorkspaceViewState();

const tabs = [
  { id: "notifications", label: "通知" },
  { id: "ai", label: "助手" },
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
      'is-ai': prefs.rightDockOpen && prefs.rightDockTab === 'ai',
    }"
  >
    <div style="display: flex; flex-direction: column; height: 100%; min-height: 0">
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
      <AiAssistantPanel v-else />
    </div>
  </aside>
</template>
