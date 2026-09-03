import assert from "node:assert/strict";
import test from "node:test";

import {
  commandsForLayer,
  contractChecks,
  executionStagesForLayer,
  policyChecks,
  runExecutionStages,
} from "./run-test-layer.mjs";

test("policy and contract layers contain only their permanent product gates", () => {
  assert.deepEqual(executionStagesForLayer("policy"), [{ mode: "parallel", commands: policyChecks }]);
  assert.deepEqual(executionStagesForLayer("contracts"), [{ mode: "parallel", commands: contractChecks }]);
  assert.equal(commandStrings(commandsForLayer("policy")).some((value) => /stage[2-9]|differential|go-retirement/i.test(value)), false);
});

test("preflight fans independent product lanes out in parallel", () => {
  const stages = executionStagesForLayer("preflight");
  assert.deepEqual(stages.map(({ mode }) => mode), ["parallel", "parallel"]);
  assert.deepEqual(commandStrings(stages[0].commands), ["pnpm run check:policy", "pnpm run check:contracts"]);
  assert.deepEqual(commandStrings(stages[1].commands), [
    "pnpm run check:rust:static",
    "pnpm run check:rust:workspace",
    "pnpm run check:web",
    "pnpm run check:pine",
    "pnpm run check:python",
  ]);
});

test("main is complete non-recursive and executes the Rust workspace exactly once", () => {
  const commands = commandStrings(commandsForLayer("main"));
  assert.equal(commands.filter((value) => value === "pnpm run check:rust:workspace").length, 1);
  assert.equal(commands.filter((value) => value === "pnpm run check:compatibility").length, 0);
  assert.equal(commands.some((value) => value === "pnpm run check:all"), false);
  assert.ok(commands.includes("pnpm run check:desktop"));
  assert.ok(commands.includes("pnpm run test:scripts -- release desktop"));
  assert.ok(commands.includes("pnpm run smoke:pinets-backtest"));
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
    runSequential: async (command) => { sequentialCommands.push(command); return 0; },
    runParallel: async ([, [name]]) => {
      if (name === "first") return { status: 7, stdout: "first stdout\n", stderr: "first stderr\n" };
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

test("successful parallel checks preserve later sequential dependency order", async () => {
  const events = [];
  const completions = new Map();
  const execution = runExecutionStages([
    { mode: "sequential", commands: [["check", ["before"]]] },
    { mode: "parallel", commands: [["check", ["alpha"]], ["check", ["beta"]]] },
    { mode: "sequential", commands: [["check", ["after"]]] },
  ], {
    stdout: outputBuffer(),
    stderr: outputBuffer(),
    runSequential: async ([, [name]]) => { events.push(name); return 0; },
    runParallel: ([, [name]]) => new Promise((complete) => {
      events.push(name);
      completions.set(name, () => complete({ status: 0, stdout: `${name} complete\n`, stderr: "" }));
    }),
  });
  await Promise.resolve();
  assert.deepEqual(events, ["before", "alpha", "beta"]);
  completions.get("beta")();
  await Promise.resolve();
  assert.deepEqual(events, ["before", "alpha", "beta"]);
  completions.get("alpha")();
  assert.equal(await execution, 0);
  assert.deepEqual(events, ["before", "alpha", "beta", "after"]);
});

function commandStrings(commands) {
  return commands.map(([command, args]) => `${command} ${args.join(" ")}`);
}

function outputBuffer() {
  return { value: "", write(chunk) { this.value += String(chunk); } };
}
