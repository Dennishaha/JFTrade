fn normalized_day(value: &str) -> Result<String, DesktopFailure> {
    let value = value.trim();
    if value.is_empty() {
        return Ok(jiff::Zoned::now().date().to_string());
    }
    let bytes = value.as_bytes();
    let valid_shape = bytes.len() == 10
        && bytes[4] == b'-'
        && bytes[7] == b'-'
        && bytes
            .iter()
            .enumerate()
            .all(|(index, byte)| matches!(index, 4 | 7) || byte.is_ascii_digit());
    let valid_date = valid_shape
        && value[0..4]
            .parse::<i32>()
            .ok()
            .zip(value[5..7].parse::<u8>().ok())
            .zip(value[8..10].parse::<u8>().ok())
            .and_then(|((year, month), day)| {
                Month::try_from(month)
                    .ok()
                    .and_then(|month| Date::from_calendar_date(year, month, day).ok())
            })
            .is_some();
    if valid_date {
        Ok(value.to_owned())
    } else {
        Err(DesktopFailure::new(
            "DESKTOP_LOG_DAY_INVALID",
            "desktop log day must use YYYY-MM-DD",
        ))
    }
}

fn log_day(file_name: &str) -> Option<String> {
    file_name
        .strip_prefix("desktop-")
        .and_then(|value| value.strip_suffix(".log"))
        .and_then(|value| normalized_day(value).ok())
}

fn parse_log_level(line: &str) -> &'static str {
    let upper = line.to_ascii_uppercase();
    for (level, tokens) in [
        (
            "ERROR",
            &[
                "LEVEL=ERROR",
                "\"LEVEL\":\"ERROR\"",
                " ERROR ",
                "[ERROR]",
                " ERROR:",
                "ERROR ",
            ][..],
        ),
        (
            "WARN",
            &[
                "LEVEL=WARN",
                "LEVEL=WARNING",
                "\"LEVEL\":\"WARN\"",
                "\"LEVEL\":\"WARNING\"",
                " WARN ",
                " WARNING ",
                "[WARN]",
                "[WARNING]",
                "WARN ",
                "WARNING ",
            ][..],
        ),
        (
            "DEBUG",
            &[
                "LEVEL=DEBUG",
                "\"LEVEL\":\"DEBUG\"",
                " DEBUG ",
                "[DEBUG]",
                "DEBUG ",
            ][..],
        ),
        (
            "INFO",
            &[
                "LEVEL=INFO",
                "\"LEVEL\":\"INFO\"",
                " INFO ",
                "[INFO]",
                "INFO ",
            ][..],
        ),
    ] {
        if tokens.iter().any(|token| upper.contains(token)) {
            return level;
        }
    }
    "INFO"
}

fn now_rfc3339() -> String {
    OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}

fn desktop_log_path(settings_path: &Path) -> PathBuf {
    settings_path
        .parent()
        .filter(|path| !path.as_os_str().is_empty())
        .unwrap_or(Path::new("."))
        .join("logs")
        .join(format!("desktop-{}.log", jiff::Zoned::now().date()))
}

fn append_native_log(path: &Path, level: &str, message: &str) {
    let result = (|| -> Result<(), std::io::Error> {
        if let Some(directory) = path.parent().filter(|value| !value.as_os_str().is_empty()) {
            fs::create_dir_all(directory)?;
        }
        let mut file = OpenOptions::new().create(true).append(true).open(path)?;
        writeln!(file, "{} {} {}", now_rfc3339(), level, message)
    })();
    if let Err(error) = result {
        eprintln!("JFTrade desktop log append failed: {error}");
    }
}

fn desktop_error<E: std::fmt::Display>(code: &'static str) -> impl FnOnce(E) -> DesktopFailure {
    move |error| DesktopFailure::new(code, error.to_string())
}
