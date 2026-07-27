import { describe, expect, it } from "vitest";

import type { components } from "@/generated/openapi";
import {
  mapPluginCatalog,
  mapPluginMutation,
  mapPluginOperation,
  mapPluginUninstallGuidance,
} from "../src/composables/pluginContract";
import {
  mapStrategyBindingRequest,
  mapStrategyRuntimeRiskRequest,
} from "../src/composables/strategyApiRequests";
import {
  mapStrategyInstance,
  mapStrategyInstances,
} from "../src/composables/strategyContract";

describe("strategy and plugin API contract mappers", () => {
  it("preserves nullable runtime limits while omitting an unbound broker account", () => {
    expect(mapStrategyRuntimeRiskRequest({
      mode: "enforce",
      closeOnly: true,
      maxOrderQuantity: null,
      maxOrderNotional: 25_000,
      pauseOnReject: true,
    })).toEqual({
      mode: "enforce",
      closeOnly: true,
      maxOrderQuantity: null,
      maxOrderNotional: 25_000,
      pauseOnReject: true,
    });

    expect(mapStrategyBindingRequest({
      instruments: [{ market: "US", code: "AAPL" }],
      symbols: ["US.AAPL"],
      interval: "5m",
      executionMode: "live",
      brokerAccount: null,
      runtimeRisk: {
        mode: "off",
        closeOnly: false,
        pauseOnReject: false,
      },
    })).toEqual({
      instruments: [{ market: "US", code: "AAPL" }],
      symbols: ["US.AAPL"],
      interval: "5m",
      chartType: "standard",
      executionMode: "live",
      runtimeRisk: {
        mode: "off",
        closeOnly: false,
        pauseOnReject: false,
      },
    });
  });

  it("includes an explicit broker binding and omits absent optional instruments", () => {
    expect(mapStrategyBindingRequest({
      symbols: ["HK.00700"],
      interval: "1m",
      chartType: "heikinashi",
      executionMode: "notify_only",
      brokerAccount: {
        brokerId: "futu",
        accountId: "SIM-1",
        tradingEnvironment: "SIMULATE",
        market: "HK",
      },
      runtimeRisk: {
        mode: "monitor",
        closeOnly: false,
        maxOrderQuantity: 500,
        maxOrderNotional: undefined,
        dailyMaxOrders: 20,
        pauseOnReject: true,
      },
    })).toEqual({
      symbols: ["HK.00700"],
      interval: "1m",
      chartType: "heikinashi",
      executionMode: "notify_only",
      brokerAccount: {
        brokerId: "futu",
        accountId: "SIM-1",
        tradingEnvironment: "SIMULATE",
        market: "HK",
      },
      runtimeRisk: {
        mode: "monitor",
        closeOnly: false,
        maxOrderQuantity: 500,
        dailyMaxOrders: 20,
        pauseOnReject: true,
      },
    });
  });

  it("normalizes unknown plugin enums and nullable operation fields", () => {
    const wire = {
      targetDir: "/plugins",
      plugins: [{
        descriptor: {
          id: "demo",
          type: "strategy",
          displayName: "Demo",
          version: "1.0.0",
          description: "",
          keywords: [],
        },
        installation: {
          status: "FUTURE_STATUS",
          installed: false,
          installPath: "",
          targetDir: "/plugins",
          markerPath: "",
          currentOperation: {
            operationId: "operation-1",
            pluginId: "demo",
            status: "FUTURE_STATUS",
            phase: "queued",
            progress: 0,
            message: "waiting",
            targetDir: "/plugins",
            installPath: "",
            startedAt: "2026-07-26T00:00:00Z",
            updatedAt: "2026-07-26T00:00:00Z",
            completedAt: null,
            error: null,
          },
          lastOperation: null,
          uninstallGuidance: {
            pluginId: "demo",
            path: "/plugins/demo.so",
            exists: true,
            commands: { posix: "rm demo.so", powershell: "Remove-Item demo.so" },
          },
        },
        compatibility: {
          mode: "native",
          supported: true,
          requiresRebuild: false,
          host: {
            jftradeVersion: "dev",
            goVersion: "go1.26.5",
            goos: "darwin",
            goarch: "arm64",
            buildMode: "desktop",
          },
        },
      }],
    } satisfies components["schemas"]["strategy.PluginCatalog"];

    const mapped = mapPluginCatalog(wire);
    expect(mapped.plugins[0]?.installation.status).toBe("NOT_INSTALLED");
    expect(mapped.plugins[0]?.installation.currentOperation).toMatchObject({
      status: "FAILED",
      completedAt: null,
      error: null,
    });
  });

  it("preserves every supported plugin lifecycle status", () => {
    for (const status of ["QUEUED", "RUNNING", "SUCCEEDED"] as const) {
      expect(mapPluginOperation({
        operationId: `operation-${status}`,
        pluginId: "demo",
        status,
        phase: "install",
        progress: 50,
        message: "working",
        targetDir: "/plugins",
        installPath: "/plugins/demo.so",
        startedAt: "2026-07-26T00:00:00Z",
        updatedAt: "2026-07-26T00:00:01Z",
        completedAt: null,
        error: null,
      }).status).toBe(status);
    }

    const sparseOperation = mapPluginOperation({
      status: "future-status",
    } as never);
    expect(sparseOperation).toEqual({
      operationId: "",
      pluginId: "",
      status: "FAILED",
      phase: "",
      progress: 0,
      message: "",
      targetDir: "",
      installPath: "",
      startedAt: "",
      updatedAt: "",
      completedAt: null,
      error: null,
    });
  });

  it("maps sparse uninstall guidance and plugin mutation responses", () => {
    expect(mapPluginUninstallGuidance({} as never)).toEqual({
      pluginId: "",
      path: "",
      exists: false,
      commands: { posix: "", powershell: "" },
    });

    const operation = {
      operationId: "operation-1",
      pluginId: "demo",
      status: "SUCCEEDED",
      phase: "complete",
      progress: 100,
      message: "installed",
      targetDir: "/plugins",
      installPath: "/plugins/demo.so",
      startedAt: "2026-07-26T00:00:00Z",
      updatedAt: "2026-07-26T00:00:01Z",
      completedAt: "2026-07-26T00:00:01Z",
      error: null,
    } as const;

    expect(mapPluginMutation({ operation })).toEqual({
      operation: expect.objectContaining({
        operationId: "operation-1",
        status: "SUCCEEDED",
        completedAt: "2026-07-26T00:00:01Z",
      }),
    });
  });

  it("keeps rebuild diagnostics and supplies safe defaults for sparse catalog entries", () => {
    const mapped = mapPluginCatalog({
      targetDir: "/plugins",
      plugins: [
        {
          descriptor: {
            id: "rebuild",
            type: "strategy",
            displayName: "Rebuild required",
            version: "1.0.0",
            description: "compiled for another host",
            keywords: ["pine"],
          },
          installation: {
            status: "INSTALLING",
            installed: true,
            installPath: "/plugins/rebuild.so",
            targetDir: "/plugins",
            markerPath: "/plugins/rebuild.marker",
            currentOperation: null,
            lastOperation: null,
            uninstallGuidance: {
              pluginId: "rebuild",
              path: "/plugins/rebuild.so",
              exists: true,
              commands: { posix: "rm rebuild.so", powershell: "Remove-Item rebuild.so" },
            },
          },
          compatibility: {
            mode: "native",
            supported: false,
            requiresRebuild: true,
            reason: "build tags differ",
            host: {
              jftradeVersion: "dev",
              goVersion: "go1.26.5",
              goos: "darwin",
              goarch: "arm64",
              buildMode: "desktop",
              buildTags: ["desktop"],
            },
            artifact: {
              jftradeVersion: "dev",
              goVersion: "go1.26.5",
              goos: "linux",
              goarch: "amd64",
              buildMode: "api",
              buildTags: ["server"],
            },
          },
        },
        {} as never,
      ],
    });

    expect(mapped.plugins[0]).toMatchObject({
      installation: { status: "INSTALLING", currentOperation: null },
      compatibility: {
        reason: "build tags differ",
        host: { buildTags: ["desktop"] },
        artifact: { buildTags: ["server"] },
      },
    });
    expect(mapped.plugins[1]).toMatchObject({
      descriptor: { id: "", keywords: [] },
      installation: {
        status: "NOT_INSTALLED",
        installed: false,
        uninstallGuidance: { pluginId: "" },
      },
      compatibility: {
        mode: "",
        supported: false,
        requiresRebuild: false,
        host: {
          jftradeVersion: "",
          goVersion: "",
          goos: "",
          goarch: "",
          buildMode: "",
        },
      },
    });
  });

  it("keeps a future strategy status observable instead of silently relabeling it", () => {
    const wire = {
      id: "instance-1",
      definition: { strategyId: "strategy-1", name: "Demo", version: "1.0.0" },
      binding: {
        symbols: ["US.AAPL"],
        interval: "5m",
        chartType: "standard",
        executionMode: "live",
        runtimeRisk: { mode: "off", closeOnly: false, pauseOnReject: false },
      },
      params: {},
      runtime: "pine-pinets",
      sourceFormat: "pine-v6",
      startable: false,
      status: "SYNCING",
      createdAt: "2026-07-26T00:00:00Z",
      logs: [],
    } satisfies components["schemas"]["strategy.InstanceView"];

    expect(mapStrategyInstance(wire).status).toBe("SYNCING");
  });

  it("maps a fully bound strategy runtime without dropping diagnostics", () => {
    const mapped = mapStrategyInstance({
      id: "instance-full",
      pluginId: "plugin-1",
      definition: {
        strategyId: "strategy-1",
        name: "Demo",
        version: "2.0.0",
      },
      binding: {
        instruments: [{ market: "HK", code: "00700" }],
        symbols: ["HK.00700"],
        interval: "1m",
        chartType: "heikinashi",
        executionMode: "live",
        brokerAccount: {
          brokerId: "futu",
          accountId: "SIM-1",
          tradingEnvironment: "SIMULATE",
          market: "HK",
        },
        runtimeRisk: {
          mode: "enforce",
          closeOnly: true,
          maxOrderQuantity: 1_000,
          maxOrderNotional: 500_000,
          dailyMaxOrders: 50,
          pauseOnReject: true,
        },
      },
      params: { length: 20 },
      runtime: "pine-pinets",
      sourceFormat: "pine-v6",
      startable: true,
      status: "RUNNING",
      createdAt: "2026-07-26T00:00:00Z",
      logs: ["started"],
      definitionSync: {
        definitionId: "definition-1",
        appliedVersion: "1.0.0",
        latestVersion: "2.0.0",
        isLatest: false,
        canApplyLatest: false,
        blockedReason: "instance is running",
      },
      runtimeObservation: {
        actualStatus: "PAUSED",
        activeSymbols: ["HK.00700"],
        lastClosedKlineAt: "2026-07-26T00:01:00Z",
        lastSignalAt: "2026-07-26T00:01:01Z",
        lastOrderAt: "2026-07-26T00:01:02Z",
        lastErrorAt: "2026-07-26T00:01:03Z",
        lastError: "risk rejected",
        updatedAt: "2026-07-26T00:01:04Z",
      },
    });

    expect(mapped).toMatchObject({
      pluginId: "plugin-1",
      binding: {
        instruments: [{ market: "HK", code: "00700" }],
        chartType: "heikinashi",
        executionMode: "live",
        brokerAccount: {
          brokerId: "futu",
          accountId: "SIM-1",
        },
        runtimeRisk: {
          mode: "enforce",
          maxOrderQuantity: 1_000,
          maxOrderNotional: 500_000,
          dailyMaxOrders: 50,
        },
      },
      definitionSync: {
        blockedReason: "instance is running",
      },
      runtimeObservation: {
        actualStatus: "PAUSED",
        lastError: "risk rejected",
        updatedAt: "2026-07-26T00:01:04Z",
      },
    });
  });

  it("uses stable defaults for incomplete legacy strategy instances", () => {
    const mapped = mapStrategyInstance({
      id: undefined,
      definition: null,
      binding: {
        instruments: [{ market: undefined, code: undefined }],
        symbols: undefined,
        interval: undefined,
        chartType: "future-chart",
        executionMode: "future-mode",
        runtimeRisk: {
          mode: "future-mode",
          closeOnly: undefined,
          pauseOnReject: undefined,
        },
      },
      params: undefined,
      runtime: undefined,
      startable: undefined,
      status: " ",
      createdAt: undefined,
      logs: undefined,
      definitionSync: {
        definitionId: undefined,
        appliedVersion: undefined,
        latestVersion: undefined,
        isLatest: undefined,
        canApplyLatest: undefined,
      },
      runtimeObservation: {
        actualStatus: undefined,
        activeSymbols: undefined,
      },
    } as never);

    expect(mapped).toMatchObject({
      id: "",
      definition: { strategyId: "", name: "", version: "" },
      runtime: "pine-pinets",
      startable: false,
      binding: {
        instruments: [{ market: "", code: "" }],
        symbols: [],
        interval: "",
        executionMode: "notify_only",
        runtimeRisk: {
          mode: "off",
          closeOnly: false,
          pauseOnReject: false,
        },
      },
      params: {},
      status: "STOPPED",
      createdAt: "",
      logs: [],
      runtimeObservation: {
        actualStatus: "STOPPED",
        activeSymbols: [],
      },
    });
    expect(mapped.binding).not.toHaveProperty("chartType");
    expect(mapStrategyInstance({ binding: null } as never)).not.toHaveProperty("binding");
    expect(mapStrategyInstances(undefined)).toEqual([]);
  });
});
