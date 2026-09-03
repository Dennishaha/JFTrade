import assert from "node:assert/strict";
import test from "node:test";

import { buildClippyArguments, normalizePnpmArguments } from "./check-clippy.mjs";

test("normalizePnpmArguments removes pnpm argument separator", () => {
  assert.deepEqual(normalizePnpmArguments(["--", "-p", "jftrade-desktop"]), ["-p", "jftrade-desktop"]);
  assert.deepEqual(normalizePnpmArguments(["-p", "jftrade-desktop"]), ["-p", "jftrade-desktop"]);
  assert.deepEqual(normalizePnpmArguments([]), []);
});

test("buildClippyArguments defaults to workspace and strictly enforces locked flags", () => {
  assert.deepEqual(buildClippyArguments([]), [
    "--workspace",
    "--all-targets",
    "--all-features",
    "--locked",
    "--",
    "-D",
    "warnings",
  ]);
});

test("buildClippyArguments preserves package selection while retaining locked flags and deny warnings", () => {
  assert.deepEqual(buildClippyArguments(["--", "-p", "jftrade-desktop"]), [
    "-p",
    "jftrade-desktop",
    "--all-targets",
    "--all-features",
    "--locked",
    "--",
    "-D",
    "warnings",
  ]);

  assert.deepEqual(buildClippyArguments(["-p", "crate-a", "-p", "crate-b"]), [
    "-p",
    "crate-a",
    "-p",
    "crate-b",
    "--all-targets",
    "--all-features",
    "--locked",
    "--",
    "-D",
    "warnings",
  ]);
});

test("buildClippyArguments preserves additional clippy options after separator", () => {
  assert.deepEqual(buildClippyArguments(["-p", "jftrade-desktop", "--", "-W", "clippy::pedantic"]), [
    "-p",
    "jftrade-desktop",
    "--all-targets",
    "--all-features",
    "--locked",
    "--",
    "-D",
    "warnings",
    "-W",
    "clippy::pedantic",
  ]);
});
