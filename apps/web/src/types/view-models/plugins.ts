import type {
  PluginBuildTupleDto,
  PluginDescriptorDto,
  PluginUninstallGuidanceDto,
} from "../../contracts/generated/plugins";

export type PluginInstallStatus =
  | "NOT_INSTALLED"
  | "INSTALLING"
  | "INSTALLED"
  | "FAILED";

export type PluginOperationStatus =
  | "QUEUED"
  | "RUNNING"
  | "SUCCEEDED"
  | "FAILED";

export interface PluginOperationDto {
  operationId: string;
  pluginId: string;
  status: PluginOperationStatus;
  phase: string;
  progress: number;
  message: string;
  targetDir: string;
  installPath: string;
  startedAt: string;
  updatedAt: string;
  completedAt: string | null;
  error: string | null;
}

export interface PluginCompatibilityDto {
  mode: string;
  supported: boolean;
  requiresRebuild: boolean;
  reason?: string | null;
  host: PluginBuildTupleDto;
  artifact?: PluginBuildTupleDto | null;
}

export interface PluginInstallationDto {
  status: PluginInstallStatus;
  installed: boolean;
  installPath: string;
  targetDir: string;
  markerPath: string;
  currentOperation: PluginOperationDto | null;
  lastOperation: PluginOperationDto | null;
  uninstallGuidance: PluginUninstallGuidanceDto;
}

export interface PluginCatalogResponse {
  targetDir: string;
  plugins: Array<{
    descriptor: PluginDescriptorDto;
    installation: PluginInstallationDto;
    compatibility?: PluginCompatibilityDto;
  }>;
}

export const emptyPluginCatalog: PluginCatalogResponse = {
  targetDir: "",
  plugins: [],
};
