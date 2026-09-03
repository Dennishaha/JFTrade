import assert from "node:assert/strict";
import test from "node:test";

import {
  assertDesktopRuntimeConfiguration,
  assertDesktopRuntimeEquivalent,
} from "./check-desktop-runtime.mjs";

const minimal = {
  tauriVersion: "2.11.5",
  profiles: [{}, {}, {}],
  links: [
    { accepted: true },
    { accepted: true },
    { accepted: true },
    { accepted: false },
  ],
  runtimePlan: {
    startOrder: ["engine", "pine-worker", "marketdata-sidecar"],
    shutdownOrder: ["marketdata-sidecar", "pine-worker", "engine"],
  },
  successfulStart: { ready: true },
  failedStart: { ready: false, failureRole: "marketdata-sidecar" },
  commands: Array.from({ length: 10 }, (_, index) => `command-${index}`),
  events: Array.from({ length: 4 }, (_, index) => `event-${index}`),
};

test("desktop runtime accepts an identical shell projection", () => {
  assert.doesNotThrow(() => assertDesktopRuntimeEquivalent(structuredClone(minimal), minimal));
});

test("desktop runtime rejects lifecycle drift", () => {
  const drifted = structuredClone(minimal);
  drifted.runtimePlan.shutdownOrder.reverse();
  assert.throws(() => assertDesktopRuntimeEquivalent(drifted, minimal));
});

test("desktop runtime configuration requires a CSP and every facade contract", () => {
  const config = {
    identifier: "com.jftrade.desktop",
    build: { frontendDist: "../../web/dist" },
    bundle: {
      active: true,
      resources: {
        "../../../var/tauri-runtime/": "runtime/node",
        "../../../runtime-assets/pine/worker.mjs": "runtime/pineworker/worker.mjs",
        "../../../proto/pineworker/": "runtime/pineworker/proto",
        "../../../runtime-assets/marketdata/": "runtime/marketdata",
      },
    },
    app: {
      windows: [{ visible: false }],
      security: { csp: { "connect-src": "ipc: http://ipc.localhost" } },
    },
  };
  const contracts = [...minimal.commands, ...minimal.events].join("\n");
  assert.doesNotThrow(() => assertDesktopRuntimeConfiguration(config, contracts, minimal));
  config.app.security.csp = null;
  assert.throws(() => assertDesktopRuntimeConfiguration(config, contracts, minimal));
});
