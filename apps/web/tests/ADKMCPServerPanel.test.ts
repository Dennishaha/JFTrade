// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent } from "vue";

import ADKMCPServerPanel from "../src/components/adk-settings/ADKMCPServerPanel.vue";
import { passthroughStub } from "./helpers";

const dialogStub = defineComponent({
  props: { modelValue: Boolean },
  emits: ["update:modelValue"],
  template: "<div v-if='modelValue' class='dialog-stub'><slot /></div>",
});

const textFieldStub = defineComponent({
  props: ["modelValue", "label"],
  template: "<label><span>{{ label }}</span><input :value='modelValue ?? \"\"' /><slot name='append-inner' /></label>",
});

const actionButtonStub = defineComponent({
  emits: ["click"],
  template: "<button type='button' @click='$emit(\"click\")'><slot /></button>",
});

const commonStubs = {
  "v-card": passthroughStub,
  "v-card-actions": passthroughStub,
  "v-card-text": passthroughStub,
  "v-card-title": passthroughStub,
  "v-chip": passthroughStub,
  "v-dialog": dialogStub,
  "v-select": passthroughStub,
  "v-spacer": { template: "<span />" },
  "v-switch": passthroughStub,
  "v-text-field": textFieldStub,
  "v-btn": actionButtonStub,
  "v-alert": passthroughStub,
};

function panelProps(overrides: Record<string, unknown> = {}) {
  return {
    form: { enabled: true, port: 8123, authMode: "token" as const },
    status: { running: true, endpoint: "http://127.0.0.1:8123/mcp" },
    tokenConfigured: true,
    oneTimeToken: "",
    saving: false,
    save: vi.fn(),
    resetToken: vi.fn(),
    clearOneTimeToken: vi.fn(),
    ...overrides,
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ADKMCPServerPanel", () => {
  it("reports running state, invokes save/reset, and copies endpoint and fresh token", async () => {
    const clipboard = { writeText: vi.fn().mockResolvedValue(undefined) };
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: clipboard });
    const props = panelProps();
    const wrapper = mount(ADKMCPServerPanel, {
      props,
      global: { stubs: commonStubs },
    });

    expect(wrapper.text()).toContain("运行中");
    expect(wrapper.text()).toContain("重置 Token");
    await wrapper.findAll("button").find((button) => button.text() === "保存")!.trigger("click");
    await wrapper.findAll("button").find((button) => button.text() === "重置 Token")!.trigger("click");
    expect(props.save).toHaveBeenCalledOnce();
    expect(props.resetToken).toHaveBeenCalledOnce();

    await wrapper.setProps({ oneTimeToken: "one-time-secret" });
    await flushPromises();
    expect(wrapper.text()).toContain("新的 MCP Token");

    await wrapper.get("button[title='复制 MCP 端点']").trigger("click");
    await flushPromises();
    expect(clipboard.writeText).toHaveBeenCalledWith("http://127.0.0.1:8123/mcp");
    expect(wrapper.text()).toContain("端点已复制");

    await wrapper.get("button[title='复制 Token']").trigger("click");
    await flushPromises();
    expect(clipboard.writeText).toHaveBeenCalledWith("one-time-secret");
    expect(wrapper.text()).toContain("Token 已复制");

    await wrapper.findAll("button").find((button) => button.text() === "完成")!.trigger("click");
    expect(props.clearOneTimeToken).toHaveBeenCalledOnce();
    expect(wrapper.find(".dialog-stub").exists()).toBe(false);
  });

  it("surfaces failed service state and gracefully reports clipboard failure", async () => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
    });
    const wrapper = mount(ADKMCPServerPanel, {
      props: panelProps({
        form: { enabled: false, port: 8123, authMode: "none" },
        status: { running: false, endpoint: "http://127.0.0.1:8123/mcp", lastError: "端口已被占用" },
        tokenConfigured: false,
        oneTimeToken: "fresh-token",
      }),
      global: { stubs: commonStubs },
    });

    await wrapper.setProps({ oneTimeToken: "new-token" });
    await flushPromises();
    expect(wrapper.text()).toContain("启动失败");
    expect(wrapper.text()).toContain("端口已被占用");
    expect(wrapper.text()).not.toContain("生成 Token");

    await wrapper.get("button[title='复制 Token']").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("复制失败，请手动复制");
  });

  it("uses the browser copy fallback and presents a stopped token-enabled server", async () => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: undefined,
    });
    const copyCommand = vi.fn().mockReturnValue(true);
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: copyCommand,
    });
    const wrapper = mount(ADKMCPServerPanel, {
      props: panelProps({
        status: { running: false, endpoint: "http://127.0.0.1:8123/mcp" },
        tokenConfigured: false,
      }),
      global: { stubs: commonStubs },
    });
    expect(wrapper.text()).toContain("已停止");
    expect(wrapper.text()).toContain("生成 Token");

    await wrapper.setProps({ oneTimeToken: "fallback-token" });
    await flushPromises();
    await wrapper.get("button[title='复制 Token']").trigger("click");
    await flushPromises();
    expect(copyCommand).toHaveBeenCalledWith("copy");
    expect(wrapper.text()).toContain("Token 已复制");

    copyCommand.mockReturnValue(false);
    await wrapper.get("button[title='复制 MCP 端点']").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("复制失败，请手动复制");
  });

  it("keeps editable MCP settings bound to the form and clears a token after the dialog close event", async () => {
    const props = panelProps({ oneTimeToken: "new-secret" });
    const modelInputStub = defineComponent({
      props: ["modelValue"],
      emits: ["update:modelValue"],
      template: "<input :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' /><slot name='append-inner' />",
    });
    const modelSwitchStub = defineComponent({
      props: ["modelValue"],
      emits: ["update:modelValue"],
      template: "<input type='checkbox' :checked='modelValue' @change='$emit(\"update:modelValue\", $event.target.checked)' />",
    });
    const modelSelectStub = defineComponent({
      props: ["modelValue"],
      emits: ["update:modelValue"],
      template: "<select :value='modelValue' @change='$emit(\"update:modelValue\", $event.target.value)'><option value='token'>token</option><option value='none'>none</option></select>",
    });
    const wrapper = mount(ADKMCPServerPanel, {
      props,
      global: {
        stubs: {
          ...commonStubs,
          "v-switch": modelSwitchStub,
          "v-text-field": modelInputStub,
          "v-select": modelSelectStub,
        },
      },
    });

    const fields = wrapper.findAll("input");
    await fields[0]!.setValue(false);
    await fields[1]!.setValue("9321");
    await wrapper.find("select").setValue("none");
    expect(props.form).toEqual({ enabled: false, port: 9321, authMode: "none" });

    wrapper.findComponent(dialogStub).vm.$emit("update:modelValue", false);
    await flushPromises();
    expect(props.clearOneTimeToken).toHaveBeenCalledOnce();
  });
});
