// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it, vi } from "vitest";

import ADKProvidersPanel from "../../../src/components/adk-settings/ADKProvidersPanel.vue";

const slotStub = defineComponent({ template: "<section><slot /></section>" });
const buttonStub = defineComponent({
  emits: ["click"],
  template: "<button type='button' @click='$emit(\"click\")'><slot /></button>",
});
const dialogStub = defineComponent({
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template: "<div v-if='modelValue' class='provider-dialog'><slot /></div>",
});
const textFieldStub = defineComponent({
  props: ["disabled", "errorMessages", "label", "modelValue"],
  emits: ["update:modelValue"],
  template: "<label><span>{{ label }}</span><input :disabled='disabled' :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' /><small>{{ errorMessages }}</small></label>",
});
const switchStub = defineComponent({
  props: ["label", "modelValue"],
  emits: ["update:modelValue"],
  template: "<label><span>{{ label }}</span><input type='checkbox' :checked='modelValue' @change='$emit(\"update:modelValue\", $event.target.checked)' /></label>",
});

function field(wrapper: ReturnType<typeof mount>, label: string) {
  const match = wrapper.findAll("label").find((candidate) => candidate.text().includes(label));
  if (match == null) throw new Error(`missing provider field: ${label}`);
  return match.get("input");
}

