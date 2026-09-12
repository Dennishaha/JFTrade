use jftrade_integration_futu::notification::*;
use jftrade_integration_futu::trade_proto::common::{ProgramStatus, ProgramStatusType};
use jftrade_integration_futu::trade_proto::notify::{
    ApiQuota, ConnectStatus, GtwEvent, GtwEventType, NotifyType, QotRight, Response, S2c, UsedQuota,
};
use jftrade_integration_futu::trade_proto::qot_common::QotRight as QotCommonRight;

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
