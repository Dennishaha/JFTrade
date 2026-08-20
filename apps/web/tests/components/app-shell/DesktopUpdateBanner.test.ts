// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const desktopState = vi.hoisted(() => ({ enabled: false }));
const onAvailable = vi.hoisted(() => vi.fn());
const installUpdate = vi.hoisted(() => vi.fn());
const openExternalUrl = vi.hoisted(() => vi.fn());
const desktopBackend = vi.hoisted(() => ({ value: "wails" }));

vi.mock("../../../src/runtimeConfig", () => ({
  resolveDesktopMode: () => desktopState.enabled,
}));
vi.mock("@/composables/shared/externalLink", () => ({ openExternalUrl }));
vi.mock("@/composables/shared/desktopFacade", () => ({
  desktopFacade: {
    backend: () => desktopBackend.value,
    updates: { install: installUpdate, onAvailable },
  },
}));

import DesktopUpdateBanner from "@/components/app-shell/DesktopUpdateBanner.vue";

afterEach(() => {
  desktopState.enabled = false;
  onAvailable.mockReset();
  installUpdate.mockReset();
  openExternalUrl.mockReset();
  desktopBackend.value = "wails";
});

describe("desktop update banner", () => {
  it("does not subscribe to desktop events in the browser runtime", async () => {
    const wrapper = mount(DesktopUpdateBanner);
    await Promise.resolve();

    expect(onAvailable).not.toHaveBeenCalled();
    expect(wrapper.find(".desktop-update-banner").exists()).toBe(false);
  });

  it("uses the signed Tauri installer only after an explicit click", async () => {
    desktopState.enabled = true;
    desktopBackend.value = "tauri";
    installUpdate.mockResolvedValue(undefined);
    onAvailable.mockImplementation((listener: (event: unknown) => void) => {
      listener({ available: true, latestVersion: "2.4.0" });
      return Promise.resolve(() => undefined);
    });

    const wrapper = mount(DesktopUpdateBanner);
    await vi.waitFor(() => expect(wrapper.text()).toContain("下载并安装"));
    expect(installUpdate).not.toHaveBeenCalled();

    await wrapper.get("button").trigger("click");
    await flushPromises();
    expect(installUpdate).toHaveBeenCalledOnce();
    expect(openExternalUrl).not.toHaveBeenCalled();
  });

  it("shows update events, opens the release URL, and removes its listener", async () => {
    desktopState.enabled = true;
    const cancel = vi.fn();
    onAvailable.mockImplementation((listener: (event: unknown) => void) => {
      listener({
        available: true,
        latestVersion: "v2.4.0",
        releaseUrl: " https://github.com/jftrade/jftrade/releases/tag/v2.4.0 ",
      });
      return Promise.resolve(cancel);
    });

    const wrapper = mount(DesktopUpdateBanner);
    await vi.waitFor(() => expect(wrapper.text()).toContain("JFTrade v2.4.0 已发布。"));
    expect(onAvailable).toHaveBeenCalledWith(expect.any(Function));

    await wrapper.get("button").trigger("click");
    expect(openExternalUrl).toHaveBeenCalledWith("https://github.com/jftrade/jftrade/releases/tag/v2.4.0");
    await wrapper.get("button[aria-label='关闭更新提示']").trigger("click");
    expect(wrapper.find(".desktop-update-banner").exists()).toBe(false);
    wrapper.unmount();
    expect(cancel).toHaveBeenCalledOnce();
  });

  it("removes a listener that resolves after the component is unmounted", async () => {
    desktopState.enabled = true;
    const cancel = vi.fn();
    let resolveListener: ((listener: () => void) => void) | undefined;
    onAvailable.mockReturnValue(
      new Promise<() => void>((resolve) => {
        resolveListener = resolve;
      }),
    );

    const wrapper = mount(DesktopUpdateBanner);
    await vi.waitFor(() => expect(onAvailable).toHaveBeenCalledOnce());
    wrapper.unmount();
    resolveListener?.(cancel);
    await flushPromises();

    expect(cancel).toHaveBeenCalledOnce();
  });
});
