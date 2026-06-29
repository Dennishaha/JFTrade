// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import SettingsPineWorkerSection from "../src/components/SettingsPineWorkerSection.vue";
import { createResponse, flushRequests } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("SettingsPineWorkerSection", () => {
  it("loads and saves the PineTS worker limit", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      expect(url).toContain("/api/v1/settings/pine-worker");
      if (init?.method === "PUT") {
        expect(JSON.parse(String(init.body))).toEqual({ workerLimit: 1000 });
        return createResponse({ workerLimit: 1000 });
      }
      return createResponse({ workerLimit: 8 });
    });
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsPineWorkerSection);
    await flushRequests();

    expect(wrapper.text()).toContain("上限 8");
    await wrapper.get("[data-testid='pine-worker-limit-input']").setValue("1200");
    expect(wrapper.text()).toContain("当前将保存为 1000");
    await wrapper.get("[data-testid='pine-worker-limit-save']").trigger("click");
    await flushRequests();

    expect(fetchMock.mock.calls.some((call) => call[1]?.method === "PUT")).toBe(true);
    expect(wrapper.text()).toContain("上限 1000");
    expect(wrapper.text()).toContain("PineTS Worker 上限已保存");
  });
});
