// @vitest-environment jsdom

import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref, type Ref } from "vue";

const testState = vi.hoisted(() => ({
  notificationsStore: null as null | { push: ReturnType<typeof vi.fn> },
  workspaceLayoutStore: null as null | {
    prefs: Ref<{
      market: string;
      symbol: string;
      rightDockOpen: boolean;
      rightDockTab: "notifications" | "ai";
      rightDockSize: number;
    }>;
    update: ReturnType<typeof vi.fn>;
  },
  consoleStore: null as null | {
    onboardingState: Ref<{ shouldShowOobe: boolean }>;
    marketInstrumentSearchOptions: Ref<Array<{ instrumentId: string; name: string }>>;
    currentMarketSecurityDetails: Ref<{
      request: { instrumentId: string };
      security?: { name?: string };
    } | null>;
    futuOpenDHealth: Ref<{
      diagnosis: {
        code: string;
        manualRetryRequired: boolean;
        restartOpenDRecommended: boolean;
        summary?: string;
      };
      runtime: { lastError?: string };
    }>;
    loadOnboardingState: ReturnType<typeof vi.fn>;
    initialize: ReturnType<typeof vi.fn>;
    loadSystemState: ReturnType<typeof vi.fn>;
    applyMarketDataTickEvent: ReturnType<typeof vi.fn>;
    dispose: ReturnType<typeof vi.fn>;
  },
  liveStore: null as null | {
    connect: ReturnType<typeof vi.fn>;
    reconnect: ReturnType<typeof vi.fn>;
    disconnect: ReturnType<typeof vi.fn>;
  },
  docsLink: null as null | {
    docsHomeUrl: string;
    openDocs: ReturnType<typeof vi.fn>;
  },
  liveEventHandler: null as null | ((event: any) => void),
  stopLiveSubscription: vi.fn(),
  marketReducerDispose: vi.fn(),
  notificationReducerDispose: vi.fn(),
}));

vi.mock("../src/composables/useNotifications", () => ({
  provideNotificationsStore: () => testState.notificationsStore,
}));

vi.mock("../src/composables/useWorkspaceLayout", () => ({
  provideWorkspaceLayoutStore: () => testState.workspaceLayoutStore,
}));

vi.mock("../src/composables/useConsoleData", () => ({
  provideConsoleDataStore: () => testState.consoleStore,
}));

vi.mock("../src/composables/useSharedLiveStream", () => ({
  provideLiveStreamStore: () => testState.liveStore,
}));

vi.mock("../src/composables/useDocsLink", () => ({
  useDocsLink: () => testState.docsLink,
}));

vi.mock("../src/composables/liveEventBus", () => ({
  getLiveEventBus: () => ({
    subscribe: (handler: (event: any) => void) => {
      testState.liveEventHandler = handler;
      return testState.stopLiveSubscription;
    },
  }),
}));

vi.mock("../src/composables/liveEventReducers", () => ({
  createMarketDataLiveReducer: () => ({
    handle: () => false,
    dispose: testState.marketReducerDispose,
  }),
  createNotificationLiveReducer: () => ({
    handle: () => false,
    dispose: testState.notificationReducerDispose,
  }),
  formatLiveEventTypeLabel: (type: string) => `label:${type}`,
}));

vi.mock("../src/composables/useTheme", () => ({
  provideThemeStore: () => ({ theme: ref("light") }),
}));

vi.mock("../src/composables/useUIColorPreferences", () => ({
  provideUIColorPreferencesStore: () => undefined,
}));

import AppShell from "../src/layout/AppShell.vue";

const WorkspaceRoute = defineComponent({
  template: "<div data-testid='workspace-route'>workspace</div>",
});

const OobeRoute = defineComponent({
  template: "<div data-testid='oobe-route'>oobe</div>",
});

const wrappers: VueWrapper[] = [];

