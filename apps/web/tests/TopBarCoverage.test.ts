// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, ref } from "vue";

const state = vi.hoisted(() => ({
  console: null as any,
  notifications: null as any,
  palette: null as any,
  workspace: null as any,
  theme: null as any,
  webLogout: vi.fn(),
  loadMarketProfiles: vi.fn(),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => state.console,
}));
vi.mock("../src/composables/useNotifications", () => ({
  useNotifications: () => state.notifications,
}));
vi.mock("../src/composables/useCommandPalette", () => ({
  useCommandPalette: () => state.palette,
}));
vi.mock("../src/composables/useWorkspaceLayout", () => ({
  useWorkspaceTradingPrefs: () => state.workspace,
  useWorkspaceViewState: () => state.workspace,
}));
vi.mock("../src/composables/useTheme", () => ({
  useTheme: () => state.theme,
}));
vi.mock("../src/composables/marketProfiles", () => ({
  useMarketProfiles: () => ({ loadMarketProfiles: state.loadMarketProfiles }),
}));
vi.mock("../src/composables/apiClient", () => ({ webLogout: () => state.webLogout() }));
vi.mock("../src/runtimeConfig", () => ({ resolveDesktopMode: () => false }));

import TopBar from "../src/layout/TopBar.vue";

beforeEach(() => {
  vi.clearAllMocks();
  state.console = {
    availableBrokerAccounts: ref([
      {
        selectionKey: "sim-futu-1",
        brokerId: "futu",
        displayName: "模拟主账户",
        accountId: "SIM-001",
        market: "HK",
        securityFirm: "富途",
        tradingEnvironment: "SIMULATE",
      },
      {
        selectionKey: "real-ib-1",
        brokerId: "ib",
        displayName: "实盘账户",
        accountId: "REAL-001",
        market: "US",
        securityFirm: null,
        tradingEnvironment: "REAL",
      },
    ]),
    selectedBrokerAccount: ref(null),
    systemStatus: ref({ defaultTradingEnvironment: "SIMULATE" }),
    selectWorkspaceInstrument: vi.fn(),
    selectBrokerAccount: vi.fn(async () => {}),
  };
  state.notifications = { unreadCount: ref(2), push: vi.fn() };
  state.palette = { show: vi.fn() };
  state.workspace = {
    prefs: ref({ favoriteBrokerAccountKeys: [] as string[] }),
    update: vi.fn(),
    updateViewState: vi.fn(),
  };
  state.theme = { theme: ref("light"), toggle: vi.fn() };
  state.webLogout.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("TopBar business coverage", () => {
  it("exposes compact back, forward, and current-view refresh controls", async () => {
    const wrapper = mountTopBar({
      compact: true,
      canGoBack: true,
      canGoForward: true,
    });

    await wrapper.get('[data-testid="topbar-navigation-back"]').trigger("click");
    await wrapper
      .get('[data-testid="topbar-navigation-forward"]')
      .trigger("click");
    await wrapper
      .get('[data-testid="topbar-navigation-refresh"]')
      .trigger("click");
    const refreshIcon = wrapper.get(
      '[data-testid="topbar-navigation-refresh"] svg',
    );

    expect(wrapper.emitted("navigate-back")).toHaveLength(1);
    expect(wrapper.emitted("navigate-forward")).toHaveLength(1);
    expect(wrapper.emitted("refresh-view")).toHaveLength(1);
    expect(refreshIcon.classes()).toContain(
      "app-navigation-controls__refresh-icon",
    );
    expect(refreshIcon.findAll("path")).toHaveLength(4);
    expect(wrapper.get(".app-navigation-controls").classes()).toContain(
      "app-navigation-controls--compact",
    );

    const unavailable = mountTopBar();
    expect(
      unavailable
        .get('[data-testid="topbar-navigation-back"]')
        .attributes("disabled"),
    ).toBeDefined();
    expect(
      unavailable
        .get('[data-testid="topbar-navigation-forward"]')
        .attributes("disabled"),
    ).toBeDefined();
    expect(
      unavailable
        .get('[data-testid="topbar-navigation-refresh"]')
        .attributes("disabled"),
    ).toBeUndefined();
  });

  it("filters and selects the intended account while preserving a user-pinned trading environment", async () => {
    const wrapper = mountTopBar();
    expect(wrapper.get('[data-testid="topbar-broker-account-picker-open"]').text()).toContain(
      "请选择模拟盘账户",
    );

    await wrapper.get('[data-testid="topbar-broker-account-picker-open"]').trigger("click");
    await wrapper.get('[data-testid="topbar-broker-account-filter"]').setValue("not-found");
    expect(wrapper.get('[data-testid="topbar-broker-account-picker-empty"]').text()).toContain(
      "筛选后暂无模拟盘账户",
    );

    await wrapper.get('[data-testid="topbar-broker-account-filter"]').setValue("SIM-001");
    await wrapper.get('[data-testid="topbar-broker-account-item-favorite"]').trigger("click");
    expect(state.workspace.update).toHaveBeenCalledWith({
      favoriteBrokerAccountKeys: ["sim-futu-1"],
    });

    await wrapper.get(".tv-topbar-account-picker__item-main").trigger("click");
    expect(state.console.selectBrokerAccount).toHaveBeenCalledWith("sim-futu-1");
    expect(wrapper.find('[data-testid="topbar-broker-account-picker-dialog"]').exists()).toBe(false);

    await wrapper.get('[data-testid="topbar-trading-environment-real"]').trigger("click");
    expect(state.console.selectBrokerAccount).toHaveBeenCalledWith("real-ib-1");
  });

  it("opens workspace utilities, resolves a searched instrument, and reports a failed web logout", async () => {
    state.webLogout.mockRejectedValueOnce(new Error("会话服务不可用"));
    const wrapper = mountTopBar({ compact: true });

    await wrapper.get('[data-testid="topbar-compact-nav-toggle"]').trigger("click");
    expect(wrapper.emitted("toggle-nav")).toHaveLength(1);
    await wrapper.get(".topbar-search-stub").trigger("click");
    expect(state.console.selectWorkspaceInstrument).toHaveBeenCalledWith({
      market: "US",
      symbol: "AAPL",
    });
    await wrapper.get('button[title="命令面板（⌘K / Ctrl+K）"]').trigger("click");
    expect(state.palette.show).toHaveBeenCalledOnce();
    await wrapper.get('button[title="通知"]').trigger("click");
    await wrapper.get('button[title="AI 助手"]').trigger("click");
    expect(state.workspace.update).toHaveBeenNthCalledWith(1, {
      rightDockOpen: true,
      rightDockTab: "notifications",
    });
    expect(state.workspace.update).toHaveBeenNthCalledWith(2, {
      rightDockOpen: true,
      rightDockTab: "ai",
    });

    await wrapper.get('[data-testid="web-logout"]').trigger("click");
    await vi.waitFor(() => expect(state.notifications.push).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "退出 Web 登录失败",
        message: "会话服务不可用",
      }),
    ));
    expect(state.loadMarketProfiles).toHaveBeenCalledOnce();
  });
});

