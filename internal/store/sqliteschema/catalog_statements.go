package sqliteschema

func backtestDefinition() Definition {
	const prototype = "local_klines__manifest__1m__forward__r__00000000"
	return Definition{
		ID:      DatabaseBacktest,
		Version: BacktestVersion,
		DynamicTable: &DynamicTableDefinition{
			Pattern:       `^local_klines__[a-z0-9_]+__[a-z0-9_]+__(forward|backward|none)__(r|x)__[0-9a-f]{8}$`,
			PrototypeName: prototype,
			Statement: `CREATE TABLE ` + prototype + ` (
				end_time INTEGER NOT NULL,
				start_time INTEGER NOT NULL,
				open TEXT NOT NULL,
				high TEXT NOT NULL,
				low TEXT NOT NULL,
				close TEXT NOT NULL,
				volume TEXT NOT NULL,
				PRIMARY KEY (end_time)
			) WITHOUT ROWID`,
		},
	}
}

func backtestRunsDefinition() Definition {
	return Definition{ID: DatabaseBacktestRuns, Version: BacktestRunsVersion, Statements: []string{
		`CREATE TABLE backtest_runs (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT '',
			request_json TEXT NOT NULL DEFAULT '',
			result_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX idx_backtest_runs_updated_at ON backtest_runs (updated_at DESC, id ASC)`,
		`CREATE INDEX idx_backtest_runs_status ON backtest_runs (status, updated_at DESC)`,
	}}
}

func strategyDefinition() Definition {
	return Definition{ID: DatabaseStrategy, Version: StrategyVersion, Statements: []string{
		`CREATE TABLE strategy_log_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			instance_id TEXT NOT NULL,
			at_ms INTEGER NOT NULL,
			raw TEXT NOT NULL,
			level TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE strategy_audit_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			instance_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			at_ms INTEGER NOT NULL
		)`,
		`CREATE TABLE strategy_runtime_observations (
			instance_id TEXT PRIMARY KEY,
			actual_status_snapshot TEXT NOT NULL DEFAULT '',
			active_symbols_json TEXT NOT NULL DEFAULT '[]',
			last_closed_kline_at_ms INTEGER,
			last_signal_at_ms INTEGER,
			last_order_at_ms INTEGER,
			last_error_at_ms INTEGER,
			last_error TEXT NOT NULL DEFAULT '',
			updated_at_ms INTEGER
		)`,
		`CREATE TABLE strategy_catalog_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE strategy_catalog_plugins (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE strategy_catalog_strategies (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE strategy_catalog_operations (operation_id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE strategy_design_definitions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			version TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			runtime TEXT NOT NULL DEFAULT '',
			source_format TEXT NOT NULL DEFAULT '',
			symbol TEXT NOT NULL DEFAULT '',
			interval TEXT NOT NULL DEFAULT '',
			script TEXT NOT NULL DEFAULT '',
			visual_model_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			deleted_at TEXT
		)`,
		`CREATE TABLE strategy_definition_versions (
			definition_id TEXT NOT NULL,
			version TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			runtime TEXT NOT NULL DEFAULT '',
			source_format TEXT NOT NULL DEFAULT '',
			symbol TEXT NOT NULL DEFAULT '',
			interval TEXT NOT NULL DEFAULT '',
			script TEXT NOT NULL DEFAULT '',
			visual_model_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			saved_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (definition_id, version),
			FOREIGN KEY (definition_id) REFERENCES strategy_design_definitions(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX idx_strategy_log_events_instance_at ON strategy_log_events (instance_id, at_ms DESC, id DESC)`,
		`CREATE INDEX idx_strategy_log_events_level ON strategy_log_events (level)`,
		`CREATE INDEX idx_strategy_audit_events_instance_at ON strategy_audit_events (instance_id, at_ms DESC, id DESC)`,
		`CREATE INDEX idx_strategy_audit_events_kind ON strategy_audit_events (kind)`,
		`CREATE INDEX idx_strategy_catalog_strategies_created_at ON strategy_catalog_strategies (created_at ASC, id ASC)`,
		`CREATE INDEX idx_strategy_catalog_operations_updated_at ON strategy_catalog_operations (updated_at DESC, operation_id ASC)`,
		`CREATE INDEX idx_strategy_design_definitions_updated_at ON strategy_design_definitions (updated_at DESC, id ASC)`,
		`CREATE INDEX idx_strategy_design_definitions_deleted_at ON strategy_design_definitions (deleted_at)`,
		`CREATE INDEX idx_strategy_definition_versions_saved_at ON strategy_definition_versions (definition_id, saved_at DESC, version DESC)`,
		`CREATE TRIGGER trg_strategy_definition_versions_immutable
			BEFORE UPDATE ON strategy_definition_versions
			BEGIN
				SELECT RAISE(ABORT, 'strategy definition versions are immutable');
			END`,
	}}
}

