import type {
  ObservabilityEvent,
  ObservabilityImportance,
  StrategyInstanceStatus,
  SystemStatusResponse,
} from "@/types";
import { emptySystemStatus } from "@/types";
import type { components } from "@/generated/openapi";

import { isBrokerDescriptor } from "./onboardingContract";

type SystemStatusWire = components["schemas"]["system.SystemStatusResponse"];
type ObservabilityEventWire = components["schemas"]["observability.Event"];

function importance(value: string): ObservabilityImportance {
  return value === "normal" || value === "high" || value === "critical"
    ? value
    : "low";
}

function strategyStatus(value: string): StrategyInstanceStatus {
  return value === "RUNNING" || value === "PAUSED" ? value : "STOPPED";
}

function mapEvent(value: ObservabilityEventWire): ObservabilityEvent {
  return {
    at: value.at,
    level: value.level,
    importance: importance(value.importance),
    message: value.message,
    ...(value.error != null ? { error: value.error } : {}),
    ...(value.method != null ? { method: value.method } : {}),
    ...(value.path != null ? { path: value.path } : {}),
    ...(value.operation != null ? { operation: value.operation } : {}),
    ...(value.status != null ? { status: value.status } : {}),
    ...(value.latencyMs != null ? { latencyMs: value.latencyMs } : {}),
    ...(value.requestId != null ? { requestId: value.requestId } : {}),
    ...(value.sessionId != null ? { sessionId: value.sessionId } : {}),
    ...(value.runId != null ? { runId: value.runId } : {}),
    ...(value.taskId != null ? { taskId: value.taskId } : {}),
    ...(value.brokerId != null ? { brokerId: value.brokerId } : {}),
    ...(value.accountId != null ? { accountId: value.accountId } : {}),
    ...(value.instrumentId != null
      ? { instrumentId: value.instrumentId }
      : {}),
    ...(value.providerId != null ? { providerId: value.providerId } : {}),
    ...(value.source != null ? { source: value.source } : {}),
  };
}

function mapEvents(
  value: readonly ObservabilityEventWire[] | null | undefined,
): ObservabilityEvent[] {
  return Array.isArray(value) ? value.map(mapEvent) : [];
}

export function mapSystemStatus(value: SystemStatusWire): SystemStatusResponse {
  const broker = isBrokerDescriptor(value.broker)
    ? value.broker
    : emptySystemStatus.broker;
  const strategyRuntime = value.strategyRuntime;
  const requestSummary = value.observability?.requests;
  const fallbackRequests = emptySystemStatus.observability.requests;

  return {
    name: value.name,
    apiPort: value.apiPort,
    build: value.build,
    defaultBroker: value.defaultBroker,
    defaultTradingEnvironment: value.defaultTradingEnvironment,
    realTradingEnabled: value.realTradingEnabled,
    realTradingKillSwitch: value.realTradingKillSwitch,
    realTradingRisk: value.realTradingRisk,
    realTradeAccess: value.realTradeAccess,
    broker,
    persistence: value.persistence,
    strategyRuntime:
      strategyRuntime == null
        ? emptySystemStatus.strategyRuntime
        : {
            status: strategyRuntime.status,
            activeStrategies: strategyRuntime.activeStrategies,
            supportsBacktestParity: strategyRuntime.supportsBacktestParity,
            activeInstances: strategyRuntime.activeInstances.map((instance) => ({
              instanceId: instance.instanceId,
              definitionName: instance.definitionName,
              actualStatus: strategyStatus(instance.actualStatus),
              activeSymbols: [...instance.activeSymbols],
              ...(instance.lastClosedKlineAt != null
                ? { lastClosedKlineAt: instance.lastClosedKlineAt }
                : {}),
              ...(instance.lastSignalAt != null
                ? { lastSignalAt: instance.lastSignalAt }
                : {}),
              ...(instance.lastOrderAt != null
                ? { lastOrderAt: instance.lastOrderAt }
                : {}),
              ...(instance.lastErrorAt != null
                ? { lastErrorAt: instance.lastErrorAt }
                : {}),
              ...(instance.lastError != null
                ? { lastError: instance.lastError }
                : {}),
              ...(instance.updatedAt != null
                ? { updatedAt: instance.updatedAt }
                : {}),
            })),
          },
    runtimeResources: value.runtimeResources,
    observability: {
      requests: {
        recentErrors: mapEvents(requestSummary?.recentErrors),
        recentSlowRequests: mapEvents(requestSummary?.recentSlowRequests),
        slowThresholdMs:
          requestSummary?.slowThresholdMs ?? fallbackRequests.slowThresholdMs,
        minimumImportance: importance(
          requestSummary?.minimumImportance ??
            fallbackRequests.minimumImportance,
        ),
        openD: requestSummary?.openD ?? fallbackRequests.openD,
      },
    },
    message: value.message,
  };
}
