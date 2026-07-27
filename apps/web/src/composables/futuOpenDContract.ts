import type {
  FutuOpenDHealthResponse,
  FutuOpenDInstallGuideResponse,
  FutuOpenDIssueCode,
} from "@/types";
import {
  emptyFutuOpenDHealth,
  emptyFutuOpenDInstallGuide,
} from "@/types";
import type { components } from "@/generated/openapi";

type HealthWire = components["schemas"]["system.FutuOpenDHealthResponse"];
type InstallGuideWire =
  components["schemas"]["system.FutuOpenDInstallGuideResponse"];

const issueCodes = new Set<FutuOpenDIssueCode>([
  "NONE",
  "LOGIN_TIMEOUT",
  "CONNECTION_LIMIT",
  "PROTOCOL_PARSE_ERROR",
  "WS_POOL_EXHAUSTED",
  "WEBSOCKET_AUTH",
  "OPEND_VERSION_UNSUPPORTED",
  "OPEND_API_CONNECTIVITY",
]);

function issueCode(value: string): FutuOpenDIssueCode {
  return issueCodes.has(value as FutuOpenDIssueCode)
    ? (value as FutuOpenDIssueCode)
    : "OPEND_API_CONNECTIVITY";
}

function record(value: unknown): Record<string, unknown> {
  return typeof value === "object" && value != null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function textOrNull(value: unknown): string | null {
  return typeof value === "string" ? value : null;
}

function numberOrNull(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

export function mapFutuOpenDInstallGuide(
  value: InstallGuideWire,
): FutuOpenDInstallGuideResponse {
  return {
    ...emptyFutuOpenDInstallGuide,
    brokerId: "futu",
    title: value.title,
    description: value.description,
    nextSteps: [...value.nextSteps],
    options: value.options.flatMap((option) =>
      option.id === "gui" || option.id === "command-line"
        ? [{ ...option, id: option.id }]
        : [],
    ),
    settings: {
      host: value.settings.host,
      apiPort: value.settings.apiPort,
      websocketPort: value.settings.websocketPort,
      maxWebSocketConnections: value.settings.maxWebSocketConnections,
      useEncryption: value.settings.useEncryption,
      websocketKeyRequired: value.settings.websocketKeyRequired,
      minimumVersion: value.settings.minimumVersion,
    },
  };
}

export function mapFutuOpenDHealth(value: HealthWire): FutuOpenDHealthResponse {
  const installation = record(value.localInstallation);
  const process = record(installation.process);
  const latest = record(value.latestVersion);
  const status =
    value.status === "healthy" || value.status === "degraded"
      ? value.status
      : "offline";
  const connectivity =
    value.runtime.connectivity === "connected" ||
    value.runtime.connectivity === "degraded"
      ? value.runtime.connectivity
      : "disconnected";
  const latestStatus = [
    "unknown",
    "not_installed",
    "up_to_date",
    "outdated",
    "ahead_of_latest",
  ].includes(String(latest.status))
    ? (latest.status as FutuOpenDHealthResponse["latestVersion"]["status"])
    : "unknown";

  return {
    checkedAt: value.checkedAt,
    status,
    runtime: {
      connectivity,
      host: value.runtime.host,
      port: value.runtime.websocketPort,
      useEncryption: value.runtime.useEncryption,
      websocketKeyConfigured: value.runtime.websocketKeyConfigured,
      quoteLoggedIn: value.runtime.quoteLoggedIn,
      tradeLoggedIn: value.runtime.tradeLoggedIn,
      programStatus: value.runtime.programStatus,
      serverVersion: value.runtime.serverVersion,
      minimumVersion: value.runtime.minimumVersion,
      lastError: value.runtime.lastError,
    },
    diagnosis: {
      code: issueCode(value.diagnosis.code),
      summary: value.diagnosis.summary,
      manualRetryRequired: value.diagnosis.manualRetryRequired,
      restartOpenDRecommended: value.diagnosis.restartOpenDRecommended,
    },
    localSocketDiagnostics: {
      websocketEstablishedConnections:
        value.localSocketDiagnostics.websocketEstablishedConnections,
      likelyConnectionSaturation:
        value.localSocketDiagnostics.likelyConnectionSaturation,
      topClientProcesses: value.localSocketDiagnostics.topClientProcesses.map(
        (entry) => ({
          processName: entry.processName,
          pid: entry.pid,
          establishedConnections: entry.establishedConnections,
        }),
      ),
    },
    localInstallation: {
      platform:
        typeof installation.platform === "string" ? installation.platform : "",
      installed: installation.installed === true,
      version: textOrNull(installation.version),
      installPath: textOrNull(installation.installPath),
      guiDetected: installation.guiDetected === true,
      process: {
        running: process.running === true,
        pid: numberOrNull(process.pid),
        executablePath: textOrNull(process.executablePath),
      },
    },
    latestVersion: {
      value: textOrNull(latest.value),
      sourceUrl: textOrNull(latest.sourceUrl),
      checkedAt: textOrNull(latest.checkedAt),
      status: latestStatus,
      error: textOrNull(latest.error),
    },
    recommendations: [...value.recommendations],
  };
}
