import type { RuntimeDependencySettingsResponse as RuntimeDependencySettingsResponseDto } from "@/contracts";

import { apiGet, apiPut } from "@/composables/shared/apiClient";

type RuntimeDependencySettingsResponse = RuntimeDependencySettingsResponseDto;

export type RuntimeDependencySettings = Required<RuntimeDependencySettingsResponse>;

export const defaultRuntimeDependencySettings: RuntimeDependencySettings = {
  pythonBinaryPath: "",
};

export async function getRuntimeDependencySettings(): Promise<RuntimeDependencySettings> {
  return normalizeRuntimeDependencySettings(
    await apiGet("/api/v1/settings/runtime-dependencies"),
  );
}

export async function putRuntimeDependencySettings(
  settings: RuntimeDependencySettings,
): Promise<RuntimeDependencySettings> {
  return normalizeRuntimeDependencySettings(
    await apiPut("/api/v1/settings/runtime-dependencies", settings),
  );
}

function normalizeRuntimeDependencySettings(
  settings: RuntimeDependencySettingsResponse,
): RuntimeDependencySettings {
  return {
    pythonBinaryPath:
      settings.pythonBinaryPath ??
      defaultRuntimeDependencySettings.pythonBinaryPath,
  };
}
