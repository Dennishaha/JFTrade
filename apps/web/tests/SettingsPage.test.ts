// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { describe, expect, it, vi } from "vitest";

const consoleState = vi.hoisted(() => ({
  brokerRuntime: {
    value: { session: { connectivity: "disconnected" }, accounts: [] },
  },
  brokerSettings: {
    value: { accounts: [], brokers: [] },
  },
  systemStatus: {
    __v_isRef: true,
    value: {
      build: {
        version: "dev",
        commit: "unknown",
        buildTime: "dev",
        goos: "",
        goarch: "",
      },
      observability: {
        requests: {
          recentErrors: [],
          recentSlowRequests: [],
          slowThresholdMs: 750,
          minimumImportance: "low",
          openD: { totalCalls: 0, failedCalls: 0 },
        },
      },
    },
  },
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => ({
    brokerRuntime: consoleState.brokerRuntime,
    brokerSettings: consoleState.brokerSettings,
    systemStatus: consoleState.systemStatus,
    createManagedBrokerAccount: vi.fn(),
    deleteManagedBrokerAccount: vi.fn(),
    updateManagedBrokerAccount: vi.fn(),
  }),
}));

vi.mock("../src/composables/settingsManagedAccounts", async () => {
  const { ref } = await import("vue");
  return {
    createSettingsManagedAccountsController: () => ({
      accountForm: ref({}),
      deletingAccountId: ref(""),
      editingAccountId: ref(null),
      importRuntimeAccount: vi.fn(),
      populateAccountForm: vi.fn(),
      removeAccount: vi.fn(),
      resetAccountForm: vi.fn(),
      savingAccount: ref(false),
      submitAccount: vi.fn(),
    }),
  };
});

import SettingsPage from "../src/pages/SettingsPage.vue";

const sectionStub = (name: string) => ({
  name,
  template: `<section data-section='${name}'><slot /></section>`,
});

const accountDiscoveryStub = {
  props: ["unavailableMessage"],
  template: "<section data-section='account-discovery'>{{ unavailableMessage }}</section>",
};

function mountSettings(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/settings/:section?", component: SettingsPage }],
  });
  return router.push(path).then(async () => {
    await router.isReady();
    const wrapper = mount(SettingsPage, {
      global: {
        plugins: [router],
        stubs: {
          RuntimeDependenciesSection: sectionStub("runtime-dependencies"),
          FutuIntegrationSection: sectionStub("futu-integration"),
          SettingsManagedAccountsSection: sectionStub("managed-accounts"),
          SettingsAccountDiscoverySection: accountDiscoveryStub,
          SettingsAppearanceSection: sectionStub("appearance"),
          SettingsExchangeCalendarSection: sectionStub("exchange-calendars"),
          SettingsSecuritySection: sectionStub("security"),
          SettingsSystemNotificationsSection: sectionStub("system-notifications"),
          SettingsPineWorkerSection: sectionStub("pine-worker"),
          SettingsADKSection: sectionStub("adk"),
          SettingsDeveloperToolsSection: sectionStub("developer-tools"),
          SettingsDataManagementSection: sectionStub("data-management"),
          SettingsOpenSourceSection: sectionStub("open-source"),
        },
      },
    });
    await flushPromises();
    return { router, wrapper };
  });
}

describe("SettingsPage", () => {
  it("keeps settings routes, navigation, and the persisted last section in sync", async () => {
    window.localStorage.clear();
    const { router, wrapper } = await mountSettings("/settings/security");
    expect(wrapper.get("[data-section='security']").exists()).toBe(true);

    const notifications = wrapper.findAll("button").find((button) =>
      button.text() === "系统通知",
    );
    if (notifications == null) throw new Error("missing notifications menu item");
    await notifications.trigger("click");
    await flushPromises();
    expect(router.currentRoute.value.path).toBe("/settings/system-notifications");
    expect(window.localStorage.getItem("jft.settings.section")).toBe("system-notifications");

    await wrapper.get("select").setValue("pine-worker");
    await flushPromises();
    expect(router.currentRoute.value.path).toBe("/settings/pine-worker");
    expect(window.localStorage.getItem("jft.settings.section")).toBe("pine-worker");

    const developerTools = wrapper.findAll("button").find((button) =>
      button.text() === "开发者工具",
    );
    if (developerTools == null) throw new Error("missing developer tools menu item");
    await developerTools.trigger("click");
    await flushPromises();
    expect(router.currentRoute.value.path).toBe("/settings/developer-tools");
    expect(window.localStorage.getItem("jft.settings.section")).toBe("developer-tools");
    expect(wrapper.get("[data-section='developer-tools']").exists()).toBe(true);

    const openSource = wrapper.findAll("button").find((button) =>
      button.text() === "开源许可",
    );
    if (openSource == null) throw new Error("missing open-source menu item");
    await openSource.trigger("click");
    await flushPromises();
    expect(router.currentRoute.value.path).toBe("/settings/open-source");
    expect(wrapper.get("[data-section='open-source']").exists()).toBe(true);
  });

  it("normalizes legacy, missing, and unknown sections before rendering settings", async () => {
    window.localStorage.setItem("jft.settings.section", "security");
    const legacy = await mountSettings("/settings/data-migration");
    expect(legacy.router.currentRoute.value.path).toBe("/settings/data-management");
    expect(window.localStorage.getItem("jft.settings.section")).toBe("data-management");

    window.localStorage.setItem("jft.settings.section", "security");
    const missing = await mountSettings("/settings");
    expect(missing.router.currentRoute.value.path).toBe("/settings/security");

    const unknown = await mountSettings("/settings/unknown-section");
    expect(unknown.router.currentRoute.value.path).toBe("/settings/runtime-dependencies");
    expect(window.localStorage.getItem("jft.settings.section")).toBe("runtime-dependencies");
  });

  it("explains why runtime account discovery is unavailable", async () => {
    consoleState.brokerSettings.value = { accounts: [], brokers: [] };
    const missingIntegration = await mountSettings("/settings/account-discovery");
    expect(missingIntegration.wrapper.text()).toContain("请先在富途接入中填写");

    consoleState.brokerSettings.value = {
      accounts: [],
      brokers: [{ descriptor: { id: "futu" }, integration: { enabled: false } }],
    };
    const disabledIntegration = await mountSettings("/settings/account-discovery");
    expect(disabledIntegration.wrapper.text()).toContain("当前富途接入已停用");

    consoleState.brokerSettings.value = {
      accounts: [],
      brokers: [{ descriptor: { id: "futu" }, integration: { enabled: true } }],
    };
    const disconnected = await mountSettings("/settings/account-discovery");
    expect(disconnected.wrapper.text()).toContain("OpenD 尚未连接成功");
  });
});
