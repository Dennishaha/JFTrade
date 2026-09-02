import assert from "node:assert/strict";
import test from "node:test";

import { countCodegenObjects, codegenObjectLimit } from "./check-rust-target-health.mjs";

function fakeFileSystem(names) {
  let index = 0;
  let closed = false;
  return {
    existsSync: () => true,
    opendirSync: () => ({
      readSync: () => {
        if (index >= names.length) return null;
        const name = names[index++];
        return { isFile: () => true, name };
      },
      closeSync: () => { closed = true; },
    }),
    state: () => ({ closed, reads: index }),
  };
}

test("target health scan stops as soon as the codegen-object limit is reached", () => {
  const names = Array.from({ length: codegenObjectLimit + 50 }, (_, index) => `crate-${index}.rcgu.o`);
  const fileSystem = fakeFileSystem(names);
  assert.equal(countCodegenObjects("/target/debug/deps", codegenObjectLimit, fileSystem), codegenObjectLimit);
  assert.deepEqual(fileSystem.state(), { closed: true, reads: codegenObjectLimit });
});

test("target health ignores non-codegen Cargo artifacts and missing directories", () => {
  const fileSystem = fakeFileSystem(["libcrate.rlib", "libcrate.rmeta", "crate.d"]);
  assert.equal(countCodegenObjects("/target/debug/deps", codegenObjectLimit, fileSystem), 0);
  assert.equal(countCodegenObjects("/missing", codegenObjectLimit, {
    existsSync: () => false,
  }), 0);
});
