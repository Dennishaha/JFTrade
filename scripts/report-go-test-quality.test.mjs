import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const scriptPath = fileURLToPath(new URL("./report-go-test-quality.mjs", import.meta.url));

test("reports legacy assertion gaps without failing", (t) => {
  const repo = temporaryRepository(t);
  write(repo, "sample/legacy_test.go", effectOnlyTest("TestLegacy"));
  commitAll(repo, "legacy test");
  const base = git(repo, ["rev-parse", "HEAD"]).trim();

  const result = runPolicy(repo, base);

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /TestLegacy \[legacy\]/);
  assert.match(result.stdout, /no new unexempted assertion gaps/);
});

test("fails when a new test only calls collaborators", (t) => {
  const repo = temporaryRepository(t);
  write(repo, "sample/asserted_test.go", assertedTest("TestExisting"));
  commitAll(repo, "asserted test");
  const base = git(repo, ["rev-parse", "HEAD"]).trim();
  write(repo, "sample/effect_test.go", effectOnlyTest("TestNewEffect"));

  const result = runPolicy(repo, base);

  assert.equal(result.status, 1);
  assert.match(result.stdout, /TestNewEffect \[new\]/);
  assert.match(result.stderr, /New Go tests must assert a business result/);
});

test("accepts a new effect-only test with a specific exemption", (t) => {
  const repo = temporaryRepository(t);
  write(repo, "sample/asserted_test.go", assertedTest("TestExisting"));
  commitAll(repo, "asserted test");
  const base = git(repo, ["rev-parse", "HEAD"]).trim();
  write(repo, "sample/effect_test.go", effectOnlyTest("TestProcessExit"));
  write(
    repo,
    "scripts/go-test-quality-exemptions.json",
    `${JSON.stringify({
      exemptions: [{
        path: "sample/effect_test.go",
        test: "TestProcessExit",
        reason: "The subprocess exit status is the observable contract.",
      }],
    }, null, 2)}\n`,
  );

  const result = runPolicy(repo, base);

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /TestProcessExit \[exempt: The subprocess exit status/);
});

test("rejects stale assertion exemptions", (t) => {
  const repo = temporaryRepository(t);
  write(repo, "sample/asserted_test.go", assertedTest("TestAlreadyAsserted"));
  commitAll(repo, "asserted test");
  const base = git(repo, ["rev-parse", "HEAD"]).trim();
  write(
    repo,
    "scripts/go-test-quality-exemptions.json",
    `${JSON.stringify({
      exemptions: [{
        path: "sample/asserted_test.go",
        test: "TestAlreadyAsserted",
        reason: "This reason should be removed with the stale entry.",
      }],
    }, null, 2)}\n`,
  );

  const result = runPolicy(repo, base);

  assert.equal(result.status, 1);
  assert.match(result.stderr, /remove stale Go test assertion exemptions/);
});

function temporaryRepository(t) {
  const repo = mkdtempSync(join(tmpdir(), "jftrade-test-quality-"));
  t.after(() => rmSync(repo, { recursive: true, force: true }));
  git(repo, ["init", "-q"]);
  git(repo, ["config", "user.email", "test@example.com"]);
  git(repo, ["config", "user.name", "Test"]);
  write(repo, "go.mod", "module example.com/test-quality\n\ngo 1.26.0\n");
  return repo;
}

function effectOnlyTest(name) {
  return `package sample
import "testing"
func ${name}(t *testing.T) {
  t.Helper()
  publisher.Publish(event)
}
`;
}

function assertedTest(name) {
  return `package sample
import "testing"
func ${name}(t *testing.T) {
  if got != want {
    t.Fatalf("got %v, want %v", got, want)
  }
}
`;
}

function write(repo, path, contents) {
  const target = join(repo, path);
  mkdirSync(dirname(target), { recursive: true });
  writeFileSync(target, contents);
}

function commitAll(repo, message) {
  git(repo, ["add", "."]);
  git(repo, ["commit", "-q", "-m", message]);
}

function git(repo, args) {
  return execFileSync("git", args, {
    cwd: repo,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function runPolicy(repo, base) {
  return spawnSync(
    process.execPath,
    [scriptPath, "--repo-root", repo, "--base", base],
    {
      cwd: repo,
      encoding: "utf8",
      env: { ...process.env, JFTRADE_DIFF_BASE: "" },
    },
  );
}
