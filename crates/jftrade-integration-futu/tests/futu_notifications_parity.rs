use jftrade_integration_futu::notification::*;
use jftrade_integration_futu::trade_proto::common::{ProgramStatus, ProgramStatusType};
use jftrade_integration_futu::trade_proto::notify::{
    ApiQuota, ConnectStatus, GtwEvent, GtwEventType, NotifyType, QotRight, Response, S2c, UsedQuota,
};
use jftrade_integration_futu::trade_proto::qot_common::QotRight as QotCommonRight;

// Parity: go:452dea11:internal/app/apiserver/tradingapp/notifications_lifecycle_test.go:29 TestOrderLifecycleNotificationMapsSubmittedCancelledAndFilled
#[test]
fn test_neutral_notification_builders_handle_nil_and_status_transitions() {
    // Parity: internal/integration/futu/notifications_test.go:13 TestNeutralNotificationBuildersHandleNilAndStatusTransitions
    assert!(connection_status_notification(None).is_none());
    assert!(program_status_notification(None).is_none());
    assert!(gateway_event_notification(None).is_none());
    assert!(quote_right_notification(None).is_none());
    assert!(api_quota_notification(None).is_none());
    assert!(used_quota_notification(None).is_none());

    // Both logined => success
    let conn_ok = connection_status_notification(Some(&ConnectStatus {
        qot_logined: true,
        trd_logined: true,
    }))
    .expect("should produce notification");
    assert_eq!(conn_ok.level, "success");
    assert_eq!(conn_ok.category, "broker.connection");
    assert_eq!(conn_ok.title, "OpenD 连接已恢复");
    assert_eq!(conn_ok.message, "行情已登录，交易已登录。");

    // Partial login => warn
    let conn_warn = connection_status_notification(Some(&ConnectStatus {
        qot_logined: true,
        trd_logined: false,
    }))
    .expect("should produce notification");
    assert_eq!(conn_warn.level, "warn");
    assert_eq!(conn_warn.title, "OpenD 连接状态变化");
    assert_eq!(conn_warn.message, "行情已登录，交易未登录。");

    // Neither logined => error
    let conn_err = connection_status_notification(Some(&ConnectStatus {
        qot_logined: false,
        trd_logined: false,
    }))
    .expect("should produce notification");
    assert_eq!(conn_err.level, "error");
    assert_eq!(conn_err.title, "OpenD 连接状态变化");
    assert_eq!(conn_err.message, "行情未登录，交易未登录。");

    // Program status: Ready => success
    let prog_ready = program_status_notification(Some(&ProgramStatus {
        r#type: ProgramStatusType::Ready as i32,
        str_ext_desc: None,
    }))
    .expect("should produce notification");
    assert_eq!(prog_ready.level, "success");
    assert_eq!(prog_ready.title, "OpenD 已就绪");
    assert_eq!(prog_ready.message, "已就绪");

    // Program status: NeedPhoneVerifyCode => warn with ext desc
    let prog_code = program_status_notification(Some(&ProgramStatus {
        r#type: ProgramStatusType::NeedPhoneVerifyCode as i32,
        str_ext_desc: Some("输入短信验证码".to_string()),
    }))
    .expect("should produce notification");
    assert_eq!(prog_code.level, "warn");
    assert_eq!(prog_code.title, "OpenD 需要手机验证码");
    assert_eq!(prog_code.message, "需要手机验证码：输入短信验证码");

    // Gateway event: KickedOut => error
    let gtw_kick = gateway_event_notification(Some(&GtwEvent {
        event_type: GtwEventType::KickedOut as i32,
        desc: "已在其它设备登录".to_string(),
    }))
    .expect("should produce notification");
    assert_eq!(gtw_kick.level, "error");
    assert_eq!(gtw_kick.title, "Futu 账户在别处登录");
    assert_eq!(gtw_kick.message, "账户在别处登录：已在其它设备登录");
}

