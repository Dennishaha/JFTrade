<script setup lang="ts">
defineOptions({ inheritAttrs: false });

type ToolbarActionTone =
  | "neutral"
  | "info"
  | "success"
  | "violet"
  | "danger"
  | "primary";

withDefaults(
  defineProps<{
    icon: string;
    label: string;
    tone?: ToolbarActionTone;
    disabled?: boolean;
    loading?: boolean;
  }>(),
  {
    tone: "neutral",
    disabled: false,
    loading: false,
  },
);

defineEmits<{
  click: [event: MouseEvent];
}>();
</script>

<template>
  <button
    v-bind="$attrs"
    type="button"
    class="tv-toolbar-action"
    :class="[`is-${tone}`, { 'is-loading': loading }]"
    :aria-label="label"
    :aria-busy="loading"
    :disabled="disabled || loading"
    @click="$emit('click', $event)"
  >
    <span class="tv-toolbar-action__surface" aria-hidden="true" />
    <span class="tv-toolbar-action__label" aria-hidden="true">{{ label }}</span>
    <span
      class="tv-toolbar-action__icon"
      :class="loading ? ['fa-solid', 'fa-spinner', 'fa-spin'] : icon.split(' ')"
      aria-hidden="true"
    />
  </button>
</template>

<style scoped>
.tv-toolbar-action {
  --tv-toolbar-action-tone: #64748b;

  position: relative;
  isolation: isolate;
  display: inline-grid;
  width: 34px;
  height: 34px;
  min-width: 34px;
  flex: 0 0 34px;
  place-items: center;
  overflow: visible;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--tv-text);
  cursor: pointer;
  padding: 0;
}

.tv-toolbar-action.is-info {
  --tv-toolbar-action-tone: #0e7490;
}

.tv-toolbar-action.is-success {
  --tv-toolbar-action-tone: #15803d;
}

.tv-toolbar-action.is-violet {
  --tv-toolbar-action-tone: #7c3aed;
}

.tv-toolbar-action.is-danger {
  --tv-toolbar-action-tone: #be123c;
}

.tv-toolbar-action.is-primary {
  --tv-toolbar-action-tone: #0369a1;
}

.tv-toolbar-action__surface {
  position: absolute;
  z-index: 0;
  right: 0;
  bottom: 0;
  left: 0;
  height: 4px;
  border-radius: 5px;
  background: var(--tv-toolbar-action-tone);
  transition: height 160ms ease, box-shadow 160ms ease;
}

.tv-toolbar-action__label {
  position: absolute;
  z-index: 2;
  bottom: 28px;
  left: 50%;
  min-height: 24px;
  transform: translate(-50%, 8px);
  border-radius: 5px 5px 0 0;
  background: var(--tv-toolbar-action-tone);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  line-height: 24px;
  opacity: 0;
  padding: 0 8px;
  pointer-events: none;
  white-space: nowrap;
  transition: opacity 120ms ease, transform 160ms ease;
}

.tv-toolbar-action__icon {
  position: relative;
  z-index: 1;
  font-size: 13px;
  line-height: 1;
}

.tv-toolbar-action:hover,
.tv-toolbar-action:focus-visible {
  color: #fff;
  outline: 0;
}

.tv-toolbar-action:hover .tv-toolbar-action__surface,
.tv-toolbar-action:focus-visible .tv-toolbar-action__surface {
  height: 100%;
  box-shadow: 0 7px 18px color-mix(in srgb, var(--tv-toolbar-action-tone) 30%, transparent);
}

.tv-toolbar-action:hover .tv-toolbar-action__label,
.tv-toolbar-action:focus-visible .tv-toolbar-action__label {
  transform: translate(-50%, 0);
  opacity: 1;
}

.tv-toolbar-action:focus-visible {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--tv-toolbar-action-tone) 40%, transparent);
}

.tv-toolbar-action:disabled {
  color: var(--tv-text-muted);
  cursor: not-allowed;
  opacity: 0.45;
}

.tv-toolbar-action:disabled .tv-toolbar-action__surface {
  height: 3px;
  background: var(--tv-text-muted);
  box-shadow: none;
}

.tv-toolbar-action:disabled .tv-toolbar-action__label {
  background: var(--tv-bg-elevated);
  color: var(--tv-text-muted);
}

@media (prefers-reduced-motion: reduce) {
  .tv-toolbar-action__surface,
  .tv-toolbar-action__label {
    transition: none;
  }
}
</style>
