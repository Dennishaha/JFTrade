#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

function run(command, args, timeoutMs = 300_000, extraEnv = {}) {
  const packageIndex = args.indexOf("-p");
  const normalizedArgs =
    command === "cargo" &&
    args[0] === "test" &&
    args[packageIndex + 1] === "jftrade-engine" &&
    !args.includes("--lib") &&
    !args.includes("--test")
      ? [...args.slice(0, packageIndex + 2), "--lib", ...args.slice(packageIndex + 2)]
      : args;
  const result = spawnSync(command, normalizedArgs, {
    cwd: repositoryRoot,
    encoding: "utf8",
    env: { ...process.env, ...extraEnv },
    stdio: ["ignore", "pipe", "pipe"],
    timeout: timeoutMs,
    killSignal: "SIGTERM",
  });
  if (result.error?.code === "ETIMEDOUT") throw new Error(`${command} timed out after ${timeoutMs}ms`);
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(
      `${command} ${normalizedArgs.join(" ")} failed:\n${result.stderr || result.stdout}`,
    );
  }
}

run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9AppearanceCorpusMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9ProviderDescriptorsMatchCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9BrokerDescriptorMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9ResearchScreenCatalogFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9SQLiteSchemaDefinitionsMatchCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9CalendarSourceProjectionFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9CalendarStatusFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9CalendarSnapshotFormatMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9CalendarControlFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9WatchlistMembershipsFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9WatchlistReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9PortfolioReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9ResearchReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9ResearchPresetReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9ExecutionReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9MarketDataProviderReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9MarketDataCatalogReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9MarketDataDerivativesReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9MarketDataOptionsReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9BrokerReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9PluginUninstallGuidanceFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9PluginsReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9AlertsReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9AlertsWriteFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9PluginsWriteFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./internal/app/apiserver/webaccess",
  "-run",
  "^TestStage9AuthSessionWriteFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9WatchlistsRemoteWriteFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9WatchlistWriteFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9BacktestsWriteFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9StrategyDefinitionsFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9WatchlistsReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9SystemReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9BacktestsReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9BacktestsSyncReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9StrategyInstanceReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9AuthSessionFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9MarketDataNewsActionsReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
