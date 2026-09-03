import assert from "node:assert/strict";
import test from "node:test";

import {
  routeContractSnapshot,
  routeDigest,
  validateRouteContracts,
} from "./check-contracts.mjs";

test("current OpenAPI, Rust route manifest, and authentication policy agree", () => {
  const snapshot = routeContractSnapshot();
  assert.equal(snapshot.operations, 278);
  assert.equal(snapshot.authenticated, 276);
  assert.equal(snapshot.public, 2);
  assert.ok(snapshot.browserWrites > 0);
  assert.equal(snapshot.digest, "afa112435ed280dd24d43bb4acaa0f7ca2ab45c01e4e5701efc5ce149e5b85b2");
});

test("rejects route count, duplicate, digest, and authentication drift", () => {
  const routes = [
    { method: "GET", path: "/api/v1/auth/session", capability: "auth" },
    { method: "POST", path: "/api/v1/auth/login", capability: "auth" },
  ];
  const manifest = { version: "production.v1", operations: routes, routeDigest: routeDigest(routes) };
  const openapi = {
    paths: {
      "/api/v1/auth/session": { get: {} },
      "/api/v1/auth/login": { post: {} },
    },
  };
  const errors = validateRouteContracts(openapi, manifest, "");
  assert.ok(errors.some((error) => error.includes("278")));
  assert.ok(errors.some((error) => error.includes("authentication policy")));

  const duplicated = { ...manifest, operations: [...routes, routes[0]] };
  assert.ok(validateRouteContracts(openapi, duplicated, "").some((error) => error.includes("duplicate")));
});
