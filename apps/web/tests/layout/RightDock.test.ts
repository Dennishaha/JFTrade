// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

const state = vi.hoisted(() => ({
  prefs: null as unknown as ReturnType<typeof ref>,
  update: vi.fn(),
}));

vi.mock("@/composables/workspace/useWorkspaceLayout", () => ({
  useWorkspaceViewState: () => ({ prefs: state.prefs, update: state.update }),
}));

import AppTabs from "../../src/components/shared/AppTabs.vue";
import RightDock from "../../src/layout/RightDock.vue";

beforeEach(() => {
  state.prefs = ref({
    rightDockOpen: true,
    rightDockTab: "notifications",
  });
  state.update.mockReset();
});

describe("RightDock", () => {
  it("rejects unknown tabs and opens valid destinations", async () => {
    const wrapper = mount(RightDock, {
      global: {
        stubs: {
          NotificationCenter: { template: "<div>notifications</div>" },
          AiAssistantPanel: { template: "<div>assistant</div>" },
        },
      },
    });
    const tabs = wrapper.findComponent(AppTabs);

    tabs.vm.$emit("update:modelValue", "invalid");
    await wrapper.vm.$nextTick();
    expect(state.update).not.toHaveBeenCalled();

    tabs.vm.$emit("update:modelValue", "ai");
    await wrapper.vm.$nextTick();
    expect(state.update).toHaveBeenCalledWith({
      rightDockTab: "ai",
      rightDockOpen: true,
    });

    await wrapper.get('button[title="收起"]').trigger("click");
    expect(state.update).toHaveBeenLastCalledWith({ rightDockOpen: false });
  });
});