function mountTopBar(
  props: {
    compact?: boolean;
    canGoBack?: boolean;
    canGoForward?: boolean;
  } = {},
) {
  return mount(TopBar, {
    props,
    global: {
      stubs: {
        InstrumentSearchBox: defineComponent({
          emits: ["select"],
          template: "<button type='button' class='topbar-search-stub' @click=\"$emit('select', { market: 'US', code: 'AAPL' })\">search</button>",
        }),
        "v-dialog": defineComponent({
          props: ["modelValue"],
          template: "<div v-if=\"modelValue\"><slot /></div>",
        }),
        "v-card": { template: "<div><slot /></div>" },
        "v-card-title": { template: "<div><slot /></div>" },
        "v-card-text": { template: "<div><slot /></div>" },
        "v-btn-toggle": { template: "<div><slot /></div>" },
        "v-btn": defineComponent({
          emits: ["click"],
          template: "<button type='button' v-bind=\"$attrs\" @click=\"$emit('click')\"><slot /></button>",
        }),
        "v-text-field": defineComponent({
          props: ["modelValue"],
          emits: ["update:modelValue"],
          template: "<input v-bind=\"$attrs\" :value=\"modelValue\" @input=\"$emit('update:modelValue', $event.target.value)\">",
        }),
      },
    },
  });
}
