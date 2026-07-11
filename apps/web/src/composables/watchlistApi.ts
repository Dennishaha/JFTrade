import type {
  WatchlistBinding,
  WatchlistGroup,
  WatchlistImportCommitRequest,
  WatchlistImportPreview,
  WatchlistImportPreviewRequest,
  WatchlistImportRun,
  WatchlistItem,
  WatchlistItemsPage,
  WatchlistMembership,
  WatchlistMembershipUpdate,
  WatchlistQuote,
  WatchlistQuoteBatch,
  WatchlistRemoteGroup,
  WatchlistSource,
} from "@/contracts";
import type { components } from "@/generated/openapi";

import {
  apiDeletePath,
  apiGet,
  apiGetPath,
  apiPatchPath,
  apiPost,
  apiPostPath,
  apiPutPath,
} from "./apiClient";

function encodePathPart(value: string): string {
  return encodeURIComponent(value.trim());
}

type Schemas = components["schemas"];
type RawGroup = Schemas["watchlist.WatchlistGroup"];
type RawMembership = Schemas["watchlist.WatchlistMemberships"];
type RawItem = Schemas["watchlist.WatchlistItem"];

function normalizeGroup(group: RawGroup): WatchlistGroup {
  return {
    id: group.groupId ?? "",
    name: group.name ?? "",
    ...(group.isDefault == null ? {} : { isDefault: group.isDefault }),
    ...(group.protected == null ? {} : { protected: group.protected }),
    revision: group.revision ?? 0,
    ...(group.itemCount == null ? {} : { itemCount: group.itemCount }),
    ...(group.createdAt == null ? {} : { createdAt: group.createdAt }),
    ...(group.updatedAt == null ? {} : { updatedAt: group.updatedAt }),
  };
}

function normalizeGroupsResponse(response: Schemas["watchlist.WatchlistGroupsData"]): WatchlistGroup[] {
  return (response.groups ?? [])
    .map(normalizeGroup)
    .filter((group) => group.id !== "");
}

function normalizeItemsResponse(
  response: Schemas["watchlist.WatchlistItemsData"],
): WatchlistItemsPage {
  const normalizedItems = (response.items ?? [])
    .map((item): WatchlistItem => {
      const groups = item.groups ?? [];
      const groupIds =
        item.groupIds ??
        groups.map((group) => group.groupId ?? "").filter(Boolean);
      return {
        instrumentId: item.instrumentId ?? "",
        market: item.market ?? "",
        symbol: item.symbol ?? "",
        ...(item.name == null || item.name === "" ? {} : { name: item.name }),
        ...(item.type == null || item.type === ""
          ? {}
          : { securityType: item.type }),
        groupIds,
        groupNames: groups
          .map((group) => group.name ?? "")
          .filter(Boolean),
        sources: (item.sourceIds ?? []).map((sourceId) => ({ sourceId })),
      };
    })
    .filter((item) => item.instrumentId !== "");
  return {
    items: normalizedItems,
    nextCursor: response.nextCursor || null,
  };
}

export interface WatchlistItemsQuery {
  groupId?: string | null;
  cursor?: string | null;
  limit?: number;
  query?: string;
  market?: string;
}

export async function listWatchlistGroups(): Promise<WatchlistGroup[]> {
  const response = await apiGet<
    Schemas["watchlist.WatchlistGroupsData"],
    "/api/v1/watchlist/groups"
  >("/api/v1/watchlist/groups");
  return normalizeGroupsResponse(response);
}

export async function createWatchlistGroup(name: string): Promise<WatchlistGroup> {
  const response = await apiPost<
    RawGroup,
    "/api/v1/watchlist/groups"
  >(
    "/api/v1/watchlist/groups",
    { name },
  );
  return normalizeGroup(response);
}

