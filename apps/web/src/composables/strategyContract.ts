import type {
  StrategyExecutionMode,
  StrategyInstanceItem,
  StrategyInstanceStatus,
  StrategyRuntimeRiskMode,
} from "@/types";
import type { components } from "@/generated/openapi";

type StrategyInstanceWire = components["schemas"]["strategy.InstanceView"];

function instanceStatus(value: string | undefined): StrategyInstanceStatus {
  const normalized = value?.trim();
  return (normalized === "" || normalized == null
    ? "STOPPED"
    : normalized) as StrategyInstanceStatus;
}

function executionMode(value: string | undefined): StrategyExecutionMode {
  return value === "live" ? "live" : "notify_only";
}

function runtimeRiskMode(value: string | undefined): StrategyRuntimeRiskMode {
  return value === "monitor" || value === "enforce" ? value : "off";
}

export function mapStrategyInstance(value: StrategyInstanceWire): StrategyInstanceItem {
  const binding = value.binding;
  const runtimeRisk = binding?.runtimeRisk;
  const observation = value.runtimeObservation;
  const definitionSync = value.definitionSync;
  return {
    id: value.id ?? "",
    ...(typeof value.pluginId === "string" ? { pluginId: value.pluginId } : {}),
    definition: {
      strategyId: value.definition?.strategyId ?? "",
      name: value.definition?.name ?? "",
      version: value.definition?.version ?? "",
    },
    runtime: value.runtime ?? "pine-pinets",
    sourceFormat: "pine-v6",
    startable: value.startable ?? false,
    ...(binding == null
      ? {}
      : {
          binding: {
            instruments: (binding.instruments ?? []).map((instrument) => ({
              market: instrument.market ?? "",
              code: instrument.code ?? "",
            })),
            symbols: binding.symbols ?? [],
            interval: binding.interval ?? "",
            ...(binding.chartType === "standard" || binding.chartType === "heikinashi"
              ? { chartType: binding.chartType }
              : {}),
            executionMode: executionMode(binding.executionMode),
            ...(binding.brokerAccount == null
              ? {}
              : {
                  brokerAccount: {
                    brokerId: binding.brokerAccount.brokerId ?? "",
                    accountId: binding.brokerAccount.accountId ?? "",
                    tradingEnvironment:
                      binding.brokerAccount.tradingEnvironment ?? "",
                    market: binding.brokerAccount.market ?? "",
                  },
                }),
            runtimeRisk: {
              mode: runtimeRiskMode(runtimeRisk?.mode),
              closeOnly: runtimeRisk?.closeOnly ?? false,
              ...(typeof runtimeRisk?.maxOrderQuantity === "number"
                ? { maxOrderQuantity: runtimeRisk.maxOrderQuantity }
                : {}),
              ...(typeof runtimeRisk?.maxOrderNotional === "number"
                ? { maxOrderNotional: runtimeRisk.maxOrderNotional }
                : {}),
              ...(typeof runtimeRisk?.dailyMaxOrders === "number"
                ? { dailyMaxOrders: runtimeRisk.dailyMaxOrders }
                : {}),
              pauseOnReject: runtimeRisk?.pauseOnReject ?? false,
            },
          },
        }),
    params: value.params ?? {},
    status: instanceStatus(value.status),
    createdAt: value.createdAt ?? "",
    logs: value.logs ?? [],
    ...(definitionSync == null
      ? {}
      : {
          definitionSync: {
            definitionId: definitionSync.definitionId ?? "",
            appliedVersion: definitionSync.appliedVersion ?? "",
            latestVersion: definitionSync.latestVersion ?? "",
            isLatest: definitionSync.isLatest ?? false,
            canApplyLatest: definitionSync.canApplyLatest ?? false,
            ...(typeof definitionSync.blockedReason === "string"
              ? { blockedReason: definitionSync.blockedReason }
              : {}),
          },
        }),
    ...(observation == null
      ? {}
      : {
          runtimeObservation: {
            actualStatus: instanceStatus(observation.actualStatus),
            activeSymbols: observation.activeSymbols ?? [],
            ...(typeof observation.lastClosedKlineAt === "string"
              ? { lastClosedKlineAt: observation.lastClosedKlineAt }
              : {}),
            ...(typeof observation.lastSignalAt === "string"
              ? { lastSignalAt: observation.lastSignalAt }
              : {}),
            ...(typeof observation.lastOrderAt === "string"
              ? { lastOrderAt: observation.lastOrderAt }
              : {}),
            ...(typeof observation.lastErrorAt === "string"
              ? { lastErrorAt: observation.lastErrorAt }
              : {}),
            ...(typeof observation.lastError === "string"
              ? { lastError: observation.lastError }
              : {}),
            ...(typeof observation.updatedAt === "string"
              ? { updatedAt: observation.updatedAt }
              : {}),
          },
        }),
  };
}

export function mapStrategyInstances(
  values: StrategyInstanceWire[] | undefined,
): StrategyInstanceItem[] {
  return (values ?? []).map(mapStrategyInstance);
}
