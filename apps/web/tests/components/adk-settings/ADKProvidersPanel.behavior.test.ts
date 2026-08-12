// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent } from "vue";

import ADKProvidersPanel from "../../../src/components/adk-settings/ADKProvidersPanel.vue";
import type { ADKProvider } from "../../../src/types";
import { dialogStub } from "../../helpers";

const singleSlotStub = {
  template: "<div><slot /></div>",
};

const safeButtonStub = defineComponent({
  props: ["disabled"],
  emits: ["click"],
  template:
    "<button type='button' :disabled='disabled' @click=\"$emit('click')\"><slot /></button>",
});

describe("ADKProvidersPanel business flows", () => {
  it("covers provider operations, capability badges, and runtime setting saves", async () => {
    const editProvider = vi.fn();
    const testProvider = vi.fn(async (_providerId: string, mode: "quick" | "full") =>
      providerTestResult(mode),
    );
    const deleteProvider = vi.fn();
    const setDefaultProvider = vi.fn();
    const saveProvider = vi.fn(async () => true);
    const saveRuntimeSettings = vi.fn();
    const providerForm = {
      id: "provider-default",
      displayName: "OpenAI",
      baseUrl: "https://api.openai.com/v1",
      model: "gpt-4.1",
      apiProtocol: "chat_completions" as const,
      reasoningRequestField: "reasoning_effort",
      reasoningMappings: buildReasoningMappings({ medium: "balanced" }),
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
          reasoningConfig: {
            requestField: "reasoning_effort",
            mappings: [{ effort: "medium", value: "balanced" }],
          },
        }),
        buildProvider({
          id: "provider-other",
          displayName: "Claude",
          model: "claude-sonnet",
          apiProtocol: "responses",
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
    expect(wrapper.text()).toContain("协议：Responses");
    expect(wrapper.text()).toContain("完整验证");

    const buttons = wrapper.findAll("button");
    await buttons.find((button) => button.text() === "快速测试")!.trigger("click");
    await buttons.find((button) => button.text() === "设为默认")!.trigger("click");
    await buttons.find((button) => button.text() === "删除")!.trigger("click");
    await buttons.find((button) => button.text() === "编辑")!.trigger("click");
    expect(testProvider).toHaveBeenCalledWith("provider-default", "quick");
    expect(setDefaultProvider).toHaveBeenCalledWith("provider-other");
    expect(deleteProvider).toHaveBeenCalledWith("provider-default");
    expect(editProvider).toHaveBeenCalledWith(
      expect.objectContaining({ id: "provider-default" }),
    );
    expect(wrapper.text()).toContain("编辑模型服务");
    expect(wrapper.text()).toContain("留空则保留原密钥");
    expect(wrapper.text()).toContain("中 · balanced");

    await wrapper.findAll("button").find((button) => button.text() === "全部关闭")!.trigger("click");
    expect(providerForm.reasoningMappings.every((mapping) => !mapping.enabled)).toBe(true);
    expect(wrapper.text()).toContain("不发送显式思考等级");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "使用常用中高档")!
      .trigger("click");
    expect(
      providerForm.reasoningMappings
        .filter((mapping) => mapping.enabled)
        .map((mapping) => mapping.effort),
    ).toEqual(["medium", "high"]);

    providerForm.reasoningRequestField = "custom.reasoning";
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "恢复协议默认字段")!
      .trigger("click");
    expect(providerForm.reasoningRequestField).toBe("reasoning_effort");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "完整验证")!
      .trigger("click");
    expect(wrapper.text()).toContain("可能耗时较长并产生相应模型调用费用");
    await wrapper
      .findAll("button")
      .filter((button) => button.text() === "取消")
      .at(-1)!
      .trigger("click");
    expect(testProvider).toHaveBeenCalledTimes(1);
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "完整验证")!
      .trigger("click");
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "继续验证")!
      .trigger("click");
    expect(testProvider).toHaveBeenLastCalledWith("provider-default", "full");

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

  it("shows pending and completed feedback next to the tested provider", async () => {
    let completeTest:
      | ((feedback: ReturnType<typeof providerTestResult>) => void)
      | undefined;
    const testProvider = vi.fn(
      () =>
        new Promise<ReturnType<typeof providerTestResult>>((resolve) => {
          completeTest = resolve;
        }),
    );
    const wrapper = mountProvidersPanel({
      providers: [buildProvider({ id: "provider-under-test" })],
      testProvider,
    });

    const testButton = wrapper
      .findAll("button")
      .find((button) => button.text() === "快速测试")!;
    await testButton.trigger("click");
    await wrapper.vm.$nextTick();

    expect(testProvider).toHaveBeenCalledWith("provider-under-test", "quick");
    expect(testButton.attributes("disabled")).toBeDefined();

    completeTest!(providerTestResult());
    await flushPromises();

    expect(wrapper.text()).toContain("快速测试 通过");
    expect(wrapper.text()).toContain("连接回复：Provider 测试成功");
    expect(wrapper.text()).toContain("极高 (xhigh)");
    expect(
      wrapper
        .findAll("button")
        .find((button) => button.text() === "快速测试")!
        .attributes("disabled"),
    ).toBeUndefined();
  });

  it("serializes provider tests and reports both Error and fallback failures", async () => {
    let rejectFirst: ((reason?: unknown) => void) | undefined;
    const testProvider = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise<ReturnType<typeof providerTestResult>>((_resolve, reject) => {
            rejectFirst = reject;
          }),
      )
      .mockRejectedValueOnce("transport unavailable")
      .mockResolvedValueOnce(providerTestResult());
    const wrapper = mountProvidersPanel({
      providers: [
        buildProvider({ id: "provider-first" }),
        buildProvider({ id: "provider-second", default: false }),
      ],
      testProvider,
    });
    const initialButtons = wrapper
      .findAll("button")
      .filter((button) => button.text() === "快速测试");

    await initialButtons[0]!.trigger("click");
    await initialButtons[1]!.trigger("click");
    expect(testProvider).toHaveBeenCalledOnce();

    rejectFirst!(new Error("provider rejected request"));
    await flushPromises();
    expect(wrapper.text()).toContain("provider rejected request");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "快速测试")!
      .trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("测试失败");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "快速测试")!
      .trigger("click");
    await flushPromises();
    expect(wrapper.text()).not.toContain("测试失败");
  });

  it("renders empty reasoning probes and detailed full-validation failures", async () => {
    const testProvider = vi
      .fn()
      .mockResolvedValueOnce({
        ...providerTestResult(),
        reply: "",
        reasoning: {
          ...providerTestResult().reasoning,
          requestField: "",
          results: [],
        },
      })
      .mockResolvedValueOnce({
        ...providerTestResult("full"),
        ok: false,
        reply: "",
        capabilities: { streaming: true, tools: false, reasoning: false },
        reasoning: {
          mode: "full" as const,
          requestField: "",
          ok: false,
          results: [
            {
              effort: "high" as const,
              value: "deep",
              ok: false,
              error: "mapping rejected",
            },
          ],
        },
      });
    const wrapper = mountProvidersPanel({
      providers: [buildProvider()],
      testProvider,
    });

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "快速测试")!
      .trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("本次未发送推理测试请求");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "完整验证")!
      .trigger("click");
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "继续验证")!
      .trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("完整验证 存在失败档位");
    expect(wrapper.text()).toContain("tools · 不支持");
    expect(wrapper.text()).toContain("高 (deep)");
    expect(wrapper.text()).toContain("失败");
    expect(wrapper.text()).toContain("mapping rejected");
  });
});

