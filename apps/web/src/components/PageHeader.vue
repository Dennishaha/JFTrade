<script setup lang="ts">
type HeaderStatTone = "good" | "warn" | "danger";

interface HeaderStat {
  label: string;
  value: string | number;
  hint?: string;
  tone?: HeaderStatTone;
}

defineProps<{
  eyebrow?: string;
  title: string;
  description: string;
  stats?: HeaderStat[];
}>();
</script>

<template>
  <section class="page-header">
    <div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(320px,0.72fr)]">
      <div class="space-y-3">
        <div
          v-if="eyebrow"
          class="text-[11px] font-semibold uppercase tracking-[0.34em] text-[var(--tv-accent)]"
        >
          {{ eyebrow }}
        </div>
        <div class="space-y-2">
          <h1 class="text-3xl font-semibold tracking-tight text-[var(--tv-text)] md:text-[2.15rem]">
            {{ title }}
          </h1>
          <p class="max-w-4xl text-sm leading-7 text-[var(--tv-text-muted)] md:text-[15px]">
            {{ description }}
          </p>
        </div>
      </div>

      <div
        v-if="stats?.length"
        class="grid gap-3 sm:grid-cols-4"
      >
        <div
          v-for="stat in stats"
          :key="stat.label"
          class="page-header-stat"
          :class="stat.tone ? `page-header-stat--${stat.tone}` : undefined"
          :data-tone="stat.tone"
        >
          <div class="text-[11px] uppercase tracking-[0.28em] text-[var(--tv-text-dim)]">
            {{ stat.label }}
          </div>
          <div class="page-header-stat__value mt-2 text-xl font-semibold">
            {{ stat.value }}
          </div>
          <div v-if="stat.hint" class="mt-2 text-xs leading-5 text-[var(--tv-text-dim)]">
            {{ stat.hint }}
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
