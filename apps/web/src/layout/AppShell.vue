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
  createMarketDataLiveReducer,
  createNotificationLiveReducer,
  formatLiveEventTypeLabel,
} from "../composables/liveEventReducers";
import { getLiveEventBus } from "../composables/liveEventBus";
import { provideNotificationsStore } from "../composables/useNotifications";
import { provideOptionComboDraftStore } from "../composables/optionComboDraft";
import { provideLiveStreamStore } from "../composables/useSharedLiveStream";
import { provideThemeStore } from "../composables/useTheme";
import { provideUIColorPreferencesStore } from "../composables/useUIColorPreferences";
import { provideWorkspaceLayoutStore } from "../composables/useWorkspaceLayout";
import CommandPalette from "./CommandPalette.vue";
import DesktopUpdateBanner from "../components/DesktopUpdateBanner.vue";
import IconRail from "./IconRail.vue";
import RightDock from "./RightDock.vue";
import StatusBar from "./StatusBar.vue";
import TopBar from "./TopBar.vue";

const themeStore = provideThemeStore();
provideUIColorPreferencesStore(themeStore.theme);
const notifications = provideNotificationsStore();
provideOptionComboDraftStore();
const workspaceLayout = provideWorkspaceLayoutStore();
const workspaceTradingPrefs = workspaceLayout;
const palette = provideCommandPaletteStore();
const console_ = provideConsoleDataStore(workspaceTradingPrefs);
const live = provideLiveStreamStore();
const shouldShowOobe = computed(
  () => console_.onboardingState.value.shouldShowOobe,
);

const router = useRouter();
const route = useRoute();
const isOobeRoute = computed(() => route.path === "/oobe");
const onboardingGateReady = ref(false);
const isCompactAppShell = ref(false);
const compactNavOpen = ref(false);
const appBodyRef = ref<HTMLElement | null>(null);
const { docsHomeUrl, openDocs } = useDocsLink();
const documentTitleSuffix = "JFTrade Console";
const APP_SHELL_COMPACT_MEDIA_QUERY = "(max-width: 1180px)";
let compactAppShellMediaQuery: MediaQueryList | null = null;
let dockResizeStart: {
  pointerId: number;
  bodyWidth: number;
} | null = null;
const activeWorkspaceInstrumentId = computed(() =>
  `${workspaceTradingPrefs.prefs.value.market}.${workspaceTradingPrefs.prefs.value.symbol}`
    .trim()
    .toUpperCase(),
);
const workspaceInstrumentName = computed(() => {
  const instrumentId = activeWorkspaceInstrumentId.value;
  const option = console_.marketInstrumentSearchOptions.value.find(
    (candidate) => candidate.instrumentId.trim().toUpperCase() === instrumentId,
  );
  const optionName = option?.name?.trim() ?? "";
  if (optionName !== "") {
    return optionName;
  }

  const securityDetails = console_.currentMarketSecurityDetails.value;
  if (
    securityDetails?.request.instrumentId.trim().toUpperCase() !== instrumentId
  ) {
    return "";
  }
  return securityDetails.security?.name?.trim() ?? "";
});
const workspaceDocumentTitle = computed(() => {
  const instrumentId = activeWorkspaceInstrumentId.value;
  const name = workspaceInstrumentName.value;
  const instrumentTitle =
    name === "" ? instrumentId : `${instrumentId}-${name}`;
  return `${instrumentTitle} - ${documentTitleSuffix}`;
});
const routeDocumentTitle = computed(() => {
  const matchedWithTitle = [...route.matched]
    .reverse()
    .find((record) => typeof record.meta.title === "string");
  const pageTitle = matchedWithTitle?.meta.title;
  return typeof pageTitle === "string" && pageTitle.trim() !== ""
    ? `${pageTitle.trim()} - ${documentTitleSuffix}`
    : documentTitleSuffix;
});
const documentTitle = computed(() =>
  route.path === "/workspace"
    ? workspaceDocumentTitle.value
    : routeDocumentTitle.value,
);

watch(
  documentTitle,
  (nextTitle) => {
    if (typeof document !== "undefined") {
      document.title = nextTitle;
    }
  },
  { immediate: true },
);

