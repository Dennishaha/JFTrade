use crate::trade_proto::common::ProgramStatusType;
use crate::trade_proto::notify::{
    ApiQuota, ConnectStatus, GtwEvent, GtwEventType, NotifyType, QotRight, Response, UsedQuota,
};
use crate::trade_proto::qot_common::QotRight as QotCommonRight;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct NeutralNotification {
    pub at: String,
    pub level: String,
    pub title: String,
    pub message: String,
    pub source: String,
    pub broker_id: String,
    pub category: String,
}

fn base_futu_notification(category: &str) -> NeutralNotification {
    NeutralNotification {
        at: "2026-09-11T12:00:00.000000000Z".to_string(),
        source: "futu-opend".to_string(),
        broker_id: "futu".to_string(),
        category: category.to_string(),
        level: String::new(),
        title: String::new(),
        message: String::new(),
    }
}

pub fn live_notification_from_response(response: Option<&Response>) -> Option<NeutralNotification> {
    let response = response?;
    if response.ret_type != 0 {
        return None;
    }
    let s2c = response.s2c.as_ref()?;
    let notify_type = s2c.r#type;

    if notify_type == NotifyType::GtwEvent as i32 {
        s2c.event
            .as_ref()
            .and_then(|e| gateway_event_notification(Some(e)))
    } else if notify_type == NotifyType::ProgramStatus as i32 {
        s2c.program_status
            .as_ref()
            .and_then(|p| program_status_notification(Some(&p.program_status)))
    } else if notify_type == NotifyType::ConnStatus as i32 {
        s2c.connect_status
            .as_ref()
            .and_then(|c| connection_status_notification(Some(c)))
    } else if notify_type == NotifyType::QotRight as i32 {
        s2c.qot_right
            .as_ref()
            .and_then(|q| quote_right_notification(Some(q)))
    } else if notify_type == NotifyType::ApiQuota as i32 {
        s2c.api_quota
            .as_ref()
            .and_then(|a| api_quota_notification(Some(a)))
    } else if notify_type == NotifyType::UsedQuota as i32 {
        s2c.used_quota
            .as_ref()
            .and_then(|u| used_quota_notification(Some(u)))
    } else {
        let mut note = base_futu_notification("broker.system");
        note.level = "info".to_string();
        note.title = "OpenD 系统通知".to_string();
        note.message = notify_type_label(notify_type).to_string();
        Some(note)
    }
}

pub fn connection_status_notification(
    status: Option<&ConnectStatus>,
) -> Option<NeutralNotification> {
    let status = status?;
    let mut note = base_futu_notification("broker.connection");
    let qot = status.qot_logined;
    let trd = status.trd_logined;

    if qot && trd {
        note.level = "success".to_string();
        note.title = "OpenD 连接已恢复".to_string();
    } else if !qot && !trd {
        note.level = "error".to_string();
        note.title = "OpenD 连接状态变化".to_string();
    } else {
        note.level = "warn".to_string();
        note.title = "OpenD 连接状态变化".to_string();
    }

    let qot_label = if qot { "已登录" } else { "未登录" };
    let trd_label = if trd { "已登录" } else { "未登录" };
    note.message = format!("行情{}，交易{}。", qot_label, trd_label);
    Some(note)
}

pub fn program_status_notification(
    status: Option<&crate::trade_proto::common::ProgramStatus>,
) -> Option<NeutralNotification> {
    let status = status?;
    let status_type = status.r#type;
    let mut note = base_futu_notification("broker.program");
    note.level = program_status_level(status_type).to_string();
    note.title = program_status_title(status_type).to_string();
    let label = program_status_label(status_type);
    let desc = status.str_ext_desc.as_deref().unwrap_or("").trim();
    if !desc.is_empty() && desc != label {
        note.message = format!("{}：{}", label, desc);
    } else {
        note.message = label.to_string();
    }
    Some(note)
}

pub fn gateway_event_notification(event: Option<&GtwEvent>) -> Option<NeutralNotification> {
    let event = event?;
    let event_type = event.event_type;
    let mut note = base_futu_notification("broker.event");
    note.level = gateway_event_level(event_type).to_string();
    note.title = gateway_event_title(event_type).to_string();
    let label = gateway_event_label(event_type);
    let desc = event.desc.trim();
    if !desc.is_empty() && desc != label {
        note.message = format!("{}：{}", label, desc);
    } else {
        note.message = label.to_string();
    }
    Some(note)
}

