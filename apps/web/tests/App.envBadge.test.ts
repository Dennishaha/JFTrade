// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import TradingEnvironmentBadge from "../src/components/TradingEnvironmentBadge.vue";

describe("TradingEnvironmentBadge", () => {
  it("renders red badge with REAL TRADING text for REAL env", () => {
    const wrapper = mount(TradingEnvironmentBadge, {
      props: { env: "REAL" },
    });

    expect(wrapper.text()).toContain("REAL TRADING");
    expect(wrapper.attributes("style")).toContain(
      "background: rgb(220, 38, 38)",
    );
  });

  it("renders green badge for PAPER env", () => {
    const wrapper = mount(TradingEnvironmentBadge, {
      props: { env: "PAPER" },
    });

    expect(wrapper.text()).toContain("PAPER");
    expect(wrapper.attributes("style")).toContain(
      "background: rgb(22, 163, 74)",
    );
  });

  it("renders grey badge with env value for unknown env", () => {
    const wrapper = mount(TradingEnvironmentBadge, {
      props: { env: "UNKNOWN" },
    });

    expect(wrapper.text()).toContain("UNKNOWN");
    expect(wrapper.attributes("style")).toContain(
      "background: rgb(107, 114, 128)",
    );
  });
});
