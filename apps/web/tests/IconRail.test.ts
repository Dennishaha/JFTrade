// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import IconRail from "../src/layout/IconRail.vue";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("IconRail", () => {
  it("opens docs through the shared external link opener", async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/workspace", component: { template: "<div />" } }],
    });
    await router.push("/workspace");
    await router.isReady();
    const open = vi.spyOn(window, "open").mockImplementation(() => null);

    const wrapper = mount(IconRail, {
      global: {
        plugins: [router],
        stubs: {
          "v-icon": { template: "<span><slot /></span>" },
        },
      },
    });

    await wrapper.get('a[href="/docs/"]').trigger("click");

    expect(open).toHaveBeenCalledWith(
      "/docs/",
      "_blank",
      "noopener,noreferrer",
    );
  });

  it("marks the ADK section active and delegates internal navigation to the router", async () => {
    const routes = [
      "/workspace",
      "/research",
      "/watchlist",
      "/adk/agents",
      "/strategy/runtime",
      "/strategy/design",
      "/backtest",
      "/account",
      "/risk",
      "/system",
      "/settings",
    ].map((path) => ({ path, component: { template: "<div />" } }));
    const router = createRouter({ history: createMemoryHistory(), routes });
    await router.push("/adk/agents");
    await router.isReady();
    const push = vi.spyOn(router, "push");
    const wrapper = mount(IconRail, {
      global: {
        plugins: [router],
        stubs: { "v-icon": { template: "<span><slot /></span>" } },
      },
    });

    expect(wrapper.get('button[title="智能体"]').classes()).toContain("is-active");
    await wrapper.get('button[title="交易"]').trigger("click");
    expect(push).toHaveBeenCalledWith("/workspace");
    await wrapper.get('button[title="风控"]').trigger("click");
    expect(push).toHaveBeenCalledWith("/risk");
    expect(wrapper.findAll(".tv-iconrail-btn")).toHaveLength(12);
  });
});