const navTargets = [
  { id: "nav.workspace", label: "打开交易工作台", to: "/workspace" },
  { id: "nav.watchlist", label: "打开自选股", to: "/watchlist" },
  {
    id: "nav.strategy.runtime",
    label: "打开策略执行",
    to: "/strategy/runtime",
  },
  { id: "nav.strategy.design", label: "打开策略设计", to: "/strategy/design" },
  { id: "nav.adk", label: "打开智能体", to: "/adk/agents" },
  { id: "nav.backtest", label: "打开回测", to: "/backtest" },
  { id: "nav.account", label: "打开我的账户", to: "/account" },
  { id: "nav.risk", label: "打开风控", to: "/risk" },
  { id: "nav.system", label: "打开系统", to: "/system" },
  { id: "nav.settings", label: "打开设置", to: "/settings" },
];
const rightDockOpen = computed(() => workspaceLayout.prefs.value.rightDockOpen);
const rightDockSize = computed(() => workspaceLayout.prefs.value.rightDockSize);
const appBodyStyle = computed(() => {
  if (isOobeRoute.value) {
    return {};
  }
  if (rightDockOpen.value && !isCompactAppShell.value) {
    return {
      "--tv-rightdock-width": `${rightDockSize.value}%`,
    };
  }
  return {};
});

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

let lastSeenFutuOpenDIssueFingerprint = "";
const marketDataLiveReducer = createMarketDataLiveReducer({
  applyMarketDataTickEvent: console_.applyMarketDataTickEvent,
});
const notificationLiveReducer = createNotificationLiveReducer({
  notifications,
  loadSystemState: console_.loadSystemState,
});
const stopLiveEventReducers = getLiveEventBus().subscribe((event) => {
  if (marketDataLiveReducer.handle(event)) {
    return;
  }
  if (notificationLiveReducer.handle(event)) {
    return;
  }
  if (
    event.type === "heartbeat" ||
    event.type === "console.refresh" ||
    event.type === "market.security-details" ||
    event.type === "market.depth"
  ) {
    return;
  }
  notifications.push({
    level: "info",
    title: `实时通道：${formatLiveEventTypeLabel(event.type)}`,
    source: "live-stream",
    at: event.serverTime,
    category: `live.${event.type}`,
  });
});

function syncCompactAppShell(
  event: MediaQueryListEvent | MediaQueryList,
): void {
  isCompactAppShell.value = event.matches;
  if (!event.matches) {
    compactNavOpen.value = false;
  }
}

function reconnectLiveStreamIfNeeded(): void {
  if (
    typeof document !== "undefined" &&
    document.visibilityState === "hidden"
  ) {
    return;
  }
  live.reconnect();
}

function toggleCompactNav(): void {
  compactNavOpen.value = !compactNavOpen.value;
}

function closeCompactNav(): void {
  compactNavOpen.value = false;
}

function closeRightDock(): void {
  workspaceLayout.update({ rightDockOpen: false });
}

function clampRightDockSize(size: number): number {
  return Math.min(48, Math.max(18, size));
}

function startRightDockResize(event: PointerEvent): void {
  if (isCompactAppShell.value || !rightDockOpen.value) {
    return;
  }
  const body = appBodyRef.value;
  if (body == null) {
    return;
  }
  const rect = body.getBoundingClientRect();
  if (!Number.isFinite(rect.width) || rect.width <= 0) {
    return;
  }
  dockResizeStart = {
    pointerId: event.pointerId,
    bodyWidth: rect.width,
  };
  window.addEventListener("pointermove", handleRightDockResizeMove);
  window.addEventListener("pointerup", stopRightDockResize);
  window.addEventListener("pointercancel", stopRightDockResize);
  event.preventDefault();
}

function handleRightDockResizeMove(event: PointerEvent): void {
  if (dockResizeStart == null) {
    return;
  }
  const body = appBodyRef.value;
  if (body == null) {
    return;
  }
  const rect = body.getBoundingClientRect();
  const bodyWidth = rect.width || dockResizeStart.bodyWidth;
  const nextSize = ((rect.right - event.clientX) / bodyWidth) * 100;
  workspaceLayout.update({ rightDockSize: clampRightDockSize(nextSize) });
}

function stopRightDockResize(event?: PointerEvent): void {
  if (
    event != null &&
    dockResizeStart != null &&
    event.pointerId !== dockResizeStart.pointerId
  ) {
    return;
  }
  dockResizeStart = null;
  window.removeEventListener("pointermove", handleRightDockResizeMove);
  window.removeEventListener("pointerup", stopRightDockResize);
  window.removeEventListener("pointercancel", stopRightDockResize);
}