export async function updateWatchlistGroup(
  groupId: string,
  input: { name: string; expectedRevision: number },
): Promise<WatchlistGroup> {
  const response = await apiPatchPath<
    RawGroup,
    "/api/v1/watchlist/groups/{groupId}"
  >(
    "/api/v1/watchlist/groups/{groupId}",
    `/api/v1/watchlist/groups/${encodePathPart(groupId)}`,
    input,
  );
  return normalizeGroup(response);
}

export async function deleteWatchlistGroup(groupId: string): Promise<void> {
  await apiDeletePath<
    Schemas["watchlist.WatchlistDeleteData"],
    "/api/v1/watchlist/groups/{groupId}"
  >(
    "/api/v1/watchlist/groups/{groupId}",
    `/api/v1/watchlist/groups/${encodePathPart(groupId)}`,
  );
}

export async function listWatchlistItems(
  input: WatchlistItemsQuery = {},
): Promise<WatchlistItemsPage> {
  const params = new URLSearchParams();
  if (input.groupId) params.set("groupId", input.groupId);
  if (input.cursor) params.set("cursor", input.cursor);
  if (input.limit != null) params.set("limit", String(input.limit));
  if (input.query?.trim()) params.set("query", input.query.trim());
  if (input.market?.trim()) params.set("market", input.market.trim().toUpperCase());
  const suffix = params.size === 0 ? "" : `?${params.toString()}`;
  const response = await apiGetPath<
    Schemas["watchlist.WatchlistItemsData"],
    "/api/v1/watchlist/items"
  >(
    "/api/v1/watchlist/items",
    `/api/v1/watchlist/items${suffix}`,
  );
  return normalizeItemsResponse(response);
}

export async function getWatchlistMembership(
  market: string,
  symbol: string,
): Promise<WatchlistMembership> {
  const response = await apiGetPath<
    RawMembership,
    "/api/v1/watchlist/instruments/{market}/{symbol}/memberships"
  >(
    "/api/v1/watchlist/instruments/{market}/{symbol}/memberships",
    `/api/v1/watchlist/instruments/${encodePathPart(market.toUpperCase())}/${encodePathPart(symbol.toUpperCase())}/memberships`,
  );
  return normalizeMembership(response, `${market}.${symbol}`);
}

export async function replaceWatchlistMembership(
  market: string,
  symbol: string,
  input: WatchlistMembershipUpdate,
): Promise<WatchlistMembership> {
  const response = await apiPutPath<
    RawMembership,
    "/api/v1/watchlist/instruments/{market}/{symbol}/memberships"
  >(
    "/api/v1/watchlist/instruments/{market}/{symbol}/memberships",
    `/api/v1/watchlist/instruments/${encodePathPart(market.toUpperCase())}/${encodePathPart(symbol.toUpperCase())}/memberships`,
    input,
  );
  return normalizeMembership(response, `${market}.${symbol}`);
}

function normalizeMembership(
  membership: RawMembership,
  fallbackInstrumentId: string,
): WatchlistMembership {
  const groups = (membership.groups ?? [])
    .map((group) => ({
      id: group.groupId ?? "",
      name: group.name ?? "",
    }))
    .filter((group) => group.id !== "");
  return {
    instrumentId: membership.instrumentId ?? fallbackInstrumentId.toUpperCase(),
    revision: membership.revision ?? 0,
    groups,
    groupIds: groups.map((group) => group.id),
  };
}

