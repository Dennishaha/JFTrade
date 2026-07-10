// @vitest-environment jsdom

import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

const testState = vi.hoisted(() => ({
  appendHandler: null as null | ((event: { data: unknown }) => void),
  cancelAppend: vi.fn(),
  eventsOn: vi.fn(),
  listDays: vi.fn(),
  openFolder: vi.fn(),
  readPage: vi.fn(),
}));

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (...args: unknown[]) => testState.eventsOn(...args),
  },
}));

vi.mock(
  "../src/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplogservice",
  () => ({
    ListDays: (...args: unknown[]) => testState.listDays(...args),
    OpenFolder: (...args: unknown[]) => testState.openFolder(...args),
    ReadPage: (...args: unknown[]) => testState.readPage(...args),
  }),
);

import DesktopLogsPage from "../src/pages/DesktopLogsPage.vue";

type LogLine = {
  level: string;
  text: string;
};

const selectedDay = "2026-07-11";
const wrappers: VueWrapper[] = [];

function line(text: string, level = "INFO"): LogLine {
  return { level, text };
}

function pageResult(options: {
  items: LogLine[];
  offset: number;
  total: number;
}) {
  return {
    day: selectedDay,
    items: options.items,
    limit: 200,
    logDir: "/tmp/jftrade/logs",
    offset: options.offset,
    total: options.total,
  };
}

async function mountPage(initialPage: ReturnType<typeof pageResult>) {
  testState.readPage.mockResolvedValue(initialPage);
  const wrapper = mount(DesktopLogsPage);
  wrappers.push(wrapper);
  await flushPromises();
  return wrapper;
}

async function emitAppend(logLine: LogLine, day = selectedDay): Promise<void> {
  expect(testState.appendHandler).not.toBeNull();
  testState.appendHandler?.({ data: { day, line: logLine } });
  await nextTick();
  await flushPromises();
}

beforeEach(() => {
  testState.appendHandler = null;
  testState.cancelAppend.mockReset();
  testState.eventsOn
    .mockReset()
    .mockImplementation(
      (_name: string, handler: (event: { data: unknown }) => void) => {
        testState.appendHandler = handler;
        return testState.cancelAppend;
      },
    );
  testState.listDays.mockReset().mockResolvedValue([{ day: selectedDay }]);
  testState.openFolder.mockReset().mockResolvedValue(undefined);
  testState.readPage.mockReset();
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount();
  vi.useRealTimers();
});

describe("DesktopLogsPage", () => {
  it("loads the latest page once and adopts the resolved tail offset", async () => {
    const wrapper = await mountPage(
      pageResult({
        items: Array.from({ length: 5 }, (_, index) =>
          line(`INFO latest-${index + 1}`),
        ),
        offset: 400,
        total: 405,
      }),
    );

    expect(testState.readPage).toHaveBeenCalledTimes(1);
    expect(testState.readPage).toHaveBeenCalledWith(
      selectedDay,
      "ALL",
      "",
      -1,
      200,
    );
    expect(wrapper.get(".desktop-logs__meta").text()).toContain(
      "第 3 / 3 页 · 共 405 行",
    );
    expect(wrapper.text()).toContain("INFO latest-5");
    expect(testState.eventsOn).toHaveBeenCalledWith(
      "jftrade:desktop-log:append",
      expect.any(Function),
    );
  });

  it("appends a matching live line without reloading or flashing", async () => {
    vi.useFakeTimers();
    const wrapper = await mountPage(
      pageResult({
        items: [line("INFO existing")],
        offset: 200,
        total: 201,
      }),
    );
    const content = wrapper.get(".desktop-logs__content")
      .element as HTMLElement;
    Object.defineProperty(content, "scrollHeight", {
      configurable: true,
      value: 720,
    });
    content.scrollTop = 0;

    const pendingReload = new Promise<never>(() => {});
    testState.readPage.mockImplementationOnce(() => pendingReload);
    await emitAppend(line("WARN appended", "WARN"));
    await vi.advanceTimersByTimeAsync(250);
    await nextTick();

    expect(testState.readPage).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).toContain("INFO existing");
    expect(wrapper.text()).toContain("WARN appended");
    expect(wrapper.text()).not.toContain("读取中…");
    expect(wrapper.get(".desktop-logs__content").attributes("aria-busy")).toBe(
      "false",
    );
    expect(wrapper.get(".desktop-logs__meta").text()).toContain("共 202 行");
    expect(content.scrollTop).toBe(720);
  });

  it("ignores a live line that does not match the active level filter", async () => {
    const wrapper = await mountPage(
      pageResult({ items: [line("INFO initial")], offset: 0, total: 1 }),
    );
    testState.readPage.mockResolvedValueOnce(
      pageResult({
        items: [line("ERROR retained", "ERROR")],
        offset: 0,
        total: 1,
      }),
    );

    await wrapper.get('select[aria-label="日志级别"]').setValue("ERROR");
    await flushPromises();
    const callsBeforeAppend = testState.readPage.mock.calls.length;
    await emitAppend(line("INFO filtered out"));

    expect(testState.readPage).toHaveBeenCalledTimes(callsBeforeAppend);
    expect(wrapper.text()).toContain("ERROR retained");
    expect(wrapper.text()).not.toContain("INFO filtered out");
    expect(wrapper.get(".desktop-logs__meta").text()).toContain("共 1 行");
  });

  it("moves a matching live line onto a new tail page when the old tail is full", async () => {
    const wrapper = await mountPage(
      pageResult({
        items: Array.from({ length: 200 }, (_, index) =>
          line(`INFO full-page-${index + 1}`),
        ),
        offset: 0,
        total: 200,
      }),
    );

    await emitAppend(line("INFO first-on-new-page"));

    expect(testState.readPage).toHaveBeenCalledTimes(1);
    expect(wrapper.findAll(".desktop-logs__line")).toHaveLength(1);
    expect(wrapper.text()).toContain("INFO first-on-new-page");
    expect(wrapper.text()).not.toContain("INFO full-page-200");
    expect(wrapper.get(".desktop-logs__meta").text()).toContain(
      "第 2 / 2 页 · 共 201 行",
    );
  });

  it("updates only the total when a matching line arrives while viewing history", async () => {
    const wrapper = await mountPage(
      pageResult({
        items: [line("INFO current-tail")],
        offset: 400,
        total: 401,
      }),
    );
    const historyLines = Array.from({ length: 200 }, (_, index) =>
      line(`INFO history-${index + 1}`),
    );
    testState.readPage.mockResolvedValueOnce(
      pageResult({ items: historyLines, offset: 200, total: 401 }),
    );

    await wrapper.get(".desktop-logs__pager button").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("INFO history-1");
    const callsBeforeAppend = testState.readPage.mock.calls.length;

    await emitAppend(line("INFO live-tail-only"));

    expect(testState.readPage).toHaveBeenCalledTimes(callsBeforeAppend);
    expect(wrapper.findAll(".desktop-logs__line")).toHaveLength(200);
    expect(wrapper.text()).toContain("INFO history-1");
    expect(wrapper.text()).toContain("INFO history-200");
    expect(wrapper.text()).not.toContain("INFO live-tail-only");
    expect(wrapper.get(".desktop-logs__meta").text()).toContain(
      "第 2 / 3 页 · 共 402 行",
    );
  });
});