// Parity: go:452dea11:internal/app/apiserver/tradingapp/notifications_test.go:10 TestOrderLifecycleNotificationHandlesUnrelatedAndPartialFillEvents
#[test]
fn test_live_notification_from_response_routes_protocol_payloads_to_neutral_categories() {
    // Parity: internal/integration/futu/notifications_test.go:73 TestLiveNotificationFromResponseRoutesProtocolPayloadsToNeutralCategories
    assert!(live_notification_from_response(None).is_none());

    // RetType non-zero => None
    let failed_resp = Response {
        ret_type: -1,
        ret_msg: Some("fail".to_string()),
        err_code: Some(100),
        s2c: None,
    };
    assert!(live_notification_from_response(Some(&failed_resp)).is_none());

    // ConnectStatus response
    let conn_resp = Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: Some(S2c {
            r#type: NotifyType::ConnStatus as i32,
            event: None,
            program_status: None,
            connect_status: Some(ConnectStatus {
                qot_logined: true,
                trd_logined: true,
            }),
            qot_right: None,
            api_quota: None,
            used_quota: None,
            api_level: None,
        }),
    };
    let note = live_notification_from_response(Some(&conn_resp)).expect("notification mapped");
    assert_eq!(note.category, "broker.connection");
    assert_eq!(note.level, "success");

    // QuoteRight response
    let qot_resp = Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: Some(S2c {
            r#type: NotifyType::QotRight as i32,
            event: None,
            program_status: None,
            connect_status: None,
            qot_right: Some(QotRight {
                hk_qot_right: QotCommonRight::Level2 as i32,
                us_qot_right: QotCommonRight::Level1 as i32,
                cn_qot_right: QotCommonRight::No as i32,
                hk_option_qot_right: None,
                has_us_option_qot_right: None,
                hk_future_qot_right: None,
                us_future_qot_right: None,
                us_option_qot_right: None,
                us_index_qot_right: None,
                us_otc_qot_right: None,
                sg_future_qot_right: None,
                jp_future_qot_right: None,
                us_cme_future_qot_right: None,
                us_cbot_future_qot_right: None,
                us_nymex_future_qot_right: None,
                us_comex_future_qot_right: None,
                us_cboe_future_qot_right: None,
                sh_qot_right: None,
                sz_qot_right: None,
                cc_qot_right: None,
                sg_stock_qot_right: None,
                my_stock_qot_right: None,
                jp_stock_qot_right: None,
                ec_qot_right: None,
            }),
            api_quota: None,
            used_quota: None,
            api_level: None,
        }),
    };
    let qot_note = live_notification_from_response(Some(&qot_resp)).expect("notification mapped");
    assert_eq!(qot_note.category, "broker.permissions");
    assert_eq!(qot_note.level, "info");
    assert!(qot_note.message.contains("HK Level 2"));
    assert!(qot_note.message.contains("US Level 1"));

    // ApiQuota response
    let quota_resp = Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: Some(S2c {
            r#type: NotifyType::ApiQuota as i32,
            event: None,
            program_status: None,
            connect_status: None,
            qot_right: None,
            api_quota: Some(ApiQuota {
                sub_quota: 50,
                history_kl_quota: 100,
            }),
            used_quota: None,
            api_level: None,
        }),
    };
    let quota_note = live_notification_from_response(Some(&quota_resp)).expect("quota mapped");
    assert_eq!(quota_note.category, "broker.quota");
    assert_eq!(quota_note.message, "订阅额度 50，历史 K 线额度 100。");

    // UsedQuota response
    let used_resp = Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: Some(S2c {
            r#type: NotifyType::UsedQuota as i32,
            event: None,
            program_status: None,
            connect_status: None,
            qot_right: None,
            api_quota: None,
            used_quota: Some(UsedQuota {
                used_sub_quota: Some(10),
                used_k_line_quota: Some(20),
            }),
            api_level: None,
        }),
    };
    let used_note = live_notification_from_response(Some(&used_resp)).expect("used quota mapped");
    assert_eq!(used_note.category, "broker.quota");
    assert_eq!(
        used_note.message,
        "已使用订阅额度 10，已使用历史 K 线额度 20。"
    );
}

