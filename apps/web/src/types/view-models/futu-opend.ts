import type { FutuOpenDInstallOptionDto } from "../../contracts/generated/system";

export interface FutuOpenDInstallGuideResponse {
  brokerId: "futu";
  title: string;
  description: string;
  options: FutuOpenDInstallOptionDto[];
  nextSteps: string[];
  settings: {
    host: string;
    apiPort: number;
    websocketPort: number;
    maxWebSocketConnections: number;
    useEncryption: boolean;
    websocketKeyRequired: boolean;
    minimumVersion: string;
  };
}

export type FutuOpenDIssueCode =
  | "NONE"
  | "LOGIN_TIMEOUT"
  | "CONNECTION_LIMIT"
  | "PROTOCOL_PARSE_ERROR"
  | "WS_POOL_EXHAUSTED"
  | "WEBSOCKET_AUTH"
  | "OPEND_VERSION_UNSUPPORTED"
  | "OPEND_API_CONNECTIVITY";

export interface FutuOpenDHealthResponse {
  checkedAt: string;
  status: "healthy" | "degraded" | "offline";
  runtime: {
    connectivity: "connected" | "degraded" | "disconnected";
    host: string;
    port: number;
    useEncryption: boolean;
    websocketKeyConfigured: boolean;
    quoteLoggedIn: boolean | null;
    tradeLoggedIn: boolean | null;
    programStatus: string | null;
    serverVersion: string | null;
    minimumVersion: string;
    lastError: string | null;
  };
  diagnosis: {
    code: FutuOpenDIssueCode;
    summary: string | null;
    manualRetryRequired: boolean;
    restartOpenDRecommended: boolean;
  };
  localSocketDiagnostics: {
    websocketEstablishedConnections: number;
    likelyConnectionSaturation: boolean;
    topClientProcesses: Array<{
      processName: string;
      pid: number;
      establishedConnections: number;
    }>;
  };
  localInstallation: {
    platform: string;
    installed: boolean;
    version: string | null;
    installPath: string | null;
    guiDetected: boolean;
    process: {
      running: boolean;
      pid: number | null;
      executablePath: string | null;
    };
  };
  latestVersion: {
    value: string | null;
    sourceUrl: string | null;
    checkedAt: string | null;
    status:
      | "unknown"
      | "not_installed"
      | "up_to_date"
      | "outdated"
      | "ahead_of_latest";
    error: string | null;
  };
  recommendations: string[];
}

export const emptyFutuOpenDInstallGuide: FutuOpenDInstallGuideResponse = {
  brokerId: "futu",
  title: "",
  description: "",
  options: [],
  nextSteps: [],
  settings: {
    host: "127.0.0.1",
    apiPort: 11110,
    websocketPort: 11111,
    maxWebSocketConnections: 20,
    useEncryption: false,
    websocketKeyRequired: false,
    minimumVersion: "10.9.6908",
  },
};

export const emptyFutuOpenDHealth: FutuOpenDHealthResponse = {
  checkedAt: "",
  status: "offline",
  runtime: {
    connectivity: "disconnected",
    host: "127.0.0.1",
    port: 11111,
    useEncryption: false,
    websocketKeyConfigured: false,
    quoteLoggedIn: null,
    tradeLoggedIn: null,
    programStatus: null,
    serverVersion: null,
    minimumVersion: "10.9.6908",
    lastError: null,
  },
  diagnosis: {
    code: "NONE",
    summary: null,
    manualRetryRequired: false,
    restartOpenDRecommended: false,
  },
  localSocketDiagnostics: {
    websocketEstablishedConnections: 0,
    likelyConnectionSaturation: false,
    topClientProcesses: [],
  },
  localInstallation: {
    platform: "",
    installed: false,
    version: null,
    installPath: null,
    guiDetected: false,
    process: {
      running: false,
      pid: null,
      executablePath: null,
    },
  },
  latestVersion: {
    value: null,
    sourceUrl: null,
    checkedAt: null,
    status: "unknown",
    error: null,
  },
  recommendations: [],
};
