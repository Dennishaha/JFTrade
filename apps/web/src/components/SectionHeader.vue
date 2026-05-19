<script setup lang="ts">
defineProps<{
  title: string;
  description?: string | undefined;
}>();
</script>

<template>
  <div class="flex items-start justify-between gap-3">
    <div>
      <div class="text-xl font-semibold text-slate-900">{{ title }}</div>
      <div v-if="description" class="mt-1 text-sm text-slate-500">
        {{ description }}
      </div>
    </div>
    <div v-if="$slots.extra" class="flex shrink-0 items-center gap-2">
      <slot name="extra" />
    </div>
  </div>
</template>
