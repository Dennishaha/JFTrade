import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchEnvelopeWithInit: vi.fn(),
}));

vi.mock("../../src/composables/apiClient", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/apiClient")>();
  return {
    ...actual,
    fetchEnvelopeWithInit: mocks.fetchEnvelopeWithInit,
  };
});

import { fetchResearchSnapshots } from "../../src/components/research/researchSnapshots";

afterEach(() => {
  mocks.fetchEnvelopeWithInit.mockReset();
});

describe("fetchResearchSnapshots", () => {
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
          entries: instrumentIds.map((instrumentId) => ({ instrumentId })),
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
        String(path).includes("brokerId=futu") && String(path).includes("refresh=true"),
      ),
    ).toBe(true);
    expect(entries.map((entry) => entry.instrumentId)).toEqual(instrumentIds);
  });
});
