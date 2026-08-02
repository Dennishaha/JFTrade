<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    active?: boolean;
    disabled?: boolean;
    title?: string;
    ariaLabel?: string;
    testId?: string;
  }>(),
  {
    active: false,
    disabled: false,
    title: "",
    ariaLabel: "",
    testId: "",
  },
);

defineEmits<{
  click: [event: MouseEvent];
}>();

const accessibleLabel = computed(
  () =>
    props.ariaLabel.trim() ||
    props.title.trim() ||
    (props.active ? "管理自选分组" : "加入自选"),
);
</script>

<template>
  <button
    type="button"
    class="watchlist-favorite-button"
    :class="{ 'is-active': active }"
    :disabled="disabled"
    :title="title || accessibleLabel"
    :aria-label="accessibleLabel"
    :data-testid="testId || undefined"
    @click="$emit('click', $event)"
  >
    <span aria-hidden="true">{{ active ? "★" : "☆" }}</span>
  </button>
</template>

<style scoped>
.watchlist-favorite-button {
  display: grid;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  place-items: center;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--tv-text-dim);
  font: inherit;
  font-size: var(--jf-text-12);
  line-height: 1;
  cursor: pointer;
  transition:
    background-color 120ms ease,
    color 120ms ease,
    box-shadow 120ms ease;
}

.watchlist-favorite-button:hover:not(:disabled),
.watchlist-favorite-button.is-active {
  background: color-mix(in srgb, var(--jf-accent-yellow) 12%, transparent);
  color: var(--jf-accent-yellow);
}

.watchlist-favorite-button:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--jf-accent-yellow) 35%, transparent);
}

.watchlist-favorite-button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}
</style>
