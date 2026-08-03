// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it } from "vitest";

import TopBarBrokerAccountPicker from "../../src/layout/TopBarBrokerAccountPicker.vue";

const simulateAccount = {
  selectionKey: "sim-futu-1",
  source: "managed" as const,
  brokerId: "futu",
  accountId: "SIM-001",
  displayName: "模拟主账户",
  tradingEnvironment: "SIMULATE",
  market: "HK",
  securityFirm: "富途",
};

describe("TopBarBrokerAccountPicker", () => {
  it("renders account state and forwards picker interactions to the shell", async () => {
    const wrapper = mountPicker({
      accounts: [simulateAccount],
      selectedSelectionKey: simulateAccount.selectionKey,
      favoriteSelectionKeys: [simulateAccount.selectionKey],
    });

    expect(wrapper.get('[data-testid="topbar-broker-account-item"]').classes()).toContain(
      "is-selected",
    );
    expect(wrapper.get('[data-testid="topbar-broker-account-item-favorite"]').text()).toBe("★");
    expect(wrapper.get(".tv-topbar-account-picker__item-main-line").text()).toContain(
      "富途 / FUTU / 模拟主账户",
    );
    expect(wrapper.get(".tv-topbar-account-picker__item-sub-line").text()).toContain(
      "SIM-001 / 模拟",
    );

    await wrapper.get('[data-testid="topbar-broker-account-filter"]').setValue("SIM-001");
    await wrapper.get('[data-testid="topbar-account-picker-trading-environment-real"]').trigger("click");
    await wrapper.get('[data-testid="topbar-broker-account-item-favorite"]').trigger("click");
    await wrapper.get(".tv-topbar-account-picker__item-main").trigger("click");
    await wrapper.get('[data-testid="topbar-broker-account-picker-close"]').trigger("click");

    expect(wrapper.emitted("update:filter-query")).toEqual([["SIM-001"]]);
    expect(wrapper.emitted("switch-environment")).toEqual([["REAL"]]);
    expect(wrapper.emitted("toggle-favorite")).toEqual([[simulateAccount.selectionKey]]);
    expect(wrapper.emitted("select")).toEqual([[simulateAccount.selectionKey]]);
    expect(wrapper.emitted("update:open")).toEqual([[false]]);
  });

  it("shows the shell-provided empty state when no account matches", () => {
    const wrapper = mountPicker({
      accounts: [],
      emptyLabel: "筛选后暂无模拟盘账户",
    });

    expect(wrapper.get('[data-testid="topbar-broker-account-picker-empty"]').text()).toBe(
      "筛选后暂无模拟盘账户",
    );
    expect(wrapper.find('[data-testid="topbar-broker-account-item"]').exists()).toBe(false);
  });
});

function mountPicker(
  overrides: Partial<InstanceType<typeof TopBarBrokerAccountPicker>["$props"]> = {},
) {
  return mount(TopBarBrokerAccountPicker, {
    props: {
      open: true,
      tradingEnvironment: "SIMULATE",
      filterQuery: "",
      accounts: [simulateAccount],
      emptyLabel: "暂无模拟盘账户",
      selectedSelectionKey: "",
      favoriteSelectionKeys: [],
      ...overrides,
    },
    global: {
      stubs: {
        "v-dialog": defineComponent({
          props: ["modelValue"],
          template: '<div v-if="modelValue"><slot /></div>',
        }),
        "v-card": { template: "<div><slot /></div>" },
        "v-card-title": { template: "<div><slot /></div>" },
        "v-card-text": { template: "<div><slot /></div>" },
        "v-btn-toggle": { template: "<div><slot /></div>" },
        "v-btn": defineComponent({
          emits: ["click"],
          template: '<button type="button" v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
        }),
        "v-text-field": defineComponent({
          props: ["modelValue"],
          emits: ["update:modelValue"],
          template:
            '<input v-bind="$attrs" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)">',
        }),
      },
    },
  });
}
