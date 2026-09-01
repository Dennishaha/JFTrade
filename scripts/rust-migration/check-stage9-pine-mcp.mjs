#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

function run(command, args) {
  const result = spawnSync(command, args, {
    cwd: repositoryRoot,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    timeout: 300_000,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} ${args.join(" ")} failed:\n${result.stderr || result.stdout}`);
}

run("go", ["test", "./scripts/rust-migration", "-run", "^TestStage9PineMCPFixtureMatchesCurrentGoOwner$", "-count=1"]);
run("cargo", ["test", "-p", "jftrade-strategy", "--test", "pine_mcp_contract", "--", "--nocapture"]);
run("cargo", ["test", "-p", "jftrade-engine", "--test", "strategy_pine_mcp_contract", "--", "--nocapture"]);
run("cargo", ["test", "-p", "jftrade-engine", "--lib", "product_mcp_server::", "--", "--nocapture"]);
run("cargo", ["test", "-p", "jftrade-engine", "--lib", "product_production_ports_strategy::pine_shadow_tests", "--", "--nocapture"]);
console.log("Stage 9 Pine MCP native leaf parity checks passed; support-matrix, v4.0 score model, externalEngine/saveHint field sets, and validation payload shape are covered; native production wiring is ready with structural fail-closed behavior for unavailable external PineTS modes.");
