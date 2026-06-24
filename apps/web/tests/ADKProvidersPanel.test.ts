// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import ADKProvidersPanel from "../src/components/adk-settings/ADKProvidersPanel.vue";
import type { ADKProvider } from "../src/contracts";
import { buttonStub, dialogStub, passthroughStub } from "./helpers";

afterEach(() => {
  document.body.innerHTML = "";
});

describe("ADKProvidersPanel", () => {
  it("marks the default provider and can set another provider as default", async () => {
    const setDefaultProvider = vi.fn();
    const wrapper = mountProvidersPanel({
      providers: [
        buildProvider({ id: "provider-default", displayName: "Default Provider", default: true }),
        buildProvider({ id: "provider-other", displayName: "Other Provider", default: false }),
      ],
      setDefaultProvider,
    });

    expect(wrapper.text()).toContain("默认");
    const defaultButtons = wrapper.findAll("button").filter((button) =>
      button.text().includes("设为默认"),
    );
    expect(defaultButtons.length).toBeGreaterThanOrEqual(1);

    await defaultButtons[0]!.trigger("click");
    expect(setDefaultProvider).toHaveBeenCalledWith("provider-other");
  });
});

function mountProvidersPanel(
  overrides: Partial<{
    providers: ADKProvider[];
    setDefaultProvider: ReturnType<typeof vi.fn>;
  }> = {},
) {
  return mount(ADKProvidersPanel, {
    attachTo: document.body,
    props: {
      providerForm: {
        id: "",
        displayName: "",
        baseUrl: "",
        model: "",
        contextWindowTokens: 0,
        requestTimeoutSeconds: 180,
        apiKey: "",
        enabled: true,
      },
      runtimeSettingsForm: {
        runTimeoutSeconds: 1800,
        streamIdleTimeoutSeconds: 300,
      },
      providers: overrides.providers ?? [],
      saveProvider: vi.fn(),
      saveRuntimeSettings: vi.fn(),
      newProviderForm: vi.fn(),
      editProvider: vi.fn(),
      testProvider: vi.fn(),
      deleteProvider: vi.fn(),
      setDefaultProvider: overrides.setDefaultProvider ?? vi.fn(),
    },
    global: {
      stubs: {
        "v-btn": buttonStub,
        "v-card": passthroughStub,
        "v-card-actions": passthroughStub,
        "v-card-title": passthroughStub,
        "v-card-text": passthroughStub,
        "v-chip": { template: "<span><slot /></span>" },
        "v-dialog": dialogStub,
        "v-progress-circular": passthroughStub,
        "v-switch": {
          props: ["modelValue", "label"],
          template: "<label>{{ label }}<input type='checkbox' :checked='modelValue' /></label>",
        },
        "v-text-field": {
          props: ["modelValue", "label"],
          template: "<label>{{ label }}<input :value='modelValue' /></label>",
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
    createdAt: "2026-06-24T00:00:00Z",
    updatedAt: "2026-06-24T00:00:00Z",
    ...overrides,
  };
}
