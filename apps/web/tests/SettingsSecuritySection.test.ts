// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import SettingsSecuritySection from "../src/components/SettingsSecuritySection.vue";
import { queryClient } from "../src/composables/serverState";
import { createResponse, flushRequests } from "./helpers";

beforeEach(() => {
  queryClient.clear();
  window.__JFTRADE_RUNTIME_CONFIG__ = { desktopMode: true };
});

afterEach(() => {
  queryClient.clear();
  delete window.__JFTRADE_RUNTIME_CONFIG__;
  vi.unstubAllGlobals();
});

describe("SettingsSecuritySection", () => {
  it("requires a confirmed password before first enabling Web access", async () => {
    const fetchMock = vi.fn(
      async (input: string | URL | Request, init?: RequestInit) => {
        const url = String(input);
        expect(url).toContain("/api/v1/settings/security");

        if (init?.method === "PUT") {
          expect(JSON.parse(String(init.body))).toEqual({
            webAccessEnabled: true,
            publicAccessEnabled: false,
            webPort: 6688,
            newPassword: "browser-web-passphrase",
          });
          return createResponse({
            webAccessEnabled: true,
            publicAccessEnabled: false,
            passwordConfigured: true,
          });
        }

        return createResponse({
          webAccessEnabled: false,
          publicAccessEnabled: false,
          passwordConfigured: false,
        });
      },
    );
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsSecuritySection);
    await flushRequests();

    expect(wrapper.text()).toContain("默认仅使用桌面应用，无需密码");
    expect(wrapper.text()).toContain("未开启");

    await wrapper.get("[data-testid='web-access-enabled-toggle']").setValue(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await wrapper.get("[data-testid='web-access-settings-form']").trigger("submit");
    expect(wrapper.text()).toContain("首次开启 Web 访问时，请设置 Web 访问密码");
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await wrapper.get("[data-testid='web-access-new-password']").setValue("short");
    await wrapper.get("[data-testid='web-access-confirm-password']").setValue("short");
    await wrapper.get("[data-testid='web-access-settings-form']").trigger("submit");
    expect(wrapper.text()).toContain("至少需要 15 个字符");
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await wrapper
      .get("[data-testid='web-access-new-password']")
      .setValue("browser-web-passphrase");
    await wrapper
      .get("[data-testid='web-access-confirm-password']")
      .setValue("browser-web-passphrase");
    await wrapper.get("[data-testid='web-access-settings-form']").trigger("submit");
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain("已开启 · 仅本机");
    expect(wrapper.get("[data-testid='web-access-local-url']").text()).toContain(
      "127.0.0.1",
    );
  });

  it("saves public exposure explicitly without resending a configured password", async () => {
    const fetchMock = vi.fn(
      async (_input: string | URL | Request, init?: RequestInit) => {
        if (init?.method === "PUT") {
          expect(JSON.parse(String(init.body))).toEqual({
            webAccessEnabled: true,
            publicAccessEnabled: true,
            webPort: 7443,
          });
          return createResponse({
            webAccessEnabled: true,
            publicAccessEnabled: true,
            webPort: 7443,
            passwordConfigured: true,
          });
        }
        return createResponse({
          webAccessEnabled: true,
          publicAccessEnabled: false,
          passwordConfigured: true,
        });
      },
    );
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsSecuritySection);
    await flushRequests();

    await wrapper
      .get("[data-testid='public-access-enabled-toggle']")
      .setValue(true);
    await wrapper.get("[data-testid='web-access-port']").setValue(7443);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).toContain("当前仅提供 HTTP");
    expect(wrapper.text()).toContain("立即监听所有网络接口");

    await wrapper.get("[data-testid='web-access-settings-form']").trigger("submit");
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain("已开启 · 目标：其他设备");
    expect(wrapper.text()).toContain("HTTPS 反向代理");
    expect(wrapper.get("[data-testid='web-access-network-hint']").text()).toContain(
      "<这台电脑的局域网 IP>",
    );
    expect(wrapper.get("[data-testid='web-access-network-hint']").text()).toContain(
      ":7443",
    );
  });

  it("keeps Web access settings read-only in a browser session", async () => {
    window.__JFTRADE_RUNTIME_CONFIG__ = {
      authRequired: true,
      desktopMode: false,
    };
    const fetchMock = vi.fn(async () =>
      createResponse({
        webAccessEnabled: true,
        publicAccessEnabled: false,
        passwordConfigured: true,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsSecuritySection);
    await flushRequests();

    expect(wrapper.text()).toContain("浏览器中仅可查看");
    expect(wrapper.text()).toContain("请在 JFTrade 桌面端修改");
    expect(wrapper.find("[data-testid='web-access-settings-form']").exists()).toBe(false);
    expect(wrapper.find("[data-testid='web-access-enabled-toggle']").exists()).toBe(false);
    expect(wrapper.get("[data-testid='web-access-local-url']").text()).toContain(
      "localhost",
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("keeps an unknown state read-only until a failed settings load is retried", async () => {
    let attempts = 0;
    const fetchMock = vi.fn(async () => {
      attempts += 1;
      if (attempts === 1) {
        throw new Error("network failed");
      }
      return createResponse({
        webAccessEnabled: true,
        publicAccessEnabled: false,
        passwordConfigured: true,
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsSecuritySection);
    await flushRequests();

    expect(wrapper.text()).toContain("状态未知");
    expect(wrapper.text()).toContain("无法读取 Web 访问设置");
    expect(
      wrapper.get("[data-testid='web-access-enabled-toggle']").attributes("disabled"),
    ).toBeDefined();
    expect(
      wrapper.get("[data-testid='save-web-access-settings']").attributes("disabled"),
    ).toBeDefined();

    const retryButton = wrapper
      .findAll("button")
      .find((node) => node.text() === "重新读取");
    expect(retryButton).toBeDefined();
    await retryButton?.trigger("click");
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain("已开启 · 仅本机");
    expect(wrapper.text()).not.toContain("状态未知");
  });

  it("does not save a password that became hidden when Web access was turned off", async () => {
    const fetchMock = vi.fn(
      async (_input: string | URL | Request, init?: RequestInit) => {
        if (init?.method === "PUT") {
          expect(JSON.parse(String(init.body))).toEqual({
            webAccessEnabled: false,
            publicAccessEnabled: false,
            webPort: 6688,
          });
          return createResponse({
            webAccessEnabled: false,
            publicAccessEnabled: false,
            passwordConfigured: true,
          });
        }
        return createResponse({
          webAccessEnabled: true,
          publicAccessEnabled: false,
          passwordConfigured: true,
        });
      },
    );
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsSecuritySection);
    await flushRequests();
    await wrapper
      .get("[data-testid='web-access-new-password']")
      .setValue("replacement browser password");
    await wrapper
      .get("[data-testid='web-access-confirm-password']")
      .setValue("replacement browser password");
    await wrapper.get("[data-testid='web-access-enabled-toggle']").setValue(false);
    await wrapper.get("[data-testid='web-access-settings-form']").trigger("submit");
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain("Web 访问已关闭");
  });
});
