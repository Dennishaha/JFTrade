<script setup lang="ts">
import { useADKChatComposerContext } from "@/composables/adk/useADKChatComposer";
import ADKComposerLeftControls from "./ADKComposerLeftControls.vue";
import ADKComposerRightControls from "./ADKComposerRightControls.vue";

const { isMobileLayout, mobileControlsExpanded } = useADKChatComposerContext();
</script>

<template>
  <div
    class="adk-composer-controls"
    :class="{
      'adk-composer-controls--mobile': isMobileLayout,
      'is-expanded': isMobileLayout && mobileControlsExpanded,
    }"
  >
    <ADKComposerLeftControls />
    <ADKComposerRightControls />
  </div>
</template>

<style scoped>
.adk-composer-controls :deep(.adk-permission-trigger) {
  display: inline-flex;
  min-height: 32px;
  flex: 0 0 auto;
  align-items: center;
  gap: 7px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  padding: 0 7px;
  font-size: var(--jf-text-7);
  font-weight: 600;
  cursor: pointer;
}

.adk-composer-controls :deep(.adk-permission-trigger:hover),
.adk-composer-controls :deep(.adk-inline-trigger:hover),
.adk-composer-controls :deep(.adk-inline-trigger[aria-expanded="true"]) {
  background: rgba(148, 163, 184, 0.12);
}

.adk-composer-controls :deep(.adk-inline-trigger) {
  display: inline-flex;
  min-width: 0;
  max-width: 210px;
  min-height: 32px;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  padding: 0 7px;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-7);
  line-height: 1;
  cursor: pointer;
}

.adk-composer-controls :deep(.adk-inline-trigger:hover),
.adk-composer-controls :deep(.adk-inline-trigger[aria-expanded="true"]) {
  color: var(--tv-text);
}

.adk-composer-controls :deep(.adk-inline-trigger:disabled) {
  cursor: default;
  opacity: 0.5;
}

.adk-composer-controls :deep(.adk-inline-trigger > span) {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.adk-composer-controls :deep(.adk-compat-select),
.adk-composer-controls :deep(.adk-sr-only) {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  border: 0;
  padding: 0;
  white-space: nowrap;
}

.adk-composer-controls :deep(.adk-agent-trigger) {
  max-width: clamp(96px, 18vw, 180px);
}

.adk-composer-controls :deep(.adk-provider-trigger) {
  max-width: clamp(92px, 19vw, 220px);
}

.adk-composer-controls :deep(.adk-work-mode-trigger) {
  max-width: 92px;
}

.adk-composer-controls :deep(.adk-permission-trigger.is-approval) {
  color: rgb(22 163 74);
}

.adk-composer-controls :deep(.adk-permission-trigger.is-less) {
  color: rgb(217 119 6);
}

.adk-composer-controls :deep(.adk-permission-trigger.is-all) {
  color: rgb(220 38 38);
}

.adk-composer-controls :deep(.adk-composer-stop),
.adk-composer-controls :deep(.adk-composer-interrupt),
.adk-composer-controls :deep(.adk-composer-send) {
  width: 38px;
  height: 38px;
  min-width: 38px;
  flex: 0 0 38px;
  border-radius: 999px;
  padding: 0;
  text-transform: none;
}

.adk-composer-controls :deep(.adk-composer-stop .v-btn__content),
.adk-composer-controls :deep(.adk-composer-interrupt .v-btn__content),
.adk-composer-controls :deep(.adk-composer-send .v-btn__content) {
  gap: 0;
}

.adk-composer-controls :deep(.adk-composer-utility),
.adk-composer-controls :deep(.adk-composer-actions) {
  display: flex;
  min-width: 0;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}

.adk-composer-controls--mobile {
  align-items: stretch;
  flex-direction: column;
  gap: 8px;
  padding: 0 8px 8px;
}

.adk-composer-controls--mobile :deep(.adk-composer-left),
.adk-composer-controls--mobile :deep(.adk-composer-right) {
  width: 100%;
  flex-wrap: wrap;
  gap: 8px;
}

.adk-composer-controls--mobile :deep(.adk-permission-trigger) {
  max-width: 148px;
}

.adk-composer-controls--mobile :deep(.adk-agent-trigger),
.adk-composer-controls--mobile :deep(.adk-provider-trigger) {
  max-width: none;
  flex: 1 1 130px;
}

.adk-composer-controls--mobile :deep(.adk-work-mode-trigger) {
  flex: 0 0 auto;
}

.adk-composer-controls--mobile :deep(.adk-context-ring) {
  flex: 0 0 34px;
}

.adk-composer-controls--mobile :deep(.adk-inline-progress) {
  width: 100%;
  max-width: none;
  flex: 1 1 100%;
}

.adk-composer-controls--mobile :deep(.adk-composer-utility),
.adk-composer-controls--mobile :deep(.adk-composer-actions) {
  width: 100%;
}

.adk-composer-controls--mobile :deep(.adk-composer-actions) {
  justify-content: flex-end;
}
</style>
