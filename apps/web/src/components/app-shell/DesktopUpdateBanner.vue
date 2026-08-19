<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";

import {
  desktopFacade,
  type DesktopUpdateResult,
} from "@/composables/shared/desktopFacade";
import { openExternalUrl } from "@/composables/shared/externalLink";
import { resolveDesktopMode } from "@/runtimeConfig";

const update = ref<DesktopUpdateResult | null>(null);
let cancelListener: (() => void) | null = null;

function openRelease(): void {
  const url = update.value?.releaseUrl?.trim() ?? "";
  if (url) void openExternalUrl(url);
}

onMounted(async () => {
  if (!resolveDesktopMode()) return;
  try {
    const cancel = await desktopFacade.updates.onAvailable((result) => {
      if (result?.available) update.value = result;
    });
    if (cancelListener === stoppedListener) cancel();
    else cancelListener = cancel;
  } catch {
    cancelListener = null;
  }
});

const stoppedListener = () => undefined;
onUnmounted(() => {
  cancelListener?.();
  cancelListener = stoppedListener;
});
</script>

<template>
  <aside v-if="update" class="desktop-update-banner" role="status">
    <span>JFTrade {{ update.latestVersion }} 已发布。</span>
    <button type="button" @click="openRelease">查看版本</button>
    <button type="button" aria-label="关闭更新提示" @click="update = null">
      ×
    </button>
  </aside>
</template>

<style scoped>
.desktop-update-banner {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-height: 36px;
  padding: 6px 12px;
  color: #dbeafe;
  background: #123456;
  border-bottom: 1px solid var(--jf-accent-blue);
}

.desktop-update-banner button {
  border: 1px solid #60a5fa;
  border-radius: 5px;
  padding: 3px 8px;
  color: inherit;
  background: transparent;
}
</style>
