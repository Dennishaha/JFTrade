<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";

type PineWorkerSettings = {
  workerLimit: number;
};

const MIN_WORKER_LIMIT = 1;
const MAX_WORKER_LIMIT = 1000;

const settings = ref<PineWorkerSettings>({ workerLimit: MIN_WORKER_LIMIT });
const workerLimitInput = ref(MIN_WORKER_LIMIT);
const loading = ref(true);
const saving = ref(false);
const errorMessage = ref("");
const noticeMessage = ref("");

const normalizedWorkerLimit = computed(() =>
  clampWorkerLimit(workerLimitInput.value),
);

onMounted(() => {
  void loadSettings();
});

async function loadSettings(): Promise<void> {
  loading.value = true;
  errorMessage.value = "";
  noticeMessage.value = "";
  try {
    settings.value = await fetchEnvelope<PineWorkerSettings>("/api/v1/settings/pine-worker");
    workerLimitInput.value = clampWorkerLimit(settings.value.workerLimit);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "读取 PineTS Worker 设置失败";
  } finally {
    loading.value = false;
  }
}

async function saveSettings(): Promise<void> {
  if (saving.value) return;
  saving.value = true;
  errorMessage.value = "";
  noticeMessage.value = "";
  try {
    const next = { workerLimit: normalizedWorkerLimit.value };
    settings.value = await fetchEnvelopeWithInit<PineWorkerSettings>("/api/v1/settings/pine-worker", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(next),
    });
    workerLimitInput.value = clampWorkerLimit(settings.value.workerLimit);
    noticeMessage.value = "PineTS Worker 上限已保存。空闲 Worker 会关闭，后续任务按新的上限启动。";
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "保存 PineTS Worker 设置失败";
  } finally {
    saving.value = false;
  }
}

function updateWorkerLimit(event: Event): void {
  const target = event.target as HTMLInputElement | null;
  if (target == null) return;
  workerLimitInput.value = Number(target.value);
}

function clampWorkerLimit(value: number): number {
  if (!Number.isFinite(value)) return MIN_WORKER_LIMIT;
  return Math.min(MAX_WORKER_LIMIT, Math.max(MIN_WORKER_LIMIT, Math.round(value)));
}
</script>

<template>
  <section class="rounded-lg border border-slate-200 bg-white px-5 py-5">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="text-base font-semibold text-slate-900">PineTS Worker</div>
        <div class="mt-1 text-xs text-slate-500">
          控制 Pine Script 计算 Worker 的启动上限。
        </div>
      </div>
      <span class="rounded border border-slate-200 px-2 py-1 text-xs font-medium text-slate-600">
        上限 {{ settings.workerLimit }}
      </span>
    </div>

    <label class="mt-5 grid gap-2 rounded-md border border-slate-200 px-4 py-3">
      <span class="text-sm font-medium text-slate-800">Worker 上限</span>
      <input
        data-testid="pine-worker-limit-input"
        type="number"
        :min="MIN_WORKER_LIMIT"
        :max="MAX_WORKER_LIMIT"
        :value="workerLimitInput"
        :disabled="loading || saving"
        class="w-full rounded-md border border-slate-200 px-3 py-2 text-sm text-slate-700 disabled:cursor-not-allowed disabled:bg-slate-50"
        @input="updateWorkerLimit"
      />
    </label>

    <div class="mt-4 flex flex-wrap items-center justify-between gap-3">
      <div class="text-xs text-slate-500">
        当前将保存为 {{ normalizedWorkerLimit }}，允许范围 {{ MIN_WORKER_LIMIT }} 到 {{ MAX_WORKER_LIMIT }}。
      </div>
      <button
        data-testid="pine-worker-limit-save"
        type="button"
        class="rounded-md bg-slate-900 px-3 py-2 text-xs font-semibold text-white transition hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-50"
        :disabled="loading || saving"
        @click="saveSettings"
      >
        保存设置
      </button>
    </div>

    <p v-if="noticeMessage" class="mt-3 text-xs font-medium text-emerald-700">
      {{ noticeMessage }}
    </p>
    <p v-if="errorMessage" class="mt-3 text-xs font-medium text-rose-600">
      {{ errorMessage }}
    </p>
  </section>
</template>