async function initializeConsoleShell(): Promise<void> {
  try {
    const onboarding = await console_.loadOnboardingState();
    if (
      onboarding.shouldShowOobe &&
      router.currentRoute.value.path !== "/oobe"
    ) {
      await router.replace("/oobe");
    }
  } finally {
    onboardingGateReady.value = true;
  }
  await console_.initialize();
}

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
      title:
        diagnosis.code === "OPEND_VERSION_UNSUPPORTED"
          ? "OpenD 版本不受支持"
          : diagnosis.manualRetryRequired
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

const stopRouteCompactOverlays = watch(
  () => route.fullPath,
  () => {
    compactNavOpen.value = false;
  },
);

onMounted(() => {
  if (typeof document !== "undefined") {
    document.addEventListener("visibilitychange", reconnectLiveStreamIfNeeded);
  }
  if (typeof window !== "undefined") {
    window.addEventListener("online", reconnectLiveStreamIfNeeded);
    if (typeof window.matchMedia === "function") {
      compactAppShellMediaQuery = window.matchMedia(
        APP_SHELL_COMPACT_MEDIA_QUERY,
      );
      isCompactAppShell.value = compactAppShellMediaQuery.matches;
      if (typeof compactAppShellMediaQuery.addEventListener === "function") {
        compactAppShellMediaQuery.addEventListener(
          "change",
          syncCompactAppShell,
        );
      } else {
        compactAppShellMediaQuery.addListener(syncCompactAppShell);
      }
    }
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
  marketDataLiveReducer.dispose();
  stopLiveEventReducers();
  stopRightDockResize();
  if (typeof document !== "undefined") {
    document.removeEventListener(
      "visibilitychange",
      reconnectLiveStreamIfNeeded,
    );
  }
  if (typeof window !== "undefined") {
    window.removeEventListener("online", reconnectLiveStreamIfNeeded);
  }
  if (compactAppShellMediaQuery) {
    if (typeof compactAppShellMediaQuery.removeEventListener === "function") {
      compactAppShellMediaQuery.removeEventListener(
        "change",
        syncCompactAppShell,
      );
    } else {
      compactAppShellMediaQuery.removeListener(syncCompactAppShell);
    }
  }
  compactAppShellMediaQuery = null;
  live.disconnect();
  console_.dispose();
  stopFutuOpenDMessages();
  stopOobeRedirect();
  stopRouteCompactOverlays();
});
</script>

<template>
  <div class="tv-app" :class="{ 'tv-app--oobe': isOobeRoute }">
    <TopBar
      v-if="!isOobeRoute"
      :compact="isCompactAppShell"
      @toggle-nav="toggleCompactNav"
    />
    <DesktopUpdateBanner v-if="!isOobeRoute" />
    <div
      ref="appBodyRef"
      class="tv-app-body"
      :class="{
        'tv-app-body--oobe': isOobeRoute,
        'tv-app-body--dock-open':
          !isOobeRoute && rightDockOpen && !isCompactAppShell,
      }"
      :style="appBodyStyle"
    >
      <IconRail v-if="!isOobeRoute && !isCompactAppShell" />
      <main class="tv-main">
        <div class="tv-main-scroll">
          <RouterView v-if="onboardingGateReady" />
        </div>
      </main>
      <button
        v-if="!isOobeRoute && isCompactAppShell && compactNavOpen"
        type="button"
        class="tv-shell-backdrop tv-shell-backdrop--nav"
        aria-label="关闭导航"
        @click="closeCompactNav"
      />
      <div
        v-if="!isOobeRoute && isCompactAppShell && compactNavOpen"
        class="tv-compact-nav-slot"
        data-testid="compact-nav-drawer"
        @click="closeCompactNav"
      >
        <IconRail />
      </div>
      <button
        v-if="!isOobeRoute && isCompactAppShell && rightDockOpen"
        type="button"
        class="tv-shell-backdrop tv-shell-backdrop--dock"
        aria-label="关闭侧栏"
        @click="closeRightDock"
      />
      <div v-if="!isOobeRoute && rightDockOpen" class="tv-rightdock-slot">
        <button
          v-if="!isCompactAppShell"
          type="button"
          class="tv-rightdock-resizer tv-resizer--vertical"
          title="调整侧栏宽度"
          aria-label="调整侧栏宽度"
          @pointerdown="startRightDockResize"
        />
        <RightDock />
      </div>
    </div>
    <StatusBar v-if="!isOobeRoute" />
    <CommandPalette />
  </div>
</template>