pub fn quote_right_notification(right: Option<&QotRight>) -> Option<NeutralNotification> {
    let right = right?;
    let mut note = base_futu_notification("broker.permissions");
    note.level = "info".to_string();
    note.title = "Futu 行情权限更新".to_string();
    let summary = format_qot_right_summary(right);
    note.message = if summary.is_empty() {
        "行情权限已更新。".to_string()
    } else {
        summary
    };
    Some(note)
}

pub fn api_quota_notification(quota: Option<&ApiQuota>) -> Option<NeutralNotification> {
    let quota = quota?;
    let mut note = base_futu_notification("broker.quota");
    note.level = "info".to_string();
    note.title = "Futu API 额度更新".to_string();
    note.message = format!(
        "订阅额度 {}，历史 K 线额度 {}。",
        quota.sub_quota, quota.history_kl_quota
    );
    Some(note)
}

pub fn used_quota_notification(quota: Option<&UsedQuota>) -> Option<NeutralNotification> {
    let quota = quota?;
    let mut note = base_futu_notification("broker.quota");
    note.level = "info".to_string();
    note.title = "Futu API 已使用额度更新".to_string();
    note.message = format!(
        "已使用订阅额度 {}，已使用历史 K 线额度 {}。",
        quota.used_sub_quota.unwrap_or(0),
        quota.used_k_line_quota.unwrap_or(0)
    );
    Some(note)
}

pub fn notify_type_label(value: i32) -> &'static str {
    match value {
        x if x == NotifyType::GtwEvent as i32 => "OpenD 运行事件",
        x if x == NotifyType::ProgramStatus as i32 => "程序状态",
        x if x == NotifyType::ConnStatus as i32 => "连接状态",
        x if x == NotifyType::QotRight as i32 => "行情权限",
        x if x == NotifyType::ApiQuota as i32 => "API 额度",
        x if x == NotifyType::UsedQuota as i32 => "已使用额度",
        _ => "系统通知",
    }
}

pub fn program_status_level(value: i32) -> &'static str {
    match value {
        x if x == ProgramStatusType::Ready as i32 => "success",
        x if x == ProgramStatusType::Loaded as i32
            || x == ProgramStatusType::Loging as i32
            || x == ProgramStatusType::NessaryDataPreparing as i32 =>
        {
            "info"
        }
        x if x == ProgramStatusType::NeedPicVerifyCode as i32
            || x == ProgramStatusType::NeedPhoneVerifyCode as i32 =>
        {
            "warn"
        }
        _ => "error",
    }
}

pub fn program_status_title(value: i32) -> &'static str {
    match value {
        x if x == ProgramStatusType::Ready as i32 => "OpenD 已就绪",
        x if x == ProgramStatusType::LoginFailed as i32 => "OpenD 登录失败",
        x if x == ProgramStatusType::NeedPicVerifyCode as i32 => "OpenD 需要图形验证码",
        x if x == ProgramStatusType::NeedPhoneVerifyCode as i32 => "OpenD 需要手机验证码",
        x if x == ProgramStatusType::ForceUpdate as i32 => "OpenD 需要升级",
        x if x == ProgramStatusType::ForceLogout as i32 => "OpenD 已被强制登出",
        x if x == ProgramStatusType::NessaryDataMissing as i32 => "OpenD 缺少必要数据",
        x if x == ProgramStatusType::UnAgreeDisclaimer as i32 => "OpenD 需要确认免责声明",
        _ => "OpenD 程序状态更新",
    }
}

pub fn program_status_label(value: i32) -> &'static str {
    match value {
        x if x == ProgramStatusType::Loaded as i32 => "已加载",
        x if x == ProgramStatusType::Loging as i32 => "登录中",
        x if x == ProgramStatusType::NeedPicVerifyCode as i32 => "需要图形验证码",
        x if x == ProgramStatusType::NeedPhoneVerifyCode as i32 => "需要手机验证码",
        x if x == ProgramStatusType::LoginFailed as i32 => "登录失败",
        x if x == ProgramStatusType::ForceUpdate as i32 => "需要升级客户端",
        x if x == ProgramStatusType::NessaryDataPreparing as i32 => "正在准备必要数据",
        x if x == ProgramStatusType::NessaryDataMissing as i32 => "缺少必要数据",
        x if x == ProgramStatusType::UnAgreeDisclaimer as i32 => "未同意免责声明",
        x if x == ProgramStatusType::Ready as i32 => "已就绪",
        x if x == ProgramStatusType::ForceLogout as i32 => "已被强制登出",
        x if x == ProgramStatusType::DisclaimerPullFailed as i32 => "拉取免责声明失败",
        _ => "程序状态已更新",
    }
}

