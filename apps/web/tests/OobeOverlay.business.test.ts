// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

const mocks = vi.hoisted(() => ({
  saveOnboardingState: vi.fn(),
  routerPush: vi.fn(),
  routerReplace: vi.fn(),
  createManagedBrokerAccount: vi.fn(),
  updateManagedBrokerAccount: vi.fn(),
  deleteManagedBrokerAccount: vi.fn(),
  importRuntimeAccount: vi.fn(),
  populateAccountForm: vi.fn(),
  removeAccount: vi.fn(),
  resetAccountForm: vi.fn(),
  submitAccount: vi.fn(),
}));

let consoleDataState: ReturnType<typeof createConsoleDataState>;

vi.mock("vue-router", () => ({
  useRouter: () => ({
    push: mocks.routerPush,
    replace: mocks.routerReplace,
  }),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => consoleDataState,
}));

vi.mock("../src/composables/settingsManagedAccounts", () => ({
  createSettingsManagedAccountsController: () => ({
    accountForm: ref({
      brokerId: "",
      accountId: "",
    }),
    deletingAccountId: ref(""),
    editingAccountId: ref(""),
    importRuntimeAccount: mocks.importRuntimeAccount,
    populateAccountForm: mocks.populateAccountForm,
    removeAccount: mocks.removeAccount,
    resetAccountForm: mocks.resetAccountForm,
    savingAccount: ref(false),
    submitAccount: mocks.submitAccount,
  }),
}));

import OobeOverlay from "../src/components/OobeOverlay.vue";

type SetupState = Record<string, unknown>;

const wrappers: VueWrapper[] = [];

function createConsoleDataState() {
  return {
    onboardingState: ref({
      state: {
        lastBrokerId: "",
      },
      recommendedBrokerId: "futu",
      brokers: [
        {
          descriptor: {
            id: "futu",
            displayName: "Futu OpenD",
          },
          available: true,
          configured: false,
        },
        {
          descriptor: {
            id: "ib",
            displayName: "Interactive Brokers",
          },
          available: true,
          configured: true,
        },
        {
          descriptor: {
            id: "alpaca",
            displayName: "Alpaca",
          },
          available: false,
          configured: false,
        },
      ],
    }),
    brokerSettings: ref({
      brokers: [],
      accounts: [],
    }),
    futuOpenDHealth: ref({
      diagnosis: {
        manualRetryRequired: false,
        summary: "",
      },
    }),
    brokerRuntime: ref({
      session: {
        connectivity: "disconnected",
        lastError: "",
      },
      accounts: [],
    }),
    createManagedBrokerAccount: mocks.createManagedBrokerAccount,
    updateManagedBrokerAccount: mocks.updateManagedBrokerAccount,
    deleteManagedBrokerAccount: mocks.deleteManagedBrokerAccount,
    saveOnboardingState: mocks.saveOnboardingState,
  };
}

function mountOobeOverlay() {
  const wrapper = mount(OobeOverlay, {
    global: {
      stubs: {
        RuntimeDependenciesSection: defineComponent({
          emits: ["status-change"],
          template: "<div data-testid='runtime-deps'></div>",
        }),
        FutuIntegrationSection: {
          template: "<div data-testid='futu-integration'>futu integration</div>",
        },
        SettingsAccountDiscoverySection: {
          props: ["accounts", "unavailableMessage"],
          template:
            "<div data-testid='account-discovery'>{{ unavailableMessage }} / {{ accounts.length }}</div>",
        },
        SettingsManagedAccountsSection: {
          props: ["accounts"],
          template:
            "<div data-testid='managed-accounts'>managed {{ accounts.length }}</div>",
        },
        "v-alert": {
          template: "<div class='v-alert'><slot /></div>",
        },
        "v-btn": {
          props: ["disabled", "loading", "variant", "color"],
          emits: ["click"],
          template:
            "<button type='button' :disabled='disabled' @click=\"$emit('click')\"><slot /></button>",
        },
      },
    },
  });
  wrappers.push(wrapper);
  return wrapper;
}

