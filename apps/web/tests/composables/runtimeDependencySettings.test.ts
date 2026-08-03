import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  apiGet: vi.fn(),
  apiPut: vi.fn(),
}));

vi.mock("@/composables/shared/apiClient", () => mocks);

import {
  getRuntimeDependencySettings,
  putRuntimeDependencySettings,
} from "@/composables/settings/runtimeDependencySettings";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("runtimeDependencySettings", () => {
  it("normalizes sparse reads and writes the independent Python path contract", async () => {
    mocks.apiGet.mockResolvedValue({});
    await expect(getRuntimeDependencySettings()).resolves.toEqual({
      pythonBinaryPath: "",
    });
    expect(mocks.apiGet).toHaveBeenCalledWith(
      "/api/v1/settings/runtime-dependencies",
    );

    const settings = { pythonBinaryPath: "/opt/python/bin/python3" };
    mocks.apiPut.mockResolvedValue(settings);
    await expect(putRuntimeDependencySettings(settings)).resolves.toEqual(
      settings,
    );
    expect(mocks.apiPut).toHaveBeenCalledWith(
      "/api/v1/settings/runtime-dependencies",
      settings,
    );
  });
});
