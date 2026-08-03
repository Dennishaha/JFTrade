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

const generateDocs = ["pnpm", ["run", "generate:docs"]];

test("preflight generates docs before running the shared checks", () => {
  const commands = commandsForLayer("preflight");
  const stages = executionStagesForLayer("preflight");

  assert.deepEqual(commands[0], generateDocs);
  assert.deepEqual(commands.slice(1), preflightChecks);
  assert.deepEqual(stages, [
    { mode: "sequential", commands: [generateDocs] },
    { mode: "parallel", commands: parallelPreflightChecks },
    { mode: "sequential", commands: sequentialPreflightChecks },
  ]);
  assert.equal(parallelPreflightChecks.length, 11);
});

test("ci-local generates docs once, checks drift, then runs shared checks inline", () => {
  const commands = commandsForLayer("ci-local");
  const generated = commands.filter(
    ([command, args]) => command === "pnpm" && args.join(" ") === "run generate:docs",
  );
  const diffIndex = commands.findIndex(([command]) => command === "git");
  const firstCheckIndex = commands.findIndex(
    ([command, args]) =>
      command === preflightChecks[0][0] &&
      args.join(" ") === preflightChecks[0][1].join(" "),
  );

  assert.deepEqual(generated, [generateDocs]);
  assert.equal(diffIndex, 1);
  assert.deepEqual(commands.slice(2, 4), [
    ["pnpm", ["run", "audit:dependencies"]],
    ["pnpm", ["run", "check:oss-license"]],
  ]);
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
  const frontendBuildIndex = commands.findIndex(
    ([command, args]) => command === "pnpm" && args.join(" ") === "run build:frontend-assets:generated",
  );
  assert.deepEqual(commands[frontendBuildIndex + 1], ["node", ["scripts/report-web-bundle.mjs"]]);
  const yfinanceBuildIndex = commands.findIndex(
    ([command, args]) =>
      command === "pnpm" && args.join(" ") === "run build:yfinance-sidecar",
  );
  assert.deepEqual(commands[yfinanceBuildIndex + 1], [
    "go",
    ["test", "-tags", "release_assets", "./internal/yfinanceassets", "-count=1"],
  ]);
});

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
