// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent } from "vue";

import ADKProvidersPanel from "../src/components/adk-settings/ADKProvidersPanel.vue";
import type { ADKProvider } from "../src/contracts";
import { dialogStub } from "./helpers";

const singleSlotStub = {
  template: "<div><slot /></div>",
};

const safeButtonStub = defineComponent({
  emits: ["click"],
  template:
    "<button type='button' @click=\"$emit('click')\"><slot /></button>",
});

describe("ADKProvidersPanel business flows", () => {
  it("covers provider operations, capability badges, and runtime setting saves", async () => {
    const editProvider = vi.fn();
    const testProvider = vi.fn();
    const deleteProvider = vi.fn();
    const setDefaultProvider = vi.fn();
    const saveProvider = vi.fn(async () => {});
    const saveRuntimeSettings = vi.fn();
    const providerForm = {
      id: "provider-default",
      displayName: "OpenAI",
      baseUrl: "https://api.openai.com/v1",
      model: "gpt-4.1",
      contextWindowTokens: 128000,
      requestTimeoutSeconds: 180,
      apiKey: "",
      enabled: true,
    };
    const runtimeSettingsForm = {
      runTimeoutSeconds: 1800,
      streamIdleTimeoutSeconds: 300,
    };
    const wrapper = mountProvidersPanel({
      providerForm,
      runtimeSettingsForm,
      providers: [
        buildProvider({
          id: "provider-default",
          displayName: "OpenAI",
          default: true,
          hasApiKey: false,
          capabilities: {
            chat: true,
            vision: false,
          },
        }),
        buildProvider({
          id: "provider-other",
          displayName: "Claude",
          model: "claude-sonnet",
          enabled: false,
          default: false,
          contextWindowTokens: 0,
          requestTimeoutMs: 61_200,
        }),
      ],
      editProvider,
      testProvider,
      deleteProvider,
      setDefaultProvider,
      saveProvider,
      saveRuntimeSettings,
    });

    expect(wrapper.text()).toContain("默认");
    expect(wrapper.text()).toContain("未配置");
    expect(wrapper.text()).toContain("chat · 支持");
    expect(wrapper.text()).toContain("vision · 不支持");
    expect(wrapper.text()).toContain("请求超时：61 秒");
    expect(wrapper.text()).toContain("上下文窗口：未配置");

    const buttons = wrapper.findAll("button");
    await buttons.find((button) => button.text() === "测试")!.trigger("click");
    await buttons.find((button) => button.text() === "设为默认")!.trigger("click");
    await buttons.find((button) => button.text() === "删除")!.trigger("click");
    await buttons.find((button) => button.text() === "编辑")!.trigger("click");
    expect(testProvider).toHaveBeenCalledWith("provider-default");
    expect(setDefaultProvider).toHaveBeenCalledWith("provider-other");
    expect(deleteProvider).toHaveBeenCalledWith("provider-default");
    expect(editProvider).toHaveBeenCalledWith(
      expect.objectContaining({ id: "provider-default" }),
    );
    expect(wrapper.text()).toContain("编辑模型服务");
    expect(wrapper.text()).toContain("留空则保留原密钥");

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("保存模型服务"))!
      .trigger("click");
    expect(saveProvider).toHaveBeenCalledOnce();
    expect(wrapper.find(".v-dialog-stub").exists()).toBe(false);

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("保存运行时设置"))!
      .trigger("click");
    expect(saveRuntimeSettings).toHaveBeenCalledOnce();
  });

  it("shows the empty-state onboarding and opens a fresh create dialog", async () => {
    const newProviderForm = vi.fn();
    const wrapper = mountProvidersPanel({
      providers: [],
      newProviderForm,
    });

    expect(wrapper.text()).toContain("还没有配置模型服务。");
    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("新增模型服务"))!
      .trigger("click");
    expect(newProviderForm).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("新增模型服务");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "取消")!
      .trigger("click");
    expect(wrapper.find(".v-dialog-stub").exists()).toBe(false);
  });
});

function mountProvidersPanel(
  overrides: Partial<{
    providerForm: {
      id: string;
      displayName: string;
      baseUrl: string;
      model: string;
      contextWindowTokens: number;
      requestTimeoutSeconds: number;
      apiKey: string;
      enabled: boolean;
    };
    runtimeSettingsForm: {
      runTimeoutSeconds: number;
      streamIdleTimeoutSeconds: number;
    };
    providers: ADKProvider[];
    saveProvider: ReturnType<typeof vi.fn>;
    saveRuntimeSettings: ReturnType<typeof vi.fn>;
    newProviderForm: ReturnType<typeof vi.fn>;
    editProvider: ReturnType<typeof vi.fn>;
    testProvider: ReturnType<typeof vi.fn>;
    deleteProvider: ReturnType<typeof vi.fn>;
    setDefaultProvider: ReturnType<typeof vi.fn>;
  }> = {},
) {
  return mount(ADKProvidersPanel, {
    attachTo: document.body,
    props: {
      providerForm: overrides.providerForm ?? {
        id: "",
        displayName: "OpenAI Compatible",
        baseUrl: "https://api.openai.com/v1",
        model: "gpt-4o-mini",
        contextWindowTokens: 0,
        requestTimeoutSeconds: 180,
        apiKey: "",
        enabled: true,
      },
      runtimeSettingsForm: overrides.runtimeSettingsForm ?? {
        runTimeoutSeconds: 1800,
        streamIdleTimeoutSeconds: 300,
      },
      providers: overrides.providers ?? [],
      saveProvider: overrides.saveProvider ?? vi.fn(async () => {}),
      saveRuntimeSettings: overrides.saveRuntimeSettings ?? vi.fn(),
      newProviderForm: overrides.newProviderForm ?? vi.fn(),
      editProvider: overrides.editProvider ?? vi.fn(),
      testProvider: overrides.testProvider ?? vi.fn(),
      deleteProvider: overrides.deleteProvider ?? vi.fn(),
      setDefaultProvider: overrides.setDefaultProvider ?? vi.fn(),
    },
    global: {
      stubs: {
        "v-btn": safeButtonStub,
        "v-card": singleSlotStub,
        "v-card-actions": singleSlotStub,
        "v-card-title": singleSlotStub,
        "v-card-text": singleSlotStub,
        "v-chip": { template: "<span><slot /></span>" },
        "v-dialog": dialogStub,
        "v-progress-circular": singleSlotStub,
        "v-switch": {
          props: ["modelValue", "label"],
          emits: ["update:modelValue"],
          template:
            "<label>{{ label }}<input type='checkbox' :checked='modelValue' @change=\"$emit('update:modelValue', $event.target.checked)\" /></label>",
        },
        "v-text-field": {
          props: ["modelValue", "label", "hint"],
          emits: ["update:modelValue"],
          template:
            "<label>{{ label }} {{ hint }}<input :value='modelValue' @input=\"$emit('update:modelValue', $event.target.value)\" /></label>",
        },
      },
    },
  });
}

function buildProvider(overrides: Partial<ADKProvider> = {}): ADKProvider {
  return {
    id: "provider-1",
    displayName: "Provider",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    requestTimeoutMs: 180_000,
    enabled: true,
    default: true,
    hasApiKey: true,
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}
