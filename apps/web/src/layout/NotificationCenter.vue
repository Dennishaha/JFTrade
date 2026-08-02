<script setup lang="ts">
import { computed, ref } from "vue";

import {
  type NotificationItem,
  useNotifications,
} from "@/composables/shared/useNotifications";
import {
  formatDateTime,
  formatNotificationLevelLabel,
} from "@/composables/shared/consoleDataFormatting";

const { items, markAllRead, clear, remove } = useNotifications();

const statusFilter = ref<"all" | "unread">("all");
const levelFilter = ref<"all" | NotificationItem["level"]>("all");
const sourceFilter = ref<string>("all");
const categoryFilter = ref<string>("all");

const sourceOptions = computed<string[]>(() =>
  [...new Set(items.value.map((item) => item.source).filter((value): value is string => value != null && value !== ""))].sort(
    (left, right) => left.localeCompare(right),
  ),
);

const categoryOptions = computed<string[]>(() =>
  [...new Set(items.value.map((item) => item.category).filter((value): value is string => value != null && value !== ""))].sort(
    (left, right) => left.localeCompare(right),
  ),
);

const visible = computed<NotificationItem[]>(() =>
  items.value.filter((item) => {
    if (statusFilter.value === "unread" && item.read) {
      return false;
    }
    if (levelFilter.value !== "all" && item.level !== levelFilter.value) {
      return false;
    }
    if (sourceFilter.value !== "all" && item.source !== sourceFilter.value) {
      return false;
    }
    if (categoryFilter.value !== "all" && item.category !== categoryFilter.value) {
      return false;
    }
    return true;
  }),
);

const sourceLabels: Record<string, string> = {
  "exchange-calendars": "交易所日历",
  "futu-opend": "富途 OpenD",
  "live-stream": "实时通道",
  "order-entry": "下单面板",
  system: "系统",
};

const categoryLabels: Record<string, string> = {
  "broker.connection": "券商连接",
  "live.market-data.tick": "实时行情",
  "live.system.notification": "实时通知",
  "market.calendar.source": "交易所日历源",
  "system.workspace": "工作台",
};

function formatNotificationSource(source: string | null | undefined): string {
  if (source == null || source.trim() === "") {
    return "系统";
  }
  return sourceLabels[source] ?? source;
}

function formatNotificationCategory(category: string | null | undefined): string {
  if (category == null || category.trim() === "") {
    return "";
  }
  if (category.startsWith("live.")) {
    return categoryLabels[category] ?? `实时通道：${category.slice(5)}`;
  }
  return categoryLabels[category] ?? category;
}

function formatNotificationMeta(item: NotificationItem): string {
  const source = item.source == null
    ? formatNotificationLevelLabel(item.level)
    : formatNotificationSource(item.source);
  const category = formatNotificationCategory(item.category);
  return category === "" ? source : `${source} · ${category}`;
}

function fmt(at: string): string {
  return formatDateTime(at);
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="jf-filter-bar">
      <div class="tv-seg">
        <button :class="{ 'is-active': statusFilter === 'all' }" @click="statusFilter = 'all'">全部</button>
        <button :class="{ 'is-active': statusFilter === 'unread' }" @click="statusFilter = 'unread'">未读</button>
      </div>
      <label class="jf-filter-label">
        <span>级别</span>
        <select
          v-model="levelFilter"
          data-testid="notification-level-filter"
          class="tv-select jf-select-compact min-w-24"
        >
          <option value="all">全部级别</option>
          <option value="info">提示</option>
          <option value="success">成功</option>
          <option value="warn">警告</option>
          <option value="error">错误</option>
        </select>
      </label>
      <label class="jf-filter-label">
        <span>来源</span>
        <select
          v-model="sourceFilter"
          data-testid="notification-source-filter"
          class="tv-select jf-select-compact min-w-32"
        >
          <option value="all">全部来源</option>
          <option v-for="source in sourceOptions" :key="source" :value="source">{{ formatNotificationSource(source) }}</option>
        </select>
      </label>
      <label class="jf-filter-label">
        <span>分类</span>
        <select
          v-model="categoryFilter"
          data-testid="notification-category-filter"
          class="tv-select jf-select-compact min-w-[156px]"
        >
          <option value="all">全部分类</option>
          <option v-for="category in categoryOptions" :key="category" :value="category">{{ formatNotificationCategory(category) }}</option>
        </select>
      </label>
      <div class="flex-1"></div>
      <button class="tv-btn tv-btn-ghost jf-btn-compact" @click="markAllRead">全部标记已读</button>
      <button class="tv-btn tv-btn-ghost jf-btn-compact" @click="clear">清空</button>
    </div>
    <div class="flex-1 overflow-y-auto p-2.5">
      <div
        v-for="item in visible"
        :key="item.id"
        class="tv-noti-item"
        :class="[{ 'is-unread': !item.read }, `level-${item.level}`]"
      >
        <div class="flex justify-between gap-2">
          <div class="font-semibold">{{ item.title }}</div>
          <div class="jf-note" :title="formatDateTime(item.at)">{{ fmt(item.at) }}</div>
        </div>
        <div v-if="item.message" class="jf-text-muted mt-1">{{ item.message }}</div>
        <div class="jf-note mt-1 flex justify-between">
          <span>{{ formatNotificationMeta(item) }}</span>
          <button class="tv-btn tv-btn-ghost jf-btn-tiny" @click="remove(item.id)">×</button>
        </div>
      </div>
      <div v-if="visible.length === 0" class="jf-text-dim py-6 text-center text-[var(--jf-text-6)]">
        {{ items.length === 0 ? "暂无通知" : "当前筛选条件下暂无通知" }}
      </div>
    </div>
  </div>
</template>
