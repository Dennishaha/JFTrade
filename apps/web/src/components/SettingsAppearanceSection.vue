<script setup lang="ts">
import { computed, type CSSProperties } from "vue";

import { useUIColorPreferences } from "../composables/useUIColorPreferences";

const { prefs, resolved, reset, update } = useUIColorPreferences();

const previewVars = computed<CSSProperties>(() => ({
  "--tv-price-up": resolved.value.upColor,
  "--tv-price-down": resolved.value.downColor,
}) as CSSProperties);

function updateUpColor(event: Event): void {
  const target = event.target as HTMLInputElement | null;
  if (target == null) return;
  update({ upColor: target.value });
}

function updateDownColor(event: Event): void {
  const target = event.target as HTMLInputElement | null;
  if (target == null) return;
  update({ downColor: target.value });
}
</script>

<template>
  <section class="rounded-lg border border-slate-200 bg-white px-5 py-5">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="text-base font-semibold text-slate-900">市场涨跌色</div>
        <div class="mt-1 text-xs text-slate-500">应用于 K 线、价格涨跌与下单买卖颜色。</div>
      </div>
      <button type="button" class="rounded-md border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 transition hover:bg-slate-50" @click="reset">
        重置默认
      </button>
    </div>

    <div class="mt-5 grid gap-4 md:grid-cols-2">
      <label class="grid gap-2 rounded-md border border-slate-200 px-4 py-3">
        <span class="text-sm font-medium text-slate-700">上涨 / 买入</span>
        <span class="flex items-center gap-3">
          <input :value="prefs.upColor" type="color" class="h-9 w-12 cursor-pointer rounded border border-slate-200 bg-white p-1" @input="updateUpColor" />
          <input :value="prefs.upColor" type="text" class="min-w-0 flex-1 rounded-md border border-slate-200 px-3 py-2 font-mono text-sm text-slate-700" @input="updateUpColor" />
        </span>
      </label>

      <label class="grid gap-2 rounded-md border border-slate-200 px-4 py-3">
        <span class="text-sm font-medium text-slate-700">下跌 / 卖出</span>
        <span class="flex items-center gap-3">
          <input :value="prefs.downColor" type="color" class="h-9 w-12 cursor-pointer rounded border border-slate-200 bg-white p-1" @input="updateDownColor" />
          <input :value="prefs.downColor" type="text" class="min-w-0 flex-1 rounded-md border border-slate-200 px-3 py-2 font-mono text-sm text-slate-700" @input="updateDownColor" />
        </span>
      </label>
    </div>

    <div class="mt-5 grid gap-3 rounded-md border border-slate-200 bg-slate-50 px-4 py-4" :style="previewVars">
      <div class="flex flex-wrap items-center gap-3 text-sm font-semibold">
        <span class="tv-up">+2.16%</span>
        <span class="tv-down">-1.08%</span>
        <span class="text-slate-500">状态色不跟随这里变化</span>
      </div>
      <div class="grid gap-2 sm:grid-cols-4">
        <div class="rounded border px-3 py-2 text-sm font-semibold text-white" style="border-color: var(--tv-price-up); background: var(--tv-price-up)">买入按钮</div>
        <div class="rounded border px-3 py-2 text-sm font-semibold text-white" style="border-color: var(--tv-price-down); background: var(--tv-price-down)">卖出按钮</div>
      </div>
    </div>
  </section>
</template>