function createTestRouter(initialPath = "/workspace") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: "/workspace",
        component: WorkspaceRoute,
        meta: { title: "交易" },
      },
      {
        path: "/oobe",
        component: OobeRoute,
        meta: { title: "初始化" },
      },
      {
        path: "/system",
        component: WorkspaceRoute,
        meta: { title: "系统" },
      },
    ],
  });
  return router.push(initialPath).then(() => router.isReady()).then(() => router);
}

function createConsoleStore() {
  return {
    onboardingState: ref({ shouldShowOobe: false }),
    marketInstrumentSearchOptions: ref([
      { instrumentId: "US.AAPL", name: "Apple" },
    ]),
    currentMarketSecurityDetails: ref(null),
    futuOpenDHealth: ref({
      diagnosis: {
        code: "NONE",
        manualRetryRequired: false,
        restartOpenDRecommended: false,
      },
      runtime: {},
    }),
    loadOnboardingState: vi.fn(async () => ({ shouldShowOobe: false })),
    initialize: vi.fn(async () => undefined),
    loadSystemState: vi.fn(async () => undefined),
    applyMarketDataTickEvent: vi.fn(),
    dispose: vi.fn(),
  };
}

function createWorkspaceLayoutStore() {
  return {
    prefs: ref({
      market: "US",
      symbol: "AAPL",
      rightDockOpen: true,
      rightDockTab: "notifications" as const,
      rightDockSize: 28,
    }),
    update: vi.fn((patch: Partial<{
      market: string;
      symbol: string;
      rightDockOpen: boolean;
      rightDockTab: "notifications" | "ai";
      rightDockSize: number;
    }>) => {
      testState.workspaceLayoutStore!.prefs.value = {
        ...testState.workspaceLayoutStore!.prefs.value,
        ...patch,
      };
    }),
  };
}

async function mountAppShell(initialPath = "/workspace") {
  const router = await createTestRouter(initialPath);
  const wrapper = mount(AppShell, {
    global: {
      plugins: [router],
      stubs: {
        TopBar: defineComponent({
          props: {
            compact: {
              type: Boolean,
              default: false,
            },
          },
          emits: ["toggle-nav"],
          template:
            "<header data-testid='top-bar'>top<button v-if='compact' data-testid='stub-compact-nav-toggle' @click=\"$emit('toggle-nav')\">nav</button></header>",
        }),
        IconRail: { template: "<aside data-testid='icon-rail'>rail</aside>" },
        RightDock: { template: "<aside data-testid='right-dock'>dock</aside>" },
        StatusBar: { template: "<footer data-testid='status-bar'>status</footer>" },
        CommandPalette: { template: "<div data-testid='command-palette'>palette</div>" },
      },
    },
  });
  wrappers.push(wrapper);
  await Promise.resolve();
  await nextTick();
  await Promise.resolve();
  await nextTick();
  return { router, wrapper, setup: wrapper.vm.$.setupState as Record<string, any> };
}

function setVisibilityState(state: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    value: state,
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  testState.notificationsStore = { push: vi.fn() };
  testState.workspaceLayoutStore = createWorkspaceLayoutStore();
  testState.consoleStore = createConsoleStore();
  testState.liveStore = {
    connect: vi.fn(),
    reconnect: vi.fn(),
    disconnect: vi.fn(),
  };
  testState.docsLink = {
    docsHomeUrl: "https://docs.example.com",
    openDocs: vi.fn(),
  };
  testState.liveEventHandler = null;
  setVisibilityState("visible");
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount();
  }
  document.body.innerHTML = "";
});