function panelSetup(wrapper: VueWrapper): SetupState {
  return wrapper.vm.$.setupState as SetupState;
}

function readSetupValue<T>(wrapper: VueWrapper, key: string): T {
  const value = panelSetup(wrapper)[key];
  if (value !== null && typeof value === "object" && "value" in value) {
    return (value as { value: T }).value;
  }
  return value as T;
}

function writeSetupValue<T>(wrapper: VueWrapper, key: string, value: T): void {
  const current = panelSetup(wrapper)[key];
  if (current !== null && typeof current === "object" && "value" in current) {
    (current as { value: T }).value = value;
    return;
  }
  panelSetup(wrapper)[key] = value;
}

function callSetup<T>(wrapper: VueWrapper, key: string, ...args: unknown[]): T {
  return (panelSetup(wrapper)[key] as (...values: unknown[]) => T)(...args);
}

async function flushOobe(): Promise<void> {
  await Promise.resolve();
  await nextTick();
}

beforeEach(() => {
  vi.clearAllMocks();
  consoleDataState = createConsoleDataState();
  mocks.saveOnboardingState.mockResolvedValue(undefined);
  mocks.routerPush.mockResolvedValue(undefined);
  mocks.routerReplace.mockResolvedValue(undefined);
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount();
  }
});

