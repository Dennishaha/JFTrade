<script setup lang="ts">
import { onMounted, onUnmounted, watch } from "vue";
import { RouterView, useRouter } from "vue-router";

import {
  provideCommandPaletteStore,
  useCommandPalette,
} from "../composables/useCommandPalette";
import { provideConsoleDataStore } from "../composables/useConsoleData";
import { useDocsLink } from "../composables/useDocsLink";
import { provideNotificationsStore } from "../composables/useNotifications";
import { provideLiveSocketStore } from "../composables/useSharedLiveSocket";
import { provideThemeStore } from "../composables/useTheme";
import { provideWorkspaceLayoutStore } from "../composables/useWorkspaceLayout";
import CommandPalette from "./CommandPalette.vue";
import IconRail from "./IconRail.vue";
import type { SystemNotificationLiveSocketEvent } from "../composables/useLiveSocket";
import RightDock from "./RightDock.vue";
import StatusBar from "./StatusBar.vue";
import TopBar from "./TopBar.vue";

provideThemeStore();
const notifications = provideNotificationsStore();
const workspaceLayout = provideWorkspaceLayoutStore();
const palette = provideCommandPaletteStore();
const console_ = provideConsoleDataStore(workspaceLayout);
const live = provideLiveSocketStore();

const router = useRouter();
const { docsHomeUrl, openDocs } = useDocsLink();

const navTargets = [
  { id: "nav.workspace", label: "Go to Workspace", to: "/workspace" },
  { id: "nav.overview", label: "Go to Overview", to: "/overview" },
  { id: "nav.market", label: "Go to Market", to: "/market" },
  { id: "nav.strategy", label: "Go to Strategy", to: "/strategy" },
  { id: "nav.execution", label: "Go to Execution", to: "/execution" },
  { id: "nav.portfolio", label: "Go to Portfolio", to: "/portfolio" },
  { id: "nav.broker", label: "Go to Broker", to: "/broker" },
  { id: "nav.risk", label: "Go to Risk", to: "/risk" },
  { id: "nav.system", label: "Go to System", to: "/system" },
  { id: "nav.settings", label: "Go to Settings", to: "/settings" },
];

for (const target of navTargets) {
  palette.register({
    id: target.id,
    group: "Navigate",
    label: target.label,
    hint: target.to,
    run: () => {
      void router.push(target.to);
    },
  });
}

palette.register({
  id: "nav.docs",
  group: "Navigate",
  label: "Open Docs (new tab)",
  hint: docsHomeUrl,
  run: () => {
    openDocs();
  },
});

palette.register({
  id: "action.refresh",
  group: "Action",
  label: "Refresh console state",
  hint: "load system + storage overview",
  run: () => {
    void console_.loadSystemState();
  },
});

// Wire live socket events into the shared console store and notify only on
// non-market-data control events to avoid spamming the UI during live quotes.
let lastSeenLiveEventKey = "";
let lastSeenFutuOpenDIssueFingerprint = "";
let pendingMarketTickEvent: Parameters<typeof console_.applyMarketDataTickEvent>[0] | null =
  null;
let marketTickFlushTimer: ReturnType<typeof setTimeout> | null = null;

function isSystemNotificationEvent(
  event: unknown,
): event is SystemNotificationLiveSocketEvent {
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
        source: ev.source ?? ev.brokerId ?? "live-socket",
        at: ev.at,
        ...(ev.category ? { category: ev.category } : {}),
        ...(ev.message ? { message: ev.message } : {}),
      });
      return;
    }

    console_.applyMarketDataTickEvent(ev);

    if (ev.type !== "heartbeat" && ev.type !== "market-data.tick") {
      notifications.push({
        level: "info",
        title: `WS: ${ev.type}`,
        source: "live-socket",
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
        "请检查 OpenD 状态；如已重启 OpenD，请到 Settings / Futu Integration 手动重试。",
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
    title: "Workspace ready",
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
        <RouterView />
      </main>
      <RightDock />
    </div>
    <StatusBar />
    <CommandPalette />
  </div>
</template>
