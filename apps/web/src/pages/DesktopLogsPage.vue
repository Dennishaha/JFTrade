<script setup lang="ts">
import { Events } from "@wailsio/runtime";
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";

import {
  ListDays,
  OpenFolder,
  ReadPage,
} from "../wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplogservice";
import type {
  DesktopLogDay,
  DesktopLogLine,
} from "../wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/models";

type DesktopLogAppend = { day: string; line: DesktopLogLine };

const latestPageOffset = -1;

const days = ref<DesktopLogDay[]>([]);
const selectedDay = ref("");
const selectedLevel = ref("ALL");
const keyword = ref("");
const pageSize = ref(200);
const offset = ref(0);
const total = ref(0);
const lines = ref<DesktopLogLine[]>([]);
const logDir = ref("");
const loading = ref(false);
const error = ref("");
const contentElement = ref<HTMLElement | null>(null);
let cancelAppend: (() => void) | null = null;
let filterTimer: ReturnType<typeof setTimeout> | null = null;
let initialized = false;

const page = computed(() => Math.floor(offset.value / pageSize.value) + 1);
const totalPages = computed(() =>
  Math.max(1, Math.ceil(total.value / pageSize.value)),
);
const canPrevious = computed(() => offset.value > 0);
const canNext = computed(() => offset.value + lines.value.length < total.value);

async function loadDays(): Promise<void> {
  days.value = (await ListDays()) ?? [];
  if (!selectedDay.value && days.value.length > 0) {
    selectedDay.value = days.value[0]?.day ?? "";
  }
}

async function scrollToBottom(): Promise<void> {
  await nextTick();
  if (contentElement.value) {
    contentElement.value.scrollTop = contentElement.value.scrollHeight;
  }
}

async function loadPage(requestedOffset = offset.value): Promise<void> {
  if (!selectedDay.value) {
    lines.value = [];
    total.value = 0;
    return;
  }
  loading.value = true;
  error.value = "";
  let loaded = false;
  try {
    const result = await ReadPage(
      selectedDay.value,
      selectedLevel.value,
      keyword.value,
      requestedOffset,
      pageSize.value,
    );
    lines.value = result.items ?? [];
    offset.value = result.offset;
    total.value = result.total;
    logDir.value = result.logDir;
    loaded = true;
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "日志读取失败";
  } finally {
    loading.value = false;
  }
  if (loaded && requestedOffset === latestPageOffset) {
    await scrollToBottom();
  }
}

function resetAndLoad(): void {
  if (!initialized) return;
  void loadPage(latestPageOffset);
}

function scheduleResetAndLoad(): void {
  if (!initialized) return;
  if (filterTimer != null) clearTimeout(filterTimer);
  filterTimer = setTimeout(() => void loadPage(latestPageOffset), 200);
}

function previousPage(): void {
  offset.value = Math.max(0, offset.value - pageSize.value);
  void loadPage();
}

function nextPage(): void {
  if (!canNext.value) return;
  offset.value += pageSize.value;
  void loadPage();
}

async function openFolder(): Promise<void> {
  try {
    await OpenFolder();
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "无法打开日志目录";
  }
}

function matchesCurrentFilters(line: DesktopLogLine): boolean {
  if (selectedLevel.value !== "ALL" && line.level !== selectedLevel.value) {
    return false;
  }
  const normalizedKeyword = keyword.value.trim().toLowerCase();
  return (
    normalizedKeyword === "" ||
    line.text.toLowerCase().includes(normalizedKeyword)
  );
}

function appendLine(payload: DesktopLogAppend): void {
  if (
    payload?.day !== selectedDay.value ||
    !payload.line ||
    !matchesCurrentFilters(payload.line)
  ) {
    return;
  }

  const wasLastPage = offset.value + lines.value.length >= total.value;
  total.value += 1;
  if (!wasLastPage) return;

  if (lines.value.length >= pageSize.value) {
    offset.value += pageSize.value;
    lines.value = [payload.line];
  } else {
    lines.value.push(payload.line);
  }
  void scrollToBottom();
}

watch([selectedDay, selectedLevel, pageSize], resetAndLoad);
watch(keyword, () => {
  scheduleResetAndLoad();
});

