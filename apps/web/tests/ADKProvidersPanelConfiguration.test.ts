// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it, vi } from "vitest";

import ADKProvidersPanel from "../src/components/adk-settings/ADKProvidersPanel.vue";

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
  props: ["label", "modelValue"],
  emits: ["update:modelValue"],
  template: "<label><span>{{ label }}</span><input :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' /></label>",
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
      contextWindowTokens: 0,
      requestTimeoutSeconds: 60,
      apiKey: "",
      enabled: true,
    };
    const newProviderForm = vi.fn(() => {
      providerForm.displayName = "New provider";
      providerForm.baseUrl = "https://initial.example";
    });
    const saveProvider = vi.fn().mockResolvedValue(undefined);
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
          "v-btn": buttonStub,
          "v-dialog": dialogStub,
          "v-switch": switchStub,
          "v-text-field": textFieldStub,
          "v-chip": slotStub,
        },
      },
    });

    await wrapper.findAll("button").find((button) => button.text() === "新增模型服务")!.trigger("click");
    expect(newProviderForm).toHaveBeenCalledOnce();
    expect(wrapper.get(".provider-dialog").exists()).toBe(true);

    await field(wrapper, "启用").setValue(false);
    await field(wrapper, "名称").setValue("研究模型");
    await field(wrapper, "服务地址").setValue("https://provider.example/v1");
    await field(wrapper, "默认模型").setValue("research-model");
    await field(wrapper, "Context Window Tokens").setValue("128000");
    await field(wrapper, "API 密钥").setValue("secret-token");
    await field(wrapper, "请求超时（秒）").setValue("120");
    await wrapper.findAll("button").find((button) => button.text() === "保存模型服务")!.trigger("click");

    expect(providerForm).toEqual({
      id: "",
      displayName: "研究模型",
      baseUrl: "https://provider.example/v1",
      model: "research-model",
      contextWindowTokens: "128000",
      requestTimeoutSeconds: "120",
      apiKey: "secret-token",
      enabled: false,
    });
    expect(saveProvider).toHaveBeenCalledOnce();
    expect(wrapper.find(".provider-dialog").exists()).toBe(false);
  });
});
