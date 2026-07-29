import { describe, expect, it } from "vitest";

import { emptySystemStatus } from "../src/types";
import { mapBrokerSettings } from "@/composables/settings/brokerSettingsContract";
import {
  mapFutuOpenDHealth,
  mapFutuOpenDInstallGuide,
} from "@/composables/market-data/futuOpenDContract";
import { mapSystemStatus } from "@/composables/settings/systemStatusContract";
import {
  isBrokerDescriptor,
  mapOnboardingState,
} from "@/composables/settings/onboardingContract";

function futuHealthWire() {
  return {
    checkedAt: "2026-07-26T10:00:00Z",
    status: "healthy",
    runtime: {
      apiPort: 11110,
      connectivity: "connected",
      host: "127.0.0.1",
      lastError: null,
      marketDataTransport: "websocket",
      minimumVersion: "10.9.6908",
      programStatus: "READY",
      quoteLoggedIn: true,
      serverVersion: "10.9.7000",
      tradeLoggedIn: true,
      useEncryption: false,
      websocketKeyConfigured: false,
      websocketPort: 11111,
    },
    diagnosis: {
      code: "NONE",
      manualRetryRequired: false,
      restartOpenDRecommended: false,
      summary: null,
    },
    localSocketDiagnostics: {
      websocketEstablishedConnections: 1,
      likelyConnectionSaturation: false,
      topClientProcesses: [],
    },
    localInstallation: {
      platform: "darwin",
      installed: true,
      version: "10.9.7000",
      installPath: "/Applications/FutuOpenD.app",
      guiDetected: true,
      process: {
        running: true,
        pid: 42,
        executablePath: "/Applications/FutuOpenD.app/OpenD",
      },
    },
    latestVersion: {
      value: "10.9.7000",
      sourceUrl: "https://example.invalid/opend",
      checkedAt: "2026-07-26T09:59:00Z",
      status: "up_to_date",
      error: null,
    },
    recommendations: [],
  } as const;
}

function systemStatusWire() {
  return {
    name: "JFTrade",
    apiPort: 3000,
    build: {
      version: "dev",
      commit: "abc123",
      buildTime: "2026-07-26T10:00:00Z",
      goos: "darwin",
      goarch: "arm64",
    },
    defaultBroker: "futu",
    defaultTradingEnvironment: "SIMULATE",
    realTradingEnabled: false,
    realTradingKillSwitch: {
      active: false,
      allowsCancel: true,
      blockedOperations: [],
      runtimeActive: false,
    },
    realTradingRisk: {
      enabled: true,
      maxOrderNotional: null,
      maxOrderQuantity: null,
      runtimeConfiguredMaxOrderNotional: null,
      runtimeConfiguredMaxOrderQuantity: null,
      runtimeRiskConfigured: false,
    },
    realTradeAccess: {
      adminAllowlistEnabled: false,
      adminCount: 0,
      approverAllowlistEnabled: false,
      approverCount: 0,
    },
    persistence: {
      checkedAt: "2026-07-26T10:00:00Z",
      databasePath: "/tmp/jftrade.db",
      engine: "sqlite",
      migrated: true,
      pendingMigrations: [],
      status: "ready",
      tables: [],
    },
    runtimeResources: {
      checkedAt: "2026-07-26T10:00:00Z",
      count: 0,
      items: [],
    },
    observability: {
      api: {},
      broker: {},
      exchangeCalendars: {},
      live: {},
      marketdata: {},
      strategyRuntime: {},
      requests: {
        minimumImportance: "future-importance",
        openD: { failedCalls: 0, totalCalls: 1 },
        recentErrors: [
          {
            at: "2026-07-26T10:00:00Z",
            importance: "future-importance",
            level: "ERROR",
            message: "request failed",
          },
        ],
        recentSlowRequests: [],
        slowThresholdMs: 500,
      },
    },
    strategyRuntime: {
      activeInstances: [
        {
          activeSymbols: ["US.AAPL"],
          actualStatus: "FUTURE_STATUS",
          definitionName: "demo",
          instanceId: "instance-1",
        },
      ],
      activeStrategies: 1,
      status: "ready",
      supportsBacktestParity: true,
    },
    message: "ok",
  };
}