for (const testName of [
  "^TestStage9ADKReadFixtureMatchesCurrentGoOwner$",
  "^TestStage9ADKChatStreamFixtureMatchesCurrentGoOwner$",
  "^TestStage9MarketDataProviderActionsFixtureMatchesCurrentGoOwner$",
  "^TestStage9MarketDataNewsSearchReadFixtureMatchesCurrentGoOwner$",
  "^TestStage9MarketDataQuoteReadFixtureMatchesCurrentGoOwner$",
  "^TestStage9MarketDataPredictionReadFixtureMatchesCurrentGoOwner$",
  "^TestStage9WSLiveFixtureMatchesCurrentGoOwner$",
  "^TestStage9StrategyPineFixtureMatchesCurrentGoOwner$",
]) {
  run("go", [
    "test",
    "./scripts/rust-migration",
    "-run",
    testName,
    "-count=1",
  ]);
}
const realTradeDirectory = mkdtempSync(join(tmpdir(), "jftrade-stage9-real-trade-"));
const realTradeReference = join(realTradeDirectory, "go-reference.json");
const brokerSettingsReference = join(realTradeDirectory, "go-broker-settings-reference.json");
const brokerSettingsWriteReference = join(realTradeDirectory, "go-broker-settings-write-reference.json");
const onboardingSettingsWriteReference = join(realTradeDirectory, "go-onboarding-settings-write-reference.json");
const providerSettingsWriteReference = join(realTradeDirectory, "go-provider-settings-write-reference.json");
const mcpSettingsWriteReference = join(realTradeDirectory, "go-mcp-settings-write-reference.json");
const securitySettingsWriteReference = join(realTradeDirectory, "go-security-settings-write-reference.json");
const assistantAgentTemplatesReference = join(realTradeDirectory, "go-assistant-agent-templates-reference.json");
const dataManagementRoot = join(realTradeDirectory, "data-management");
const dataManagementReference = join(dataManagementRoot, "go-reference.json");
const dataManagementCleanupRoot = join(realTradeDirectory, "data-management-cleanup");
const dataManagementCleanupReference = join(dataManagementCleanupRoot, "go-reference.json");
mkdirSync(dataManagementRoot, { recursive: true });
mkdirSync(dataManagementCleanupRoot, { recursive: true });
try {
  run(
    "go",
    [
      "test",
      "./scripts/rust-migration",
      "-run",
      "^TestStage9RealTradeReadReference$",
      "-count=1",
    ],
    300_000,
    { JFTRADE_STAGE9_REAL_TRADE_REFERENCE: realTradeReference },
  );
  run(
    "go",
    [
      "test",
      "./scripts/rust-migration",
      "-run",
      "^TestStage9BrokerSettingsReadReference$",
      "-count=1",
    ],
    300_000,
    { JFTRADE_STAGE9_BROKER_SETTINGS_REFERENCE: brokerSettingsReference },
  );
  run(
    "go",
    [
      "test",
      "./scripts/rust-migration",
      "-run",
      "^TestStage9BrokerSettingsWriteReference$",
      "-count=1",
    ],
    300_000,
    { JFTRADE_STAGE9_BROKER_SETTINGS_WRITE_REFERENCE: brokerSettingsWriteReference },
  );
  run(
    "go",
    [
      "test",
      "./scripts/rust-migration",
      "-run",
      "^TestStage9OnboardingSettingsWriteReference$",
      "-count=1",
    ],
    300_000,
    { JFTRADE_STAGE9_ONBOARDING_SETTINGS_WRITE_REFERENCE: onboardingSettingsWriteReference },
  );
  run(
    "go",
    [
      "test",
      "./scripts/rust-migration",
      "-run",
      "^TestStage9ProviderSettingsWriteReference$",
      "-count=1",
    ],
    300_000,
    { JFTRADE_STAGE9_PROVIDER_SETTINGS_WRITE_REFERENCE: providerSettingsWriteReference },
  );
  run(
    "go",
    [
      "test",
      "./scripts/rust-migration",
      "-run",
      "^TestStage9MCPSettingsWriteReference$",
      "-count=1",
    ],
    300_000,
    { JFTRADE_STAGE9_MCP_SETTINGS_WRITE_REFERENCE: mcpSettingsWriteReference },
  );
  run(
    "go",
    [
      "test",
      "./scripts/rust-migration",
      "-run",
      "^TestStage9SecuritySettingsWriteReference$",
      "-count=1",
    ],
    300_000,
    { JFTRADE_STAGE9_SECURITY_SETTINGS_WRITE_REFERENCE: securitySettingsWriteReference },
  );
  run(
    "go",
    [
      "test",
      "./scripts/rust-migration",
      "-run",
      "^TestStage9AssistantAgentTemplatesReference$",
      "-count=1",
    ],
    300_000,
    { JFTRADE_STAGE9_ASSISTANT_AGENT_TEMPLATES_REFERENCE: assistantAgentTemplatesReference },
  );
  run(
    "go",
    [
      "test",
      "./scripts/rust-migration",
      "-run",
      "^TestStage9DataManagementOverviewReference$",
      "-count=1",
    ],
    300_000,
    {
      JFTRADE_STAGE9_DATA_MANAGEMENT_ROOT: dataManagementRoot,
      JFTRADE_STAGE9_DATA_MANAGEMENT_REFERENCE: dataManagementReference,
    },
  );
  run(
    "go",
    [
      "test",
      "./scripts/rust-migration",
      "-run",
      "^TestStage9DataManagementCleanupPreviewReference$",
      "-count=1",
    ],
    300_000,
    {
      JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_ROOT: dataManagementCleanupRoot,
      JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_REFERENCE: dataManagementCleanupReference,
    },
  );
  run(
    "cargo",
    [
      "test",
      "-p",
      "jftrade-engine",
      "product::tests::stage9_broker_settings_writes_match_current_go_owner",
      "--",
      "--exact",
    ],
    300_000,
    { JFTRADE_STAGE9_BROKER_SETTINGS_WRITE_REFERENCE: brokerSettingsWriteReference },
  );
  run(
    "cargo",
    [
      "test",
      "-p",
      "jftrade-engine",
      "product::tests::stage9_onboarding_settings_writes_match_current_go_owner",
      "--",
      "--exact",
    ],
    300_000,
    { JFTRADE_STAGE9_ONBOARDING_SETTINGS_WRITE_REFERENCE: onboardingSettingsWriteReference },
  );
  run(
    "cargo",
    [
      "test",
      "-p",
      "jftrade-engine",
      "product::tests::stage9_provider_settings_writes_match_current_go_owner",
      "--",
      "--exact",
    ],
    300_000,
    { JFTRADE_STAGE9_PROVIDER_SETTINGS_WRITE_REFERENCE: providerSettingsWriteReference },
  );
  run(
    "cargo",
    [
      "test",
      "-p",
      "jftrade-store-settings-file",
      "stage9_mcp_settings_writes_match_current_go_owner",
      "--",
      "--exact",
    ],
    300_000,
    { JFTRADE_STAGE9_MCP_SETTINGS_WRITE_REFERENCE: mcpSettingsWriteReference },
  );
  run(
    "cargo",
    [
      "test",
      "-p",
      "jftrade-store-settings-file",
      "stage9_security_settings_writes_match_current_go_owner",
      "--",
      "--exact",
    ],
    300_000,
    { JFTRADE_STAGE9_SECURITY_SETTINGS_WRITE_REFERENCE: securitySettingsWriteReference },
  );
  run(
    "cargo",
    [
      "test",
      "-p",
      "jftrade-engine",
      "product::tests::stage9_assistant_agent_templates_match_current_go_owner",
      "--",
      "--exact",
    ],
    300_000,
    { JFTRADE_STAGE9_ASSISTANT_AGENT_TEMPLATES_REFERENCE: assistantAgentTemplatesReference },
  );
  run(
    "cargo",
    [
      "test",
      "-p",
      "jftrade-engine",
      "product_data_management::tests::stage9_data_management_overview_matches_current_go_owner",
      "--",
      "--exact",
    ],
    300_000,
    {
      JFTRADE_STAGE9_DATA_MANAGEMENT_ROOT: dataManagementRoot,
      JFTRADE_STAGE9_DATA_MANAGEMENT_REFERENCE: dataManagementReference,
    },
  );
  run(
    "cargo",
    [
      "test",
      "-p",
      "jftrade-engine",
      "product::tests::cleanup_preview_route_returns_candidates_and_rejects_bad_payloads",
      "--",
      "--exact",
    ],
  );
  run(
    "cargo",
    [
      "test",
      "-p",
      "jftrade-engine",
      "product_data_management::tests::stage9_data_management_cleanup_preview_matches_current_go_owner",
      "--",
      "--exact",
    ],
    300_000,
    {
      JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_ROOT: dataManagementCleanupRoot,
      JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_REFERENCE: dataManagementCleanupReference,
    },
  );
  run(
    "cargo",
    [
      "test",
      "-p",
      "jftrade-engine",
      "product::tests::stage9_broker_settings_reads_match_current_go_owner",
      "--",
      "--exact",
    ],
    300_000,
    { JFTRADE_STAGE9_BROKER_SETTINGS_REFERENCE: brokerSettingsReference },
  );
  run(
    "cargo",
    [
      "test",
      "-p",
      "jftrade-engine",
      "real_trade_control::tests::stage9_real_trade_reads_match_current_go_owner",
      "--",
      "--exact",
    ],
    300_000,
    { JFTRADE_STAGE9_REAL_TRADE_REFERENCE: realTradeReference },
  );
} finally {
  rmSync(realTradeDirectory, { recursive: true, force: true });
}
run("cargo", [
  "test",
  "-p",
  "jftrade-store-settings-file",
  "stage9_product_corpus_matches_go_and_preserves_unowned_fields",
  "--",
  "--exact",
]);
for (const testName of [
  "^TestStage9ResearchPresetsWriteFixtureMatchesCurrentGoOwner$",
  "^TestStage9StrategyDefinitionsWriteFixtureMatchesCurrentGoOwner$",
]) {
  run("go", [
    "test",
    "./scripts/rust-migration",
    "-run",
    testName,
    "-count=1",
  ]);
}
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "--test",
  "stage9_research_presets_write",
  "--",
  "--nocapture",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "--test",
  "stage9_strategy_definitions_write",
  "--",
  "--nocapture",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "--test",
  "stage9_auth_session_write",
  "--",
  "--nocapture",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "--test",
  "stage9_watchlists_remote_write",
  "--",
  "--nocapture",
]);
run("node", ["scripts/rust-migration/check-stage9-watchlist-write.mjs"]);
run("node", ["scripts/rust-migration/check-stage9-backtests-write.mjs"]);
for (const testName of [
  "product::tests::market_data_quote_read_tests::market_data_quote_read_routes_match_group_fixture_in_cutover_only",
  "product::tests::market_data_quote_read_tests::market_data_quote_read_routes_fail_closed_when_snapshot_is_unavailable",
  "product::tests::market_data_quote_read_tests::market_data_quote_read_routes_are_not_registered_without_snapshot_port",
  "product::tests::market_data_prediction_read_tests::market_data_prediction_read_routes_match_group_fixture_in_cutover_only",
  "product::tests::market_data_prediction_read_tests::market_data_prediction_read_routes_fail_closed_when_snapshot_is_unavailable",
  "product::tests::market_data_prediction_read_tests::market_data_prediction_read_routes_are_not_registered_without_snapshot_port",
]) {
  run("cargo", [
    "test",
    "-p",
    "jftrade-engine",
    testName,
    "--",
    "--exact",
  ]);
}
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::static_provider_catalog_matches_current_go_wire_fixture",
  "--",
  "--exact",
]);
for (const testName of [
  "product::tests::market_data_news_search_read_tests::market_data_news_search_read_route_matches_group_fixture_in_cutover_only",
  "product::tests::market_data_news_search_read_tests::market_data_news_search_read_route_fails_closed_when_snapshot_is_unavailable",
  "product::tests::market_data_news_search_read_tests::market_data_news_search_read_route_is_not_registered_without_snapshot_port",
  "product::tests::adk_read_tests::adk_read_routes_match_group_fixture_in_cutover_only",
  "product::tests::adk_read_tests::adk_read_routes_fail_closed_without_snapshot_port",
  "product::tests::adk_read_tests::adk_read_dynamic_routes_validate_suffixes_and_identifiers",
  "product::tests::adk_read_tests::adk_read_streams_preserve_event_ids_and_payloads",
  "product::tests::adk_chat_stream_product_tests::adk_chat_stream_routes_register_only_with_explicit_test_port",
  "product::tests::adk_chat_stream_product_tests::adk_chat_stream_routes_are_isolated_without_port",
  "product::tests::market_data_provider_actions_register_only_with_explicit_test_port",
  "product::tests::market_data_provider_actions_product_registers_with_explicit_test_port",
]) {
  run("cargo", [
    "test",
    "-p",
    "jftrade-engine",
    testName,
    "--",
    "--exact",
  ]);
}
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::ws_live_tests::ws_live_route_is_registered_only_with_explicit_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::strategy_pine_tests::strategy_pine_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::strategy_pine_tests::strategy_pine_routes_preserve_snapshot_failures_and_retry_after",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::strategy_pine_tests::strategy_pine_route_is_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_news_actions_read_tests::market_data_news_actions_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_news_actions_read_tests::market_data_news_actions_read_routes_fail_closed_when_snapshot_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_news_actions_read_tests::market_data_news_actions_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::auth_session_tests::auth_session_route_matches_go_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::auth_session_tests::auth_session_route_fails_closed_when_snapshot_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::auth_session_tests::auth_session_route_is_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::strategy_read_tests::strategy_instance_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::strategy_read_tests::strategy_instance_read_routes_fail_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::strategy_read_tests::strategy_instance_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::research_screen_catalog_route_matches_go_fixture_for_all_variants",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::calendar_control_plane_routes_share_the_real_manager_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::calendar_control_plane_routes_fail_closed_without_a_manager",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::calendar_unknown_market_control_requests_keep_the_go_noop_wire",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::watchlist_memberships_route_matches_go_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::watchlist_memberships_route_fails_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::watchlist_read_tests::watchlist_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::watchlist_read_tests::watchlist_read_routes_fail_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::watchlist_read_tests::watchlist_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::portfolio_tests::portfolio_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::portfolio_tests::portfolio_read_routes_fail_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::portfolio_tests::portfolio_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::research_read_tests::research_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::research_read_tests::research_read_routes_fail_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::research_read_tests::research_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::research_preset_read_tests::research_preset_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::research_preset_read_tests::research_preset_read_routes_fail_closed_and_keep_mutations_unregistered",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::research_preset_read_tests::research_preset_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::execution_read_tests::execution_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::execution_read_tests::execution_read_routes_fail_closed_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::execution_read_tests::execution_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_provider_read_tests::market_data_provider_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_provider_read_tests::market_data_provider_read_routes_fail_closed_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_provider_read_tests::market_data_provider_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_catalog_read_tests::market_data_catalog_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_catalog_read_tests::market_data_catalog_read_routes_fail_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_catalog_read_tests::market_data_catalog_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_derivative_read_tests::market_data_derivatives_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_derivative_read_tests::market_data_derivatives_read_routes_fail_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_derivative_read_tests::market_data_derivatives_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_options_read_tests::market_data_options_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_options_read_tests::market_data_options_read_routes_fail_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::market_data_options_read_tests::market_data_options_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::broker_read_tests::broker_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::broker_read_tests::broker_read_routes_fail_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::broker_read_tests::broker_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::plugin_uninstall_guidance_route_matches_go_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::plugin_uninstall_guidance_route_fails_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::plugin_tests::plugins_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::plugin_tests::plugins_read_routes_fail_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::plugin_tests::plugins_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::alerts_read_routes_match_go_fixture_as_cutover_only_batch",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::alerts_read_routes_fail_closed_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::alerts_write_product_tests::alerts_write_routes_register_only_with_explicit_test_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::alerts_write_product_tests::alerts_write_routes_preserve_provider_failure_and_default_route_isolation",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::plugins_write_product_tests::plugins_write_routes_register_only_with_explicit_test_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::plugins_write_product_tests::plugins_write_routes_preserve_go_error_and_default_route_isolation",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::auth_session_write_product_tests::auth_session_write_routes_register_only_with_explicit_test_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::watchlist_remote_write_product_tests::remote_watchlist_write_route_registers_only_with_explicit_test_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::watchlist_write_product_tests::watchlist_write_routes_register_only_with_explicit_test_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::backtests_write_product_tests::backtests_write_routes_register_only_with_explicit_test_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::strategy_definition_tests::strategy_definition_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::strategy_definition_tests::strategy_definition_routes_fail_closed_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::remote_watchlist_tests::remote_watchlist_read_route_matches_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::remote_watchlist_tests::remote_watchlist_read_route_fails_closed_when_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::remote_watchlist_tests::remote_watchlist_read_route_is_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::system_read_tests::system_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::system_read_tests::system_read_routes_fail_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::system_read_tests::system_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::backtests_read_tests::backtests_read_routes_match_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::backtests_read_tests::backtests_read_routes_fail_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::backtests_read_tests::backtests_read_routes_are_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::backtests_sync_read_tests::backtests_sync_read_route_matches_group_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::backtests_sync_read_tests::backtests_sync_read_route_fails_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::backtests_sync_read_tests::backtests_sync_read_route_is_not_registered_without_snapshot_port",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-integration-futu",
  "provider::tests::broker_descriptor_matches_current_go_wire_fixture",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "runtime_dependencies::tests::stage9_node_version_corpus_matches_go",
  "--",
  "--exact",
]);
run("cargo", ["test", "-p", "jftrade-calendar"]);

console.log("Go/Rust Stage 9 product-slice differential passed: auth-session, settings/system/alerts/alerts-write/plugins/plugins-write/research-presets-write/strategy-definitions-write/strategy-definitions-read/strategy-instance/strategy-pine/watchlist/portfolio/research/research-preset/execution/market-data-provider/market-data-provider-actions/market-data-catalog/market-data-derivatives/market-data-options/market-data-news-actions/market-data-news-search/adk/adk-chat-stream/market-data-quote-read/market-data-prediction-read/broker/backtests read projections, backtest sync progress, test-cutover snapshots and fail-closed behavior, calendar/watchlist/plugin control-plane slices, data-management rehearsal, and existing product compatibility corpus.");