var executionStatements = []string{
	`CREATE TABLE execution_orders (
			internal_order_id TEXT PRIMARY KEY,
			broker_id TEXT NOT NULL DEFAULT '',
			broker_order_id TEXT,
			broker_order_id_ex TEXT,
			source TEXT NOT NULL DEFAULT '',
			source_detail TEXT NOT NULL DEFAULT '',
			trading_environment TEXT NOT NULL DEFAULT '',
			account_id TEXT NOT NULL DEFAULT '',
			market TEXT NOT NULL DEFAULT '',
			symbol TEXT,
			side TEXT,
			order_type TEXT,
			status TEXT NOT NULL DEFAULT '',
			requested_quantity REAL,
			requested_price REAL,
			filled_quantity REAL,
			filled_average_price REAL,
			remark TEXT,
			last_error TEXT,
			last_error_code TEXT,
			last_error_source TEXT,
			submitted_at TEXT,
			updated_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			raw_broker_status TEXT,
			order_kind TEXT NOT NULL DEFAULT 'single',
			product_class TEXT NOT NULL DEFAULT 'unknown',
			quantity_mode TEXT NOT NULL DEFAULT 'units',
			client_order_id TEXT,
			preview_id TEXT,
			normalized_request TEXT NOT NULL DEFAULT '{}',
			requested_amount REAL,
			payout REAL,
			fees REAL
		)`,
	`CREATE TABLE execution_order_legs (
			id TEXT PRIMARY KEY,
			internal_order_id TEXT NOT NULL,
			leg_index INTEGER NOT NULL,
			broker_leg_id TEXT,
			instrument_id TEXT NOT NULL,
			product_class TEXT NOT NULL DEFAULT 'unknown',
			side TEXT NOT NULL DEFAULT '',
			ratio INTEGER NOT NULL DEFAULT 1,
			prediction_side TEXT NOT NULL DEFAULT '',
			requested_quantity REAL,
			requested_amount REAL,
			requested_price REAL,
			status TEXT NOT NULL DEFAULT '',
			filled_quantity REAL,
			filled_amount REAL,
			average_price REAL,
			fees REAL,
			payout REAL,
			updated_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
	`CREATE TABLE execution_order_previews (
			preview_id TEXT PRIMARY KEY,
			request_hash TEXT NOT NULL,
			broker_id TEXT NOT NULL,
			capability_version TEXT NOT NULL,
			account_id TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			quote_expires_at TEXT,
			rfq_id TEXT,
			normalized_request TEXT NOT NULL,
			created_at TEXT NOT NULL,
			consumed_at TEXT
		)`,
	`CREATE TABLE execution_prediction_quotes (
			quote_id TEXT PRIMARY KEY,
			broker_id TEXT NOT NULL,
			account_id TEXT NOT NULL,
			trading_environment TEXT NOT NULL,
			mvc TEXT NOT NULL,
			legs_hash TEXT NOT NULL,
			bid_price REAL,
			ask_price REAL,
			should_retry INTEGER NOT NULL DEFAULT 0,
			received_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			expiry_source TEXT NOT NULL DEFAULT 'jftrade_policy',
			status TEXT NOT NULL DEFAULT 'active',
			consumed_at TEXT,
			consumed_preview_id TEXT,
			consumed_client_order_id TEXT
		)`,
	`CREATE TABLE execution_order_events (
			id TEXT PRIMARY KEY,
			internal_order_id TEXT NOT NULL,
			event_type TEXT NOT NULL DEFAULT '',
			previous_status TEXT,
			next_status TEXT NOT NULL DEFAULT '',
			payload_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
	`CREATE TABLE execution_seen_fills (fill_key TEXT PRIMARY KEY, created_at TEXT NOT NULL DEFAULT '')`,
	`CREATE TABLE execution_sequences (name TEXT PRIMARY KEY, value INTEGER NOT NULL DEFAULT 0)`,
	`CREATE INDEX idx_execution_orders_updated ON execution_orders (updated_at DESC, created_at DESC, internal_order_id DESC)`,
	`CREATE INDEX idx_execution_orders_broker_order ON execution_orders (broker_id, trading_environment, account_id, market, broker_order_id)`,
	`CREATE INDEX idx_execution_orders_broker_order_ex ON execution_orders (broker_id, trading_environment, account_id, market, broker_order_id_ex)`,
	`CREATE INDEX idx_execution_order_events_order ON execution_order_events (internal_order_id, created_at ASC, id ASC)`,
	`CREATE UNIQUE INDEX idx_execution_orders_client_id ON execution_orders (broker_id, trading_environment, account_id, client_order_id) WHERE client_order_id IS NOT NULL AND TRIM(client_order_id) <> ''`,
	`CREATE INDEX idx_execution_order_legs_order ON execution_order_legs (internal_order_id, leg_index ASC)`,
	`CREATE INDEX idx_execution_prediction_quotes_expiry ON execution_prediction_quotes (status, expires_at)`,
}

func executionDefinition() Definition {
	return Definition{ID: DatabaseExecution, Version: ExecutionVersion, Statements: append([]string(nil), executionStatements...)}
}

func adkDefinition() Definition {
	return Definition{ID: DatabaseADK, Version: ADKVersion, Statements: []string{
		`CREATE TABLE adk_providers (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_agents (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_sessions (id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_runs (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, agent_id TEXT NOT NULL, status TEXT NOT NULL, client_request_id TEXT NOT NULL DEFAULT '', request_fingerprint TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_approvals (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, agent_id TEXT NOT NULL, status TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_skills (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_audit_events (id TEXT PRIMARY KEY, kind TEXT NOT NULL, subject_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE adk_optimization_tasks (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_tasks (id TEXT PRIMARY KEY, status TEXT NOT NULL, agent_id TEXT NOT NULL, run_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_memory (id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, scope TEXT NOT NULL, memory_key TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_session_contexts (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_handoff_segments (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, active INTEGER NOT NULL, sequence_no INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, payload_json TEXT NOT NULL)`,
		`CREATE TABLE adk_session_context_state (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_session_notices (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, run_id TEXT NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_session_composer_state (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_workflows (id TEXT PRIMARY KEY, status TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_workflow_triggers (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, trigger_type TEXT NOT NULL, status TEXT NOT NULL, next_run_at TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_workflow_trigger_logs (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, trigger_id TEXT NOT NULL, trigger_type TEXT NOT NULL, status TEXT NOT NULL, run_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_run_leases (run_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, fencing_token INTEGER NOT NULL, heartbeat_at_unix_ms INTEGER NOT NULL, expires_at_unix_ms INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE adk_tool_invocations (run_id TEXT NOT NULL, idempotency_key TEXT NOT NULL, tool_name TEXT NOT NULL, status TEXT NOT NULL, owner_id TEXT NOT NULL, fencing_token INTEGER NOT NULL, run_lease_token INTEGER NOT NULL, input_json TEXT NOT NULL, output_json TEXT NOT NULL, lease_expires_at_unix_ms INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY (run_id, idempotency_key))`,
		`CREATE INDEX idx_adk_sessions_agent ON adk_sessions (agent_id, updated_at DESC)`,
		`CREATE INDEX idx_adk_runs_session ON adk_runs (session_id, created_at DESC)`,
		`CREATE UNIQUE INDEX idx_adk_runs_client_request ON adk_runs (client_request_id) WHERE client_request_id <> ''`,
		`CREATE INDEX idx_adk_approvals_status ON adk_approvals (status, updated_at DESC)`,
		`CREATE UNIQUE INDEX idx_adk_approvals_confirmation_call ON adk_approvals (json_extract(payload_json, '$.confirmationCallId')) WHERE COALESCE(json_extract(payload_json, '$.confirmationCallId'), '') <> ''`,
		`CREATE INDEX idx_adk_audit_kind ON adk_audit_events (kind, created_at DESC)`,
		`CREATE INDEX idx_adk_tasks_status ON adk_tasks (status, updated_at DESC)`,
		`CREATE INDEX idx_adk_tasks_agent ON adk_tasks (agent_id, updated_at DESC)`,
		`CREATE UNIQUE INDEX idx_adk_memory_agent_scope_key ON adk_memory (agent_id, scope, memory_key)`,
		`CREATE INDEX idx_adk_session_contexts_updated ON adk_session_contexts (updated_at DESC)`,
		`CREATE INDEX idx_adk_handoff_segments_session ON adk_handoff_segments (session_id, sequence_no ASC)`,
		`CREATE INDEX idx_adk_session_context_state_updated ON adk_session_context_state (updated_at DESC)`,
		`CREATE INDEX idx_adk_session_notices_session ON adk_session_notices (session_id, created_at ASC)`,
		`CREATE INDEX idx_adk_workflows_status ON adk_workflows (status, updated_at DESC)`,
		`CREATE INDEX idx_adk_workflow_triggers_workflow ON adk_workflow_triggers (workflow_id, updated_at DESC)`,
		`CREATE INDEX idx_adk_workflow_triggers_due ON adk_workflow_triggers (trigger_type, status, next_run_at ASC)`,
		`CREATE INDEX idx_adk_workflow_trigger_logs_workflow ON adk_workflow_trigger_logs (workflow_id, created_at DESC)`,
		`CREATE INDEX idx_adk_workflow_trigger_logs_trigger ON adk_workflow_trigger_logs (trigger_id, created_at DESC)`,
		`CREATE INDEX idx_adk_workflow_trigger_logs_status ON adk_workflow_trigger_logs (status, updated_at DESC)`,
		`CREATE INDEX idx_adk_run_leases_expires ON adk_run_leases (expires_at_unix_ms ASC)`,
		`CREATE INDEX idx_adk_tool_invocations_status ON adk_tool_invocations (status, lease_expires_at_unix_ms ASC)`,
	}}
}

func adkSessionDefinition() Definition {
	return Definition{ID: DatabaseADKSession, Version: ADKSessionVersion, Statements: []string{
		`CREATE TABLE sessions (
			app_name TEXT,
			user_id TEXT,
			id TEXT,
			state TEXT,
			create_time TIMESTAMP,
			update_time TIMESTAMP,
			PRIMARY KEY (app_name, user_id, id)
		)`,
		`CREATE TABLE events (
			id TEXT,
			app_name TEXT,
			user_id TEXT,
			session_id TEXT,
			invocation_id TEXT,
			author TEXT,
			actions BLOB,
			long_running_tool_ids_json TEXT,
			routes_json TEXT,
			output_json TEXT,
			node_info_json TEXT,
			requested_input_json TEXT,
			branch TEXT,
			isolation_scope TEXT,
			timestamp TIMESTAMP,
			content TEXT,
			grounding_metadata TEXT,
			custom_metadata TEXT,
			usage_metadata TEXT,
			citation_metadata TEXT,
			partial NUMERIC,
			turn_complete NUMERIC,
			error_code TEXT,
			error_message TEXT,
			interrupted NUMERIC,
			PRIMARY KEY (id, app_name, user_id, session_id),
			FOREIGN KEY (app_name, user_id, session_id) REFERENCES sessions(app_name, user_id, id) ON DELETE CASCADE
		)`,
		`CREATE TABLE app_states (app_name TEXT PRIMARY KEY, state TEXT, update_time TIMESTAMP)`,
		`CREATE TABLE user_states (app_name TEXT, user_id TEXT, state TEXT, update_time TIMESTAMP, PRIMARY KEY (app_name, user_id))`,
	}}
}

func adkArtifactDefinition() Definition {
	return Definition{ID: DatabaseADKArtifact, Version: ADKArtifactVersion, Statements: []string{
		`CREATE TABLE artifacts (
			app_name TEXT NOT NULL,
			user_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			file_name TEXT NOT NULL,
			version INTEGER NOT NULL,
			part_json TEXT NOT NULL,
			mime_type TEXT NOT NULL,
			custom_metadata_json TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (app_name, user_id, session_id, file_name, version)
		)`,
	}}
}

var watchlistStatements = []string{
	`CREATE TABLE watchlist_groups (
			group_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			name_key TEXT NOT NULL UNIQUE,
			is_default INTEGER NOT NULL DEFAULT 0,
			protected INTEGER NOT NULL DEFAULT 0,
			revision INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	`CREATE UNIQUE INDEX watchlist_groups_one_default ON watchlist_groups(is_default) WHERE is_default = 1`,
	`CREATE TABLE watchlist_instruments (
			instrument_id TEXT PRIMARY KEY,
			market TEXT NOT NULL,
			symbol TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			instrument_type TEXT NOT NULL DEFAULT '',
			membership_revision INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	`CREATE TABLE watchlist_memberships (
			group_id TEXT NOT NULL,
			instrument_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (group_id, instrument_id)
		)`,
	`CREATE INDEX watchlist_memberships_instrument ON watchlist_memberships(instrument_id, group_id)`,
	`CREATE TABLE watchlist_sources (
			source_id TEXT PRIMARY KEY,
			broker TEXT NOT NULL,
			display_name TEXT NOT NULL,
			status TEXT NOT NULL,
			last_error TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
	`CREATE TABLE watchlist_remote_groups (
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			name TEXT NOT NULL,
			group_type TEXT NOT NULL,
			ambiguous INTEGER NOT NULL DEFAULT 0,
			member_count INTEGER NOT NULL DEFAULT 0,
			remote_hash TEXT NOT NULL DEFAULT '',
			observed_at TEXT NOT NULL,
			PRIMARY KEY (source_id, remote_group_id)
		)`,
	`CREATE TABLE watchlist_bindings (
			binding_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			remote_name TEXT NOT NULL,
			local_group_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (source_id, remote_group_id)
		)`,
	`CREATE INDEX watchlist_bindings_local_group ON watchlist_bindings(local_group_id)`,
	`CREATE TABLE watchlist_remote_memberships (
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			instrument_id TEXT NOT NULL,
			remote_hash TEXT NOT NULL,
			observed_at TEXT NOT NULL,
			PRIMARY KEY (source_id, remote_group_id, instrument_id)
		)`,
	`CREATE TABLE watchlist_membership_origins (
			group_id TEXT NOT NULL,
			instrument_id TEXT NOT NULL,
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			last_imported_at TEXT NOT NULL,
			PRIMARY KEY (group_id, instrument_id, source_id, remote_group_id)
		)`,
	`CREATE INDEX watchlist_membership_origins_instrument ON watchlist_membership_origins(instrument_id, group_id)`,
	`CREATE TABLE watchlist_instrument_aliases (
			source_id TEXT NOT NULL,
			alias_kind TEXT NOT NULL,
			alias_value TEXT NOT NULL,
			instrument_id TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (source_id, alias_kind, alias_value)
		)`,
	`CREATE TABLE watchlist_import_previews (
			preview_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			remote_group_name TEXT NOT NULL,
			local_group_id TEXT NOT NULL DEFAULT '',
			new_group_name TEXT NOT NULL DEFAULT '',
			remote_hash TEXT NOT NULL,
			local_group_revision INTEGER NOT NULL,
			added_json TEXT NOT NULL,
			unchanged_json TEXT NOT NULL,
			local_only_json TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		)`,
	`CREATE INDEX watchlist_import_previews_expiry ON watchlist_import_previews(status, expires_at)`,
	`CREATE TABLE watchlist_import_runs (
			run_id TEXT PRIMARY KEY,
			preview_id TEXT NOT NULL,
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			remote_group_name TEXT NOT NULL,
			local_group_id TEXT NOT NULL,
			status TEXT NOT NULL,
			added_count INTEGER NOT NULL,
			removed_count INTEGER NOT NULL,
			unchanged_count INTEGER NOT NULL,
			remote_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			completed_at TEXT NOT NULL
		)`,
	`CREATE INDEX watchlist_import_runs_source ON watchlist_import_runs(source_id, run_id DESC)`,
}

func watchlistDefinition() Definition {
	return Definition{ID: DatabaseWatchlist, Version: WatchlistVersion, Statements: append([]string(nil), watchlistStatements...)}
}

func researchDefinition() Definition {
	return Definition{ID: DatabaseResearch, Version: ResearchVersion, Statements: []string{
		`CREATE TABLE research_screen_presets (
			preset_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			name_key TEXT NOT NULL UNIQUE,
			query_schema_version INTEGER NOT NULL,
			query_json TEXT NOT NULL,
			revision INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX research_screen_presets_updated_at ON research_screen_presets(updated_at DESC, preset_id)`,
	}}
}
