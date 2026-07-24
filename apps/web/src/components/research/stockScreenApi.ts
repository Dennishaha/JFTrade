import {
  ApiClientError,
  fetchEnvelope,
  fetchEnvelopeWithInit,
} from "../../composables/apiClient";
import type {
  StockScreenCatalog,
  StockScreenDefinitionV2,
  StockScreenPreset,
  StockScreenPresetList,
  StockScreenQuery,
  StockScreenResult,
} from "./stockScreenTypes";

function brokerQuery(brokerId?: string): string {
  return brokerId ? `&brokerId=${encodeURIComponent(brokerId)}` : "";
}

export function fetchStockScreenCatalog(
  market: string,
  brokerId?: string,
): Promise<StockScreenCatalog> {
  return fetchEnvelope(
    `/api/v1/research/screens/catalog?market=${encodeURIComponent(market)}${brokerQuery(brokerId)}`,
  );
}

export function runStockScreen(
  query: StockScreenQuery,
): Promise<StockScreenResult> {
  return fetchEnvelopeWithInit("/api/v1/research/screens", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(query),
  });
}

export function fetchStockScreenPresets(): Promise<StockScreenPresetList> {
  return fetchEnvelope("/api/v1/research/screens/presets");
}

export function createStockScreenPreset(
  name: string,
  definition: StockScreenDefinitionV2,
): Promise<StockScreenPreset> {
  return fetchEnvelopeWithInit("/api/v1/research/screens/presets", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, definition }),
  });
}

export function updateStockScreenPreset(
  presetId: string,
  name: string,
  definition: StockScreenDefinitionV2,
  expectedRevision: number,
): Promise<StockScreenPreset> {
  return fetchEnvelopeWithInit(
    `/api/v1/research/screens/presets/${encodeURIComponent(presetId)}`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        name,
        definition,
        expectedRevision,
      }),
    },
  );
}

export function deleteStockScreenPreset(presetId: string): Promise<void> {
  return fetchEnvelopeWithInit(
    `/api/v1/research/screens/presets/${encodeURIComponent(presetId)}`,
    { method: "DELETE" },
  );
}

export function isPresetConflict(error: unknown): boolean {
  return error instanceof ApiClientError && error.status === 409;
}
