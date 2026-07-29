<script setup lang="ts">
import { useADKChatComposerContext } from "@/composables/adk/useADKChatComposer";

const {
  effectivePermissionOption,
  isMobileLayout,
  mobileContextSummaryLabel,
  mobileControlsExpanded,
  selectedAgentLabel,
  selectedProviderLabel,
  selectedWorkModeLabel,
} = useADKChatComposerContext();
</script>

<template>
<div
        v-if="isMobileLayout"
        class="adk-mobile-composer-summary"
        data-testid="adk-mobile-composer-summary"
      >
        <div class="adk-mobile-composer-summary__chips">
          <span
            class="adk-mobile-composer-summary__chip"
            :title="`Agent · ${selectedAgentLabel}`"
          >
            <span class="adk-mobile-composer-summary__label">Agent</span>
            <span class="adk-mobile-composer-summary__value">{{
              selectedAgentLabel
            }}</span>
          </span>
          <span
            class="adk-mobile-composer-summary__chip"
            :title="`模型 · ${selectedProviderLabel}`"
          >
            <span class="adk-mobile-composer-summary__label">模型</span>
            <span class="adk-mobile-composer-summary__value">{{
              selectedProviderLabel
            }}</span>
          </span>
          <span
            class="adk-mobile-composer-summary__chip"
            :title="`审批 · ${effectivePermissionOption.title}`"
          >
            <span class="adk-mobile-composer-summary__label">审批</span>
            <span class="adk-mobile-composer-summary__value">{{
              effectivePermissionOption.title
            }}</span>
          </span>
          <span
            class="adk-mobile-composer-summary__chip"
            :title="`模式 · ${selectedWorkModeLabel}`"
          >
            <span class="adk-mobile-composer-summary__label">模式</span>
            <span class="adk-mobile-composer-summary__value">{{
              selectedWorkModeLabel
            }}</span>
          </span>
          <span
            class="adk-mobile-composer-summary__chip adk-mobile-composer-summary__chip--context"
            :title="`上下文 · ${mobileContextSummaryLabel}`"
          >
            <span class="adk-mobile-composer-summary__label">上下文</span>
            <span class="adk-mobile-composer-summary__value">{{
              mobileContextSummaryLabel
            }}</span>
          </span>
        </div>
        <button
          type="button"
          class="adk-mobile-composer-summary__toggle"
          data-testid="adk-mobile-composer-toggle"
          :aria-expanded="mobileControlsExpanded ? 'true' : 'false'"
          @click="mobileControlsExpanded = !mobileControlsExpanded"
        >
          {{ mobileControlsExpanded ? "收起" : "设置" }}
        </button>
      </div>
</template>

<style scoped>
.adk-mobile-composer-summary {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  padding: 0 8px 8px;
}

.adk-mobile-composer-summary__chips {
  min-width: 0;
  max-width: 100%;
  flex: 1 1 auto;
  display: flex;
  gap: 6px;
  overflow-x: auto;
  overscroll-behavior-x: contain;
  scrollbar-width: none;
}

.adk-mobile-composer-summary__chips::-webkit-scrollbar {
  display: none;
}

.adk-mobile-composer-summary__chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  max-width: min(168px, 100%);
  padding: 4px 8px;
  border: 1px solid color-mix(in srgb, var(--tv-border) 86%, var(--tv-accent) 14%);
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-bg-surface) 90%, transparent);
  color: var(--tv-text-muted);
  flex: 0 0 auto;
  font-size: 11px;
  line-height: 1.2;
}

.adk-mobile-composer-summary__chip--context {
  max-width: min(176px, 100%);
}

.adk-mobile-composer-summary__label {
  flex: 0 0 auto;
  color: var(--tv-text-dim);
}

.adk-mobile-composer-summary__value {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--tv-text);
}

.adk-mobile-composer-summary__toggle {
  flex: 0 0 auto;
  min-height: 28px;
  padding: 0 10px;
  border: 1px solid var(--adk-accent-border);
  border-radius: 999px;
  background: var(--adk-accent-bg-soft);
  color: var(--adk-accent-fg);
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
}
</style>
