<script setup lang="ts">
import { ref, watch } from "vue";

import { workflowQueueTone } from "../../composables/useADKWorkflowQueueState";

const props = withDefaults(
  defineProps<{
    title: string;
    count: number;
    status?: string;
    statusLabel?: string;
    summary?: string;
    defaultExpanded?: boolean;
  }>(),
  {
    status: "",
    statusLabel: "",
    summary: "",
    defaultExpanded: false,
  },
);

const expanded = ref(props.defaultExpanded);

watch(
  () => props.count,
  () => {
    expanded.value = props.defaultExpanded;
  },
);
</script>

<template>
  <section class="adk-workspace-queue" :aria-label="title">
    <button
      type="button"
      class="adk-workspace-queue__header"
      :aria-expanded="expanded ? 'true' : 'false'"
      @click="expanded = !expanded"
    >
      <span class="adk-workspace-queue__title">
        <span>{{ title }}</span>
        <span class="adk-workspace-queue__count">{{ count }}</span>
      </span>
      <span v-if="status" class="adk-workspace-queue__badge" :class="`is-${workflowQueueTone(status)}`">
        {{ statusLabel || status }}
      </span>
      <span v-if="summary" class="adk-workspace-queue__summary">{{ summary }}</span>
      <span class="adk-workspace-queue__toggle">
        {{ expanded ? "收起" : "展开" }}
      </span>
    </button>

    <div v-if="expanded" class="adk-workspace-queue__body">
      <slot />
    </div>
  </section>
</template>

<style scoped>
.adk-workspace-queue {
  display: grid;
  gap: 8px;
  box-sizing: border-box;
  width: min(980px, calc(100% - 36px));
  margin: 0 auto 8px;
  padding: 10px 15px;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--tv-bg-surface) 88%, transparent);
  color: var(--tv-text);
  box-shadow: 0 10px 30px rgba(2, 6, 23, 0.18);
}

.adk-workspace-queue__header {
  display: grid;
  grid-template-columns: auto auto minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
  width: 100%;
  border: 0;
  padding: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.adk-workspace-queue__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: 12px;
  font-weight: 700;
}

.adk-workspace-queue__count {
  min-width: 20px;
  padding: 1px 6px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-text-dim) 18%, transparent);
  color: var(--tv-text-muted);
  font-size: 11px;
  line-height: 1.5;
  text-align: center;
}

.adk-workspace-queue__badge {
  justify-self: start;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  line-height: 1.4;
  text-transform: uppercase;
}

.adk-workspace-queue__badge.is-muted {
  color: var(--adk-muted-fg);
  background: var(--adk-muted-bg);
}

.adk-workspace-queue__badge.is-info {
  color: var(--adk-info-fg);
  background: var(--adk-info-bg);
}

.adk-workspace-queue__badge.is-warning {
  color: var(--adk-warning-fg);
  background: var(--adk-warning-bg);
}

.adk-workspace-queue__badge.is-success {
  color: var(--adk-success-fg);
  background: var(--adk-success-bg);
}

.adk-workspace-queue__badge.is-error {
  color: var(--adk-error-fg);
  background: var(--adk-error-bg);
}

.adk-workspace-queue__summary {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--tv-text-muted);
  font-size: 12px;
}

.adk-workspace-queue__toggle {
  color: var(--adk-accent-fg);
  font-size: 12px;
}

.adk-workspace-queue__body {
  display: grid;
  gap: 8px;
  max-height: 430px;
  overflow: auto;
  padding-right: 2px;
}

:deep(.adk-workspace-queue-list) {
  display: grid;
  gap: 8px;
}

:deep(.adk-workspace-queue-item) {
  display: grid;
  grid-template-columns: auto auto minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
  min-height: 34px;
  min-width: 0;
  padding: 7px 8px;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 76%, transparent);
}

:deep(.adk-workspace-queue-item__index) {
  color: var(--tv-text-dim);
  font-size: 12px;
  white-space: nowrap;
}

:deep(.adk-workspace-queue-item__main) {
  min-width: 0;
}

:deep(.adk-workspace-queue-item__title) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 600;
}

:deep(.adk-workspace-queue-item__meta) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--tv-text-muted);
  font-size: 12px;
}

:deep(.adk-workspace-queue-item__actions) {
  display: inline-flex;
  gap: 6px;
  justify-content: flex-end;
}

:deep(.adk-workspace-queue-status) {
  justify-self: start;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  line-height: 1.4;
  text-transform: uppercase;
}

:deep(.adk-workspace-queue-status.is-muted) {
  color: var(--adk-muted-fg);
  background: var(--adk-muted-bg);
}

:deep(.adk-workspace-queue-status.is-info) {
  color: var(--adk-info-fg);
  background: var(--adk-info-bg);
}

:deep(.adk-workspace-queue-status.is-warning) {
  color: var(--adk-warning-fg);
  background: var(--adk-warning-bg);
}

:deep(.adk-workspace-queue-status.is-success) {
  color: var(--adk-success-fg);
  background: var(--adk-success-bg);
}

:deep(.adk-workspace-queue-status.is-error) {
  color: var(--adk-error-fg);
  background: var(--adk-error-bg);
}

:deep(.adk-workspace-queue-button) {
  border: 0;
  border-radius: 999px;
  padding: 4px 8px;
  background: var(--adk-accent-bg);
  color: var(--adk-accent-fg);
  cursor: pointer;
  font-size: 12px;
}

:deep(.adk-workspace-queue-button.is-danger) {
  background: var(--adk-danger-bg);
  color: var(--adk-danger-fg);
}

:deep(.adk-workspace-queue-button:disabled) {
  cursor: default;
  opacity: 0.5;
}

@media (max-width: 720px) {
  .adk-workspace-queue {
    width: calc(100% - 24px);
  }

  .adk-workspace-queue__header {
    grid-template-columns: auto auto auto;
  }

  .adk-workspace-queue__summary {
    grid-column: 1 / -1;
  }

  :deep(.adk-workspace-queue-item) {
    grid-template-columns: auto auto minmax(0, 1fr);
  }

  :deep(.adk-workspace-queue-item__actions) {
    grid-column: 1 / -1;
    justify-content: flex-start;
  }
}
</style>
