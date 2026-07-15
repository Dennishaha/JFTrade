// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

const stores = vi.hoisted(() => ({
  consoleData: null as ReturnType<typeof createConsoleDataState> | null,
}));
const externalLinks = vi.hoisted(() => ({
  handleExternalLinkClick: vi.fn(),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => stores.consoleData,
}));

vi.mock("../src/composables/externalLink", () => ({
  useExternalLink: () => externalLinks,
}));

import FutuIntegrationSection from "../src/components/FutuIntegrationSection.vue";

const sectionHeaderStub = defineComponent({
  props: ["title", "description"],
  template: "<div><h2>{{ title }}</h2><p>{{ description }}</p></div>",
});

const chipStub = {
  template: "<span class='chip-stub'><slot /></span>",
};

const alertStub = defineComponent({
  props: ["title", "type"],
  template:
    "<div class='alert-stub'><strong v-if='title'>{{ title }}</strong><slot /></div>",
});

const buttonStub = defineComponent({
  props: ["loading", "color"],
  emits: ["click"],
  template:
    "<button type='button' :data-color='color' @click=\"$emit('click')\"><slot /></button>",
});

const switchStub = defineComponent({
  props: ["modelValue", "label"],
  emits: ["update:modelValue"],
  template:
    "<label><input type='checkbox' :checked='!!modelValue' @change=\"$emit('update:modelValue', $event.target.checked)\" />{{ label }}</label>",
});

const textFieldStub = defineComponent({
  props: ["modelValue", "type", "placeholder"],
  emits: ["update:modelValue"],
  template:
    "<input :type='type || \"text\"' :placeholder='placeholder' :value='modelValue ?? \"\"' @input=\"$emit('update:modelValue', $event.target.value)\" />",
});

function createConsoleDataState() {
  return {
    brokerRuntime: ref({
      session: {
        connectivity: "disconnected",
        checkedAt: "",
        lastError: null,
        globalState: null,
      },
    }),
    brokerSettings: ref({
      brokers: [
        {
          descriptor: { id: "futu" },
          integration: null,
          defaults: {
            host: "127.0.0.1",
            apiPort: 11110,
            websocketPort: 11111,
            maxWebSocketConnections: 20,
            websocketKey: "",
            tradeMarket: "HK",
            securityFirm: "FUTUSECURITIES",
          },
        },
      ],
    }),
    futuOpenDHealth: ref({
      diagnosis: {
        manualRetryRequired: false,
        restartOpenDRecommended: false,
        summary: "OpenD healthy",
      },
      localSocketDiagnostics: {
        websocketEstablishedConnections: 0,
        topClientProcesses: [],
      },
    }),
    isLoading: ref(false),
    loadSystemState: vi.fn().mockResolvedValue(undefined),
    requestFutuOpenDManualRetry: vi.fn().mockResolvedValue(undefined),
    saveBrokerIntegration: vi.fn().mockResolvedValue(undefined),
    unsubscribeAllMarketData: vi.fn().mockResolvedValue(undefined),
  };
}

function mountSection(mode: "oobe" | "settings" = "settings") {
  return mount(FutuIntegrationSection, {
    props: { mode },
    global: {
      stubs: {
        SectionHeader: sectionHeaderStub,
        "v-chip": chipStub,
        "v-alert": alertStub,
        "v-btn": buttonStub,
        "v-switch": switchStub,
        "v-text-field": textFieldStub,
      },
    },
    attachTo: document.body,
  });
}

afterEach(() => {
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});

