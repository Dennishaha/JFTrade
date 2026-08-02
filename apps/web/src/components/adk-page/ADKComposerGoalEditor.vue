<script setup lang="ts">
import { useADKChatComposerContext } from "@/composables/adk/useADKChatComposer";

const {
  activeRunId,
  canSaveGoalObjective,
  goalEditorExpanded,
  goalLifecycleBusy,
  goalLifecycleButtonDisabled,
  goalLifecycleButtonIcon,
  goalLifecycleButtonLabel,
  goalObjectiveDraft,
  goalObjectiveError,
  goalObjectiveSaving,
  goalObjectiveStatus,
  goalObjectiveSummary,
  goalObjectiveTone,
  goalPauseRequested,
  handleCancelGoalObjective,
  handleGoalLifecycleAction,
  handleGoalObjectiveInput,
  isMobileLayout,
  showGoalObjectiveEditor,
  showGoalLifecycleButton,
  updateGoalObjective,
} = useADKChatComposerContext();
</script>

<template>
<div
        v-if="showGoalObjectiveEditor"
        class="adk-goal-editor"
        :class="{ 'is-expanded': goalEditorExpanded, 'adk-goal-editor--mobile': isMobileLayout }"
      >
        <div class="adk-goal-editor__header">
          <button
            type="button"
            class="adk-goal-editor__summary-button"
            :aria-expanded="goalEditorExpanded ? 'true' : 'false'"
            @click="goalEditorExpanded = !goalEditorExpanded"
          >
            <span class="adk-goal-editor__title">
              <span>目标</span>
              <span class="adk-goal-editor__count">1</span>
            </span>
            <span class="adk-goal-editor__badge" :class="goalObjectiveTone">
              {{ goalObjectiveStatus }}
            </span>
            <span
              class="adk-goal-editor__summary"
              :title="goalObjectiveSummary"
            >
              {{ goalObjectiveSummary }}
            </span>
            <span class="adk-goal-editor__toggle">
              {{ goalEditorExpanded ? "收起" : "展开" }}
            </span>
          </button>
          <span class="adk-goal-editor__icon-group">
            <button
              v-if="showGoalLifecycleButton"
              type="button"
              class="adk-goal-editor__action"
              :class="{ 'is-busy': goalLifecycleBusy || goalPauseRequested }"
              :title="goalLifecycleButtonLabel"
              :aria-label="goalLifecycleButtonLabel"
              :disabled="goalLifecycleButtonDisabled"
              @click="void handleGoalLifecycleAction()"
            >
              <v-icon size="12">{{ goalLifecycleButtonIcon }}</v-icon>
              <span>{{ goalLifecycleButtonLabel }}</span>
            </button>
            <button
              type="button"
              class="adk-goal-editor__icon"
              title="编辑目标"
              aria-label="编辑目标"
              @click="goalEditorExpanded = true"
            >
              <v-icon size="12">fa-solid fa-pen</v-icon>
            </button>
            <button
              type="button"
              class="adk-goal-editor__icon"
              title="取消目标"
              aria-label="取消目标"
              @click="void handleCancelGoalObjective()"
            >
              <v-icon size="12">fa-solid fa-arrow-rotate-left</v-icon>
            </button>
          </span>
        </div>
        <div v-if="goalEditorExpanded" class="adk-goal-editor__body">
          <span v-if="goalObjectiveError" class="adk-goal-editor__error">
            {{ goalObjectiveError }}
          </span>
          <v-textarea
            :model-value="goalObjectiveDraft"
            variant="plain"
            density="compact"
            :rows="2"
            auto-grow
            :max-rows="4"
            hide-details
            class="adk-goal-editor__input"
            @update:model-value="handleGoalObjectiveInput"
          />
          <v-btn
            v-if="activeRunId"
            size="small"
            variant="tonal"
            color="primary"
            :loading="goalObjectiveSaving"
            :disabled="!canSaveGoalObjective"
            @click="void updateGoalObjective?.()"
          >
            保存
          </v-btn>
        </div>
      </div>
</template>

<style scoped>
.adk-goal-editor {
  display: grid;
  gap: 8px;
  margin-bottom: 10px;
  padding: 10px 12px;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--tv-bg-surface) 88%, transparent);
  color: var(--tv-text);
}

.adk-goal-editor.is-expanded {
  border-color: color-mix(in srgb, var(--tv-accent) 34%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 7%, var(--tv-bg-surface));
}

.adk-goal-editor__header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
}

.adk-goal-editor__summary-button {
  display: grid;
  grid-template-columns: auto auto minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
  min-width: 0;
  border: 0;
  padding: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.adk-goal-editor__title {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  min-width: 0;
  font-size: var(--jf-text-6);
  font-weight: 700;
  color: var(--tv-text);
}

.adk-goal-editor__count {
  min-width: 20px;
  padding: 1px 6px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-text-dim) 18%, transparent);
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
  line-height: 1.5;
  text-align: center;
}

.adk-goal-editor__badge {
  justify-self: start;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: var(--jf-text-5);
  line-height: 1.4;
}

.adk-goal-editor__badge.is-muted {
  color: var(--adk-muted-fg);
  background: var(--adk-muted-bg);
}

.adk-goal-editor__badge.is-info {
  color: var(--adk-info-fg);
  background: var(--adk-info-bg);
}

.adk-goal-editor__badge.is-warning {
  color: var(--adk-warning-fg);
  background: var(--adk-warning-bg);
}

.adk-goal-editor__badge.is-error {
  color: var(--adk-error-fg);
  background: var(--adk-error-bg);
}

.adk-goal-editor__summary {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-6);
}

.adk-goal-editor__toggle {
  color: var(--adk-accent-fg);
  font-size: var(--jf-text-6);
  white-space: nowrap;
}

.adk-goal-editor__icon-group {
  display: inline-flex;
  gap: 6px;
  align-items: center;
}

.adk-goal-editor__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: 1px solid var(--adk-accent-border);
  border-radius: 999px;
  background: var(--adk-accent-bg-soft);
  color: var(--adk-accent-fg);
  cursor: pointer;
}

.adk-goal-editor__action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 76px;
  height: 26px;
  padding: 0 10px;
  border: 1px solid var(--adk-accent-border);
  border-radius: 999px;
  background: var(--adk-accent-bg);
  color: var(--adk-accent-fg);
  font-size: var(--jf-text-6);
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
}

.adk-goal-editor__icon:disabled,
.adk-goal-editor__action:disabled {
  cursor: default;
  opacity: 0.55;
}

.adk-goal-editor__icon.is-busy,
.adk-goal-editor__action.is-busy {
  color: var(--adk-warning-fg);
  background: var(--adk-warning-bg);
}

.adk-goal-editor__body {
  display: flex;
  gap: 10px;
  align-items: center;
}

.adk-goal-editor__error {
  flex: 0 0 auto;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--jf-text-6);
  color: var(--adk-error-fg);
}

.adk-goal-editor__input {
  min-width: 0;
  flex: 1 1 auto;
}

.adk-goal-editor--mobile {
  margin: 0 8px 8px;
  padding: 8px 10px;
  border-radius: 14px;
  gap: 6px;
}

.adk-goal-editor--mobile__header,
.adk-goal-editor--mobile__summary-button,
.adk-goal-editor--mobile__icon-group,
.adk-goal-editor--mobile__body {
  gap: 6px;
}

.adk-goal-editor--mobile__body {
  flex-wrap: wrap;
}
</style>
