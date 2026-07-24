import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  apiGet: vi.fn(),
  apiPut: vi.fn(),
}));

vi.mock("../src/composables/apiClient", () => ({
  apiGet: mocks.apiGet,
  apiPut: mocks.apiPut,
}));

import {
  defaultPineWorkerSettings,
  getPineWorkerSettings,
  putPineWorkerSettings,
} from "../src/composables/pineWorkerSettings";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("pine worker settings transport", () => {
  it("fills missing generated response fields with current defaults", async () => {
    mocks.apiGet.mockResolvedValue({});

    await expect(getPineWorkerSettings()).resolves.toEqual(defaultPineWorkerSettings);
    expect(mocks.apiGet).toHaveBeenCalledWith("/api/v1/settings/pine-worker");
  });

  it("preserves current fields returned after an update", async () => {
    const settings = {
      backtestWorkerLimit: 4,
      instanceWorkerLimit: 12,
      nodeBinaryPath: "/opt/node",
    };
    mocks.apiPut.mockResolvedValue(settings);

    await expect(putPineWorkerSettings(settings)).resolves.toEqual(settings);
    expect(mocks.apiPut).toHaveBeenCalledWith("/api/v1/settings/pine-worker", settings);
  });
});
