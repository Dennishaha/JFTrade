<script setup lang="ts">
import { useADKChatComposerContext } from "../../composables/useADKChatComposer";

const {
  canRevokeQueueItem,
  hasBlockingRun,
  isMobileLayout,
  queueItemBadge,
  queueItemStateClass,
  queueItems,
  revokeQueuedMessage,
} = useADKChatComposerContext();
</script>

<template>
<div v-if="queueItems.length > 0" class="adk-queue-strip" :class="{ 'adk-queue-strip--mobile': isMobileLayout }">
        <div class="adk-queue-strip__header">
          <span class="adk-queue-strip__title">待发送队列</span>
          <span v-if="hasBlockingRun" class="adk-queue-strip__hint">
            当前运行结束后自动发送
          </span>
        </div>
        <div class="adk-queue-list">
          <div
            v-for="(item, index) in queueItems"
            :key="item.id"
            class="adk-queue-item"
          >
            <span class="adk-queue-item__index">#{{ index + 1 }}</span>
            <span
              class="adk-queue-item__badge"
              :class="queueItemStateClass(item, index)"
            >
              {{ queueItemBadge(item, index) }}
            </span>
            <span class="adk-queue-item__text" :title="item.text">{{
              item.text
            }}</span>
            <button
              type="button"
              class="adk-queue-item__remove"
              :disabled="!canRevokeQueueItem(item)"
              @click="void revokeQueuedMessage?.(item.id)"
            >
              撤回
            </button>
          </div>
        </div>
      </div>
</template>

<style scoped>
.adk-queue-strip {
  display: grid;
  gap: 8px;
  margin-bottom: 10px;
  padding: 10px 12px;
  border: 1px solid rgb(226 232 240);
  border-radius: 14px;
  background: rgb(248 250 252);
}

.adk-queue-strip__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.adk-queue-strip__title {
  font-size: 12px;
  font-weight: 600;
  color: rgb(15 23 42);
}

.adk-queue-strip__hint {
  font-size: 12px;
  color: rgb(100 116 139);
}

.adk-queue-list {
  display: grid;
  gap: 8px;
}

.adk-queue-item {
  display: grid;
  grid-template-columns: auto auto 1fr auto;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.adk-queue-item__index {
  font-size: 12px;
  color: rgb(100 116 139);
}

.adk-queue-item__badge {
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  line-height: 1.4;
  text-transform: lowercase;
  color: var(--adk-queue-queued-fg);
  background: var(--adk-queue-queued-bg);
}

.adk-queue-item__badge.is-queued {
  color: var(--adk-queue-queued-fg);
  background: var(--adk-queue-queued-bg);
}

.adk-queue-item__badge.is-interrupt,
.adk-queue-item__badge.is-interrupting {
  color: var(--adk-queue-interrupt-fg);
  background: var(--adk-queue-interrupt-bg);
}

.adk-queue-item__badge.is-sending-next {
  color: var(--adk-queue-sending-fg);
  background: var(--adk-queue-sending-bg);
}

.adk-queue-item__text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: rgb(15 23 42);
}

.adk-queue-item__remove {
  border: 0;
  background: transparent;
  color: var(--adk-danger-fg);
  font-size: 12px;
  cursor: pointer;
}

.adk-queue-item__remove:disabled {
  opacity: 0.45;
  cursor: default;
}

.adk-queue-strip--mobile {
  margin: 0 8px 8px;
  padding: 8px 10px;
  border-radius: 12px;
}

.adk-queue-strip--mobile .adk-queue-item {
  gap: 6px;
}
</style>
