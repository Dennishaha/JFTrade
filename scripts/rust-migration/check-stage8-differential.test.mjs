import assert from "node:assert/strict";
import test from "node:test";

import {
  assertStage8Configuration,
  assertStage8Equivalent,
} from "./check-stage8-differential.mjs";

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

test("Stage 8 comparison accepts an identical shell projection", () => {
  assert.doesNotThrow(() => assertStage8Equivalent(structuredClone(minimal), minimal));
});

test("Stage 8 comparison rejects lifecycle drift", () => {
  const drifted = structuredClone(minimal);
  drifted.runtimePlan.shutdownOrder.reverse();
  assert.throws(() => assertStage8Equivalent(drifted, minimal));
});

test("Stage 8 configuration requires a CSP and every facade contract", () => {
  const config = {
    identifier: "com.jftrade.desktop",
    build: { frontendDist: "../../web/dist" },
    bundle: {
      active: true,
      resources: {
        "../../../var/tauri-runtime/": "runtime/node",
        "../../../internal/pineworkerassets/assets/bin/worker.mjs": "runtime/pineworker/worker.mjs",
        "../../../pkg/strategy/pineworker/proto/": "runtime/pineworker/proto",
        "../../../internal/marketdataassets/assets/bin/": "runtime/marketdata",
      },
    },
    app: {
      windows: [{ visible: false }],
      security: { csp: { "connect-src": "ipc: http://ipc.localhost" } },
    },
  };
  const contracts = [...minimal.commands, ...minimal.events].join("\n");
  assert.doesNotThrow(() => assertStage8Configuration(config, contracts, minimal));
  config.app.security.csp = null;
  assert.throws(() => assertStage8Configuration(config, contracts, minimal));
});
