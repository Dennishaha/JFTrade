import type { Ref } from "vue";

import {
  type PluginCatalogResponse,
  type PluginInstallResponse,
  type PluginOperationDto,
  type PluginUninstallGuidanceDto,
} from "@jftrade/ui-contracts";

import {
  fetchEnvelope,
  fetchEnvelopeWithInit,
} from "./apiClient";

interface CreateConsoleDataPluginControllerOptions {
  pluginCatalog: Ref<PluginCatalogResponse>;
  pluginError: Ref<string>;
  installingPluginIds: Ref<string[]>;
  uninstallingPluginIds: Ref<string[]>;
}

function addPluginOperationId(ids: string[], pluginId: string): string[] {
  return Array.from(new Set([...ids, pluginId]));
}

function removePluginOperationId(ids: string[], pluginId: string): string[] {
  return ids.filter((id) => id !== pluginId);
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export function createConsoleDataPluginController(
  options: CreateConsoleDataPluginControllerOptions,
) {
  async function fetchPluginCatalog(): Promise<PluginCatalogResponse> {
    return fetchEnvelope<PluginCatalogResponse>("/api/v1/plugins");
  }

  async function loadPlugins(): Promise<void> {
    options.pluginError.value = "";

    try {
      options.pluginCatalog.value = await fetchPluginCatalog();
    } catch (error) {
      options.pluginError.value =
        error instanceof Error ? error.message : "插件列表加载失败。";
    }
  }

  async function pollPluginOperation(
    operationId: string,
  ): Promise<PluginOperationDto> {
    for (let attempt = 0; attempt < 30; attempt += 1) {
      const operation = await fetchEnvelope<PluginOperationDto>(
        `/api/v1/plugins/operations/${encodeURIComponent(operationId)}`,
      );

      if (operation.status === "SUCCEEDED" || operation.status === "FAILED") {
        if (operation.status === "FAILED") {
          throw new Error(operation.error ?? operation.message);
        }

        return operation;
      }

      await delay(500);
    }

    throw new Error("插件操作未在预期时间内完成。");
  }

  async function installPlugin(pluginId: string): Promise<void> {
    options.pluginError.value = "";
    options.installingPluginIds.value = addPluginOperationId(
      options.installingPluginIds.value,
      pluginId,
    );

    try {
      const response = await fetchEnvelopeWithInit<PluginInstallResponse>(
        `/api/v1/plugins/${encodeURIComponent(pluginId)}/install`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({}),
        },
      );

      await pollPluginOperation(response.operation.operationId);
      await loadPlugins();
    } catch (error) {
      options.pluginError.value =
        error instanceof Error ? error.message : "插件安装失败。";
    } finally {
      options.installingPluginIds.value = removePluginOperationId(
        options.installingPluginIds.value,
        pluginId,
      );
    }
  }

  async function uninstallPlugin(pluginId: string): Promise<void> {
    options.pluginError.value = "";
    options.uninstallingPluginIds.value = addPluginOperationId(
      options.uninstallingPluginIds.value,
      pluginId,
    );

    try {
      const response = await fetchEnvelopeWithInit<PluginInstallResponse>(
        `/api/v1/plugins/${encodeURIComponent(pluginId)}/uninstall`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({}),
        },
      );

      await pollPluginOperation(response.operation.operationId);
      await loadPlugins();
    } catch (error) {
      options.pluginError.value =
        error instanceof Error
          ? error.message
          : "插件卸载失败。";
    } finally {
      options.uninstallingPluginIds.value = removePluginOperationId(
        options.uninstallingPluginIds.value,
        pluginId,
      );
    }
  }

  async function loadPluginUninstallGuidance(
    pluginId: string,
  ): Promise<PluginUninstallGuidanceDto | null> {
    options.pluginError.value = "";

    try {
      return await fetchEnvelope<PluginUninstallGuidanceDto>(
        `/api/v1/plugins/${encodeURIComponent(pluginId)}/uninstall-guidance`,
      );
    } catch (error) {
      options.pluginError.value =
        error instanceof Error
          ? error.message
          : "插件卸载指引加载失败。";
      return null;
    }
  }

  return {
    fetchPluginCatalog,
    installPlugin,
    loadPluginUninstallGuidance,
    loadPlugins,
    uninstallPlugin,
  };
}