describe("system contract mappers", () => {
  it("keeps partial broker capabilities usable and defaults nullable config fields", () => {
    const mapped = mapBrokerSettings({
      brokers: [
        {
          descriptor: {
            id: "futu",
            displayName: "Futu OpenD",
            environments: ["SIMULATE", "REAL"],
            capabilities: [
              { market: "HK", supportsQuote: true, supportsTrade: true },
            ],
            notes: [],
          },
          integration: {
            brokerId: "futu",
            enabled: true,
            config: { host: null, websocketKey: null },
          },
          defaults: null,
        },
      ],
      accounts: [
        {
          accountId: "SIM-1",
          brokerId: "futu",
          enabled: true,
          securityFirm: null,
        },
      ],
    } as never);

    expect(mapped.brokers[0]?.descriptor.capabilities[0]?.readFeatures).toBeUndefined();
    expect(mapped.brokers[0]?.integration?.config).toMatchObject({
      host: "127.0.0.1",
      websocketKey: "",
      apiPort: 11110,
    });
    expect(mapped.accounts[0]).toMatchObject({
      accountId: "SIM-1",
      displayName: "",
      securityFirm: null,
    });
  });

  it("drops malformed brokers while preserving disabled integration and explicit defaults", () => {
    const descriptor = {
      id: "futu",
      displayName: "Futu OpenD",
      environments: ["SIMULATE", "REAL"],
      capabilities: [
        { market: "HK", supportsQuote: true, supportsTrade: true },
      ],
      notes: [],
    };
    const mapped = mapBrokerSettings({
      brokers: [
        {
          descriptor,
          integration: null,
          defaults: {
            host: "10.0.0.2",
            apiPort: 12000,
            websocketPort: 12001,
            maxWebSocketConnections: 8,
            useEncryption: true,
            websocketKey: "secret",
            tradeMarket: "HK",
            securityFirm: "FUTUSECURITIES",
          },
        },
        { descriptor: { id: "broken" } },
      ],
      accounts: [{}],
    } as never);

    expect(mapped.brokers).toHaveLength(1);
    expect(mapped.brokers[0]).toMatchObject({
      descriptor,
      integration: null,
      defaults: {
        host: "10.0.0.2",
        apiPort: 12000,
        websocketPort: 12001,
        maxWebSocketConnections: 8,
        useEncryption: true,
        websocketKey: "secret",
        tradeMarket: "HK",
        securityFirm: "FUTUSECURITIES",
      },
    });
    expect(mapped.accounts[0]).toEqual({
      id: "",
      brokerId: "",
      accountId: "",
      displayName: "",
      tradingEnvironment: "",
      market: "",
      securityFirm: null,
      enabled: false,
      updatedAt: "",
      createdAt: "",
    });
  });

  it("maps unknown onboarding severities to a visible neutral value", () => {
    const mapped = mapOnboardingState({
      state: null,
      shouldShowOobe: true,
      reasons: [
        { code: "FUTURE_REASON", severity: "future", message: "review" },
      ],
      recommendedBrokerId: "futu",
      brokers: [],
    } as never);

    expect(mapped.state).toMatchObject({ completed: false, lastBrokerId: "" });
    expect(mapped.reasons[0]).toEqual({
      code: "FUTURE_REASON",
      severity: "info",
      message: "review",
    });
  });

  it("validates broker read-feature capabilities field by field", () => {
    const descriptor = {
      id: "futu",
      displayName: "Futu OpenD",
      environments: ["SIMULATE"],
      notes: ["OpenD required"],
      capabilities: [{
        market: "HK",
        supportsQuote: true,
        supportsTrade: true,
        readFeatures: {
          history: {
            supportedEnvironments: ["SIMULATE", "REAL"],
            supportsHistory: true,
            requiresSymbols: true,
            requiresClearingDate: false,
            requiresPrice: false,
            requiresOrderIdEx: false,
            requiresSymbol: true,
            requiresPassword: false,
            supportsRealTimePush: true,
            defaultNum: 10,
            minNum: 1,
            maxNum: 100,
            numPresets: [10, 20, 50],
          },
        },
      }],
    };

    expect(isBrokerDescriptor(descriptor)).toBe(true);
    expect(isBrokerDescriptor(null)).toBe(false);
    expect(isBrokerDescriptor({ ...descriptor, environments: ["SIMULATE", 1] })).toBe(false);
    expect(isBrokerDescriptor({
      ...descriptor,
      capabilities: [{ ...descriptor.capabilities[0], supportsQuote: "yes" }],
    })).toBe(false);
    expect(isBrokerDescriptor({
      ...descriptor,
      capabilities: [{ ...descriptor.capabilities[0], readFeatures: "history" }],
    })).toBe(false);
    expect(isBrokerDescriptor({
      ...descriptor,
      capabilities: [{
        ...descriptor.capabilities[0],
        readFeatures: {
          history: {
            supportedEnvironments: ["SIMULATE"],
            supportsHistory: "yes",
          },
        },
      }],
    })).toBe(false);
    expect(isBrokerDescriptor({
      ...descriptor,
      capabilities: [{
        ...descriptor.capabilities[0],
        readFeatures: {
          history: {
            supportedEnvironments: ["SIMULATE"],
            defaultNum: "ten",
          },
        },
      }],
    })).toBe(false);
    expect(isBrokerDescriptor({
      ...descriptor,
      capabilities: [{
        ...descriptor.capabilities[0],
        readFeatures: {
          history: {
            supportedEnvironments: ["SIMULATE"],
            numPresets: [10, "twenty"],
          },
        },
      }],
    })).toBe(false);
  });

  it("preserves onboarding timestamps, supported severities, and valid brokers", () => {
    const descriptor = {
      id: "futu",
      displayName: "Futu OpenD",
      environments: ["SIMULATE"],
      capabilities: [
        { market: "HK", supportsQuote: true, supportsTrade: true },
      ],
      notes: [],
    };
    const mapped = mapOnboardingState({
      state: {
        completed: true,
        lastBrokerId: "futu",
        completedAt: "2026-07-26T10:00:00Z",
        dismissedAt: "2026-07-26T10:01:00Z",
      },
      shouldShowOobe: false,
      reasons: [
        { code: "OPEND_OFFLINE", severity: "error", message: "start OpenD" },
        { code: "ACCOUNT_MISSING", severity: "warning", message: "select account" },
        { code: undefined, severity: undefined, message: undefined },
      ],
      recommendedBrokerId: undefined,
      brokers: [
        {
          descriptor,
          enabled: true,
          available: true,
          configured: true,
        },
        { descriptor: { id: "broken" } },
      ],
    } as never);

    expect(mapped.state).toEqual({
      completed: true,
      lastBrokerId: "futu",
      completedAt: "2026-07-26T10:00:00Z",
      dismissedAt: "2026-07-26T10:01:00Z",
    });
    expect(mapped.reasons.map((reason) => reason.severity)).toEqual([
      "error",
      "warning",
      "info",
    ]);
    expect(mapped.reasons[2]).toEqual({
      code: "UNKNOWN",
      severity: "info",
      message: "",
    });
    expect(mapped.recommendedBrokerId).toBe("futu");
    expect(mapped.brokers).toEqual([{
      descriptor,
      enabled: true,
      available: true,
      configured: true,
    }]);
  });

  it("normalizes nullable OpenD diagnostics and future enum values", () => {
    const wire = {
      ...futuHealthWire(),
      status: "future-status",
      runtime: {
        ...futuHealthWire().runtime,
        connectivity: "future-connectivity",
      },
      diagnosis: { ...futuHealthWire().diagnosis, code: "FUTURE_ISSUE" },
      localInstallation: null,
      latestVersion: null,
    };

    const mapped = mapFutuOpenDHealth(wire as never);

    expect(mapped.status).toBe("offline");
    expect(mapped.runtime.connectivity).toBe("disconnected");
    expect(mapped.runtime.port).toBe(11111);
    expect(mapped.diagnosis.code).toBe("OPEND_API_CONNECTIVITY");
    expect(mapped.localInstallation).toMatchObject({
      installed: false,
      version: null,
      process: { running: false, pid: null },
    });
    expect(mapped.latestVersion).toMatchObject({ status: "unknown", value: null });
  });

  it("drops unsupported install options while preserving the generated settings", () => {
    const mapped = mapFutuOpenDInstallGuide({
      brokerId: "future-broker",
      title: "安装 OpenD",
      description: "guide",
      nextSteps: ["start"],
      options: [
        {
          id: "gui",
          label: "GUI",
          description: "desktop",
          recommended: true,
          url: "https://example.invalid/gui",
        },
        {
          id: "future-option",
          label: "Future",
          description: "unsupported",
          recommended: false,
          url: "https://example.invalid/future",
        },
      ],
      settings: {
        apiPort: 11110,
        host: "127.0.0.1",
        marketDataTransport: "websocket",
        maxWebSocketConnections: 20,
        minimumVersion: "10.9.6908",
        useEncryption: false,
        websocketKeyRequired: false,
        websocketPort: 11111,
      },
    } as never);

    expect(mapped.brokerId).toBe("futu");
    expect(mapped.options.map((option) => option.id)).toEqual(["gui"]);
    expect(mapped.settings.websocketPort).toBe(11111);
  });

  it("uses visible fallbacks for invalid brokers and future status values", () => {
    const wire = {
      ...systemStatusWire(),
      broker: { id: 123 },
    };

    const mapped = mapSystemStatus(wire as never);

    expect(mapped.broker).toEqual(emptySystemStatus.broker);
    expect(mapped.strategyRuntime.activeInstances[0]?.actualStatus).toBe("STOPPED");
    expect(mapped.observability.requests.minimumImportance).toBe("low");
    expect(mapped.observability.requests.recentErrors[0]?.importance).toBe("low");
  });

  it("preserves optional observability context when it is present", () => {
    const wire = systemStatusWire();
    wire.observability.requests.recentErrors[0] = {
      ...wire.observability.requests.recentErrors[0]!,
      method: "GET",
      path: "/api/v1/system/status",
      requestId: "request-1",
    } as never;

    const mapped = mapSystemStatus(wire as never);

    expect(mapped.observability.requests.recentErrors[0]).toMatchObject({
      method: "GET",
      path: "/api/v1/system/status",
      requestId: "request-1",
    });
  });

  it("normalizes null request observability emitted by Go empty slices", () => {
    const wire = systemStatusWire();
    wire.observability.requests.recentErrors = null as never;
    wire.observability.requests.recentSlowRequests = null as never;

    const mapped = mapSystemStatus(wire as never);

    expect(mapped.observability.requests.recentErrors).toEqual([]);
    expect(mapped.observability.requests.recentSlowRequests).toEqual([]);

    wire.observability.requests = null as never;
    expect(mapSystemStatus(wire as never).observability.requests).toEqual(
      emptySystemStatus.observability.requests,
    );
  });

  it("maps complete request diagnostics and runtime observation timestamps", () => {
    const wire = systemStatusWire();
    wire.observability.requests.recentSlowRequests = [{
      at: "2026-07-26T10:00:00Z",
      level: "WARN",
      importance: "critical",
      message: "slow broker request",
      error: "deadline exceeded",
      method: "POST",
      path: "/api/v1/trading/orders",
      operation: "place_order",
      status: 504,
      latencyMs: 1_500,
      requestId: "request-2",
      sessionId: "session-1",
      runId: "run-1",
      taskId: "task-1",
      brokerId: "futu",
      accountId: "SIM-1",
      instrumentId: "HK.00700",
      providerId: "provider-1",
      source: "opend",
    }] as never;
    wire.strategyRuntime.activeInstances[0] = {
      ...wire.strategyRuntime.activeInstances[0]!,
      actualStatus: "RUNNING",
      lastClosedKlineAt: "2026-07-26T10:00:01Z",
      lastSignalAt: "2026-07-26T10:00:02Z",
      lastOrderAt: "2026-07-26T10:00:03Z",
      lastErrorAt: "2026-07-26T10:00:04Z",
      lastError: "order rejected",
      updatedAt: "2026-07-26T10:00:05Z",
    } as never;

    const mapped = mapSystemStatus(wire as never);

    expect(mapped.observability.requests.recentSlowRequests[0]).toMatchObject({
      importance: "critical",
      operation: "place_order",
      status: 504,
      latencyMs: 1_500,
      sessionId: "session-1",
      runId: "run-1",
      taskId: "task-1",
      brokerId: "futu",
      accountId: "SIM-1",
      instrumentId: "HK.00700",
      providerId: "provider-1",
      source: "opend",
    });
    expect(mapped.strategyRuntime.activeInstances[0]).toMatchObject({
      actualStatus: "RUNNING",
      lastClosedKlineAt: "2026-07-26T10:00:01Z",
      lastSignalAt: "2026-07-26T10:00:02Z",
      lastOrderAt: "2026-07-26T10:00:03Z",
      lastErrorAt: "2026-07-26T10:00:04Z",
      lastError: "order rejected",
      updatedAt: "2026-07-26T10:00:05Z",
    });
  });

  it("uses the empty runtime summary when an older status omits it", () => {
    const wire = {
      ...systemStatusWire(),
      strategyRuntime: null,
    };

    const mapped = mapSystemStatus(wire as never);

    expect(mapped.strategyRuntime).toEqual(emptySystemStatus.strategyRuntime);
  });
});
