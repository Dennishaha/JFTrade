import type { PineWorkerSettingsResponse as PineWorkerSettingsResponseDto } from "@/contracts";

import { apiGet, apiPut } from "@/composables/shared/apiClient";

type PineWorkerSettingsResponse =
  PineWorkerSettingsResponseDto;

export type PineWorkerSettings = Required<PineWorkerSettingsResponse>;

export const defaultPineWorkerSettings: PineWorkerSettings = {
  backtestWorkerLimit: 2,
  instanceWorkerLimit: 10,
  nodeBinaryPath: "",
};

export async function getPineWorkerSettings(): Promise<PineWorkerSettings> {
  return normalizePineWorkerSettings(
    await apiGet("/api/v1/settings/pine-worker"),
  );
}

export async function putPineWorkerSettings(
  settings: PineWorkerSettings,
): Promise<PineWorkerSettings> {
  return normalizePineWorkerSettings(
    await apiPut("/api/v1/settings/pine-worker", settings),
  );
}

function normalizePineWorkerSettings(
  settings: PineWorkerSettingsResponse,
): PineWorkerSettings {
  return {
    backtestWorkerLimit:
      settings.backtestWorkerLimit ?? defaultPineWorkerSettings.backtestWorkerLimit,
    instanceWorkerLimit:
      settings.instanceWorkerLimit ?? defaultPineWorkerSettings.instanceWorkerLimit,
    nodeBinaryPath:
      settings.nodeBinaryPath ?? defaultPineWorkerSettings.nodeBinaryPath,
  };
}
