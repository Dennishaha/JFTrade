fn open_route_window(
    app: &AppHandle<Wry>,
    label: &str,
    title: &str,
    route: &str,
    width: f64,
    height: f64,
) -> Result<(), DesktopFailure> {
    if let Some(window) = app.get_webview_window(label) {
        window
            .show()
            .and_then(|()| window.set_focus())
            .map_err(desktop_error("DESKTOP_WINDOW_FAILED"))?;
        return Ok(());
    }
    let url = WebviewUrl::App(PathBuf::from(route.trim_start_matches('/')));
    WebviewWindowBuilder::new(app, label, url)
        .title(title)
        .inner_size(width, height)
        .min_inner_size(760.0, 520.0)
        .center()
        .build()
        .map(|_| ())
        .map_err(desktop_error("DESKTOP_WINDOW_FAILED"))
}

fn configure_system_tray(
    app: &AppHandle<Wry>,
    application_name: &str,
    quit_requested: Arc<AtomicBool>,
) -> Result<(), NativeError> {
    const OPEN: &str = "desktop-tray-open";
    const SETTINGS: &str = "desktop-tray-settings";
    const DOCS: &str = "desktop-tray-docs";
    const LOGS: &str = "desktop-tray-logs";
    const QUIT: &str = "desktop-tray-quit";

    let menu = MenuBuilder::new(app)
        .text(OPEN, format!("打开 {application_name}"))
        .separator()
        .text(SETTINGS, "设置")
        .text(DOCS, "文档")
        .text(LOGS, "查看日志")
        .separator()
        .text(QUIT, "退出")
        .build()?;
    let icon = tauri::include_image!("../../../internal/desktop/icons/jftrade-tray-light.png");
    TrayIconBuilder::with_id("jftrade-main")
        .menu(&menu)
        .icon(icon)
        .icon_as_template(false)
        .tooltip(application_name)
        .show_menu_on_left_click(true)
        .on_menu_event(move |app, event| match event.id().as_ref() {
            OPEN => show_and_focus_main(app),
            SETTINGS => {
                show_and_focus_main(app);
                let _ = app.emit(crate::contract::DESKTOP_MENU_SETTINGS_EVENT, ());
            }
            DOCS => {
                let _ = open_route_window(app, "docs", "JFTrade 文档", "/docs/", 1_120.0, 760.0);
            }
            LOGS => {
                let _ = open_route_window(
                    app,
                    "desktop-logs",
                    "JFTrade 日志",
                    "/desktop-logs",
                    1_040.0,
                    720.0,
                );
            }
            QUIT => {
                quit_requested.store(true, Ordering::SeqCst);
                app.exit(0);
            }
            _ => {}
        })
        .build(app)?;
    Ok(())
}

fn show_and_focus_main(app: &AppHandle<Wry>) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.set_focus();
    }
}
