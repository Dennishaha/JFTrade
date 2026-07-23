// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import SparklineChart from "../../src/components/research/SparklineChart.vue";

describe("SparklineChart", () => {
  it("renders line and area paths for points", () => {
    const wrapper = mount(SparklineChart, {
      props: { points: [1, 3, 2, 5, 4], direction: "up" },
    });
    const line = wrapper.get("path.sparkline-chart__line");
    expect(line.attributes("d")).toMatch(/^M/);
    expect(line.attributes("d")).toContain("L");
    const area = wrapper.get("path.sparkline-chart__area");
    expect(area.attributes("d")).toContain("Z");
    expect(area.attributes("fill")).toContain("url(#");
    expect(wrapper.get("linearGradient").exists()).toBe(true);
  });

  it("applies direction classes for up/down/flat", () => {
    for (const direction of ["up", "down", "flat"] as const) {
      const wrapper = mount(SparklineChart, {
        props: { points: [1, 2, 3], direction },
      });
      expect(wrapper.get("svg").classes()).toContain(
        `sparkline-chart--${direction}`,
      );
    }
  });

  it("uses default size 120x48", () => {
    const wrapper = mount(SparklineChart, {
      props: { points: [1, 2, 3] },
    });
    const svg = wrapper.get("svg");
    expect(svg.attributes("width")).toBe("120");
    expect(svg.attributes("height")).toBe("48");
  });

  it("renders an empty placeholder without enough points", () => {
    const wrapper = mount(SparklineChart, { props: { points: [1] } });
    expect(wrapper.find("path.sparkline-chart__line").exists()).toBe(false);
    expect(wrapper.get("rect.sparkline-chart__placeholder").exists()).toBe(true);

    const empty = mount(SparklineChart, { props: { points: [] } });
    expect(empty.get("rect.sparkline-chart__placeholder").exists()).toBe(true);
  });
});
