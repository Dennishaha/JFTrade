<script setup lang="ts">
import { useADKChatComposerContext } from "@/composables/adk/useADKChatComposer";

const {
  breakdownRows,
  compactionModeLabel,
  contextBusy,
  contextMenuOpen,
  contextPillLabel,
  contextProgressColor,
  contextProgressValue,
  contextRevisionLabel,
  contextSnapshot,
  contextStatusLabel,
  contextSummaryPreview,
  contextTone,
  contextWindowLabel,
  formatTokenCount,
  hasKnownContextWindow,
  openContextPopover,
  rawBreakdownRows,
  rawContextDiagnosticsVisible,
  showContextControl,
} = useADKChatComposerContext();
</script>

<template>
<v-menu
              v-if="showContextControl"
              v-model="contextMenuOpen"
              location="top start"
              :close-on-content-click="false"
              open-on-hover
            >
              <template #activator="{ props: menuProps }">
                <button
                  v-bind="menuProps"
                  type="button"
                  class="adk-context-ring adk-context-pill"
                  :class="`is-${contextTone}`"
                  :title="`上下文：${contextPillLabel}`"
                  @click="openContextPopover"
                >
                  <v-progress-circular
                    :model-value="contextProgressValue"
                    :indeterminate="contextBusy && !hasKnownContextWindow"
                    :color="contextProgressColor"
                    size="24"
                    width="4"
                  />
                  <span class="adk-sr-only">{{ contextPillLabel }}</span>
                </button>
              </template>

              <v-card min-width="360" class="adk-context-card">
                <v-card-title class="text-subtitle-2">
                  上下文使用情况
                </v-card-title>
                <v-card-text class="adk-context-card__body">
                  <div class="adk-context-stat">
                    <span>当前上下文 Token</span>
                    <strong>{{
                      formatTokenCount(contextSnapshot?.currentInputTokens ?? 0)
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>下一轮预计 Token</span>
                    <strong>{{
                      formatTokenCount(
                        contextSnapshot?.projectedNextTurnTokens ?? 0,
                      )
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>模型窗口</span>
                    <strong>{{ contextWindowLabel(contextSnapshot) }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>状态</span>
                    <strong>{{ contextStatusLabel }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>保留最近用户消息条数</span>
                    <strong>{{ contextSnapshot?.recentUserWindow ?? 0 }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>当前保留用户消息</span>
                    <strong>{{
                      contextSnapshot?.retainedRecentUserCount ?? 0
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>生效 handoff 段数</span>
                    <strong>{{
                      contextSnapshot?.activeHandoffCount ?? 0
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>当前上下文版本</span>
                    <strong>{{ contextRevisionLabel(contextSnapshot) }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>已压缩事件数</span>
                    <strong>{{
                      contextSnapshot?.compactedEventCount ?? 0
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>最近压缩方式</span>
                    <strong>{{
                      compactionModeLabel(contextSnapshot?.lastCompactionMode)
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>自动压缩</span>
                    <strong>{{
                      contextSnapshot?.autoCompacted ? "是" : "否"
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>降级摘要</span>
                    <strong>{{
                      contextSnapshot?.degradedSummary ? "是" : "否"
                    }}</strong>
                  </div>
                  <template v-if="rawContextDiagnosticsVisible">
                    <div class="adk-context-stat">
                      <span>原始会话当前估算</span>
                      <strong>{{
                        formatTokenCount(
                          contextSnapshot?.rawCurrentInputTokens ??
                            contextSnapshot?.currentInputTokens ??
                            0,
                        )
                      }}</strong>
                    </div>
                    <div class="adk-context-stat">
                      <span>原始会话下一轮估算</span>
                      <strong>{{
                        formatTokenCount(
                          contextSnapshot?.rawProjectedNextTurnTokens ??
                            contextSnapshot?.projectedNextTurnTokens ??
                            0,
                        )
                      }}</strong>
                    </div>
                    <div class="adk-context-stat">
                      <span>已裁剪工具响应</span>
                      <strong>{{
                        contextSnapshot?.trimmedToolResponseCount ?? 0
                      }}</strong>
                    </div>
                  </template>
                  <div class="adk-context-breakdown">
                    <div class="adk-context-summary__title">
                      当前上下文 Token 构成
                    </div>
                    <div
                      v-for="item in breakdownRows"
                      :key="item.label"
                      class="adk-context-stat"
                    >
                      <span>{{ item.label }}</span>
                      <strong>{{ formatTokenCount(item.value) }}</strong>
                    </div>
                  </div>
                  <div
                    v-if="
                      rawContextDiagnosticsVisible &&
                      rawBreakdownRows.length > 0
                    "
                    class="adk-context-breakdown"
                  >
                    <div class="adk-context-summary__title">
                      原始会话诊断 Token 构成
                    </div>
                    <div
                      v-for="item in rawBreakdownRows"
                      :key="`page-raw-${item.label}`"
                      class="adk-context-stat"
                    >
                      <span>{{ item.label }}</span>
                      <strong>{{ formatTokenCount(item.value) }}</strong>
                    </div>
                  </div>
                  <div class="adk-context-summary">
                    <div class="adk-context-summary__title">
                      最新 handoff 摘要
                    </div>
                    <div class="adk-context-summary__content">
                      {{ contextSummaryPreview }}
                    </div>
                  </div>
                  <div
                    v-if="contextSnapshot?.lastCompactionReason"
                    class="adk-context-summary"
                  >
                    <div class="adk-context-summary__title">最近压缩原因</div>
                    <div class="adk-context-summary__content">
                      {{ contextSnapshot.lastCompactionReason }}
                    </div>
                  </div>
                  <div
                    v-if="contextSnapshot?.contextRevisionCreatedAt"
                    class="adk-context-summary"
                  >
                    <div class="adk-context-summary__title">版本创建时间</div>
                    <div class="adk-context-summary__content">
                      {{ contextSnapshot.contextRevisionCreatedAt }}
                    </div>
                  </div>
                  <div
                    v-if="contextSnapshot?.lastCompactedAt"
                    class="adk-context-summary"
                  >
                    <div class="adk-context-summary__title">最近压缩时间</div>
                    <div class="adk-context-summary__content">
                      {{ contextSnapshot.lastCompactedAt }}
                    </div>
                  </div>
                </v-card-text>
              </v-card>
            </v-menu>
</template>

<style scoped>
.adk-context-ring {
  display: inline-flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
}

.adk-context-ring:hover,
.adk-context-ring[aria-expanded="true"] {
  background: rgba(148, 163, 184, 0.12);
}

.adk-context-ring.is-warning {
  color: var(--adk-warning-fg);
}

.adk-context-ring.is-error {
  color: var(--adk-error-fg);
}

.adk-context-card__body {
  display: grid;
  gap: 8px;
}

.adk-context-stat {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
}

.adk-context-breakdown {
  display: grid;
  gap: 6px;
  margin-top: 4px;
}

.adk-context-summary {
  display: grid;
  gap: 4px;
  margin-top: 4px;
}

.adk-context-summary__title {
  color: rgb(100 116 139);
  font-size: 12px;
}

.adk-context-summary__content {
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
