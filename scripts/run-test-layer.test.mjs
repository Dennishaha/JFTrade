import assert from "node:assert/strict";
import test from "node:test";

import {
  commandsForLayer,
  executionStagesForLayer,
  parallelPreflightChecks,
  preflightChecks,
  runExecutionStages,
  sequentialPreflightChecks,
} from "./run-test-layer.mjs";

const checkGenerated = ["pnpm", ["run", "check:generated"]];
const checkDiff = ["pnpm", ["run", "check:diff"]];
const checkActionlint = ["pnpm", ["run", "check:actionlint"]];

test("preflight checks generated docs before running the shared checks", () => {
  const commands = commandsForLayer("preflight");
  const stages = executionStagesForLayer("preflight");

  assert.deepEqual(commands[0], checkGenerated);
  assert.deepEqual(commands.slice(2), preflightChecks);
  assert.deepEqual(stages, [
    { mode: "sequential", commands: [checkGenerated, checkDiff] },
    { mode: "parallel", commands: parallelPreflightChecks },
    { mode: "sequential", commands: sequentialPreflightChecks },
  ]);
  assert.equal(parallelPreflightChecks.length, 10);
});

test("main is the complete non-recursive gate and runs actionlint", () => {
  const commands = commandsForLayer("main");
  assert.equal(commands.some(([command, args]) => command === "pnpm" && args.join(" ") === "run test:ci-local"), false);
  assert.ok(commands.some(([command, args]) => command === checkDiff[0] && args.join(" ") === checkDiff[1].join(" ")));
  assert.equal(countPnpmScript(commands, "check:rust:workspace"), 1);
  assert.equal(countPnpmScript(commands, "check:rust:differential"), 1);
  assert.equal(countPnpmScript(commands, "check:rust"), 0);
  assert.deepEqual(commands.slice(-3), [
    checkActionlint,
    ["pnpm", ["run", "test:desktop"]],
    ["pnpm", ["run", "smoke:pinets-backtest"]],
  ]);
});

test("ci-local checks the working projection before running shared checks inline", () => {
  const commands = commandsForLayer("ci-local");
  const generated = commands.filter(
    ([command, args]) => command === "pnpm" && args.join(" ") === "run check:generated",
  );
  const firstCheckIndex = commands.findIndex(
    ([command, args]) =>
      command === preflightChecks[0][0] &&
      args.join(" ") === preflightChecks[0][1].join(" "),
  );

  assert.deepEqual(generated, [checkGenerated]);
  assert.deepEqual(commands.slice(0, 4), [
    checkGenerated,
    checkDiff,
    ["pnpm", ["run", "audit:dependencies"]],
    ["pnpm", ["run", "check:oss-license"]],
  ]);
  assert.equal(commands.some(([command]) => command === "git"), false);
  assert.equal(firstCheckIndex, 4);
  assert.deepEqual(
    commands.slice(firstCheckIndex, firstCheckIndex + preflightChecks.length),
    preflightChecks,
  );
  assert.equal(
    commands.some(
      ([command, args]) => command === "pnpm" && args.join(" ") === "run test:preflight",
    ),
    false,
  );
  assert.equal(
    commands.some(
      ([command, args]) => command === "pnpm" && args.join(" ") === "run test:scripts -- desktop",
    ),
    true,
  );
  assert.equal(
    commands.some(
      ([command, args]) => command === "pnpm" && args.join(" ") === "run test:tauri-release-runtime",
    ),
    true,
  );
  assert.equal(
    commands.some(
      ([command, args]) => command === "pnpm" && args.join(" ") === "run check:tauri-release-runtime",
    ),
    false,
  );
  assert.equal(countPnpmScript(commands, "check:rust:workspace"), 1);
  assert.equal(countPnpmScript(commands, "check:rust:differential"), 1);
  const frontendBuildIndex = commands.findIndex(
    ([command, args]) => command === "pnpm" && args.join(" ") === "run build:frontend-assets:generated",
  );
  assert.deepEqual(commands[frontendBuildIndex + 1], ["node", ["scripts/report-web-bundle.mjs"]]);
  const marketDataBuildIndex = commands.findIndex(
    ([command, args]) =>
      command === "pnpm" && args.join(" ") === "run build:marketdata-sidecar",
  );
  assert.deepEqual(commands[marketDataBuildIndex + 1], [
    "pnpm",
    ["run", "smoke:marketdata-sidecar"],
  ]);
  assert.equal(commands[marketDataBuildIndex + 2], undefined);
});

function countPnpmScript(commands, script) {
  return commands.filter(
    ([command, args]) => command === "pnpm" && args.join(" ") === `run ${script}`,
  ).length;
}

test("rejects unknown test layers", () => {
  assert.throws(() => commandsForLayer("unknown"), /unknown test layer/);
});

test("parallel checks buffer output in declaration order and report every failure", async () => {
  const stdout = outputBuffer();
  const stderr = outputBuffer();
  const sequentialCommands = [];
  const first = ["check", ["first"]];
  const second = ["check", ["second"]];
  const after = ["check", ["after"]];

  const status = await runExecutionStages([
    { mode: "parallel", commands: [first, second] },
    { mode: "sequential", commands: [after] },
  ], {
    stdout,
    stderr,
    runSequential: async (command) => {
      sequentialCommands.push(command);
      return 0;
    },
    runParallel: async ([, [name]]) => {
      if (name === "first") {
        return { status: 7, stdout: "first stdout\n", stderr: "first stderr\n" };
      }
      throw new Error("second runner rejected");
    },
  });

  assert.equal(status, 7);
  assert.deepEqual(sequentialCommands, []);
  assert.ok(stdout.value.indexOf("first stdout") < stdout.value.lastIndexOf("> check second"));
  assert.match(stderr.value, /first stderr/);
  assert.match(stderr.value, /second runner rejected/);
  assert.match(stderr.value, /check first \(exit 7\)/);
  assert.match(stderr.value, /check second \(exit 1\)/);
});

test("successful parallel checks preserve the later sequential dependency order", async () => {
  const events = [];
  const completions = new Map();
  const before = ["check", ["before"]];
  const parallel = [["check", ["alpha"]], ["check", ["beta"]]];
  const after = ["check", ["after"]];

  const execution = runExecutionStages([
    { mode: "sequential", commands: [before] },
    { mode: "parallel", commands: parallel },
    { mode: "sequential", commands: [after] },
  ], {
    stdout: outputBuffer(),
    stderr: outputBuffer(),
    runSequential: async ([, [name]]) => {
      events.push(name);
      return 0;
    },
    runParallel: ([, [name]]) => new Promise((complete) => {
      events.push(name);
      completions.set(name, () => complete({
        status: 0,
        stdout: `${name} complete\n`,
        stderr: "",
      }));
    }),
  });

  await Promise.resolve();
  assert.deepEqual(events, ["before", "alpha", "beta"]);
  completions.get("beta")();
  await Promise.resolve();
  assert.deepEqual(events, ["before", "alpha", "beta"]);
  completions.get("alpha")();
  const status = await execution;

  assert.equal(status, 0);
  assert.deepEqual(events, ["before", "alpha", "beta", "after"]);
});

function outputBuffer() {
  return {
    value: "",
    write(chunk) {
      this.value += String(chunk);
    },
  };
}
