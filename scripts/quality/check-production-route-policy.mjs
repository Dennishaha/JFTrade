#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const productionSourceRoot = "crates/jftrade-engine/src";
const generalForbiddenTokens = Object.freeze([
  "ProductRoutePorts::all",
  "RUST_OWNER_NOT_IMPLEMENTED",
]);

const marketDataForbiddenTokens = Object.freeze([
  "fixture=",
  "fixture-time",
  "fixture-candles",
  "fixture-empty",
  "fixture-quote",
  "Fixture Apple",
  "Fixture Moutai",
  "Fixture Security",
  "101.25",
  "resolve_instrument_candidates",
  "block_on_async",
  "block_in_place",
  "synthetic",
]);

function isProductionSource(name) {
  return name.endsWith(".rs")
    && !name.endsWith("_tests.rs")
    && (name === "product.rs" || name.startsWith("product_"));
}

export function productionSourceFiles(root = repositoryRoot) {
  const sourceRoot = path.join(root, productionSourceRoot);
  if (!fs.existsSync(sourceRoot)) return [];
  return fs.readdirSync(sourceRoot, { withFileTypes: true })
    .filter((entry) => entry.isFile() && isProductionSource(entry.name))
    .map((entry) => path.join(sourceRoot, entry.name))
    .sort();
}

export function validateProductionRoutePolicy(files) {
  const errors = [];
  for (const [file, contents] of files) {
    for (const token of generalForbiddenTokens) {
      if (contents.includes(token)) {
        errors.push(`${file} contains forbidden production route token: ${token}`);
      }
    }
    if (file.includes("product_production_ports_market_data")) {
      for (const token of marketDataForbiddenTokens) {
        if (contents.includes(token)) {
          errors.push(`${file} contains forbidden market data production token: ${token}`);
        }
      }
    }
  }
  return errors;
}

export function checkProductionRoutePolicy(root = repositoryRoot) {
  const files = productionSourceFiles(root).map((file) => [
    path.relative(root, file).split(path.sep).join("/"),
    fs.readFileSync(file, "utf8"),
  ]);
  if (files.length === 0) {
    return [`${productionSourceRoot} has no production ProductApi sources`];
  }
  return validateProductionRoutePolicy(files);
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const errors = checkProductionRoutePolicy();
  if (errors.length > 0) {
    console.error(errors.map((error) => `- ${error}`).join("\n"));
    process.exitCode = 1;
  } else {
    console.log("Rust production route policy passed.");
  }
}
