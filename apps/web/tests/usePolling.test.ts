import { effectScope, nextTick } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import { usePolling } from "../src/composables/usePolling";

afterEach(() => {
  vi.useRealTimers();
});

describe("usePolling", () => {
  it("waits for an async run to finish before scheduling the next one", async () => {
    vi.useFakeTimers();
    let finishFirstRun: (() => void) | undefined;
    const task = vi.fn(() => {
      if (task.mock.calls.length === 1) {
        return new Promise<void>((resolve) => {
          finishFirstRun = resolve;
        });
      }
      return undefined;
    });
    const scope = effectScope();
    const polling = scope.run(() => usePolling(task, { intervalMs: 1_000 }));

    polling?.start();
    await vi.advanceTimersByTimeAsync(1_000);
    expect(task).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(5_000);
    expect(task).toHaveBeenCalledTimes(1);

    finishFirstRun?.();
    await nextTick();
    await vi.advanceTimersByTimeAsync(999);
    expect(task).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(task).toHaveBeenCalledTimes(2);

    scope.stop();
    await vi.advanceTimersByTimeAsync(5_000);
    expect(task).toHaveBeenCalledTimes(2);
  });

  it("supports immediate execution, task-driven stop, and a maximum run count", async () => {
    vi.useFakeTimers();
    const scope = effectScope();
    const stopTask = vi.fn(({ runCount }: { runCount: number }) => runCount < 2);
    const stopped = scope.run(() =>
      usePolling(stopTask, { intervalMs: 100, immediate: true }),
    );

    stopped?.start();
    await vi.advanceTimersByTimeAsync(100);
    expect(stopTask).toHaveBeenCalledTimes(2);
    expect(stopped?.isActive.value).toBe(false);

    const cappedTask = vi.fn();
    const capped = scope.run(() =>
      usePolling(cappedTask, { intervalMs: 100, maxRuns: 3 }),
    );
    capped?.start();
    await vi.advanceTimersByTimeAsync(1_000);
    expect(cappedTask).toHaveBeenCalledTimes(3);
    expect(capped?.runCount.value).toBe(3);
    expect(capped?.isActive.value).toBe(false);

    capped?.start({ resetRunCount: false });
    await vi.advanceTimersByTimeAsync(1_000);
    expect(cappedTask).toHaveBeenCalledTimes(3);

    capped?.start();
    await vi.advanceTimersByTimeAsync(100);
    expect(cappedTask).toHaveBeenCalledTimes(4);
    scope.stop();
  });

  it("stops after an unexpected task failure and reports the cause", async () => {
    vi.useFakeTimers();
    const cause = new Error("offline");
    const onError = vi.fn();
    const scope = effectScope();
    const polling = scope.run(() =>
      usePolling(
        () => {
          throw cause;
        },
        { intervalMs: 100, onError },
      ),
    );

    polling?.start();
    await vi.advanceTimersByTimeAsync(100);
    expect(onError).toHaveBeenCalledWith(cause);
    expect(polling?.isActive.value).toBe(false);
    scope.stop();
  });
});
