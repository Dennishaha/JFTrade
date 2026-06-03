<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { RouterView, useRoute, useRouter } from "vue-router";

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
const shouldShowOobe = computed(
  () => console_.onboardingState.value.shouldShowOobe,
);

const router = useRouter();
const route = useRoute();
const isOobeRoute = computed(() => route.path === "/oobe");
const onboardingGateReady = ref(false);
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

function reconnectLiveStreamIfNeeded(): void {
  if (typeof document !== "undefined" && document.visibilityState === "hidden") {
    return;
  }
  live.connect();
}

async function initializeConsoleShell(): Promise<void> {
  try {
    const onboarding = await console_.loadOnboardingState();
    if (onboarding.shouldShowOobe && router.currentRoute.value.path !== "/oobe") {
      await router.replace("/oobe");
    }
  } finally {
    onboardingGateReady.value = true;
  }
  await console_.initialize();
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

const stopOobeRedirect = watch(
  shouldShowOobe,
  (show) => {
    if (show && route.path !== "/oobe") {
      void router.replace("/oobe");
    }
  },
  { immediate: true },
);

onMounted(() => {
  if (typeof document !== "undefined") {
    document.addEventListener("visibilitychange", reconnectLiveStreamIfNeeded);
  }
  if (typeof window !== "undefined") {
    window.addEventListener("online", reconnectLiveStreamIfNeeded);
  }
  void initializeConsoleShell();
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
  if (typeof document !== "undefined") {
    document.removeEventListener("visibilitychange", reconnectLiveStreamIfNeeded);
  }
  if (typeof window !== "undefined") {
    window.removeEventListener("online", reconnectLiveStreamIfNeeded);
  }
  live.disconnect();
  console_.dispose();
  stop();
  stopFutuOpenDMessages();
  stopOobeRedirect();
});
</script>

<template>
  <div class="tv-app" :class="{ 'tv-app--oobe': isOobeRoute }">
    <TopBar v-if="!isOobeRoute" />
    <div class="tv-app-body" :class="{ 'tv-app-body--oobe': isOobeRoute }">
      <IconRail v-if="!isOobeRoute" />
      <main class="tv-main">
        <div class="tv-main-scroll">
          <RouterView v-if="onboardingGateReady" />
        </div>
      </main>
      <RightDock v-if="!isOobeRoute" />
    </div>
    <StatusBar v-if="!isOobeRoute" />
    <CommandPalette />
  </div>
</template>
