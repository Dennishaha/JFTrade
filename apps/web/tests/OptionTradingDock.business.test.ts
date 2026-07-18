// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent, ref } from "vue";
import { describe, expect, it, vi } from "vitest";

const consoleState = {
  activeExecutionOrders: ref({
    orders: [{ orderKind: "option_combo" }, { orderKind: "single" }],
  }),
  historicalExecutionOrders: ref({
    orders: [{ orderKind: "option_combo" }],
  }),
  portfolioPositions: ref({ positions: [{ symbol: "US.BABA" }] }),
  selectedBrokerAccount: ref({
    accountId: "option-account",
    displayName: "期权保证金账户",
    tradingEnvironment: "SIMULATE",
  }),
  systemStatus: ref({ defaultTradingEnvironment: "SIMULATE" }),
};

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => consoleState,
}));

import OptionTradingDock from "../src/components/product/OptionTradingDock.vue";
import {
  provideOptionComboDraftStore,
  type OptionComboDraftStore,
} from "../src/composables/optionComboDraft";

const PositionsStub = defineComponent({
  name: "PositionsPanel",
  props: {
    view: String,
    focusOrderId: String,
  },
  template:
    "<div class='positions-stub'>{{ view }}|{{ focusOrderId }}</div>",
});

const BuilderStub = defineComponent({
  name: "OptionComboBuilder",
  emits: ["submitted"],
  template:
    "<button class='submit-stub' @click=\"$emit('submitted', 'combo-new')\">submit</button>",
});

describe("OptionTradingDock", () => {
  it("uses one compact tab bar and focuses the submitted parent combo order", async () => {
    let store!: OptionComboDraftStore;
    let collapsedState = ref(false);
    const Host = defineComponent({
      components: { OptionTradingDock },
      setup() {
        store = provideOptionComboDraftStore();
        store.setContext("US.BABA", "US");
        collapsedState = ref(false);
        return { collapsedState };
      },
      template:
        "<OptionTradingDock v-model:collapsed='collapsedState' />",
    });
    const wrapper = mount(Host, {
      global: {
        stubs: {
          OptionComboBuilder: BuilderStub,
          PositionsPanel: PositionsStub,
        },
      },
    });

    expect(wrapper.text()).toContain("期权保证金账户");
    expect(wrapper.text()).toContain("持仓 1");
    expect(wrapper.text()).toContain("订单 1");
    expect(wrapper.text()).toContain("历史 1");
    expect(wrapper.get(".positions-stub").text()).toBe("positions|");

    const tradeTab = wrapper
      .findAll("nav button")
      .find((button) => button.text().startsWith("期权交易"));
    await tradeTab!.trigger("click");
    expect(store.activeDockTab.value).toBe("trade");
    await wrapper.get(".submit-stub").trigger("click");
    expect(store.activeDockTab.value).toBe("orders");
    expect(store.submittedOrderId.value).toBe("combo-new");
    expect(wrapper.get(".positions-stub").text()).toBe("active|combo-new");

    await wrapper.get('button[aria-label="折叠期权交易区"]').trigger("click");
    expect(wrapper.get(".option-trading-dock").classes()).toContain(
      "is-collapsed",
    );
    expect(collapsedState.value).toBe(true);
    await wrapper.get('button[aria-label="展开期权交易区"]').trigger("click");
    expect(wrapper.get(".option-trading-dock").classes()).not.toContain(
      "is-collapsed",
    );
    expect(collapsedState.value).toBe(false);
  });
});
