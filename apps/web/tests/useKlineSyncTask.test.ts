import { afterEach, describe, expect, it, vi } from "vitest";

import { getLiveEventBus, resetLiveEventBusForTests } from "../src/composables/liveEventBus";
import { useKlineSyncTask } from "../src/composables/useKlineSyncTask";

afterEach(() => {
  resetLiveEventBusForTests();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("useKlineSyncTask", () => {
  it("tracks a completed sync task", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(envelope({ taskId: "sync-1", message: "queued" }))
      .mockResolvedValueOnce(envelope(progress("running")))
      .mockResolvedValueOnce(envelope(progress("completed")));
    vi.stubGlobal("fetch", fetchMock);

    const task = useKlineSyncTask();
    const promise = task.startSync({
      market: "HK",
      code: "00700",
      symbol: "HK.00700",
      intervals: ["5m"],
      startDate: "2026-06-01",
      endDate: "2026-06-25",
      rehabType: "forward",
      sessionScope: "regular",
    });

    await vi.runOnlyPendingTimersAsync();
    await vi.runOnlyPendingTimersAsync();
    await expect(promise).resolves.toMatchObject({ status: "completed" });
    expect(task.syncing.value).toBe(false);
    expect(task.syncProgress.value?.completedIntervals).toBe(1);
  });

  it("records failed sync errors", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("fetch", vi.fn()
      .mockResolvedValueOnce(envelope({ taskId: "sync-2", message: "queued" }))
      .mockResolvedValueOnce(envelope(progress("failed", "OpenD unavailable"))));

    const task = useKlineSyncTask();
    const promise = task.startSync({
      market: "US",
      code: "AAPL",
      symbol: "US.AAPL",
      intervals: ["1m"],
      startDate: "2026-06-01",
      endDate: "2026-06-25",
      rehabType: "forward",
      sessionScope: "regular",
    });

    await vi.runOnlyPendingTimersAsync();
    await expect(promise).resolves.toMatchObject({ status: "failed" });
    expect(task.syncError.value).toContain("OpenD unavailable");
  });

  it("updates progress from the unified live event bus", () => {
    const task = useKlineSyncTask();
    task.syncTaskId.value = "sync-live";
    task.syncing.value = true;

    getLiveEventBus().publish({
      eventId: "sync-live-v2",
      type: "backtest.kline-sync.progress",
      source: "backtest",
      entityId: "sync-live",
      version: 2,
      serverTime: "2026-06-30T00:00:02Z",
      payload: { ...progress("completed"), taskId: "sync-live" },
    });

    expect(task.syncProgress.value?.status).toBe("completed");
    expect(task.syncing.value).toBe(false);
  });
});

function envelope(data: unknown): Response {
  return {
    ok: true,
    status: 200,
    statusText: "OK",
    text: async () => JSON.stringify({ ok: true, data, timestamp: "2026-06-25T00:00:00Z" }),
  } as Response;
}

function progress(status: string, error = "") {
  return {
    taskId: "sync",
    status,
    symbol: "HK.00700",
    currentInterval: "5m",
    totalIntervals: 1,
    completedIntervals: status === "completed" ? 1 : 0,
    totalBatches: 1,
    completedBatches: status === "completed" ? 1 : 0,
    retries: error === "" ? 0 : 2,
    error,
    startedAt: "2026-06-25T00:00:00Z",
    updatedAt: "2026-06-25T00:00:01Z",
  };
}
