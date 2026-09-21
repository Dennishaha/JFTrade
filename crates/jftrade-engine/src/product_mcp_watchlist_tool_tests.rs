//! Executor coverage for the `watchlist.list` tool boundary.
//!
//! Owner: `internal/assistant/assembly/watchlist_adapter_test.go` on the `go`
//! branch. The reference adapter reads the stored groups, resolves an operator
//! supplied group reference by id or case-insensitive name, lists that group's
//! members and only fetches quotes when the caller opts in. The Rust owner is
//! the production MCP executor plus the production watchlist read/write ports,
//! so this coverage lives beside the MCP server catalog that hosts them.

use super::tests::production_bundle;
use crate::product::product_mcp_production_executor::ProductionMcpToolExecutor;
use crate::product::product_production_ports::ProductionPortBundle;
use crate::product::product_watchlist_write_port::WatchlistWriteMutation;
use crate::product::{WatchlistReadSnapshotError, WatchlistReadSnapshotPort};
use serde_json::{Value, json};
use std::sync::{Arc, Mutex};
use tempfile::TempDir;

fn bundle_with<F: FnOnce(&mut ProductionPortBundle)>(
    configure: F,
) -> (TempDir, Arc<ProductionPortBundle>) {
    let (directory, mut ports) = production_bundle();
    configure(&mut ports);
    (directory, Arc::new(ports))
}

/// Records the read paths the tool reaches without answering real data, so a
/// rejected call can be proven to stop before it lists members.
#[derive(Debug, Default)]
struct CountingWatchlistRead {
    reads: Mutex<Vec<(String, String)>>,
}

impl WatchlistReadSnapshotPort for CountingWatchlistRead {
    fn read(&self, path: &str, query: &str) -> Result<Value, WatchlistReadSnapshotError> {
        self.reads
            .lock()
            .expect("watchlist reads")
            .push((path.to_owned(), query.to_owned()));
        Ok(json!({"items": [], "groups": []}))
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/watchlist_adapter_test.go:24
/// `TestADKWatchlistListReturnsRealDataWithoutImplicitQuoteCalls` (reachable
/// half). Go lists the stored groups, then one group's members, and fetches
/// quotes only when the caller opts in. Rust reads the production watchlist
/// store: the default call returns the stored group, the grouped call resolves
/// a case-insensitive group name and returns its member, and neither call
/// carries quote data. The opt-in has no production owner yet
/// (`includeQuotes:true` fails closed with `WATCHLIST_QUOTES_UNAVAILABLE`),
/// which stays registered as the batch 47/57 P2 boundary.
#[test]
fn watchlist_list_serves_stored_groups_and_members_without_quote_enrichment() {
    let (_directory, ports) = production_bundle();
    let group = ports
        .watchlist_write
        .mutate(&WatchlistWriteMutation {
            value: json!({"route": "create-group", "name": "US Tech"}),
        })
        .expect("seed watchlist group");
    let group_id = group["groupId"].as_str().expect("group id").to_owned();
    ports
        .watchlist_write
        .mutate(&WatchlistWriteMutation {
            value: json!({
                "route": "replace-memberships",
                "instrumentId": "US.AAPL",
                "groupIds": [group_id],
                "newGroupNames": [],
                "expectedRevision": 0,
            }),
        })
        .expect("seed watchlist membership");

    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let summary = executor
        .execute_production("watchlist.list", &json!({}))
        .expect("watchlist.list groups");
    let groups = summary["groups"].as_array().expect("groups array");
    assert_eq!(groups.len(), 1, "{summary}");
    assert_eq!(groups[0]["groupId"], group_id, "{summary}");
    assert!(
        summary.get("quotes").is_none() && summary.get("quoteErrors").is_none(),
        "the default listing must not enrich quotes: {summary}"
    );

    let members = executor
        .execute_production("watchlist.list", &json!({"group": "us tech"}))
        .expect("watchlist.list members");
    let items = members["items"].as_array().expect("items array");
    assert_eq!(items.len(), 1, "{members}");
    assert_eq!(items[0]["instrumentId"], "US.AAPL", "{members}");
    assert!(
        members.get("quotes").is_none() && members.get("quoteErrors").is_none(),
        "listing members must not enrich quotes: {members}"
    );

    let quotes = executor
        .execute_production("watchlist.list", &json!({"includeQuotes": true}))
        .expect_err("quote enrichment is unavailable in production");
    assert_eq!(quotes.status, 503);
    assert_eq!(quotes.code, "WATCHLIST_QUOTES_UNAVAILABLE");
}

/// Parity: go:452dea11:internal/assistant/assembly/watchlist_adapter_test.go:72
/// `TestWatchlistToolAdapterUnavailableAndMissingGroupBoundaries`: a watchlist
/// tool without a service reports unavailable, and an unknown group never
/// resolves to a listing. Rust's equivalents are the port-less executor
/// (`503 MCP_PRODUCTION_EXECUTOR_UNAVAILABLE`) and the tool's group resolution
/// (`404 WATCHLIST_NOT_FOUND`) which stops after the group read.
#[test]
fn watchlist_list_fails_closed_without_ports_and_for_unknown_groups() {
    let (_directory, ports) = production_bundle();
    let portless = ProductionMcpToolExecutor::new(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
    );
    let failure = portless
        .execute_production("watchlist.list", &json!({"group": "Favorites"}))
        .expect_err("watchlist.list without production ports must fail closed");
    assert_eq!(failure.status, 503);
    assert_eq!(failure.code, "MCP_PRODUCTION_EXECUTOR_UNAVAILABLE");

    let recorder: Arc<CountingWatchlistRead> = Arc::new(CountingWatchlistRead::default());
    let recorder_handle = Arc::clone(&recorder);
    let (_directory, ports) = bundle_with(|ports| {
        let port: Arc<dyn WatchlistReadSnapshotPort> = recorder_handle;
        ports.watchlist = port;
    });
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));

    let failure = executor
        .execute_production("watchlist.list", &json!({"group": "missing"}))
        .expect_err("an unknown watchlist group must fail closed");
    assert_eq!(failure.status, 404, "{failure:?}");
    assert_eq!(failure.code, "WATCHLIST_NOT_FOUND");
    assert_eq!(
        recorder.reads.lock().expect("watchlist reads").as_slice(),
        [("/api/v1/watchlist/groups".to_owned(), "limit=50".to_owned())],
        "an unresolved group must never reach the member listing"
    );
}
