<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";

import { desktopFacade } from "@/composables/shared/desktopFacade";

const emit = defineEmits<{ ready: [] }>();

const pollIntervalMs = 200;
const message = ref("正在启动本地服务…");
const failed = ref(false);
const actionError = ref("");
let pollTimer: ReturnType<typeof setTimeout> | null = null;
let stopped = false;

async function refresh(): Promise<void> {
  if (stopped) return;
  try {
    const snapshot = await desktopFacade.startup.snapshot();
    if (stopped) return;
    message.value = snapshot.message || "正在启动本地服务…";
    if (snapshot.state === "ready") {
      emit("ready");
      return;
    }
    if (snapshot.state === "failed") {
      failed.value = true;
      return;
    }
  } catch {
    // The WebView runtime can become callable one frame after Vue mounts.
    message.value = "正在连接桌面启动服务…";
  }
  pollTimer = setTimeout(() => void refresh(), pollIntervalMs);
}

async function openLogs(): Promise<void> {
  actionError.value = "";
  try {
    await desktopFacade.logs.openFolder();
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : String(error);
  }
}

function quit(): void {
  void desktopFacade.startup.quit();
}

onMounted(() => void refresh());
onBeforeUnmount(() => {
  stopped = true;
  if (pollTimer != null) clearTimeout(pollTimer);
});
</script>

<template>
  <main
    class="grid min-h-screen place-items-center bg-slate-950 p-8 text-slate-100"
    aria-live="polite"
  >
    <section
      class="grid w-full max-w-md justify-items-center gap-4 rounded-2xl border border-slate-700/40 bg-slate-900/90 p-10 text-center shadow-2xl"
    >
      <div
        class="grid size-14 place-items-center rounded-xl bg-indigo-600 text-lg font-extrabold text-white"
        aria-hidden="true"
      >
        JF
      </div>
      <h1 class="m-0 text-2xl font-bold">JFTrade</h1>
      <template v-if="!failed">
        <span
          class="size-7 animate-spin rounded-full border-[3px] border-slate-600 border-t-blue-400"
          aria-hidden="true"
        />
        <p class="m-0">{{ message }}</p>
        <small class="text-slate-400">
          窗口已经就绪，行情和本地数据正在后台加载
        </small>
      </template>
      <template v-else>
        <p class="m-0 text-red-300">{{ message }}</p>
        <small class="text-slate-400">
          应用未进入主界面，避免使用不完整的本地服务。
        </small>
        <div class="mt-2 flex gap-3">
          <button
            type="button"
            class="cursor-pointer rounded-lg border border-slate-500 px-4 py-2 text-slate-200"
            @click="openLogs"
          >
            打开日志目录
          </button>
          <button
            type="button"
            class="cursor-pointer rounded-lg border border-blue-500 bg-blue-700 px-4 py-2 text-white"
            @click="quit"
          >
            退出应用
          </button>
        </div>
        <small v-if="actionError" class="text-red-300">
          {{ actionError }}
        </small>
      </template>
    </section>
  </main>
</template>
