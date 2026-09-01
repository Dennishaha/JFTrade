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

    #[test]
    fn log_reader_matches_go_filter_paging_and_day_order() {
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
        assert_eq!(default_page.limit, DEFAULT_LOG_LIMIT);
    }

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
}
