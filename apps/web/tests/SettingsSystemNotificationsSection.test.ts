// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import SettingsSystemNotificationsSection from "../src/components/SettingsSystemNotificationsSection.vue";
import { queryClient } from "../src/composables/serverState";
import { createResponse, flushRequests } from "./helpers";

type Settings = {
  enabled: boolean;
  mode: "important" | "all" | "custom";
  levels?: string[];
  categories?: string[];
  soundEnabled: boolean;
};

const initialSettings: Settings = {
  enabled: true,
  mode: "custom",
  levels: ["warn"],
  categories: ["broker.connection"],
  soundEnabled: true,
};

afterEach(() => {
  queryClient.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("SettingsSystemNotificationsSection", () => {
  it("persists each supported notification preference through the settings contract", async () => {
    const saved: Settings[] = [];
    let settings = { ...initialSettings };
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/api/v1/settings/system-notifications") && init?.method === "GET") {
        return createResponse(settings);
      }
      if (url.endsWith("/api/v1/settings/system-notifications") && init?.method === "PUT") {
        const next = JSON.parse(String(init.body)) as Settings;
        saved.push(next);
        settings = next;
        return createResponse(next);
      }
      throw new Error(`unexpected request: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsSystemNotificationsSection);
    await flushRequests();

    const checkboxes = wrapper.findAll("input[type='checkbox']");
    await checkboxes[0]!.setValue(false);
    await flushRequests();
    await wrapper.get("input[type='radio'][value='all']").setValue();
    await flushRequests();
    await wrapper.get("input[type='radio'][value='custom']").setValue();
    await flushRequests();
    await checkboxes[1]!.setValue(true);
    await flushRequests();
    await checkboxes[7]!.setValue(true);
    await flushRequests();
    await checkboxes.at(-1)!.setValue(false);
    await flushRequests();

    expect(saved).toEqual([
      expect.objectContaining({ enabled: false }),
      expect.objectContaining({ enabled: false, mode: "all" }),
      expect.objectContaining({ enabled: false, mode: "custom" }),
      expect.objectContaining({ mode: "custom", levels: ["warn", "info"] }),
      expect.objectContaining({ mode: "custom", categories: ["broker.connection", "execution.order"] }),
      expect.objectContaining({ soundEnabled: false }),
    ]);
    expect(wrapper.text()).toContain("系统通知设置已保存。");
    expect(wrapper.text()).toContain("已关闭");
  });

  it("reports delivered, recorded, unknown, and failed notification test outcomes", async () => {
    const deliveries: Array<Record<string, unknown>> = [
      { delivery: { delivered: true, status: "sent" } },
      { delivery: { delivered: false, status: "recorded", message: "桌面通知被系统拦截" } },
      { delivery: { delivered: false, status: "unknown" } },
    ];
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/api/v1/settings/system-notifications")) {
        return createResponse(initialSettings);
      }
      if (url.endsWith("/api/v1/settings/system-notifications/test") && init?.method === "POST") {
        const delivery = deliveries.shift();
        if (delivery != null) return createResponse(delivery);
        return new Response(JSON.stringify({
          ok: false,
          error: { code: "NOTIFICATION_FAILED", message: "系统通知服务不可用" },
          timestamp: "2026-07-16T00:00:00Z",
        }), { status: 503, statusText: "Service Unavailable" });
      }
      throw new Error(`unexpected request: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsSystemNotificationsSection);
    await flushRequests();
    const button = wrapper.get("button");

    await button.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("测试通知已发送到系统通知中心。");

    await button.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("测试通知已记录：桌面通知被系统拦截");

    await button.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("测试通知已记录，但当前桌面通知状态未知。");

    await button.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("系统通知服务不可用");
  });

  it("removes custom filters and keeps a non-Error transport failure actionable", async () => {
    let settings = { ...initialSettings };
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/api/v1/settings/system-notifications") && init?.method === "GET") {
        return createResponse(settings);
      }
      if (url.endsWith("/api/v1/settings/system-notifications") && init?.method === "PUT") {
        const next = JSON.parse(String(init.body)) as Settings;
        if (next.soundEnabled === false) {
          throw "socket closed before notification preferences were saved";
        }
        settings = next;
        return createResponse(next);
      }
      throw new Error(`unexpected request: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsSystemNotificationsSection);
    await flushRequests();
    const checkboxes = wrapper.findAll("input[type='checkbox']");

    await checkboxes[3]!.setValue(false);
    await flushRequests();
    expect(settings.levels).toEqual([]);

    await checkboxes[5]!.setValue(false);
    await flushRequests();
    expect(settings.categories).toEqual([]);

    await checkboxes.at(-1)!.setValue(false);
    await flushRequests();
    expect(wrapper.text()).toContain("保存系统通知设置失败");
  });
});
