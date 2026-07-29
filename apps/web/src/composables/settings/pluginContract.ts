import type {
  PluginCatalogResponse,
  PluginCompatibilityDto,
  PluginInstallResponse,
  PluginInstallStatus,
  PluginOperationDto,
  PluginOperationStatus,
} from "@/types";
import type {
  PluginCatalogDto as PluginCatalogWire,
  PluginCompatibilityDto as PluginCompatibilityWire,
  PluginMutationDto as PluginMutationWire,
  PluginOperationDto as PluginOperationWire,
  PluginUninstallGuidanceDto,
} from "@/contracts";

function pluginInstallStatus(value: string): PluginInstallStatus {
  switch (value) {
    case "INSTALLING":
    case "INSTALLED":
    case "FAILED":
      return value;
    default:
      return "NOT_INSTALLED";
  }
}

function pluginOperationStatus(value: string): PluginOperationStatus {
  switch (value) {
    case "QUEUED":
    case "RUNNING":
    case "SUCCEEDED":
      return value;
    default:
      return "FAILED";
  }
}

export function mapPluginOperation(
  value: PluginOperationWire,
): PluginOperationDto {
  return {
    operationId: value.operationId ?? "",
    pluginId: value.pluginId ?? "",
    status: pluginOperationStatus(value.status ?? ""),
    phase: value.phase ?? "",
    progress: value.progress ?? 0,
    message: value.message ?? "",
    targetDir: value.targetDir ?? "",
    installPath: value.installPath ?? "",
    startedAt: value.startedAt ?? "",
    updatedAt: value.updatedAt ?? "",
    completedAt: value.completedAt ?? null,
    error: value.error ?? null,
  };
}

export function mapPluginUninstallGuidance(
  value: PluginUninstallGuidanceDto,
): PluginUninstallGuidanceDto {
  return {
    pluginId: value.pluginId ?? "",
    path: value.path ?? "",
    exists: value.exists ?? false,
    commands: {
      posix: value.commands?.posix ?? "",
      powershell: value.commands?.powershell ?? "",
    },
  };
}

function mapPluginCompatibility(
  value: PluginCompatibilityWire | null | undefined,
): PluginCompatibilityDto {
  return {
    mode: value?.mode ?? "",
    supported: value?.supported ?? false,
    requiresRebuild: value?.requiresRebuild ?? false,
    ...(value?.reason == null ? {} : { reason: value.reason }),
    host: {
      jftradeVersion: value?.host?.jftradeVersion ?? "",
      goVersion: value?.host?.goVersion ?? "",
      goos: value?.host?.goos ?? "",
      goarch: value?.host?.goarch ?? "",
      buildMode: value?.host?.buildMode ?? "",
      ...(value?.host?.buildTags == null ? {} : { buildTags: value.host.buildTags }),
    },
    ...(value?.artifact == null
      ? {}
      : {
          artifact: {
            jftradeVersion: value.artifact.jftradeVersion ?? "",
            goVersion: value.artifact.goVersion ?? "",
            goos: value.artifact.goos ?? "",
            goarch: value.artifact.goarch ?? "",
            buildMode: value.artifact.buildMode ?? "",
            ...(value.artifact.buildTags == null
              ? {}
              : { buildTags: value.artifact.buildTags }),
          },
        }),
  };
}

export function mapPluginCatalog(value: PluginCatalogWire): PluginCatalogResponse {
  return {
    targetDir: value.targetDir ?? "",
    plugins: (value.plugins ?? []).map((entry) => ({
      descriptor: {
        id: entry.descriptor?.id ?? "",
        type: entry.descriptor?.type ?? "",
        displayName: entry.descriptor?.displayName ?? "",
        version: entry.descriptor?.version ?? "",
        description: entry.descriptor?.description ?? "",
        keywords: entry.descriptor?.keywords ?? [],
      },
      installation: {
        status: pluginInstallStatus(entry.installation?.status ?? ""),
        installed: entry.installation?.installed ?? false,
        installPath: entry.installation?.installPath ?? "",
        targetDir: entry.installation?.targetDir ?? "",
        markerPath: entry.installation?.markerPath ?? "",
        currentOperation:
          entry.installation?.currentOperation == null
            ? null
            : mapPluginOperation(entry.installation.currentOperation),
        lastOperation:
          entry.installation?.lastOperation == null
            ? null
            : mapPluginOperation(entry.installation.lastOperation),
        uninstallGuidance: mapPluginUninstallGuidance(
          entry.installation?.uninstallGuidance ?? {
            commands: { posix: "", powershell: "" },
            exists: false,
            path: "",
            pluginId: entry.descriptor?.id ?? "",
          },
        ),
      },
      compatibility: mapPluginCompatibility(entry.compatibility),
    })),
  };
}

export function mapPluginMutation(
  value: PluginMutationWire,
): PluginInstallResponse {
  return { operation: mapPluginOperation(value.operation) };
}
