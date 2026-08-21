#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

function run(command, args, timeoutMs = 300_000, extraEnv = {}) {
  const result = spawnSync(command, args, {
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
    throw new Error(`${command} ${args.join(" ")} failed:\n${result.stderr || result.stdout}`);
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
  "^TestStage9WatchlistMembershipsFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9PluginUninstallGuidanceFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
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
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::static_provider_catalog_matches_current_go_wire_fixture",
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
  "product::tests::calendar_sources_route_matches_go_manager_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::calendar_sources_route_fails_closed_when_snapshot_port_is_unavailable",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::calendar_status_route_matches_go_manager_fixture_in_cutover_only",
  "--",
  "--exact",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "product::tests::calendar_status_route_fails_closed_when_snapshot_port_is_unavailable",
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

console.log("Go/Rust Stage 9 product-slice differential passed: appearance/onboarding read-write/Futu install/execution/security read-write/password/broker read-write/market-data and backtest provider read-write/catalog/calendar/research-screen catalog/calendar source and status/watchlist membership/plugin uninstall-guidance test-cutover snapshots and fail-closed/ADK/MCP read-write/token/notification/Pine settings, real-trade read controls, exact nine-database schema/overview, fenced cleanup execute/backup/compact/rebuild rehearsal, static storage, and Node runtime diagnostics.");
