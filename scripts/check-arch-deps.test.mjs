import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
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

test("only Rule 16 and Rule 16a opt into package-family matching", async () => {
  const source = await readFile(scriptPath, "utf8");
  const executable = source.slice(source.indexOf("arch_deps_main()"));
  assert.equal((executable.match(/\bcheck_no_import_family\b/g) ?? []).length, 1);
  assert.equal((executable.match(/\bcheck_no_test_import_family\b/g) ?? []).length, 1);
});
