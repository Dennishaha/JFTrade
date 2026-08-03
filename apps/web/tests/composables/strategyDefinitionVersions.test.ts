import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  apiGetPath: vi.fn(),
}));

vi.mock("@/composables/shared/apiClient", () => ({
  apiGetPath: (...args: unknown[]) => apiMocks.apiGetPath(...args),
}));

import {
  fetchStrategyDefinitionVersion,
  fetchStrategyDefinitionVersions,
  normalizeStrategyDefinitionVersionDocument,
  normalizeStrategyDefinitionVersionSummary,
  sortStrategyDefinitionVersions,
  strategyDefinitionVersionQueryKey,
  strategyDefinitionVersionsQueryKey,
} from "@/composables/strategy/strategyDefinitionVersions";

describe("strategy definition versions", () => {
  beforeEach(() => {
    apiMocks.apiGetPath.mockReset();
  });

  it("normalizes summaries, documents, and stable newest-first ordering", () => {
    expect(normalizeStrategyDefinitionVersionSummary(null)).toBeNull();
    expect(normalizeStrategyDefinitionVersionSummary([])).toBeNull();
    expect(normalizeStrategyDefinitionVersionSummary({ version: " " })).toBeNull();
    expect(normalizeStrategyDefinitionVersionDocument("invalid")).toBeNull();

    expect(normalizeStrategyDefinitionVersionSummary({
      version: " 0.2.0 ",
      name: 42,
      updatedAt: "2026-07-02T00:00:00Z",
      isCurrent: "true",
    }, "definition-fallback")).toEqual({
      definitionId: "definition-fallback",
      version: "0.2.0",
      name: "",
      savedAt: "2026-07-02T00:00:00Z",
      isCurrent: false,
    });

    const document = normalizeStrategyDefinitionVersionDocument({
      definitionId: "definition-1",
      version: "0.1.0",
      name: "Initial",
      createdAt: "2026-07-01T00:00:00Z",
      script: "strategy('initial')",
      isCurrent: true,
    });
    expect(document).toMatchObject({
      definitionId: "definition-1",
      version: "0.1.0",
      savedAt: "2026-07-01T00:00:00Z",
      script: "strategy('initial')",
      isCurrent: true,
    });

    expect(sortStrategyDefinitionVersions([
      { definitionId: "d", version: "invalid-a", name: "", savedAt: "invalid", isCurrent: false },
      { definitionId: "d", version: "new", name: "", savedAt: "2026-07-03T00:00:00Z", isCurrent: true },
      { definitionId: "d", version: "invalid-b", name: "", savedAt: "", isCurrent: false },
    ]).map((entry) => entry.version)).toEqual(["new", "invalid-a", "invalid-b"]);
    expect(strategyDefinitionVersionsQueryKey("d")).toEqual(["strategyDefinitions", "d", "versions"]);
    expect(strategyDefinitionVersionQueryKey("d", "v1")).toEqual(["strategyDefinitions", "d", "versions", "v1"]);
  });

  it("fetches list payload variants and filters malformed entries", async () => {
    expect(await fetchStrategyDefinitionVersions("  ")).toEqual([]);
    expect(apiMocks.apiGetPath).not.toHaveBeenCalled();

    apiMocks.apiGetPath.mockResolvedValueOnce({
      versions: [
        { version: "0.1.0", savedAt: "2026-07-01T00:00:00Z" },
        null,
        { version: "" },
        { definitionId: "definition/one", version: "0.2.0", savedAt: "2026-07-02T00:00:00Z" },
      ],
    });
    await expect(fetchStrategyDefinitionVersions(" definition/one ")).resolves.toEqual([
      expect.objectContaining({ definitionId: "definition/one", version: "0.2.0" }),
      expect.objectContaining({ definitionId: "definition/one", version: "0.1.0" }),
    ]);
    expect(apiMocks.apiGetPath).toHaveBeenLastCalledWith(
      "/api/v1/strategy-definitions/{definitionId}/versions",
      "/api/v1/strategy-definitions/definition%2Fone/versions",
    );

    apiMocks.apiGetPath.mockResolvedValueOnce("not-a-list");
    await expect(fetchStrategyDefinitionVersions("definition-2")).resolves.toEqual([]);
  });

  it("fetches immutable snapshots and rejects missing or malformed identifiers", async () => {
    await expect(fetchStrategyDefinitionVersion("", "v1")).rejects.toThrow("策略版本标识不能为空");
    await expect(fetchStrategyDefinitionVersion("definition-1", " ")).rejects.toThrow("策略版本标识不能为空");

    apiMocks.apiGetPath.mockResolvedValueOnce({
      version: "v 1",
      name: "Snapshot",
      script: "strategy('snapshot')",
      savedAt: "2026-07-01T00:00:00Z",
    });
    await expect(fetchStrategyDefinitionVersion(" definition/1 ", " v 1 ")).resolves.toMatchObject({
      definitionId: "definition/1",
      version: "v 1",
      script: "strategy('snapshot')",
    });
    expect(apiMocks.apiGetPath).toHaveBeenLastCalledWith(
      "/api/v1/strategy-definitions/{definitionId}/versions/{version}",
      "/api/v1/strategy-definitions/definition%2F1/versions/v%201",
    );

    apiMocks.apiGetPath.mockResolvedValueOnce({ name: "missing version" });
    await expect(fetchStrategyDefinitionVersion("definition-1", "v2")).rejects.toThrow("策略版本快照格式无效");
  });
});
