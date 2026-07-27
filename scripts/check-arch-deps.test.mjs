import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import test from "node:test";

const scriptPath = new URL("./check-arch-deps.sh", import.meta.url);

function familyMatch(imports, forbidden) {
  const command = [
    `source "${scriptPath.pathname}"`,
    "imports_contain_family \"$1\" \"$2\"",
  ].join("; ");
  return spawnSync("bash", ["-c", command, "arch-deps-test", imports, forbidden], {
    encoding: "utf8",
  });
}

test("package-family matching includes the root and descendants only", () => {
  const forbidden = "github.com/jftrade/jftrade-main/pkg/futu";
  assert.equal(familyMatch(forbidden, forbidden).status, 0);
  assert.equal(familyMatch(`${forbidden}/opend`, forbidden).status, 0);
  assert.equal(familyMatch(`${forbidden}/pb/common`, forbidden).status, 0);
  assert.equal(familyMatch(`${forbidden}-mock`, forbidden).status, 1);
  assert.equal(familyMatch(`${forbidden}x/opend`, forbidden).status, 1);
});

test("package-family matching scans complete import lists", () => {
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
