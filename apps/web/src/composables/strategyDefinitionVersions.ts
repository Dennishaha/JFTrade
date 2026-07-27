import type { StrategyDefinitionDocument } from "@/types";

import { apiGetPath } from "./apiClient";

/**
 * A saved, immutable snapshot of a strategy definition.  These types are
 * intentionally local until the generated OpenAPI contract is refreshed.
 */
export interface StrategyDefinitionVersionSummary {
  definitionId: string;
  version: string;
  name: string;
  savedAt: string;
  isCurrent: boolean;
}

export type StrategyDefinitionVersionDocument =
  StrategyDefinitionDocument & StrategyDefinitionVersionSummary;

function recordOf(value: unknown): Record<string, unknown> | null {
  if (value == null || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function flag(value: unknown): boolean {
  return value === true;
}

function readVersionEntries(payload: unknown): unknown[] {
  if (Array.isArray(payload)) {
    return payload;
  }
  const record = recordOf(payload);
  return Array.isArray(record?.versions) ? record.versions : [];
}

function timestamp(value: string): number {
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

/** Sort from newest to oldest while preserving a stable response order. */
export function sortStrategyDefinitionVersions(
  versions: readonly StrategyDefinitionVersionSummary[],
): StrategyDefinitionVersionSummary[] {
  return versions
    .map((version, index) => ({ version, index }))
    .sort((left, right) => {
      const order = timestamp(right.version.savedAt) - timestamp(left.version.savedAt);
      return order === 0 ? left.index - right.index : order;
    })
    .map(({ version }) => version);
}

export function normalizeStrategyDefinitionVersionSummary(
  value: unknown,
  definitionId = "",
): StrategyDefinitionVersionSummary | null {
  const record = recordOf(value);
  if (record == null) {
    return null;
  }
  const version = text(record.version);
  if (version === "") {
    return null;
  }
  return {
    definitionId: text(record.definitionId) || definitionId,
    version,
    name: text(record.name),
    savedAt: text(record.savedAt) || text(record.updatedAt) || text(record.createdAt),
    isCurrent: flag(record.isCurrent),
  };
}

export function normalizeStrategyDefinitionVersionDocument(
  value: unknown,
  definitionId = "",
): StrategyDefinitionVersionDocument | null {
  const record = recordOf(value);
  const summary = normalizeStrategyDefinitionVersionSummary(value, definitionId);
  if (record == null || summary == null) {
    return null;
  }
  return {
    ...(record as StrategyDefinitionDocument),
    ...summary,
  };
}

export function strategyDefinitionVersionsQueryKey(definitionId: string) {
  return ["strategyDefinitions", definitionId, "versions"] as const;
}

export function strategyDefinitionVersionQueryKey(
  definitionId: string,
  version: string,
) {
  return ["strategyDefinitions", definitionId, "versions", version] as const;
}

export async function fetchStrategyDefinitionVersions(
  definitionId: string,
): Promise<StrategyDefinitionVersionSummary[]> {
  const normalizedDefinitionId = definitionId.trim();
  if (normalizedDefinitionId === "") {
    return [];
  }
  const payload = await apiGetPath(
    "/api/v1/strategy-definitions/{definitionId}/versions",
    `/api/v1/strategy-definitions/${encodeURIComponent(normalizedDefinitionId)}/versions`,
  );
  return sortStrategyDefinitionVersions(
    readVersionEntries(payload)
      .map((entry) =>
        normalizeStrategyDefinitionVersionSummary(entry, normalizedDefinitionId),
      )
      .filter((entry): entry is StrategyDefinitionVersionSummary => entry != null),
  );
}

export async function fetchStrategyDefinitionVersion(
  definitionId: string,
  version: string,
): Promise<StrategyDefinitionVersionDocument> {
  const normalizedDefinitionId = definitionId.trim();
  const normalizedVersion = version.trim();
  if (normalizedDefinitionId === "" || normalizedVersion === "") {
    throw new Error("策略版本标识不能为空");
  }
  const payload = await apiGetPath(
    "/api/v1/strategy-definitions/{definitionId}/versions/{version}",
    `/api/v1/strategy-definitions/${encodeURIComponent(normalizedDefinitionId)}/versions/${encodeURIComponent(normalizedVersion)}`,
  );
  const normalized = normalizeStrategyDefinitionVersionDocument(
    payload,
    normalizedDefinitionId,
  );
  if (normalized == null) {
    throw new Error("策略版本快照格式无效");
  }
  return normalized;
}
