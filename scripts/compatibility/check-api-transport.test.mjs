import assert from "node:assert/strict";
import test from "node:test";

import { assertApiTransportEquivalent } from "./check-api-transport.mjs";

const minimal = {
  routes: Array.from({ length: 278 }, (_, index) => ({ method: "GET", path: `/api/v1/test/${index}` })),
  routeGroups: Object.fromEntries(Array.from({ length: 18 }, (_, index) => [`group-${index}`, 1])),
  routeProbes: [{ allowed: true }, { allowed: false }],
  transport: { websocketLimit: 20, sse: "retry: 3000\n\n" },
  security: { applyListenerAfterPersist: true },
  provider: { activateBeforePersist: true },
  cleanup: { preview: { candidates: [{ id: "a" }] }, approvedCandidates: [{ id: "a" }] },
};

test("API transport accepts an identical compatibility projection", () => {
  assert.doesNotThrow(() => assertApiTransportEquivalent(structuredClone(minimal), minimal));
});

test("API transport rejects route or cleanup drift", () => {
  const drifted = structuredClone(minimal);
  drifted.routeProbes.at(-1).allowed = true;
  assert.throws(() => assertApiTransportEquivalent(drifted, minimal));
});