onMounted(async () => {
  try {
    await loadDays();
    initialized = true;
    await loadPage(latestPageOffset);
    cancelAppend = Events.On("jftrade:desktop-log:append", (event) => {
      appendLine(event.data as DesktopLogAppend);
    });
  } catch (reason) {
    error.value =
      reason instanceof Error ? reason.message : "桌面日志服务不可用";
  }
});

onUnmounted(() => {
  cancelAppend?.();
  if (filterTimer != null) clearTimeout(filterTimer);
});
</script>

<template>
  <main class="desktop-logs">
    <header class="desktop-logs__toolbar">
      <select v-model="selectedDay" aria-label="日志日期">
        <option v-for="item in days" :key="item.day" :value="item.day">
          {{ item.day }}
        </option>
      </select>
      <select v-model="selectedLevel" aria-label="日志级别">
        <option
          v-for="level in ['ALL', 'ERROR', 'WARN', 'INFO', 'DEBUG']"
          :key="level"
        >
          {{ level }}
        </option>
      </select>
      <input v-model="keyword" type="search" placeholder="过滤日志内容" />
      <select v-model.number="pageSize" aria-label="每页行数">
        <option :value="100">100</option>
        <option :value="200">200</option>
        <option :value="500">500</option>
      </select>
      <button type="button" @click="openFolder">打开日志文件夹</button>
    </header>

    <div class="desktop-logs__meta">
      <span>{{ logDir || "日志目录" }}</span>
      <span>第 {{ page }} / {{ totalPages }} 页 · 共 {{ total }} 行</span>
    </div>

    <div v-if="error" class="desktop-logs__error">{{ error }}</div>
    <section
      ref="contentElement"
      class="desktop-logs__content"
      :aria-busy="loading"
    >
      <div v-if="loading" class="desktop-logs__empty">读取中…</div>
      <div v-else-if="lines.length === 0" class="desktop-logs__empty">
        没有匹配的日志
      </div>
      <div
        v-for="(line, index) in lines"
        v-else
        :key="`${offset}-${index}`"
        class="desktop-logs__line"
      >
        <strong :class="`is-${line.level.toLowerCase()}`">{{
          line.level
        }}</strong>
        <span>{{ line.text }}</span>
      </div>
    </section>

    <footer class="desktop-logs__pager">
      <button type="button" :disabled="!canPrevious" @click="previousPage">
        上一页
      </button>
      <button type="button" :disabled="!canNext" @click="nextPage">
        下一页
      </button>
    </footer>
  </main>
</template>

<style scoped>
.desktop-logs {
  display: flex;
  flex-direction: column;
  height: 100vh;
  color: #e7ecef;
  background: #101214;
  font:
    13px/1.45 ui-monospace,
    SFMono-Regular,
    Menlo,
    monospace;
}
.desktop-logs__toolbar,
.desktop-logs__pager {
  display: flex;
  gap: 10px;
  padding: 10px 12px;
  background: #171a1d;
  border-color: #262b30;
}
.desktop-logs__toolbar {
  border-bottom: 1px solid #262b30;
}
.desktop-logs__toolbar input {
  flex: 1;
  min-width: 160px;
}
.desktop-logs select,
.desktop-logs input,
.desktop-logs button {
  border: 1px solid #343a40;
  border-radius: 5px;
  padding: 6px 8px;
  color: inherit;
  background: #101214;
}
.desktop-logs button:disabled {
  opacity: 0.45;
}
.desktop-logs__meta {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 7px 12px;
  color: #99a3ad;
  border-bottom: 1px solid #262b30;
}
.desktop-logs__content {
  flex: 1;
  overflow: auto;
  padding: 8px 12px;
}
.desktop-logs__line {
  display: grid;
  grid-template-columns: 56px minmax(0, 1fr);
  gap: 10px;
  padding: 2px 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.desktop-logs__line strong.is-error,
.desktop-logs__error {
  color: #ff6b6b;
}
.desktop-logs__line strong.is-warn {
  color: #f4b942;
}
.desktop-logs__line strong.is-info {
  color: #73d28b;
}
.desktop-logs__line strong.is-debug {
  color: #a99cff;
}
.desktop-logs__empty,
.desktop-logs__error {
  padding: 18px 12px;
}
.desktop-logs__pager {
  justify-content: flex-end;
  border-top: 1px solid #262b30;
}
</style>