// Parity: go:452dea11:internal/app/apiserver/tradingapp/notifications_lifecycle_test.go:10 TestOrderPlacedNotificationMapsBrokerLabelAndMessage
/// The table below freezes level/title/label for every program-status state the
/// Go baseline enumerates, including the unknown fallback.
#[test]
fn test_notification_labels_cover_every_supported_program_and_gateway_state() {
    // Parity: internal/integration/futu/notifications_test.go:135 TestNotificationLabelsCoverEverySupportedProgramAndGatewayState
    const PROGRAM_STATES: &[(i32, &str, &str, &str)] = &[
        (
            ProgramStatusType::Loaded as i32,
            "info",
            "OpenD 程序状态更新",
            "已加载",
        ),
        (
            ProgramStatusType::Loging as i32,
            "info",
            "OpenD 程序状态更新",
            "登录中",
        ),
        (
            ProgramStatusType::NeedPicVerifyCode as i32,
            "warn",
            "OpenD 需要图形验证码",
            "需要图形验证码",
        ),
        (
            ProgramStatusType::NeedPhoneVerifyCode as i32,
            "warn",
            "OpenD 需要手机验证码",
            "需要手机验证码",
        ),
        (
            ProgramStatusType::LoginFailed as i32,
            "error",
            "OpenD 登录失败",
            "登录失败",
        ),
        (
            ProgramStatusType::ForceUpdate as i32,
            "error",
            "OpenD 需要升级",
            "需要升级客户端",
        ),
        (
            ProgramStatusType::NessaryDataPreparing as i32,
            "info",
            "OpenD 程序状态更新",
            "正在准备必要数据",
        ),
        (
            ProgramStatusType::NessaryDataMissing as i32,
            "error",
            "OpenD 缺少必要数据",
            "缺少必要数据",
        ),
        (
            ProgramStatusType::UnAgreeDisclaimer as i32,
            "error",
            "OpenD 需要确认免责声明",
            "未同意免责声明",
        ),
        (
            ProgramStatusType::Ready as i32,
            "success",
            "OpenD 已就绪",
            "已就绪",
        ),
        (
            ProgramStatusType::ForceLogout as i32,
            "error",
            "OpenD 已被强制登出",
            "已被强制登出",
        ),
        (
            ProgramStatusType::DisclaimerPullFailed as i32,
            "error",
            "OpenD 程序状态更新",
            "拉取免责声明失败",
        ),
        (9999, "error", "OpenD 程序状态更新", "程序状态已更新"),
    ];
    for (value, level, title, label) in PROGRAM_STATES {
        assert_eq!(program_status_level(*value), *level, "level {value}");
        assert_eq!(program_status_title(*value), *title, "title {value}");
        assert_eq!(program_status_label(*value), *label, "label {value}");
    }

    const GATEWAY_EVENTS: &[(i32, &str, &str, &str)] = &[
        (
            GtwEventType::None as i32,
            "info",
            "OpenD 运行事件",
            "无异常",
        ),
        (
            GtwEventType::LocalCfgLoadFailed as i32,
            "error",
            "OpenD 运行事件",
            "加载本地配置失败",
        ),
        (
            GtwEventType::ApiSvrRunFailed as i32,
            "error",
            "OpenD 运行事件",
            "OpenD 服务启动失败",
        ),
        (
            GtwEventType::ForceUpdate as i32,
            "warn",
            "OpenD 需要升级",
            "客户端版本过低",
        ),
        (
            GtwEventType::LoginFailed as i32,
            "error",
            "OpenD 登录失败",
            "登录失败",
        ),
        (
            GtwEventType::UnAgreeDisclaimer as i32,
            "error",
            "OpenD 运行事件",
            "未同意免责声明",
        ),
        (
            GtwEventType::NetCfgMissing as i32,
            "error",
            "OpenD 运行事件",
            "缺少必要网络配置",
        ),
        (
            GtwEventType::KickedOut as i32,
            "error",
            "Futu 账户在别处登录",
            "账户在别处登录",
        ),
        (
            GtwEventType::LoginPwdChanged as i32,
            "error",
            "OpenD 运行事件",
            "登录密码已修改",
        ),
        (
            GtwEventType::BanLogin as i32,
            "error",
            "Futu 账户被禁止登录",
            "用户被禁止登录",
        ),
        (
            GtwEventType::NeedPicVerifyCode as i32,
            "warn",
            "OpenD 需要图形验证码",
            "需要图形验证码",
        ),
        (
            GtwEventType::NeedPhoneVerifyCode as i32,
            "warn",
            "OpenD 需要手机验证码",
            "需要手机验证码",
        ),
        (
            GtwEventType::AppDataNotExist as i32,
            "error",
            "OpenD 运行事件",
            "程序自带数据不存在",
        ),
        (
            GtwEventType::NessaryDataMissing as i32,
            "error",
            "OpenD 运行事件",
            "缺少必要数据",
        ),
        (
            GtwEventType::TradePwdChanged as i32,
            "error",
            "OpenD 运行事件",
            "交易密码已修改",
        ),
        (
            GtwEventType::EnableDeviceLock as i32,
            "warn",
            "OpenD 运行事件",
            "已启用设备锁",
        ),
        (9999, "error", "OpenD 运行事件", "运行事件已更新"),
    ];
    for (value, level, title, label) in GATEWAY_EVENTS {
        assert_eq!(gateway_event_level(*value), *level, "level {value}");
        assert_eq!(gateway_event_title(*value), *title, "title {value}");
        assert_eq!(gateway_event_label(*value), *label, "label {value}");
    }
}

