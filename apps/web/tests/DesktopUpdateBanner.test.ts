// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const desktopState = vi.hoisted(() => ({ enabled: false }));
const eventsOn = vi.hoisted(() => vi.fn());
const openExternalUrl = vi.hoisted(() => vi.fn());

vi.mock("../src/runtimeConfig", () => ({
  resolveDesktopMode: () => desktopState.enabled,
}));
vi.mock("../src/composables/externalLink", () => ({ openExternalUrl }));
vi.mock("@wailsio/runtime", () => ({ Events: { On: eventsOn } }));

import DesktopUpdateBanner from "../src/components/DesktopUpdateBanner.vue";

afterEach(() => {
  desktopState.enabled = false;
  eventsOn.mockReset();
  openExternalUrl.mockReset();
});

describe("desktop update banner", () => {
  it("does not subscribe to desktop events in the browser runtime", async () => {
    const wrapper = mount(DesktopUpdateBanner);
    await Promise.resolve();

    expect(eventsOn).not.toHaveBeenCalled();
    expect(wrapper.find(".desktop-update-banner").exists()).toBe(false);
  });

  it("shows update events, opens the release URL, and removes its listener", async () => {
    desktopState.enabled = true;
    const cancel = vi.fn();
    eventsOn.mockImplementation((_name: string, listener: (event: unknown) => void) => {
      listener({
        data: {
          available: true,
          latestVersion: "v2.4.0",
          releaseUrl: " https://github.com/jftrade/jftrade/releases/tag/v2.4.0 ",
        },
      });
      return cancel;
    });

    const wrapper = mount(DesktopUpdateBanner);
    await vi.waitFor(() => expect(wrapper.text()).toContain("JFTrade v2.4.0 已发布。"));
    expect(eventsOn).toHaveBeenCalledWith(
      "jftrade:desktop-update:available",
      expect.any(Function),
    );

    await wrapper.get("button").trigger("click");
    expect(openExternalUrl).toHaveBeenCalledWith("https://github.com/jftrade/jftrade/releases/tag/v2.4.0");
    await wrapper.get("button[aria-label='关闭更新提示']").trigger("click");
    expect(wrapper.find(".desktop-update-banner").exists()).toBe(false);
    wrapper.unmount();
    expect(cancel).toHaveBeenCalledOnce();
  });
});
