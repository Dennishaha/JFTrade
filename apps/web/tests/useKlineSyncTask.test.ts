import { describe, expect, it, vi } from "vitest";

import { useKlineSyncTask } from "../src/composables/useKlineSyncTask";

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

    vi.useRealTimers();
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

    vi.useRealTimers();
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