export async function getWatchlistQuotes(
  instrumentIds: string[],
): Promise<WatchlistQuoteBatch> {
  if (instrumentIds.length === 0) {
    return { quotes: [], errors: [], observedAt: new Date().toISOString() };
  }
  const response = await apiPost<
    Schemas["watchlist.WatchlistQuotesData"],
    "/api/v1/watchlist/quotes/batch"
  >(
    "/api/v1/watchlist/quotes/batch",
    { instrumentIds },
  );
  return {
    quotes: (response.quotes ?? []).map((quote) => {
      const preMarket = quote.extended?.pre;
      const afterHours = quote.extended?.after;
      const overnight = quote.extended?.overnight;
      return {
        instrumentId: quote.instrumentId ?? "",
        ...(quote.name == null || quote.name === "" ? {} : { name: quote.name }),
        ...(quote.type == null || quote.type === ""
          ? {}
          : { securityType: quote.type }),
        ...(quote.price == null ? {} : { price: quote.price }),
        ...(quote.previousClose == null
          ? {}
          : { previousClose: quote.previousClose }),
        ...(quote.change == null ? {} : { change: quote.change }),
        ...(quote.changePercent == null
          ? {}
          : { changePercent: quote.changePercent }),
        ...(quote.session == null || quote.session === ""
          ? {}
          : { session: quote.session }),
        ...(quote.observedAt == null ? {} : { observedAt: quote.observedAt }),
        ...(quote.updateTime == null ? {} : { updateTime: quote.updateTime }),
        ...(quote.source == null || quote.source === ""
          ? {}
          : { source: quote.source }),
        ...(preMarket == null ? {} : { preMarket }),
        ...(afterHours == null ? {} : { afterHours }),
        ...(overnight == null ? {} : { overnight }),
      };
    }).filter((quote) => quote.instrumentId !== ""),
    errors: (response.errors ?? [])
      .map((error) => ({
        instrumentId: error.instrumentId ?? "",
        ...(error.code == null ? {} : { code: error.code }),
        message: error.message ?? "行情快照不可用",
      }))
      .filter((error) => error.instrumentId !== ""),
    observedAt: response.observedAt ?? new Date().toISOString(),
  };
}

export async function listWatchlistSources(): Promise<WatchlistSource[]> {
  const response = await apiGet<
    Schemas["watchlist.WatchlistSourcesData"],
    "/api/v1/watchlist/sources"
  >("/api/v1/watchlist/sources");
  return (response.sources ?? []).map((source) => {
    const status = source.status?.trim().toLowerCase() ?? "";
    const message = source.error;
    return {
      id: source.sourceId ?? "",
      ...(source.broker == null ? {} : { broker: source.broker }),
      displayName:
        source.displayName || source.broker || source.sourceId || "券商数据源",
      available:
        (source.error == null || source.error === "") &&
        (status === "" || status === "ready"),
      ...(source.status == null ? {} : { status: source.status }),
      ...(message == null ? {} : { message }),
    };
  }).filter((source) => source.id !== "");
}

export async function listWatchlistSourceGroups(
  sourceId: string,
): Promise<WatchlistRemoteGroup[]> {
  const response = await apiGetPath<
    Schemas["watchlist.WatchlistRemoteGroupsData"],
    "/api/v1/watchlist/sources/{sourceId}/groups"
  >(
    "/api/v1/watchlist/sources/{sourceId}/groups",
    `/api/v1/watchlist/sources/${encodePathPart(sourceId)}/groups`,
  );
  return (response.groups ?? []).map((group) => {
    const type = group.type;
    return {
      remoteGroupId: group.remoteGroupId ?? "",
      name: group.name ?? "",
      ...(type == null ? {} : { type }),
      system: type?.toUpperCase() === "SYSTEM",
      ...(group.ambiguous == null ? {} : { ambiguous: group.ambiguous }),
      ...(group.memberCount == null ? {} : { memberCount: group.memberCount }),
    };
  }).filter((group) => group.remoteGroupId !== "" && group.name !== "");
}

export async function listWatchlistBindings(): Promise<WatchlistBinding[]> {
  const response = await apiGet<
    Schemas["watchlist.WatchlistBindingsData"],
    "/api/v1/watchlist/bindings"
  >("/api/v1/watchlist/bindings");
  return (response.bindings ?? []).map((binding) => ({
    id: binding.bindingId ?? "",
    sourceId: binding.sourceId ?? "",
    remoteGroupId: binding.remoteGroupId ?? "",
    remoteGroupName: binding.remoteName ?? "",
    localGroupId: binding.localGroupId ?? "",
  })).filter((binding) => binding.id !== "");
}