describe("OobeOverlay business flows", () => {
  it("uses dependency gating and the recommended broker to enter connection setup", async () => {
    consoleDataState.onboardingState.value.recommendedBrokerId = "ib";

    const wrapper = mountOobeOverlay();

    await callSetup<Promise<void>>(wrapper, "goToConnectionStep");
    expect(readSetupValue(wrapper, "activeStep")).toBe("dependencies");

    callSetup<void>(wrapper, "goToBrokerStep");
    expect(readSetupValue(wrapper, "dependencyWarningSkipped")).toBe(true);
    expect(readSetupValue(wrapper, "activeStep")).toBe("broker");

    await callSetup<Promise<void>>(wrapper, "goToConnectionStep");

    expect(mocks.saveOnboardingState).toHaveBeenCalledWith({
      completed: false,
      lastBrokerId: "ib",
    });
    expect(readSetupValue(wrapper, "selectedBrokerId")).toBe("ib");
    expect(readSetupValue(wrapper, "activeStep")).toBe("connection");
  });

  it("renders broker selection cards and persists a clicked broker choice", async () => {
    const wrapper = mountOobeOverlay();
    writeSetupValue(wrapper, "activeStep", "broker");
    writeSetupValue(wrapper, "runtimeDependenciesSatisfied", true);
    await nextTick();

    const buttons = wrapper.findAll("button");
    const brokerButton = buttons.find((button) =>
      button.text().includes("Interactive Brokers"),
    );
    const disabledBrokerButton = buttons.find((button) =>
      button.text().includes("Alpaca"),
    );

    expect(disabledBrokerButton?.attributes("disabled")).toBeDefined();

    await brokerButton?.trigger("click");

    expect(mocks.saveOnboardingState).toHaveBeenCalledWith({
      completed: false,
      lastBrokerId: "ib",
    });
    expect(readSetupValue(wrapper, "selectedBrokerId")).toBe("ib");
    expect(wrapper.text()).toContain("当前券商：Interactive Brokers");
  });

  it("shows blocking connection guidance for a saved but disconnected futu integration", async () => {
    consoleDataState.onboardingState.value.state.lastBrokerId = "futu";
    consoleDataState.brokerSettings.value = {
      brokers: [
        {
          descriptor: {
            id: "futu",
          },
          integration: {
            enabled: true,
          },
        },
      ],
      accounts: [],
    };
    consoleDataState.futuOpenDHealth.value.diagnosis = {
      manualRetryRequired: true,
      summary: "OpenD 登录状态异常",
    };
    consoleDataState.brokerRuntime.value.session = {
      connectivity: "disconnected",
      lastError: "socket down",
    };

    const wrapper = mountOobeOverlay();
    writeSetupValue(wrapper, "activeStep", "connection");
    await nextTick();

    expect(wrapper.text()).toContain("检测到券商连接中断");
    expect(wrapper.text()).toContain("OpenD 登录状态异常");
    expect(readSetupValue(wrapper, "connectionStatusLabel")).toBe("未连接");
    expect(readSetupValue(wrapper, "accountStepHint")).toBe(
      "等待 OpenD 连接成功后，才能进入账户确认步骤。",
    );
    expect(readSetupValue(wrapper, "accountDiscoveryUnavailableMessage")).toBe(
      "OpenD 尚未连接成功。连接恢复后，这里会显示发现到的账户。",
    );
  });

  it("enters the account step only after futu connectivity is ready and falls back when it breaks again", async () => {
    consoleDataState.onboardingState.value.state.lastBrokerId = "futu";
    consoleDataState.brokerSettings.value = {
      brokers: [
        {
          descriptor: {
            id: "futu",
          },
          integration: {
            enabled: true,
          },
        },
      ],
      accounts: [{ accountId: "REAL-001" }],
    };
    consoleDataState.brokerRuntime.value.session = {
      connectivity: "connected",
      lastError: "",
    };

    const wrapper = mountOobeOverlay();

    callSetup<void>(wrapper, "handleRuntimeDependencyStatus", {
      allRequiredSatisfied: true,
    });
    await callSetup<Promise<void>>(wrapper, "goToAccountStep");

    expect(readSetupValue(wrapper, "canEnterAccountStep")).toBe(true);
    expect(readSetupValue(wrapper, "activeStep")).toBe("account");

    consoleDataState.brokerRuntime.value.session = {
      connectivity: "degraded",
      lastError: "lagging",
    };
    await nextTick();

    expect(readSetupValue(wrapper, "activeStep")).toBe("connection");
  });

  it("completes onboarding and can continue into settings after dismissal", async () => {
    consoleDataState.onboardingState.value.state.lastBrokerId = "futu";

    const wrapper = mountOobeOverlay();

    await callSetup<Promise<void>>(wrapper, "completeOnboarding", false);

    expect(mocks.saveOnboardingState).toHaveBeenCalledWith({
      completed: true,
      dismissed: false,
      lastBrokerId: "futu",
    });
    expect(mocks.routerReplace).toHaveBeenCalledWith("/workspace");
    expect(readSetupValue(wrapper, "savingOnboarding")).toBe(false);

    mocks.saveOnboardingState.mockClear();
    mocks.routerReplace.mockClear();
    mocks.routerPush.mockClear();

    await callSetup<Promise<void>>(wrapper, "openSettings");

    expect(mocks.saveOnboardingState).toHaveBeenCalledWith({
      completed: true,
      dismissed: true,
      lastBrokerId: "futu",
    });
    expect(mocks.routerReplace).toHaveBeenCalledWith("/workspace");
    expect(mocks.routerPush).toHaveBeenCalledWith("/settings");
    expect(readSetupValue(wrapper, "savingOnboarding")).toBe(false);
  });

  it("reacts to onboarding broker updates and keeps disabled futu integrations gated", async () => {
    const wrapper = mountOobeOverlay();

    consoleDataState.onboardingState.value.state.lastBrokerId = "futu";
    await nextTick();
    expect(readSetupValue(wrapper, "selectedBrokerId")).toBe("futu");

    consoleDataState.brokerSettings.value = {
      brokers: [
        {
          descriptor: {
            id: "futu",
          },
          integration: {
            enabled: false,
          },
        },
      ],
      accounts: [],
    };
    writeSetupValue(wrapper, "activeStep", "connection");
    await nextTick();

    expect(readSetupValue(wrapper, "connectionStatusLabel")).toBe("已停用");
    expect(readSetupValue(wrapper, "accountStepHint")).toBe(
      "当前富途接入已停用。启用并保存后，才能继续确认账户。",
    );
    expect(readSetupValue(wrapper, "accountDiscoveryUnavailableMessage")).toBe(
      "当前富途接入已停用。启用并保存后，JFTrade 才会尝试发现 OpenD 账户。",
    );

    await callSetup<Promise<void>>(wrapper, "goToAccountStep");
    expect(readSetupValue(wrapper, "activeStep")).toBe("dependencies");

    callSetup<void>(wrapper, "handleRuntimeDependencyStatus", {
      allRequiredSatisfied: true,
    });
    writeSetupValue(wrapper, "selectedBrokerId", "");
    consoleDataState.onboardingState.value.recommendedBrokerId = "";

    await callSetup<Promise<void>>(wrapper, "goToAccountStep");

    expect(mocks.saveOnboardingState).toHaveBeenCalledWith({
      completed: false,
      lastBrokerId: "futu",
    });
    expect(readSetupValue(wrapper, "activeStep")).toBe("connection");
  });

  it("wires header, step, and footer buttons to the expected navigation actions", async () => {
    consoleDataState.onboardingState.value.state.lastBrokerId = "futu";
    consoleDataState.brokerSettings.value = {
      brokers: [
        {
          descriptor: {
            id: "futu",
          },
          integration: {
            enabled: true,
          },
        },
      ],
      accounts: [{ accountId: "REAL-001" }],
    };
    consoleDataState.brokerRuntime.value.session = {
      connectivity: "connected",
      lastError: "",
    };

    const wrapper = mountOobeOverlay();
    callSetup<void>(wrapper, "handleRuntimeDependencyStatus", {
      allRequiredSatisfied: true,
    });
    await nextTick();

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("跳过"))
      ?.trigger("click");
    await flushOobe();
    expect(mocks.saveOnboardingState).toHaveBeenCalledWith({
      completed: true,
      dismissed: true,
      lastBrokerId: "futu",
    });

    mocks.saveOnboardingState.mockClear();
    mocks.routerReplace.mockClear();
    mocks.routerPush.mockClear();

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("进入工作台"))
      ?.trigger("click");
    await flushOobe();
    expect(mocks.saveOnboardingState).toHaveBeenCalledWith({
      completed: true,
      dismissed: false,
      lastBrokerId: "futu",
    });

    writeSetupValue(wrapper, "activeStep", "broker");
    await nextTick();
    const stepButtons = wrapper
      .findAll(".oobe-steps button")
      .filter((button) => button.text().trim() !== "");
    await stepButtons[0]?.trigger("click");
    expect(readSetupValue(wrapper, "activeStep")).toBe("dependencies");

    await stepButtons[1]?.trigger("click");
    expect(readSetupValue(wrapper, "activeStep")).toBe("broker");

    const panels = wrapper.findAll(".oobe-panel");

    await panels[1]?.find(".oobe-footer-actions button")?.trigger("click");
    expect(readSetupValue(wrapper, "activeStep")).toBe("dependencies");

    writeSetupValue(wrapper, "activeStep", "connection");
    await nextTick();
    await panels[2]?.find(".oobe-footer-actions button")?.trigger("click");
    expect(readSetupValue(wrapper, "activeStep")).toBe("broker");

    writeSetupValue(wrapper, "activeStep", "account");
    await nextTick();
    await panels[3]?.find(".oobe-footer-actions button")?.trigger("click");
    expect(readSetupValue(wrapper, "activeStep")).toBe("connection");

    mocks.saveOnboardingState.mockClear();
    mocks.routerReplace.mockClear();

    await panels[3]?.findAll(".oobe-footer-actions button")[2]?.trigger("click");
    await flushOobe();

    expect(mocks.saveOnboardingState).toHaveBeenCalledWith({
      completed: true,
      dismissed: false,
      lastBrokerId: "futu",
    });
    expect(mocks.routerReplace).toHaveBeenCalledWith("/workspace");
  });
});
