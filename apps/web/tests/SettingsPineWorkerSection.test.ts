// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import SettingsPineWorkerSection from "../src/components/SettingsPineWorkerSection.vue";
import { queryClient } from "../src/composables/serverState";
import { createResponse, flushRequests } from "./helpers";

beforeEach(() => {
  queryClient.clear();
});

afterEach(() => {
  queryClient.clear();
  vi.unstubAllGlobals();
});

describe("SettingsPineWorkerSection", () => {
  it("loads and saves separate PineTS worker limits", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      expect(url).toContain("/api/v1/settings/pine-worker");
      if (init?.method === "PUT") {
        expect(JSON.parse(String(init.body))).toEqual({
          backtestWorkerLimit: 1000,
          instanceWorkerLimit: 1,
          nodeBinaryPath: "/opt/node/bin/node",
        });
        return createResponse({
          backtestWorkerLimit: 1000,
          instanceWorkerLimit: 1,
          nodeBinaryPath: "/opt/node/bin/node",
        });
      }
      return createResponse({
        backtestWorkerLimit: 2,
        instanceWorkerLimit: 10,
        nodeBinaryPath: "/opt/node/bin/node",
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsPineWorkerSection);
    await flushRequests();

    expect(wrapper.text()).toContain("回测 2 / 运行 10");
    await wrapper.get("[data-testid='pine-worker-backtest-limit-input']").setValue("1200");
    await wrapper.get("[data-testid='pine-worker-instance-limit-input']").setValue("0");
    expect(wrapper.text()).toContain("回测 1000，运行实例 1");
    await wrapper.get("[data-testid='pine-worker-limits-save']").trigger("click");
    await flushRequests();

    expect(fetchMock.mock.calls.some((call) => call[1]?.method === "PUT")).toBe(true);
    expect(wrapper.text()).toContain("回测 1000 / 运行 1");
    expect(wrapper.text()).toContain("PineTS Worker 最大值已保存");
  });
});
