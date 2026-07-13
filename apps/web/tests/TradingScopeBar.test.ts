// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { afterEach, describe, expect, it } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import {
  emptyBrokerRuntime,
  emptyBrokerSettings,
  emptySystemStatus,
} from "@/contracts";

import TradingScopeBar from "../src/components/TradingScopeBar.vue";
import { provideConsoleDataStore } from "../src/composables/useConsoleData";
import { provideWorkspaceTradingPreferencesStore } from "../src/composables/useWorkspaceLayout";

afterEach(() => {
  window.localStorage?.clear();
  window.sessionStorage?.clear();
});

describe("TradingScopeBar", () => {
  it("shows the active trading scope with a strong real-trading status", async () => {
    const { wrapper } = mountTradingScopeBar();

    await nextTick();

    expect(wrapper.get('[data-testid="trading-scope-env"]').text()).toBe("实盘");
    expect(wrapper.get('[data-testid="trading-scope-real-status"]').text()).toBe(
      "实盘可下单",
    );
    expect(wrapper.get('[data-testid="trading-scope-account"]').text()).toContain(
      "Futu Real US",
    );
    expect(wrapper.get('[data-testid="trading-scope-market"]').text()).toContain(
      "美股",
    );
    expect(wrapper.get('[data-testid="trading-scope-symbol"]').text()).toContain(
      "US.AAPL",
    );
    expect(wrapper.get('[data-testid="trading-scope-broker"]').text()).toContain(
      "Futu OpenD",
    );
    expect(
      wrapper.get('[data-testid="trading-scope-connectivity"]').text(),
    ).toContain("已连接");
    expect(wrapper.get('[data-testid="trading-scope-bar"]').classes()).toContain(
      "trading-scope-bar--real",
    );
  });

  it("shows A shares as the parent market with the concrete exchange tag", async () => {
    const { wrapper } = mountTradingScopeBar("SZ", "000001");

    await nextTick();

    expect(wrapper.get('[data-testid="trading-scope-market"]').text()).toContain(
      "沪深",
    );
    const instrument = wrapper.get('[data-testid="trading-scope-symbol"]').text();
    expect(instrument).toContain("000001");
    expect(instrument).toContain("深证");
    expect(instrument).not.toContain("SZ.000001");
  });
});

function mountTradingScopeBar(market = "US", symbol = "AAPL") {
  const Host = defineComponent({
    setup() {
      const workspaceLayout = provideWorkspaceTradingPreferencesStore();
      workspaceLayout.update({ market, symbol });
      const store = provideConsoleDataStore(workspaceLayout);
      store.systemStatus.value = {
        ...emptySystemStatus,
        defaultTradingEnvironment: "REAL",
        realTradingEnabled: true,
      };
      store.brokerRuntime.value = {
        ...emptyBrokerRuntime,
        descriptor: {
          ...emptyBrokerRuntime.descriptor,
          id: "futu",
          displayName: "Futu OpenD",
        },
        session: {
          ...emptyBrokerRuntime.session,
          displayName: "Futu OpenD",
          connectivity: "connected",
        },
        accounts: [
          {
            accountId: "real-us-1",
            tradingEnvironment: "REAL",
            accountType: "MARGIN",
            accountRole: "TRADING",
            securityFirm: "FUTU",
            marketAuthorities: [market],
            simulatedAccountType: null,
          },
        ],
      };
      store.brokerSettings.value = {
        ...emptyBrokerSettings,
        accounts: [
          {
            id: "account-1",
            brokerId: "futu",
            accountId: "real-us-1",
            displayName: "Futu Real US",
            tradingEnvironment: "REAL",
            market,
            securityFirm: "FUTU",
            enabled: true,
            updatedAt: "2026-07-04T00:00:00Z",
            createdAt: "2026-07-04T00:00:00Z",
          },
        ],
      };
      return () => h(TradingScopeBar);
    },
  });

  return {
    wrapper: mount(Host, {
      global: {
        plugins: [createPinia()],
      },
    }),
  };
}
