import assert from "node:assert/strict";
import test from "node:test";

import { commandsForLayer, preflightChecks } from "./run-test-layer.mjs";

const generateDocs = ["pnpm", ["run", "generate:docs"]];

test("preflight generates docs before running the shared checks", () => {
  const commands = commandsForLayer("preflight");

  assert.deepEqual(commands[0], generateDocs);
  assert.deepEqual(commands.slice(1), preflightChecks);
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
});

test("rejects unknown test layers", () => {
  assert.throws(() => commandsForLayer("unknown"), /unknown test layer/);
});
