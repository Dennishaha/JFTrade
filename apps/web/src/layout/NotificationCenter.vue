<script setup lang="ts">
import { computed, ref } from "vue";

import {
  type NotificationItem,
  useNotifications,
} from "../composables/useNotifications";

const { items, markAllRead, clear, remove } = useNotifications();

const filter = ref<"all" | "unread">("all");

const visible = computed<NotificationItem[]>(() =>
  filter.value === "unread"
    ? items.value.filter((item) => !item.read)
    : items.value,
);

function fmt(at: string): string {
  try {
    return new Date(at).toISOString().substring(11, 19);
  } catch {
    return at;
  }
}
</script>

<template>
  <div style="display: flex; flex-direction: column; height: 100%; min-height: 0">
    <div style="display: flex; gap: 6px; align-items: center; padding: 8px 10px; border-bottom: 1px solid var(--tv-border)">
      <div class="tv-seg">
        <button :class="{ 'is-active': filter === 'all' }" @click="filter = 'all'">All</button>
        <button :class="{ 'is-active': filter === 'unread' }" @click="filter = 'unread'">Unread</button>
      </div>
      <div style="flex: 1"></div>
      <button class="tv-btn tv-btn-ghost" style="height: 26px; font-size: 11px" @click="markAllRead">Mark read</button>
      <button class="tv-btn tv-btn-ghost" style="height: 26px; font-size: 11px" @click="clear">Clear</button>
    </div>
    <div style="flex: 1; overflow-y: auto; padding: 10px">
      <div
        v-for="item in visible"
        :key="item.id"
        class="tv-noti-item"
        :class="[{ 'is-unread': !item.read }, `level-${item.level}`]"
      >
        <div style="display: flex; justify-content: space-between; gap: 8px">
          <div style="font-weight: 600">{{ item.title }}</div>
          <div style="color: var(--tv-text-dim); font-size: 11px">{{ fmt(item.at) }}</div>
        </div>
        <div v-if="item.message" style="margin-top: 4px; color: var(--tv-text-muted)">{{ item.message }}</div>
        <div style="display: flex; justify-content: space-between; margin-top: 4px; color: var(--tv-text-dim); font-size: 11px">
          <span>{{ item.source || item.level }}</span>
          <button class="tv-btn tv-btn-ghost" style="height: 20px; font-size: 10px; padding: 0 6px" @click="remove(item.id)">×</button>
        </div>
      </div>
      <div v-if="visible.length === 0" style="color: var(--tv-text-dim); text-align: center; padding: 24px 0; font-size: 12px">
        暂无通知
      </div>
    </div>
  </div>
</template>