export async function deleteWatchlistBinding(bindingId: string): Promise<void> {
  await apiDeletePath<
    Schemas["watchlist.WatchlistDeleteData"],
    "/api/v1/watchlist/bindings"
  >(
    "/api/v1/watchlist/bindings",
    `/api/v1/watchlist/bindings?bindingId=${encodePathPart(bindingId)}`,
  );
}

export async function previewWatchlistImport(
  input: WatchlistImportPreviewRequest,
): Promise<WatchlistImportPreview> {
  const response = await apiPost<
    Schemas["watchlist.WatchlistImportPreview"],
    "/api/v1/watchlist/imports/preview"
  >(
    "/api/v1/watchlist/imports/preview",
    {
      sourceId: input.sourceId,
      remoteGroupId: input.remoteGroupId,
      ...(input.localGroupId == null ? {} : { localGroupId: input.localGroupId }),
      ...(input.newGroupName == null ? {} : { newGroupName: input.newGroupName }),
    },
  );
  return {
    id: response.previewId ?? "",
    sourceId: response.sourceId ?? input.sourceId,
    remoteGroupName: response.remoteGroupName ?? input.remoteGroupName ?? "",
    ...(response.localGroupId == null ? {} : { localGroupId: response.localGroupId }),
    ...(response.newGroupName == null ? {} : { localGroupName: response.newGroupName }),
    added: (response.added ?? []).map(normalizeImportDiffItem),
    unchanged: (response.unchanged ?? []).map(normalizeImportDiffItem),
    localOnly: (response.localOnly ?? []).map(normalizeImportDiffItem),
    ...(response.expiresAt == null ? {} : { expiresAt: response.expiresAt }),
    ...(response.remoteHash == null ? {} : { remoteHash: response.remoteHash }),
    ...(response.localGroupRevision == null
      ? {}
      : { localRevision: response.localGroupRevision }),
  };
}

function normalizeImportDiffItem(
  item: Schemas["watchlist.ImportDiffItem"],
) {
  return {
    instrumentId: item.instrumentId ?? "",
    ...(item.name == null ? {} : { name: item.name }),
    ...(item.selected == null ? {} : { selected: item.selected }),
  };
}

export async function commitWatchlistImport(
  previewId: string,
  input: WatchlistImportCommitRequest,
): Promise<WatchlistImportRun> {
  const response = await apiPostPath<
    Schemas["watchlist.WatchlistImportRun"],
    "/api/v1/watchlist/imports/{previewId}/commit"
  >(
    "/api/v1/watchlist/imports/{previewId}/commit",
    `/api/v1/watchlist/imports/${encodePathPart(previewId)}/commit`,
    {
      deleteInstrumentIds: input.deleteLocalOnlyInstrumentIds,
    },
  );
  return normalizeImportRun(response);
}

export async function listWatchlistImportRuns(): Promise<WatchlistImportRun[]> {
  const response = await apiGet<
    Schemas["watchlist.WatchlistImportRunsData"],
    "/api/v1/watchlist/import-runs"
  >("/api/v1/watchlist/import-runs");
  return (response.items ?? []).map(normalizeImportRun);
}

function normalizeImportRun(
  run: Schemas["watchlist.WatchlistImportRun"],
): WatchlistImportRun {
  return {
    id: run.runId ?? "",
    ...(run.previewId == null ? {} : { previewId: run.previewId }),
    sourceId: run.sourceId ?? "",
    ...(run.remoteGroupName == null
      ? {}
      : { remoteGroupName: run.remoteGroupName }),
    ...(run.localGroupId == null ? {} : { localGroupId: run.localGroupId }),
    ...(run.addedCount == null ? {} : { addedCount: run.addedCount }),
    ...(run.removedCount == null ? {} : { deletedCount: run.removedCount }),
    ...(run.unchangedCount == null
      ? {}
      : { unchangedCount: run.unchangedCount }),
    status: run.status ?? "unknown",
    ...(run.createdAt == null ? {} : { createdAt: run.createdAt }),
    ...(run.completedAt == null ? {} : { completedAt: run.completedAt }),
  };
}
