#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const fixtureRoot = path.join(repositoryRoot, "tests/fixtures/compatibility/api-transport");
const corpusPath = path.join(fixtureRoot, "api-control-plane-corpus.json");
const expectedPath = path.join(fixtureRoot, "api-control-plane-corpus.expected.json");
const operationMethods = new Set(["get", "post", "put", "patch", "delete"]);

function compareText(left, right) {
  return left < right ? -1 : left > right ? 1 : 0;
}

function concretePath(template) {
  return template.replaceAll(/\{[^{}]+\}/g, "fixture");
}

function routeGroup(route) {
  return route.path.slice("/api/v1/".length).split("/")[0];
}

export function buildApiTransportCorpus(openapi) {
  const routes = [];
  for (const [routePath, item] of Object.entries(openapi.paths ?? {})) {
    for (const method of Object.keys(item)) {
      if (operationMethods.has(method)) {
        routes.push({ method: method.toUpperCase(), path: routePath });
      }
    }
  }
  routes.sort((left, right) => compareText(left.method, right.method) || compareText(left.path, right.path));
  const firstByGroup = new Map();
  for (const route of routes) {
    if (!firstByGroup.has(routeGroup(route))) firstByGroup.set(routeGroup(route), route);
  }
  const routeProbes = [...firstByGroup.values()].map((route) => ({
    method: route.method,
    path: concretePath(route.path),
  }));
  routeProbes.push({ method: "DELETE", path: "/api/v1/unknown-stage7-route" });
  return {
    version: "stage7.v1",
    routes,
    routeProbes,
    research: {
      presetId: " preset-stage7 ",
      name: " Momentum 研究 ",
      definition: { conditions: [{ field: "close", operator: "gt", value: 100 }] },
      update: { name: "Momentum v2", definition: null, expectedRevision: 1 },
    },
    watchlist: {
      instrumentId: " hk.00700 ",
      groupIds: ["g2", " g1 ", "g1"],
      newGroupNames: [" Tech ", "tech", " 长线 "],
      expectedRevision: 7,
      requestedLimit: 999,
    },
    settings: {
      security: {
        webAccessEnabled: true,
        publicAccessEnabled: true,
        webPort: 3000,
        password: "correct horse battery staple",
        passwordConfigured: false,
      },
      providerId: " AKShare ",
    },
    calendar: {
      market: " hk ",
      sources: [" Manual ", "futu", "manual"],
      sessions: [
        { openMinute: 780, closeMinute: 960 },
        { openMinute: 570, closeMinute: 720 },
      ],
    },
    cleanup: {
      databaseId: "assistant",
      previewCandidates: [
        { id: "run-b", category: "run" },
        { id: "run-a", category: "run" },
      ],
      executeCandidates: [
        { id: "run-a", category: "run" },
        { id: "run-b", category: "run" },
      ],
    },
  };
}

function run() {
  const openapi = JSON.parse(fs.readFileSync(
    path.join(repositoryRoot, "contracts/openapi/openapi.json"),
    "utf8",
  ));
  fs.mkdirSync(fixtureRoot, { recursive: true });
  fs.writeFileSync(corpusPath, `${JSON.stringify(buildApiTransportCorpus(openapi), null, 2)}\n`);
  if (process.argv.includes("--write-expected")) {
    const result = spawnSync("cargo", [
      "run", "--quiet", "-p", "jftrade-engine", "--bin", "jftrade-api-transport-replay", "--",
      "--input", corpusPath,
    ], { cwd: repositoryRoot, encoding: "utf8", stdio: ["ignore", "pipe", "inherit"] });
    if (result.error) throw result.error;
    if (result.status !== 0) process.exit(result.status ?? 1);
    fs.writeFileSync(expectedPath, `${JSON.stringify(JSON.parse(result.stdout), null, 2)}\n`);
  }
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) run();
