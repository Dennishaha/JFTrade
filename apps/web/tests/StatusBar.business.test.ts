// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

const mocks = vi.hoisted(() => ({
  consoleData: null as Record<string, unknown> | null,
  liveStream: null as Record<string, unknown> | null,
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => mocks.consoleData,
}));

vi.mock("../src/composables/useSharedLiveStream", () => ({
  useSharedLiveStream: () => mocks.liveStream,
}));

import StatusBar from "../src/layout/StatusBar.vue";

beforeEach(() => {
  mocks.consoleData = {
    selectedBrokerAccount: ref(null),
    systemStatus: ref({
      persistence: { engine: "sqlite", status: "healthy" },
      broker: { displayName: "Futu" },
      defaultTradingEnvironment: "SIMULATE",
    }),
    liveStreamCheckedAt: ref("2026-07-18T12:44:06Z"),
    consoleRefreshError: ref(""),
    realTradeKillSwitchState: ref({ killSwitchActive: false }),
  };
  mocks.liveStream = {
    connectionState: ref("connected"),
    lastHeartbeat: ref("2026-07-18T12:44:05Z"),
  };
});

afterEach(() => {
  vi.useRealTimers();
});

describe("StatusBar", () => {
  it("hides healthy event-stream and realtime-channel diagnostics", () => {
    const wrapper = mount(StatusBar);

    expect(wrapper.text()).not.toContain("事件流");
    expect(wrapper.text()).not.toContain("实时通道");
    expect(wrapper.find('[data-testid="console-refresh-issue"]').exists()).toBe(false);
  });

  it("leaves shared connection failures to the provider indicator", async () => {
    const connectionState = mocks.liveStream!.connectionState as ReturnType<typeof ref>;
    const consoleRefreshError = mocks.consoleData!.consoleRefreshError as ReturnType<typeof ref>;
    const wrapper = mount(StatusBar);

    connectionState.value = "error";
    consoleRefreshError.value = "refresh failed";
    await nextTick();

    expect(wrapper.find('[data-testid="console-refresh-issue"]').exists()).toBe(false);
  });

  it("shows only an independent console refresh failure with diagnostic times", async () => {
    const consoleRefreshError = mocks.consoleData!.consoleRefreshError as ReturnType<typeof ref>;
    const wrapper = mount(StatusBar);

    consoleRefreshError.value = "refresh failed";
    await nextTick();

    const issue = wrapper.get('[data-testid="console-refresh-issue"]');
    const title = issue.attributes("title");
    expect(issue.text()).toContain("控制台刷新异常");
    expect(title).toContain("实时通道已连接");
    expect(title).toContain("错误：refresh failed");
    expect(title).toContain("最近控制台刷新：");
    expect(title).toContain("最近实时心跳：");
    expect(title).not.toContain("：—");
  });

  it("advances the local clock once per second", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-18T12:44:05Z"));
    const wrapper = mount(StatusBar);
    const initialClock = wrapper.text();

    await vi.advanceTimersByTimeAsync(1_000);
    await nextTick();

    expect(wrapper.text()).not.toBe(initialClock);
    wrapper.unmount();
  });
});
