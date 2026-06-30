// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import SettingsSecuritySection from "../src/components/SettingsSecuritySection.vue";
import { queryClient } from "../src/composables/serverState";
import { createResponse, flushRequests } from "./helpers";

beforeEach(() => {
  queryClient.clear();
});

afterEach(() => {
  queryClient.clear();
  vi.unstubAllGlobals();
});

describe("SettingsSecuritySection", () => {
  it("loads and saves administrator authentication through the settings query", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      expect(url).toContain("/api/v1/settings/security");

      if (init?.method === "PUT") {
        expect(JSON.parse(String(init.body))).toEqual({ adminAuthRequired: true });
        return createResponse({ adminAuthRequired: true });
      }

      return createResponse({ adminAuthRequired: false });
    });
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsSecuritySection);
    await flushRequests();

    expect(wrapper.text()).toContain("已关闭");
    await wrapper.get("[data-testid='admin-auth-required-toggle']").setValue(true);
    await flushRequests();

    expect(fetchMock.mock.calls.some((call) => call[1]?.method === "PUT")).toBe(true);
    expect(wrapper.text()).toContain("已开启");
  });
});
