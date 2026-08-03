// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import RiskPostureSidebar from "../../../src/components/risk/RiskPostureSidebar.vue";

describe("RiskPostureSidebar", () => {
  it("requests a fresh risk snapshot from the parent view", async () => {
    const wrapper = mount(RiskPostureSidebar, {
      props: {
        posture: { label: "需关注", tone: "warning", hint: "存在待确认风险事件" },
        statusRows: [
          { key: "live", label: "实盘保护", value: "监控中", tone: "success" },
        ],
        facts: [{ label: "最近同步", value: "刚刚" }],
      },
    });

    expect(wrapper.get("[data-status-key='live']").text()).toContain("监控中");
    await wrapper.get("button").trigger("click");

    expect(wrapper.emitted("refresh")).toEqual([[]]);
  });
});
