import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const scriptPath = fileURLToPath(new URL("./check-arch-deps.sh", import.meta.url))
  .replaceAll("\\", "/");
const bashAvailable = spawnSync("bash", ["-c", "exit 0"], {
  stdio: "ignore",
}).status === 0;

function familyMatch(imports, forbidden) {
  const command = [
    `source "${scriptPath}"`,
    "imports_contain_family \"$(cat)\" \"$1\"",
  ].join("; ");
  return spawnSync("bash", ["-c", command, "arch-deps-test", forbidden], {
    input: imports,
    encoding: "utf8",
  });
}

function checkTopLevelDirectoryAllowlist(root, allowed) {
  const command = [
    `source "${scriptPath}"`,
    "PASS=0",
    "FAIL=0",
    "WARN=0",
    "check_top_level_directory_allowlist \"$1\" \"test public package set\" \"${@:2}\"",
    "test \"$FAIL\" -eq 0",
  ].join("; ");
  return spawnSync("bash", ["-c", command, "arch-deps-test", root, ...allowed], {
    encoding: "utf8",
  });
}

test("package-family matching includes the root and descendants only", (t) => {
  if (!bashAvailable) {
    t.skip("bash is unavailable in this environment");
    return;
  }
  const forbidden = "github.com/jftrade/jftrade-main/pkg/futu";
  assert.equal(familyMatch(forbidden, forbidden).status, 0);
  assert.equal(familyMatch(`${forbidden}/opend`, forbidden).status, 0);
  assert.equal(familyMatch(`${forbidden}/pb/common`, forbidden).status, 0);
  assert.equal(familyMatch(`${forbidden}-mock`, forbidden).status, 1);
  assert.equal(familyMatch(`${forbidden}x/opend`, forbidden).status, 1);
});

test("package-family matching scans complete import lists", (t) => {
  if (!bashAvailable) {
    t.skip("bash is unavailable in this environment");
    return;
  }
  const forbidden = "github.com/jftrade/jftrade-main/pkg/futu";
  const imports = [
    "context",
    "github.com/jftrade/jftrade-main/internal/live",
    `${forbidden}/codec`,
  ].join("\n");
  assert.equal(familyMatch(imports, forbidden).status, 0);
});

test("public package allowlist is exact and detects growth or stale entries", async (t) => {
  if (!bashAvailable) {
    t.skip("bash is unavailable in this environment");
    return;
  }

  const root = await mkdtemp(join(tmpdir(), "jftrade-pkg-allowlist-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  await Promise.all([mkdir(join(root, "alpha")), mkdir(join(root, "beta"))]);

  assert.equal(checkTopLevelDirectoryAllowlist(root, ["alpha", "beta"]).status, 0);
  assert.notEqual(checkTopLevelDirectoryAllowlist(root, ["alpha"]).status, 0);
  assert.notEqual(checkTopLevelDirectoryAllowlist(root, ["alpha", "beta", "gamma"]).status, 0);
});

test("assistant and servercore boundaries opt into package-family matching", async () => {
  const source = await readFile(scriptPath, "utf8");
  const executable = source.slice(source.indexOf("arch_deps_main()"));
  assert.equal((executable.match(/\bcheck_no_import_family\b/g) ?? []).length, 1);
  assert.equal((executable.match(/\bcheck_no_test_import_family\b/g) ?? []).length, 1);
  assert.equal(
    (executable.match(/\bcheck_package_set_no_import_family_except\b/g) ?? []).length,
    4,
  );
  assert.equal(
    (executable.match(/\bcheck_import_family_allowlist\b/g) ?? []).length,
    2,
  );
});
