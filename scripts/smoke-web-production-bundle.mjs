#!/usr/bin/env node

import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFileSync } from "node:fs";
import { basename, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const repositoryRoot = resolve(import.meta.dirname, "..");
const requireFromWeb = createRequire(
  new URL("../apps/web/package.json", import.meta.url),
);
const { JSDOM } = requireFromWeb("jsdom");
const distRoot = resolve(
  process.env.JFTRADE_WEB_DIST ?? resolve(repositoryRoot, "apps/web/dist"),
);
const indexHTML = readFileSync(resolve(distRoot, "index.html"), "utf8");
const entry = indexHTML.match(/<script[^>]+type="module"[^>]+src="([^"]+)"/)?.[1];

assert.ok(entry, "production frontend index.html has no module entry");

const entryPath = resolve(distRoot, entry.replace(/^\/+/, ""));
const dom = new JSDOM(indexHTML, {
  pretendToBeVisual: true,
  runScripts: "outside-only",
  url: "http://127.0.0.1:6699/",
});
const { window } = dom;
const fetchMock = async (input) => {
  return {
    ok: true,
    status: 200,
    headers: { get: () => "application/json" },
    json: async () => ({ ok: true, data: {} }),
    text: async () => JSON.stringify({ ok: true, data: {} }),
  };
};

window.__JFTRADE_RUNTIME_CONFIG__ = {
  authRequired: false,
  desktopMode: true,
};
window.__TAURI_INTERNALS__ = {
  invoke: () =>
    Promise.resolve({
      apiBaseUrl: "http://127.0.0.1:6699",
      authRequired: false,
      desktopMode: true,
      desktopApiToken: "a".repeat(64),
    }),
};
window.CSS = { escape: (value) => String(value) };
window.matchMedia = () => ({
  addEventListener: () => {},
  addListener: () => {},
  matches: false,
  removeEventListener: () => {},
  removeListener: () => {},
});
window.fetch = fetchMock;
window.ResizeObserver = class {
  disconnect() {}
  observe() {}
  unobserve() {}
};
window.IntersectionObserver = class {
  disconnect() {}
  observe() {}
  unobserve() {}
};
window.WebSocket = class {
  addEventListener() {}
  removeEventListener() {}
  close() {}
  send() {}
};

const globals = {
  CSS: window.CSS,
  CustomEvent: window.CustomEvent,
  Element: window.Element,
  Event: window.Event,
  EventTarget: window.EventTarget,
  FocusEvent: window.FocusEvent,
  HTMLElement: window.HTMLElement,
  IntersectionObserver: window.IntersectionObserver,
  KeyboardEvent: window.KeyboardEvent,
  MouseEvent: window.MouseEvent,
  MutationObserver: window.MutationObserver,
  Node: window.Node,
  PointerEvent: window.PointerEvent,
  ResizeObserver: window.ResizeObserver,
  SVGElement: window.SVGElement,
  TouchEvent: window.TouchEvent,
  WebSocket: window.WebSocket,
  WheelEvent: window.WheelEvent,
  cancelAnimationFrame: window.cancelAnimationFrame.bind(window),
  crypto: window.crypto ?? globalThis.crypto,
  document: window.document,
  fetch: fetchMock,
  getComputedStyle: window.getComputedStyle.bind(window),
  history: window.history,
  location: window.location,
  localStorage: window.localStorage,
  matchMedia: window.matchMedia,
  navigator: window.navigator,
  requestAnimationFrame: window.requestAnimationFrame.bind(window),
  sessionStorage: window.sessionStorage,
  window,
};
const previousGlobals = new Map();
for (const [name, value] of Object.entries(globals)) {
  previousGlobals.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
  Object.defineProperty(globalThis, name, {
    configurable: true,
    value,
    writable: true,
  });
}

const runtimeErrors = [];
const recordRuntimeError = (event) => {
  runtimeErrors.push(String(event.error?.stack ?? event.reason ?? event.message));
};
window.addEventListener("error", recordRuntimeError);
window.addEventListener("unhandledrejection", recordRuntimeError);

try {
  try {
    await import(`${pathToFileURL(entryPath).href}?jftrade-production-smoke=1`);
  } catch (error) {
    const message = error instanceof Error ? error.stack ?? error.message : String(error);
    if (/reading ['"]Call['"]/.test(message)) {
      throw new Error(
        `production frontend bundle still has a desktop runtime chunk cycle: ${message}`,
        { cause: error },
      );
    }
    throw new Error(`production frontend bundle failed before Vue mounted: ${message}`, {
      cause: error,
    });
  }

  await new Promise((resolvePromise) => setTimeout(resolvePromise, 0));
  assert.deepEqual(
    runtimeErrors,
    [],
    `production frontend bundle emitted runtime errors:\n${runtimeErrors.join("\n")}`,
  );

  const appRoot = window.document.querySelector("#app");
  assert.ok(appRoot, "production frontend bundle did not render #app");
  assert.ok(
    appRoot.hasAttribute("data-v-app"),
    "production frontend bundle did not mount Vue on #app",
  );
  assert.match(
    appRoot.textContent ?? "",
    /JFTrade/,
    "production frontend bundle rendered no startup UI",
  );
  console.log(`Production frontend bundle smoke passed (${basename(entryPath)}).`);
} finally {
  window.removeEventListener("error", recordRuntimeError);
  window.removeEventListener("unhandledrejection", recordRuntimeError);
  for (const [name, descriptor] of previousGlobals) {
    if (descriptor) {
      Object.defineProperty(globalThis, name, descriptor);
    } else {
      delete globalThis[name];
    }
  }
  dom.window.close();
}

process.exit(0);
