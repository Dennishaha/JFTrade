// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const bindings = vi.hoisted(() => ({
  snapshot: vi.fn(),
  quit: vi.fn(),
  openFolder: vi.fn(),
}));

vi.mock(
  "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop",
  () => ({
    DesktopStartupService: {
      Snapshot: bindings.snapshot,
      Quit: bindings.quit,
    },
    DesktopLogService: { OpenFolder: bindings.openFolder },
  }),
);

import DesktopStartupGate from "@/components/app-shell/DesktopStartupGate.vue";

afterEach(() => {
  vi.useRealTimers();
  bindings.snapshot.mockReset();
  bindings.quit.mockReset();
  bindings.openFolder.mockReset();
});

describe("desktop startup gate", () => {
  it("keeps the loading screen visible until the local API is ready", async () => {
    vi.useFakeTimers();
    bindings.snapshot
      .mockResolvedValueOnce({
        state: "starting",
        phase: "api-starting",
        message: "正在启动本地 API 与行情服务…",
        startedAt: "2026-08-02T00:00:00Z",
      })
      .mockResolvedValueOnce({
        state: "ready",
        phase: "api-ready",
        message: "本地服务已就绪",
        startedAt: "2026-08-02T00:00:00Z",
      });
    const wrapper = mount(DesktopStartupGate);
    await flushPromises();

    expect(wrapper.text()).toContain("正在启动本地 API 与行情服务");
    expect(wrapper.emitted("ready")).toBeUndefined();

    await vi.advanceTimersByTimeAsync(200);
    await flushPromises();
    expect(wrapper.emitted("ready")).toHaveLength(1);
  });

  it("shows safe failure actions without retrying startup", async () => {
    bindings.snapshot.mockResolvedValue({
      state: "failed",
      phase: "api-failed",
      message: "本地服务启动失败，请查看日志后重新启动应用。",
      startedAt: "2026-08-02T00:00:00Z",
    });
    bindings.openFolder.mockResolvedValue(undefined);
    const wrapper = mount(DesktopStartupGate);
    await flushPromises();

    expect(wrapper.text()).toContain("本地服务启动失败");
    expect(wrapper.text()).not.toContain("重试");
    const buttons = wrapper.findAll("button");
    await buttons[0].trigger("click");
    await buttons[1].trigger("click");
    expect(bindings.openFolder).toHaveBeenCalledOnce();
    expect(bindings.quit).toHaveBeenCalledOnce();
    expect(wrapper.emitted("ready")).toBeUndefined();
  });

  it("recovers when the Wails bridge becomes callable after mount", async () => {
    vi.useFakeTimers();
    bindings.snapshot
      .mockRejectedValueOnce(new Error("bridge not ready"))
      .mockResolvedValueOnce({
        state: "ready",
        phase: "api-ready",
        message: "",
        startedAt: "2026-08-02T00:00:00Z",
      });
    const wrapper = mount(DesktopStartupGate);
    await flushPromises();

    expect(wrapper.text()).toContain("正在连接桌面启动服务");
    await vi.advanceTimersByTimeAsync(200);
    await flushPromises();
    expect(wrapper.emitted("ready")).toHaveLength(1);
  });

  it("reports log-folder action failures safely", async () => {
    bindings.snapshot.mockResolvedValue({
      state: "failed",
      phase: "api-failed",
      message: "启动失败",
      startedAt: "2026-08-02T00:00:00Z",
    });
    bindings.openFolder
      .mockRejectedValueOnce(new Error("日志目录不可用"))
      .mockRejectedValueOnce("再次失败");
    const wrapper = mount(DesktopStartupGate);
    await flushPromises();

    const openLogs = wrapper.findAll("button")[0];
    await openLogs.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("日志目录不可用");

    await openLogs.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("再次失败");
  });

  it("stops an in-flight poll and clears scheduled polling on unmount", async () => {
    vi.useFakeTimers();
    let resolveSnapshot!: (value: {
      state: string;
      phase: string;
      message: string;
      startedAt: string;
    }) => void;
    bindings.snapshot.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveSnapshot = resolve;
        }),
    );
    const inFlight = mount(DesktopStartupGate);
    await flushPromises();
    inFlight.unmount();
    resolveSnapshot({
      state: "ready",
      phase: "api-ready",
      message: "已就绪",
      startedAt: "2026-08-02T00:00:00Z",
    });
    await flushPromises();
    expect(inFlight.emitted("ready")).toBeUndefined();

    bindings.snapshot.mockResolvedValueOnce({
      state: "starting",
      phase: "api-starting",
      message: "",
      startedAt: "2026-08-02T00:00:00Z",
    });
    const scheduled = mount(DesktopStartupGate);
    await flushPromises();
    expect(scheduled.text()).toContain("正在启动本地服务");
    scheduled.unmount();
    await vi.advanceTimersByTimeAsync(200);
    expect(bindings.snapshot).toHaveBeenCalledTimes(2);
  });
});
