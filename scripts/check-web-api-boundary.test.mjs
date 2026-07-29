import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
  directFetchViolations,
  manualEnvelopeViolations,
  publicManualEnvelopeViolations,
} from "./check-web-api-boundary.mjs";

test("allows the shared client and rejects direct feature fetches", () => {
  const root = mkdtempSync(join(tmpdir(), "jftrade-web-boundary-"));
  mkdirSync(join(root, "composables/shared"), { recursive: true });
  mkdirSync(join(root, "features"), { recursive: true });
  writeFileSync(join(root, "composables/shared/apiClient.ts"), "fetch('/api/v1/status');\n");
  writeFileSync(join(root, "features/orders.ts"), "await fetch('/api/v1/orders');\n");
  writeFileSync(join(root, "features/query.ts"), "query.refetch();\n");

  assert.deepEqual(directFetchViolations(root), [
    { path: "features/orders.ts", line: 1 },
  ]);
});

test("rejects caller-selected envelope response types outside the shared client", () => {
  const root = mkdtempSync(join(tmpdir(), "jftrade-web-envelope-boundary-"));
  mkdirSync(join(root, "composables/shared"), { recursive: true });
  mkdirSync(join(root, "pages"), { recursive: true });
  writeFileSync(
    join(root, "composables/shared/apiClient.ts"),
    "export function fetchEnvelope<T>(): Promise<T> { throw new Error(); }\n",
  );
  writeFileSync(
    join(root, "pages/Orders.ts"),
    "const orders = await fetchEnvelope<Order[]>('/api/v1/orders');\n",
  );

  assert.deepEqual(manualEnvelopeViolations(root), [
    { path: "pages/Orders.ts", line: 1 },
  ]);
  assert.deepEqual(publicManualEnvelopeViolations(root), [
    { path: "composables/shared/apiClient.ts", line: 1 },
  ]);
});

test("allows a private envelope parser behind generated operation helpers", () => {
  const root = mkdtempSync(join(tmpdir(), "jftrade-web-private-envelope-"));
  mkdirSync(join(root, "composables/shared"), { recursive: true });
  writeFileSync(
    join(root, "composables/shared/apiClient.ts"),
    "async function requestEnvelope<T>(): Promise<T> { throw new Error(); }\nexport function apiGet() { return requestEnvelope(); }\n",
  );

  assert.deepEqual(publicManualEnvelopeViolations(root), []);
});