pub fn gateway_event_level(value: i32) -> &'static str {
    match value {
        x if x == GtwEventType::None as i32 => "info",
        x if x == GtwEventType::ForceUpdate as i32
            || x == GtwEventType::NeedPicVerifyCode as i32
            || x == GtwEventType::NeedPhoneVerifyCode as i32
            || x == GtwEventType::EnableDeviceLock as i32 =>
        {
            "warn"
        }
        _ => "error",
    }
}

pub fn gateway_event_title(value: i32) -> &'static str {
    match value {
        x if x == GtwEventType::LoginFailed as i32 => "OpenD 登录失败",
        x if x == GtwEventType::ForceUpdate as i32 => "OpenD 需要升级",
        x if x == GtwEventType::KickedOut as i32 => "Futu 账户在别处登录",
        x if x == GtwEventType::NeedPicVerifyCode as i32 => "OpenD 需要图形验证码",
        x if x == GtwEventType::NeedPhoneVerifyCode as i32 => "OpenD 需要手机验证码",
        x if x == GtwEventType::BanLogin as i32 => "Futu 账户被禁止登录",
        _ => "OpenD 运行事件",
    }
}

pub fn gateway_event_label(value: i32) -> &'static str {
    match value {
        x if x == GtwEventType::None as i32 => "无异常",
        x if x == GtwEventType::LocalCfgLoadFailed as i32 => "加载本地配置失败",
        x if x == GtwEventType::ApiSvrRunFailed as i32 => "OpenD 服务启动失败",
        x if x == GtwEventType::ForceUpdate as i32 => "客户端版本过低",
        x if x == GtwEventType::LoginFailed as i32 => "登录失败",
        x if x == GtwEventType::UnAgreeDisclaimer as i32 => "未同意免责声明",
        x if x == GtwEventType::NetCfgMissing as i32 => "缺少必要网络配置",
        x if x == GtwEventType::KickedOut as i32 => "账户在别处登录",
        x if x == GtwEventType::LoginPwdChanged as i32 => "登录密码已修改",
        x if x == GtwEventType::BanLogin as i32 => "用户被禁止登录",
        x if x == GtwEventType::NeedPicVerifyCode as i32 => "需要图形验证码",
        x if x == GtwEventType::NeedPhoneVerifyCode as i32 => "需要手机验证码",
        x if x == GtwEventType::AppDataNotExist as i32 => "程序自带数据不存在",
        x if x == GtwEventType::NessaryDataMissing as i32 => "缺少必要数据",
        x if x == GtwEventType::TradePwdChanged as i32 => "交易密码已修改",
        x if x == GtwEventType::EnableDeviceLock as i32 => "已启用设备锁",
        _ => "运行事件已更新",
    }
}

pub fn quote_right_label(value: i32) -> &'static str {
    match value {
        x if x == QotCommonRight::Bmp as i32 => "BMP",
        x if x == QotCommonRight::Level1 as i32 => "Level 1",
        x if x == QotCommonRight::Level2 as i32 => "Level 2",
        x if x == QotCommonRight::Level3 as i32 => "Level 3",
        x if x == QotCommonRight::Sf as i32 => "高级行情",
        x if x == QotCommonRight::No as i32 => "无权限",
        _ => "未知",
    }
}

fn format_qot_right_summary(right: &QotRight) -> String {
    let mut parts = Vec::with_capacity(8);
    let mut append = |label: &str, val: Option<i32>| {
        if let Some(v) = val {
            parts.push(format!("{} {}", label, quote_right_label(v)));
        }
    };
    append("HK", Some(right.hk_qot_right));
    append("HK Option", right.hk_option_qot_right);
    append("HK Future", right.hk_future_qot_right);
    append("US", Some(right.us_qot_right));
    append("US Option", right.us_option_qot_right);
    append("CN", Some(right.cn_qot_right));
    append("US Index", right.us_index_qot_right);
    append("US OTC", right.us_otc_qot_right);
    append("SG Future", right.sg_future_qot_right);
    append("JP Future", right.jp_future_qot_right);
    append("CME", right.us_cme_future_qot_right);
    append("CBOT", right.us_cbot_future_qot_right);
    append("NYMEX", right.us_nymex_future_qot_right);
    append("COMEX", right.us_comex_future_qot_right);
    append("CBOE", right.us_cboe_future_qot_right);
    append("SH", right.sh_qot_right);
    append("SZ", right.sz_qot_right);
    append("Crypto", right.cc_qot_right);
    append("SG", right.sg_stock_qot_right);
    append("MY", right.my_stock_qot_right);
    append("JP", right.jp_stock_qot_right);
    parts.join("；")
}
