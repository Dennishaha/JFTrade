import assert from "node:assert/strict";
import {
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { materializeDirectorySymlinks } from "./materialize-directory-symlinks.mjs";

test(
  "materializes internal file and directory symlinks as regular bundle entries",
  { skip: process.platform === "win32" },
  () => {
    const temp = mkdtempSync(join(tmpdir(), "jftrade-bundle-links-"));
    try {
      const root = join(temp, "bundle");
      mkdirSync(join(root, "runtime", "version"), { recursive: true });
      writeFileSync(join(root, "runtime", "version", "Python"), "runtime");
      symlinkSync("version/Python", join(root, "runtime", "Python"));
      symlinkSync("version", join(root, "runtime", "Current"));

      assert.equal(materializeDirectorySymlinks(root), 2);
      assert.equal(lstatSync(join(root, "runtime", "Python")).isFile(), true);
      assert.equal(lstatSync(join(root, "runtime", "Current")).isDirectory(), true);
      assert.equal(
        readFileSync(join(root, "runtime", "Current", "Python"), "utf8"),
        "runtime",
      );
    } finally {
      rmSync(temp, { recursive: true, force: true });
    }
  },
);

test(
  "rejects bundle symlinks that escape the output directory",
  { skip: process.platform === "win32" },
  () => {
    const temp = mkdtempSync(join(tmpdir(), "jftrade-bundle-links-"));
    try {
      const root = join(temp, "bundle");
      mkdirSync(root);
      writeFileSync(join(temp, "outside"), "outside");
      symlinkSync(join(temp, "outside"), join(root, "unsafe"));

      assert.throws(
        () => materializeDirectorySymlinks(root),
        /resolves outside the bundle/,
      );
      assert.equal(existsSync(join(root, "unsafe")), true);
      assert.equal(lstatSync(join(root, "unsafe")).isSymbolicLink(), true);
    } finally {
      rmSync(temp, { recursive: true, force: true });
    }
  },
);
