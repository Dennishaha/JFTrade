import assert from "node:assert/strict";
import test from "node:test";

import { compareBudget, inspectServercore } from "./check-servercore-budget.mjs";

const futu = "github.com/jftrade/jftrade-main/pkg/futu";

test("counts production/test lines, shell/application methods, and direct-import files", () => {
  const actual = inspectServercore([
    {
      name: "server.go",
      contents:
        `package sample\nimport "${futu}"\n` +
        "func (s *Server) Start() {}\n" +
        "func (a serverApplication) Configure() {}\n",
    },
    { name: "other.go", contents: "package sample\n" },
  ], [futu], [
    { name: "server_test.go", contents: "package sample\n\nfunc TestStart(t *testing.T) {}\n" },
  ]);
  assert.deepEqual(actual, {
    productionLines: 5,
    testLines: 3,
    serverMethods: 1,
    applicationMethods: 1,
    effectiveServerMethods: 2,
    serverFields: 0,
    applicationFields: 0,
    aggregateFields: 0,
    directImportFiles: { [futu]: ["server.go"] },
  });
});

test("counts embedded and named aggregate fields", () => {
  const actual = inspectServercore([
    {
      name: "server.go",
      contents: `package sample
type Server struct {
  serverApplication
  router any
}
type serverApplication struct {
  store any
  first, second int
  nested struct {
    ignored any
  }
}
`,
    },
  ], []);
  assert.equal(actual.serverFields, 2);
  assert.equal(actual.applicationFields, 4);
  assert.equal(actual.aggregateFields, 6);
});

test("allows every budget dimension to shrink", () => {
  const actual = {
    productionLines: 9,
    testLines: 8,
    serverMethods: 1,
    applicationMethods: 2,
    effectiveServerMethods: 3,
    aggregateFields: 4,
    directImportFiles: { [futu]: [] },
  };
  const budget = {
    productionLinesMax: 10,
    testLinesMax: 10,
    serverMethodsMax: 2,
    applicationMethodsMax: 3,
    effectiveServerMethodsMax: 5,
    aggregateFieldsMax: 6,
    directImportFiles: { [futu]: ["server.go"] },
  };
  assert.deepEqual(compareBudget(actual, budget), []);
});

test("reports line, shell/application method, and direct-import file-set growth", () => {
  const actual = {
    productionLines: 11,
    testLines: 12,
    serverMethods: 3,
    applicationMethods: 4,
    effectiveServerMethods: 7,
    aggregateFields: 8,
    directImportFiles: { [futu]: ["new.go"] },
  };
  const budget = {
    productionLinesMax: 10,
    testLinesMax: 11,
    serverMethodsMax: 2,
    applicationMethodsMax: 3,
    effectiveServerMethodsMax: 6,
    aggregateFieldsMax: 7,
    directImportFiles: { [futu]: ["server.go"] },
  };
  assert.deepEqual(compareBudget(actual, budget), [
    "production lines 11 exceed budget 10",
    "test lines 12 exceed budget 11",
    "*Server methods 3 exceed budget 2",
    "serverApplication methods 4 exceed budget 3",
    "effective *Server method surface 7 exceed budget 6",
    "aggregate Server fields 8 exceed budget 7",
    `${futu} direct-import file set grew: new.go`,
  ]);
});

test("fails closed when a required budget dimension is missing", () => {
  const actual = {
    productionLines: 0,
    testLines: 0,
    serverMethods: 0,
    applicationMethods: 0,
    effectiveServerMethods: 0,
    aggregateFields: 0,
    directImportFiles: {},
  };
  const budget = {
    productionLinesMax: 0,
    serverMethodsMax: 0,
    applicationMethodsMax: 0,
    aggregateFieldsMax: 0,
    directImportFiles: {},
  };
  assert.deepEqual(compareBudget(actual, budget), [
    "testLinesMax must be a non-negative integer",
    "effectiveServerMethodsMax must be a non-negative integer",
  ]);
});

test("rejects invalid test-line budgets", () => {
  const actual = {
    productionLines: 0,
    testLines: 0,
    serverMethods: 0,
    applicationMethods: 0,
    effectiveServerMethods: 0,
    aggregateFields: 0,
    directImportFiles: {},
  };
  const baseBudget = {
    productionLinesMax: 0,
    serverMethodsMax: 0,
    applicationMethodsMax: 0,
    effectiveServerMethodsMax: 0,
    aggregateFieldsMax: 0,
    directImportFiles: {},
  };

  for (const testLinesMax of [-1, 1.5, "10"]) {
    assert.deepEqual(compareBudget(actual, { ...baseBudget, testLinesMax }), [
      "testLinesMax must be a non-negative integer",
    ]);
  }
});