describe("FutuIntegrationSection", () => {
  it("guides pending setup in oobe mode and saves the edited integration form", async () => {
    stores.consoleData = createConsoleDataState();
    const wrapper = mountSection("oobe");

    expect(wrapper.text()).toContain("填写并保存富途接入配置后");
    expect(wrapper.text()).toContain("先填写连接信息并保存");
    expect(wrapper.text()).toContain("保存并检测 OpenD");
    expect(wrapper.text()).toContain("最低 10.8.6808");
    expect(
      wrapper.get('a[href="https://www.futunn.com/download/OpenAPI"]').text(),
    ).toBe("下载或升级 OpenD");
    await wrapper
      .get('a[href="https://www.futunn.com/download/OpenAPI"]')
      .trigger("click");
    expect(externalLinks.handleExternalLinkClick).toHaveBeenCalledWith(
      expect.anything(),
      "https://www.futunn.com/download/OpenAPI",
    );

    const inputs = wrapper.findAll("input");
    await inputs[1]?.setValue("192.168.0.8");
    await inputs[5]?.setValue("US");
    await inputs[6]?.setValue("FUTUHK");
    await inputs[7]?.setValue("socket-secret");
    await wrapper.find('input[type="checkbox"]').setValue(true);

    await wrapper.findAll("button").at(-1)?.trigger("click");

    expect(stores.consoleData.saveBrokerIntegration).toHaveBeenCalledWith(
      "futu",
      {
        enabled: true,
        config: {
          type: "futu",
          host: "192.168.0.8",
          apiPort: 11110,
          websocketPort: 11111,
          maxWebSocketConnections: 20,
          useEncryption: false,
          websocketKey: "socket-secret",
          tradeMarket: "US",
          securityFirm: "FUTUHK",
        },
      },
    );
  });

  it("shows runtime diagnostics, manual retry guidance, and action buttons for enabled integrations", async () => {
    stores.consoleData = createConsoleDataState();
    stores.consoleData.brokerSettings.value.brokers[0] = {
      ...stores.consoleData.brokerSettings.value.brokers[0],
      integration: {
        enabled: true,
        config: {
          host: "127.0.0.1",
          apiPort: 11110,
          websocketPort: 11111,
          maxWebSocketConnections: 20,
          websocketKey: "secret",
          tradeMarket: "HK",
          securityFirm: "FUTUSECURITIES",
        },
      },
    };
    stores.consoleData.brokerRuntime.value.session = {
      connectivity: "degraded",
      checkedAt: "2026-07-03T09:00:00.000Z",
      lastError: null,
      globalState: {
        quoteLoggedIn: true,
        tradeLoggedIn: false,
        programStatus: "READY",
      },
    };
    stores.consoleData.futuOpenDHealth.value = {
      diagnosis: {
        manualRetryRequired: true,
        restartOpenDRecommended: false,
        summary: "OpenD websocket pool is exhausted",
      },
      localSocketDiagnostics: {
        websocketEstablishedConnections: 2,
        topClientProcesses: [
          {
            processName: "node",
            pid: 4321,
            establishedConnections: 2,
          },
        ],
      },
    };

    const wrapper = mountSection("settings");

    expect(wrapper.text()).toContain("OpenD websocket pool is exhausted");
    expect(wrapper.text()).toContain("node(4321) x2");
    expect(wrapper.text()).toContain("行情登录");
    expect(wrapper.text()).toContain("已登录");
    expect(wrapper.text()).toContain("交易登录");
    expect(wrapper.text()).toContain("未登录");
    expect(wrapper.text()).toContain("清理闲置网页订阅");

    const buttons = wrapper.findAll("button");
    await buttons[0]?.trigger("click");
    await buttons[1]?.trigger("click");
    await buttons.at(-2)?.trigger("click");

    expect(stores.consoleData.loadSystemState).toHaveBeenCalledWith({
      bypassCooldown: true,
    });
    expect(stores.consoleData.requestFutuOpenDManualRetry).toHaveBeenCalled();
    expect(stores.consoleData.unsubscribeAllMarketData).toHaveBeenCalled();
  });

  it("shows the detected build and explicit upgrade guidance for unsupported OpenD", () => {
    stores.consoleData = createConsoleDataState();
    stores.consoleData.brokerSettings.value.brokers[0] = {
      ...stores.consoleData.brokerSettings.value.brokers[0],
      integration: {
        enabled: true,
        config: {
          host: "127.0.0.1",
          apiPort: 11110,
          websocketPort: 11111,
          maxWebSocketConnections: 20,
          websocketKey: "",
          tradeMarket: "HK",
          securityFirm: "FUTUSECURITIES",
        },
      },
    };
    stores.consoleData.brokerRuntime.value.session = {
      connectivity: "degraded",
      checkedAt: "2026-07-13T12:00:00.000Z",
      lastError: "OpenD 版本 10.8.6708 低于最低支持版本 10.8.6808",
      globalState: null,
    };
    stores.consoleData.futuOpenDHealth.value = {
      runtime: {
        serverVersion: "10.8.6708",
        minimumVersion: "10.8.6808",
      },
      diagnosis: {
        code: "OPEND_VERSION_UNSUPPORTED",
        manualRetryRequired: true,
        restartOpenDRecommended: false,
        summary: "OpenD 版本 10.8.6708 低于最低支持版本 10.8.6808",
      },
      localSocketDiagnostics: {
        websocketEstablishedConnections: 0,
        topClientProcesses: [],
      },
    };

    const wrapper = mountSection("settings");

    expect(wrapper.text()).toContain("OpenD 版本不受支持");
    expect(wrapper.text()).toContain("10.8.6708（最低 10.8.6808）");
    expect(wrapper.text()).toContain("请升级至 OpenD 10.8.6808 或更高版本");
  });

  it("switches between disabled, warning, error, and success runtime messaging", async () => {
    stores.consoleData = createConsoleDataState();
    stores.consoleData.brokerSettings.value.brokers[0] = {
      ...stores.consoleData.brokerSettings.value.brokers[0],
      integration: {
        enabled: false,
        config: {
          host: "127.0.0.1",
          apiPort: 11110,
          websocketPort: 11111,
          maxWebSocketConnections: 20,
          websocketKey: "",
          tradeMarket: "HK",
          securityFirm: "FUTUSECURITIES",
        },
      },
    };

    const wrapper = mountSection("settings");
    expect(wrapper.text()).toContain("当前已保存但未启用");

    stores.consoleData.brokerSettings.value.brokers[0] = {
      ...stores.consoleData.brokerSettings.value.brokers[0],
      integration: {
        enabled: true,
        config: {
          host: "127.0.0.1",
          apiPort: 11110,
          websocketPort: 11111,
          maxWebSocketConnections: 20,
          websocketKey: "",
          tradeMarket: "HK",
          securityFirm: "FUTUSECURITIES",
        },
      },
    };
    stores.consoleData.brokerRuntime.value.session = {
      connectivity: "degraded",
      checkedAt: "2026-07-03T09:00:00.000Z",
      lastError: "OpenD is not reachable",
      globalState: {
        quoteLoggedIn: null,
        tradeLoggedIn: null,
        programStatus: null,
      },
    };
    await nextTick();

    expect(wrapper.text()).toContain("OpenD 连接错误");
    expect(wrapper.text()).toContain("OpenD is not reachable");

    stores.consoleData.brokerRuntime.value.session = {
      connectivity: "connected",
      checkedAt: "2026-07-03T09:00:00.000Z",
      lastError: null,
      globalState: {
        quoteLoggedIn: true,
        tradeLoggedIn: true,
        programStatus: "READY",
      },
    };
    stores.consoleData.futuOpenDHealth.value = {
      diagnosis: {
        manualRetryRequired: false,
        restartOpenDRecommended: true,
        summary: "Restart OpenD to recycle stale websocket clients",
      },
      localSocketDiagnostics: {
        websocketEstablishedConnections: 0,
        topClientProcesses: [],
      },
    };
    await nextTick();

    expect(wrapper.text()).toContain("建议重启 OpenD 后再手动重试");
    expect(wrapper.text()).toContain(
      "Restart OpenD to recycle stale websocket clients",
    );

    stores.consoleData.futuOpenDHealth.value.diagnosis.restartOpenDRecommended =
      false;
    await nextTick();

    expect(wrapper.text()).toContain("OpenD 已连接");
  });

  it("uses settings copy before setup and keeps form defaults when no Futu broker exists", () => {
    stores.consoleData = createConsoleDataState();
    stores.consoleData.brokerSettings.value.brokers = [];

    const wrapper = mountSection("settings");

    expect(wrapper.text()).toContain("当前还没有已保存的富途接入配置");
    expect(wrapper.text()).toContain(
      "填写并保存富途接入配置后，这里会显示 OpenD 连接状态与诊断信息",
    );
    expect(wrapper.text()).toContain("保存富途配置");
    wrapper.unmount();
  });

  it("covers disconnected and unknown runtime states plus numeric form fallbacks", async () => {
    stores.consoleData = createConsoleDataState();
    stores.consoleData.brokerSettings.value.brokers[0] = {
      ...stores.consoleData.brokerSettings.value.brokers[0],
      integration: {
        enabled: true,
        config: {
          host: "localhost",
          websocketKey: "",
          tradeMarket: "US",
          securityFirm: "FUTUSECURITIES",
        } as never,
      },
    };
    stores.consoleData.brokerRuntime.value.session = {
      connectivity: "disconnected",
      checkedAt: "",
      lastError: null,
      globalState: {
        quoteLoggedIn: false,
        tradeLoggedIn: true,
        programStatus: "READY",
      },
    };
    stores.consoleData.futuOpenDHealth.value = {
      runtime: { serverVersion: "" },
      diagnosis: {
        manualRetryRequired: true,
        restartOpenDRecommended: false,
        summary: "retry required",
      },
      localSocketDiagnostics: {
        websocketEstablishedConnections: 1,
        topClientProcesses: [],
      },
    };

    const wrapper = mountSection("settings");
    expect(wrapper.text()).toContain("尚未检测");
    expect(wrapper.text()).toContain("未登录");
    expect(wrapper.text()).toContain("已登录");
    expect(wrapper.text()).toContain("待检测（最低 10.8.6808）");
    expect(wrapper.text()).toContain("1 条本地已建立 WebSocket 连接");

    const inputs = wrapper.findAll("input");
    expect((inputs[2]?.element as HTMLInputElement).value).toBe("11110");
    expect((inputs[3]?.element as HTMLInputElement).value).toBe("11111");
    expect((inputs[4]?.element as HTMLInputElement).value).toBe("20");
    await inputs[2]?.setValue("12000");
    await inputs[3]?.setValue("12001");
    await inputs[4]?.setValue("12");
    await wrapper.findAll("button").at(-1)?.trigger("click");

    expect(stores.consoleData.saveBrokerIntegration).toHaveBeenCalledWith(
      "futu",
      expect.objectContaining({
        config: expect.objectContaining({
          apiPort: 12000,
          websocketPort: 12001,
          maxWebSocketConnections: 12,
        }),
      }),
    );

    stores.consoleData.brokerRuntime.value.session.connectivity = "connecting" as never;
    await nextTick();
    expect(wrapper.find(".chip-stub").exists()).toBe(true);
    wrapper.unmount();
  });
});
