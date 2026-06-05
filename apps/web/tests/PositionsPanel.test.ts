// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { afterEach, describe, expect, it } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import PositionsPanel from "../src/components/workspace/PositionsPanel.vue";
import { provideConsoleDataStore } from "../src/composables/useConsoleData";
import { provideWorkspaceTradingPreferencesStore } from "../src/composables/useWorkspaceLayout";

afterEach(() => {
  window.localStorage?.clear();
});

describe("PositionsPanel", () => {
  it("shows an active orders loading message before empty order state", async () => {
    const wrapper = mountPositionsPanel({ isLoadingBrokerOrders: true });

    await wrapper.findAll("button").find((button) =>
      button.text().includes("近期订单"),
    )?.trigger("click");
    await nextTick();

    expect(wrapper.text()).toContain("正在加载近期订单...");
    expect(wrapper.text()).not.toContain("暂无近期订单");
  });

  it("does not show count in tab label when data is not loaded", async () => {
    const wrapper = mountPositionsPanel({ isLoadingBrokerOrders: true });

    const activeTabButton = wrapper.findAll("button").find((button) =>
      button.text().includes("近期订单"),
    );
    expect(activeTabButton?.text()).toBe("近期订单");
  });
});

function mountPositionsPanel(options: {
  isLoadingBrokerOrders?: boolean;
}) {
  const Host = defineComponent({
    setup() {
      const workspaceLayout = provideWorkspaceTradingPreferencesStore();
      const store = provideConsoleDataStore(workspaceLayout);
      store.isLoadingBrokerOrders.value =
        options.isLoadingBrokerOrders === true;
      return () => h(PositionsPanel);
    },
  });

  return mount(Host, {
    global: {
      plugins: [createPinia()],
    },
  });
}
