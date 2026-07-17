// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const authMocks = vi.hoisted(() => ({
  setCSRFToken: vi.fn(),
  webLogin: vi.fn(),
  webSession: vi.fn(),
}));

vi.mock("../src/composables/apiClient", () => {
  class ApiClientError extends Error {
    constructor(
      message: string,
      readonly code: string,
      readonly status: number,
    ) {
      super(message);
    }
  }
  return {
    ApiClientError,
    setCSRFToken: authMocks.setCSRFToken,
    webLogin: authMocks.webLogin,
    webSession: authMocks.webSession,
  };
});

import AuthGate from "../src/components/AuthGate.vue";
import { ApiClientError } from "../src/composables/apiClient";

beforeEach(() => {
  authMocks.webSession.mockResolvedValue({ authenticated: false });
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("AuthGate", () => {
  it("keeps the form usable after an anonymous session and explains invalid passwords", async () => {
    authMocks.webLogin.mockRejectedValue(
      new ApiClientError("denied", "INVALID_PASSWORD", 401),
    );
    const wrapper = mount(AuthGate);
    await flushPromises();

    const input = wrapper.get("#web-access-password");
    expect((input.element as HTMLInputElement).disabled).toBe(false);
    await input.setValue("wrong-password");
    await wrapper.get("form").trigger("submit");
    await flushPromises();

    expect(authMocks.webLogin).toHaveBeenCalledWith("wrong-password");
    expect(wrapper.text()).toContain("Web 访问密码不正确，请重试。");
    expect(wrapper.emitted("authenticated")).toBeUndefined();
  });

  it("sets the CSRF token and emits only after login creates an authenticated session", async () => {
    authMocks.webLogin.mockResolvedValue({ authenticated: true });
    authMocks.webSession
      .mockResolvedValueOnce({ authenticated: false })
      .mockResolvedValueOnce({ authenticated: true, csrfToken: "csrf-123" });
    const wrapper = mount(AuthGate);
    await flushPromises();

    await wrapper.get("#web-access-password").setValue("correct-password");
    await wrapper.get("form").trigger("submit");
    await flushPromises();

    expect(authMocks.webSession).toHaveBeenCalledTimes(2);
    expect(authMocks.setCSRFToken).toHaveBeenCalledWith("csrf-123");
    expect(wrapper.emitted("authenticated")).toEqual([[]]);
    expect((wrapper.get("#web-access-password").element as HTMLInputElement).value).toBe("");
  });

  it("reports a non-persistent login session and a failing startup session clearly", async () => {
    authMocks.webLogin.mockResolvedValue({ authenticated: true });
    authMocks.webSession
      .mockResolvedValueOnce({ authenticated: false })
      .mockResolvedValueOnce({ authenticated: false });
    const wrapper = mount(AuthGate);
    await flushPromises();
    await wrapper.get("#web-access-password").setValue("same-host-check");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper.text()).toContain("登录会话未生效，请确认前端访问地址与 API 地址使用相同主机名。");

    authMocks.webSession.mockRejectedValueOnce(
      new ApiClientError("origin", "ORIGIN_FORBIDDEN", 403),
    );
    const failingWrapper = mount(AuthGate);
    await flushPromises();
    expect(failingWrapper.text()).toContain("当前浏览器地址不受信任，请使用设置页显示的访问地址。");
  });

  it("maps each actionable Web-access login failure to its recovery guidance", async () => {
    const cases = [
      ["LOGIN_RATE_LIMITED", "尝试次数过多，请稍后再试。"],
      ["WEB_ACCESS_DISABLED", "Web 访问尚未开启"],
      ["REMOTE_WEB_ACCESS_DISABLED", "当前仅允许本机浏览器访问"],
      ["WEB_AUTH_UNAVAILABLE", "Web 登录暂时不可用"],
      ["WEB_AUTH_CONFIGURATION_CHANGED", "Web 访问设置刚刚发生变化"],
    ] as const;

    for (const [code, expected] of cases) {
      authMocks.webSession.mockResolvedValue({ authenticated: false });
      authMocks.webLogin.mockRejectedValue(new ApiClientError("denied", code, 403));
      const wrapper = mount(AuthGate);
      await flushPromises();
      await wrapper.get("#web-access-password").setValue("long-password");
      await wrapper.get("form").trigger("submit");
      await flushPromises();
      expect(wrapper.text()).toContain(expected);
      wrapper.unmount();
    }

    authMocks.webSession.mockRejectedValue(new Error("network unavailable"));
    const unavailable = mount(AuthGate);
    await flushPromises();
    expect(unavailable.text()).toContain("无法确认 Web 登录状态");
  });

  it("accepts an already-authenticated startup session and ignores an empty submit", async () => {
    authMocks.webSession.mockResolvedValue({ authenticated: true, csrfToken: "startup-csrf" });
    const authenticated = mount(AuthGate);
    await flushPromises();
    expect(authMocks.setCSRFToken).toHaveBeenCalledWith("startup-csrf");
    expect(authenticated.emitted("authenticated")).toEqual([[]]);

    authMocks.webSession.mockResolvedValue({ authenticated: false });
    const anonymous = mount(AuthGate);
    await flushPromises();
    await anonymous.get("form").trigger("submit");
    expect(authMocks.webLogin).not.toHaveBeenCalled();
  });
});