describe("AppShell business flows", () => {
  it("handles modern media listeners, reconnects live streams, and dedupes OpenD issue notifications", async () => {
    const mediaQuery = {
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    };
    vi.stubGlobal("matchMedia", vi.fn(() => mediaQuery));

    const { wrapper } = await mountAppShell();

    expect(document.title).toContain("US.AAPL-Apple");
    expect(testState.liveStore?.connect).toHaveBeenCalledTimes(1);
    expect(testState.notificationsStore?.push).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "工作台已就绪",
      }),
    );
    expect(mediaQuery.addEventListener).toHaveBeenCalledWith(
      "change",
      expect.any(Function),
    );
    expect(window.matchMedia).toHaveBeenCalledWith("(max-width: 1180px)");

    testState.notificationsStore?.push.mockClear();

    setVisibilityState("hidden");
    document.dispatchEvent(new Event("visibilitychange"));
    expect(testState.liveStore?.reconnect).not.toHaveBeenCalled();

    setVisibilityState("visible");
    document.dispatchEvent(new Event("visibilitychange"));
    window.dispatchEvent(new Event("online"));
    expect(testState.liveStore?.reconnect).toHaveBeenCalledTimes(2);

    testState.consoleStore!.futuOpenDHealth.value = {
      diagnosis: {
        code: "OPEND_RETRY_PAUSED",
        manualRetryRequired: true,
        restartOpenDRecommended: false,
        summary: "OpenD paused",
      },
      runtime: { lastError: "dial refused" },
    };
    await nextTick();
    expect(testState.notificationsStore?.push).toHaveBeenCalledWith(
      expect.objectContaining({
        level: "error",
        title: "OpenD 自动重试已暂停",
        message: "OpenD paused",
      }),
    );

    testState.notificationsStore?.push.mockClear();
    testState.consoleStore!.futuOpenDHealth.value = {
      diagnosis: {
        code: "OPEND_RETRY_PAUSED",
        manualRetryRequired: true,
        restartOpenDRecommended: false,
        summary: "OpenD paused",
      },
      runtime: { lastError: "dial refused" },
    };
    await nextTick();
    expect(testState.notificationsStore?.push).not.toHaveBeenCalled();

    testState.consoleStore!.futuOpenDHealth.value = {
      diagnosis: {
        code: "NONE",
        manualRetryRequired: false,
        restartOpenDRecommended: false,
      },
      runtime: {},
    };
    await nextTick();
    testState.consoleStore!.futuOpenDHealth.value = {
      diagnosis: {
        code: "OPEND_RETRY_PAUSED",
        manualRetryRequired: true,
        restartOpenDRecommended: false,
        summary: "OpenD paused",
      },
      runtime: { lastError: "dial refused" },
    };
    await nextTick();
    expect(testState.notificationsStore?.push).toHaveBeenCalledTimes(1);

    testState.notificationsStore?.push.mockClear();
    testState.liveEventHandler?.({
      type: "strategy.event",
      serverTime: "2026-07-02T00:00:00Z",
    });
    expect(testState.notificationsStore?.push).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "实时通道：label:strategy.event",
      }),
    );

    wrapper.unmount();
    expect(mediaQuery.removeEventListener).toHaveBeenCalledWith(
      "change",
      expect.any(Function),
    );
    expect(testState.liveStore?.disconnect).toHaveBeenCalledTimes(1);
    expect(testState.consoleStore?.dispose).toHaveBeenCalledTimes(1);
  });

  it("supports legacy media listeners, resizes the right dock, and redirects to OOBE when onboarding state changes", async () => {
    const legacyMediaQuery = {
      matches: false,
      addListener: vi.fn(),
      removeListener: vi.fn(),
    };
    vi.stubGlobal("matchMedia", vi.fn(() => legacyMediaQuery));

    const { router, wrapper, setup } = await mountAppShell();

    expect(legacyMediaQuery.addListener).toHaveBeenCalledWith(
      expect.any(Function),
    );
    expect(wrapper.find(".tv-rightdock-resizer").exists()).toBe(true);

    const appBody = wrapper.get(".tv-app-body").element as HTMLElement;
    appBody.getBoundingClientRect = () =>
      ({
        width: 1000,
        right: 1000,
      }) as DOMRect;

    setup.startRightDockResize({
      pointerId: 7,
      preventDefault: vi.fn(),
    });
    setup.handleRightDockResizeMove({ clientX: 0 });
    setup.handleRightDockResizeMove({ clientX: 900 });
    setup.stopRightDockResize({ pointerId: 99 });
    setup.handleRightDockResizeMove({ clientX: 750 });
    setup.stopRightDockResize({ pointerId: 7 });

    expect(testState.workspaceLayoutStore?.update).toHaveBeenCalledWith({
      rightDockSize: 48,
    });
    expect(testState.workspaceLayoutStore?.update).toHaveBeenCalledWith({
      rightDockSize: 18,
    });
    expect(testState.workspaceLayoutStore?.update).toHaveBeenCalledWith({
      rightDockSize: 25,
    });

    setup.syncCompactAppShell({ matches: true });
    await nextTick();
    expect(wrapper.find(".tv-rightdock-resizer").exists()).toBe(false);
    expect(wrapper.find("[data-testid='icon-rail']").exists()).toBe(false);

    await wrapper.get("[data-testid='stub-compact-nav-toggle']").trigger("click");
    expect(wrapper.find("[data-testid='compact-nav-drawer']").exists()).toBe(true);
    expect(wrapper.find("[data-testid='icon-rail']").exists()).toBe(true);

    await wrapper.get(".tv-shell-backdrop--nav").trigger("click");
    expect(wrapper.find("[data-testid='compact-nav-drawer']").exists()).toBe(false);

    await wrapper.get(".tv-shell-backdrop--dock").trigger("click");
    expect(testState.workspaceLayoutStore?.update).toHaveBeenCalledWith({
      rightDockOpen: false,
    });

    testState.consoleStore!.onboardingState.value.shouldShowOobe = true;
    await nextTick();
    await flushPromises();
    expect(router.currentRoute.value.path).toBe("/oobe");

    wrapper.unmount();
    expect(legacyMediaQuery.removeListener).toHaveBeenCalledWith(
      expect.any(Function),
    );
  });

  it("redirects to OOBE during initialization when onboarding requires it", async () => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn(() => ({
        matches: false,
        addListener: vi.fn(),
        removeListener: vi.fn(),
      })),
    );
    testState.consoleStore!.loadOnboardingState.mockResolvedValueOnce({
      shouldShowOobe: true,
    });

    const { router, wrapper } = await mountAppShell();

    await flushPromises();
    expect(router.currentRoute.value.path).toBe("/oobe");
    expect(wrapper.find("[data-testid='oobe-route']").exists()).toBe(true);
    expect(testState.consoleStore?.initialize).toHaveBeenCalledTimes(1);
  });

  it("runs command-palette navigation, documentation, and refresh actions against the live shell stores", async () => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn(() => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    );
    const { router, setup, wrapper } = await mountAppShell();
    const actions = setup.palette.actions.value as Array<{
      id: string;
      run: () => void;
    }>;

    actions.find((action) => action.id === "nav.docs")!.run();
    actions.find((action) => action.id === "action.refresh")!.run();
    actions.find((action) => action.id === "nav.system")!.run();
    await flushPromises();

    expect(testState.docsLink?.openDocs).toHaveBeenCalledOnce();
    expect(testState.consoleStore?.loadSystemState).toHaveBeenCalledOnce();
    expect(router.currentRoute.value.path).toBe("/system");
    expect(document.title).toBe("系统 - JFTrade Console");

    setup.syncCompactAppShell({ matches: true });
    setup.toggleCompactNav();
    await nextTick();
    expect(wrapper.find("[data-testid='compact-nav-drawer']").exists()).toBe(true);
    setup.syncCompactAppShell({ matches: false });
    await nextTick();
    expect(wrapper.find("[data-testid='compact-nav-drawer']").exists()).toBe(false);
  });
});
