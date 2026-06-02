<script setup lang="ts">
import { onMounted, onUnmounted, watch } from "vue";
import { RouterView, useRouter } from "vue-router";

import {
  provideCommandPaletteStore,
  useCommandPalette,
} from "../composables/useCommandPalette";
import { provideConsoleDataStore } from "../composables/useConsoleData";
import { useDocsLink } from "../composables/useDocsLink";
import {
  type SystemNotificationLiveStreamEvent,
} from "../composables/useLiveStream";
import { provideNotificationsStore } from "../composables/useNotifications";
import { provideLiveStreamStore } from "../composables/useSharedLiveStream";
import { provideThemeStore } from "../composables/useTheme";
import { provideUIColorPreferencesStore } from "../composables/useUIColorPreferences";
import { provideWorkspaceLayoutStore } from "../composables/useWorkspaceLayout";
import CommandPalette from "./CommandPalette.vue";
import IconRail from "./IconRail.vue";
import RightDock from "./RightDock.vue";
import StatusBar from "./StatusBar.vue";
import TopBar from "./TopBar.vue";

const themeStore = provideThemeStore();
provideUIColorPreferencesStore(themeStore.theme);
const notifications = provideNotificationsStore();
const workspaceLayout = provideWorkspaceLayoutStore();
const palette = provideCommandPaletteStore();
const console_ = provideConsoleDataStore(workspaceLayout);
const live = provideLiveStreamStore();

const router = useRouter();
const { docsHomeUrl, openDocs } = useDocsLink();

const navTargets = [
  { id: "nav.workspace", label: "打开交易工作台", to: "/workspace" },
  { id: "nav.overview", label: "打开概览", to: "/overview" },
  { id: "nav.market", label: "打开行情", to: "/market" },
  { id: "nav.strategy", label: "打开策略", to: "/strategy" },
  { id: "nav.backtest", label: "打开回测", to: "/backtest" },
  { id: "nav.account", label: "打开我的账户", to: "/account" },
  { id: "nav.risk", label: "打开风控", to: "/risk" },
  { id: "nav.system", label: "打开系统", to: "/system" },
  { id: "nav.settings", label: "打开设置", to: "/settings" },
];

for (const target of navTargets) {
  palette.register({
    id: target.id,
    group: "导航",
    label: target.label,
    hint: target.to,
    run: () => {
      void router.push(target.to);
    },
  });
}

palette.register({
  id: "nav.docs",
  group: "导航",
  label: "打开文档（新标签页）",
  hint: docsHomeUrl,
  run: () => {
    openDocs();
  },
});

palette.register({
  id: "action.refresh",
  group: "操作",
  label: "刷新控制台状态",
  hint: "重新加载系统与存储概览",
  run: () => {
    void console_.loadSystemState();
  },
});

// Wire live stream events into the shared console store and notify only on
// non-market-data control events to avoid spamming the UI during live quotes.
let lastSeenLiveEventKey = "";
let lastSeenFutuOpenDIssueFingerprint = "";
let pendingMarketTickEvent: Parameters<typeof console_.applyMarketDataTickEvent>[0] | null =
  null;
let marketTickFlushTimer: ReturnType<typeof setTimeout> | null = null;

function isSystemNotificationEvent(
  event: unknown,
): event is SystemNotificationLiveStreamEvent {
  return (
    event != null &&
    typeof event === "object" &&
    "type" in event &&
    (event as { type?: unknown }).type === "system.notification" &&
    "id" in event &&
    "level" in event &&
    "title" in event
  );
}

function formatLiveEventTypeLabel(type: string): string {
  if (type === "heartbeat") return "心跳";
  if (type === "market-data.tick") return "行情推送";
  if (type === "system.notification") return "系统通知";
  return type;
}

function shouldReloadSystemStateForNotification(
  event: SystemNotificationLiveStreamEvent,
): boolean {
  const source = event.source?.trim().toLowerCase() ?? "";
  const category = event.category?.trim().toLowerCase() ?? "";
  return source === "execution-orders" || category.startsWith("broker.order.");
}

