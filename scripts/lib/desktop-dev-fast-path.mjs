export function nativeBundleCacheReusable({
  executableAvailable,
  fingerprint,
  signatureValid,
  storedFingerprint,
}) {
  return Boolean(
    executableAvailable &&
      fingerprint &&
      storedFingerprint === fingerprint &&
      signatureValid,
  );
}

export function selectMarketDataDevelopmentRuntime(options) {
  if (options.explicitHelper) {
    if (!options.explicitHelperUsable) {
      throw new Error("Configured market-data helper is unusable");
    }
    return { kind: "explicit-helper", executable: options.explicitHelper };
  }
  if (options.explicitPython || options.explicitSource) {
    if (!options.explicitPython || !options.explicitSource) {
      throw new Error(
        "JFTRADE_MARKETDATA_DEV_PYTHON and JFTRADE_MARKETDATA_DEV_PYTHONPATH must be set together",
      );
    }
    if (!options.explicitPythonUsable) {
      throw new Error("Configured market-data Python source runtime is unusable");
    }
    return {
      kind: "python-source",
      python: options.explicitPython,
      source: options.explicitSource,
    };
  }
  if (options.defaultPythonUsable) {
    return {
      kind: "python-source",
      python: options.defaultPython,
      source: options.defaultSource,
    };
  }
  if (options.frozenAvailable) {
    return { kind: "frozen-helper", executable: options.frozenHelper };
  }
  if (options.allowUnavailable) {
    return { kind: "unavailable" };
  }
  throw new Error(
    "No usable market-data development runtime. Run: python -m pip install --editable \"workers/marketdata-sidecar[runtime,build,test]\"",
  );
}
