// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import RuntimeDependenciesSection from "../src/components/RuntimeDependenciesSection.vue";
import { queryClient } from "../src/composables/serverState";
import { createResponse, flushRequests } from "./helpers";

beforeEach(() => {
  queryClient.clear();
});

afterEach(() => {
  queryClient.clear();
  vi.unstubAllGlobals();
});

describe("RuntimeDependenciesSection", () => {
  it("renders Node dependency state and saves a custom binary path", async () => {
    let dependencyChecks = 0;
    const fetchMock = vi.fn(
      async (input: string | URL | Request, init?: RequestInit) => {
        const url = String(input);
        const method = String(init?.method ?? "GET").toUpperCase();

        if (url.includes("/api/v1/settings/pine-worker") && method === "PUT") {
          expect(JSON.parse(String(init?.body))).toEqual({
            backtestWorkerLimit: 2,
            instanceWorkerLimit: 10,
            nodeBinaryPath: "/opt/node/bin/node",
          });
          return createResponse({
            backtestWorkerLimit: 2,
            instanceWorkerLimit: 10,
            nodeBinaryPath: "/opt/node/bin/node",
          });
        }

        if (url.includes("/api/v1/settings/pine-worker")) {
          return createResponse({
            backtestWorkerLimit: 2,
            instanceWorkerLimit: 10,
            nodeBinaryPath: "",
          });
        }

        if (url.includes("/api/v1/system/runtime-dependencies")) {
          dependencyChecks += 1;
          return createResponse({
            checkedAt: "2026-06-29T00:00:00Z",
            allRequiredSatisfied: dependencyChecks > 1,
            dependencies: [
              {
                id: "node",
                displayName: "Node.js",
                required: true,
                status: dependencyChecks > 1 ? "ok" : "missing",
                minimumVersion: "22.0.0",
                detectedVersion: dependencyChecks > 1 ? "22.1.0" : "",
                configuredPath:
                  dependencyChecks > 1 ? "/opt/node/bin/node" : "",
                effectivePath:
                  dependencyChecks > 1 ? "/opt/node/bin/node" : "node",
                resolvedPath:
                  dependencyChecks > 1 ? "/opt/node/bin/node" : "",
                source: dependencyChecks > 1 ? "settings" : "path",
                homepageUrl: "https://nodejs.org/",
                message:
                  dependencyChecks > 1
                    ? "Node.js 22.1.0 is available."
                    : "Node.js was not found on PATH.",
              },
            ],
          });
        }

        throw new Error(`Unexpected request: ${url}`);
      },
    );
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(RuntimeDependenciesSection, {
      props: { mode: "settings" },
    });
    await flushRequests();

    expect(wrapper.text()).toContain("依赖项管理");
    expect(wrapper.text()).toContain("1 个必需依赖需要处理");
    expect(wrapper.text()).toContain("打开 Node.js 网站");

    await wrapper
      .get("[data-testid='runtime-dependency-node-path-input']")
      .setValue("  /opt/node/bin/node  ");
    await wrapper
      .get("[data-testid='runtime-dependency-node-path-save']")
      .trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("必需依赖已满足");
    expect(wrapper.text()).toContain("Node.js 路径已保存");
    expect(dependencyChecks).toBe(2);
  });

  it("renders warning/error dependency labels and reports failed Node path saves", async () => {
    const fetchMock = vi.fn(
      async (input: string | URL | Request, init?: RequestInit) => {
        const url = String(input);
        const method = String(init?.method ?? "GET").toUpperCase();

        if (url.includes("/api/v1/settings/pine-worker") && method === "PUT") {
          throw new Error("save denied");
        }

        if (url.includes("/api/v1/settings/pine-worker")) {
          return createResponse({
            backtestWorkerLimit: 2,
            instanceWorkerLimit: 10,
            nodeBinaryPath: "",
          });
        }

        if (url.includes("/api/v1/system/runtime-dependencies")) {
          return createResponse({
            checkedAt: "2026-06-29T00:00:00Z",
            allRequiredSatisfied: false,
            dependencies: [
              {
                id: "node",
                displayName: "Node.js",
                required: true,
                status: "outdated",
                minimumVersion: "22.0.0",
                detectedVersion: "20.1.0",
                configuredPath: "",
                effectivePath: "/usr/local/bin/node",
                resolvedPath: "/usr/local/bin/node",
                source: "env:NODE_HOME",
                homepageUrl: "https://nodejs.org/",
                message: "Node.js version is too old.",
              },
              {
                id: "helper",
                displayName: "Worker Helper",
                required: false,
                status: "error",
                minimumVersion: "",
                detectedVersion: "",
                configuredPath: "",
                effectivePath: "",
                resolvedPath: "",
                source: "",
                homepageUrl: "https://example.com/helper",
                message: "Probe failed.",
              },
            ],
          });
        }

        throw new Error(`Unexpected request: ${url}`);
      },
    );
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(RuntimeDependenciesSection, {
      props: { mode: "settings" },
    });
    await flushRequests();

    expect(wrapper.text()).toContain("版本过低");
    expect(wrapper.text()).toContain("异常");
    expect(wrapper.text()).toContain("环境变量 NODE_HOME");
    expect(wrapper.text()).toContain("自动检测");

    await wrapper
      .get("[data-testid='runtime-dependency-node-path-input']")
      .setValue("/custom/node");
    await wrapper
      .get("[data-testid='runtime-dependency-node-path-save']")
      .trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("save denied");
  });
});
