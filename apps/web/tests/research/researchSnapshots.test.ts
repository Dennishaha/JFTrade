import { afterEach, describe, expect, it, vi } from "vitest";
import { effectScope, ref } from "vue";
import { flushPromises } from "@vue/test-utils";

const mocks = vi.hoisted(() => ({
  fetchEnvelopeWithInit: vi.fn(),
}));

vi.mock("@/composables/shared/apiClient", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/composables/shared/apiClient")>();
  return {
    ...actual,
    fetchEnvelopeWithInit: mocks.fetchEnvelopeWithInit,
    apiPostPath: (_template: string, path: string, body: unknown) =>
      mocks.fetchEnvelopeWithInit(path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      }),
  };
});

import {
  fetchResearchSnapshots,
  ResearchSnapshotBatchError,
  useResearchSnapshots,
} from "../../src/components/research/researchSnapshots";

afterEach(() => {
  mocks.fetchEnvelopeWithInit.mockReset();
});

describe("fetchResearchSnapshots", () => {
  it("preserves successful quotes and exposes per-instrument errors", async () => {
    mocks.fetchEnvelopeWithInit.mockResolvedValueOnce({
      quotes: [{ instrumentId: "US.AAPL", price: 215 }],
      errors: [
        {
          instrumentId: "hk.HTIMAIN",
          code: "UNKNOWN_SECURITY",
          message: "未知股票 HTIMAIN",
        },
      ],
    });

    const result = fetchResearchSnapshots(["US.AAPL", "HK.HTIMAIN"], "futu");

    await expect(result).rejects.toBeInstanceOf(ResearchSnapshotBatchError);
    await expect(result).rejects.toMatchObject({
      quotes: [{ instrumentId: "US.AAPL", price: 215 }],
      errors: [
        {
          instrumentId: "HK.HTIMAIN",
          code: "UNKNOWN_SECURITY",
          message: "未知股票 HTIMAIN",
        },
      ],
    });
    await expect(result).rejects.toThrow("1 个标的暂不可用，已保留其余行情");
  });

  it("keeps successful quotes visible while surfacing partial failures", async () => {
    mocks.fetchEnvelopeWithInit.mockResolvedValueOnce({
      quotes: [{ instrumentId: "US.AAPL", price: 215 }],
      errors: [{ instrumentId: "HK.HTIMAIN", message: "未知股票 HTIMAIN" }],
    });
    const scope = effectScope();
    const state = scope.run(() =>
      useResearchSnapshots(ref(["US.AAPL", "HK.HTIMAIN"]), ref("futu")),
    )!;

    await flushPromises();

    expect(state.entries.value).toEqual([
      { instrumentId: "US.AAPL", price: 215 },
    ]);
    expect(state.byInstrumentId.value["US.AAPL"]).toMatchObject({ price: 215 });
    expect(state.error.value).toBe("1 个标的暂不可用，已保留其余行情");
    expect(state.loading.value).toBe(false);
    scope.stop();
  });

  it("summarizes unavailable instruments when no quotes can be retained", async () => {
    mocks.fetchEnvelopeWithInit.mockResolvedValueOnce({
      quotes: [],
      errors: [
        {
          instrumentId: "US.BBKCF",
          code: "UNKNOWN_SECURITY",
          message: "未知股票 BBKCF",
        },
        {
          instrumentId: "US.KXIAY",
          code: "UNSUPPORTED_OTC",
          message: "暂不提供美股 OTC 市场行情 KXIAY",
        },
      ],
    });
    const scope = effectScope();
    const state = scope.run(() =>
      useResearchSnapshots(ref(["US.BBKCF", "US.KXIAY"]), ref("futu")),
    )!;

    await flushPromises();

    expect(state.entries.value).toEqual([]);
    expect(state.error.value).toBe("2 个标的暂不可用");
    expect(state.loading.value).toBe(false);
    scope.stop();
  });

  it("chunks large catalogs at 200 IDs with bounded concurrency and stable merge order", async () => {
    let active = 0;
    let maximumActive = 0;
    mocks.fetchEnvelopeWithInit.mockImplementation(
      async (_path: string, init: RequestInit) => {
        active += 1;
        maximumActive = Math.max(maximumActive, active);
        const instrumentIds = JSON.parse(String(init.body)).instrumentIds as string[];
        await Promise.resolve();
        active -= 1;
        return {
          quotes: instrumentIds.map((instrumentId) => ({ instrumentId })),
        };
      },
    );
    const instrumentIds = Array.from(
      { length: 1_005 },
      (_, index) => `US.FUND${String(index).padStart(4, "0")}`,
    );

    const entries = await fetchResearchSnapshots(instrumentIds, "futu", true);

    expect(mocks.fetchEnvelopeWithInit).toHaveBeenCalledTimes(6);
    const batches = mocks.fetchEnvelopeWithInit.mock.calls.map(([, init]) =>
      JSON.parse(String((init as RequestInit).body)).instrumentIds as string[],
    );
    expect(batches.map((batch) => batch.length)).toEqual([200, 200, 200, 200, 200, 5]);
    expect(maximumActive).toBeLessThanOrEqual(3);
    expect(
      mocks.fetchEnvelopeWithInit.mock.calls.every(([path]) =>
        String(path) === "/api/v1/watchlist/quotes/batch",
      ),
    ).toBe(true);
    expect(entries.map((entry) => entry.instrumentId)).toEqual(instrumentIds);
  });
});
