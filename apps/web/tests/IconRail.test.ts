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
});