function flushPendingMarketTickEvent(): void {
  if (pendingMarketTickEvent != null) {
    console_.applyMarketDataTickEvent(pendingMarketTickEvent);
    pendingMarketTickEvent = null;
  }

  if (marketTickFlushTimer != null) {
    clearTimeout(marketTickFlushTimer);
    marketTickFlushTimer = null;
  }
}

function scheduleMarketTickFlush(): void {
  if (marketTickFlushTimer != null) {
    return;
  }

  marketTickFlushTimer = setTimeout(() => {
    flushPendingMarketTickEvent();
  }, Math.floor(1000 / 30));
}

const stop = watch(
  () => live.events.value.at(-1),
  (ev) => {
    if (ev == null) {
      return;
    }

    const eventKey =
      ev.type === "market-data.tick" && "instrument" in ev && "snapshot" in ev
        ? `${ev.type}|${ev.instrument.instrumentId}|${ev.at}|${ev.snapshot.price}`
        : isSystemNotificationEvent(ev)
          ? `${ev.type}|${ev.id}`
        : `${ev.type}|${ev.at}`;
    if (eventKey === lastSeenLiveEventKey) {
      return;
    }
    lastSeenLiveEventKey = eventKey;

    if (ev.type === "market-data.tick") {
      pendingMarketTickEvent = ev;
      scheduleMarketTickFlush();
      return;
    }

    if (isSystemNotificationEvent(ev)) {
      notifications.push({
        level: ev.level,
        title: ev.title,
        source: ev.source ?? ev.brokerId ?? "live-stream",
        at: ev.at,
        ...(ev.category ? { category: ev.category } : {}),
        ...(ev.message ? { message: ev.message } : {}),
      });

      if (shouldReloadSystemStateForNotification(ev)) {
        void console_.loadSystemState({
          background: true,
          bypassCooldown: true,
        });
      }
      return;
    }

    console_.applyMarketDataTickEvent(ev);

    if (ev.type !== "heartbeat" && ev.type !== "market-data.tick") {
      notifications.push({
        level: "info",
        title: `实时通道：${formatLiveEventTypeLabel(ev.type)}`,
        source: "live-stream",
        at: ev.at,
        category: `live.${ev.type}`,
      });
    }
  },
);

const stopFutuOpenDMessages = watch(
  () => {
    const diagnosis = console_.futuOpenDHealth.value.diagnosis;
    return [
      diagnosis.code,
      diagnosis.manualRetryRequired ? "manual" : "auto",
      diagnosis.restartOpenDRecommended ? "restart" : "keep",
      console_.futuOpenDHealth.value.runtime.lastError ?? "",
    ].join("|");
  },
  (fingerprint) => {
    const diagnosis = console_.futuOpenDHealth.value.diagnosis;
    if (diagnosis.code === "NONE") {
      lastSeenFutuOpenDIssueFingerprint = "";
      return;
    }

    if (fingerprint === lastSeenFutuOpenDIssueFingerprint) {
      return;
    }

    lastSeenFutuOpenDIssueFingerprint = fingerprint;
    notifications.push({
      level: diagnosis.manualRetryRequired ? "error" : "warn",
      title: diagnosis.manualRetryRequired
        ? "OpenD 自动重试已暂停"
        : "OpenD 连接需要处理",
      message:
        diagnosis.summary ??
        "请检查 OpenD 状态；如已重启 OpenD，请到设置 / 富途接入中手动重试。",
      source: "futu-opend",
      category: "broker.connection",
    });
  },
);

onMounted(() => {
  void console_.initialize();
  live.connect();
  notifications.push({
    level: "info",
    title: "工作台已就绪",
    message: "JFTrade 控制台已加载。按 ⌘K / Ctrl+K 调出命令面板。",
    source: "system",
    category: "system.workspace",
  });
});

onUnmounted(() => {
  flushPendingMarketTickEvent();
  live.disconnect();
  console_.dispose();
  stop();
  stopFutuOpenDMessages();
});
</script>

<template>
  <div class="tv-app">
    <TopBar />
    <div class="tv-app-body">
      <IconRail />
      <main class="tv-main">
        <div class="tv-main-scroll">
          <RouterView />
        </div>
      </main>
      <RightDock />
    </div>
    <StatusBar />
    <CommandPalette />
  </div>
</template>
