<script setup lang="ts">
export interface SegmentedControlItem {
  value: string;
  label: string;
  icon?: string;
  count?: number | string;
  disabled?: boolean;
  testId?: string;
}

const props = withDefaults(
  defineProps<{
    modelValue: string;
    items: readonly SegmentedControlItem[];
    label: string;
    size?: "small" | "medium";
  }>(),
  { size: "medium" },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

function select(item: SegmentedControlItem): void {
  if (!item.disabled && item.value !== props.modelValue) {
    emit("update:modelValue", item.value);
  }
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
  const group = (event.currentTarget as HTMLElement).closest<HTMLElement>('[role="group"]');
  const buttons = Array.from(group?.querySelectorAll<HTMLButtonElement>("button:not(:disabled)") ?? []);
  const currentIndex = buttons.indexOf(event.currentTarget as HTMLButtonElement);
  if (currentIndex < 0 || buttons.length === 0) return;

  event.preventDefault();
  const delta = event.key === "ArrowRight" ? 1 : -1;
  const next = buttons[(currentIndex + delta + buttons.length) % buttons.length];
  const value = next?.dataset.segmentValue;
  if (value !== undefined) emit("update:modelValue", value);
  next?.focus();
}
</script>

<template>
  <div class="segmented-control" :class="`segmented-control--${size}`" role="group" :aria-label="label">
    <button
      v-for="item in items"
      :key="item.value"
      type="button"
      class="segmented-control__item"
      :class="{ 'is-active': item.value === modelValue }"
      :aria-pressed="item.value === modelValue"
      :disabled="item.disabled"
      :data-segment-value="item.value"
      :data-testid="item.testId"
      @click="select(item)"
      @keydown="onKeydown"
    >
      <i v-if="item.icon" :class="item.icon" aria-hidden="true" />
      <span>{{ item.label }}</span>
      <span v-if="item.count !== undefined" class="segmented-control__count">{{ item.count }}</span>
    </button>
  </div>
</template>

<style>
.segmented-control {
  display: inline-flex;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--tv-border);
  border-radius: var(--jf-radius-lg);
  background: var(--tv-bg-surface-2);
}

.segmented-control__item {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--jf-space-1);
  min-height: var(--jf-control-height-md);
  padding: 0 var(--jf-space-3);
  border: 0;
  border-radius: var(--jf-radius-md);
  color: var(--tv-text-muted);
  background: transparent;
  font: inherit;
  font-size: var(--jf-text-6);
  white-space: nowrap;
  cursor: pointer;
}

.segmented-control--small .segmented-control__item {
  min-height: var(--jf-control-height-sm);
  padding-inline: var(--jf-space-2);
  font-size: var(--jf-text-5);
}

.segmented-control__item:hover:not(:disabled) {
  color: var(--tv-text);
  background: var(--tv-bg-hover);
}

.segmented-control__item[aria-pressed="true"] {
  color: var(--tv-accent);
  background: var(--tv-bg-elevated);
}

.segmented-control__item:focus-visible {
  outline: 2px solid var(--tv-accent);
  outline-offset: -2px;
}

.segmented-control__item:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.segmented-control__count {
  min-width: 1.4em;
  padding: 0 var(--jf-space-1);
  border-radius: var(--jf-radius-pill);
  background: var(--tv-bg-elevated);
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
}
</style>