describe("ADK providers panel configuration", () => {
  it("edits every provider setting in the dialog and closes only after the save succeeds", async () => {
    const providerForm = {
      id: "",
      displayName: "",
      baseUrl: "",
      model: "",
      apiProtocol: "chat_completions" as const,
      reasoningRequestField: "reasoning_effort",
      reasoningMappings: buildReasoningMappings({
        medium: "medium",
        high: "high",
      }),
      contextWindowTokens: 0,
      requestTimeoutSeconds: 60,
      apiKey: "",
      enabled: true,
    };
    const newProviderForm = vi.fn(() => {
      providerForm.displayName = "New provider";
      providerForm.baseUrl = "https://initial.example";
    });
    const saveProvider = vi
      .fn()
      .mockResolvedValueOnce(false)
      .mockResolvedValueOnce(true);
    const wrapper = mount(ADKProvidersPanel, {
      props: {
        providerForm,
        runtimeSettingsForm: { runTimeoutSeconds: 3600, streamIdleTimeoutSeconds: 90 },
        providers: [],
        saveProvider,
        saveRuntimeSettings: vi.fn(),
        newProviderForm,
        editProvider: vi.fn(),
        testProvider: vi.fn(),
        deleteProvider: vi.fn(),
        setDefaultProvider: vi.fn(),
      },
      global: {
        stubs: {
          "v-card": slotStub,
          "v-card-title": slotStub,
          "v-card-text": slotStub,
          "v-card-actions": slotStub,
          "v-alert": slotStub,
          "v-btn": buttonStub,
          "v-dialog": dialogStub,
          "v-switch": switchStub,
          "v-text-field": textFieldStub,
          "v-select": slotStub,
          "v-chip": slotStub,
        },
      },
    });

    await wrapper.findAll("button").find((button) => button.text() === "新增模型服务")!.trigger("click");
    expect(newProviderForm).toHaveBeenCalledOnce();
    expect(wrapper.get(".provider-dialog").exists()).toBe(true);
    const reasoningSettings = wrapper.get(
      '[data-testid="provider-reasoning-settings"]',
    );
    expect(reasoningSettings.text()).toContain("思考等级");
    expect(reasoningSettings.text()).toContain("2 档已启用");
    for (const label of ["低", "中", "高", "极高", "最大"]) {
      expect(reasoningSettings.text()).toContain(label);
    }
    const advancedButton = wrapper
      .findAll("button")
      .find((button) => button.text() === "高级配置")!;
    expect(advancedButton.attributes("aria-expanded")).toBe("false");
    await advancedButton.trigger("click");
    expect(advancedButton.attributes("aria-expanded")).toBe("true");

    await field(wrapper, "启用").setValue(false);
    await field(wrapper, "名称").setValue("研究模型");
    await field(wrapper, "服务地址").setValue("https://provider.example/v1");
    await field(wrapper, "默认模型").setValue("research-model");
    await field(wrapper, "Context Window Tokens").setValue("128000");
    await field(wrapper, "API 密钥").setValue("secret-token");
    await field(wrapper, "请求超时（秒）").setValue("120");
    await field(wrapper, "低").setValue(true);
    await field(wrapper, "low 的 Provider 值").setValue("minimal");
    await field(wrapper, "medium 的 Provider 值").setValue("balanced");
    await wrapper.findAll("button").find((button) => button.text() === "保存模型服务")!.trigger("click");

    expect(providerForm).toEqual({
      id: "",
      displayName: "研究模型",
      baseUrl: "https://provider.example/v1",
      model: "research-model",
      apiProtocol: "chat_completions",
      reasoningRequestField: "reasoning_effort",
      reasoningMappings: [
        { effort: "low", value: "minimal", enabled: true },
        { effort: "medium", value: "balanced", enabled: true },
        { effort: "high", value: "high", enabled: true },
        { effort: "xhigh", value: "xhigh", enabled: false },
        { effort: "max", value: "max", enabled: false },
      ],
      contextWindowTokens: "128000",
      requestTimeoutSeconds: "120",
      apiKey: "secret-token",
      enabled: false,
    });
    expect(saveProvider).toHaveBeenCalledOnce();
    expect(wrapper.find(".provider-dialog").exists()).toBe(true);

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "保存模型服务")!
      .trigger("click");

    expect(saveProvider).toHaveBeenCalledTimes(2);
    expect(wrapper.find(".provider-dialog").exists()).toBe(false);
  });

  it("shows an inline error for an enabled level without a provider value", async () => {
    const providerForm = {
      id: "provider-1",
      displayName: "Provider",
      baseUrl: "https://provider.example/v1",
      model: "reasoning-model",
      apiProtocol: "responses" as const,
      reasoningRequestField: "reasoning.effort",
      reasoningMappings: buildReasoningMappings({ high: "high" }),
      contextWindowTokens: 128_000,
      requestTimeoutSeconds: 180,
      apiKey: "",
      enabled: true,
    };
    const wrapper = mount(ADKProvidersPanel, {
      props: {
        providerForm,
        runtimeSettingsForm: { runTimeoutSeconds: 3600, streamIdleTimeoutSeconds: 90 },
        providers: [
          {
            id: "provider-1",
            displayName: "Provider",
            baseUrl: "https://provider.example/v1",
            model: "reasoning-model",
            apiProtocol: "responses",
            reasoningConfig: {
              requestField: "reasoning.effort",
              mappings: [{ effort: "high", value: "high" }],
            },
            requestTimeoutMs: 180_000,
            enabled: true,
            default: true,
            hasApiKey: true,
            createdAt: "2026-08-12T00:00:00Z",
            updatedAt: "2026-08-12T00:00:00Z",
          },
        ],
        saveProvider: vi.fn(async () => false),
        saveRuntimeSettings: vi.fn(),
        newProviderForm: vi.fn(),
        editProvider: vi.fn(),
        testProvider: vi.fn(),
        deleteProvider: vi.fn(),
        setDefaultProvider: vi.fn(),
      },
      global: {
        stubs: {
          "v-card": slotStub,
          "v-card-title": slotStub,
          "v-card-text": slotStub,
          "v-card-actions": slotStub,
          "v-alert": slotStub,
          "v-btn": buttonStub,
          "v-dialog": dialogStub,
          "v-switch": switchStub,
          "v-text-field": textFieldStub,
          "v-select": slotStub,
          "v-chip": slotStub,
        },
      },
    });

    await wrapper.findAll("button").find((button) => button.text() === "编辑")!.trigger("click");
    await field(wrapper, "high 的 Provider 值").setValue("   ");

    expect(wrapper.text()).toContain("请输入 Provider 值");
  });
});

function buildReasoningMappings(
  enabled: Partial<Record<"low" | "medium" | "high" | "xhigh" | "max", string>>,
) {
  return (["low", "medium", "high", "xhigh", "max"] as const).map((effort) => ({
    effort,
    value: enabled[effort] ?? effort,
    enabled: enabled[effort] != null,
  }));
}
