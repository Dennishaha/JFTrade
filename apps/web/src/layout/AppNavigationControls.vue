<script setup lang="ts">
defineProps<{
  canGoBack?: boolean;
  canGoForward?: boolean;
  compact?: boolean;
}>();

defineEmits<{
  back: [];
  forward: [];
  refresh: [];
}>();
</script>

<template>
  <nav
    class="app-navigation-controls"
    :class="{ 'app-navigation-controls--compact': compact }"
    aria-label="页面导航"
  >
    <button
      type="button"
      class="tv-icon-btn app-navigation-controls__button"
      :disabled="!canGoBack"
      title="返回"
      aria-label="返回"
      data-testid="topbar-navigation-back"
      @click="$emit('back')"
    >
      <svg viewBox="0 0 24 24" aria-hidden="true">
        <path d="m14.5 5-7 7 7 7" />
      </svg>
    </button>
    <button
      type="button"
      class="tv-icon-btn app-navigation-controls__button"
      :disabled="!canGoForward"
      title="前进"
      aria-label="前进"
      data-testid="topbar-navigation-forward"
      @click="$emit('forward')"
    >
      <svg viewBox="0 0 24 24" aria-hidden="true">
        <path d="m9.5 5 7 7-7 7" />
      </svg>
    </button>
    <button
      type="button"
      class="tv-icon-btn app-navigation-controls__button"
      title="刷新当前视图"
      aria-label="刷新当前视图"
      data-testid="topbar-navigation-refresh"
      @click="$emit('refresh')"
    >
      <svg viewBox="0 0 24 24" aria-hidden="true">
        <path d="M19 8V4m0 0h-4m4 0-3.2 3.2a7 7 0 1 0 1.4 8" />
      </svg>
    </button>
  </nav>
</template>

<style scoped>
.app-navigation-controls {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 2px;
  padding-inline: 2px;
  border-inline: 1px solid var(--tv-border);
}

.app-navigation-controls__button {
  width: 32px;
  height: 32px;
}

.app-navigation-controls__button svg {
  width: 17px;
  height: 17px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.app-navigation-controls__button:disabled {
  cursor: default;
  opacity: 0.32;
}

.app-navigation-controls--compact {
  gap: 0;
  padding-inline: 0;
  border-inline: 0;
}

.app-navigation-controls--compact .app-navigation-controls__button {
  width: 28px;
  height: 28px;
}
</style>
