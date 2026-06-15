# Pine v6 v2.0 Language Foundation Roadmap

v2.0 starts the full Pine v6 language foundation for JFTrade while keeping trading execution on the closed-bar strategy runtime.

Status: complete. The release is locked by `TestPineV20LanguageFoundationGate` plus the Pine analyze API and frontend regression suites.

## Scope

- Parse and diagnose collection syntax for `array`, `map`, and `matrix`; runtime execution can arrive in staged subsets.
- Add minimal semantic models for `method`, `type`, `library`, and `import`.
- Permit statically expandable, side-effect-free subsets to lower into the current JFTrade runtime where practical.
- Upgrade visual APIs from warning/no-op toward optional metadata output, separate from trading execution.

## Current Foundation

- The AST now separates visual calls, collection namespaces, and declaration/import lines from generic unsupported statements.
- Collection assignments such as `arr = array.new_float(0)`, typed declarations such as `array<float> arr = na`, `map.new<string, float>()`, and `matrix.new<float>()` remain non-executable, but `AnalyzeScript` now preserves their AST kind/type annotation and emits semantic records with `name`, `namespace`, `call`, `typeArgs`, `executable=false`, and a parse-only `reason`.
- Collection calls also emit `collectionOperations` metadata with `namespace`, `operation`, `call`, `signature`, `supported`, `target`, `arguments`, `mutates`, and `executable=false`, covering constructors plus namespace and method-style operations such as `array.push(arr, x)`, `arr.push(x)`, `map.put`, and `matrix.set`; common operation arity mismatches produce `PINE_SEMANTIC_COLLECTION_SIGNATURE`.
- Typed collection declarations validate annotation and constructor compatibility. Namespace mismatches, invalid generic arity, and conflicting element/key/value types (including legacy constructors such as `array.new_float`) produce `PINE_SEMANTIC_COLLECTION_TYPE` without enabling runtime execution.
- The semantic summary includes non-executable declaration records for `type`, `method`, `import`, and `library(...)`, including the extracted declaration name, type fields, method receiver/parameters/defaults, import path/version/alias, and parse-only reason.
- Declaration semantics report `PINE_SEMANTIC_DECLARATION` for duplicate type names/fields, duplicate method signatures/parameter names, missing or unknown method receiver types, and duplicate import aliases, while still preserving the parse-only metadata payload. Distinct method overloads remain registered and object calls resolve an arity-compatible signature.
- User-defined type constructors and method calls such as `TradeBox.new(...)` and `box.reset(...)` now emit non-executable `objectOperations` metadata with `type`, `method`, `call`, `signature`, `target`, `arguments`, and parse-only execution flags when they can be resolved from prior `type`/`method` declarations; constructor/method arity mismatches produce `PINE_SEMANTIC_OBJECT_SIGNATURE`.
- The analyze API also exposes parse-only declaration and object operation records at the top-level `declarations` and `objectOperations` keys, mirroring top-level `visuals` for consumers that do not need the full semantic payload.
- Visual calls remain warning/no-op for trading execution, but `AnalyzeScript` now returns optional classified visual metadata (`line`, `kind`, `call`, `variable`, `target`, `title`, `arguments`, `namedArgs`, `text`) for consumers that want to render or inspect them outside the trading path, including assigned drawing/table constructors such as `lbl = label.new(...)`.
- Unsupported collection/declaration diagnostics are still emitted as errors, so the compiler does not silently approximate behavior that the closed-bar runtime cannot execute yet.

## Non-Goals

- Do not implement TradingView broker emulator parity in v2.0 language foundation work.
- Keep OCA, partial fill, tick recalculation, intrabar path inference, and order-fill simulation on a separate broker-emulator roadmap.
- Do not turn the frontend into a full TradingView Pine editor before the language and diagnostics base is reliable.

## Migration Principle

The compiler should continue to prefer explicit diagnostics over silent approximation. Unsupported language features should be parseable and explainable before they become executable.

## Completion Evidence

- `strategy.pine_spec` reports `productVersion=v2.0` and score model `closed-bar-strategy-v2.0`.
- Collection and declaration capabilities use the `analyzed` status: parser/semantic/API/frontend are available while planner/runtime remain disabled.
- `TestPineV20LanguageFoundationGate` covers typed array/map/matrix declarations and operations, type/method/import/library metadata, UDT constructor/method resolution, visual metadata, and the explicit broker-emulator boundary.
