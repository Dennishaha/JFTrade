import { describe, expect, it } from "vitest";

import {
  dependencyPathLabel,
  dependencySourceLabel,
  dependencyStatusClass,
  dependencyStatusLabel,
  dependencyVersionLabel,
  mapRuntimeDependencies,
} from "@/composables/settings/runtimeDependencyPresentation";

describe("runtimeDependencyPresentation", () => {
  it("normalizes sparse dependency payloads", () => {
    expect(mapRuntimeDependencies({})).toEqual({
      checkedAt: "",
      allRequiredSatisfied: false,
      dependencies: [],
    });
    expect(
      mapRuntimeDependencies({ dependencies: [{ id: "python" }, {}] }),
    ).toEqual({
      checkedAt: "",
      allRequiredSatisfied: false,
      dependencies: [
        {
          id: "python",
          displayName: "python",
          required: false,
          configurable: false,
          status: "error",
          minimumVersion: "",
          detectedVersion: "",
          configuredPath: "",
          effectivePath: "",
          resolvedPath: "",
          source: "",
          homepageUrl: "",
          message: "",
        },
        {
          id: "",
          displayName: "",
          required: false,
          configurable: false,
          status: "error",
          minimumVersion: "",
          detectedVersion: "",
          configuredPath: "",
          effectivePath: "",
          resolvedPath: "",
          source: "",
          homepageUrl: "",
          message: "",
        },
      ],
    });
  });

  it.each([
    ["ok", "可用", "status-ok"],
    ["MISSING", "缺失", "status-warning"],
    ["outdated", "版本过低", "status-warning"],
    ["probe-error", "异常", "status-error"],
  ])("formats the %s dependency status", (status, label, className) => {
    expect(dependencyStatusLabel(status)).toBe(label);
    expect(dependencyStatusClass(status)).toBe(className);
  });

  it.each([
    ["settings", "设置"],
    ["path", "PATH"],
    ["bundled", "应用内嵌"],
    ["external-helper", "Frozen helper"],
    ["workspace-venv", "项目虚拟环境"],
    ["env:PYTHON_HOME", "环境变量 PYTHON_HOME"],
    ["custom", "custom"],
    ["", "-"],
  ])("formats the %s dependency source", (source, label) => {
    expect(dependencySourceLabel(source)).toBe(label);
  });

  it("formats empty and detected versions and paths", () => {
    expect(dependencyVersionLabel("  ")).toBe("-");
    expect(dependencyVersionLabel("3.12.1")).toBe("3.12.1");
    expect(dependencyPathLabel("  ")).toBe("自动检测");
    expect(dependencyPathLabel("/usr/bin/python3")).toBe("/usr/bin/python3");
  });
});
