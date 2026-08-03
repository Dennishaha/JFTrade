<script setup lang="ts">
export type AppTabsVariant = "page" | "section" | "compact";

export interface AppTabItem {
  value: string;
  label: string;
  icon?: string;
  count?: number | string;
  disabled?: boolean;
  testId?: string;
  surfaceId?: string;
  tabId?: string;
  panelId?: string;
}

const props = withDefaults(
  defineProps<{
    modelValue: string;
    items: readonly AppTabItem[];
    label: string;
    variant?: AppTabsVariant;
    fill?: boolean;
  }>(),
  {
    variant: "section",
    fill: false,
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

function select(item: AppTabItem): void {
  if (!item.disabled && item.value !== props.modelValue) {
    emit("update:modelValue", item.value);
  }
}

function enabledTabs(currentTarget: EventTarget | null): HTMLButtonElement[] {
  const tablist = (currentTarget as HTMLElement | null)?.closest<HTMLElement>('[role="tablist"]');
  return Array.from(tablist?.querySelectorAll<HTMLButtonElement>('[role="tab"]:not(:disabled)') ?? []);
}

function onKeydown(event: KeyboardEvent): void {
  const tabs = enabledTabs(event.currentTarget);
  const currentIndex = tabs.indexOf(event.currentTarget as HTMLButtonElement);
  if (currentIndex < 0 || tabs.length === 0) return;

  let nextIndex: number | undefined;
  if (event.key === "ArrowRight") nextIndex = (currentIndex + 1) % tabs.length;
  if (event.key === "ArrowLeft") nextIndex = (currentIndex - 1 + tabs.length) % tabs.length;
  if (event.key === "Home") nextIndex = 0;
  if (event.key === "End") nextIndex = tabs.length - 1;
  if (nextIndex === undefined) return;

  event.preventDefault();
  const nextTab = tabs[nextIndex];
  const value = nextTab?.dataset.tabValue;
  if (value !== undefined) emit("update:modelValue", value);
  nextTab?.focus();
}
</script>

<template>
  <div
    class="app-tabs"
    :class="[`app-tabs--${variant}`, { 'app-tabs--fill': fill }]"
    role="tablist"
    :aria-label="label"
  >
    <button
      v-for="item in items"
      :id="item.tabId"
      :key="item.value"
      type="button"
      class="app-tabs__tab"
      :class="{ 'is-active': item.value === modelValue }"
      role="tab"
      :aria-controls="item.panelId"
      :aria-selected="item.value === modelValue"
      :tabindex="item.value === modelValue ? 0 : -1"
      :disabled="item.disabled"
      :data-tab-value="item.value"
      :data-testid="item.testId"
      :data-capability-surface="item.surfaceId"
      @click="select(item)"
      @keydown="onKeydown"
    >
      <slot name="item" :item="item" :active="item.value === modelValue">
        <i v-if="item.icon" :class="item.icon" aria-hidden="true" />
        <span class="app-tabs__label">{{ item.label }}</span>
        <span v-if="item.count !== undefined" class="app-tabs__count">{{ item.count }}</span>
      </slot>
    </button>
  </div>
</template>

<style>
.app-tabs {
  display: flex;
  min-width: 0;
  overflow-x: auto;
  color: var(--tv-text-muted);
  scrollbar-width: none;
}

.app-tabs::-webkit-scrollbar {
  display: none;
}

.app-tabs__tab {
  position: relative;
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  gap: var(--jf-space-2);
  min-height: var(--jf-control-height-md);
  padding: 0 var(--jf-space-3);
  border: 0;
  color: inherit;
  background: transparent;
  font: inherit;
  font-size: var(--jf-text-6);
  white-space: nowrap;
  cursor: pointer;
}

.app-tabs__tab:hover:not(:disabled) {
  color: var(--tv-text);
  background: var(--tv-bg-hover);
}

.app-tabs__tab[aria-selected="true"] {
  color: var(--tv-accent);
}

.app-tabs__tab:focus-visible {
  z-index: 1;
  outline: 2px solid var(--tv-accent);
  outline-offset: -2px;
}

.app-tabs__tab:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.app-tabs__tab[aria-selected="true"]::after {
  position: absolute;
  right: var(--jf-space-2);
  bottom: 0;
  left: var(--jf-space-2);
  height: 2px;
  border-radius: var(--jf-radius-pill);
  background: var(--tv-accent);
  content: "";
}

.app-tabs--page {
  gap: var(--jf-space-1);
  padding: var(--jf-space-1);
  border: 1px solid var(--tv-border);
  border-radius: var(--jf-radius-lg);
  background: var(--tv-bg-surface-2);
}

.app-tabs--page .app-tabs__tab {
  border-radius: var(--jf-radius-md);
}

.app-tabs--page .app-tabs__tab[aria-selected="true"] {
  background: var(--tv-bg-elevated);
}

.app-tabs--compact {
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--tv-border);
  border-radius: var(--jf-radius-md);
  background: var(--tv-bg-surface-2);
}

.app-tabs--compact .app-tabs__tab {
  min-height: var(--jf-control-height-sm);
  padding-inline: var(--jf-space-2);
  border-radius: var(--jf-radius-xs);
}

.app-tabs--compact .app-tabs__tab[aria-selected="true"] {
  background: var(--tv-bg-elevated);
}

.app-tabs--page .app-tabs__tab::after,
.app-tabs--compact .app-tabs__tab::after {
  display: none;
}

.app-tabs--fill .app-tabs__tab {
  flex: 1 1 0;
}

.app-tabs__count {
  min-width: 1.4em;
  padding: 0 var(--jf-space-1);
  border-radius: var(--jf-radius-pill);
  background: var(--tv-bg-elevated);
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
  line-height: 1.4;
}
</style>
