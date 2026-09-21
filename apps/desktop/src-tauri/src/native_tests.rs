mod tests {
    use super::*;

    fn test_port(root: &Path) -> NativeDesktopPort {
        NativeDesktopPort::new(
            DesktopStartupSnapshot {
                state: "ready".to_owned(),
                phase: "test".to_owned(),
                message: String::new(),
                started_at: "2026-08-19T00:00:00Z".to_owned(),
            },
            &root.join("settings.json"),
            "JFTrade Dev",
            NativeUpdaterConfig::Disabled,
            Arc::new(|| {}),
        )
    }

    #[test]
    fn updater_requires_complete_https_release_configuration() {
        assert_eq!(
            NativeUpdaterConfig::from_values(false, None, None).expect("development disabled"),
            NativeUpdaterConfig::Disabled
        );
        assert_eq!(
            NativeUpdaterConfig::from_values(true, None, None).expect("release unconfigured"),
            NativeUpdaterConfig::Unconfigured
        );
        assert!(
            NativeUpdaterConfig::from_values(
                true,
                Some("https://updates.jftrade.example/{{target}}/{{arch}}".to_owned()),
                None,
            )
            .is_err()
        );
        for endpoint in [
            "http://updates.jftrade.example/latest",
            "https://user:password@updates.jftrade.example/latest",
        ] {
            assert!(
                NativeUpdaterConfig::from_values(
                    true,
                    Some(endpoint.to_owned()),
                    Some("test-public-key".to_owned()),
                )
                .is_err(),
                "accepted {endpoint}"
            );
        }
        assert!(matches!(
            NativeUpdaterConfig::from_values(
                true,
                Some("https://updates.jftrade.example/{{target}}/{{arch}}".to_owned()),
                Some("test-public-key".to_owned()),
            )
            .expect("valid release updater"),
            NativeUpdaterConfig::Ready { .. }
        ));
    }

    // Parity: go:452dea11:cmd/jftrade-desktop/desktop_updates_test.go:33 TestDesktopUpdateServiceDisabledForDevelopment
    #[test]
    fn development_channel_skips_release_checks_while_the_release_channel_runs_them() {
        let paths = PlatformPaths {
            platform: DesktopPlatform::Darwin,
            home_dir: "/fixture/home".to_owned(),
            config_dir: String::new(),
            local_app_data: String::new(),
            xdg_data_home: String::new(),
        };
        let development =
            DesktopProfile::resolve(DesktopChannel::Dev, &paths).expect("development profile");
        assert!(
            !development.update_checks_enabled,
            "development builds never poll for desktop releases"
        );
        assert_eq!(
            NativeUpdaterConfig::from_values(development.update_checks_enabled, None, None)
                .expect("development updater"),
            NativeUpdaterConfig::Disabled
        );
        let release =
            DesktopProfile::resolve(DesktopChannel::Release, &paths).expect("release profile");
        assert!(
            release.update_checks_enabled,
            "release builds own desktop update checks"
        );
    }

    #[test]
    fn log_reader_matches_go_filter_paging_and_day_order() {
        // Parity: cmd/jftrade-desktop/main_test.go:395 TestListDesktopLogDaysAndReadsFilteredPage
        // Parity: cmd/jftrade-desktop/main_test.go:426 TestDesktopLogPageCapsLimitAndPaginatesAllLines
        // Parity: cmd/jftrade-desktop/main_test.go:464 TestDesktopLogPageTailOffsetReturnsLastPageInFileOrder
        // Parity: cmd/jftrade-desktop/main_test.go:506 TestDesktopLogPageTailOffsetAppliesFiltersBeforePaging
        let directory = tempfile::tempdir().expect("temporary directory");
        let port = test_port(directory.path());
        fs::create_dir_all(&port.log_dir).expect("create logs");
        fs::write(port.log_dir.join("desktop-2026-08-18.log"), "INFO older\n").expect("older log");
        let mut current = String::new();
        for index in 0..501 {
            let level = if index % 2 == 0 { "WARN" } else { "INFO" };
            current.push_str(&format!("{level} item-{index}\n"));
        }
        fs::write(
            port.log_dir.join("desktop-2026-08-19.log"),
            current.as_bytes(),
        )
        .expect("current log");
        fs::write(port.log_dir.join("desktop-2026-99-99.log"), b"ignored")
            .expect("invalid log name");

        let days = port.log_list_days().expect("list days");
        assert_eq!(
            days.into_iter().map(|value| value.day).collect::<Vec<_>>(),
            ["2026-08-19", "2026-08-18"]
        );
        let page = port
            .log_read_page("2026-08-19", "WARN", "item", LATEST_LOG_OFFSET, 100)
            .expect("read last page");
        assert_eq!(page.total, 251);
        assert_eq!(page.offset, 200);
        assert_eq!(page.items.len(), 51);
        assert!(page.items[0].text.contains("item-400"));
        let default_page = port
            .log_read_page("2026-08-18", "ALL", "", 0, 0)
            .expect("default page");
        assert_eq!(default_page.limit, 200);
    }

    #[test]
    fn log_reader_caps_page_limit_and_paginates_all_lines() {
        // Parity: cmd/jftrade-desktop/main_test.go:426 TestDesktopLogPageCapsLimitAndPaginatesAllLines
        let directory = tempfile::tempdir().expect("temporary directory");
        let port = test_port(directory.path());
        fs::create_dir_all(&port.log_dir).expect("create logs");
        let mut contents = String::new();
        for index in 1..=2_005 {
            contents.push_str(&format!("INFO line-{index:04}\n"));
        }
        fs::write(port.log_dir.join("desktop-2026-08-19.log"), contents)
            .expect("write log");

        let first = port
            .log_read_page("2026-08-19", "ALL", "", 0, 1_000)
            .expect("read first page");
        assert_eq!(first.total, 2_005);
        assert_eq!(first.offset, 0);
        assert_eq!(first.limit, 500);
        assert_eq!(first.items.len(), 500);
        assert_eq!(first.items[0].text, "INFO line-0001");
        assert_eq!(first.items[499].text, "INFO line-0500");

        let last = port
            .log_read_page("2026-08-19", "ALL", "", 2_000, 500)
            .expect("read last page");
        assert_eq!(last.total, 2_005);
        assert_eq!(last.offset, 2_000);
        assert_eq!(last.items.len(), 5);
        assert_eq!(last.items[4].text, "INFO line-2005");
    }

    #[test]
    fn test_list_desktop_log_days_missing_dir_returns_empty() {
        // Parity: cmd/jftrade-desktop/main_test.go:523 TestListDesktopLogDaysMissingDirReturnsEmpty
        let directory = tempfile::tempdir().expect("temporary directory");
        let missing = directory.path().join("missing_logs");
        let port = test_port(&missing);
        let days = port.log_list_days().expect("missing dir returns empty");
        assert!(days.is_empty());
    }

    // Parity: go:452dea11:internal/app/apiserver/runtime/runtime_test.go:155 TestDeriveDesktopLogPaths
    #[test]
    fn native_runtime_events_append_to_the_existing_daily_log_contract() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let log_path = desktop_log_path(&settings_path);
        append_native_log(&log_path, "INFO", "runtime ready");
        append_native_log(&log_path, "ERROR", "worker stopped");
        let contents = fs::read_to_string(&log_path).expect("read desktop log");
        assert!(contents.contains(" INFO runtime ready"));
        assert!(contents.contains(" ERROR worker stopped"));
        assert_eq!(
            log_path.parent(),
            Some(directory.path().join("logs").as_path())
        );
    }

    #[test]
    fn development_profile_paths_are_anchored_to_the_repository_not_process_cwd() {
        let root = Path::new("/fixture/jftrade-main");
        let mut profile = DesktopProfile::resolve(
            DesktopChannel::Dev,
            &PlatformPaths {
                platform: DesktopPlatform::Darwin,
                home_dir: "/fixture/home".to_owned(),
                config_dir: String::new(),
                local_app_data: String::new(),
                xdg_data_home: String::new(),
            },
        )
        .expect("development profile");
        absolutize_development_profile(&mut profile, root);
        assert_eq!(
            profile.settings_path,
            "/fixture/jftrade-main/var/jftrade-api/settings.json"
        );
        assert_eq!(
            profile.backtest_db_path,
            "/fixture/jftrade-main/var/jftrade-api/backtest.db"
        );
        assert_eq!(profile.window_state_path, None);
    }

    // Parity: go:452dea11:internal/desktop/runtime_path_test.go:41 TestProductDataDirUsesCurrentPlatform
    #[test]
    fn product_data_dir_uses_the_current_platform_base_directory() {
        let paths = platform_paths().expect("current platform paths");
        let directory = crate::profile::product_data_dir(&paths).expect("product data directory");
        assert!(
            directory.ends_with("/JFTrade") || directory.ends_with("/jftrade"),
            "unexpected product data directory: {directory:?}"
        );
    }

    // Parity: go:452dea11:internal/desktop/runtime_path_matching_test.go:5 TestProductDataDirReportsMissingHome
    #[test]
    fn platform_paths_report_missing_home_environment() {
        assert!(matches!(
            platform_paths_from(|_| None),
            Err(NativeError::MissingHome)
        ));
        assert!(matches!(
            platform_paths_from(|key| (key == "HOME").then(std::ffi::OsString::new)),
            Err(NativeError::MissingHome)
        ));

        let fallback = platform_paths_from(|key| {
            (key == "USERPROFILE").then(|| std::ffi::OsString::from("/Users/alice"))
        })
        .expect("user profile fallback");
        assert_eq!(fallback.home_dir, "/Users/alice");
    }

    // Parity: go:452dea11:cmd/internal/protogen/repository_test.go:12 TestFindRepoRoot
    #[test]
    fn repository_root_walks_up_from_a_nested_directory_to_the_workspace_markers() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let root = directory.path().join("workspace");
        fs::create_dir_all(root.join("workers/pineworker")).expect("worker marker directory");
        fs::write(root.join("Cargo.toml"), b"[workspace]\n").expect("workspace manifest");
        // A directory named like the marker must not qualify: the probe only
        // accepts a regular manifest file, mirroring the retired Go stat check.
        fs::create_dir_all(root.join("one/Cargo.toml")).expect("marker-shaped directory");
        fs::create_dir_all(root.join("one/workers/pineworker"))
            .expect("marker-shaped directory worker path");
        let nested = root.join("one/two");
        fs::create_dir_all(&nested).expect("nested directory");

        assert_eq!(
            repository_root_from(&nested).expect("nested start resolves the workspace root"),
            root
        );
        assert_eq!(
            repository_root_from(&root).expect("marker directory resolves itself"),
            root
        );
    }

    // Parity: go:452dea11:cmd/internal/protogen/repository_test.go:23 TestFindRepoRootRejectsMissingModule
    #[test]
    fn repository_root_rejects_a_tree_without_the_workspace_markers() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let nested = directory.path().join("one/two");
        fs::create_dir_all(&nested).expect("nested directory");
        // A manifest without the worker directory is not a repository root, so
        // the probe keeps walking and fails closed instead of returning a
        // partial match.
        fs::write(directory.path().join("Cargo.toml"), b"[workspace]\n").expect("manifest");

        for start in [directory.path(), nested.as_path()] {
            let error =
                repository_root_from(start).expect_err("missing markers must fail closed");
            assert!(
                matches!(error, NativeError::MissingRepositoryRoot),
                "unexpected error: {error}"
            );
        }
    }

    #[test]
    fn native_boundaries_reject_invalid_days_and_generate_strong_tokens() {
        for invalid in ["2026-02-30", "2026-13-01", "../2026-08-19", "20260819"] {
            assert!(normalized_day(invalid).is_err(), "accepted {invalid}");
        }
        let token = random_token().expect("random token");
        assert_eq!(token.len(), 64);
        assert!(token.bytes().all(|byte| byte.is_ascii_hexdigit()));
    }

    #[test]
    fn runtime_readiness_allows_external_degradation_but_rejects_incomplete_startup() {
        assert!(runtime_readiness_failure("ready").is_none());
        assert!(runtime_readiness_failure("degraded").is_none());
        for state in ["starting", "rehearsal", "unavailable", "failed", "unknown"] {
            let error = runtime_readiness_failure(state).expect("unsafe startup state must fail");
            assert!(matches!(error, NativeError::RuntimeUnavailable { readiness, .. } if readiness == state));
        }
    }

    // Parity: go:452dea11:internal/app/apiserver/desktop_api_startup_test.go:87 TestLoadFrontendFSPreservesUnavailableAndEmbeddedAssetSemantics
    #[test]
    fn required_asset_does_not_fallback_for_missing_static_asset() {
        let missing = std::path::PathBuf::from("/nonexistent/static/asset/path.png");
        let result = required_asset("NONEXISTENT_ENV_KEY", Some(missing.clone()), "test asset");
        assert!(matches!(
            result,
            Err(NativeError::MissingAsset { name: "test asset", path }) if path == missing.to_string_lossy()
        ));
    }

    #[test]
    fn development_pine_runtime_without_staged_bundle_reports_unavailable_asset() {
        let repository = tempfile::tempdir().expect("temporary repository root");
        let result = retained_runtime_config(Some(repository.path()), Path::new("/unused"), None);
        assert!(matches!(
            result,
            Err(NativeError::MissingAsset {
                name: "PineTS worker bundle",
                ..
            })
        ));
    }

    #[test]
    fn pine_worker_bundle_file_name_is_platform_independent() {
        assert_eq!(PINE_WORKER_BUNDLE_FILE_NAME, "worker.mjs");
        assert!(!PINE_WORKER_BUNDLE_FILE_NAME.contains(['/', '\\']));

        let development = pine_worker_bundle_fallback(Some(Path::new("/repository")), Path::new("/unused"))
            .expect("development bundle path");
        assert!(development.ends_with("var/pineworker/worker.mjs"));

        let release = pine_worker_bundle_fallback(None, Path::new("/resources"))
            .expect("release bundle path");
        assert!(release.ends_with("runtime/pineworker/worker.mjs"));
    }


    // Parity: go:452dea11:internal/app/apiserver/runtimes/handle_lifecycle_test.go:417 TestHandleConcurrentCloseIsIdempotentAndAggregatesErrors
    // Parity: go:452dea11:internal/app/apiserver/application/resources_test.go:78 TestResourcesCloseIsIdempotentAndConcurrentSafe
    #[test]
    fn stop_product_is_idempotent_across_concurrent_invocations() {
        use std::sync::{Arc, Mutex};
        use std::thread;

        let product = Arc::new(Mutex::new(None));
        let p1 = Arc::clone(&product);
        let p2 = Arc::clone(&product);

        let t1 = thread::spawn(move || {
            stop_product(&p1);
        });
        let t2 = thread::spawn(move || {
            stop_product(&p2);
        });

        t1.join().expect("t1 join");
        t2.join().expect("t2 join");

        assert!(product.lock().unwrap().is_none());
    }

}
