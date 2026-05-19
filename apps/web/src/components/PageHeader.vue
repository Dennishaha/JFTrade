<script setup lang="ts">
interface HeaderStat {
  label: string;
  value: string | number;
  hint?: string;
  tone?: string;
}

defineProps<{
  eyebrow?: string;
  title: string;
  description: string;
  stats?: HeaderStat[];
}>();

function resolveValueClass(tone: string | undefined): string {
  switch (tone) {
    case "good":
      return "text-emerald-400";
    case "warn":
      return "text-amber-300";
    case "danger":
      return "text-rose-300";
    default:
      return "text-slate-100";
  }
}
</script>

<template>
  <section class="page-header">
    <div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(320px,0.72fr)]">
      <div class="space-y-3">
        <div
          v-if="eyebrow"
          class="text-[11px] font-semibold uppercase tracking-[0.34em] text-cyan-300"
        >
          {{ eyebrow }}
        </div>
        <div class="space-y-2">
          <h1 class="text-3xl font-semibold tracking-tight text-white md:text-[2.15rem]">
            {{ title }}
          </h1>
          <p class="max-w-4xl text-sm leading-7 text-slate-300 md:text-[15px]">
            {{ description }}
          </p>
        </div>
      </div>

      <div
        v-if="stats?.length"
        class="grid gap-3 sm:grid-cols-2"
      >
        <div
          v-for="stat in stats"
          :key="stat.label"
          class="page-header-stat"
        >
          <div class="text-[11px] uppercase tracking-[0.28em] text-slate-400">
            {{ stat.label }}
          </div>
          <div class="mt-2 text-xl font-semibold" :class="resolveValueClass(stat.tone)">
            {{ stat.value }}
          </div>
          <div v-if="stat.hint" class="mt-2 text-xs leading-5 text-slate-400">
            {{ stat.hint }}
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