// Parity: go:452dea11:internal/app/apiserver/tradingapp/notifications_lifecycle_test.go:62 TestExecutionOrderNotificationMessageOmitsBlankParts
/// Notification-kind and quote-right labels are part of the neutral wire
/// contract consumed by the console, so they must stay stable.
#[test]
fn test_notification_and_quote_right_labels_remain_stable() {
    // Parity: internal/integration/futu/notifications_test.go:207 TestNotificationAndQuoteRightLabelsRemainStable
    const NOTIFY_TYPES: &[(i32, &str)] = &[
        (NotifyType::GtwEvent as i32, "OpenD 运行事件"),
        (NotifyType::ProgramStatus as i32, "程序状态"),
        (NotifyType::ConnStatus as i32, "连接状态"),
        (NotifyType::QotRight as i32, "行情权限"),
        (NotifyType::ApiQuota as i32, "API 额度"),
        (NotifyType::UsedQuota as i32, "已使用额度"),
        (9999, "系统通知"),
    ];
    for (value, label) in NOTIFY_TYPES {
        assert_eq!(notify_type_label(*value), *label, "notify type {value}");
    }

    const QOT_RIGHTS: &[(i32, &str)] = &[
        (QotCommonRight::Bmp as i32, "BMP"),
        (QotCommonRight::Level1 as i32, "Level 1"),
        (QotCommonRight::Level2 as i32, "Level 2"),
        (QotCommonRight::Level3 as i32, "Level 3"),
        (QotCommonRight::Sf as i32, "高级行情"),
        (QotCommonRight::No as i32, "无权限"),
        (QotCommonRight::Unknow as i32, "未知"),
    ];
    for (value, label) in QOT_RIGHTS {
        assert_eq!(quote_right_label(*value), *label, "quote right {value}");
    }
}
