# Pine v6 v3.0 Compatibility

v3.0 establishes a stable Pine v6 language-foundation payload while keeping JFTrade execution on the closed-bar `strategy(...)` path.

## Additions

- `SemanticDeclaration` now exposes stable `signature` and `unsupportedReason` fields while retaining the legacy `reason` field.
- Type, method, export, import, library, and collection declarations share a more consistent semantic metadata shape.
- Executable type and method declarations clear `unsupportedReason` once the compiler confirms they are part of the closed-bar runtime surface.
- `varip` is accepted with closed-bar `var` semantics and emits a compatibility warning.
- Existing whitespace and inline-comment parsing behavior is locked into the v3.0 corpus.

## Boundaries

- `library` and `import` remain metadata/diagnostic surfaces; JFTrade does not load external TradingView libraries.
- `varip` does not implement intrabar update semantics in the closed-bar runtime.
- Dynamic symbol/timeframe execution, nested `request.security`, `lookahead_on`, `gaps_on`, broker emulator parity, OCA, partial fill, and intrabar tick recalculation remain unsupported.

## Evidence

- `TestPineV30MigrationCorpusGate`: at least 2850 scripts, 620 runnable cases, and weighted score >= 99.95.
- Focused coverage includes semantic declaration signatures, unsupported reasons, import metadata, export metadata, and `varip` warning behavior.