function mountProvidersPanel(
  overrides: Partial<{
    providerForm: {
      id: string;
      displayName: string;
      baseUrl: string;
      model: string;
      apiProtocol: "chat_completions" | "responses";
      reasoningRequestField: string;
      reasoningMappings: Array<{
        effort: "low" | "medium" | "high" | "xhigh" | "max";
        value: string;
        enabled: boolean;
      }>;
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
        apiProtocol: "chat_completions",
        reasoningRequestField: "reasoning_effort",
        reasoningMappings: buildReasoningMappings({
          medium: "medium",
          high: "high",
        }),
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
      saveProvider: overrides.saveProvider ?? vi.fn(async () => true),
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
        "v-alert": singleSlotStub,
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
        "v-select": singleSlotStub,
      },
    },
  });
}

function providerTestResult(mode: "quick" | "full" = "quick") {
  return {
    ok: true,
    reply: "Provider 测试成功",
    capabilities: { streaming: true, tools: true, reasoning: true },
    reasoning: {
      mode,
      requestField: "reasoning.effort",
      ok: true,
      results: [{ effort: "xhigh" as const, value: "xhigh", ok: true }],
    },
    checkedAt: "2026-08-11T00:00:00Z",
  };
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

function buildReasoningMappings(
  enabled: Partial<Record<"low" | "medium" | "high" | "xhigh" | "max", string>>,
) {
  return (["low", "medium", "high", "xhigh", "max"] as const).map((effort) => ({
    effort,
    value: enabled[effort] ?? effort,
    enabled: enabled[effort] != null,
  }));
}
